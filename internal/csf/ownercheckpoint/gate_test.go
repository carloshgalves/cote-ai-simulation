package ownercheckpoint

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

type testDurableOwner struct {
	mu              sync.Mutex
	key             ed25519.PrivateKey
	durable         map[string][]byte
	persisted       int
	restored        int
	acked           int
	failPersist     bool
	failPostDurable bool
	failRestore     bool
	failAck         bool
	badSignature    bool
	badLocator      bool
	mutateRestore   bool
}

func (o *testDurableOwner) PersistAndSign(_ context.Context, req PersistRequest) (SignedReceipt, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.failPersist {
		return SignedReceipt{}, errors.New("crash before external persist")
	}
	locator := "owner-cut-01"
	raw := bytes.Clone(req.Checkpoint.Content)
	if prev, ok := o.durable[locator]; ok && !bytes.Equal(prev, raw) {
		return SignedReceipt{}, errors.New("conflicting durable owner state")
	}
	o.durable[locator] = raw
	o.persisted++
	if o.failPostDurable {
		return SignedReceipt{}, errors.New("crash after durable flush before signature")
	}
	signed, err := sqlite.CognitiveCheckpointSigningBytesV1(req.Scope.Run, req.Checkpoint, locator)
	if err != nil {
		return SignedReceipt{}, err
	}
	signature := ed25519.Sign(o.key, signed)
	if o.badSignature {
		signature[0] ^= 0x40
	}
	if o.badLocator {
		locator = "different-locator"
	}
	return SignedReceipt{Locator: locator, Signature: signature}, nil
}

func (o *testDurableOwner) Restore(_ context.Context, req RestoreRequest) (RestoredContent, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.failRestore {
		return RestoredContent{}, errors.New("provider offline during restore")
	}
	value, ok := o.durable[req.Locator]
	if !ok {
		return RestoredContent{}, errors.New("owner durable locator missing")
	}
	o.restored++
	raw := bytes.Clone(value)
	if o.mutateRestore {
		raw[0] ^= 1
	}
	return RestoredContent{Locator: req.Locator, Content: raw}, nil
}
func (o *testDurableOwner) Acknowledge(_ context.Context, _ RestoreAck) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.failAck {
		return errors.New("ACK lost before durable persistence")
	}
	o.acked++
	return nil
}

type testEscrowRecord struct {
	scope     Scope
	cp        sqlite.SnapshotCheckpoint
	locator   string
	signature []byte
}
type testEscrow struct {
	mu                sync.Mutex
	public            ed25519.PublicKey
	records           map[[32]byte]testEscrowRecord
	snapshotID        [32]byte
	snapshotCanonical []byte
	snapshotPublished bool
	tailCommits       uint64
	failPersist       bool
	failLoad          bool
}

func (e *testEscrow) PersistCognitiveCheckpointV1(_ context.Context, run [32]byte,
	cp sqlite.SnapshotCheckpoint, locator string, signature []byte, _ *policy.Bundle) ([32]byte, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	var zero [32]byte
	if e.failPersist {
		return zero, errors.New("crash before CSF escrow")
	}
	payload, err := sqlite.CognitiveCheckpointSigningBytesV1(run, cp, locator)
	if err != nil {
		return zero, err
	}
	if !ed25519.Verify(e.public, payload, signature) {
		return zero, errors.New("signature invalid")
	}
	id := identity.OpaqueContentHash(payload)
	scope := Scope{Run: run, Namespace: cp.ActorNamespace, Actor: cp.ActorID}
	for _, record := range e.records {
		if record.scope == scope && record.cp.CoversInstant == cp.CoversInstant &&
			record.cp.CoversRevision == cp.CoversRevision &&
			record.cp.CoversEvidenceCursor == cp.CoversEvidenceCursor &&
			!bytes.Equal(record.cp.Content, cp.Content) {
			return zero, errors.New("conflicting signed owner state at same cut")
		}
	}
	if existing, ok := e.records[id]; ok {
		if !bytes.Equal(existing.cp.Content, cp.Content) ||
			!bytes.Equal(existing.signature, signature) {
			return zero, errors.New("escrow collision")
		}
		return id, nil
	}
	e.records[id] = testEscrowRecord{scope, cloneCheckpoint(cp), locator, bytes.Clone(signature)}
	return id, nil
}
func (e *testEscrow) LoadCognitiveCheckpointV1(_ context.Context, run, receiptID [32]byte,
	namespace, actor string, _ *policy.Bundle) (sqlite.SnapshotCheckpoint, string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	var zero sqlite.SnapshotCheckpoint
	if e.failLoad {
		return zero, "", errors.New("escrow unavailable")
	}
	record, ok := e.records[receiptID]
	if !ok || record.scope != (Scope{Run: run, Namespace: namespace, Actor: actor}) {
		return zero, "", errors.New("receipt not addressed to this actor and run")
	}
	payload, err := sqlite.CognitiveCheckpointSigningBytesV1(run, record.cp, record.locator)
	if err != nil {
		return zero, "", err
	}
	if !ed25519.Verify(e.public, payload, record.signature) ||
		identity.OpaqueContentHash(payload) != receiptID {
		return zero, "", errors.New("stored receipt signature invalid")
	}
	return cloneCheckpoint(record.cp), record.locator, nil
}
func (e *testEscrow) VerifySnapshotV1(_ context.Context, _ [32]byte, snapshot [32]byte,
	_ *policy.Bundle) (sqlite.ResumeCheckpointV1, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.snapshotPublished || snapshot != e.snapshotID {
		return sqlite.ResumeCheckpointV1{}, errors.New("sealed snapshot absent")
	}
	return sqlite.ResumeCheckpointV1{
		Snapshot:    sqlite.SnapshotV1{ID: e.snapshotID, Canonical: bytes.Clone(e.snapshotCanonical)},
		TailCommits: e.tailCommits,
	}, nil
}

type testSetup struct {
	gate     *Gate
	owner    *testDurableOwner
	escrow   *testEscrow
	scope    Scope
	cp       sqlite.SnapshotCheckpoint
	snapshot [32]byte
}

func setupOwnerGate(t *testing.T) testSetup {
	t.Helper()
	pub, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	run := [32]byte{0x14}
	scope := Scope{Run: run, Namespace: "cognition", Actor: "agent"}
	cp := sqlite.SnapshotCheckpoint{
		ActorNamespace: scope.Namespace, ActorID: scope.Actor,
		StoreID: "memory-v1", SchemaVersion: 1,
		Content:  []byte("private opaque checkpoint content"),
		Producer: "memory-owner", ProducerVersion: 1,
		CoversInstant: 40, CoversRevision: 0, CoversEvidenceCursor: 0,
	}
	snapshot := [32]byte{0x25}
	owner := &testDurableOwner{key: private, durable: map[string][]byte{}}
	escrow := &testEscrow{public: pub, records: map[[32]byte]testEscrowRecord{}, snapshotID: snapshot}
	gate, err := NewGate(owner, escrow, &policy.Bundle{})
	if err != nil {
		t.Fatal(err)
	}
	return testSetup{gate, owner, escrow, scope, cp, snapshot}
}
func (s testSetup) publishSnapshot(t *testing.T, cp sqlite.SnapshotCheckpoint) {
	t.Helper()
	digest := identity.OpaqueContentHash(cp.Content)
	ref := []any{
		[]any{cp.ActorNamespace, cp.ActorID}, cp.StoreID, uint64(cp.SchemaVersion),
		[]any{uint64(1), bytes.Clone(cp.Content)}, digest[:],
		cp.Producer, uint64(cp.ProducerVersion),
		[]any{cp.CoversInstant, cp.CoversRevision, cp.CoversEvidenceCursor},
	}
	fields := make([]any, 19)
	fields[0] = s.scope.Run[:]
	fields[14] = []any{ref}
	raw, err := codec.CanonicalPayload(fields)
	if err != nil {
		t.Fatal(err)
	}
	s.escrow.mu.Lock()
	defer s.escrow.mu.Unlock()
	s.escrow.snapshotCanonical = raw
	s.escrow.snapshotPublished = true
}

func testStage(t *testing.T, s testSetup) [32]byte {
	t.Helper()
	id, err := s.gate.Stage(context.Background(), s.scope, s.cp)
	if err != nil {
		t.Fatal(err)
	}
	return id
}
func assertNotPermitted(t *testing.T, s testSetup, receipt [32]byte) {
	t.Helper()
	calls := 0
	err := s.gate.RunWork(context.Background(), s.scope, s.snapshot, receipt, func(context.Context) error {
		calls++
		return nil
	})
	if !errors.Is(err, ErrNotAcknowledged) || calls != 0 {
		t.Fatalf("work bypassed restore ACK: calls=%d err=%v", calls, err)
	}
}

func TestOwner58S0PersistReceiptEscrowSnapshotRestoreACKWork(t *testing.T) {
	s := setupOwnerGate(t)
	ctx := context.Background()
	assertNotPermitted(t, s, [32]byte{0x55})
	receipt := testStage(t, s)
	assertNotPermitted(t, s, receipt)
	if err := s.gate.RestoreAndAcknowledge(ctx, s.scope, s.snapshot, receipt); err == nil {
		t.Fatal("owner restored without a published sealed snapshot")
	}
	assertNotPermitted(t, s, receipt)
	s.publishSnapshot(t, s.cp)
	if err := s.gate.RestoreAndAcknowledge(ctx, s.scope, s.snapshot, receipt); err != nil {
		t.Fatal(err)
	}
	calls := 0
	if err := s.gate.RunWork(ctx, s.scope, s.snapshot, receipt, func(context.Context) error {
		calls++
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if calls != 1 || s.owner.acked != 1 {
		t.Fatalf("WORK not gated by durable restore ACK: calls=%d ACKs=%d", calls, s.owner.acked)
	}
	wrong := s.scope
	wrong.Run[0] ^= 0xa5
	if err := s.gate.RunWork(ctx, wrong, s.snapshot, receipt, func(context.Context) error {
		calls++
		return nil
	}); !errors.Is(err, ErrNotAcknowledged) {
		t.Fatal("foreign run used a restore permit:", err)
	}
	wrong = s.scope
	wrong.Actor = "other-agent"
	if err := s.gate.RunWork(ctx, wrong, s.snapshot, receipt, func(context.Context) error {
		calls++
		return nil
	}); !errors.Is(err, ErrNotAcknowledged) {
		t.Fatal("foreign actor used a restore permit:", err)
	}
	if err := s.gate.RunWork(ctx, s.scope, [32]byte{0x90}, receipt, func(context.Context) error {
		calls++
		return nil
	}); !errors.Is(err, ErrNotAcknowledged) {
		t.Fatal("different snapshot inherited owner permit:", err)
	}
	if err := s.gate.RunWork(ctx, s.scope, s.snapshot, [32]byte{0x91}, func(context.Context) error {
		calls++
		return nil
	}); !errors.Is(err, ErrNotAcknowledged) {
		t.Fatal("different receipt inherited owner permit:", err)
	}
	if calls != 1 {
		t.Fatal("unauthorized model invocation executed")
	}
	again := testStage(t, s)
	if again != receipt {
		t.Fatal("identical durable retry changed signed receipt identity")
	}
	assertNotPermitted(t, s, receipt)
	if err := s.gate.RestoreAndAcknowledge(ctx, s.scope, s.snapshot, receipt); err != nil {
		t.Fatal(err)
	}
	if err := s.gate.RunWork(ctx, s.scope, s.snapshot, receipt, func(context.Context) error {
		calls++
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatal("valid repeated restore failed to authorize exactly one callback")
	}
}

func TestOwner58S0FailureMatrixNeverInvokesModel(t *testing.T) {
	for _, fault := range []string{
		"before-persist", "after-durable-before-sign", "forged-signature",
		"wrong-locator", "escrow-unavailable", "restore-unavailable",
		"escrow-load-unavailable", "restored-bytes-tampered", "lost-ACK",
	} {
		t.Run(fault, func(t *testing.T) {
			s := setupOwnerGate(t)
			ctx := context.Background()
			s.publishSnapshot(t, s.cp)
			switch fault {
			case "before-persist":
				s.owner.failPersist = true
			case "after-durable-before-sign":
				s.owner.failPostDurable = true
			case "forged-signature":
				s.owner.badSignature = true
			case "wrong-locator":
				s.owner.badLocator = true
			case "escrow-unavailable":
				s.escrow.failPersist = true
			}
			receipt, stageErr := s.gate.Stage(ctx, s.scope, s.cp)
			if fault == "before-persist" || fault == "after-durable-before-sign" ||
				fault == "forged-signature" || fault == "wrong-locator" ||
				fault == "escrow-unavailable" {
				if stageErr == nil {
					t.Fatal("stage accepted provider/escrow failure")
				}
				assertNotPermitted(t, s, receipt)
				return
			}
			if stageErr != nil {
				t.Fatal(stageErr)
			}
			switch fault {
			case "restore-unavailable":
				s.owner.failRestore = true
			case "escrow-load-unavailable":
				s.escrow.failLoad = true
			case "restored-bytes-tampered":
				s.owner.mutateRestore = true
			case "lost-ACK":
				s.owner.failAck = true
			}
			if err := s.gate.RestoreAndAcknowledge(ctx, s.scope, s.snapshot, receipt); err == nil {
				t.Fatal("external checkpoint failure produced a ready permit")
			}
			assertNotPermitted(t, s, receipt)
			// A healthy retry must restore the SAME bytes without republishing
			// owner state or invoking a model during recovery.
			s.owner.failRestore = false
			s.owner.mutateRestore = false
			s.owner.failAck = false
			s.escrow.failLoad = false
			if err := s.gate.RestoreAndAcknowledge(ctx, s.scope, s.snapshot, receipt); err != nil {
				t.Fatal("same-receipt restore after fault:", err)
			}
			if s.owner.persisted != 1 {
				t.Fatal("restore retried external persistence instead of rehydration")
			}
		})
	}
}

func TestOwner58S0CrashAfterEscrowRetryNoModel(t *testing.T) {
	s := setupOwnerGate(t)
	ctx := context.Background()
	receipt := testStage(t, s)
	assertNotPermitted(t, s, receipt)
	// Crash boundary after signed SQLite escrow but before snapshot commit:
	// no owner callback is performed, no external re-sign is needed.
	s.escrow.snapshotPublished = false
	if err := s.gate.RestoreAndAcknowledge(ctx, s.scope, s.snapshot, receipt); err == nil {
		t.Fatal("restored before snapshot seal")
	}
	s.publishSnapshot(t, s.cp)
	if err := s.gate.RestoreAndAcknowledge(ctx, s.scope, s.snapshot, receipt); err != nil {
		t.Fatal("could not recover signed escrow after snapshot publication:", err)
	}
	if s.owner.persisted != 1 {
		t.Fatal("crash recovery consulted persist again")
	}
	before := s.cp.Content[0]
	s.owner.durable["owner-cut-01"][0] ^= 0xff
	if err := s.gate.RestoreAndAcknowledge(ctx, s.scope, s.snapshot, receipt); err == nil {
		t.Fatal("durable owner bytes swapped after signed escrow were accepted")
	}
	assertNotPermitted(t, s, receipt)
	s.owner.durable["owner-cut-01"][0] = before
	if err := s.gate.RestoreAndAcknowledge(ctx, s.scope, s.snapshot, receipt); err != nil {
		t.Fatal("exact persisted bytes were not restorable:", err)
	}
}

func TestOwner58S0StageRevokesPermitBeforeNewCheckpoint(t *testing.T) {
	s := setupOwnerGate(t)
	ctx := context.Background()
	r := testStage(t, s)
	s.publishSnapshot(t, s.cp)
	if err := s.gate.RestoreAndAcknowledge(ctx, s.scope, s.snapshot, r); err != nil {
		t.Fatal(err)
	}
	modified := cloneCheckpoint(s.cp)
	modified.CoversRevision = 1
	modified.Content = []byte("different checkpoint")
	// Even if the owner refuses a competing persist, the old permit must
	// be revoked before performing any external work.
	if _, err := s.gate.Stage(ctx, s.scope, modified); err == nil {
		t.Fatal("same locator reused with conflicting private bytes")
	}
	assertNotPermitted(t, s, r)
}

func TestOwner58S0SnapshotMustContainExactSignedActorCheckpoint(t *testing.T) {
	for _, variant := range []string{"absent", "other-actor", "different-bytes", "different-coverage", "duplicated"} {
		t.Run(variant, func(t *testing.T) {
			s := setupOwnerGate(t)
			receipt := testStage(t, s)
			ctx := context.Background()
			s.publishSnapshot(t, s.cp)
			raw, err := codec.StrictDecodePayload(s.escrow.snapshotCanonical)
			if err != nil {
				t.Fatal(err)
			}
			fields := raw.([]any)
			ref := fields[14].([]any)[0].([]any)
			switch variant {
			case "absent":
				fields[14] = []any{}
			case "other-actor":
				name := append([]any(nil), ref[0].([]any)...)
				name[1] = "other-agent"
				ref[0] = name
			case "different-bytes":
				typed := append([]any(nil), ref[3].([]any)...)
				typed[1] = []byte("substituted but structurally valid")
				ref[3] = typed
			case "different-coverage":
				cover := append([]any(nil), ref[7].([]any)...)
				cover[1] = uint64(1)
				ref[7] = cover
			case "duplicated":
				fields[14] = []any{ref, ref}
			}
			bad, err := codec.CanonicalPayload(fields)
			if err != nil {
				t.Fatal(err)
			}
			s.escrow.snapshotCanonical = bad
			if err := s.gate.RestoreAndAcknowledge(ctx, s.scope, s.snapshot, receipt); err == nil {
				t.Fatal("individually valid receipt authorized under unrelated snapshot roster")
			}
			assertNotPermitted(t, s, receipt)
			if s.owner.restored != 0 || s.owner.acked != 0 {
				t.Fatal("external owner was contacted before snapshot binding passed")
			}
			s.publishSnapshot(t, s.cp)
			if err := s.gate.RestoreAndAcknowledge(ctx, s.scope, s.snapshot, receipt); err != nil {
				t.Fatal("exact signed checkpoint was not restorable:", err)
			}
		})
	}
}

// A delayed owner persist must not publish a superseded receipt after a
// newer actor-scoped stage has already advanced the local generation.
type blockingFirstOwner struct {
	DurableOwner
	once    sync.Once
	started chan struct{}
	release chan struct{}
}

func (o *blockingFirstOwner) PersistAndSign(ctx context.Context, req PersistRequest) (SignedReceipt, error) {
	block := false
	o.once.Do(func() { block = true; close(o.started) })
	if block {
		select {
		case <-o.release:
		case <-ctx.Done():
			return SignedReceipt{}, ctx.Err()
		}
	}
	return o.DurableOwner.PersistAndSign(ctx, req)
}

func TestOwner58S0SupersededPersistCannotOverwriteNewerGeneration(t *testing.T) {
	s := setupOwnerGate(t)
	owner := &blockingFirstOwner{
		DurableOwner: s.owner, started: make(chan struct{}), release: make(chan struct{}),
	}
	gate, err := NewGate(owner, s.escrow, &policy.Bundle{})
	if err != nil {
		t.Fatal(err)
	}
	s.gate = gate
	ctx := context.Background()
	first := make(chan error, 1)
	go func() {
		_, err := gate.Stage(ctx, s.scope, s.cp)
		first <- err
	}()
	select {
	case <-owner.started:
	case <-time.After(10 * time.Second):
		t.Fatal("first durable persist did not start")
	}
	second := make(chan struct {
		receipt [32]byte
		err     error
	}, 1)
	go func() {
		receipt, err := gate.Stage(ctx, s.scope, s.cp)
		second <- struct {
			receipt [32]byte
			err     error
		}{receipt, err}
	}()
	timeout := time.NewTimer(10 * time.Second)
	defer timeout.Stop()
	for {
		gate.mu.RLock()
		current := gate.generation[s.scope]
		gate.mu.RUnlock()
		if current >= 2 {
			break
		}
		select {
		case <-timeout.C:
			t.Fatal("second stage never invalidated first generation")
		default:
			runtime.Gosched()
		}
	}
	close(owner.release)
	select {
	case err := <-first:
		if err == nil {
			t.Fatal("superseded owner persist returned a successful escrow")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("superseded stage did not exit")
	}
	select {
	case result := <-second:
		if result.err != nil || result.receipt == ([32]byte{}) {
			t.Fatal("latest stage rejected after superseding old generation:", result.err)
		}
		assertNotPermitted(t, s, result.receipt)
	case <-time.After(10 * time.Second):
		t.Fatal("latest stage did not finish")
	}
}

func TestOwner58S0HistoricalSnapshotCannotAuthorizeNewWORK(t *testing.T) {
	s := setupOwnerGate(t)
	receipt := testStage(t, s)
	s.publishSnapshot(t, s.cp)
	s.escrow.tailCommits = 1
	if err := s.gate.RestoreAndAcknowledge(context.Background(), s.scope, s.snapshot, receipt); err == nil {
		t.Fatal("historical snapshot with later committed tail granted WORK")
	}
	assertNotPermitted(t, s, receipt)
	if s.owner.acked != 0 || s.owner.restored != 0 {
		t.Fatal("external owner contacted for stale WORK checkpoint")
	}
	s.escrow.tailCommits = 0
	if err := s.gate.RestoreAndAcknowledge(context.Background(), s.scope, s.snapshot, receipt); err != nil {
		t.Fatal("current verified snapshot refused:", err)
	}
}

func TestOwner58S0ReadyPermitCannotOutliveSubsequentJournalCommits(t *testing.T) {
	s := setupOwnerGate(t)
	receipt := testStage(t, s)
	s.publishSnapshot(t, s.cp)
	if err := s.gate.RestoreAndAcknowledge(context.Background(), s.scope, s.snapshot, receipt); err != nil {
		t.Fatal(err)
	}
	called := 0
	s.escrow.tailCommits = 1
	if err := s.gate.RunWork(context.Background(), s.scope, s.snapshot, receipt, func(context.Context) error {
		called++
		return nil
	}); err == nil || called != 0 {
		t.Fatalf("stale in-memory ACK allowed post-commit model call: %v calls=%d", err, called)
	}
	s.escrow.tailCommits = 0
	if err := s.gate.RunWork(context.Background(), s.scope, s.snapshot, receipt, func(context.Context) error {
		called++
		return nil
	}); err != nil || called != 1 {
		t.Fatalf("current verified snapshot wrongly blocked: %v calls=%d", err, called)
	}
}

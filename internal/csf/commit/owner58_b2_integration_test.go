package commit

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/ownercheckpoint"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/ownercheckpoint/localdemo"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/ownercheckpoint/localowner"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
)

func TestCSFRV18R58B2OwnerLocalHostStageSnapshotRestartRestoreACKWork(t *testing.T) {
	ctx := context.Background()
	_, csf, _, run, bundle, csfPath := testFixture(t)
	scope := ownercheckpoint.Scope{Run: [32]byte(run), Namespace: "cognition", Actor: "owner-b2"}
	pub, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if err := csf.RegisterCognitiveActor(ctx, scope.Run, scope.Namespace, scope.Actor, bundle); err != nil {
		t.Fatal(err)
	}
	if err := csf.RegisterCognitiveOwnerKeyV1(ctx, scope.Run, scope.Namespace, scope.Actor, pub, bundle); err != nil {
		t.Fatal(err)
	}
	state, err := csf.LoadClock(ctx, scope.Run)
	if err != nil {
		t.Fatal(err)
	}
	cp := sqlite.SnapshotCheckpoint{
		ActorNamespace: scope.Namespace, ActorID: scope.Actor, StoreID: "local-owner-b2", SchemaVersion: 1,
		Producer: "cognitive-host-b2", ProducerVersion: 1, Content: []byte("synthetic actor-only memory B2"),
		CoversInstant: int64(state.Instant), CoversRevision: uint64(state.Revision),
	}
	ownerDir := t.TempDir()
	if err := os.Chmod(ownerDir, 0700); err != nil {
		t.Fatal(err)
	}
	ownerPath := filepath.Join(ownerDir, "owner.sqlite")
	owner, err := localowner.Open(ctx, ownerPath, private)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	worker := func(context.Context) error { count++; return nil }
	host, err := localdemo.New(owner, csf, bundle, scope, worker)
	if err != nil {
		t.Fatal(err)
	}

	// A caller may not stage another actor's content through the fixed actor host.
	wrongCP := cp
	wrongCP.ActorID = "foreign-actor"
	if _, err := host.StageSnapshot(ctx, wrongCP); err == nil {
		t.Fatal("foreign actor staged under fixed host")
	}
	ref, err := host.StageSnapshot(ctx, cp)
	if err != nil {
		t.Fatal(err)
	}
	if ref.Snapshot == ([32]byte{}) || ref.Receipt == ([32]byte{}) {
		t.Fatal("snapshot or receipt identity absent")
	}
	// Alter caller memory after publication: the causal/owner checkpoint remains byte-exact.
	original := bytes.Clone(cp.Content)
	cp.Content[0] ^= 0x2a
	if err := host.Run(ctx, ref); !errors.Is(err, ownercheckpoint.ErrNotAcknowledged) || count != 0 {
		t.Fatalf("work before restore ACK: calls=%d err=%v", count, err)
	}
	foreignReceipt := ref
	foreignReceipt.Receipt[0] ^= 0x40
	if err := host.Restore(ctx, foreignReceipt); err == nil {
		t.Fatal("forged receipt restored")
	}
	if err := host.Run(ctx, ref); !errors.Is(err, ownercheckpoint.ErrNotAcknowledged) || count != 0 {
		t.Fatalf("forged restore authorized work: count=%d err=%v", count, err)
	}
	if err := owner.Close(); err != nil {
		t.Fatal(err)
	}
	if err := csf.Close(); err != nil {
		t.Fatal(err)
	}

	// Simulate a fresh session: neither causal SQLite nor owner handle nor Gate
	// carries process-local permits; B3 owns independent OS-process crash drills.
	csfReopened, err := sqlite.Open(ctx, csfPath)
	if err != nil {
		t.Fatal(err)
	}
	defer csfReopened.Close()
	ownerReopened, err := localowner.Open(ctx, ownerPath, private)
	if err != nil {
		t.Fatal(err)
	}
	defer ownerReopened.Close()
	fresh, err := localdemo.New(ownerReopened, csfReopened, bundle, scope, worker)
	if err != nil {
		t.Fatal(err)
	}
	if err := fresh.Run(ctx, ref); !errors.Is(err, ownercheckpoint.ErrNotAcknowledged) || count != 0 {
		t.Fatalf("new session inherited permit: count=%d err=%v", count, err)
	}
	sealed, err := csfReopened.VerifySnapshotV1(ctx, scope.Run, ref.Snapshot, bundle)
	if err != nil || sealed.Snapshot.ID != ref.Snapshot || sealed.TailCommits != 0 {
		t.Fatalf("published snapshot not sealed at current tip: %v", err)
	}
	wrongSnap := ref
	wrongSnap.Snapshot[0] ^= 0x20
	if err := fresh.Restore(ctx, wrongSnap); err == nil {
		t.Fatal("foreign snapshot restored")
	}
	if err := fresh.Run(ctx, ref); !errors.Is(err, ownercheckpoint.ErrNotAcknowledged) || count != 0 {
		t.Fatal("foreign snapshot authorized work")
	}
	escrowed, locator, err := csfReopened.LoadCognitiveCheckpointV1(ctx, scope.Run, ref.Receipt, scope.Namespace, scope.Actor, bundle)
	if err != nil || locator == "" || !bytes.Equal(escrowed.Content, original) {
		t.Fatalf("CSF escrow differs from original published bytes: %v", err)
	}
	if err := fresh.Restore(ctx, ref); err != nil {
		t.Fatal("exact owner restore/ACK:", err)
	}
	if err := fresh.Run(ctx, ref); err != nil || count != 1 {
		t.Fatalf("valid cognitive work did not run exactly once: calls=%d err=%v", count, err)
	}
	if err := fresh.Run(ctx, wrongSnap); !errors.Is(err, ownercheckpoint.ErrNotAcknowledged) || count != 1 {
		t.Fatal("different snapshot inherited permit")
	}
	if err := fresh.Run(ctx, foreignReceipt); !errors.Is(err, ownercheckpoint.ErrNotAcknowledged) || count != 1 {
		t.Fatal("different receipt inherited permit")
	}
	if err := fresh.Restore(ctx, ref); err != nil {
		t.Fatal("idempotent restore:", err)
	}

	foreignActor := scope
	foreignActor.Actor = "owner-other"
	crossActor, err := localdemo.New(ownerReopened, csfReopened, bundle, foreignActor, worker)
	if err != nil {
		t.Fatal(err)
	}
	if err := crossActor.Restore(ctx, ref); err == nil {
		t.Fatal("cross-actor restored owner memory")
	}
	if err := crossActor.Run(ctx, ref); !errors.Is(err, ownercheckpoint.ErrNotAcknowledged) || count != 1 {
		t.Fatal("cross-actor executed callback")
	}
	foreignRun := scope
	foreignRun.Run[0] ^= 0x3f
	crossRun, err := localdemo.New(ownerReopened, csfReopened, bundle, foreignRun, worker)
	if err != nil {
		t.Fatal(err)
	}
	if err := crossRun.Restore(ctx, ref); err == nil {
		t.Fatal("cross-run restored owner memory")
	}
	if err := crossRun.Run(ctx, ref); !errors.Is(err, ownercheckpoint.ErrNotAcknowledged) || count != 1 {
		t.Fatal("cross-run executed callback")
	}
}

func TestCSFRV18R58B2LocalHostRejectsMissingDependencies(t *testing.T) {
	ctx := context.Background()
	_, csf, _, run, bundle, _ := testFixture(t)
	scope := ownercheckpoint.Scope{Run: [32]byte(run), Namespace: "cognition", Actor: "owner-b2"}
	worker := func(context.Context) error { return nil }
	if _, err := localdemo.New(nil, csf, bundle, scope, worker); err == nil {
		t.Fatal("nil owner accepted")
	}
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	owner, err := localowner.Open(ctx, filepath.Join(dir, "owner.sqlite"), private)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	if _, err := localdemo.New(owner, nil, bundle, scope, worker); err == nil {
		t.Fatal("nil store accepted")
	}
	if _, err := localdemo.New(owner, csf, nil, scope, worker); err == nil {
		t.Fatal("nil policy accepted")
	}
	if _, err := localdemo.New(owner, csf, bundle, scope, nil); err == nil {
		t.Fatal("nil cognitive callback accepted")
	}
	bad := scope
	bad.Actor = ""
	if _, err := localdemo.New(owner, csf, bundle, bad, worker); err == nil {
		t.Fatal("empty actor accepted")
	}
	bad = scope
	bad.Namespace = "not valid"
	if _, err := localdemo.New(owner, csf, bundle, bad, worker); err == nil {
		t.Fatal("invalid namespace accepted")
	}
}

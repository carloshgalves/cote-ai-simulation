package commit

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/adapter/embodimentv1"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/admission"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/genesis"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/inputledger"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/reducer"
	"github.com/carloshgalves/cote-ai-simulation/internal/physicalhost"
	_ "modernc.org/sqlite"
)

func physicalTestGenesis(t *testing.T, b *policy.Bundle) *genesis.Genesis {
	t.Helper()
	reg, err := b.PhysicalRegistration()
	if err != nil {
		t.Fatal(err)
	}
	base, err := policy.Load(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	g := validGenesis(t, base)
	env, err := codec.StrictDecodeEnvelope(g.CanonicalBytes)
	if err != nil {
		t.Fatal(err)
	}
	fields := append([]any(nil), env.Value.([]any)...)
	fields[12] = []any{[]any{reg.ExtensionID, reg.BundleSHA256[:]}}
	fields[13] = []any{reg.SourceID}
	initial := make([]reducer.PhysicalBody, 3)
	for i := range initial {
		initial[i] = reducer.PhysicalBody{
			Subject: []string{"npc.0001", "npc.0002", "npc.0003"}[i],
			Hours:   0, WPrimeCapacity: 2000, WPrimeRemaining: 2000, WPrimeTau: 300,
			WorkRestRatio: 1, Substrate: 1,
		}
		for j := range initial[i].Soreness {
			initial[i].Soreness[j] = reducer.PhysicalSoreness{Latency: []float64{0, 0, 0}}
		}
	}
	typed, _, err := reducer.EncodePhysical(initial)
	if err != nil {
		t.Fatal(err)
	}
	fields[15] = []any{[]any{reducer.PhysicalSchemaID, uint64(1)}}
	fields[16] = typed
	encoded, err := codec.CanonicalBytes(env.DomainTag, env.SchemaID, env.SchemaVersion, fields)
	if err != nil {
		t.Fatal(err)
	}
	verified, err := genesis.ValidateCanonical(encoded, b)
	if err != nil {
		t.Fatal(err)
	}
	return verified
}

func realPhysicalFixture(t *testing.T) (*Coordinator, *sqlite.Store, *inputledger.Ledger,
	admission.Cycle, admission.Fence, embodimentv1.ApprovedCut, string, physicalhost.ProducerHost) {
	t.Helper()
	if os.Getenv("PHYSICAL_A1_PRODUCER_TEST") != "1" {
		t.Skip("actual Python producer and A1 causal terminal run only under physical CI")
	}
	ctx := context.Background()
	base, err := policy.Load(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	py, err := exec.LookPath("python")
	if err != nil {
		t.Fatal(err)
	}
	py, err = filepath.EvalSymlinks(py)
	if err != nil {
		t.Fatal(err)
	}
	executable, err := os.ReadFile(py)
	if err != nil {
		t.Fatal(err)
	}
	executableHash := sha256.Sum256(executable)
	// The Actions toolcache may be group-writable. A1 must never relax the
	// producer executable invariant, so test with a privately staged, pinned
	// read-only interpreter and the trusted original Python library home.
	executableRoot := t.TempDir()
	pinnedPython := filepath.Join(executableRoot, "python-a1-pinned")
	if err := os.WriteFile(pinnedPython, executable, 0o500); err != nil {
		t.Fatal(err)
	}
	pythonHome := filepath.Dir(filepath.Dir(py))
	dependencyPath, err := exec.Command(py, "-S", "-c",
		"import sysconfig; print(sysconfig.get_path('purelib'))").Output()
	if err != nil {
		t.Fatalf("cannot locate pinned physical dependency directory: %v", err)
	}
	dependencyRoot := strings.TrimSpace(string(dependencyPath))
	module, err := os.ReadFile("../../../src/embodiment/a1_capture.py")
	if err != nil {
		t.Fatal(err)
	}
	moduleHash := sha256.Sum256(module)
	importRoot, err := filepath.Abs("../../../src")
	if err != nil {
		t.Fatal(err)
	}
	importTreeHash, err := physicalhost.PhysicalImportTreeSHA256(importRoot)
	if err != nil {
		t.Fatalf("physical producer package pin invalid: %v", err)
	}
	public, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	manifest := policy.PhysicalManifest{
		Kind: "cote.embodiment.physical-registration.v1", Version: 1,
		ExtensionID: "cote.embodiment.physical.v1",
		StateSchema: reducer.PhysicalSchemaID, StateVersion: 1, StateVariant: reducer.PhysicalVariant,
		ActionType: "physical.advance", ActionVersion: 1,
		ActionOperation: "physical.advance", ActionProducer: "physical.a1",
		SourceID: "physical.a1", ProducerExecutableSHA256: hex.EncodeToString(executableHash[:]),
		ProducerModuleSHA256:     hex.EncodeToString(moduleHash[:]),
		ProducerImportTreeSHA256: hex.EncodeToString(importTreeHash[:]),
		ProducerPublicKeyEd25519: hex.EncodeToString(public), Disclosure: "ENGINE_ONLY",
	}
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	b, err := policy.PhysicalExtensionV1(base, raw)
	if err != nil {
		t.Fatal(err)
	}
	g := physicalTestGenesis(t, b)
	dbPath := t.TempDir() + "/physical.db"
	store, err := sqlite.Open(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.PersistGenesis(ctx, g, b); err != nil {
		t.Fatal(err)
	}
	if err := store.BootstrapClock(ctx, g, b); err != nil {
		t.Fatal(err)
	}
	if err := store.BootstrapGenesisTriggers(ctx, g, b, nil); err != nil {
		t.Fatal(err)
	}
	stagingRoot, err := filepath.Abs(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(stagingRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	host := physicalhost.ProducerHost{
		Bundle: b, GenesisCanonical: g.CanonicalBytes, PrivateKey: key,
		ExecutablePath: pinnedPython, PythonHome: pythonHome, ImportRoot: importRoot,
		DependencyRoot: dependencyRoot, PrivateStagingRoot: stagingRoot,
		Epoch: 1, Plan: physicalhost.ProducerPlan{
			WorldSeed: 42, Count: 3,
			Advances: []physicalhost.ProducerAdvance{
				{Subject: "npc.0001", Days: 2, Sleep: "4h", Quality: "poor"},
				{Subject: "npc.0002", Days: 1, Sleep: "8h", Quality: "good"},
			},
		},
	}
	approved, err := physicalhost.RunSupervisedPhysical(ctx, host)
	if err != nil {
		t.Fatalf("A1 host failed: %v", err)
	}
	ledger, err := inputledger.New(ctx, g.RunID, b, store)
	if err != nil {
		t.Fatal(err)
	}
	var baseHash domain.Digest
	copy(baseHash[:], g.Payload[16].([]any)[3].([]byte))
	d := identity.New(b)
	id, _, err := d.Sum("cote.csf.id.cycle", "cote.csf.schema.cycle-id-preimage",
		1, []any{g.RunID[:], []any{int64(g.InitialInstant), uint64(0)}})
	if err != nil {
		t.Fatal(err)
	}
	cycle := admission.Cycle{ID: domain.ID(id), Attempt: 0, Instant: g.InitialInstant,
		Ordinal: 0, BaseRevision: 0, BaseStateHash: baseHash, RetryRef: []any{uint64(0)}}
	refs, err := policiesByRole(b)
	if err != nil {
		t.Fatal(err)
	}
	policies := admission.Policies{
		AdmissionOrder: refs["admission-order"], CausalIdentity: refs["causal-identity"],
		Fence: refs["admission-fence"], Rules: [][]any{},
	}
	c := New(store, b, policies)
	if _, err := c.StagePhysicalInput(ctx, ledger, approved, cycle); err != nil {
		t.Fatal(err)
	}
	fence, err := admission.NewCoordinator(admission.NewFenceLog(store)).
		AdmitThenEvaluate(ctx, b, ledger, cycle, policies, nil)
	if err != nil {
		t.Fatal(err)
	}
	return c, store, ledger, cycle, fence, approved, dbPath, host
}

func TestPhysicalAtomicCommitAndCrashReplay(t *testing.T) {
	ctx := context.Background()
	c, store, ledger, cycle, fence, approved, path, _ := realPhysicalFixture(t)
	_, _, _, _, _, run := ledger.Snapshot()
	before, err := store.LoadClock(ctx, [32]byte(run))
	if err != nil {
		t.Fatal(err)
	}
	beforeWorld, beforeHash, err := store.LoadWorldState(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	for step := uint8(1); step <= 7; step++ {
		c.failAfterStep = step
		if _, err := c.CommitPhysical(ctx, ledger, cycle, fence, approved); err == nil {
			t.Fatalf("crash point %d unexpectedly committed private physical batch", step)
		}
		current, err := store.LoadClock(ctx, [32]byte(run))
		if err != nil || current != before {
			t.Fatalf("crash point %d changed clock: %v", step, err)
		}
		world, hash, err := store.LoadWorldState(ctx, 0)
		if err != nil || hash != beforeHash || !samePhysicalState(world, beforeWorld) {
			t.Fatalf("crash point %d mutated world without CycleCommit", step)
		}
	}
	c.failAfterStep = 0
	journal, err := c.CommitPhysical(ctx, ledger, cycle, fence, approved)
	if err != nil {
		t.Fatalf("registered physical terminal refused: %v", err)
	}
	if len(journal.Canonical) == 0 {
		t.Fatal("physical CycleCommit missing")
	}
	current, err := store.LoadClock(ctx, [32]byte(run))
	if err != nil || current.Revision != 1 || current.NextLogicalSequence != 1 {
		t.Fatalf("physical clock not atomically advanced: %+v %v", current, err)
	}
	world, hash, err := store.LoadWorldState(ctx, 1)
	if err != nil || hash == beforeHash {
		t.Fatalf("physical reducer result missing: %v", err)
	}
	bodies, err := reducer.DecodePhysical(world)
	if err != nil || len(bodies) != 3 || bodies[0].Hours != 48 || bodies[1].Hours != 24 ||
		bodies[2].Hours != 0 {
		t.Fatalf("physical WorldRevision wrong: %+v %v", bodies, err)
	}
	if _, err := c.CommitPhysical(ctx, ledger, cycle, fence, approved); err == nil {
		t.Fatal("duplicate physical terminal published twice")
	}
	// Check that the same physical source is no longer selected as due work.
	reopened, err := sqlite.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	loaded, err := inputledger.Open(ctx, run, c.bundle, reopened)
	if err != nil {
		t.Fatalf("restored physical InputLedger failed: %v", err)
	}
	_, inputs, _, _, _, _ := loaded.Snapshot()
	if len(inputs) != 0 {
		t.Fatal("completed physical input still eligible for next AdmissionFence")
	}
	pending, err := reopened.PendingClock(ctx, [32]byte(run))
	if err != nil {
		t.Fatalf("physical replay verification failed: %v", err)
	}
	if _, err := reopened.ReplayClock(ctx, [32]byte(run), c.bundle); err != nil {
		t.Fatalf("complete physical replay through canonical Clock refused: %v", err)
	}
	if len(pending) != 0 {
		t.Fatal("committed physical source remains pending after recovery")
	}
}

func samePhysicalState(a, b []any) bool {
	left, err := codec.CanonicalPayload(a)
	if err != nil {
		return false
	}
	right, err := codec.CanonicalPayload(b)
	return err == nil && bytes.Equal(left, right)
}

// TestPhysicalAdversarialMutationsRejectedBeforeDurablePublication injects
// individually plausible forged WorkBatch fields AFTER Go materialization,
// requiring the SQLite terminal to independently revalidate every authority.
func TestPhysicalAdversarialMutationsRejectedBeforeDurablePublication(t *testing.T) {
	ctx := context.Background()
	c, store, ledger, cycle, fence, approved, _, _ := realPhysicalFixture(t)
	_, _, _, _, _, run := ledger.Snapshot()
	before, err := store.LoadClock(ctx, [32]byte(run))
	if err != nil {
		t.Fatal(err)
	}
	_, hash, err := store.LoadWorldState(ctx, uint64(before.Revision))
	if err != nil {
		t.Fatal(err)
	}
	mutants := []struct {
		name   string
		change func(*ports.WorkBatch)
	}{
		{"physical-extension-manifest", func(b *ports.WorkBatch) {
			b.Physical.Manifest = bytes.Clone(b.Physical.Manifest)
			b.Physical.Manifest[0] ^= 1
		}},
		{"signed-A1-cut", func(b *ports.WorkBatch) {
			b.Physical.SignedCut = bytes.Clone(b.Physical.SignedCut)
			b.Physical.SignedCut[len(b.Physical.SignedCut)-1] ^= 1
		}},
		{"A1-signature", func(b *ports.WorkBatch) {
			b.Physical.Signature = bytes.Clone(b.Physical.Signature)
			b.Physical.Signature[0] ^= 1
		}},
		{"physical-state-digest", func(b *ports.WorkBatch) {
			b.Physical.CompletedTyped = append([]any(nil), b.Physical.CompletedTyped...)
			b.Physical.CompletedTyped[3] = bytes.Repeat([]byte{0xEE}, 32)
		}},
		{"candidate-identity", func(b *ports.WorkBatch) { b.Physical.Candidate.Identity[0] ^= 1 }},
		{"candidate-canonical", func(b *ports.WorkBatch) {
			b.Physical.Candidate.Canonical = bytes.Clone(b.Physical.Candidate.Canonical)
			b.Physical.Candidate.Canonical[len(b.Physical.Candidate.Canonical)-1] ^= 1
		}},
		{"DecisionRecord-canonical", func(b *ports.WorkBatch) {
			b.Decisions[0].Canonical = bytes.Clone(b.Decisions[0].Canonical)
			b.Decisions[0].Canonical[len(b.Decisions[0].Canonical)-1] ^= 1
		}},
		{"physical-EventStore-event", func(b *ports.WorkBatch) {
			b.Events[0].Canonical = bytes.Clone(b.Events[0].Canonical)
			b.Events[0].Canonical[len(b.Events[0].Canonical)-1] ^= 1
		}},
		{"physical-EventStore-ID", func(b *ports.WorkBatch) { b.Events[0].Identity[0] ^= 1 }},
		{"CycleCommit-journal", func(b *ports.WorkBatch) {
			b.Journal.Canonical = bytes.Clone(b.Journal.Canonical)
			b.Journal.Canonical[len(b.Journal.Canonical)-1] ^= 1
		}},
		{"WorldRevision", func(b *ports.WorkBatch) {
			b.RevisionCanonical = bytes.Clone(b.RevisionCanonical)
			b.RevisionCanonical[len(b.RevisionCanonical)-1] ^= 1
		}},
		{"result-world-digest", func(b *ports.WorkBatch) { b.ResultStateHash[0] ^= 1 }},
		{"unregistered-perception-task", func(b *ports.WorkBatch) {
			b.PerceptionTasks = append(b.PerceptionTasks, ports.CanonicalRecord{Identity: [32]byte{1}, Canonical: []byte{0x80}})
		}},
		{"foreign-system-candidate", func(b *ports.WorkBatch) {
			b.SystemCandidate = ports.CanonicalRecord{Identity: [32]byte{1}, Canonical: []byte{0x80}}
		}},
		{"foreign-resource-candidate", func(b *ports.WorkBatch) {
			b.ResourceCandidate = ports.CanonicalRecord{Identity: [32]byte{1}, Canonical: []byte{0x80}}
		}},
		{"foreign-contest-proof", func(b *ports.WorkBatch) {
			b.ContestProofs = append(b.ContestProofs, ports.CanonicalRecord{Identity: [32]byte{1}, Canonical: []byte{0x80}})
		}},
		{"foreign-system-successor", func(b *ports.WorkBatch) {
			b.SystemSuccessors = append(b.SystemSuccessors, ports.CanonicalRecord{Identity: [32]byte{1}, Canonical: []byte{0x80}})
		}},
		{"foreign-communication-candidate", func(b *ports.WorkBatch) {
			b.CommunicationCandidate = ports.CanonicalRecord{Identity: [32]byte{1}, Canonical: []byte{0x80}}
		}},
	}
	for _, mutant := range mutants {
		t.Run(mutant.name, func(t *testing.T) {
			c.tamperWorkBatch = mutant.change
			if _, err := c.CommitPhysical(ctx, ledger, cycle, fence, approved); err == nil {
				t.Fatal("forged physical batch reached a terminal CycleCommit")
			}
			current, err := store.LoadClock(ctx, [32]byte(run))
			if err != nil || current != before {
				t.Fatalf("forged batch changed clock: %+v %v", current, err)
			}
			_, actual, err := store.LoadWorldState(ctx, uint64(before.Revision))
			if err != nil || actual != hash {
				t.Fatalf("forged batch changed predecessor state: %v", err)
			}
		})
	}
	c.tamperWorkBatch = nil
	if _, err := c.CommitPhysical(ctx, ledger, cycle, fence, approved); err != nil {
		t.Fatalf("failed adversarial attempts must leave original terminal retryable: %v", err)
	}
}

// This tests durable, independently revalidated provenance AFTER publication.
// Every corruption is restored before the next one so failures cannot be
// explained by a previous unrelated broken field.
func TestPhysicalReplayRejectsPersistedSourceDecisionEventAndWorldTampering(t *testing.T) {
	ctx := context.Background()
	c, _, ledger, cycle, fence, approved, path, _ := realPhysicalFixture(t)
	journal, err := c.CommitPhysical(ctx, ledger, cycle, fence, approved)
	if err != nil {
		t.Fatal(err)
	}
	_, inputs, _, _, _, run := ledger.Snapshot()
	if len(inputs) != 1 {
		t.Fatal("fixture lost physical source")
	}
	source := inputs[0].ID
	jv, err := codec.StrictDecodePayload(journal.Canonical)
	if err != nil {
		t.Fatal(err)
	}
	j := jv.([]any)
	events := j[15].([]any)
	eventID := events[0].([]byte)
	decisionID := j[16].([]any)[0].([]byte)
	replay, err := sqlite.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer replay.Close()
	if _, err := replay.ReplayClock(ctx, [32]byte(run), c.bundle); err != nil {
		t.Fatalf("healthy physical replay failed: %v", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, tc := range []struct {
		name, selectSQL, updateSQL string
		where                      any
	}{
		{"source-ingress",
			"SELECT canonical FROM input_ledger WHERE id=?",
			"UPDATE input_ledger SET canonical=? WHERE id=?", source[:]},
		{"decision-record",
			"SELECT canonical FROM decision_ledger WHERE id=?",
			"UPDATE decision_ledger SET canonical=? WHERE id=?", decisionID},
		{"EventStore",
			"SELECT canonical FROM event_store WHERE id=?",
			"UPDATE event_store SET canonical=? WHERE id=?", eventID},
		{"world-revision",
			"SELECT canonical FROM world_revisions WHERE revision=?",
			"UPDATE world_revisions SET canonical=? WHERE revision=?", uint64(1)},
		{"world-revision-hash",
			"SELECT state_hash FROM world_revisions WHERE revision=?",
			"UPDATE world_revisions SET state_hash=? WHERE revision=?", uint64(1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var original []byte
			if err := db.QueryRowContext(ctx, tc.selectSQL, tc.where).Scan(&original); err != nil {
				t.Fatal(err)
			}
			if len(original) < 4 {
				t.Fatal("fixture has empty authenticated record")
			}
			damage := bytes.Clone(original)
			damage[len(damage)-1] ^= 1
			if _, err := db.ExecContext(ctx, tc.updateSQL, damage, tc.where); err != nil {
				t.Fatal(err)
			}
			if _, err := replay.ReplayClock(ctx, [32]byte(run), c.bundle); err == nil {
				t.Fatalf("ReplayClock accepted poisoned %s", tc.name)
			}
			if _, err := db.ExecContext(ctx, tc.updateSQL, original, tc.where); err != nil {
				t.Fatal(err)
			}
			if _, err := replay.ReplayClock(ctx, [32]byte(run), c.bundle); err != nil {
				t.Fatalf("restored %s failed canonical replay: %v", tc.name, err)
			}
		})
	}
}

// Every physically committed world is recoverable from an IDLE snapshot and
// migrates to an independent child run without inheriting the parent's source
// authority. This is deliberately a positive test with a real Python A1 cut.
func TestPhysicalSnapshotRestoreAndIndependentFork(t *testing.T) {
	ctx := context.Background()
	c, parent, ledger, cycle, fence, approved, path, _ := realPhysicalFixture(t)
	_, _, _, _, _, run := ledger.Snapshot()
	if _, err := c.CommitPhysical(ctx, ledger, cycle, fence, approved); err != nil {
		t.Fatalf("physical parent commit refused: %v", err)
	}
	// The parent can continue accepting future A1 epochs, so StagePhysicalInput
	// deliberately uses a NON-final closure. A fork may archive this source
	// only after the host explicitly finalizes its ingestion cursor.
	if _, err := ledger.CloseSource(ctx, "physical.a1", inputledger.Closure{
		ProducerKey: "physical.a1.final.epoch.1",
		ClosedThrough: domain.EligibilityCoordinate{
			NotBeforeInstant: cycle.Instant, NotBeforeCycleOrdinal: cycle.Ordinal + 1,
		},
		Final: true,
	}); err != nil {
		t.Fatalf("cannot finalize physical origin before fork: %v", err)
	}
	expected, expectedHash, err := parent.LoadWorldState(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := parent.CreateSnapshotV1(ctx, [32]byte(run), c.bundle, nil)
	if err != nil {
		t.Fatalf("physical world cannot create IDLE snapshot: %v", err)
	}
	if _, err := parent.VerifySnapshotV1(ctx, [32]byte(run), snapshot.ID, c.bundle); err != nil {
		t.Fatalf("physical snapshot verification rejected its own canonical truth: %v", err)
	}
	reopened, err := sqlite.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if _, err := reopened.VerifySnapshotV1(ctx, [32]byte(run), snapshot.ID, c.bundle); err != nil {
		t.Fatalf("physical snapshot cannot restore after SQLite reopen: %v", err)
	}
	child, err := sqlite.Open(ctx, t.TempDir()+"/physical-child.db")
	if err != nil {
		t.Fatal(err)
	}
	defer child.Close()
	childRun := [32]byte(run)
	childRun[0] ^= 0x51
	childGenesis, err := parent.ForkIdleSnapshotV1(ctx, [32]byte(run), snapshot.ID, childRun, child, c.bundle)
	if err != nil {
		t.Fatalf("physical canonical snapshot cannot fork: %v", err)
	}
	if [32]byte(childGenesis.RunID) != childRun {
		t.Fatal("fork reused its parent's run identity")
	}
	forked, forkHash, err := child.LoadWorldState(ctx, 0)
	if err != nil || forkHash != expectedHash || !samePhysicalState(forked, expected) {
		t.Fatalf("fork lost signed physical world state: %v", err)
	}
	if _, err := child.ReplayClock(ctx, childRun, c.bundle); err != nil {
		t.Fatalf("independent physical child is not replayable: %v", err)
	}
	if _, err := reopened.VerifySnapshotV1(ctx, [32]byte(run), snapshot.ID, c.bundle); err != nil {
		t.Fatalf("physical child mutated immutable parent checkpoint: %v", err)
	}
}

// A non-final producer cursor cannot be silently dropped when a child run
// receives the physical world snapshot. The parent remains untouched.
func TestPhysicalForkRejectsUnfinishedSourceCursor(t *testing.T) {
	ctx := context.Background()
	coordinator, parent, ledger, cycle, fence, approved, _, _ := realPhysicalFixture(t)
	if _, err := coordinator.CommitPhysical(ctx, ledger, cycle, fence, approved); err != nil {
		t.Fatal(err)
	}
	_, _, _, _, _, run := ledger.Snapshot()
	snapshot, err := parent.CreateSnapshotV1(ctx, [32]byte(run), coordinator.bundle, nil)
	if err != nil {
		t.Fatal(err)
	}
	child, err := sqlite.Open(ctx, t.TempDir()+"/unfinished-physical-fork.db")
	if err != nil {
		t.Fatal(err)
	}
	defer child.Close()
	childRun := [32]byte(run)
	childRun[0] ^= 0x52
	if forked, err := parent.ForkIdleSnapshotV1(ctx, [32]byte(run), snapshot.ID,
		childRun, child, coordinator.bundle); err == nil || forked != nil {
		t.Fatalf("fork silently dropped an unfinished physical source: fork=%v err=%v", forked, err)
	}
	if _, err := child.ValidatedGenesis(ctx, childRun, coordinator.bundle); err == nil {
		t.Fatal("rejected physical fork left a committed child Genesis")
	}
	if _, err := parent.VerifySnapshotV1(ctx, [32]byte(run), snapshot.ID, coordinator.bundle); err != nil {
		t.Fatalf("rejected physical fork damaged its parent: %v", err)
	}
}

// A fully valid A1 cut from another installed Genesis must be rejected before
// a physical SourceDeclaration is appended. Otherwise an untrusted caller
// could irreversibly freeze/pollute a source namespace merely by proposing
// a cut signed for a different registration.
func TestPhysicalForeignAttestationHasNoIngressSideEffects(t *testing.T) {
	ctx := context.Background()
	original, _, _, cycle, _, approved, _, _ := realPhysicalFixture(t)
	baseBundle, err := policy.Load(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := original.bundle.PhysicalManifestCanonical()
	if err != nil {
		t.Fatal(err)
	}
	var manifest policy.PhysicalManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	foreignPublic, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	manifest.ProducerPublicKeyEd25519 = hex.EncodeToString(foreignPublic)
	foreignRaw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	foreignBundle, err := policy.PhysicalExtensionV1(baseBundle, foreignRaw)
	if err != nil {
		t.Fatal(err)
	}
	foreignGenesis := physicalTestGenesis(t, foreignBundle)
	foreignStore, err := sqlite.Open(ctx, t.TempDir()+"/foreign-physical.db")
	if err != nil {
		t.Fatal(err)
	}
	defer foreignStore.Close()
	if err := foreignStore.PersistGenesis(ctx, foreignGenesis, foreignBundle); err != nil {
		t.Fatal(err)
	}
	if err := foreignStore.BootstrapClock(ctx, foreignGenesis, foreignBundle); err != nil {
		t.Fatal(err)
	}
	if err := foreignStore.BootstrapGenesisTriggers(ctx, foreignGenesis, foreignBundle, nil); err != nil {
		t.Fatal(err)
	}
	foreignLedger, err := inputledger.New(ctx, foreignGenesis.RunID, foreignBundle, foreignStore)
	if err != nil {
		t.Fatal(err)
	}
	foreignCoordinator := New(foreignStore, foreignBundle, original.admissionPolicies)
	if _, err := foreignCoordinator.StagePhysicalInput(ctx, foreignLedger, approved, cycle); err == nil {
		t.Fatal("foreign signed cut was accepted")
	}
	if len(foreignLedger.Sources()) != 0 {
		t.Fatal("foreign signed cut left an unauthorized physical SourceDeclaration")
	}
	records, err := foreignStore.Records(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 0 {
		t.Fatal("foreign signed cut left durable ingress records")
	}
}

// Two completed A1 epochs at successive global ordinals must replay from
// durable source/fence roots without pulling the second epoch into the first
// cycle, re-admitting the first, or reusing Python local sequence numbers.
func TestPhysicalTwoEpochsKeepGlobalCausalityAndReplay(t *testing.T) {
	ctx := context.Background()
	c, store, ledger, first, fence, approved, _, host := realPhysicalFixture(t)
	if _, err := c.CommitPhysical(ctx, ledger, first, fence, approved); err != nil {
		t.Fatalf("first physical epoch failed: %v", err)
	}
	_, _, _, _, _, run := ledger.Snapshot()
	state, err := store.LoadClock(ctx, [32]byte(run))
	if err != nil {
		t.Fatal(err)
	}
	_, hash, err := store.LoadWorldState(ctx, uint64(state.Revision))
	if err != nil {
		t.Fatal(err)
	}
	nextOrdinal := domain.CycleOrdinal(state.LastOrdinal + 1)
	cycleID, _, err := identity.New(c.bundle).Sum(
		"cote.csf.id.cycle", "cote.csf.schema.cycle-id-preimage", 1,
		[]any{run[:], []any{int64(state.Instant), uint64(nextOrdinal)}})
	if err != nil {
		t.Fatal(err)
	}
	second := admission.Cycle{
		ID: domain.ID(cycleID), Attempt: 0, Instant: state.Instant,
		Ordinal: nextOrdinal, BaseRevision: state.Revision,
		BaseStateHash: domain.Digest(hash), RetryRef: []any{uint64(0)},
	}
	host.Epoch = 2
	host.Plan.Advances = []physicalhost.ProducerAdvance{
		{Subject: "npc.0001", Days: 3, Sleep: "4h", Quality: "poor"},
		{Subject: "npc.0002", Days: 2, Sleep: "8h", Quality: "good"},
	}
	approvedSecond, err := physicalhost.RunSupervisedPhysical(ctx, host)
	if err != nil {
		t.Fatalf("second supervised A1 cut refused: %v", err)
	}
	if _, err := c.StagePhysicalInput(ctx, ledger, approvedSecond, second); err != nil {
		t.Fatalf("second A1 epoch could not enter InputLedger: %v", err)
	}
	secondFence, err := admission.NewCoordinator(admission.NewFenceLog(store)).
		AdmitThenEvaluate(ctx, c.bundle, ledger, second, c.admissionPolicies, nil)
	if err != nil {
		t.Fatalf("second physical AdmissionFence rejected: %v", err)
	}
	if _, err := c.CommitPhysical(ctx, ledger, second, secondFence, approvedSecond); err != nil {
		t.Fatalf("second physical terminal refused: %v", err)
	}
	finalClock, err := store.LoadClock(ctx, [32]byte(run))
	if err != nil || finalClock.Revision != 2 || finalClock.NextLogicalSequence != 2 ||
		finalClock.LastOrdinal != int64(nextOrdinal) {
		t.Fatalf("two physical epochs did not advance global causality: %+v %v", finalClock, err)
	}
	finalWorld, _, err := store.LoadWorldState(ctx, uint64(finalClock.Revision))
	if err != nil {
		t.Fatal(err)
	}
	bodies, err := reducer.DecodePhysical(finalWorld)
	if err != nil || len(bodies) != 3 || bodies[0].Hours != 72 ||
		bodies[1].Hours != 48 || bodies[2].Hours != 0 {
		t.Fatalf("two physical epochs corrupted canonical BodyState: %+v %v", bodies, err)
	}
	report, err := store.ReplayClock(ctx, [32]byte(run), c.bundle)
	if err != nil || report.ReplayedCommits != 2 || report.ReplayedEvents != 2 {
		t.Fatalf("multi-epoch replay diverged from journal: %+v %v", report, err)
	}
	pending, err := store.PendingClock(ctx, [32]byte(run))
	if err != nil || len(pending) != 0 {
		t.Fatalf("liquidated physical epochs reappeared as pending: %+v %v", pending, err)
	}
}

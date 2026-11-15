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
	admission.Cycle, admission.Fence, embodimentv1.ApprovedCut, string) {
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
	module, err := os.ReadFile("../../../src/embodiment/a1_capture.py")
	if err != nil {
		t.Fatal(err)
	}
	moduleHash := sha256.Sum256(module)
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
	importRoot, err := filepath.Abs("../../../src")
	if err != nil {
		t.Fatal(err)
	}
	stagingRoot, err := filepath.Abs(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(stagingRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	approved, err := physicalhost.RunSupervisedPhysical(ctx, physicalhost.ProducerHost{
		Bundle: b, GenesisCanonical: g.CanonicalBytes, PrivateKey: key,
		ExecutablePath: pinnedPython, PythonHome: pythonHome, ImportRoot: importRoot, PrivateStagingRoot: stagingRoot,
		Epoch: 1, Plan: physicalhost.ProducerPlan{
			WorldSeed: 42, Count: 3,
			Advances: []physicalhost.ProducerAdvance{
				{Subject: "npc.0001", Days: 2, Sleep: "4h", Quality: "poor"},
				{Subject: "npc.0002", Days: 1, Sleep: "8h", Quality: "good"},
			},
		},
	})
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
	return c, store, ledger, cycle, fence, approved, dbPath
}

func TestPhysicalAtomicCommitAndCrashReplay(t *testing.T) {
	ctx := context.Background()
	c, store, ledger, cycle, fence, approved, path := realPhysicalFixture(t)
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
	c, store, ledger, cycle, fence, approved, _ := realPhysicalFixture(t)
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
	c, _, ledger, cycle, fence, approved, path := realPhysicalFixture(t)
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

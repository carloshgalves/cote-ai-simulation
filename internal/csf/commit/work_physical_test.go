package commit

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
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
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/reducer"
	"github.com/carloshgalves/cote-ai-simulation/internal/physicalhost"
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

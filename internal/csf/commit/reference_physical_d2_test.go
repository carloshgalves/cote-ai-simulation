//go:build linux

package commit

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/reducer"
	"github.com/carloshgalves/cote-ai-simulation/internal/physicalhost"
)

func referenceD2TestHost(t *testing.T) (physicalhost.ProducerHost, [2]physicalhost.ProducerPlan) {
	t.Helper()
	if os.Getenv("PHYSICAL_A1_PRODUCER_TEST") != "1" {
		t.Skip("real Python + memfd D2 requires PHYSICAL_A1_PRODUCER_TEST=1 and pinned CI dependencies")
	}
	base, err := policy.Load(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	python, err := exec.LookPath("python")
	if err != nil {
		t.Fatal(err)
	}
	python, err = filepath.EvalSymlinks(python)
	if err != nil {
		t.Fatal(err)
	}
	executable, err := os.ReadFile(python)
	if err != nil {
		t.Fatal(err)
	}
	execHash := sha256.Sum256(executable)
	root := t.TempDir()
	pinned := filepath.Join(root, "pinned-a1-python")
	if err := os.WriteFile(pinned, executable, 0500); err != nil {
		t.Fatal(err)
	}
	home := filepath.Dir(filepath.Dir(python))
	deps, err := exec.Command(python, "-S", "-c", "import sysconfig;print(sysconfig.get_path('purelib'))").Output()
	if err != nil {
		t.Fatalf("cannot resolve python pinned dependency root: %v", err)
	}
	depRoot := strings.TrimSpace(string(deps))
	rootImport, err := filepath.Abs("../../../src")
	if err != nil {
		t.Fatal(err)
	}
	module, err := os.ReadFile(filepath.Join(rootImport, "embodiment", "a1_capture.py"))
	if err != nil {
		t.Fatal(err)
	}
	moduleHash := sha256.Sum256(module)
	importHash, err := physicalhost.PhysicalImportTreeSHA256(rootImport)
	if err != nil {
		t.Fatalf("not a clean pinned A1 import tree: %v", err)
	}
	pub, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	manifest := policy.PhysicalManifest{
		Kind: "cote.embodiment.physical-registration.v1", Version: 1,
		ExtensionID: "cote.embodiment.physical.v1",
		StateSchema: reducer.PhysicalSchemaID, StateVersion: 1,
		StateVariant: reducer.PhysicalVariant,
		ActionType:   "physical.advance", ActionVersion: 1,
		ActionOperation: "physical.advance", ActionProducer: "physical.a1",
		SourceID:                 "physical.a1",
		ProducerExecutableSHA256: hex.EncodeToString(execHash[:]),
		ProducerModuleSHA256:     hex.EncodeToString(moduleHash[:]),
		ProducerImportTreeSHA256: hex.EncodeToString(importHash[:]),
		ProducerPublicKeyEd25519: hex.EncodeToString(pub),
		Disclosure:               "ENGINE_ONLY",
	}
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	installed, err := policy.PhysicalExtensionV1(base, raw)
	if err != nil {
		t.Fatal(err)
	}
	privateRoot := t.TempDir()
	if err := os.Chmod(privateRoot, 0700); err != nil {
		t.Fatal(err)
	}
	plans := [2]physicalhost.ProducerPlan{
		{WorldSeed: 42, Count: 3, Advances: []physicalhost.ProducerAdvance{
			{Subject: "npc.0001", Days: 2, Sleep: "4h", Quality: "poor"},
			{Subject: "npc.0002", Days: 1, Sleep: "8h", Quality: "good"},
		}},
		{WorldSeed: 42, Count: 3, Advances: []physicalhost.ProducerAdvance{
			{Subject: "npc.0001", Days: 3, Sleep: "4h", Quality: "poor"},
			{Subject: "npc.0002", Days: 2, Sleep: "8h", Quality: "good"},
		}},
	}
	return physicalhost.ProducerHost{Bundle: installed, PrivateKey: private,
		ExecutablePath: pinned, PythonHome: home, ImportRoot: rootImport,
		DependencyRoot: depRoot, PrivateStagingRoot: privateRoot}, plans
}

func TestReferenceD2RealTwoEpochsOnSameC4RunID(t *testing.T) {
	ctx := context.Background()
	host, plans := referenceD2TestHost(t)
	dir := t.TempDir()
	ownerDir := filepath.Join(dir, "owner")
	if err := os.Mkdir(ownerDir, 0700); err != nil {
		t.Fatal(err)
	}
	got, err := RunReferenceD2(ctx, filepath.Join(dir, "causal.db"),
		filepath.Join(ownerDir, "b.db"), bundlePath,
		ReferenceD2Config{Host: host, EpochPlans: plans}, ReferenceOptions{})
	if err != nil {
		t.Fatalf("actual A1+C4 two-epoch run failed: %v", err)
	}
	if got.Status != "RUN_LOCAL_10D2" || got.C4 == nil ||
		got.RunID != got.C4.RunID || got.C4.Status != "RUN_LOCAL_10C4" ||
		len(got.C4.Cycles) != 6 || got.FinalRevision != 8 || got.FinalInstant != 35 ||
		got.CommittedPhysicalEpochs != 2 || !got.ReplayVerified ||
		!got.SnapshotVerifiedWithTwoTailCommits || !got.DigestChainVerified ||
		!got.A1SignedByGenesisKey || !got.SameCausalRun ||
		!got.ResourceBranchUnchanged || !got.BodyContinuityVerified {
		t.Fatalf("D2 missing same-run invariant: %+v", got)
	}
	if want := map[string]float64{"npc.0001": 72, "npc.0002": 48, "npc.0003": 0}; !reflect.DeepEqual(got.PrivilegedBodyHours, want) {
		t.Fatalf("two epochs failed actual Go BodyState continuation: got=%v want=%v",
			got.PrivilegedBodyHours, want)
	}
	if got.PrivilegedFinalStock != 0 {
		t.Fatal("registered A1 physical producer replenished consumed resource")
	}
	if got.PhysicalInputIDs[0] == got.PhysicalInputIDs[1] ||
		got.PhysicalJournalIDs[0] == got.PhysicalJournalIDs[1] ||
		got.PhysicalEventIDs[0] == got.PhysicalEventIDs[1] {
		t.Fatal("physical source or terminal epoch was duplicated")
	}
	public, err := got.PublicReport()
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(public)
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{"npc.0001", "npc.0002", "npc.0003",
		"contest_winner", "final_stock", "privileged_body_hours",
		"final_world_hash", "operator_private_key", "resource.stolen"} {
		if strings.Contains(string(encoded), private) {
			t.Fatalf("D2 public report leaked privileged state: %s", private)
		}
	}
	var raw map[string]any
	if err := json.Unmarshal(encoded, &raw); err != nil {
		t.Fatal(err)
	}
	if len(raw) != 5 || raw["status"] != "RUN_LOCAL_10D2" {
		t.Fatalf("D2 public report is not fixed allowlist: %s", encoded)
	}
}

func TestReferenceD2ForgedPhysicalOperatorDeniedBeforeCausalGenesis(t *testing.T) {
	ctx := context.Background()
	host, plans := referenceD2TestHost(t)
	host.PrivateKey = append(ed25519.PrivateKey(nil), host.PrivateKey...)
	host.PrivateKey[ed25519.PrivateKeySize-1] ^= 0x01
	dir := t.TempDir()
	causal := filepath.Join(dir, "run.db")
	ownerDir := filepath.Join(dir, "owner")
	if err := os.Mkdir(ownerDir, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := RunReferenceD2(ctx, causal, filepath.Join(ownerDir, "b.db"),
		bundlePath, ReferenceD2Config{Host: host, EpochPlans: plans},
		ReferenceOptions{}); err == nil {
		t.Fatal("forged A1 private signing key accepted")
	}
	if _, err := os.Lstat(causal); !os.IsNotExist(err) {
		t.Fatalf("forged A1 signer created causal Genesis or database: %v", err)
	}
}

func TestReferenceD2TwoSourceWorkerOrdersYieldIdenticalAuthenticatedPhysicalTail(t *testing.T) {
	host, plans := referenceD2TestHost(t)
	run := func(opts ReferenceOptions) ReferenceD2Proof {
		t.Helper()
		dir := t.TempDir()
		ownerDir := filepath.Join(dir, "owner")
		if err := os.Mkdir(ownerDir, 0700); err != nil {
			t.Fatal(err)
		}
		got, err := RunReferenceD2(context.Background(), filepath.Join(dir, "run.db"),
			filepath.Join(ownerDir, "b.db"), bundlePath,
			ReferenceD2Config{Host: host, EpochPlans: plans}, opts)
		if err != nil {
			t.Fatalf("D2 permutation failed: %v", err)
		}
		return got
	}
	original := run(ReferenceOptions{})
	flipped := run(ReferenceOptions{ReverseSources: true, ReverseResponses: true})
	if original.RunID != flipped.RunID ||
		!reflect.DeepEqual(original.PhysicalEventIDs, flipped.PhysicalEventIDs) ||
		!reflect.DeepEqual(original.PhysicalJournalIDs, flipped.PhysicalJournalIDs) ||
		!reflect.DeepEqual(original.PhysicalInputIDs, flipped.PhysicalInputIDs) ||
		!reflect.DeepEqual(original.FiveLedgerDigests, flipped.FiveLedgerDigests) ||
		original.FinalWorldHash != flipped.FinalWorldHash ||
		original.GenesisSHA256 != flipped.GenesisSHA256 {
		t.Fatalf("worker and source order changed canonical D2 causal authority")
	}
}

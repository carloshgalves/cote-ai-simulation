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

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/adapter/embodimentv1"
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
		len(got.C4.Cycles) != 6 || got.FinalRevision != 9 || got.FinalInstant != 35 ||
		got.CommittedPhysicalEpochs != 2 || !got.ScriptedNoProposalVerified ||
		len(got.ScriptedNoProposalJournalID) != 64 || !got.ReplayVerified ||
		!got.SnapshotVerifiedWithThreeTailCommits || !got.DigestChainVerified ||
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
	// True controlled experiment: produce TWO real, signed A1 captures
	// once. Each raw physical log includes authentic wall_time. Re-running
	// Python independently would change exogenous source evidence and is
	// therefore NOT a valid byte-identical source/worker permutation.
	g, err := buildReferenceGenesis(host.Bundle)
	if err != nil { t.Fatal(err) }
	host.GenesisCanonical = g.CanonicalBytes
	var cuts [2]embodimentv1.ApprovedCut
	for i := range cuts {
		source := host
		source.Epoch = uint64(i + 1)
		source.Plan = plans[i]
		cuts[i], err = physicalhost.RunSupervisedPhysical(context.Background(), source)
		if err != nil { t.Fatalf("real frozen A1 source %d refused: %v", i+1, err) }
		if _, _, _, err := cuts[i].CanonicalProof(); err != nil {
			t.Fatalf("frozen source %d has no signed proof: %v", i+1, err)
		}
	}
	// The D2 runner must derive and persist Genesis itself. The only
	// test-only replay material is the already supervised authenticated
	// cut, not a caller-supplied Genesis or manually created WorldState.
	host.GenesisCanonical = nil
	run := func(opts ReferenceOptions) (ReferenceD2Proof, map[string][]string) {
		t.Helper()
		dir := t.TempDir()
		ownerDir := filepath.Join(dir, "owner")
		if err := os.Mkdir(ownerDir, 0700); err != nil {
			t.Fatal(err)
		}
		got, err := RunReferenceD2(context.Background(), filepath.Join(dir, "run.db"),
			filepath.Join(ownerDir, "b.db"), bundlePath,
			ReferenceD2Config{Host: host, EpochPlans: plans, frozenSupervisedCuts: &cuts}, opts)
		if err != nil {
			t.Fatalf("D2 permutation failed: %v", err)
		}
		// Compare exact persisted row identities and byte digests, not only
		// whole-prefix hashes: distinguish an A1/C4 authority bug from
		// an out-of-order NoProposal or source-ingress receipt.
		db, err := openReferenceReadOnlyDB(filepath.Join(dir, "run.db"))
		if err != nil { t.Fatal(err) }
		defer db.Close()
		fingerprints := map[string][]string{}
		for _, table := range []string{"input_ledger", "decision_ledger", "event_store"} {
			rows, err := db.Query("SELECT lower(hex(id)), canonical FROM " + table + " ORDER BY id")
			if err != nil { t.Fatal(err) }
			for rows.Next() {
				var id string
				var canonical []byte
				if err := rows.Scan(&id, &canonical); err != nil { rows.Close(); t.Fatal(err) }
				digest := sha256.Sum256(canonical)
				fingerprints[table] = append(fingerprints[table], id + ":" + hex.EncodeToString(digest[:]))
			}
			if err := rows.Err(); err != nil { rows.Close(); t.Fatal(err) }
			rows.Close()
		}
		return got, fingerprints
	}
	original, originalRows := run(ReferenceOptions{})
	flipped, flippedRows := run(ReferenceOptions{ReverseSources: true, ReverseResponses: true})
	for _, table := range []string{"input_ledger", "decision_ledger", "event_store"} {
		a, b := originalRows[table], flippedRows[table]
		if reflect.DeepEqual(a, b) { continue }
		t.Logf("D2 divergence %s: record counts original=%d flipped=%d; NoProposal journals equal=%t", table, len(a), len(b), original.ScriptedNoProposalJournalID == flipped.ScriptedNoProposalJournalID)
		for i := 0; i < len(a) || i < len(b); i++ {
			left, right := "<missing>", "<missing>"
			if i < len(a) { left = a[i] }
			if i < len(b) { right = b[i] }
			if left != right { t.Logf("first divergent %s row index=%d original=%s flipped=%s", table, i, left, right); break }
		}
	}
	if original.RunID != flipped.RunID ||
		!reflect.DeepEqual(original.PhysicalEventIDs, flipped.PhysicalEventIDs) ||
		!reflect.DeepEqual(original.PhysicalJournalIDs, flipped.PhysicalJournalIDs) ||
		!reflect.DeepEqual(original.PhysicalInputIDs, flipped.PhysicalInputIDs) ||
		!reflect.DeepEqual(original.FiveLedgerDigests, flipped.FiveLedgerDigests) ||
		original.FinalWorldHash != flipped.FinalWorldHash ||
		original.GenesisSHA256 != flipped.GenesisSHA256 {
		t.Fatalf("D2 permutation divergence: RunID %t, physical events %t, journals %t, inputs %t, five ledgers %t, world hash %t, Genesis %t; C4 five-ledger equality %t; input original=%v flipped=%v; journal original=%v flipped=%v; event original=%v flipped=%v; C4 prefix original=%v flipped=%v; ledger original=%v flipped=%v",
			original.RunID == flipped.RunID,
			reflect.DeepEqual(original.PhysicalEventIDs, flipped.PhysicalEventIDs),
			reflect.DeepEqual(original.PhysicalJournalIDs, flipped.PhysicalJournalIDs),
			reflect.DeepEqual(original.PhysicalInputIDs, flipped.PhysicalInputIDs),
			reflect.DeepEqual(original.FiveLedgerDigests, flipped.FiveLedgerDigests),
			original.FinalWorldHash == flipped.FinalWorldHash,
			original.GenesisSHA256 == flipped.GenesisSHA256,
			reflect.DeepEqual(original.C4.CrossAcceptance.LedgerDigests, flipped.C4.CrossAcceptance.LedgerDigests),
			original.PhysicalInputIDs, flipped.PhysicalInputIDs,
			original.PhysicalJournalIDs, flipped.PhysicalJournalIDs,
			original.PhysicalEventIDs, flipped.PhysicalEventIDs,
			original.C4.CrossAcceptance.LedgerDigests, flipped.C4.CrossAcceptance.LedgerDigests,
			original.FiveLedgerDigests, flipped.FiveLedgerDigests)
	}
}

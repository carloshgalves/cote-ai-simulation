//go:build linux

package commit

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/adapter/embodimentv1"
	"github.com/carloshgalves/cote-ai-simulation/internal/physicalhost"
)

// E1 is a REAL C4+D2+D3 comparison, not a disconnected stock/physical
// fixture. Capture two genuinely supervised Python A1 inputs once, then
// hold their exact signed bytes constant across source/worker permutations.
// A separately produced A1 cut contains real wall_time, and is not an
// equivalent exogenous source for byte-identical evidence.
func TestReferenceE1SameSignedSourcesWholeCausalAndOwnerByteTranscript(t *testing.T) {
	ctx := context.Background()
	host, plans := referenceD2TestHost(t)
	genesis, err := buildReferenceGenesis(host.Bundle)
	if err != nil {
		t.Fatal(err)
	}
	host.GenesisCanonical = genesis.CanonicalBytes
	var cuts [2]embodimentv1.ApprovedCut
	for i := range cuts {
		source := host
		source.Epoch = uint64(i + 1)
		source.Plan = plans[i]
		cuts[i], err = physicalhost.RunSupervisedPhysical(ctx, source)
		if err != nil {
			t.Fatalf("genuine supervised A1 epoch %d: %v", i+1, err)
		}
		if _, _, _, err := cuts[i].CanonicalProof(); err != nil {
			t.Fatal(err)
		}
	}
	host.GenesisCanonical = nil

	run := func(options ReferenceOptions) referenceE1Snapshot {
		t.Helper()
		root := t.TempDir()
		private := filepath.Join(root, "owners")
		if err := os.Mkdir(private, 0700); err != nil {
			t.Fatal(err)
		}
		causal := filepath.Join(root, "causal.db")
		bOwner := filepath.Join(private, "actor-b.db")
		proof, err := RunReferenceD3(ctx, causal, bOwner, bundlePath,
			ReferenceD2Config{Host: host, EpochPlans: plans, frozenSupervisedCuts: &cuts}, options)
		if err != nil {
			t.Fatalf("E1 integrated run: %v", err)
		}
		if proof.Status != "RUN_LOCAL_10D3" || proof.Matrix.ReplayCommits != 9 ||
			proof.Matrix.SnapshotTailCommits != 3 {
			t.Fatalf("E1 is not the real completed D3 run: %+v", proof.Matrix)
		}
		if _, err := proof.PublicReport(); err != nil {
			t.Fatal(err)
		}
		snapshot, err := referenceE1Read(ctx, []referenceE1Location{
			{Name: "causal", Path: causal},
			{Name: "owner-a", Path: bOwner + ".actor-a"},
			{Name: "owner-b", Path: bOwner},
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := snapshot.requireCausalAndOwners(); err != nil {
			t.Fatal(err)
		}
		return snapshot
	}
	baseline := run(ReferenceOptions{})
	for i, options := range []ReferenceOptions{
		{ReverseSources: true},
		{ReverseResponses: true},
		{ReverseSources: true, ReverseResponses: true},
	} {
		other := run(options)
		if err := referenceE1Compare(baseline, other); err != nil {
			t.Fatalf("E1 same signed source changes byte transcript in permutation %d: %v", i, err)
		}
		if !reflect.DeepEqual(baseline.Manifest(), other.Manifest()) {
			t.Fatalf("E1 manifest order or digest changed on permutation %d", i)
		}
	}
	public, err := json.Marshal(baseline.Manifest())
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"npc.0001", "physical.advanced", "physical.a1", "private_key",
		"BodyState", "source_raw", "operator_private", "signed_cut", "owner_content"} {
		if bytes.Contains(public, []byte(secret)) {
			t.Fatalf("E1 manifest exposed private source/actor payload token %q", secret)
		}
	}
	// The exported optional artifact is a hash/count-only certificate.
	// Never write underlying rows, private owner bytes or raw A1 cuts.
	if path := os.Getenv("CSF_E1_EVIDENCE_PATH"); path != "" {
		report := struct {
			Status string
			Reference string
			CaptureMode string
			AuthenticatedA1Cuts int
			EquivalentRuns int
			Databases int
			Manifest []referenceE1ManifestEntry
		}{
			Status:"E1_LOCAL_EQUAL_CANONICAL_BYTES",
			Reference:"CSFRV1-10/E1 #140",
			CaptureMode:"two genuine A1 supervised signed cuts reused unchanged across four D3 runs",
			AuthenticatedA1Cuts:2,EquivalentRuns:4,Databases:3,
			Manifest:baseline.Manifest(),
		}
		raw,err:=json.MarshalIndent(report,"","  ")
		if err!=nil{t.Fatal(err)}
		f,err:=os.OpenFile(path,os.O_WRONLY|os.O_CREATE|os.O_EXCL,0600)
		if err!=nil{t.Fatalf("E1 evidence must be new (no overwrite): %v",err)}
		if _,err:=f.Write(append(raw,'\n'));err!=nil{f.Close();t.Fatal(err)}
		if err:=f.Close();err!=nil{t.Fatal(err)}
	}
	t.Logf("E1 verified %d database/table digest entries; two real authenticated cuts, 4 deterministic same-source runs",
		len(baseline.Manifest()))
}

func TestReferenceE1MismatchAndMissingAuthorityFailClosed(t *testing.T) {
	if err := referenceE1Compare(referenceE1Snapshot{}, referenceE1Snapshot{}); err == nil {
		t.Fatal("two empty transcripts falsely proved determinism")
	}
	// A small synthetic transcript exercises the verifier even when Python is
	// unavailable. The positive E1 end-to-end test does require real A1.
	left := referenceE1Snapshot{Databases: map[string]referenceE1Database{
		"causal": {Tables: map[string]referenceE1Table{
			"clock_states": {Rows: [][]byte{{1, 2, 3}}},
			"event_store":  {Rows: [][]byte{{4, 5, 6}}},
		}},
	}}
	right := left.clone()
	if err := referenceE1Compare(left, right); err != nil {
		t.Fatal(err)
	}
	row := right.Databases["causal"].Tables["clock_states"].Rows[0]
	row[0] ^= 0x7f
	if err := referenceE1Compare(left, right); err == nil || !strings.Contains(err.Error(), "clock_states") {
		t.Fatalf("altered causal byte went undetected or diagnostic did not identify table: %v", err)
	}
	right = left.clone()
	delete(right.Databases["causal"].Tables, "event_store")
	if err := referenceE1Compare(left, right); err == nil || !strings.Contains(err.Error(), "event_store") {
		t.Fatalf("missing causal table accepted: %v", err)
	}
	right = left.clone()
	delete(right.Databases, "causal")
	if err := referenceE1Compare(left, right); err == nil || !strings.Contains(err.Error(), "causal") {
		t.Fatalf("missing causal database accepted: %v", err)
	}
}

// Real source hashes are validated independently by D3; a byte-level
// corrupt canonical stored row must ALSO make the E1 read refuse the capture.
func TestReferenceE1RejectsCorruptCanonicalAndReadOnlyCannotWrite(t *testing.T) {
	ctx := context.Background()
	host, plans := referenceD2TestHost(t)
	root := t.TempDir()
	private := filepath.Join(root, "owners")
	if err := os.Mkdir(private, 0700); err != nil {
		t.Fatal(err)
	}
	causal := filepath.Join(root, "causal.db")
	owner := filepath.Join(private, "actor-b.db")
	if _, err := RunReferenceD3(ctx, causal, owner, bundlePath,
		ReferenceD2Config{Host: host, EpochPlans: plans}, ReferenceOptions{}); err != nil {
		t.Fatal(err)
	}
	db, err := openReferenceReadOnlyDB(causal)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "DELETE FROM clock_states WHERE 1=0"); err == nil {
		db.Close()
		t.Fatal("read-only E1 reader unexpectedly gained write authority")
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	snap, err := referenceE1Read(ctx, []referenceE1Location{{Name: "causal", Path: causal}})
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Databases["causal"].Tables["world_revisions"].Rows) != 10 {
		t.Fatal("nine committed D3 transitions did not leave Genesis + nine WorldRevisions")
	}
	// Deliberately damage the first canonical clock row via a separate
	// *test-only* writer. This must not be silently normalized or repaired.
	db, err = openReferenceWritableForE1Test(causal)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE clock_states SET canonical=x'80'"); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := referenceE1Read(ctx, []referenceE1Location{{Name: "causal", Path: causal}}); err == nil {
		t.Fatal("E1 canonical corruption was not fail-closed")
	}
}

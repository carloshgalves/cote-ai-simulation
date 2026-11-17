//go:build linux

package commit

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/ownercheckpoint"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/ownercheckpoint/localowner"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
)

// D3 must use the real supervised A1 pair. The old C4 cut has a non-final
// physical source; the later R9 cut has signed cognitive owners and a physical
// extension pin. Neither may be silently degraded into an ordinary V1/V2 fork.
func TestReferenceE4D3RejectUnfinishedSourceAndUnsupportedPhysicalFork(t *testing.T) {
	ctx := context.Background()
	host, plans, cuts := referenceE2SignedPair(t)
	path, ownerPath := referenceE2Paths(t)
	proof, err := RunReferenceD3(ctx, path, ownerPath, bundlePath,
		ReferenceD2Config{Host: host, EpochPlans: plans, frozenSupervisedCuts: &cuts}, ReferenceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	v1 := host.Bundle
	v2 := referenceE4Bundle(t)
	run := referenceC2Decode32(t, proof.RunID)
	old := referenceC2Decode32(t, proof.d2.C4.Epistemic.SnapshotID)
	parent, err := sqlite.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	verified, err := parent.VerifySnapshotV1(ctx, run, old, v1)
	if err != nil || verified.TailCommits != 3 {
		t.Fatalf("D3 historical pre-closure cut not verified: %v", err)
	}
	childID := run
	childID[0] ^= 0x69
	childPath := filepath.Join(t.TempDir(), "rejected.db")
	child, err := sqlite.Open(ctx, childPath)
	if err != nil {
		t.Fatal(err)
	}
	defer child.Close()
	before := referenceE3Capture(t, path, ownerPath)
	// Historical generic V2 fork checks closure AS OF the selected snapshot,
	// even though the live parent source later reached final closure.
	if _, err := parent.ForkIdleSnapshotV1ToV2(ctx, run, old, childID, child, v1, v2); err == nil || !strings.Contains(err.Error(), "unfinished") {
		t.Fatalf("pre-final physical source not denied at its historical cut: %v", err)
	}
	assertCSFRV18EmptyRejectedFork(t, childPath)
	referenceE4Equal(t, before, referenceE3Capture(t, path, ownerPath))
	state, err := parent.LoadClock(ctx, run)
	if err != nil || state.Revision != 9 {
		t.Fatalf("D3 tip not settled: %v", err)
	}
	var checkpoints []sqlite.SnapshotCheckpoint
	for _, actor := range []string{"a", "b"} {
		ownerFile, token := ownerPath, proof.d2.C4.Epistemic.OwnerReceiptID
		if actor == "a" {
			ownerFile += ".actor-a"
			token = proof.d2.C4.Epistemic.AOwnerReceiptID
		}
		cp, _, err := parent.LoadCognitiveCheckpointV1(ctx, run, referenceC2Decode32(t, token), "school", actor, v1)
		if err != nil {
			t.Fatal(err)
		}
		// Same opaque synthetic bytes; updated coverage is signed by the
		// fixture owner, never manufactured by the causal engine.
		cp.CoversInstant = int64(state.Instant)
		cp.CoversRevision = 9
		cp.CoversEvidenceCursor = uint64(len(before.Databases["causal"].Tables["evidence_ledger"].Rows))
		o, err := localowner.Open(ctx, ownerFile, referenceC2SyntheticSigner(run, actor))
		if err != nil {
			t.Fatal(err)
		}
		defer o.Close()
		gate, err := ownercheckpoint.NewGate(o, parent, v1)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := gate.Stage(ctx, ownercheckpoint.Scope{Run: run, Namespace: "school", Actor: actor}, cp); err != nil {
			t.Fatal(err)
		}
		checkpoints = append(checkpoints, cp)
	}
	final, err := parent.CreateSnapshotV1AtRevision(ctx, run, v1, checkpoints, 9)
	if err != nil {
		t.Fatal(err)
	}
	settled, err := parent.VerifySnapshotV1(ctx, run, final.ID, v1)
	if err != nil || !settled.OwnerWorkReady {
		t.Fatalf("D3 final snapshot lacks settled epistemic barrier: %v", err)
	}
	before = referenceE3Capture(t, path, ownerPath)
	if _, err := parent.ForkIdleSnapshotV1(ctx, run, final.ID, childID, child, v1); err == nil || !strings.Contains(err.Error(), "opaque cognitive checkpoints") {
		t.Fatalf("same-policy V1 unsupported owner migration not denied: %v", err)
	}
	assertCSFRV18EmptyRejectedFork(t, childPath)
	if _, err := v2.PhysicalRegistration(); err == nil {
		t.Fatal("test premise changed: V2 now registers physical A1; review E4 contract")
	}
	if _, err := parent.ForkClosedReferenceV1ToV2(ctx, run, final.ID, childID, child, v1, v2); err == nil {
		t.Fatal("unregistered physical extension accepted by V2 child")
	}
	assertCSFRV18EmptyRejectedFork(t, childPath)
	referenceE4Equal(t, before, referenceE3Capture(t, path, ownerPath))
}

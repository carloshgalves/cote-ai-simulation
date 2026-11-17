//go:build linux

package commit

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/admission"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/inputledger"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
)

// A1 has completed and its source is durable, but no AdmissionFence exists.
// Reopening this exact file must not implicitly admit, commit or resample it.
// The original sealed cut is supplied only for the initial ingress; after
// fencing, a new child process is able to commit without that old capability.
func TestReferenceE2PreFenceReopenAndAdmitSameSignedSource(t *testing.T) {
	ctx := context.Background()
	_, original, priorLedger, cycle, _, approved, _, host := realPhysicalFixture(t)
	_, _, _, _, _, run := priorLedger.Snapshot()
	g, err := original.ValidatedGenesis(ctx, [32]byte(run), host.Bundle)
	if err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "pre-fence.db")
	first, err := sqlite.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.PersistGenesis(ctx, g, host.Bundle); err != nil {
		t.Fatal(err)
	}
	if err := first.BootstrapClock(ctx, g, host.Bundle); err != nil {
		t.Fatal(err)
	}
	if err := first.BootstrapGenesisTriggers(ctx, g, host.Bundle, nil); err != nil {
		t.Fatal(err)
	}
	refs, err := policiesByRole(host.Bundle)
	if err != nil {
		t.Fatal(err)
	}
	policies := admission.Policies{
		AdmissionOrder: refs["admission-order"],
		CausalIdentity: refs["causal-identity"],
		Fence: refs["admission-fence"],
		Rules: [][]any{},
	}
	ingress, err := inputledger.New(ctx, run, host.Bundle, first)
	if err != nil {
		t.Fatal(err)
	}
	initialCoordinator := New(first, host.Bundle, policies)
	if _, err := initialCoordinator.StagePhysicalInput(ctx, ingress, approved, cycle); err != nil {
		t.Fatal(err)
	}
	before := referenceE2CausalTranscript(t, path)
	if referenceE2Count(t, path, "committed_cycles") != 0 ||
		referenceE2Count(t, path, "world_revisions") != 1 ||
		referenceE2Count(t, path, "event_store") != 0 {
		t.Fatal("pre-fence ingress published a terminal or physical world event")
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	// Fresh SQLite+ledger after abandoning the prior coordinator. Source
	// closure, ingress receipt and eligibility must survive exactly.
	reopened, err := sqlite.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := inputledger.Open(ctx, run, host.Bundle, reopened)
	if err != nil {
		reopened.Close()
		t.Fatal(err)
	}
	if err := referenceE1Compare(before, referenceE2CausalTranscript(t, path)); err != nil {
		reopened.Close()
		t.Fatalf("pre-fence cold reopen changed committed source/cursor: %v", err)
	}
	state, err := reopened.LoadClock(ctx, [32]byte(run))
	if err != nil || state.Revision != 0 {
		reopened.Close()
		t.Fatalf("pre-fence restart advanced the logical clock: %+v %v", state, err)
	}
	// Repeated source publication is prohibited or identical; a retry must
	// never append a second physical input/receipt.
	candidate := New(reopened, host.Bundle, policies)
	_, retryErr := candidate.StagePhysicalInput(ctx, loaded, approved, cycle)
	if retryErr != nil {
		reopened.Close()
		t.Fatalf("re-staging the SAME authenticated cut after restart was not idempotent: %v", retryErr)
	}
	if err := referenceE1Compare(before, referenceE2CausalTranscript(t, path)); err != nil {
		reopened.Close()
		t.Fatalf("re-staging the SAME physical cut duplicated ingress: %v", err)
	}
	fence, err := admission.NewCoordinator(admission.NewFenceLog(reopened)).
		AdmitThenEvaluate(ctx, host.Bundle, loaded, cycle, policies, nil)
	if err != nil {
		reopened.Close()
		t.Fatal(err)
	}
	if len(fence.Canonical) == 0 {
		reopened.Close()
		t.Fatal("fence after cold reopen not durably published")
	}
	snapshot, err := reopened.CreateSnapshotV1(ctx, [32]byte(run), host.Bundle, nil)
	if err != nil {
		reopened.Close()
		t.Fatal(err)
	}
	if err := reopened.Close(); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	manifest, err := host.Bundle.PhysicalManifestCanonical()
	if err != nil {
		t.Fatal(err)
	}
	pin := filepath.Join(dir, "public-a1-manifest.json")
	if err := os.WriteFile(pin, manifest, 0600); err != nil {
		t.Fatal(err)
	}
	referenceE2CrashAfterTerminal(t, path, pin, [32]byte(run), [32]byte(snapshot.ID))
	terminal, err := sqlite.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer terminal.Close()
	if _, err := terminal.ReplayClock(ctx, [32]byte(run), host.Bundle); err != nil {
		t.Fatal(err)
	}
	if referenceE2Count(t, path, "committed_cycles") != 1 ||
		referenceE2Count(t, path, "event_store") != 1 {
		t.Fatal("pre-fence recovery created missing or duplicate physical terminal")
	}
	// Explicitly no new Python A1 call is made after the first original
	// supervised production in realPhysicalFixture.
}

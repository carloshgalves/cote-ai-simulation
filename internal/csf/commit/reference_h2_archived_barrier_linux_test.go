//go:build linux

package commit

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/adapter/embodimentv1"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/inputledger"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
	"github.com/carloshgalves/cote-ai-simulation/internal/physicalhost"
)

const h2BarrierPendingExit = 88
const h2BarrierCompletedExit = 89

// Exercise a real pending delivery before C2 dispatch, then finish the same
// C4+D2+D3 scenario using the archive's exact signed physical cuts. The
// simulation driver waits at the harness hook while separate perception
// workers terminate and reopen; this is not a full driver-process restart.
func h2ArchivedBarrierRecovery(t *testing.T, root, indexHash string,
	host physicalhost.ProducerHost, cuts [2]embodimentv1.ApprovedCut, expected referenceE1Snapshot) {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	db := filepath.Join(dir, "causal.db")
	// testing.T.TempDir may create its numbered child with 0755. C2
	// deliberately refuses a non-private immediate owner directory.
	private := filepath.Join(dir, "owner")
	if err := os.Mkdir(private, 0700); err != nil {
		t.Fatal("H2-C could not create private owner directory")
	}
	owner := filepath.Join(private, "actor-b.db")
	hookCalls := 0
	opts := ReferenceOptions{
		referenceBeforeDeliveryPerception: func(_ context.Context, path string) error {
			if path != db || hookCalls != 0 {
				return errors.New("H2-C barrier hook invoked outside the intended reference cut")
			}
			hookCalls++
			before, err := referenceE1Read(ctx, []referenceE1Location{{Name: "causal", Path: db}})
			if err != nil {
				return err
			}
			h2BarrierProcess(t, root, indexHash, db, "pending", h2BarrierPendingExit)
			after, err := referenceE1Read(ctx, []referenceE1Location{{Name: "causal", Path: db}})
			if err != nil {
				return err
			}
			if err := referenceE1Compare(before, after); err != nil {
				return errors.New("H2-C interrupted perception worker changed pending authority")
			}
			h2BarrierProcess(t, root, indexHash, db, "complete", h2BarrierCompletedExit)
			return nil
		},
	}
	proof, err := RunReferenceD3(ctx, db, owner, bundlePath,
		ReferenceD2Config{Host: host, frozenSupervisedCuts: &cuts}, opts)
	if err != nil {
		// Preserve stage-level diagnostics without exposing private record
		// contents in GitHub Actions logs.
		stage := "unclassified reference failure"
		for _, candidate := range []string{
			"C2 owner store requires a private 0700 directory",
			"H2-C barrier hook invoked outside the intended reference cut",
			"H2-C interrupted perception worker changed pending authority",
			"C1 send barrier", "C1 terminal clock coordinate invalid",
			"C1 B KnowledgeInput delta", "C1 integrated causal replay mismatch",
			"C4 restored perception barrier", "D2 prerequisite C4 failed",
			"D2 atomic CommitPhysical", "D3 missing real D2 baseline",
		} {
			if strings.Contains(err.Error(), candidate) {
				stage = candidate
				break
			}
		}
		t.Fatalf("H2-C pending-barrier D3 failed: stage=%s hook_calls=%d", stage, hookCalls)
	}
	if proof.Status != "RUN_LOCAL_10D3" || hookCalls != 1 {
		t.Fatalf("H2-C pending-barrier scenario did not finish the real D3 run: hook_calls=%d", hookCalls)
	}
	if _, err := proof.PublicReport(); err != nil {
		t.Fatal("H2-C pending-barrier scenario did not pass disclosure")
	}
	actual, err := referenceE1Read(ctx, []referenceE1Location{
		{Name: "causal", Path: db},
		{Name: "owner-a", Path: owner + ".actor-a"},
		{Name: "owner-b", Path: owner},
	})
	if err != nil {
		t.Fatal("H2-C final pending-barrier authority unavailable")
	}
	if err := actual.requireCausalAndOwners(); err != nil {
		t.Fatal("H2-C final pending-barrier authority incomplete")
	}
	if err := referenceE1Compare(expected, actual); err != nil {
		t.Fatal("H2-C pending-barrier recovery differs from the original archive")
	}
}

func h2BarrierProcess(t *testing.T, root, indexHash, db, mode string, code int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestH2ArchivedBarrierWorkerChild$")
	cmd.Env = []string{
		"H2_C_BARRIER_CHILD=" + mode,
		"H2_ARCHIVED_SOURCE=" + root,
		"H2_ARCHIVED_INDEX_SHA256=" + indexHash,
		"H2_C_BARRIER_DB=" + db,
		"GORACE=halt_on_error=1",
	}
	output, err := cmd.CombinedOutput()
	var exited *exec.ExitError
	if ctx.Err() != nil || !errors.As(err, &exited) || exited.ExitCode() != code || len(output) != 0 {
		t.Fatalf("H2-C isolated barrier worker did not reach expected outcome: %s", mode)
	}
}

// Both workers independently reopen the real persisted cut. No producer key
// is provided. The first exits with the barrier still closed; the second
// uses native C1 projection and verifies idempotence before exiting.
func TestH2ArchivedBarrierWorkerChild(t *testing.T) {
	mode := os.Getenv("H2_C_BARRIER_CHILD")
	if mode == "" {
		return
	}
	if mode != "pending" && mode != "complete" {
		t.Fatal("H2-C invalid barrier worker mode")
	}
	ctx := context.Background()
	host, _, _, _, _ := h2ArchivedInputs(t, os.Getenv("H2_ARCHIVED_SOURCE"), os.Getenv("H2_ARCHIVED_INDEX_SHA256"))
	g, err := buildReferenceGenesis(host.Bundle)
	if err != nil {
		t.Fatal("H2-C barrier Genesis unavailable")
	}
	path := os.Getenv("H2_C_BARRIER_DB")
	store, err := sqlite.Open(ctx, path)
	if err != nil {
		t.Fatal("H2-C barrier worker could not reopen causal authority")
	}
	validated, err := store.ValidatedGenesis(ctx, [32]byte(g.RunID), host.Bundle)
	if err != nil || validated.Digest != g.Digest {
		t.Fatal("H2-C barrier worker Genesis mismatch")
	}
	state, err := store.LoadClock(ctx, [32]byte(g.RunID))
	if err != nil || state.Revision != 6 || state.Instant != 35 || state.LastOrdinal != 0 {
		t.Fatal("H2-C worker is not at committed C1 delivery cut")
	}
	capture := func() referenceE1Snapshot {
		t.Helper()
		s, err := referenceE1Read(ctx, []referenceE1Location{{Name: "causal", Path: path}})
		if err != nil {
			t.Fatal("H2-C barrier worker could not read canonical authority")
		}
		return s
	}
	before := capture()
	bInputs, err := referenceC1BInputs(ctx, path)
	if err != nil {
		t.Fatal("H2-C worker could not count B evidence")
	}
	// A genuine absent completion (not malformed state or a generic
	// error) must block both the barrier and the actual next-round path.
	const missing = "dispatch held: prior perception has no durable completion"
	if err := store.CheckPerceptionBarrier(ctx, [32]byte(g.RunID), 6); err == nil || !strings.HasSuffix(err.Error(), missing) {
		t.Fatal("H2-C expected exact pending-perception denial")
	}
	ledger, err := inputledger.Open(ctx, g.RunID, host.Bundle, store)
	if err != nil {
		t.Fatal("H2-C barrier input authority unavailable")
	}
	for _, actor := range []struct {
		name    string
		ordinal uint32
	}{{"b", 3}, {"a", 4}} {
		if _, err := referenceC2NextDispatch(ctx, store, ledger, host.Bundle, g.RunID, actor.name, actor.ordinal); err == nil ||
			!strings.HasSuffix(err.Error(), missing) {
			t.Fatal("H2-C pending barrier did not block actual actor dispatch")
		}
	}
	if err := referenceE1Compare(before, capture()); err != nil {
		t.Fatal("H2-C rejected dispatch mutated the pending authority")
	}
	if mode == "pending" {
		// No completion or close is performed by this interrupted worker.
		os.Exit(h2BarrierPendingExit)
	}
	if err := referenceCompleteC1Tasks(ctx, store, host.Bundle, path, g.RunID, 6); err != nil {
		t.Fatal("H2-C restarted worker failed registered perception completion")
	}
	if n, err := referenceC1BInputs(ctx, path); err != nil || n != bInputs+1 {
		t.Fatal("H2-C restarted worker did not add exactly one B KnowledgeInput")
	}
	if err := store.CheckPerceptionBarrier(ctx, [32]byte(g.RunID), 6); err != nil {
		t.Fatal("H2-C verified completion did not open barrier")
	}
	after := capture()
	if err := referenceCompleteC1Tasks(ctx, store, host.Bundle, path, g.RunID, 6); err != nil {
		t.Fatal("H2-C registered perception retry failed")
	}
	if err := referenceE1Compare(after, capture()); err != nil {
		t.Fatal("H2-C repeated completion mutated authority")
	}
	if err := store.Close(); err != nil {
		t.Fatal("H2-C completed worker could not close causal authority")
	}
	os.Exit(h2BarrierCompletedExit)
}

package commit

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/inputledger"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/trigger"
)

func r21EvolvedTriggerSource(t *testing.T) (*Coordinator, *sqlite.Store, *inputledger.Ledger, domain.ID, *policy.Bundle, sqlite.SnapshotV1, string) {
	t.Helper()
	ctx := context.Background()
	rising := activeTrigger()
	repeating := rising
	repeating.ID[0]++
	repeating.Policy = trigger.RepeatWhileTrue
	repeating.RepeatEvery = 50
	once := rising
	once.ID[0] += 2
	once.Policy = trigger.OnceWhenTrue
	c, parent, ledger, run, b, path := testFixture(t, rising, repeating, once)
	state, err := parent.LoadClock(ctx, [32]byte(run))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.CloseSource(ctx, "driver", inputledger.Closure{ProducerKey: "r21-final", ClosedThrough: domain.EligibilityCoordinate{NotBeforeInstant: state.Instant + 1000}, Final: true}); err != nil {
		t.Fatal(err)
	}
	initial, err := parent.CreateSnapshotV1(ctx, [32]byte(run), b, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Advance(ctx, ledger, run); err != nil {
		t.Fatal(err)
	}
	if _, err := parent.DrainPendingPerceptionV1(ctx, [32]byte(run), initial.ID, b); err != nil {
		t.Fatal(err)
	}
	// Consume the three first activations, then the ordinal-1 Genesis unit.
	for i := 0; i < 2; i++ {
		cycle, fence := r20CurrentWork(t, c, parent, ledger, run)
		if _, err := c.DispatchFencedWorkV1(ctx, ledger, cycle, fence); err != nil {
			t.Fatal(err)
		}
		if _, err := parent.DrainPendingPerceptionV1(ctx, [32]byte(run), initial.ID, b); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := c.Advance(ctx, ledger, run); err != nil {
		t.Fatal(err)
	}
	if _, err := parent.DrainPendingPerceptionV1(ctx, [32]byte(run), initial.ID, b); err != nil {
		t.Fatal(err)
	}
	source, err := parent.LoadRegisteredTriggers(ctx, [32]byte(run), b)
	if err != nil {
		t.Fatal(err)
	}
	if len(source) != 3 {
		t.Fatal("source trigger fixture incomplete")
	}
	if source[1].Runtime.ActivationCount != 2 || !source[2].Runtime.Exhausted {
		t.Fatal("source did not evolve as required")
	}
	snapshot, err := parent.CreateSnapshotV1(ctx, [32]byte(run), b, nil)
	if err != nil {
		t.Fatal(err)
	}
	return c, parent, ledger, run, b, snapshot, path
}

func TestCSFRV18R21RepeatCadenceReplayUsesHistoricalRuntime(t *testing.T) {
	ctx := context.Background()
	_, parent, _, run, b, snapshot, path := r21EvolvedTriggerSource(t)
	report, err := parent.ReplayClock(ctx, [32]byte(run), b)
	if err != nil {
		t.Fatal(err)
	}
	if report.ReplayedCommits != 4 || report.ReplayedTriggerTransitions != 6 {
		t.Fatalf("incomplete repeat history: %+v", report)
	}
	reopened, err := sqlite.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if _, err := reopened.ReplayClock(ctx, [32]byte(run), b); err != nil {
		t.Fatal("repeat history lost after reopen:", err)
	}
	registered, err := reopened.LoadRegisteredTriggers(ctx, [32]byte(run), b)
	if err != nil {
		t.Fatal(err)
	}
	repeat := registered[1]
	bad := repeat.Runtime
	bad.NextRepeat += 500
	corrupt, err := trigger.RuntimeBytes(bad)
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.ExecContext(ctx, "UPDATE trigger_runtime SET canonical=? WHERE run_id=? AND definition_id=?", corrupt, run[:], repeat.Definition.ID[:]); err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.ReplayClock(ctx, [32]byte(run), b); err == nil || !strings.Contains(err.Error(), "runtime differs") {
		t.Fatal("historical planning trusted the corrupted live cadence:", err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE trigger_runtime SET canonical=? WHERE run_id=? AND definition_id=?", repeat.RuntimeCanonical, run[:], repeat.Definition.ID[:]); err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.VerifySnapshotV1(ctx, [32]byte(run), snapshot.ID, b); err != nil {
		t.Fatal("exact repeat runtime restoration failed:", err)
	}
}

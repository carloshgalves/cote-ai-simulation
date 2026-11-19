package commit

import (
	"bytes"
	"context"
	"testing"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/inputledger"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
)

// A retry fence outranks the previous abort; aborting it must halt at N+2,
// not authorize another attempt. All checks use the same historical snapshot.
func TestReferenceG3RetryFencePrecedenceAndSecondAbort(t *testing.T) {
	ctx := context.Background()
	c, store, _, first, run, cycle, path := abortFixture(t)
	snap, err := store.CreateSnapshotV1(ctx, [32]byte(run), c.bundle, nil)
	if err != nil {
		t.Fatal(err)
	}
	abort, err := c.Abort(ctx, run, cycle, first, 4, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Retry(ctx, run, cycle, abort, "operator.retry"); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	cold, err := sqlite.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer cold.Close()
	l, err := inputledger.Open(ctx, run, c.bundle, cold)
	if err != nil {
		t.Fatal(err)
	}
	resumer := New(cold, c.bundle, c.admissionPolicies)
	next, fence, before, err := resumer.ReopenAuthorizedRetry(ctx, l, [32]byte(run), snap.ID)
	if err != nil || next.ID != cycle || next.Attempt != 1 || before.ControlStatus != 2 || fence.InputDigest != first.InputDigest {
		t.Fatal("authorized retry failed to preserve semantic cut", err)
	}
	inflight, err := cold.VerifySnapshotV1(ctx, [32]byte(run), snap.ID, c.bundle)
	if err != nil || inflight.ControlStatus != 1 || inflight.Action.FenceID != [32]byte(fence.ID) || inflight.Action.AttemptOrdinal != 1 {
		t.Fatal("retry fence did not take precedence over previous abort", err)
	}
	if _, _, _, err := resumer.ReopenAuthorizedRetry(ctx, l, [32]byte(run), snap.ID); err == nil {
		t.Fatal("retry was consumed more than once")
	}
	second, err := resumer.Abort(ctx, run, cycle, fence, 4, nil)
	if err != nil || bytes.Equal(second.Canonical, abort.Canonical) || second.Identity == abort.Identity {
		t.Fatal("second attempt did not produce distinct abort", err)
	}
	halted, err := cold.VerifySnapshotV1(ctx, [32]byte(run), snap.ID, c.bundle)
	if err != nil || halted.ControlStatus != 3 || halted.Action.Kind != sqlite.ResumeHalted || halted.TailCommits != 0 {
		t.Fatal("second abort did not restore halted state", err)
	}
	state, err := cold.LoadCycleControl(ctx, [32]byte(run))
	if err != nil || len(state) != 7 || state[0] != uint64(3) || state[6] != uint64(2) {
		t.Fatal("second abort advanced the next attempt incorrectly", err)
	}
	if _, _, _, err := resumer.ReopenFencedCycle(ctx, [32]byte(run), snap.ID); err == nil {
		t.Fatal("second abort authorized implicit replay")
	}
}

// The continuous and recovered runs get the same late input. It is appended
// only after a fence exists and its eligibility is normalized beyond the
// closed target. Compare the actual terminal and event bytes.
func TestReferenceG3FrozenAdvanceAfterAppend(t *testing.T) {
	ctx := context.Background()
	var baseline []byte
	var fenceBytes []byte
	for _, recover := range []bool{false, true} {
		c, store, ledger, run, bundle, path := testFixture(t)
		state, err := store.LoadClock(ctx, [32]byte(run))
		if err != nil {
			t.Fatal(err)
		}
		appendLate := func() {
			t.Helper()
			late, err := ledger.AppendInput(ctx, "driver", inputledger.Input{
				ProducerUnitKey: "late-after-fence", Kind: "external.event", SchemaVersion: 1,
				Actor: []any{uint64(0)}, Payload: []any{}, DeclaredEffectiveAt: state.Instant + 100,
				Eligibility: domain.EligibilityCoordinate{NotBeforeInstant: state.Instant + 100},
				Provenance: []any{uint64(0)}, IdempotencyKey: []any{uint64(0)},
			})
			if err != nil || !referenceC3Same(late.Fields[9], []any{int64(state.Instant + 100), uint64(2)}) {
				t.Fatal("late append did not normalize beyond closed target", err)
			}
		}
		if !recover {
			// Append before planning; the exact same input is already beyond
			// the target closure and must not affect the resulting fence.
			appendLate()
			out, err := c.Advance(ctx, ledger, run)
			if err != nil {
				t.Fatal(err)
			}
			baseline = bytes.Clone(out.Commit.Canonical)
			fenceBytes = bytes.Clone(out.Fence.Canonical)
			continue
		}
		c.failAfterStep = 1
		if _, err := c.Advance(ctx, ledger, run); err == nil {
			t.Fatal("clock rollback did not leave a frozen fence")
		}
		snap, err := store.CreateSnapshotV1(ctx, [32]byte(run), bundle, nil)
		if err != nil {
			t.Fatal(err)
		}
		appendLate()
		if err := store.Close(); err != nil {
			t.Fatal(err)
		}
		cold, err := sqlite.Open(ctx, path)
		if err != nil {
			t.Fatal(err)
		}
		defer cold.Close()
		reopened, err := inputledger.Open(ctx, run, bundle, cold)
		if err != nil {
			t.Fatal(err)
		}
		resumer := New(cold, bundle, c.admissionPolicies)
		result, err := resumer.ResumeRegisteredWorkV1(ctx, reopened, [32]byte(run), snap.ID)
		if err != nil || !result.Continued || !bytes.Equal(result.Publication.Canonical, baseline) ||
			!bytes.Equal(result.Fence.Canonical, fenceBytes) || len(result.Cycle.Plan) != 4 {
			t.Fatal("resume reselected or failed the original CLOCK_ADVANCE", err)
		}
		after, err := cold.LoadClock(ctx, [32]byte(run))
		if err != nil || after.Instant != state.Instant+100 || after.LastOrdinal != -1 || after.Revision != 1 {
			t.Fatal("wrong recovered temporal successor", err)
		}
		again, err := resumer.ResumeRegisteredWorkV1(ctx, reopened, [32]byte(run), snap.ID)
		if err != nil || again.Continued {
			t.Fatal("recovery duplicated the terminal", err)
		}
	}
}

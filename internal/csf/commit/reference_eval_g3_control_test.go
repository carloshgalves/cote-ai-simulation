package commit

import (
	"bytes"
	"context"
	"testing"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/admission"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/clock"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/genesis"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/inputledger"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
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
				Actor: []any{uint64(0)}, Payload: validGenesis(t, bundle).Payload[16], DeclaredEffectiveAt: state.Instant + 100,
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

// The scheduler must not create a decorative empty commit for an ordinal
// hole. Verify two real interrupted-coordinate histories plus their replay.
func TestReferenceG3OrdinalGapAndAdvance(t *testing.T) {
	ctx := context.Background()
	for _, advance := range []bool{false, true} {
		bundle, err := policy.Load(bundlePath)
		if err != nil {
			t.Fatal(err)
		}
		c, store, ledger, cycle, first, path := systemWorkFixture(t, "reject", func(s *sqlite.Store, g *genesis.Genesis) {
			d := identity.New(bundle)
			origin := []any{uint64(2), []any{g.ID[:], g.Digest[:]}}
			for _, n := range []uint64{1, 3} {
				at := domain.EligibilityCoordinate{NotBeforeInstant: g.InitialInstant, NotBeforeCycleOrdinal: domain.CycleOrdinal(n)}
				if advance && n == 3 {
					at.NotBeforeInstant += 100
					at.NotBeforeCycleOrdinal = 0
				}
				id, _, err := d.Sum("cote.csf.id.occurrence", "cote.csf.schema.occurrence-id-preimage", 1, []any{origin, uint64(0), n})
				if err != nil {
					t.Fatal(err)
				}
				fields := []any{id[:], g.RunID[:], "schedule", int64(at.NotBeforeInstant),
					[]any{int64(at.NotBeforeInstant), uint64(at.NotBeforeCycleOrdinal)}, "test.occurrence", uint64(1),
					g.Payload[16], uint64(0), origin, uint64(0), n, []any{uint64(0)}, []any{uint64(0)}}
				digest, _, err := d.Sum("cote.csf.digest.occurrence-unit", "cote.csf.schema.scheduled-occurrence.body", 1, fields)
				if err != nil {
					t.Fatal(err)
				}
				raw, err := codec.CanonicalPayload(append(cloneAny(fields), digest[:]))
				if err != nil {
					t.Fatal(err)
				}
				if err := s.SeedGenesisOccurrence(ctx, domain.ID(id), g.RunID, at, raw); err != nil {
					t.Fatal(err)
				}
			}
		})
		run := ledger.RunID()
		if _, err := c.CommitSystemUnit(ctx, ledger, cycle, first); err != nil {
			t.Fatal(err)
		}
		work := func(want uint64) {
			t.Helper()
			state, err := store.LoadClock(ctx, [32]byte(run))
			if err != nil {
				t.Fatal(err)
			}
			pending, err := store.PendingClock(ctx, [32]byte(run))
			if err != nil {
				t.Fatal(err)
			}
			plan, ok, err := clock.Next(state, pending)
			if err != nil || !ok || plan.Kind != clock.Work || uint64(plan.OriginOrdinal) != want {
				t.Fatal("wrong next WORK coordinate", plan, err)
			}
			cut, err := referenceC1Cycle(ctx, store, bundle, run, state, plan.OriginOrdinal)
			if err != nil {
				t.Fatal(err)
			}
			fence, err := admission.NewCoordinator(admission.NewFenceLog(store)).AdmitThenEvaluate(ctx, bundle, ledger, cut, c.admissionPolicies, nil)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := c.CommitSystemUnit(ctx, ledger, cut, fence); err != nil {
				t.Fatal(err)
			}
		}
		work(1)
		if advance {
			out, err := c.Advance(ctx, ledger, run)
			if err != nil || out.Plan.Kind != clock.ClockAdvance || out.Plan.OriginOrdinal != 2 ||
				out.ResultClock.Instant != cycle.Instant+100 || out.ResultClock.LastOrdinal != -1 ||
				len(out.Fence.Fields[11].([]any)) != 0 {
				t.Fatal("wrong explicit clock-advance across gap", err)
			}
			work(0)
		} else {
			work(3)
		}
		state, err := store.LoadClock(ctx, [32]byte(run))
		if err != nil {
			t.Fatal(err)
		}
		wantCommits, wantOrdinal := uint64(3), int64(3)
		if advance {
			wantCommits, wantOrdinal = 4, 0
		}
		if uint64(state.Revision) != wantCommits || state.LastOrdinal != wantOrdinal {
			t.Fatal("decorative commit or wrong ordinal was published")
		}
		if err := store.Close(); err != nil {
			t.Fatal(err)
		}
		cold, err := sqlite.Open(ctx, path)
		if err != nil {
			t.Fatal(err)
		}
		replay, err := cold.ReplayClock(ctx, [32]byte(run), bundle)
		cold.Close()
		if err != nil || uint64(replay.ReplayedCommits) != wantCommits || replay.ReconstructedClock != state {
			t.Fatal("cold replay changed gap/advance history", err)
		}
	}
}

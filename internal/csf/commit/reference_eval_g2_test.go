package commit

import (
	"bytes"
	"context"
	"fmt"
	"errors"
	"reflect"
	"testing"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/admission"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/clock"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/decision"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/extensions/testresourcev1"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/inputledger"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/trigger"
)

// Read actual durable authority through an independent verified projection,
// and compare exact Event bytes obtained from the store, not report counts.
func g2EventBytes(t *testing.T, store *sqlite.Store, refs [][32]byte) [][]byte {
	t.Helper()
	out := make([][]byte, 0, len(refs))
	for _, id := range refs {
		raw, err := store.ReadClockEvent(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, raw)
	}
	return out
}

func TestReferenceG2EarlyResponseCohort(t *testing.T) {
	ctx := context.Background()
	var baseFence, baseCommit []byte
	var baseEvents [][]byte
	for i, spec := range []struct{ reverse, early bool }{{false, false}, {true, false}, {false, true}} {
		c, store, ledger, cycle, fence, path := twoResourceContestFixtureWithSystem(t, spec.reverse, false, false, spec.early)
		_, _, _, rounds, slots, run := ledger.Snapshot()
		if len(rounds) != 2 || len(slots) != 2 || len(fence.Fields[10].([]any)[1].([]any)) != 2 ||
			len(fence.Fields[11].([]any)) != 2 {
			t.Fatal("incomplete real co-admission")
		}
		for _, round := range rounds {
			r := slots[round.ID]["act"]
			if r.Kind != 0 || r.SubmittedAgainstRevision != 0 || r.CycleID != cycle.ID ||
				r.EffectiveAt != cycle.Instant || r.RoundID != round.ID {
				t.Fatal("contender not bound to common snapshot")
			}
		}
		journal, err := c.CommitTwoResourceContest(ctx, ledger, cycle, fence)
		if err != nil {
			t.Fatal(err)
		}
		q, err := sqlite.OpenPrivilegedObservatory(ctx, path, c.bundle)
		if err != nil {
			t.Fatal(err)
		}
		report, err := q.ReadPrivilegedReport(ctx, [32]byte(run))
		q.Close()
		if err != nil || len(report.Commits) != 1 || len(report.Commits[0].DecisionLinks) != 2 ||
			report.Commits[0].BaseRevision != 0 || report.Commits[0].ResultRevision != 1 {
			t.Fatal("one common terminal did not settle both contenders", err)
		}
		a, b := report.Commits[0].DecisionLinks[0], report.Commits[0].DecisionLinks[1]
		if len(a.Conflicts) != 1 || len(b.Conflicts) != 1 || a.Conflicts[0] != b.Conflicts[0] ||
			a.Candidate == nil || b.Candidate == nil || a.Candidate.ID == b.Candidate.ID {
			t.Fatal("rounds did not compete in one conflict space")
		}
		eventIDs := make([][32]byte, 0, len(report.Commits[0].Events))
		for _, v := range report.Commits[0].Events {
			eventIDs = append(eventIDs, v.ID)
		}
		events := g2EventBytes(t, store, eventIDs)
		if len(events) != 3 {
			t.Fatal("unexpected event batch")
		}
		if i == 0 {
			baseFence, baseCommit, baseEvents = fence.Canonical, journal.Canonical, events
		} else if !bytes.Equal(baseFence, fence.Canonical) || !bytes.Equal(baseCommit, journal.Canonical) ||
			!reflect.DeepEqual(baseEvents, events) {
			t.Fatal("early/reverse response timing changed causal result")
		}
	}
}

func TestReferenceG2NoProposalExactLifecycle(t *testing.T) {
	ctx := context.Background()
	c, store, ledger, cycle, fence, path := noProposalWorkFixture(t)
	_, _, _, rounds, slots, run := ledger.Snapshot()
	if len(rounds) != 2 {
		t.Fatal("missing round cohort")
	}
	journal, err := c.CommitNoProposal(ctx, ledger, cycle, fence)
	if err != nil {
		t.Fatal(err)
	}
	q, err := sqlite.OpenPrivilegedObservatory(ctx, path, c.bundle)
	if err != nil {
		t.Fatal(err)
	}
	report, err := q.ReadPrivilegedReport(ctx, [32]byte(run))
	q.Close()
	if err != nil || len(report.Commits) != 1 {
		t.Fatal("missing canonical commit", err)
	}
	commit := report.Commits[0]
	if commit.Commit.ID != journal.Identity || len(commit.DecisionLinks) != 2 || len(commit.Events) != 2 ||
		commit.BaseWorldHash != commit.ResultWorldHash || len(commit.Tasks) != 0 {
		t.Fatal("NoProposal batch lacks exact lifecycle/decision cardinality")
	}
	want := map[domain.ID]domain.ID{}
	for _, round := range rounds {
		want[round.ID] = slots[round.ID]["act"].ID
	}
	seen := map[domain.ID]bool{}
	for _, event := range commit.Events {
		raw, err := store.ReadClockEvent(ctx, event.ID)
		if err != nil {
			t.Fatal(err)
		}
		v, err := codec.StrictDecodePayload(raw)
		if err != nil {
			t.Fatal(err)
		}
		f := v.([]any)
		if len(f) != 19 || f[3] != "round.consumed" {
			t.Fatal("NoProposal generated domain event")
		}
		env, err := codec.StrictDecodeEnvelope(f[9].([]any)[2].([]byte))
		if err != nil || env.DomainTag != "cote.csf.round.consumed.v1" {
			t.Fatal("missing lifecycle envelope", err)
		}
		fields := env.Value.([]any)
		var roundID domain.ID
		copy(roundID[:], fields[0].([]byte))
		unit, ok := want[roundID]
		refs := fields[2].([]any)
		if !ok || seen[roundID] || fields[1] != uint64(2) || len(refs) != 1 ||
			!bytes.Equal(refs[0].([]any)[2].([]byte), unit[:]) {
			t.Fatal("round lifecycle does not consume precisely its slot")
		}
		seen[roundID] = true
	}
	decisionSeen := map[domain.ID]bool{}
	for _, entry := range commit.DecisionLinks {
		unit := domain.ID(entry.Source.ID)
		if entry.Status != 3 || entry.Candidate != nil || len(entry.ProducedEvents) != 0 || decisionSeen[unit] {
			t.Fatal("NoProposal gained candidate or duplicate disposition")
		}
		decisionSeen[unit] = true
	}
	for _, unit := range want {
		if !decisionSeen[unit] {
			t.Fatal("slot missing terminal NoProposal")
		}
	}
	if report.State.Clock.Revision != 1 || report.State.Clock.LastOrdinal != 0 ||
		report.State.Clock.NextLogicalSequence != 2 || report.PendingUnits != 0 {
		t.Fatal("replay did not restore terminal rounds")
	}
}

func g2NoProposal(t *testing.T, bundle *policy.Bundle, d inputledger.Dispatch) inputledger.Response {
	t.Helper()
	deriver := identity.New(bundle)
	id, _, err := deriver.Sum("cote.csf.id.no-proposal", "cote.csf.schema.slot-response-id-preimage", 1, []any{d.ID[:], uint64(1)})
	if err != nil {
		t.Fatal(err)
	}
	body := []any{id[:], d.ID[:], d.RunID[:], d.CycleID[:], d.RoundID[:], d.SlotID,
		[]any{d.Actor[0], d.Actor[1]}, int64(d.EffectiveAt), uint64(d.BaseRevision), uint64(0)}
	digest, _, err := deriver.Sum("cote.csf.digest.no-proposal-unit", "cote.csf.schema.no-proposal.body", 1, body)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := codec.CanonicalPayload(append(cloneAny(body), digest[:]))
	if err != nil {
		t.Fatal(err)
	}
	return inputledger.Response{Kind: 1, ID: domain.ID(id), Digest: digest, Canonical: raw, Body: body,
		DispatchID: d.ID, RunID: d.RunID, CycleID: d.CycleID, RoundID: d.RoundID, SlotID: d.SlotID,
		Actor: d.Actor, EffectiveAt: d.EffectiveAt, SubmittedAgainstRevision: d.BaseRevision}
}

func TestReferenceG2SlotCASAndClosedSource(t *testing.T) {
	ctx := context.Background()
	for _, timeoutFirst := range []bool{true, false} {
		c, store, ledger, run, bundle, path := freshResourceWorkFixture(t)
		g := validGenesis(t, bundle)
		state, err := store.LoadClock(ctx, [32]byte(run))
		if err != nil {
			t.Fatal(err)
		}
		at := domain.EligibilityCoordinate{NotBeforeInstant: state.Instant}
		origin := []any{uint64(2), []any{g.ID[:], g.Digest[:]}}
		round, err := ledger.DeclareRound(ctx, "driver", [2]string{"school", "a"}, []string{"act"}, at, origin, 0, 0, 0, []any{uint64(0)})
		if err != nil {
			t.Fatal(err)
		}
		d := identity.New(bundle)
		cid, _, err := d.Sum("cote.csf.id.cycle", "cote.csf.schema.cycle-id-preimage", 1, []any{run[:], []any{int64(state.Instant), uint64(0)}})
		if err != nil {
			t.Fatal(err)
		}
		did, _, err := d.Sum("cote.csf.id.dispatch", "cote.csf.schema.dispatch-id-preimage", 1,
			[]any{run[:], cid[:], round.ID[:], "act", uint64(0)})
		if err != nil {
			t.Fatal(err)
		}
		dispatch := inputledger.Dispatch{ID: domain.ID(did), RunID: run, CycleID: domain.ID(cid), RoundID: round.ID,
			SlotID: "act", Actor: round.Actor, Eligibility: at, BaseRevision: 0, EffectiveAt: state.Instant}
		if err := ledger.RegisterDispatch(ctx, dispatch, inputledger.CycleBinding{ID: domain.ID(cid), BaseRevision: 0, Instant: state.Instant}); err != nil {
			t.Fatal(err)
		}
		proposal, err := testresourcev1.ProposalForDispatch(bundle, dispatch)
		if err != nil {
			t.Fatal(err)
		}
		timeout := g2NoProposal(t, bundle, dispatch)
		// Mismatches must not populate the slot even when the expected
		// response ID is derived correctly from the genuine dispatch.
		for _, mutate := range []func(*inputledger.Response){
			func(r *inputledger.Response) { r.Actor = [2]string{"school", "b"} },
			func(r *inputledger.Response) { r.RoundID[0] ^= 1 },
			func(r *inputledger.Response) { r.SlotID = "other" },
			func(r *inputledger.Response) { r.SubmittedAgainstRevision++ },
			func(r *inputledger.Response) { r.EffectiveAt++ },
			func(r *inputledger.Response) { r.RunID[0] ^= 1 },
			func(r *inputledger.Response) { r.CycleID[0] ^= 1 },
		} {
			bad := proposal
			mutate(&bad)
			if err := ledger.Respond(ctx, bad); !errors.Is(err, inputledger.ErrSlotMismatch) {
				t.Fatal("foreign response admitted", err)
			}
		}
		_, _, _, _, slots, _ := ledger.Snapshot()
		if len(slots[round.ID]) != 0 {
			t.Fatal("mismatch filled slot")
		}
		_, _, cursor := ledger.Prefix("driver")
		first, second := proposal, timeout
		if timeoutFirst {
			first, second = timeout, proposal
		}
		// Open a stale ledger BEFORE the winner to exercise the durable
		// conditional store, rather than just the in-memory response cache.
		other, err := sqlite.Open(ctx, path)
		if err != nil {
			t.Fatal(err)
		}
		stale, err := inputledger.Open(ctx, run, bundle, other)
		if err != nil {
			other.Close()
			t.Fatal(err)
		}
		if err := ledger.Respond(ctx, first); err != nil {
			other.Close()
			t.Fatal(err)
		}
		if err := stale.Respond(ctx, second); !errors.Is(err, inputledger.ErrSlotFilled) {
			other.Close()
			t.Fatal("stale conditional append did not reject second response", err)
		}
		other.Close()
		_, hash, err := store.LoadWorldState(ctx, 0)
		if err != nil {
			t.Fatal(err)
		}
		cycle := admission.Cycle{ID: domain.ID(cid), Instant: state.Instant, Ordinal: 0,
			BaseRevision: 0, BaseStateHash: domain.Digest(hash), RetryRef: []any{uint64(0)}}
		fence, err := admission.NewCoordinator(admission.NewFenceLog(store)).AdmitThenEvaluate(ctx, bundle, ledger, cycle, c.admissionPolicies, nil)
		if err != nil {
			t.Fatal(err)
		}
		units := fence.Fields[11].([]any)
		if len(units) != 1 {
			t.Fatal("expected exactly one slot root in fence")
		}
		u := units[0].([]any)
		if !reflect.DeepEqual(u[0], []any{int64(state.Instant), uint64(0)}) ||
			u[1] != uint64(0) || u[2] != "driver" || !bytes.Equal(u[3].([]byte), first.ID[:]) ||
			!bytes.Equal(u[4].([]byte), first.Digest[:]) {
			t.Fatal("fence failed to bind original closed-coordinate slot winner")
		}
		restored, err := inputledger.Open(ctx, run, bundle, store)
		if err != nil {
			t.Fatal(err)
		}
		_, _, _, _, slots, _ = restored.Snapshot()
		_, _, afterCursor := restored.Prefix("driver")
		if slots[round.ID]["act"].ID != first.ID || afterCursor != cursor {
			t.Fatal("slot response moved source ingress sequence or replaced winner")
		}
		again, err := admission.Derive(bundle, restored, cycle, c.admissionPolicies)
		if err != nil || !bytes.Equal(again.Canonical, fence.Canonical) {
			t.Fatal("persisted response does not rederive exact fence", err)
		}
	}
}

func TestReferenceG2AbortSubjectPermutation(t *testing.T) {
	ctx := context.Background()
	var baseline []byte
	for _, reverse := range []bool{false, true} {
		c, store, ledger, cycle, fence, path := twoResourceContestFixture(t, reverse)
		_, _, _, _, _, run := ledger.Snapshot()
		state, err := store.LoadClock(ctx, [32]byte(run))
		if err != nil {
			t.Fatal(err)
		}
		request, err := decision.FromPersistedFence(ctx, c.bundle, ledger, cycle, c.admissionPolicies, fence, store,
			decision.Snapshot{BaseRevision: 0, ResourceKind: "inventory", ResourceID: "last.resource", Available: 1, Known: false,
				AccessibleActors: map[string]bool{"school:a": true, "school:b": true},
				ForbiddenActors: map[string]bool{"school:a": false, "school:b": false}},
			[]decision.ActionSchema{{Type: "resource.consume", Version: 1, Operation: "resource.consume", Producer: "resource.consume"}},
			domain.Digest{}, nil)
		if err != nil {
			t.Fatal(err)
		}
		evaluated, err := decision.Run(request)
		if !errors.Is(err, decision.ErrIndeterminate) || len(evaluated.Indeterminate) != 2 || len(evaluated.Dispositions) != 0 {
			t.Fatal("missing two actual unknown subjects", err)
		}
		if reverse {
			evaluated.Indeterminate[0], evaluated.Indeterminate[1] = evaluated.Indeterminate[1], evaluated.Indeterminate[0]
		}
		a, b := evaluated.Indeterminate[0], evaluated.Indeterminate[1]
		if a.Subject == b.Subject || a.Facet != b.Facet || a.Reason != b.Reason ||
			!reflect.DeepEqual(a.Evidence, b.Evidence) || !reflect.DeepEqual(a.Validator, b.Validator) {
			t.Fatal("control did not produce matching failures on distinct subjects")
		}
		abort, err := c.Abort(ctx, run, cycle.ID, fence, 0, evaluated.Indeterminate)
		if err != nil {
			t.Fatal(err)
		}
		v, err := codec.StrictDecodePayload(abort.Canonical)
		if err != nil {
			t.Fatal(err)
		}
		fields := v.([]any)
		records := fields[11].([]any)
		if len(fields) != 15 || len(records) != 2 || !referenceC3Same(fields[6], fence.Fields[6]) || !referenceC3Same(fields[13], fence.Fields[16]) {
			t.Fatal("abort omitted subjects or frozen plan")
		}
		expected := map[domain.ID]decision.Indeterminate{a.Subject: a, b.Subject: b}
		for _, raw := range records {
			record := raw.([]any)
			ref := record[0].([]any)
			var id domain.ID
			copy(id[:], ref[1].([]byte))
			item, ok := expected[id]
			if !ok || !bytes.Equal(ref[2].([]byte), item.SubjectDigest[:]) ||
				!reflect.DeepEqual(record[7], item.Validator) {
				t.Fatal("indeterminate record not bound to subject and versioned validator")
			}
			delete(expected, id)
		}
		if len(expected) != 0 {
			t.Fatal("normalization dropped a subject")
		}
		q, err := sqlite.OpenPrivilegedObservatory(ctx, path, c.bundle)
		if err != nil {
			t.Fatal(err)
		}
		report, err := q.ReadPrivilegedReport(ctx, [32]byte(run))
		q.Close()
		if err != nil || len(report.Commits) != 0 || len(report.Attempts) != 1 ||
			report.Attempts[0].Abort == nil || report.Attempts[0].Abort.ID != abort.Identity ||
			report.State.Clock != state {
			t.Fatal("abort changed world or was not durably recoverable", err)
		}
		if baseline == nil {
			baseline = abort.Canonical
		} else if !bytes.Equal(baseline, abort.Canonical) {
			t.Fatal("subject order influenced canonical abort")
		}
	}
}

// ClockAdvance must use the first covering closure even when a newer
// closure was already appended; a final finite marker covers future time.
func TestReferenceG2FirstClosureAndFinalBeyondCut(t *testing.T) {
	ctx := context.Background()
	var firstFence, firstCommit []byte
	for i, appendLater := range []bool{false, true} {
		c, store, ledger, run, bundle, path := testFixture(t)
		state, err := store.LoadClock(ctx, [32]byte(run))
		if err != nil {
			t.Fatal(err)
		}
		_, closures, _ := ledger.Prefix("driver")
		if len(closures) != 1 {
			t.Fatal("baseline closure missing")
		}
		original := closures[0]
		if appendLater {
			if _, err := ledger.CloseSource(ctx, "driver", inputledger.Closure{ProducerKey: "later",
				ClosedThrough: domain.EligibilityCoordinate{NotBeforeInstant: state.Instant + 110}}); err != nil {
				t.Fatal(err)
			}
		}
		// A second source finalizes at the INITIAL instant, far short of
		// the target. It still must satisfy the future advance's closure.
		if err := ledger.DeclareSource(inputledger.Source{ID: "final-source"}); err != nil {
			t.Fatal(err)
		}
		final, err := ledger.CloseSource(ctx, "final-source", inputledger.Closure{ProducerKey: "finite-final",
			ClosedThrough: domain.EligibilityCoordinate{NotBeforeInstant: state.Instant}, Final: true})
		if err != nil {
			t.Fatal(err)
		}
		_, _, before := ledger.Prefix("final-source")
		in := inputledger.Input{ProducerUnitKey: "late", Kind: "event.input", SchemaVersion: 1, Actor: []any{uint64(0)}, Payload: []any{},
			DeclaredEffectiveAt: state.Instant + 101, Eligibility: domain.EligibilityCoordinate{NotBeforeInstant: state.Instant + 101},
			Provenance: []any{uint64(0)}, IdempotencyKey: []any{uint64(0)}}
		if _, err := ledger.AppendInput(ctx, "final-source", in); !errors.Is(err, inputledger.ErrFinalSource) {
			t.Fatal("post-final input accepted", err)
		}
		if _, err := ledger.CloseSource(ctx, "final-source", inputledger.Closure{ProducerKey: "new-final", Final: true,
			ClosedThrough: domain.EligibilityCoordinate{NotBeforeInstant: state.Instant + 101}}); !errors.Is(err, inputledger.ErrFinalSource) {
			t.Fatal("post-final closure accepted", err)
		}
		inputs, finalClosures, after := ledger.Prefix("final-source")
		if len(inputs) != 0 || len(finalClosures) != 1 || before != after {
			t.Fatal("post-final rejection changed durable prefix")
		}
		got, err := c.Advance(ctx, ledger, run)
		if err != nil || got.ResultClock.Instant != state.Instant+100 || got.ResultClock.LastOrdinal != -1 {
			t.Fatal("final finite closure did not cover future cut", err)
		}
		proofs := got.Fence.Fields[9].([]any)
		if len(proofs) != 2 {
			t.Fatal("missing source closure witnesses")
		}
		for _, raw := range proofs {
			proof := raw.([]any)
			expected := original
			if proof[0] == "final-source" {
				expected = final
			} else if proof[0] != "driver" {
				t.Fatal("foreign source closure")
			}
			if !bytes.Equal(proof[1].([]any)[1].([]byte), expected.ID[:]) || proof[2] != expected.Receipt.Sequence {
				t.Fatal("newer closure replaced first covering closure")
			}
		}
		if i == 0 {
			// The opposite interleaving: append the same later closure
			// AFTER publication. Cold replay must keep the original fence.
			if _, err := ledger.CloseSource(ctx, "driver", inputledger.Closure{ProducerKey: "later",
				ClosedThrough: domain.EligibilityCoordinate{NotBeforeInstant: state.Instant + 110}}); err != nil {
				t.Fatal(err)
			}
			firstFence, firstCommit = got.Fence.Canonical, got.Commit.Canonical
		} else if !bytes.Equal(firstFence, got.Fence.Canonical) || !bytes.Equal(firstCommit, got.Commit.Canonical) {
			t.Fatal("later closure timing changed causal cut")
		}
		cold, err := sqlite.Open(ctx, path)
		if err != nil {
			t.Fatal(err)
		}
		replay, err := cold.ReplayClock(ctx, [32]byte(run), bundle)
		cold.Close()
		if err != nil || replay.ReplayedCommits != 1 || replay.ReconstructedClock != got.ResultClock {
			t.Fatal("append-stable closure proof did not replay", err)
		}
	}
}

// Exercise a closure gap on real SQLite. No evaluator may leave a fence
// while a declared source has not covered the requested target.
func TestReferenceG2AdvanceWaitsForAllClosures(t *testing.T) {
	ctx := context.Background()
	c, store, ledger, run, _, path := testFixture(t)
	if err := ledger.DeclareSource(inputledger.Source{ID: "delayed-source"}); err != nil {
		t.Fatal(err)
	}
	initial, err := store.LoadClock(ctx, [32]byte(run))
	if err != nil {
		t.Fatal(err)
	}
	for _, cover := range []domain.SimulationInstant{initial.Instant, initial.Instant + 50} {
		if _, err := ledger.CloseSource(ctx, "delayed-source", inputledger.Closure{
			ProducerKey: fmt.Sprintf("partial-%d", cover), ClosedThrough: domain.EligibilityCoordinate{NotBeforeInstant: cover},
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := c.Advance(ctx, ledger, run); !errors.Is(err, admission.ErrWaitingForClosure) {
			t.Fatal("advance did not wait for the full target", err)
		}
		db, err := openReferenceReadOnlyDB(path)
		if err != nil {
			t.Fatal(err)
		}
		var decisions, events, revisions int
		for _, table := range []struct {
			name string
			out  *int
		}{{"decision_ledger", &decisions}, {"event_store", &events}, {"world_revisions", &revisions}} {
			if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table.name).Scan(table.out); err != nil {
				db.Close()
				t.Fatal(err)
			}
		}
		db.Close()
		if decisions != 0 || events != 0 || revisions != 1 {
			t.Fatal("closure gap created causal artifacts")
		}
	}
	if _, err := ledger.CloseSource(ctx, "delayed-source", inputledger.Closure{ProducerKey: "final",
		ClosedThrough: domain.EligibilityCoordinate{NotBeforeInstant: initial.Instant + 50}, Final: true}); err != nil {
		t.Fatal(err)
	}
	out, err := c.Advance(ctx, ledger, run)
	if err != nil || out.ResultClock.Instant != initial.Instant+100 {
		t.Fatal("positive final closure failed", err)
	}
}

func TestReferenceG2TriggerAtomicOriginAndColdSettlement(t *testing.T) {
	ctx := context.Background()
	def := trigger.Definition{ID: domain.ID{0x91}, Version: 1, Policy: trigger.RisingEdge,
		FireOnInitialTrue: true, Dependencies: []string{"body", "clock"}}
	c, store, ledger, run, bundle, path := testFixture(t, def)
	advance, err := c.Advance(ctx, ledger, run)
	if err != nil || len(advance.Activations) != 1 || len(advance.Events) != 4 {
		t.Fatal("origin commit did not produce registered activation", err)
	}
	act := advance.Activations[0]
	raw, err := codec.StrictDecodePayload(act.Canonical)
	if err != nil {
		t.Fatal(err)
	}
	f := raw.([]any)
	if len(f) != 12 || f[10] != uint64(0) ||
		!referenceC3Same(f[6], []any{int64(advance.ResultClock.Instant), uint64(0)}) {
		t.Fatal("origin did not create pending activation at destination floor")
	}
	causes := f[5].([]any)[3].([]any)
	if len(causes) != 2 || !bytes.Equal(causes[0].([]byte), advance.Events[0].Identity[:]) ||
		!bytes.Equal(causes[1].([]byte), advance.Events[1].Identity[:]) {
		t.Fatal("dependency filter failed to include both causal pre-trigger events")
	}
	db, err := openReferenceReadOnlyDB(path)
	if err != nil {
		t.Fatal(err)
	}
	var persisted []byte
	err = db.QueryRowContext(ctx, "SELECT canonical FROM trigger_activations WHERE id=?", act.Identity[:]).Scan(&persisted)
	db.Close()
	if err != nil || !bytes.Equal(persisted, act.Canonical) {
		t.Fatal("activation not atomically durable before expansion", err)
	}
	// No owner request or candidate expansion has occurred. Drop the
	// producing handle, then build the next plan from the durable prefix.
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	cold, err := sqlite.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer cold.Close()
	restored, err := inputledger.Open(ctx, run, bundle, cold)
	if err != nil {
		t.Fatal(err)
	}
	proof, err := cold.ReplayClock(ctx, [32]byte(run), bundle)
	if err != nil || proof.ReplayedCommits != 1 || proof.VerifiedActivations != 1 {
		t.Fatal("cold origin replay lost activation", err)
	}
	state, err := cold.LoadClock(ctx, [32]byte(run))
	if err != nil {
		t.Fatal(err)
	}
	pending, err := cold.PendingClock(ctx, [32]byte(run))
	if err != nil {
		t.Fatal(err)
	}
	plan, ok, err := clock.Next(state, pending)
	if err != nil || !ok || plan.Kind != clock.Work || plan.OriginOrdinal != 0 {
		t.Fatal("trigger did not precede later due occurrence", err)
	}
	_, hash, err := cold.LoadWorldState(ctx, uint64(state.Revision))
	if err != nil {
		t.Fatal(err)
	}
	cid, _, err := identity.New(bundle).Sum("cote.csf.id.cycle", "cote.csf.schema.cycle-id-preimage", 1,
		[]any{run[:], []any{int64(state.Instant), uint64(0)}})
	if err != nil {
		t.Fatal(err)
	}
	cycle := admission.Cycle{ID: domain.ID(cid), Instant: state.Instant, BaseRevision: state.Revision,
		BaseStateHash: domain.Digest(hash), RetryRef: []any{uint64(0)}}
	fence, err := admission.NewCoordinator(admission.NewFenceLog(cold)).AdmitThenEvaluate(ctx, bundle, restored, cycle, c.admissionPolicies, nil)
	if err != nil {
		t.Fatal(err)
	}
	units := fence.Fields[11].([]any)
	if len(units) != 1 || units[0].([]any)[1] != uint64(3) ||
		!bytes.Equal(units[0].([]any)[3].([]byte), act.Identity[:]) {
		t.Fatal("resume recomputed or duplicated activation identity")
	}
	coordinator := New(cold, bundle, c.admissionPolicies)
	if _, err := coordinator.CommitSystemUnit(ctx, restored, cycle, fence); err != nil {
		t.Fatal(err)
	}
	if _, err := coordinator.CommitSystemUnit(ctx, restored, cycle, fence); err == nil {
		t.Fatal("activation settled twice")
	}
	pending, err = cold.PendingClock(ctx, [32]byte(run))
	if err != nil || len(pending) != 1 || pending[0].ID == domain.ID(act.Identity) ||
		pending[0].Eligibility.NotBeforeCycleOrdinal != 1 {
		t.Fatal("wrong remaining lifecycle after trigger terminal", err)
	}
	proof, err = cold.ReplayClock(ctx, [32]byte(run), bundle)
	if err != nil || proof.ReplayedCommits != 2 || proof.VerifiedActivations != 1 {
		t.Fatal("replay resurrected trigger", err)
	}
	q, err := sqlite.OpenPrivilegedObservatory(ctx, path, bundle)
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close()
	report, err := q.ReadPrivilegedReport(ctx, [32]byte(run))
	if err != nil || len(report.Commits) != 2 || len(report.Commits[1].DecisionLinks) != 1 ||
		report.Commits[1].DecisionLinks[0].Source.ID != act.Identity || report.Commits[1].DecisionLinks[0].Status != 0 {
		t.Fatal("activation not consumed by its exact decision", err)
	}
}


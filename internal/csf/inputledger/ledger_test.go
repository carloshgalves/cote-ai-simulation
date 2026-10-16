package inputledger

import (
	"context"
	"errors"
	"testing"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/memory"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

const bundlePath = "../../../docs/architecture/canonical-codec-v1-bundle"

func TestClosureIsMonotonicAndLateInputMovesToNextCoordinate(t *testing.T) {
	bundle, err := policy.Load(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	var runID domain.ID
	runID[0] = 0x51
	ledger := New(runID, bundle, memory.New())
	if err := ledger.DeclareSource(Source{ID: "external"}); err != nil {
		t.Fatal(err)
	}
	coord := domain.EligibilityCoordinate{NotBeforeInstant: 42, NotBeforeCycleOrdinal: 3}
	input := func(key string) Input {
		return Input{ProducerUnitKey: key, Kind: "event.input", SchemaVersion: 1, Actor: []any{uint64(0)}, Payload: []any{}, DeclaredEffectiveAt: 42, Eligibility: coord, Provenance: []any{uint64(0)}, IdempotencyKey: []any{uint64(0)}}
	}
	if _, err := ledger.AppendInput(context.Background(), "external", input("before")); err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.CloseSource(context.Background(), "external", Closure{ProducerKey: "close-1", ClosedThrough: coord}); err != nil {
		t.Fatal(err)
	}
	late, err := ledger.AppendInput(context.Background(), "external", input("after"))
	if err != nil {
		t.Fatal(err)
	}
	got := coordinate(late.Fields[9])
	if got.NotBeforeInstant != 42 || got.NotBeforeCycleOrdinal != 4 {
		t.Fatalf("late input coordinate = %+v", got)
	}
	if _, err := ledger.CloseSource(context.Background(), "external", Closure{ProducerKey: "close-backwards", ClosedThrough: domain.EligibilityCoordinate{NotBeforeInstant: 41}}); err == nil {
		t.Fatal("accepted backward closure")
	}
	_, _, cursor := ledger.Prefix("external")
	if cursor != 3 {
		t.Fatalf("failed closure changed receipt cursor: %d", cursor)
	}
}

func TestFinalClosureRejectsFutureInputAndClosureAtomically(t *testing.T) {
	bundle, err := policy.Load(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	var runID domain.ID
	runID[0] = 0x52
	ledger := New(runID, bundle, memory.New())
	if err := ledger.DeclareSource(Source{ID: "external"}); err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.CloseSource(context.Background(), "external", Closure{ProducerKey: "final", ClosedThrough: domain.EligibilityCoordinate{NotBeforeInstant: 5}, Final: true}); err != nil {
		t.Fatal(err)
	}
	input := Input{ProducerUnitKey: "late", Kind: "event.input", SchemaVersion: 1, Actor: []any{uint64(0)}, Payload: []any{}, DeclaredEffectiveAt: 6, Eligibility: domain.EligibilityCoordinate{NotBeforeInstant: 6}, Provenance: []any{uint64(0)}, IdempotencyKey: []any{uint64(0)}}
	if _, err := ledger.AppendInput(context.Background(), "external", input); !errors.Is(err, ErrFinalSource) {
		t.Fatalf("expected final-source rejection, got %v", err)
	}
	if _, err := ledger.CloseSource(context.Background(), "external", Closure{ProducerKey: "after-final", ClosedThrough: domain.EligibilityCoordinate{NotBeforeInstant: 9}}); !errors.Is(err, ErrFinalSource) {
		t.Fatalf("expected final closure rejection, got %v", err)
	}
	inputs, closures, cursor := ledger.Prefix("external")
	if len(inputs) != 0 || len(closures) != 1 || cursor != 1 {
		t.Fatalf("rejected append mutated prefix: inputs=%d closures=%d cursor=%d", len(inputs), len(closures), cursor)
	}
}

func TestRevocationGatesLateResponseAndSlotHasOneWinner(t *testing.T) {
	bundle, err := policy.Load(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	var runID, cycleID domain.ID
	runID[0] = 0x53
	cycleID[0] = 0x54
	store := memory.New()
	ledger := New(runID, bundle, store)
	if err := ledger.DeclareSource(Source{ID: "external"}); err != nil {
		t.Fatal(err)
	}
	var genesisID, genesisDigest domain.ID
	genesisID[0] = 1
	genesisDigest[0] = 2
	round, err := ledger.DeclareRound(context.Background(), "schedule", [2]string{"cote", "actor-a"}, []string{"act"}, domain.EligibilityCoordinate{NotBeforeInstant: 9}, []any{uint64(2), []any{genesisID[:], genesisDigest[:]}}, 0, 0, 0, []any{uint64(0)})
	if err != nil {
		t.Fatal(err)
	}
	var deriver = identity.New(bundle)
	makeDispatch := func(ordinal uint32) Dispatch {
		id, _, deriveErr := deriver.Sum("cote.csf.id.dispatch", "cote.csf.schema.dispatch-id-preimage", 1, []any{runID[:], cycleID[:], round.ID[:], "act", uint64(ordinal)})
		if deriveErr != nil {
			t.Fatal(deriveErr)
		}
		var dispatchID domain.ID
		copy(dispatchID[:], id[:])
		return Dispatch{ID: dispatchID, RunID: runID, CycleID: cycleID, RoundID: round.ID, SlotID: "act", Ordinal: ordinal, Actor: round.Actor, Eligibility: round.Eligibility, EffectiveAt: 9}
	}
	first := makeDispatch(0)
	if err := ledger.RegisterDispatch(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	if err := ledger.RevokeDispatch(context.Background(), round.ID, "act", 0, []any{uint64(0)}); err != nil {
		t.Fatal(err)
	}
	late := noProposal(t, deriver, first, 0)
	if err := ledger.Respond(context.Background(), late); !errors.Is(err, ErrSlotRevoked) {
		t.Fatalf("revoked dispatch response accepted: %v", err)
	}
	second := makeDispatch(1)
	if err := ledger.RegisterDispatch(context.Background(), second); err != nil {
		t.Fatal(err)
	}
	winner := noProposal(t, deriver, second, 0)
	if err := ledger.Respond(context.Background(), winner); err != nil {
		t.Fatal(err)
	}
	loser := noProposal(t, deriver, second, 1)
	if err := ledger.Respond(context.Background(), loser); !errors.Is(err, ErrSlotFilled) {
		t.Fatalf("second slot response accepted: %v", err)
	}
}

func noProposal(t *testing.T, deriver identity.Deriver, d Dispatch, reason uint64) Response {
	t.Helper()
	id, _, err := deriver.Sum("cote.csf.id.no-proposal", "cote.csf.schema.slot-response-id-preimage", 1, []any{d.ID[:], uint64(1)})
	if err != nil {
		t.Fatal(err)
	}
	var responseID domain.ID
	copy(responseID[:], id[:])
	body := []any{responseID[:], d.ID[:], d.RunID[:], d.CycleID[:], d.RoundID[:], d.SlotID, []any{d.Actor[0], d.Actor[1]}, int64(d.EffectiveAt), uint64(d.BaseRevision), reason}
	return Response{Kind: 1, ID: responseID, DispatchID: d.ID, RunID: d.RunID, CycleID: d.CycleID, RoundID: d.RoundID, SlotID: d.SlotID, Actor: d.Actor, EffectiveAt: d.EffectiveAt, SubmittedAgainstRevision: d.BaseRevision, Body: body}
}

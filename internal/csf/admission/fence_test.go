package admission

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/inputledger"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/memory"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

const bundlePath = "../../../docs/architecture/canonical-codec-v1-bundle"

func TestFenceStableAcrossIngressAndRoundCompletionPermutations(t *testing.T) {
	bundle, err := policy.Load(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	first := buildScenario(t, bundle, false)
	second := buildScenario(t, bundle, true)
	fenceA, err := Derive(bundle, first.ledger, first.cycle, first.policies)
	if err != nil {
		t.Fatal(err)
	}
	fenceB, err := Derive(bundle, second.ledger, second.cycle, second.policies)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(fenceA.Canonical, fenceB.Canonical) {
		t.Fatalf("fence differs under append/completion permutation\nA=%x\nB=%x", fenceA.Canonical, fenceB.Canonical)
	}
	if len(fenceA.Fields[9].([]any)) != 2 {
		t.Fatalf("expected both source closure proofs")
	}
	cohort := fenceA.Fields[10].([]any)
	if len(cohort[1].([]any)) != 2 {
		t.Fatalf("expected both simultaneous rounds in one cohort")
	}
	if len(fenceA.Fields[11].([]any)) != 4 {
		t.Fatalf("expected two slot units and two inputs")
	}
}

func TestFenceWaitsForEveryClosureAndSlot(t *testing.T) {
	bundle, err := policy.Load(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	s := buildScenario(t, bundle, false)
	// A fresh source declared after the fixture's records has no closure proof.
	if err := s.ledger.DeclareSource(inputledger.Source{ID: "source-z"}); err != nil {
		t.Fatal(err)
	}
	if _, err := Derive(bundle, s.ledger, s.cycle, s.policies); !errors.Is(err, ErrWaitingForClosure) {
		t.Fatalf("expected explicit closure wait, got %v", err)
	}
}

func TestCoordinatorPersistsFenceBeforeEvaluation(t *testing.T) {
	bundle, err := policy.Load(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	s := buildScenario(t, bundle, false)
	store := memory.New()
	log := NewFenceLog(store)
	coordinator := NewCoordinator(log)
	called := false
	_, err = coordinator.AdmitThenEvaluate(context.Background(), bundle, s.ledger, s.cycle, s.policies, func(f Fence) error {
		called = true
		persisted, readErr := log.Get(context.Background(), f.ID)
		if readErr != nil {
			return errors.New("evaluation ran before fence append")
		}
		if !bytes.Equal(persisted, f.Canonical) {
			return errors.New("persisted fence bytes differ")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("evaluation was not invoked")
	}
}

func TestDivergentUnitBytesFailBeforeFenceOrEvaluation(t *testing.T) {
	bundle, err := policy.Load(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	s := buildScenario(t, bundle, false)
	inputs, _, _ := s.ledger.Prefix("source-a")
	inputs[0].Fields[0].([]byte)[0] ^= 1
	store := memory.New()
	called := false
	coordinator := NewCoordinator(NewFenceLog(store))
	_, err = coordinator.AdmitThenEvaluate(context.Background(), bundle, s.ledger, s.cycle, s.policies, func(Fence) error { called = true; return nil })
	if err == nil {
		t.Fatal("accepted a unit whose ID bytes diverged from its persisted record")
	}
	if called {
		t.Fatal("evaluation ran before unit verification")
	}
	if store.Len() != 0 {
		t.Fatal("divergent unit wrote an admission fence")
	}
}

type scenario struct {
	ledger   *inputledger.Ledger
	cycle    Cycle
	policies Policies
}

func buildScenario(t *testing.T, bundle *policy.Bundle, reverse bool) scenario {
	t.Helper()
	var runID domain.ID
	runID[0] = 0x41
	ledger := inputledger.New(runID, bundle, memory.New())
	sources := []string{"source-a", "source-b"}
	if reverse {
		sources[0], sources[1] = sources[1], sources[0]
	}
	for _, source := range sources {
		if err := ledger.DeclareSource(inputledger.Source{ID: source}); err != nil {
			t.Fatal(err)
		}
	}
	inputs := []struct {
		source string
		input  inputledger.Input
	}{
		{source: "source-a", input: inputledger.Input{ProducerUnitKey: "event-a", Kind: "external.event", SchemaVersion: 1, Actor: []any{uint64(0)}, Payload: []any{}, DeclaredEffectiveAt: 100, Eligibility: domain.EligibilityCoordinate{NotBeforeInstant: 100}, Provenance: []any{uint64(0)}, IdempotencyKey: []any{uint64(0)}}},
		{source: "source-b", input: inputledger.Input{ProducerUnitKey: "event-b", Kind: "external.event", SchemaVersion: 1, Actor: []any{uint64(0)}, Payload: []any{}, DeclaredEffectiveAt: 100, Eligibility: domain.EligibilityCoordinate{NotBeforeInstant: 100}, Provenance: []any{uint64(0)}, IdempotencyKey: []any{uint64(0)}}},
	}
	if reverse {
		inputs[0], inputs[1] = inputs[1], inputs[0]
	}
	for _, entry := range inputs {
		if _, err := ledger.AppendInput(context.Background(), entry.source, entry.input); err != nil {
			t.Fatal(err)
		}
	}
	closures := []string{"source-a", "source-b"}
	if reverse {
		closures[0], closures[1] = closures[1], closures[0]
	}
	for _, source := range closures {
		if _, err := ledger.CloseSource(context.Background(), source, inputledger.Closure{ProducerKey: "close-through-100", ClosedThrough: domain.EligibilityCoordinate{NotBeforeInstant: 100}, Final: false}); err != nil {
			t.Fatal(err)
		}
	}
	var genesisID, genesisDigest domain.ID
	genesisID[0] = 1
	genesisDigest[0] = 2
	genesisRef := []any{genesisID[:], genesisDigest[:]}
	actors := [2]string{"cote", "actor-a"}
	rounds := []struct {
		ordinal uint32
		actor   [2]string
	}{{ordinal: 0, actor: actors}, {ordinal: 1, actor: [2]string{"cote", "actor-b"}}}
	if reverse {
		rounds[0], rounds[1] = rounds[1], rounds[0]
	}
	created := []any{uint64(2), genesisRef}
	declared := make([]inputledger.Round, 0, 2)
	for _, r := range rounds {
		round, err := ledger.DeclareRound(context.Background(), "schedule", r.actor, []string{"act"}, domain.EligibilityCoordinate{NotBeforeInstant: 100}, created, 0, r.ordinal, 0, []any{uint64(0)})
		if err != nil {
			t.Fatal(err)
		}
		declared = append(declared, round)
	}
	var cycleID domain.ID
	cycleID[0] = 0x61
	base := domain.Digest{}
	base[0] = 0x7f
	d := identity.New(bundle)
	for _, round := range declared {
		dispatchID, _, err := d.Sum("cote.csf.id.dispatch", "cote.csf.schema.dispatch-id-preimage", 1, []any{runID[:], cycleID[:], round.ID[:], "act", uint64(0)})
		if err != nil {
			t.Fatal(err)
		}
		var did domain.ID
		copy(did[:], dispatchID[:])
		dispatch := inputledger.Dispatch{ID: did, RunID: runID, CycleID: cycleID, RoundID: round.ID, SlotID: "act", Actor: round.Actor, Eligibility: round.Eligibility, BaseRevision: 0, EffectiveAt: 100}
		if err := ledger.RegisterDispatch(context.Background(), dispatch); err != nil {
			t.Fatal(err)
		}
	}
	responses := append([]inputledger.Round(nil), declared...)
	if reverse {
		responses[0], responses[1] = responses[1], responses[0]
	}
	for _, round := range responses {
		dispatchID, _, _ := d.Sum("cote.csf.id.dispatch", "cote.csf.schema.dispatch-id-preimage", 1, []any{runID[:], cycleID[:], round.ID[:], "act", uint64(0)})
		var did domain.ID
		copy(did[:], dispatchID[:])
		responseID, _, err := d.Sum("cote.csf.id.no-proposal", "cote.csf.schema.slot-response-id-preimage", 1, []any{did[:], uint64(1)})
		if err != nil {
			t.Fatal(err)
		}
		var rid domain.ID
		copy(rid[:], responseID[:])
		body := []any{rid[:], did[:], runID[:], cycleID[:], round.ID[:], "act", []any{round.Actor[0], round.Actor[1]}, int64(100), uint64(0), uint64(0)}
		resp := inputledger.Response{Kind: 1, ID: rid, DispatchID: did, RunID: runID, CycleID: cycleID, RoundID: round.ID, SlotID: "act", Actor: round.Actor, EffectiveAt: 100, SubmittedAgainstRevision: 0, Body: body}
		if err := ledger.Respond(context.Background(), resp); err != nil {
			t.Fatal(err)
		}
	}
	cycle := Cycle{ID: cycleID, Instant: 100, Ordinal: 0, BaseRevision: 0, BaseStateHash: base, RetryRef: []any{uint64(0)}}
	pol := Policies{AdmissionOrder: []any{"admission-order-v1", uint64(1), bytes.Repeat([]byte{2}, 32)}, CausalIdentity: []any{"causal-identity-v1", uint64(1), bytes.Repeat([]byte{1}, 32)}, Fence: []any{"admission-fence-v1", uint64(1), bytes.Repeat([]byte{3}, 32)}, Rules: [][]any{}}
	return scenario{ledger: ledger, cycle: cycle, policies: pol}
}

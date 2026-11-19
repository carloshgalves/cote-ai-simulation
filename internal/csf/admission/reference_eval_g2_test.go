package admission

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/inputledger"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/memory"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

func TestReferenceG2ClosureRaceStableMembership(t *testing.T) {
	ctx := context.Background()
	bundle, err := policy.Load(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	var baseline []byte
	for _, lateBeforeFence := range []bool{false, true} {
		store := memory.New()
		run := domain.ID{0x61}
		l, err := inputledger.New(ctx, run, bundle, store)
		if err != nil {
			t.Fatal(err)
		}
		if err := l.DeclareSource(inputledger.Source{ID: "driver"}); err != nil {
			t.Fatal(err)
		}
		at := domain.EligibilityCoordinate{NotBeforeInstant: 100}
		cycle := Cycle{ID: domain.ID{0x62}, Instant: 100, BaseStateHash: domain.Digest{0x63}, RetryRef: []any{uint64(0)}}
		policies := Policies{AdmissionOrder: []any{"admission-order-v1", uint64(1), bytes.Repeat([]byte{2}, 32)},
			CausalIdentity: []any{"causal-identity-v1", uint64(1), bytes.Repeat([]byte{1}, 32)},
			Fence:          []any{"admission-fence-v1", uint64(1), bytes.Repeat([]byte{3}, 32)}, Rules: [][]any{}}
		appendInput := func(key string) inputledger.StoredInput {
			t.Helper()
			in, err := l.AppendInput(ctx, "driver", inputledger.Input{ProducerUnitKey: key, Kind: "external.event",
				SchemaVersion: 1, Actor: []any{uint64(0)}, Payload: []any{}, DeclaredEffectiveAt: 100,
				Eligibility: at, Provenance: []any{uint64(0)}, IdempotencyKey: []any{uint64(0)}})
			if err != nil {
				t.Fatal(err)
			}
			return in
		}
		first := appendInput("first")
		if _, err := Derive(bundle, l, cycle, policies); !errors.Is(err, ErrWaitingForClosure) {
			t.Fatal("coordinator derived before closed prefix", err)
		}
		if _, err := l.CloseSource(ctx, "driver", inputledger.Closure{ProducerKey: "closed", ClosedThrough: at}); err != nil {
			t.Fatal(err)
		}
		var second inputledger.StoredInput
		if lateBeforeFence {
			second = appendInput("second")
		}
		fence, err := Derive(bundle, l, cycle, policies)
		if err != nil {
			t.Fatal(err)
		}
		if !lateBeforeFence {
			second = appendInput("second")
		}
		if second.ID == first.ID || second.Fields[8] != int64(100) ||
			!reflect.DeepEqual(second.Fields[9], []any{int64(100), uint64(1)}) {
			t.Fatal("late input changed declared time or entered closed coordinate")
		}
		reopened, err := inputledger.Open(ctx, run, bundle, store)
		if err != nil {
			t.Fatal(err)
		}
		rederived, err := Derive(bundle, reopened, cycle, policies)
		if err != nil || !bytes.Equal(rederived.Canonical, fence.Canonical) {
			t.Fatal("append timing changed first cut", err)
		}
		units := rederived.Fields[11].([]any)
		if len(units) != 1 || !bytes.Equal(units[0].([]any)[3].([]byte), first.ID[:]) {
			t.Fatal("closed cut lost first input or included late delivery")
		}
		if baseline == nil {
			baseline = bytes.Clone(rederived.Canonical)
		} else if !bytes.Equal(baseline, rederived.Canonical) {
			t.Fatal("same closed ledger produced distinct fence")
		}
	}
}

func TestReferenceG2SameSourcePermutation(t *testing.T) {
	ctx := context.Background()
	bundle, err := policy.Load(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	var baseline Fence
	var originals map[string]inputledger.StoredInput
	for _, reverse := range []bool{false, true} {
		store := memory.New()
		run := domain.ID{0x71}
		l, err := inputledger.New(ctx, run, bundle, store)
		if err != nil {
			t.Fatal(err)
		}
		if err := l.DeclareSource(inputledger.Source{ID: "driver"}); err != nil {
			t.Fatal(err)
		}
		at := domain.EligibilityCoordinate{NotBeforeInstant: 100}
		cycle := Cycle{ID: domain.ID{0x72}, Instant: 100, BaseStateHash: domain.Digest{0x73}, RetryRef: []any{uint64(0)}}
		policies := Policies{AdmissionOrder: []any{"admission-order-v1", uint64(1), bytes.Repeat([]byte{2}, 32)},
			CausalIdentity: []any{"causal-identity-v1", uint64(1), bytes.Repeat([]byte{1}, 32)},
			Fence:          []any{"admission-fence-v1", uint64(1), bytes.Repeat([]byte{3}, 32)}, Rules: [][]any{}}
		keys := []string{"first", "second", "third"}
		if reverse {
			keys[0], keys[2] = keys[2], keys[0]
		}
		inputs := map[string]inputledger.StoredInput{}
		for _, key := range keys {
			in, err := l.AppendInput(ctx, "driver", inputledger.Input{ProducerUnitKey: key, Kind: "external.event",
				SchemaVersion: 1, Actor: []any{uint64(0)}, Payload: []any{}, DeclaredEffectiveAt: 100,
				Eligibility: at, Provenance: []any{uint64(0)}, IdempotencyKey: []any{uint64(0)}})
			if err != nil {
				t.Fatal(err)
			}
			inputs[key] = in
		}
		close, err := l.CloseSource(ctx, "driver", inputledger.Closure{ProducerKey: "closed", ClosedThrough: at})
		if err != nil {
			t.Fatal(err)
		}
		if close.Receipt.Sequence != 3 || inputs["first"].ID == inputs["second"].ID {
			t.Fatal("permutation did not retain distinct acts and exact prefix")
		}
		fence, err := Derive(bundle, l, cycle, policies)
		if err != nil {
			t.Fatal(err)
		}
		units := fence.Fields[11].([]any)
		if len(units) != 3 {
			t.Fatal("fence omitted admitted same-source inputs")
		}
		seen := map[domain.ID]bool{}
		for i, raw := range units {
			unit := raw.([]any)
			var id domain.ID
			copy(id[:], unit[3].([]byte))
			if seen[id] || unit[1] != uint64(1) || unit[2] != "driver" {
				t.Fatal("duplicated or wrong-kind same-source root")
			}
			seen[id] = true
			if i > 0 {
				prev := units[i-1].([]any)
				if cmp := bytes.Compare(prev[4].([]byte), unit[4].([]byte)); cmp > 0 || (cmp == 0 && bytes.Compare(prev[3].([]byte), unit[3].([]byte)) >= 0) {
					t.Fatal("fence departed from normative digest/ID ordering")
				}
			}
		}
		if !reverse {
			baseline, originals = fence, inputs
		} else {
			for key, in := range inputs {
				if in.ID != originals[key].ID || in.Digest != originals[key].Digest || !bytes.Equal(in.Canonical, originals[key].Canonical) || !seen[in.ID] {
					t.Fatal("ingress order escaped into canonical input or fence identity")
				}
			}
			if inputs["first"].Receipt.Sequence == originals["first"].Receipt.Sequence || inputs["third"].Receipt.Sequence == originals["third"].Receipt.Sequence {
				t.Fatal("permutation was vacuous")
			}
			if !bytes.Equal(baseline.Canonical, fence.Canonical) || baseline.InputDigest != fence.InputDigest || baseline.Digest != fence.Digest {
				t.Fatal("ingress sequence influenced admitted-unit ordering")
			}
		}
	}
}

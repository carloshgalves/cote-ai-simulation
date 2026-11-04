package inputledger_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/inputledger"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/memory"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

func TestCSFRV18R19Finding3ExogenousInputUsesNormativeBody(t *testing.T) {
	for _, variant := range [][2]uint8{{1, 0}, {1, 1}, {2, 0}, {2, 1}} {
		t.Run(string(rune('0'+variant[0]))+"/"+string(rune('0'+variant[1])), func(t *testing.T) {
			ctx := context.Background()
			b, err := policy.Load("../../../docs/architecture/canonical-codec-v1-bundle")
			if variant[0] == 2 {
				b, err = policy.LoadV2("../../../docs/architecture/canonical-codec-v2-bundle")
			}
			if err != nil {
				t.Fatal(err)
			}
			store := memory.New()
			run := domain.ID{0xc3}
			l, err := inputledger.Open(ctx, run, b, store)
			if err != nil {
				t.Fatal(err)
			}
			if err := l.DeclareSource(inputledger.Source{ID: "external"}); err != nil {
				t.Fatal(err)
			}
			coordinate := domain.EligibilityCoordinate{NotBeforeInstant: 42, NotBeforeCycleOrdinal: 3}
			if variant[1] == 1 {
				if _, err := l.CloseSource(ctx, "external", inputledger.Closure{ProducerKey: "closed", ClosedThrough: coordinate}); err != nil {
					t.Fatal(err)
				}
			}
			input := inputledger.Input{ProducerUnitKey: "unit", Kind: "event.input", SchemaVersion: 1, Actor: []any{uint64(0)}, Payload: []any{}, DeclaredEffectiveAt: 42, Eligibility: coordinate, Provenance: []any{uint64(0)}, IdempotencyKey: []any{uint64(0)}}
			stored, err := l.AppendInput(ctx, "external", input)
			if err != nil {
				t.Fatal(err)
			}
			want, _, err := identity.New(b).Sum("cote.csf.digest.exogenous-input-unit", "cote.csf.schema.exogenous-input.body", 1, stored.Fields[:12])
			if err != nil {
				t.Fatal(err)
			}
			if stored.Digest != want {
				t.Fatal("runtime exogenous unit digest does not match normative flat body")
			}
			got := stored.Fields[9].([]any)[1].(uint64)
			if got != uint64(3+variant[1]) {
				t.Fatal("late-input causal normalization changed", got)
			}
			if _, err := l.CloseSource(ctx, "external", inputledger.Closure{ProducerKey: "later", ClosedThrough: domain.EligibilityCoordinate{NotBeforeInstant: 50}}); err != nil {
				t.Fatal(err)
			}
			again, err := l.AppendInput(ctx, "external", input)
			if err != nil || !bytes.Equal(again.Canonical, stored.Canonical) || !bytes.Equal(again.Receipt.Canonical, stored.Receipt.Canonical) {
				t.Fatalf("retry changed the original receipt/body: %v", err)
			}
			if _, err := inputledger.Open(ctx, run, b, store); err != nil {
				t.Fatal("normative unit not restorable:", err)
			}
			wrong, _, err := identity.New(b).Sum("cote.csf.digest.exogenous-input-unit", "cote.csf.schema.exogenous-input.body", 1, []any{stored.Fields[:12]})
			if err != nil {
				t.Fatal(err)
			}
			bad, err := codec.CanonicalPayload(append(append([]any{}, stored.Fields[:12]...), wrong[:]))
			if err != nil {
				t.Fatal(err)
			}
			records, err := store.Records(ctx)
			if err != nil {
				t.Fatal(err)
			}
			corrupt := memory.New()
			for _, record := range records {
				if record.Identity == [32]byte(stored.ID) {
					record.Canonical = bad
				}
				if err := corrupt.Append(ctx, record); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := inputledger.Open(ctx, run, b, corrupt); err == nil {
				t.Fatal("wrapped non-normative input accepted")
			}
		})
	}
}

package reducer

import (
	"bytes"
	"errors"
	"reflect"
	"testing"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
)

func referenceCompositeFixture(t *testing.T) ([]any, PhysicalBody, PhysicalBody) {
	t.Helper()
	a := physicalTestBody("npc.0001", 0)
	b := physicalTestBody("npc.0002", 0)
	typed, _, err := EncodeComposite(Stock{Available: 1}, []PhysicalBody{a, b})
	if err != nil {
		t.Fatal(err)
	}
	return typed, a, b
}
func TestCompositeWorldSharesOneAuthenticatedVariant(t *testing.T) {
	typed, a, b := referenceCompositeFixture(t)
	stock, bodies, err := DecodeComposite(typed)
	if err != nil || stock.Available != 1 || len(bodies) != 2 || !reflect.DeepEqual(a, bodies[0]) || !reflect.DeepEqual(b, bodies[1]) {
		t.Fatalf("composite not round-trippable: stock=%+v bodies=%+v err=%v", stock, bodies, err)
	}
	inventory, err := DecodeStock(typed)
	if err != nil || inventory.Available != 1 {
		t.Fatal("stock not accessible without losing physical context", err)
	}
	body, err := DecodePhysical(typed)
	if err != nil || len(body) != 2 {
		t.Fatal("physical state not accessible without losing inventory", err)
	}
	reversed, _, err := EncodeComposite(Stock{Available: 1}, []PhysicalBody{b, a})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(typed, reversed) {
		t.Fatal("subject input order changed composite canonical bytes")
	}
}
func TestCompositeResourceConsumptionPreservesEveryPhysicalField(t *testing.T) {
	before, a, b := referenceCompositeFixture(t)
	next, hash, err := Consume(before, 1)
	if err != nil {
		t.Fatal(err)
	}
	stock, bodies, err := DecodeComposite(next)
	if err != nil || stock.Available != 0 || !reflect.DeepEqual(bodies, []PhysicalBody{a, b}) {
		t.Fatalf("stock consume removed or changed independent body state: %+v %+v %v", stock, bodies, err)
	}
	priorRaw := referenceCompositePhysicalBytes(t, before)
	afterRaw := referenceCompositePhysicalBytes(t, next)
	if !bytes.Equal(priorRaw, afterRaw) {
		t.Fatal("stock reducer changed signed physical branch bytes")
	}
	again, other, err := Consume(before, 1)
	if err != nil || hash != other || !reflect.DeepEqual(next, again) {
		t.Fatal("resource reducer nondeterminism", err)
	}
	if _, _, err := Consume(next, 1); !errors.Is(err, ErrInsufficientStock) {
		t.Fatalf("double resource allocation accepted: %v", err)
	}
}
func TestCompositePhysicalEpochsKeepStockAndCausalCAS(t *testing.T) {
	before, a, b := referenceCompositeFixture(t)
	afterStock, _, err := Consume(before, 1)
	if err != nil {
		t.Fatal(err)
	}
	a.Hours = 48
	a.SleepDebt = 1
	first, _, err := ApplyCompletedPhysical(afterStock, []PhysicalBody{a})
	if err != nil {
		t.Fatal(err)
	}
	st, bodies, err := DecodeComposite(first)
	if err != nil || st.Available != 0 || bodies[0].Hours != 48 || bodies[1].Hours != 0 {
		t.Fatal("first body epoch wiped stock or foreign subject", err)
	}
	if !bytes.Equal(referenceCompositeStockBytes(t, afterStock), referenceCompositeStockBytes(t, first)) {
		t.Fatal("physical reducer mutated resource branch bytes")
	}
	b.Hours = 72
	b.SleepDebt = 0
	second, _, err := ApplyCompletedPhysical(first, []PhysicalBody{b})
	if err != nil {
		t.Fatal(err)
	}
	_, bodies, err = DecodeComposite(second)
	if err != nil || bodies[0].Hours != 48 || bodies[1].Hours != 72 {
		t.Fatal("second epoch lost first committed body state", err)
	}
	if _, _, err := ApplyCompletedPhysical(second, []PhysicalBody{b}); !errors.Is(err, ErrInvalidPhysicalState) {
		t.Fatal("stale repeated physical epoch accepted")
	}
	if !bytes.Equal(referenceCompositeStockBytes(t, second), referenceCompositeStockBytes(t, first)) {
		t.Fatal("later body epoch mutated inventory")
	}
}
func referenceCompositeEnvelope(t *testing.T, v []any) []any {
	t.Helper()
	raw := v[2].([]byte)
	env, err := codec.StrictDecodeEnvelope(raw)
	if err != nil {
		t.Fatal(err)
	}
	return env.Value.([]any)
}
func referenceCompositePhysicalBytes(t *testing.T, v []any) []byte {
	return referenceCompositeEnvelope(t, v)[2].([]any)[1].([]any)[2].([]byte)
}
func referenceCompositeStockBytes(t *testing.T, v []any) []byte {
	return referenceCompositeEnvelope(t, v)[2].([]any)[0].([]any)[2].([]byte)
}
func TestCompositeRejectsMutationMissingOrDuplicatedBranch(t *testing.T) {
	orig, a, _ := referenceCompositeFixture(t)
	stock, _, err := EncodeStock(Stock{Available: 1})
	if err != nil {
		t.Fatal(err)
	}
	physical, _, err := EncodePhysical([]PhysicalBody{a})
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string][]any{
		"missing physical":           []any{CompositeVariant, uint64(1), []any{stock}},
		"duplicate stock":            []any{CompositeVariant, uint64(1), []any{stock, stock}},
		"duplicate physical":         []any{CompositeVariant, uint64(1), []any{physical, physical}},
		"reversed order":             []any{CompositeVariant, uint64(1), []any{physical, stock}},
		"extra branch":               []any{CompositeVariant, uint64(1), []any{stock, physical, stock}},
		"unknown version":            []any{CompositeVariant, uint64(99), []any{stock, physical}},
		"unknown variant":            []any{"untrusted.composite", uint64(1), []any{stock, physical}},
		"arbitrary producer content": []any{CompositeVariant, uint64(1), []any{[]any{"raw", "human"}, physical}},
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			if err := ValidateCompositePayload(payload); err == nil {
				t.Fatal("malformed composite accepted in world schema", name)
			}
			raw, err := codec.CanonicalBytes(StockDomainTag, StockSchemaID, 1, payload)
			if err != nil {
				t.Fatal(err)
			}
			h := identity.OpaqueContentHash(raw)
			tampered := []any{StockSchemaID, uint64(1), raw, h[:]}
			if _, _, err := DecodeComposite(tampered); err == nil {
				t.Fatal("malformed composite decoded", name)
			}
			if _, err := DecodeStock(tampered); err == nil {
				t.Fatal("malformed composite gave unauthorized stock", name)
			}
			if _, err := DecodePhysical(tampered); err == nil {
				t.Fatal("malformed composite gave unauthorized body", name)
			}
		})
	}
	fields := referenceCompositeEnvelope(t, orig)
	parts := fields[2].([]any)
	badStock := append([]any(nil), parts[0].([]any)...)
	badStock[3] = bytes.Repeat([]byte{0xab}, 32)
	payload := []any{CompositeVariant, uint64(1), []any{badStock, parts[1]}}
	if err := ValidateCompositePayload(payload); err == nil {
		t.Fatal("composite accepted tampered stock digest")
	}
	good := append([]any(nil), orig...)
	good[3] = bytes.Repeat([]byte{0x55}, 32)
	if _, _, err := DecodeComposite(good); err == nil {
		t.Fatal("composite accepted forged outer digest")
	}
}
func TestCompositeOldSingleVariantContractsRemainValid(t *testing.T) {
	body := physicalTestBody("npc.0001", 0)
	st, _, err := EncodeStock(Stock{Available: 1})
	if err != nil {
		t.Fatal(err)
	}
	phys, _, err := EncodePhysical([]PhysicalBody{body})
	if err != nil {
		t.Fatal(err)
	}
	if x, err := DecodeStock(st); err != nil || x.Available != 1 {
		t.Fatal("standalone stock regression")
	}
	if x, err := DecodePhysical(phys); err != nil || len(x) != 1 {
		t.Fatal("standalone physical regression")
	}
	if _, err := DecodeStock(phys); err == nil {
		t.Fatal("standalone physical erroneously exposed stock")
	}
	if _, err := DecodePhysical(st); err == nil {
		t.Fatal("standalone stock erroneously exposed body")
	}
	if _, _, err := DecodeComposite(st); err == nil {
		t.Fatal("standalone stock accepted as composite")
	}
	if _, _, err := DecodeComposite(phys); err == nil {
		t.Fatal("standalone physical accepted as composite")
	}
}

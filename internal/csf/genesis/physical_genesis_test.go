package genesis

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/reducer"
)

func registeredPhysicalBundle(t *testing.T) *policy.Bundle {
	t.Helper()
	base, err := policy.Load(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	contract := policy.PhysicalManifest{
		Kind: "cote.embodiment.physical-registration.v1", Version: 1, ExtensionID: "cote.embodiment.physical.v1",
		StateSchema: reducer.PhysicalSchemaID, StateVersion: 1, StateVariant: reducer.PhysicalVariant,
		ActionType: "physical.advance", ActionVersion: 1, ActionOperation: "physical.advance", ActionProducer: "physical.a1",
		SourceID: "physical.a1", ProducerExecutableSHA256: strings.Repeat("ab", 32),
		ProducerPublicKeyEd25519: strings.Repeat("cd", 32), Disclosure: "ENGINE_ONLY",
	}
	raw, err := json.Marshal(contract)
	if err != nil {
		t.Fatal(err)
	}
	installed, err := policy.PhysicalExtensionV1(base, raw)
	if err != nil {
		t.Fatal(err)
	}
	return installed
}

func registeredPhysicalGenesis(t *testing.T, b *policy.Bundle) []byte {
	t.Helper()
	reg, err := b.PhysicalRegistration()
	if err != nil {
		t.Fatal(err)
	}
	env, err := codec.StrictDecodeEnvelope(genesisFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	payload := append([]any(nil), env.Value.([]any)...)
	payload[12] = []any{[]any{reg.ExtensionID, reg.BundleSHA256[:]}}
	payload[13] = []any{reg.SourceID}
	body := reducer.PhysicalBody{Subject: "npc.0001", WPrimeCapacity: 1, WPrimeTau: 1, WorkRestRatio: 1}
	for i := range body.Soreness {
		body.Soreness[i] = reducer.PhysicalSoreness{Latency: []float64{0}}
	}
	typed, _, err := reducer.EncodePhysical([]reducer.PhysicalBody{body})
	if err != nil {
		t.Fatal(err)
	}
	payload[15] = []any{[]any{reducer.PhysicalSchemaID, uint64(1)}}
	payload[16] = typed
	out, err := codec.CanonicalBytes(env.DomainTag, env.SchemaID, env.SchemaVersion, payload)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestPhysicalExtensionMustBeRegisteredAndPinnedInGenesis(t *testing.T) {
	installed := registeredPhysicalBundle(t)
	raw := registeredPhysicalGenesis(t, installed)
	registered, err := ValidateCanonical(raw, installed)
	if err != nil || registered == nil {
		t.Fatalf("physical Genesis refused: %v", err)
	}
	base, err := policy.Load(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateCanonical(raw, base); err == nil {
		t.Fatal("base policy accepted uninstalled extension")
	}
	mutate := func(fn func([]any)) []byte {
		t.Helper()
		env, err := codec.StrictDecodeEnvelope(raw)
		if err != nil {
			t.Fatal(err)
		}
		payload := append([]any(nil), env.Value.([]any)...)
		fn(payload)
		result, err := codec.CanonicalBytes(env.DomainTag, env.SchemaID, env.SchemaVersion, payload)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	cases := map[string]func([]any){
		"missing extension pin":       func(p []any) { p[12] = []any{} },
		"wrong extension hash":        func(p []any) { p[12] = []any{[]any{"cote.embodiment.physical.v1", make([]byte, 32)}} },
		"missing authorized producer": func(p []any) { p[13] = []any{} },
		"unregistered synthetic stock": func(p []any) {
			v, _, err := reducer.EncodeStock(reducer.Stock{Available: 1})
			if err != nil {
				t.Fatal(err)
			}
			p[16] = v
		},
	}
	for name, fn := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := ValidateCanonical(mutate(fn), installed); err == nil {
				t.Fatal("unregistered physical Genesis accepted")
			}
		})
	}
}

package genesis

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

const bundlePath = "../../../docs/architecture/canonical-codec-v1-bundle"

func genesisFixture(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile(bundlePath + "/fixtures.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures struct {
		Semantic []struct {
			CaseID     string `json:"case_id"`
			GenesisHex string `json:"genesis_envelope_hex"`
		} `json:"semantic"`
	}
	if err := json.Unmarshal(raw, &fixtures); err != nil {
		t.Fatal(err)
	}
	for _, item := range fixtures.Semantic {
		if item.CaseID == "genesis.pinning" {
			encoded, err := hex.DecodeString(item.GenesisHex)
			if err != nil {
				t.Fatal(err)
			}
			return productionGenesis(t, encoded)
		}
	}
	t.Fatal("genesis fixture not found")
	return nil
}

func productionGenesis(t *testing.T, encoded []byte) []byte {
	t.Helper()
	envelope, err := codec.StrictDecodeEnvelope(encoded)
	if err != nil {
		t.Fatal(err)
	}
	payload := envelope.Value.([]any)
	envelope.DomainTag = "cote.csf.digest.genesis-manifest"
	envelope.SchemaID = "cote.csf.schema.genesis-manifest"
	payload[15] = []any{[]any{"cote.csf.schema.snapshot-pending-state", uint64(1)}}
	state := make([]any, 10)
	for i := range state {
		state[i] = []any{}
	}
	stateEnvelope, err := codec.CanonicalBytes("cote.csf.snapshot.pending-state", "cote.csf.schema.snapshot-pending-state", 1, state)
	if err != nil {
		t.Fatal(err)
	}
	digest := identity.OpaqueContentHash(stateEnvelope)
	payload[16] = []any{"cote.csf.schema.snapshot-pending-state", uint64(1), stateEnvelope, digest[:]}
	result, err := codec.CanonicalBytes(envelope.DomainTag, envelope.SchemaID, envelope.SchemaVersion, payload)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestValidateCanonicalGenesis(t *testing.T) {
	bundle, err := policy.Load(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := ValidateCanonical(genesisFixture(t), bundle)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.InitialInstant != 0 {
		t.Fatalf("unexpected initial instant %d", manifest.InitialInstant)
	}
}

func TestRejectGenesisWithWrongTimezone(t *testing.T) {
	bundle, err := policy.Load(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := codec.StrictDecodeEnvelope(genesisFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	payload := envelope.Value.([]any)
	payload[2] = "UTC/Etc"
	corrupt, err := codec.CanonicalBytes(envelope.DomainTag, envelope.SchemaID, envelope.SchemaVersion, payload)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateCanonical(corrupt, bundle); err == nil {
		t.Fatal("accepted genesis with wrong timezone")
	}
}

func mutateGenesis(t *testing.T, mutate func(envelope *codec.Envelope, payload []any)) []byte {
	t.Helper()
	envelope, err := codec.StrictDecodeEnvelope(genesisFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	payload := envelope.Value.([]any)
	mutate(&envelope, payload)
	corrupt, err := codec.CanonicalBytes(envelope.DomainTag, envelope.SchemaID, envelope.SchemaVersion, payload)
	if err != nil {
		t.Fatal(err)
	}
	return corrupt
}

func requireRejectedGenesis(t *testing.T, encoded []byte) {
	t.Helper()
	bundle, err := policy.Load(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateCanonical(encoded, bundle); err == nil {
		t.Fatal("accepted invalid genesis")
	}
}

func TestRejectGenesisWithMismatchedPolicyVersionOrHash(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(ref []any)
	}{
		{"version", func(ref []any) { ref[1] = uint64(2) }},
		{"hash", func(ref []any) { ref[2] = bytes.Repeat([]byte{0xff}, 32) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			encoded := mutateGenesis(t, func(_ *codec.Envelope, payload []any) {
				ref := payload[14].([]any)[0].([]any)
				test.mutate(ref)
			})
			requireRejectedGenesis(t, encoded)
		})
	}
}

func TestRejectGenesisFieldsOutsideManifestContract(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(envelope *codec.Envelope, payload []any)
	}{
		{"domain-tag", func(envelope *codec.Envelope, _ []any) { envelope.DomainTag = "cote.csf.record.event" }},
		{"extension-bundles", func(_ *codec.Envelope, payload []any) { payload[12] = []any{[]any{"extension", []byte{1}}} }},
		{"exogenous-source-ids", func(_ *codec.Envelope, payload []any) { payload[13] = []any{uint64(1)} }},
		{"schemas", func(_ *codec.Envelope, payload []any) {
			payload[15] = []any{[]any{"cote.csf.schema.unknown", uint64(1)}}
		}},
		{"initial-world-state", func(_ *codec.Envelope, payload []any) { payload[16] = nil }},
		{"epistemic-checkpoints", func(_ *codec.Envelope, payload []any) { payload[17] = true }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requireRejectedGenesis(t, mutateGenesis(t, test.mutate))
		})
	}
}

func TestRejectIntegralFloatForUnsignedGenesisField(t *testing.T) {
	corrupt := mutateGenesis(t, func(_ *codec.Envelope, payload []any) { payload[6] = float64(1) })
	requireRejectedGenesis(t, corrupt)
}

func TestRejectInitialWorldStateWithWrongSchemaShapeEvenWhenDigestMatches(t *testing.T) {
	corrupt := mutateGenesis(t, func(_ *codec.Envelope, payload []any) {
		inner, err := codec.CanonicalBytes("cote.csf.snapshot.pending-state", "cote.csf.schema.snapshot-pending-state", 1, true)
		if err != nil {
			t.Fatal(err)
		}
		digest := identity.OpaqueContentHash(inner)
		payload[16] = []any{"cote.csf.schema.snapshot-pending-state", uint64(1), inner, digest[:]}
	})
	requireRejectedGenesis(t, corrupt)
}

func TestRejectInitialWorldStateWithMalformedNestedValuesEvenWhenDigestMatches(t *testing.T) {
	validCompletion := []any{bytes.Repeat([]byte{3}, 32), bytes.Repeat([]byte{4}, 32), []any{}, []any{}, bytes.Repeat([]byte{5}, 32)}
	tests := []struct {
		name    string
		section int
		value   any
	}{
		{name: "occurrence-causal-ref-shape", section: 0, value: []any{true}},
		{name: "pending-unit-input-ref-discriminant", section: 7, value: []any{uint64(0), uint64(9), bytes.Repeat([]byte{1}, 32), bytes.Repeat([]byte{2}, 32)}},
		{name: "perception-completion-field-count", section: 9, value: []any{validCompletion[0], validCompletion[1], []any{}, []any{}}},
		{name: "trigger-runtime-state-field-type", section: 1, value: []any{bytes.Repeat([]byte{1}, 32), uint64(1), []any{uint64(1), uint64(1)}, true, false, uint64(0), []any{uint64(0)}, uint64(0)}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			corrupt := mutateGenesis(t, func(_ *codec.Envelope, payload []any) {
				state := make([]any, 10)
				for i := range state {
					state[i] = []any{}
				}
				state[test.section] = []any{test.value}
				inner, err := codec.CanonicalBytes("cote.csf.snapshot.pending-state", "cote.csf.schema.snapshot-pending-state", 1, state)
				if err != nil {
					t.Fatal(err)
				}
				digest := identity.OpaqueContentHash(inner)
				payload[16] = []any{"cote.csf.schema.snapshot-pending-state", uint64(1), inner, digest[:]}
			})
			requireRejectedGenesis(t, corrupt)
		})
	}
}

func TestRejectInitialPendingStateWithUnseededAuthoritativeReferences(t *testing.T) {
	tests := []struct {
		name    string
		section int
		value   any
	}{
		{
			name:    "occurrence-ref-wrong-kind",
			section: 0,
			value:   []any{uint64(17), bytes.Repeat([]byte{0x11}, 32), bytes.Repeat([]byte{0x22}, 32)},
		},
		{
			name:    "occurrence-ref-orphan",
			section: 0,
			value:   []any{uint64(6), bytes.Repeat([]byte{0x33}, 32), bytes.Repeat([]byte{0x44}, 32)},
		},
		{
			name:    "pending-input-orphan",
			section: 7,
			value:   []any{uint64(1), bytes.Repeat([]byte{0x55}, 32), bytes.Repeat([]byte{0x66}, 32)},
		},
		{
			name:    "trigger-runtime-orphan",
			section: 1,
			value: []any{
				bytes.Repeat([]byte{0x77}, 32), uint64(1), []any{uint64(0)},
				true, false, uint64(0), []any{uint64(0)}, uint64(0),
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			corrupt := mutateGenesis(t, func(_ *codec.Envelope, payload []any) {
				typed := payload[16].([]any)
				inner, err := codec.StrictDecodeEnvelope(typed[2].([]byte))
				if err != nil {
					t.Fatal(err)
				}
				state := inner.Value.([]any)
				state[test.section] = []any{test.value}
				encoded, err := codec.CanonicalBytes(inner.DomainTag, inner.SchemaID, inner.SchemaVersion, state)
				if err != nil {
					t.Fatal(err)
				}
				digest := identity.OpaqueContentHash(encoded)
				payload[16] = []any{typed[0], typed[1], encoded, digest[:]}
			})
			requireRejectedGenesis(t, corrupt)
		})
	}
}

func TestRejectGenesisWithUnverifiedExternalDependencies(t *testing.T) {
	framedParent, err := hex.DecodeString(
		"435346485245460001001a636f74652e6373662e636f6465632e63626f722d6465742e76310020000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f0016636f74652e6373662e686173682e736e617073686f740018636f74652e6373662e736368656d612e736e617073686f74000000000000000100077368612d3235360020000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f",
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name   string
		mutate func([]any)
	}{
		{
			name: "unloaded-extension-bundle",
			mutate: func(payload []any) {
				payload[12] = []any{[]any{"missing-extension", bytes.Repeat([]byte{0x5a}, 32)}}
			},
		},
		{
			name: "malformed-parent-checkpoint",
			mutate: func(payload []any) {
				payload[4] = []any{uint64(1), []byte{0x00}}
			},
		},
		{
			name: "well-framed-but-unresolved-parent-checkpoint",
			mutate: func(payload []any) {
				payload[4] = []any{uint64(1), framedParent}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			encoded := mutateGenesis(t, func(_ *codec.Envelope, payload []any) {
				test.mutate(payload)
			})
			requireRejectedGenesis(t, encoded)
		})
	}
}

func TestProductionGenesisRejectsFixtureOperationAndSchema(t *testing.T) {
	fixture, err := os.ReadFile(bundlePath + "/fixtures.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Semantic []struct {
			CaseID     string `json:"case_id"`
			GenesisHex string `json:"genesis_envelope_hex"`
		} `json:"semantic"`
	}
	if err := json.Unmarshal(fixture, &doc); err != nil {
		t.Fatal(err)
	}
	for _, item := range doc.Semantic {
		if item.CaseID == "genesis.pinning" {
			encoded, err := hex.DecodeString(item.GenesisHex)
			if err != nil {
				t.Fatal(err)
			}
			requireRejectedGenesis(t, encoded)
			return
		}
	}
	t.Fatal("genesis fixture not found")
}

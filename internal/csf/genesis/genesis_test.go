package genesis

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
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
			return encoded
		}
	}
	t.Fatal("genesis fixture not found")
	return nil
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

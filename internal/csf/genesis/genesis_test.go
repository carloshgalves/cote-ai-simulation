package genesis

import (
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

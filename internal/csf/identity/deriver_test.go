package identity

import (
	"encoding/hex"
	"testing"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

const bundlePath = "../../../docs/architecture/canonical-codec-v1-bundle"

func TestDeriveOnlyAllowsRegisteredOperations(t *testing.T) {
	bundle, err := policy.Load(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	deriver := New(bundle)
	if _, _, err := deriver.Sum("cote.csf.hash.generic", "cote.csf.schema.genesis-id-preimage", 1, []any{}); err == nil {
		t.Fatal("accepted unregistered generic causal hash")
	}
	digest, _, err := deriver.Sum("cote.csf.id.genesis", "cote.csf.schema.genesis-id-preimage", 1, []any{make([]byte, 32)})
	if err != nil {
		t.Fatal(err)
	}
	if hex.EncodeToString(digest[:]) != "99f8c067aef2a5d219e01efad7304ac6ebf3ec1ef768f23860fbc349554103fe" {
		t.Fatalf("genesis id vector mismatch: %x", digest)
	}
}

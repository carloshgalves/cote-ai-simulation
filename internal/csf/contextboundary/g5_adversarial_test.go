package contextboundary

import (
	"context"
	"testing"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
)

// These adversarial inputs keep every changed record's digest and
// Observation/KnowledgeInput reference coherent. They must be denied by
// the *semantic* allowlist, not merely by a mismatch in CBOR hash.
func TestKnowledgeBoundaryG5CoherentlyRehashedForbiddenEvidence(t *testing.T) {
	ctx := context.Background()
	positiveObs, positiveInput := buildCommunicationTestPair(t)
	if view, err := Build(ctx, testInboxReader{
		observations: []ports.CanonicalRecord{positiveObs},
		inputs:       []ports.CanonicalRecord{positiveInput},
	}, [2]string{"school", "b"}); err != nil || len(view.Inbox) != 1 {
		t.Fatalf("valid received unverified claim missing from addressed context: %v", err)
	}
	for _, tc := range []struct {
		name  string
		alter func(*testing.T, []any)
	}{
		{"actual-sender-revealed", func(_ *testing.T, o []any) {
			o[11] = []any{"actual-sender-revealed", "claim-unverified"}
		}},
		{"claim-promoted-to-world-truth", func(_ *testing.T, o []any) {
			o[11] = []any{"actual-sender-redacted", "claim-verified"}
		}},
		{"duplicate-world-truth-reference", func(_ *testing.T, o []any) {
			claims := o[10].([]any)
			o[10] = append(claims, claims[0])
		}},
		{"privileged-physical-domain", func(t *testing.T, o []any) {
			replaceG5NoticeDomain(t, o, "cote.csf.body.synthetic-advance.v1")
		}},
		{"future-canon-domain", func(t *testing.T, o []any) {
			replaceG5NoticeDomain(t, o, "cote.csf.canon.future-after-divergence.v1")
		}},
		{"vip-exam-secret-domain", func(t *testing.T, o []any) {
			replaceG5NoticeDomain(t, o, "cote.csf.exam.vip-secret.v1")
		}},
		{"white-room-background-domain", func(t *testing.T, o []any) {
			replaceG5NoticeDomain(t, o, "cote.csf.background.white-room.v1")
		}},
		{"privileged-observatory-domain", func(t *testing.T, o []any) {
			replaceG5NoticeDomain(t, o, "cote.csf.observatory.privileged.v1")
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			decoded, err := codec.StrictDecodePayload(positiveObs.Canonical)
			if err != nil {
				t.Fatal(err)
			}
			obsFields := decoded.([]any)
			tc.alter(t, obsFields)
			obsDigest, err := sum("cote.csf.digest.observation", "cote.csf.schema.observation.body", obsFields[:14])
			if err != nil {
				t.Fatal(err)
			}
			obsFields[14] = obsDigest[:]
			obsRaw, err := codec.CanonicalPayload(obsFields)
			if err != nil {
				t.Fatal(err)
			}
			decodedInput, err := codec.StrictDecodePayload(positiveInput.Canonical)
			if err != nil {
				t.Fatal(err)
			}
			inputFields := decodedInput.([]any)
			inputFields[7].([]any)[2] = obsDigest[:]
			// Keep the evidence/claim tuple in the KnowledgeInput coherent
			// with the modified Observation. Otherwise this could fail
			// before it reaches the security property under examination.
			inputFields[8] = obsFields[10]
			inputDigest, err := sum("cote.csf.digest.knowledge-input", "cote.csf.schema.knowledge-input.body", inputFields[:10])
			if err != nil {
				t.Fatal(err)
			}
			inputFields[10] = inputDigest[:]
			inputRaw, err := codec.CanonicalPayload(inputFields)
			if err != nil {
				t.Fatal(err)
			}
			obs := ports.CanonicalRecord{Identity: positiveObs.Identity, Canonical: obsRaw}
			input := ports.CanonicalRecord{Identity: positiveInput.Identity, Canonical: inputRaw}
			if _, err := Build(ctx, testInboxReader{
				observations: []ports.CanonicalRecord{obs},
				inputs:       []ports.CanonicalRecord{input},
			}, [2]string{"school", "b"}); err == nil {
				t.Fatal("coherently rehashed privileged or truth-promoting input bypassed semantic allowlist")
			}
		})
	}
}

func replaceG5NoticeDomain(t *testing.T, observation []any, domain string) {
	t.Helper()
	percept := observation[9].([]any)[0].([]any)
	existing, err := codec.StrictDecodeEnvelope(percept[2].([]byte))
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := codec.CanonicalBytes(domain, "cote.csf.schema.typed-value", 1, existing.Value)
	if err != nil {
		t.Fatal(err)
	}
	digest := identity.OpaqueContentHash(encoded)
	percept[2] = encoded
	percept[3] = digest[:]
}

//go:build linux

package embodimentv1

import (
	"crypto/ed25519"
	"errors"
	"os"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/genesis"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

// SignedCut is a producer-attested pair of immutable physical source hashes.
// Only the producer's Genesis-pinned Ed25519 key may sign this record.
// This proves signed-pair provenance, not that an arbitrary producer actually
// serialized its Python writes; the A1 supervisor must enforce quiescence.
type SignedCut struct {
	RunID            domain.ID
	GenesisDigest    domain.Digest
	SourceID         string
	ExecutableSHA256 [32]byte
	SnapshotSHA256   [32]byte
	EventsSHA256     [32]byte
	Epoch            uint64
	Signature        [ed25519.SignatureSize]byte
}

// signedCutPayload is canonical, length-delimited CBOR. Transport JSON,
// wall_time, Python seq and filesystem paths cannot alter the signed pair.
func signedCutPayload(c SignedCut) ([]byte, error) {
	if c.SourceID == "" || c.Epoch == 0 ||
		c.SnapshotSHA256 == ([32]byte{}) || c.EventsSHA256 == ([32]byte{}) {
		return nil, ErrInvalidPhysical
	}
	return codec.CanonicalPayload([]any{
		"cote.embodiment.a1.signed-cut.v1",
		c.RunID[:], c.GenesisDigest[:], c.SourceID,
		c.ExecutableSHA256[:], c.SnapshotSHA256[:], c.EventsSHA256[:],
		c.Epoch,
	})
}

// InspectSignedSealedUnadmitted requires all three distinct controls:
// (1) installed, fingerprinted physical policy and an exact Genesis extension
// pin; (2) a signature over both immutable source hashes by the Genesis-pinned
// producer key; (3) actual Linux F_SEAL_WRITE source handles and A2 completion.
//
// A signature can be generated only by its holder, but the signer must still
// be supervised so that a single coherent cut is signed after its writer
// quiesces. This function never exposes an admission/commit capability.
func InspectSignedSealedUnadmitted(
	snapshot, events *os.File, evidence []byte,
	genesisCanonical []byte, installed *policy.Bundle, cut SignedCut,
) (Verified, error) {
	var zero Verified
	if installed == nil {
		return zero, ErrUnadmitted
	}
	registration, err := installed.PhysicalRegistration()
	if err != nil {
		return zero, ErrUnadmitted
	}
	g, err := genesis.ValidateCanonical(genesisCanonical, installed)
	if err != nil {
		return zero, ErrUnadmitted
	}
	if cut.RunID != g.RunID || cut.GenesisDigest != g.Digest ||
		cut.SourceID != registration.SourceID ||
		cut.ExecutableSHA256 != registration.ExecutableSHA256 ||
		registration.Disclosure != "ENGINE_ONLY" {
		return zero, ErrUnadmitted
	}
	payload, err := signedCutPayload(cut)
	if err != nil || !ed25519.Verify(
		ed25519.PublicKey(registration.PublicKeyEd25519[:]),
		payload, cut.Signature[:],
	) {
		return zero, ErrUnadmitted
	}
	if _, err := immutableMemfdBytes(snapshot, maxSnapshot); err != nil {
		return zero, errors.Join(ErrUnadmitted, ErrInvalidPhysical)
	}
	if _, err := immutableMemfdBytes(events, maxLog); err != nil {
		return zero, errors.Join(ErrUnadmitted, ErrInvalidPhysical)
	}
	return InspectSealedUnadmitted(snapshot, events, evidence, SourcePins{
		SnapshotSHA256: cut.SnapshotSHA256,
		EventsSHA256:   cut.EventsSHA256,
	})
}

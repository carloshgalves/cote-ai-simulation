//go:build linux

package embodimentv1

import (
	"bytes"
	"crypto/ed25519"
	"errors"
	"os"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/genesis"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/reducer"
)

// SignedCut is a producer-attested pair of immutable physical source hashes.
// Only the producer's Genesis-pinned Ed25519 key may sign this record.
// This proves signed-pair provenance, not that an arbitrary producer actually
// serialized its Python writes; the A1 supervisor must enforce quiescence.
type SignedCut struct {
	RunID                domain.ID
	GenesisDigest        domain.Digest
	SourceID             string
	ExecutableSHA256     [32]byte
	SnapshotSHA256       [32]byte
	EventsSHA256         [32]byte
	EvidenceSHA256       [32]byte
	CompletedStateSHA256 [32]byte
	Epoch                uint64
	Signature            [ed25519.SignatureSize]byte
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
		c.Epoch, c.EvidenceSHA256[:], c.CompletedStateSHA256[:],
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
	v, err := InspectSealedUnadmitted(snapshot, events, evidence, SourcePins{
		SnapshotSHA256: cut.SnapshotSHA256,
		EventsSHA256:   cut.EventsSHA256,
	})
	if err != nil {
		return zero, err
	}
	if cut.EvidenceSHA256 != ([32]byte{}) && identity.OpaqueContentHash(evidence) != cut.EvidenceSHA256 {
		return zero, ErrUnadmitted
	}
	if cut.CompletedStateSHA256 != ([32]byte{}) {
		typed, err := v.PhysicalCompletedState()
		if err != nil {
			return zero, err
		}
		digest, ok := typed[3].([]byte)
		if !ok || !bytes.Equal(digest, cut.CompletedStateSHA256[:]) {
			return zero, ErrUnadmitted
		}
	}
	return v, nil
}

// ApprovedCut is a capability created only after Genesis-pinned producer,
// signature and immutable A2 completion verification. It cannot write a ledger.
type ApprovedCut struct {
	cut       SignedCut
	typed     []any
	recovered bool // Only a previously persisted, authenticated ingress; NEVER stage this cut anew.
}

// InspectSignedSealedAuthorized requires signed A2 evidence and completed
// body state digests, not merely an unsigned source-pair declaration.
func InspectSignedSealedAuthorized(snapshot, events *os.File, evidence []byte,
	genesisCanonical []byte, installed *policy.Bundle, cut SignedCut) (ApprovedCut, error) {
	var zero ApprovedCut
	if cut.EvidenceSHA256 == ([32]byte{}) || cut.CompletedStateSHA256 == ([32]byte{}) {
		return zero, ErrUnadmitted
	}
	v, err := InspectSignedSealedUnadmitted(snapshot, events, evidence, genesisCanonical, installed, cut)
	if err != nil {
		return zero, err
	}
	typed, err := v.PhysicalCompletedState()
	if err != nil {
		return zero, err
	}
	return ApprovedCut{cut: cut, typed: typed}, nil
}


 
// IsDurablyRecovered distinguishes a producer-sealed ApprovedCut from a
// post-crash reconstruction. Recovered cuts are terminal-only and MUST NEVER
// be accepted by StagePhysicalInput to create new source authority.
func (a ApprovedCut) IsDurablyRecovered() bool { return a.recovered }

// RecoverApprovedCutFromDurableIngress reconstructs a terminal-only
// representation of a cut whose original sealed A1 capture has ALREADY
// been admitted into the authoritative InputLedger. This function is NOT
// a replacement for InspectSignedSealedAuthorized at initial ingestion.
// The caller must independently verify the stored source ID/digest,
// complete fence and immutable Genesis BEFORE using CommitPhysical.
// CommitPhysical repeats those ledger/fence validations under its CAS.
func RecoverApprovedCutFromDurableIngress(
	encoded, signature []byte, completed []any,
	reg policy.PhysicalRegistration, run domain.ID, genesisDigest domain.Digest,
) (ApprovedCut, error) {
	var zero ApprovedCut
	cut, err := DecodeSignedCut(encoded, signature)
	if err != nil {
		return zero, ErrUnadmitted
	}
	if err := VerifySignedCut(cut, reg, run, genesisDigest); err != nil {
		return zero, ErrUnadmitted
	}
	bodies, err := reducer.DecodePhysical(completed)
	if err != nil {
		return zero, ErrUnadmitted
	}
	normalized, _, err := reducer.EncodePhysical(bodies)
	if err != nil {
		return zero, ErrUnadmitted
	}
	originalBytes, err := codec.CanonicalPayload(completed)
	if err != nil {
		return zero, ErrUnadmitted
	}
	normalizedBytes, err := codec.CanonicalPayload(normalized)
	if err != nil || !bytes.Equal(originalBytes, normalizedBytes) {
		return zero, ErrUnadmitted
	}
	digest, ok := normalized[3].([]byte)
	if !ok || len(digest) != 32 || !bytes.Equal(digest, cut.CompletedStateSHA256[:]) {
		return zero, ErrUnadmitted
	}
	return ApprovedCut{cut: cut, typed: normalized, recovered: true}, nil
}

// CanonicalProof returns defensive copies for durable InputLedger ingress.
func (a ApprovedCut) CanonicalProof() ([]byte, []byte, []any, error) {
	if a.cut.CompletedStateSHA256 == ([32]byte{}) || len(a.typed) != 4 {
		return nil, nil, nil, ErrUnadmitted
	}
	body, err := codec.CanonicalPayload(a.typed)
	if err != nil {
		return nil, nil, nil, err
	}
	digest, ok := a.typed[3].([]byte)
	if !ok || !bytes.Equal(digest, a.cut.CompletedStateSHA256[:]) {
		return nil, nil, nil, ErrUnadmitted
	}
	cut, err := signedCutPayload(a.cut)
	if err != nil {
		return nil, nil, nil, err
	}
	copyBody, err := codec.StrictDecodePayload(body)
	if err != nil {
		return nil, nil, nil, err
	}
	return cut, append([]byte(nil), a.cut.Signature[:]...), copyBody.([]any), nil
}

// DecodeSignedCut validates the closed format, not its producer authority.
// A durable terminal must verify the signature against persisted Genesis.
func DecodeSignedCut(encoded, signature []byte) (SignedCut, error) {
	var c SignedCut
	if len(signature) != ed25519.SignatureSize {
		return c, ErrInvalidPhysical
	}
	value, err := codec.StrictDecodePayload(encoded)
	if err != nil {
		return c, ErrInvalidPhysical
	}
	fields, ok := value.([]any)
	if !ok || len(fields) != 10 || fields[0] != "cote.embodiment.a1.signed-cut.v1" {
		return c, ErrInvalidPhysical
	}
	take32 := func(index int, dest *[32]byte) bool {
		raw, ok := fields[index].([]byte)
		if !ok || len(raw) != 32 {
			return false
		}
		copy(dest[:], raw)
		return true
	}
	var run, g [32]byte
	if !take32(1, &run) || !take32(2, &g) ||
		!take32(4, &c.ExecutableSHA256) || !take32(5, &c.SnapshotSHA256) ||
		!take32(6, &c.EventsSHA256) || !take32(8, &c.EvidenceSHA256) ||
		!take32(9, &c.CompletedStateSHA256) {
		return SignedCut{}, ErrInvalidPhysical
	}
	c.RunID = domain.ID(run)
	c.GenesisDigest = domain.Digest(g)
	c.SourceID, ok = fields[3].(string)
	if !ok {
		return SignedCut{}, ErrInvalidPhysical
	}
	c.Epoch, ok = fields[7].(uint64)
	if !ok {
		return SignedCut{}, ErrInvalidPhysical
	}
	copy(c.Signature[:], signature)
	expected, err := signedCutPayload(c)
	if err != nil || !bytes.Equal(encoded, expected) {
		return SignedCut{}, ErrInvalidPhysical
	}
	return c, nil
}

// VerifySignedCut authenticates a closed source declaration against the
// installed Genesis-pinned producer key and semantic physical output.
func VerifySignedCut(c SignedCut, registration policy.PhysicalRegistration,
	run domain.ID, genesisDigest domain.Digest) error {
	if c.RunID != run || c.GenesisDigest != genesisDigest ||
		c.SourceID != registration.SourceID ||
		c.ExecutableSHA256 != registration.ExecutableSHA256 ||
		c.EvidenceSHA256 == ([32]byte{}) || c.CompletedStateSHA256 == ([32]byte{}) ||
		registration.Disclosure != "ENGINE_ONLY" {
		return ErrUnadmitted
	}
	encoded, err := signedCutPayload(c)
	if err != nil || !ed25519.Verify(ed25519.PublicKey(registration.PublicKeyEd25519[:]), encoded, c.Signature[:]) {
		return ErrUnadmitted
	}
	return nil
}

// SignableCutBytes yields the canonical signed record for a trusted host.
// This exports no private key, token issuer or causal publication handle.
func SignableCutBytes(c SignedCut) ([]byte, error) { return signedCutPayload(c) }

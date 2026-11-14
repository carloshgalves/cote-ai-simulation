package policy

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// PhysicalManifest is the complete v1 physical-extension contract. It must
// be canonical JSON and pinned by the immutable Genesis extension ref. There
// is no fallback to a similarly named, unregistered schema or action.
type PhysicalManifest struct {
	Kind string `json:"kind"`
	Version uint64 `json:"version"`
	ExtensionID string `json:"extension_id"`
	StateSchema string `json:"state_schema"`
	StateVersion uint64 `json:"state_version"`
	StateVariant string `json:"state_variant"`
	ActionType string `json:"action_type"`
	ActionVersion uint64 `json:"action_version"`
	ActionOperation string `json:"action_operation"`
	ActionProducer string `json:"action_producer"`
	SourceID string `json:"source_id"`
	ProducerExecutableSHA256 string `json:"producer_executable_sha256"`
	ProducerPublicKeyEd25519 string `json:"producer_public_key_ed25519"`
	Disclosure string `json:"disclosure"`
}

// PhysicalRegistration is a read-only view of the exact Genesis-pinnable
// registration. Its arrays contain copies of the private loader state.
type PhysicalRegistration struct {
	ExtensionID string
	BundleSHA256 [32]byte
	StateSchema string
	StateVersion uint64
	StateVariant string
	ActionType string
	ActionVersion uint64
	ActionOperation string
	ActionProducer string
	SourceID string
	ExecutableSHA256 [32]byte
	PublicKeyEd25519 [32]byte
	Disclosure string
}

// PhysicalExtensionV1 derives an installed bundle from a verified base
// bundle plus one strict physical manifest. The manifest is NOT trusted solely
// because this function returned a Bundle: Genesis must pin its exact hash,
// and each durable run must validate that pin before any physical operation.
func PhysicalExtensionV1(base *Bundle, raw []byte) (*Bundle, error) {
	if err := base.VerifyInstalledBundle(); err != nil { return nil, err }
	if base.physical != nil || len(raw) == 0 || len(raw) > 64*1024 {
		return nil, errors.New("invalid or duplicate physical extension")
	}
	var manifest PhysicalManifest
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&manifest); err != nil { return nil, fmt.Errorf("decode physical manifest: %w", err) }
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return nil, errors.New("physical manifest must contain exactly one canonical JSON object")
	}
	canonical, err := json.Marshal(manifest)
	if err != nil || !bytes.Equal(canonical, raw) {
		return nil, errors.New("physical manifest contains unknown, duplicate or noncanonical JSON")
	}
	if manifest.Kind != "cote.embodiment.physical-registration.v1" ||
		manifest.Version != 1 || manifest.ExtensionID != "cote.embodiment.physical.v1" ||
		manifest.StateSchema != "cote.csf.schema.typed-value" ||
		manifest.StateVersion != 1 || manifest.StateVariant != "physical.body-state.v1" ||
		manifest.ActionType != "physical.advance" || manifest.ActionVersion != 1 ||
		manifest.ActionOperation != "physical.advance" || manifest.ActionProducer != "physical.a1" ||
		manifest.SourceID != "physical.a1" || manifest.Disclosure != "ENGINE_ONLY" {
		return nil, errors.New("unregistered physical schema, action or disclosure")
	}
	executable, err := hex.DecodeString(manifest.ProducerExecutableSHA256)
	if err != nil || len(executable) != 32 || manifest.ProducerExecutableSHA256 != hex.EncodeToString(executable) {
		return nil, errors.New("invalid pinned producer executable digest")
	}
	key, err := hex.DecodeString(manifest.ProducerPublicKeyEd25519)
	if err != nil || len(key) != 32 || manifest.ProducerPublicKeyEd25519 != hex.EncodeToString(key) {
		return nil, errors.New("invalid pinned producer Ed25519 public key")
	}
	if bytes.Equal(executable, make([]byte, 32)) || bytes.Equal(key, make([]byte, 32)) {
		return nil, errors.New("zero producer pin is forbidden")
	}
	var executablePin, publicKey [32]byte
	copy(executablePin[:], executable)
	copy(publicKey[:], key)
	extension := PhysicalRegistration{
		ExtensionID: manifest.ExtensionID,
		BundleSHA256: sha256.Sum256(raw),
		StateSchema: manifest.StateSchema,
		StateVersion: manifest.StateVersion,
		StateVariant: manifest.StateVariant,
		ActionType: manifest.ActionType,
		ActionVersion: manifest.ActionVersion,
		ActionOperation: manifest.ActionOperation,
		ActionProducer: manifest.ActionProducer,
		SourceID: manifest.SourceID,
		ExecutableSHA256: executablePin,
		PublicKeyEd25519: publicKey,
		Disclosure: manifest.Disclosure,
	}
	derived := *base
	derived.physical = &extension
	derived.loadedFingerprint = [32]byte{}
	derived.loadedFingerprint, err = derived.RuntimeFingerprint()
	if err != nil { return nil, err }
	if err := derived.VerifyInstalledBundle(); err != nil { return nil, err }
	return &derived, nil
}

func (b *Bundle) PhysicalRegistration() (PhysicalRegistration, error) {
	if err := b.VerifyInstalledBundle(); err != nil { return PhysicalRegistration{}, err }
	if b.physical == nil { return PhysicalRegistration{}, errors.New("physical extension not registered") }
	return *b.physical, nil
}

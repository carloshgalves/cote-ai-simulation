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
	Kind                     string `json:"kind"`
	Version                  uint64 `json:"version"`
	ExtensionID              string `json:"extension_id"`
	StateSchema              string `json:"state_schema"`
	StateVersion             uint64 `json:"state_version"`
	StateVariant             string `json:"state_variant"`
	ActionType               string `json:"action_type"`
	ActionVersion            uint64 `json:"action_version"`
	ActionOperation          string `json:"action_operation"`
	ActionProducer           string `json:"action_producer"`
	SourceID                 string `json:"source_id"`
	ProducerExecutableSHA256 string `json:"producer_executable_sha256"`
	ProducerModuleSHA256     string `json:"producer_module_sha256"`
	ProducerImportTreeSHA256 string `json:"producer_import_tree_sha256"`
	ProducerPublicKeyEd25519 string `json:"producer_public_key_ed25519"`
	Disclosure               string `json:"disclosure"`
}

// PhysicalRegistration is a read-only view of the exact Genesis-pinnable
// registration. Its arrays contain copies of the private loader state.
type PhysicalRegistration struct {
	ExtensionID      string
	BundleSHA256     [32]byte
	StateSchema      string
	StateVersion     uint64
	StateVariant     string
	ActionType       string
	ActionVersion    uint64
	ActionOperation  string
	ActionProducer   string
	SourceID         string
	ExecutableSHA256 [32]byte
	ModuleSHA256     [32]byte
	ImportTreeSHA256 [32]byte
	PublicKeyEd25519 [32]byte
	Disclosure       string
}

// PhysicalExtensionV1 derives an installed bundle from a verified base
// bundle plus one strict physical manifest. The manifest is NOT trusted solely
// because this function returned a Bundle: Genesis must pin its exact hash,
// and each durable run must validate that pin before any physical operation.
func PhysicalExtensionV1(base *Bundle, raw []byte) (*Bundle, error) {
	if err := base.VerifyInstalledBundle(); err != nil {
		return nil, err
	}
	if base.physical != nil || len(raw) == 0 || len(raw) > 64*1024 {
		return nil, errors.New("invalid or duplicate physical extension")
	}
	var manifest PhysicalManifest
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&manifest); err != nil {
		return nil, fmt.Errorf("decode physical manifest: %w", err)
	}
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
	var modulePin [32]byte
	if manifest.ProducerModuleSHA256 != "" {
		module, err := hex.DecodeString(manifest.ProducerModuleSHA256)
		if err != nil || len(module) != 32 || manifest.ProducerModuleSHA256 != hex.EncodeToString(module) {
			return nil, errors.New("invalid producer module digest")
		}
		copy(modulePin[:], module)
		if modulePin == ([32]byte{}) {
			return nil, errors.New("zero producer module pin")
		}
	}
	packageRaw, err := hex.DecodeString(manifest.ProducerImportTreeSHA256)
	if err != nil || len(packageRaw) != 32 || manifest.ProducerImportTreeSHA256 != hex.EncodeToString(packageRaw) {
		return nil, errors.New("invalid pinned full Python import tree digest")
	}
	var treePin [32]byte
	copy(treePin[:], packageRaw)
	if treePin == ([32]byte{}) {
		return nil, errors.New("zero physical Python import tree pin forbidden")
	}
	var executablePin, publicKey [32]byte
	copy(executablePin[:], executable)
	copy(publicKey[:], key)
	extension := PhysicalRegistration{
		ExtensionID:      manifest.ExtensionID,
		BundleSHA256:     sha256.Sum256(raw),
		StateSchema:      manifest.StateSchema,
		StateVersion:     manifest.StateVersion,
		StateVariant:     manifest.StateVariant,
		ActionType:       manifest.ActionType,
		ActionVersion:    manifest.ActionVersion,
		ActionOperation:  manifest.ActionOperation,
		ActionProducer:   manifest.ActionProducer,
		SourceID:         manifest.SourceID,
		ExecutableSHA256: executablePin,
		ModuleSHA256:     modulePin,
		ImportTreeSHA256: treePin,
		PublicKeyEd25519: publicKey,
		Disclosure:       manifest.Disclosure,
	}
	derived := *base
	derived.physical = &extension
	derived.physicalManifest = bytes.Clone(raw)
	derived.loadedFingerprint = [32]byte{}
	derived.loadedFingerprint, err = derived.RuntimeFingerprint()
	if err != nil {
		return nil, err
	}
	if err := derived.VerifyInstalledBundle(); err != nil {
		return nil, err
	}
	return &derived, nil
}

func (b *Bundle) PhysicalRegistration() (PhysicalRegistration, error) {
	if err := b.VerifyInstalledBundle(); err != nil {
		return PhysicalRegistration{}, err
	}
	if b.physical == nil {
		return PhysicalRegistration{}, errors.New("physical extension not registered")
	}
	return *b.physical, nil
}

// PhysicalManifestCanonical returns the complete pinned extension bytes.
// InputLedger includes these so SQLite restore/fork can independently verify
// the cryptographic producer key without reopening mutable host policy paths.
func (b *Bundle) PhysicalManifestCanonical() ([]byte, error) {
	if err := b.VerifyInstalledBundle(); err != nil {
		return nil, err
	}
	if b.physical == nil || len(b.physicalManifest) == 0 {
		return nil, errors.New("physical manifest not installed")
	}
	if sha256.Sum256(b.physicalManifest) != b.physical.BundleSHA256 {
		return nil, errors.New("physical manifest changed")
	}
	return bytes.Clone(b.physicalManifest), nil
}

// ParsePhysicalManifest is the independent replay parser. The durable
// Genesis extension pin MUST be compared to BundleSHA256 by the caller;
// parsing alone does not grant trust in a user-supplied JSON manifest.
func ParsePhysicalManifest(raw []byte) (PhysicalRegistration, error) {
	var zero PhysicalRegistration
	if len(raw) == 0 || len(raw) > 64*1024 {
		return zero, errors.New("physical manifest size invalid")
	}
	var m PhysicalManifest
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&m); err != nil {
		return zero, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return zero, errors.New("physical manifest has trailing JSON")
	}
	canonical, err := json.Marshal(m)
	if err != nil || !bytes.Equal(canonical, raw) {
		return zero, errors.New("physical manifest is not canonical")
	}
	if m.Kind != "cote.embodiment.physical-registration.v1" || m.Version != 1 ||
		m.ExtensionID != "cote.embodiment.physical.v1" ||
		m.StateSchema != "cote.csf.schema.typed-value" || m.StateVersion != 1 ||
		m.StateVariant != "physical.body-state.v1" || m.ActionType != "physical.advance" ||
		m.ActionVersion != 1 || m.ActionOperation != "physical.advance" ||
		m.ActionProducer != "physical.a1" || m.SourceID != "physical.a1" ||
		m.Disclosure != "ENGINE_ONLY" {
		return zero, errors.New("unregistered physical extension")
	}
	getHex := func(encoded string) ([32]byte, error) {
		var hash [32]byte
		decoded, err := hex.DecodeString(encoded)
		if err != nil || len(decoded) != 32 || encoded != hex.EncodeToString(decoded) {
			return hash, errors.New("invalid physical extension digest")
		}
		copy(hash[:], decoded)
		if hash == ([32]byte{}) {
			return hash, errors.New("zero physical extension digest")
		}
		return hash, nil
	}
	executable, err := getHex(m.ProducerExecutableSHA256)
	if err != nil {
		return zero, err
	}
	module, err := getHex(m.ProducerModuleSHA256)
	if err != nil {
		return zero, err
	}
	key, err := getHex(m.ProducerPublicKeyEd25519)
	if err != nil {
		return zero, err
	}
	return PhysicalRegistration{
		ExtensionID: m.ExtensionID, BundleSHA256: sha256.Sum256(raw),
		StateSchema: m.StateSchema, StateVersion: m.StateVersion, StateVariant: m.StateVariant,
		ActionType: m.ActionType, ActionVersion: m.ActionVersion,
		ActionOperation: m.ActionOperation, ActionProducer: m.ActionProducer,
		SourceID: m.SourceID, ExecutableSHA256: executable, ModuleSHA256: module,
		PublicKeyEd25519: key, Disclosure: m.Disclosure,
	}, nil
}

// Package policy verifies and loads the immutable codec/schema bundle.
package policy

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
)

const (
	CodecPolicyID       = "cote.csf.codec.cbor-det.v1"
	CodecVersion        = 1
	IdentityAlgorithm   = "cote.csf.sha256.v1"
	UnicodeVersion      = "unicode-15.1.0"
	NormalizationID     = "unicode.npss.nfc.15.1.0"
	CodecPolicyHashHex  = "513111dc82a5e58c07aecdabf410633f5aa5418908d2461ef0dff0d9ae5d8203"
	SchemaBundleHashHex = "ef7c1e4cec18f1491e4b72bd7dd35f49a1c28d77a5bdd89d435122074f5cba18"
	SuiteHashHex        = "05a226340c4211045d6f3df8f034434f3ac336001158379833f7230df4038dfd"
)

type Bundle struct {
	Directory       string
	Profile         map[string]any
	Registries      map[string]any
	Transitions     map[string]any
	CodecPolicyHash [32]byte
	SchemaHash      [32]byte
	SuiteHash       [32]byte
	// Set exclusively by a verified loader after manifest and receipt validation.
	loadedFingerprint [32]byte
	// Canonical fixtures loaded once from verified manifest authority.
	fixtureBytes []byte
}

// RuntimeFingerprint identifies the exact parsed authority installed for
// historical checkpoint interpretation. This is an in-memory mutation seal,
// not a causal identity or a substitute for manifest verification at Load.
// Only the policy adapter may perform the underlying SHA256 operation.
func (b *Bundle) RuntimeFingerprint() ([32]byte,error) {
 if b==nil { return [32]byte{},fmt.Errorf("nil policy bundle") }
 data,err:=json.Marshal(struct {
  Directory string
  Profile map[string]any
  Registries map[string]any
  Transitions map[string]any
  CodecHash [32]byte
  SchemaHash [32]byte
  SuiteHash [32]byte
  FixtureDigest [32]byte
 }{b.Directory,b.Profile,b.Registries,b.Transitions,b.CodecPolicyHash,b.SchemaHash,b.SuiteHash,sha256.Sum256(b.fixtureBytes)})
 if err!=nil{return [32]byte{},err}
 return sha256.Sum256(data),nil
}

// VerifyInstalledBundle requires the private provenance seal written by a
// normative manifest loader. A caller cannot register a fabricated Bundle
// merely by copying a V1 policy ID or hash into exported struct fields.
func (b *Bundle) VerifyInstalledBundle() error {
 if b==nil || b.loadedFingerprint==([32]byte{}) {
  return fmt.Errorf("policy bundle was not installed by a verified loader")
 }
 current,err:=b.RuntimeFingerprint()
 if err!=nil{return fmt.Errorf("fingerprinting installed bundle: %w",err)}
 if current!=b.loadedFingerprint {
  return fmt.Errorf("policy bundle changed after normative manifest verification")
 }
 return nil
}


type Operation struct {
	DomainTag     string
	SchemaID      string
	SchemaVersion uint32
}

func (b *Bundle) Operation(domainTag, schemaID string, schemaVersion uint32, includeFixtures bool) (Operation, error) {
	keys := []string{"domain_operations"}
	if includeFixtures {
		keys = append(keys, "fixture_domain_operations")
	}
	for _, key := range keys {
		entries, ok := b.Registries[key].([]any)
		if !ok {
			return Operation{}, fmt.Errorf("operation registry %s missing", key)
		}
		for _, raw := range entries {
			fields, ok := raw.([]any)
			if !ok || len(fields) != 3 {
				return Operation{}, fmt.Errorf("invalid operation registry entry")
			}
			version, ok := fields[2].(float64)
			if fields[0] == domainTag && fields[1] == schemaID && ok && uint32(version) == schemaVersion {
				return Operation{DomainTag: domainTag, SchemaID: schemaID, SchemaVersion: schemaVersion}, nil
			}
		}
	}
	return Operation{}, fmt.Errorf("unregistered domain operation %s/%s@%d", domainTag, schemaID, schemaVersion)
}

type manifest struct {
	BundleID        string              `json:"bundle_id"`
	ManifestVersion uint32              `json:"manifest_version"`
	Artifacts       [][]json.RawMessage `json:"artifacts"`
}

func Load(directory string) (*Bundle, error) {
	if err := verifyReceiptFile(directory); err != nil {
		return nil, err
	}
	bundle := &Bundle{Directory: directory}
	checks := []struct {
		manifest string
		prefix   string
		expected string
		target   *[32]byte
	}{
		{"policy-manifest.json", "cote.csf.bundle.codec-policy.v1", CodecPolicyHashHex, &bundle.CodecPolicyHash},
		{"schema-manifest.json", "cote.csf.bundle.schema.v1", SchemaBundleHashHex, &bundle.SchemaHash},
		{"conformance-manifest.json", "cote.csf.bundle.conformance.v1", SuiteHashHex, &bundle.SuiteHash},
	}
	for _, check := range checks {
		hash, err := verifyManifest(directory, check.manifest, check.prefix)
		if err != nil {
			return nil, err
		}
		if hex.EncodeToString(hash[:]) != check.expected {
			return nil, fmt.Errorf("%s bootstrap hash mismatch", check.manifest)
		}
		*check.target = hash
	}
	if err := decodeStrictJSON(filepath.Join(directory, "profile.json"), &bundle.Profile); err != nil {
		return nil, err
	}
	if err := decodeStrictJSON(filepath.Join(directory, "registries.json"), &bundle.Registries); err != nil {
		return nil, err
	}
	if err := decodeStrictJSON(filepath.Join(directory, "causal-transition-contracts.json"), &bundle.Transitions); err != nil {
		return nil, err
	}
	if bundle.Profile["codec_policy_id"] != CodecPolicyID || bundle.Profile["codec_version"] != float64(CodecVersion) {
		return nil, fmt.Errorf("unsupported codec profile identity")
	}
	// Fixture policy refs participate in runtime authority. Freeze the exact
	// verified bundle bytes instead of reopening an attacker-replaceable file
	// during Genesis validation or cross-policy fork replay.
	fixtures,err:=os.ReadFile(filepath.Join(directory,"fixtures.json"))
	if err!=nil{return nil,fmt.Errorf("loading verified fixtures: %w",err)}
	bundle.fixtureBytes=bytes.Clone(fixtures)
	// Recheck the conformance manifest after the capture; it binds fixtures
	// to their registered artifact hash, including concurrent on-disk edits.
	if check,err:=verifyManifest(directory,"conformance-manifest.json","cote.csf.bundle.conformance.v1");err!=nil||hex.EncodeToString(check[:])!=SuiteHashHex{
		return nil,fmt.Errorf("fixture manifest changed during load: %v",err)
	}
	fingerprint,err:=bundle.RuntimeFingerprint()
	if err!=nil{return nil,fmt.Errorf("sealing verified policy: %w",err)}
	bundle.loadedFingerprint=fingerprint
	return bundle, nil
}

func verifyManifest(directory, name, prefix string) ([32]byte, error) {
	var zero [32]byte
	path := filepath.Join(directory, name)
	raw, err := os.ReadFile(path)
	if err != nil {
		return zero, err
	}
	var parsed manifest
	if err := decodeStrict(raw, &parsed); err != nil {
		return zero, fmt.Errorf("decode %s: %w", name, err)
	}
	if parsed.ManifestVersion != 1 || len(parsed.Artifacts) == 0 {
		return zero, fmt.Errorf("invalid %s version or empty artifact list", name)
	}
	seen := map[string]struct{}{}
	for _, fields := range parsed.Artifacts {
		if len(fields) != 3 {
			return zero, fmt.Errorf("invalid artifact tuple in %s", name)
		}
		var artifact string
		var size int64
		var expected string
		if json.Unmarshal(fields[0], &artifact) != nil || json.Unmarshal(fields[1], &size) != nil || json.Unmarshal(fields[2], &expected) != nil {
			return zero, fmt.Errorf("invalid artifact fields in %s", name)
		}
		if artifact == "" || filepath.Base(artifact) != artifact || strings.ContainsAny(artifact, `\\`) {
			return zero, fmt.Errorf("artifact paths in %s must be basenames", name)
		}
		if _, duplicate := seen[artifact]; duplicate {
			return zero, fmt.Errorf("duplicate artifact %s", artifact)
		}
		seen[artifact] = struct{}{}
		content, err := os.ReadFile(filepath.Join(directory, artifact))
		if err != nil {
			return zero, err
		}
		digest := sha256.Sum256(content)
		if int64(len(content)) != size || hex.EncodeToString(digest[:]) != expected {
			return zero, fmt.Errorf("artifact %s size/hash mismatch", artifact)
		}
	}
	return sha256.Sum256(append(append([]byte(prefix), 0), raw...)), nil
}

func verifyReceiptFile(directory string) error {
	raw, err := os.ReadFile(filepath.Join(directory, "BUNDLE.sha256"))
	if err != nil {
		return err
	}
	values := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		parts := strings.Fields(line)
		if len(parts) == 2 {
			values[parts[0]] = parts[1]
			continue
		}
		if len(parts) == 3 && parts[0] == "sha256" {
			content, readErr := os.ReadFile(filepath.Join(directory, parts[2]))
			if readErr != nil {
				return readErr
			}
			digest := sha256.Sum256(content)
			if hex.EncodeToString(digest[:]) != parts[1] {
				return fmt.Errorf("raw manifest receipt mismatch for %s", parts[2])
			}
			continue
		}
		if len(parts) != 2 {
			return fmt.Errorf("invalid BUNDLE.sha256 line")
		}
	}
	expected := map[string]string{"codec_policy_hash": CodecPolicyHashHex, "schema_bundle_hash": SchemaBundleHashHex, "conformance_suite_hash": SuiteHashHex}
	for key, value := range expected {
		if values[key] != value {
			return fmt.Errorf("bundle receipt %s mismatch", key)
		}
	}
	return nil
}

func decodeStrictJSON(path string, target any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return decodeStrict(raw, target)
}

func decodeStrict(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if decoder.More() {
		return fmt.Errorf("trailing JSON value")
	}
	return nil
}

func (b *Bundle) RequiredPolicyRoles() ([]string, error) {
	values, ok := b.Transitions["required_genesis_policy_roles"].([]any)
	if !ok {
		return nil, fmt.Errorf("required genesis policy roles missing")
	}
	roles := make([]string, len(values))
	for index, value := range values {
		role, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("invalid genesis policy role")
		}
		roles[index] = role
	}
	return roles, nil
}

// GenesisPolicyRefs returns the exact, canonically ordered policy refs pinned by
// this bundle. Callers must not synthesize refs from a recognized policy ID.
func (b *Bundle) GenesisPolicyRefs() ([][]any, error) {
	if err:=b.VerifyInstalledBundle();err!=nil{return nil,err}
	var fixtures map[string]any
	if err := decodeStrict(b.fixtureBytes, &fixtures); err != nil {
		return nil, err
	}
	var refs [][]any
	for _, raw := range fixtures["semantic"].([]any) {
		item := raw.(map[string]any)
		if item["case_id"] != "genesis.pinning" {
			continue
		}
		encoded, err := hex.DecodeString(item["genesis_envelope_hex"].(string))
		if err != nil {
			return nil, err
		}
		envelope, err := codec.StrictDecodeEnvelope(encoded)
		if err != nil {
			return nil, err
		}
		payload, ok := envelope.Value.([]any)
		if !ok || len(payload) != 18 {
			return nil, fmt.Errorf("canonical genesis fixture has invalid shape")
		}
		policies, ok := payload[14].([]any)
		if !ok {
			return nil, fmt.Errorf("canonical genesis policy set missing")
		}
		refs = make([][]any, len(policies))
		for index, rawRef := range policies {
			ref, ok := rawRef.([]any)
			if !ok || len(ref) != 3 {
				return nil, fmt.Errorf("invalid canonical genesis policy ref")
			}
			hash, hashOK := ref[2].([]byte)
			if !hashOK || len(hash) != 32 {
				return nil, fmt.Errorf("invalid canonical genesis policy hash")
			}
			refs[index] = []any{ref[0], ref[1], bytes.Clone(hash)}
		}
		break
	}
	if len(refs) == 0 {
		return nil, fmt.Errorf("canonical genesis policy fixture missing")
	}
	return refs, nil
}

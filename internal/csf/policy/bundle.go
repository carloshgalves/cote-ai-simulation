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
 CodecPolicyV2ID = "cote.csf.codec.cbor-det.v2"
 CodecPolicyV2HashHex = "528168c4032e53d4c8b50a9a890b0b857dfc85b3722211c39b523b8de645da89"
 SchemaBundleV2HashHex = "2316d97c90f577c285d0dae02fe6431a683e0ae636667036786c23e6795babf5"
 SuiteV2HashHex = "a92891972d8f2d136d59aa2dad3489f18380985c9e84ca4380a78850812b5c70"
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
	genesisRefs [][]any
 historicalV1 *Bundle
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
  HistoricalV1Hash [32]byte
 }{b.Directory,b.Profile,b.Registries,b.Transitions,b.CodecPolicyHash,b.SchemaHash,b.SuiteHash,sha256.Sum256(b.fixtureBytes),func()[32]byte{if b.historicalV1!=nil{return b.historicalV1.CodecPolicyHash};return [32]byte{}}()})
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
 if b.historicalV1!=nil {
  if err:=b.historicalV1.VerifyInstalledBundle();err!=nil{
   return fmt.Errorf("installed historical V1 policy mutated: %w",err)
  }
 }
 return nil
}


// CodecIdentity is the loader-verified codec ID/version and exact manifest
// hashes for this installed bundle. A cross-policy consumer must use this
// authority, never substitute compile-time V1 constants for a future V2.
func (b *Bundle) CodecIdentity() (string,uint64,[32]byte,[32]byte,error) {
 var zero [32]byte
 if err:=b.VerifyInstalledBundle();err!=nil{
  return "",0,zero,zero,err
 }
 id,ok:=b.Profile["codec_policy_id"].(string)
 version,versionOK:=b.Profile["codec_version"].(float64)
 if !ok||id==""||!versionOK||version<1||version>float64(^uint32(0))||
  float64(uint64(version))!=version {
  return "",0,zero,zero,fmt.Errorf("installed codec identity malformed")
 }
 return id,uint64(version),b.CodecPolicyHash,b.SchemaHash,nil
}

// ResolveHistoricalOrigin requires the exact policy ID/hash. V2 interprets
// historical checkpoints with its separately verified V1 policy, never by
// substituting the V2 manifest or a caller-selected codec.
func (b *Bundle) ResolveHistoricalOrigin(id string,hash [32]byte)(*Bundle,error){
 if err:=b.VerifyInstalledBundle();err!=nil{return nil,err}
 local,_,localHash,_,err:=b.CodecIdentity()
 if err!=nil{return nil,err}
 if id==local&&hash==localHash{return b,nil}
 if b.historicalV1==nil{return nil,fmt.Errorf("historical origin policy not installed")}
 originID,_,originHash,_,err:=b.historicalV1.CodecIdentity()
 if err!=nil{return nil,err}
 if id!=originID||hash!=originHash{return nil,fmt.Errorf("no policy with required historical ID and hash")}
 return b.historicalV1,nil
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

type normativeLoaderSpec struct {
 id string
 version uint64
 policy, schema, suite, domainSuffix string
}

// LoadV2 verifies the independently sealed V2 causal fork policy. V2 retains
// deterministic CBOR v1 wire encoding, but introduces normative, versioned
// fork-migration semantics and Genesis policy identity.
func LoadV2(directory string) (*Bundle,error) {
 return loadNormative(directory,normativeLoaderSpec{
  CodecPolicyV2ID,2,CodecPolicyV2HashHex,SchemaBundleV2HashHex,SuiteV2HashHex,"v2",
 })
}
func Load(directory string) (*Bundle,error){
 return loadNormative(directory,normativeLoaderSpec{
  CodecPolicyID,CodecVersion,CodecPolicyHashHex,SchemaBundleHashHex,SuiteHashHex,"v1",
 })
}
func loadNormative(directory string,spec normativeLoaderSpec) (*Bundle,error) {
 if err := verifyReceiptFile(directory,[3]string{spec.policy,spec.schema,spec.suite}); err != nil {
		return nil, err
	}
	bundle := &Bundle{Directory: directory}
	checks := []struct {
		manifest string
		prefix   string
		expected string
		target   *[32]byte
	}{
		{"policy-manifest.json", "cote.csf.bundle.codec-policy."+spec.domainSuffix, spec.policy, &bundle.CodecPolicyHash},
  {"schema-manifest.json", "cote.csf.bundle.schema."+spec.domainSuffix, spec.schema, &bundle.SchemaHash},
  {"conformance-manifest.json", "cote.csf.bundle.conformance."+spec.domainSuffix, spec.suite, &bundle.SuiteHash},
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
	if bundle.Profile["codec_policy_id"] != spec.id || bundle.Profile["codec_version"] != float64(spec.version) {
		return nil, fmt.Errorf("unsupported codec profile identity")
	}
	// Fixture policy refs participate in runtime authority. Freeze the exact
	// verified bundle bytes instead of reopening an attacker-replaceable file
	// during Genesis validation or cross-policy fork replay.
	fixtures,err:=os.ReadFile(filepath.Join(directory,"fixtures.json"))
	if err!=nil{return nil,fmt.Errorf("loading verified fixtures: %w",err)}
	bundle.fixtureBytes=bytes.Clone(fixtures)
	// Authenticate the CAPTURED bytes, not a second filesystem read:
	// a malicious swap-and-restore between ReadFile and verifyManifest
	// must not install unverified runtime policy references.
	if err:=verifyCapturedConformanceFixture(directory,bundle.fixtureBytes,bundle.SuiteHash,"cote.csf.bundle.conformance."+spec.domainSuffix);err!=nil{
		return nil,fmt.Errorf("verified Genesis fixture capture: %w",err)
	}
	bundle.genesisRefs,err=parseGenesisPolicyRefs(bundle.fixtureBytes)
 if err!=nil{return nil,fmt.Errorf("invalid signed Genesis fixture: %w",err)}
 if spec.version==2 {
  if err:=validateForkV2Contract(directory,bundle);err!=nil{return nil,err}
  originDir:=filepath.Join(filepath.Dir(directory),"canonical-codec-v1-bundle")
  historic,err:=Load(originDir)
  if err!=nil{return nil,fmt.Errorf("V2 requires installed signed historical V1: %w",err)}
  bundle.historicalV1=historic
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

// verifyCapturedConformanceFixture binds the bytes retained by Load to the
// normative, hash-pinned manifest. Reopening fixtures.json inside
// verifyManifest was insufficient: an adversary could return altered bytes
// for the first read and restore the file before manifest verification.
func verifyCapturedConformanceFixture(directory string,captured []byte,expected [32]byte,domain string) error {
 raw,err:=os.ReadFile(filepath.Join(directory,"conformance-manifest.json"))
 if err!=nil{return err}
 preimage:=append(append([]byte(domain),0),raw...)
 if sha256.Sum256(preimage)!=expected{
  return fmt.Errorf("fixture manifest not the pinned conformance authority")
 }
 var parsed manifest
 if err:=decodeStrict(raw,&parsed);err!=nil{return err}
 if parsed.ManifestVersion!=1{return fmt.Errorf("unknown conformance manifest version")}
 found:=false
 for _,fields:=range parsed.Artifacts {
  if len(fields)!=3{return fmt.Errorf("malformed conformance artifact")}
  var name,expectedHex string
  var size int64
  if json.Unmarshal(fields[0],&name)!=nil||json.Unmarshal(fields[1],&size)!=nil||
   json.Unmarshal(fields[2],&expectedHex)!=nil{return fmt.Errorf("invalid conformance artifact fields")}
  if name!="fixtures.json"{continue}
  if found{return fmt.Errorf("duplicate signed fixture entry")}
  found=true
  sum:=sha256.Sum256(captured)
  if int64(len(captured))!=size||hex.EncodeToString(sum[:])!=expectedHex{
   return fmt.Errorf("captured fixture bytes differ from signed manifest")
  }
 }
 if !found{return fmt.Errorf("conformance manifest omits Genesis fixture")}
 return nil
}

func verifyReceiptFile(directory string, hashes [3]string) error {
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
	expected := map[string]string{"codec_policy_hash": hashes[0], "schema_bundle_hash": hashes[1], "conformance_suite_hash": hashes[2]}
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
// GenesisPolicyRefs returns defensive copies of the references parsed once
// from the verified, frozen fixture. Callers cannot mutate the loader cache
// or make the policy depend on mutable files after installation.
func (b *Bundle) GenesisPolicyRefs() ([][]any, error) {
 if err:=b.VerifyInstalledBundle();err!=nil{return nil,err}
 if len(b.genesisRefs)==0{return nil,fmt.Errorf("installed Genesis fixture refs missing")}
 out:=make([][]any,len(b.genesisRefs))
 for i,ref:=range b.genesisRefs {
  if len(ref)!=3{return nil,fmt.Errorf("installed Genesis ref shape invalid")}
  hash,ok:=ref[2].([]byte)
  if !ok||len(hash)!=32{return nil,fmt.Errorf("installed Genesis ref hash invalid")}
  out[i]=[]any{ref[0],ref[1],bytes.Clone(hash)}
 }
 return out,nil
}

func parseGenesisPolicyRefs(raw []byte) ([][]any, error) {
	var fixtures map[string]any
	if err := decodeStrict(raw, &fixtures); err != nil {
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

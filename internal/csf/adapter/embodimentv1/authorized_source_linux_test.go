//go:build linux

package embodimentv1

import (
	"crypto/ed25519"
 "crypto/sha256"
 "os/exec"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/genesis"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/reducer"
)

const physicalBundlePath = "../../../../docs/architecture/canonical-codec-v1-bundle"

func signedCutFixture(t *testing.T) ([]byte, *policy.Bundle, SignedCut, ed25519.PrivateKey) {
	t.Helper()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	executablePin:=strings.Repeat("ab",32)
 modulePin:=strings.Repeat("ef",32)
 if os.Getenv("PHYSICAL_A1_PRODUCER_TEST")=="1"{
  py,err:=exec.LookPath("python")
  if err!=nil{t.Fatal(err)}
  py,err=filepath.EvalSymlinks(py)
  if err!=nil{t.Fatal(err)}
  executable,err:=os.ReadFile(py)
  if err!=nil{t.Fatal(err)}
  hash:=sha256.Sum256(executable)
  executablePin=hex.EncodeToString(hash[:])
  module,err:=os.ReadFile("../../../../src/embodiment/a1_capture.py")
  if err!=nil{t.Fatal(err)}
  moduleHash:=sha256.Sum256(module)
  modulePin=hex.EncodeToString(moduleHash[:])
 }
 base, err := policy.Load(physicalBundlePath)
	if err != nil {
		t.Fatal(err)
	}
	contract := policy.PhysicalManifest{
		Kind: "cote.embodiment.physical-registration.v1", Version: 1,
		ExtensionID: "cote.embodiment.physical.v1",
		StateSchema: reducer.PhysicalSchemaID, StateVersion: 1, StateVariant: reducer.PhysicalVariant,
		ActionType: "physical.advance", ActionVersion: 1, ActionOperation: "physical.advance", ActionProducer: "physical.a1",
		SourceID: "physical.a1", ProducerExecutableSHA256: executablePin,
  ProducerModuleSHA256: modulePin,
		ProducerPublicKeyEd25519: hex.EncodeToString(public), Disclosure: "ENGINE_ONLY",
	}
	raw, err := json.Marshal(contract)
	if err != nil {
		t.Fatal(err)
	}
	installed, err := policy.PhysicalExtensionV1(base, raw)
	if err != nil {
		t.Fatal(err)
	}
	registered, err := installed.PhysicalRegistration()
	if err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile(filepath.Join(physicalBundlePath, "fixtures.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixtures struct {
		Semantic []struct {
			CaseID     string `json:"case_id"`
			GenesisHex string `json:"genesis_envelope_hex"`
		} `json:"semantic"`
	}
	if err := json.Unmarshal(source, &fixtures); err != nil {
		t.Fatal(err)
	}
	var rawGenesis []byte
	for _, f := range fixtures.Semantic {
		if f.CaseID == "genesis.pinning" {
			rawGenesis, err = hex.DecodeString(f.GenesisHex)
			if err != nil {
				t.Fatal(err)
			}
			break
		}
	}
	if len(rawGenesis) == 0 {
		t.Fatal("no canonical Genesis fixture")
	}
	env, err := codec.StrictDecodeEnvelope(rawGenesis)
	if err != nil {
		t.Fatal(err)
	}
	fields := append([]any(nil), env.Value.([]any)...)
	fields[12] = []any{[]any{registered.ExtensionID, registered.BundleSHA256[:]}}
	fields[13] = []any{registered.SourceID}
	body := reducer.PhysicalBody{Subject: "npc.0001", WPrimeCapacity: 1, WPrimeTau: 1, WorkRestRatio: 1}
	for i := range body.Soreness {
		body.Soreness[i] = reducer.PhysicalSoreness{Latency: []float64{0}}
	}
	typed, _, err := reducer.EncodePhysical([]reducer.PhysicalBody{body})
	if err != nil {
		t.Fatal(err)
	}
	fields[15] = []any{[]any{reducer.PhysicalSchemaID, uint64(1)}}
	fields[16] = typed
	rawGenesis, err = codec.CanonicalBytes("cote.csf.digest.genesis-manifest", "cote.csf.schema.genesis-manifest", 1, fields)
	if err != nil {
		t.Fatal(err)
	}
	g, err := genesis.ValidateCanonical(rawGenesis, installed)
	if err != nil {
		t.Fatal(err)
	}
	return rawGenesis, installed, SignedCut{
		RunID: g.RunID, GenesisDigest: g.Digest, SourceID: registered.SourceID,
		ExecutableSHA256: registered.ExecutableSHA256,
		Epoch:            1,
	}, private
}
func signCut(t *testing.T, c *SignedCut, key ed25519.PrivateKey) {
	t.Helper()
	message, err := signedCutPayload(*c)
	if err != nil {
		t.Fatal(err)
	}
	copy(c.Signature[:], ed25519.Sign(key, message))
}

func TestSignedPhysicalPairPinnedByGenesisStillUnadmitted(t *testing.T) {
	source := realSource(t)
	genesisCanonical, installed, cut, key := signedCutFixture(t)
	cut.SnapshotSHA256 = source.PinnedSnapshotSHA256
	cut.EventsSHA256 = source.PinnedEventsSHA256
	signCut(t, &cut, key)
	s := testMemfd(t, source.Snapshot, requiredMemfdSeals)
	e := testMemfd(t, source.Events, requiredMemfdSeals)
	v, err := InspectSignedSealedUnadmitted(s, e, source.Evidence, genesisCanonical, installed, cut)
	if err != nil {
		t.Fatalf("signed PSV1-2 source rejected: %v", err)
	}
	if v.commandCount != 2 || v.transitionCount != 6 {
		t.Fatalf("unexpected verified source counters")
	}
	if _, ok := any(v).(interface{ AdmitToWorld() error }); ok {
		t.Fatal("signed source incorrectly gained commit capability")
	}
}

func TestSignedPhysicalCutRejectsTamperingReplayAndMissingAuthority(t *testing.T) {
	source := realSource(t)
	genesisCanonical, installed, base, key := signedCutFixture(t)
	base.SnapshotSHA256 = source.PinnedSnapshotSHA256
	base.EventsSHA256 = source.PinnedEventsSHA256
	signCut(t, &base, key)
	s := testMemfd(t, source.Snapshot, requiredMemfdSeals)
	e := testMemfd(t, source.Events, requiredMemfdSeals)
	check := func(name string, canonical []byte, bundle *policy.Bundle, cut SignedCut, view *os.File) {
		t.Helper()
		if _, err := InspectSignedSealedUnadmitted(view, e, source.Evidence, canonical, bundle, cut); err == nil {
			t.Fatalf("%s: untrusted source admitted", name)
		}
	}
	bad := base
	bad.EventsSHA256[0] ^= 1
	check("changed raw pin", genesisCanonical, installed, bad, s)
	bad = base
	bad.Epoch++
	check("replayed signature to different epoch", genesisCanonical, installed, bad, s)
	bad = base
	bad.SourceID = "physical.unsupervised"
	check("wrong source authority", genesisCanonical, installed, bad, s)
	bad = base
	bad.Signature[0] ^= 1
	check("bad signature", genesisCanonical, installed, bad, s)
	bad = base
	bad.GenesisDigest[0] ^= 1
	check("wrong Genesis", genesisCanonical, installed, bad, s)
	bad = base
	bad.ExecutableSHA256[0] ^= 1
	check("unverified producer binary", genesisCanonical, installed, bad, s)
	withoutExtension, err := policy.Load(physicalBundlePath)
	if err != nil {
		t.Fatal(err)
	}
	check("missing extension bootstrap", genesisCanonical, withoutExtension, base, s)
	unsealed := testMemfd(t, source.Snapshot, 0)
	check("mutable memfd", genesisCanonical, installed, base, unsealed)
	_, wrong, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	bad = base
	signCut(t, &bad, wrong)
	check("unregistered signer", genesisCanonical, installed, bad, s)
}

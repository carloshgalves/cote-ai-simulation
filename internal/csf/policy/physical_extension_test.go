package policy

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func physicalManifestFixture() []byte {
	m := PhysicalManifest{
		Kind: "cote.embodiment.physical-registration.v1", Version: 1,
		ExtensionID: "cote.embodiment.physical.v1",
		StateSchema: "cote.csf.schema.typed-value", StateVersion: 1, StateVariant: "physical.body-state.v1",
		ActionType: "physical.advance", ActionVersion: 1, ActionOperation: "physical.advance", ActionProducer: "physical.a1",
		SourceID: "physical.a1", ProducerExecutableSHA256: strings.Repeat("ab", 32),
		ProducerPublicKeyEd25519: strings.Repeat("cd", 32), Disclosure: "ENGINE_ONLY",
	}
	raw, _ := json.Marshal(m)
	return raw
}

func TestPhysicalPolicyRequiresVerifiedBundleAndExactManifest(t *testing.T) {
	base, err := Load(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	raw := physicalManifestFixture()
	if _, err := base.PhysicalRegistration(); err == nil {
		t.Fatal("base policy unexpectedly registered physical input")
	}
	derived, err := PhysicalExtensionV1(base, raw)
	if err != nil {
		t.Fatal(err)
	}
	reg, err := derived.PhysicalRegistration()
	if err != nil || reg.ActionType != "physical.advance" || reg.StateVariant != "physical.body-state.v1" {
		t.Fatalf("incorrect physical registration: %#v %v", reg, err)
	}
	if _, err := base.PhysicalRegistration(); err == nil {
		t.Fatal("installation mutated original base policy")
	}
	bad := append([]byte(nil), raw...)
	bad = append(bad, ' ')
	if _, err := PhysicalExtensionV1(base, bad); err == nil {
		t.Fatal("noncanonical manifest accepted")
	}
	if _, err := PhysicalExtensionV1(derived, raw); err == nil {
		t.Fatal("duplicate extension accepted")
	}
	if _, err := PhysicalExtensionV1(&Bundle{}, raw); err == nil {
		t.Fatal("unverified base accepted")
	}
	unknown := bytes.Replace(raw, []byte("physical.advance"), []byte("resource.consume"), 1)
	if _, err := PhysicalExtensionV1(base, unknown); err == nil {
		t.Fatal("unregistered ActionSchema accepted")
	}
	duplicate := bytes.Replace(raw, []byte(`"version":1`), []byte(`"version":1,"version":1`), 1)
	if _, err := PhysicalExtensionV1(base, duplicate); err == nil {
		t.Fatal("duplicate JSON key accepted")
	}
	mutated := *derived
	mutated.physical = &PhysicalRegistration{}
	if err := mutated.VerifyInstalledBundle(); err == nil {
		t.Fatal("mutated registration passed fingerprint")
	}
	if derived.VerifyInstalledBundle() != nil {
		t.Fatal("untampered registration failed provenance check")
	}
}

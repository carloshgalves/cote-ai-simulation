package policy

import (
	"os"
	"path/filepath"
	"testing"
)

const bundlePath = "../../../docs/architecture/canonical-codec-v1-bundle"

func TestLoadNormativeBundle(t *testing.T) {
	bundle, err := Load(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	roles, err := bundle.RequiredPolicyRoles()
	if err != nil {
		t.Fatal(err)
	}
	if len(roles) != 13 {
		t.Fatalf("want 13 required policy roles, got %d", len(roles))
	}
}

func TestBundleCorruptionFailsClosed(t *testing.T) {
	destination := t.TempDir()
	entries, err := os.ReadDir(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(bundlePath, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if entry.Name() == "profile.json" {
			raw = append(raw, ' ')
		}
		if err := os.WriteFile(filepath.Join(destination, entry.Name()), raw, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := Load(destination); err == nil {
		t.Fatal("accepted corrupted policy artifact")
	}
}

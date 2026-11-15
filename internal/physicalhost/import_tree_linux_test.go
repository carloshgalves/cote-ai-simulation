//go:build linux

package physicalhost

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPhysicalImportTreePinsTransitivePythonModulesAndRejectsSymlinks(t *testing.T) {
	repoRoot := t.TempDir()
	root := filepath.Join(repoRoot, "src")
	pkg := filepath.Join(root, "embodiment")
	if err := os.MkdirAll(pkg, 0o700); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string]string{
		"__init__.py":   "# package\n",
		"a1_capture.py": "from . import cli\n",
		"cli.py":        "MESSAGE = 'trusted'\n",
	} {
		if err := os.WriteFile(filepath.Join(pkg, name), []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	modelRoot := filepath.Join(repoRoot, "data", "models", "physical")
	if err := os.MkdirAll(filepath.Join(modelRoot, "schema"), 0o700); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string]string{
		"population-prior.yaml": "population: original\n",
		"body-dynamics.yaml": "physics: original\n",
		"schema/model-file.schema.json": "{}\n",
	} {
		if err := os.WriteFile(filepath.Join(modelRoot, name), []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	before, err := PhysicalImportTreeSHA256(root)
	if err != nil || before == ([32]byte{}) {
		t.Fatalf("closed producer package refused: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pkg, "cli.py"), []byte("MESSAGE = 'forged'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	after, err := PhysicalImportTreeSHA256(root)
	if err != nil || before == after {
		t.Fatalf("transitive Python module replacement was not detected: %v", err)
	}
	// The model files are part of the causal producer closure, not an
	// unauthenticated external input that can change physical outcomes.
	if err := os.WriteFile(filepath.Join(modelRoot, "body-dynamics.yaml"), []byte("physics: malicious\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	modelTampered, err := PhysicalImportTreeSHA256(root)
	if err != nil || modelTampered == after {
		t.Fatalf("physical parameter mutation escaped source pin: %v", err)
	}
	if err := os.Symlink(filepath.Join(pkg, "cli.py"), filepath.Join(pkg, "backdoor.py")); err != nil {
		t.Fatal(err)
	}
	if _, err := PhysicalImportTreeSHA256(root); err == nil {
		t.Fatal("symlink injected into Python module tree was accepted")
	}
	if err := os.Remove(filepath.Join(pkg, "backdoor.py")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkg, "native.so"), []byte("plugin"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := PhysicalImportTreeSHA256(root); err == nil {
		t.Fatal("unexpected extension module was accepted")
	}
	if err := os.Remove(filepath.Join(pkg, "native.so")); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(pkg, "cli.py"), 0o622); err != nil {
		t.Fatal(err)
	}
	if _, err := PhysicalImportTreeSHA256(root); err == nil {
		t.Fatal("group/world-writable producer source was accepted")
	}
}

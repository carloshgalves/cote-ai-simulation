//go:build linux

package physicalhost

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"golang.org/x/sys/unix"
)

// A writable/renamable host pathname must not be consulted at process launch
// after the executable bytes were authenticated. Kernel seals protect the
// inspected image from mutation as well as from accidental reuse.
func TestPinnedExecutorDoesNotReopenMutablePath(t *testing.T) {
	python, err := exec.LookPath("python")
	if err != nil {
		t.Skip("test requires Python")
	}
	python, err = filepath.EvalSymlinks(python)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(python)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "producer-executable")
	if err := os.WriteFile(path, raw, 0o500); err != nil {
		t.Fatal(err)
	}
	checked, err := securePinnedFile(path, 128*1024*1024)
	if err != nil || !bytes.Equal(raw, checked) {
		t.Fatalf("pinned executable not authenticated: %v", err)
	}
	pinned, err := pinnedExecutableFD(checked)
	if err != nil {
		t.Fatalf("sealed executable cannot be prepared: %v", err)
	}
	defer pinned.Close()
	seals, err := unix.FcntlInt(pinned.Fd(), unix.F_GET_SEALS, 0)
	if err != nil || seals&requiredMemfdSeals != requiredMemfdSeals {
		t.Fatalf("executable is not immutable: seals=%d err=%v", seals, err)
	}
	if _, err := pinned.WriteAt([]byte{0}, 0); err == nil {
		t.Fatal("sealed executable accepted a write")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\necho HIJACKED\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("/proc/self/fd/3", "-c", "print('SEALED_EXECUTABLE_OK')")
	cmd.ExtraFiles = []*os.File{pinned}
	cmd.Env = []string{"PYTHONHOME=" + filepath.Dir(filepath.Dir(python)), "LC_ALL=C.UTF-8"}
	output, err := cmd.CombinedOutput()
	if err != nil || string(output) != "SEALED_EXECUTABLE_OK\n" {
		t.Fatalf("pathname replacement affected pinned executable: err=%v output=%q", err, output)
	}
	if _, err := pinnedExecutableFD(nil); err == nil {
		t.Fatal("empty executable was accepted")
	}
}

// Python -s suppresses user site packages but still initializes site hooks;
// -S is mandatory so an unregistered sitecustomize.py or .pth startup hook
// never executes before the A1 source is authenticated.
func TestPhysicalPythonStartupDoesNotExecuteSiteCustomize(t *testing.T) {
	python, err := exec.LookPath("python")
	if err != nil {
		t.Skip("test requires Python")
	}
	python, err = filepath.EvalSymlinks(python)
	if err != nil {
		t.Fatal(err)
	}
	site := t.TempDir()
	marker := filepath.Join(t.TempDir(), "startup-hook-executed")
	payload := "open(" + strconv.Quote(marker) + ", 'w').write('injected')\n"
	if err := os.WriteFile(filepath.Join(site, "sitecustomize.py"), []byte(payload), 0o600); err != nil {
		t.Fatal(err)
	}
	environment := []string{"PYTHONPATH=" + site, "PYTHONHOME=" + filepath.Dir(filepath.Dir(python)), "LC_ALL=C.UTF-8"}
	unrestricted := exec.Command(python, "-B", "-s", "-c", "print('UNRESTRICTED')")
	unrestricted.Env = environment
	if out, err := unrestricted.CombinedOutput(); err != nil {
		t.Fatalf("negative control did not start: %v %q", err, out)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("negative control failed to exercise Python site hook: %v", err)
	}
	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}
	isolated := exec.Command(python, "-S", "-B", "-s", "-c", "print('NO_STARTUP_HOOK')")
	isolated.Env = environment
	out, err := isolated.CombinedOutput()
	if err != nil || string(out) != "NO_STARTUP_HOOK\n" {
		t.Fatalf("isolated Python startup failed: %v %q", err, out)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("sitecustomize executed despite isolated Python startup")
	}
}

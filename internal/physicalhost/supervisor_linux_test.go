//go:build linux

package physicalhost

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
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

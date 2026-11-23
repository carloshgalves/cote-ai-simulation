//go:build linux

package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHRejectsPublicOrSymlinkedEvidenceFiles(t *testing.T) {
	dir := t.TempDir()
	private := filepath.Join(dir, "private")
	if err := os.WriteFile(private, []byte("evidence"), 0600); err != nil {
		t.Fatal(err)
	}
	if got, err := readPrivateRegular(private); err != nil || string(got) != "evidence" {
		t.Fatalf("private regular file refused: %v", err)
	}
	exposed := filepath.Join(dir, "public")
	if err := os.WriteFile(exposed, []byte("exposed"), 0644); err != nil { t.Fatal(err) }
	if _, err := readPrivateRegular(exposed); err == nil {
		t.Fatal("public-readable owner evidence accepted")
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(private, link); err != nil { t.Fatal(err) }
	if _, err := readPrivateRegular(link); err == nil {
		t.Fatal("symlinked private archive input accepted")
	}
}

func TestHRejectsUnexpectedFilesAndPrivateOwnerSymlink(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "owner"), 0700); err != nil { t.Fatal(err) }
	if err := rejectExtraneous(dir); err != nil { t.Fatal(err) }
	foreign := filepath.Join(dir, "foreign.db")
	if err := os.WriteFile(foreign, []byte("foreign"), 0600); err != nil { t.Fatal(err) }
	if err := rejectExtraneous(dir); err == nil { t.Fatal("foreign archive file accepted") }
	if err := os.Remove(foreign); err != nil { t.Fatal(err) }
	if err := os.Remove(filepath.Join(dir, "owner")); err != nil { t.Fatal(err) }
	if err := os.Symlink(t.TempDir(), filepath.Join(dir, "owner")); err != nil { t.Fatal(err) }
	if err := rejectExtraneous(dir); err == nil { t.Fatal("foreign owner symlink accepted") }
}

func TestHManifestDetectsOneByteCorruptionBeforeAnyReplay(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil { t.Fatal(err) }
	if err := os.Mkdir(filepath.Join(dir, "owner"), 0700); err != nil { t.Fatal(err) }
	index := archiveIndex{Schema: "csfrv1-10h.local-hashes.v1"}
	for _, name := range expectedFiles {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.WriteFile(path, []byte("stub"), 0600); err != nil { t.Fatal(err) }
		index.Files = append(index.Files, fileRecord{
			Path: name, Size: 4, SHA256: hashBytes([]byte("stub")),
		})
	}
	if err := exclusiveJSON(filepath.Join(dir, "checksums.json"), index); err != nil { t.Fatal(err) }
	path := filepath.Join(dir, "owner", "actor-b.db")
	if err := os.WriteFile(path, []byte("evil"), 0600); err != nil { t.Fatal(err) }
	if err := verifyLocal(context.Background(), dir); err == nil ||
		!strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("tampered private owner accepted or reached replay: %v", err)
	}
}

func TestHIncludesWALSidecarAndRefusesSymlink(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "owner"), 0700); err != nil { t.Fatal(err) }
	sidecar := filepath.Join(dir, "causal.db-shm")
	if err := os.WriteFile(sidecar, []byte("private sqlite index"), 0600); err != nil { t.Fatal(err) }
	paths, err := collectedPaths(dir)
	if err != nil { t.Fatal(err) }
	if paths[len(paths)-1] != "causal.db-shm" {
		t.Fatal("retained SQLite sidecar omitted from archive inventory")
	}
	if err := rejectExtraneous(dir); err != nil { t.Fatalf("safe SQLite sidecar refused: %v", err) }
	if err := os.Remove(sidecar); err != nil { t.Fatal(err) }
	if err := os.Symlink(filepath.Join(dir, "owner"), sidecar); err != nil { t.Fatal(err) }
	if err := rejectExtraneous(dir); err == nil { t.Fatal("symbolic sidecar accepted") }
}

func TestHRefusesExistingOrSourceCheckoutOutput(t *testing.T) {
	exists := t.TempDir()
	if err := runLocal(context.Background(), exists); err == nil {
		t.Fatal("overwrote existing archive")
	}
	root, err := gitOutput(".", "rev-parse", "--show-toplevel")
	if err != nil { t.Fatal(err) }
	target := filepath.Join(root, "untracked-secret-h-output")
	if err := runLocal(context.Background(), target); err == nil {
		t.Fatal("permitted private owner DB or A1 evidence inside source checkout")
	}
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Fatalf("created rejected in-repository archive: %v", err)
	}
}

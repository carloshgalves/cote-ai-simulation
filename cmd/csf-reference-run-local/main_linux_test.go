//go:build linux

package main

import (
	"context"
	"encoding/json"
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

func TestHRejectsHardlinkedEvidence(t *testing.T) {
	dir := t.TempDir()
	original := filepath.Join(dir, "private.db")
	external := filepath.Join(dir, "alias.db")
	if err := os.WriteFile(original, []byte("private"), 0600); err != nil { t.Fatal(err) }
	if err := os.Link(original, external); err != nil { t.Fatal(err) }
	if _, err := readPrivateRegular(original); err == nil {
		t.Fatal("hardlinked private SQLite artifact accepted")
	}
}

func TestHStrictJSONAcceptsOnlyExactClosedReportShape(t *testing.T) {
	sample := archiveIndex{Schema: "csfrv1-10h.local-hashes.v1", Files: []fileRecord{}}
	raw, err := json.MarshalIndent(sample, "", "  ")
	if err != nil { t.Fatal(err) }
	raw = append(raw, '\n')
	if !exactLocalJSON(raw, sample) { t.Fatal("generated closed report unexpectedly refused") }
	withUnknown := []byte(strings.Replace(string(raw), "\"schema\":", "\"hidden_actor\": true, \"schema\":", 1))
	var parsed archiveIndex
	if err := json.Unmarshal(withUnknown, &parsed); err != nil { t.Fatal(err) }
	if exactLocalJSON(withUnknown, parsed) {
		t.Fatal("unregistered actor field accepted in checksum index")
	}
	withDuplicate := []byte(strings.Replace(string(raw), "\"schema\":", "\"schema\": \"forged\", \"schema\":", 1))
	if err := json.Unmarshal(withDuplicate, &parsed); err != nil { t.Fatal(err) }
	if exactLocalJSON(withDuplicate, parsed) {
		t.Fatal("duplicate JSON object key silently admitted")
	}
}

func TestHRefusesExistingOrSourceCheckoutOutput(t *testing.T) {
	exists := t.TempDir()
	if err := runLocal(context.Background(), exists); err == nil ||
		!strings.Contains(err.Error(), "requires fresh directory") {
		t.Fatalf("existing archive not rejected for expected reason: %v", err)
	}
	root, err := gitOutput(".", "rev-parse", "--show-toplevel")
	if err != nil { t.Fatal(err) }
	target := filepath.Join(root, "untracked-secret-h-output")
	if err := runLocal(context.Background(), target); err == nil ||
		!strings.Contains(err.Error(), "private evidence must be outside") {
		t.Fatalf("in-repo archive not rejected for expected reason: %v", err)
	}
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Fatalf("created rejected in-repository archive: %v", err)
	}
	alias := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(root, alias); err != nil { t.Fatal(err) }
	escape := filepath.Join(alias, "untracked-private")
	if err := runLocal(context.Background(), escape); err == nil ||
		!strings.Contains(err.Error(), "resolved private evidence") {
		t.Fatalf("in-repository symlink alias bypassed privacy guard: %v", err)
	}
}

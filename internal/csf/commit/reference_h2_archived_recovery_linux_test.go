//go:build linux

package commit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
	"github.com/carloshgalves/cote-ai-simulation/internal/physicalhost"
)

// H2-C consumes the exact private archive previously produced and cold-verified
// by csf-reference-run-local. No Python invocation or signing key is available
// in this test. SQLite is opened only on disposable copies or new authorities.
func TestH2ArchivedSourceCrashRecovery(t *testing.T) {
	root := os.Getenv("H2_ARCHIVED_SOURCE")
	if root == "" {
		if os.Getenv("H2_ARCHIVED_SOURCE_REQUIRED") == "1" {
			t.Fatal("required H2-C archive missing")
		}
		t.Skip("requires a real, cold-verified H2 CLI archive")
	}
	if !filepath.IsAbs(root) {
		t.Fatal("H2-C requires an absolute archive path")
	}
	for _, dir := range []string{root, filepath.Join(root, "owner"), filepath.Join(root, "physical-cuts"),
		filepath.Join(root, "physical-cuts", "epoch-1"), filepath.Join(root, "physical-cuts", "epoch-2")} {
		info, err := os.Lstat(dir)
		if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
			t.Fatal("H2-C requires real private archive directories")
		}
	}

	indexRaw, err := h2ArchivedRead(filepath.Join(root, "checksums.json"))
	if err != nil {
		t.Fatal("H2-C checksum index unavailable")
	}
	var index struct {
		Schema string `json:"schema"`
		Files  []struct {
			Path   string `json:"path"`
			Size   int64  `json:"size"`
			SHA256 string `json:"sha256"`
		} `json:"files"`
	}
	if err := json.Unmarshal(indexRaw, &index); err != nil || index.Schema != "csfrv1-10h.local-hashes.v1" {
		t.Fatal("H2-C invalid checksum index")
	}
	checks := make(map[string]string)
	sizes := make(map[string]int64)
	for _, file := range index.Files {
		if _, exists := checks[file.Path]; exists || file.Path == "" || filepath.IsAbs(file.Path) ||
			filepath.Clean(file.Path) != file.Path || file.Path == ".." {
			t.Fatal("H2-C invalid checksum entry")
		}
		checks[file.Path], sizes[file.Path] = file.SHA256, file.Size
	}
	read := func(name string) []byte {
		t.Helper()
		raw, err := h2ArchivedRead(filepath.Join(root, name))
		digest := sha256.Sum256(raw)
		if err != nil || int64(len(raw)) != sizes[name] || hex.EncodeToString(digest[:]) != checks[name] {
			t.Fatal("H2-C archive entry failed checksum:", name)
		}
		return raw
	}

	registration := read("registration.json")
	base, err := policy.Load(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := policy.PhysicalExtensionV1(base, registration)
	if err != nil {
		t.Fatal("H2-C invalid archived registration")
	}
	host := physicalhost.ProducerHost{Bundle: bundle}
	cfg := ReferenceD2Config{Host: host}
	var originals [2]OfflineOriginalA1Epoch
	for i, epoch := range []string{"epoch-1", "epoch-2"} {
		prefix := filepath.Join("physical-cuts", epoch)
		originals[i] = OfflineOriginalA1Epoch{
			Snapshot:  read(filepath.Join(prefix, "snapshot.yaml")),
			Events:    read(filepath.Join(prefix, "events.jsonl")),
			Evidence:  read(filepath.Join(prefix, "evidence.json")),
			Cut:       read(filepath.Join(prefix, "cut.cbor")),
			Signature: read(filepath.Join(prefix, "signature.bin")),
		}
	}
	cuts, err := verifyOfflineOriginalA1(cfg, originals)
	if err != nil {
		t.Fatal("H2-C sealed original A1 validation failed:", err)
	}

	// Baseline is the actual CLI-produced SQLite authority, not a newly
	// simulated baseline. Copy the exact indexed sidecars before reading.
	scratch := t.TempDir()
	if err := os.Mkdir(filepath.Join(scratch, "owner"), 0700); err != nil {
		t.Fatal(err)
	}
	dbs := []string{"causal.db", "owner/actor-b.db.actor-a", "owner/actor-b.db"}
	for _, db := range dbs {
		for _, suffix := range []string{"", "-wal", "-shm"} {
			name := db + suffix
			if _, ok := checks[name]; !ok && suffix != "" {
				if _, err := os.Lstat(filepath.Join(root, name)); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("H2-C unindexed SQLite sidecar")
				}
				continue
			}
			if err := os.WriteFile(filepath.Join(scratch, name), read(name), 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	expected, err := referenceE1Read(context.Background(), []referenceE1Location{
		{Name: "causal", Path: filepath.Join(scratch, dbs[0])},
		{Name: "owner-a", Path: filepath.Join(scratch, dbs[1])},
		{Name: "owner-b", Path: filepath.Join(scratch, dbs[2])},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := expected.requireCausalAndOwners(); err != nil {
		t.Fatal(err)
	}
	if len(expected.Databases["causal"].Tables["committed_cycles"].Rows) != 9 {
		t.Fatal("H2-C original archive lacks nine committed cycles")
	}
	runReferenceE2IntegratedMatrix(t, host, [2]physicalhost.ProducerPlan{}, cuts, &expected)
	if t.Failed() {
		return
	}
	// Recheck all consumed originals and the digest registry after the matrix.
	for _, name := range []string{"registration.json", "causal.db", "owner/actor-b.db", "owner/actor-b.db.actor-a"} {
		read(name)
	}
	for _, epoch := range []string{"epoch-1", "epoch-2"} {
		for _, name := range []string{"snapshot.yaml", "events.jsonl", "evidence.json", "cut.cbor", "signature.bin"} {
			read(filepath.Join("physical-cuts", epoch, name))
		}
	}
	after, err := h2ArchivedRead(filepath.Join(root, "checksums.json"))
	if err != nil || sha256.Sum256(after) != sha256.Sum256(indexRaw) {
		t.Fatal("H2-C original checksum index changed")
	}
	// Only this digest-only report is emitted, and only after all 14 cases
	// pass. No raw inputs or private actor material enter logs or artifacts.
	if report := os.Getenv("H2_ARCHIVED_RECOVERY_REPORT"); report != "" {
		digest := sha256.Sum256(indexRaw)
		raw, err := json.Marshal(struct {
			Status             string `json:"status"`
			ArchiveIndexSHA256 string `json:"archive_index_sha256"`
			Cases              int    `json:"cases"`
			Commits            int    `json:"commits"`
		}{"H2_C_ARCHIVED_SOURCE_RECOVERY", hex.EncodeToString(digest[:]), 14, 9})
		if err != nil {
			t.Fatal(err)
		}
		f, err := os.OpenFile(report, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write(append(raw, '\n')); err != nil {
			f.Close()
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func h2ArchivedRead(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() < 0 || info.Size() > 128<<20 {
		return nil, errors.New("H2-C invalid private evidence file")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	actual, err := f.Stat()
	if err != nil || !os.SameFile(info, actual) {
		return nil, errors.New("H2-C evidence identity changed")
	}
	native, ok := actual.Sys().(*syscall.Stat_t)
	if !ok || native.Nlink != 1 {
		return nil, errors.New("H2-C hardlinked evidence")
	}
	raw, err := io.ReadAll(io.LimitReader(f, info.Size()+1))
	if err != nil || int64(len(raw)) != info.Size() {
		return nil, errors.New("H2-C incomplete evidence")
	}
	after, err := f.Stat()
	if err != nil || !os.SameFile(info, after) || after.Size() != info.Size() {
		return nil, errors.New("H2-C evidence changed")
	}
	return raw, nil
}

//go:build linux

package commit

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/adapter/embodimentv1"
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
	indexHash := os.Getenv("H2_ARCHIVED_INDEX_SHA256")
	host, cuts, checks, read, indexRaw := h2ArchivedInputs(t, root, indexHash)

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
	completed := runReferenceE2IntegratedMatrix(t, host, [2]physicalhost.ProducerPlan{}, cuts, &expected,
		func(t *testing.T, db, owner string, epoch int, step uint8) {
			h2ArchivedFaultProcess(t, root, indexHash, db, owner, epoch, step)
		})
	if completed != 14 {
		t.Fatal("H2-C requires all fourteen fault cases")
	}
	if t.Failed() {
		return
	}
	// Recheck all consumed originals and the digest registry after the matrix.
	for _, name := range []string{"registration.json", "run-manifest.json"} {
		read(name)
	}
	for _, db := range dbs {
		for _, suffix := range []string{"", "-wal", "-shm"} {
			name := db + suffix
			if _, ok := checks[name]; ok {
				read(name)
			} else if _, err := os.Lstat(filepath.Join(root, name)); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("H2-C original SQLite sidecar set changed")
			}
		}
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
		}{"H2_C_ARCHIVED_SOURCE_RECOVERY", hex.EncodeToString(digest[:]), completed, 9})
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

// The parent and every fault process bind to the exact same checksum index
// that was captured before the CLI cold verifier. The CLI is still required
// to audit the complete package; this loader verifies consumed bytes again.
func h2ArchivedInputs(t *testing.T, root, expectedHash string) (physicalhost.ProducerHost, [2]embodimentv1.ApprovedCut, map[string]string, func(string) []byte, []byte) {
	t.Helper()
	if !filepath.IsAbs(root) || filepath.Clean(root) != root {
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
	digest := sha256.Sum256(indexRaw)
	if expectedHash == "" || hex.EncodeToString(digest[:]) != expectedHash {
		t.Fatal("H2-C archive index differs from the cold-verified input")
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
	canonical, err := json.MarshalIndent(index, "", "  ")
	if err != nil || !bytes.Equal(indexRaw, append(canonical, '\n')) {
		t.Fatal("H2-C checksum index is not canonical")
	}
	checks := make(map[string]string)
	sizes := make(map[string]int64)
	for _, file := range index.Files {
		if _, exists := checks[file.Path]; exists || file.Path == "" || filepath.IsAbs(file.Path) ||
			filepath.Clean(file.Path) != file.Path || file.Path == ".." || strings.HasPrefix(file.Path, "../") ||
			file.Size < 0 || len(file.SHA256) != 64 {
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

	var meta struct {
		EvidenceProfile string `json:"evidence_profile"`
	}
	if err := json.Unmarshal(read("run-manifest.json"), &meta); err != nil || meta.EvidenceProfile != "H2_SAME_A1_REPLAY_V1" {
		t.Fatal("H2-C requires the complete H2-I profile")
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

	return host, cuts, checks, read, indexRaw
}

const h2FaultExit = 87

// A separate executable instance owns the failing transaction. The normal
// injected failure returns after rollback; os.Exit then abandons this process.
// The next independent E2 child can recover only from the persisted fence.
func TestH2ArchivedFaultProcessChild(t *testing.T) {
	if os.Getenv("H2_C_FAULT_CHILD") != "1" {
		return
	}
	epoch, err := strconv.Atoi(os.Getenv("H2_C_FAULT_EPOCH"))
	if err != nil || epoch < 1 || epoch > 2 {
		t.Fatal("invalid H2-C child epoch")
	}
	step, err := strconv.Atoi(os.Getenv("H2_C_FAULT_STEP"))
	if err != nil || step < 1 || step > 7 {
		t.Fatal("invalid H2-C child step")
	}
	host, cuts, _, _, _ := h2ArchivedInputs(t, os.Getenv("H2_ARCHIVED_SOURCE"), os.Getenv("H2_ARCHIVED_INDEX_SHA256"))
	_, err = RunReferenceD2(context.Background(), os.Getenv("H2_C_FAULT_DB"), os.Getenv("H2_C_FAULT_OWNER"), bundlePath,
		ReferenceD2Config{Host: host, frozenSupervisedCuts: &cuts, FailPhysicalAtEpoch: epoch, FailPhysicalAfterStep: uint8(step)},
		ReferenceOptions{})
	if !referenceE2ExpectedFault(err, epoch) {
		t.Fatal("H2-C child did not reach the selected physical atomic rollback")
	}
	os.Exit(h2FaultExit)
}

func h2ArchivedFaultProcess(t *testing.T, root, indexHash, db, owner string, epoch int, step uint8) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestH2ArchivedFaultProcessChild$")
	// The child gets an explicit public-source input capability only; no
	// inherited producer configuration, private key, or Python runtime.
	cmd.Env = []string{
		"H2_C_FAULT_CHILD=1",
		"H2_ARCHIVED_SOURCE=" + root,
		"H2_ARCHIVED_INDEX_SHA256=" + indexHash,
		"H2_C_FAULT_DB=" + db,
		"H2_C_FAULT_OWNER=" + owner,
		"H2_C_FAULT_EPOCH=" + strconv.Itoa(epoch),
		"H2_C_FAULT_STEP=" + strconv.Itoa(int(step)),
		"GORACE=halt_on_error=1",
	}
	output, err := cmd.CombinedOutput()
	var exited *exec.ExitError
	if ctx.Err() != nil || !errors.As(err, &exited) || exited.ExitCode() != h2FaultExit || len(output) != 0 {
		t.Fatalf("H2-C isolated fault process did not reach rollback/exit: epoch=%d step=%d", epoch, step)
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

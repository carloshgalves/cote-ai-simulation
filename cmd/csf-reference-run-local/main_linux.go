//go:build linux

// Command csf-reference-run-local is an experimental offline A1/C4/D3
// reference runner. Its outputs are private, not production attestations.
package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/commit"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/reducer"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/runbook"
	"github.com/carloshgalves/cote-ai-simulation/internal/physicalhost"
)

const bundleRelative = "docs/architecture/canonical-codec-v1-bundle"

type runManifest struct {
	Schema             string `json:"schema"`
	Disposition        string `json:"disposition"`
	SourceSHA          string `json:"source_sha"`
	RunID              string `json:"run_id"`
	GenesisSHA256      string `json:"genesis_sha256"`
	RegistrationSHA256 string `json:"registration_sha256"`
	CodecPolicyHash    string `json:"codec_policy_hash"`
	SchemaBundleHash   string `json:"schema_bundle_hash"`
	Seed               int64  `json:"seed"`
	GoVersion          string `json:"go_version"`
	PythonVersion      string `json:"python_version"`
	SQLiteVersion      string `json:"sqlite_version"`
	HostOS             string `json:"host_os"`
	HostArch           string `json:"host_arch"`
	HostCPUs           int    `json:"host_cpus"`
	HostRAMBytes       uint64 `json:"host_ram_bytes"`
	StartedAtUTC       string `json:"started_at_utc"`
	EvidenceProfile    string `json:"evidence_profile,omitempty"`
	ReviewStatus       string `json:"review_status"`
}

type fileRecord struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

type archiveIndex struct {
	Schema string       `json:"schema"`
	Files  []fileRecord `json:"files"`
}

func main() {
	// All operations may create private scratch pager files, including
	// read-only detached diagnosis. Do not rely on the caller's umask.
	syscall.Umask(0077)
	os.Exit(localCommand(context.Background(), os.Args[1:], os.Stdout, os.Stderr))
}

func runLocal(ctx context.Context, output string) error {
	root, err := gitOutput(".", "rev-parse", "--show-toplevel")
	if err != nil {
		return err
	}
	sha, err := gitOutput(root, "rev-parse", "HEAD")
	if err != nil || len(sha) != 40 {
		return errors.New("exact source SHA unavailable")
	}
	dirty, err := gitOutput(root, "status", "--porcelain", "--untracked-files=all")
	if err != nil || dirty != "" {
		return errors.New("unclean checkout; refuse unversioned reference run")
	}
	out, err := filepath.Abs(output)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(root, out)
	if err != nil || rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))) {
		return errors.New("private evidence must be outside the source checkout")
	}
	realParent, err := filepath.EvalSymlinks(filepath.Dir(out))
	if err != nil {
		return err
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	resolved := filepath.Join(realParent, filepath.Base(out))
	realRel, err := filepath.Rel(realRoot, resolved)
	if err != nil || realRel == "." || (realRel != ".." && !strings.HasPrefix(realRel, ".."+string(filepath.Separator))) {
		return errors.New("resolved private evidence path enters source checkout")
	}
	if err := os.Mkdir(out, 0700); err != nil {
		return fmt.Errorf("requires fresh directory: %w", err)
	}
	if err := os.Mkdir(filepath.Join(out, "owner"), 0700); err != nil {
		return err
	}
	if err := os.Mkdir(filepath.Join(out, "physical-cuts"), 0700); err != nil {
		return fmt.Errorf("cannot initialize private A1 original source archive: %w", err)
	}
	// The per-run signer is used only inside the A1 supervisor. No private
	// key is put in an environment variable, manifest, archive, or model.
	host, plans, registration, pyVersion, cleanup, err := localPhysicalHost(root)
	if err != nil {
		return err
	}
	defer cleanup()
	// Preserve the public registration before the first causal write, so a
	// stopped run can be diagnosed without reconstructing a signing key.
	if err := exclusiveBytes(filepath.Join(out, "registration.json"), registration); err != nil {
		return err
	}
	run, err := commit.ReferenceRunID(host.Bundle)
	if err != nil {
		return err
	}
	fmt.Println("LOCAL_REFERENCE_SOURCE_SHA=" + sha)
	fmt.Println("LOCAL_REFERENCE_RUN_ID=" + hex.EncodeToString(run[:]))
	fmt.Println("LOCAL_REFERENCE_REGISTRATION_SHA256=" + hashBytes(registration))
	// Host-owned trusted archive target: A1 writes only AFTER sealed A2 and
	// registered Ed25519 verification, never to an actor-provided directory.
	host.PrivateCaptureRoot = filepath.Join(out, "physical-cuts")
	proof, err := commit.RunReferenceD3(ctx, filepath.Join(out, "causal.db"),
		filepath.Join(out, "owner", "actor-b.db"), filepath.Join(root, bundleRelative),
		commit.ReferenceD2Config{Host: host, EpochPlans: plans}, commit.ReferenceOptions{})
	if err != nil {
		return fmt.Errorf("real A1/C4/D3 source admission failed: %w", err)
	}
	public, err := proof.PublicReport()
	if err != nil {
		return err
	}
	if err := exclusiveJSON(filepath.Join(out, "report.json"), public); err != nil {
		return err
	}
	if public.RunID != hex.EncodeToString(run[:]) {
		return errors.New("completed reference run differs from the initial run pin")
	}
	genesisSHA, err := replayAndGenesis(ctx, filepath.Join(out, "causal.db"), host.Bundle, public.RunID)
	if err != nil {
		return fmt.Errorf("post-D3 local replay failed: %w", err)
	}
	manifest := runManifest{
		Schema:      "csfrv1-10h.local-manifest.v1",
		Disposition: "EXPERIMENTAL_LOCAL_ONLY_NO_PRODUCTION_GO",
		SourceSHA:   sha, RunID: public.RunID, GenesisSHA256: genesisSHA,
		RegistrationSHA256: hashBytes(registration),
		CodecPolicyHash:    hex.EncodeToString(host.Bundle.CodecPolicyHash[:]),
		SchemaBundleHash:   hex.EncodeToString(host.Bundle.SchemaHash[:]), Seed: 42,
		GoVersion: runtime.Version(), PythonVersion: pyVersion,
		SQLiteVersion: "3.53.4", HostOS: runtime.GOOS, HostArch: runtime.GOARCH,
		HostCPUs: runtime.NumCPU(), HostRAMBytes: localRAMBytes(),
		StartedAtUTC:    time.Now().UTC().Format(time.RFC3339Nano),
		EvidenceProfile: "H2_SAME_A1_REPLAY_V1",
		ReviewStatus:    "NOT_INDEPENDENTLY_ATTESTED",
	}
	if err := exclusiveJSON(filepath.Join(out, "run-manifest.json"), manifest); err != nil {
		return err
	}
	if err := exportH2Ledgers(ctx, out, sha, public.RunID, host.Bundle); err != nil {
		return fmt.Errorf("independent H2 canonical ledger export: %w", err)
	}
	if err := exportOwnersH2(ctx, out, sha, public.RunID); err != nil {
		return fmt.Errorf("H2 signed owner proof: %w", err)
	}
	if err := exportSignedIngressH2(ctx, out, sha, public.RunID, host.Bundle); err != nil {
		return fmt.Errorf("H2-S signed ingress provenance: %w", err)
	}
	if err := exportRawA1H2(ctx, out, sha, public.RunID, host.Bundle); err != nil {
		return fmt.Errorf("H2-R cold A1 original source archive verification: %w", err)
	}
	if err := runExactSameA1H2(ctx, out, root, sha, public.RunID, host, plans); err != nil {
		return fmt.Errorf("H2 independently re-executed same original A1 cuts: %w", err)
	}
	finalSHA, err := gitOutput(root, "rev-parse", "HEAD")
	if err != nil || finalSHA != sha {
		return errors.New("source SHA changed during local A1/D3 run")
	}
	finalDirty, err := gitOutput(root, "status", "--porcelain", "--untracked-files=all")
	if err != nil || finalDirty != "" {
		return errors.New("source changed during local A1/D3 run")
	}
	paths, err := collectedPaths(out)
	if err != nil {
		return err
	}
	files := make([]fileRecord, 0, len(paths))
	for _, path := range paths {
		raw, err := readPrivateRegular(filepath.Join(out, filepath.FromSlash(path)))
		if err != nil {
			return fmt.Errorf("missing completed D3 evidence %s: %w", path, err)
		}
		files = append(files, fileRecord{Path: path, Size: int64(len(raw)), SHA256: hashBytes(raw)})
	}
	if err := exclusiveJSON(filepath.Join(out, "checksums.json"),
		archiveIndex{Schema: "csfrv1-10h.local-hashes.v1", Files: files}); err != nil {
		return err
	}
	// Independent verification operates on a detached copy. The original
	// causal database and actor-owner roots are never reopened for writing.
	if err := verifyLocal(ctx, out); err != nil {
		return fmt.Errorf("%w: %w", runbook.ErrUnverified, err)
	}
	fmt.Println("FIRST_REFERENCE_RUN_LOCAL=EXECUTED_AND_CHECKED")
	fmt.Println("H2_FIVE_CANONICAL_LEDGERS=VERIFIED_PRIVATE")
	fmt.Println("H2_GENESIS_BOUND_SIGNED_INGRESS=VERIFIED_PRIVATE")
	fmt.Println("H2_RAW_PYTHON_A1_CUTS=SEALED_AND_VERIFIED_PRIVATE")
	fmt.Println("H2_SAME_SIGNED_A1_INDEPENDENT_GO_D3=BYTE_IDENTICAL_AUTHORITIES")
	fmt.Println("CSFRV1_10_V1_DOD_ACCEPTED=NO")
	fmt.Println("FIRST_REAL_RUN_PRODUCTION_GO=NO")
	fmt.Println("PRIVATE_EVIDENCE_DIRECTORY=" + out)
	return nil
}

func localPhysicalHost(root string) (physicalhost.ProducerHost, [2]physicalhost.ProducerPlan, []byte, string, func(), error) {
	var noHost physicalhost.ProducerHost
	var noPlans [2]physicalhost.ProducerPlan
	noCleanup := func() {}
	base, err := policy.Load(filepath.Join(root, bundleRelative))
	if err != nil {
		return noHost, noPlans, nil, "", noCleanup, err
	}
	python, err := exec.LookPath("python")
	if err != nil {
		return noHost, noPlans, nil, "", noCleanup, fmt.Errorf("%w: %w", runbook.ErrIncompatible, err)
	}
	python, err = filepath.EvalSymlinks(python)
	if err != nil {
		return noHost, noPlans, nil, "", noCleanup, err
	}
	raw, err := os.ReadFile(python)
	if err != nil || len(raw) == 0 || len(raw) > 128*1024*1024 {
		return noHost, noPlans, nil, "", noCleanup, errors.New("invalid pinned Python executable")
	}
	tmp, err := os.MkdirTemp("", "csf-local-a1-")
	if err != nil {
		return noHost, noPlans, nil, "", noCleanup, err
	}
	cleanup := func() { _ = os.RemoveAll(tmp) }
	if err := os.Chmod(tmp, 0700); err != nil {
		cleanup()
		return noHost, noPlans, nil, "", noCleanup, err
	}
	pinned := filepath.Join(tmp, "pinned-python")
	if err := os.WriteFile(pinned, raw, 0500); err != nil {
		cleanup()
		return noHost, noPlans, nil, "", noCleanup, err
	}
	importRoot := filepath.Join(root, "src")
	module, err := os.ReadFile(filepath.Join(importRoot, "embodiment", "a1_capture.py"))
	if err != nil {
		cleanup()
		return noHost, noPlans, nil, "", noCleanup, err
	}
	importHash, err := physicalhost.PhysicalImportTreeSHA256(importRoot)
	if err != nil {
		cleanup()
		return noHost, noPlans, nil, "", noCleanup, err
	}
	deps, err := exec.Command(python, "-S", "-c", "import sysconfig;print(sysconfig.get_path('purelib'))").Output()
	if err != nil {
		cleanup()
		return noHost, noPlans, nil, "", noCleanup, err
	}
	version, err := exec.Command(python, "--version").Output()
	if err != nil {
		cleanup()
		return noHost, noPlans, nil, "", noCleanup, err
	}
	if strings.TrimSpace(string(version)) != "Python 3.13.18" {
		cleanup()
		return noHost, noPlans, nil, "", noCleanup, runbook.ErrIncompatible
	}
	depRoot := strings.TrimSpace(string(deps))
	if !filepath.IsAbs(depRoot) {
		cleanup()
		return noHost, noPlans, nil, "", noCleanup, errors.New("Python dependency root is not absolute")
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		cleanup()
		return noHost, noPlans, nil, "", noCleanup, err
	}
	executableHash := sha256.Sum256(raw)
	moduleHash := sha256.Sum256(module)
	reg := policy.PhysicalManifest{
		Kind: "cote.embodiment.physical-registration.v1", Version: 1,
		ExtensionID: "cote.embodiment.physical.v1",
		StateSchema: reducer.PhysicalSchemaID, StateVersion: 1,
		StateVariant: reducer.PhysicalVariant,
		ActionType:   "physical.advance", ActionVersion: 1,
		ActionOperation: "physical.advance", ActionProducer: "physical.a1",
		SourceID:                 "physical.a1",
		ProducerExecutableSHA256: hex.EncodeToString(executableHash[:]),
		ProducerModuleSHA256:     hex.EncodeToString(moduleHash[:]),
		ProducerImportTreeSHA256: hex.EncodeToString(importHash[:]),
		ProducerPublicKeyEd25519: hex.EncodeToString(pub),
		Disclosure:               "ENGINE_ONLY",
	}
	registration, err := json.Marshal(reg)
	if err != nil {
		cleanup()
		return noHost, noPlans, nil, "", noCleanup, err
	}
	installed, err := policy.PhysicalExtensionV1(base, registration)
	if err != nil {
		cleanup()
		return noHost, noPlans, nil, "", noCleanup, err
	}
	plans := [2]physicalhost.ProducerPlan{
		{WorldSeed: 42, Count: 3, Advances: []physicalhost.ProducerAdvance{
			{Subject: "npc.0001", Days: 2, Sleep: "4h", Quality: "poor"},
			{Subject: "npc.0002", Days: 1, Sleep: "8h", Quality: "good"},
		}},
		{WorldSeed: 42, Count: 3, Advances: []physicalhost.ProducerAdvance{
			{Subject: "npc.0001", Days: 3, Sleep: "4h", Quality: "poor"},
			{Subject: "npc.0002", Days: 2, Sleep: "8h", Quality: "good"},
		}},
	}
	host := physicalhost.ProducerHost{
		Bundle: installed, PrivateKey: priv,
		ExecutablePath: pinned, PythonHome: filepath.Dir(filepath.Dir(python)),
		ImportRoot: importRoot, DependencyRoot: depRoot, PrivateStagingRoot: tmp,
	}
	return host, plans, registration, strings.TrimSpace(string(version)), cleanup, nil
}

func localRAMBytes() uint64 {
	var info syscall.Sysinfo_t
	if err := syscall.Sysinfo(&info); err != nil {
		return 0
	}
	return info.Totalram * uint64(info.Unit)
}

func gitOutput(root string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	raw, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(raw)), nil
}

func exclusiveBytes(path string, raw []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	n, writeErr := f.Write(raw)
	closeErr := f.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	if n != len(raw) {
		return errors.New("short artifact write")
	}
	return nil
}

func exclusiveJSON(path string, value any) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return exclusiveBytes(path, append(raw, '\n'))
}

func hashBytes(raw []byte) string {
	hash := sha256.Sum256(raw)
	return hex.EncodeToString(hash[:])
}

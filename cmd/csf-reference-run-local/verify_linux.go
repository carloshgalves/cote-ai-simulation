//go:build linux

package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"syscall"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/commit"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/runbook"
)

// All files are private operational material. Never upload the database,
// owner stores, raw A1 state, or full archived data to a public CI artifact.
var expectedFiles = []string{
	"causal.db",
	"owner/actor-b.db",
	"owner/actor-b.db.actor-a",
	"registration.json",
	"report.json",
	"run-manifest.json",
}

// A correctly closed SQLite WAL database may retain a -shm or -wal sidecar.
// Include exact optional files in the sealed archive; NEVER silently delete
// them or copy only the main db and omit a still-uncheckpointed WAL.
var optionalFiles = []string{
	"causal.db-wal", "causal.db-shm",
	"owner/actor-b.db-wal", "owner/actor-b.db-shm",
	"owner/actor-b.db.actor-a-wal", "owner/actor-b.db.actor-a-shm",
}

func collectedPaths(out string) ([]string, error) {
	paths := append([]string(nil), expectedFiles...)
	if info, err := os.Lstat(filepath.Join(out, "ledger-export")); err == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
			return nil, errors.New("H2 evidence must use a private real directory")
		}
		paths = append(paths, h2PrivatePaths...)
		if _, err := os.Lstat(filepath.Join(out, causalReportPathH2)); err == nil {
			paths = append(paths, causalReportPathH2)
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		if _, err := os.Lstat(filepath.Join(out, signedIngressProofPathH2)); err == nil {
			paths = append(paths, signedIngressProofPathH2)
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if info, err := os.Lstat(filepath.Join(out, "physical-cuts")); err == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
			return nil, errors.New("H2-R original source must use a private real directory")
		}
		for epoch := 1; epoch <= 2; epoch++ {
			epochDir := filepath.Join(out, "physical-cuts", fmt.Sprintf("epoch-%d", epoch))
			child, err := os.Lstat(epochDir)
			if err != nil || !child.IsDir() || child.Mode()&os.ModeSymlink != 0 ||
				child.Mode().Perm()&0077 != 0 {
				return nil, errors.New("H2-R original source epoch must be private and complete")
			}
		}
		paths = append(paths, rawA1PathsH2()...)
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if replayDir, err := os.Lstat(filepath.Join(out, "replay")); err == nil {
		if !replayDir.IsDir() || replayDir.Mode()&os.ModeSymlink != 0 ||
			replayDir.Mode().Perm()&0077 != 0 {
			return nil, errors.New("H2 same-cut replay directory is not private")
		}
		ownerDir, err := os.Lstat(filepath.Join(out, "replay", "owner"))
		if err != nil || !ownerDir.IsDir() || ownerDir.Mode()&os.ModeSymlink != 0 ||
			ownerDir.Mode().Perm()&0077 != 0 {
			return nil, errors.New("H2 same-cut replay owner directory missing or exposed")
		}
		paths = append(paths, replayRequiredPathsH2[:]...)
		for _, name := range replaySidecarPathsH2 {
			if _, err := os.Lstat(filepath.Join(out, filepath.FromSlash(name))); err == nil {
				paths = append(paths, name)
			} else if !errors.Is(err, os.ErrNotExist) {
				return nil, err
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	for _, name := range optionalFiles {
		_, err := os.Lstat(filepath.Join(out, filepath.FromSlash(name)))
		if err == nil {
			paths = append(paths, name)
		} else if !os.IsNotExist(err) {
			return nil, err
		}
	}
	return paths, nil
}

func readPrivateRegular(path string) ([]byte, error) {
	stat, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !stat.Mode().IsRegular() || stat.Mode().Perm()&0077 != 0 ||
		stat.Size() < 0 || stat.Size() > 128*1024*1024 {
		return nil, errors.New("refuse nonregular, exposed or oversized evidence")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	actual, err := file.Stat()
	if err != nil || !os.SameFile(stat, actual) {
		return nil, errors.New("evidence file identity changed during read")
	}
	native, ok := actual.Sys().(*syscall.Stat_t)
	if !ok || native.Nlink != 1 {
		return nil, errors.New("hardlinked or unverifiable private artifact")
	}
	raw, err := io.ReadAll(io.LimitReader(file, stat.Size()+1))
	if err != nil || int64(len(raw)) != stat.Size() {
		return nil, errors.New("cannot read complete evidence")
	}
	after, err := file.Stat()
	if err != nil || !os.SameFile(stat, after) || after.Size() != stat.Size() {
		return nil, errors.New("evidence inode changed during read")
	}
	return raw, nil
}

// Every JSON artifact is produced by a closed Go struct and its exact
// MarshalIndent representation. A document with extra fields, duplicate JSON
// keys, or noncanonical serialization cannot silently pass as a safe report.
func exactLocalJSON(raw []byte, decoded any) bool {
	canonical, err := json.MarshalIndent(decoded, "", "  ")
	return err == nil && bytes.Equal(raw, append(canonical, '\n'))
}

func rejectExtraneous(out string) error {
	seen := map[string]bool{}
	for _, name := range expectedFiles {
		seen[name] = true
	}
	for _, name := range optionalFiles {
		seen[name] = true
	}
	for _, name := range h2PrivatePaths {
		seen[name] = true
	}
	seen[signedIngressProofPathH2] = true
	seen[causalReportPathH2] = true
	for _, name := range rawA1PathsH2() {
		seen[name] = true
	}
	seen["physical-cuts"] = true
	seen["physical-cuts/epoch-1"] = true
	seen["physical-cuts/epoch-2"] = true
	for _, name := range replayRequiredPathsH2 {
		seen[name] = true
	}
	for _, name := range replaySidecarPathsH2 {
		seen[name] = true
	}
	seen["replay"] = true
	seen["replay/owner"] = true
	seen["ledger-export"] = true
	seen["checksums.json"] = true
	seen["owner"] = true
	return filepath.WalkDir(out, func(path string, info os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == out {
			return nil
		}
		rel, err := filepath.Rel(out, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if !seen[rel] || info.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("unknown or symlink evidence path: %s", rel)
		}
		if info.IsDir() {
			if rel != "owner" && rel != "ledger-export" &&
				rel != "physical-cuts" && rel != "physical-cuts/epoch-1" && rel != "physical-cuts/epoch-2" &&
				rel != "replay" && rel != "replay/owner" {
				return errors.New("unexpected evidence directory")
			}
			stat, err := info.Info()
			if err != nil || stat.Mode().Perm()&0077 != 0 {
				return errors.New("cognitive owner directory is not private")
			}
		}
		return nil
	})
}

func replayAndGenesis(ctx context.Context, original string, installed *policy.Bundle, runID string) (string, error) {
	// SQLite WAL recovery may discard malformed trailing frames. For an
	// archived reference-run oracle that would constitute silent data loss.
	if _, err := verifySQLiteSidecarsH2(original); err != nil {
		return "", err
	}
	raw, err := readPrivateRegular(original)
	if err != nil {
		return "", err
	}
	tmp, err := os.MkdirTemp("", "csf-h-verify-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)
	if err := os.Chmod(tmp, 0700); err != nil {
		return "", err
	}
	copyPath := filepath.Join(tmp, "detached.db")
	if err := os.WriteFile(copyPath, raw, 0600); err != nil {
		return "", err
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		sidecar, err := readPrivateRegular(original + suffix)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return "", err
		}
		if err := os.WriteFile(copyPath+suffix, sidecar, 0600); err != nil {
			return "", err
		}
	}
	// sqlite.Open uses WAL and may create tables. Run it ONLY on the detached
	// copy; never open the original reference archive for mutation.
	store, err := sqlite.Open(ctx, copyPath)
	if err != nil {
		return "", err
	}
	defer store.Close()
	idBytes, err := hex.DecodeString(runID)
	if err != nil || len(idBytes) != 32 {
		return "", errors.New("invalid run identity")
	}
	var id [32]byte
	copy(id[:], idBytes)
	genesis, err := store.ValidatedGenesis(ctx, id, installed)
	if err != nil {
		return "", fmt.Errorf("Genesis validation: %w", err)
	}
	replayed, err := store.ReplayClock(ctx, id, installed)
	if err != nil {
		return "", fmt.Errorf("replay validation: %w", err)
	}
	if replayed.ReplayedCommits != 9 || replayed.ReconstructedClock.Revision != 9 ||
		replayed.ReconstructedClock.Instant != 35 {
		return "", errors.New("unsealed temporal successor or wrong number of commits")
	}
	persisted, err := store.LoadClock(ctx, id)
	if err != nil || persisted != replayed.ReconstructedClock {
		return "", errors.New("clock differs from deterministic replay")
	}
	if _, _, err := store.LoadWorldState(ctx, 9); err != nil {
		return "", fmt.Errorf("terminal world revision is absent: %w", err)
	}
	return hashBytes(genesis.CanonicalBytes), nil
}

func verifyLocal(ctx context.Context, dir string) error {
	if ctx == nil {
		return errors.New("nil audit context")
	}
	root, err := gitOutput(".", "rev-parse", "--show-toplevel")
	if err != nil {
		return err
	}
	out, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	info, err := os.Lstat(out)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 ||
		info.Mode().Perm()&0077 != 0 {
		return errors.New("evidence root must be a private, real directory")
	}
	if err := rejectExtraneous(out); err != nil {
		return err
	}
	indexBytes, err := readPrivateRegular(filepath.Join(out, "checksums.json"))
	if err != nil {
		return err
	}
	var index archiveIndex
	if err := json.Unmarshal(indexBytes, &index); err != nil {
		return err
	}
	paths, err := collectedPaths(out)
	if err != nil {
		return err
	}
	if !exactLocalJSON(indexBytes, index) {
		return errors.New("noncanonical or extended archive checksum registry")
	}
	if index.Schema != "csfrv1-10h.local-hashes.v1" || len(index.Files) != len(paths) {
		return errors.New("unrecognized evidence digest registry or omitted SQLite sidecar")
	}
	for i, name := range paths {
		entry := index.Files[i]
		if entry.Path != name || len(entry.SHA256) != 64 {
			return errors.New("archive has foreign, reordered or missing files")
		}
		data, err := readPrivateRegular(filepath.Join(out, filepath.FromSlash(name)))
		if err != nil {
			return err
		}
		if int64(len(data)) != entry.Size || hashBytes(data) != entry.SHA256 {
			return fmt.Errorf("evidence checksum mismatch: %s", name)
		}
	}
	metaRaw, err := readPrivateRegular(filepath.Join(out, "run-manifest.json"))
	if err != nil {
		return err
	}
	var meta runManifest
	if err := json.Unmarshal(metaRaw, &meta); err != nil {
		return err
	}
	if !exactLocalJSON(metaRaw, meta) {
		return errors.New("noncanonical or extended run manifest")
	}
	sha, err := gitOutput(root, "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	if meta.Schema != "csfrv1-10h.local-manifest.v1" ||
		meta.SourceSHA != sha ||
		meta.CodecPolicyHash != policy.CodecPolicyHashHex ||
		meta.SchemaBundleHash != policy.SchemaBundleHashHex ||
		meta.GoVersion != runtime.Version() ||
		meta.PythonVersion != "Python 3.13.18" ||
		meta.HostOS != "linux" || meta.HostArch != runtime.GOARCH ||
		meta.SQLiteVersion != sqlite.ExpectedVersion {
		return runbook.ErrIncompatible
	}
	if meta.Disposition != "EXPERIMENTAL_LOCAL_ONLY_NO_PRODUCTION_GO" ||
		meta.ReviewStatus != "NOT_INDEPENDENTLY_ATTESTED" || meta.Seed != 42 ||
		(meta.EvidenceProfile != "" &&
			meta.EvidenceProfile != "H2_SIGNED_INGRESS_V1" &&
			meta.EvidenceProfile != "H2_RAW_SOURCE_V1" &&
			meta.EvidenceProfile != "H2_SAME_A1_REPLAY_V1") {
		return errors.New("wrong source policy or unapproved release disposition")
	}
	publicRaw, err := readPrivateRegular(filepath.Join(out, "report.json"))
	if err != nil {
		return err
	}
	var public commit.ReferenceD3PublicReport
	if err := json.Unmarshal(publicRaw, &public); err != nil {
		return err
	}
	if !exactLocalJSON(publicRaw, public) {
		return errors.New("noncanonical or extended public D3 report")
	}
	d := public.Disclosure
	if public.Status != "RUN_LOCAL_10D3" || public.RunID != meta.RunID ||
		d.PhysicalEvents != 2 || d.PhysicalPerceptionTasks != 0 ||
		!d.RecipientIsolationVerified || !d.ActorPhysicalDenied ||
		!d.PublicPhysicalDenied || d.PublicPublications != 0 ||
		d.ReplayCommits != 9 || d.SnapshotTailCommits != 3 ||
		!d.FiveLedgerPrefixStable || !d.ReadonlyVerified {
		return errors.New("D3 disclosure and receipt are not complete")
	}
	registration, err := readPrivateRegular(filepath.Join(out, "registration.json"))
	if err != nil {
		return err
	}
	if hashBytes(registration) != meta.RegistrationSHA256 {
		return errors.New("altered A1 public registration")
	}
	var registrationSpec policy.PhysicalManifest
	if err := json.Unmarshal(registration, &registrationSpec); err != nil {
		return err
	}
	reencoded, err := json.Marshal(registrationSpec)
	if err != nil || !bytes.Equal(reencoded, registration) {
		return errors.New("noncanonical or extended physical registration")
	}
	base, err := policy.Load(filepath.Join(root, bundleRelative))
	if err != nil {
		return err
	}
	installed, err := policy.PhysicalExtensionV1(base, registration)
	if err != nil {
		return err
	}
	reg, err := installed.PhysicalRegistration()
	if err != nil || reg.Disclosure != "ENGINE_ONLY" {
		return errors.New("invalid installed physical disclosure")
	}
	genesisHash, err := replayAndGenesis(ctx, filepath.Join(out, "causal.db"),
		installed, public.RunID)
	if err != nil {
		return err
	}
	if genesisHash != meta.GenesisSHA256 || len(meta.GenesisSHA256) != 64 {
		return errors.New("Genesis differs from archived run manifest")
	}
	if meta.EvidenceProfile == "H2_SIGNED_INGRESS_V1" ||
		meta.EvidenceProfile == "H2_RAW_SOURCE_V1" ||
		meta.EvidenceProfile == "H2_SAME_A1_REPLAY_V1" {
		if _, err := os.Lstat(filepath.Join(out, "ledger-export")); err != nil {
			return errors.New("H2-S requires non-optional private ledger export directory")
		}
	}
	if _, err := os.Lstat(filepath.Join(out, "ledger-export")); err == nil {
		if err := verifyH2Ledgers(ctx, out, meta.SourceSHA, public.RunID, installed); err != nil {
			return fmt.Errorf("H2 full canonical ledger verification: %w", err)
		}
		if meta.EvidenceProfile == "H2_SAME_A1_REPLAY_V1" {
			if err := verifyCausalReportH2(ctx, out, meta.SourceSHA, public.RunID, installed); err != nil {
				return fmt.Errorf("H2 private causal report verification: %w", err)
			}
			fmt.Println("H2_CAUSAL_REPORT=VERIFIED_PRIVATE")
		} else if _, err := os.Lstat(filepath.Join(out, causalReportPathH2)); !errors.Is(err, os.ErrNotExist) {
			return errors.New("H2 private causal report/profile mismatch")
		}
		if err := verifyOwnersH2(ctx, out, meta.SourceSHA, public.RunID); err != nil {
			return fmt.Errorf("H2 independent signed owner verification: %w", err)
		}
		present := false
		if _, err := os.Lstat(filepath.Join(out, signedIngressProofPathH2)); err == nil {
			present = true
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if present != (meta.EvidenceProfile == "H2_SIGNED_INGRESS_V1" ||
			meta.EvidenceProfile == "H2_RAW_SOURCE_V1" ||
			meta.EvidenceProfile == "H2_SAME_A1_REPLAY_V1") {
			return errors.New("H2-S signed ingress profile does not match evidence contents")
		}
		if present {
			if err := verifySignedIngressH2(ctx, out, meta.SourceSHA, public.RunID, installed); err != nil {
				return fmt.Errorf("H2-S source receipt and Genesis-bound signature audit: %w", err)
			}
			fmt.Println("H2_GENESIS_BOUND_SIGNED_INGRESS=VERIFIED_PRIVATE")
		}
		rawRootPresent := false
		if _, err := os.Lstat(filepath.Join(out, "physical-cuts")); err == nil {
			rawRootPresent = true
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if rawRootPresent != (meta.EvidenceProfile == "H2_RAW_SOURCE_V1" ||
			meta.EvidenceProfile == "H2_SAME_A1_REPLAY_V1") {
			return errors.New("H2-R original A1 source archive/profile mismatch")
		}
		if rawRootPresent {
			if err := verifyRawA1H2(ctx, out, meta.SourceSHA, public.RunID, installed); err != nil {
				return fmt.Errorf("H2-R original Python A1 cold source validation: %w", err)
			}
			fmt.Println("H2_RAW_PYTHON_A1_CUTS=SEALED_AND_VERIFIED_PRIVATE")
		}
		replayPresent := false
		if _, err := os.Lstat(filepath.Join(out, "replay")); err == nil {
			replayPresent = true
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if replayPresent != (meta.EvidenceProfile == "H2_SAME_A1_REPLAY_V1") {
			return errors.New("H2 same-cut replay profile mismatches the private second DB authority")
		}
		if replayPresent {
			if err := coldVerifyReplayH2(ctx, out, meta.SourceSHA, public.RunID, installed); err != nil {
				return fmt.Errorf("H2 same A1 second independent D3 verification: %w", err)
			}
			fmt.Println("H2_SAME_SIGNED_A1_INDEPENDENT_GO_D3=BYTE_IDENTICAL_AUTHORITIES")
		}
		fmt.Println("H2_FIVE_CANONICAL_LEDGERS=VERIFIED_PRIVATE")
		fmt.Println("H2_SIGNED_OWNER_CHECKPOINTS=VERIFIED_PRIVATE")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	fmt.Println("LOCAL_REFERENCE_ARCHIVE_VERIFIED=YES")
	fmt.Println("PRODUCTION_APPROVAL=NO")
	return nil
}

// The index is not an external signature or independent operator attestation.
// It detects accidental and uncoordinated byte modifications; an attacker
// able to rewrite the entire archive/checksums must be addressed separately.

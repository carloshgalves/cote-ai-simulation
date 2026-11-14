//go:build linux

package physicalhost

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 embodimentv1 "github.com/carloshgalves/cote-ai-simulation/internal/csf/adapter/embodimentv1"
 "regexp"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/genesis"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
	"golang.org/x/sys/unix"
)

// ProducerAdvance is a closed, bounded physical-engine instruction, never a
// shell argument, source path or global simulation coordinate.
type ProducerAdvance struct {
	Subject string `json:"subject"`
	Days    int    `json:"days"`
	Sleep   string `json:"sleep"`
	Quality string `json:"quality"`
}
type ProducerPlan struct {
	WorldSeed int64             `json:"world_seed"`
	Count     int               `json:"count"`
	Advances  []ProducerAdvance `json:"advances"`
}

type ApprovedCut = embodimentv1.ApprovedCut
type SourcePins = embodimentv1.SourcePins
type SignedCut = embodimentv1.SignedCut
var ErrInvalidPhysical = embodimentv1.ErrInvalidPhysical
var ErrUnadmitted = embodimentv1.ErrUnadmitted
var InspectSealedUnadmitted = embodimentv1.InspectSealedUnadmitted
var InspectSignedSealedAuthorized = embodimentv1.InspectSignedSealedAuthorized
const maxSnapshot = 16 * 1024 * 1024
const maxLog = 64 * 1024 * 1024
const maxEnvelope = 4_194_304
const requiredMemfdSeals = unix.F_SEAL_WRITE | unix.F_SEAL_SHRINK | unix.F_SEAL_GROW | unix.F_SEAL_SEAL
var hostSubjectID = regexp.MustCompile(`^[a-z][a-z0-9]*(?:[.-][a-z0-9]+)*$`)
func validSubjectID(subject string) bool {return hostSubjectID.MatchString(subject)}
func signedCutPayload(c SignedCut)([]byte,error){return embodimentv1.SignableCutBytes(c)}
func (p ProducerPlan) validate() error {
	if p.Count < 1 || p.Count > 4096 || len(p.Advances) == 0 || len(p.Advances) > p.Count {
		return ErrInvalidPhysical
	}
	seen := map[string]bool{}
	for _, a := range p.Advances {
		if len(a.Subject) > 128 || !validSubjectID(a.Subject) || seen[a.Subject] ||
			a.Days < 1 || a.Days > 30 || (a.Sleep != "4h" && a.Sleep != "6h" && a.Sleep != "8h") ||
			(a.Quality != "good" && a.Quality != "fair" && a.Quality != "poor") {
			return ErrInvalidPhysical
		}
		seen[a.Subject] = true
	}
	return nil
}

// ProducerHost is trusted deployment configuration, NOT request-supplied
// source paths. The signer key must be stored outside the producer's process
// environment and must match the registered Genesis public key.
type ProducerHost struct {
	Bundle             *policy.Bundle
	GenesisCanonical   []byte
	PrivateKey         ed25519.PrivateKey
	ExecutablePath     string
	ImportRoot         string
	PrivateStagingRoot string
	Epoch              uint64
	Plan               ProducerPlan
}

func securePinnedFile(path string, max int64) ([]byte, error) {
	if !filepath.IsAbs(path) || max <= 0 {
		return nil, ErrUnadmitted
	}
	fd, err := unix.Openat2(unix.AT_FDCWD, path, &unix.OpenHow{
		Flags:   uint64(unix.O_RDONLY | unix.O_CLOEXEC),
		Resolve: uint64(unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_MAGICLINKS),
	})
	if err != nil {
		return nil, fmt.Errorf("%w: untrusted producer path", ErrUnadmitted)
	}
	file := os.NewFile(uintptr(fd), "trusted-a1")
	defer file.Close()
	stat, err := file.Stat()
	if err != nil || !stat.Mode().IsRegular() || stat.Size() <= 0 || stat.Size() > max ||
		stat.Mode().Perm()&0o022 != 0 {
		return nil, ErrUnadmitted
	}
	data, err := io.ReadAll(io.LimitReader(file, max+1))
	if err != nil || len(data) == 0 || int64(len(data)) != stat.Size() {
		return nil, ErrUnadmitted
	}
	return data, nil
}

func secureCapturedFile(path string, max int64) ([]byte, error) {
	// PrivateStagingRoot and the fresh child directory are host-owned and
	// unshared; this open still refuses every symlink, nonregular file and
	// oversize result before admitting bytes into a sealed memfd.
	return securePinnedFile(path, max)
}
func makeSealedPhysicalFD(raw []byte) (*os.File, error) {
	if len(raw) == 0 {
		return nil, ErrInvalidPhysical
	}
	fd, err := unix.MemfdCreate("physical-a1-sealed", unix.MFD_CLOEXEC|unix.MFD_ALLOW_SEALING)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), "physical-a1-sealed")
	if n, err := file.Write(raw); err != nil || n != len(raw) {
		file.Close()
		return nil, ErrInvalidPhysical
	}
	if _, err := unix.FcntlInt(file.Fd(), unix.F_ADD_SEALS, requiredMemfdSeals); err != nil {
		file.Close()
		return nil, ErrInvalidPhysical
	}
	return file, nil
}
func checkedPrivateRoot(path string) error {
	if !filepath.IsAbs(path) {
		return ErrUnadmitted
	}
	fi, err := os.Lstat(path)
	if err != nil || !fi.IsDir() || fi.Mode()&os.ModeSymlink != 0 ||
		fi.Mode().Perm()&0o077 != 0 {
		return ErrUnadmitted
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok || st.Uid != uint32(os.Geteuid()) {
		return ErrUnadmitted
	}
	return nil
}

// RunSupervisedPhysical executes the only registered A1 Python worker, which
// creates a new run in a host-owned private directory. Both source files are
// captured ONLY after the process exits. The host terminates its process
// group (including descendants), then verifies the pair against the
// independent Go A2 validator before signing its sealed bytes. The producer
// never receives the signing key, CSF ledger, clock, or database handles.
//
// Limitations: the allowed Python runtime and its nonlocal dependencies must
// be deployed as an immutable, trusted application installation, not a
// same-UID attacker-writable import root. A compromised trusted producer or
// an unsandboxed child escaping its process group is outside this host's
// cryptographic trust guarantee and must be blocked at deployment level.
func RunSupervisedPhysical(ctx context.Context, host ProducerHost) (ApprovedCut, error) {
	var zero ApprovedCut
	if ctx == nil || host.Bundle == nil || host.Epoch == 0 ||
		len(host.PrivateKey) != ed25519.PrivateKeySize || host.Plan.validate() != nil {
		return zero, ErrUnadmitted
	}
	reg, err := host.Bundle.PhysicalRegistration()
	if err != nil || reg.ModuleSHA256 == ([32]byte{}) {
		return zero, ErrUnadmitted
	}
	g, err := genesis.ValidateCanonical(host.GenesisCanonical, host.Bundle)
	if err != nil {
		return zero, ErrUnadmitted
	}
	if !bytes.Equal(host.PrivateKey.Public().(ed25519.PublicKey), reg.PublicKeyEd25519[:]) {
		return zero, ErrUnadmitted
	}
	executable, err := securePinnedFile(host.ExecutablePath, 128*1024*1024)
	if err != nil || sha256.Sum256(executable) != reg.ExecutableSHA256 {
		return zero, ErrUnadmitted
	}
	if !filepath.IsAbs(host.ImportRoot) {
		return zero, ErrUnadmitted
	}
	modulePath := filepath.Join(host.ImportRoot, "embodiment", "a1_capture.py")
	module, err := securePinnedFile(modulePath, 1024*1024)
	if err != nil || sha256.Sum256(module) != reg.ModuleSHA256 {
		return zero, ErrUnadmitted
	}
	if err := checkedPrivateRoot(host.PrivateStagingRoot); err != nil {
		return zero, err
	}
	stage, err := os.MkdirTemp(host.PrivateStagingRoot, "a1-source-")
	if err != nil {
		return zero, err
	}
	defer os.RemoveAll(stage)
	if err := os.Chmod(stage, 0o700); err != nil {
		return zero, err
	}
	plan, err := json.Marshal(host.Plan)
	if err != nil {
		return zero, err
	}
	planPath := filepath.Join(stage, "plan.json")
	if err := os.WriteFile(planPath, plan, 0o600); err != nil {
		return zero, err
	}

	// Never reuse a previous run directory or accept a source path from the
	// producer's response. No shell, network socket or generic command input.
	processCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(processCtx, host.ExecutablePath,
		"-B", "-s", "-m", "embodiment.a1_capture", "--out", stage, "--plan", planPath)
	cmd.Dir = stage
	cmd.Env = []string{"PYTHONPATH=" + host.ImportRoot, "PYTHONNOUSERSITE=1",
		"PYTHONSAFEPATH=1", "PYTHONDONTWRITEBYTECODE=1", "LC_ALL=C.UTF-8"}
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGKILL}
	cmd.WaitDelay = 1 * time.Second
	if err := cmd.Start(); err != nil {
		return zero, ErrUnadmitted
	}
	pid := cmd.Process.Pid
	err = cmd.Wait()
	// End the producer process group before opening any source artifact.
	// Signer and Python process never run simultaneously over those bytes.
	killErr := syscall.Kill(-pid, syscall.SIGKILL)
	if killErr != nil && !errors.Is(killErr, syscall.ESRCH) {
		return zero, ErrUnadmitted
	}
	if err != nil || processCtx.Err() != nil {
		return zero, ErrUnadmitted
	}

	stagingCtx, stageCancel := context.WithTimeout(ctx, 10*time.Second)
	defer stageCancel()
	if err := stagingCtx.Err(); err != nil {
		return zero, ErrUnadmitted
	}
	snapshot, err := secureCapturedFile(filepath.Join(stage, "snapshot.yaml"), maxSnapshot)
	if err != nil {
		return zero, err
	}
	if err := stagingCtx.Err(); err != nil {
		return zero, ErrUnadmitted
	}
	events, err := secureCapturedFile(filepath.Join(stage, "events.jsonl"), maxLog)
	if err != nil {
		return zero, err
	}
	if err := stagingCtx.Err(); err != nil {
		return zero, ErrUnadmitted
	}
	evidence, err := secureCapturedFile(filepath.Join(stage, "evidence.json"), maxEnvelope)
	if err != nil {
		return zero, err
	}
	if err := stagingCtx.Err(); err != nil {
		return zero, ErrUnadmitted
	}
	snapshotFD, err := makeSealedPhysicalFD(snapshot)
	if err != nil {
		return zero, err
	}
	defer snapshotFD.Close()
	eventsFD, err := makeSealedPhysicalFD(events)
	if err != nil {
		return zero, err
	}
	defer eventsFD.Close()
	first, err := InspectSealedUnadmitted(snapshotFD, eventsFD, evidence, SourcePins{
		SnapshotSHA256: identity.OpaqueContentHash(snapshot),
		EventsSHA256:   identity.OpaqueContentHash(events),
	})
	if err != nil {
		return zero, err
	}
	typed, err := first.PhysicalCompletedState()
	if err != nil {
		return zero, err
	}
	hash, ok := typed[3].([]byte)
	if !ok || len(hash) != 32 {
		return zero, ErrUnadmitted
	}
	cut := SignedCut{
		RunID: domain.ID(g.RunID), GenesisDigest: g.Digest, SourceID: reg.SourceID,
		ExecutableSHA256: reg.ExecutableSHA256,
		SnapshotSHA256:   identity.OpaqueContentHash(snapshot),
		EventsSHA256:     identity.OpaqueContentHash(events),
		EvidenceSHA256:   identity.OpaqueContentHash(evidence), Epoch: host.Epoch,
	}
	copy(cut.CompletedStateSHA256[:], hash)
	message, err := signedCutPayload(cut)
	if err != nil {
		return zero, err
	}
	copy(cut.Signature[:], ed25519.Sign(host.PrivateKey, message))
	return InspectSignedSealedAuthorized(snapshotFD, eventsFD, evidence,
		host.GenesisCanonical, host.Bundle, cut)
}

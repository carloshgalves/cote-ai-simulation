//go:build linux

package commit

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/adapter/embodimentv1"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
	"golang.org/x/sys/unix"
)

// OfflineOriginalA1Epoch is copied solely from an already-authenticated,
// operator-private H2-R archive. This offline reference path MUST NOT be
// exposed as an ordinary admission interface to an agent, actor or client.
// It never invokes Python or signs new source material.
type OfflineOriginalA1Epoch struct {
	Snapshot  []byte
	Events    []byte
	Evidence  []byte
	Cut       []byte
	Signature []byte
}

func offlineA1SealedFD(raw []byte) (*os.File, error) {
	if len(raw) == 0 || len(raw) > 64<<20 {
		return nil, errors.New("H2 same-cut replay: invalid original source size")
	}
	fd, err := unix.MemfdCreate("csf-h2-offline-same-source", unix.MFD_CLOEXEC|unix.MFD_ALLOW_SEALING)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), "csf-h2-offline-same-source")
	if n, writeErr := file.Write(raw); writeErr != nil || n != len(raw) {
		file.Close()
		return nil, errors.New("H2 same-cut replay: incomplete sealed source")
	}
	if _, err := unix.FcntlInt(file.Fd(), unix.F_ADD_SEALS,
		unix.F_SEAL_WRITE|unix.F_SEAL_SHRINK|unix.F_SEAL_GROW|unix.F_SEAL_SEAL); err != nil {
		file.Close()
		return nil, err
	}
	return file, nil
}

// RunReferenceD3OfflineSameSignedA1 executes a SECOND independent C4+D2+D3
// causal database using the EXACT, previously archived and signed Python A1
// bytes. It does not call Python, alter the normal D2 producer path, or revive
// an ApprovedCut recovered from an existing InputLedger. Fresh unadmitted
// copies are independently verified by the original sealed A2 verifier before
// creating an unrelated new local experimental DB.
func RunReferenceD3OfflineSameSignedA1(
	ctx context.Context, dbPath, ownerPath, bundlePath string,
	cfg ReferenceD2Config, original [2]OfflineOriginalA1Epoch,
	options ReferenceOptions,
) (ReferenceD3Proof, error) {
	var zero ReferenceD3Proof
	if ctx == nil || dbPath == "" || ownerPath == "" || bundlePath == "" ||
		cfg.Host.Bundle == nil || cfg.Host.PrivateCaptureRoot != "" ||
		cfg.FailPhysicalAtEpoch != 0 || cfg.FailPhysicalAfterStep != 0 ||
		cfg.frozenSupervisedCuts != nil || len(cfg.Host.GenesisCanonical) != 0 {
		return zero, errors.New("H2 same-cut replay requires a fresh offline-only signed-source configuration")
	}
	// A standalone replay cannot mutate, append to or re-open an already
	// existing causal/owner authority. Reject supplied paths before C4 writes.
	for _, target := range []string{dbPath, ownerPath, ownerPath + ".actor-a"} {
		targetAbs, err := filepath.Abs(target)
		if err != nil {
			return zero, err
		}
		parent, err := os.Lstat(filepath.Dir(targetAbs))
		if err != nil || !parent.IsDir() || parent.Mode()&os.ModeSymlink != 0 ||
			parent.Mode().Perm()&0077 != 0 {
			return zero, errors.New("H2 same-cut replay needs a private real destination directory")
		}
		if _, err := os.Lstat(targetAbs); !errors.Is(err, os.ErrNotExist) {
			return zero, errors.New("H2 same-cut replay refuses an existing causal or owner database")
		}
	}
	absDb, _ := filepath.Abs(dbPath)
	absOwner, _ := filepath.Abs(ownerPath)
	if absDb == absOwner || absDb == absOwner+".actor-a" {
		return zero, errors.New("H2 same-cut replay cannot alias causal and owner authorities")
	}
	g, err := buildReferenceGenesis(cfg.Host.Bundle)
	if err != nil {
		return zero, fmt.Errorf("H2 offline Genesis creation: %w", err)
	}
	reg, err := cfg.Host.Bundle.PhysicalRegistration()
	if err != nil || reg.Disclosure != "ENGINE_ONLY" {
		return zero, errors.New("H2 same-cut replay refuses non-private physical producer")
	}
	var verified [2]embodimentv1.ApprovedCut
	for i, originalEpoch := range original {
		signed, err := embodimentv1.DecodeSignedCut(originalEpoch.Cut, originalEpoch.Signature)
		if err != nil || signed.Epoch != uint64(i+1) {
			return zero, fmt.Errorf("H2 same-cut replay: invalid signed epoch %d", i+1)
		}
		if err := embodimentv1.VerifySignedCut(signed, reg, g.RunID, g.Digest); err != nil {
			return zero, fmt.Errorf("H2 same-cut replay: epoch %d lacks Genesis-bound signature", i+1)
		}
		if signed.SnapshotSHA256 != identity.OpaqueContentHash(originalEpoch.Snapshot) ||
			signed.EventsSHA256 != identity.OpaqueContentHash(originalEpoch.Events) ||
			signed.EvidenceSHA256 != identity.OpaqueContentHash(originalEpoch.Evidence) {
			return zero, fmt.Errorf("H2 same-cut replay: raw epoch %d differs from signed source bytes", i+1)
		}
		snapshot, err := offlineA1SealedFD(originalEpoch.Snapshot)
		if err != nil {
			return zero, err
		}
		events, err := offlineA1SealedFD(originalEpoch.Events)
		if err != nil {
			snapshot.Close()
			return zero, err
		}
		approved, checkErr := embodimentv1.InspectSignedSealedAuthorized(
			snapshot, events, originalEpoch.Evidence, g.CanonicalBytes, cfg.Host.Bundle, signed)
		_ = events.Close()
		_ = snapshot.Close()
		if checkErr != nil || approved.IsDurablyRecovered() {
			return zero, fmt.Errorf("H2 same-cut replay: epoch %d did not pass fresh sealed A2 validation: %w",
				i+1, checkErr)
		}
		actualCut, actualSignature, _, err := approved.CanonicalProof()
		if err != nil || !bytes.Equal(actualCut, originalEpoch.Cut) ||
			!bytes.Equal(actualSignature, originalEpoch.Signature) {
			return zero, errors.New("H2 same-cut replay would alter the authenticated original source proof")
		}
		verified[i] = approved
	}
	cfg.frozenSupervisedCuts = &verified
	return RunReferenceD3(ctx, dbPath, ownerPath, bundlePath, cfg, options)
}

//go:build linux

// Package embodimentv1 requires kernel-sealed, immutable source handles for
// a physical cut. This adapter verifies only byte immutability and external
// content pins: it does NOT authorize the producer, attest pair coherence
// during writing, or grant a physical WorldRevision.
package embodimentv1

import (
	"io"
	"os"

	"golang.org/x/sys/unix"
)

// SourcePins are EXTERNALLY authenticated expected digests. This struct does
// not authenticate itself; until Genesis pins a producer/source closure and
// the supervisor owns the capture, the outcome stays a preflight only.
type SourcePins struct {
	SnapshotSHA256 [32]byte
	EventsSHA256   [32]byte
}

const requiredMemfdSeals = unix.F_SEAL_WRITE | unix.F_SEAL_SHRINK |
	unix.F_SEAL_GROW | unix.F_SEAL_SEAL

// immutableMemfdBytes rejects ordinary paths, symlinks, unsealed regular
// files, partial seals, oversized/empty files and concurrent truncation.
// The caller owns the descriptors and is responsible for their lifetime.
func immutableMemfdBytes(file *os.File, maxBytes int64) ([]byte, error) {
	if file == nil || maxBytes <= 0 {
		return nil, ErrInvalidPhysical
	}
	fd := file.Fd()
	seals, err := unix.FcntlInt(fd, unix.F_GET_SEALS, 0)
	if err != nil || seals&requiredMemfdSeals != requiredMemfdSeals {
		return nil, ErrInvalidPhysical
	}
	stat, err := file.Stat()
	if err != nil || !stat.Mode().IsRegular() || stat.Size() <= 0 ||
		stat.Size() > maxBytes {
		return nil, ErrInvalidPhysical
	}
	// ReadAt does not depend on a mutable shared file offset.
	data, err := io.ReadAll(io.NewSectionReader(file, 0, stat.Size()))
	if err != nil || int64(len(data)) != stat.Size() {
		return nil, ErrInvalidPhysical
	}
	after, err := file.Stat()
	if err != nil || after.Size() != stat.Size() {
		return nil, ErrInvalidPhysical
	}
	rechecked, err := unix.FcntlInt(fd, unix.F_GET_SEALS, 0)
	if err != nil || rechecked&requiredMemfdSeals != requiredMemfdSeals {
		return nil, ErrInvalidPhysical
	}
	return data, nil
}

// InspectSealedUnadmitted verifies the Linux F_SEAL_WRITE read-only
// source, externally pinned raw hashes, typed Go evidence and physical
// command-completion chain. There is intentionally no commit capability.
func InspectSealedUnadmitted(
	snapshot, events *os.File, evidence []byte, pins SourcePins,
) (Verified, error) {
	var zero Verified
	rawSnapshot, err := immutableMemfdBytes(snapshot, maxSnapshot)
	if err != nil {
		return zero, ErrInvalidPhysical
	}
	rawEvents, err := immutableMemfdBytes(events, maxLog)
	if err != nil {
		return zero, ErrInvalidPhysical
	}
	return InspectUnadmitted(Source{
		Snapshot: rawSnapshot,
		Events: rawEvents,
		Evidence: evidence,
		PinnedSnapshotSHA256: pins.SnapshotSHA256,
		PinnedEventsSHA256: pins.EventsSHA256,
	})
}

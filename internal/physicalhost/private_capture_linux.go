//go:build linux

package physicalhost

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// writePrivateA1Capture preserves the unmodified, supervised Python A1 bytes
// only AFTER sealing, A2 verification and Genesis-pinned Ed25519 authorization.
// This is an optional trusted host configuration, never an actor-supplied path.
// The exported data are ENGINE_ONLY and MUST NOT be uploaded to CI artifacts.
func writePrivateA1Capture(root string, epoch uint64, snapshot, events, evidence []byte, cut SignedCut) error {
	if root == "" || (epoch != 1 && epoch != 2) || cut.Epoch != epoch {
		return errors.New("invalid trusted private A1 archive target")
	}
	if err := checkedPrivateRoot(root); err != nil {
		return fmt.Errorf("private A1 archive root rejected: %w", err)
	}
	message, err := signedCutPayload(cut)
	if err != nil {
		return err
	}
	dir := filepath.Join(root, fmt.Sprintf("epoch-%d", epoch))
	if err := os.Mkdir(dir, 0700); err != nil {
		return fmt.Errorf("cannot create exclusive A1 capture: %w", err)
	}
	complete := false
	defer func() {
		if !complete {
			_ = os.RemoveAll(dir)
		}
	}()
	for _, entry := range []struct {
		name string
		raw  []byte
	}{
		{"snapshot.yaml", snapshot},
		{"events.jsonl", events},
		{"evidence.json", evidence},
		{"cut.cbor", message},
		{"signature.bin", cut.Signature[:]},
	} {
		if len(entry.raw) == 0 {
			return errors.New("private A1 archive refuses empty evidence")
		}
		path := filepath.Join(dir, entry.name)
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return err
		}
		count, writeErr := file.Write(entry.raw)
		syncErr := file.Sync()
		closeErr := file.Close()
		if writeErr != nil || syncErr != nil || closeErr != nil || count != len(entry.raw) {
			return errors.New("cannot durably commit the complete raw private A1 capture")
		}
	}
	dirFD, err := os.Open(dir)
	if err != nil {
		return err
	}
	err = dirFD.Sync()
	closeErr := dirFD.Close()
	if err != nil || closeErr != nil {
		return errors.New("A1 capture epoch directory sync failed")
	}
	rootFD, err := os.Open(root)
	if err != nil {
		return err
	}
	err = rootFD.Sync()
	closeErr = rootFD.Close()
	if err != nil || closeErr != nil {
		return errors.New("A1 capture root directory sync failed")
	}
	complete = true
	return nil
}

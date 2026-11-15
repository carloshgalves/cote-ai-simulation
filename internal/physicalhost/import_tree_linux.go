//go:build linux

package physicalhost

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"hash"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const maxPythonModuleBytes = 2 * 1024 * 1024
const maxPhysicalModules = 256

// PhysicalImportTreeSHA256 hashes the *complete* registered producer package,
// not only a1_capture.py. Names are relative and sorted, lengths are explicit,
// contents are read through openat2 without traversing symlinks. The output is
// pinned by the physical policy manifest and hence by Genesis.
//
// This detects unregistered changes to Python source. It does not make an
// attacker-writable installation immutable; production additionally needs a
// separately enforced read-only installation and isolated producer identity.
func PhysicalImportTreeSHA256(importRoot string) ([32]byte, error) {
	var zero [32]byte
	if !filepath.IsAbs(importRoot) {
		return zero, errors.New("physical Python import root must be absolute")
	}
	root := filepath.Join(importRoot, "embodiment")
	fi, err := os.Lstat(root)
	if err != nil || !fi.IsDir() || fi.Mode()&os.ModeSymlink != 0 {
		return zero, errors.New("physical producer package root not a real directory")
	}
	var files []string
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.Type()&os.ModeSymlink != 0 {
			return errors.New("symlink forbidden in physical producer source")
		}
		if d.IsDir() {
			if d.Name() == "__pycache__" {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() || !strings.HasSuffix(d.Name(), ".py") {
			return errors.New("unregistered non-Python source in physical package")
		}
		if len(files) >= maxPhysicalModules {
			return errors.New("physical Python module count exceeded")
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		return zero, err
	}
	if len(files) == 0 {
		return zero, errors.New("physical producer package is empty")
	}
	sort.Strings(files)
	h := sha256.New()
	h.Write([]byte("cote.a1.python-import-tree.v1\x00"))
	var foundInit, foundA1 bool
	for _, path := range files {
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return zero, err
		}
		relative = filepath.ToSlash(relative)
		if relative == "__init__.py" {
			foundInit = true
		}
		if relative == "a1_capture.py" {
			foundA1 = true
		}
		raw, err := securePinnedFile(path, maxPythonModuleBytes)
		if err != nil {
			return zero, err
		}
		var length [4]byte
		binary.BigEndian.PutUint32(length[:], uint32(len(relative)))
		h.Write(length[:])
		h.Write([]byte(relative))
		digest := sha256.Sum256(raw)
		h.Write(digest[:])
	}
	if !foundInit || !foundA1 {
		return zero, errors.New("physical producer package missing closed A1 entrypoint")
	}
	return sumPhysicalImportTree(h), nil
}

func sumPhysicalImportTree(h hash.Hash) [32]byte {
	var digest [32]byte
	copy(digest[:], h.Sum(nil))
	return digest
}

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

// PhysicalImportTreeSHA256 pins the complete Python producer package AND its
// canonical physical parameter/model files. Names are relative and sorted,
// lengths explicit, contents read through openat2 without following symlinks.
// This digest is pinned by policy/Genesis, not by a caller-supplied path.
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
				// -B prevents writing pyc, NOT loading an existing cache.
				// Unpinned bytecode could override authenticated .py modules.
				return errors.New("cached Python bytecode forbidden in A1 source")
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
	// The Python dynamics/prior loaders derive this exact directory from
	// src/embodiment/modelfile.py. Source-only attestation would otherwise
	// sign attacker-changed physical parameters with an untampered program.
	modelRoot := filepath.Clean(filepath.Join(importRoot, "..", "data", "models", "physical"))
	modelDir, err := os.Lstat(modelRoot)
	if err != nil || !modelDir.IsDir() || modelDir.Mode()&os.ModeSymlink != 0 {
		return zero, errors.New("physical model root is not an authenticated directory")
	}
	models := make([]string, 0)
	err = filepath.WalkDir(modelRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.Type()&os.ModeSymlink != 0 {
			return errors.New("symlink forbidden in physical parameter tree")
		}
		if d.IsDir() {
			return nil
		}
		if !d.Type().IsRegular() {
			return errors.New("physical model tree contains a nonregular file")
		}
		if strings.HasSuffix(d.Name(), ".md") {
			// Human-only notes are not consumed by the physical producer.
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".yaml") && !strings.HasSuffix(d.Name(), ".json") {
			return errors.New("unexpected artifact in physical parameter tree")
		}
		if len(models) >= maxPhysicalModules {
			return errors.New("physical model file count exceeded")
		}
		models = append(models, path)
		return nil
	})
	if err != nil {
		return zero, err
	}
	sort.Strings(models)
	required := map[string]bool{
		"population-prior.yaml":         false,
		"body-dynamics.yaml":            false,
		"schema/model-file.schema.json": false,
	}
	h.Write([]byte("cote.a1.physical-parameter-tree.v1\x00"))
	for _, path := range models {
		relative, err := filepath.Rel(modelRoot, path)
		if err != nil {
			return zero, err
		}
		relative = filepath.ToSlash(relative)
		if _, mustExist := required[relative]; mustExist {
			required[relative] = true
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
	for name, found := range required {
		if !found {
			return zero, errors.New("required physical parameter absent: " + name)
		}
	}
	return sumPhysicalImportTree(h), nil
}

func sumPhysicalImportTree(h hash.Hash) [32]byte {
	var digest [32]byte
	copy(digest[:], h.Sum(nil))
	return digest
}

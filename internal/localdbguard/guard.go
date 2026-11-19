// Package localdbguard validates local SQLite file families before opening
// them. It is not a sandbox against a malicious process with the same UID
// or root. The operator owns the directory; untrusted users must not have
// write access to its ancestors or files.
package localdbguard

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

var suffixes = []string{"", "-wal", "-shm", "-journal"}

// Family returns an absolute SQLite filename after checking every ancestor
// and all pager companions. Never resolve symlinks as approved aliases.
// Missing pager companions are normal; a missing main file is allowed only
// for an explicitly opted-in creation path.
func Family(path string, allowMissing bool) (string, error) {
	if path == "" {
		return "", errors.New("empty database path")
	}
	a, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	for dir := filepath.Dir(a); ; dir = filepath.Dir(dir) {
		info, err := os.Lstat(dir)
		if err != nil {
			return "", err
		}
		if !info.IsDir() || !trustedDir(info) {
			return "", errors.New("database ancestor is not a trusted directory")
		}
		if dir == filepath.Dir(dir) {
			break
		}
	}
	for _, suffix := range suffixes {
		info, err := os.Lstat(a + suffix)
		if errors.Is(err, os.ErrNotExist) && (suffix != "" || allowMissing) {
			continue
		}
		if err != nil {
			return "", err
		}
		if !info.Mode().IsRegular() || !trustedFile(info) {
			return "", fmt.Errorf("unsafe database/pager file %q", suffix)
		}
	}
	return a, nil
}

func trustedDir(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || (stat.Uid != uint32(os.Geteuid()) && stat.Uid != 0) {
		return false
	}
	// A root-owned sticky directory such as /tmp permits creation but not
	// replacement of another owner's trusted child. Never use it directly
	// as the SQLite parent; Pin/CheckDistinct enforce a private parent.
	return info.Mode().Perm()&0022 == 0 || (stat.Uid == 0 && info.Mode()&os.ModeSticky != 0)
}

func trustedFile(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	// POSIX ACL effective group/named-user write rights are reflected in
	// the mode mask. Refuse all group/other write access and multiple links.
	return ok && stat.Uid == uint32(os.Geteuid()) && stat.Nlink == 1 && info.Mode().Perm()&0022 == 0
}

func privateParent(path string) (os.FileInfo, error) {
	info, err := os.Lstat(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != uint32(os.Geteuid()) || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("SQLite parent must be operator-owned and private (0700)")
	}
	return info, nil
}

// Pin retains main-file and parent identities, independent of the normal
// SQLite lifecycle of WAL/SHM. Recheck on entry and exit of each query.
type Pin struct {
	path   string
	main   os.FileInfo
	parent os.FileInfo
}

func OpenPin(path string) (*Pin, error) {
	a, err := Family(path, false)
	if err != nil {
		return nil, err
	}
	parent, err := privateParent(a)
	if err != nil {
		return nil, err
	}
	main, err := os.Lstat(a)
	if err != nil {
		return nil, err
	}
	return &Pin{path: a, main: main, parent: parent}, nil
}
func (p *Pin) Path() string { return p.path }

func (p *Pin) Check() error {
	if p == nil {
		return errors.New("missing database path pin")
	}
	if _, err := Family(p.path, false); err != nil {
		return err
	}
	parent, err := privateParent(p.path)
	if err != nil {
		return err
	}
	main, err := os.Lstat(p.path)
	if err != nil {
		return err
	}
	if !os.SameFile(p.main, main) || !os.SameFile(p.parent, parent) {
		return errors.New("database or parent identity changed")
	}
	return nil
}

// Distinct refuses overlap of the full reserved pager namespaces, even
// when files do not yet exist, not just equality of the two main paths.
func Distinct(causal, metrics string) (string, error) {
	a, err := Family(causal, true)
	if err != nil {
		return "", err
	}
	b, err := Family(metrics, true)
	if err != nil {
		return "", err
	}
	if _, err := privateParent(a); err != nil {
		return "", err
	}
	if _, err := privateParent(b); err != nil {
		return "", err
	}
	for _, left := range suffixes {
		for _, right := range suffixes {
			if a+left == b+right {
				return "", errors.New("operational/causal SQLite pager namespaces overlap")
			}
		}
	}
	for _, suffix := range suffixes[1:] {
		if strings.HasSuffix(b, suffix) {
			return "", errors.New("operational filename reserved for pager companions")
		}
	}
	return b, nil
}

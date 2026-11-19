package localdbguard

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// Construct real Linux POSIX ACL xattr bytes, without relying on setfacl
// being installed. A named user receives rwx through the ACL mask.
func f4ACL() []byte {
	out := make([]byte, 4+5*8)
	binary.LittleEndian.PutUint32(out, 2)
	for i, entry := range []struct {
		tag, perm uint16
		id        uint32
	}{
		{1, 7, ^uint32(0)},
		{2, 7, 65534},
		{4, 0, ^uint32(0)},
		{16, 7, ^uint32(0)},
		{32, 0, ^uint32(0)},
	} {
		at := 4 + i*8
		binary.LittleEndian.PutUint16(out[at:], entry.tag)
		binary.LittleEndian.PutUint16(out[at+2:], entry.perm)
		binary.LittleEndian.PutUint32(out[at+4:], entry.id)
	}
	return out
}

func TestF4ActualACLAndPinReplacement(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "private")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "run.db")
	if err := os.WriteFile(path, []byte("authority"), 0600); err != nil {
		t.Fatal(err)
	}
	pin, err := OpenPin(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, attribute := range []struct {
		path, name string
	}{
		{path, "system.posix_acl_access"},
		{dir, "system.posix_acl_default"},
	} {
		if err := syscall.Setxattr(attribute.path, attribute.name, f4ACL(), 0); err != nil {
			t.Fatal("CI must exercise a real POSIX ACL", err)
		}
		if _, err := OpenPin(path); err == nil {
			t.Fatal("unsafe ACL accepted")
		}
		if err := pin.Check(); err == nil {
			t.Fatal("existing pin ignored ACL mutation")
		}
		if err := syscall.Removexattr(attribute.path, attribute.name); err != nil {
			t.Fatal(err)
		}
		if attribute.path == path {
			if err := os.Chmod(path, 0600); err != nil {
				t.Fatal(err)
			}
		}
		if err := pin.Check(); err != nil {
			t.Fatal("restored ACL denied", err)
		}
	}
	if err := os.Rename(path, path+".saved"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("authority"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := pin.Check(); err == nil {
		t.Fatal("byte-identical replacement accepted as the pinned inode")
	}
}

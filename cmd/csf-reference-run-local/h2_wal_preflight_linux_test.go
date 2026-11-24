//go:build linux

package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	_ "modernc.org/sqlite"
)

// A genuinely live committed SQLite WAL must be captured while its writer
// stays connected: after clean Close, SQLite can checkpoint and unlink both
// sidecars. Fixtures with arbitrary WAL-looking bytes do not prove recovery.
func liveWALFixtureH2(t *testing.T) string {
	t.Helper()
	previous := syscall.Umask(0077)
	t.Cleanup(func() { syscall.Umask(previous) })
	path := filepath.Join(t.TempDir(), "live.db")
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?_pragma=journal_mode%28WAL%29&_pragma=synchronous%28FULL%29")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(1)
	if _, err := db.Exec("PRAGMA wal_autocheckpoint=0"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("CREATE TABLE wal_authority (id INTEGER PRIMARY KEY, content BLOB NOT NULL)"); err != nil {
		t.Fatal(err)
	}
	// Ensure the table's schema and first row are checkpointed, so a new
	// uncheckpointed transaction below is genuine WAL-dependent state.
	if _, err := db.Exec("INSERT INTO wal_authority VALUES (1,x'112233')"); err != nil {
		t.Fatal(err)
	}
	var busy, logFrames, checkpointed int
	if err := db.QueryRow("PRAGMA wal_checkpoint(TRUNCATE)").Scan(&busy, &logFrames, &checkpointed); err != nil || busy != 0 {
		t.Fatalf("could not freeze a real pre-WAL baseline: %v busy=%d", err, busy)
	}
	if _, err := db.Exec("INSERT INTO wal_authority VALUES (2,x'aabbccdd')"); err != nil {
		t.Fatal(err)
	}
	// Force an actual SHM index construction under the same writer.
	var count int
	if err := db.QueryRow("SELECT count(*) FROM wal_authority").Scan(&count); err != nil || count != 2 {
		t.Fatalf("committed live WAL source not visible: %v count=%d", err, count)
	}
	for _, suffix := range []string{"", "-wal", "-shm"} {
		target := path + suffix
		info, err := os.Stat(target)
		if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
			t.Fatalf("genuine active SQLite sidecar unavailable: %s: %v", suffix, err)
		}
		if err := os.Chmod(target, 0600); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

func copyLiveWALH2(t *testing.T, source string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "copy.db")
	for _, suffix := range []string{"", "-wal", "-shm"} {
		raw, err := readPrivateRegular(source + suffix)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path+suffix, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

func TestH2LiveSQLiteWALProvesCommittedFramesBeforeDetachedOpen(t *testing.T) {
	source := liveWALFixtureH2(t)
	proof, err := verifySQLiteSidecarsH2(source)
	if err != nil || !proof.HasWAL || !proof.HasSHM || proof.Frames < 1 || proof.Commits < 1 {
		t.Fatalf("genuine pinned SQLite WAL refused: %+v %v", proof, err)
	}
	// A fresh standalone copy must recover the uncheckpointed committed row,
	// with zero modifications to the original archived bytes.
	originalWAL, err := readPrivateRegular(source + "-wal")
	if err != nil {
		t.Fatal(err)
	}
	copyPath, cleanup, err := detachedDBH2(source)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	db, err := readOnlyDBH2(context.Background(), copyPath)
	if err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM wal_authority").Scan(&count); err != nil || count != 2 {
		t.Fatalf("detached WAL lost genuinely committed uncheckpointed SQLite state: %d %v", count, err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	after, err := readPrivateRegular(source + "-wal")
	if err != nil || !bytes.Equal(originalWAL, after) {
		t.Fatalf("detached WAL verification mutated the real original: %v", err)
	}
	// SHM is a regenerated cache, not a normative source: a valid WAL
	// without SHM remains recoverable, rather than falsely requiring SHM.
	copied := copyLiveWALH2(t, source)
	if err := os.Remove(copied + "-shm"); err != nil {
		t.Fatal(err)
	}
	withoutSHM, err := verifySQLiteSidecarsH2(copied)
	if err != nil || !withoutSHM.HasWAL || withoutSHM.HasSHM {
		t.Fatalf("valid WAL without expendable SHM was refused: %+v %v", withoutSHM, err)
	}
}

func TestH2ZeroByteWALAfterRealCheckpointKeepsCommittedRows(t *testing.T) {
	source := liveWALFixtureH2(t)
	conn, err := sql.Open("sqlite", "file:"+filepath.ToSlash(source))
	if err != nil {
		t.Fatal(err)
	}
	var busy, logFrames, checkpointed int
	if err := conn.QueryRow("PRAGMA wal_checkpoint(TRUNCATE)").
		Scan(&busy, &logFrames, &checkpointed); err != nil || busy != 0 {
		conn.Close()
		t.Fatalf("real checkpoint could not drain WAL: %v busy=%d", err, busy)
	}
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
	walInfo, err := os.Stat(source + "-wal")
	if err != nil || walInfo.Size() != 0 {
		t.Fatalf("real SQLite did not produce the checkpointed empty WAL witness: %v", err)
	}
	p, err := verifySQLiteSidecarsH2(source)
	if err != nil || !p.HasWAL || p.Frames != 0 || p.Commits != 0 {
		t.Fatalf("legitimate checkpointed SQLite WAL refused: %+v %v", p, err)
	}
	copied, cleanup, err := detachedDBH2(source)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	ro, err := readOnlyDBH2(context.Background(), copied)
	if err != nil {
		t.Fatal(err)
	}
	defer ro.Close()
	var count int
	if err := ro.QueryRow("SELECT count(*) FROM wal_authority").Scan(&count); err != nil || count != 2 {
		t.Fatalf("zero-length post-checkpoint WAL lost committed main-db rows: %d %v", count, err)
	}
}

func TestH2WALRejectsImpossiblePagesEvenWithRecomputedRollingChecksums(t *testing.T) {
	source := liveWALFixtureH2(t)
	for _, variant := range []string{"page-number-overflow", "commit-size-overflow"} {
		t.Run(variant, func(t *testing.T) {
			path := copyLiveWALH2(t, source)
			wal, err := os.ReadFile(path + "-wal")
			if err != nil {
				t.Fatal(err)
			}
			// Deliberately forge a new fully self-consistent SQLite rolling
			// checksum sequence (and, in an H2 archive, rehash the local
			// SHA256 index). Only strict frame semantics may reject it.
			offset := 32
			if variant == "page-number-overflow" {
				binary.BigEndian.PutUint32(wal[offset:offset+4], ^uint32(0))
			} else {
				binary.BigEndian.PutUint32(wal[offset+4:offset+8], ^uint32(0))
			}
			var order binary.ByteOrder = binary.LittleEndian
			if binary.BigEndian.Uint32(wal[:4]) == 0x377f0683 {
				order = binary.BigEndian
			}
			s0, s1 := walChecksumH2(wal[:24], order, 0, 0)
			pageSize := int(binary.BigEndian.Uint32(wal[8:12]))
			for off := 32; off < len(wal); off += 24 + pageSize {
				s0, s1 = walChecksumH2(wal[off:off+8], order, s0, s1)
				s0, s1 = walChecksumH2(wal[off+24:off+24+pageSize], order, s0, s1)
				binary.BigEndian.PutUint32(wal[off+16:off+20], s0)
				binary.BigEndian.PutUint32(wal[off+20:off+24], s1)
			}
			if err := os.WriteFile(path+"-wal", wal, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := verifySQLiteSidecarsH2(path); err == nil ||
				!strings.Contains(err.Error(), "invalid page number") {
				t.Fatalf("correctly rechecksummed impossible WAL frame accepted: %v", err)
			}
		})
	}
}

func TestH2LiveSQLiteWALTruncationsAndTamperedFramesFailBeforeRecovery(t *testing.T) {
	source := liveWALFixtureH2(t)
	type mutant struct {
		name   string
		suffix string
		edit   func([]byte) []byte
	}
	cases := []mutant{
		{"wal-truncated-header", "-wal", func(b []byte) []byte { return append([]byte(nil), b[:29]...) }},
		{"wal-truncated-to-one-byte", "-wal", func(b []byte) []byte { return append([]byte(nil), b[:1]...) }},
		{"wal-truncated-final-frame", "-wal", func(b []byte) []byte { return append([]byte(nil), b[:len(b)-1]...) }},
		{"wal-invalid-magic", "-wal", func(b []byte) []byte { b[0] ^= 0x55; return b }},
		{"wal-invalid-header-checksum", "-wal", func(b []byte) []byte { b[24] ^= 0x55; return b }},
		{"wal-wrong-page-size", "-wal", func(b []byte) []byte { b[11] ^= 0x55; return b }},
		{"wal-foreign-frame-salt", "-wal", func(b []byte) []byte { b[32+8] ^= 0x55; return b }},
		{"wal-corrupted-commit-page", "-wal", func(b []byte) []byte { b[32+24+8] ^= 0x55; return b }},
		{"wal-corrupted-frame-checksum", "-wal", func(b []byte) []byte { b[32+16] ^= 0x55; return b }},
		{"shm-truncated-mapping", "-shm", func(b []byte) []byte { return append([]byte(nil), b[:len(b)-1]...) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := copyLiveWALH2(t, source)
			original, err := os.ReadFile(path + tc.suffix)
			if err != nil {
				t.Fatal(err)
			}
			mutated := tc.edit(bytes.Clone(original))
			if err := os.WriteFile(path+tc.suffix, mutated, 0600); err != nil {
				t.Fatal(err)
			}
			// This variant can be re-indexed in checksums.json to match
			// every altered byte: rejection must use WAL semantics, not
			// archive SHA256 comparisons alone.
			if _, err := verifySQLiteSidecarsH2(path); err == nil ||
				!strings.Contains(err.Error(), "H2 WAL preflight") {
				t.Fatalf("malformed live sidecar passed independent preflight: %v", err)
			}
			if _, cleanup, err := detachedDBH2(path); err == nil {
				cleanup()
				t.Fatal("invalid WAL/SHM entered SQLite detached recovery")
			}
			after, err := os.ReadFile(path + tc.suffix)
			if err != nil || !bytes.Equal(after, mutated) {
				t.Fatalf("rejected archived sidecar was silently changed: %v", err)
			}
		})
	}
	// A lost WAL accompanied by an orphan SHM must not be silently
	// reconstructed as if committed frames never existed.
	t.Run("wal-removed-shm-survives", func(t *testing.T) {
		path := copyLiveWALH2(t, source)
		if err := os.Remove(path + "-wal"); err != nil {
			t.Fatal(err)
		}
		if _, err := verifySQLiteSidecarsH2(path); err == nil ||
			!strings.Contains(err.Error(), "orphan SHM") {
			t.Fatalf("removed WAL incorrectly accepted: %v", err)
		}
	})
}

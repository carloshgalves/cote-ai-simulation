//go:build linux

package main

import (
	"bytes"
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	_ "modernc.org/sqlite"
)

func TestH2SameSignedSourceMultisetComparesEveryTypedRow(t *testing.T) {
	ctx := context.Background()
	dbs := [2]string{filepath.Join(t.TempDir(), "left.db"), filepath.Join(t.TempDir(), "right.db")}
	for i, path := range dbs {
		db, err := sql.Open("sqlite", path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`CREATE TABLE authoritative (id INTEGER, b BLOB, content TEXT, measure REAL, blank INTEGER)`); err != nil {
			t.Fatal(err)
		}
		for j := 0; j < 3; j++ {
			row := j
			if i == 1 {
				row = 2 - j
			}
			if _, err := db.Exec("INSERT INTO authoritative VALUES(?,?,?,?,?)", row, []byte{byte(row), 0xff}, "private", float64(row)+0.125, nil); err != nil {
				t.Fatal(err)
			}
		}
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
		// The private replay verifier intentionally rejects world-readable
		// SQLite files. The synthetic test must honor the same 0600 invariant.
		if err := os.Chmod(path, 0600); err != nil {
			t.Fatal(err)
		}
	}
	left, err := replayDbTableSetH2(ctx, dbs[0])
	if err != nil {
		t.Fatal(err)
	}
	right, err := replayDbTableSetH2(ctx, dbs[1])
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(left, right) || left["authoritative"].Rows != 3 {
		t.Fatal("logically identical table rows must be equal independent of insertion order")
	}
	db, err := sql.Open("sqlite", dbs[1])
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("UPDATE authoritative SET b=x'0001' WHERE id=1"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	changed, err := replayDbTableSetH2(ctx, dbs[1])
	if err != nil {
		t.Fatal(err)
	}
	if reflect.DeepEqual(left, changed) {
		t.Fatal("byte-level BLOB semantic change not detected")
	}
	db, err = sql.Open("sqlite", dbs[1])
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("CREATE TABLE quiet_auxiliary (id INTEGER)"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	extra, err := replayDbTableSetH2(ctx, dbs[1])
	if err != nil {
		t.Fatal(err)
	}
	if reflect.DeepEqual(left, extra) {
		t.Fatal("injected auxiliary schema not detected")
	}
}

func TestH2ReplayRefusesOversizedMultiColumnPrivateRows(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "large.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("CREATE TABLE authoritative (part_a BLOB, part_b BLOB)"); err != nil {
		t.Fatal(err)
	}
	chunk := bytes.Repeat([]byte{0x42}, 5<<20)
	if _, err := db.Exec("INSERT INTO authoritative VALUES (?,?)", chunk, chunk); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := replayDbTableSetH2(ctx, path); err == nil {
		t.Fatal("large multi-column private row bypassed global H2 memory bound")
	}
}

func TestH2ReplaySourceInventoryRequiresBothIndependentAuthorities(t *testing.T) {
	original := t.TempDir()
	for _, dirname := range []string{"replay", "replay/owner"} {
		if err := os.Mkdir(filepath.Join(original, filepath.FromSlash(dirname)), 0700); err != nil {
			t.Fatal(err)
		}
	}
	paths, err := collectedPaths(original)
	if err != nil {
		t.Fatal(err)
	}
	for _, needed := range replayRequiredPathsH2 {
		present := false
		for _, got := range paths {
			if got == needed {
				present = true
				break
			}
		}
		if !present {
			t.Fatalf("mandatory second-run authority omitted from archived roster: %s", needed)
		}
	}
	if err := os.Chmod(filepath.Join(original, "replay"), 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := collectedPaths(original); err == nil {
		t.Fatal("exposed replay authority accepted")
	}
}

//go:build linux

package main

import (
	"bytes"
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
	_ "modernc.org/sqlite"
)

func TestH2CanonicalFiveLedgerCaptureAndRejections(t *testing.T) {
	ctx := context.Background()
	p, err := policy.Load("../../docs/architecture/canonical-codec-v1-bundle")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "ledger.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("CREATE TABLE input_ledger(id BLOB, canonical BLOB)"); err != nil {
		t.Fatal(err)
	}
	canonical, err := codec.CanonicalPayload([]any{uint64(1), []byte("private-only")})
	if err != nil {
		t.Fatal(err)
	}
	id := bytes.Repeat([]byte{0x11}, 32)
	if _, err := db.Exec("INSERT INTO input_ledger(id,canonical) VALUES(?,?)", id, canonical); err != nil {
		t.Fatal(err)
	}
	first, proof, err := captureLedgerH2(ctx, db, "input_ledger", p)
	if err != nil {
		t.Fatal(err)
	}
	second, also, err := captureLedgerH2(ctx, db, "input_ledger", p)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) || proof != also || proof.Count != 1 ||
		!bytes.HasPrefix(first, []byte(ledgerMagicH2)) {
		t.Fatal("canonical private export is not stable or counted correctly")
	}
	if _, _, err := captureLedgerH2(ctx, db, "sqlite_master", p); err == nil {
		t.Fatal("unregistered SQL ledger admitted")
	}
	if _, err := db.Exec("INSERT INTO input_ledger(id,canonical) VALUES(?,?)", id, canonical); err != nil {
		t.Fatal(err)
	}
	if _, _, err := captureLedgerH2(ctx, db, "input_ledger", p); err == nil ||
		!strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate canonical ledger identity accepted: %v", err)
	}
	if _, err := db.Exec("DELETE FROM input_ledger"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO input_ledger(id,canonical) VALUES(?,?)", id, []byte{0xff}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := captureLedgerH2(ctx, db, "input_ledger", p); err == nil {
		t.Fatal("unparseable canonical CBOR admitted")
	}
}

func TestH2ArchiveRosterRequiresAllPrivateStreams(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	ledgers := filepath.Join(dir, "ledger-export")
	if err := os.Mkdir(ledgers, 0700); err != nil {
		t.Fatal(err)
	}
	paths, err := collectedPaths(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range h2PrivatePaths {
		found := false
		for _, path := range paths {
			if path == name {
				found = true
			}
		}
		if !found {
			t.Fatalf("missing H2 file required by archive index: %s", name)
		}
	}
	if err := os.Symlink(filepath.Join(dir, "owner"), filepath.Join(ledgers, "evil")); err != nil {
		t.Fatal(err)
	}
	if err := rejectExtraneous(dir); err == nil {
		t.Fatal("unknown symlink accepted inside private export")
	}
	if err := os.RemoveAll(ledgers); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), ledgers); err != nil {
		t.Fatal(err)
	}
	if _, err := collectedPaths(dir); err == nil {
		t.Fatal("symlink export root accepted")
	}
}

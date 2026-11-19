package sqlite

import (
	"context"
	"database/sql"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/observatory/queryhost"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

func TestCSFRV19O1SQLiteReadOnlyModeAndIndependentQueryOnlyDefense(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "authority.db")
	writer, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	b, err := policy.Load("../../../../docs/architecture/canonical-codec-v1-bundle")
	if err != nil {
		t.Fatal(err)
	}
	r, err := openObservatoryAuthority(ctx, path, b)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	var flag int
	if err := r.store.db.QueryRowContext(ctx, "PRAGMA query_only").Scan(&flag); err != nil || flag != 1 {
		t.Fatalf("query_only not enabled on the actual connection: %d %v", flag, err)
	}
	if _, err := r.store.db.ExecContext(ctx, "INSERT INTO input_ledger(id,canonical) VALUES(zeroblob(32),?)", []byte{0x01}); err == nil {
		t.Fatal("read-only connection was allowed to INSERT")
	}
	if _, err := r.store.db.ExecContext(ctx, "CREATE TABLE o1_write_bypass (x INTEGER)"); err == nil {
		t.Fatal("read-only connection was allowed to change schema")
	}
	// Even if a future internal caller disables the advisory connection
	// query_only pragma, SQLite mode=ro must still reject all file writes.
	for _, query := range []string{"PRAGMA query_only=OFF", "ATTACH DATABASE ':memory:' AS bypass",
		"SELECT 1; CREATE TABLE bypass(x)", "PRAGMA journal_mode=DELETE", "DELETE FROM input_ledger"} {
		if _, err := r.store.db.ExecContext(ctx, query); err == nil {
			t.Fatal("driver-level write denial bypassed")
		}
		rows, err := r.store.db.QueryContext(ctx, query)
		if err == nil {
			rows.Close()
			t.Fatal("query path permitted mutable SQL")
		}
	}
	// Prove independent pager denial using a deliberately unwrapped
	// mode=ro connection with query_only disabled, not a weakened reader.
	uri := url.URL{Scheme: "file", Path: filepath.ToSlash(path), RawQuery: "mode=ro"}
	raw, err := sql.Open("sqlite", uri.String())
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	if _, err := raw.ExecContext(ctx, "PRAGMA query_only=OFF"); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.ExecContext(ctx, "INSERT INTO input_ledger(id,canonical) VALUES(zeroblob(32),?)", []byte{0x01}); err == nil {
		t.Fatal("pager was writable after query_only was disabled: mode=ro missing")
	}
	var rows int
	audit, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer audit.Close()
	if err := audit.QueryRowContext(ctx, "SELECT count(*) FROM input_ledger").Scan(&rows); err != nil || rows != 0 {
		t.Fatalf("read-only mode leaked a write: %d %v", rows, err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("read-only query corrupted/removed sqlite database:", err)
	}
}

func TestF4SegregatedReadCapabilities(t *testing.T) {
	for _, restricted := range []any{coreAuthority{}, actorAuthority{}, publicAuthority{}} {
		if _, ok := restricted.(queryhost.AuditAuthority); ok {
			t.Fatal("restricted host can recover privileged raw query methods")
		}
		if _, ok := restricted.(interface {
			ExecContext(context.Context, string, ...any) (sql.Result, error)
		}); ok {
			t.Fatal("restricted host exposes a SQL mutation port")
		}
	}
	if _, ok := any(publicAuthority{}).(queryhost.POVAuthority); ok {
		t.Fatal("public host can obtain actor-sensitive methods")
	}
	if _, ok := any(actorAuthority{}).(queryhost.PublicAuthority); ok {
		t.Fatal("POV host unexpectedly gains public authority")
	}
}

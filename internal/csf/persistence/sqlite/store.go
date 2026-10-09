// Package sqlite is the single-file authoritative persistence adapter for CSF V1.
package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/genesis"
	modernsqlite "modernc.org/sqlite"
)

const (
	ExpectedVersion  = "3.53.4"
	ExpectedSourceID = "2026-07-24 19:02:57 bf7c7f30031888f4e796e429ab3978879485813aaca6f641c7b33e4e09459bcc"
)

//go:embed compile-options-linux-amd64.json
var compileOptionsManifest []byte

type runtimeManifest struct {
	GOOS           string   `json:"goos"`
	GOARCH         string   `json:"goarch"`
	Driver         string   `json:"driver"`
	SQLiteVersion  string   `json:"sqlite_version"`
	SQLiteSourceID string   `json:"sqlite_source_id"`
	CompileOptions []string `json:"compile_options"`
}

type Store struct{ db *sql.DB }

func Open(ctx context.Context, path string) (*Store, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	dsn := (&url.URL{Scheme: "file", Path: filepath.ToSlash(absolute), RawQuery: "_pragma=journal_mode%28WAL%29&_pragma=synchronous%28FULL%29&_pragma=foreign_keys%28ON%29"}).String()
	connector, err := modernsqlite.NewConnector(dsn)
	if err != nil {
		return nil, err
	}
	db := sql.OpenDB(connector)
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	store := &Store{db: db}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}
	if err := store.verifyRuntime(ctx); err != nil {
		db.Close()
		return nil, err
	}
	if err := store.createSchema(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) verifyRuntime(ctx context.Context) error {
	var version, sourceID, journal string
	var synchronous int
	if err := s.db.QueryRowContext(ctx, "SELECT sqlite_version(), sqlite_source_id()").Scan(&version, &sourceID); err != nil {
		return err
	}
	if version != ExpectedVersion || sourceID != ExpectedSourceID {
		return fmt.Errorf("unsupported SQLite identity: version=%s source_id=%s", version, sourceID)
	}
	if err := s.db.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&journal); err != nil {
		return err
	}
	if !strings.EqualFold(journal, "wal") {
		return fmt.Errorf("journal_mode must be WAL, got %s", journal)
	}
	if err := s.db.QueryRowContext(ctx, "PRAGMA synchronous").Scan(&synchronous); err != nil {
		return err
	}
	if synchronous != 2 {
		return fmt.Errorf("synchronous must be FULL(2), got %d", synchronous)
	}
	rows, err := s.db.QueryContext(ctx, "PRAGMA compile_options")
	if err != nil {
		return err
	}
	defer rows.Close()
	actual := []string{}
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return err
		}
		actual = append(actual, value)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	sort.Strings(actual)
	var expected runtimeManifest
	if err := json.Unmarshal(compileOptionsManifest, &expected); err != nil {
		return fmt.Errorf("decode SQLite compile-options manifest: %w", err)
	}
	if expected.GOOS != runtime.GOOS || expected.GOARCH != runtime.GOARCH || expected.Driver != "modernc.org/sqlite@v1.60.1" || expected.SQLiteVersion != version || expected.SQLiteSourceID != sourceID {
		return fmt.Errorf("SQLite runtime manifest identity mismatch for %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	if len(actual) != len(expected.CompileOptions) {
		return fmt.Errorf("SQLite compile options count mismatch")
	}
	for index := range actual {
		if actual[index] != expected.CompileOptions[index] {
			return fmt.Errorf("SQLite compile option mismatch at %d: got %q want %q", index, actual[index], expected.CompileOptions[index])
		}
	}
	return nil
}

func (s *Store) createSchema(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS genesis_records (
  run_id BLOB PRIMARY KEY CHECK(length(run_id)=32),
  genesis_id BLOB NOT NULL UNIQUE CHECK(length(genesis_id)=32),
  digest BLOB NOT NULL CHECK(length(digest)=32),
  canonical BLOB NOT NULL
) STRICT;
CREATE TABLE IF NOT EXISTS input_ledger (id BLOB PRIMARY KEY CHECK(length(id)=32), canonical BLOB NOT NULL) STRICT;
CREATE TABLE IF NOT EXISTS decision_ledger (id BLOB PRIMARY KEY CHECK(length(id)=32), canonical BLOB NOT NULL) STRICT;
CREATE TABLE IF NOT EXISTS event_store (id BLOB PRIMARY KEY CHECK(length(id)=32), canonical BLOB NOT NULL) STRICT;
CREATE TABLE IF NOT EXISTS evidence_ledger (id BLOB PRIMARY KEY CHECK(length(id)=32), canonical BLOB NOT NULL) STRICT;
CREATE TABLE IF NOT EXISTS causal_outbox (id BLOB PRIMARY KEY CHECK(length(id)=32), canonical BLOB NOT NULL) STRICT;
CREATE TABLE IF NOT EXISTS world_revisions (revision INTEGER PRIMARY KEY, state_hash BLOB NOT NULL CHECK(length(state_hash)=32), canonical BLOB NOT NULL) STRICT;
CREATE TABLE IF NOT EXISTS cycle_control_state (run_id BLOB PRIMARY KEY CHECK(length(run_id)=32), canonical BLOB NOT NULL) STRICT;
CREATE TABLE IF NOT EXISTS snapshots (id BLOB PRIMARY KEY CHECK(length(id)=32), canonical BLOB NOT NULL) STRICT;
`)
	return err
}

func (s *Store) PersistGenesis(ctx context.Context, manifest *genesis.Genesis) error {
	result, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO genesis_records(run_id, genesis_id, digest, canonical) VALUES(?,?,?,?)`, manifest.RunID[:], manifest.ID[:], manifest.Digest[:], manifest.CanonicalBytes)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 1 {
		return nil
	}
	var existingID, existingDigest, existingCanonical []byte
	if err := s.db.QueryRowContext(ctx, `SELECT genesis_id,digest,canonical FROM genesis_records WHERE run_id=?`, manifest.RunID[:]).Scan(&existingID, &existingDigest, &existingCanonical); err != nil {
		return err
	}
	if !bytes.Equal(existingID, manifest.ID[:]) || !bytes.Equal(existingDigest, manifest.Digest[:]) || !bytes.Equal(existingCanonical, manifest.CanonicalBytes) {
		return errors.New("genesis identity collision or pin corruption")
	}
	return nil
}

func (s *Store) LoadGenesis(ctx context.Context, runID [32]byte) ([]byte, error) {
	var encoded []byte
	err := s.db.QueryRowContext(ctx, `SELECT canonical FROM genesis_records WHERE run_id=?`, runID[:]).Scan(&encoded)
	return encoded, err
}

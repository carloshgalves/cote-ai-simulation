//go:build linux

package commit

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"net/url"
	"path/filepath"
	"sort"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/clock"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
)

// This is a test-only, engine-privileged oracle. The canonical records below
// include BodyState and private owner checkpoints and MUST NOT be serialized
// into a public result or passed to a model, actor, public Observatory or RAG.
type referenceE1Location struct{ Name, Path string }
type referenceE1Table struct {
	Columns []string
	Rows    [][]byte
}
type referenceE1Database struct{ Tables map[string]referenceE1Table }
type referenceE1Snapshot struct {
	Databases map[string]referenceE1Database
}
type referenceE1ManifestEntry struct {
	Database string
	Table    string
	Rows     int
	SHA256   string
}

func (table referenceE1Table) digest() string {
	h := sha256.New()
	var size [8]byte
	write := func(v []byte) {
		binary.BigEndian.PutUint64(size[:], uint64(len(v)))
		_, _ = h.Write(size[:])
		_, _ = h.Write(v)
	}
	write([]byte("csfrv1-10e1.table-bytes.v1"))
	binary.BigEndian.PutUint64(size[:], uint64(len(table.Columns)))
	_, _ = h.Write(size[:])
	for _, name := range table.Columns {
		write([]byte(name))
	}
	binary.BigEndian.PutUint64(size[:], uint64(len(table.Rows)))
	_, _ = h.Write(size[:])
	for _, row := range table.Rows {
		write(row)
	}
	return hex.EncodeToString(h.Sum(nil))
}
func (s referenceE1Snapshot) Manifest() []referenceE1ManifestEntry {
	result := make([]referenceE1ManifestEntry, 0)
	names := make([]string, 0, len(s.Databases))
	for name := range s.Databases {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		tables := s.Databases[name].Tables
		keys := make([]string, 0, len(tables))
		for table := range tables {
			keys = append(keys, table)
		}
		sort.Strings(keys)
		for _, key := range keys {
			table := tables[key]
			result = append(result, referenceE1ManifestEntry{
				Database: name, Table: key, Rows: len(table.Rows), SHA256: table.digest(),
			})
		}
	}
	return result
}
func (s referenceE1Snapshot) clone() referenceE1Snapshot {
	copySnapshot := referenceE1Snapshot{Databases: make(map[string]referenceE1Database, len(s.Databases))}
	for name, db := range s.Databases {
		copied := referenceE1Database{Tables: make(map[string]referenceE1Table, len(db.Tables))}
		for key, table := range db.Tables {
			value := referenceE1Table{Columns: append([]string(nil), table.Columns...), Rows: make([][]byte, len(table.Rows))}
			for i, row := range table.Rows {
				value.Rows[i] = bytes.Clone(row)
			}
			copied.Tables[key] = value
		}
		copySnapshot.Databases[name] = copied
	}
	return copySnapshot
}

// Compare literal rows; SHA256 is only a safe reporting summary. Diagnostic
// errors identify the earliest table/ordinal, NEVER raw private bytes.
func referenceE1Compare(a, b referenceE1Snapshot) error {
	// An absent causal prefix is not proof of determinism. Never allow two
	// empty/unfinished snapshots to compare equal by vacuous truth.
	if len(a.Databases) == 0 || len(b.Databases) == 0 {
		return errors.New("E1 empty authority transcript cannot be accepted")
	}
	names := make([]string, 0, len(a.Databases))
	for name := range a.Databases {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		x := a.Databases[name]
		y, ok := b.Databases[name]
		if !ok {
			return fmt.Errorf("E1 missing database %s", name)
		}
		keys := make([]string, 0, len(x.Tables))
		for key := range x.Tables {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			l := x.Tables[key]
			r, ok := y.Tables[key]
			if !ok {
				return fmt.Errorf("E1 missing table %s/%s", name, key)
			}
			if len(l.Columns) != len(r.Columns) {
				return fmt.Errorf("E1 column counts differ at %s/%s", name, key)
			}
			for i := range l.Columns {
				if l.Columns[i] != r.Columns[i] {
					return fmt.Errorf("E1 column %d differs at %s/%s", i, name, key)
				}
			}
			if len(l.Rows) != len(r.Rows) {
				return fmt.Errorf("E1 row counts differ at %s/%s", name, key)
			}
			for i := range l.Rows {
				if !bytes.Equal(l.Rows[i], r.Rows[i]) {
					return fmt.Errorf("E1 canonical row bytes differ at %s/%s ordinal %d", name, key, i)
				}
			}
		}
		for key := range y.Tables {
			if _, ok := x.Tables[key]; !ok {
				return fmt.Errorf("E1 extra table %s/%s", name, key)
			}
		}
	}
	for name := range b.Databases {
		if _, ok := a.Databases[name]; !ok {
			return fmt.Errorf("E1 extra database %s", name)
		}
	}
	return nil
}
func (s referenceE1Snapshot) requireCausalAndOwners() error {
	if len(s.Databases) != 3 {
		return errors.New("E1 requires causal and two owner databases")
	}
	causal, ok := s.Databases["causal"]
	if !ok {
		return errors.New("E1 missing causal database")
	}
	for _, name := range []string{"__schema__", "genesis_records", "input_ledger",
		"decision_ledger", "event_store", "evidence_ledger", "causal_outbox",
		"world_revisions", "clock_states", "source_ingress_receipts",
		"input_source_cursors", "committed_cycles", "snapshots",
		"snapshot_prefix_members", "scheduled_occurrences", "trigger_activations",
		"cognitive_owner_receipts", "evidence_routes"} {
		t, ok := causal.Tables[name]
		if !ok || len(t.Columns) == 0 {
			return fmt.Errorf("E1 missing canonical table %s", name)
		}
	}
	if len(causal.Tables["world_revisions"].Rows) != 10 ||
		len(causal.Tables["committed_cycles"].Rows) != 9 {
		return errors.New("E1 expected genesis plus nine D3 world revisions/commits")
	}
	for _, name := range []string{"owner-a", "owner-b"} {
		db, ok := s.Databases[name]
		if !ok {
			return fmt.Errorf("E1 missing %s database", name)
		}
		for _, table := range []string{"__schema__", "checkpoints", "restore_acks"} {
			v, ok := db.Tables[table]
			if !ok || len(v.Rows) == 0 {
				return fmt.Errorf("E1 missing %s/%s", name, table)
			}
		}
	}
	return nil
}

// E1 is read-only and opens a consistent SQLite snapshot transaction for
// each authority. D3 must authenticate the completed causal run FIRST.
func referenceE1Read(ctx context.Context, locations []referenceE1Location) (referenceE1Snapshot, error) {
	var zero referenceE1Snapshot
	if ctx == nil || len(locations) == 0 {
		return zero, errors.New("E1 missing context or database paths")
	}
	out := referenceE1Snapshot{Databases: make(map[string]referenceE1Database, len(locations))}
	for _, loc := range locations {
		if loc.Name == "" || loc.Path == "" {
			return zero, errors.New("E1 empty authority path")
		}
		if _, ok := out.Databases[loc.Name]; ok {
			return zero, errors.New("E1 duplicate authority name")
		}
		database, err := referenceE1ReadDatabase(ctx, loc.Path)
		if err != nil {
			return zero, fmt.Errorf("E1 %s: %w", loc.Name, err)
		}
		out.Databases[loc.Name] = database
	}
	return out, nil
}
func referenceE1Value(v any) ([]any, error) {
	switch x := v.(type) {
	case nil:
		return []any{"null"}, nil
	case []byte:
		return []any{"blob", bytes.Clone(x)}, nil
	case string:
		return []any{"text", x}, nil
	case int64:
		return []any{"integer", x}, nil
	case bool:
		return []any{"bool", x}, nil
	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return nil, errors.New("E1 non-finite SQLite value")
		}
		return []any{"float", x}, nil
	default:
		return nil, fmt.Errorf("E1 unregistered SQLite value %T", v)
	}
}
func referenceE1ReadDatabase(ctx context.Context, path string) (referenceE1Database, error) {
	var zero referenceE1Database
	db, err := openReferenceReadOnlyDB(path)
	if err != nil {
		return zero, err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return zero, err
	}
	defer tx.Rollback()
	schema, err := referenceE1ReadQuery(ctx, tx, "__schema__",
		"SELECT type,name,tbl_name,sql FROM sqlite_master WHERE name NOT GLOB 'sqlite_*' ORDER BY type,name")
	if err != nil {
		return zero, err
	}
	out := referenceE1Database{Tables: map[string]referenceE1Table{"__schema__": schema}}
	rows, err := tx.QueryContext(ctx,
		"SELECT name FROM sqlite_master WHERE type='table' AND name NOT GLOB 'sqlite_*' ORDER BY name")
	if err != nil {
		return zero, err
	}
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return zero, err
		}
		// Table names come from trusted SQLite schema but do not allow arbitrary
		// identifiers to become SQL text. New schema must be reviewed explicitly.
		if name == "" || name == "__schema__" {
			rows.Close()
			return zero, errors.New("E1 reserved table name")
		}
		for _, r := range name {
			if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_') {
				rows.Close()
				return zero, errors.New("E1 unregistered SQLite table identifier")
			}
		}
		tables = append(tables, name)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return zero, err
	}
	if err := rows.Close(); err != nil {
		return zero, err
	}
	if len(tables) == 0 {
		return zero, errors.New("E1 empty authority database")
	}
	for _, table := range tables {
		value, err := referenceE1ReadQuery(ctx, tx, table, "SELECT * FROM "+table)
		if err != nil {
			return zero, fmt.Errorf("E1 table %s: %w", table, err)
		}
		out.Tables[table] = value
	}
	if err := tx.Commit(); err != nil {
		return zero, err
	}
	return out, nil
}
func referenceE1ReadQuery(ctx context.Context, tx *sql.Tx, tableName, query string) (referenceE1Table, error) {
	var zero referenceE1Table
	rows, err := tx.QueryContext(ctx, query)
	if err != nil {
		return zero, err
	}
	columns, err := rows.Columns()
	if err != nil {
		rows.Close()
		return zero, err
	}
	if len(columns) == 0 {
		rows.Close()
		return zero, errors.New("E1 row without columns")
	}
	out := referenceE1Table{Columns: append([]string(nil), columns...), Rows: make([][]byte, 0)}
	for rows.Next() {
		vals := make([]any, len(columns))
		dst := make([]any, len(columns))
		for i := range dst {
			dst[i] = &vals[i]
		}
		if err := rows.Scan(dst...); err != nil {
			rows.Close()
			return zero, err
		}
		typed := make([]any, len(vals))
		for i, v := range vals {
			if columns[i] == "canonical" {
				raw, ok := v.([]byte)
				if !ok {
					rows.Close()
					return zero, errors.New("E1 canonical record is not a BLOB")
				}
				if _, err := codec.StrictDecodePayload(raw); err != nil {
					rows.Close()
					return zero, fmt.Errorf("E1 corrupt canonical row in column %d: %w", i, err)
				}
				// Syntactic CBOR alone is not a verified clock. A canonical
				// empty array (0x80) is CBOR-valid yet cannot be a CSF clock.
				if tableName == "clock_states" {
					if _, err := clock.Decode(raw); err != nil {
						rows.Close()
						return zero, fmt.Errorf("E1 invalid authoritative clock payload: %w", err)
					}
				}
			}
			typed[i], err = referenceE1Value(v)
			if err != nil {
				rows.Close()
				return zero, err
			}
		}
		canonical, err := codec.CanonicalPayload(typed)
		if err != nil {
			rows.Close()
			return zero, err
		}
		out.Rows = append(out.Rows, canonical)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return zero, err
	}
	if err := rows.Close(); err != nil {
		return zero, err
	}
	sort.Slice(out.Rows, func(i, j int) bool { return bytes.Compare(out.Rows[i], out.Rows[j]) < 0 })
	return out, nil
}

// The writer exists only in the negative test: the oracle itself never calls
// it or obtains a writable Store.
func openReferenceWritableForE1Test(path string) (*sql.DB, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(abs)}
	query := url.Values{}
	query.Set("mode", "rw")
	u.RawQuery = query.Encode()
	return sql.Open("sqlite", u.String())
}

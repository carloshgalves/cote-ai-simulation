// Package operationalmetrics stores untrusted host/worker measurements in
// a physically distinct SQLite sidecar. It never imports causal stores or
// changes a run. Metrics are not inputs to replay, actor cognition or hashes.
package operationalmetrics

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"

	modernsqlite "modernc.org/sqlite"
)

// Sample is an extracausal observation supplied by the local host. All
// counters are operator measurements, never authenticated simulation facts.
// ID is an operator-assigned idempotency key; Run is correlation metadata.
type Sample struct {
	ID                [32]byte
	Run               [32]byte
	ObservedUnixNanos int64
	Stage             string
	DurationNanos     int64
	HostID            string
	WorkerID          string
	QueueDepth        int64
	TransportRetries  int64
	WorkerCalls       int64
	CostMicros        int64
	Currency          string
	RejectedCount     int64
	RejectCode        string
}

var identifier = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,63}$`)

func validSample(s Sample) error {
	switch s.Stage {
	case "derive", "evaluate", "commit", "perception", "replay", "transport", "worker":
	default:
		return errors.New("unregistered operational metric stage")
	}
	if s.ID == ([32]byte{}) {
		return errors.New("operational sample id must be nonzero")
	}
	if s.ObservedUnixNanos < 0 || s.DurationNanos < 0 || s.QueueDepth < 0 ||
		s.TransportRetries < 0 || s.WorkerCalls < 0 || s.CostMicros < 0 || s.RejectedCount < 0 {
		return errors.New("negative operational metric")
	}
	if !identifier.MatchString(s.HostID) {
		return errors.New("invalid operational host ID")
	}
	if s.WorkerID != "" && !identifier.MatchString(s.WorkerID) {
		return errors.New("invalid worker identifier")
	}
	if s.CostMicros != 0 {
		if s.Currency != "BRL" && s.Currency != "USD" {
			return errors.New("nonzero cost requires an explicit supported currency")
		}
	} else if s.Currency != "" && s.Currency != "BRL" && s.Currency != "USD" {
		return errors.New("unsupported operational cost currency")
	}
	if s.RejectedCount > 0 {
		switch s.RejectCode {
		case "INVALID", "TIMEOUT", "CONFLICT", "OTHER":
		default:
			return errors.New("rejection count requires an allowlisted reason")
		}
	} else if s.RejectCode != "" {
		return errors.New("rejection reason without rejected samples")
	}
	return nil
}

// sidecarPath rejects identical paths, symlinks and hardlinks to the causal
// database before opening the sidecar. Production must keep these paths under
// a trusted local operator directory; there is no remote untrusted host.
func sidecarPath(causal, sidecar string) (string, error) {
	if causal == "" || sidecar == "" {
		return "", errors.New("both causal and operational paths required")
	}
	a, err := filepath.Abs(causal)
	if err != nil {
		return "", err
	}
	b, err := filepath.Abs(sidecar)
	if err != nil {
		return "", err
	}
	if a == b {
		return "", errors.New("operational database must not alias causal database")
	}
	// Reject sidecar symlinks even when their targets are not yet created.
	// os.Stat(dangling) looks like ENOENT; opening it with mode=rwc could
	// otherwise CREATE the causal database at that target before our second
	// alias check has a chance to run.
	link, linkErr := os.Lstat(b)
	if linkErr != nil && !errors.Is(linkErr, os.ErrNotExist) {
		return "", linkErr
	}
	if linkErr == nil && link.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("operational sidecar must not be a symbolic link")
	}
	// Check both canonical parent directories *before* opening either file.
	// A directory symlink can make distinct absent textual paths aliases.
	parentA, err := filepath.EvalSymlinks(filepath.Dir(a))
	if err != nil {
		return "", fmt.Errorf("causal parent not accessible: %w", err)
	}
	parentB, err := filepath.EvalSymlinks(filepath.Dir(b))
	if err != nil {
		return "", fmt.Errorf("operational parent not accessible: %w", err)
	}
	if filepath.Join(parentA, filepath.Base(a)) == filepath.Join(parentB, filepath.Base(b)) {
		return "", errors.New("operational file resolves to the future causal path")
	}
	statA, errA := os.Stat(a)
	if errA != nil && !errors.Is(errA, os.ErrNotExist) {
		return "", errA
	}
	statB, errB := os.Stat(b)
	if errB != nil && !errors.Is(errB, os.ErrNotExist) {
		return "", errB
	}
	if errA == nil && errB == nil && os.SameFile(statA, statB) {
		return "", errors.New("operational file aliases causal file (hardlink/symlink)")
	}
	return b, nil
}
func metricURI(path, mode string) string {
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(path)}
	q := url.Values{}
	q.Set("mode", mode)
	q.Add("_pragma", "busy_timeout(5000)")
	if mode == "ro" {
		q.Add("_pragma", "query_only(ON)")
	}
	u.RawQuery = q.Encode()
	return u.String()
}
func openDB(ctx context.Context, path, mode string) (*sql.DB, error) {
	connector, err := modernsqlite.NewConnector(metricURI(path, mode))
	if err != nil {
		return nil, err
	}
	db := sql.OpenDB(connector)
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

const schema = `CREATE TABLE IF NOT EXISTS operational_samples (
 id BLOB PRIMARY KEY CHECK(length(id)=32),
 run_id BLOB NOT NULL CHECK(length(run_id)=32),
 at_ns INTEGER NOT NULL CHECK(at_ns>=0),
 body BLOB NOT NULL CHECK(length(body)>0 AND length(body)<=4096)
) STRICT;
CREATE INDEX IF NOT EXISTS idx_operational_sample_run ON operational_samples(run_id,at_ns,id);`

// Recorder is deliberately NOT a member of the Observatory and owns only
// the operator's separate metrics file. It never receives a causal Store.
type Recorder struct{ db *sql.DB }

func OpenRecorder(ctx context.Context, causal, sidecar string) (*Recorder, error) {
	p, err := sidecarPath(causal, sidecar)
	if err != nil {
		return nil, err
	}
	_, statErr := os.Stat(p)
	fresh := errors.Is(statErr, os.ErrNotExist)
	if statErr != nil && !fresh {
		return nil, statErr
	}
	db, err := openDB(ctx, p, "rwc")
	if err != nil {
		return nil, err
	}
	fail := func(e error) (*Recorder, error) { db.Close(); return nil, e }
	if _, err := sidecarPath(causal, p); err != nil {
		return fail(err)
	}
	var version int
	if err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return fail(err)
	}
	if fresh {
		if version != 0 {
			return fail(errors.New("new operational metrics DB has unexpected version"))
		}
	} else if version != 1 {
		return fail(errors.New("existing metrics file has no registered v1 schema"))
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, schema); err != nil {
		return fail(err)
	}
	if fresh {
		if _, err := tx.ExecContext(ctx, "PRAGMA user_version=1"); err != nil {
			return fail(err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fail(err)
	}
	return &Recorder{db: db}, nil
}
func (r *Recorder) Close() error {
	if r == nil || r.db == nil {
		return errors.New("nil operational recorder")
	}
	return r.db.Close()
}
func (r *Recorder) Record(ctx context.Context, s Sample) error {
	if r == nil || r.db == nil {
		return errors.New("nil operational recorder")
	}
	if err := validSample(s); err != nil {
		return err
	}
	raw, err := json.Marshal(s)
	if err != nil {
		return err
	}
	if len(raw) > 4096 {
		return errors.New("operational sample exceeds bounded schema")
	}
	res, err := r.db.ExecContext(ctx, "INSERT OR IGNORE INTO operational_samples(id,run_id,at_ns,body) VALUES (?,?,?,?)",
		s.ID[:], s.Run[:], s.ObservedUnixNanos, raw)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 1 {
		return nil
	}
	var stored []byte
	var run []byte
	var at int64
	if err := r.db.QueryRowContext(ctx, "SELECT run_id,at_ns,body FROM operational_samples WHERE id=?", s.ID[:]).
		Scan(&run, &at, &stored); err != nil {
		return err
	}
	if !bytes.Equal(stored, raw) || !bytes.Equal(run, s.Run[:]) || at != s.ObservedUnixNanos {
		return errors.New("operational sample identity collision")
	}
	return nil
}

// Reader uses an enforced mode=ro connection independently of Recorder.
// The absence of the sidecar is an error (not proof of zero measured cost).
type Reader struct{ db *sql.DB }

func OpenReader(ctx context.Context, causal, sidecar string) (*Reader, error) {
	p, err := sidecarPath(causal, sidecar)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(p); err != nil {
		return nil, fmt.Errorf("operational sidecar unavailable: %w", err)
	}
	db, err := openDB(ctx, p, "ro")
	if err != nil {
		return nil, err
	}
	fail := func(e error) (*Reader, error) { db.Close(); return nil, e }
	if _, err := sidecarPath(causal, p); err != nil {
		return fail(err)
	}
	var mode int
	if err := db.QueryRowContext(ctx, "PRAGMA query_only").Scan(&mode); err != nil || mode != 1 {
		return fail(errors.New("operational reader lacks SQLite query_only protection"))
	}
	var version int
	if err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil || version != 1 {
		return fail(errors.New("unsupported operational metrics schema version"))
	}
	return &Reader{db: db}, nil
}
func (r *Reader) Close() error {
	if r == nil || r.db == nil {
		return errors.New("nil operational reader")
	}
	return r.db.Close()
}

// Read returns a complete bounded snapshot or an error; it never truncates
// silently. It does not query or validate the causal run and cannot become
// authoritative for world facts, actor context or decisions.
func (r *Reader) Read(ctx context.Context, run [32]byte, limit int) ([]Sample, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("nil operational reader")
	}
	if limit <= 0 || limit > 1000 {
		return nil, errors.New("operational query bound must be in 1..1000")
	}
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, "SELECT id,run_id,at_ns,body FROM operational_samples WHERE run_id=? ORDER BY at_ns,id LIMIT ?", run[:], limit+1)
	if err != nil {
		return nil, err
	}
	out := make([]Sample, 0)
	for rows.Next() {
		var id, owner, body []byte
		var at int64
		if err := rows.Scan(&id, &owner, &at, &body); err != nil {
			rows.Close()
			return nil, err
		}
		if len(id) != 32 || len(owner) != 32 || !bytes.Equal(owner, run[:]) || len(body) > 4096 {
			rows.Close()
			return nil, errors.New("operational sample header corrupted")
		}
		dec := json.NewDecoder(bytes.NewReader(body))
		dec.DisallowUnknownFields()
		var s Sample
		if err := dec.Decode(&s); err != nil {
			rows.Close()
			return nil, err
		}
		var extra any
		if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
			rows.Close()
			return nil, errors.New("operational sample has trailing data")
		}
		canonical, err := json.Marshal(s)
		if err != nil || !bytes.Equal(canonical, body) || s.ID != idToArray(id) ||
			s.Run != run || s.ObservedUnixNanos != at {
			rows.Close()
			return nil, errors.New("operational sample bytes/SQL metadata disagree")
		}
		if err := validSample(s); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, s)
		if len(out) > limit {
			rows.Close()
			return nil, errors.New("operational query exceeds requested bound")
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return out, nil
}
func idToArray(value []byte) (id [32]byte) { copy(id[:], value); return id }

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
	"reflect"
	"runtime"
	"sort"
	"strings"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/clock"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/genesis"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/ingressguard"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
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
CREATE TABLE IF NOT EXISTS round_integrity_seals (
 id BLOB PRIMARY KEY CHECK(length(id)=32),
 run_id BLOB NOT NULL CHECK(length(run_id)=32),
 canonical_hash BLOB NOT NULL CHECK(length(canonical_hash)=32)
) STRICT;
CREATE TABLE IF NOT EXISTS input_slot_responses (slot_key BLOB PRIMARY KEY CHECK(length(slot_key)=32), record_id BLOB NOT NULL UNIQUE CHECK(length(record_id)=32), canonical BLOB NOT NULL) STRICT;
CREATE TABLE IF NOT EXISTS input_source_cursors (source_id TEXT PRIMARY KEY, next_seq INTEGER NOT NULL CHECK(next_seq>=0)) STRICT;
CREATE TABLE IF NOT EXISTS source_ingress_receipts (source_id TEXT NOT NULL, ingress_seq INTEGER NOT NULL CHECK(ingress_seq>=0), record_id BLOB NOT NULL CHECK(length(record_id)=32), canonical BLOB NOT NULL, PRIMARY KEY(source_id,ingress_seq)) STRICT;
CREATE TABLE IF NOT EXISTS decision_ledger (id BLOB PRIMARY KEY CHECK(length(id)=32), canonical BLOB NOT NULL) STRICT;
CREATE TABLE IF NOT EXISTS event_store (id BLOB PRIMARY KEY CHECK(length(id)=32), canonical BLOB NOT NULL) STRICT;
CREATE TABLE IF NOT EXISTS evidence_ledger (id BLOB PRIMARY KEY CHECK(length(id)=32), canonical BLOB NOT NULL) STRICT;
CREATE TABLE IF NOT EXISTS causal_outbox (id BLOB PRIMARY KEY CHECK(length(id)=32), canonical BLOB NOT NULL) STRICT;
CREATE TABLE IF NOT EXISTS world_revisions (revision INTEGER PRIMARY KEY, state_hash BLOB NOT NULL CHECK(length(state_hash)=32), canonical BLOB NOT NULL) STRICT;
CREATE TABLE IF NOT EXISTS cycle_control_state (run_id BLOB PRIMARY KEY CHECK(length(run_id)=32), canonical BLOB NOT NULL) STRICT;
CREATE TABLE IF NOT EXISTS snapshots (id BLOB PRIMARY KEY CHECK(length(id)=32), canonical BLOB NOT NULL) STRICT;
CREATE TABLE IF NOT EXISTS clock_states (
 run_id BLOB PRIMARY KEY CHECK(length(run_id)=32),
 canonical BLOB NOT NULL
) STRICT;
CREATE TABLE IF NOT EXISTS scheduled_occurrences (
 id BLOB PRIMARY KEY CHECK(length(id)=32),
 run_id BLOB NOT NULL CHECK(length(run_id)=32),
 eligibility_instant INTEGER NOT NULL,
 eligibility_ordinal INTEGER NOT NULL CHECK(eligibility_ordinal>=0),
 lifecycle INTEGER NOT NULL CHECK(lifecycle IN (0,1,2)),
 canonical BLOB NOT NULL
) STRICT;
CREATE INDEX IF NOT EXISTS idx_occurrence_next ON scheduled_occurrences(run_id,lifecycle,eligibility_instant,eligibility_ordinal,id);
CREATE TABLE IF NOT EXISTS trigger_registry_seals (
 run_id BLOB PRIMARY KEY CHECK(length(run_id)=32),
 genesis_id BLOB NOT NULL CHECK(length(genesis_id)=32),
 genesis_digest BLOB NOT NULL CHECK(length(genesis_digest)=32),
 registry_hash BLOB NOT NULL CHECK(length(registry_hash)=32)
) STRICT;
CREATE TABLE IF NOT EXISTS trigger_definitions (
 definition_id BLOB PRIMARY KEY CHECK(length(definition_id)=32),
 run_id BLOB NOT NULL CHECK(length(run_id)=32),
 genesis_id BLOB NOT NULL CHECK(length(genesis_id)=32),
 genesis_digest BLOB NOT NULL CHECK(length(genesis_digest)=32),
 content_digest BLOB NOT NULL CHECK(length(content_digest)=32),
 canonical BLOB NOT NULL
) STRICT;
CREATE INDEX IF NOT EXISTS idx_trigger_definitions_run ON trigger_definitions(run_id,definition_id);
CREATE TABLE IF NOT EXISTS trigger_runtime (
 definition_id BLOB PRIMARY KEY CHECK(length(definition_id)=32),
 run_id BLOB NOT NULL CHECK(length(run_id)=32),
 canonical BLOB NOT NULL
) STRICT;
CREATE TABLE IF NOT EXISTS trigger_activations (
 id BLOB PRIMARY KEY CHECK(length(id)=32),
 run_id BLOB NOT NULL CHECK(length(run_id)=32),
 eligibility_instant INTEGER NOT NULL,
 eligibility_ordinal INTEGER NOT NULL CHECK(eligibility_ordinal>=0),
 lifecycle INTEGER NOT NULL CHECK(lifecycle IN (0,2)),
 canonical BLOB NOT NULL
) STRICT;
CREATE INDEX IF NOT EXISTS idx_trigger_next ON trigger_activations(run_id,lifecycle,eligibility_instant,eligibility_ordinal,id);
CREATE TABLE IF NOT EXISTS committed_cycles (
 run_id BLOB NOT NULL CHECK(length(run_id)=32),
 cycle_id BLOB NOT NULL CHECK(length(cycle_id)=32),
 commit_id BLOB NOT NULL CHECK(length(commit_id)=32),
 PRIMARY KEY(run_id,cycle_id),
 UNIQUE(commit_id)
) STRICT;
`)
	return err
}

// Append stores one canonical input-ledger record idempotently.
func (s *Store) Append(ctx context.Context, record ports.CanonicalRecord) error {
	// Guarded records cannot bypass the seal even through the generic adapter API.
	kind, runID, instant, ordinal, err := guardedKind(record)
	if err != nil { return err }
	switch kind {
	case "source": return s.AppendSourceDeclaration(ctx, runID, record)
	case "round": return s.AppendRoundDeclaration(ctx, runID, instant, ordinal, record)
 default:
  // ExogenousInput and SourceClosure must enter with their ingress receipt
  // and source cursor in one transaction, never through generic Append.
  decoded,err:=codec.StrictDecodePayload(record.Canonical)
  if err==nil{
   fields,ok:=decoded.([]any)
   if ok&&(len(fields)==13||len(fields)==7)&&len(fields)>2{
    _,runOK:=fields[1].([]byte)
    _,sourceOK:=fields[2].(string)
    if runOK&&sourceOK{return errors.New("source ingress requires AppendIngress and atomic receipt")}
   }
  }
  return appendTo(ctx,s.db,"input_ledger",record)
	}
}

// CBOR positive instants decode as uint64 despite an int64 input.
func decodedInstant(v any) (int64,bool) {
	switch n:=v.(type) {
	case int64:return n,true
	case uint64:if n<=uint64(^uint64(0)>>1){return int64(n),true}
	}
	return 0,false
}

func guardedKind(record ports.CanonicalRecord) (string, [32]byte, int64, uint64, error) {
	var runID [32]byte
	payload, err := codec.StrictDecodePayload(record.Canonical)
	if err != nil { return "", runID, 0, 0, nil } // envelope receipt, etc.
	fields, ok := payload.([]any)
	if !ok { return "", runID, 0, 0, nil }
	isSource := len(fields)==3 && fields[0]=="source-declaration-v1"
	isRound := len(fields)==11 && fields[2]!=nil && isRoundSource(fields[2])
	if !isSource && !isRound { return "", runID, 0, 0, nil }
	if len(fields)<2 { return "",runID,0,0,errors.New("guarded append invalid fields") }
	id, ok := fields[1].([]byte)
	if !ok || len(id)!=32 { return "",runID,0,0,errors.New("guarded append invalid run id") }
	copy(runID[:],id)
	if isSource { return "source",runID,0,0,nil }
	coord,ok := fields[5].([]any)
	if !ok || len(coord)!=2 { return "",runID,0,0,errors.New("guarded round invalid coordinate") }
	instant,ok := decodedInstant(coord[0])
	if !ok { return "",runID,0,0,errors.New("guarded round invalid instant") }
	ordinal,ok := coord[1].(uint64)
	if !ok { return "",runID,0,0,errors.New("guarded round invalid ordinal") }
	return "round",runID,instant,ordinal,nil
}

func isRoundSource(value any) bool { _, ok := value.(string); return ok }

// The guard's authority comes from the persisted canonical record, never from
// caller-supplied routing arguments. A mismatch is rejected before opening the
// admission transaction.
func validateGuardArguments(record ports.CanonicalRecord, wantedKind string, runID [32]byte, instant int64, ordinal uint64) error {
	kind, canonicalRun, canonicalInstant, canonicalOrdinal, err := guardedKind(record)
	if err != nil { return err }
	if kind != wantedKind || canonicalRun != runID {
		return errors.New("guarded record kind or run does not match canonical bytes")
	}
	if wantedKind == "round" && (canonicalInstant != instant || canonicalOrdinal != ordinal) {
		return errors.New("guarded round coordinate does not match canonical bytes")
	}
	return nil
}

func (s *Store) AppendSourceDeclaration(ctx context.Context, runID [32]byte, record ports.CanonicalRecord) error {
	if err := validateGuardArguments(record, "source", runID, 0, 0); err != nil { return err }
	return s.guardedAppend(ctx, runID, nil, record)
}

func (s *Store) AppendRoundDeclaration(ctx context.Context, runID [32]byte, instant int64, ordinal uint64, record ports.CanonicalRecord) error {
	if err := validateGuardArguments(record, "round", runID, instant, ordinal); err != nil { return err }
	return s.guardedAppend(ctx, runID, &roundGuardCoordinate{instant: instant, ordinal: ordinal}, record)
}

type roundGuardCoordinate struct { instant int64; ordinal uint64 }

// The current clock is read in the SAME transaction as ingress publication.
// The first executable coordinate is (genesis instant,0), and after a commit
// is (current instant,last ordinal+1); old pending cannot be made causal later.
func verifyIngressClockFloor(ctx context.Context,tx *sql.Tx,runID [32]byte,at domain.EligibilityCoordinate)error{
 var raw []byte
 err:=tx.QueryRowContext(ctx,"SELECT canonical FROM clock_states WHERE run_id=?",runID[:]).Scan(&raw)
 var floor domain.EligibilityCoordinate
 if err==sql.ErrNoRows {
  // The admission ledger has legacy standalone tests that intentionally do
  // not initialize a run. With an actual persisted genesis, however, its
  // verified initial instant is already a durable eligibility floor.
  var genesisRaw []byte
  err=tx.QueryRowContext(ctx,"SELECT canonical FROM genesis_records WHERE run_id=?",runID[:]).Scan(&genesisRaw)
  if err==sql.ErrNoRows{return nil}
  if err!=nil{return err}
  env,err:=codec.StrictDecodeEnvelope(genesisRaw);if err!=nil{return err}
  payload,ok:=env.Value.([]any);if !ok||len(payload)<4{return errors.New("genesis floor malformed")}
  instant,ok:=decodedInstant(payload[3]);if !ok{return errors.New("genesis instant malformed")}
  floor=domain.EligibilityCoordinate{NotBeforeInstant:domain.SimulationInstant(instant)}
 }else{
  if err!=nil{return err}
  current,err:=clock.Decode(raw);if err!=nil{return err}
  if current.LastOrdinal==int64(^uint64(0)>>1){return errors.New("causal ordinal exhausted")}
  floor=domain.EligibilityCoordinate{NotBeforeInstant:current.Instant,NotBeforeCycleOrdinal:domain.CycleOrdinal(current.LastOrdinal+1)}
 }
 if at.Compare(floor)<0{return errors.New("causal ingress precedes current clock floor")}
 return nil
}


func (s *Store) guardedAppend(ctx context.Context, runID [32]byte, round *roundGuardCoordinate, record ports.CanonicalRecord) error {
	tx, err := s.db.BeginTx(ctx,nil)
	if err!=nil { return err }
	defer tx.Rollback()
 if round!=nil{
  at:=domain.EligibilityCoordinate{NotBeforeInstant:domain.SimulationInstant(round.instant),NotBeforeCycleOrdinal:domain.CycleOrdinal(round.ordinal)}
  if err:=verifyIngressClockFloor(ctx,tx,runID,at);err!=nil{return err}
 }
	rows,err := tx.QueryContext(ctx,"SELECT canonical FROM decision_ledger")
	if err!=nil { return err }
	for rows.Next() {
		var canonical []byte
		if err:=rows.Scan(&canonical);err!=nil { rows.Close();return err }
		fields,err:=codec.StrictDecodePayload(canonical)
		if err!=nil {rows.Close();return errors.New("invalid persisted fence")}
		body,ok:=fields.([]any)
		// CycleCommit lives in decision_ledger too; it is NOT an admission
		// fence and must never be mistaken for a causal seal.
		if !ok {rows.Close();return errors.New("invalid decision record body")}
		// Published terminal journal records (commit, abort, retry) and
		// candidate audit roots are not admission fences. Their canonical
		// identities are checked at their individual transaction insertion
		// gates, and they cannot widen the previous input cut.
		if len(body)==24||len(body)==15||len(body)==7||len(body)==17 {continue}
		if len(body)!=18 && len(body)!=6 {rows.Close();return errors.New("unknown decision record in admission guard")}
		fenceRun,ok:=body[0].([]byte)
		if !ok || len(fenceRun)!=32 {rows.Close();return errors.New("invalid persisted fence run")}
		if !bytes.Equal(fenceRun,runID[:]) {continue}
		if round==nil {rows.Close();return errors.New("source declaration after admission fence")}
        _,cut,err:=ingressguard.FenceCoordinate(canonical)
        if err!=nil{rows.Close();return err}
        proposed:=domain.EligibilityCoordinate{NotBeforeInstant:domain.SimulationInstant(round.instant),NotBeforeCycleOrdinal:domain.CycleOrdinal(round.ordinal)}
        if proposed.Compare(cut)<=0{
            rows.Close()
            return errors.New("round eligible at or before sealed admission target")
        }
	}
	if err:=rows.Err();err!=nil {rows.Close();return err}
	if err:=rows.Close();err!=nil{return err}
	if err:=appendTx(ctx,tx,record);err!=nil{return err}
 if round!=nil{
  // Round IDs do not include the actor and slots. Record their complete
  // canonical hash atomically as a derived integrity index; never let a
  // later payload substitution keep the same causal identity unnoticed.
  hash:=identity.OpaqueContentHash(record.Canonical)
  if _,err:=tx.ExecContext(ctx,"INSERT OR IGNORE INTO round_integrity_seals(id,run_id,canonical_hash) VALUES(?,?,?)",record.Identity[:],runID[:],hash[:]);err!=nil{return err}
  var pinned []byte
  if err:=tx.QueryRowContext(ctx,"SELECT canonical_hash FROM round_integrity_seals WHERE id=? AND run_id=?",record.Identity[:],runID[:]).Scan(&pinned);err!=nil{return err}
  if !bytes.Equal(pinned,hash[:]){return errors.New("round declaration changed after canonical sealing")}
 }
 return tx.Commit()
}

// Direct fence writes cannot prove membership of the closed input prefix.
// Publication is only supported via AppendFenceIfPrefix's atomic CAS.
func (s *Store) AppendFence(_ context.Context, _ ports.CanonicalRecord) error {
	return errors.New("admission fence requires prefix-conditional publication")
}

// AppendFenceIfPrefix compares the input prefix and inserts the fence in one
// SQLite transaction. A concurrent writer invalidates the read transaction
// or changes the compared prefix; in either case publication fails closed.
func (s *Store) AppendFenceIfPrefix(ctx context.Context, fence ports.CanonicalRecord, expected []ports.CanonicalRecord) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil { return err }
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, "SELECT id,canonical FROM input_ledger ORDER BY rowid")
	if err != nil { return err }
	index := 0
	for rows.Next() {
		var id, canonical []byte
		if err := rows.Scan(&id, &canonical); err != nil { rows.Close(); return err }
		if index >= len(expected) || !bytes.Equal(id, expected[index].Identity[:]) || !bytes.Equal(canonical, expected[index].Canonical) {
			rows.Close()
			return errors.New("input ledger changed before fence publication")
		}
		index++
	}
	if err := rows.Err(); err != nil { rows.Close(); return err }
	if err := rows.Close(); err != nil { return err }
	if index != len(expected) { return errors.New("input ledger changed before fence publication") }
	result, err := tx.ExecContext(ctx, "INSERT OR IGNORE INTO decision_ledger(id,canonical) VALUES(?,?)", fence.Identity[:], fence.Canonical)
	if err != nil { return err }
	affected, err := result.RowsAffected()
	if err != nil { return err }
	if affected == 0 {
		var persisted []byte
		if err := tx.QueryRowContext(ctx, "SELECT canonical FROM decision_ledger WHERE id=?", fence.Identity[:]).Scan(&persisted); err != nil { return err }
		if !bytes.Equal(persisted, fence.Canonical) { return errors.New("admission fence identity collision") }
	}
    // Reconcile the exact persisted fence even when the SQLite driver reports
    // an idempotent INSERT OR IGNORE. An existing byte-identical fence must
    // not bypass the cycle-control transition.
    next,err:=admitTerminalFence(ctx,tx,fence);if err!=nil{return err}
    if next!=nil{
        // CycleControlState is indexed by RUN, not by cycle identity.
        // The control payload's optional cycle ID must never be mistaken for
        // its owning run key.
        decoded,err:=codec.StrictDecodePayload(fence.Canonical);if err!=nil{return err}
        fields:=decoded.([]any)
        runBytes:=fields[0].([]byte)
        var run [32]byte;copy(run[:],runBytes)
        if err:=storeCycleControl(ctx,tx,run,next);err!=nil{return err}
    }
	return tx.Commit()
}

func (s *Store) GetFence(ctx context.Context, id [32]byte) ([]byte, error) {
	var canonical []byte
	err := s.db.QueryRowContext(ctx, `SELECT canonical FROM decision_ledger WHERE id=?`, id[:]).Scan(&canonical)
	return canonical, err
}

func appendTo(ctx context.Context, db *sql.DB, table string, record ports.CanonicalRecord) error {
	if table != "input_ledger" && table != "decision_ledger" {
		return errors.New("unsupported causal append authority")
	}
	result, err := db.ExecContext(ctx, `INSERT OR IGNORE INTO `+table+`(id,canonical) VALUES(?,?)`, record.Identity[:], record.Canonical)
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
	var existing []byte
	if err := db.QueryRowContext(ctx, `SELECT canonical FROM `+table+` WHERE id=?`, record.Identity[:]).Scan(&existing); err != nil {
		return err
	}
	if !bytes.Equal(existing, record.Canonical) {
		return errors.New("causal record identity collision")
	}
	return nil
}

func (s *Store) Get(ctx context.Context, id [32]byte) ([]byte, error) {
	var canonical []byte
	err := s.db.QueryRowContext(ctx, `SELECT canonical FROM input_ledger WHERE id=?`, id[:]).Scan(&canonical)
	return canonical, err
}

func (s *Store) Records(ctx context.Context) ([]ports.CanonicalRecord, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,canonical FROM input_ledger ORDER BY rowid`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []ports.CanonicalRecord
	for rows.Next() {
		var id, canonical []byte
		if err := rows.Scan(&id, &canonical); err != nil {
			return nil, err
		}
		if len(id) != 32 {
			return nil, errors.New("input ledger contains an invalid record identity")
		}
		var identity [32]byte
		copy(identity[:], id)
		records = append(records, ports.CanonicalRecord{Identity: identity, Canonical: bytes.Clone(canonical)})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return records, nil
}

func (s *Store) AppendIngress(ctx context.Context, source string, sequence uint64, record, receipt ports.CanonicalRecord) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Compare projections with the canonical ledger while the same writer
 // transaction holds the admission boundary. A stale SQL cursor must not
 // mint a gapless-looking but causally invalid sequence.
 if err:=verifyIngressProjections(ctx,tx);err!=nil{return err}
 // A source record identity has exactly one canonical ingress receipt.
 // Stale Ledger instances may retry an existing producer ID with the next
 // sequence. Reject atomically; callers can reopen to obtain the original
 // receipt rather than creating a second receipt for the same causal unit.
 var existingIngress []byte
 err=tx.QueryRowContext(ctx,"SELECT canonical FROM input_ledger WHERE id=?",record.Identity[:]).Scan(&existingIngress)
 if err==nil {
  if !bytes.Equal(existingIngress,record.Canonical) {
   return errors.New("source ingress identity collision")
  }
  return errors.New("source ingress record already published; reopen ledger for existing receipt")
 }
 if err!=sql.ErrNoRows{return err}
 var current uint64
	err = tx.QueryRowContext(ctx, `SELECT next_seq FROM input_source_cursors WHERE source_id=?`, source).Scan(&current)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if sequence != current {
		return errors.New("source ingress sequence is not gapless")
	}
	// Parse and compare against the actual durable clock even when no fence
 // has yet been published. No input/closure may enter before genesis.
 entry,parseErr:=ingressguard.Parse(record,source)
 if parseErr!=nil {
  // Keep raw atomic-store unit tests working only when the adapter has not
  // bootstrapped ANY temporal or fence authority. In an initialized run
  // malformed causal ingress must fail closed.
  var authorityCount int
  if err:=tx.QueryRowContext(ctx,"SELECT (SELECT count(*) FROM genesis_records)+(SELECT count(*) FROM clock_states)+(SELECT count(*) FROM decision_ledger)").Scan(&authorityCount);err!=nil{return err}
  if authorityCount!=0{return parseErr}
 }else{
  if err:=validateCausalIngress(ctx,tx,source,sequence,record,receipt,entry);err!=nil{return err}
  if err:=verifyIngressClockFloor(ctx,tx,entry.RunID,entry.Coordinate);err!=nil{return err}
 }
	// A second Ledger opened before closure/fence can hold a stale source
	// snapshot. Check the canonical eligibility against all durable fences in
	// the *same transaction* as record+receipt+cursor publication.
	seals, err := tx.QueryContext(ctx, "SELECT canonical FROM decision_ledger")
	if err != nil { return err }
	for seals.Next() {
		var fence []byte
		if err := seals.Scan(&fence); err != nil { seals.Close(); return err }
		fields,err:=codec.StrictDecodePayload(fence)
		if err!=nil {seals.Close();return err}
		body,ok:=fields.([]any)
		if !ok {seals.Close();return errors.New("invalid decision ledger record")}
		if len(body)==24 {continue} // A CycleCommit is not a fence.
		if len(body)!=18 && len(body)!=6 {seals.Close();return errors.New("unknown decision record kind")}
		sealedRun,cut,err:=ingressguard.FenceCoordinate(fence)
		if err!=nil {seals.Close();return err}
		if err:=ingressguard.Check(entry,sealedRun,cut);err!=nil {seals.Close();return err}
	}
	if err:=seals.Err();err!=nil {seals.Close();return err}
	if err:=seals.Close();err!=nil {return err}
	if err := appendTx(ctx, tx, record); err != nil {
		return err
	}
	receiptID := [32]byte(identity.OpaqueContentHash(receipt.Canonical))
	if err := appendTx(ctx, tx, ports.CanonicalRecord{Identity: receiptID, Canonical: receipt.Canonical}); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO source_ingress_receipts(source_id,ingress_seq,record_id,canonical) VALUES(?,?,?,?)`, source, sequence, record.Identity[:], receipt.Canonical); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO input_source_cursors(source_id,next_seq) VALUES(?,?) ON CONFLICT(source_id) DO UPDATE SET next_seq=excluded.next_seq`, source, sequence+1); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) NextIngressSequence(ctx context.Context, source string) (uint64, error) {
 if err:=verifyIngressProjections(ctx,s.db);err!=nil{return 0,err}
 var sequence uint64
	err := s.db.QueryRowContext(ctx, `SELECT next_seq FROM input_source_cursors WHERE source_id=?`, source).Scan(&sequence)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return sequence, err
}

func appendTx(ctx context.Context, tx *sql.Tx, record ports.CanonicalRecord) error {
	result, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO input_ledger(id,canonical) VALUES(?,?)`, record.Identity[:], record.Canonical)
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
	var existing []byte
	if err := tx.QueryRowContext(ctx, `SELECT canonical FROM input_ledger WHERE id=?`, record.Identity[:]).Scan(&existing); err != nil {
		return err
	}
	if !bytes.Equal(existing, record.Canonical) {
		return errors.New("input-ledger identity collision")
	}
	return nil
}

// AppendIfAbsent persists a response under the stable slot key in one SQLite
// transaction. Only one process can win the slot, regardless of response ID.
func (s *Store) AppendIfAbsent(ctx context.Context, key [32]byte, record ports.CanonicalRecord) (bool, []byte, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, nil, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO input_slot_responses(slot_key,record_id,canonical) VALUES(?,?,?)`, key[:], record.Identity[:], record.Canonical)
	if err != nil {
		return false, nil, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, nil, err
	}
	if affected == 0 {
		var existing []byte
		if err := tx.QueryRowContext(ctx, `SELECT canonical FROM input_slot_responses WHERE slot_key=?`, key[:]).Scan(&existing); err != nil {
			return false, nil, err
		}
		return false, existing, nil
	}
	result, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO input_ledger(id,canonical) VALUES(?,?)`, record.Identity[:], record.Canonical)
	if err != nil {
		return false, nil, err
	}
	affected, err = result.RowsAffected()
	if err != nil {
		return false, nil, err
	}
	if affected == 0 {
		var existing []byte
		if err := tx.QueryRowContext(ctx, `SELECT canonical FROM input_ledger WHERE id=?`, record.Identity[:]).Scan(&existing); err != nil {
			return false, nil, err
		}
		if !bytes.Equal(existing, record.Canonical) {
			return false, nil, errors.New("input-ledger identity collision")
		}
	}
	if err := tx.Commit(); err != nil {
		return false, nil, err
	}
	return true, nil, nil
}

// PersistGenesis checks all mutable fields against the canonical root before
// the first insert, and persists only the independently verified snapshot.
func (s *Store) PersistGenesis(ctx context.Context, manifest *genesis.Genesis, bundle *policy.Bundle) error {
	if manifest == nil || bundle == nil {
		return fmt.Errorf("genesis manifest and policy bundle are required")
	}
	verified, err := genesis.ValidateCanonical(manifest.CanonicalBytes, bundle)
	if err != nil {
		return fmt.Errorf("invalid persisted genesis: %w", err)
	}
	if manifest.RunID != verified.RunID || manifest.ID != verified.ID ||
		manifest.Digest != verified.Digest || manifest.WorldSeed != verified.WorldSeed ||
		manifest.InitialInstant != verified.InitialInstant ||
		!reflect.DeepEqual(manifest.Payload, verified.Payload) {
		return fmt.Errorf("genesis identity or payload does not match canonical bytes")
	}
	result, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO genesis_records(run_id, genesis_id, digest, canonical) VALUES(?,?,?,?)`, verified.RunID[:], verified.ID[:], verified.Digest[:], verified.CanonicalBytes)
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
	if err := s.db.QueryRowContext(ctx, `SELECT genesis_id,digest,canonical FROM genesis_records WHERE run_id=?`, verified.RunID[:]).Scan(&existingID, &existingDigest, &existingCanonical); err != nil {
		return err
	}
	if !bytes.Equal(existingID, verified.ID[:]) || !bytes.Equal(existingDigest, verified.Digest[:]) || !bytes.Equal(existingCanonical, verified.CanonicalBytes) {
		return errors.New("genesis identity collision or pin corruption")
	}
	return nil
}

func (s *Store) LoadGenesis(ctx context.Context, runID [32]byte) ([]byte, error) {
	var encoded []byte
	err := s.db.QueryRowContext(ctx, `SELECT canonical FROM genesis_records WHERE run_id=?`, runID[:]).Scan(&encoded)
	return encoded, err
}

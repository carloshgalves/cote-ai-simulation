package commit

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/clock"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/observatory"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
	_ "modernc.org/sqlite"
)

// A snapshot of EVERY application table, not just the five causal ledgers.
// Queries must not create observations, ACKs, snapshots, knowledge receipts,
// new runtime rows, or any other persistent content.
func o1WholeDatabaseDigest(t *testing.T, path string) [32]byte {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	names, err := db.Query("SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'")
	if err != nil {
		t.Fatal(err)
	}
	var tables []string
	for names.Next() {
		var name string
		if err := names.Scan(&name); err != nil {
			names.Close()
			t.Fatal(err)
		}
		tables = append(tables, name)
	}
	if err := names.Err(); err != nil {
		names.Close()
		t.Fatal(err)
	}
	if err := names.Close(); err != nil {
		t.Fatal(err)
	}
	sort.Strings(tables)
	h := sha256.New()
	frame := func(data []byte) {
		var length [8]byte
		binary.BigEndian.PutUint64(length[:], uint64(len(data)))
		_, _ = h.Write(length[:])
		_, _ = h.Write(data)
	}
	for _, table := range tables {
		frame([]byte(table))
		quoted := strings.ReplaceAll(table, "\"", "\"\"")
		rows, err := db.Query("SELECT * FROM \"" + quoted + "\" ORDER BY rowid")
		if err != nil {
			t.Fatalf("digest table %q: %v", table, err)
		}
		columns, err := rows.Columns()
		if err != nil {
			rows.Close()
			t.Fatal(err)
		}
		frame([]byte(fmt.Sprintf("cols=%d", len(columns))))
		for rows.Next() {
			values := make([]any, len(columns))
			dest := make([]any, len(columns))
			for i := range dest {
				dest[i] = &values[i]
			}
			if err := rows.Scan(dest...); err != nil {
				rows.Close()
				t.Fatal(err)
			}
			frame([]byte("row"))
			for _, item := range values {
				switch v := item.(type) {
				case nil:
					frame([]byte("null"))
				case []byte:
					frame([]byte("blob"))
					frame(v)
				case string:
					frame([]byte("text"))
					frame([]byte(v))
				case int64:
					frame([]byte("integer"))
					frame([]byte(fmt.Sprintf("%d", v)))
				case float64:
					frame([]byte("float"))
					frame([]byte(fmt.Sprintf("%.17g", v)))
				default:
					rows.Close()
					t.Fatalf("unexpected sqlite value type %T", item)
				}
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		if err := rows.Close(); err != nil {
			t.Fatal(err)
		}
	}
	var digest [32]byte
	copy(digest[:], h.Sum(nil))
	return digest
}

func TestCSFRV19O1PristineAndCommittedRunReadonly(t *testing.T) {
	ctx := context.Background()
	coordinator, store, ledger, run, bundle, path := testFixture(t)
	reader, err := sqlite.OpenObservatory(ctx, path, bundle)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	var _ observatory.Queries = reader
	before := o1WholeDatabaseDigest(t, path)
	initial, err := reader.ReadRun(ctx, [32]byte(run))
	if err != nil {
		t.Fatal("pristine run:", err)
	}
	exact, err := store.LoadClock(ctx, [32]byte(run))
	if err != nil {
		t.Fatal(err)
	}
	if initial.Run != [32]byte(run) || initial.Clock != exact || initial.Control.Status != observatory.IDLE || initial.Control.CycleID != nil || initial.Control.Attempt != nil {
		t.Fatalf("invalid first-run diagnostic: %+v", initial)
	}
	if initial.WorldHash == ([32]byte{}) {
		t.Fatal("missing authoritative genesis state hash")
	}
	if got := o1WholeDatabaseDigest(t, path); got != before {
		t.Fatal("pristine Observatory query wrote to a causal or epistemic table")
	}
	if _, err := coordinator.Advance(ctx, ledger, run); err != nil {
		t.Fatal(err)
	}
	advanced, err := reader.ReadRun(ctx, [32]byte(run))
	if err != nil {
		t.Fatal("committed run:", err)
	}
	updated, err := store.LoadClock(ctx, [32]byte(run))
	if err != nil {
		t.Fatal(err)
	}
	if advanced.Clock != updated || advanced.Clock.Revision != 1 || advanced.Control.Status != observatory.IDLE {
		t.Fatalf("committed clock/state mismatch: %+v", advanced)
	}
	baseline := o1WholeDatabaseDigest(t, path)
	for i := 0; i < 100; i++ {
		got, err := reader.ReadRun(ctx, [32]byte(run))
		if err != nil || got.Clock != advanced.Clock || got.Control.Status != observatory.IDLE || got.WorldHash != advanced.WorldHash {
			t.Fatalf("repeat %d changed result: %+v %v", i, got, err)
		}
	}
	if got := o1WholeDatabaseDigest(t, path); got != baseline {
		t.Fatal("100 Observatory queries modified persistent simulation authorities")
	}
}

func TestCSFRV19O1InFlightAbortAndExplicitRetry(t *testing.T) {
	ctx := context.Background()
	coordinator, _, _, fence, run, cycle, path := abortFixture(t)
	b, err := policy.Load(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := sqlite.OpenObservatory(ctx, path, b)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	before := o1WholeDatabaseDigest(t, path)
	inFlight, err := reader.ReadRun(ctx, [32]byte(run))
	if err != nil {
		t.Fatal("fenced run:", err)
	}
	if inFlight.Control.Status != observatory.ATTEMPT_IN_FLIGHT || inFlight.Control.CycleID == nil ||
		*inFlight.Control.CycleID != [32]byte(cycle) || inFlight.Control.Attempt == nil || *inFlight.Control.Attempt != 0 {
		t.Fatalf("in-flight diagnosis incomplete: %+v", inFlight.Control)
	}
	if got := o1WholeDatabaseDigest(t, path); got != before {
		t.Fatal("diagnostic mutated in-flight causal run")
	}
	aborted, err := coordinator.Abort(ctx, run, cycle, fence, 4, nil)
	if err != nil {
		t.Fatal(err)
	}
	halted, err := reader.ReadRun(ctx, [32]byte(run))
	if err != nil || halted.Control.Status != observatory.HALTED_ON_ABORT || halted.Control.NextAttempt != 1 {
		t.Fatalf("abort diagnostic: %+v %v", halted, err)
	}
	if _, err := coordinator.Retry(ctx, run, cycle, aborted, "o1.operator.retry"); err != nil {
		t.Fatal(err)
	}
	authorized, err := reader.ReadRun(ctx, [32]byte(run))
	if err != nil || authorized.Control.Status != observatory.RETRY_AUTHORIZED || authorized.Control.NextAttempt != 1 {
		t.Fatalf("authorized retry diagnostic: %+v %v", authorized, err)
	}
	if authorized.Clock != inFlight.Clock || authorized.WorldHash != inFlight.WorldHash {
		t.Fatal("abort/retry falsely changed clock or world hash")
	}
}

func TestCSFRV19O1RejectsForeignRunCorruptClockAndControl(t *testing.T) {
	ctx := context.Background()
	_, store, _, run, bundle, path := testFixture(t)
	reader, err := sqlite.OpenObservatory(ctx, path, bundle)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	other := [32]byte(run)
	other[0] ^= 0x85
	if _, err := reader.ReadRun(ctx, other); err == nil {
		t.Fatal("foreign run id read succeeded")
	}
	another := other
	another[1] ^= 0x41
	if _, err := reader.ReadRun(ctx, another); err == nil {
		t.Fatal("second foreign run accepted")
	}
	original, err := store.LoadClock(ctx, [32]byte(run))
	if err != nil {
		t.Fatal(err)
	}
	broken := original
	broken.Revision = domain.WorldRevision(uint64(original.Revision) + 13)
	bad, err := clock.Encode(broken)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := clock.Encode(original)
	if err != nil {
		t.Fatal(err)
	}
	admin, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	if _, err := admin.ExecContext(ctx, "UPDATE clock_states SET canonical=? WHERE run_id=?", bad, run[:]); err != nil {
		t.Fatal(err)
	}
	if _, err := reader.ReadRun(ctx, [32]byte(run)); err == nil {
		t.Fatal("self-consistent but forged clock was trusted")
	}
	if _, err := admin.ExecContext(ctx, "UPDATE clock_states SET canonical=? WHERE run_id=?", raw, run[:]); err != nil {
		t.Fatal(err)
	}
	if _, err := reader.ReadRun(ctx, [32]byte(run)); err != nil {
		t.Fatal("restored clock not readable:", err)
	}
	// Reject a syntactically valid but unauthenticated control projection.
	if _, err := admin.ExecContext(ctx, "INSERT INTO cycle_control_state(run_id,canonical) VALUES(?,?)",
		run[:], []byte{0x80}); err != nil {
		t.Fatal(err)
	}
	if _, err := reader.ReadRun(ctx, [32]byte(run)); err == nil {
		t.Fatal("corrupted control state was displayed")
	}
	// Preserve CBOR well-formedness while inventing an in-flight fence ref.
	// A parser-only Observatory would incorrectly present ATTEMPT_IN_FLIGHT.
	var invented [32]byte
	invented[0] = 0x85
	noRef := []any{uint64(0)}
	forged := []any{
		uint64(1),
		[]any{uint64(1), invented[:]},
		[]any{uint64(1), uint64(0)},
		[]any{uint64(1), []any{uint64(11), invented[:], invented[:]}},
		noRef, noRef, uint64(0),
	}
	forgedCanonical, err := codec.CanonicalPayload(forged)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admin.ExecContext(ctx,
		"UPDATE cycle_control_state SET canonical=? WHERE run_id=?",
		forgedCanonical, run[:]); err != nil {
		t.Fatal(err)
	}
	if _, err := reader.ReadRun(ctx, [32]byte(run)); err == nil {
		t.Fatal("valid CBOR with unauthenticated fence reference was displayed")
	}
	if _, err := admin.ExecContext(ctx,
		"DELETE FROM cycle_control_state WHERE run_id=?", run[:]); err != nil {
		t.Fatal(err)
	}
	if _, err := reader.ReadRun(ctx, [32]byte(run)); err != nil {
		t.Fatal("valid genesis unreadable after restoring control:", err)
	}
	// Forging a 32-byte world hash also must not become trustworthy merely
	// because it has the expected length and occupies a valid revision row.
	var genuineWorldHash []byte
	if err := admin.QueryRowContext(ctx,
		"SELECT state_hash FROM world_revisions WHERE revision=?",
		uint64(original.Revision)).Scan(&genuineWorldHash); err != nil {
		t.Fatal(err)
	}
	adversarialHash := bytes.Repeat([]byte{0x85}, 32)
	if bytes.Equal(genuineWorldHash, adversarialHash) {
		t.Fatal("mutated world hash accidentally equals authoritative hash")
	}
	if _, err := admin.ExecContext(ctx,
		"UPDATE world_revisions SET state_hash=? WHERE revision=?",
		adversarialHash, uint64(original.Revision)); err != nil {
		t.Fatal(err)
	}
	if _, err := reader.ReadRun(ctx, [32]byte(run)); err == nil {
		t.Fatal("forged world revision hash was presented as authoritative")
	}
	if _, err := admin.ExecContext(ctx,
		"UPDATE world_revisions SET state_hash=? WHERE revision=?",
		genuineWorldHash, uint64(original.Revision)); err != nil {
		t.Fatal(err)
	}
	if _, err := reader.ReadRun(ctx, [32]byte(run)); err != nil {
		t.Fatal("world revision not readable after restoration:", err)
	}
}

func TestCSFRV19O1RejectsAbsentSQLiteWithoutCreatingFiles(t *testing.T) {
	ctx := context.Background()
	b, err := policy.Load(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	path := t.TempDir() + "/absent.sqlite"
	if _, err := sqlite.OpenObservatory(ctx, path, b); err == nil {
		t.Fatal("missing sqlite was created by read-only opener")
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("read-only connection created database: %v", err)
	}
}

func TestCSFRV19O1PublicReaderExposesNoWriteMethods(t *testing.T) {
	// The package sqlite tests raw SQL denial; this test protects the public
	// Reader method set against accidental exposure of write-capable methods.
	_, _, _, run, bundle, path := testFixture(t)
	reader, err := sqlite.OpenObservatory(context.Background(), path, bundle)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	_, err = reader.ReadRun(context.Background(), [32]byte(run))
	if err != nil {
		t.Fatal(err)
	}
	methods := reflect.TypeOf(reader)
	if methods.NumMethod() != 2 || methods.Method(0).Name != "Close" || methods.Method(1).Name != "ReadRun" {
		t.Fatalf("public Observatory Reader exposes unapproved method: %s", methods.String())
	}
}

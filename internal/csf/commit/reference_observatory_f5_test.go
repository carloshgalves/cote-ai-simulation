//go:build linux

package commit

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/clock"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/observatory"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/operationalmetrics"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

// This aggregate exists ONLY in the privileged test oracle. It is never a
// public or actor-facing DTO, and is not serialized into the CI certificate.
type f5Views struct {
	State       observatory.RunState
	Audit       observatory.PrivilegedReport
	Operational observatory.OperationalReport
	A, B        observatory.POVView
	Public      observatory.PublicView
}
type f5Readers struct {
	run   [32]byte
	state *sqlite.ObservatoryReader
	audit *sqlite.PrivilegedReader
	a, b  *sqlite.POVReader
	pub   *sqlite.PublicReader
}

func f5OpenReaders(t *testing.T, path string, run [32]byte, bundle *policy.Bundle) f5Readers {
	t.Helper()
	ctx := context.Background()
	db, err := openReferenceReadOnlyDB(path)
	if err != nil {
		t.Fatal(err)
	}
	dispatches, err := referenceD3ActorDispatches(ctx, db, run)
	if closeErr := db.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	if err != nil {
		t.Fatal(err)
	}
	out := f5Readers{run: run}
	out.state, err = sqlite.OpenObservatory(ctx, path, bundle)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { out.state.Close() })
	out.audit, err = sqlite.OpenPrivilegedObservatory(ctx, path, bundle)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { out.audit.Close() })
	out.a, err = sqlite.OpenPOVObservatory(ctx, path, bundle, run, dispatches[0], [2]string{"school", "a"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { out.a.Close() })
	out.b, err = sqlite.OpenPOVObservatory(ctx, path, bundle, run, dispatches[1], [2]string{"school", "b"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { out.b.Close() })
	out.pub, err = sqlite.OpenPublicObservatory(ctx, path, bundle)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { out.pub.Close() })
	return out
}

func (r f5Readers) read(t *testing.T) f5Views {
	t.Helper()
	ctx := context.Background()
	var v f5Views
	var err error
	if v.State, err = r.state.ReadRun(ctx, r.run); err != nil {
		t.Fatal("F5 run failed", err)
	}
	if v.Audit, err = r.audit.ReadPrivilegedReport(ctx, r.run); err != nil {
		t.Fatal("F5 provenance failed", err)
	}
	if v.Operational, err = r.audit.ReadOperationalReport(ctx, r.run); err != nil {
		t.Fatal("F5 operation failed", err)
	}
	if v.A, err = r.a.ReadPOV(ctx); err != nil {
		t.Fatal(err)
	}
	if v.B, err = r.b.ReadPOV(ctx); err != nil {
		t.Fatal(err)
	}
	if v.Public, err = r.pub.ReadPublic(ctx, r.run); err != nil {
		t.Fatal(err)
	}
	if v.State.Run != r.run || !reflect.DeepEqual(v.State, v.Audit.State) ||
		!reflect.DeepEqual(v.State, v.Operational.State) ||
		v.State.Clock.Revision != 9 || v.State.Control.Status != observatory.IDLE ||
		len(v.Audit.Commits) != 9 || len(v.Audit.Attempts) != 10 ||
		v.Audit.PendingUnits != 0 || v.Audit.PendingPerceptionTasks != 0 ||
		v.Operational.NextPendingEligibility != nil || len(v.Operational.Pending) != 0 ||
		v.Operational.PendingPerceptionTasks != 0 || len(v.Operational.Sources) != 2 ||
		v.Operational.RejectedIngress != "NOT_PERSISTED" ||
		v.A.Run != r.run || v.B.Run != r.run ||
		v.A.Actor != [2]string{"school", "a"} || v.B.Actor != [2]string{"school", "b"} ||
		v.A.ThroughRevision != 6 || v.B.ThroughRevision != 6 ||
		len(v.A.Entries) == 0 || len(v.B.Entries) == 0 ||
		v.Public.Run != r.run || v.Public.PublicationStatus != observatory.NoPublicRoute ||
		len(v.Public.Publications) != 0 {
		t.Fatal("F5 incomplete or incoherent four-view terminal projection")
	}
	for _, source := range v.Operational.Sources {
		if !source.Final || source.Coverage != "FINAL" || source.LatestClosure == nil {
			t.Fatal("F5 final closure omitted")
		}
	}
	return v
}

func (r f5Readers) denied(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	if got, err := r.state.ReadRun(ctx, r.run); err == nil || !reflect.DeepEqual(got, observatory.RunState{}) {
		t.Fatal("corrupt WAL produced run-state")
	}
	if got, err := r.audit.ReadPrivilegedReport(ctx, r.run); err == nil || !reflect.DeepEqual(got, observatory.PrivilegedReport{}) {
		t.Fatal("corrupt WAL produced privileged report")
	}
	if got, err := r.audit.ReadOperationalReport(ctx, r.run); !errors.Is(err, observatory.ErrInvalidReplay) || !reflect.DeepEqual(got, observatory.OperationalReport{}) {
		t.Fatal("corrupt WAL produced operational report or wrong diagnosis", err)
	}
	for _, actor := range []*sqlite.POVReader{r.a, r.b} {
		if got, err := actor.ReadPOV(ctx); err != observatory.ErrViewUnavailable || !reflect.DeepEqual(got, observatory.POVView{}) {
			t.Fatal("corrupt WAL leaked restricted view")
		}
	}
	if got, err := r.pub.ReadPublic(ctx, r.run); err != observatory.ErrViewUnavailable || !reflect.DeepEqual(got, observatory.PublicView{}) {
		t.Fatal("corrupt WAL leaked public view")
	}
}

// Non-vacuous final acceptance: independent true D3 invocations with the
// SAME authentic supervised signed cuts, E5 scheduling traces, and genuine
// atomic failure/recovery. Compare every projected field and literal E1
// rows; changing slow/fast host metrics must not change any projection.
func TestReferenceF5EquivalentIntegratedViews(t *testing.T) {
	ctx := context.Background()
	host, plans, cuts := referenceE2SignedPair(t)
	path, owner, proof := referenceE5Run(t, host, plans, &cuts, nil, 0, 0, nil)
	baseline := referenceE2Capture(t, path, owner)
	run := referenceC2Decode32(t, proof.RunID)
	var expected f5Views

	for i, spec := range []struct {
		name    string
		reverse bool
		epoch   int
		step    uint8
	}{
		{"normal", false, 0, 0},
		{"reverse", true, 0, 0},
		{"recovered", false, 2, 7},
	} {
		t.Run(spec.name, func(t *testing.T) {
			dbPath, ownerPath := path, owner
			if i != 0 {
				order := &referenceOrder{seed: 999, reverse: spec.reverse, trace: map[string][]int{}}
				var actual ReferenceD3Proof
				dbPath, ownerPath, actual = referenceE5Run(t, host, plans, &cuts, order, spec.epoch, spec.step, &proof)
				if actual.RunID != proof.RunID || actual.Matrix.ReplayCommits != 9 {
					t.Fatal("F5 perturbed run is not equivalent D3")
				}
				for _, phase := range []string{"ingress-declarations", "worker-responses", "abort-validator-inputs", "contest-validator-inputs", "abort-validator-workers", "contest-validator-workers", "candidate-completions", "validator-assessments"} {
					got := order.trace[phase]
					if len(got) != 2 || got[0]+got[1] != 1 || got[0] == got[1] ||
						(spec.reverse && !reflect.DeepEqual(got, []int{1, 0})) {
						t.Fatal("F5 missing realized scheduling perturbation", phase)
					}
				}
			}
			before := referenceE2Capture(t, dbPath, ownerPath)
			if err := referenceE1Compare(baseline, before); err != nil {
				t.Fatal("F5 equivalent run changed authority", err)
			}
			anchor, err := sqlite.Open(ctx, dbPath)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { anchor.Close() })
			sidecar := filepath.Join(filepath.Dir(dbPath), "metrics.db")
			rec, err := operationalmetrics.OpenRecorder(ctx, dbPath, sidecar)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { rec.Close() })
			first := operationalmetrics.Sample{ID: [32]byte{1}, Run: run, Stage: "replay",
				HostID: "host-" + spec.name, WorkerID: "worker-" + spec.name,
				ObservedUnixNanos: int64(100+i), DurationNanos: int64(1000 * (i + 1)), WorkerCalls: int64(i + 1)}
			if err := rec.Record(ctx, first); err != nil {
				t.Fatal(err)
			}
			reader := f5OpenReaders(t, dbPath, run, host.Bundle)
			durable := f4DurableFiles(t, dbPath, ownerPath, ownerPath+".actor-a")
			v := reader.read(t)
			if i == 0 {
				expected = v
			} else if !reflect.DeepEqual(expected, v) {
				t.Fatal("F5 equivalent same-source run changed complete projection")
			}
			second := first
			second.ID = [32]byte{2}
			second.ObservedUnixNanos++
			second.DurationNanos += 1000000
			second.TransportRetries++
			if err := rec.Record(ctx, second); err != nil {
				t.Fatal(err)
			}
			metrics, err := operationalmetrics.OpenReader(ctx, dbPath, sidecar)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { metrics.Close() })
			ops, err := metrics.Read(ctx, run, 10)
			if err != nil || !reflect.DeepEqual(ops, []operationalmetrics.Sample{first, second}) {
				t.Fatal("F5 metrics perturbation not observed", err)
			}
			if after := reader.read(t); !reflect.DeepEqual(v, after) {
				t.Fatal("F5 changed host metrics influenced causal projection")
			}
			if err := referenceE1Compare(before, referenceE2Capture(t, dbPath, ownerPath)); err != nil {
				t.Fatal("F5 queries/metrics changed literal E1 authority", err)
			}
			if !reflect.DeepEqual(durable, f4DurableFiles(t, dbPath, ownerPath, ownerPath+".actor-a")) {
				t.Fatal("F5 queries/metrics changed durable file bytes")
			}
			t.Log("F5 equivalent D3: nine commits, ten attempts, six read surfaces, unchanged authorities and durable bytes")
		})
	}
}

// Keep a genuine WAL transaction uncheckpointed. Its changed source cursor
// is invisible in the main file but visible to live readers. If an opener
// used immutable=1, the corrupt live source could incorrectly look valid.
func TestReferenceF5LiveWALFailClosed(t *testing.T) {
	ctx := context.Background()
	host, plans, cuts := referenceE2SignedPair(t)
	path, owner, proof := referenceE5Run(t, host, plans, &cuts, nil, 0, 0, nil)
	run := referenceC2Decode32(t, proof.RunID)
	admin, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	admin.SetMaxOpenConns(1)
	if _, err := admin.ExecContext(ctx, "PRAGMA wal_autocheckpoint=0"); err != nil {
		t.Fatal(err)
	}
	var busy, frames, done int
	if err := admin.QueryRowContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)").Scan(&busy, &frames, &done); err != nil || busy != 0 {
		t.Fatal("F5 could not establish clean main-file baseline", err)
	}
	before := referenceE2Capture(t, path, owner)
	reader := f5OpenReaders(t, path, run, host.Bundle)
	expected := reader.read(t)
	main, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var cursor uint64
	if err := admin.QueryRowContext(ctx, "SELECT next_seq FROM input_source_cursors WHERE source_id='physical.a1'").Scan(&cursor); err != nil || cursor == 0 {
		t.Fatal("missing real physical ingress cursor", err)
	}
	result, err := admin.ExecContext(ctx, "UPDATE input_source_cursors SET next_seq=next_seq+1 WHERE source_id='physical.a1'")
	if err != nil {
		t.Fatal(err)
	}
	if n, err := result.RowsAffected(); err != nil || n != 1 {
		t.Fatal("F5 WAL mutation missed actual authority")
	}
	wal, err := os.ReadFile(path + "-wal")
	if err != nil || len(wal) <= 32 {
		t.Fatal("F5 lacks actual uncheckpointed WAL frames", err)
	}
	stillMain, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(main, stillMain) {
		t.Fatal("F5 corrupt transaction was checkpointed into main file", err)
	}
	damaged := referenceE2Capture(t, path, owner)
	if referenceE1Compare(before, damaged) == nil {
		t.Fatal("F5 WAL mutation did not change source transcript")
	}
	durable := f4DurableFiles(t, path, owner, owner+".actor-a")
	reader.denied(t)
	cold, err := sqlite.OpenPublicObservatory(ctx, path, host.Bundle)
	if err != nil {
		t.Fatal(err)
	}
	if v, err := cold.ReadPublic(ctx, run); err != observatory.ErrViewUnavailable || !reflect.DeepEqual(v, observatory.PublicView{}) {
		cold.Close()
		t.Fatal("cold public query ignored uncheckpointed corrupt WAL")
	}
	if err := cold.Close(); err != nil {
		t.Fatal(err)
	}
	if err := referenceE1Compare(damaged, referenceE2Capture(t, path, owner)); err != nil ||
		!reflect.DeepEqual(durable, f4DurableFiles(t, path, owner, owner+".actor-a")) {
		t.Fatal("F5 denial repaired or mutated corrupt authority", err)
	}
	if _, err := admin.ExecContext(ctx, "UPDATE input_source_cursors SET next_seq=? WHERE source_id='physical.a1'", cursor); err != nil {
		t.Fatal(err)
	}
	if restored := reader.read(t); !reflect.DeepEqual(expected, restored) {
		t.Fatal("F5 original valid projection not recovered")
	}
	if err := referenceE1Compare(before, referenceE2Capture(t, path, owner)); err != nil {
		t.Fatal("F5 restored canonical rows differ", err)
	}
	t.Log("F5 live WAL: main file stayed unchanged, all six queries denied altered cursor, restored exact authority")
}

// Parent #120 requires an allowlist for EACH projection, not only the
// restricted DTOs. Pin nested human-only field rosters alongside F3.
func TestReferenceF5HumanAllowlists(t *testing.T) {
	specs := []struct {
		v     any
		names string
	}{
		{observatory.RunState{}, "Run Clock WorldHash Control"},
		{clock.State{}, "Instant Revision NextLogicalSequence LastOrdinal"},
		{observatory.CycleControl{}, "Status CycleID Attempt NextAttempt"},
		{observatory.PrivilegedReport{}, "State Genesis Commits Attempts PendingUnits PendingClosureSources PendingPerceptionTasks RejectedIngress"},
		{observatory.GenesisAuthority{}, "Run GenesisID Digest ContentHash"},
		{observatory.CausalCommit{}, "CycleID Instant Ordinal Attempt Plan BaseRevision ResultRevision BaseWorldHash ResultWorldHash InputDigest DecisionDigest Fence Commit Closures Admitted Decisions DecisionLinks PerceptionLinks Events Tasks Completions Evidence Revision"},
		{observatory.CausalAttempt{}, "CycleID Instant Ordinal Attempt Fence Closures Admitted Commit Abort Retry AbortReason"},
		{observatory.DecisionLink{}, "Decision Source SourceDigest DecisionDigest Status Candidate Assessment Conflicts Draws ProducedEvents"},
		{observatory.PerceptionLink{}, "Event Task Completion Observations KnowledgeInputs"},
		{observatory.AuthorityRef{}, "Ledger ID ContentHash"},
		{observatory.RevisionAuthority{}, "Ledger Revision ContentHash"},
		{observatory.OperationalReport{}, "State NextPendingEligibility Sources Pending PendingPerceptionTasks RejectedIngress"},
		{observatory.SourceDiagnostic{}, "Source LatestClosure ClosedThrough Final Coverage"},
		{observatory.PendingDiagnostic{}, "Authority Kind Eligibility"},
		{domain.EligibilityCoordinate{}, "NotBeforeInstant NotBeforeCycleOrdinal"},
	}
	registered := map[reflect.Type]bool{}
	for _, spec := range specs {
		registered[reflect.TypeOf(spec.v)] = true
	}
	var safe func(reflect.Type) bool
	safe = func(typ reflect.Type) bool {
		switch typ.Kind() {
		case reflect.Pointer:
			return safe(typ.Elem())
		case reflect.Slice:
			return typ.Elem().Kind() != reflect.Uint8 && safe(typ.Elem())
		case reflect.Array:
			return typ == reflect.TypeOf([32]byte{})
		case reflect.Struct:
			return registered[typ]
		case reflect.String, reflect.Bool, reflect.Int, reflect.Int64, reflect.Uint64:
			return true
		default:
			return false
		}
	}
	for _, spec := range specs {
		typ := reflect.TypeOf(spec.v)
		names := make([]string, typ.NumField())
		for i := range names {
			field := typ.Field(i)
			names[i] = field.Name
			if !safe(field.Type) {
				t.Fatal("human projection gained arbitrary or raw payload", typ.Name(), field.Name)
			}
		}
		if strings.Join(names, " ") != spec.names {
			t.Fatal("human projection field roster changed", typ.Name())
		}
	}
}

func TestReferenceF5OracleIncludesNearReservedNames(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "oracle.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.ExecContext(ctx, "CREATE TABLE actual_authority (n INTEGER); INSERT INTO actual_authority VALUES (1)"); err != nil {
		t.Fatal(err)
	}
	locations := []referenceE1Location{{Name: "causal", Path: path}}
	before, err := referenceE1Read(ctx, locations)
	if err != nil {
		t.Fatal(err)
	}
	// SQL LIKE 'sqlite_%' treats underscore as a wildcard. This legal
	// non-internal table used to vanish from both the schema and row oracle.
	if _, err := db.ExecContext(ctx, "CREATE TABLE sqliteX_authority (n INTEGER); INSERT INTO sqliteX_authority VALUES (2)"); err != nil {
		t.Fatal(err)
	}
	after, err := referenceE1Read(ctx, locations)
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Databases["causal"].Tables["sqliteX_authority"].Rows) != 1 || referenceE1Compare(before, after) == nil {
		t.Fatal("E1 oracle ignored a legal non-internal SQLite table")
	}
}

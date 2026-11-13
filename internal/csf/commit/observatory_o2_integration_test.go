package commit

import (
	"bytes"
	"context"
	"database/sql"
	"reflect"
	"strings"
	"testing"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/inputledger"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/observatory"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
	_ "modernc.org/sqlite"
)

func TestCSFRV19O2CommittedCausalChainAndNoSideEffects(t *testing.T) {
	ctx := context.Background()
	coordinator, _, ledger, run, bundle, path := testFixture(t)
	outcome, err := coordinator.Advance(ctx, ledger, run)
	if err != nil {
		t.Fatal(err)
	}
	audit, err := sqlite.OpenPrivilegedObservatory(ctx, path, bundle)
	if err != nil {
		t.Fatal(err)
	}
	defer audit.Close()
	baseline := o1WholeDatabaseDigest(t, path)
	report, err := audit.ReadPrivilegedReport(ctx, [32]byte(run))
	if err != nil {
		t.Fatal(err)
	}
	if report.State.Run != [32]byte(run) || report.State.Clock.Revision != 1 || report.State.Control.Status != observatory.IDLE {
		t.Fatalf("incorrect run/control: %+v", report.State)
	}
	if len(report.Commits) != 1 || len(report.Attempts) != 1 {
		t.Fatalf("expected exactly one committed cycle and its fence: commits=%d attempts=%d", len(report.Commits), len(report.Attempts))
	}
	commit := report.Commits[0]
	if commit.CycleID != [32]byte(outcome.Fence.Fields[1].([]byte)) {
		t.Fatal("commit cycle identity differs from fence")
	}
	if commit.Commit.ID != outcome.Commit.Identity || commit.Fence.ID != [32]byte(outcome.Fence.ID) ||
		commit.Commit.Ledger != "decision_ledger" || commit.Fence.Ledger != "decision_ledger" ||
		commit.BaseRevision != 0 || commit.ResultRevision != 1 || len(commit.Events) != len(outcome.Events) {
		t.Fatalf("causal chain incomplete: %+v", commit)
	}
	for i, ev := range commit.Events {
		if ev.Ledger != "event_store" || ev.ID != outcome.Events[i].Identity || ev.ContentHash == ([32]byte{}) {
			t.Fatalf("event[%d] is not authenticated: %+v", i, ev)
		}
	}
	if commit.InputDigest == ([32]byte{}) || commit.DecisionDigest == ([32]byte{}) || commit.Fence.ContentHash == ([32]byte{}) {
		t.Fatal("missing causal roots or hashes")
	}
	if report.Attempts[0].Commit == nil || report.Attempts[0].Commit.ID != outcome.Commit.Identity || report.Attempts[0].Abort != nil {
		t.Fatal("committed attempt reconstructed incorrectly")
	}
	for i := 0; i < 40; i++ {
		again, err := audit.ReadPrivilegedReport(ctx, [32]byte(run))
		if err != nil || !reflect.DeepEqual(report, again) {
			t.Fatalf("read %d was not deterministic: %v", i, err)
		}
	}
	if o1WholeDatabaseDigest(t, path) != baseline {
		t.Fatal("report query changed a causal or epistemic table")
	}
}

func TestCSFRV19O2AbortRetryLifecycleWithoutWorldMutation(t *testing.T) {
	ctx := context.Background()
	c, _, _, fence, run, cycle, path := abortFixture(t)
	bundle, err := policy.Load(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	q, err := sqlite.OpenPrivilegedObservatory(ctx, path, bundle)
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close()
	before := o1WholeDatabaseDigest(t, path)
	live, err := q.ReadPrivilegedReport(ctx, [32]byte(run))
	if err != nil {
		t.Fatal(err)
	}
	if live.State.Control.Status != observatory.ATTEMPT_IN_FLIGHT || len(live.Commits) != 1 || len(live.Attempts) != 2 {
		t.Fatalf("fenced attempt missing from report: %+v", live)
	}
	active := live.Attempts[len(live.Attempts)-1]
	if len(active.Closures) == 0 || len(active.Admitted) == 0 {
		t.Fatalf("in-flight fence lost verified closure/admitted provenance: %+v", active)
	}
	for _, ref := range active.Closures {
		if ref.Ledger != "input_ledger" || ref.ContentHash == ([32]byte{}) {
			t.Fatal("closure proof lacks ledger authority")
		}
	}
	for _, ref := range active.Admitted {
		if ref.Ledger != "scheduled_occurrences" && ref.Ledger != "input_ledger" && ref.Ledger != "trigger_activations" {
			t.Fatal("unrecognized admitted source ledger")
		}
	}
	if got := o1WholeDatabaseDigest(t, path); got != before {
		t.Fatal("in-flight query mutated durable state")
	}
	abort, err := c.Abort(ctx, run, cycle, fence, 4, nil)
	if err != nil {
		t.Fatal(err)
	}
	halted, err := q.ReadPrivilegedReport(ctx, [32]byte(run))
	if err != nil {
		t.Fatal(err)
	}
	last := halted.Attempts[len(halted.Attempts)-1]
	if halted.State.Control.Status != observatory.HALTED_ON_ABORT || last.Abort == nil || last.Abort.ID != abort.Identity || last.AbortReason == nil || *last.AbortReason != 4 || last.Retry != nil {
		t.Fatalf("abort not bound to original fence: %+v", last)
	}
	retry, err := c.Retry(ctx, run, cycle, abort, "o2.operator.retry")
	if err != nil {
		t.Fatal(err)
	}
	authorized, err := q.ReadPrivilegedReport(ctx, [32]byte(run))
	if err != nil {
		t.Fatal(err)
	}
	last = authorized.Attempts[len(authorized.Attempts)-1]
	if authorized.State.Control.Status != observatory.RETRY_AUTHORIZED || last.Retry == nil || last.Retry.ID != retry.Identity ||
		last.Abort == nil || last.Abort.ID != abort.Identity || last.Fence.ID != [32]byte(fence.ID) {
		t.Fatalf("retry linkage incomplete: %+v", last)
	}
	if authorized.State.Clock != live.State.Clock || authorized.State.WorldHash != live.State.WorldHash {
		t.Fatal("abort/retry fabricated a world revision")
	}
	stable := o1WholeDatabaseDigest(t, path)
	for i := 0; i < 20; i++ {
		next, err := q.ReadPrivilegedReport(ctx, [32]byte(run))
		if err != nil || !reflect.DeepEqual(authorized, next) {
			t.Fatalf("retry report unstable at %d: %v", i, err)
		}
	}
	if o1WholeDatabaseDigest(t, path) != stable {
		t.Fatal("retry report read mutated persistent state")
	}
}

func TestCSFRV19O2RejectsTamperedCycleAndEventAuthorities(t *testing.T) {
	ctx := context.Background()
	coordinator, _, ledger, run, bundle, path := testFixture(t)
	outcome, err := coordinator.Advance(ctx, ledger, run)
	if err != nil {
		t.Fatal(err)
	}
	q, err := sqlite.OpenPrivilegedObservatory(ctx, path, bundle)
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close()
	admin, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	if _, err := q.ReadPrivilegedReport(ctx, [32]byte(run)); err != nil {
		t.Fatal(err)
	}
	mutations := []struct {
		table    string
		id       [32]byte
		original []byte
	}{
		{"decision_ledger", outcome.Commit.Identity, outcome.Commit.Canonical},
		{"event_store", outcome.Events[0].Identity, outcome.Events[0].Canonical},
	}
	for _, c := range mutations {
		t.Run(c.table, func(t *testing.T) {
			if _, err := admin.ExecContext(ctx, "UPDATE "+c.table+" SET canonical=? WHERE id=?", []byte{0x80}, c.id[:]); err != nil {
				t.Fatal(err)
			}
			if _, err := q.ReadPrivilegedReport(ctx, [32]byte(run)); err == nil {
				t.Fatal("report accepted tampered authoritative record")
			}
			if _, err := admin.ExecContext(ctx, "UPDATE "+c.table+" SET canonical=? WHERE id=?", c.original, c.id[:]); err != nil {
				t.Fatal(err)
			}
			if _, err := q.ReadPrivilegedReport(ctx, [32]byte(run)); err != nil {
				t.Fatalf("restoration failed: %v", err)
			}
		})
	}
	foreign := [32]byte(run)
	foreign[0] ^= 0x86
	if _, err := q.ReadPrivilegedReport(ctx, foreign); err == nil {
		t.Fatal("foreign run reported")
	}
}

func TestCSFRV19O2RejectsForgedAbortAndRetryWithoutLeakingRows(t *testing.T) {
	ctx := context.Background()
	c, _, _, fence, run, cycle, path := abortFixture(t)
	bundle, err := policy.Load(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	abort, err := c.Abort(ctx, run, cycle, fence, 4, nil)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := c.Retry(ctx, run, cycle, abort, "o2.operator.retry")
	if err != nil {
		t.Fatal(err)
	}
	q, err := sqlite.OpenPrivilegedObservatory(ctx, path, bundle)
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close()
	admin, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	for _, item := range []struct {
		id   [32]byte
		raw  []byte
		name string
	}{
		{abort.Identity, abort.Canonical, "abort"}, {retry.Identity, retry.Canonical, "retry"},
	} {
		t.Run(item.name, func(t *testing.T) {
			forged := bytes.Clone(item.raw)
			forged[len(forged)-1] ^= 0x01
			if _, err := admin.ExecContext(ctx, "UPDATE decision_ledger SET canonical=? WHERE id=?", forged, item.id[:]); err != nil {
				t.Fatal(err)
			}
			got, err := q.ReadPrivilegedReport(ctx, [32]byte(run))
			if err == nil {
				t.Fatalf("forged %s was included as valid: %+v", item.name, got)
			}
			if !strings.Contains(strings.ToLower(err.Error()), item.name) && !strings.Contains(strings.ToLower(err.Error()), "cycle") {
				t.Fatalf("unlocalized %s corruption: %v", item.name, err)
			}
			if _, err := admin.ExecContext(ctx, "UPDATE decision_ledger SET canonical=? WHERE id=?", item.raw, item.id[:]); err != nil {
				t.Fatal(err)
			}
			if _, err := q.ReadPrivilegedReport(ctx, [32]byte(run)); err != nil {
				t.Fatalf("restored %s failed: %v", item.name, err)
			}
		})
	}
}

func TestCSFRV19O2RejectsOrphanTerminalJournal(t *testing.T) {
	ctx := context.Background()
	c, _, ledger, run, bundle, path := testFixture(t)
	outcome, err := c.Advance(ctx, ledger, run)
	if err != nil {
		t.Fatal(err)
	}
	q, err := sqlite.OpenPrivilegedObservatory(ctx, path, bundle)
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := q.ReadPrivilegedReport(ctx, [32]byte(run)); err != nil {
		t.Fatal(err)
	}
	invented := outcome.Commit.Identity
	invented[0] ^= 0x86
	if _, err := db.ExecContext(ctx, "INSERT INTO decision_ledger(id,canonical) VALUES(?,?)",
		invented[:], outcome.Commit.Canonical); err != nil {
		t.Fatal(err)
	}
	if got, err := q.ReadPrivilegedReport(ctx, [32]byte(run)); err == nil {
		t.Fatalf("orphan CycleCommit without committed_cycles membership was accepted: %+v", got)
	}
	if _, err := db.ExecContext(ctx, "DELETE FROM decision_ledger WHERE id=?", invented[:]); err != nil {
		t.Fatal(err)
	}
	if _, err := q.ReadPrivilegedReport(ctx, [32]byte(run)); err != nil {
		t.Fatal("restored report:", err)
	}
}

func TestCSFRV19O2ReportsExplicitSourceClosureGap(t *testing.T) {
	ctx := context.Background()
	_, _, ledger, run, bundle, path := testFixture(t)
	if err := ledger.DeclareSource(inputledger.Source{ID: "unclosed"}); err != nil {
		t.Fatal(err)
	}
	q, err := sqlite.OpenPrivilegedObservatory(ctx, path, bundle)
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close()
	before := o1WholeDatabaseDigest(t, path)
	report, err := q.ReadPrivilegedReport(ctx, [32]byte(run))
	if err != nil {
		t.Fatal(err)
	}
	if len(report.PendingClosureSources) != 1 || report.PendingClosureSources[0] != "unclosed" {
		t.Fatalf("unclosed declared source was not diagnosed: %+v", report.PendingClosureSources)
	}
	if got := o1WholeDatabaseDigest(t, path); got != before {
		t.Fatal("closure-gaps query wrote to ledger")
	}
}

func TestCSFRV19O2ReportsPendingPerceptionTasks(t *testing.T) {
	ctx := context.Background()
	c, _, ledger, run, bundle, path := testFixture(t, activeTrigger())
	if _, err := c.Advance(ctx, ledger, run); err != nil {
		t.Fatal(err)
	}
	q, err := sqlite.OpenPrivilegedObservatory(ctx, path, bundle)
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close()
	before := o1WholeDatabaseDigest(t, path)
	report, err := q.ReadPrivilegedReport(ctx, [32]byte(run))
	if err != nil {
		t.Fatal(err)
	}
	if report.PendingPerceptionTasks < 1 || report.RejectedIngress != "NOT_PERSISTED" {
		t.Fatalf("pending work was misclassified as complete or fabricated rejected ingress: %+v", report)
	}
	if len(report.Commits) != 1 || len(report.Commits[0].Tasks) < 1 {
		t.Fatal("commit did not expose verified perception task refs")
	}
	if got := o1WholeDatabaseDigest(t, path); got != before {
		t.Fatal("pending-task query wrote to ledger")
	}
}

func TestCSFRV19O2RejectsForeignFenceAuthority(t *testing.T) {
	ctx := context.Background()
	c, _, ledger, run, bundle, path := testFixture(t)
	result, err := c.Advance(ctx, ledger, run)
	if err != nil {
		t.Fatal(err)
	}
	q, err := sqlite.OpenPrivilegedObservatory(ctx, path, bundle)
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close()
	admin, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	decoded, err := codec.StrictDecodePayload(result.Fence.Canonical)
	if err != nil {
		t.Fatal(err)
	}
	fields := decoded.([]any)
	foreign := [32]byte(run)
	foreign[0] ^= 0x86
	fields[0] = foreign[:]
	forged, err := codec.CanonicalPayload(fields)
	if err != nil {
		t.Fatal(err)
	}
	id := [32]byte(result.Fence.ID)
	id[0] ^= 0x86
	if _, err := admin.ExecContext(ctx, "INSERT INTO decision_ledger(id,canonical) VALUES(?,?)", id[:], forged); err != nil {
		t.Fatal(err)
	}
	if report, err := q.ReadPrivilegedReport(ctx, [32]byte(run)); err == nil {
		t.Fatalf("foreign-run fence in single-world ledger was ignored: %+v", report)
	}
	if _, err := admin.ExecContext(ctx, "DELETE FROM decision_ledger WHERE id=?", id[:]); err != nil {
		t.Fatal(err)
	}
	if _, err := q.ReadPrivilegedReport(ctx, [32]byte(run)); err != nil {
		t.Fatal("valid restored report:", err)
	}
}

func TestCSFRV19O2RejectsFalseIdleAfterUnrepairedAbort(t *testing.T) {
	ctx := context.Background()
	c, _, _, fence, run, cycle, path := abortFixture(t)
	if _, err := c.Abort(ctx, run, cycle, fence, 4, nil); err != nil {
		t.Fatal(err)
	}
	bundle, err := policy.Load(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	q, err := sqlite.OpenPrivilegedObservatory(ctx, path, bundle)
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close()
	if report, err := q.ReadPrivilegedReport(ctx, [32]byte(run)); err != nil || report.State.Control.Status != observatory.HALTED_ON_ABORT {
		t.Fatalf("real halted run not correctly audited: %+v %v", report.State.Control, err)
	}
	admin, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	var original []byte
	if err := admin.QueryRowContext(ctx, "SELECT canonical FROM cycle_control_state WHERE run_id=?", run[:]).Scan(&original); err != nil {
		t.Fatal(err)
	}
	absent := []any{uint64(0)}
	falselyIdle, err := codec.CanonicalPayload([]any{uint64(0), absent, absent, absent, absent, absent, uint64(0)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admin.ExecContext(ctx, "UPDATE cycle_control_state SET canonical=? WHERE run_id=?", falselyIdle, run[:]); err != nil {
		t.Fatal(err)
	}
	if report, err := q.ReadPrivilegedReport(ctx, [32]byte(run)); err == nil {
		t.Fatalf("orphan abort without explicit retry was misreported as IDLE: %+v", report)
	}
	if _, err := admin.ExecContext(ctx, "UPDATE cycle_control_state SET canonical=? WHERE run_id=?", original, run[:]); err != nil {
		t.Fatal(err)
	}
	if _, err := q.ReadPrivilegedReport(ctx, [32]byte(run)); err != nil {
		t.Fatal("restored halt not reportable:", err)
	}
}

func TestCSFRV19O2AddressedEvidenceAndWorldRevisionAuthority(t *testing.T) {
	ctx := context.Background()
	c, store, ledger, cycle, fence, path := singleResourceFixture(t)
	if _, err := c.CommitSingleResource(ctx, ledger, cycle, fence); err != nil {
		t.Fatal(err)
	}
	_, _, _, _, _, run := ledger.Snapshot()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	task := findTerminalRoundTask(t, ctx, db)
	delivered, err := store.DeliverRoundPerception(ctx, task, c.bundle)
	if err != nil {
		t.Fatal(err)
	}
	q, err := sqlite.OpenPrivilegedObservatory(ctx, path, c.bundle)
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close()
	before := o1WholeDatabaseDigest(t, path)
	report, err := q.ReadPrivilegedReport(ctx, [32]byte(run))
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Commits) != 1 {
		t.Fatalf("expected single resource commit: %+v", report)
	}
	item := report.Commits[0]
	if item.Revision.Ledger != "world_revisions" || item.Revision.Revision != 1 ||
		item.Revision.ContentHash == ([32]byte{}) {
		t.Fatalf("world revision lacks authenticated owner: %+v", item.Revision)
	}
	if len(item.Evidence) != 2 || len(item.Completions) != 1 {
		t.Fatalf("addressed perception did not resolve full durable refs: evidence=%d completions=%d", len(item.Evidence), len(item.Completions))
	}
	if item.Completions[0].ID != delivered.Completion.Identity {
		t.Fatal("wrong task-completion record was reported")
	}
	observed := map[[32]byte]bool{}
	for _, ref := range item.Evidence {
		if ref.Ledger != "evidence_ledger" || ref.ContentHash == ([32]byte{}) {
			t.Fatal("invalid privileged evidence ref")
		}
		observed[ref.ID] = true
	}
	if !observed[delivered.Observation.Identity] || !observed[delivered.Input.Identity] {
		t.Fatal("Observation/KnowledgeInput identities were not reported exactly")
	}
	if report.PendingPerceptionTasks < 1 {
		t.Fatal("uncompleted internal task was hidden by addressed evidence")
	}
	if o1WholeDatabaseDigest(t, path) != before {
		t.Fatal("evidence audit mutated authoritative data")
	}
	var original []byte
	if err := db.QueryRowContext(ctx, "SELECT canonical_hash FROM evidence_routes WHERE record_id=?", delivered.Observation.Identity[:]).Scan(&original); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE evidence_routes SET canonical_hash=zeroblob(32) WHERE record_id=?", delivered.Observation.Identity[:]); err != nil {
		t.Fatal(err)
	}
	if _, err := q.ReadPrivilegedReport(ctx, [32]byte(run)); err == nil {
		t.Fatal("tampered addressed route was exposed as valid")
	}
	if _, err := db.ExecContext(ctx, "UPDATE evidence_routes SET canonical_hash=? WHERE record_id=?", original, delivered.Observation.Identity[:]); err != nil {
		t.Fatal(err)
	}
	if _, err := q.ReadPrivilegedReport(ctx, [32]byte(run)); err != nil {
		t.Fatal("restored route failed:", err)
	}
}

func TestCSFRV19O2RejectsOrphanDecisionRecord(t *testing.T) {
	ctx := context.Background()
	c, _, ledger, cycle, fence, path := singleResourceFixture(t)
	if _, err := c.CommitSingleResource(ctx, ledger, cycle, fence); err != nil {
		t.Fatal(err)
	}
	_, _, _, _, _, run := ledger.Snapshot()
	observer, err := sqlite.OpenPrivilegedObservatory(ctx, path, c.bundle)
	if err != nil {
		t.Fatal(err)
	}
	defer observer.Close()
	if _, err := observer.ReadPrivilegedReport(ctx, [32]byte(run)); err != nil {
		t.Fatal("baseline report:", err)
	}
	admin, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	rows, err := admin.QueryContext(ctx, "SELECT id, canonical FROM decision_ledger ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}
	var decisionID [32]byte
	var original []byte
	for rows.Next() {
		var id, raw []byte
		if err := rows.Scan(&id, &raw); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		decoded, err := codec.StrictDecodePayload(raw)
		if err != nil {
			rows.Close()
			t.Fatal(err)
		}
		fields, ok := decoded.([]any)
		if ok && len(fields) == 16 {
			if len(id) != 32 {
				rows.Close()
				t.Fatal("decision id is not 32 bytes")
			}
			copy(decisionID[:], id)
			original = bytes.Clone(raw)
			break
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		t.Fatal(err)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	if len(original) == 0 {
		t.Fatal("single resource terminal fixture has no DecisionRecord")
	}
	// The inserted row is a recognizable DecisionRecord copied from a real
	// committed cycle but indexed under an unreferenced SQL identity. The
	// privileged report must reject it, not silently omit this corrupt row.
	orphan := decisionID
	orphan[0] ^= 0x86
	if _, err := admin.ExecContext(ctx, "INSERT INTO decision_ledger(id, canonical) VALUES(?, ?)", orphan[:], original); err != nil {
		t.Fatal(err)
	}
	if got, err := observer.ReadPrivilegedReport(ctx, [32]byte(run)); err == nil {
		t.Fatalf("orphan DecisionRecord bypassed membership audit: %+v", got)
	}
	if _, err := admin.ExecContext(ctx, "DELETE FROM decision_ledger WHERE id=?", orphan[:]); err != nil {
		t.Fatal(err)
	}
	// A second orphan has a valid normative ID and body digest; checking
	// only whether the SQL key equals the payload identity is insufficient.
	parsed, err := codec.StrictDecodePayload(original)
	if err != nil {
		t.Fatal(err)
	}
	fields := parsed.([]any)
	fakeCycle, ok := fields[2].([]byte)
	if !ok || len(fakeCycle) != 32 {
		t.Fatal("DecisionRecord has no canonical cycle ID")
	}
	fakeCycle = bytes.Clone(fakeCycle)
	fakeCycle[0] ^= 0x71
	fields[2] = fakeCycle
	source, ok := fields[4].([]any)
	if !ok || len(source) < 3 {
		t.Fatal("DecisionRecord lacks a canonical source ref")
	}
	sourceID, ok := source[2].([]byte)
	if !ok || len(sourceID) != 32 {
		t.Fatal("DecisionRecord source ID is malformed")
	}
	d := identity.New(c.bundle)
	normativeID, _, err := d.Sum("cote.csf.id.decision",
		"cote.csf.schema.decision-id-preimage", 1,
		[]any{fakeCycle, fields[3], sourceID})
	if err != nil {
		t.Fatal(err)
	}
	fields[0] = normativeID[:]
	digest, _, err := d.Sum("cote.csf.digest.decision-record",
		"cote.csf.schema.decision-record.body", 1, fields[:15])
	if err != nil {
		t.Fatal(err)
	}
	fields[15] = digest[:]
	forgedValid, err := codec.CanonicalPayload(fields)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admin.ExecContext(ctx, "INSERT INTO decision_ledger(id, canonical) VALUES(?, ?)",
		normativeID[:], forgedValid); err != nil {
		t.Fatal(err)
	}
	if got, err := observer.ReadPrivilegedReport(ctx, [32]byte(run)); err == nil {
		t.Fatalf("self-consistent orphan DecisionRecord without CycleCommit was accepted: %+v", got)
	}
	if _, err := admin.ExecContext(ctx, "DELETE FROM decision_ledger WHERE id=?", normativeID[:]); err != nil {
		t.Fatal(err)
	}
	if _, err := observer.ReadPrivilegedReport(ctx, [32]byte(run)); err != nil {
		t.Fatal("restored valid run no longer reportable:", err)
	}
}

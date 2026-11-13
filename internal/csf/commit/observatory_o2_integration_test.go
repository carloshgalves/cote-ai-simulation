package commit

import (
	"bytes"
	"context"
	"database/sql"
	"reflect"
	"strings"
	"testing"

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

package commit

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/hex"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

// STRESS-41 requires one actual ConflictSet, not just two dispositions
// and a nonnegative stock. Verify the registered conflict body directly
// from the decision ledger, then independently compare the real query
// projection and both outcomes under reversed worker order.
func TestReferenceG1ExactLastUnitConflictSet(t *testing.T) {
	ctx := context.Background()
	bundle, err := policy.Load(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	var baseline []byte
	var winner string
	for i, reverse := range []bool{false, true} {
		path := filepath.Join(t.TempDir(), "last-unit.db")
		result, err := RunReference(ctx, path, bundlePath, ReferenceOptions{ReverseResponses: reverse, ReverseSources: reverse})
		if err != nil {
			t.Fatal(err)
		}
		runRaw, err := hex.DecodeString(result.RunID)
		if err != nil || len(runRaw) != 32 || result.FinalStock != 0 || len(result.ContestDecisions) != 2 {
			t.Fatal("missing genuine two-contender run")
		}
		var run [32]byte
		copy(run[:], runRaw)
		q, err := sqlite.OpenPrivilegedObservatory(ctx, path, bundle)
		if err != nil {
			t.Fatal(err)
		}
		report, err := q.ReadPrivilegedReport(ctx, run)
		q.Close()
		if err != nil || len(report.Commits) != 3 || len(report.Commits[0].DecisionLinks) != 2 {
			t.Fatal("missing two terminal decisions in same base revision", err)
		}
		commit := report.Commits[0]
		if commit.BaseRevision != 0 || commit.ResultRevision != 1 {
			t.Fatal("contenders not resolved on one base revision")
		}
		a, b := commit.DecisionLinks[0], commit.DecisionLinks[1]
		if a.Candidate == nil || b.Candidate == nil || a.Candidate.ID == b.Candidate.ID ||
			len(a.Conflicts) != 1 || len(b.Conflicts) != 1 || a.Conflicts[0] != b.Conflicts[0] ||
			len(a.Draws) != 1 || len(b.Draws) != 1 || a.Draws[0] != b.Draws[0] {
			t.Fatal("two distinct contenders do not share one exact conflict/draw")
		}
		db, err := sql.Open("sqlite", path)
		if err != nil {
			t.Fatal(err)
		}
		var raw []byte
		conflict := a.Conflicts[0]
		err = db.QueryRowContext(ctx, "SELECT canonical FROM decision_ledger WHERE id=?", conflict.ID[:]).Scan(&raw)
		if err != nil {
			db.Close()
			t.Fatal(err)
		}
		value, err := codec.StrictDecodePayload(raw)
		if err != nil {
			db.Close()
			t.Fatal(err)
		}
		fields, ok := value.([]any)
		if !ok || len(fields) != 7 || fields[3] != "last.resource" {
			db.Close()
			t.Fatal("wrong persisted conflict record")
		}
		members, ok := fields[1].([]any)
		if !ok || len(members) != 2 {
			db.Close()
			t.Fatal("conflict does not enumerate both candidates")
		}
		one, two := a.Candidate.ID, b.Candidate.ID
		if bytes.Compare(one[:], two[:]) > 0 {
			one, two = two, one
		}
		if !reflect.DeepEqual(members, []any{one[:], two[:]}) {
			db.Close()
			t.Fatal("conflict membership differs from committed candidates")
		}
		count := 0
		rows, err := db.QueryContext(ctx, "SELECT id,canonical FROM decision_ledger")
		if err != nil {
			db.Close()
			t.Fatal(err)
		}
		for rows.Next() {
			var id, canonical []byte
			if err := rows.Scan(&id, &canonical); err != nil {
				rows.Close()
				db.Close()
				t.Fatal(err)
			}
			v, err := codec.StrictDecodePayload(canonical)
			if err != nil {
				rows.Close()
				db.Close()
				t.Fatal(err)
			}
			f, ok := v.([]any)
			if !ok || len(f) != 7 {
				continue
			}
			dg, _, err := identity.New(bundle).Sum("cote.csf.digest.conflict-set", "cote.csf.schema.conflict-set.body", 1, f[:6])
			if err == nil && reflect.DeepEqual(f[6], dg[:]) {
				count++
				if !bytes.Equal(id, conflict.ID[:]) || !bytes.Equal(canonical, raw) {
					rows.Close()
					db.Close()
					t.Fatal("unexpected additional conflict")
				}
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			db.Close()
			t.Fatal(err)
		}
		rows.Close()
		db.Close()
		if count != 1 || result.ContestDecisions[result.ContestWinner] != "COMMIT" {
			t.Fatal("not exactly one authenticated conflict and winner")
		}
		for actor, status := range result.ContestDecisions {
			if actor != result.ContestWinner && status != "REJECT" {
				t.Fatal("loser did not reject")
			}
		}
		if i == 0 {
			baseline, winner = bytes.Clone(raw), result.ContestWinner
		} else if !bytes.Equal(baseline, raw) || winner != result.ContestWinner {
			t.Fatal("arrival order changed conflict authority or winner")
		}
	}
}

//go:build linux

package commit

import (
	"bytes"
	"context"
	"database/sql"
	"reflect"
	"testing"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/observatory"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
)

// Independent SQL membership/hash checks for the read-only projection,
// against the real 9-commit integrated execution. No mock report or store.
func TestReferenceF1IntegratedProvenance(t *testing.T) {
	ctx := context.Background()
	host, plans, cuts := referenceE2SignedPair(t)
	path, owner := referenceE2Paths(t)
	p, err := RunReferenceD3(ctx, path, owner, bundlePath,
		ReferenceD2Config{Host: host, EpochPlans: plans, frozenSupervisedCuts: &cuts}, ReferenceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	run := referenceC2Decode32(t, p.RunID)
	before := referenceE2Capture(t, path, owner)
	q, err := sqlite.OpenPrivilegedObservatory(ctx, path, host.Bundle)
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close()
	report, err := q.ReadPrivilegedReport(ctx, run)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Commits) != 9 || len(report.Attempts) != 10 ||
		report.State.Clock.Revision != 9 || report.State.Control.Status != observatory.IDLE {
		t.Fatal("not the real integrated terminal chain")
	}
	db, err := openReferenceReadOnlyDB(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	read := func(ref observatory.AuthorityRef) []byte {
		t.Helper()
		switch ref.Ledger {
		case "input_ledger", "decision_ledger", "event_store", "causal_outbox", "evidence_ledger", "scheduled_occurrences", "trigger_activations":
		default:
			t.Fatal("unregistered projection authority", ref.Ledger)
		}
		var raw []byte
		if err := db.QueryRowContext(ctx, "SELECT canonical FROM "+ref.Ledger+" WHERE id=?", ref.ID[:]).Scan(&raw); err != nil {
			t.Fatal(err)
		}
		if [32]byte(identity.OpaqueContentHash(raw)) != ref.ContentHash {
			t.Fatal("reported hash not derived from actual SQL authority")
		}
		return raw
	}
	aborts, retries, links, candidates, conflicts, draws := 0, 0, 0, 0, 0, 0
	var contested [][32]byte
	for _, a := range report.Attempts {
		read(a.Fence)
		if a.Abort != nil {
			read(*a.Abort)
			aborts++
			if a.Abort.ID != referenceC2Decode32(t, p.d2.C4.AbortRetry.AbortID) || a.Commit != nil {
				t.Fatal("abort not the historical C3 terminal")
			}
		}
		if a.Retry != nil {
			read(*a.Retry)
			retries++
			if a.Retry.ID != referenceC2Decode32(t, p.d2.C4.AbortRetry.RetryID) {
				t.Fatal("retry not the historical C3 authorization")
			}
		}
	}
	if aborts != 1 || retries != 1 {
		t.Fatal("historical abort/retry disappeared")
	}
	physical, addressed := 0, 0
	for _, c := range report.Commits {
		read(c.Fence)
		read(c.Commit)
		for _, refs := range [][]observatory.AuthorityRef{c.Closures, c.Admitted, c.Events, c.Tasks, c.Completions, c.Evidence} {
			for _, ref := range refs {
				read(ref)
			}
		}
		var raw []byte
		if err := db.QueryRowContext(ctx, "SELECT canonical FROM world_revisions WHERE revision=?", c.ResultRevision).Scan(&raw); err != nil {
			t.Fatal(err)
		}
		if c.Revision.Revision != c.ResultRevision || c.Revision.ContentHash != [32]byte(identity.OpaqueContentHash(raw)) {
			t.Fatal("world revision authority mismatch")
		}
		if len(c.Decisions) != len(c.DecisionLinks) {
			t.Fatal("terminal decision links incomplete")
		}
		for i, link := range c.DecisionLinks {
			links++
			if link.Decision != c.Decisions[i] {
				t.Fatal("decision order changed")
			}
			val, err := codec.StrictDecodePayload(read(link.Decision))
			if err != nil {
				t.Fatal(err)
			}
			fields := val.([]any)
			source := fields[4].([]any)
			read(link.Source)
			if !bytes.Equal(source[len(source)-2].([]byte), link.Source.ID[:]) ||
				!bytes.Equal(source[len(source)-1].([]byte), link.SourceDigest[:]) ||
				!bytes.Equal(fields[15].([]byte), link.DecisionDigest[:]) {
				t.Fatal("decision source/digest link mismatch")
			}
			if link.Candidate != nil {
				candidates++
				read(*link.Candidate)
				if !bytes.Equal(fields[6].([]any)[1].([]byte), link.Candidate.ID[:]) {
					t.Fatal("candidate link differs from terminal")
				}
			} else if len(fields[6].([]any)) != 1 {
				t.Fatal("candidate reference omitted")
			}
			for _, ref := range link.Conflicts {
				conflicts++
				read(ref)
			}
			for _, ref := range link.Draws {
				draws++
				read(ref)
			}
			if len(link.Conflicts) != 0 {
				if link.Candidate == nil {
					t.Fatal("contested decision missing candidate")
				}
				contested = append(contested, link.Candidate.ID)
			}
			for _, e := range link.ProducedEvents {
				val, err := codec.StrictDecodePayload(read(e))
				if err != nil {
					t.Fatal(err)
				}
				if val.([]any)[3] == "physical.advanced" {
					physical++
					if len(c.Tasks) != 0 || len(c.Evidence) != 0 || link.Source.Ledger != "input_ledger" {
						t.Fatal("physical engine-only event acquired addressed evidence")
					}
				}
			}
		}
		if len(c.PerceptionLinks) != len(c.Tasks) {
			t.Fatal("perception link roster differs from committed tasks")
		}
		for _, link := range c.PerceptionLinks {
			raw := read(link.Task)
			value, err := codec.StrictDecodePayload(raw)
			if err != nil || !bytes.Equal(value.([]any)[3].([]byte), link.Event.ID[:]) {
				t.Fatal("task-to-event link absent or forged", err)
			}
			read(link.Event)
			if link.Completion == nil {
				t.Fatal("completed D3 retained an unfinished task")
			}
			read(*link.Completion)
			for _, refs := range [][]observatory.AuthorityRef{link.Observations, link.KnowledgeInputs} {
				for _, ref := range refs { read(ref) }
			}
			addressed += len(link.Observations) + len(link.KnowledgeInputs)
		}
	}
	if links == 0 || candidates == 0 || conflicts != 2 || draws != 2 || len(contested) != 2 || contested[0] == contested[1] || physical != 2 || addressed != 2 {
		t.Fatalf("incomplete chain links=%d candidates=%d conflict=%d draws=%d physics=%d evidence=%d", links, candidates, conflicts, draws, physical, addressed)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := q.Close(); err != nil {
		t.Fatal(err)
	}
	q, err = sqlite.OpenPrivilegedObservatory(ctx, path, host.Bundle)
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close()
	again, err := q.ReadPrivilegedReport(ctx, run)
	if err != nil || !reflect.DeepEqual(report, again) {
		t.Fatal("cold read changed report", err)
	}
	if err := referenceE1Compare(before, referenceE2Capture(t, path, owner)); err != nil {
		t.Fatal("privileged query changed causal/owner authority", err)
	}

	// Damage a real candidate, then require an empty report and byte-exact
	// persistence of the damaged transcript. Restore before next mutation.
	admin, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	var candidateRaw []byte
	if err := admin.QueryRowContext(ctx, "SELECT canonical FROM decision_ledger WHERE id=?", contested[0][:]).Scan(&candidateRaw); err != nil {
		t.Fatal(err)
	}
	val, err := codec.StrictDecodePayload(candidateRaw)
	if err != nil {
		t.Fatal(err)
	}
	fields := val.([]any)
	fields[3] = []any{contested[1][:]}
	mutant, err := codec.CanonicalPayload(fields)
	if err != nil {
		t.Fatal(err)
	}
	result, err := admin.ExecContext(ctx, "UPDATE decision_ledger SET canonical=? WHERE id=?", mutant, contested[0][:])
	if err != nil {
		t.Fatal(err)
	}
	if n, err := result.RowsAffected(); err != nil || n != 1 {
		t.Fatal("candidate mutation missed actual authority")
	}
	damaged := referenceE2Capture(t, path, owner)
	if err := referenceE1Compare(before, damaged); err == nil {
		t.Fatal("mutation did not change transcript")
	}
	bad, err := q.ReadPrivilegedReport(ctx, run)
	if err == nil || !reflect.DeepEqual(bad, observatory.PrivilegedReport{}) {
		t.Fatal("corrupt candidate acquired privileged report")
	}
	if err := referenceE1Compare(damaged, referenceE2Capture(t, path, owner)); err != nil {
		t.Fatal("rejected query mutated authority", err)
	}
	if _, err := admin.ExecContext(ctx, "UPDATE decision_ledger SET canonical=? WHERE id=?", candidateRaw, contested[0][:]); err != nil {
		t.Fatal(err)
	}
	if restored, err := q.ReadPrivilegedReport(ctx, run); err != nil || !reflect.DeepEqual(restored, report) {
		t.Fatal("positive control did not recover after restoration", err)
	}
	// A valid candidate record under an unrelated SQL key must not be
	// ignored just because no terminal decision cites the forged key.
	orphan := contested[0]
	orphan[0] ^= 0xff
	if _, err := admin.ExecContext(ctx, "INSERT INTO decision_ledger(id,canonical) VALUES(?,?)", orphan[:],candidateRaw); err != nil {
		t.Fatal(err)
	}
	damaged = referenceE2Capture(t,path,owner)
	if bad,err := q.ReadPrivilegedReport(ctx,run); err==nil || !reflect.DeepEqual(bad,observatory.PrivilegedReport{}) {
		t.Fatal("orphan candidate acquired verified report",err)
	}
	if err:=referenceE1Compare(damaged,referenceE2Capture(t,path,owner));err!=nil{t.Fatal(err)}
	if _,err:=admin.ExecContext(ctx,"DELETE FROM decision_ledger WHERE id=?",orphan[:]);err!=nil{t.Fatal(err)}
	if err := referenceE1Compare(before, referenceE2Capture(t, path, owner)); err != nil {
		t.Fatal("restored transcript differs", err)
	}
}

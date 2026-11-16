package commit

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/inputledger"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
)

func TestReferenceRunRealCausalSQLiteAndReplay(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "reference.db")
	result, err := RunReference(ctx, path, bundlePath, ReferenceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "RUN_LOCAL_10B" || len(result.Cycles) != 3 || result.TriggerActivations != 1 || result.ConsumedSystemUnits != 2 {
		t.Fatalf("incomplete real causal flow: %+v", result)
	}
	if result.FinalRevision != 3 || result.FinalInstant != 10 || result.FinalStock != 0 || result.ReplayedCommits != 3 || result.ReplayedEvents < 7 {
		t.Fatalf("incorrect causal state/replay: %+v", result)
	}
	if len(result.EventBytes) == 0 || len(result.ContestFenceBytes) == 0 || len(result.ContestCommitBytes) == 0 {
		t.Fatal("authoritative event/fence/commit bytes absent")
	}
	if result.ContestDecisions[result.ContestWinner] != "COMMIT" || len(result.ContestDecisions) != 2 {
		t.Fatal("winner is not derived from the two committed DecisionRecords")
	}
	for actor, status := range result.ContestDecisions {
		if actor != result.ContestWinner && status != "REJECT" {
			t.Fatal("loser not rejected", actor, status)
		}
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("SQLite database not persisted", err)
	}
	reopened, err := sqlite.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	rawID, err := hex.DecodeString(result.RunID)
	if err != nil || len(rawID) != 32 { t.Fatal("invalid report RunID",err) }
	var run domain.ID
	copy(run[:],rawID)
	bundle, err := policy.Load(bundlePath)
	if err != nil { t.Fatal(err) }
	ledger, err := inputledger.Open(ctx, run, bundle, reopened)
	if err != nil {
		t.Fatal("cannot reconstruct input ledger", err)
	}
	if _, err := ledger.CloseSource(ctx, "physical.a1", inputledger.Closure{ProducerKey: "new-final", Final: true}); !errors.Is(err, inputledger.ErrFinalSource) {
		t.Fatalf("final SourceClosure admitted a new closure: %v", err)
	}
}

func TestReferenceRunSourceAndWorkerPermutationIsByteIdentical(t *testing.T) {
	ctx := context.Background()
	var first ReferenceResult
	for i, options := range []ReferenceOptions{{}, {ReverseSources: true, ReverseResponses: true}, {ReverseResponses: true}, {ReverseSources: true}} {
		path := filepath.Join(t.TempDir(), "independent.db")
		r, err := RunReference(ctx, path, bundlePath, options)
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			first = r
			continue
		}
		if r.RunID != first.RunID || r.ContestWinner != first.ContestWinner || r.FinalWorldHash != first.FinalWorldHash ||
			!bytes.Equal(r.EventBytes, first.EventBytes) || !bytes.Equal(r.ContestFenceBytes, first.ContestFenceBytes) ||
			!bytes.Equal(r.ContestCommitBytes, first.ContestCommitBytes) {
			t.Fatalf("causal output depends on source or response order: iteration %d", i)
		}
        if len(r.CycleFenceBytes) != 3 || len(r.CycleCommitBytes) != 3 ||
            len(first.CycleFenceBytes) != 3 || len(first.CycleCommitBytes) != 3 {
            t.Fatal("cycle byte oracle omitted one of the three committed cycles")
        }
        for cycle := 0; cycle < 3; cycle++ {
            if !bytes.Equal(r.CycleFenceBytes[cycle], first.CycleFenceBytes[cycle]) ||
                !bytes.Equal(r.CycleCommitBytes[cycle], first.CycleCommitBytes[cycle]) {
                t.Fatalf("causal cycle %d fence/journal depends on arrival order", cycle)
            }
        }
	}
}

func TestReferenceRunMissingSourceClosureFailsBeforeFence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "unclosed.db")
	_, err := RunReference(context.Background(), path, bundlePath, ReferenceOptions{LeaveSourceOpen: true})
	if err == nil {
		t.Fatal("unclosed exogenous source permitted a causal fence")
	}
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for table, want := range map[string]int{"committed_cycles": 0, "event_store": 0, "world_revisions": 1} {
		var got int
		if err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("unclosed source changed %s: %d", table, got)
		}
	}
}

func TestReferenceRunNeverReusesExistingDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "existing.db")
	if err := os.WriteFile(path, []byte("existing data"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := RunReference(context.Background(), path, bundlePath, ReferenceOptions{}); err == nil {
		t.Fatal("overwrote an existing database")
	}
	content, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(content, []byte("existing data")) {
		t.Fatal("existing database contents destroyed", err)
	}
}

func TestReferenceRunContestFailpointNeverPublishesHalfRevision(t *testing.T) {
	for step := uint8(1); step <= 7; step++ {
		t.Run(string(rune('a'+step)), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "fault.db")
			_, err := RunReference(context.Background(), path, bundlePath, ReferenceOptions{FailContestAt: step})
			if err == nil {
				t.Fatal("expected failure injection to reject atomic commit")
			}
			db, err := sql.Open("sqlite", "file:"+path)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			for table, want := range map[string]int{"committed_cycles": 0, "event_store": 0, "world_revisions": 1} {
				var count int
				if err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil {
					t.Fatal(err)
				}
				if count != want {
					t.Fatalf("fault %d leaked %s=%d", step, table, count)
				}
			}
		})
	}
}

// TestReferenceRunLateInputCannotRetroactAndFinalRejectsFurtherIngress
// checks the *real* durable source ledger, rather than trusting a manifest
// flag or a precomputed coordinate in a planning-only fixture.
func TestReferenceRunLateInputCannotRetroactAndFinalRejectsFurtherIngress(t *testing.T) {
    ctx:=context.Background()
    bundle,err:=policy.Load(bundlePath);if err!=nil{t.Fatal(err)}
    g,err:=buildReferenceGenesis(bundle);if err!=nil{t.Fatal(err)}
    path:=filepath.Join(t.TempDir(),"source-contract.db")
    store,err:=sqlite.Open(ctx,path);if err!=nil{t.Fatal(err)}
    defer store.Close()
    if err:=store.PersistGenesis(ctx,g,bundle);err!=nil{t.Fatal(err)}
    if err:=store.BootstrapClock(ctx,g,bundle);err!=nil{t.Fatal(err)}
    ledger,err:=inputledger.New(ctx,g.RunID,bundle,store);if err!=nil{t.Fatal(err)}
    if err:=ledger.DeclareSource(inputledger.Source{ID:"school-events"});err!=nil{t.Fatal(err)}
    zero:=domain.EligibilityCoordinate{NotBeforeInstant:g.InitialInstant,NotBeforeCycleOrdinal:0}
    if _,err:=ledger.CloseSource(ctx,"school-events",inputledger.Closure{
        ProducerKey:"partial.0",ClosedThrough:zero});err!=nil{t.Fatal(err)}
    input:=inputledger.Input{ProducerUnitKey:"school.e1",Kind:"resource.notice",
        SchemaVersion:1,Actor:[]any{uint64(0)},Payload:g.Payload[16],
        DeclaredEffectiveAt:g.InitialInstant,Eligibility:zero,
        Provenance:[]any{uint64(0)},IdempotencyKey:[]any{uint64(0)}}
    received,err:=ledger.AppendInput(ctx,"school-events",input);if err!=nil{t.Fatal(err)}
    coord,ok:=received.Fields[9].([]any)
    if !ok||len(coord)!=2||coord[0]!=int64(g.InitialInstant)||coord[1]!=uint64(1){
        t.Fatalf("late input retroactively entered already closed coordinate: %#v",received.Fields[9])
    }
    if received.Receipt.Sequence!=1 {t.Fatal("ingress receipt did not follow partial closure")}
    atTen:=domain.EligibilityCoordinate{NotBeforeInstant:g.InitialInstant+10,NotBeforeCycleOrdinal:0}
    if _,err:=ledger.CloseSource(ctx,"school-events",inputledger.Closure{
        ProducerKey:"final.10",ClosedThrough:atTen,Final:true});err!=nil{t.Fatal(err)}
    input.ProducerUnitKey="school.e2"
    if _,err:=ledger.AppendInput(ctx,"school-events",input);!errors.Is(err,inputledger.ErrFinalSource){
        t.Fatalf("post-final input accepted: %v",err)
    }
    if _,err:=ledger.CloseSource(ctx,"school-events",inputledger.Closure{
        ProducerKey:"after-final",ClosedThrough:atTen,Final:true});!errors.Is(err,inputledger.ErrFinalSource){
        t.Fatalf("post-final closure accepted: %v",err)
    }
    db,err:=sql.Open("sqlite","file:"+path);if err!=nil{t.Fatal(err)}
    defer db.Close()
    for table,want:=range map[string]int{"committed_cycles":0,"event_store":0,"world_revisions":1}{
        var got int
        if err:=db.QueryRow("SELECT COUNT(*) FROM "+table).Scan(&got);err!=nil{t.Fatal(err)}
        if got!=want{t.Fatalf("source normalization published half-world %s=%d",table,got)}
    }
}

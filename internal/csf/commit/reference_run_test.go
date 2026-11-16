package commit

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/inputledger"
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
	ledger, err := inputledger.Open(ctx, mustReferenceRunID(t, result.RunID), mustReferenceBundle(t), reopened)
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
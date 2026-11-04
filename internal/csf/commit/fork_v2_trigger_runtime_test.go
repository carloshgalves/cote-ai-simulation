package commit

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/inputledger"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/trigger"
)

func r21EvolvedTriggerSource(t *testing.T) (*Coordinator, *sqlite.Store, *inputledger.Ledger, domain.ID, *policy.Bundle, sqlite.SnapshotV1, string) {
	t.Helper()
	ctx := context.Background()
	rising := activeTrigger()
	repeating := rising
	repeating.ID[0]++
	repeating.Policy = trigger.RepeatWhileTrue
	repeating.RepeatEvery = 50
	once := rising
	once.ID[0] += 2
	once.Policy = trigger.OnceWhenTrue
	c, parent, ledger, run, b, path := testFixture(t, rising, repeating, once)
	state, err := parent.LoadClock(ctx, [32]byte(run))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.CloseSource(ctx, "driver", inputledger.Closure{ProducerKey: "r21-final", ClosedThrough: domain.EligibilityCoordinate{NotBeforeInstant: state.Instant + 1000}, Final: true}); err != nil {
		t.Fatal(err)
	}
	initial, err := parent.CreateSnapshotV1(ctx, [32]byte(run), b, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Advance(ctx, ledger, run); err != nil {
		t.Fatal(err)
	}
	if _, err := parent.DrainPendingPerceptionV1(ctx, [32]byte(run), initial.ID, b); err != nil {
		t.Fatal(err)
	}
	// Consume the three first activations, then the ordinal-1 Genesis unit.
	for i := 0; i < 2; i++ {
		cycle, fence := r20CurrentWork(t, c, parent, ledger, run)
		if _, err := c.DispatchFencedWorkV1(ctx, ledger, cycle, fence); err != nil {
			t.Fatal(err)
		}
		if _, err := parent.DrainPendingPerceptionV1(ctx, [32]byte(run), initial.ID, b); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := c.Advance(ctx, ledger, run); err != nil {
		t.Fatal(err)
	}
	if _, err := parent.DrainPendingPerceptionV1(ctx, [32]byte(run), initial.ID, b); err != nil {
		t.Fatal(err)
	}
	source, err := parent.LoadRegisteredTriggers(ctx, [32]byte(run), b)
	if err != nil {
		t.Fatal(err)
	}
	if len(source) != 3 {
		t.Fatal("source trigger fixture incomplete")
	}
	if source[1].Runtime.ActivationCount != 2 || !source[2].Runtime.Exhausted {
		t.Fatal("source did not evolve as required")
	}
	snapshot, err := parent.CreateSnapshotV1(ctx, [32]byte(run), b, nil)
	if err != nil {
		t.Fatal(err)
	}
	return c, parent, ledger, run, b, snapshot, path
}

func TestCSFRV18R21RepeatCadenceReplayUsesHistoricalRuntime(t *testing.T) {
	ctx := context.Background()
	_, parent, _, run, b, snapshot, path := r21EvolvedTriggerSource(t)
	report, err := parent.ReplayClock(ctx, [32]byte(run), b)
	if err != nil {
		t.Fatal(err)
	}
	if report.ReplayedCommits != 4 || report.ReplayedTriggerTransitions != 6 {
		t.Fatalf("incomplete repeat history: %+v", report)
	}
	reopened, err := sqlite.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if _, err := reopened.ReplayClock(ctx, [32]byte(run), b); err != nil {
		t.Fatal("repeat history lost after reopen:", err)
	}
	registered, err := reopened.LoadRegisteredTriggers(ctx, [32]byte(run), b)
	if err != nil {
		t.Fatal(err)
	}
	repeat := registered[1]
	bad := repeat.Runtime
	bad.NextRepeat += 500
	corrupt, err := trigger.RuntimeBytes(bad)
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.ExecContext(ctx, "UPDATE trigger_runtime SET canonical=? WHERE run_id=? AND definition_id=?", corrupt, run[:], repeat.Definition.ID[:]); err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.ReplayClock(ctx, [32]byte(run), b); err == nil || !strings.Contains(err.Error(), "runtime differs") {
		t.Fatal("historical planning trusted the corrupted live cadence:", err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE trigger_runtime SET canonical=? WHERE run_id=? AND definition_id=?", repeat.RuntimeCanonical, run[:], repeat.Definition.ID[:]); err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.VerifySnapshotV1(ctx, [32]byte(run), snapshot.ID, b); err != nil {
		t.Fatal("exact repeat runtime restoration failed:", err)
	}
}

func TestCSFRV18R21V2ForkPreservesEvolvedTriggerSeed(t *testing.T) {
	ctx := context.Background()
	c, parent, _, run, v1, checkpoint, _ := r21EvolvedTriggerSource(t)
	v2, err := policy.LoadV2("../../../docs/architecture/canonical-codec-v2-bundle")
	if err != nil {
		t.Fatal(err)
	}
	path := t.TempDir() + "/evolved-v2.db"
	child, err := sqlite.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer child.Close()
	childRun := [32]byte(run)
	childRun[0] ^= 0x73
	if _, err := parent.ForkIdleSnapshotV1ToV2(ctx, [32]byte(run), checkpoint.ID, childRun, child, v1, v2); err != nil {
		t.Fatal("evolved V2 trigger migration:", err)
	}
	source, err := parent.LoadRegisteredTriggers(ctx, [32]byte(run), v1)
	if err != nil {
		t.Fatal(err)
	}
	got, err := child.LoadRegisteredTriggers(ctx, childRun, v2)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(source) {
		t.Fatal("trigger roster lost")
	}
	for i, item := range source {
		want := item.Runtime
		want.EvaluatedThrough = 0
		if got[i].Runtime != want || string(got[i].DefinitionCanonical) != string(item.DefinitionCanonical) {
			t.Fatalf("seed changed beyond revision rebase: got %+v want %+v", got[i].Runtime, want)
		}
	}
	report, err := child.ReplayClock(ctx, childRun, v2)
	if err != nil || report.ReplayedCommits != 0 || report.VerifiedActivations != 1 {
		t.Fatalf("seed replay: %+v %v", report, err)
	}
	if _, err := child.CreateSnapshotV1(ctx, childRun, v2, nil); err != nil {
		t.Fatal(err)
	}
	direct, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"event_store", "evidence_ledger", "causal_outbox"} {
		var count int
		if err := direct.QueryRowContext(ctx, "SELECT count(*) FROM "+table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("trigger import fabricated %s rows", table)
		}
	}
	direct.Close()
	reopened, err := sqlite.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if _, err := reopened.ReplayClock(ctx, childRun, v2); err != nil {
		t.Fatal("seed lost across reopen:", err)
	}
	// Settle the imported second activation, then fire and settle two more
	// child cadence activations. Rising/once may not refire at child birth.
	ledger, err := inputledger.Open(ctx, domain.ID(childRun), v2, child)
	if err != nil {
		t.Fatal(err)
	}
	coordinator := New(child, v2, c.admissionPolicies)
	seedSnapshot, err := child.CreateSnapshotV1(ctx, childRun, v2, nil)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		cycle, fence := r20CurrentWork(t, coordinator, child, ledger, domain.ID(childRun))
		if _, err := coordinator.DispatchFencedWorkV1(ctx, ledger, cycle, fence); err != nil {
			t.Fatal(err)
		}
		if _, err := child.DrainPendingPerceptionV1(ctx, childRun, seedSnapshot.ID, v2); err != nil {
			t.Fatal(err)
		}
		if _, err := child.ReplayClock(ctx, childRun, v2); err != nil {
			t.Fatal("child settlement replay:", err)
		}
		if i < 2 {
			if _, err := coordinator.Advance(ctx, ledger, domain.ID(childRun)); err != nil {
				t.Fatal(err)
			}
			if _, err := child.DrainPendingPerceptionV1(ctx, childRun, seedSnapshot.ID, v2); err != nil {
				t.Fatal(err)
			}
		}
	}
	final, err := child.LoadRegisteredTriggers(ctx, childRun, v2)
	if err != nil {
		t.Fatal(err)
	}
	if final[0].Runtime.ActivationCount != 1 || final[1].Runtime.ActivationCount != 4 || !final[2].Runtime.Exhausted || final[2].Runtime.ActivationCount != 1 {
		t.Fatalf("trigger histories restarted: %+v", final)
	}
	if _, err := reopened.ReplayClock(ctx, childRun, v2); err != nil {
		t.Fatal("child tail reopen replay:", err)
	}
	// Mutate persisted proofs, never caller-owned definitions or a second
	// mutable runtime oracle. Restoration must recover the exact same tail.
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var manifest, manifestHash []byte
	if err := db.QueryRowContext(ctx, "SELECT canonical,canonical_hash FROM fork_trigger_imports WHERE child_run=?", childRun[:]).Scan(&manifest, &manifestHash); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "DELETE FROM fork_trigger_imports WHERE child_run=?", childRun[:]); err != nil {
		t.Fatal(err)
	}
	if _, err := child.ValidatedGenesis(ctx, childRun, v2); err == nil {
		t.Fatal("missing evolved seed proof accepted")
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO fork_trigger_imports(child_run,canonical,canonical_hash) VALUES(?,?,?)", childRun[:], manifest, manifestHash); err != nil {
		t.Fatal(err)
	}
	for _, index := range []int{1, 2, 3, 4, 5} {
		value, err := codec.StrictDecodePayload(manifest)
		if err != nil {
			t.Fatal(err)
		}
		fields := value.([]any)
		switch index {
		case 1:
			fields[1].([]byte)[0] ^= 1
		case 2:
			fields[2] = []any{}
		case 3:
			fields[3] = []any{}
		case 4:
			definitions := fields[2].([]any)
			value, err := codec.StrictDecodePayload(definitions[0].([]byte))
			if err != nil {
				t.Fatal(err)
			}
			definition := value.([]any)
			definition[3] = uint64(1)
			changed, err := codec.CanonicalPayload(definition)
			if err != nil {
				t.Fatal(err)
			}
			definitions[0] = changed
		case 5:
			activations := fields[3].([]any)
			value, err := codec.StrictDecodePayload(activations[0].([]byte))
			if err != nil {
				t.Fatal(err)
			}
			activation := value.([]any)
			activation[7] = activation[7].(uint64) + 1
			changed, err := codec.CanonicalPayload(activation)
			if err != nil {
				t.Fatal(err)
			}
			activations[0] = changed
		}
		bad, err := codec.CanonicalPayload(fields)
		if err != nil {
			t.Fatal(err)
		}
		hash := identity.OpaqueContentHash(bad)
		if _, err := db.ExecContext(ctx, "UPDATE fork_trigger_imports SET canonical=?,canonical_hash=? WHERE child_run=?", bad, hash[:], childRun[:]); err != nil {
			t.Fatal(err)
		}
		if _, err := child.ReplayClock(ctx, childRun, v2); err == nil {
			t.Fatalf("rehashed trigger proof mutation %d accepted", index)
		}
	}
	if _, err := db.ExecContext(ctx, "UPDATE fork_trigger_imports SET canonical=?,canonical_hash=? WHERE child_run=?", manifest, manifestHash, childRun[:]); err != nil {
		t.Fatal(err)
	}
	if _, err := child.ReplayClock(ctx, childRun, v2); err != nil {
		t.Fatal("exact trigger proof restoration:", err)
	}
	if _, err := parent.VerifySnapshotV1(ctx, [32]byte(run), checkpoint.ID, v1); err != nil {
		t.Fatal("parent changed:", err)
	}
}

func TestCSFRV18R21V2ImportsMultiplePendingPoliciesWithoutGenesisRefire(t *testing.T) {
	ctx := context.Background()
	rising := activeTrigger()
	repeating := rising
	repeating.ID[0]++
	repeating.Policy = trigger.RepeatWhileTrue
	repeating.RepeatEvery = 50
	once := rising
	once.ID[0] += 2
	once.Policy = trigger.OnceWhenTrue
	c, parent, ledger, run, v1, _ := testFixture(t, rising, repeating, once)
	state, err := parent.LoadClock(ctx, [32]byte(run))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.CloseSource(ctx, "driver", inputledger.Closure{ProducerKey: "r21-multiple-final", ClosedThrough: domain.EligibilityCoordinate{NotBeforeInstant: state.Instant + 1000}, Final: true}); err != nil {
		t.Fatal(err)
	}
	initial, err := parent.CreateSnapshotV1(ctx, [32]byte(run), v1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Advance(ctx, ledger, run); err != nil {
		t.Fatal(err)
	}
	if _, err := parent.DrainPendingPerceptionV1(ctx, [32]byte(run), initial.ID, v1); err != nil {
		t.Fatal(err)
	}
	checkpoint, err := parent.CreateSnapshotV1(ctx, [32]byte(run), v1, nil)
	if err != nil {
		t.Fatal(err)
	}
	v2, err := policy.LoadV2("../../../docs/architecture/canonical-codec-v2-bundle")
	if err != nil {
		t.Fatal(err)
	}
	child, err := sqlite.Open(ctx, t.TempDir()+"/multiple.db")
	if err != nil {
		t.Fatal(err)
	}
	defer child.Close()
	childRun := [32]byte(run)
	childRun[0] ^= 0x75
	if _, err := parent.ForkIdleSnapshotV1ToV2(ctx, [32]byte(run), checkpoint.ID, childRun, child, v1, v2); err != nil {
		t.Fatal(err)
	}
	report, err := child.ReplayClock(ctx, childRun, v2)
	if err != nil || report.VerifiedActivations != 3 {
		t.Fatalf("incomplete pending-policy import: %+v %v", report, err)
	}
	childLedger, err := inputledger.Open(ctx, domain.ID(childRun), v2, child)
	if err != nil {
		t.Fatal(err)
	}
	coordinator := New(child, v2, c.admissionPolicies)
	cycle, fence := r20CurrentWork(t, coordinator, child, childLedger, domain.ID(childRun))
	if _, err := coordinator.DispatchFencedWorkV1(ctx, childLedger, cycle, fence); err != nil {
		t.Fatal(err)
	}
	if _, err := child.ReplayClock(ctx, childRun, v2); err != nil {
		t.Fatal(err)
	}
	if _, err := parent.VerifySnapshotV1(ctx, [32]byte(run), checkpoint.ID, v1); err != nil {
		t.Fatal(err)
	}
}

func TestCSFRV18R21V2TriggerImportRollbackAndRetry(t *testing.T) {
	ctx := context.Background()
	_, parent, _, run, v1, checkpoint, _ := r21EvolvedTriggerSource(t)
	v2, err := policy.LoadV2("../../../docs/architecture/canonical-codec-v2-bundle")
	if err != nil {
		t.Fatal(err)
	}
	for _, boundary := range []string{"fork_trigger_imports", "trigger_definitions", "trigger_runtime", "trigger_activations"} {
		t.Run(boundary, func(t *testing.T) {
			path := t.TempDir() + "/rollback.db"
			child, err := sqlite.Open(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			defer child.Close()
			db, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if _, err := db.ExecContext(ctx, "CREATE TRIGGER stop_import BEFORE INSERT ON "+boundary+" BEGIN SELECT RAISE(ABORT,'injected trigger import crash'); END"); err != nil {
				t.Fatal(err)
			}
			childRun := [32]byte(run)
			childRun[0] ^= 0x74
			if _, err := parent.ForkIdleSnapshotV1ToV2(ctx, [32]byte(run), checkpoint.ID, childRun, child, v1, v2); err == nil || !strings.Contains(err.Error(), "injected trigger import crash") {
				t.Fatal("fault not reached:", err)
			}
			for _, table := range []string{"genesis_records", "fork_lineage", "fork_history_records", "fork_trigger_imports", "clock_states", "world_revisions", "trigger_definitions", "trigger_registry_seals", "trigger_runtime", "trigger_activations", "input_ledger", "decision_ledger", "event_store", "evidence_ledger", "causal_outbox", "scheduled_occurrences"} {
				var count int
				if err := db.QueryRowContext(ctx, "SELECT count(*) FROM "+table).Scan(&count); err != nil {
					t.Fatal(err)
				}
				if count != 0 {
					t.Fatalf("%s crash left %d rows in %s", boundary, count, table)
				}
			}
			if _, err := db.ExecContext(ctx, "DROP TRIGGER stop_import"); err != nil {
				t.Fatal(err)
			}
			if _, err := parent.ForkIdleSnapshotV1ToV2(ctx, [32]byte(run), checkpoint.ID, childRun, child, v1, v2); err != nil {
				t.Fatal("retry:", err)
			}
			if _, err := child.ReplayClock(ctx, childRun, v2); err != nil {
				t.Fatal(err)
			}
		})
	}
	if _, err := parent.VerifySnapshotV1(ctx, [32]byte(run), checkpoint.ID, v1); err != nil {
		t.Fatal(err)
	}
}

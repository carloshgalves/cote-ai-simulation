package commit

import (
	"context"
	"database/sql"
	"testing"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/admission"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/inputledger"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

func TestCSFRV18R19V2MigratesEventOwnedRoundAlongsideGenesisRound(t *testing.T) {
	ctx := context.Background()
	c, parent, ledger, _, _, sourcePath := noProposalWorkFixture(t)
	_, _, _, _, _, run := ledger.Snapshot()
	state, err := parent.LoadClock(ctx, [32]byte(run))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.CloseSource(ctx, "driver", inputledger.Closure{ProducerKey: "event-round-final", ClosedThrough: domain.EligibilityCoordinate{NotBeforeInstant: state.Instant + 1000}, Final: true}); err != nil {
		t.Fatal(err)
	}
	frozen, err := parent.CreateSnapshotV1(ctx, [32]byte(run), c.bundle, nil)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err := c.ResumeRegisteredWorkV1(ctx, ledger, [32]byte(run), frozen.ID); err != nil {
			t.Fatal(err)
		}
	}
	state, err = parent.LoadClock(ctx, [32]byte(run))
	if err != nil {
		t.Fatal(err)
	}
	root, err := parent.ValidatedGenesis(ctx, [32]byte(run), c.bundle)
	if err != nil {
		t.Fatal(err)
	}
	rawDB, err := sql.Open("sqlite", sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	defer rawDB.Close()
	var eventID, eventRaw []byte
	if err := rawDB.QueryRowContext(ctx, "SELECT id,canonical FROM event_store ORDER BY id LIMIT 1").Scan(&eventID, &eventRaw); err != nil {
		t.Fatal(err)
	}
	value, err := codec.StrictDecodePayload(eventRaw)
	if err != nil {
		t.Fatal(err)
	}
	event := value.([]any)
	ledger, err = inputledger.Open(ctx, run, c.bundle, parent)
	if err != nil {
		t.Fatal(err)
	}
	cid, _, err := identity.New(c.bundle).Sum("cote.csf.id.cycle", "cote.csf.schema.cycle-id-preimage", 1, []any{run[:], []any{int64(state.Instant), uint64(state.LastOrdinal + 1)}})
	if err != nil {
		t.Fatal(err)
	}
	for i, origin := range []any{[]any{uint64(0), []any{eventID, event[18]}}, []any{uint64(2), []any{root.ID[:], root.Digest[:]}}} {
		actor := []string{"a", "b"}[i]
		// Same local role/ordinal is legal because the two source origins differ.
		round, err := ledger.DeclareRound(ctx, "schedule", [2]string{"school", actor}, []string{"act"}, domain.EligibilityCoordinate{NotBeforeInstant: state.Instant, NotBeforeCycleOrdinal: domain.CycleOrdinal(state.LastOrdinal + 1)}, origin, 2, 0, 0, []any{uint64(0)})
		if err != nil {
			t.Fatal(err)
		}
		d := r19Dispatch(t, c.bundle, run, domain.ID(cid), round, "act", 0, state.Revision, state.Instant)
		if err := ledger.RegisterDispatch(ctx, d, inputledger.CycleBinding{ID: d.CycleID, BaseRevision: d.BaseRevision, Instant: d.EffectiveAt}); err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			for ordinal := uint32(1); ordinal <= 2; ordinal++ {
				if err := ledger.RevokeDispatch(ctx, round.ID, "act", 0, []any{uint64(0)}); err != nil {
					t.Fatal(err)
				}
				d = r19Dispatch(t, c.bundle, run, domain.ID(cid), round, "act", ordinal, state.Revision, state.Instant)
				if err := ledger.RegisterDispatch(ctx, d, inputledger.CycleBinding{ID: d.CycleID, BaseRevision: d.BaseRevision, Instant: d.EffectiveAt}); err != nil {
					t.Fatal(err)
				}
			}
		}
		if err := ledger.Respond(ctx, r19NoProposal(t, c.bundle, d, 0)); err != nil {
			t.Fatal(err)
		}
	}
	snap, err := parent.CreateSnapshotV1(ctx, [32]byte(run), c.bundle, csfrv18SyntheticActorCheckpoints(t, sourcePath, state.Instant, int64(state.Revision)))
	if err != nil {
		t.Fatal(err)
	}
	v2, err := policy.LoadV2("../../../docs/architecture/canonical-codec-v2-bundle")
	if err != nil {
		t.Fatal(err)
	}
	childID := [32]byte(run)
	childID[0] ^= 0x7c
	path := t.TempDir() + "/event-round.db"
	child, err := sqlite.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	childDB, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer childDB.Close()
	if _, err := childDB.ExecContext(ctx, `CREATE TRIGGER event_round_fault BEFORE INSERT ON input_slot_responses BEGIN SELECT RAISE(ABORT,'event round response crash'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := parent.ForkIdleSnapshotV1ToV2(ctx, [32]byte(run), snap.ID, childID, child, c.bundle, v2); err == nil {
		t.Fatal("event round bootstrap did not roll back")
	}
	assertCSFRV18EmptyRejectedFork(t, path)
	if _, err := childDB.ExecContext(ctx, "DROP TRIGGER event_round_fault"); err != nil {
		t.Fatal(err)
	}
	if _, err := parent.ForkIdleSnapshotV1ToV2(ctx, [32]byte(run), snap.ID, childID, child, c.bundle, v2); err != nil {
		t.Fatal("event-owned round migration:", err)
	}
	if err := child.Close(); err != nil {
		t.Fatal(err)
	}
	child, err = sqlite.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer child.Close()
	restored, err := inputledger.Open(ctx, domain.ID(childID), v2, child)
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, rounds, responses, _ := restored.Snapshot()
	if len(rounds) != 2 {
		t.Fatal("migration omitted a round")
	}
	if rounds[0].ID == rounds[1].ID || rounds[0].LocalOrdinal == rounds[1].LocalOrdinal {
		t.Fatal("distinct historical origins aliased in child")
	}
	for _, round := range rounds {
		if round.Actor[1] == "a" {
			response := responses[round.ID]["act"]
			expected := r19Dispatch(t, v2, domain.ID(childID), response.CycleID, round, "act", 2, 0, state.Instant)
			if response.DispatchID != expected.ID {
				t.Fatal("source revocation/redispatch ordinal chain was not migrated")
			}
		}
	}
	refs, err := policiesByRole(v2)
	if err != nil {
		t.Fatal(err)
	}
	policies := admission.Policies{AdmissionOrder: refs["admission-order"], CausalIdentity: refs["causal-identity"], Fence: refs["admission-fence"], Rules: [][]any{}}
	childRoot, err := child.ValidatedGenesis(ctx, childID, v2)
	if err != nil {
		t.Fatal(err)
	}
	var hash domain.Digest
	copy(hash[:], childRoot.Payload[16].([]any)[3].([]byte))
	cycle := admission.Cycle{ID: responses[rounds[0].ID]["act"].CycleID, Instant: state.Instant, BaseStateHash: hash, RetryRef: []any{uint64(0)}}
	fence, err := admission.NewCoordinator(admission.NewFenceLog(child)).AdmitThenEvaluate(ctx, v2, restored, cycle, policies, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := New(child, v2, policies).CommitNoProposal(ctx, restored, cycle, fence); err != nil {
		t.Fatal(err)
	}
	if _, err := child.ReplayClock(ctx, childID, v2); err != nil {
		t.Fatal(err)
	}
	if _, err := parent.VerifySnapshotV1(ctx, [32]byte(run), snap.ID, c.bundle); err != nil {
		t.Fatal(err)
	}
	if _, err := childDB.ExecContext(ctx, "DELETE FROM fork_history_records WHERE child_run=? AND ledger_name='event_store' AND record_id=?", childID[:], eventID); err != nil {
		t.Fatal(err)
	}
	if _, err := child.ReplayClock(ctx, childID, v2); err == nil {
		t.Fatal("event-owned round survived deletion of causal origin")
	}
}

func TestCSFRV18R19V2RejectsRoundWithMissingEventOrigin(t *testing.T) {
	ctx := context.Background()
	parent, ledger, v1, run, _ := csfrv18GenesisRoundSource(t)
	state, err := parent.LoadClock(ctx, [32]byte(run))
	if err != nil {
		t.Fatal(err)
	}
	fake := [32]byte(run)
	fake[0] ^= 0x47
	if _, err := ledger.DeclareRound(ctx, "event.schedule", [2]string{"school", "a"}, []string{"act"}, domain.EligibilityCoordinate{NotBeforeInstant: state.Instant + 5}, []any{uint64(0), []any{fake[:], fake[:]}}, 1, 0, 0, []any{uint64(0)}); err != nil {
		t.Fatal(err)
	}
	snap, err := parent.CreateSnapshotV1(ctx, [32]byte(run), v1, nil)
	if err != nil {
		t.Fatal(err)
	}
	v2, err := policy.LoadV2("../../../docs/architecture/canonical-codec-v2-bundle")
	if err != nil {
		t.Fatal(err)
	}
	path := t.TempDir() + "/missing-event.db"
	child, err := sqlite.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer child.Close()
	childID := [32]byte(run)
	childID[0] ^= 0x7d
	if _, err := parent.ForkIdleSnapshotV1ToV2(ctx, [32]byte(run), snap.ID, childID, child, v1, v2); err == nil {
		t.Fatal("round acquired a fictional Event origin")
	}
	assertCSFRV18EmptyRejectedFork(t, path)
}

package commit

import (
	"context"
	"database/sql"
	"testing"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/admission"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/genesis"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/inputledger"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

func r20SeedOccurrence(t *testing.T, s *sqlite.Store, g *genesis.Genesis, b *policy.Bundle, ordinal uint64, due int64, recurring bool, roles ...uint64) {
	t.Helper()
	role := uint64(0)
	if len(roles) > 0 {
		role = roles[0]
	}
	origin := []any{uint64(2), []any{g.ID[:], g.Digest[:]}}
	id, _, err := identity.New(b).Sum("cote.csf.id.occurrence", "cote.csf.schema.occurrence-id-preimage", 1, []any{origin, role, ordinal})
	if err != nil {
		t.Fatal(err)
	}
	recurrence := []any{uint64(0)}
	if recurring {
		recurrence = []any{uint64(1), []any{uint64(0), int64(100), []any{uint64(0)}}}
	}
	f := []any{id[:], g.RunID[:], "schedule", due, []any{due, uint64(0)}, "test.occurrence", uint64(1), g.Payload[16], uint64(0), origin, role, ordinal, recurrence, []any{uint64(0)}}
	digest, _, err := identity.New(b).Sum("cote.csf.digest.occurrence-unit", "cote.csf.schema.scheduled-occurrence.body", 1, f)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := codec.CanonicalPayload(append(f, digest[:]))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SeedGenesisOccurrence(context.Background(), domain.ID(id), g.RunID, domain.EligibilityCoordinate{NotBeforeInstant: domain.SimulationInstant(due)}, raw); err != nil {
		t.Fatal(err)
	}
}

func TestCSFRV18R20EventOccurrencesCoexistWithGenesisAndRound(t *testing.T) {
	ctx := context.Background()
	b, err := policy.Load(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	c, parent, ledger, cycle, fence, sourcePath := systemWorkFixture(t, "defer", func(s *sqlite.Store, g *genesis.Genesis) {
		r20SeedOccurrence(t, s, g, b, 1, int64(g.InitialInstant), true)
		r20SeedOccurrence(t, s, g, b, 2, int64(g.InitialInstant)+150, false)
	})
	_, _, _, _, _, run := ledger.Snapshot()
	if _, err := c.DispatchFencedWorkV1(ctx, ledger, cycle, fence); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, "SELECT id FROM causal_outbox")
	if err != nil {
		t.Fatal(err)
	}
	var tasks [][32]byte
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			t.Fatal(err)
		}
		var id [32]byte
		copy(id[:], raw)
		tasks = append(tasks, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	rows.Close()
	for _, id := range tasks {
		if _, err := parent.CompleteInternalPerception(ctx, id, b); err != nil {
			t.Fatal(err)
		}
	}
	state, err := parent.LoadClock(ctx, [32]byte(run))
	if err != nil {
		t.Fatal(err)
	}
	root, err := parent.ValidatedGenesis(ctx, [32]byte(run), b)
	if err != nil {
		t.Fatal(err)
	}
	ledger, err = inputledger.Open(ctx, run, b, parent)
	if err != nil {
		t.Fatal(err)
	}
	round, err := ledger.DeclareRound(ctx, "schedule", [2]string{"school", "a"}, []string{"act"}, domain.EligibilityCoordinate{NotBeforeInstant: state.Instant, NotBeforeCycleOrdinal: domain.CycleOrdinal(state.LastOrdinal + 1)}, []any{uint64(2), []any{root.ID[:], root.Digest[:]}}, 5, 0, 0, []any{uint64(0)})
	if err != nil {
		t.Fatal(err)
	}
	cid, _, err := identity.New(b).Sum("cote.csf.id.cycle", "cote.csf.schema.cycle-id-preimage", 1, []any{run[:], []any{int64(state.Instant), uint64(state.LastOrdinal + 1)}})
	if err != nil {
		t.Fatal(err)
	}
	d := r19Dispatch(t, b, run, domain.ID(cid), round, "act", 0, state.Revision, state.Instant)
	if err := ledger.RegisterDispatch(ctx, d, inputledger.CycleBinding{ID: d.CycleID, BaseRevision: d.BaseRevision, Instant: d.EffectiveAt}); err != nil {
		t.Fatal(err)
	}
	if err := ledger.Respond(ctx, r19NoProposal(t, b, d, 0)); err != nil {
		t.Fatal(err)
	}
	snap, err := parent.CreateSnapshotV1(ctx, [32]byte(run), b, csfrv18SyntheticActorCheckpoints(t, sourcePath, state.Instant, int64(state.Revision)))
	if err != nil {
		t.Fatal(err)
	}
	v2, err := policy.LoadV2("../../../docs/architecture/canonical-codec-v2-bundle")
	if err != nil {
		t.Fatal(err)
	}
	path := t.TempDir() + "/event-occurrences.db"
	child, err := sqlite.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { child.Close() })
	childID := [32]byte(run)
	childID[0] ^= 0x67
	probe, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer probe.Close()
	if _, err := probe.ExecContext(ctx, "CREATE TRIGGER occurrence_proof_fault BEFORE INSERT ON fork_occurrence_imports BEGIN SELECT RAISE(ABORT,'occurrence proof crash'); END"); err != nil {
		t.Fatal(err)
	}
	if _, err := parent.ForkIdleSnapshotV1ToV2(ctx, [32]byte(run), snap.ID, childID, child, b, v2); err == nil {
		t.Fatal("occurrence proof crash ignored")
	}
	assertCSFRV18EmptyRejectedFork(t, path)
	if _, err := probe.ExecContext(ctx, "DROP TRIGGER occurrence_proof_fault"); err != nil {
		t.Fatal(err)
	}
	childRoot, err := parent.ForkIdleSnapshotV1ToV2(ctx, [32]byte(run), snap.ID, childID, child, b, v2)
	if err != nil {
		t.Fatal("Event-owned occurrence migration:", err)
	}
	if err := child.Close(); err != nil {
		t.Fatal(err)
	}
	child, err = sqlite.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := child.ReplayClock(ctx, childID, v2); err != nil {
		t.Fatal(err)
	}
	pending, err := child.PendingClock(ctx, childID)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 4 {
		t.Fatalf("lost a migrated authority: %d", len(pending))
	}
	childDB, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer childDB.Close()
	rows, err = childDB.QueryContext(ctx, "SELECT id,canonical FROM scheduled_occurrences ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}
	ordinals := map[uint64]bool{}
	for rows.Next() {
		var id, raw []byte
		if err := rows.Scan(&id, &raw); err != nil {
			t.Fatal(err)
		}
		value, err := codec.StrictDecodePayload(raw)
		if err != nil {
			t.Fatal(err)
		}
		f := value.([]any)
		if !sameTestCanonical(f[9], []any{uint64(2), []any{childRoot.ID[:], childRoot.Digest[:]}}) {
			t.Fatal("source Event retained child authority")
		}
		ord := f[11].(uint64)
		if ordinals[ord] {
			t.Fatal("Event/Genesis coordinate alias")
		}
		ordinals[ord] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	rows.Close()
	if len(ordinals) != 3 || !ordinals[0] || !ordinals[1] || !ordinals[2] {
		t.Fatal("noncanonical migration allocation", ordinals)
	}
	restored, err := inputledger.Open(ctx, domain.ID(childID), v2, child)
	if err != nil {
		t.Fatal(err)
	}
	clock, err := child.LoadClock(ctx, childID)
	if err != nil {
		t.Fatal(err)
	}
	_, hash, err := child.LoadWorldState(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	childCycle, _, err := identity.New(v2).Sum("cote.csf.id.cycle", "cote.csf.schema.cycle-id-preimage", 1, []any{childID[:], []any{int64(clock.Instant), uint64(0)}})
	if err != nil {
		t.Fatal(err)
	}
	cc := admission.Cycle{ID: domain.ID(childCycle), Instant: clock.Instant, BaseStateHash: domain.Digest(hash), RetryRef: []any{uint64(0)}}
	sealed, err := admission.NewCoordinator(admission.NewFenceLog(child)).AdmitThenEvaluate(ctx, v2, restored, cc, c.admissionPolicies, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := New(child, v2, c.admissionPolicies).CommitNoProposal(ctx, restored, cc, sealed); err != nil {
		t.Fatal(err)
	}
	if _, err := child.ReplayClock(ctx, childID, v2); err != nil {
		t.Fatal(err)
	}
	clock, err = child.LoadClock(ctx, childID)
	if err != nil {
		t.Fatal(err)
	}
	barrier, err := child.CreateSnapshotV1(ctx, childID, v2, csfrv18SyntheticActorCheckpoints(t, path, clock.Instant, int64(clock.Revision)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := child.DrainPendingPerceptionV1(ctx, childID, barrier.ID, v2); err != nil {
		t.Fatal(err)
	}
	coordinator := New(child, v2, c.admissionPolicies)
	if _, err := coordinator.Advance(ctx, restored, domain.ID(childID)); err != nil {
		t.Fatal(err)
	}
	if _, err := child.DrainPendingPerceptionV1(ctx, childID, barrier.ID, v2); err != nil {
		t.Fatal(err)
	}
	clock, err = child.LoadClock(ctx, childID)
	if err != nil {
		t.Fatal(err)
	}
	_, hash, err = child.LoadWorldState(ctx, uint64(clock.Revision))
	if err != nil {
		t.Fatal(err)
	}
	workID, _, err := identity.New(v2).Sum("cote.csf.id.cycle", "cote.csf.schema.cycle-id-preimage", 1, []any{childID[:], []any{int64(clock.Instant), uint64(clock.LastOrdinal + 1)}})
	if err != nil {
		t.Fatal(err)
	}
	work := admission.Cycle{ID: domain.ID(workID), Instant: clock.Instant, Ordinal: domain.CycleOrdinal(clock.LastOrdinal + 1), BaseRevision: clock.Revision, BaseStateHash: domain.Digest(hash), RetryRef: []any{uint64(0)}}
	frozen, err := admission.NewCoordinator(admission.NewFenceLog(child)).AdmitThenEvaluate(ctx, v2, restored, work, c.admissionPolicies, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(frozen.Fields[11].([]any)) != 2 {
		t.Fatal("migrated recurring occurrences not jointly eligible")
	}
	if _, err := coordinator.DispatchFencedWorkV1(ctx, restored, work, frozen); err != nil {
		t.Fatal("migrated Event births did not settle:", err)
	}
	if _, err := child.ReplayClock(ctx, childID, v2); err != nil {
		t.Fatal(err)
	}
	// Audit both an exact content mutation and deletion of a required Event proof.
	rows, err = childDB.QueryContext(ctx, "SELECT parent_occurrence_id,canonical FROM fork_occurrence_imports WHERE child_run=?", childID[:])
	if err != nil {
		t.Fatal(err)
	}
	var sourceID, sourceRaw []byte
	var genesisID, genesisRaw []byte
	for rows.Next() {
		var id, raw []byte
		if err := rows.Scan(&id, &raw); err != nil {
			t.Fatal(err)
		}
		value, err := codec.StrictDecodePayload(raw)
		if err != nil {
			t.Fatal(err)
		}
		if value.([]any)[9].([]any)[0] == uint64(2) {
			genesisID, genesisRaw = id, raw
		}
		if sourceID == nil && value.([]any)[9].([]any)[0] == uint64(0) {
			sourceID = id
			sourceRaw = raw
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	rows.Close()
	if sourceID == nil {
		t.Fatal("Event birth proof absent")
	}
	if genesisID == nil {
		t.Fatal("Genesis source proof absent")
	}
	if _, err := childDB.ExecContext(ctx, "DELETE FROM fork_occurrence_imports WHERE child_run=? AND parent_occurrence_id=?", childID[:], genesisID); err != nil {
		t.Fatal(err)
	}
	if _, err := child.ReplayClock(ctx, childID, v2); err != nil {
		t.Fatal("legacy Genesis proof reconstruction failed:", err)
	}
	if _, err := childDB.ExecContext(ctx, "INSERT INTO fork_occurrence_imports(child_run,parent_occurrence_id,canonical) VALUES(?,?,?)", childID[:], genesisID, genesisRaw); err != nil {
		t.Fatal(err)
	}
	value, err := codec.StrictDecodePayload(sourceRaw)
	if err != nil {
		t.Fatal(err)
	}
	fields := value.([]any)
	fields[14].([]byte)[0] ^= 1
	corrupt, err := codec.CanonicalPayload(fields)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := childDB.ExecContext(ctx, "UPDATE fork_occurrence_imports SET canonical=? WHERE child_run=? AND parent_occurrence_id=?", corrupt, childID[:], sourceID); err != nil {
		t.Fatal(err)
	}
	if _, err := child.ReplayClock(ctx, childID, v2); err == nil {
		t.Fatal("corrupt Event occurrence proof accepted")
	}
	if _, err := childDB.ExecContext(ctx, "UPDATE fork_occurrence_imports SET canonical=? WHERE child_run=? AND parent_occurrence_id=?", sourceRaw, childID[:], sourceID); err != nil {
		t.Fatal(err)
	}
	if _, err := child.ReplayClock(ctx, childID, v2); err != nil {
		t.Fatal("exact proof restoration failed:", err)
	}
	if _, err := childDB.ExecContext(ctx, "DELETE FROM fork_occurrence_imports WHERE child_run=? AND parent_occurrence_id=?", childID[:], sourceID); err != nil {
		t.Fatal(err)
	}
	if _, err := child.ReplayClock(ctx, childID, v2); err == nil {
		t.Fatal("missing Event occurrence proof accepted")
	}
	if _, err := childDB.ExecContext(ctx, "INSERT INTO fork_occurrence_imports(child_run,parent_occurrence_id,canonical) VALUES(?,?,?)", childID[:], sourceID, sourceRaw); err != nil {
		t.Fatal(err)
	}
	if _, err := child.ReplayClock(ctx, childID, v2); err != nil {
		t.Fatal(err)
	}
}

func r20CurrentWork(t *testing.T, c *Coordinator, s *sqlite.Store, l *inputledger.Ledger, run domain.ID) (admission.Cycle, admission.Fence) {
	t.Helper()
	ctx := context.Background()
	state, err := s.LoadClock(ctx, [32]byte(run))
	if err != nil {
		t.Fatal(err)
	}
	_, hash, err := s.LoadWorldState(ctx, uint64(state.Revision))
	if err != nil {
		t.Fatal(err)
	}
	id, _, err := identity.New(c.bundle).Sum("cote.csf.id.cycle", "cote.csf.schema.cycle-id-preimage", 1, []any{run[:], []any{int64(state.Instant), uint64(state.LastOrdinal + 1)}})
	if err != nil {
		t.Fatal(err)
	}
	cycle := admission.Cycle{ID: domain.ID(id), Instant: state.Instant, Ordinal: domain.CycleOrdinal(state.LastOrdinal + 1), BaseRevision: state.Revision, BaseStateHash: domain.Digest(hash), RetryRef: []any{uint64(0)}}
	fence, err := admission.NewCoordinator(admission.NewFenceLog(s)).AdmitThenEvaluate(ctx, c.bundle, l, cycle, c.admissionPolicies, nil)
	if err != nil {
		t.Fatal(err)
	}
	return cycle, fence
}

func TestCSFRV18R20EventClaimAndRoundShareTransactionalBootstrap(t *testing.T) {
	ctx := context.Background()
	c, parent, ledger, cycle, fence, sourcePath := communicationFixtureWithChannel(t, "school.message.v1", func(s *sqlite.Store, g *genesis.Genesis, b *policy.Bundle) {
		r20SeedOccurrence(t, s, g, b, 1, int64(g.InitialInstant)+1, true, 65535)
		r20SeedOccurrence(t, s, g, b, 0, int64(g.InitialInstant)+150, false, 65535)
	})
	_, _, _, _, _, run := ledger.Snapshot()
	if _, err := c.CommitCommunicationSend(ctx, ledger, cycle, fence); err != nil {
		t.Fatal(err)
	}
	csfrv18SettleSendOutbox(t, parent, c.bundle, sourcePath)
	state, err := parent.LoadClock(ctx, [32]byte(run))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.CloseSource(ctx, "driver", inputledger.Closure{ProducerKey: "r20-final", ClosedThrough: domain.EligibilityCoordinate{NotBeforeInstant: state.Instant + 1000}, Final: true}); err != nil {
		t.Fatal(err)
	}
	before, err := parent.CreateSnapshotV1(ctx, [32]byte(run), c.bundle, csfrv18SyntheticActorCheckpoints(t, sourcePath, state.Instant, int64(state.Revision)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Advance(ctx, ledger, run); err != nil {
		t.Fatal(err)
	}
	if _, err := parent.DrainPendingPerceptionV1(ctx, [32]byte(run), before.ID, c.bundle); err != nil {
		t.Fatal(err)
	}
	cycle, fence = r20CurrentWork(t, c, parent, ledger, run)
	if _, err := c.DispatchFencedWorkV1(ctx, ledger, cycle, fence); err != nil {
		t.Fatal(err)
	}
	if _, err := parent.DrainPendingPerceptionV1(ctx, [32]byte(run), before.ID, c.bundle); err != nil {
		t.Fatal(err)
	}
	state, err = parent.LoadClock(ctx, [32]byte(run))
	if err != nil {
		t.Fatal(err)
	}
	root, err := parent.ValidatedGenesis(ctx, [32]byte(run), c.bundle)
	if err != nil {
		t.Fatal(err)
	}
	ledger, err = inputledger.Open(ctx, run, c.bundle, parent)
	if err != nil {
		t.Fatal(err)
	}
	checkpoints := func() []sqlite.SnapshotCheckpoint {
		return csfrv18SyntheticActorCheckpoints(t, sourcePath, state.Instant, int64(state.Revision))
	}
	_, _, _, snap := r19SourceWithActionOn(t, parent, ledger, c.bundle, run, []any{uint64(2), []any{root.ID[:], root.Digest[:]}}, false, []any{uint64(0)}, checkpoints)
	v2, err := policy.LoadV2("../../../docs/architecture/canonical-codec-v2-bundle")
	if err != nil {
		t.Fatal(err)
	}
	path := t.TempDir() + "/combined-occurrences.db"
	child, err := sqlite.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { child.Close() })
	childID := [32]byte(run)
	childID[0] ^= 0x58
	if _, err := parent.ForkIdleSnapshotV1ToV2(ctx, [32]byte(run), snap.ID, childID, child, c.bundle, v2); err != nil {
		t.Fatal("combined Event/Claim/round migration:", err)
	}
	if err := child.Close(); err != nil {
		t.Fatal(err)
	}
	child, err = sqlite.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := child.ReplayClock(ctx, childID, v2); err != nil {
		t.Fatal(err)
	}
	pending, err := child.PendingClock(ctx, childID)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 4 {
		t.Fatal("combined bootstrap dropped pending authority", len(pending))
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, table := range []string{"event_store", "evidence_ledger", "causal_outbox"} {
		var count int
		if err := db.QueryRowContext(ctx, "SELECT count(*) FROM "+table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatal("import fabricated send or knowledge", table)
		}
	}
	rows, err := db.QueryContext(ctx, "SELECT canonical FROM scheduled_occurrences")
	if err != nil {
		t.Fatal(err)
	}
	ordinals := map[uint64]bool{}
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			t.Fatal(err)
		}
		value, err := codec.StrictDecodePayload(raw)
		if err != nil {
			t.Fatal(err)
		}
		f := value.([]any)
		if f[10] != uint64(65535) {
			t.Fatal("migration role changed")
		}
		ord := f[11].(uint64)
		if ordinals[ord] {
			t.Fatal("communication/Genesis/Event coordinate collision")
		}
		ordinals[ord] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	rows.Close()
	if len(ordinals) != 3 || !ordinals[0] || !ordinals[1] || !ordinals[2] {
		t.Fatal("noncanonical reserved-coordinate migration", ordinals)
	}
	childState, err := child.LoadClock(ctx, childID)
	if err != nil {
		t.Fatal(err)
	}
	barrier, err := child.CreateSnapshotV1(ctx, childID, v2, csfrv18SyntheticActorCheckpoints(t, path, childState.Instant, int64(childState.Revision)))
	if err != nil {
		t.Fatal(err)
	}
	restored, err := inputledger.Open(ctx, domain.ID(childID), v2, child)
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, rounds, _, _ := restored.Snapshot()
	r := rounds[0]
	id, _, err := identity.New(v2).Sum("cote.csf.id.cycle", "cote.csf.schema.cycle-id-preimage", 1, []any{childID[:], []any{int64(childState.Instant), uint64(0)}})
	if err != nil {
		t.Fatal(err)
	}
	open := r19Dispatch(t, v2, domain.ID(childID), domain.ID(id), r, "open", 0, 0, childState.Instant)
	if err := restored.Respond(ctx, r19NoProposal(t, v2, open, 0)); err != nil {
		t.Fatal(err)
	}
	replacement := r19Dispatch(t, v2, domain.ID(childID), domain.ID(id), r, "revoked", 1, 0, childState.Instant)
	if err := restored.RegisterDispatch(ctx, replacement, inputledger.CycleBinding{ID: replacement.CycleID, Instant: replacement.EffectiveAt}); err != nil {
		t.Fatal(err)
	}
	if err := restored.Respond(ctx, r19NoProposal(t, v2, replacement, 0)); err != nil {
		t.Fatal(err)
	}
	coordinator := New(child, v2, c.admissionPolicies)
	cycle, fence = r20CurrentWork(t, coordinator, child, restored, domain.ID(childID))
	if _, err := coordinator.DispatchFencedWorkV1(ctx, restored, cycle, fence); err != nil {
		t.Fatal(err)
	}
	if _, err := child.DrainPendingPerceptionV1(ctx, childID, barrier.ID, v2); err != nil {
		t.Fatal(err)
	}
	if _, err := coordinator.Advance(ctx, restored, domain.ID(childID)); err != nil {
		t.Fatal(err)
	}
	if _, err := child.DrainPendingPerceptionV1(ctx, childID, barrier.ID, v2); err != nil {
		t.Fatal(err)
	}
	cycle, fence = r20CurrentWork(t, coordinator, child, restored, domain.ID(childID))
	if _, err := coordinator.DispatchFencedWorkV1(ctx, restored, cycle, fence); err != nil {
		t.Fatal("communication due lost after round settlement:", err)
	}
	if _, err := child.DrainPendingPerceptionV1(ctx, childID, barrier.ID, v2); err != nil {
		t.Fatal(err)
	}
	if _, err := child.ReplayClock(ctx, childID, v2); err != nil {
		t.Fatal(err)
	}
	var delivered int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM communication_transmissions WHERE run_id=? AND status=1", childID[:]).Scan(&delivered); err != nil {
		t.Fatal(err)
	}
	if delivered != 1 {
		t.Fatal("migrated communication was not delivered exactly once")
	}
}

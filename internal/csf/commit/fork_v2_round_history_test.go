package commit

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/admission"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/extensions/testresourcev1"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/inputledger"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

func r19NoProposal(t *testing.T, b *policy.Bundle, d inputledger.Dispatch, reason uint64) inputledger.Response {
	t.Helper()
	id, _, err := identity.New(b).Sum("cote.csf.id.no-proposal", "cote.csf.schema.slot-response-id-preimage", 1, []any{d.ID[:], uint64(1)})
	if err != nil {
		t.Fatal(err)
	}
	return inputledger.Response{Kind: 1, ID: domain.ID(id), DispatchID: d.ID, RunID: d.RunID, CycleID: d.CycleID,
		RoundID: d.RoundID, SlotID: d.SlotID, Actor: d.Actor, EffectiveAt: d.EffectiveAt, SubmittedAgainstRevision: d.BaseRevision,
		Body: []any{id[:], d.ID[:], d.RunID[:], d.CycleID[:], d.RoundID[:], d.SlotID, []any{d.Actor[0], d.Actor[1]}, int64(d.EffectiveAt), uint64(d.BaseRevision), reason}}
}

func r19Dispatch(t *testing.T, b *policy.Bundle, run, cycle domain.ID, r inputledger.Round, slot string, ordinal uint32, revision domain.WorldRevision, at domain.SimulationInstant) inputledger.Dispatch {
	t.Helper()
	id, _, err := identity.New(b).Sum("cote.csf.id.dispatch", "cote.csf.schema.dispatch-id-preimage", 1, []any{run[:], cycle[:], r.ID[:], slot, uint64(ordinal)})
	if err != nil {
		t.Fatal(err)
	}
	return inputledger.Dispatch{ID: domain.ID(id), RunID: run, CycleID: cycle, RoundID: r.ID, SlotID: slot, Ordinal: ordinal, Actor: r.Actor, Eligibility: r.Eligibility, BaseRevision: revision, EffectiveAt: at}
}

func r19Source(t *testing.T) (*sqlite.Store, *policy.Bundle, domain.ID, sqlite.SnapshotV1) {
	return r19SourceWithAction(t, false)
}
func r19SourceWithAction(t *testing.T, action bool, intention ...any) (*sqlite.Store, *policy.Bundle, domain.ID, sqlite.SnapshotV1) {
	t.Helper()
	parent, ledger, v1, run, origin := csfrv18GenesisRoundSource(t)
	return r19SourceWithActionOn(t, parent, ledger, v1, run, origin, action, intention...)
}

func r19SourceWithActionOn(t *testing.T, parent *sqlite.Store, ledger *inputledger.Ledger, v1 *policy.Bundle, run domain.ID, origin []any, action bool, intention ...any) (*sqlite.Store, *policy.Bundle, domain.ID, sqlite.SnapshotV1) {
	t.Helper()
	ctx := context.Background()
	state, err := parent.LoadClock(ctx, [32]byte(run))
	if err != nil {
		t.Fatal(err)
	}
	nextOrdinal := uint64(state.LastOrdinal + 1)
	round, err := ledger.DeclareRound(ctx, "genesis.schedule", [2]string{"school", "a"}, []string{"answered", "open", "revoked"},
		domain.EligibilityCoordinate{NotBeforeInstant: state.Instant, NotBeforeCycleOrdinal: domain.CycleOrdinal(nextOrdinal)}, origin, 1, 0, 0, []any{uint64(0)})
	if err != nil {
		t.Fatal(err)
	}
	cid, _, err := identity.New(v1).Sum("cote.csf.id.cycle", "cote.csf.schema.cycle-id-preimage", 1, []any{run[:], []any{int64(state.Instant), nextOrdinal}})
	if err != nil {
		t.Fatal(err)
	}
	for _, slot := range round.Slots {
		d := r19Dispatch(t, v1, run, domain.ID(cid), round, slot, 0, state.Revision, state.Instant)
		if err := ledger.RegisterDispatch(ctx, d, inputledger.CycleBinding{ID: d.CycleID, BaseRevision: d.BaseRevision, Instant: d.EffectiveAt}); err != nil {
			t.Fatal(err)
		}
		if slot == "answered" {
			response := r19NoProposal(t, v1, d, 0)
			if action {
				response, err = testresourcev1.ProposalForDispatch(v1, d)
				if err != nil {
					t.Fatal(err)
				}
				if len(intention) > 0 {
					response.Body[16] = intention[0]
					if build, ok := intention[0].(func([]any) any); ok {
						response.Body[16] = build(origin)
					}
					if build, ok := intention[0].(func(inputledger.Round, inputledger.Dispatch) any); ok {
						response.Body[16] = build(round, d)
					}
					if build, ok := intention[0].(func(*sqlite.Store, *policy.Bundle, []any) any); ok {
						response.Body[16] = build(parent, v1, origin)
					}
				}
			}
			if err := ledger.Respond(ctx, response); err != nil {
				t.Fatal(err)
			}
		}
		if slot == "revoked" {
			if err := ledger.RevokeDispatch(ctx, round.ID, slot, 0, []any{uint64(0)}); err != nil {
				t.Fatal(err)
			}
		}
	}
	var checkpoints []sqlite.SnapshotCheckpoint
	if len(intention) > 1 {
		checkpoints, _ = intention[1].([]sqlite.SnapshotCheckpoint)
		if build, ok := intention[1].(func() []sqlite.SnapshotCheckpoint); ok {
			checkpoints = build()
		}
	}
	snap, err := parent.CreateSnapshotV1(ctx, [32]byte(run), v1, checkpoints)
	if err != nil {
		t.Fatal(err)
	}
	return parent, v1, run, snap
}

func TestCSFRV18R19V2RoundHistorySurvivesReopenAndPreventsDuplicateResponse(t *testing.T) {
	ctx := context.Background()
	parent, v1, run, snap := r19Source(t)
	before, err := parent.Records(ctx)
	if err != nil {
		t.Fatal(err)
	}
	v2, err := policy.LoadV2("../../../docs/architecture/canonical-codec-v2-bundle")
	if err != nil {
		t.Fatal(err)
	}
	path := t.TempDir() + "/round-history.db"
	child, err := sqlite.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	childID := [32]byte(run)
	childID[0] ^= 0x69
	if _, err := parent.ForkIdleSnapshotV1ToV2(ctx, [32]byte(run), snap.ID, childID, child, v1, v2); err != nil {
		t.Fatal("partial round migration:", err)
	}
	if _, err := child.ReplayClock(ctx, childID, v2); err != nil {
		t.Fatal(err)
	}
	checkpoint, err := child.CreateSnapshotV1(ctx, childID, v2, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := child.Close(); err != nil {
		t.Fatal(err)
	}
	child, err = sqlite.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer child.Close()
	if _, err := child.VerifySnapshotV1(ctx, childID, checkpoint.ID, v2); err != nil {
		t.Fatal(err)
	}
	restored, err := inputledger.Open(ctx, domain.ID(childID), v2, child)
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, rounds, responses, _ := restored.Snapshot()
	if len(rounds) != 1 || len(responses[rounds[0].ID]) != 1 {
		t.Fatal("partial roster/response lost")
	}
	r := rounds[0]
	accepted := responses[r.ID]["answered"]
	for _, p := range before {
		if [32]byte(accepted.ID) == p.Identity {
			t.Fatal("parent identity survived in child authority")
		}
	}
	// Same bytes are idempotent; changing the answer is forbidden even after restart.
	if err := restored.Respond(ctx, accepted); err != nil {
		t.Fatal(err)
	}
	altered := accepted
	altered.Body = append([]any(nil), accepted.Body...)
	altered.Body[9] = uint64(1)
	if err := restored.Respond(ctx, altered); !errors.Is(err, inputledger.ErrSlotFilled) {
		t.Fatalf("answered slot accepted a second response: %v", err)
	}
	cid, _, err := identity.New(v2).Sum("cote.csf.id.cycle", "cote.csf.schema.cycle-id-preimage", 1, []any{childID[:], []any{int64(r.Eligibility.NotBeforeInstant), uint64(0)}})
	if err != nil {
		t.Fatal(err)
	}
	open := r19Dispatch(t, v2, domain.ID(childID), domain.ID(cid), r, "open", 0, 0, r.Eligibility.NotBeforeInstant)
	if err := restored.Respond(ctx, r19NoProposal(t, v2, open, 0)); err != nil {
		t.Fatal("migrated open capability not usable:", err)
	}
	revoked := r19Dispatch(t, v2, domain.ID(childID), domain.ID(cid), r, "revoked", 0, 0, r.Eligibility.NotBeforeInstant)
	if err := restored.Respond(ctx, r19NoProposal(t, v2, revoked, 0)); !errors.Is(err, inputledger.ErrSlotRevoked) {
		t.Fatalf("revoked capability accepted: %v", err)
	}
	replacement := r19Dispatch(t, v2, domain.ID(childID), domain.ID(cid), r, "revoked", 1, 0, r.Eligibility.NotBeforeInstant)
	if err := restored.RegisterDispatch(ctx, replacement, inputledger.CycleBinding{ID: replacement.CycleID, Instant: replacement.EffectiveAt}); err != nil {
		t.Fatal("revocation ordinal lost:", err)
	}
	if err := restored.Respond(ctx, r19NoProposal(t, v2, replacement, 0)); err != nil {
		t.Fatal(err)
	}
	refs, err := policiesByRole(v2)
	if err != nil {
		t.Fatal(err)
	}
	policies := admission.Policies{AdmissionOrder: refs["admission-order"], CausalIdentity: refs["causal-identity"], Fence: refs["admission-fence"], Rules: [][]any{}}
	root, err := child.ValidatedGenesis(ctx, childID, v2)
	if err != nil {
		t.Fatal(err)
	}
	var hash domain.Digest
	copy(hash[:], root.Payload[16].([]any)[3].([]byte))
	cycle := admission.Cycle{ID: domain.ID(cid), Instant: r.Eligibility.NotBeforeInstant, BaseStateHash: hash, RetryRef: []any{uint64(0)}}
	fence, err := admission.NewCoordinator(admission.NewFenceLog(child)).AdmitThenEvaluate(ctx, v2, restored, cycle, policies, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := New(child, v2, policies).CommitNoProposal(ctx, restored, cycle, fence); err != nil {
		t.Fatal("migrated round terminal:", err)
	}
	if _, err := child.ReplayClock(ctx, childID, v2); err != nil {
		t.Fatal("child terminal replay:", err)
	}
	if _, err := child.VerifySnapshotV1(ctx, childID, checkpoint.ID, v2); err != nil {
		t.Fatal("snapshot plus completed round tail:", err)
	}
	after, err := parent.Records(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != len(after) {
		t.Fatal("child changed parent ledger")
	}
	for i := range before {
		if !bytes.Equal(before[i].Canonical, after[i].Canonical) {
			t.Fatal("parent bytes changed")
		}
	}
	if _, err := parent.VerifySnapshotV1(ctx, [32]byte(run), snap.ID, v1); err != nil {
		t.Fatal(err)
	}
}

func TestCSFRV18R19V2RoundHistoryRollback(t *testing.T) {
	ctx := context.Background()
	parent, v1, run, snap := r19Source(t)
	v2, err := policy.LoadV2("../../../docs/architecture/canonical-codec-v2-bundle")
	if err != nil {
		t.Fatal(err)
	}
	path := t.TempDir() + "/rollback.db"
	child, err := sqlite.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer child.Close()
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	if _, err := raw.ExecContext(ctx, `CREATE TRIGGER r19_fault BEFORE INSERT ON input_slot_responses BEGIN SELECT RAISE(ABORT,'r19 response crash'); END`); err != nil {
		t.Fatal(err)
	}
	childID := [32]byte(run)
	childID[0] ^= 0x6a
	if _, err := parent.ForkIdleSnapshotV1ToV2(ctx, [32]byte(run), snap.ID, childID, child, v1, v2); err == nil {
		t.Fatal("missing rollback")
	}
	assertCSFRV18EmptyRejectedFork(t, path)
	if _, err := raw.ExecContext(ctx, "DROP TRIGGER r19_fault"); err != nil {
		t.Fatal(err)
	}
	if _, err := parent.ForkIdleSnapshotV1ToV2(ctx, [32]byte(run), snap.ID, childID, child, v1, v2); err != nil {
		t.Fatal("retry after crash:", err)
	}
}

func TestCSFRV18R19V2RoundHistoryCorruptionFailsReplay(t *testing.T) {
	for _, mutation := range []string{"delete-response", "alter-dispatch", "delete-revocation", "delete-slot-key"} {
		t.Run(mutation, func(t *testing.T) {
			ctx := context.Background()
			parent, v1, run, snap := r19Source(t)
			v2, err := policy.LoadV2("../../../docs/architecture/canonical-codec-v2-bundle")
			if err != nil {
				t.Fatal(err)
			}
			path := t.TempDir() + "/corrupt.db"
			child, err := sqlite.Open(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			defer child.Close()
			childID := [32]byte(run)
			childID[0] ^= 0x6b
			if _, err := parent.ForkIdleSnapshotV1ToV2(ctx, [32]byte(run), snap.ID, childID, child, v1, v2); err != nil {
				t.Fatal(err)
			}
			records, err := child.Records(ctx)
			if err != nil {
				t.Fatal(err)
			}
			raw, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			defer raw.Close()
			for _, record := range records {
				v, err := codec.StrictDecodePayload(record.Canonical)
				if err != nil {
					t.Fatal(err)
				}
				f := v.([]any)
				switch mutation {
				case "delete-response":
					if len(f) == 11 {
						if _, ok := f[4].([]byte); ok {
							_, err = raw.ExecContext(ctx, "DELETE FROM input_ledger WHERE id=?", record.Identity[:])
						}
					}
				case "alter-dispatch":
					if len(f) == 11 {
						if _, ok := f[4].(string); ok {
							f[10] = bytes.Repeat([]byte{9}, 32)
							altered, e := codec.CanonicalPayload(f)
							if e != nil {
								t.Fatal(e)
							}
							_, err = raw.ExecContext(ctx, "UPDATE input_ledger SET canonical=? WHERE id=?", altered, record.Identity[:])
						}
					}
				case "delete-revocation":
					if len(f) == 3 {
						_, err = raw.ExecContext(ctx, "DELETE FROM input_ledger WHERE id=?", record.Identity[:])
					}
				}
				if err != nil {
					t.Fatal(err)
				}
			}
			if mutation == "delete-slot-key" {
				if _, err := raw.ExecContext(ctx, "DELETE FROM input_slot_responses"); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := child.ReplayClock(ctx, childID, v2); err == nil {
				t.Fatal("corrupted inherited slot history accepted")
			}
		})
	}
}

func TestCSFRV18R19V2ActionResponseIsRebuiltWithoutChangingIntent(t *testing.T) {
	ctx := context.Background()
	parent, v1, run, snap := r19SourceWithAction(t, true)
	original, err := inputledger.Open(ctx, run, v1, parent)
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, rounds, slots, _ := original.Snapshot()
	old := slots[rounds[0].ID]["answered"]
	v2, err := policy.LoadV2("../../../docs/architecture/canonical-codec-v2-bundle")
	if err != nil {
		t.Fatal(err)
	}
	child, err := sqlite.Open(ctx, t.TempDir()+"/action.db")
	if err != nil {
		t.Fatal(err)
	}
	defer child.Close()
	childID := [32]byte(run)
	childID[0] ^= 0x6c
	if _, err := parent.ForkIdleSnapshotV1ToV2(ctx, [32]byte(run), snap.ID, childID, child, v1, v2); err != nil {
		t.Fatal(err)
	}
	ledger, err := inputledger.Open(ctx, domain.ID(childID), v2, child)
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, rounds, slots, _ = ledger.Snapshot()
	got := slots[rounds[0].ID]["answered"]
	if got.Kind != 0 || got.ID == old.ID || got.DispatchID == old.DispatchID || got.RoundID == old.RoundID || got.RunID == old.RunID {
		t.Fatal("action identities were not rebuilt")
	}
	for _, i := range []int{4, 5, 6, 7, 8, 9, 10, 11, 12, 14, 16, 17} {
		a, err := codec.CanonicalPayload(old.Body[i])
		if err != nil {
			t.Fatal(err)
		}
		b, err := codec.CanonicalPayload(got.Body[i])
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(a, b) {
			t.Fatalf("action semantic field %d changed", i)
		}
	}
	if err := ledger.Respond(ctx, got); err != nil {
		t.Fatal(err)
	}
	if _, err := child.ReplayClock(ctx, childID, v2); err != nil {
		t.Fatal(err)
	}
}

func TestCSFRV18R19V2UsesFrozenRoundCutDespiteLaterSourceResponse(t *testing.T) {
	ctx := context.Background()
	parent, v1, run, snap := r19Source(t)
	ledger, err := inputledger.Open(ctx, run, v1, parent)
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, rounds, _, _ := ledger.Snapshot()
	r := rounds[0]
	cid, _, err := identity.New(v1).Sum("cote.csf.id.cycle", "cote.csf.schema.cycle-id-preimage", 1, []any{run[:], []any{int64(r.Eligibility.NotBeforeInstant), uint64(0)}})
	if err != nil {
		t.Fatal(err)
	}
	open := r19Dispatch(t, v1, run, domain.ID(cid), r, "open", 0, 0, r.Eligibility.NotBeforeInstant)
	if err := ledger.Respond(ctx, r19NoProposal(t, v1, open, 0)); err != nil {
		t.Fatal(err)
	}
	v2, err := policy.LoadV2("../../../docs/architecture/canonical-codec-v2-bundle")
	if err != nil {
		t.Fatal(err)
	}
	child, err := sqlite.Open(ctx, t.TempDir()+"/frozen-cut.db")
	if err != nil {
		t.Fatal(err)
	}
	defer child.Close()
	childID := [32]byte(run)
	childID[0] ^= 0x6d
	if _, err := parent.ForkIdleSnapshotV1ToV2(ctx, [32]byte(run), snap.ID, childID, child, v1, v2); err != nil {
		t.Fatal(err)
	}
	restored, err := inputledger.Open(ctx, domain.ID(childID), v2, child)
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, rounds, slots, _ := restored.Snapshot()
	if len(slots[rounds[0].ID]) != 1 {
		t.Fatal("child imported post-checkpoint answer")
	}
	if _, err := child.ReplayClock(ctx, childID, v2); err != nil {
		t.Fatal(err)
	}
}

func TestCSFRV18R19V2RebasesRoundAfterPriorCommittedCycle(t *testing.T) {
	ctx := context.Background()
	c, parent, ledger, _, _, sourcePath := noProposalWorkFixture(t)
	_, _, _, _, _, run := ledger.Snapshot()
	clock, err := parent.LoadClock(ctx, [32]byte(run))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.CloseSource(ctx, "driver", inputledger.Closure{ProducerKey: "r19-final", ClosedThrough: domain.EligibilityCoordinate{NotBeforeInstant: clock.Instant + 1000}, Final: true}); err != nil {
		t.Fatal(err)
	}
	frozen, err := parent.CreateSnapshotV1(ctx, [32]byte(run), c.bundle, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.ResumeRegisteredWorkV1(ctx, ledger, [32]byte(run), frozen.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ResumeRegisteredWorkV1(ctx, ledger, [32]byte(run), frozen.ID); err != nil {
		t.Fatal(err)
	}
	clock, err = parent.LoadClock(ctx, [32]byte(run))
	if err != nil {
		t.Fatal(err)
	}
	if clock.LastOrdinal != 0 || clock.Revision != 1 {
		t.Fatal("fixture did not commit a preceding cycle")
	}
	root, err := parent.ValidatedGenesis(ctx, [32]byte(run), c.bundle)
	if err != nil {
		t.Fatal(err)
	}
	ledger, err = inputledger.Open(ctx, run, c.bundle, parent)
	if err != nil {
		t.Fatal(err)
	}
	round, err := ledger.DeclareRound(ctx, "schedule", [2]string{"school", "a"}, []string{"act"}, domain.EligibilityCoordinate{NotBeforeInstant: clock.Instant, NotBeforeCycleOrdinal: 1}, []any{uint64(2), []any{root.ID[:], root.Digest[:]}}, 2, 0, 0, []any{uint64(0)})
	if err != nil {
		t.Fatal(err)
	}
	cid, _, err := identity.New(c.bundle).Sum("cote.csf.id.cycle", "cote.csf.schema.cycle-id-preimage", 1, []any{run[:], []any{int64(clock.Instant), uint64(1)}})
	if err != nil {
		t.Fatal(err)
	}
	d := r19Dispatch(t, c.bundle, run, domain.ID(cid), round, "act", 0, clock.Revision, clock.Instant)
	if err := ledger.RegisterDispatch(ctx, d, inputledger.CycleBinding{ID: d.CycleID, BaseRevision: d.BaseRevision, Instant: d.EffectiveAt}); err != nil {
		t.Fatal(err)
	}
	if err := ledger.Respond(ctx, r19NoProposal(t, c.bundle, d, 0)); err != nil {
		t.Fatal(err)
	}
	checkpoints := csfrv18SyntheticActorCheckpoints(t, sourcePath, clock.Instant, int64(clock.Revision))
	snap, err := parent.CreateSnapshotV1(ctx, [32]byte(run), c.bundle, checkpoints)
	if err != nil {
		t.Fatal(err)
	}
	v2, err := policy.LoadV2("../../../docs/architecture/canonical-codec-v2-bundle")
	if err != nil {
		t.Fatal(err)
	}
	child, err := sqlite.Open(ctx, t.TempDir()+"/next-cycle.db")
	if err != nil {
		t.Fatal(err)
	}
	defer child.Close()
	childID := [32]byte(run)
	childID[0] ^= 0x6e
	if _, err := parent.ForkIdleSnapshotV1ToV2(ctx, [32]byte(run), snap.ID, childID, child, c.bundle, v2); err != nil {
		t.Fatal(err)
	}
	restored, err := inputledger.Open(ctx, domain.ID(childID), v2, child)
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, rounds, responses, _ := restored.Snapshot()
	if len(rounds) != 1 || rounds[0].Eligibility.NotBeforeCycleOrdinal != 0 {
		t.Fatal("child round retained parent cycle cursor")
	}
	r := rounds[0]
	response := responses[r.ID]["act"]
	if response.SubmittedAgainstRevision != 0 {
		t.Fatal("child response retained source world revision")
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
	cycle := admission.Cycle{ID: response.CycleID, Instant: clock.Instant, BaseStateHash: hash, RetryRef: []any{uint64(0)}}
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
}

// R19-1: each injected record has valid canonical bytes and a valid causal ID.
// Corruption is in the lifecycle transition, so ID/digest checks alone cannot
// reject it. Replay must reject without relying on a later child snapshot.
func TestCSFRV18R19Finding1RejectsIllegalSlotHistory(t *testing.T) {
	for _, mutation := range []string{"second-response-kind", "response-to-revoked", "redispatch-without-revocation", "response-to-superseded", "revoke-filled-slot", "revoke-twice", "nonzero-first-ordinal"} {
		t.Run(mutation, func(t *testing.T) {
			ctx := context.Background()
			parent, v1, run, snap := r19Source(t)
			v2, err := policy.LoadV2("../../../docs/architecture/canonical-codec-v2-bundle")
			if err != nil {
				t.Fatal(err)
			}
			path := t.TempDir() + "/illegal-history.db"
			child, err := sqlite.Open(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			defer child.Close()
			childID := [32]byte(run)
			childID[0] ^= 0x7a
			root, err := parent.ForkIdleSnapshotV1ToV2(ctx, [32]byte(run), snap.ID, childID, child, v1, v2)
			if err != nil {
				t.Fatal(err)
			}
			ledger, err := inputledger.Open(ctx, domain.ID(childID), v2, child)
			if err != nil {
				t.Fatal(err)
			}
			_, _, _, rounds, responses, _ := ledger.Snapshot()
			round := rounds[0]
			cid := responses[round.ID]["answered"].CycleID
			slot := "answered"
			switch mutation {
			case "response-to-revoked", "response-to-superseded", "revoke-twice":
				slot = "revoked"
			case "redispatch-without-revocation":
				slot = "open"
			}
			d := r19Dispatch(t, v2, domain.ID(childID), cid, round, slot, 0, 0, round.Eligibility.NotBeforeInstant)
			if mutation == "nonzero-first-ordinal" {
				round, err = ledger.DeclareRound(ctx, "child.schedule", round.Actor, []string{"new"}, round.Eligibility, []any{uint64(2), []any{root.ID[:], root.Digest[:]}}, 3, 0, 0, []any{uint64(0)})
				if err != nil {
					t.Fatal(err)
				}
				d = r19Dispatch(t, v2, domain.ID(childID), cid, round, "new", 4, 0, round.Eligibility.NotBeforeInstant)
			}
			if mutation == "response-to-superseded" {
				replacement := r19Dispatch(t, v2, domain.ID(childID), cid, round, slot, 1, 0, d.EffectiveAt)
				if err := ledger.RegisterDispatch(ctx, replacement, inputledger.CycleBinding{ID: cid, Instant: d.EffectiveAt}); err != nil {
					t.Fatal(err)
				}
			}
			var raw []byte
			var id [32]byte
			switch mutation {
			case "second-response-kind":
				response, err := testresourcev1.ProposalForDispatch(v2, d)
				if err != nil {
					t.Fatal(err)
				}
				raw = response.Canonical
				id = [32]byte(response.ID)
			case "response-to-revoked", "response-to-superseded":
				response := r19NoProposal(t, v2, d, 0)
				digest, _, err := identity.New(v2).Sum("cote.csf.digest.no-proposal-unit", "cote.csf.schema.no-proposal.body", 1, response.Body)
				if err != nil {
					t.Fatal(err)
				}
				raw, err = codec.CanonicalPayload(append(response.Body, digest[:]))
				if err != nil {
					t.Fatal(err)
				}
				id = [32]byte(response.ID)
			case "revoke-filled-slot", "revoke-twice":
				raw, err = codec.CanonicalPayload([]any{d.ID[:], uint64(1), []any{uint64(0)}})
				if err != nil {
					t.Fatal(err)
				}
				id = [32]byte(identity.OpaqueContentHash(raw))
			case "redispatch-without-revocation", "nonzero-first-ordinal":
				if mutation == "redispatch-without-revocation" {
					d = r19Dispatch(t, v2, domain.ID(childID), cid, round, slot, 1, 0, d.EffectiveAt)
				}
				raw, err = codec.CanonicalPayload([]any{d.ID[:], d.RunID[:], d.CycleID[:], d.RoundID[:], d.SlotID, uint64(d.Ordinal), []any{d.Actor[0], d.Actor[1]}, []any{int64(d.Eligibility.NotBeforeInstant), uint64(d.Eligibility.NotBeforeCycleOrdinal)}, uint64(d.BaseRevision), int64(d.EffectiveAt), d.RequestDigest[:]})
				if err != nil {
					t.Fatal(err)
				}
				id = [32]byte(d.ID)
			}
			sqlDB, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			defer sqlDB.Close()
			if _, err := sqlDB.ExecContext(ctx, "INSERT INTO input_ledger(id,canonical) VALUES(?,?)", id[:], raw); err != nil {
				t.Fatal(err)
			}
			if _, err := inputledger.Open(ctx, domain.ID(childID), v2, child); err == nil {
				t.Errorf("R19-1: ledger restore accepted %s", mutation)
			}
			if _, err := child.ReplayClock(ctx, childID, v2); err == nil {
				t.Fatalf("R19-1: replay accepted %s with valid individual IDs", mutation)
			}
		})
	}
}

func TestCSFRV18R19Finding1AllowsValidRepeatedRevocationAndReopen(t *testing.T) {
	ctx := context.Background()
	parent, v1, run, snap := r19Source(t)
	v2, err := policy.LoadV2("../../../docs/architecture/canonical-codec-v2-bundle")
	if err != nil {
		t.Fatal(err)
	}
	path := t.TempDir() + "/valid-history.db"
	child, err := sqlite.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	childID := [32]byte(run)
	childID[0] ^= 0x7b
	if _, err := parent.ForkIdleSnapshotV1ToV2(ctx, [32]byte(run), snap.ID, childID, child, v1, v2); err != nil {
		t.Fatal(err)
	}
	ledger, err := inputledger.Open(ctx, domain.ID(childID), v2, child)
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, rounds, responses, _ := ledger.Snapshot()
	r := rounds[0]
	cid := responses[r.ID]["answered"].CycleID
	for ordinal := uint32(1); ordinal <= 3; ordinal++ {
		if err := ledger.RevokeDispatch(ctx, r.ID, "open", 0, []any{uint64(0)}); err != nil {
			t.Fatal(err)
		}
		d := r19Dispatch(t, v2, domain.ID(childID), cid, r, "open", ordinal, 0, r.Eligibility.NotBeforeInstant)
		if err := ledger.RegisterDispatch(ctx, d, inputledger.CycleBinding{ID: cid, Instant: d.EffectiveAt}); err != nil {
			t.Fatal(err)
		}
		if ordinal == 3 {
			if err := ledger.Respond(ctx, r19NoProposal(t, v2, d, 0)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := child.Close(); err != nil {
		t.Fatal(err)
	}
	child, err = sqlite.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer child.Close()
	if _, err := child.ReplayClock(ctx, childID, v2); err != nil {
		t.Fatal("valid lifecycle rejected:", err)
	}
	checkpoint, err := child.CreateSnapshotV1(ctx, childID, v2, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := child.VerifySnapshotV1(ctx, childID, checkpoint.ID, v2); err != nil {
		t.Fatal(err)
	}
}

func TestCSFRV18R19OpaqueIntentionSurvivesForkWithoutGrantingAuthority(t *testing.T) {
	ctx := context.Background()
	var intention any
	parent, v1, run, snap := r19SourceWithAction(t, true, func(origin []any) any {
		ref := origin[1].([]any)
		intention = []any{uint64(1), []any{uint64(21), ref[0], ref[1]}}
		return intention
	})
	v2, err := policy.LoadV2("../../../docs/architecture/canonical-codec-v2-bundle")
	if err != nil {
		t.Fatal(err)
	}
	path := t.TempDir() + "/opaque-intention.db"
	child, err := sqlite.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	childID := [32]byte(run)
	childID[0] ^= 0x61
	if _, err := parent.ForkIdleSnapshotV1ToV2(ctx, [32]byte(run), snap.ID, childID, child, v1, v2); err != nil {
		t.Fatal("opaque intention migration:", err)
	}
	if err := child.Close(); err != nil {
		t.Fatal(err)
	}
	child, err = sqlite.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer child.Close()
	ledger, err := inputledger.Open(ctx, domain.ID(childID), v2, child)
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, rounds, slots, _ := ledger.Snapshot()
	got := slots[rounds[0].ID]["answered"]
	before, err := codec.CanonicalPayload(intention)
	if err != nil {
		t.Fatal(err)
	}
	after, err := codec.CanonicalPayload(got.Body[16])
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("opaque owner reference was regenerated or reinterpreted")
	}
	if _, err := child.ReplayClock(ctx, childID, v2); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, table := range []string{"event_store", "evidence_ledger", "causal_outbox"} {
		var n int
		if err := db.QueryRowContext(ctx, "SELECT count(*) FROM "+table).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Fatal("opaque intention became knowledge/world authority", table)
		}
	}
}

func TestCSFRV18R19IntentionRequiresAuthenticatedHistoricalRoot(t *testing.T) {
	for _, mutation := range []string{"id", "digest", "kind", "option", "missing-root", "missing-snapshot", "missing-epistemic"} {
		t.Run(mutation, func(t *testing.T) {
			ctx := context.Background()
			parent, v1, run, snap := r19SourceWithAction(t, true, func(origin []any) any {
				root := origin[1].([]any)
				ref := []any{uint64(21), bytes.Clone(root[0].([]byte)), bytes.Clone(root[1].([]byte))}
				switch mutation {
				case "id":
					ref[1].([]byte)[0] ^= 1
				case "digest":
					ref[2].([]byte)[0] ^= 1
				case "kind":
					ref[0] = uint64(65535)
				case "option":
					return []any{uint64(0), ref}
				case "missing-root":
					ref[0] = uint64(17)
				case "missing-snapshot":
					ref[0] = uint64(20)
				case "missing-epistemic":
					ref[0] = uint64(22)
				}
				return []any{uint64(1), ref}
			})
			v2, err := policy.LoadV2("../../../docs/architecture/canonical-codec-v2-bundle")
			if err != nil {
				t.Fatal(err)
			}
			path := t.TempDir() + "/rejected-intention.db"
			child, err := sqlite.Open(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			defer child.Close()
			childID := [32]byte(run)
			childID[0] ^= 0x62
			if _, err := parent.ForkIdleSnapshotV1ToV2(ctx, [32]byte(run), snap.ID, childID, child, v1, v2); err == nil {
				t.Fatal("unresolved intention gained authority")
			}
			assertCSFRV18EmptyRejectedFork(t, path)
		})
	}
}

func TestCSFRV18R19RoundAndDispatchIntentionsResolveInFrozenArchive(t *testing.T) {
	for _, kind := range []uint64{3, 4} {
		t.Run(string(rune('0'+kind)), func(t *testing.T) {
			ctx := context.Background()
			bundle, err := policy.Load(bundlePath)
			if err != nil {
				t.Fatal(err)
			}
			parent, v1, run, snap := r19SourceWithAction(t, true, func(r inputledger.Round, d inputledger.Dispatch) any {
				id, digest := r.ID, r.Digest
				if kind == 4 {
					id = d.ID
					var err error
					digest, _, err = identity.New(bundle).Sum("cote.csf.digest.slot-dispatch", "cote.csf.schema.slot-dispatch", 1,
						[]any{d.ID[:], d.RunID[:], d.CycleID[:], d.RoundID[:], d.SlotID, uint64(d.Ordinal), []any{d.Actor[0], d.Actor[1]}, []any{int64(d.Eligibility.NotBeforeInstant), uint64(d.Eligibility.NotBeforeCycleOrdinal)}, uint64(d.BaseRevision), int64(d.EffectiveAt), d.RequestDigest[:]})
					if err != nil {
						t.Fatal(err)
					}
				}
				return []any{uint64(1), []any{kind, id[:], digest[:]}}
			})
			v2, err := policy.LoadV2("../../../docs/architecture/canonical-codec-v2-bundle")
			if err != nil {
				t.Fatal(err)
			}
			child, err := sqlite.Open(ctx, t.TempDir()+"/archive-intention.db")
			if err != nil {
				t.Fatal(err)
			}
			defer child.Close()
			childID := [32]byte(run)
			childID[0] ^= 0x64
			if _, err := parent.ForkIdleSnapshotV1ToV2(ctx, [32]byte(run), snap.ID, childID, child, v1, v2); err != nil {
				t.Fatal("archived intention:", err)
			}
			if _, err := child.ReplayClock(ctx, childID, v2); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCSFRV18R19EarlierRevokedRequestDoesNotRenewAuthorization(t *testing.T) {
	ctx := context.Background()
	var sourcePath string
	parent, ledger, v1, run, origin := csfrv18GenesisRoundSource(t, &sourcePath)
	state, err := parent.LoadClock(ctx, [32]byte(run))
	if err != nil {
		t.Fatal(err)
	}
	round, err := ledger.DeclareRound(ctx, "schedule", [2]string{"school", "a"}, []string{"act"},
		domain.EligibilityCoordinate{NotBeforeInstant: state.Instant + 5}, origin, 1, 0, 0, []any{uint64(0)})
	if err != nil {
		t.Fatal(err)
	}
	cid, _, err := identity.New(v1).Sum("cote.csf.id.cycle", "cote.csf.schema.cycle-id-preimage", 1,
		[]any{run[:], []any{int64(state.Instant), uint64(0)}})
	if err != nil {
		t.Fatal(err)
	}
	old := r19Dispatch(t, v1, run, domain.ID(cid), round, "act", 0, 0, state.Instant)
	// Compatibility fixture for already-extinguished legacy authority. The
	// public write paths must now reject premature future-round dispatches.
	legacy, err := sql.Open("sqlite", sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	defer legacy.Close()
	legacyTx, err := legacy.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer legacyTx.Rollback()
	raw, err := codec.CanonicalPayload([]any{old.ID[:], run[:], cid[:], round.ID[:], old.SlotID, uint64(0), []any{old.Actor[0], old.Actor[1]}, []any{int64(round.Eligibility.NotBeforeInstant), uint64(0)}, uint64(0), int64(old.EffectiveAt), old.RequestDigest[:]})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := legacyTx.ExecContext(ctx, "INSERT INTO input_ledger(id,canonical) VALUES(?,?)", old.ID[:], raw); err != nil {
		t.Fatal(err)
	}
	revoked, err := codec.CanonicalPayload([]any{old.ID[:], uint64(2), []any{uint64(0)}})
	if err != nil {
		t.Fatal(err)
	}
	revokedID := identity.OpaqueContentHash(revoked)
	if _, err := legacyTx.ExecContext(ctx, "INSERT INTO input_ledger(id,canonical) VALUES(?,?)", revokedID[:], revoked); err != nil {
		t.Fatal(err)
	}
	if err := legacyTx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}
	ledger, err = inputledger.Open(ctx, run, v1, parent)
	if err != nil {
		t.Fatal(err)
	}
	base, err := parent.CreateSnapshotV1(ctx, [32]byte(run), v1, nil)
	if err != nil {
		t.Fatal(err)
	}
	refs, err := policiesByRole(v1)
	if err != nil {
		t.Fatal(err)
	}
	policies := admission.Policies{AdmissionOrder: refs["admission-order"], CausalIdentity: refs["causal-identity"], Fence: refs["admission-fence"], Rules: [][]any{}}
	c := New(parent, v1, policies)
	if _, err := c.Advance(ctx, ledger, run); err != nil {
		t.Fatal(err)
	}
	if _, err := parent.DrainPendingPerceptionV1(ctx, [32]byte(run), base.ID, v1); err != nil {
		t.Fatal(err)
	}
	state, err = parent.LoadClock(ctx, [32]byte(run))
	if err != nil {
		t.Fatal(err)
	}
	cid, _, err = identity.New(v1).Sum("cote.csf.id.cycle", "cote.csf.schema.cycle-id-preimage", 1,
		[]any{run[:], []any{int64(state.Instant), uint64(0)}})
	if err != nil {
		t.Fatal(err)
	}
	current := r19Dispatch(t, v1, run, domain.ID(cid), round, "act", 1, state.Revision, state.Instant)
	if err := ledger.RegisterDispatch(ctx, current, inputledger.CycleBinding{ID: current.CycleID, BaseRevision: state.Revision, Instant: state.Instant}); err != nil {
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
	path := t.TempDir() + "/prior-revoked.db"
	child, err := sqlite.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	childID := [32]byte(run)
	childID[0] ^= 0x63
	if _, err := parent.ForkIdleSnapshotV1ToV2(ctx, [32]byte(run), snap.ID, childID, child, v1, v2); err != nil {
		t.Fatal("older revoked request:", err)
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
	_, _, _, rounds, _, _ := restored.Snapshot()
	cid, _, err = identity.New(v2).Sum("cote.csf.id.cycle", "cote.csf.schema.cycle-id-preimage", 1,
		[]any{childID[:], []any{int64(state.Instant), uint64(0)}})
	if err != nil {
		t.Fatal(err)
	}
	for ordinal := uint32(0); ordinal <= 1; ordinal++ {
		d := r19Dispatch(t, v2, domain.ID(childID), domain.ID(cid), rounds[0], "act", ordinal, 0, state.Instant)
		err := restored.Respond(ctx, r19NoProposal(t, v2, d, 0))
		if ordinal == 0 && err == nil {
			t.Fatalf("old authorization revived: %v", err)
		}
		if ordinal == 1 && err != nil {
			t.Fatal("current authorization lost:", err)
		}
	}
	if _, err := child.ReplayClock(ctx, childID, v2); err != nil {
		t.Fatal(err)
	}
}

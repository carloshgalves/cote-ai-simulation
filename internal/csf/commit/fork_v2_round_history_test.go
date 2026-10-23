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
func r19SourceWithAction(t *testing.T, action bool) (*sqlite.Store, *policy.Bundle, domain.ID, sqlite.SnapshotV1) {
	t.Helper()
	ctx := context.Background()
	parent, ledger, v1, run, origin := csfrv18GenesisRoundSource(t)
	state, err := parent.LoadClock(ctx, [32]byte(run))
	if err != nil {
		t.Fatal(err)
	}
	round, err := ledger.DeclareRound(ctx, "genesis.schedule", [2]string{"school", "a"}, []string{"answered", "open", "revoked"},
		domain.EligibilityCoordinate{NotBeforeInstant: state.Instant}, origin, 1, 0, 0, []any{uint64(0)})
	if err != nil {
		t.Fatal(err)
	}
	cid, _, err := identity.New(v1).Sum("cote.csf.id.cycle", "cote.csf.schema.cycle-id-preimage", 1, []any{run[:], []any{int64(state.Instant), uint64(0)}})
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
	snap, err := parent.CreateSnapshotV1(ctx, [32]byte(run), v1, nil)
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

//go:build linux

package commit

import (
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/adapter/embodimentv1"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/admission"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/inputledger"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
)

// Test-harness-only E2 recovery: never consumes a model authored body state,
// reconstructs Python output, or resumes an unimplemented Python PSV1-8
// process. A genuine signed A1 input has ALREADY been ingested and sealed.
// Reopen the SQLite authority, rederive its frozen cycle from its current
// clock/WorldState, and use the original coordinator and CommitWork path.
var errReferenceE2AlreadyCommitted = errors.New("E2 physical cycle already committed")

type referenceE2Session struct {
	store       *sqlite.Store
	ledger      *inputledger.Ledger
	coordinator *Coordinator
	policies    admission.Policies
	run         domain.ID
}

func referenceE2Open(ctx context.Context, path string, bundle *policy.Bundle) (*referenceE2Session, error) {
	if ctx == nil || path == "" || bundle == nil {
		return nil, errors.New("E2 requires explicit persisted authority")
	}
	if err := bundle.VerifyInstalledBundle(); err != nil {
		return nil, err
	}
	g, err := buildReferenceGenesis(bundle)
	if err != nil {
		return nil, err
	}
	store, err := sqlite.Open(ctx, path)
	if err != nil {
		return nil, err
	}
	fail := func(err error) (*referenceE2Session, error) { store.Close(); return nil, err }
	validated, err := store.ValidatedGenesis(ctx, [32]byte(g.RunID), bundle)
	if err != nil || validated.RunID != g.RunID || validated.Digest != g.Digest {
		return fail(errors.New("E2 restart refused non-matching persisted Genesis"))
	}
	ledger, err := inputledger.Open(ctx, g.RunID, bundle, store)
	if err != nil {
		return fail(fmt.Errorf("E2 rebuild source ledger: %w", err))
	}
	roles, err := policiesByRole(bundle)
	if err != nil {
		return fail(err)
	}
	policies := admission.Policies{
		AdmissionOrder: roles["admission-order"],
		CausalIdentity: roles["causal-identity"],
		Fence:          roles["admission-fence"],
		Rules:          [][]any{},
	}
	return &referenceE2Session{store: store, ledger: ledger,
		coordinator: New(store, bundle, policies),
		policies:    policies, run: g.RunID}, nil
}

// Only a valid signed source/frozen input ledger may reach a physical
// CommitWork terminal. This creates no source on recovery of the failed
// epoch, so a previously FINAL source remains final.
func (s *referenceE2Session) physicalCycle(ctx context.Context, bundle *policy.Bundle, epoch int) (admission.Cycle, admission.Fence, error) {
	var empty admission.Cycle
	var absent admission.Fence
	state, err := s.store.LoadClock(ctx, [32]byte(s.run))
	if err != nil {
		return empty, absent, err
	}
	if state.Revision == 9 {
		return empty, absent, errReferenceE2AlreadyCommitted
	}
	if epoch != 1 && epoch != 2 {
		return empty, absent, errors.New("E2 invalid recovery epoch")
	}
	if uint64(state.Revision) != uint64(6+epoch) || state.Instant != 35 ||
		state.LastOrdinal != int64(epoch) || state.LastOrdinal >= math.MaxInt64 {
		return empty, absent, errors.New("E2 no matching frozen physical predecessor; refusing out-of-order restart")
	}
	if replay, err := s.store.ReplayClock(ctx, [32]byte(s.run), bundle); err != nil ||
		replay.ReplayedCommits != int(state.Revision) || replay.ReconstructedClock != state {
		return empty, absent, fmt.Errorf("E2 failed authenticated pre-recovery replay: %v", err)
	}
	_, hash, err := s.store.LoadWorldState(ctx, uint64(state.Revision))
	if err != nil {
		return empty, absent, err
	}
	ordinal := domain.CycleOrdinal(state.LastOrdinal + 1)
	cycleID, _, err := identity.New(bundle).Sum("cote.csf.id.cycle",
		"cote.csf.schema.cycle-id-preimage", 1,
		[]any{s.run[:], []any{int64(state.Instant), uint64(ordinal)}})
	if err != nil {
		return empty, absent, err
	}
	cycle := admission.Cycle{
		ID: domain.ID(cycleID), Attempt: 0, Instant: state.Instant, Ordinal: ordinal,
		BaseRevision: state.Revision, BaseStateHash: domain.Digest(hash),
		RetryRef: []any{uint64(0)},
	}
	fence, err := admission.NewCoordinator(admission.NewFenceLog(s.store)).
		AdmitThenEvaluate(ctx, bundle, s.ledger, cycle, s.policies, nil)
	if err != nil {
		return empty, absent, fmt.Errorf("E2 re-derive exact persisted admission fence: %w", err)
	}
	return cycle, fence, nil
}

// Resume the *failed* epoch through the existing authoritative recovery
// protocol. It validates the C4 owner/snapshot seal, derives the exact
// frozen fence from persisted cycle control and independently checks the
// published terminal. This avoids a second, weaker E2 recovery algorithm.
func referenceE2ResumeFrozen(ctx context.Context, path string, s *referenceE2Session,
	bundle *policy.Bundle, epoch int, cut embodimentv1.ApprovedCut) error {
	if epoch != 1 && epoch != 2 {
		return errors.New("E2 invalid frozen epoch")
	}
	reader, err := openReferenceReadOnlyDB(path)
	if err != nil {
		return err
	}
	var count int
	err = reader.QueryRowContext(ctx, "SELECT COUNT(*) FROM snapshot_seals WHERE run_id=?", s.run[:]).Scan(&count)
	if err != nil || count != 1 {
		reader.Close()
		return errors.New("E2 requires exactly one authoritative C4 owner snapshot")
	}
	var raw []byte
	err = reader.QueryRowContext(ctx, "SELECT snapshot_id FROM snapshot_seals WHERE run_id=?", s.run[:]).Scan(&raw)
	closeErr := reader.Close()
	if err != nil || closeErr != nil || len(raw) != 32 {
		return errors.New("E2 sealed C4 snapshot ID absent or malformed")
	}
	var snapshot [32]byte
	copy(snapshot[:], raw)
	verified, err := s.coordinator.ResumeRunV1(ctx, s.ledger, [32]byte(s.run), snapshot,
		func(ctx context.Context, ledger *inputledger.Ledger, cycle admission.Cycle,
			fence admission.Fence) (ports.CanonicalRecord, error) {
			if cycle.Attempt != 0 || cycle.Plan != nil || cycle.Instant != 35 ||
				uint64(cycle.Ordinal) != uint64(epoch+1) ||
				uint64(cycle.BaseRevision) != uint64(6+epoch) ||
				cycle.ID == (domain.ID{}) {
				return ports.CanonicalRecord{}, errors.New("E2 recovered the wrong cycle or attempt")
			}
			return s.coordinator.CommitPhysical(ctx, ledger, cycle, fence, cut)
		})
	if err != nil {
		return fmt.Errorf("E2 ResumeRunV1 refused frozen physical epoch %d: %w", epoch, err)
	}
	if !verified.Continued || verified.Checkpoint.Action.Kind != sqlite.ResumeReexecuteFencedAttempt ||
		verified.Publication.Identity == ([32]byte{}) || len(verified.Publication.Canonical) == 0 {
		return errors.New("E2 native resume did not independently verify the physical terminal")
	}
	return nil
}

// Called only on the TEST side of the trusted A1 protocol: the original two
// approved cuts are frozen from a real supervised producer. No Python replay,
// new signature, new source key or artificial terminal is generated.
func referenceE2RecoverPhysical(ctx context.Context, path string, bundle *policy.Bundle,
	cuts [2]embodimentv1.ApprovedCut, failedEpoch int) error {
	if failedEpoch != 1 && failedEpoch != 2 {
		return errors.New("E2 invalid failed epoch")
	}
	s, err := referenceE2Open(ctx, path, bundle)
	if err != nil {
		return err
	}
	defer s.store.Close()
	initial, err := s.store.LoadClock(ctx, [32]byte(s.run))
	if err != nil {
		return err
	}
	if initial.Revision == 9 {
		return errReferenceE2AlreadyCommitted
	}
	for epoch := failedEpoch; epoch <= 2; epoch++ {
		if epoch == failedEpoch {
			if err := referenceE2ResumeFrozen(ctx, path, s, bundle, epoch, cuts[epoch-1]); err != nil {
				return err
			}
			continue
		}
		if epoch == 2 && failedEpoch == 1 {
			// This is a brand-new epoch: its authenticated A1 cut has not
			// yet been staged. The producer is NOT executed in recovery.
			// Calculate the next coordinates from SQLite, not wall clock.
			state, err := s.store.LoadClock(ctx, [32]byte(s.run))
			if err != nil {
				return err
			}
			if state.Revision != 8 || state.LastOrdinal != 2 {
				return errors.New("E2 prior resumed epoch is not committed")
			}
			_, hash, err := s.store.LoadWorldState(ctx, 8)
			if err != nil {
				return err
			}
			next := domain.CycleOrdinal(state.LastOrdinal + 1)
			cid, _, err := identity.New(bundle).Sum("cote.csf.id.cycle",
				"cote.csf.schema.cycle-id-preimage", 1,
				[]any{s.run[:], []any{int64(state.Instant), uint64(next)}})
			if err != nil {
				return err
			}
			cycle := admission.Cycle{ID: domain.ID(cid), Attempt: 0,
				Instant: state.Instant, Ordinal: next, BaseRevision: state.Revision,
				BaseStateHash: domain.Digest(hash), RetryRef: []any{uint64(0)}}
			if _, err := s.coordinator.StagePhysicalInput(ctx, s.ledger, cuts[1], cycle); err != nil {
				return err
			}
			reg, err := bundle.PhysicalRegistration()
			if err != nil {
				return err
			}
			_, err = s.ledger.CloseSource(ctx, reg.SourceID, inputledger.Closure{
				ProducerKey: "physical.a1.final.epoch-2",
				ClosedThrough: domain.EligibilityCoordinate{
					NotBeforeInstant: cycle.Instant, NotBeforeCycleOrdinal: cycle.Ordinal},
				Final: true,
			})
			if err != nil {
				return err
			}
		}
		cycle, fence, err := s.physicalCycle(ctx, bundle, epoch)
		if err != nil {
			return err
		}
		if _, err := s.coordinator.CommitPhysical(ctx, s.ledger, cycle, fence, cuts[epoch-1]); err != nil {
			return fmt.Errorf("E2 resumed authentic physical epoch %d: %w", epoch, err)
		}
	}
	end, err := s.store.LoadClock(ctx, [32]byte(s.run))
	if err != nil || end.Revision != 9 || end.LastOrdinal != 3 {
		return errors.New("E2 replay did not finish exactly nine commits")
	}
	replayed, err := s.store.ReplayClock(ctx, [32]byte(s.run), bundle)
	if err != nil || replayed.ReplayedCommits != 9 || replayed.ReconstructedClock != end {
		return fmt.Errorf("E2 post-recovery replay invalid: %v", err)
	}
	return nil
}

func referenceE2RecoverWrongEpoch(ctx context.Context, path string, bundle *policy.Bundle,
	wrong embodimentv1.ApprovedCut) error {
	s, err := referenceE2Open(ctx, path, bundle)
	if err != nil {
		return err
	}
	defer s.store.Close()
	err = referenceE2ResumeFrozen(ctx, path, s, bundle, 1, wrong)
	if err == nil {
		return errors.New("E2 wrong future cut was unexpectedly accepted")
	}
	return err
}

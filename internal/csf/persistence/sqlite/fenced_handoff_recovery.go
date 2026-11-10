package sqlite

import (
    "context"
    "errors"
    "fmt"

    "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

// VerifyFrozenFenceCommittedOnce supports retry after a crash between the V1
// source terminal commit and a V2 child fork. It authenticates the original
// frozen WORK/retry snapshot, its exact cycle and the single committed terminal
// using the authoritative journal. An unrelated later commit, an abort, or an
// uncommitted fence cannot be mistaken for the frozen terminal. Legitimate
// subsequent world revisions and in-flight work cannot change that old cut.
//
// This method does not restore cognitive bytes. Those must still be supplied
// from an authoritative, durable owner checkpoint before publishing the child.
func (s *Store) VerifyFrozenFenceCommittedOnce(
    ctx context.Context, run, originalSnapshot [32]byte, b *policy.Bundle,
) error {
    if s == nil || b == nil {
        return errors.New("fenced handoff recovery requires a verified store and policy")
    }
    proof, err := s.VerifySnapshotV1(ctx, run, originalSnapshot, b)
    if err != nil {
        return err
    }
    if proof.SnapshotControlStatus != 1 && proof.SnapshotControlStatus != 2 {
        return errors.New("handoff is not a committed frozen WORK/retry snapshot")
    }
    decoded, err := codec.StrictDecodePayload(proof.Snapshot.Canonical)
    if err != nil {
        return err
    }
    fields, ok := decoded.([]any)
    if !ok || len(fields) != 19 {
        return errors.New("frozen handoff snapshot schema malformed")
    }
    control, ok := fields[13].([]any)
    if !ok || len(control) != 7 {
        return errors.New("frozen handoff control schema malformed")
    }
    opt, ok := control[1].([]any)
    if !ok || len(opt) != 2 || opt[0] != uint64(1) {
        return errors.New("frozen handoff cycle absent")
    }
    cycleBytes, ok := opt[1].([]byte)
    if !ok || len(cycleBytes) != 32 {
        return errors.New("frozen handoff cycle ID malformed")
    }
    var cycle [32]byte
    copy(cycle[:], cycleBytes)

    var terminalBytes, canonical []byte
    err = s.db.QueryRowContext(ctx,
        "SELECT c.commit_id,d.canonical FROM committed_cycles c "+
            "JOIN decision_ledger d ON d.id=c.commit_id "+
            "WHERE c.run_id=? AND c.cycle_id=?",
        run[:], cycle[:]).Scan(&terminalBytes, &canonical)
    if err != nil {
        return fmt.Errorf("original frozen cycle has no committed terminal: %w", err)
    }
    if len(terminalBytes) != 32 {
        return errors.New("frozen handoff terminal ID malformed")
    }
    var terminal [32]byte
    copy(terminal[:], terminalBytes)
    // A causal replay has already authenticated the entire journal, including
    // tails beyond this commit. The historical proof must bind this exact
    // frozen cycle to the terminal at snapshot revision R+1, not to the
    // *current* clock/control of a potentially much later parent.
    value, err := codec.StrictDecodePayload(canonical)
    if err != nil { return err }
    fields, ok := value.([]any)
    if !ok || len(fields) != 24 || !equalBytes(fields[0], run[:]) ||
        !equalBytes(fields[1], cycle[:]) {
        return errors.New("historical frozen terminal does not match run/cycle")
    }
    revision, ok := fields[8].(uint64)
    baseRevision := uint64(proof.SnapshotClock.Revision)
    if !ok || baseRevision == ^uint64(0) || revision != baseRevision+1 ||
        uint64(proof.Clock.Revision) < revision {
        return errors.New("historical frozen terminal did not advance exactly one revision")
    }
    var count int
    if err := s.db.QueryRowContext(ctx,
        "SELECT count(*) FROM committed_cycles WHERE run_id=? AND cycle_id=?",
        run[:], cycle[:]).Scan(&count); err != nil { return err }
    if count != 1 { return errors.New("frozen cycle has nonunique committed terminal") }
    // Preserve the stronger live-tip attestation when the source still sits
    // at the original IDLE successor; otherwise the authenticated journal
    // and exact terminal revision provide the immutable historical proof.
    if proof.ControlStatus == 0 && uint64(proof.Clock.Revision) == revision {
        if err := s.VerifyRecoveredTerminal(ctx, run, cycle, terminal,
            canonical, baseRevision, b); err != nil {
            return fmt.Errorf("frozen handoff terminal provenance mismatch: %w", err)
        }
    }
    return nil
}

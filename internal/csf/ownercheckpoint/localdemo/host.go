// Package localdemo is a single-operator synthetic cognitive dispatch seam.
// It proves Stage -> sealed snapshot -> fresh session -> restore/ACK -> WORK.
// This package is not a product model host, a server, or a second CSF authority.
package localdemo

import (
    "bytes"
    "context"
    "errors"
    "fmt"

    "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/ownercheckpoint"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

// Reference identifies the exact sealed CSF snapshot and owner receipt that
// an independently restarted host must revalidate. It is not a work permit.
type Reference struct {
    Snapshot [32]byte
    Receipt  [32]byte
}

// Host pins one actor and its synthetic cognitive callback for the lifetime
// of this local session. It cannot issue cognitive WORK except via Gate.RunWork.
// The caller must never invoke the underlying model/callback directly: only
// work submitted to this demo host is protected by the Gate.
type Host struct {
    scope  ownercheckpoint.Scope
    gate   *ownercheckpoint.Gate
    causal *sqlite.Store
    bundle *policy.Bundle
    work   func(context.Context) error
}

// New creates a fresh host/gate with no inherited restore permit. The caller
// registers the actor and Ed25519 public key with CSF before publication.
// The host doesn't own or close its two stores. To simulate restart, close
// both stores and open new stores, then construct a new Host with New.
func New(owner ownercheckpoint.DurableOwner, causal *sqlite.Store, bundle *policy.Bundle,
    scope ownercheckpoint.Scope, cognitiveWork func(context.Context) error) (*Host, error) {
    if owner == nil || causal == nil || bundle == nil || cognitiveWork == nil {
        return nil, errors.New("local cognitive host needs owner, causal store, policy and callback")
    }
    if err := domain.ValidateMachineID(scope.Namespace); err != nil {
        return nil, fmt.Errorf("local host namespace: %w", err)
    }
    if err := domain.ValidateMachineID(scope.Actor); err != nil {
        return nil, fmt.Errorf("local host actor: %w", err)
    }
    gate, err := ownercheckpoint.NewGate(owner, causal, bundle)
    if err != nil {
        return nil, err
    }
    return &Host{scope: scope, gate: gate, causal: causal, bundle: bundle, work: cognitiveWork}, nil
}

// StageSnapshot publishes one actor's immutable synthetic checkpoint. Order is
// strictly owner durable commit/sign -> CSF escrow -> sealed CSF snapshot.
// If publication fails, no Reference is returned and no WORK permit exists;
// a staged orphan is not itself snapshot authority (the B3 crash matrix).
func (h *Host) StageSnapshot(ctx context.Context, checkpoint sqlite.SnapshotCheckpoint) (Reference, error) {
    var zero Reference
    if h == nil || ctx == nil {
        return zero, errors.New("local host stage unconfigured")
    }
    if checkpoint.ActorNamespace != h.scope.Namespace || checkpoint.ActorID != h.scope.Actor {
        return zero, errors.New("local host refuses another actor's checkpoint")
    }
    // Freeze the caller-owned memory once for both sequential transactions.
    // Mutation concurrent with the copy is outside the caller contract.
    frozen := checkpoint
    frozen.Content = bytes.Clone(checkpoint.Content)
    defer clear(frozen.Content)
    receipt, err := h.gate.Stage(ctx, h.scope, frozen)
    if err != nil {
        return zero, fmt.Errorf("owner stage/CSF escrow: %w", err)
    }
    snapshot, err := h.causal.CreateSnapshotV1(ctx, h.scope.Run, h.bundle, []sqlite.SnapshotCheckpoint{frozen})
    if err != nil {
        return zero, fmt.Errorf("CSF snapshot after owner escrow: %w", err)
    }
    if snapshot.ID == ([32]byte{}) || receipt == ([32]byte{}) {
        return zero, errors.New("CSF snapshot or owner receipt returned empty identity")
    }
    return Reference{Snapshot: snapshot.ID, Receipt: receipt}, nil
}

// Restore revalidates the exact CSF snapshot at the run tip, checks the owner
// escrow signature/bytes, and only then requests local durable owner restore
// and ACK. Invalid restore attempts revoke any permit in this host session.
func (h *Host) Restore(ctx context.Context, ref Reference) error {
    if h == nil || ctx == nil {
        return errors.New("local host restore unconfigured")
    }
    return h.gate.RestoreAndAcknowledge(ctx, h.scope, ref.Snapshot, ref.Receipt)
}

// Run is the single dispatch entry point for the synthetic cognitive worker
// bound in New. The worker is never invoked in the constructor, StageSnapshot,
// Restore, causal replay, snapshot verification or a rejected gate call.
func (h *Host) Run(ctx context.Context, ref Reference) error {
    if h == nil || ctx == nil {
        return errors.New("local host work unconfigured")
    }
    return h.gate.RunWork(ctx, h.scope, ref.Snapshot, ref.Receipt, h.work)
}

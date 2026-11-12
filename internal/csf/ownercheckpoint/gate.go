package ownercheckpoint

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/subtle"
	"errors"
	"fmt"
	"sync"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

// Scope is the complete identity of an owner in a single causal run. A
// checkpoint restored for one scope never authorizes another scope or fork.
type Scope struct {
	Run       [32]byte
	Namespace string
	Actor     string
}

// DurableOwner is the external trust boundary. PersistAndSign MUST flush the
// exact private bytes to its own durable store before signing; Restore MUST
// reload those durable bytes into the named actor, not synthesize them; ACK
// MUST be durable/idempotent before returning nil. This interface cannot
// itself prove the provider physically honored those promises.
type DurableOwner interface {
	PersistAndSign(context.Context, PersistRequest) (SignedReceipt, error)
	Restore(context.Context, RestoreRequest) (RestoredContent, error)
	Acknowledge(context.Context, RestoreAck) error
}

// Escrow is the already-signed CSF authority, deliberately not a second
// cognitive store. The SQLite Store implements this interface.
type Escrow interface {
	PersistCognitiveCheckpointV1(context.Context, [32]byte, sqlite.SnapshotCheckpoint,
		string, []byte, *policy.Bundle) ([32]byte, error)
	LoadCognitiveCheckpointV1(context.Context, [32]byte, [32]byte,
		string, string, *policy.Bundle) (sqlite.SnapshotCheckpoint, string, error)
	VerifySnapshotV1(context.Context, [32]byte, [32]byte,
		*policy.Bundle) (sqlite.ResumeCheckpointV1, error)
}

type PersistRequest struct {
	Scope      Scope
	Checkpoint sqlite.SnapshotCheckpoint
}

type SignedReceipt struct {
	Locator   string
	Signature []byte
}

type RestoreRequest struct {
	Scope        Scope
	Locator      string
	ReceiptID    [32]byte
	ContentHash  [32]byte
}

type RestoredContent struct {
	Locator string
	Content []byte
}

type RestoreAck struct {
	Scope       Scope
	Locator     string
	ReceiptID   [32]byte
	SnapshotID  [32]byte
	ContentHash [32]byte
}

type permit struct {
	snapshot [32]byte
	receipt  [32]byte
}

var ErrNotAcknowledged = errors.New("cognitive owner has not restored and acknowledged the exact checkpoint")

// Gate deliberately owns only process-local work admission. No permit
// survives a restart. Every restart must reverify the snapshot and owner
// receipt, restore externally, and receive ACK before dispatching new work.
//
// The host must invoke RunWork for EVERY cognitive WORK/LLM dispatch. Direct
// calls to an unguarded model bypass this adapter and remain a release blocker.
type Gate struct {
	owner  DurableOwner
	escrow Escrow
	bundle *policy.Bundle

	mu         sync.RWMutex
	permits    map[Scope]permit
	generation map[Scope]uint64
}

func NewGate(owner DurableOwner, escrow Escrow, bundle *policy.Bundle) (*Gate, error) {
	if owner == nil || escrow == nil || bundle == nil {
		return nil, errors.New("owner gate requires provider, escrow and pinned policy")
	}
	return &Gate{
		owner: owner, escrow: escrow, bundle: bundle,
		permits: make(map[Scope]permit),
		generation: make(map[Scope]uint64),
	}, nil
}

func validateScope(s Scope) error {
	if s.Run == ([32]byte{}) || s.Namespace == "" || s.Actor == "" {
		return errors.New("cognitive owner scope requires run, namespace and actor")
	}
	return nil
}

// invalidate revokes previous work permits before any potentially failing
// provider operation. It serializes against in-progress RunWork callbacks.
func (g *Gate) invalidate(scope Scope) uint64 {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.permits, scope)
	g.generation[scope]++
	return g.generation[scope]
}

func cloneCheckpoint(cp sqlite.SnapshotCheckpoint) sqlite.SnapshotCheckpoint {
	cp.Content = bytes.Clone(cp.Content)
	return cp
}

// Stage establishes owner persist -> signed receipt -> CSF SQLite escrow.
// It does not publish a snapshot or authorize WORK. The host must publish
// the snapshot with the same checkpoint only after this returns successfully.
func (g *Gate) Stage(ctx context.Context, scope Scope,
	cp sqlite.SnapshotCheckpoint) ([32]byte, error) {
	var zero [32]byte
	if g == nil || ctx == nil || g.owner == nil || g.escrow == nil || g.bundle == nil {
		return zero, errors.New("owner checkpoint stage unconfigured")
	}
	if err := validateScope(scope); err != nil {
		return zero, err
	}
	if cp.ActorNamespace != scope.Namespace || cp.ActorID != scope.Actor {
		return zero, errors.New("owner checkpoint actor differs from requested scope")
	}
	g.invalidate(scope)
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	owned := cloneCheckpoint(cp)
	signed, err := g.owner.PersistAndSign(ctx, PersistRequest{Scope: scope, Checkpoint: owned})
	if err != nil {
		return zero, fmt.Errorf("external owner durable persist: %w", err)
	}
	if len(signed.Signature) != ed25519.SignatureSize {
		return zero, errors.New("external owner returned malformed signature")
	}
	if _, err := sqlite.CognitiveCheckpointSigningBytesV1(scope.Run, cp, signed.Locator); err != nil {
		return zero, fmt.Errorf("external owner receipt binding: %w", err)
	}
	receipt, err := g.escrow.PersistCognitiveCheckpointV1(ctx, scope.Run,
		cloneCheckpoint(cp), signed.Locator, bytes.Clone(signed.Signature), g.bundle)
	if err != nil {
		return zero, fmt.Errorf("signed owner checkpoint escrow: %w", err)
	}
	return receipt, nil
}

// RestoreAndAcknowledge verifies a sealed snapshot and escrowed signature,
// obtains the SAME bytes from the external durable store, and requires an
// explicit owner ACK. It never decodes or interprets private cognition.
func (g *Gate) RestoreAndAcknowledge(ctx context.Context, scope Scope,
	snapshotID, receiptID [32]byte) error {
	if g == nil || ctx == nil || g.owner == nil || g.escrow == nil || g.bundle == nil {
		return errors.New("owner checkpoint restore unconfigured")
	}
	if err := validateScope(scope); err != nil {
		return err
	}
	epoch := g.invalidate(scope)
	if snapshotID == ([32]byte{}) || receiptID == ([32]byte{}) {
		return errors.New("restore requires exact snapshot and receipt identities")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := g.escrow.VerifySnapshotV1(ctx, scope.Run, snapshotID, g.bundle); err != nil {
		return fmt.Errorf("snapshot integrity before owner restore: %w", err)
	}
	cp, locator, err := g.escrow.LoadCognitiveCheckpointV1(ctx, scope.Run,
		receiptID, scope.Namespace, scope.Actor, g.bundle)
	if err != nil {
		return fmt.Errorf("signed owner escrow restore: %w", err)
	}
	if cp.ActorNamespace != scope.Namespace || cp.ActorID != scope.Actor {
		return errors.New("restored checkpoint actor differs from requested scope")
	}
	if _, err := sqlite.CognitiveCheckpointSigningBytesV1(scope.Run, cp, locator); err != nil {
		return err
	}
	digest := identity.OpaqueContentHash(cp.Content)
	restored, err := g.owner.Restore(ctx, RestoreRequest{
		Scope: scope, Locator: locator, ReceiptID: receiptID, ContentHash: digest,
	})
	if err != nil {
		return fmt.Errorf("external owner restore: %w", err)
	}
	if restored.Locator != locator ||
		subtle.ConstantTimeCompare(restored.Content, cp.Content) != 1 {
		return errors.New("external owner restored different checkpoint bytes or locator")
	}
	if err := g.owner.Acknowledge(ctx, RestoreAck{
		Scope: scope, Locator: locator, ReceiptID: receiptID,
		SnapshotID: snapshotID, ContentHash: digest,
	}); err != nil {
		return fmt.Errorf("external owner restore ACK: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.generation[scope] != epoch {
		return errors.New("owner restore superseded by newer stage or restore")
	}
	g.permits[scope] = permit{snapshot: snapshotID, receipt: receiptID}
	return nil
}

// RunWork is the only entry point that an integrating host may use for
// cognitive WORK or LLM. It blocks until RESTORE/ACK for the EXACT run,
// actor, snapshot and receipt. The read lock prevents Stage/Restore revoking
// the permit midway through a callback; callbacks must not re-enter Gate.
func (g *Gate) RunWork(ctx context.Context, scope Scope,
	snapshotID, receiptID [32]byte, work func(context.Context) error) error {
	if g == nil || ctx == nil || work == nil {
		return errors.New("cognitive work gate unconfigured")
	}
	if err := validateScope(scope); err != nil {
		return err
	}
	g.mu.RLock()
	defer g.mu.RUnlock()
	if granted, ok := g.permits[scope]; !ok ||
		granted.snapshot != snapshotID || granted.receipt != receiptID {
		return ErrNotAcknowledged
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return work(ctx)
}

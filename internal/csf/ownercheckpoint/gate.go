package ownercheckpoint

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/subtle"
	"errors"
	"fmt"
	"sync"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
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
	Scope       Scope
	Locator     string
	ReceiptID   [32]byte
	ContentHash [32]byte
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
		permits:    make(map[Scope]permit),
		generation: make(map[Scope]uint64),
	}, nil
}

func validateScope(s Scope) error {
	// A zero-valued 32-byte RunID is legal if the pinned Genesis and
	// signature authenticate it; no extra run-ID policy is invented here.
	if s.Namespace == "" || s.Actor == "" {
		return errors.New("cognitive owner scope requires namespace and actor")
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
	epoch := g.invalidate(scope)
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
	// Serialize the exact escrow publication against a newer Stage/Restore.
	// Without this lock, another generation could begin between the check
	// and SQL insertion and the superseded Stage could publish afterward.
	g.mu.RLock()
	if g.generation[scope] != epoch {
		g.mu.RUnlock()
		return zero, errors.New("superseded owner persist cannot enter escrow")
	}
	receipt, err := g.escrow.PersistCognitiveCheckpointV1(ctx, scope.Run,
		cloneCheckpoint(cp), signed.Locator, bytes.Clone(signed.Signature), g.bundle)
	g.mu.RUnlock()
	if err != nil {
		return zero, fmt.Errorf("signed owner checkpoint escrow: %w", err)
	}
	return receipt, nil
}

// snapshotIncludesCheckpoint proves the signed escrow checkpoint is exactly
// the one committed into the validated snapshot's epistemic reference roster.
// Two individually valid artifacts do not imply this binding.
func snapshotIncludesCheckpoint(snapshot sqlite.SnapshotV1, scope Scope, cp sqlite.SnapshotCheckpoint) error {
	if snapshot.ID == ([32]byte{}) || len(snapshot.Canonical) == 0 {
		return errors.New("verified snapshot canonical bytes absent")
	}
	decoded, err := codec.StrictDecodePayload(snapshot.Canonical)
	if err != nil {
		return err
	}
	fields, ok := decoded.([]any)
	if !ok || len(fields) != 19 {
		return errors.New("verified snapshot schema incompatible")
	}
	runID, ok := fields[0].([]byte)
	if !ok || !bytes.Equal(runID, scope.Run[:]) {
		return errors.New("verified snapshot run differs from owner")
	}
	roster, ok := fields[14].([]any)
	if !ok {
		return errors.New("verified snapshot epistemic roster malformed")
	}
	digest := identity.OpaqueContentHash(cp.Content)
	expected, err := codec.CanonicalPayload([]any{
		[]any{cp.ActorNamespace, cp.ActorID}, cp.StoreID, uint64(cp.SchemaVersion),
		[]any{uint64(1), cp.Content}, digest[:], cp.Producer,
		uint64(cp.ProducerVersion),
		[]any{cp.CoversInstant, cp.CoversRevision, cp.CoversEvidenceCursor},
	})
	if err != nil {
		return err
	}
	found := 0
	for _, raw := range roster {
		value, ok := raw.([]any)
		if !ok || len(value) != 8 {
			return errors.New("verified epistemic reference malformed")
		}
		actor, ok := value[0].([]any)
		if !ok || len(actor) != 2 {
			return errors.New("verified epistemic actor malformed")
		}
		if actor[0] != scope.Namespace || actor[1] != scope.Actor {
			continue
		}
		encoded, err := codec.CanonicalPayload(value)
		if err != nil {
			return err
		}
		if !bytes.Equal(encoded, expected) {
			return errors.New("signed escrow checkpoint differs from snapshot actor reference")
		}
		found++
	}
	if found != 1 {
		return errors.New("signed escrow checkpoint absent or duplicated in sealed snapshot")
	}
	return nil
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
	verified, err := g.escrow.VerifySnapshotV1(ctx, scope.Run, snapshotID, g.bundle)
	if err != nil {
		return fmt.Errorf("snapshot integrity before owner restore: %w", err)
	}
	if verified.Snapshot.ID != snapshotID {
		return errors.New("snapshot verifier returned a different identity")
	}
	if verified.TailCommits != 0 ||
		uint64(verified.Clock.Revision) != verified.Snapshot.Revision {
		return errors.New("historical snapshot cannot authorize new cognitive WORK")
	}
	cp, locator, err := g.escrow.LoadCognitiveCheckpointV1(ctx, scope.Run,
		receiptID, scope.Namespace, scope.Actor, g.bundle)
	if err != nil {
		return fmt.Errorf("signed owner escrow restore: %w", err)
	}
	if cp.ActorNamespace != scope.Namespace || cp.ActorID != scope.Actor {
		return errors.New("restored checkpoint actor differs from requested scope")
	}
	if err := snapshotIncludesCheckpoint(verified.Snapshot, scope, cp); err != nil {
		return fmt.Errorf("signed escrow not bound to sealed snapshot: %w", err)
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
	if g == nil || ctx == nil || work == nil || g.escrow == nil || g.bundle == nil {
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
	// The model callback may happen long after restore. An old permit
	// alone cannot establish that this snapshot is still the live run tip.
	verified, err := g.escrow.VerifySnapshotV1(ctx, scope.Run, snapshotID, g.bundle)
	if err != nil {
		return fmt.Errorf("cognitive WORK snapshot revalidation: %w", err)
	}
	if verified.Snapshot.ID != snapshotID || verified.TailCommits != 0 ||
		uint64(verified.Clock.Revision) != verified.Snapshot.Revision {
		return ErrNotAcknowledged
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return work(ctx)
}

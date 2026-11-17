//go:build linux

package commit

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/admission"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/inputledger"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/ownercheckpoint"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/ownercheckpoint/localdemo"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/ownercheckpoint/localowner"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

// RED: the C4 snapshot is perfectly valid historically, yet after admission
// of the next two scripted slots its revision is unchanged and new cognition
// must NOT start. Exercise both already-ACKed and newly opened owner gates.
func TestReferenceE3SameRevisionFrozenFenceRevokesBothOwners(t *testing.T) {
	ctx := context.Background()
	result, path, ownerPath := c4Fixture(t, ReferenceOptions{})
	b, err := policy.Load(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	run, err := referenceC4DecodeID(result.RunID)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := referenceC4DecodeID(result.Epistemic.SnapshotID)
	if err != nil {
		t.Fatal(err)
	}
	store, err := sqlite.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ledger, err := inputledger.Open(ctx, domain.ID(run), b, store)
	if err != nil {
		t.Fatal(err)
	}
	refs, err := policiesByRole(b)
	if err != nil {
		t.Fatal(err)
	}
	policies := admission.Policies{
		AdmissionOrder: refs["admission-order"],
		CausalIdentity: refs["causal-identity"],
		Fence: refs["admission-fence"], Rules: [][]any{},
	}
	calls := 0
	type actorHost struct {
		actor string
		scope ownercheckpoint.Scope
		ref   localdemo.Reference
		owner *localowner.Store
		host  *localdemo.Host
	}
	actors := make([]actorHost, 0, 2)
	for _, actor := range []string{"a", "b"} {
		ownerFile, receiptHex := ownerPath, result.Epistemic.OwnerReceiptID
		if actor == "a" {
			ownerFile += ".actor-a"
			receiptHex = result.Epistemic.AOwnerReceiptID
		}
		receipt, err := referenceC4DecodeID(receiptHex)
		if err != nil {
			t.Fatal(err)
		}
		o, err := localowner.Open(ctx, ownerFile, referenceC2SyntheticSigner(run, actor))
		if err != nil {
			t.Fatal(err)
		}
		defer o.Close()
		scope := ownercheckpoint.Scope{Run: run, Namespace: "school", Actor: actor}
		h, err := localdemo.New(o, store, b, scope, func(context.Context) error {
			calls++
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		ref := localdemo.Reference{Snapshot: snapshot, Receipt: receipt}
		if err := h.Run(ctx, ref); !errors.Is(err, ownercheckpoint.ErrNotAcknowledged) {
			t.Fatalf("owner %s inherited ACK across reopen: %v", actor, err)
		}
		if err := h.Restore(ctx, ref); err != nil {
			t.Fatal(err)
		}
		if err := h.Run(ctx, ref); err != nil {
			t.Fatal(err)
		}
		actors = append(actors, actorHost{actor, scope, ref, o, h})
	}
	if calls != 2 {
		t.Fatalf("expected exactly one authorized callback per owner, got %d", calls)
	}
	coordinator := New(store, b, policies)
	coordinator.failAfterStep = 1
	if _, err := referenceD2SettleScriptedCognitiveSlots(ctx, store, ledger, b, domain.ID(run),
		coordinator, policies, path); err == nil {
		t.Fatal("injected terminal rollback did not leave a frozen fence")
	}
	verified, err := store.VerifySnapshotV1(ctx, run, snapshot, b)
	if err != nil {
		t.Fatalf("historical C4 snapshot must remain verifiable: %v", err)
	}
	if verified.ControlStatus != 1 || verified.TailCommits != 0 || verified.Clock.Revision != 6 {
		t.Fatal("RED fixture failed to create same-revision frozen fence")
	}
	frozenBytes := referenceE3Capture(t, path, ownerPath)
	checkpoints := make([]sqlite.SnapshotCheckpoint, 0, len(actors))
	for _, a := range actors {
		cp, _, err := store.LoadCognitiveCheckpointV1(ctx, run, a.ref.Receipt, "school", a.actor, b)
		if err != nil {
			t.Fatal(err)
		}
		checkpoints = append(checkpoints, cp)
	}
	// Supply both authentic signed refs, so a roster error cannot mask
	// failure of the writer-locked IDLE check.
	if _, err := store.CreateSnapshotV1AtRevision(ctx, run, b, checkpoints, 6); err == nil ||
		!strings.Contains(err.Error(), "requires IDLE") {
		t.Fatalf("revision-pinned owner snapshot was not denied by the frozen control guard: %v", err)
	}
	for _, a := range actors {
		if err := a.host.Run(ctx, a.ref); !errors.Is(err, ownercheckpoint.ErrNotAcknowledged) {
			t.Errorf("E3 barrier: already acknowledged owner %s invoked new WORK on frozen fence: %v", a.actor, err)
		}
		fresh, err := localdemo.New(a.owner, store, b, a.scope, func(context.Context) error {
			calls++
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := fresh.Restore(ctx, a.ref); err == nil {
			t.Errorf("E3 barrier: new owner %s restored stale cognitive permission on frozen fence", a.actor)
		}
		if err := fresh.Run(ctx, a.ref); !errors.Is(err, ownercheckpoint.ErrNotAcknowledged) {
			t.Errorf("E3 barrier: new owner %s obtained WORK: %v", a.actor, err)
		}
	}
	if calls != 2 {
		t.Fatalf("E3 barrier: denied callbacks actually ran, got %d", calls)
	}
	if err := referenceE1Compare(frozenBytes, referenceE3Capture(t, path, ownerPath)); err != nil {
		t.Fatalf("rejected snapshot/restore/work mutated frozen authorities: %v", err)
	}
}

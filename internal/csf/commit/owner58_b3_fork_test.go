package commit

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/ownercheckpoint"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/ownercheckpoint/localdemo"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/ownercheckpoint/localowner"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

// Create a genuine two-file B3 fixture; the signer and private actor bytes are
// synthetic. The test owns each store, not the causal engine's world truth.
func b3NewOwner(t *testing.T, key ed25519.PrivateKey) *localowner.Store {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	owner, err := localowner.Open(context.Background(), filepath.Join(dir, "owner.sqlite"), key)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close() })
	return owner
}

func TestCSFRV18R58B3ForkChildMustSignForNewRunAndRestoreIndependently(t *testing.T) {
	ctx := context.Background()
	v1, err := policy.Load(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	g := validGenesis(t, v1)
	sourceRun := [32]byte(g.RunID)
	parent, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "parent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	if err := parent.PersistGenesis(ctx, g, v1); err != nil {
		t.Fatal(err)
	}
	if err := parent.BootstrapClock(ctx, g, v1); err != nil {
		t.Fatal(err)
	}
	if err := parent.BootstrapGenesisTriggers(ctx, g, v1, nil); err != nil {
		t.Fatal(err)
	}
	parentScope := ownercheckpoint.Scope{Run: sourceRun, Namespace: "cognition", Actor: "owner-b3"}
	key := b3Key()
	if err := parent.RegisterCognitiveActor(ctx, sourceRun, parentScope.Namespace, parentScope.Actor, v1); err != nil {
		t.Fatal(err)
	}
	if err := parent.RegisterCognitiveOwnerKeyV1(ctx, sourceRun, parentScope.Namespace, parentScope.Actor, key.Public().(ed25519.PublicKey), v1); err != nil {
		t.Fatal(err)
	}
	initial, err := parent.LoadClock(ctx, sourceRun)
	if err != nil {
		t.Fatal(err)
	}
	cp := b3Checkpoint(parentScope, int64(initial.Instant), uint64(initial.Revision))
	parentOwner := b3NewOwner(t, key)
	parentCalls := 0
	parentHost, err := localdemo.New(parentOwner, parent, v1, parentScope, func(context.Context) error { parentCalls++; return nil })
	if err != nil {
		t.Fatal(err)
	}
	parentRef, err := parentHost.StageSnapshot(ctx, cp)
	if err != nil {
		t.Fatal("parent signed sealed snapshot:", err)
	}
	if err := parentHost.Restore(ctx, parentRef); err != nil {
		t.Fatal(err)
	}
	if err := parentHost.Run(ctx, parentRef); err != nil || parentCalls != 1 {
		t.Fatalf("parent permit not proven %d %v", parentCalls, err)
	}
	// Prove fork source snapshot and receipt are real, not test-faked values.
	historical, loc, err := parent.LoadCognitiveCheckpointV1(ctx, sourceRun, parentRef.Receipt, parentScope.Namespace, parentScope.Actor, v1)
	if err != nil || loc == "" || !bytes.Equal(historical.Content, cp.Content) {
		t.Fatalf("parent owner escrow invalid: %v", err)
	}

	v2, err := policy.LoadV2("../../../docs/architecture/canonical-codec-v2-bundle")
	if err != nil {
		t.Fatal(err)
	}
	childPath := filepath.Join(t.TempDir(), "child.db")
	child, err := sqlite.Open(ctx, childPath)
	if err != nil {
		t.Fatal(err)
	}
	childRun := sourceRun
	childRun[0] ^= 0x52
	if childRun == ([32]byte{}) || childRun == sourceRun {
		t.Fatal("child identifier collision")
	}
	root, err := parent.ForkIdleSnapshotV1ToV2(ctx, sourceRun, parentRef.Snapshot, childRun, child, v1, v2)
	if err != nil {
		t.Fatal("signed parent fork rejected:", err)
	}
	if [32]byte(root.RunID) != childRun {
		t.Fatal("fork child identity collision")
	}
	if _, err := child.ReplayClock(ctx, childRun, v2); err != nil {
		t.Fatal("child causal replay:", err)
	}
	if _, err := child.RecoverPublishedForkV1ToV2(ctx, childRun, sourceRun, parentRef.Snapshot, v2); err != nil {
		t.Fatal("child signed lineage:", err)
	}
	childScope := parentScope
	childScope.Run = childRun
	childClock, err := child.LoadClock(ctx, childRun)
	if err != nil {
		t.Fatal(err)
	}
	childCP := b3Checkpoint(childScope, int64(childClock.Instant), uint64(childClock.Revision))
	if _, err := child.CreateSnapshotV1(ctx, childRun, v2, []sqlite.SnapshotCheckpoint{childCP}); err == nil {
		t.Fatal("parent owner receipt was trusted as child run without re-attestation")
	}
	childOwner := b3NewOwner(t, key)
	childCalls := 0
	childHost, err := localdemo.New(childOwner, child, v2, childScope, func(context.Context) error { childCalls++; return nil })
	if err != nil {
		t.Fatal(err)
	}
	if err := childHost.Run(ctx, parentRef); !errors.Is(err, ownercheckpoint.ErrNotAcknowledged) || childCalls != 0 {
		t.Fatal("parent WORK permit crossed fork")
	}
	if err := childHost.Restore(ctx, parentRef); err == nil {
		t.Fatal("parent signed receipt restored on child")
	}
	childRef, err := childHost.StageSnapshot(ctx, childCP)
	if err != nil {
		t.Fatal("child new-run signature/escrow:", err)
	}
	if parentRef.Receipt == childRef.Receipt || parentRef.Snapshot == childRef.Snapshot {
		t.Fatal("child reused parent receipt or snapshot identity")
	}
	if err := childHost.Run(ctx, childRef); !errors.Is(err, ownercheckpoint.ErrNotAcknowledged) || childCalls != 0 {
		t.Fatal("child WORK before its own restore")
	}
	if err := childHost.Restore(ctx, childRef); err != nil {
		t.Fatal("child owner restore:", err)
	}
	if err := childHost.Run(ctx, childRef); err != nil || childCalls != 1 {
		t.Fatalf("child callback count=%d err=%v", childCalls, err)
	}
	if parentCalls != 1 {
		t.Fatal("child fork called parent cognition")
	}
	if err := child.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := sqlite.Open(ctx, childPath)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if _, err := reopened.ReplayClock(ctx, childRun, v2); err != nil {
		t.Fatal("reopened child replay:", err)
	}
	if _, err := reopened.RecoverPublishedForkV1ToV2(ctx, childRun, sourceRun, parentRef.Snapshot, v2); err != nil {
		t.Fatal("reopened child lineage:", err)
	}
	mistakenParent := parentRef.Snapshot
	mistakenParent[0] ^= 0x12
	if got, err := reopened.RecoverPublishedForkV1ToV2(ctx, childRun, sourceRun, mistakenParent, v2); err == nil || got != nil {
		t.Fatal("child fork accepted a different parent snapshot")
	}
	other := childScope
	other.Actor = "foreign-b3"
	wrong, err := localdemo.New(childOwner, reopened, v2, other, func(context.Context) error { childCalls++; return nil })
	if err != nil {
		t.Fatal(err)
	}
	if err := wrong.Restore(ctx, childRef); err == nil {
		t.Fatal("foreign child actor restored memory")
	}
	if err := wrong.Run(ctx, childRef); !errors.Is(err, ownercheckpoint.ErrNotAcknowledged) || childCalls != 1 {
		t.Fatal("foreign child actor called cognition")
	}
	if _, err := parent.VerifySnapshotV1(ctx, sourceRun, parentRef.Snapshot, v1); err != nil {
		t.Fatal("fork changed parent world truth:", err)
	}
}

// b3BarrierOwner introduces a deterministic synchronization cut in the
// *test adapter*, without adding fault toggles to the production owner.
type b3BarrierOwner struct {
	*localowner.Store
	mu             sync.Mutex
	stageEntered   chan struct{}
	stageRelease   chan struct{}
	restoreEntered chan struct{}
	restoreRelease chan struct{}
	ackEntered     chan struct{}
	ackRelease     chan struct{}
}

func (o *b3BarrierOwner) PersistAndSign(ctx context.Context, req ownercheckpoint.PersistRequest) (ownercheckpoint.SignedReceipt, error) {
	o.mu.Lock()
	entered, release := o.stageEntered, o.stageRelease
	o.mu.Unlock()
	if entered != nil {
		close(entered)
		select {
		case <-release:
		case <-ctx.Done():
			return ownercheckpoint.SignedReceipt{}, ctx.Err()
		}
	}
	return o.Store.PersistAndSign(ctx, req)
}
func (o *b3BarrierOwner) Restore(ctx context.Context, req ownercheckpoint.RestoreRequest) (ownercheckpoint.RestoredContent, error) {
	restored, err := o.Store.Restore(ctx, req)
	if err != nil {
		return restored, err
	}
	o.mu.Lock()
	entered, release := o.restoreEntered, o.restoreRelease
	o.mu.Unlock()
	if entered != nil {
		close(entered)
		select {
		case <-release:
		case <-ctx.Done():
			return ownercheckpoint.RestoredContent{}, ctx.Err()
		}
	}
	return restored, nil
}
func (o *b3BarrierOwner) Acknowledge(ctx context.Context, ack ownercheckpoint.RestoreAck) error {
	err := o.Store.Acknowledge(ctx, ack)
	if err != nil {
		return err
	}
	o.mu.Lock()
	entered, release := o.ackEntered, o.ackRelease
	o.mu.Unlock()
	if entered != nil {
		close(entered)
		select {
		case <-release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

// Stage and restore/ACK interleavings must never let a superseded snapshot
// permit execute WORK. Precise barriers avoid time.Sleep and flaky schedules.
func TestCSFRV18R58B3ConcurrentStageAndRestoreRevocation(t *testing.T) {
	ctx := context.Background()
	_, causal, _, run, b, _ := testFixture(t)
	scope := ownercheckpoint.Scope{Run: [32]byte(run), Namespace: "cognition", Actor: "owner-b3"}
	private := b3Key()
	if err := causal.RegisterCognitiveActor(ctx, scope.Run, scope.Namespace, scope.Actor, b); err != nil {
		t.Fatal(err)
	}
	if err := causal.RegisterCognitiveOwnerKeyV1(ctx, scope.Run, scope.Namespace, scope.Actor, private.Public().(ed25519.PublicKey), b); err != nil {
		t.Fatal(err)
	}
	now, err := causal.LoadClock(ctx, scope.Run)
	if err != nil {
		t.Fatal(err)
	}
	cp := b3Checkpoint(scope, int64(now.Instant), uint64(now.Revision))
	o := &b3BarrierOwner{Store: b3NewOwner(t, private)}
	calls := 0
	host, err := localdemo.New(o, causal, b, scope, func(context.Context) error { calls++; return nil })
	if err != nil {
		t.Fatal(err)
	}
	ref, err := host.StageSnapshot(ctx, cp)
	if err != nil {
		t.Fatal(err)
	}
	if err := host.Restore(ctx, ref); err != nil {
		t.Fatal(err)
	}
	if err := host.Run(ctx, ref); err != nil || calls != 1 {
		t.Fatal("initial permit not established", err)
	}
	o.mu.Lock()
	o.stageEntered = make(chan struct{})
	o.stageRelease = make(chan struct{})
	enter, release := o.stageEntered, o.stageRelease
	o.mu.Unlock()
	staged := make(chan error, 1)
	go func() { _, err := host.StageSnapshot(ctx, cp); staged <- err }()
	<-enter
	if err := host.Run(ctx, ref); !errors.Is(err, ownercheckpoint.ErrNotAcknowledged) || calls != 1 {
		t.Fatalf("WORK while Stage waiting on owner: %d %v", calls, err)
	}
	close(release)
	if err := <-staged; err != nil {
		t.Fatal("stage after gate revocation:", err)
	}
	if err := host.Run(ctx, ref); !errors.Is(err, ownercheckpoint.ErrNotAcknowledged) || calls != 1 {
		t.Fatal("Stage granted work without restore")
	}
	o.mu.Lock()
	o.stageEntered = nil
	o.stageRelease = nil
	o.mu.Unlock()
	if err := host.Restore(ctx, ref); err != nil {
		t.Fatal(err)
	}
	if err := host.Run(ctx, ref); err != nil || calls != 2 {
		t.Fatal("fresh restore did not authorize:", err)
	}
	o.mu.Lock()
	o.restoreEntered = make(chan struct{})
	o.restoreRelease = make(chan struct{})
	restoreEnter, restoreRelease := o.restoreEntered, o.restoreRelease
	o.mu.Unlock()
	restored := make(chan error, 1)
	go func() { restored <- host.Restore(ctx, ref) }()
	<-restoreEnter
	o.mu.Lock()
	o.stageEntered = make(chan struct{})
	o.stageRelease = make(chan struct{})
	o.mu.Unlock()
	// Allow stage to fully complete; its generation supersedes blocked restore.
	o.mu.Lock()
	o.stageEntered = nil
	o.stageRelease = nil
	o.mu.Unlock()
	if _, err := host.StageSnapshot(ctx, cp); err != nil {
		t.Fatal(err)
	}
	close(restoreRelease)
	if err := <-restored; err == nil {
		t.Fatal("superseded blocked restore granted permit")
	}
	if err := host.Run(ctx, ref); !errors.Is(err, ownercheckpoint.ErrNotAcknowledged) || calls != 2 {
		t.Fatal("superseded restore granted work")
	}
	o.mu.Lock()
	o.restoreEntered = nil
	o.restoreRelease = nil
	o.mu.Unlock()
	if err := host.Restore(ctx, ref); err != nil {
		t.Fatal(err)
	}
	if err := host.Run(ctx, ref); err != nil || calls != 3 {
		t.Fatal("fresh restore after stale cut:", err)
	}
	o.mu.Lock()
	o.ackEntered = make(chan struct{})
	o.ackRelease = make(chan struct{})
	ackEnter, ackRelease := o.ackEntered, o.ackRelease
	o.mu.Unlock()
	acknowledged := make(chan error, 1)
	go func() { acknowledged <- host.Restore(ctx, ref) }()
	<-ackEnter
	if _, err := host.StageSnapshot(ctx, cp); err != nil {
		t.Fatal(err)
	}
	close(ackRelease)
	if err := <-acknowledged; err == nil {
		t.Fatal("superseded ACK granted permit")
	}
	if err := host.Run(ctx, ref); !errors.Is(err, ownercheckpoint.ErrNotAcknowledged) || calls != 3 {
		t.Fatal("superseded ACK granted new work")
	}
	o.mu.Lock()
	o.ackEntered = nil
	o.ackRelease = nil
	o.mu.Unlock()
	if err := host.Restore(ctx, ref); err != nil {
		t.Fatal(err)
	}
	if err := host.Run(ctx, ref); err != nil || calls != 4 {
		t.Fatal("final valid restored work failed", err)
	}
}

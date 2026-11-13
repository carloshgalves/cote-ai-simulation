package commit

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/ownercheckpoint"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/ownercheckpoint/localdemo"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

// This test is an isolated OS process, not an in-process mock of fork commit.
// The child disappears after the V2 bootstrap transaction succeeds, before
// returning the child Genesis to the source orchestrator.
func TestCSFRV18R58B3ForkExitHelper(t *testing.T) {
	if os.Getenv("COTE_B3_FORK_CHILD") != "isolated-test-process" {
		return
	}
	decode := func(s string) [32]byte {
		t.Helper()
		raw, err := hex.DecodeString(s)
		if err != nil || len(raw) != 32 {
			t.Fatal("malformed synthetic fork fixture")
		}
		var id [32]byte
		copy(id[:], raw)
		return id
	}
	ctx := context.Background()
	v1, err := policy.Load(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	v2, err := policy.LoadV2("../../../docs/architecture/canonical-codec-v2-bundle")
	if err != nil {
		t.Fatal(err)
	}
	source, err := sqlite.Open(ctx, os.Getenv("COTE_B3_FORK_SOURCE"))
	if err != nil {
		t.Fatal(err)
	}
	child, err := sqlite.Open(ctx, os.Getenv("COTE_B3_FORK_DEST"))
	if err != nil {
		t.Fatal(err)
	}
	sourceRun := decode(os.Getenv("COTE_B3_FORK_SOURCE_RUN"))
	childRun := decode(os.Getenv("COTE_B3_FORK_CHILD_RUN"))
	snapshot := decode(os.Getenv("COTE_B3_FORK_SNAPSHOT"))
	if _, err := source.ForkIdleSnapshotV1ToV2(ctx, sourceRun, snapshot, childRun, child, v1, v2); err != nil {
		t.Fatal("fork bootstrap never committed:", err)
	}
	// Deliberately no source.Close/child.Close; prove recovery from OS exit.
	os.Exit(b3CrashExit)
}

func TestCSFRV18R58B3ProcessDiesAfterV2ForkCommit(t *testing.T) {
	ctx := context.Background()
	v1, err := policy.Load(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	v2, err := policy.LoadV2("../../../docs/architecture/canonical-codec-v2-bundle")
	if err != nil {
		t.Fatal(err)
	}
	genesis := validGenesis(t, v1)
	sourceRun := [32]byte(genesis.RunID)
	sourcePath := filepath.Join(t.TempDir(), "source-v1.db")
	source, err := sqlite.Open(ctx, sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := source.PersistGenesis(ctx, genesis, v1); err != nil {
		t.Fatal(err)
	}
	if err := source.BootstrapClock(ctx, genesis, v1); err != nil {
		t.Fatal(err)
	}
	if err := source.BootstrapGenesisTriggers(ctx, genesis, v1, nil); err != nil {
		t.Fatal(err)
	}
	scope := ownercheckpoint.Scope{Run: sourceRun, Namespace: "cognition", Actor: "owner-b3"}
	key := b3Key()
	if err := source.RegisterCognitiveActor(ctx, sourceRun, scope.Namespace, scope.Actor, v1); err != nil {
		t.Fatal(err)
	}
	if err := source.RegisterCognitiveOwnerKeyV1(ctx, sourceRun, scope.Namespace, scope.Actor, key.Public().(ed25519.PublicKey), v1); err != nil {
		t.Fatal(err)
	}
	state, err := source.LoadClock(ctx, sourceRun)
	if err != nil {
		t.Fatal(err)
	}
	cp := b3Checkpoint(scope, int64(state.Instant), uint64(state.Revision))
	owner := b3NewOwner(t, key)
	host, err := localdemo.New(owner, source, v1, scope, func(context.Context) error { t.Fatal("unexpected WORK before parent snapshot"); return nil })
	if err != nil {
		t.Fatal(err)
	}
	ref, err := host.StageSnapshot(ctx, cp)
	if err != nil {
		t.Fatal(err)
	}
	ledgerBefore := b3LedgerDigest(t, sourcePath)
	childRun := sourceRun
	childRun[0] ^= 0x61
	if childRun == sourceRun || childRun == ([32]byte{}) {
		t.Fatal("invalid child fixture")
	}
	childPath := filepath.Join(t.TempDir(), "child-v2.db")
	if err := owner.Close(); err != nil {
		t.Fatal(err)
	}
	if err := source.Close(); err != nil {
		t.Fatal(err)
	}
	timed, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	cmd := exec.CommandContext(timed, os.Args[0], "-test.run=^TestCSFRV18R58B3ForkExitHelper$", "-test.v=false")
	cmd.Env = append(os.Environ(),
		"COTE_B3_FORK_CHILD=isolated-test-process",
		"COTE_B3_FORK_SOURCE="+sourcePath,
		"COTE_B3_FORK_DEST="+childPath,
		"COTE_B3_FORK_SOURCE_RUN="+hex.EncodeToString(sourceRun[:]),
		"COTE_B3_FORK_CHILD_RUN="+hex.EncodeToString(childRun[:]),
		"COTE_B3_FORK_SNAPSHOT="+hex.EncodeToString(ref.Snapshot[:]))
	output, err := cmd.CombinedOutput()
	var e *exec.ExitError
	if timed.Err() != nil {
		t.Fatal("fork child timed out:", timed.Err())
	}
	if !errors.As(err, &e) || e.ExitCode() != b3CrashExit || len(output) != 0 {
		t.Fatalf("child fork did not exit at durable commit: exit=%v err=%v output=%s", cmd.ProcessState, err, string(output))
	}
	// Now the caller has no returned child Genesis: it must authenticate
	// the already committed child from its OWN durable SQLite authority.
	sourceReopen, err := sqlite.Open(ctx, sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	defer sourceReopen.Close()
	child, err := sqlite.Open(ctx, childPath)
	if err != nil {
		t.Fatal(err)
	}
	defer child.Close()
	recovered, err := child.RecoverPublishedForkV1ToV2(ctx, childRun, sourceRun, ref.Snapshot, v2)
	if err != nil || recovered == nil || [32]byte(recovered.RunID) != childRun {
		t.Fatalf("committed V2 child not recoverable after OS exit: %v", err)
	}
	if _, err := child.ReplayClock(ctx, childRun, v2); err != nil {
		t.Fatal("child replay broken:", err)
	}
	if got := b3Rows(t, childPath, "fork_cognitive_owner_proofs"); got != 1 {
		t.Fatalf("signed parent proof not migrated: %d", got)
	}
	if _, err := sourceReopen.VerifySnapshotV1(ctx, sourceRun, ref.Snapshot, v1); err != nil {
		t.Fatal("fork changed source:", err)
	}
	if got := b3LedgerDigest(t, sourcePath); got != ledgerBefore {
		t.Fatal("process-crashed fork altered parent five-ledger prefixes")
	}
	childLedgerBefore := b3LedgerDigest(t, childPath)
	incorrectParent := ref.Snapshot
	incorrectParent[0] ^= 0x14
	if got, err := child.RecoverPublishedForkV1ToV2(ctx, childRun, sourceRun, incorrectParent, v2); err == nil || got != nil {
		t.Fatal("wrong parent fork cut accepted")
	}
	// The historical parent signer is imported for lineage, but no new
	// cognitive WORK is permitted for childRun until that run has its own
	// attested checkpoint and local restore/ACK.
	childScope := scope
	childScope.Run = childRun
	childClock, err := child.LoadClock(ctx, childRun)
	if err != nil {
		t.Fatal(err)
	}
	childCP := b3Checkpoint(childScope, int64(childClock.Instant), uint64(childClock.Revision))
	if _, err := child.CreateSnapshotV1(ctx, childRun, v2, []sqlite.SnapshotCheckpoint{childCP}); err == nil {
		t.Fatal("child accepted parent's receipt without child-run signing")
	}
	childOwner := b3NewOwner(t, key)
	calls := 0
	childHost, err := localdemo.New(childOwner, child, v2, childScope, func(context.Context) error { calls++; return nil })
	if err != nil {
		t.Fatal(err)
	}
	if err := childHost.Run(ctx, ref); !errors.Is(err, ownercheckpoint.ErrNotAcknowledged) || calls != 0 {
		t.Fatal("fork child inherited parent WORK permit")
	}
	childRef, err := childHost.StageSnapshot(ctx, childCP)
	if err != nil {
		t.Fatal(err)
	}
	if childRef.Receipt == ref.Receipt || childRef.Snapshot == ref.Snapshot {
		t.Fatal("child reused parent authority")
	}
	if err := childHost.Restore(ctx, ref); err == nil {
		t.Fatal("parent receipt restored on new child")
	}
	if err := childHost.Restore(ctx, childRef); err != nil {
		t.Fatal(err)
	}
	if err := childHost.Run(ctx, childRef); err != nil || calls != 1 {
		t.Fatalf("fork child failed after exact owner ACK: calls=%d err=%v", calls, err)
	}
	if got := b3LedgerDigest(t, childPath); got != childLedgerBefore {
		t.Fatal("child owner re-attestation changed child world-truth ledgers")
	}
	if got := b3LedgerDigest(t, sourcePath); got != ledgerBefore {
		t.Fatal("child cognition mutated parent world-truth ledgers")
	}
	parentBytes, _, err := sourceReopen.LoadCognitiveCheckpointV1(ctx, sourceRun, ref.Receipt, scope.Namespace, scope.Actor, v1)
	if err != nil || !bytes.Equal(parentBytes.Content, cp.Content) {
		t.Fatal("fork recovery changed parent opaque bytes:", err)
	}
}

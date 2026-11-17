//go:build linux

package commit

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/admission"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/inputledger"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/ownercheckpoint"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/ownercheckpoint/localdemo"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/ownercheckpoint/localowner"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

func referenceE4Bundle(t *testing.T) *policy.Bundle {
	t.Helper()
	b, err := policy.LoadV2("../../../docs/architecture/canonical-codec-v2-bundle")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// Compare actual archive members to the parent snapshot's sealed membership,
// not a digest of the live parent tip or a count of unrelated records.
func referenceE4Prefix(t *testing.T, path string, id [32]byte, archive bool) referenceE1Snapshot {
	t.Helper()
	ctx := context.Background()
	db, err := openReferenceReadOnlyDB(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	tables := map[string]referenceE1Table{}
	for _, name := range referenceC4LedgerNames {
		query := fmt.Sprintf("SELECT m.ordinal,m.record_id,t.canonical FROM snapshot_prefix_members m JOIN %s t ON t.id=m.record_id WHERE m.snapshot_id=x'%x' AND m.ledger_name='%s'", name, id, name)
		if archive {
			query = fmt.Sprintf("SELECT ordinal,record_id,canonical FROM fork_history_records WHERE child_run=x'%x' AND ledger_name='%s'", id, name)
		}
		table, err := referenceE1ReadQuery(ctx, tx, name, query)
		if err != nil || len(table.Rows) == 0 {
			t.Fatalf("missing actual E4 archive members for %s: %v", name, err)
		}
		tables[name] = table
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return referenceE1Snapshot{Databases: map[string]referenceE1Database{"prefix": {Tables: tables}}}
}

func referenceE4Equal(t *testing.T, left, right referenceE1Snapshot) {
	t.Helper()
	if err := referenceE1Compare(left, right); err != nil {
		t.Fatal(err)
	}
}

func referenceE4Policies(t *testing.T, b *policy.Bundle) admission.Policies {
	t.Helper()
	refs, err := policiesByRole(b)
	if err != nil {
		t.Fatal(err)
	}
	return admission.Policies{AdmissionOrder: refs["admission-order"], CausalIdentity: refs["causal-identity"], Fence: refs["admission-fence"], Rules: [][]any{}}
}

func referenceE4ChildOwners(t *testing.T, parent, child *sqlite.Store, v1, b *policy.Bundle, parentRun, childRun, parentSnapshot [32]byte, parentReceipts [2][32]byte) ([32]byte, [2][32]byte, [2]string) {
	t.Helper()
	ctx := context.Background()
	state, err := child.LoadClock(ctx, childRun)
	if err != nil || state.Revision != 0 {
		t.Fatalf("missing child genesis clock: %v", err)
	}
	var receipts [2][32]byte
	var paths [2]string
	var cps []sqlite.SnapshotCheckpoint
	var hosts []*localdemo.Host
	var calls [2]int
	for i, actor := range []string{"a", "b"} {
		// The existing V2 migration imports the historical actor public key;
		// the new receipt still binds the child's RunID and an independent DB.
		// Re-attest the exact opaque synthetic parent bytes; do not
		// reconstruct cognition from archived EventStore or BodyState.
		dir := t.TempDir()
		if err := os.Chmod(dir, 0700); err != nil {
			t.Fatal(err)
		}
		paths[i] = filepath.Join(dir, "owner.db")
		o, err := localowner.Open(ctx, paths[i], referenceC2SyntheticSigner(parentRun, actor))
		if err != nil {
			t.Fatal(err)
		}
		defer o.Close()
		scope := ownercheckpoint.Scope{Run: childRun, Namespace: "school", Actor: actor}
		h, err := localdemo.New(o, child, b, scope, func(context.Context) error { calls[i]++; return nil })
		if err != nil {
			t.Fatal(err)
		}
		hosts = append(hosts, h)
		foreign := localdemo.Reference{Snapshot: parentSnapshot, Receipt: parentReceipts[i]}
		if err := h.Restore(ctx, foreign); err == nil {
			t.Fatal("parent receipt granted child cognition")
		}
		if err := h.Run(ctx, foreign); !errors.Is(err, ownercheckpoint.ErrNotAcknowledged) || calls[i] != 0 {
			t.Fatal("parent permit crossed into child")
		}
		cp, _, err := parent.LoadCognitiveCheckpointV1(ctx, parentRun, parentReceipts[i], "school", actor, v1)
		if err != nil {
			t.Fatal(err)
		}
		cp.StoreID = "reference-e4-" + actor
		cp.CoversInstant = int64(state.Instant)
		cp.CoversRevision = 0
		cp.CoversEvidenceCursor = 0
		gate, err := ownercheckpoint.NewGate(o, child, b)
		if err != nil {
			t.Fatal(err)
		}
		receipts[i], err = gate.Stage(ctx, scope, cp)
		if err != nil || receipts[i] == parentReceipts[i] {
			t.Fatalf("failed child-specific receipt: %v", err)
		}
		cps = append(cps, cp)
	}
	if bytes.Equal(cps[0].Content, cps[1].Content) {
		t.Fatal("C4 opaque A/B checkpoint fixture is not distinct")
	}
	if receipts[0] == receipts[1] {
		t.Fatal("A/B owner receipt collision")
	}
	snap, err := child.CreateSnapshotV1AtRevision(ctx, childRun, b, cps, 0)
	if err != nil {
		t.Fatal(err)
	}
	for i, h := range hosts {
		ref := localdemo.Reference{Snapshot: snap.ID, Receipt: receipts[i]}
		if err := h.Run(ctx, ref); !errors.Is(err, ownercheckpoint.ErrNotAcknowledged) {
			t.Fatal("child work without ACK")
		}
		if err := h.Restore(ctx, localdemo.Reference{Snapshot: snap.ID, Receipt: receipts[1-i]}); err == nil {
			t.Fatal("child actor accepted sibling actor receipt")
		}
		if err := h.Restore(ctx, ref); err != nil {
			t.Fatal(err)
		}
		restored, _, err := child.LoadCognitiveCheckpointV1(ctx, childRun, receipts[i], "school", cps[i].ActorID, b)
		if err != nil || !bytes.Equal(restored.Content, cps[i].Content) {
			t.Fatal("child receipt did not preserve the exact opaque parent bytes", err)
		}
		if err := h.Run(ctx, ref); err != nil || calls[i] != 1 {
			t.Fatalf("child owner did not obtain scoped permit: %v", err)
		}
	}
	return snap.ID, receipts, paths
}

// This is a native NoProposal terminal on migrated C4 pending actor rounds,
// not a SQL insertion or a replay of a parent CycleCommit.
func referenceE4CommitChild(t *testing.T, child *sqlite.Store, path string, id [32]byte, b *policy.Bundle, reason uint64) [32]byte {
	t.Helper()
	ctx := context.Background()
	run := domain.ID(id)
	l, err := inputledger.Open(ctx, run, b, child)
	if err != nil {
		t.Fatal(err)
	}
	state, err := child.LoadClock(ctx, id)
	if err != nil || state.Revision != 0 {
		t.Fatalf("unexpected fork clock: %v", err)
	}
	_, _, _, rounds, _, gotRun := l.Snapshot()
	if gotRun != run {
		t.Fatal("child ledger belongs to wrong run")
	}
	var active []inputledger.Round
	for _, round := range rounds {
		if round.Lifecycle == 0 {
			active = append(active, round)
		}
	}
	if len(active) != 2 {
		t.Fatalf("wanted two migrated C4 actor rounds, got %d", len(active))
	}
	cycle, err := referenceC1Cycle(ctx, child, b, run, state, active[0].Eligibility.NotBeforeCycleOrdinal)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, round := range active {
		if round.Actor[0] != "school" || (round.Actor[1] != "a" && round.Actor[1] != "b") || seen[round.Actor[1]] || len(round.Slots) != 1 || round.Slots[0] != "act" || round.Eligibility != active[0].Eligibility || round.Eligibility.NotBeforeInstant != state.Instant {
			t.Fatal("migrated C4 round roster or eligibility differs")
		}
		seen[round.Actor[1]] = true
		d := r19Dispatch(t, b, run, cycle.ID, round, "act", 0, state.Revision, state.Instant)
		if err := l.Respond(ctx, r19NoProposal(t, b, d, reason)); err != nil {
			t.Fatal(err)
		}
	}
	p := referenceE4Policies(t, b)
	fence, err := admission.NewCoordinator(admission.NewFenceLog(child)).AdmitThenEvaluate(ctx, b, l, cycle, p, nil)
	if err != nil {
		t.Fatal(err)
	}
	journal, err := New(child, b, p).CommitNoProposal(ctx, l, cycle, fence)
	if err != nil {
		t.Fatal(err)
	}
	if err := referenceCompleteC1Tasks(ctx, child, b, path, run, 1); err != nil {
		t.Fatal(err)
	}
	replay, err := child.ReplayClock(ctx, id, b)
	if err != nil || replay.ReplayedCommits != 1 || replay.ReconstructedClock.Revision != 1 {
		t.Fatalf("child terminal failed replay: %v", err)
	}
	return journal.Identity
}

func TestReferenceE4C4TwoChildrenAndIndependentTails(t *testing.T) {
	ctx := context.Background()
	parentResult, parentPath, parentOwner := c4Fixture(t, ReferenceOptions{})
	v1, err := policy.Load(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	v2 := referenceE4Bundle(t)
	parent, err := sqlite.Open(ctx, parentPath)
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	run := referenceC2Decode32(t, parentResult.RunID)
	snap := referenceC2Decode32(t, parentResult.Epistemic.SnapshotID)
	receipts := [2][32]byte{referenceC2Decode32(t, parentResult.Epistemic.AOwnerReceiptID), referenceC2Decode32(t, parentResult.Epistemic.OwnerReceiptID)}
	before := referenceE3Capture(t, parentPath, parentOwner)
	prefix := referenceE4Prefix(t, parentPath, snap, false)
	var children [2]*sqlite.Store
	var paths [2]string
	var ids [2][32]byte
	var childSnaps [2][32]byte
	var childReceipts [2][2][32]byte
	var ownerPaths [2][2]string
	var genesisIDs [2]domain.ID
	for i := range children {
		ids[i] = run
		ids[i][0] ^= byte(0x51 + i)
		paths[i] = filepath.Join(t.TempDir(), "child.db")
		child, err := sqlite.Open(ctx, paths[i])
		if err != nil {
			t.Fatal(err)
		}
		children[i] = child
		t.Cleanup(func() { _ = child.Close() })
		root, err := parent.ForkClosedReferenceV1ToV2(ctx, run, snap, ids[i], child, v1, v2)
		if err != nil {
			t.Fatalf("C4 closed fork %d: %v", i, err)
		}
		genesisIDs[i] = root.ID
		if [32]byte(root.RunID) != ids[i] || root.Payload[5] != policy.CodecPolicyV2ID {
			t.Fatal("child Genesis identity/policy mismatch")
		}
		referenceE4Equal(t, prefix, referenceE4Prefix(t, paths[i], ids[i], true))
		if _, err := child.RecoverPublishedForkV1ToV2(ctx, ids[i], run, snap, v2); err != nil {
			t.Fatal(err)
		}
		// Parent history cannot be imported as live Event or Evidence authority.
		initial := referenceE2CausalTranscript(t, paths[i])
		for _, table := range []string{"event_store", "evidence_ledger", "causal_outbox", "input_source_cursors", "source_ingress_receipts"} {
			if len(initial.Databases["causal"].Tables[table].Rows) != 0 {
				t.Fatalf("historical %s promoted to child authority", table)
			}
		}
		childSnaps[i], childReceipts[i], ownerPaths[i] = referenceE4ChildOwners(t, parent, child, v1, v2, run, ids[i], snap, receipts)
	}
	if genesisIDs[0] == genesisIDs[1] || childSnaps[0] == childSnaps[1] || childReceipts[0][0] == childReceipts[1][0] || childReceipts[0][1] == childReceipts[1][1] {
		t.Fatal("children reused causal or owner identities")
	}
	referenceE4Equal(t, before, referenceE3Capture(t, parentPath, parentOwner))
	// Same actor signing key is inherited by design, so this negative must
	// distinguish the sibling by run, not merely by actor or key mismatch.
	for i, child := range children {
		for j, actor := range []string{"a", "b"} {
			o, err := localowner.Open(ctx, ownerPaths[i][j], referenceC2SyntheticSigner(run, actor))
			if err != nil {
				t.Fatal(err)
			}
			calls := 0
			h, err := localdemo.New(o, child, v2, ownercheckpoint.Scope{Run: ids[i], Namespace: "school", Actor: actor}, func(context.Context) error { calls++; return nil })
			if err != nil {
				t.Fatal(err)
			}
			own := localdemo.Reference{Snapshot: childSnaps[i], Receipt: childReceipts[i][j]}
			foreign := localdemo.Reference{Snapshot: childSnaps[1-i], Receipt: childReceipts[1-i][j]}
			if err := h.Restore(ctx, own); err != nil {
				t.Fatal(err)
			}
			if err := h.Restore(ctx, foreign); err == nil {
				t.Fatal("same actor restored private sibling-run checkpoint")
			}
			if err := h.Run(ctx, own); !errors.Is(err, ownercheckpoint.ErrNotAcknowledged) || calls != 0 {
				t.Fatal("failed sibling restore did not revoke local permit")
			}
			if err := o.Close(); err != nil {
				t.Fatal(err)
			}
		}
	}
	ownersBefore, err := referenceE1Read(ctx, []referenceE1Location{
		{Name: "first-a", Path: ownerPaths[0][0]}, {Name: "first-b", Path: ownerPaths[0][1]},
		{Name: "second-a", Path: ownerPaths[1][0]}, {Name: "second-b", Path: ownerPaths[1][1]},
	})
	if err != nil {
		t.Fatal(err)
	}
	secondBefore := referenceE2CausalTranscript(t, paths[1])
	first := referenceE4CommitChild(t, children[0], paths[0], ids[0], v2, 0)
	referenceE4Equal(t, secondBefore, referenceE2CausalTranscript(t, paths[1]))
	firstBefore := referenceE2CausalTranscript(t, paths[0])
	second := referenceE4CommitChild(t, children[1], paths[1], ids[1], v2, 1)
	if first == second || first == ([32]byte{}) || second == ([32]byte{}) {
		t.Fatal("independent tails reused CycleCommit identity")
	}
	referenceE4Equal(t, firstBefore, referenceE2CausalTranscript(t, paths[0]))
	referenceE4Equal(t, before, referenceE3Capture(t, parentPath, parentOwner))
	childBefore := [2]referenceE1Snapshot{referenceE2CausalTranscript(t, paths[0]), referenceE2CausalTranscript(t, paths[1])}
	// A real later parent terminal cannot introduce any new child fact.
	ledger, err := inputledger.Open(ctx, domain.ID(run), v1, parent)
	if err != nil {
		t.Fatal(err)
	}
	p := referenceE4Policies(t, v1)
	if _, err := referenceD2SettleScriptedCognitiveSlots(ctx, parent, ledger, v1, domain.ID(run), New(parent, v1, p), p, parentPath); err != nil {
		t.Fatal(err)
	}
	for i, child := range children {
		if err := child.Close(); err != nil {
			t.Fatal(err)
		}
		reopened, err := sqlite.Open(ctx, paths[i])
		if err != nil {
			t.Fatal(err)
		}
		defer reopened.Close()
		if _, err := reopened.VerifySnapshotV1(ctx, ids[i], childSnaps[i], v2); err != nil {
			t.Fatal(err)
		}
		referenceE4Equal(t, prefix, referenceE4Prefix(t, paths[i], ids[i], true))
		referenceE4Equal(t, childBefore[i], referenceE2CausalTranscript(t, paths[i]))
		if _, err := reopened.RecoverPublishedForkV1ToV2(ctx, ids[i], run, snap, v2); err != nil {
			t.Fatal(err)
		}
		if _, err := reopened.RecoverPublishedForkV1ToV2(ctx, ids[i], ids[1-i], childSnaps[1-i], v2); err == nil {
			t.Fatal("fork accepted sibling as historical origin")
		}
		referenceE4Equal(t, childBefore[i], referenceE2CausalTranscript(t, paths[i]))
	}
	ownersAfter, err := referenceE1Read(ctx, []referenceE1Location{
		{Name: "first-a", Path: ownerPaths[0][0]}, {Name: "first-b", Path: ownerPaths[0][1]},
		{Name: "second-a", Path: ownerPaths[1][0]}, {Name: "second-b", Path: ownerPaths[1][1]},
	})
	if err != nil {
		t.Fatal(err)
	}
	referenceE4Equal(t, ownersBefore, ownersAfter)
}

func TestReferenceE4RejectFrozenTipWrongPolicyAndCorruptSnapshot(t *testing.T) {
	ctx := context.Background()
	result, path, owner := c4Fixture(t, ReferenceOptions{})
	v1, err := policy.Load(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	v2 := referenceE4Bundle(t)
	run := referenceC2Decode32(t, result.RunID)
	snap := referenceC2Decode32(t, result.Epistemic.SnapshotID)
	parent, err := sqlite.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	childID := run
	childID[0] ^= 0x73
	childPath := filepath.Join(t.TempDir(), "rejected.db")
	child, err := sqlite.Open(ctx, childPath)
	if err != nil {
		t.Fatal(err)
	}
	defer child.Close()
	before := referenceE3Capture(t, path, owner)
	if _, err := parent.ForkClosedReferenceV1ToV2(ctx, run, snap, childID, child, v1, v1); err == nil {
		t.Fatal("wrong child policy accepted")
	}
	assertCSFRV18EmptyRejectedFork(t, childPath)
	referenceE4Equal(t, before, referenceE3Capture(t, path, owner))
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	var raw []byte
	if err := db.QueryRowContext(ctx, "SELECT canonical FROM snapshots WHERE id=?", snap[:]).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	decoded, err := codec.StrictDecodePayload(raw)
	if err != nil {
		t.Fatal(err)
	}
	fields := decoded.([]any)
	fields[18] = bytes.Repeat([]byte{0x55}, 32)
	mutant, err := codec.CanonicalPayload(fields)
	if err != nil {
		t.Fatal(err)
	}
	n, err := db.ExecContext(ctx, "UPDATE snapshots SET canonical=? WHERE id=?", mutant, snap[:])
	if err != nil {
		t.Fatal(err)
	}
	if count, err := n.RowsAffected(); err != nil || count != 1 {
		t.Fatal("mutation did not target the sealed source snapshot")
	}
	if _, err := parent.ForkClosedReferenceV1ToV2(ctx, run, snap, childID, child, v1, v2); err == nil {
		t.Fatal("corrupt source snapshot accepted")
	}
	assertCSFRV18EmptyRejectedFork(t, childPath)
	if _, err := db.ExecContext(ctx, "UPDATE snapshots SET canonical=? WHERE id=?", raw, snap[:]); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	referenceE4Equal(t, before, referenceE3Capture(t, path, owner))
	ledger, err := inputledger.Open(ctx, domain.ID(run), v1, parent)
	if err != nil {
		t.Fatal(err)
	}
	p := referenceE4Policies(t, v1)
	c := New(parent, v1, p)
	c.failAfterStep = 1
	if _, err := referenceD2SettleScriptedCognitiveSlots(ctx, parent, ledger, v1, domain.ID(run), c, p, path); err == nil {
		t.Fatal("missing genuine frozen fence")
	}
	proof, err := parent.VerifySnapshotV1(ctx, run, snap, v1)
	if err != nil || proof.ControlStatus != 1 || proof.TailCommits != 0 || proof.OwnerWorkReady {
		t.Fatalf("negative fixture did not leave same-revision frozen fence: %v", err)
	}
	frozen := referenceE3Capture(t, path, owner)
	if _, err := parent.ForkClosedReferenceV1ToV2(ctx, run, snap, childID, child, v1, v2); err == nil || !strings.Contains(err.Error(), "current settled epistemic cut") {
		t.Fatalf("reference fork failed to deny live fence for the right reason: %v", err)
	}
	assertCSFRV18EmptyRejectedFork(t, childPath)
	referenceE4Equal(t, frozen, referenceE3Capture(t, path, owner))
}

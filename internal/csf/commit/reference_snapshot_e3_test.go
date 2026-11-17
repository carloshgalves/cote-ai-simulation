//go:build linux

package commit

import (
	"context"
	"errors"
	"testing"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/contextboundary"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/ownercheckpoint"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/ownercheckpoint/localdemo"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/ownercheckpoint/localowner"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

func referenceE3Capture(t *testing.T, path, owner string) referenceE1Snapshot {
	t.Helper()
	proof, err := referenceE1Read(context.Background(), []referenceE1Location{
		{Name: "causal", Path: path},
		{Name: "owner-a", Path: owner + ".actor-a"},
		{Name: "owner-b", Path: owner},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"owner-a", "owner-b"} {
		if len(proof.Databases[name].Tables["checkpoints"].Rows) == 0 ||
			len(proof.Databases[name].Tables["restore_acks"].Rows) == 0 {
			t.Fatalf("missing %s private checkpoint or ACK", name)
		}
	}
	return proof
}

func TestReferenceE3C4JointOwnerReopenByteExactAndNegativeReceipts(t *testing.T) {
	ctx := context.Background()
	result, path, owner := c4Fixture(t, ReferenceOptions{})
	before := referenceE3Capture(t, path, owner)
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
	receiptA, err := referenceC4DecodeID(result.Epistemic.AOwnerReceiptID)
	if err != nil {
		t.Fatal(err)
	}
	receiptB, err := referenceC4DecodeID(result.Epistemic.OwnerReceiptID)
	if err != nil {
		t.Fatal(err)
	}
	claim, err := referenceC4DecodeID(result.Communication.KnowledgeInputID)
	if err != nil {
		t.Fatal(err)
	}
	for restart := 0; restart < 2; restart++ {
		causal, err := sqlite.Open(ctx, path)
		if err != nil {
			t.Fatal(err)
		}
		verified, err := causal.VerifySnapshotV1(ctx, run, snapshot, b)
		if err != nil || !verified.OwnerWorkReady || verified.TailCommits != 0 {
			causal.Close()
			t.Fatalf("settled joint snapshot is not current: %v", err)
		}
		dispatches, err := func() ([2][32]byte, error) {
			ro, err := openReferenceReadOnlyDB(path)
			if err != nil {
				return [2][32]byte{}, err
			}
			defer ro.Close()
			return referenceD3ActorDispatches(ctx, ro, run)
		}()
		if err != nil {
			causal.Close()
			t.Fatal(err)
		}
		for _, tc := range []struct {
			actor, file string
			receipt, foreign, dispatch [32]byte
		}{
			{"a", owner + ".actor-a", receiptA, receiptB, dispatches[0]},
			{"b", owner, receiptB, receiptA, dispatches[1]},
		} {
			calls := 0
			o, err := localowner.Open(ctx, tc.file, referenceC2SyntheticSigner(run, tc.actor))
			if err != nil {
				t.Fatal(err)
			}
			scope := ownercheckpoint.Scope{Run: run, Namespace: "school", Actor: tc.actor}
			h, err := localdemo.New(o, causal, b, scope, func(ctx context.Context) error {
				reader, err := causal.BindActorInbox(ctx, tc.dispatch, [2]string{"school", tc.actor})
				if err != nil {
					return err
				}
				view, err := contextboundary.Build(ctx, reader, [2]string{"school", tc.actor})
				if err != nil {
					return err
				}
				matches := 0
				for _, entry := range view.Inbox {
					if entry.Input.Identity == claim {
						matches++
					}
				}
				if (tc.actor == "a" && matches != 0) || (tc.actor == "b" && matches != 1) {
					return errors.New("restored owner has wrong recipient-specific evidence")
				}
				calls++
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			ref := localdemo.Reference{Snapshot: snapshot, Receipt: tc.receipt}
			if err := h.Run(ctx, ref); !errors.Is(err, ownercheckpoint.ErrNotAcknowledged) {
				t.Fatal("fresh gate inherited ACK")
			}
			if err := h.Restore(ctx, ref); err != nil {
				t.Fatal(err)
			}
			foreign := localdemo.Reference{Snapshot: snapshot, Receipt: tc.foreign}
			if err := h.Restore(ctx, foreign); err == nil {
				t.Fatal("foreign actor receipt accepted")
			}
			if err := h.Run(ctx, ref); !errors.Is(err, ownercheckpoint.ErrNotAcknowledged) {
				t.Fatal("failed restore did not revoke existing local permit")
			}
			bad := ref
			bad.Snapshot[0] ^= 0x80
			if err := h.Restore(ctx, bad); err == nil {
				t.Fatal("forged snapshot accepted")
			}
			if err := h.Restore(ctx, ref); err != nil {
				t.Fatal(err)
			}
			if err := h.Run(ctx, ref); err != nil || calls != 1 {
				t.Fatalf("owner callback after ACK: calls=%d err=%v", calls, err)
			}
			if err := o.Close(); err != nil {
				t.Fatal(err)
			}
		}
		if err := causal.Close(); err != nil {
			t.Fatal(err)
		}
		if err := referenceE1Compare(before, referenceE3Capture(t, path, owner)); err != nil {
			t.Fatalf("cold owner restart mutated canonical SQLite transcript: %v", err)
		}
	}
}

func TestReferenceE3InTransitRecoveryRootDoesNotAuthorizeOwnerCut(t *testing.T) {
	ctx := context.Background()
	c, store, ledger, cycle, fence, path := communicationFixture(t)
	_, _, _, _, _, run := ledger.Snapshot()
	if _, err := c.CommitCommunicationSend(ctx, ledger, cycle, fence); err != nil {
		t.Fatal(err)
	}
	before := referenceE2CausalTranscript(t, path)
	if _, err := store.CreateSnapshotV1AtRevision(ctx, [32]byte(run), c.bundle, nil, 1); err == nil {
		t.Fatal("revision-pinned owner snapshot accepted undrained perception")
	}
	if err := referenceE1Compare(before, referenceE2CausalTranscript(t, path)); err != nil {
		t.Fatalf("rejected owner snapshot mutated authority: %v", err)
	}
	root, err := store.CreateSnapshotV1(ctx, [32]byte(run), c.bundle, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := sqlite.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	probe, err := reopened.VerifySnapshotV1(ctx, [32]byte(run), root.ID, c.bundle)
	if err != nil || probe.PendingTasks != 2 || probe.OwnerWorkReady {
		t.Fatalf("in-transit root confused with settled owner cut: %v", err)
	}
	drained, err := reopened.DrainPendingPerceptionV1(ctx, [32]byte(run), root.ID, c.bundle)
	if err != nil || drained.PendingTasks != 0 || drained.TailCommits != 0 || drained.OwnerWorkReady {
		t.Fatalf("historical root acquired cognitive privilege after same-revision drain: %v", err)
	}
	settled := referenceE2CausalTranscript(t, path)
	if _, err := reopened.DrainPendingPerceptionV1(ctx, [32]byte(run), root.ID, c.bundle); err != nil {
		t.Fatal(err)
	}
	if err := referenceE1Compare(settled, referenceE2CausalTranscript(t, path)); err != nil {
		t.Fatalf("repeated drain duplicated evidence or receipt: %v", err)
	}
	if _, err := reopened.CreateSnapshotV1AtRevision(ctx, [32]byte(run), c.bundle, nil, 1); err == nil {
		t.Fatal("same-revision historical recovery root silently overwritten")
	}
	if err := referenceE1Compare(settled, referenceE2CausalTranscript(t, path)); err != nil {
		t.Fatalf("collision changed historical authority: %v", err)
	}
}

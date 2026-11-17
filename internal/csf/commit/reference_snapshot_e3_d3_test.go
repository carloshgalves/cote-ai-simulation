//go:build linux

package commit

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/ownercheckpoint"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/ownercheckpoint/localdemo"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/ownercheckpoint/localowner"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

func TestReferenceE3D3PostPhysicalTailHistoricalOwnerDeniedNewCutRestored(t *testing.T) {
	ctx := context.Background()
	host, plans, cuts := referenceE2SignedPair(t)
	path, ownerPath := referenceE2Paths(t)
	proof, err := RunReferenceD3(ctx, path, ownerPath, bundlePath,
		ReferenceD2Config{Host: host, EpochPlans: plans, frozenSupervisedCuts: &cuts}, ReferenceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	run, err := referenceC4DecodeID(proof.RunID)
	if err != nil {
		t.Fatal(err)
	}
	oldSnap, err := referenceC4DecodeID(proof.d2.C4.Epistemic.SnapshotID)
	if err != nil {
		t.Fatal(err)
	}
	before := referenceE3Capture(t, path, ownerPath)
	store, err := sqlite.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	b := host.Bundle
	old, err := store.VerifySnapshotV1(ctx, run, oldSnap, b)
	if err != nil || old.TailCommits != 3 || old.OwnerWorkReady {
		t.Fatalf("historical C4 owner cut failed or acquired D3 privilege: %v", err)
	}
	count := len(before.Databases["causal"].Tables["evidence_ledger"].Rows)
	owners := make([]*localowner.Store, 2)
	scopes := make([]ownercheckpoint.Scope, 2)
	receipts := make([][32]byte, 2)
	checkpoints := make([]sqlite.SnapshotCheckpoint, 2)
	for i, actor := range []string{"a", "b"} {
		file, oldReceipt := ownerPath, proof.d2.C4.Epistemic.OwnerReceiptID
		if actor == "a" {
			file += ".actor-a"
			oldReceipt = proof.d2.C4.Epistemic.AOwnerReceiptID
		}
		oldID, err := referenceC4DecodeID(oldReceipt)
		if err != nil {
			t.Fatal(err)
		}
		o, err := localowner.Open(ctx, file, referenceC2SyntheticSigner(run, actor))
		if err != nil {
			t.Fatal(err)
		}
		owners[i] = o
		scope := ownercheckpoint.Scope{Run: run, Namespace: "school", Actor: actor}
		scopes[i] = scope
		gate, err := ownercheckpoint.NewGate(o, store, b)
		if err != nil {
			t.Fatal(err)
		}
		if err := gate.RestoreAndAcknowledge(ctx, scope, oldSnap, oldID); !errors.Is(err, ownercheckpoint.ErrNotAcknowledged) {
			t.Fatalf("historical owner restore %s was not denied: %v", actor, err)
		}
	}
	if err := referenceE1Compare(before, referenceE3Capture(t, path, ownerPath)); err != nil {
		t.Fatalf("historical restore attempt changed D3 transcript: %v", err)
	}
	for i, actor := range []string{"a", "b"} {
		// Synthetic fixture only: never reconstruct cognition from BodyState.
		payload, err := codec.CanonicalPayload([]any{"reference.e3.settled-owner", actor, uint64(9)})
		if err != nil {
			t.Fatal(err)
		}
		cp := sqlite.SnapshotCheckpoint{
			ActorNamespace: "school", ActorID: actor, StoreID: "reference-e3-owner-" + actor,
			SchemaVersion: 1, Content: payload, Producer: "reference-e3-scripted", ProducerVersion: 1,
			CoversInstant: int64(old.Clock.Instant), CoversRevision: 9, CoversEvidenceCursor: uint64(count),
		}
		gate, err := ownercheckpoint.NewGate(owners[i], store, b)
		if err != nil {
			t.Fatal(err)
		}
		receipts[i], err = gate.Stage(ctx, scopes[i], cp)
		if err != nil {
			t.Fatal(err)
		}
		checkpoints[i] = cp
	}
	snap, err := store.CreateSnapshotV1AtRevision(ctx, run, b, checkpoints, 9)
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range owners {
		if err := o.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	for restart := 0; restart < 2; restart++ {
		reopened, err := sqlite.Open(ctx, path)
		if err != nil {
			t.Fatal(err)
		}
		verified, err := reopened.VerifySnapshotV1(ctx, run, snap.ID, b)
		if err != nil || !verified.OwnerWorkReady || verified.Clock.Revision != 9 {
			t.Fatalf("post-D3 owner cut not ready: %v", err)
		}
		for i, actor := range []string{"a", "b"} {
			file := ownerPath
			if actor == "a" {
				file += ".actor-a"
			}
			o, err := localowner.Open(ctx, file, referenceC2SyntheticSigner(run, actor))
			if err != nil {
				t.Fatal(err)
			}
			gate, err := ownercheckpoint.NewGate(o, reopened, b)
			if err != nil {
				t.Fatal(err)
			}
			calls := 0
			work := func(context.Context) error { calls++; return nil }
			if err := gate.RunWork(ctx, scopes[i], snap.ID, receipts[i], work); !errors.Is(err, ownercheckpoint.ErrNotAcknowledged) {
				t.Fatal("persisted ACK alone authorized fresh gate")
			}
			if err := gate.RestoreAndAcknowledge(ctx, scopes[i], snap.ID, receipts[i]); err != nil {
				t.Fatal(err)
			}
			if err := gate.RunWork(ctx, scopes[i], snap.ID, receipts[i], work); err != nil || calls != 1 {
				t.Fatalf("post-D3 work not acknowledged: %v", err)
			}
			if err := o.Close(); err != nil {
				t.Fatal(err)
			}
		}
		if err := reopened.Close(); err != nil {
			t.Fatal(err)
		}
		current := referenceE3Capture(t, path, ownerPath)
		if restart == 0 {
			for _, table := range []string{"input_ledger", "decision_ledger", "event_store", "evidence_ledger",
				"causal_outbox", "source_ingress_receipts", "input_source_cursors", "committed_cycles"} {
				if before.Databases["causal"].Tables[table].digest() != current.Databases["causal"].Tables[table].digest() {
					t.Fatalf("sealing/ACK altered existing causal authority %s", table)
				}
			}
			before = current
		} else if err := referenceE1Compare(before, current); err != nil {
			t.Fatalf("second D3 owner restart changed byte transcript: %v", err)
		}
	}
}

func TestReferenceE3PersistedOwnerAndSnapshotCorruptionFailsClosed(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name, stmt string
		owner bool
	}{
		{"receipt-signature", "UPDATE cognitive_owner_receipts SET signature=zeroblob(64) WHERE actor_id='b'", false},
		{"snapshot-version", "", false},
		{"owner-version", "UPDATE checkpoints SET schema_version=schema_version+1 WHERE actor_id='b'", true},
		{"owner-content", "UPDATE checkpoints SET content=x'80' WHERE actor_id='b'", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, path, owner := c4Fixture(t, ReferenceOptions{})
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
			receipt, err := referenceC4DecodeID(result.Epistemic.OwnerReceiptID)
			if err != nil {
				t.Fatal(err)
			}
			dbPath := path
			if tc.owner {
				dbPath = owner
			}
			db, err := sql.Open("sqlite", dbPath)
			if err != nil {
				t.Fatal(err)
			}
			var changed sql.Result
			if tc.name == "snapshot-version" {
				var raw []byte
				if err := db.QueryRowContext(ctx, "SELECT canonical FROM snapshots WHERE id=?", snapshot[:]).Scan(&raw); err != nil {
					t.Fatal(err)
				}
				x, err := codec.StrictDecodePayload(raw)
				if err != nil {
					t.Fatal(err)
				}
				fields := x.([]any)
				fields[1] = uint64(2)
				mutant, err := codec.CanonicalPayload(fields)
				if err != nil {
					t.Fatal(err)
				}
				changed, err = db.ExecContext(ctx, "UPDATE snapshots SET canonical=? WHERE id=?", mutant, snapshot[:])
			} else {
				changed, err = db.ExecContext(ctx, tc.stmt)
			}
			if err != nil {
				t.Fatal(err)
			}
			if n, err := changed.RowsAffected(); err != nil || n != 1 {
				t.Fatalf("corruption targeted %d rows, want one: %v", n, err)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			store, err := sqlite.Open(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			o, err := localowner.Open(ctx, owner, referenceC2SyntheticSigner(run, "b"))
			if err != nil {
				t.Fatal(err)
			}
			defer o.Close()
			scope := ownercheckpoint.Scope{Run: run, Namespace: "school", Actor: "b"}
			calls := 0
			h, err := localdemo.New(o, store, b, scope, func(context.Context) error { calls++; return nil })
			if err != nil {
				t.Fatal(err)
			}
			ref := localdemo.Reference{Snapshot: snapshot, Receipt: receipt}
			if err := h.Restore(ctx, ref); err == nil {
				t.Fatal("corrupt persisted owner/snapshot acquired authority")
			}
			if err := h.Run(ctx, ref); !errors.Is(err, ownercheckpoint.ErrNotAcknowledged) || calls != 0 {
				t.Fatal("corrupt restore invoked cognitive callback")
			}
		})
	}
}

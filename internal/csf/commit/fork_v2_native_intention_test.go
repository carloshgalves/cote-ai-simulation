package commit

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/inputledger"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/trigger"
)

func r19NativeIntentionSource(t *testing.T, kind uint64) (*sqlite.Store, *policy.Bundle, domain.ID, sqlite.SnapshotV1) {
	t.Helper()
	ctx := context.Background()
	if kind == 20 || kind == 22 {
		c, parent, ledger, _, _, path := noProposalWorkFixtureWithSetup(t, func(s *sqlite.Store, run domain.ID, b *policy.Bundle) error {
			return s.RegisterCognitiveActor(ctx, [32]byte(run), "school", "a", b)
		})
		_, _, _, _, _, run := ledger.Snapshot()
		state, err := parent.LoadClock(ctx, [32]byte(run))
		if err != nil {
			t.Fatal(err)
		}
		old, err := parent.CreateSnapshotV1(ctx, [32]byte(run), c.bundle, csfrv18SyntheticActorCheckpoints(t, path, state.Instant, int64(state.Revision)))
		if err != nil {
			t.Fatal(err)
		}
		ref := []any{uint64(20), old.ID[:], old.Digest[:]}
		if kind == 22 {
			value, err := codec.StrictDecodePayload(old.Canonical)
			if err != nil {
				t.Fatal(err)
			}
			fields := value.([]any)[14].([]any)[0].([]any)
			preimage, err := identity.RecordIdentityFields("epistemic-checkpoint-ref", fields, nil)
			if err != nil {
				t.Fatal(err)
			}
			id, _, err := identity.New(c.bundle).Sum("cote.csf.id.epistemic-checkpoint", "cote.csf.schema.epistemic-checkpoint-id-preimage", 1, preimage)
			if err != nil {
				t.Fatal(err)
			}
			digest, _, err := identity.New(c.bundle).Sum("cote.csf.digest.epistemic-checkpoint", "cote.csf.schema.epistemic-checkpoint-ref", 1, fields)
			if err != nil {
				t.Fatal(err)
			}
			ref = []any{uint64(22), id[:], digest[:]}
		}
		for i := 0; i < 2; i++ {
			if _, err := c.ResumeRegisteredWorkV1(ctx, ledger, [32]byte(run), old.ID); err != nil {
				t.Fatal(err)
			}
		}
		state, err = parent.LoadClock(ctx, [32]byte(run))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ledger.CloseSource(ctx, "driver", inputledger.Closure{ProducerKey: "r19-checkpoint-final", ClosedThrough: domain.EligibilityCoordinate{NotBeforeInstant: state.Instant + 1000}, Final: true}); err != nil {
			t.Fatal(err)
		}
		root, err := parent.ValidatedGenesis(ctx, [32]byte(run), c.bundle)
		if err != nil {
			t.Fatal(err)
		}
		ledger, err = inputledger.Open(ctx, run, c.bundle, parent)
		if err != nil {
			t.Fatal(err)
		}
		checkpoints := func() []sqlite.SnapshotCheckpoint {
			return csfrv18SyntheticActorCheckpoints(t, path, state.Instant, int64(state.Revision))
		}
		return r19SourceWithActionOn(t, parent, ledger, c.bundle, run, []any{uint64(2), []any{root.ID[:], root.Digest[:]}}, true, []any{uint64(1), ref}, checkpoints)
	}
	if kind == 18 || kind == 19 {
		c, parent, ledger, cycle, fence, path := communicationFixture(t)
		_, _, _, _, _, run := ledger.Snapshot()
		if _, err := c.CommitCommunicationSend(ctx, ledger, cycle, fence); err != nil {
			t.Fatal(err)
		}
		csfrv18SettleSendOutbox(t, parent, c.bundle, path)
		state, err := parent.LoadClock(ctx, [32]byte(run))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ledger.CloseSource(ctx, "driver", inputledger.Closure{ProducerKey: "r19-native-final", ClosedThrough: domain.EligibilityCoordinate{NotBeforeInstant: state.Instant + 1000}, Final: true}); err != nil {
			t.Fatal(err)
		}
		root, err := parent.ValidatedGenesis(ctx, [32]byte(run), c.bundle)
		if err != nil {
			t.Fatal(err)
		}
		db, err := sql.Open("sqlite", path)
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		table := "communication_claims"
		if kind == 19 {
			table = "communication_transmissions"
		}
		var raw []byte
		if err := db.QueryRowContext(ctx, "SELECT canonical FROM "+table+" WHERE run_id=?", run[:]).Scan(&raw); err != nil {
			t.Fatal(err)
		}
		value, err := codec.StrictDecodePayload(raw)
		if err != nil {
			t.Fatal(err)
		}
		f := value.([]any)
		ref := []any{uint64(1), []any{kind, f[0], f[len(f)-1]}}
		// Declaring the pending round registers another eligible cognitive owner.
		// Build the checkpoint roster after that declaration, at the exact cut.
		checkpoints := func() []sqlite.SnapshotCheckpoint {
			return csfrv18SyntheticActorCheckpoints(t, path, state.Instant, int64(state.Revision))
		}
		ledger, err = inputledger.Open(ctx, run, c.bundle, parent)
		if err != nil {
			t.Fatal(err)
		}
		return r19SourceWithActionOn(t, parent, ledger, c.bundle, run, []any{uint64(2), []any{root.ID[:], root.Digest[:]}}, true, ref, checkpoints)
	}
	b, err := policy.Load(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	g := validGenesis(t, b)
	parent, err := sqlite.Open(ctx, t.TempDir()+"/native-parent.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { parent.Close() })
	if err := parent.PersistGenesis(ctx, g, b); err != nil {
		t.Fatal(err)
	}
	if err := parent.BootstrapClock(ctx, g, b); err != nil {
		t.Fatal(err)
	}
	var defs []trigger.Definition
	if kind == 7 {
		d := activeTrigger()
		d.PredicateThreshold = g.InitialInstant
		defs = []trigger.Definition{d}
	}
	if err := parent.BootstrapGenesisTriggers(ctx, g, b, defs); err != nil {
		t.Fatal(err)
	}
	ledger, err := inputledger.Open(ctx, g.RunID, b, parent)
	if err != nil {
		t.Fatal(err)
	}
	return r19SourceWithActionOn(t, parent, ledger, b, g.RunID, []any{uint64(2), []any{g.ID[:], g.Digest[:]}}, true,
		func(store *sqlite.Store, b *policy.Bundle, origin []any) any {
			if kind == 7 {
				units, err := store.SystemUnits(ctx, [32]byte(g.RunID), domain.EligibilityCoordinate{NotBeforeInstant: g.InitialInstant})
				if err != nil {
					t.Fatal(err)
				}
				for _, u := range units {
					if u.Kind == 3 {
						return []any{uint64(1), []any{uint64(7), u.ID[:], u.Digest[:]}}
					}
				}
				t.Fatal("initial activation not present")
			}
			id, _, err := identity.New(b).Sum("cote.csf.id.occurrence", "cote.csf.schema.occurrence-id-preimage", 1, []any{origin, uint64(3), uint64(0)})
			if err != nil {
				t.Fatal(err)
			}
			at := g.InitialInstant + 50
			fields := []any{id[:], g.RunID[:], "schedule", int64(at), []any{int64(at), uint64(0)}, "test.occurrence", uint64(1), g.Payload[16], uint64(0), origin, uint64(3), uint64(0), []any{uint64(0)}, []any{uint64(0)}}
			digest, _, err := identity.New(b).Sum("cote.csf.digest.occurrence-unit", "cote.csf.schema.scheduled-occurrence.body", 1, fields)
			if err != nil {
				t.Fatal(err)
			}
			raw, err := codec.CanonicalPayload(append(fields, digest[:]))
			if err != nil {
				t.Fatal(err)
			}
			if err := store.SeedGenesisOccurrence(ctx, domain.ID(id), g.RunID, domain.EligibilityCoordinate{NotBeforeInstant: at}, raw); err != nil {
				t.Fatal(err)
			}
			return []any{uint64(1), []any{uint64(6), id[:], digest[:]}}
		})
}

func TestCSFRV18R19NativeIntentionRootSurvivesRollbackReopenAndAudit(t *testing.T) {
	for _, kind := range []uint64{6, 7, 18, 19, 20, 22} {
		t.Run(fmt.Sprint(kind), func(t *testing.T) {
			ctx := context.Background()
			parent, v1, run, snap := r19NativeIntentionSource(t, kind)
			v2, err := policy.LoadV2("../../../docs/architecture/canonical-codec-v2-bundle")
			if err != nil {
				t.Fatal(err)
			}
			path := t.TempDir() + "/native-child.db"
			child, err := sqlite.Open(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { child.Close() })
			childID := [32]byte(run)
			childID[0] ^= 0x65
			db, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if _, err := db.ExecContext(ctx, "CREATE TRIGGER fail_native_proof BEFORE INSERT ON fork_history_roots BEGIN SELECT RAISE(ABORT,'native proof fault'); END"); err != nil {
				t.Fatal(err)
			}
			if _, err := parent.ForkIdleSnapshotV1ToV2(ctx, [32]byte(run), snap.ID, childID, child, v1, v2); err == nil {
				t.Fatal("native proof fault was ignored")
			}
			assertCSFRV18EmptyRejectedFork(t, path)
			if _, err := db.ExecContext(ctx, "DROP TRIGGER fail_native_proof"); err != nil {
				t.Fatal(err)
			}
			if _, err := parent.ForkIdleSnapshotV1ToV2(ctx, [32]byte(run), snap.ID, childID, child, v1, v2); err != nil {
				t.Fatal("native intention migration:", err)
			}
			if err := child.Close(); err != nil {
				t.Fatal(err)
			}
			child, err = sqlite.Open(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := child.ReplayClock(ctx, childID, v2); err != nil {
				t.Fatal(err)
			}
			ledger, err := inputledger.Open(ctx, domain.ID(childID), v2, child)
			if err != nil {
				t.Fatal(err)
			}
			_, _, _, rounds, slots, _ := ledger.Snapshot()
			ref := slots[rounds[0].ID]["answered"].Body[16].([]any)[1].([]any)
			if ref[0] != kind {
				t.Fatal("historical intention type changed")
			}
			if kind == 18 || kind == 19 {
				db, err := sql.Open("sqlite", path)
				if err != nil {
					t.Fatal(err)
				}
				table := "communication_claims"
				if kind == 19 {
					table = "communication_transmissions"
				}
				var inherited int
				if err := db.QueryRowContext(ctx, "SELECT count(*) FROM "+table+" WHERE run_id=? AND id=?", childID[:], ref[1]).Scan(&inherited); err != nil {
					t.Fatal(err)
				}
				if inherited != 0 {
					t.Fatal("historical communication ID gained child live authority")
				}
				for _, table := range []string{"event_store", "evidence_ledger", "causal_outbox"} {
					var n int
					if err := db.QueryRowContext(ctx, "SELECT count(*) FROM "+table).Scan(&n); err != nil {
						t.Fatal(err)
					}
					if n != 0 {
						t.Fatal("historical communication was republished in the child", table)
					}
				}
				if err := db.Close(); err != nil {
					t.Fatal(err)
				}
			}
			pending, err := child.PendingClock(ctx, childID)
			if err != nil {
				t.Fatal(err)
			}
			for _, unit := range pending {
				if string(unit.ID[:]) == string(ref[1].([]byte)) {
					t.Fatal("historical intention root became child scheduling authority")
				}
			}
			state, err := child.LoadClock(ctx, childID)
			if err != nil {
				t.Fatal(err)
			}
			checkpoints := csfrv18SyntheticActorCheckpoints(t, path, state.Instant, int64(state.Revision))
			checkpoint, err := child.CreateSnapshotV1(ctx, childID, v2, checkpoints)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := child.VerifySnapshotV1(ctx, childID, checkpoint.ID, v2); err != nil {
				t.Fatal(err)
			}
			var roots int
			if err := db.QueryRowContext(ctx, "SELECT count(*) FROM fork_history_roots WHERE child_run=?", childID[:]).Scan(&roots); err != nil {
				t.Fatal(err)
			}
			wantRoots := 1
			if kind == 18 || kind == 19 {
				wantRoots = 2
			}
			if roots != wantRoots {
				t.Fatal("native intention not archived exactly once", roots)
			}
			var original []byte
			if err := db.QueryRowContext(ctx, "SELECT canonical FROM fork_history_roots WHERE child_run=? AND reference_kind=?", childID[:], kind).Scan(&original); err != nil {
				t.Fatal(err)
			}
			value, err := codec.StrictDecodePayload(original)
			if err != nil {
				t.Fatal(err)
			}
			f := value.([]any)
			digestIndex := len(f) - 1
			if kind == 22 {
				digestIndex = 4
			}
			f[digestIndex].([]byte)[0] ^= 1
			altered, err := codec.CanonicalPayload(f)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.ExecContext(ctx, "UPDATE fork_history_roots SET canonical=? WHERE child_run=? AND reference_kind=?", altered, childID[:], kind); err != nil {
				t.Fatal(err)
			}
			if _, err := child.ReplayClock(ctx, childID, v2); err == nil {
				t.Fatal("corrupt native embedded digest accepted")
			}
			if _, err := db.ExecContext(ctx, "UPDATE fork_history_roots SET canonical=? WHERE child_run=? AND reference_kind=?", original, childID[:], kind); err != nil {
				t.Fatal(err)
			}
			if _, err := child.ReplayClock(ctx, childID, v2); err != nil {
				t.Fatal("exact proof restoration failed:", err)
			}
			wrongKind := uint64(6)
			if kind == 6 {
				wrongKind = 7
			}
			if _, err := db.ExecContext(ctx, "UPDATE fork_history_roots SET reference_kind=? WHERE child_run=? AND reference_kind=?", wrongKind, childID[:], kind); err != nil {
				t.Fatal(err)
			}
			if _, err := child.ReplayClock(ctx, childID, v2); err == nil {
				t.Fatal("native proof retyped to a different authority")
			}
			if _, err := db.ExecContext(ctx, "UPDATE fork_history_roots SET reference_kind=? WHERE child_run=? AND reference_kind=?", kind, childID[:], wrongKind); err != nil {
				t.Fatal(err)
			}
			if _, err := child.ReplayClock(ctx, childID, v2); err != nil {
				t.Fatal(err)
			}
			if kind == 18 || kind == 19 {
				companion := uint64(18)
				if kind == 18 {
					companion = 19
				}
				var companionID, companionRaw []byte
				if err := db.QueryRowContext(ctx, "SELECT record_id,canonical FROM fork_history_roots WHERE child_run=? AND reference_kind=?", childID[:], companion).Scan(&companionID, &companionRaw); err != nil {
					t.Fatal(err)
				}
				value, err := codec.StrictDecodePayload(companionRaw)
				if err != nil {
					t.Fatal(err)
				}
				fields := value.([]any)
				fields[len(fields)-1].([]byte)[0] ^= 1
				corrupt, err := codec.CanonicalPayload(fields)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := db.ExecContext(ctx, "UPDATE fork_history_roots SET canonical=? WHERE child_run=? AND reference_kind=?", corrupt, childID[:], companion); err != nil {
					t.Fatal(err)
				}
				if _, err := child.ReplayClock(ctx, childID, v2); err == nil {
					t.Fatal("native communication graph accepted a corrupt companion root")
				}
				if _, err := db.ExecContext(ctx, "UPDATE fork_history_roots SET canonical=? WHERE child_run=? AND reference_kind=?", companionRaw, childID[:], companion); err != nil {
					t.Fatal(err)
				}
				if _, err := db.ExecContext(ctx, "DELETE FROM fork_history_roots WHERE child_run=? AND reference_kind=?", childID[:], companion); err != nil {
					t.Fatal(err)
				}
				if _, err := child.ReplayClock(ctx, childID, v2); err == nil {
					t.Fatal("native communication graph accepted a missing companion root")
				}
				if _, err := db.ExecContext(ctx, "INSERT INTO fork_history_roots(child_run,reference_kind,record_id,canonical) VALUES(?,?,?,?)", childID[:], companion, companionID, companionRaw); err != nil {
					t.Fatal(err)
				}
				if _, err := child.ReplayClock(ctx, childID, v2); err != nil {
					t.Fatal("communication companion restoration failed:", err)
				}
			}
			if _, err := db.ExecContext(ctx, "DELETE FROM fork_history_roots WHERE child_run=?", childID[:]); err != nil {
				t.Fatal(err)
			}
			if _, err := child.ReplayClock(ctx, childID, v2); err == nil {
				t.Fatal("missing native causal root accepted")
			}
		})
	}
}

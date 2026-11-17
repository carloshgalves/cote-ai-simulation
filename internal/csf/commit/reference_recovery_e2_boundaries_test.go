//go:build linux

package commit

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/inputledger"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
)

// The same signed/Genesis-pinned C4+D2 entrypoint must stop before the
// Python producer when C3 or communication atomicity fails. These are
// prefix/rollback controls; only the physical epoch cases in E2 have an
// end-to-end completed recovery to nine commits.
func TestReferenceE2PrePhysicalAtomicFaultsReopenWithoutPartialHistory(t *testing.T) {
	ctx := context.Background()
	host, plans := referenceD2TestHost(t)
	g, err := buildReferenceGenesis(host.Bundle)
	if err != nil {
		t.Fatal(err)
	}
	type candidate struct {
		name, contains string
		opts           ReferenceOptions
		revision       int
		control        uint64
	}
	cases := []candidate{}
	for step := uint8(1); step <= 2; step++ {
		cases = append(cases,
			candidate{fmt.Sprintf("abort-step%d", step), "C3 atomic terminal abort", ReferenceOptions{FailAbortAt: step}, 0, 1},
			candidate{fmt.Sprintf("retry-step%d", step), "C3 operator-authorized", ReferenceOptions{FailRetryAt: step}, 0, 3},
		)
	}
	for step := uint8(1); step <= 7; step++ {
		cases = append(cases,
			candidate{fmt.Sprintf("contest-step%d", step), "contested WORK", ReferenceOptions{FailContestAt: step}, 0, 1},
			candidate{fmt.Sprintf("communication-send-step%d", step), "C1 communication send", ReferenceOptions{FailCommunicationSendAt: step}, 3, 1},
			candidate{fmt.Sprintf("communication-due-step%d", step), "C1 due commit", ReferenceOptions{FailCommunicationDeliveryAt: step}, 5, 1},
		)
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			directory := filepath.Join(root, "owners")
			if err := os.Mkdir(directory, 0700); err != nil {
				t.Fatal(err)
			}
			db := filepath.Join(root, "causal.db")
			owner := filepath.Join(directory, "owner-b.db")
			_, err := RunReferenceD2(ctx, db, owner, bundlePath,
				ReferenceD2Config{Host: host, EpochPlans: plans}, tc.opts)
			if err == nil || !strings.Contains(err.Error(), tc.contains) {
				t.Fatalf("wrong fault stage or missing transactional error: %v", err)
			}
			before, err := referenceE1Read(ctx, []referenceE1Location{{Name: "causal", Path: db}})
			if err != nil {
				t.Fatal(err)
			}
			reopened, err := sqlite.Open(ctx, db)
			if err != nil {
				t.Fatal(err)
			}
			clock, err := reopened.LoadClock(ctx, [32]byte(g.RunID))
			if err != nil || uint64(clock.Revision) != uint64(tc.revision) {
				reopened.Close()
				t.Fatalf("failed atomic stage changed causal clock (want %d): %+v / %v", tc.revision, clock, err)
			}
			for _, table := range []string{"committed_cycles", "world_revisions"} {
				ro, err := openReferenceReadOnlyDB(db)
				if err != nil {
					reopened.Close()
					t.Fatal(err)
				}
				var count int
				err = ro.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count)
				closeErr := ro.Close()
				if err != nil || closeErr != nil {
					reopened.Close()
					t.Fatal(err, closeErr)
				}
				expected := tc.revision
				if table == "world_revisions" {
					expected++
				}
				if count != expected {
					reopened.Close()
					t.Fatalf("%s had partial rows after rollback: got %d expected %d", table, count, expected)
				}
			}
			control, err := reopened.LoadCycleControl(ctx, [32]byte(g.RunID))
			if err != nil || len(control) != 7 || control[0] != tc.control {
				reopened.Close()
				t.Fatalf("failed phase changed durable control, want %d: %v %v", tc.control, control, err)
			}
			replay, err := reopened.ReplayClock(ctx, [32]byte(g.RunID), host.Bundle)
			if err != nil || replay.ReplayedCommits != tc.revision || replay.ReconstructedClock != clock {
				reopened.Close()
				t.Fatalf("incomplete crash prefix fails verified historical replay: %+v %v", replay, err)
			}
			ledger, err := inputledger.Open(ctx, g.RunID, host.Bundle, reopened)
			if err != nil {
				reopened.Close()
				t.Fatalf("reopening exogenous ingress failed: %v", err)
			}
			if !ledger.SharesFenceAuthority(reopened) {
				reopened.Close()
				t.Fatal("reopen abandoned source/fence CAS authority")
			}
			if err := reopened.Close(); err != nil {
				t.Fatal(err)
			}
			after, err := referenceE1Read(ctx, []referenceE1Location{{Name: "causal", Path: db}})
			if err != nil {
				t.Fatal(err)
			}
			if err := referenceE1Compare(before, after); err != nil {
				t.Fatalf("read-only reopen/replay changed incomplete but valid prefix: %v", err)
			}
		})
	}
}

// The ninth fully committed cycle is not permission to deliver a second
// B-only KnowledgeInput or duplicate a task on process restart. This checks
// all three databases, not only a COUNT of public events.
func TestReferenceE2CommittedD3PerceptionReopenAndAckEvidenceIsImmutable(t *testing.T) {
	ctx := context.Background()
	host, plans, cuts := referenceE2SignedPair(t)
	db, owner := referenceE2Paths(t)
	full, err := RunReferenceD3(ctx, db, owner, bundlePath,
		ReferenceD2Config{Host: host, EpochPlans: plans, frozenSupervisedCuts: &cuts}, ReferenceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	pre := referenceE2Capture(t, db, owner)
	store, err := sqlite.Open(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if full.d2.C4 == nil || full.d2.C4.Communication == nil || full.d2.C4.Epistemic == nil {
		store.Close()
		t.Fatal("C4 owner/communication evidence missing")
	}
	task, err := referenceC4DecodeID(full.d2.C4.Communication.DeliveryTaskID)
	if err != nil {
		store.Close()
		t.Fatal(err)
	}
	if _, err := store.DeliverCommunicationPerception(ctx, task); err != nil {
		store.Close()
		t.Fatalf("second read/delivery cannot reconcile idempotently: %v", err)
	}
	receipt, err := referenceC4DecodeID(full.d2.C4.Epistemic.SnapshotID)
	if err != nil {
		store.Close()
		t.Fatal(err)
	}
	validated, err := store.VerifySnapshotV1(ctx, [32]byte(referenceE2RunID(t, full.RunID)), receipt, host.Bundle)
	if err != nil || validated.TailCommits != 3 {
		store.Close()
		t.Fatalf("after-reopen checkpoint acknowledgement lost: %v", err)
	}
	if err := store.CheckPerceptionBarrier(ctx, [32]byte(referenceE2RunID(t, full.RunID)), 6); err != nil {
		store.Close()
		t.Fatal(err)
	}
	replay, err := store.ReplayClock(ctx, [32]byte(referenceE2RunID(t, full.RunID)), host.Bundle)
	if err != nil || replay.ReplayedCommits != 9 {
		store.Close()
		t.Fatalf("post-ACK replay lacks nine commits: %+v %v", replay, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	post := referenceE2Capture(t, db, owner)
	if err := referenceE1Compare(pre, post); err != nil {
		t.Fatalf("re-delivery/replay modified causal or private A/B owner stores: %v", err)
	}
}
func referenceE2RunID(t *testing.T, text string) [32]byte {
	t.Helper()
	run, err := referenceC4DecodeID(text)
	if err != nil {
		t.Fatal(err)
	}
	return run
}

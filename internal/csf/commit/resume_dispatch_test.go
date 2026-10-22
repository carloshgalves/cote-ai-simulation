package commit

import (
 "context"
 "testing"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/inputledger"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
)

func TestCSFRV18RegisteredResumeDispatchesAllVerifiedV1Cohorts(t *testing.T) {
 ctx := context.Background()
 t.Run("NoProposal", func(t *testing.T) {
  c, store, ledger, _, _, _ := noProposalWorkFixture(t)
  _,_,_,_,_,run := ledger.Snapshot()
  snapshot, err := store.CreateSnapshotV1(ctx, [32]byte(run), c.bundle, nil)
  if err != nil { t.Fatal(err) }
  result, err := c.ResumeRegisteredWorkV1(ctx, ledger, [32]byte(run), snapshot.ID)
  if err != nil { t.Fatal(err) }
  if !result.Continued { t.Fatal("NoProposal did not commit from saved V1 fence") }
  again, err := c.ResumeRegisteredWorkV1(ctx, ledger, [32]byte(run), snapshot.ID)
  if err != nil || again.Continued {
   t.Fatalf("NoProposal recovery repeated terminal: %+v, %v", again, err)
  }
 })
 t.Run("mixed resource contest and system", func(t *testing.T) {
  source, store, ledger, _, _, path := twoResourceContestFixtureWithSystem(t, false, true, true)
  _,_,_,_,_,run := ledger.Snapshot()
  snap, err := store.CreateSnapshotV1(ctx, [32]byte(run), source.bundle, nil)
  if err != nil { t.Fatal(err) }
  opened, err := sqlite.Open(ctx, path)
  if err != nil { t.Fatal(err) }
  defer opened.Close()
  reloaded, err := inputledger.Open(ctx, run, source.bundle, opened)
  if err != nil { t.Fatal(err) }
  recovered := New(opened, source.bundle, source.admissionPolicies)
  result, err := recovered.ResumeRegisteredWorkV1(ctx, reloaded, [32]byte(run), snap.ID)
  if err != nil { t.Fatal(err) }
  if !result.Continued { t.Fatal("mixed contest did not enter its registered atomic handler") }
  if _, err := opened.ReplayClock(ctx, [32]byte(run), source.bundle); err != nil {
   t.Fatal("mixed recovered terminal was not causally replayable:", err)
  }
 })
 t.Run("communication send", func(t *testing.T) {
  c, store, ledger, _, _, path := communicationFixture(t)
  _,_,_,_,_,run := ledger.Snapshot()
  snap, err := store.CreateSnapshotV1(ctx, [32]byte(run), c.bundle, nil)
  if err != nil { t.Fatal(err) }
  reopened, err := sqlite.Open(ctx, path)
  if err != nil { t.Fatal(err) }
  defer reopened.Close()
  restored, err := inputledger.Open(ctx, run, c.bundle, reopened)
  if err != nil { t.Fatal(err) }
  resumer := New(reopened, c.bundle, c.admissionPolicies)
  result, err := resumer.ResumeRegisteredWorkV1(ctx, restored, [32]byte(run), snap.ID)
  if err != nil { t.Fatal(err) }
  if !result.Continued { t.Fatal("registered communication send was not committed") }
  if _, err := reopened.ReplayClock(ctx, [32]byte(run), c.bundle); err != nil {
   t.Fatal("recovered transmission is not causally replayable:", err)
  }
 })
}

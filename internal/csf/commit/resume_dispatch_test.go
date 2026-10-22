package commit

import (
 "context"
 "testing"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/admission"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/clock"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
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

func csfrv18SealNextSystemWork(t *testing.T,c *Coordinator,store *sqlite.Store,ledger *inputledger.Ledger,run domain.ID){
 t.Helper()
 ctx:=context.Background()
 state,err:=store.LoadClock(ctx,[32]byte(run))
 if err!=nil{t.Fatal(err)}
 pending,err:=store.PendingClock(ctx,[32]byte(run))
 if err!=nil{t.Fatal(err)}
 next,ok,err:=clock.Next(state,pending)
 if err!=nil||!ok||next.Kind!=clock.Work{t.Fatal("expected due WORK",err)}
 cid,_,err:=identity.New(c.bundle).Sum("cote.csf.id.cycle","cote.csf.schema.cycle-id-preimage",1,
  []any{run[:],[]any{int64(state.Instant),uint64(next.OriginOrdinal)}})
 if err!=nil{t.Fatal(err)}
 _,hash,err:=store.LoadWorldState(ctx,uint64(state.Revision))
 if err!=nil{t.Fatal(err)}
 cycle:=admission.Cycle{ID:domain.ID(cid),Attempt:0,Instant:state.Instant,
  Ordinal:next.OriginOrdinal,BaseRevision:state.Revision,
  BaseStateHash:domain.Digest(hash),RetryRef:[]any{uint64(0)}}
 if _,err:=admission.NewCoordinator(admission.NewFenceLog(store)).
  AdmitThenEvaluate(ctx,c.bundle,ledger,cycle,c.admissionPolicies,nil);err!=nil{t.Fatal(err)}
 snapshot,err:=store.CreateSnapshotV1(ctx,[32]byte(run),c.bundle,nil)
 if err!=nil{t.Fatal(err)}
 result,err:=c.ResumeRegisteredWorkV1(ctx,ledger,[32]byte(run),snapshot.ID)
 if err!=nil{t.Fatal("registered due system resume:",err)}
 if !result.Continued{t.Fatal("registered handler did not publish due WORK")}
 if _,err:=store.ReplayClock(ctx,[32]byte(run),c.bundle);err!=nil{t.Fatal(err)}
}

func TestCSFRV18RegisteredResumeRoutesDueSystemAndTransmission(t *testing.T){
 t.Run("single scheduled system unit",func(t *testing.T){
  ctx:=context.Background()
  c,store,ledger,run,_,_:=testFixture(t)
  if _,err:=c.Advance(ctx,ledger,run);err!=nil{t.Fatal(err)}
  csfrv18SealNextSystemWork(t,c,store,ledger,run)
 })
 t.Run("communication due",func(t *testing.T){
  ctx:=context.Background()
  c,store,ledger,cycle,fence,_:=communicationFixture(t)
  _,_,_,_,_,run:=ledger.Snapshot()
  if _,err:=c.CommitCommunicationSend(ctx,ledger,cycle,fence);err!=nil{t.Fatal(err)}
  if _,err:=c.Advance(ctx,ledger,run);err!=nil{t.Fatal(err)}
  csfrv18SealNextSystemWork(t,c,store,ledger,run)
 })
}

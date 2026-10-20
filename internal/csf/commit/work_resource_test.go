package commit

import (
 "bytes"
 "context"
 "database/sql"
 "testing"

 _ "modernc.org/sqlite"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/admission"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/clock"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/extensions/testresourcev1"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/inputledger"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/reducer"
)
func singleResourceFixture(t *testing.T)(*Coordinator,*sqlite.Store,*inputledger.Ledger,admission.Cycle,admission.Fence,string){
 t.Helper();ctx:=context.Background()
 bundle,err:=policy.Load(bundlePath);if err!=nil{t.Fatal(err)}
 g:=stockGenesis(t,bundle)
 path:=t.TempDir()+"/single-stock.db"
 store,err:=sqlite.Open(ctx,path);if err!=nil{t.Fatal(err)};t.Cleanup(func(){store.Close()})
 if err:=store.PersistGenesis(ctx,g,bundle);err!=nil{t.Fatal(err)}
 if err:=store.BootstrapClock(ctx,g,bundle);err!=nil{t.Fatal(err)}
 if err:=store.BootstrapGenesisTriggers(ctx,g,bundle,nil);err!=nil{t.Fatal(err)}
 ledger,err:=inputledger.New(ctx,g.RunID,bundle,store);if err!=nil{t.Fatal(err)}
 if err:=ledger.DeclareSource(inputledger.Source{ID:"driver"});err!=nil{t.Fatal(err)}
 at:=domain.EligibilityCoordinate{NotBeforeInstant:g.InitialInstant}
 if _,err:=ledger.CloseSource(ctx,"driver",inputledger.Closure{ProducerKey:"source.closed",ClosedThrough:at});err!=nil{t.Fatal(err)}
 d:=identity.New(bundle)
 cycleID,_,err:=d.Sum("cote.csf.id.cycle","cote.csf.schema.cycle-id-preimage",1,
  []any{g.RunID[:],[]any{int64(g.InitialInstant),uint64(0)}});if err!=nil{t.Fatal(err)}
 created:=[]any{uint64(2),[]any{g.ID[:],g.Digest[:]}}
 round,err:=ledger.DeclareRound(ctx,"schedule",[2]string{"school","a"},[]string{"act"},at,created,0,0,0,[]any{uint64(0)})
 if err!=nil{t.Fatal(err)}
 dispatchID,_,err:=d.Sum("cote.csf.id.dispatch","cote.csf.schema.dispatch-id-preimage",1,
  []any{g.RunID[:],cycleID[:],round.ID[:],"act",uint64(0)});if err!=nil{t.Fatal(err)}
 dispatch:=inputledger.Dispatch{ID:domain.ID(dispatchID),RunID:g.RunID,CycleID:domain.ID(cycleID),RoundID:round.ID,
  SlotID:"act",Actor:round.Actor,Eligibility:round.Eligibility,BaseRevision:0,EffectiveAt:g.InitialInstant}
 if err:=ledger.RegisterDispatch(ctx,dispatch,inputledger.CycleBinding{
  ID:domain.ID(cycleID),BaseRevision:0,Instant:g.InitialInstant});err!=nil{t.Fatal(err)}
 response,err:=testresourcev1.ProposalForDispatch(bundle,dispatch);if err!=nil{t.Fatal(err)}
 if err:=ledger.Respond(ctx,response);err!=nil{t.Fatal(err)}
 var base domain.Digest;copy(base[:],g.Payload[16].([]any)[3].([]byte))
 cycle:=admission.Cycle{ID:domain.ID(cycleID),Attempt:0,Instant:g.InitialInstant,Ordinal:0,
  BaseRevision:0,BaseStateHash:base,RetryRef:[]any{uint64(0)}}
 refs,err:=policiesByRole(bundle);if err!=nil{t.Fatal(err)}
 policies:=admission.Policies{AdmissionOrder:refs["admission-order"],
  CausalIdentity:refs["causal-identity"],Fence:refs["admission-fence"],Rules:[][]any{}}
 coordinator:=New(store,bundle,policies)
 fence,err:=admission.NewCoordinator(admission.NewFenceLog(store)).AdmitThenEvaluate(ctx,bundle,ledger,cycle,policies,nil)
 if err!=nil{t.Fatal(err)}
 return coordinator,store,ledger,cycle,fence,path
}
func TestSingleResourceCommitAtomicAndReplayable(t *testing.T){
 ctx:=context.Background()
 c,store,ledger,cycle,fence,path:=singleResourceFixture(t)
 _,_,_,_,_,run:=ledger.Snapshot()
 before,err:=store.LoadClock(ctx,[32]byte(run));if err!=nil{t.Fatal(err)}
 for step:=uint8(1);step<=7;step++{
  c.failAfterStep=step
  if _,err:=c.CommitSingleResource(ctx,ledger,cycle,fence);err==nil{t.Fatalf("resource crash point %d accepted",step)}
  clockAfter,err:=store.LoadClock(ctx,[32]byte(run))
  if err!=nil||clockAfter!=before{t.Fatalf("resource crash changed clock: %v %+v",err,clockAfter)}
  stock,_,err:=store.LoadWorldState(ctx,0)
  if err!=nil{t.Fatal(err)}
  current,err:=reducer.DecodeStock(stock);if err!=nil||current.Available!=1{t.Fatal("crash consumed authoritative stock",err)}
 }
 c.failAfterStep=0
 commit,err:=c.CommitSingleResource(ctx,ledger,cycle,fence)
 if err!=nil{t.Fatal(err)}
 stored,err:=store.GetFence(ctx,commit.Identity)
 if err!=nil||!bytes.Equal(stored,commit.Canonical){t.Fatal("resource CycleCommit missing")}
 successor,err:=store.LoadClock(ctx,[32]byte(run))
 if err!=nil||successor.Revision!=1||successor.LastOrdinal!=0||successor.NextLogicalSequence!=2{
  t.Fatalf("resource state clock invalid: %+v %v",successor,err)
 }
 world,_,err:=store.LoadWorldState(ctx,1)
 if err!=nil{t.Fatal(err)}
 stock,err:=reducer.DecodeStock(world);if err!=nil||stock.Available!=0{t.Fatal("successful cycle failed to consume stock",err)}
 db,err:=sql.Open("sqlite","file:"+path);if err!=nil{t.Fatal(err)};defer db.Close()
 for _,item:=range []struct{table string;want int}{{"event_store",2},{"decision_ledger",4},{"world_revisions",2},{"committed_cycles",1}}{
  var got int
  if err:=db.QueryRowContext(ctx,"SELECT count(*) FROM "+item.table).Scan(&got);err!=nil{t.Fatal(err)}
  if got!=item.want{t.Fatalf("resource terminal %s contains %d, want %d",item.table,got,item.want)}
 }
 reopened,err:=sqlite.Open(ctx,path);if err!=nil{t.Fatal(err)};defer reopened.Close()
 report,err:=reopened.ReplayClock(ctx,[32]byte(run),c.bundle)
 if err!=nil||report.ReplayedCommits!=1||report.ReplayedEvents!=2||
  report.ReconstructedClock!=successor{
  t.Fatalf("resource terminal replay diverged: %+v %v",report,err)
 }
 nextPending,err:=reopened.PendingClock(ctx,[32]byte(run))
 if err!=nil{t.Fatal(err)}
 if _,available,err:=clock.Next(successor,nextPending);err!=nil||available{
  t.Fatalf("committed resource round became pending again: %v %v",available,err)
 }
}

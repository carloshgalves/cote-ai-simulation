package commit

import (
 "bytes"
 "context"
 "database/sql"
 "testing"

 _ "modernc.org/sqlite"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/admission"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/clock"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/inputledger"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
)
func noProposalWorkFixture(t *testing.T)(*Coordinator,*sqlite.Store,*inputledger.Ledger,
 admission.Cycle,admission.Fence,string){
 t.Helper()
 ctx:=context.Background()
 c,store,ledger,run,bundle,path:=freshResourceWorkFixture(t)
 state,err:=store.LoadClock(ctx,[32]byte(run));if err!=nil{t.Fatal(err)}
 g:=validGenesis(t,bundle)
 d:=identity.New(bundle)
 cycleID,_,err:=d.Sum("cote.csf.id.cycle","cote.csf.schema.cycle-id-preimage",1,
  []any{run[:],[]any{int64(state.Instant),uint64(0)}});if err!=nil{t.Fatal(err)}
 created:=[]any{uint64(2),[]any{g.ID[:],g.Digest[:]}}
 for i,actor:=range []string{"a","b"}{
  round,err:=ledger.DeclareRound(ctx,"schedule",[2]string{"school",actor},[]string{"act"},
   domain.EligibilityCoordinate{NotBeforeInstant:state.Instant},created,0,uint32(i),0,[]any{uint64(0)})
  if err!=nil{t.Fatal(err)}
  dispatchID,_,err:=d.Sum("cote.csf.id.dispatch","cote.csf.schema.dispatch-id-preimage",1,
   []any{run[:],cycleID[:],round.ID[:],"act",uint64(0)});if err!=nil{t.Fatal(err)}
  dispatch:=inputledger.Dispatch{ID:domain.ID(dispatchID),RunID:run,CycleID:domain.ID(cycleID),RoundID:round.ID,
   SlotID:"act",Actor:round.Actor,Eligibility:round.Eligibility,BaseRevision:state.Revision,EffectiveAt:state.Instant}
  if err:=ledger.RegisterDispatch(ctx,dispatch,inputledger.CycleBinding{
   ID:domain.ID(cycleID),BaseRevision:state.Revision,Instant:state.Instant});err!=nil{t.Fatal(err)}
  id,_,err:=d.Sum("cote.csf.id.no-proposal","cote.csf.schema.slot-response-id-preimage",1,
   []any{dispatch.ID[:],uint64(1)});if err!=nil{t.Fatal(err)}
  body:=[]any{id[:],dispatch.ID[:],run[:],cycleID[:],round.ID[:],dispatch.SlotID,
   []any{dispatch.Actor[0],dispatch.Actor[1]},int64(dispatch.EffectiveAt),uint64(dispatch.BaseRevision),uint64(0)}
  digest,_,err:=d.Sum("cote.csf.digest.no-proposal-unit","cote.csf.schema.no-proposal.body",1,[]any{body})
  if err!=nil{t.Fatal(err)}
  canonical,err:=codec.CanonicalPayload(append(cloneAny(body),digest[:]));if err!=nil{t.Fatal(err)}
  response:=inputledger.Response{Kind:1,ID:domain.ID(id),Digest:digest,Canonical:canonical,Body:body,
   DispatchID:dispatch.ID,RunID:run,CycleID:domain.ID(cycleID),RoundID:round.ID,SlotID:dispatch.SlotID,
   Actor:dispatch.Actor,EffectiveAt:dispatch.EffectiveAt,SubmittedAgainstRevision:dispatch.BaseRevision}
  if err:=ledger.Respond(ctx,response);err!=nil{t.Fatal(err)}
 }
 var base domain.Digest;copy(base[:],g.Payload[16].([]any)[3].([]byte))
 cycle:=admission.Cycle{ID:domain.ID(cycleID),Attempt:0,Instant:state.Instant,Ordinal:0,
  BaseRevision:state.Revision,BaseStateHash:base,RetryRef:[]any{uint64(0)}}
 fence,err:=admission.NewCoordinator(admission.NewFenceLog(store)).AdmitThenEvaluate(
  ctx,bundle,ledger,cycle,c.admissionPolicies,nil);if err!=nil{t.Fatal(err)}
 return c,store,ledger,cycle,fence,path
}
func TestNoProposalWorkAtomicityAcrossAllFailurePoints(t *testing.T){
 ctx:=context.Background()
 c,store,ledger,cycle,fence,path:=noProposalWorkFixture(t)
 before,err:=store.LoadClock(ctx,[32]byte(func()domain.ID{_,_,_,_,_,run:=ledger.Snapshot();return run}()));if err!=nil{t.Fatal(err)}
 for stage:=uint8(1);stage<=6;stage++{
  c.failAfterStep=stage
  if _,err:=c.CommitNoProposal(ctx,ledger,cycle,fence);err==nil{t.Fatalf("WORK failpoint %d skipped",stage)}
  _,_,_,_,_,run:=ledger.Snapshot()
  current,err:=store.LoadClock(ctx,[32]byte(run))
  if err!=nil||current!=before{t.Fatalf("WORK crash changed clock at %d: %v %v",stage,current,err)}
  if state,err:=store.LoadCycleControl(ctx,[32]byte(run));err!=nil||state[0]!=uint64(1){
   t.Fatalf("WORK crash lost in-flight fence at %d: %v %v",stage,state,err)
  }
  sqlDB,err:=sql.Open("sqlite","file:"+path);if err!=nil{t.Fatal(err)}
  var decisions,world,events int
  for _,test:=range []struct{table string;dst *int}{{"decision_ledger",&decisions},{"world_revisions",&world},{"event_store",&events}}{
   if err:=sqlDB.QueryRowContext(ctx,"SELECT count(*) FROM "+test.table).Scan(test.dst);err!=nil{t.Fatal(err)}
  }
  sqlDB.Close()
  if decisions!=1||world!=1||events!=0{t.Fatalf("half-WORK survived at step %d: decisions=%d world=%d events=%d",stage,decisions,world,events)}
 }
 c.failAfterStep=0
 commit,err:=c.CommitNoProposal(ctx,ledger,cycle,fence)
 if err!=nil{t.Fatal(err)}
 _,_,_,_,_,run:=ledger.Snapshot()
 after,err:=store.LoadClock(ctx,[32]byte(run))
 if err!=nil||after.Revision!=before.Revision+1||after.LastOrdinal!=0||
  after.NextLogicalSequence!=before.NextLogicalSequence{t.Fatalf("wrong WORK terminal clock: %+v %v",after,err)}
 persisted,err:=store.GetFence(ctx,commit.Identity)
 if err!=nil||!bytes.Equal(persisted,commit.Canonical){t.Fatal("WORK journal not durably stored")}
 ctl,err:=store.LoadCycleControl(ctx,[32]byte(run))
 if err!=nil||ctl[0]!=uint64(0){t.Fatal("WORK did not enter IDLE",ctl,err)}
 if _,err:=c.CommitNoProposal(ctx,ledger,cycle,fence);err==nil{t.Fatal("duplicate WORK succeeded")}
 reopened,err:=sqlite.Open(ctx,path);if err!=nil{t.Fatal(err)};defer reopened.Close()
 recovered,err:=reopened.LoadClock(ctx,[32]byte(run))
 if err!=nil||recovered!=after{t.Fatal("resume changed WORK successor",err)}
 sqlDB,err:=sql.Open("sqlite","file:"+path);if err!=nil{t.Fatal(err)};defer sqlDB.Close()
 var decisions,events,world,committed int
 for _,p:=range []struct{table string;out *int}{{"decision_ledger",&decisions},{"event_store",&events},
  {"world_revisions",&world},{"committed_cycles",&committed}}{
  if err:=sqlDB.QueryRowContext(ctx,"SELECT count(*) FROM "+p.table).Scan(p.out);err!=nil{t.Fatal(err)}
 }
 if decisions!=4||world!=2||committed!=1||events!=0{
  t.Fatalf("WORK publication missing a part: decisions=%d events=%d world=%d committed=%d",decisions,events,world,committed)
 }
}
var _=clock.State{}

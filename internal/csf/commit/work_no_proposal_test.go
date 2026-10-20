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
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
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
 pending,err:=reopened.PendingClock(ctx,[32]byte(run));if err!=nil{t.Fatal(err)}
 if next,ok,err:=clock.Next(recovered,pending);err!=nil||ok{
  t.Fatalf("completed NoProposal rounds re-entered pending clock: %+v %v %v",next,ok,err)
 }
 view,_,err:=ledger.AdmissionView(ctx,c.bundle);if err!=nil{t.Fatal(err)}
 _,_,_,rounds,_,_:=view.Snapshot()
 for _,round:=range rounds{
  if round.Lifecycle!=2{t.Fatalf("reopened input ledger re-admitted settled round %x",round.ID[:])}
 }
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

func TestNoProposalWorkRejectsTamperedTerminalRecordAndRecovers(t *testing.T){
 bundle,err:=policy.Load(bundlePath);if err!=nil{t.Fatal(err)}
 cases:=[]struct{
  name string
  mutate func(*ports.WorkBatch)
 }{
  {"decision-order",func(b *ports.WorkBatch){
   b.Decisions[0],b.Decisions[1]=b.Decisions[1],b.Decisions[0]
  }},
  {"decision-body",func(b *ports.WorkBatch){
   b.Decisions[0].Canonical=[]byte{0x80}
  }},
  {"policy-forgery-with-valid-record-digest",func(b *ports.WorkBatch){
   raw,e:=codec.StrictDecodePayload(b.Decisions[0].Canonical);if e!=nil{t.Fatal(e)}
   fields:=raw.([]any)
   fields[9]=[]any{"rogue-validator-v1",uint64(1),make([]byte,32)}
   digest,_,e:=identity.New(bundle).Sum("cote.csf.digest.decision-record",
    "cote.csf.schema.decision-record.body",1,fields[:15])
   if e!=nil{t.Fatal(e)}
   b.Decisions[0].Canonical,e=codec.CanonicalPayload(append(cloneAny(fields[:15]),digest[:]))
   if e!=nil{t.Fatal(e)}
  }},
  {"commit-batch-digest",func(b *ports.WorkBatch){
   b.Journal.Canonical=[]byte{0x80}
  }},
  {"world-revision",func(b *ports.WorkBatch){
   b.RevisionCanonical=[]byte{0x80}
  }},
  {"foreign-input-prefix",func(b *ports.WorkBatch){
   b.ExpectedInputPrefix=b.ExpectedInputPrefix[:len(b.ExpectedInputPrefix)-1]
  }},
  {"missing-terminal-record",func(b *ports.WorkBatch){
   b.Decisions=b.Decisions[:1]
  }},
 }
 for _,tc:=range cases{
  t.Run(tc.name,func(t *testing.T){
   ctx:=context.Background()
   c,store,ledger,cycle,fence,path:=noProposalWorkFixture(t)
   c.tamperWorkBatch=tc.mutate
   if _,err:=c.CommitNoProposal(ctx,ledger,cycle,fence);err==nil{
    t.Fatal("transaction accepted tampered authoritative WORK input")
   }
   sqlDB,err:=sql.Open("sqlite","file:"+path);if err!=nil{t.Fatal(err)};defer sqlDB.Close()
   var decisions,world int
   if err:=sqlDB.QueryRowContext(ctx,"SELECT count(*) FROM decision_ledger").Scan(&decisions);err!=nil{t.Fatal(err)}
   if err:=sqlDB.QueryRowContext(ctx,"SELECT count(*) FROM world_revisions").Scan(&world);err!=nil{t.Fatal(err)}
   if decisions!=1||world!=1{t.Fatalf("tampering persisted an orphan: decision=%d world=%d",decisions,world)}
   c.tamperWorkBatch=nil
   if _,err:=c.CommitNoProposal(ctx,ledger,cycle,fence);err!=nil{
    t.Fatalf("valid deterministic retry after rejected tamper failed: %v",err)
   }
   _=store
  })
 }
}

func TestNoProposalWorkCanCommitOnlyAfterExplicitAbortedAttemptRetry(t *testing.T){
 ctx:=context.Background()
 c,store,ledger,cycle,fence,_:=noProposalWorkFixture(t)
 _,_,_,_,_,run:=ledger.Snapshot()
 abort,err:=c.Abort(ctx,run,cycle.ID,fence,4,nil)
 if err!=nil{t.Fatal(err)}
 if _,err:=c.CommitNoProposal(ctx,ledger,cycle,fence);err==nil{
  t.Fatal("HALTED_ON_ABORT allowed a WORK commit without an explicit retry")
 }
 retry,err:=c.Retry(ctx,run,cycle.ID,abort,"operator.retry")
 if err!=nil{t.Fatal(err)}
 retryRaw,err:=codec.StrictDecodePayload(retry.Canonical);if err!=nil{t.Fatal(err)}
 digest,_,err:=identity.New(c.bundle).Sum("cote.csf.digest.attempt-retry",
  "cote.csf.schema.attempt-retry",1,retryRaw)
 if err!=nil{t.Fatal(err)}
 cycle.Attempt=1
 cycle.RetryRef=[]any{uint64(1),[]any{uint64(14),retry.Identity[:],digest[:]}}
 newFence,err:=admission.NewCoordinator(admission.NewFenceLog(store)).AdmitThenEvaluate(
  ctx,c.bundle,ledger,cycle,c.admissionPolicies,nil)
 if err!=nil{t.Fatal(err)}
 if newFence.InputDigest!=fence.InputDigest||newFence.ID==fence.ID{
  t.Fatal("retry fence did not preserve semantic admitted units")
 }
 journal,err:=c.CommitNoProposal(ctx,ledger,cycle,newFence)
 if err!=nil{t.Fatal(err)}
 canonical,err:=store.GetFence(ctx,journal.Identity)
 if err!=nil||!bytes.Equal(canonical,journal.Canonical){t.Fatal("retried successful WORK journal missing")}
 state,err:=store.LoadClock(ctx,[32]byte(run))
 if err!=nil||state.Revision!=1||state.LastOrdinal!=0{t.Fatalf("WORK retry failed to commit revision: %v %+v",err,state)}
 ctl,err:=store.LoadCycleControl(ctx,[32]byte(run))
 if err!=nil||ctl[0]!=uint64(0){t.Fatalf("WORK retry did not release control state: %v %v",ctl,err)}
}

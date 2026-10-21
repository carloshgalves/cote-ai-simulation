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
 return noProposalWorkFixtureWithSetup(t,nil)
}
func noProposalWorkFixtureWithSetup(t *testing.T,setup func(*sqlite.Store,domain.ID,*policy.Bundle)error)(*Coordinator,*sqlite.Store,*inputledger.Ledger,
 admission.Cycle,admission.Fence,string){
 return noProposalWorkFixtureWithDelivery(t,setup,false)
}
func noProposalWorkFixtureWithDelivery(t *testing.T,setup func(*sqlite.Store,domain.ID,*policy.Bundle)error,reverse bool)(*Coordinator,*sqlite.Store,*inputledger.Ledger,
 admission.Cycle,admission.Fence,string){
 t.Helper()
 ctx:=context.Background()
 c,store,ledger,run,bundle,path:=freshResourceWorkFixture(t)
 if setup!=nil{if err:=setup(store,run,bundle);err!=nil{t.Fatal(err)}}
 state,err:=store.LoadClock(ctx,[32]byte(run));if err!=nil{t.Fatal(err)}
 g:=validGenesis(t,bundle)
 d:=identity.New(bundle)
 cycleID,_,err:=d.Sum("cote.csf.id.cycle","cote.csf.schema.cycle-id-preimage",1,
  []any{run[:],[]any{int64(state.Instant),uint64(0)}});if err!=nil{t.Fatal(err)}
 created:=[]any{uint64(2),[]any{g.ID[:],g.Digest[:]}}
 responseQueue:=make([]inputledger.Response,0,2)
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
  responseQueue=append(responseQueue,response)
 }
 if reverse{responseQueue[0],responseQueue[1]=responseQueue[1],responseQueue[0]}
 for _,response:=range responseQueue{
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
 for stage:=uint8(1);stage<=7;stage++{
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
  after.NextLogicalSequence!=before.NextLogicalSequence+2{t.Fatalf("wrong WORK terminal clock: %+v %v",after,err)}
 persisted,err:=store.GetFence(ctx,commit.Identity)
 if err!=nil||!bytes.Equal(persisted,commit.Canonical){t.Fatal("WORK journal not durably stored")}
 ctl,err:=store.LoadCycleControl(ctx,[32]byte(run))
 if err!=nil||ctl[0]!=uint64(0){t.Fatal("WORK did not enter IDLE",ctl,err)}
 if _,err:=c.CommitNoProposal(ctx,ledger,cycle,fence);err==nil{t.Fatal("duplicate WORK succeeded")}
 reopened,err:=sqlite.Open(ctx,path);if err!=nil{t.Fatal(err)};defer reopened.Close()
 recovered,err:=reopened.LoadClock(ctx,[32]byte(run))
 if err!=nil||recovered!=after{t.Fatal("resume changed WORK successor",err)}
 report,err:=reopened.ReplayClock(ctx,[32]byte(run),c.bundle)
 if err!=nil||report.ReplayedCommits!=1||report.ReplayedEvents!=2||
  report.ReconstructedClock!=after{
  t.Fatalf("zero-effect WORK not independently replayable: %+v %v",report,err)
 }
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
 if decisions!=4||world!=2||committed!=1||events!=2{
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

func TestNoProposalWorkRejectsUnsettledScheduledOccurrenceAtSameCoordinate(t *testing.T){
 ctx:=context.Background()
 c,store,ledger,cycle,fence,_:=noProposalWorkFixtureWithSetup(t,
  func(s *sqlite.Store,run domain.ID,b *policy.Bundle)error{
   g:=validGenesis(t,b)
   d:=identity.New(b)
   created:=[]any{uint64(2),[]any{g.ID[:],g.Digest[:]}}
   id,_,err:=d.Sum("cote.csf.id.occurrence","cote.csf.schema.occurrence-id-preimage",1,
    []any{created,uint64(0),uint64(0)});if err!=nil{return err}
   fields:=[]any{id[:],run[:],"schedule",int64(g.InitialInstant),
    []any{int64(g.InitialInstant),uint64(0)},"test.occurrence",uint64(1),
    g.Payload[16],uint64(0),created,uint64(0),uint64(0),
    []any{uint64(0)},[]any{uint64(0)}}
   digest,_,err:=d.Sum("cote.csf.digest.occurrence-unit",
    "cote.csf.schema.scheduled-occurrence.body",1,fields)
   if err!=nil{return err}
   raw,err:=codec.CanonicalPayload(append(cloneAny(fields),digest[:]))
   if err!=nil{return err}
   return s.SeedGenesisOccurrence(ctx,domain.ID(id),run,
    domain.EligibilityCoordinate{NotBeforeInstant:g.InitialInstant},raw)
  })
 if _,err:=c.CommitNoProposal(ctx,ledger,cycle,fence);err==nil{
  t.Fatal("NoProposal-only WORK hid an unrelated due scheduled occurrence")
 }
 _,_,_,_,_,run:=ledger.Snapshot()
 state,err:=store.LoadClock(ctx,[32]byte(run))
 if err!=nil||state.Revision!=0{t.Fatal("invalid WORK mutated world",err)}
 ctl,err:=store.LoadCycleControl(ctx,[32]byte(run))
 if err!=nil||ctl[0]!=uint64(1){t.Fatalf("rejected WORK did not retain in-flight fence: %v %v",ctl,err)}
}

func TestWorkReplayRejectsMissingTerminalDecision(t *testing.T){
 ctx:=context.Background()
 c,_,ledger,cycle,fence,path:=noProposalWorkFixture(t)
 if _,err:=c.CommitNoProposal(ctx,ledger,cycle,fence);err!=nil{t.Fatal(err)}
 db,err:=sql.Open("sqlite","file:"+path);if err!=nil{t.Fatal(err)}
 // Delete exactly one terminal decision while retaining the CycleCommit
 // and revision bytes: replay must reject the incomplete publication.
 // Fetch the first decision via the committed journal ID list, rather
 // than assuming rowid or DB append order means causal priority.
 _,_,_,_,_,run:=ledger.Snapshot()
 var journalRaw []byte
 if err:=db.QueryRowContext(ctx,"SELECT canonical FROM decision_ledger WHERE id IN (SELECT commit_id FROM committed_cycles WHERE run_id=?)",run[:]).Scan(&journalRaw);err!=nil{t.Fatal(err)}
 v,err:=codec.StrictDecodePayload(journalRaw);if err!=nil{t.Fatal(err)}
 ids:=v.([]any)[16].([]any)
 target:=ids[0].([]byte)
 if _,err:=db.ExecContext(ctx,"DELETE FROM decision_ledger WHERE id=?",target);err!=nil{t.Fatal(err)}
 db.Close()
 reopened,err:=sqlite.Open(ctx,path);if err!=nil{t.Fatal(err)}
 defer reopened.Close()
 if _,err:=reopened.ReplayClock(ctx,[32]byte(run),c.bundle);err==nil{
  t.Fatal("replay accepted an orphaned CycleCommit with a missing terminal decision")
 }
}

func TestReplayClockAdvanceThenNoProposalWorkInOneDurableHistory(t *testing.T){
 ctx:=context.Background()
 c,store,ledger,run,bundle,path:=testFixture(t)
 g:=validGenesis(t,bundle)
 d:=identity.New(bundle)
 created:=[]any{uint64(2),[]any{g.ID[:],g.Digest[:]}}
 // The round MUST be declared before the future-clock fence is sealed.
 // A post-fence round retroeligible at the target is rightly rejected.
 round,err:=ledger.DeclareRound(ctx,"driver",[2]string{"school","a"},[]string{"act"},
  domain.EligibilityCoordinate{NotBeforeInstant:g.InitialInstant+100},
  created,0,0,0,[]any{uint64(0)})
 if err!=nil{t.Fatal(err)}
 if _,err:=c.Advance(ctx,ledger,run);err!=nil{t.Fatal(err)}
 clockBefore,err:=store.LoadClock(ctx,[32]byte(run));if err!=nil{t.Fatal(err)}
 if clockBefore.Revision!=1||clockBefore.LastOrdinal!=-1{t.Fatal("expected temporal successor before WORK")}
 cycleID,_,err:=d.Sum("cote.csf.id.cycle","cote.csf.schema.cycle-id-preimage",1,
  []any{run[:],[]any{int64(clockBefore.Instant),uint64(0)}});if err!=nil{t.Fatal(err)}
 dispatchID,_,err:=d.Sum("cote.csf.id.dispatch","cote.csf.schema.dispatch-id-preimage",1,
  []any{run[:],cycleID[:],round.ID[:],"act",uint64(0)});if err!=nil{t.Fatal(err)}
 dispatch:=inputledger.Dispatch{ID:domain.ID(dispatchID),RunID:run,CycleID:domain.ID(cycleID),RoundID:round.ID,
  SlotID:"act",Actor:round.Actor,Eligibility:round.Eligibility,
  BaseRevision:clockBefore.Revision,EffectiveAt:clockBefore.Instant}
 if err:=ledger.RegisterDispatch(ctx,dispatch,inputledger.CycleBinding{
  ID:domain.ID(cycleID),BaseRevision:clockBefore.Revision,Instant:clockBefore.Instant});err!=nil{t.Fatal(err)}
 unitID,_,err:=d.Sum("cote.csf.id.no-proposal","cote.csf.schema.slot-response-id-preimage",1,
  []any{dispatch.ID[:],uint64(1)});if err!=nil{t.Fatal(err)}
 fields:=[]any{unitID[:],dispatch.ID[:],run[:],cycleID[:],round.ID[:],dispatch.SlotID,
  []any{dispatch.Actor[0],dispatch.Actor[1]},int64(dispatch.EffectiveAt),uint64(dispatch.BaseRevision),uint64(0)}
 dig,_,err:=d.Sum("cote.csf.digest.no-proposal-unit","cote.csf.schema.no-proposal.body",1,[]any{fields})
 if err!=nil{t.Fatal(err)}
 canonical,err:=codec.CanonicalPayload(append(cloneAny(fields),dig[:]));if err!=nil{t.Fatal(err)}
 response:=inputledger.Response{Kind:1,ID:domain.ID(unitID),Digest:dig,Canonical:canonical,Body:fields,
  DispatchID:dispatch.ID,RunID:run,CycleID:domain.ID(cycleID),RoundID:round.ID,SlotID:dispatch.SlotID,
  Actor:dispatch.Actor,EffectiveAt:dispatch.EffectiveAt,SubmittedAgainstRevision:dispatch.BaseRevision}
 if err:=ledger.Respond(ctx,response);err!=nil{t.Fatal(err)}
 typed,hash,err:=store.LoadWorldState(ctx,uint64(clockBefore.Revision))
 if err!=nil||len(typed)!=4{t.Fatalf("no authoritative revision after clock advance: %v",err)}
 cycle:=admission.Cycle{ID:domain.ID(cycleID),Attempt:0,Instant:clockBefore.Instant,Ordinal:0,
  BaseRevision:clockBefore.Revision,BaseStateHash:domain.Digest(hash),RetryRef:[]any{uint64(0)}}
 fence,err:=admission.NewCoordinator(admission.NewFenceLog(store)).AdmitThenEvaluate(
  ctx,bundle,ledger,cycle,c.admissionPolicies,nil);if err!=nil{t.Fatal(err)}
 if _,err:=c.CommitNoProposal(ctx,ledger,cycle,fence);err!=nil{t.Fatal(err)}
 reopened,err:=sqlite.Open(ctx,path);if err!=nil{t.Fatal(err)};defer reopened.Close()
 report,err:=reopened.ReplayClock(ctx,[32]byte(run),bundle)
 if err!=nil||report.ReplayedCommits!=2||report.ReplayedEvents!=3||report.ReconstructedClock.Revision!=2{
  t.Fatalf("mixed journal does not replay: %+v %v",report,err)
 }
}

func TestNoProposalWorkerResponsePermutationPreservesTerminalArtifacts(t *testing.T){
 ctx:=context.Background()
 type output struct{fence,commit []byte;events,decisions [][]byte}
 var baseline *output
 for _,reverse:=range []bool{false,true}{
  c,store,ledger,cycle,fence,path:=noProposalWorkFixtureWithDelivery(t,nil,reverse)
  terminal,err:=c.CommitNoProposal(ctx,ledger,cycle,fence);if err!=nil{t.Fatal(err)}
  db,err:=sql.Open("sqlite","file:"+path);if err!=nil{t.Fatal(err)}
  result:=&output{fence:fence.Canonical,commit:terminal.Canonical}
  for _,query:=range []struct{stmt string;out *[][]byte}{
   {"SELECT canonical FROM event_store ORDER BY id",&result.events},
   {"SELECT canonical FROM decision_ledger WHERE id NOT IN (?,?) ORDER BY id",&result.decisions},
  }{
   var rows *sql.Rows
   if query.stmt=="SELECT canonical FROM event_store ORDER BY id"{
    rows,err=db.QueryContext(ctx,query.stmt)
   }else{
    rows,err=db.QueryContext(ctx,query.stmt,fence.ID[:],terminal.Identity[:])
   }
   if err!=nil{t.Fatal(err)}
   for rows.Next(){var raw []byte;if err:=rows.Scan(&raw);err!=nil{t.Fatal(err)};*query.out=append(*query.out,raw)}
   if err:=rows.Err();err!=nil{t.Fatal(err)};rows.Close()
  }
  db.Close()
  if baseline==nil{baseline=result;continue}
  if !bytes.Equal(result.fence,baseline.fence)||!bytes.Equal(result.commit,baseline.commit){
   t.Fatal("worker response arrival order changed fenced or terminal canonical bytes")
  }
  for i,pair:=range []struct{a,b [][]byte}{{result.events,baseline.events},{result.decisions,baseline.decisions}}{
   if len(pair.a)!=len(pair.b){t.Fatalf("permutation changed artifact set %d",i)}
   for j:=range pair.a{if !bytes.Equal(pair.a[j],pair.b[j]){t.Fatalf("worker arrival order altered artifact %d/%d",i,j)}}
  }
  _=store;_=ledger
 }
}

func TestScheduledOccurrenceIsExplicitlyAdmittedToSealedFence(t *testing.T){
 ctx:=context.Background()
 var seeded domain.ID
 c,store,ledger,cycle,fence,_:=noProposalWorkFixtureWithSetup(t,func(store *sqlite.Store,run domain.ID,bundle *policy.Bundle)error{
  g:=validGenesis(t,bundle)
  d:=identity.New(bundle)
  origin:=[]any{uint64(2),[]any{g.ID[:],g.Digest[:]}}
  value,_,err:=d.Sum("cote.csf.id.occurrence","cote.csf.schema.occurrence-id-preimage",1,
   []any{origin,uint64(0),uint64(0)});if err!=nil{return err}
  seeded=domain.ID(value)
  fields:=[]any{value[:],run[:],"schedule",int64(g.InitialInstant),
   []any{int64(g.InitialInstant),uint64(0)},"test.occurrence",uint64(1),
   g.Payload[16],uint64(0),origin,uint64(0),uint64(0),[]any{uint64(0)},[]any{uint64(0)}}
  dg,_,err:=d.Sum("cote.csf.digest.occurrence-unit","cote.csf.schema.scheduled-occurrence.body",1,fields)
  if err!=nil{return err}
  raw,err:=codec.CanonicalPayload(append(cloneAny(fields),dg[:]));if err!=nil{return err}
  return store.SeedGenesisOccurrence(ctx,seeded,run,
   domain.EligibilityCoordinate{NotBeforeInstant:g.InitialInstant},raw)
 })
 entries,ok:=fence.Fields[11].([]any)
 if !ok||len(entries)!=3{t.Fatalf("fence lost due occurrence, admitted=%#v",fence.Fields[11])}
 found:=false
 for _,item:=range entries{
  v,ok:=item.([]any);if !ok||len(v)!=5{t.Fatal("malformed source ref")}
  if v[1]==uint64(2){
   if !bytes.Equal(v[3].([]byte),seeded[:]){t.Fatal("wrong due occurrence admitted")}
   found=true
  }
 }
 if !found{t.Fatal("occurrence not in frozen admitted source cut")}
 if _,err:=c.CommitNoProposal(ctx,ledger,cycle,fence);err==nil{
  t.Fatal("slot-only handler settled occurrence without an explicit terminal lifecycle")
 }
 _,_,_,_,_,run:=ledger.Snapshot()
 state,err:=store.LoadClock(ctx,[32]byte(run))
 if err!=nil||state.Revision!=0{t.Fatal("failed closed WORK mutated clock",err)}
}

func TestMixedNoProposalAndOccurrenceMustCloseOneFrozenCohort(t *testing.T){
 ctx:=context.Background()
 c,store,ledger,cycle,fence,path:=noProposalWorkFixtureWithSetup(t,func(store *sqlite.Store,run domain.ID,bundle *policy.Bundle)error{
  g:=validGenesis(t,bundle)
  d:=identity.New(bundle)
  origin:=[]any{uint64(2),[]any{g.ID[:],g.Digest[:]}}
  id,_,err:=d.Sum("cote.csf.id.occurrence","cote.csf.schema.occurrence-id-preimage",1,
   []any{origin,uint64(0),uint64(0)});if err!=nil{return err}
  fields:=[]any{id[:],run[:],"schedule",int64(g.InitialInstant),
   []any{int64(g.InitialInstant),uint64(0)},"test.occurrence",uint64(1),
   g.Payload[16],uint64(0),origin,uint64(0),uint64(0),[]any{uint64(0)},[]any{uint64(0)}}
  dg,_,err:=d.Sum("cote.csf.digest.occurrence-unit","cote.csf.schema.scheduled-occurrence.body",1,fields)
  if err!=nil{return err}
  raw,err:=codec.CanonicalPayload(append(cloneAny(fields),dg[:]));if err!=nil{return err}
  return store.SeedGenesisOccurrence(ctx,domain.ID(id),run,
   domain.EligibilityCoordinate{NotBeforeInstant:g.InitialInstant},raw)
 })
 if len(fence.Fields[11].([]any))!=3{t.Fatal("mixed cohort not completely admitted")}
 _,_,_,_,_,run:=ledger.Snapshot()
 previous,err:=store.LoadClock(ctx,[32]byte(run));if err!=nil{t.Fatal(err)}
 for step:=uint8(1);step<=7;step++{
  c.failAfterStep=step
  if _,err:=c.CommitNoProposal(ctx,ledger,cycle,fence);err==nil{t.Fatalf("mixed WORK failpoint %d bypassed",step)}
  state,err:=store.LoadClock(ctx,[32]byte(run))
  if err!=nil||state!=previous{t.Fatal("failed mixed WORK half-published clock",err)}
 }
 c.failAfterStep=0
 if _,err:=c.CommitNoProposal(ctx,ledger,cycle,fence);err!=nil{
  t.Fatalf("mixed NoProposal/occurrence cohort cannot complete as one terminal WORK: %v",err)
 }
 current,err:=store.LoadClock(ctx,[32]byte(run))
 if err!=nil||current.Revision!=1||current.NextLogicalSequence!=3{t.Fatal("mixed WORK missed event/revision",err)}
 pending,err:=store.PendingClock(ctx,[32]byte(run))
 if err!=nil||len(pending)!=0{t.Fatal("mixed terminal left admitted work pending",err)}
 db,err:=sql.Open("sqlite","file:"+path);if err!=nil{t.Fatal(err)}
 var count int
 if err:=db.QueryRowContext(ctx,"SELECT count(*) FROM event_store").Scan(&count);err!=nil{t.Fatal(err)}
 db.Close()
 if count!=3{t.Fatalf("mixed WORK published %d, expected 3 lifecycle events",count)}
 reopened,err:=sqlite.Open(ctx,path);if err!=nil{t.Fatal(err)}
 report,err:=reopened.ReplayClock(ctx,[32]byte(run),c.bundle)
 reopened.Close()
 if err!=nil||report.ReplayedCommits!=1||report.ReplayedEvents!=3{
  t.Fatalf("mixed WORK not replayable: %+v %v",report,err)
 }
}

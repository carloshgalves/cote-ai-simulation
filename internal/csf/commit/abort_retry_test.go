package commit

import (
 "bytes"
 "context"
 "database/sql"
 "errors"
 "testing"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/admission"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/clock"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/decision"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/inputledger"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/extensions/testresourcev1"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
)

func abortFixture(t *testing.T)(*Coordinator,*sqlite.Store,*inputledger.Ledger,admission.Fence,domain.ID,domain.ID,string){
 t.Helper()
 ctx:=context.Background()
 c,store,ledger,runID,bundle,path:=testFixture(t)
 if _,err:=c.Advance(ctx,ledger,runID);err!=nil{t.Fatal(err)}
 state,err:=store.LoadClock(ctx,[32]byte(runID));if err!=nil{t.Fatal(err)}
 pending,err:=store.PendingClock(ctx,[32]byte(runID));if err!=nil{t.Fatal(err)}
 plan,ok,err:=clock.Next(state,pending)
 if err!=nil||!ok||plan.Kind!=clock.Work{t.Fatalf("WORK not ready: %v %v",plan,err)}
 d:=identity.New(bundle)
 id,_,err:=d.Sum("cote.csf.id.cycle","cote.csf.schema.cycle-id-preimage",1,
  []any{runID[:],[]any{int64(state.Instant),uint64(plan.OriginOrdinal)}})
 if err!=nil{t.Fatal(err)}
 genesis:=validGenesis(t,bundle)
 var worldHash domain.Digest
 copy(worldHash[:],genesis.Payload[16].([]any)[3].([]byte))
 cycle:=admission.Cycle{ID:domain.ID(id),Attempt:0,Instant:state.Instant,
  Ordinal:plan.OriginOrdinal,BaseRevision:state.Revision,BaseStateHash:worldHash,RetryRef:[]any{uint64(0)}}
 fence,err:=admission.NewCoordinator(admission.NewFenceLog(store)).AdmitThenEvaluate(
  ctx,bundle,ledger,cycle,c.admissionPolicies,nil)
 if err!=nil{t.Fatal(err)}
 return c,store,ledger,fence,runID,cycle.ID,path
}

func TestAbortAtomicFailpointsAndExplicitRetry(t *testing.T){
 ctx:=context.Background()
 c,store,_,fence,runID,cycleID,_:=abortFixture(t)
 original,err:=store.LoadClock(ctx,[32]byte(runID));if err!=nil{t.Fatal(err)}
 input,err:=clock.Encode(original);if err!=nil{t.Fatal(err)}
 for _,stage:=range []uint8{1,2}{
  c.failAfterStep=stage
  if _,err:=c.Abort(ctx,runID,cycleID,fence,4,nil);err==nil{t.Fatalf("abort step %d did not fail",stage)}
  if after,err:=store.LoadClock(ctx,[32]byte(runID));err!=nil||after!=original{t.Fatal("abort mutated clock",err)}
  ctl,err:=store.LoadCycleControl(ctx,[32]byte(runID))
  if err!=nil||ctl[0]!=uint64(1){t.Fatalf("abort rollback lost the in-flight fence at %d: %v %v",stage,ctl,err)}
 }
 c.failAfterStep=0
 abort,err:=c.Abort(ctx,runID,cycleID,fence,4,nil)
 if err!=nil{t.Fatal(err)}
 raw,err:=store.GetFence(ctx,abort.Identity);if err!=nil||!bytes.Equal(raw,abort.Canonical){t.Fatal("abort journal missing")}
 ctl,err:=store.LoadCycleControl(ctx,[32]byte(runID))
 if err!=nil||ctl[0]!=uint64(3)||ctl[6]!=uint64(1){t.Fatalf("not halted: %v %v",ctl,err)}
 if _,err:=c.Abort(ctx,runID,cycleID,fence,4,nil);err==nil{t.Fatal("second abort accepted")}
 for _,stage:=range []uint8{1,2}{
  c.failAfterStep=stage
  if _,err:=c.Retry(ctx,runID,cycleID,abort,"operator.retry");err==nil{t.Fatalf("retry stage %d did not fail",stage)}
  ctl,err:=store.LoadCycleControl(ctx,[32]byte(runID))
  if err!=nil||ctl[0]!=uint64(3){t.Fatalf("partial retry state at %d: %v %v",stage,ctl,err)}
 }
 c.failAfterStep=0
 retry,err:=c.Retry(ctx,runID,cycleID,abort,"operator.retry")
 if err!=nil{t.Fatal(err)}
 raw,err=store.GetFence(ctx,retry.Identity)
 if err!=nil||!bytes.Equal(raw,retry.Canonical){t.Fatal("retry journal missing")}
 ctl,err=store.LoadCycleControl(ctx,[32]byte(runID))
 if err!=nil||ctl[0]!=uint64(2)||ctl[6]!=uint64(1){t.Fatalf("not retry-authorized: %v %v",ctl,err)}
 if _,err:=c.Retry(ctx,runID,cycleID,abort,"operator.retry");err==nil{t.Fatal("second retry accepted")}
 after,err:=store.LoadClock(ctx,[32]byte(runID))
 if err!=nil||after!=original{t.Fatalf("terminal abort/retry mutated world clock: %v",err)}
 if !bytes.Equal(input,func()[]byte{now,_:=clock.Encode(after);return now}()){t.Fatal("clock bytes changed")}
}
func TestAbortRejectsMalformedOrForeignFence(t *testing.T){
 ctx:=context.Background()
 c,store,_,fence,runID,cycleID,_:=abortFixture(t)
 forged:=fence
 forged.ID[0]^=0xff
 if _,err:=c.Abort(ctx,runID,cycleID,forged,4,nil);err==nil{t.Fatal("accepted foreign fence root")}
 ctl,err:=store.LoadCycleControl(ctx,[32]byte(runID))
 if err!=nil||ctl[0]!=uint64(1){t.Fatal("foreign fence changed the authenticated in-flight state")}
}

// Compile-time assertion: the existing coordinator store remains the one
// canonical transaction authority for temporal commit, abort and retry.
var _ ports.ClockAdvanceStore=(*sqlite.Store)(nil)
var _ *policy.Bundle
var _ *inputledger.Ledger
var _ =codec.CanonicalPayload

func TestRetryMustReopenTheSameSemanticCut(t *testing.T){
 ctx:=context.Background()
 c,store,ledger,fence,runID,cycleID,_:=abortFixture(t)
 abort,err:=c.Abort(ctx,runID,cycleID,fence,4,nil)
 if err!=nil{t.Fatal(err)}
 d:=identity.New(c.bundle)
 // HALTED is not permission to open a new admission fence.
 next:=admission.Cycle{ID:cycleID,Attempt:1}
 next.Instant=domain.SimulationInstant(func()int64{x,_:=fence.Fields[4].(int64);return x}())
 next.Ordinal=domain.CycleOrdinal(fence.Fields[5].(uint64))
 next.BaseRevision=domain.WorldRevision(fence.Fields[7].(uint64))
 copy(next.BaseStateHash[:],fence.Fields[8].([]byte))
 next.RetryRef=[]any{uint64(0)}
 guard:=admission.NewCoordinator(admission.NewFenceLog(store))
 if _,err:=guard.AdmitThenEvaluate(ctx,c.bundle,ledger,next,c.admissionPolicies,nil);err==nil{
  t.Fatal("halted run accepted fence before explicit retry")
 }
 retry,err:=c.Retry(ctx,runID,cycleID,abort,"operator.retry")
 if err!=nil{t.Fatal(err)}
 dig,_,err:=d.Sum("cote.csf.digest.attempt-retry","cote.csf.schema.attempt-retry",1,
  []any{runID[:],cycleID[:],uint64(0),uint64(1),
   []any{uint64(13),abort.Identity[:],abortDigest(t,c.bundle,abort)}, "operator.retry",fence.Fields[16]})
 if err!=nil{t.Fatal(err)}
 next.RetryRef=[]any{uint64(1),[]any{uint64(14),retry.Identity[:],dig[:]}}
 newFence,err:=guard.AdmitThenEvaluate(ctx,c.bundle,ledger,next,c.admissionPolicies,nil)
 if err!=nil{t.Fatal(err)}
 if newFence.ID==fence.ID||newFence.InputDigest!=fence.InputDigest{
  t.Fatal("retry did not retain same semantic input digest with new attempt identity")
 }
 ctl,err:=store.LoadCycleControl(ctx,[32]byte(runID))
 if err!=nil||ctl[0]!=uint64(1){t.Fatalf("retry fence not IN_FLIGHT: %v %v",ctl,err)}
 if _,err:=c.Abort(ctx,runID,cycleID,newFence,4,nil);err!=nil{t.Fatalf("retried attempt failed to abort: %v",err)}
 ctl,err=store.LoadCycleControl(ctx,[32]byte(runID))
 if err!=nil||ctl[0]!=uint64(3)||ctl[6]!=uint64(2){t.Fatalf("second terminal state invalid: %v %v",ctl,err)}
}
func abortDigest(t *testing.T,b *policy.Bundle,record ports.CanonicalRecord)[]byte{
 t.Helper()
 raw,err:=codec.StrictDecodePayload(record.Canonical);if err!=nil{t.Fatal(err)}
 fields:=raw.([]any)
 dig,_,err:=identity.New(b).Sum("cote.csf.digest.abort","cote.csf.schema.cycle-abort.body",1,fields[:14])
 if err!=nil{t.Fatal(err)}
 return dig[:]
}

func TestTerminalControlSurvivesIndependentSQLiteReopen(t *testing.T){
 ctx:=context.Background()
 c,_,_,fence,run,cycle,path:=abortFixture(t)
 abort,err:=c.Abort(ctx,run,cycle,fence,4,nil);if err!=nil{t.Fatal(err)}
 second,err:=sqlite.Open(ctx,path);if err!=nil{t.Fatal(err)}
 defer second.Close()
 ctl,err:=second.LoadCycleControl(ctx,[32]byte(run))
 if err!=nil||ctl[0]!=uint64(3)||ctl[6]!=uint64(1){
  t.Fatalf("reopened process missed HALTED_ON_ABORT: %v %v",ctl,err)
 }
 if _,err:=second.GetFence(ctx,abort.Identity);err!=nil{t.Fatal("abort not durable",err)}
 retry,err:=c.Retry(ctx,run,cycle,abort,"operator.retry");if err!=nil{t.Fatal(err)}
 ctl,err=second.LoadCycleControl(ctx,[32]byte(run))
 if err!=nil||ctl[0]!=uint64(2)||ctl[6]!=uint64(1){
  t.Fatalf("reopened process missed RETRY_AUTHORIZED: %v %v",ctl,err)
 }
 if _,err:=second.GetFence(ctx,retry.Identity);err!=nil{t.Fatal("retry not durable",err)}
}

func TestIndeterminateResourceFenceOnlyPublishesAbort(t *testing.T){
 ctx:=context.Background()
 c,store,ledger,run,bundle,path:=freshResourceWorkFixture(t)
 state,err:=store.LoadClock(ctx,[32]byte(run));if err!=nil{t.Fatal(err)}
 g:=validGenesis(t,bundle)
 created:=[]any{uint64(2),[]any{g.ID[:],g.Digest[:]}}
 d:=identity.New(bundle)
 cycleDigest,_,err:=d.Sum("cote.csf.id.cycle","cote.csf.schema.cycle-id-preimage",1,
  []any{run[:],[]any{int64(state.Instant),uint64(0)}})
 if err!=nil{t.Fatal(err)}
 cycleID:=domain.ID(cycleDigest)
 for i,actor:=range []string{"a","b"}{
  round,err:=ledger.DeclareRound(ctx,"schedule",[2]string{"school",actor},[]string{"act"},
   domain.EligibilityCoordinate{NotBeforeInstant:state.Instant},created,0,uint32(i),0,[]any{uint64(0)})
  if err!=nil{t.Fatal(err)}
  id,_,err:=d.Sum("cote.csf.id.dispatch","cote.csf.schema.dispatch-id-preimage",1,
   []any{run[:],cycleID[:],round.ID[:],"act",uint64(0)})
  if err!=nil{t.Fatal(err)}
  dispatch:=inputledger.Dispatch{ID:domain.ID(id),RunID:run,CycleID:cycleID,RoundID:round.ID,
   SlotID:"act",Actor:round.Actor,Eligibility:round.Eligibility,
   BaseRevision:state.Revision,EffectiveAt:state.Instant}
  if err:=ledger.RegisterDispatch(ctx,dispatch,inputledger.CycleBinding{
   ID:cycleID,BaseRevision:state.Revision,Instant:state.Instant});err!=nil{t.Fatal(err)}
  response,err:=testresourcev1.ProposalForDispatch(bundle,dispatch);if err!=nil{t.Fatal(err)}
  if err:=ledger.Respond(ctx,response);err!=nil{t.Fatal(err)}
 }
 var hash domain.Digest;copy(hash[:],g.Payload[16].([]any)[3].([]byte))
 cycle:=admission.Cycle{ID:cycleID,Attempt:0,Instant:state.Instant,Ordinal:0,
  BaseRevision:state.Revision,BaseStateHash:hash,RetryRef:[]any{uint64(0)}}
 fence,err:=admission.NewCoordinator(admission.NewFenceLog(store)).AdmitThenEvaluate(
  ctx,bundle,ledger,cycle,c.admissionPolicies,nil)
 if err!=nil{t.Fatal(err)}
 request,err:=decision.FromPersistedFence(ctx,bundle,ledger,cycle,c.admissionPolicies,fence,store,
  decision.Snapshot{BaseRevision:state.Revision,ResourceKind:"inventory",ResourceID:"last.resource",
   Available:1,Known:false,AccessibleActors:map[string]bool{"school:a":true,"school:b":true},
   ForbiddenActors:map[string]bool{"school:a":false,"school:b":false}},
  []decision.ActionSchema{{Type:"resource.consume",Version:1,Operation:"resource.consume",Producer:"resource.consume"}},
  domain.Digest{},nil)
 if err!=nil{t.Fatal(err)}
 evaluated,err:=decision.Run(request)
 if !errors.Is(err,decision.ErrIndeterminate)||len(evaluated.Indeterminate)!=2||
  len(evaluated.Dispositions)!=0{t.Fatalf("invalid tentative decision: %v %+v",err,evaluated)}
 abort,err:=c.Abort(ctx,run,cycleID,fence,0,evaluated.Indeterminate)
 if err!=nil{t.Fatalf("INDETERMINATE did not close as terminal abort: %v",err)}
 if _,err:=store.GetFence(ctx,abort.Identity);err!=nil{t.Fatal(err)}
 after,err:=store.LoadClock(ctx,[32]byte(run))
 if err!=nil||after!=state{t.Fatal("INDETERMINATE changed world revision/clock",err)}
 other,err:=sql.Open("sqlite","file:"+path);if err!=nil{t.Fatal(err)};defer other.Close()
 var decisionRecords, events int
 if err:=other.QueryRowContext(ctx,"SELECT count(*) FROM decision_ledger").Scan(&decisionRecords);err!=nil{t.Fatal(err)}
 if err:=other.QueryRowContext(ctx,"SELECT count(*) FROM event_store").Scan(&events);err!=nil{t.Fatal(err)}
 // This fresh run contains exactly the durable WORK fence and abort,
 // and no event, terminal decision or world revision successor.
 if decisionRecords!=2||events!=0{t.Fatalf("abort invented terminal decisions/events: ledger=%d events=%d",decisionRecords,events)}
 for _,unknown:=range evaluated.Indeterminate{
  forged:=unknown
  forged.Subject[0]^=0xff
  if _,err:=c.Abort(ctx,run,cycleID,fence,0,[]decision.Indeterminate{forged});err==nil{
   t.Fatal("accepted fabricated admitted source in an abort record")
  }
 }
}

func freshResourceWorkFixture(t *testing.T)(*Coordinator,*sqlite.Store,*inputledger.Ledger,domain.ID,*policy.Bundle,string){
 t.Helper()
 ctx:=context.Background()
 bundle,err:=policy.Load(bundlePath);if err!=nil{t.Fatal(err)}
 g:=validGenesis(t,bundle)
 path:=t.TempDir()+"/resource-work.db"
 store,err:=sqlite.Open(ctx,path);if err!=nil{t.Fatal(err)}
 t.Cleanup(func(){store.Close()})
 if err:=store.PersistGenesis(ctx,g,bundle);err!=nil{t.Fatal(err)}
 if err:=store.BootstrapClock(ctx,g,bundle);err!=nil{t.Fatal(err)}
 ledger,err:=inputledger.New(ctx,g.RunID,bundle,store);if err!=nil{t.Fatal(err)}
 if err:=ledger.DeclareSource(inputledger.Source{ID:"driver"});err!=nil{t.Fatal(err)}
 covered:=domain.EligibilityCoordinate{NotBeforeInstant:g.InitialInstant}
 if _,err:=ledger.CloseSource(ctx,"driver",inputledger.Closure{ProducerKey:"ready",ClosedThrough:covered});err!=nil{t.Fatal(err)}
 refs,err:=policiesByRole(bundle);if err!=nil{t.Fatal(err)}
 policies:=admission.Policies{AdmissionOrder:refs["admission-order"],
  CausalIdentity:refs["causal-identity"],Fence:refs["admission-fence"],Rules:[][]any{}}
 return New(store,bundle,policies),store,ledger,g.RunID,bundle,path
}

func TestCandidateIndeterminateEvidenceBoundToSealedAdmittedUnits(t *testing.T){
 ctx:=context.Background()
 c,store,ledger,run,bundle,path:=freshResourceWorkFixture(t)
 state,err:=store.LoadClock(ctx,[32]byte(run));if err!=nil{t.Fatal(err)}
 genesis:=validGenesis(t,bundle)
 created:=[]any{uint64(2),[]any{genesis.ID[:],genesis.Digest[:]}}
 d:=identity.New(bundle)
 cycleHash,_,err:=d.Sum("cote.csf.id.cycle","cote.csf.schema.cycle-id-preimage",1,
  []any{run[:],[]any{int64(state.Instant),uint64(0)}});if err!=nil{t.Fatal(err)}
 cycleID:=domain.ID(cycleHash)
 access:=map[string]bool{}
 forbidden:=map[string]bool{}
 for i,actor:=range []string{"a","b","c"}{
  round,err:=ledger.DeclareRound(ctx,"schedule",[2]string{"school",actor},[]string{"act"},
   domain.EligibilityCoordinate{NotBeforeInstant:state.Instant},created,0,uint32(i),0,[]any{uint64(0)})
  if err!=nil{t.Fatal(err)}
  id,_,err:=d.Sum("cote.csf.id.dispatch","cote.csf.schema.dispatch-id-preimage",1,
   []any{run[:],cycleID[:],round.ID[:],"act",uint64(0)});if err!=nil{t.Fatal(err)}
  dispatch:=inputledger.Dispatch{ID:domain.ID(id),RunID:run,CycleID:cycleID,RoundID:round.ID,
   SlotID:"act",Actor:round.Actor,Eligibility:round.Eligibility,
   BaseRevision:state.Revision,EffectiveAt:state.Instant}
  if err:=ledger.RegisterDispatch(ctx,dispatch,inputledger.CycleBinding{
   ID:cycleID,BaseRevision:state.Revision,Instant:state.Instant});err!=nil{t.Fatal(err)}
  response,err:=testresourcev1.ProposalForDispatch(bundle,dispatch);if err!=nil{t.Fatal(err)}
  if err:=ledger.Respond(ctx,response);err!=nil{t.Fatal(err)}
  access["school:"+actor]=true;forbidden["school:"+actor]=false
 }
 var worldHash domain.Digest;copy(worldHash[:],genesis.Payload[16].([]any)[3].([]byte))
 cycle:=admission.Cycle{ID:cycleID,Attempt:0,Instant:state.Instant,Ordinal:0,
  BaseRevision:state.Revision,BaseStateHash:worldHash,RetryRef:[]any{uint64(0)}}
 fence,err:=admission.NewCoordinator(admission.NewFenceLog(store)).AdmitThenEvaluate(
  ctx,bundle,ledger,cycle,c.admissionPolicies,nil);if err!=nil{t.Fatal(err)}
 req,err:=decision.FromPersistedFence(ctx,bundle,ledger,cycle,c.admissionPolicies,fence,store,
  decision.Snapshot{BaseRevision:state.Revision,ResourceKind:"inventory",ResourceID:"last.resource",
   Available:1,Known:true,AccessibleActors:access,ForbiddenActors:forbidden},
  []decision.ActionSchema{{Type:"resource.consume",Version:1,Operation:"resource.consume",Producer:"resource.consume"}},
  domain.Digest{},nil)
 if err!=nil{t.Fatal(err)}
 out,err:=decision.Run(req)
 if !errors.Is(err,decision.ErrIndeterminate)||len(out.Indeterminate)!=3||
  len(out.Candidates)!=3||len(out.Dispositions)!=0{
  t.Fatalf("three contenders unexpectedly received an unapproved resolver: %v %+v",err,out)
 }
 forged:=append([]decision.Candidate(nil),out.Candidates...)
 forged[0].Canonical=[]byte{0x80}
 if _,err:=c.Abort(ctx,run,cycleID,fence,0,out.Indeterminate,forged...);err==nil{
  t.Fatal("accepted malformed candidate proof")
 }
 ctl,err:=store.LoadCycleControl(ctx,[32]byte(run))
 if err!=nil||ctl[0]!=uint64(1){t.Fatalf("failed candidate proof did not roll back: %v %v",ctl,err)}
 abort,err:=c.Abort(ctx,run,cycleID,fence,0,out.Indeterminate,out.Candidates...)
 if err!=nil{t.Fatalf("fenced candidate proof failed: %v",err)}
 second,err:=sql.Open("sqlite","file:"+path);if err!=nil{t.Fatal(err)};defer second.Close()
 var records int
 if err:=second.QueryRowContext(ctx,"SELECT count(*) FROM decision_ledger").Scan(&records);err!=nil{t.Fatal(err)}
 if records!=5{t.Fatalf("candidate audit roots not durably grouped with abort: %d",records)}
 for _,candidate:=range out.Candidates{
  raw,err:=store.GetFence(ctx,candidate.ID)
  if err!=nil||!bytes.Equal(raw,candidate.Canonical){t.Fatalf("candidate evidence lost: %v",err)}
 }
 if _,err:=store.GetFence(ctx,abort.Identity);err!=nil{t.Fatal(err)}
 after,err:=store.LoadClock(ctx,[32]byte(run));if err!=nil||after!=state{t.Fatal("failed candidate resolution mutated world",err)}
}

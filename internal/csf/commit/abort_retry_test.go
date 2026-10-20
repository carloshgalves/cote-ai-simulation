package commit

import (
 "bytes"
 "context"
 "testing"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/admission"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/clock"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/inputledger"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
)

func abortFixture(t *testing.T)(*Coordinator,*sqlite.Store,admission.Fence,domain.ID,domain.ID){
 t.Helper()
 ctx:=context.Background()
 c,store,ledger,runID,bundle,_:=testFixture(t)
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
 return c,store,fence,runID,cycle.ID
}

func TestAbortAtomicFailpointsAndExplicitRetry(t *testing.T){
 ctx:=context.Background()
 c,store,fence,runID,cycleID:=abortFixture(t)
 original,err:=store.LoadClock(ctx,[32]byte(runID));if err!=nil{t.Fatal(err)}
 input,err:=clock.Encode(original);if err!=nil{t.Fatal(err)}
 for _,stage:=range []uint8{1,2}{
  c.failAfterStep=stage
  if _,err:=c.Abort(ctx,runID,cycleID,fence,4,nil);err==nil{t.Fatalf("abort step %d did not fail",stage)}
  if after,err:=store.LoadClock(ctx,[32]byte(runID));err!=nil||after!=original{t.Fatal("abort mutated clock",err)}
  ctl,err:=store.LoadCycleControl(ctx,[32]byte(runID))
  if err!=nil||ctl[0]!=uint64(0){t.Fatalf("partial abort state at %d: %v %v",stage,ctl,err)}
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
 c,store,fence,runID,cycleID:=abortFixture(t)
 forged:=fence
 forged.ID[0]^=0xff
 if _,err:=c.Abort(ctx,runID,cycleID,forged,4,nil);err==nil{t.Fatal("accepted foreign fence root")}
 ctl,err:=store.LoadCycleControl(ctx,[32]byte(runID))
 if err!=nil||ctl[0]!=uint64(0){t.Fatal("foreign fence mutated run")}
}

// Compile-time assertion: the existing coordinator store remains the one
// canonical transaction authority for temporal commit, abort and retry.
var _ ports.ClockAdvanceStore=(*sqlite.Store)(nil)
var _ *policy.Bundle
var _ *inputledger.Ledger
var _ =codec.CanonicalPayload

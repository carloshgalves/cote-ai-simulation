package commit

import (
 "context"
 "errors"
 "fmt"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/admission"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/inputledger"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
)

// ResumeWorkHandler is the same typed terminal handler used by continuous
// execution. It cannot write an Event or replay a proposal directly: all
// registered handlers must go through CommitWork's SQLite compare-and-swap.
// ResumeRunV1 never invokes arbitrary text-generation or selects a new cycle.
type ResumeWorkHandler func(context.Context,*inputledger.Ledger,admission.Cycle,admission.Fence)(ports.CanonicalRecord,error)

type ResumeWorkResult struct{
 Checkpoint sqlite.ResumeCheckpointV1
 Cycle admission.Cycle
 Fence admission.Fence
 Publication ports.CanonicalRecord
 Continued bool
}

// ResumeRunV1 is the shared crash-recovery entrypoint. It returns without
// dispatch for idle (caller may then select a fresh cycle) or halted runs.
// In-flight and authorized retry use the exact stored clock coordinate,
// journal and semantic cut; the provided registered terminal handler is
// responsible for its existing strict scope/authorization checks.
func (c *Coordinator) ResumeRunV1(ctx context.Context,ledger *inputledger.Ledger,
 run,snapshot [32]byte,handler ResumeWorkHandler)(ResumeWorkResult,error){
 var result ResumeWorkResult
 if c==nil||c.bundle==nil||ledger==nil||c.store==nil||!ledger.SharesFenceAuthority(c.store){
  return result,errors.New("recovery must share admission and commit authority")
 }
 _,_,_,_,_,ledgerRun:=ledger.Snapshot()
 if [32]byte(ledgerRun)!=run{return result,errors.New("resume ledger belongs to another run")}
 s,ok:=c.store.(*sqlite.Store)
 if !ok{return result,errors.New("resume requires a verifiable SQLite authority")}
 probe,err:=s.VerifySnapshotV1(ctx,run,snapshot,c.bundle)
 if err!=nil{return result,err}
 // Perception-only recovery at IDLE is a complete operation: no WORK
 // terminal exists to dispatch, so requiring a handler would deadlock an
 // otherwise valid outbox drain. Frozen WORK/retry states still require one
 // before any mutation, even when perception must be drained first.
 if handler==nil&&(probe.ControlStatus==1||probe.ControlStatus==2){
  return result,errors.New("resume requires a registered terminal handler before any mutation")
 }
 state,err:=s.DrainPendingPerceptionV1(ctx,run,snapshot,c.bundle)
 if err!=nil{return result,err}
 result.Checkpoint=state
 switch state.Action.Kind{
 case sqlite.ResumeHalted,sqlite.ResumeIdle:
  return result,nil
 case sqlite.ResumeDrainPerception:
  return result,errors.New("causal perception not fully drained")
 case sqlite.ResumeReexecuteFencedAttempt:
  result.Cycle,result.Fence,result.Checkpoint,err=c.ReopenFencedCycle(ctx,run,snapshot)
 case sqlite.ResumeFenceAuthorizedRetry:
  result.Cycle,result.Fence,result.Checkpoint,err=c.ReopenAuthorizedRetry(ctx,ledger,run,snapshot)
 default:
  return result,fmt.Errorf("unsupported verified resume directive %q",state.Action.Kind)
 }
 if err!=nil{return result,err}
 if handler==nil{return result,errors.New("no registered typed terminal handler for recovered cycle")}
 // Terminal handlers verify the saved fence against admission authority and
 // reject wrong-cycle/stale revisions; there is no direct ledger writer here.
 publication,err:=handler(ctx,ledger,result.Cycle,result.Fence)
 if err!=nil{return result,err}
 if publication.Identity==([32]byte{})||len(publication.Canonical)==0{
  return result,errors.New("registered handler did not publish canonical terminal record")
 }
 if err:=s.VerifyRecoveredTerminal(ctx,run,result.Checkpoint.Action.CycleID,
  [32]byte(publication.Identity),publication.Canonical,uint64(result.Cycle.BaseRevision),c.bundle);err!=nil{
  return result,fmt.Errorf("resume callback failed independent commit-journal audit: %w",err)
 }
 result.Publication=publication
 result.Continued=true
 return result,nil
}

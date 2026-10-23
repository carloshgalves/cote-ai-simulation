package commit

import (
 "bytes"
 "context"
 "errors"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/admission"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/inputledger"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
)

// ReopenFencedCycle returns the persisted, independently authenticated cycle
// and AdmissionFence after verifying a causal snapshot plus journal tail.
// No NextClock(), admission resampling, LLM, or wall clock is called. Its
// result can be given to the existing typed CommitWork/CommitCommunication
// handlers; those still enforce terminal CAS and idempotence themselves.
func (c *Coordinator) ReopenFencedCycle(ctx context.Context,run,snapshotID [32]byte)(
 admission.Cycle,admission.Fence,sqlite.ResumeCheckpointV1,error){
 var cycle admission.Cycle
 var fence admission.Fence
 var empty sqlite.ResumeCheckpointV1
 if c==nil||c.store==nil||c.bundle==nil{return cycle,fence,empty,errors.New("reopen requires coordinator and pinned policy")}
 authoritative,ok:=c.store.(*sqlite.Store)
 if !ok{return cycle,fence,empty,errors.New("reopen requires the same durable SQLite transaction authority")}
 resume,err:=authoritative.DrainPendingPerceptionV1(ctx,run,snapshotID,c.bundle)
 if err!=nil{return cycle,fence,empty,err}
 if resume.Action.Kind!=sqlite.ResumeReexecuteFencedAttempt{
  return cycle,fence,resume,errors.New("no in-flight frozen fence authorized for commit")
 }
 id:=resume.Action.FenceID
 raw,err:=authoritative.GetFence(ctx,id);if err!=nil{return cycle,fence,resume,err}
 parsed,err:=codec.StrictDecodePayload(raw);if err!=nil{return cycle,fence,resume,err}
 fields,ok:=parsed.([]any)
 if !ok||len(fields)!=18{return cycle,fence,resume,errors.New("frozen fence schema invalid")}
 rawRun,runOK:=fields[0].([]byte);rawCycle,cycleOK:=fields[1].([]byte)
 if !runOK||!cycleOK||!bytes.Equal(rawRun,run[:])||!bytes.Equal(rawCycle,resume.Action.CycleID[:]){
  return cycle,fence,resume,errors.New("frozen fence has unexpected run/cycle")
 }
 attempt,ok:=fields[2].(uint64);if !ok||attempt!=resume.Action.AttemptOrdinal||attempt>uint64(^uint32(0)){
  return cycle,fence,resume,errors.New("frozen fence attempt invalid")
 }
 wanted,_,err:=identity.New(c.bundle).Sum("cote.csf.id.admission-fence",
  "cote.csf.schema.admission-fence-id-preimage",1,[]any{run[:],resume.Action.CycleID[:],attempt})
 if err!=nil{return cycle,fence,resume,err}
 if !bytes.Equal(wanted[:],id[:]){return cycle,fence,resume,errors.New("frozen fence identity was forged")}
 digest,_,err:=identity.New(c.bundle).Sum("cote.csf.digest.fence",
  "cote.csf.schema.admission-fence.body",1,fields[:17])
 if err!=nil{return cycle,fence,resume,err}
 publishedDigest,ok:=fields[17].([]byte)
 if !ok||!bytes.Equal(publishedDigest,digest[:]){
  return cycle,fence,resume,errors.New("frozen admission digest diverged from persisted journal")
 }
 instant,ok:=resumeSignedInstant(fields[4]);if !ok{return cycle,fence,resume,errors.New("frozen instant invalid")}
 ordinal,ok:=fields[5].(uint64);if !ok{return cycle,fence,resume,errors.New("frozen ordinal invalid")}
 revision,ok:=fields[7].(uint64);if !ok{return cycle,fence,resume,errors.New("frozen revision invalid")}
 stateHash,ok:=fields[8].([]byte);if !ok||len(stateHash)!=32{return cycle,fence,resume,errors.New("frozen state hash malformed")}
 inputHash,ok:=fields[12].([]byte);if !ok||len(inputHash)!=32{return cycle,fence,resume,errors.New("frozen input hash malformed")}
 var base,source domain.Digest
 copy(base[:],stateHash)
 copy(source[:],inputHash)
 plan,ok:=fields[6].([]any);if !ok||len(plan)==0{return cycle,fence,resume,errors.New("frozen plan malformed")}
 if len(plan)==1&&plan[0]==uint64(0){plan=nil}
 cycle=admission.Cycle{ID:domain.ID(resume.Action.CycleID),Attempt:uint32(attempt),
  Plan:plan,Instant:domain.SimulationInstant(instant),Ordinal:domain.CycleOrdinal(ordinal),
  BaseRevision:domain.WorldRevision(revision),BaseStateHash:base,RetryRef:fields[3]}
 fence=admission.Fence{ID:domain.ID(id),Digest:domain.Digest(digest),
  Fields:fields,Canonical:bytes.Clone(raw),InputDigest:source}
 return cycle,fence,resume,nil
}

func resumeSignedInstant(v any)(int64,bool){
 switch value:=v.(type){
 case int64:return value,true
 case uint64:
  if value<=uint64(^uint64(0)>>1){return int64(value),true}
 }
 return 0,false
}

// ReopenAuthorizedRetry admits only the next explicit AttemptRetryRecord
// successor. Its previous fence fixes the semantic cut, source cohort,
// clock plan, base revision and world hash. Admission's existing durable
// retry guard rejects any changed input digest or fabricated retry ref.
func (c *Coordinator) ReopenAuthorizedRetry(ctx context.Context,ledger *inputledger.Ledger,
 run,snapshotID [32]byte)(admission.Cycle,admission.Fence,sqlite.ResumeCheckpointV1,error){
 var cycle admission.Cycle
 var fence admission.Fence
 var empty sqlite.ResumeCheckpointV1
 if c==nil||c.bundle==nil||ledger==nil{return cycle,fence,empty,errors.New("retry requires coordinator, ledger and policy")}
 s,ok:=c.store.(*sqlite.Store)
 if !ok{return cycle,fence,empty,errors.New("retry requires the committed SQLite adapter")}
 if !ledger.SharesFenceAuthority(c.store){return cycle,fence,empty,errors.New("retry source ledger is not the persisted admission authority")}
 restored,err:=s.DrainPendingPerceptionV1(ctx,run,snapshotID,c.bundle)
 if err!=nil{return cycle,fence,restored,err}
 if restored.Action.Kind!=sqlite.ResumeFenceAuthorizedRetry{
  return cycle,fence,restored,errors.New("run has no authorized frozen retry")
 }
 if restored.Action.CycleID==([32]byte{})||restored.Action.FenceID==([32]byte{})||
  restored.Action.RetryID==([32]byte{}){
  return cycle,fence,restored,errors.New("retry lost journal identities")
 }
 control,err:=s.LoadCycleControl(ctx,run);if err!=nil{return cycle,fence,restored,err}
 if len(control)!=7||control[0]!=uint64(2){return cycle,fence,restored,errors.New("retry control changed before admission")}
 retried,ok:=control[4].([]any)
 if !ok||len(retried)!=2||retried[0]!=uint64(1){return cycle,fence,restored,errors.New("retry reference absent")}
 ref,ok:=retried[1].([]any)
 if !ok||len(ref)!=3||ref[0]!=uint64(14){return cycle,fence,restored,errors.New("retry reference malformed")}
 refID,ok:=ref[1].([]byte)
 if !ok||!bytes.Equal(refID,restored.Action.RetryID[:]){return cycle,fence,restored,errors.New("retry ref inconsistent with verified journal")}
 prior,err:=s.GetFence(ctx,restored.Action.FenceID);if err!=nil{return cycle,fence,restored,err}
 v,err:=codec.StrictDecodePayload(prior);if err!=nil{return cycle,fence,restored,err}
 fields,ok:=v.([]any);if !ok||len(fields)!=18{return cycle,fence,restored,errors.New("previous frozen fence malformed")}
 runRaw,runOK:=fields[0].([]byte);cycleRaw,cycOK:=fields[1].([]byte)
 if !runOK||!cycOK||!bytes.Equal(runRaw,run[:])||!bytes.Equal(cycleRaw,restored.Action.CycleID[:]){
  return cycle,fence,restored,errors.New("retry crosses cycle or run")
 }
 previousAttempt,ok:=fields[2].(uint64)
 if !ok{return cycle,fence,restored,errors.New("previous retry attempt malformed")}
 nextAttempt,err:=verifiedRetrySuccessor(previousAttempt,restored.Action.AttemptOrdinal)
 if err!=nil{return cycle,fence,restored,err}
 instant,ok:=resumeSignedInstant(fields[4]);if !ok{return cycle,fence,restored,errors.New("prior instant invalid")}
 ordinal,ok:=fields[5].(uint64);if !ok{return cycle,fence,restored,errors.New("prior ordinal invalid")}
 plan,ok:=fields[6].([]any);if !ok||len(plan)==0{return cycle,fence,restored,errors.New("prior clock plan invalid")}
 if len(plan)==1&&plan[0]==uint64(0){plan=nil}
 revision,ok:=fields[7].(uint64);if !ok{return cycle,fence,restored,errors.New("prior base revision invalid")}
 hash,ok:=fields[8].([]byte);if !ok||len(hash)!=32{return cycle,fence,restored,errors.New("prior state hash invalid")}
 var stateHash domain.Digest;copy(stateHash[:],hash)
 cycle=admission.Cycle{ID:domain.ID(restored.Action.CycleID),Attempt:nextAttempt,
  Plan:plan,Instant:domain.SimulationInstant(instant),Ordinal:domain.CycleOrdinal(ordinal),
  BaseRevision:domain.WorldRevision(revision),BaseStateHash:stateHash,RetryRef:retried}
 fence,err=admission.NewCoordinator(admission.NewFenceLog(c.store)).
  AdmitThenEvaluate(ctx,c.bundle,ledger,cycle,c.admissionPolicies,nil)
 if err!=nil{return cycle,admission.Fence{},restored,err}
 if fence.ID==domain.ID(restored.Action.FenceID){return cycle,admission.Fence{},restored,errors.New("retry reused prior attempt identity")}
 return cycle,fence,restored,nil
}

// verifiedRetrySuccessor forbids uint64 wraparound and uint32 truncation
// *before* a retry creates a new fence under an apparently reused ordinal.
func verifiedRetrySuccessor(previous,proposed uint64)(uint32,error){
 if previous>=uint64(^uint32(0)) || proposed>uint64(^uint32(0)) ||
  proposed!=previous+1 {
  return 0,errors.New("retry attempt exceeds V1 ordinal limit or is not the immediate successor")
 }
 return uint32(proposed),nil
}

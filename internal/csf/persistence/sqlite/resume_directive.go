package sqlite

import (
 "bytes"
 "errors"
 "fmt"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
)

// ResumeActionKind is deliberately a planner output rather than an implicit
// mutating restart. The executor must re-use the already persisted cycle,
// fence, retry and cohort; it must not select the coordinate again.
type ResumeActionKind string

const (
 ResumeIdle ResumeActionKind = "IDLE_SELECT_NEXT"
 ResumeDrainPerception ResumeActionKind = "DRAIN_PRIOR_PERCEPTION"
 ResumeReexecuteFencedAttempt ResumeActionKind = "REEXECUTE_FROZEN_FENCE"
 ResumeFenceAuthorizedRetry ResumeActionKind = "FENCE_AUTHORIZED_RETRY"
 ResumeHalted ResumeActionKind = "HALTED_ON_ABORT"
)

type ResumeAction struct{
 Kind ResumeActionKind
 CycleID [32]byte
 AttemptOrdinal uint64
 FenceID [32]byte
 RetryID [32]byte
 // Do not permit the executor to accidentally recalculate the frozen
 // CLOCK_ADVANCE destination or cohort from current, possibly later inputs.
 FrozenCyclePlan []byte
 FrozenAdmissionCut []byte
}

func resumeOptionalID(raw any)([32]byte,bool,error){
 var out [32]byte
 f,ok:=raw.([]any);if !ok||(len(f)!=1&&len(f)!=2){return out,false,errors.New("resume optional has invalid arity")}
 v,ok:=f[0].(uint64);if !ok{return out,false,errors.New("resume optional tag invalid")}
 if len(f)==1&&v==0{return out,false,nil}
 if len(f)!=2||v!=1{return out,false,errors.New("resume optional variant invalid")}
 id,ok:=f[1].([]byte);if !ok||len(id)!=32{return out,false,errors.New("resume optional ID invalid")}
 copy(out[:],id)
 return out,true,nil
}

func resumeOptionalRef(raw any,kind uint64)([32]byte,bool,error){
 var out [32]byte
 f,ok:=raw.([]any);if !ok||(len(f)!=1&&len(f)!=2){return out,false,errors.New("resume reference option malformed")}
 if len(f)==1&&f[0]==uint64(0){return out,false,nil}
 if len(f)!=2||f[0]!=uint64(1){return out,false,errors.New("resume reference variant invalid")}
 ref,ok:=f[1].([]any);if !ok||len(ref)!=3||ref[0]!=kind{return out,false,fmt.Errorf("resume requires record kind %d",kind)}
 id,ok:=ref[1].([]byte);digest,good:=ref[2].([]byte)
 if !ok||!good||len(id)!=32||len(digest)!=32{return out,false,errors.New("resume reference identity/digest malformed")}
 copy(out[:],id)
 return out,true,nil
}

func resumeActionForControl(ctl []any,taskCount int,fetchFence func([32]byte)([]any,error))(ResumeAction,error){
 var zero ResumeAction
 if len(ctl)!=7||taskCount<0{return zero,errors.New("resume control incomplete")}
 status,ok:=ctl[0].(uint64);if !ok||status>3{return zero,errors.New("resume status unknown")}
 next,ok:=ctl[6].(uint64);if !ok||next>uint64(^uint32(0)){return zero,errors.New("resume attempt ordinal invalid")}
 cycle,hasCycle,err:=resumeOptionalID(ctl[1]);if err!=nil{return zero,err}
 attemptPart,ok:=ctl[2].([]any);if !ok||(len(attemptPart)!=1&&len(attemptPart)!=2){return zero,errors.New("resume attempt option malformed")}
 var attempt uint64
 hasAttempt:=false
 if len(attemptPart)==1{
  if attemptPart[0]!=uint64(0){return zero,errors.New("resume attempt empty tag invalid")}
 }else{
  v,tagOK:=attemptPart[0].(uint64)
  n,numOK:=attemptPart[1].(uint64)
  if !tagOK||v!=1||!numOK||n>uint64(^uint32(0)){return zero,errors.New("resume attempt invalid")}
  attempt=n;hasAttempt=true
 }
 fence,hasFence,err:=resumeOptionalRef(ctl[3],11);if err!=nil{return zero,err}
 retry,hasRetry,err:=resumeOptionalRef(ctl[4],14);if err!=nil{return zero,err}
 _,hasAbort,err:=resumeOptionalRef(ctl[5],13);if err!=nil{return zero,err}
 action:=ResumeAction{CycleID:cycle,AttemptOrdinal:attempt,FenceID:fence,RetryID:retry}
 switch status{
 case 0:
  if hasCycle||hasAttempt||hasFence||hasRetry||hasAbort||next!=0{return zero,errors.New("resume idle contradicts journal")}
  if taskCount>0{action.Kind=ResumeDrainPerception}else{action.Kind=ResumeIdle}
  return action,nil
 case 1:
  if !hasCycle||!hasAttempt||!hasFence||next!=attempt||fetchFence==nil{return zero,errors.New("resume in-flight lacks frozen fence")}
  record,err:=fetchFence(fence);if err!=nil{return zero,err}
  if len(record)!=18 {return zero,errors.New("resume fence malformed")}
  if !equalBytes(record[1],cycle[:])||record[2]!=attempt{return zero,errors.New("resume fence cycle or attempt mismatch")}
  plan,err:=canonicalCopy(record[6]);if err!=nil{return zero,err}
  cut,err:=canonicalCopy(record[9:13]);if err!=nil{return zero,err}
  action.FrozenCyclePlan=plan;action.FrozenAdmissionCut=cut
  if taskCount>0{action.Kind=ResumeDrainPerception}else{action.Kind=ResumeReexecuteFencedAttempt}
  return action,nil
 case 2:
  if !hasCycle||!hasAttempt||!hasFence||!hasRetry||!hasAbort||next!=attempt+1 {
   return zero,errors.New("authorized retry has incomplete journal refs")
  }
  action.AttemptOrdinal=next
  if taskCount>0{action.Kind=ResumeDrainPerception}else{action.Kind=ResumeFenceAuthorizedRetry}
  return action,nil
 case 3:
  if !hasCycle||!hasAttempt||!hasFence||hasRetry||!hasAbort||next!=attempt+1 {
   return zero,errors.New("halted control missing abort refs")
  }
  action.Kind=ResumeHalted
  return action,nil
 }
 return zero,errors.New("invalid resume state")
}

// Full CBOR payload is copied before returning: no pointer into a SQL-owned
// buffer or map reinterprets this journal plan on a later worker attempt.
func canonicalCopy(v any)([]byte,error){
 raw,err:=codec.CanonicalPayload(v)
 if err!=nil{return nil,err}
 return bytes.Clone(raw),nil
}

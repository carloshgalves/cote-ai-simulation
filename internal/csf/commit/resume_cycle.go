package commit

import (
 "bytes"
 "context"
 "errors"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/admission"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
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
 if !ok||len(fields)!=18||!bytes.Equal(fields[0].([]byte),run[:])||
  !bytes.Equal(fields[1].([]byte),resume.Action.CycleID[:]){
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
 if !bytes.Equal(fields[17].([]byte),digest[:]){
  return cycle,fence,resume,errors.New("frozen admission digest diverged from persisted journal")
 }
 instant,ok:=asSigned64(fields[4]);if !ok{return cycle,fence,resume,errors.New("frozen instant invalid")}
 ordinal,ok:=fields[5].(uint64);if !ok{return cycle,fence,resume,errors.New("frozen ordinal invalid")}
 revision,ok:=fields[7].(uint64);if !ok{return cycle,fence,resume,errors.New("frozen revision invalid")}
 stateHash,ok:=fields[8].([]byte);if !ok||len(stateHash)!=32{return cycle,fence,resume,errors.New("frozen state hash malformed")}
 inputHash,ok:=fields[13].([]byte);if !ok||len(inputHash)!=32{return cycle,fence,resume,errors.New("frozen input hash malformed")}
 var base,source domain.Digest
 copy(base[:],stateHash)
 copy(source[:],inputHash)
 plan,ok:=fields[6].([]any);if !ok||len(plan)==0{return cycle,fence,resume,errors.New("frozen plan malformed")}
 cycle=admission.Cycle{ID:domain.ID(resume.Action.CycleID),Attempt:uint32(attempt),
  Plan:plan,Instant:domain.SimulationInstant(instant),Ordinal:domain.CycleOrdinal(ordinal),
  BaseRevision:domain.WorldRevision(revision),BaseStateHash:base,RetryRef:fields[3]}
 fence=admission.Fence{ID:domain.ID(id),Digest:domain.Digest(digest),
  Fields:fields,Canonical:bytes.Clone(raw),InputDigest:source}
 return cycle,fence,resume,nil
}

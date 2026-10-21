package commit

import (
 "context"
 "errors"
 "sort"
 "bytes"
 "fmt"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/admission"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/clock"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/decision"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
)

// Abort is authoritative only after CommitCycleAbort confirms the persisted
// fence and publishes the abort envelope/control state in one SQLite tx.
// No provisional disposition is translated into a terminal DecisionRecord.
func (c *Coordinator) Abort(ctx context.Context,runID,cycleID domain.ID,fence admission.Fence,
 reason uint64,unknown []decision.Indeterminate,candidates ...decision.Candidate)(ports.CanonicalRecord,error){
 if c==nil||c.bundle==nil||c.store==nil{return ports.CanonicalRecord{},errors.New("terminal coordinator not initialized")}
 if len(fence.Fields)!=18||reason>6{return ports.CanonicalRecord{},errors.New("invalid abort request")}
 if reason==0&&len(unknown)==0{return ports.CanonicalRecord{},errors.New("missing indeterminate subjects")}
 attempt,ok:=fence.Fields[2].(uint64)
 if !ok||attempt>uint64(^uint32(0)){return ports.CanonicalRecord{},errors.New("invalid abort attempt")}
 state,err:=c.store.LoadClock(ctx,[32]byte(runID));if err!=nil{return ports.CanonicalRecord{},err}
 expectedClock,err:=clock.Encode(state);if err!=nil{return ports.CanonicalRecord{},err}
 sorted:=append([]decision.Indeterminate(nil),unknown...)
 sort.Slice(sorted,func(i,j int)bool{
  if sorted[i].SubjectKind!=sorted[j].SubjectKind{return sorted[i].SubjectKind<sorted[j].SubjectKind}
  return bytes.Compare(sorted[i].Subject[:],sorted[j].Subject[:])<0
 })
 ind:=make([]any,0,len(sorted))
 var last domain.ID
 for i,u:=range sorted{
  if i>0&&u.SubjectKind==sorted[i-1].SubjectKind&&u.Subject==last{return ports.CanonicalRecord{},errors.New("duplicate validation subject")}
  last=u.Subject
  if len(u.Validator)!=3{return ports.CanonicalRecord{},errors.New("unversioned validator")}
  ind=append(ind,[]any{[]any{uint64(u.SubjectKind),u.Subject[:],u.SubjectDigest[:]},
   uint64(u.Facet),u.Validator[0],u.Validator[1],u.Validator[2],u.Reason,u.Evidence,u.Validator})
 }
 fields:=[]any{runID[:],cycleID[:],attempt,
  fence.Fields[4],fence.Fields[5],fence.Fields[6],fence.Fields[7],fence.Fields[8],
  fence.Digest[:],fence.Fields[12],reason,ind,[]any{},fence.Fields[16]}
 d:=identity.New(c.bundle)
 id,_,err:=d.Sum("cote.csf.id.cycle-abort","cote.csf.schema.cycle-abort-id-preimage",1,
  []any{runID[:],cycleID[:],attempt});if err!=nil{return ports.CanonicalRecord{},err}
 dg,_,err:=d.Sum("cote.csf.digest.abort","cote.csf.schema.cycle-abort.body",1,fields)
 if err!=nil{return ports.CanonicalRecord{},err}
 canonical,err:=codec.CanonicalPayload(append(cloneAny(fields),dg[:]));if err!=nil{return ports.CanonicalRecord{},err}
 record:=ports.CanonicalRecord{Identity:[32]byte(id),Canonical:canonical}
 proof:=make([]ports.CanonicalRecord,0,len(candidates))
 for _,candidate:=range candidates{
  proof=append(proof,ports.CanonicalRecord{Identity:[32]byte(candidate.ID),Canonical:candidate.Canonical})
 }
 sort.Slice(proof,func(i,j int)bool{return bytes.Compare(proof[i].Identity[:],proof[j].Identity[:])<0})
 err=c.store.CommitCycleAbort(ctx,ports.TerminalAbortBatch{
  RunID:[32]byte(runID),CycleID:[32]byte(cycleID),FenceID:[32]byte(fence.ID),
  ExpectedClock:expectedClock,Envelope:record,CandidateEvidence:proof,FailAfterStep:c.failAfterStep})
 return record,err
}

// Retry does not execute the cycle. It durably authorizes precisely attempt+1.
// Only a subsequent AdmissionFence with an authenticated retry reference may
// start a new attempt; calling Retry twice is a hard error, never a second pass.
func isExactBytes(v any,want []byte)bool{
 actual,ok:=v.([]byte);return ok&&bytes.Equal(actual,want)
}
func (c *Coordinator) Retry(ctx context.Context,runID,cycleID domain.ID,abort ports.CanonicalRecord,
 reason string)(ports.CanonicalRecord,error){
 if c==nil||c.bundle==nil||c.store==nil{return ports.CanonicalRecord{},errors.New("terminal coordinator not initialized")}
 if domain.ValidateMachineID(reason)!=nil{return ports.CanonicalRecord{},errors.New("invalid retry reason")}
 parsed,err:=codec.StrictDecodePayload(abort.Canonical);if err!=nil{return ports.CanonicalRecord{},err}
 fields,ok:=parsed.([]any)
 if !ok||len(fields)!=15||!isExactBytes(fields[0],runID[:])||!isExactBytes(fields[1],cycleID[:]){
  return ports.CanonicalRecord{},errors.New("invalid canonical abort for retry")
 }
 from,ok:=fields[2].(uint64);if !ok||from>=uint64(^uint32(0)){return ports.CanonicalRecord{},errors.New("invalid abort ordinal")}
 d:=identity.New(c.bundle)
 digest,_,err:=d.Sum("cote.csf.digest.abort","cote.csf.schema.cycle-abort.body",1,fields[:14]);if err!=nil{return ports.CanonicalRecord{},err}
 if !isExactBytes(fields[14],digest[:]){return ports.CanonicalRecord{},errors.New("invalid abort digest")}
 id,_,err:=d.Sum("cote.csf.id.attempt-retry","cote.csf.schema.attempt-retry-id-preimage",1,
  []any{runID[:],cycleID[:],from,from+1});if err!=nil{return ports.CanonicalRecord{},err}
 retry:=[]any{runID[:],cycleID[:],from,from+1,
  []any{uint64(13),abort.Identity[:],digest[:]},reason,fields[13]}
 canonical,err:=codec.CanonicalPayload(retry);if err!=nil{return ports.CanonicalRecord{},err}
 state,err:=c.store.LoadClock(ctx,[32]byte(runID));if err!=nil{return ports.CanonicalRecord{},err}
 expectedClock,err:=clock.Encode(state);if err!=nil{return ports.CanonicalRecord{},err}
 record:=ports.CanonicalRecord{Identity:[32]byte(id),Canonical:canonical}
 if err:=c.store.AuthorizeAttemptRetry(ctx,ports.AttemptRetryBatch{
  RunID:[32]byte(runID),CycleID:[32]byte(cycleID),AbortID:abort.Identity,
  ExpectedClock:expectedClock,Record:record,FailAfterStep:c.failAfterStep,
 });err!=nil{return record,fmt.Errorf("attempt retry not authorized: %w",err)}
 return record,nil
}

package commit

import (
 "bytes"
 "context"
 "errors"
 "math"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/admission"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/clock"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/communication"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/inputledger"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
)

// CommitCommunicationDue consumes exactly one pending delivery occurrence.
// A deterministic channel outcome creates one source lifecycle Event and a
// PerceptionTask in the existing atomic CommitWork boundary; the recipient
// never receives a direct send-time KnowledgeInput.
func (c *Coordinator) CommitCommunicationDue(ctx context.Context,ledger *inputledger.Ledger,
 cycle admission.Cycle,fence admission.Fence)(ports.CanonicalRecord,error){
 var zero ports.CanonicalRecord
 if c==nil||c.bundle==nil||c.store==nil||ledger==nil||!ledger.SharesFenceAuthority(c.store){
  return zero,errors.New("due communication requires shared authoritative ledger")
 }
 authority,ok:=c.store.(interface{
  LoadSystemUnit(context.Context,[32]byte,[32]byte)([]byte,uint8,error)
  LoadCommunicationForOccurrence(context.Context,[32]byte,[32]byte)(ports.CommunicationPublication,error)
 })
 if !ok{return zero,errors.New("communication persistence authority is unavailable")}
 view,prefix,err:=ledger.AdmissionView(ctx,c.bundle);if err!=nil{return zero,err}
 if err:=admission.VerifyFence(c.bundle,view,cycle,c.admissionPolicies,fence);err!=nil{return zero,err}
 saved,err:=c.store.GetFence(ctx,[32]byte(fence.ID))
 if err!=nil||!bytes.Equal(saved,fence.Canonical){return zero,errors.New("due communication fence is not persisted")}
 _,_,_,_,_,run:=view.Snapshot()
 state,err:=c.store.LoadClock(ctx,[32]byte(run));if err!=nil{return zero,err}
 if state.Revision!=cycle.BaseRevision||state.Instant!=cycle.Instant||
  state.LastOrdinal>=int64(cycle.Ordinal)||cycle.Ordinal>math.MaxInt64||
  uint64(state.Revision)==math.MaxUint64||uint64(state.NextLogicalSequence)==math.MaxUint64{
  return zero,errors.New("stale due communication cycle")
 }
 if abort,hit,err:=c.abortIfCascadeExceeded(ctx,run,cycle,fence);hit||err!=nil{return abort,err}
 typed,hash,err:=c.store.LoadWorldState(ctx,uint64(state.Revision))
 if err!=nil||hash!=cycle.BaseStateHash{return zero,errors.New("due communication WorldState differs from fence")}
 units,ok:=fence.Fields[11].([]any)
 if !ok||len(units)!=1{return zero,errors.New("due delivery requires full one-occurrence fence")}
 u,ok:=units[0].([]any)
 if !ok||len(u)!=5||u[1]!=uint64(2){return zero,errors.New("due delivery cannot silently skip another source")}
 idBytes,ok:=u[3].([]byte);if !ok||len(idBytes)!=32{return zero,errors.New("due source missing")}
 digestBytes,ok:=u[4].([]byte);if !ok||len(digestBytes)!=32{return zero,errors.New("due source digest missing")}
 var id domain.ID;copy(id[:],idBytes)
 var digest domain.Digest;copy(digest[:],digestBytes)
 sourceRaw,kind,err:=authority.LoadSystemUnit(ctx,[32]byte(run),[32]byte(id))
 if err!=nil||kind!=2{return zero,errors.New("fenced source is not live ScheduledOccurrence")}
 data,err:=codec.StrictDecodePayload(sourceRaw);if err!=nil{return zero,err}
 fields,ok:=data.([]any)
 if !ok||len(fields)!=15||fields[5]!="communication.delivery"||
  !bytes.Equal(fields[14].([]byte),digest[:]){
  return zero,errors.New("occurrence kind or canonical digest forged")
 }
 pub,err:=authority.LoadCommunicationForOccurrence(ctx,[32]byte(run),[32]byte(id))
 if err!=nil{return zero,err}
 plan:=communication.SendPlan{Run:run,Claim:pub.Claim,Transmission:pub.Transmission,Occurrence:pub.Occurrence}
 due,err:=communication.ValidateOccurrence(c.bundle,plan)
 if err!=nil||due>state.Instant{return zero,errors.New("delivery before registered semantic due")}
 outcome,err:=communication.PlanDueDelivery(c.bundle,plan.Transmission,plan.Claim,id,due,state.Instant)
 if err!=nil{return zero,err}
 e,err:=communication.NewDueEvent(c.bundle,communication.EventContext{
  Run:run,Cycle:cycle.ID,At:state.Instant,BaseRevision:state.Revision,
  Sequence:state.NextLogicalSequence},
  plan,outcome,communication.Origin{Kind:6,ID:id,Digest:digest})
 if err!=nil{return zero,err}
 policies,err:=policiesByRole(c.bundle);if err!=nil{return zero,err}
 d:=identity.New(c.bundle)
 candidateID,_,err:=d.Sum("cote.csf.id.commit-candidate","cote.csf.schema.candidate-id-preimage",1,
  []any{run[:],cycle.ID[:],[]any{id[:]},"communication.delivery",uint64(1),uint64(0),uint64(0)})
 if err!=nil{return zero,err}
 eventValues,err:=codec.StrictDecodePayload(e.Canonical);if err!=nil{return zero,err}
 ev:=eventValues.([]any)
 sourceRef:=[]any{uint64(2),id[:],digest[:]}
 candidateBody:=[]any{candidateID[:],run[:],cycle.ID[:],[]any{id[:]},
  "communication.delivery",uint64(1),uint64(0),uint64(0),
  []any{[]any{uint64(0),uint64(0),outcome.EventType,ev[9],[]any{}}},
  []any{},[]any{},[]any{},[]any{"source.lifecycle"},[]any{sourceRef},
  int64(0),policies["conflict-resolvers"]}
 candidateDigest,_,err:=d.Sum("cote.csf.digest.commit-candidate","cote.csf.schema.commit-candidate.body",1,candidateBody)
 if err!=nil{return zero,err}
 candidateBytes,err:=codec.CanonicalPayload(append(cloneAny(candidateBody),candidateDigest[:]));if err!=nil{return zero,err}
 candidate:=ports.CanonicalRecord{Identity:[32]byte(candidateID),Canonical:candidateBytes}
 decisionID,_,err:=d.Sum("cote.csf.id.decision","cote.csf.schema.decision-id-preimage",1,
  []any{cycle.ID[:],uint64(cycle.Attempt),id[:]});if err!=nil{return zero,err}
 decisionBody:=[]any{decisionID[:],run[:],cycle.ID[:],uint64(cycle.Attempt),sourceRef,
  uint64(0),[]any{uint64(1),candidateID[:]},outcome.EventType,[]any{},
  policies["validators"],policies["conflict-resolvers"],[]any{},
  []any{e.Identity[:]},[]any{uint64(0)},[]any{uint64(0)}}
 decisionDigest,_,err:=d.Sum("cote.csf.digest.decision-record","cote.csf.schema.decision-record.body",1,decisionBody)
 if err!=nil{return zero,err}
 decisionBytes,err:=codec.CanonicalPayload(append(cloneAny(decisionBody),decisionDigest[:]))
 if err!=nil{return zero,err}
 decision:=ports.CanonicalRecord{Identity:[32]byte(decisionID),Canonical:decisionBytes}
 dd,_,err:=d.Sum("cote.csf.digest.decision","cote.csf.schema.decision-pair-list",1,
  []any{[]any{id[:],decisionID[:],decisionDigest[:]}});if err!=nil{return zero,err}
 taskHash,_,err:=d.Sum("cote.csf.digest.perception-task-list","cote.csf.schema.perception-task-id-list",1,[]any{})
 if err!=nil{return zero,err}
 next:=state;next.Revision++;next.LastOrdinal=int64(cycle.Ordinal);next.NextLogicalSequence++
 oldClock,err:=clock.Encode(state);if err!=nil{return zero,err}
 nextClock,err:=clock.Encode(next);if err!=nil{return zero,err}
 revision,err:=codec.CanonicalPayload([]any{uint64(state.Revision),typed,hash[:],
  uint64(next.Revision),typed,hash[:]});if err!=nil{return zero,err}
 journalBody:=[]any{run[:],cycle.ID[:],uint64(cycle.Attempt),int64(cycle.Instant),
  uint64(cycle.Ordinal),[]any{uint64(0)},uint64(cycle.BaseRevision),hash[:],
  uint64(next.Revision),hash[:],fence.Digest[:],[]any{id[:]},fence.InputDigest[:],
  policies["cycle-coordinate"],policies["event-order"],[]any{e.Identity[:]},
  []any{decisionID[:]},dd[:],[]any{},taskHash[:],
  policies["perception"],policies["perception-identity"],uint64(next.NextLogicalSequence)}
 hashBatch,_,err:=d.Sum("cote.csf.digest.batch","cote.csf.schema.batch-preimage",1,
  []any{journalBody,[]any{e.Canonical}});if err!=nil{return zero,err}
 commitID,_,err:=d.Sum("cote.csf.id.cycle-commit","cote.csf.schema.cycle-commit-id-preimage",1,
  []any{run[:],cycle.ID[:],uint64(cycle.Attempt)});if err!=nil{return zero,err}
 journalBytes,err:=codec.CanonicalPayload(append(cloneAny(journalBody),hashBatch[:]))
 if err!=nil{return zero,err}
 batch:=ports.WorkBatch{Mode:8,RunID:[32]byte(run),CycleID:[32]byte(cycle.ID),
  FenceID:[32]byte(fence.ID),ExpectedClock:oldClock,ResultClock:nextClock,
  ExpectedInputPrefix:prefix,BaseStateHash:[32]byte(hash),ResultStateHash:[32]byte(hash),
  RevisionCanonical:revision,Journal:ports.CanonicalRecord{Identity:[32]byte(commitID),Canonical:journalBytes},
  Events:[]ports.CanonicalRecord{e},Decisions:[]ports.CanonicalRecord{decision},
  Communication:&pub,CommunicationBundle:c.bundle,CommunicationStatus:outcome.NextStatus,
  CommunicationCandidate:candidate,FailAfterStep:c.failAfterStep}
 if err:=c.commitPreparedWork(ctx,&batch);err!=nil{return zero,err}
 return batch.Journal,nil
}

package commit

import (
 "bytes"
 "context"
 "errors"
 "math"
 "sort"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/admission"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/clock"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/decision"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/inputledger"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/settlement"
)

type systemOutput struct{
 kind uint64
 source domain.ID
 digest domain.Digest
 fields []any
 disposition uint64
 reason,producer string
 due int64
 recurrence any
 event ports.CanonicalRecord
}

// commitSystemBatch publishes multiple simultaneous schedule/trigger sources
// as ONE atomic WORK. The admission cut cannot be split across cycles.
func (c *Coordinator) commitSystemBatch(ctx context.Context,ledger *inputledger.Ledger,
 cycle admission.Cycle,fence admission.Fence)(ports.CanonicalRecord,error){
 var empty ports.CanonicalRecord
 loader,ok:=c.store.(interface{LoadSystemUnit(context.Context,[32]byte,[32]byte)([]byte,uint8,error)})
 if !ok{return empty,errors.New("system batch requires authenticated loader")}
 view,prefix,err:=ledger.AdmissionView(ctx,c.bundle);if err!=nil{return empty,err}
 if err:=admission.VerifyFence(c.bundle,view,cycle,c.admissionPolicies,fence);err!=nil{return empty,err}
 persisted,err:=c.store.GetFence(ctx,[32]byte(fence.ID))
 if err!=nil||!bytes.Equal(persisted,fence.Canonical){return empty,errors.New("system batch fence not durable")}
 _,_,_,_,_,run:=view.Snapshot()
 state,err:=c.store.LoadClock(ctx,[32]byte(run));if err!=nil{return empty,err}
 if state.Instant!=cycle.Instant||state.Revision!=cycle.BaseRevision||
  cycle.Ordinal>math.MaxInt64||state.LastOrdinal>=int64(cycle.Ordinal)||
  uint64(state.Revision)==math.MaxUint64{return empty,errors.New("stale system batch coordinate")}
 if abort,hit,err:=c.abortIfCascadeExceeded(ctx,run,cycle,fence);hit||err!=nil{return abort,err}
 typed,hash,err:=c.store.LoadWorldState(ctx,uint64(state.Revision))
 if err!=nil||hash!=cycle.BaseStateHash{return empty,errors.New("system batch predecessor invalid")}
 entries,ok:=fence.Fields[11].([]any)
 if !ok||len(entries)<2||uint64(len(entries))>math.MaxUint64-uint64(state.NextLogicalSequence){
  return empty,errors.New("system batch count or sequence invalid")
 }
 policies,err:=policiesByRole(c.bundle);if err!=nil{return empty,err}
 d:=identity.New(c.bundle)
 prepared:=make([]systemOutput,len(entries))
 seen:=map[domain.ID]bool{}
 for i,rawEntry:=range entries{
  entry,ok:=rawEntry.([]any)
  if !ok||len(entry)!=5{return empty,errors.New("malformed system batch admitted unit")}
  kind,ok:=entry[1].(uint64)
  if !ok||(kind!=2&&kind!=3){return empty,errors.New("unregistered source in system-only WORK")}
  id,ok:=entry[3].([]byte)
  if !ok||len(id)!=32{return empty,errors.New("system source ID invalid")}
  dig,ok:=entry[4].([]byte)
  if !ok||len(dig)!=32{return empty,errors.New("system source digest invalid")}
  var source domain.ID;copy(source[:],id)
  var digest domain.Digest;copy(digest[:],dig)
  if seen[source]{return empty,errors.New("system source repeated")}
  seen[source]=true
  canonical,actualKind,err:=loader.LoadSystemUnit(ctx,[32]byte(run),[32]byte(source))
  if err!=nil||uint64(actualKind)!=kind{return empty,errors.New("system unit not in active source authority")}
  decoded,err:=codec.StrictDecodePayload(canonical);if err!=nil{return empty,err}
  fields,ok:=decoded.([]any)
  if !ok||len(fields)<12{return empty,errors.New("system canonical source field count invalid")}
  first,validID:=fields[0].([]byte)
  last,validDigest:=fields[len(fields)-1].([]byte)
  if !validID||!validDigest||!bytes.Equal(first,id)||
   !bytes.Equal(last,dig)||fields[2]!=entry[2]{
   return empty,errors.New("source canonical bytes differ from admitted unit")
  }
  item:=systemOutput{kind:kind,source:source,digest:digest,fields:fields}
  if kind==2{
   item.disposition,item.due,item.recurrence,err=settlement.OccurrenceOutcome(run,fields,state.Instant)
   if err!=nil{return empty,err}
   item.producer="schedule.lifecycle"
   if item.disposition==1{item.reason="schedule.rejected"}else{item.reason="schedule.deferred"}
  }else{
   definition,ok:=fields[3].([]byte)
   if !ok||len(definition)!=32{return empty,errors.New("trigger source definition invalid")}
   registered,err:=c.store.LoadRegisteredTriggers(ctx,[32]byte(run),c.bundle)
   if err!=nil{return empty,err}
   bound:=false
   for _,t:=range registered{
    if bytes.Equal(t.Definition.ID[:],definition)&&fields[4]==uint64(t.Definition.Version){bound=true;break}
   }
   if !bound{return empty,errors.New("trigger source lacks pinned handler")}
   item.disposition=0;item.reason="trigger.consumed";item.producer="trigger.lifecycle"
  }
  prepared[i]=item
 }
 order:=make([]int,len(prepared))
 for i:=range order{order[i]=i}
 sort.Slice(order,func(i,j int)bool{
  return bytes.Compare(prepared[order[i]].source[:],prepared[order[j]].source[:])<0
 })
 events:=make([]ports.CanonicalRecord,len(prepared))
 eventIDs:=make([]any,len(prepared))
 rawEvents:=make([]any,len(prepared))
 for i,idx:=range order{
  item:=&prepared[idx]
  event,err:=settlement.SystemLifecycleEvent(run,cycle.ID,item.source,item.digest,
   item.kind,item.disposition,state.Instant,state.Revision,
   state.NextLogicalSequence+domain.LogicalSequence(i))
  if err!=nil{return empty,err}
  item.event=event
  events[i]=event;eventIDs[i]=event.Identity[:];rawEvents[i]=event.Canonical
 }
 decisions:=make([]ports.CanonicalRecord,len(prepared))
 candidates:=make([]ports.CanonicalRecord,len(prepared))
 decisionIDs:=make([]any,len(prepared))
 pairs:=make([]any,len(prepared))
 admitted:=make([]any,len(prepared))
 successors:=make([]ports.CanonicalRecord,0)
 for i,item:=range prepared{
  sourceRef:=[]any{item.kind,item.source[:],item.digest[:]}
  successorRef:=[]any{uint64(0)}
  if item.kind==2&&item.disposition==2{
   successor,err:=settlement.OccurrenceSuccessor(run,item.fields,state.Instant,
    item.event,item.due,item.recurrence)
   if err!=nil{return empty,err}
   successors=append(successors,successor)
   successorRef=[]any{uint64(1),successor.Identity[:]}
  }
  candidate,err:=decision.MakeCandidate(d,run,cycle.ID,[]domain.ID{item.source},
   item.producer,1,0,0,[]any{},[]any{sourceRef},0,policies["conflict-resolvers"])
  if err!=nil{return empty,err}
  eventRaw,err:=codec.StrictDecodePayload(item.event.Canonical);if err!=nil{return empty,err}
  payload:=eventRaw.([]any)[9]
  candidate.Fields[8]=[]any{[]any{uint64(0),uint64(0),item.reason,payload,[]any{}}}
  candidate.Fields[12]=[]any{"source.lifecycle"}
  candidate.Digest,_,err=d.Sum("cote.csf.digest.commit-candidate",
   "cote.csf.schema.commit-candidate.body",1,candidate.Fields)
  if err!=nil{return empty,err}
  candidate.Canonical,err=codec.CanonicalPayload(append(cloneAny(candidate.Fields),candidate.Digest[:]))
  if err!=nil{return empty,err}
  candidates[i]=ports.CanonicalRecord{Identity:[32]byte(candidate.ID),Canonical:candidate.Canonical}
  decisionID,_,err:=d.Sum("cote.csf.id.decision","cote.csf.schema.decision-id-preimage",1,
   []any{cycle.ID[:],uint64(cycle.Attempt),item.source[:]});if err!=nil{return empty,err}
  body:=[]any{decisionID[:],run[:],cycle.ID[:],uint64(cycle.Attempt),sourceRef,item.disposition,
   []any{uint64(1),candidate.ID[:]},item.reason,[]any{},policies["validators"],
   policies["conflict-resolvers"],[]any{},[]any{item.event.Identity[:]},successorRef,[]any{uint64(0)}}
  recordHash,_,err:=d.Sum("cote.csf.digest.decision-record","cote.csf.schema.decision-record.body",1,body)
  if err!=nil{return empty,err}
  canonical,err:=codec.CanonicalPayload(append(cloneAny(body),recordHash[:]))
  if err!=nil{return empty,err}
  decisions[i]=ports.CanonicalRecord{Identity:[32]byte(decisionID),Canonical:canonical}
  decisionIDs[i]=decisionID[:]
  pairs[i]=[]any{item.source[:],decisionID[:],recordHash[:]}
  admitted[i]=item.source[:]
 }
 decisionHash,_,err:=d.Sum("cote.csf.digest.decision","cote.csf.schema.decision-pair-list",1,pairs)
 if err!=nil{return empty,err}
 taskHash,_,err:=d.Sum("cote.csf.digest.perception-task-list","cote.csf.schema.perception-task-id-list",1,[]any{})
 if err!=nil{return empty,err}
 next:=state;next.Revision++;next.LastOrdinal=int64(cycle.Ordinal)
 next.NextLogicalSequence+=domain.LogicalSequence(len(events))
 oldClock,err:=clock.Encode(state);if err!=nil{return empty,err}
 newClock,err:=clock.Encode(next);if err!=nil{return empty,err}
 revision,err:=codec.CanonicalPayload([]any{uint64(state.Revision),typed,hash[:],
  uint64(next.Revision),typed,hash[:]});if err!=nil{return empty,err}
 fields:=[]any{run[:],cycle.ID[:],uint64(cycle.Attempt),int64(state.Instant),
  uint64(cycle.Ordinal),[]any{uint64(0)},uint64(state.Revision),hash[:],
  uint64(next.Revision),hash[:],fence.Digest[:],admitted,fence.InputDigest[:],
  policies["cycle-coordinate"],policies["event-order"],eventIDs,decisionIDs,
  decisionHash[:],[]any{},taskHash[:],policies["perception"],policies["perception-identity"],
  uint64(next.NextLogicalSequence)}
 batchHash,_,err:=d.Sum("cote.csf.digest.batch","cote.csf.schema.batch-preimage",1,
  []any{fields,rawEvents});if err!=nil{return empty,err}
 commitID,_,err:=d.Sum("cote.csf.id.cycle-commit","cote.csf.schema.cycle-commit-id-preimage",1,
  []any{run[:],cycle.ID[:],uint64(cycle.Attempt)});if err!=nil{return empty,err}
 journalBytes,err:=codec.CanonicalPayload(append(cloneAny(fields),batchHash[:]))
 if err!=nil{return empty,err}
 batch:=ports.WorkBatch{Mode:3,RunID:[32]byte(run),CycleID:[32]byte(cycle.ID),
  FenceID:[32]byte(fence.ID),ExpectedClock:oldClock,ResultClock:newClock,
  ExpectedInputPrefix:prefix,Journal:ports.CanonicalRecord{Identity:[32]byte(commitID),Canonical:journalBytes},
  SystemCandidate:candidates[0],SystemExtraCandidates:candidates[1:],SystemSuccessors:successors,
  Decisions:decisions,Events:events,BaseStateHash:[32]byte(hash),ResultStateHash:[32]byte(hash),
  RevisionCanonical:revision,FailAfterStep:c.failAfterStep}
 if c.tamperWorkBatch!=nil{c.tamperWorkBatch(&batch)}
 if err:=c.store.CommitWork(ctx,batch);err!=nil{return empty,err}
 return batch.Journal,nil
}

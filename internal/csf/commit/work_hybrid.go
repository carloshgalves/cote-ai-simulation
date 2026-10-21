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
type mixedSlot struct{slot string;id domain.ID;digest domain.Digest}
type mixedSystem struct{
 kind uint64;source domain.ID;digest domain.Digest;fields []any
 disposition uint64;due int64;recurrence any;producer,reason string
 event ports.CanonicalRecord
}
type mixedEvent struct{origin domain.ID;round bool;system int;refs []any}

// commitMixedWork preserves the complete frozen fence: all NoProposal slots
// and all registered system sources settle inside ONE durable CycleCommit.
func (c *Coordinator) commitMixedWork(ctx context.Context,ledger *inputledger.Ledger,
 cycle admission.Cycle,fence admission.Fence)(ports.CanonicalRecord,error){
 var empty ports.CanonicalRecord
 loader,ok:=c.store.(interface{LoadSystemUnit(context.Context,[32]byte,[32]byte)([]byte,uint8,error)})
 if !ok{return empty,errors.New("mixed WORK requires authenticated system-unit store")}
 view,prefix,err:=ledger.AdmissionView(ctx,c.bundle);if err!=nil{return empty,err}
 if err:=admission.VerifyFence(c.bundle,view,cycle,c.admissionPolicies,fence);err!=nil{return empty,err}
 saved,err:=c.store.GetFence(ctx,[32]byte(fence.ID))
 if err!=nil||!bytes.Equal(saved,fence.Canonical){return empty,errors.New("mixed fence was not durably sealed")}
 _,_,_,rounds,slots,run:=view.Snapshot()
 state,err:=c.store.LoadClock(ctx,[32]byte(run));if err!=nil{return empty,err}
 if state.Instant!=cycle.Instant||state.Revision!=cycle.BaseRevision||
  cycle.Ordinal>math.MaxInt64||state.LastOrdinal>=int64(cycle.Ordinal)||
  uint64(state.Revision)==math.MaxUint64{return empty,errors.New("stale mixed cycle coordinate")}
 if abort,hit,err:=c.abortIfCascadeExceeded(ctx,run,cycle,fence);hit||err!=nil{return abort,err}
 typed,hash,err:=c.store.LoadWorldState(ctx,uint64(state.Revision))
 if err!=nil||hash!=cycle.BaseStateHash{return empty,errors.New("mixed predecessor differs from authoritative world")}
 units,ok:=fence.Fields[11].([]any)
 if !ok||len(units)<2{return empty,errors.New("mixed terminal needs two admitted units")}
 responseByID:=map[domain.ID]inputledger.Response{}
 for _,round:=range rounds{
  for _,response:=range slots[round.ID]{
   if _,seen:=responseByID[response.ID];seen{return empty,errors.New("duplicate slot response ID")}
   responseByID[response.ID]=response
  }
 }
 policies,err:=policiesByRole(c.bundle);if err!=nil{return empty,err}
 d:=identity.New(c.bundle)
 sys:=make([]mixedSystem,0)
 roundsByID:=map[domain.ID][]mixedSlot{}
 members:=map[domain.ID]bool{}
 slotCount,systemCount:=0,0
 for _,rawUnit:=range units{
  entry,ok:=rawUnit.([]any)
  if !ok||len(entry)!=5{return empty,errors.New("mixed fence unit malformed")}
  kind,ok:=entry[1].(uint64);if !ok{return empty,errors.New("mixed unit kind invalid")}
  rawID,ok:=entry[3].([]byte);if !ok||len(rawID)!=32{return empty,errors.New("mixed source identity missing")}
  rawDigest,ok:=entry[4].([]byte);if !ok||len(rawDigest)!=32{return empty,errors.New("mixed source digest missing")}
  var source domain.ID;copy(source[:],rawID)
  var digest domain.Digest;copy(digest[:],rawDigest)
  if members[source]{return empty,errors.New("duplicate mixed source in fence")}
  members[source]=true
  switch kind{
  case 0:
   response,found:=responseByID[source]
   if !found||response.Kind!=1||response.Digest!=digest{return empty,errors.New("mixed source not authenticated NoProposal")}
   roundsByID[response.RoundID]=append(roundsByID[response.RoundID],mixedSlot{
    slot:response.SlotID,id:source,digest:digest})
   slotCount++
  case 2,3:
   raw,actual,err:=loader.LoadSystemUnit(ctx,[32]byte(run),[32]byte(source))
   if err!=nil||uint64(actual)!=kind{return empty,errors.New("mixed system unit unauthenticated")}
   val,err:=codec.StrictDecodePayload(raw);if err!=nil{return empty,err}
   fields,ok:=val.([]any)
   if !ok||len(fields)<12{return empty,errors.New("mixed system source malformed")}
   a,ok:=fields[0].([]byte);if !ok||!bytes.Equal(a,rawID){return empty,errors.New("mixed source ID mismatch")}
   b,ok:=fields[len(fields)-1].([]byte);if !ok||!bytes.Equal(b,rawDigest)||fields[2]!=entry[2]{
    return empty,errors.New("mixed system source digest/source mismatch")
   }
   output:=mixedSystem{kind:kind,source:source,digest:digest,fields:fields}
   if kind==2{
    output.disposition,output.due,output.recurrence,err=settlement.OccurrenceOutcome(run,fields,state.Instant)
    if err!=nil{return empty,err}
    output.producer="schedule.lifecycle"
    if output.disposition==1{output.reason="schedule.rejected"}else{output.reason="schedule.deferred"}
   }else{
    definition,ok:=fields[3].([]byte)
    if !ok||len(definition)!=32{return empty,errors.New("mixed activation definition invalid")}
    registered,err:=c.store.LoadRegisteredTriggers(ctx,[32]byte(run),c.bundle)
    if err!=nil{return empty,err}
    found:=false
    for _,item:=range registered{
     if bytes.Equal(item.Definition.ID[:],definition)&&fields[4]==uint64(item.Definition.Version){found=true;break}
    }
    if !found{return empty,errors.New("mixed trigger not registered")}
    output.disposition=0;output.producer="trigger.lifecycle";output.reason="trigger.consumed"
   }
   sys=append(sys,output);systemCount++
  default:
   return empty,errors.New("mixed terminal refuses unsupported exogenous/response unit")
  }
 }
 if slotCount==0||systemCount==0{return empty,errors.New("mixed WORK requires both classes")}
 plans:=make([]mixedEvent,0,len(roundsByID)+len(sys))
 for round,parts:=range roundsByID{
  sort.Slice(parts,func(i,j int)bool{return parts[i].slot<parts[j].slot})
  refs:=make([]any,len(parts))
  for i,p:=range parts{
   if i>0&&p.slot==parts[i-1].slot{return empty,errors.New("duplicate mixed round slot")}
   refs[i]=[]any{uint64(0),uint64(1),p.id[:],p.digest[:]}
  }
  plans=append(plans,mixedEvent{origin:round,round:true,refs:refs})
 }
 for i,v:=range sys{plans=append(plans,mixedEvent{origin:v.source,system:i})}
 sort.Slice(plans,func(i,j int)bool{return bytes.Compare(plans[i].origin[:],plans[j].origin[:])<0})
 if uint64(len(plans))>math.MaxUint64-uint64(state.NextLogicalSequence){return empty,errors.New("mixed logical sequence overflow")}
 events:=make([]ports.CanonicalRecord,len(plans))
 eventIDs,canonicalEvents:=make([]any,len(plans)),make([]any,len(plans))
 for i,p:=range plans{
  if i>0&&p.origin==plans[i-1].origin{return empty,errors.New("mixed EventOrderKey collision")}
  var event ports.CanonicalRecord
  seq:=state.NextLogicalSequence+domain.LogicalSequence(i)
  if p.round{
   event,err=settlement.ConsumedRoundEvent(run,cycle.ID,p.origin,state.Instant,state.Revision,seq,p.refs)
  }else{
   v:=&sys[p.system]
   event,err=settlement.SystemLifecycleEvent(run,cycle.ID,v.source,v.digest,v.kind,
    v.disposition,state.Instant,state.Revision,seq)
   v.event=event
  }
  if err!=nil{return empty,err}
  events[i]=event;eventIDs[i]=event.Identity[:];canonicalEvents[i]=event.Canonical
 }
 sysByID:=map[domain.ID]mixedSystem{}
 for _,v:=range sys{sysByID[v.source]=v}
 decisions:=make([]ports.CanonicalRecord,len(units))
 allIDs,pairs,admitted:=make([]any,len(units)),make([]any,len(units)),make([]any,len(units))
 candidates:=make([]ports.CanonicalRecord,0,systemCount)
 successors:=make([]ports.CanonicalRecord,0)
 for i,item:=range units{
  entry:=item.([]any)
  kind:=entry[1].(uint64)
  sourceRaw:=entry[3].([]byte)
  var source domain.ID;copy(source[:],sourceRaw)
  digest:=entry[4].([]byte)
  id,_,err:=d.Sum("cote.csf.id.decision","cote.csf.schema.decision-id-preimage",1,
   []any{cycle.ID[:],uint64(cycle.Attempt),sourceRaw});if err!=nil{return empty,err}
  var fields []any
  if kind==0{
   fields=[]any{id[:],run[:],cycle.ID[:],uint64(cycle.Attempt),
    []any{uint64(0),uint64(1),sourceRaw,digest},uint64(3),[]any{uint64(0)},
    "slot.no-proposal",[]any{},policies["validators"],policies["conflict-resolvers"],
    []any{},[]any{},[]any{uint64(0)},[]any{uint64(0)}}
  }else{
   v:=sysByID[source]
   sourceRef:=[]any{kind,sourceRaw,digest}
   successorRef:=[]any{uint64(0)}
   if kind==2&&v.disposition==2{
    child,err:=settlement.OccurrenceSuccessor(run,v.fields,state.Instant,v.event,v.due,v.recurrence)
    if err!=nil{return empty,err}
    successors=append(successors,child);successorRef=[]any{uint64(1),child.Identity[:]}
   }
   candidate,err:=decision.MakeCandidate(d,run,cycle.ID,[]domain.ID{source},
    v.producer,1,0,0,[]any{},[]any{sourceRef},0,policies["conflict-resolvers"])
   if err!=nil{return empty,err}
   e,err:=codec.StrictDecodePayload(v.event.Canonical);if err!=nil{return empty,err}
   candidate.Fields[8]=[]any{[]any{uint64(0),uint64(0),v.reason,e.([]any)[9],[]any{}}}
   candidate.Fields[12]=[]any{"source.lifecycle"}
   candidate.Digest,_,err=d.Sum("cote.csf.digest.commit-candidate","cote.csf.schema.commit-candidate.body",1,candidate.Fields)
   if err!=nil{return empty,err}
   candidate.Canonical,err=codec.CanonicalPayload(append(cloneAny(candidate.Fields),candidate.Digest[:]))
   if err!=nil{return empty,err}
   candidates=append(candidates,ports.CanonicalRecord{Identity:[32]byte(candidate.ID),Canonical:candidate.Canonical})
   fields=[]any{id[:],run[:],cycle.ID[:],uint64(cycle.Attempt),sourceRef,
    v.disposition,[]any{uint64(1),candidate.ID[:]},v.reason,[]any{},policies["validators"],
    policies["conflict-resolvers"],[]any{},[]any{v.event.Identity[:]},successorRef,[]any{uint64(0)}}
  }
  digestID,_,err:=d.Sum("cote.csf.digest.decision-record","cote.csf.schema.decision-record.body",1,fields)
  if err!=nil{return empty,err}
  canonical,err:=codec.CanonicalPayload(append(cloneAny(fields),digestID[:]))
  if err!=nil{return empty,err}
  decisions[i]=ports.CanonicalRecord{Identity:[32]byte(id),Canonical:canonical}
  allIDs[i]=id[:];pairs[i]=[]any{sourceRaw,id[:],digestID[:]};admitted[i]=sourceRaw
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
 journalBody:=[]any{run[:],cycle.ID[:],uint64(cycle.Attempt),int64(state.Instant),
  uint64(cycle.Ordinal),[]any{uint64(0)},uint64(state.Revision),hash[:],
  uint64(next.Revision),hash[:],fence.Digest[:],admitted,fence.InputDigest[:],
  policies["cycle-coordinate"],policies["event-order"],eventIDs,allIDs,decisionHash[:],
  []any{},taskHash[:],policies["perception"],policies["perception-identity"],
  uint64(next.NextLogicalSequence)}
 hashBatch,_,err:=d.Sum("cote.csf.digest.batch","cote.csf.schema.batch-preimage",1,
  []any{journalBody,canonicalEvents});if err!=nil{return empty,err}
 commitID,_,err:=d.Sum("cote.csf.id.cycle-commit","cote.csf.schema.cycle-commit-id-preimage",1,
  []any{run[:],cycle.ID[:],uint64(cycle.Attempt)});if err!=nil{return empty,err}
 journalCanonical,err:=codec.CanonicalPayload(append(cloneAny(journalBody),hashBatch[:]))
 if err!=nil{return empty,err}
 batch:=ports.WorkBatch{Mode:4,RunID:[32]byte(run),CycleID:[32]byte(cycle.ID),FenceID:[32]byte(fence.ID),
  ExpectedClock:oldClock,ResultClock:newClock,ExpectedInputPrefix:prefix,
  Journal:ports.CanonicalRecord{Identity:[32]byte(commitID),Canonical:journalCanonical},
  Decisions:decisions,Events:events,SystemCandidate:candidates[0],
  SystemExtraCandidates:candidates[1:],SystemSuccessors:successors,BaseStateHash:[32]byte(hash),
  ResultStateHash:[32]byte(hash),RevisionCanonical:revision,FailAfterStep:c.failAfterStep}
 if c.tamperWorkBatch!=nil{c.tamperWorkBatch(&batch)}
 if err:=c.store.CommitWork(ctx,batch);err!=nil{return empty,err}
 return batch.Journal,nil
}

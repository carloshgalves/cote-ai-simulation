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

type resourceSystemPart struct{
 index int
 kind uint64
 source domain.ID
 digest domain.Digest
 fields []any
 status uint64
 due int64
 recurrence any
 producer,reason string
 event ports.CanonicalRecord
}
type resourceSystemLifecycle struct{
 origin domain.ID
 refs []any
 system int // -1 for consumed resource RoundDeclaration
}

// commitResourceSystem only combines one registered resource consume
// with complete co-admitted schedule/trigger sources. Candidate/reducer
// authority stays in CommitSingleResource and is independently checked
// under the same CommitWork CAS as each lifecycle transition.
func (c *Coordinator) commitResourceSystem(ctx context.Context,ledger *inputledger.Ledger,
 cycle admission.Cycle,fence admission.Fence,b ports.WorkBatch)(ports.CanonicalRecord,error){
 var empty ports.CanonicalRecord
 loader,ok:=c.store.(interface{LoadSystemUnit(context.Context,[32]byte,[32]byte)([]byte,uint8,error)})
 if !ok{return empty,errors.New("resource+system requires authenticated source loader")}
 entries,ok:=fence.Fields[11].([]any)
 if !ok||len(entries)<2{return empty,errors.New("resource+system requires co-admitted sources")}
 saved,err:=c.store.GetFence(ctx,[32]byte(fence.ID))
 if err!=nil||!bytes.Equal(saved,fence.Canonical){return empty,errors.New("resource+system fence not persisted")}
 before,err:=clock.Decode(b.ExpectedClock);if err!=nil{return empty,err}
 var run domain.ID;copy(run[:],b.RunID[:])
 policies,err:=policiesByRole(c.bundle);if err!=nil{return empty,err}
 d:=identity.New(c.bundle)
 slotCount:=len(b.Decisions)
 if (b.Mode!=1&&b.Mode!=2)||slotCount<1||slotCount>2||
  len(b.Events)!=slotCount+1||len(entries)<=slotCount{
  return empty,errors.New("resource+system base handler must settle 1 or 2 complete resource slots")
 }
 parts:=make([]resourceSystemPart,0,len(entries)-slotCount)
 for i:=0;i<slotCount;i++{
  unit,ok:=entries[i].([]any)
  if !ok||len(unit)!=5||unit[1]!=uint64(0){
   return empty,errors.New("resource+system source slot order malformed")
  }
 }
 lifecycles:=make([]resourceSystemLifecycle,0,len(entries))
 for i:=1;i<len(b.Events);i++{
  roundValue,err:=codec.StrictDecodePayload(b.Events[i].Canonical);if err!=nil{return empty,err}
  roundEvent,ok:=roundValue.([]any)
  if !ok||len(roundEvent)!=19{return empty,errors.New("resource round lifecycle Event malformed")}
  key,ok:=roundEvent[2].([]any)
  if !ok||len(key)!=6||key[0]!=uint64(2){return empty,errors.New("resource round EventOrderKey invalid")}
  roundIDBytes,ok:=key[1].([]byte)
  if !ok||len(roundIDBytes)!=32{return empty,errors.New("resource RoundDeclaration ID invalid")}
  var roundID domain.ID;copy(roundID[:],roundIDBytes)
  refs,ok:=roundEvent[11].([]any)
  if !ok||len(refs)!=1{return empty,errors.New("resource round must settle one slot")}
  lifecycles=append(lifecycles,resourceSystemLifecycle{origin:roundID,refs:refs,system:-1})
 }
 for index,entryRaw:=range entries{
  if index<slotCount{continue}
  entry,ok:=entryRaw.([]any)
  if !ok||len(entry)!=5{return empty,errors.New("malformed resource+system admitted unit")}
  kind,ok:=entry[1].(uint64)
  if !ok||(kind!=2&&kind!=3){return empty,errors.New("resource+system cannot omit a second slot or unknown source")}
  id,ok:=entry[3].([]byte)
  if !ok||len(id)!=32{return empty,errors.New("resource+system source ID missing")}
  dig,ok:=entry[4].([]byte)
  if !ok||len(dig)!=32{return empty,errors.New("resource+system source digest missing")}
  var source domain.ID;copy(source[:],id)
  var digest domain.Digest;copy(digest[:],dig)
  raw,actualKind,err:=loader.LoadSystemUnit(ctx,b.RunID,[32]byte(source))
  if err!=nil||uint64(actualKind)!=kind{return empty,errors.New("resource+system unit not authenticated")}
  value,err:=codec.StrictDecodePayload(raw);if err!=nil{return empty,err}
  fields,ok:=value.([]any)
  if !ok||len(fields)<12{return empty,errors.New("resource+system canonical source malformed")}
  first,ok:=fields[0].([]byte)
  tail,good:=fields[len(fields)-1].([]byte)
  if !ok||!good||!bytes.Equal(first,id)||!bytes.Equal(tail,dig)||fields[2]!=entry[2]{
   return empty,errors.New("resource+system source differs from sealed fence")
  }
  part:=resourceSystemPart{index:index,kind:kind,source:source,digest:digest,fields:fields}
  if kind==2{
   part.status,part.due,part.recurrence,err=settlement.OccurrenceOutcome(run,fields,before.Instant)
   if err!=nil{return empty,err}
   part.producer="schedule.lifecycle"
   if part.status==1{part.reason="schedule.rejected"}else{part.reason="schedule.deferred"}
  }else{
   definition,ok:=fields[3].([]byte)
   if !ok||len(definition)!=32{return empty,errors.New("resource+system trigger definition invalid")}
   registered,err:=c.store.LoadRegisteredTriggers(ctx,b.RunID,c.bundle)
   if err!=nil{return empty,err}
   found:=false
   for _,item:=range registered{
    if bytes.Equal(item.Definition.ID[:],definition)&&fields[4]==uint64(item.Definition.Version){found=true;break}
   }
   if !found{return empty,errors.New("resource+system trigger handler unregistered")}
   part.status=0;part.reason="trigger.consumed";part.producer="trigger.lifecycle"
  }
  lifecycles=append(lifecycles,resourceSystemLifecycle{origin:source,system:len(parts)})
  parts=append(parts,part)
 }
 if len(parts)==0||uint64(len(parts)+1+slotCount)>math.MaxUint64-uint64(before.NextLogicalSequence){
  return empty,errors.New("resource+system event sequence overflow or missing system unit")
 }
 sort.Slice(lifecycles,func(i,j int)bool{return bytes.Compare(lifecycles[i].origin[:],lifecycles[j].origin[:])<0})
 for i:=1;i<len(lifecycles);i++{
  if lifecycles[i].origin==lifecycles[i-1].origin{return empty,errors.New("resource+system lifecycle EventOrderKey identity collision")}
 }
 events:=make([]ports.CanonicalRecord,0,len(lifecycles)+1)
 events=append(events,b.Events[0]) // phase 1 resource effect precedes all phase 2 source lifecycle
 for i,item:=range lifecycles{
  seq:=before.NextLogicalSequence+domain.LogicalSequence(i+1)
  var event ports.CanonicalRecord
  if item.system<0{
   event,err=settlement.ConsumedRoundEvent(run,cycle.ID,item.origin,before.Instant,before.Revision,seq,item.refs)
  }else{
   part:=&parts[item.system]
   event,err=settlement.SystemLifecycleEvent(run,cycle.ID,part.source,part.digest,part.kind,
    part.status,before.Instant,before.Revision,seq)
   part.event=event
  }
  if err!=nil{return empty,err}
  events=append(events,event)
 }
 successors:=make([]ports.CanonicalRecord,0)
 candidates:=make([]ports.CanonicalRecord,len(parts))
 decisions:=make([]ports.CanonicalRecord,len(entries))
 copy(decisions,b.Decisions)
 for i,part:=range parts{
  sourceRef:=[]any{part.kind,part.source[:],part.digest[:]}
  successorRef:=[]any{uint64(0)}
  if part.kind==2&&part.status==2{
   child,err:=settlement.OccurrenceSuccessor(run,part.fields,before.Instant,part.event,part.due,part.recurrence)
   if err!=nil{return empty,err}
   successors=append(successors,child)
   successorRef=[]any{uint64(1),child.Identity[:]}
  }
  candidate,err:=decision.MakeCandidate(d,run,cycle.ID,[]domain.ID{part.source},
   part.producer,1,0,0,[]any{},[]any{sourceRef},0,policies["conflict-resolvers"])
  if err!=nil{return empty,err}
  parsed,err:=codec.StrictDecodePayload(part.event.Canonical);if err!=nil{return empty,err}
  candidate.Fields[8]=[]any{[]any{uint64(0),uint64(0),part.reason,parsed.([]any)[9],[]any{}}}
  candidate.Fields[12]=[]any{"source.lifecycle"}
  candidate.Digest,_,err=d.Sum("cote.csf.digest.commit-candidate","cote.csf.schema.commit-candidate.body",1,candidate.Fields)
  if err!=nil{return empty,err}
  candidate.Canonical,err=codec.CanonicalPayload(append(cloneAny(candidate.Fields),candidate.Digest[:]))
  if err!=nil{return empty,err}
  candidates[i]=ports.CanonicalRecord{Identity:[32]byte(candidate.ID),Canonical:candidate.Canonical}
  decisionID,_,err:=d.Sum("cote.csf.id.decision","cote.csf.schema.decision-id-preimage",1,
   []any{cycle.ID[:],uint64(cycle.Attempt),part.source[:]})
  if err!=nil{return empty,err}
  fields:=[]any{decisionID[:],run[:],cycle.ID[:],uint64(cycle.Attempt),sourceRef,
   part.status,[]any{uint64(1),candidate.ID[:]},part.reason,[]any{},policies["validators"],
   policies["conflict-resolvers"],[]any{},[]any{part.event.Identity[:]},successorRef,[]any{uint64(0)}}
  recordHash,_,err:=d.Sum("cote.csf.digest.decision-record","cote.csf.schema.decision-record.body",1,fields)
  if err!=nil{return empty,err}
  raw,err:=codec.CanonicalPayload(append(cloneAny(fields),recordHash[:]))
  if err!=nil{return empty,err}
  decisions[part.index]=ports.CanonicalRecord{Identity:[32]byte(decisionID),Canonical:raw}
 }
 ids,pairs,admitted:=make([]any,len(entries)),make([]any,len(entries)),make([]any,len(entries))
 for i,entryRaw:=range entries{
  entry:=entryRaw.([]any)
  source:=entry[3].([]byte)
  v,err:=codec.StrictDecodePayload(decisions[i].Canonical);if err!=nil{return empty,err}
  body:=v.([]any)
  ids[i]=decisions[i].Identity[:]
  pairs[i]=[]any{source,decisions[i].Identity[:],body[15]}
  admitted[i]=source
 }
 pairHash,_,err:=d.Sum("cote.csf.digest.decision","cote.csf.schema.decision-pair-list",1,pairs)
 if err!=nil{return empty,err}
 oldJournal,err:=codec.StrictDecodePayload(b.Journal.Canonical);if err!=nil{return empty,err}
 journal,ok:=oldJournal.([]any)
 if !ok||len(journal)!=24{return empty,errors.New("resource+system starter journal malformed")}
 eventIDs,rawEvents:=make([]any,len(events)),make([]any,len(events))
 for i,item:=range events{eventIDs[i]=item.Identity[:];rawEvents[i]=item.Canonical}
 journal[11]=admitted
 journal[15]=eventIDs
 journal[16]=ids
 journal[17]=pairHash[:]
 next:=before;next.Revision++;next.LastOrdinal=int64(cycle.Ordinal)
 next.NextLogicalSequence+=domain.LogicalSequence(len(events))
 newClock,err:=clock.Encode(next);if err!=nil{return empty,err}
 journal[22]=uint64(next.NextLogicalSequence)
 hash,_,err:=d.Sum("cote.csf.digest.batch","cote.csf.schema.batch-preimage",1,[]any{journal[:23],rawEvents})
 if err!=nil{return empty,err}
 journal[23]=hash[:]
 canonical,err:=codec.CanonicalPayload(journal);if err!=nil{return empty,err}
 if slotCount==1{b.Mode=5}else{b.Mode=6}
 b.ResultClock=newClock;b.Events=events;b.Decisions=decisions
 b.SystemCandidate=candidates[0];b.SystemExtraCandidates=candidates[1:];b.SystemSuccessors=successors
 b.Journal.Canonical=canonical
 if err:=c.commitPreparedWork(ctx,&b);err!=nil{return empty,err}
 return b.Journal,nil
}

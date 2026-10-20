package commit

import (
 "bytes"
 "context"
 "errors"
 "math"

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

// CommitSystemUnit closes exactly one due system source. Its closed synthetic
// V1 handlers have no gameplay WorldState side effects and emit only a
// SECRET source-lifecycle event. All publication is through CommitWork.
func (c *Coordinator) CommitSystemUnit(ctx context.Context,ledger *inputledger.Ledger,
 cycle admission.Cycle,fence admission.Fence)(ports.CanonicalRecord,error){
 var empty ports.CanonicalRecord
 if c==nil||c.store==nil||c.bundle==nil||ledger==nil||!ledger.SharesFenceAuthority(c.store){
  return empty,errors.New("system WORK requires shared durable authority")
 }
 loader,ok:=c.store.(interface{LoadSystemUnit(context.Context,[32]byte,[32]byte)([]byte,uint8,error)})
 if !ok{return empty,errors.New("store cannot authenticate system input")}
 view,prefix,err:=ledger.AdmissionView(ctx,c.bundle);if err!=nil{return empty,err}
 if err:=admission.VerifyFence(c.bundle,view,cycle,c.admissionPolicies,fence);err!=nil{return empty,err}
 stored,err:=c.store.GetFence(ctx,[32]byte(fence.ID))
 if err!=nil||!bytes.Equal(stored,fence.Canonical){return empty,errors.New("system fence not persisted")}
 _,_,_,_,_,run:=view.Snapshot()
 clockBefore,err:=c.store.LoadClock(ctx,[32]byte(run));if err!=nil{return empty,err}
 if clockBefore.Instant!=cycle.Instant||clockBefore.Revision!=cycle.BaseRevision||
  cycle.Ordinal>math.MaxInt64||clockBefore.LastOrdinal>=int64(cycle.Ordinal)||
  uint64(clockBefore.NextLogicalSequence)==math.MaxUint64||
  uint64(clockBefore.Revision)==math.MaxUint64{return empty,errors.New("system cycle is stale or overflowing")}
 if abort,hit,err:=c.abortIfCascadeExceeded(ctx,run,cycle,fence);hit||err!=nil{return abort,err}
 typed,hash,err:=c.store.LoadWorldState(ctx,uint64(clockBefore.Revision))
 if err!=nil||hash!=cycle.BaseStateHash{return empty,errors.New("system cycle predecessor state hash invalid")}
 entries,ok:=fence.Fields[11].([]any)
 if !ok||len(entries)!=1{return empty,errors.New("synthetic system handler requires one admitted unit")}
 value,ok:=entries[0].([]any)
 if !ok||len(value)!=5{return empty,errors.New("system admitted unit malformed")}
 kind,ok:=value[1].(uint64)
 if !ok||(kind!=2&&kind!=3){return empty,errors.New("not a system-unit admission")}
 unitID,ok:=value[3].([]byte)
 if !ok||len(unitID)!=32{return empty,errors.New("system source ID invalid")}
 dg,ok:=value[4].([]byte)
 if !ok||len(dg)!=32{return empty,errors.New("system source digest invalid")}
 var source domain.ID;copy(source[:],unitID)
 var digest domain.Digest;copy(digest[:],dg)
 raw,actual,err:=loader.LoadSystemUnit(ctx,[32]byte(run),[32]byte(source))
 if err!=nil||uint64(actual)!=kind{return empty,errors.New("source not in authenticated active system store")}
 decoded,err:=codec.StrictDecodePayload(raw);if err!=nil{return empty,err}
 fields,ok:=decoded.([]any)
 if !ok||len(fields)==0||!bytes.Equal(fields[0].([]byte),unitID)||
  !bytes.Equal(fields[len(fields)-1].([]byte),dg)||
  fields[2]!=value[2]{return empty,errors.New("system unit differs from frozen canonical source")}
 var status uint64
 var reason,producer string
 var newDue int64
 var recurrence any
 switch kind{
 case 2:
  status,newDue,recurrence,err=settlement.OccurrenceOutcome(run,fields,clockBefore.Instant)
  if err!=nil{return empty,err}
  if status==1{reason="schedule.rejected"}else{reason="schedule.deferred"}
  producer="schedule.lifecycle"
 case 3:
  if len(fields)!=12{return empty,errors.New("trigger activation schema malformed")}
  registered,err:=c.store.LoadRegisteredTriggers(ctx,[32]byte(run),c.bundle)
  if err!=nil{return empty,err}
  definition,ok:=fields[3].([]byte);if !ok||len(definition)!=32{return empty,errors.New("activation definition missing")}
  matched:=false
  for _,t:=range registered{
   if bytes.Equal(t.Definition.ID[:],definition)&&fields[4]==uint64(t.Definition.Version){matched=true;break}
  }
  if !matched{return empty,errors.New("trigger activation has no registered definition/handler")}
  status=0;reason="trigger.consumed";producer="trigger.lifecycle"
 }
 event,err:=settlement.SystemLifecycleEvent(run,cycle.ID,source,digest,kind,status,
  clockBefore.Instant,clockBefore.Revision,clockBefore.NextLogicalSequence)
 if err!=nil{return empty,err}
 successors:=[]ports.CanonicalRecord{}
 successorRef:=[]any{uint64(0)}
 if kind==2&&status==2{
  successor,err:=settlement.OccurrenceSuccessor(run,fields,clockBefore.Instant,event,newDue,recurrence)
  if err!=nil{return empty,err}
  successors=append(successors,successor)
  successorRef=[]any{uint64(1),successor.Identity[:]}
 }
 policies,err:=policiesByRole(c.bundle);if err!=nil{return empty,err}
 d:=identity.New(c.bundle)
 sourceRef:=[]any{kind,source[:],digest[:]}
 candidate,err:=decision.MakeCandidate(d,run,cycle.ID,[]domain.ID{source},producer,1,0,0,
  []any{},[]any{sourceRef},0,policies["conflict-resolvers"])
 if err!=nil{return empty,err}
 payload,err:=codec.StrictDecodePayload(event.Canonical);if err!=nil{return empty,err}
 eventFields:=payload.([]any)
 candidate.Fields[8]=[]any{[]any{uint64(0),uint64(0),reason,eventFields[9],[]any{}}}
 candidate.Fields[12]=[]any{"source.lifecycle"}
 candidate.Digest,_,err=d.Sum("cote.csf.digest.commit-candidate",
  "cote.csf.schema.commit-candidate.body",1,candidate.Fields)
 if err!=nil{return empty,err}
 candidate.Canonical,err=codec.CanonicalPayload(append(cloneAny(candidate.Fields),candidate.Digest[:]))
 if err!=nil{return empty,err}
 decisionID,_,err:=d.Sum("cote.csf.id.decision","cote.csf.schema.decision-id-preimage",1,
  []any{cycle.ID[:],uint64(cycle.Attempt),source[:]});if err!=nil{return empty,err}
 decisionFields:=[]any{decisionID[:],run[:],cycle.ID[:],uint64(cycle.Attempt),sourceRef,
  status,[]any{uint64(1),candidate.ID[:]},reason,[]any{},policies["validators"],
  policies["conflict-resolvers"],[]any{},[]any{event.Identity[:]},successorRef,[]any{uint64(0)}}
 recordDigest,_,err:=d.Sum("cote.csf.digest.decision-record","cote.csf.schema.decision-record.body",1,decisionFields)
 if err!=nil{return empty,err}
 recordBytes,err:=codec.CanonicalPayload(append(cloneAny(decisionFields),recordDigest[:]))
 if err!=nil{return empty,err}
 decisionRecord:=ports.CanonicalRecord{Identity:[32]byte(decisionID),Canonical:recordBytes}
 pairHash,_,err:=d.Sum("cote.csf.digest.decision","cote.csf.schema.decision-pair-list",1,
  []any{[]any{source[:],decisionID[:],recordDigest[:]}});if err!=nil{return empty,err}
 taskHash,_,err:=d.Sum("cote.csf.digest.perception-task-list","cote.csf.schema.perception-task-id-list",1,[]any{})
 if err!=nil{return empty,err}
 next:=clockBefore;next.Revision++;next.LastOrdinal=int64(cycle.Ordinal);next.NextLogicalSequence++
 oldClock,err:=clock.Encode(clockBefore);if err!=nil{return empty,err}
 newClock,err:=clock.Encode(next);if err!=nil{return empty,err}
 revision,err:=codec.CanonicalPayload([]any{uint64(clockBefore.Revision),typed,hash[:],
  uint64(next.Revision),typed,hash[:]});if err!=nil{return empty,err}
 refs:=[]any{event.Identity[:]}
 journalFields:=[]any{run[:],cycle.ID[:],uint64(cycle.Attempt),int64(cycle.Instant),
  uint64(cycle.Ordinal),[]any{uint64(0)},uint64(cycle.BaseRevision),hash[:],
  uint64(next.Revision),hash[:],fence.Digest[:],[]any{source[:]},fence.InputDigest[:],
  policies["cycle-coordinate"],policies["event-order"],refs,[]any{decisionID[:]},
  pairHash[:],[]any{},taskHash[:],policies["perception"],policies["perception-identity"],
  uint64(next.NextLogicalSequence)}
 batchDigest,_,err:=d.Sum("cote.csf.digest.batch","cote.csf.schema.batch-preimage",1,
  []any{journalFields,[]any{event.Canonical}});if err!=nil{return empty,err}
 commitID,_,err:=d.Sum("cote.csf.id.cycle-commit","cote.csf.schema.cycle-commit-id-preimage",1,
  []any{run[:],cycle.ID[:],uint64(cycle.Attempt)});if err!=nil{return empty,err}
 journalBytes,err:=codec.CanonicalPayload(append(cloneAny(journalFields),batchDigest[:]))
 if err!=nil{return empty,err}
 batch:=ports.WorkBatch{Mode:3,RunID:[32]byte(run),CycleID:[32]byte(cycle.ID),FenceID:[32]byte(fence.ID),
  SystemCandidate:ports.CanonicalRecord{Identity:[32]byte(candidate.ID),Canonical:candidate.Canonical},
  SystemSuccessors:successors,ExpectedClock:oldClock,ResultClock:newClock,ExpectedInputPrefix:prefix,
  Journal:ports.CanonicalRecord{Identity:[32]byte(commitID),Canonical:journalBytes},
  Events:[]ports.CanonicalRecord{event},Decisions:[]ports.CanonicalRecord{decisionRecord},
  BaseStateHash:[32]byte(hash),ResultStateHash:[32]byte(hash),RevisionCanonical:revision,
  FailAfterStep:c.failAfterStep}
 if c.tamperWorkBatch!=nil{c.tamperWorkBatch(&batch)}
 if err:=c.store.CommitWork(ctx,batch);err!=nil{return empty,err}
 return batch.Journal,nil
}

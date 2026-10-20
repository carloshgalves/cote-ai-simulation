package commit

import (
 "bytes"
 "context"
 "errors"
 "fmt"
 "math"
 "sort"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/admission"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/clock"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/inputledger"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/settlement"
)

// CommitNoProposal is the first successful WORK terminal slice. Its
// narrowness is deliberate: every admitted unit must be a real, recorded
// NoProposal, so there is no world mutation, EventDraft, or PerceptionTask
// to invent. A subsequent extension must use the SAME CommitWork authority
// when it adds reducer-owned events and lifecycle transitions.
func (c *Coordinator) CommitNoProposal(ctx context.Context,ledger *inputledger.Ledger,
 cycle admission.Cycle,fence admission.Fence)(ports.CanonicalRecord,error){
 var empty ports.CanonicalRecord
 if c==nil||c.bundle==nil||c.store==nil||ledger==nil||!ledger.SharesFenceAuthority(c.store){
  return empty,errors.New("NoProposal work requires shared authoritative store")
 }
 view,prefix,err:=ledger.AdmissionView(ctx,c.bundle);if err!=nil{return empty,err}
 if err:=admission.VerifyFence(c.bundle,view,cycle,c.admissionPolicies,fence);err!=nil{return empty,err}
 persisted,err:=c.store.GetFence(ctx,[32]byte(fence.ID))
 if err!=nil||!bytes.Equal(persisted,fence.Canonical){return empty,errors.New("NoProposal fence is not durably published")}
 _,_,_,rounds,slots,runID:=view.Snapshot()
 if runID==cycle.ID{return empty,errors.New("cycle identity must differ from run identity")}
 state,err:=c.store.LoadClock(ctx,[32]byte(runID));if err!=nil{return empty,err}
 if state.Instant!=cycle.Instant||state.Revision!=cycle.BaseRevision||state.LastOrdinal>=int64(cycle.Ordinal)||
  state.Revision==domain.WorldRevision(math.MaxUint64)||cycle.Ordinal>math.MaxInt64{
  return empty,errors.New("stale or overflowing WORK clock")
 }
 if abort,hit,err:=c.abortIfCascadeExceeded(ctx,runID,cycle,fence);hit||err!=nil{return abort,err}
 typed,hash,err:=c.store.LoadWorldState(ctx,uint64(state.Revision))
 if err!=nil{return empty,err}
 if !bytes.Equal(cycle.BaseStateHash[:],hash[:]){return empty,errors.New("NoProposal requires the authenticated current world state")}
 byID:=map[domain.ID]inputledger.Response{}
 for _,round:=range rounds{
  for _,response:=range slots[round.ID]{
   if _,dup:=byID[response.ID];dup{return empty,errors.New("duplicate recorded slot root")}
   byID[response.ID]=response
  }
 }
 unitValues,ok:=fence.Fields[11].([]any);if !ok{return empty,errors.New("fence admitted source list missing")}
 d:=identity.New(c.bundle)
 policies,err:=policiesByRole(c.bundle);if err!=nil{return empty,err}
 decisions:=make([]ports.CanonicalRecord,0,len(unitValues))
 ids,pairs,admitted:=make([]any,0,len(unitValues)),make([]any,0,len(unitValues)),make([]any,0,len(unitValues))
 type roundSlot struct{slot string;id domain.ID;dig domain.Digest}
 roundSources:=map[domain.ID][]roundSlot{}
 seen:=map[domain.ID]bool{}
 for _,raw:=range unitValues{
  entry,ok:=raw.([]any)
  if !ok||len(entry)!=5||entry[1]!=uint64(0){return empty,errors.New("WORK unit is not an admitted slot response")}
  b,ok:=entry[3].([]byte);if !ok||len(b)!=32{return empty,errors.New("invalid source response ID")}
  var id domain.ID;copy(id[:],b)
  if seen[id]{return empty,errors.New("duplicate source response")}
  seen[id]=true
  response,found:=byID[id]
  dig,valid:=entry[4].([]byte)
  if !valid||len(dig)!=32||!found||response.Kind!=1||!bytes.Equal(dig,response.Digest[:]){
   return empty,errors.New("cannot commit proposal or an unauthenticated NoProposal as no-world-effect WORK")
  }
  roundSources[response.RoundID]=append(roundSources[response.RoundID],roundSlot{
   slot:response.SlotID,id:response.ID,dig:response.Digest})
  decisionID,_,err:=d.Sum("cote.csf.id.decision","cote.csf.schema.decision-id-preimage",1,
   []any{cycle.ID[:],uint64(cycle.Attempt),id[:]})
  if err!=nil{return empty,err}
  recordFields:=[]any{decisionID[:],runID[:],cycle.ID[:],uint64(cycle.Attempt),
   []any{uint64(0),uint64(1),id[:],response.Digest[:]},uint64(3),
   []any{uint64(0)},"slot.no-proposal",[]any{},policies["validators"],
   policies["conflict-resolvers"],[]any{},[]any{},[]any{uint64(0)},[]any{uint64(0)}}
  recordHash,_,err:=d.Sum("cote.csf.digest.decision-record","cote.csf.schema.decision-record.body",1,recordFields)
  if err!=nil{return empty,err}
  canonical,err:=codec.CanonicalPayload(append(cloneAny(recordFields),recordHash[:]))
  if err!=nil{return empty,err}
  decisions=append(decisions,ports.CanonicalRecord{Identity:[32]byte(decisionID),Canonical:canonical})
  ids=append(ids,decisionID[:])
  pairs=append(pairs,[]any{id[:],decisionID[:],recordHash[:]})
  admitted=append(admitted,id[:])
 }
 if len(unitValues)==0{return empty,errors.New("NoProposal WORK needs admitted slot responses")}
 decisionDigest,_,err:=d.Sum("cote.csf.digest.decision","cote.csf.schema.decision-pair-list",1,pairs)
 if err!=nil{return empty,err}
 taskDigest,_,err:=d.Sum("cote.csf.digest.perception-task-list","cote.csf.schema.perception-task-id-list",1,[]any{})
 if err!=nil{return empty,err}
 // Exactly one SOURCE_LIFECYCLE event per selected round, ordered by
 // canonical Round ID and sorted slot IDs; worker delivery cannot reorder it.
 cohort,ok:=fence.Fields[10].([]any)
 if !ok||len(cohort)!=2{return empty,errors.New("invalid round cohort")}
 rawRounds,ok:=cohort[1].([]any)
 if !ok||len(rawRounds)!=len(roundSources){return empty,errors.New("nonterminal or omitted round in NoProposal batch")}
 orderedRounds:=make([]domain.ID,0,len(rawRounds))
 for _,raw:=range rawRounds{
  fields,ok:=raw.([]any);if !ok||len(fields)!=2{return empty,errors.New("malformed admitted cohort round")}
  idBytes,ok:=fields[0].([]byte);if !ok||len(idBytes)!=32{return empty,errors.New("cohort round identity missing")}
  var id domain.ID;copy(id[:],idBytes)
  orderedRounds=append(orderedRounds,id)
 }
 sort.Slice(orderedRounds,func(i,j int)bool{return bytes.Compare(orderedRounds[i][:],orderedRounds[j][:])<0})
 events:=make([]ports.CanonicalRecord,0,len(orderedRounds))
 eventIDs:=make([]any,0,len(orderedRounds))
 canonicalEvents:=make([]any,0,len(orderedRounds))
 for i,roundID:=range orderedRounds{
  if i>0&&roundID==orderedRounds[i-1]{return empty,errors.New("duplicate round in cohort")}
  rows:=roundSources[roundID]
  if len(rows)==0{return empty,errors.New("round has no terminal slot response")}
  sort.Slice(rows,func(i,j int)bool{return rows[i].slot<rows[j].slot})
  refs:=make([]any,0,len(rows))
  for n,row:=range rows{
   if n>0&&row.slot==rows[n-1].slot{return empty,errors.New("round slot duplicated")}
   refs=append(refs,[]any{uint64(0),uint64(1),row.id[:],row.dig[:]})
  }
  if uint64(i)>math.MaxUint64-uint64(state.NextLogicalSequence){return empty,errors.New("WORK logical sequence overflow")}
  event,err:=settlement.ConsumedRoundEvent(runID,cycle.ID,roundID,
   state.Instant,state.Revision,state.NextLogicalSequence+domain.LogicalSequence(i),refs)
  if err!=nil{return empty,err}
  events=append(events,event);eventIDs=append(eventIDs,event.Identity[:])
  canonicalEvents=append(canonicalEvents,event.Canonical)
 }
 next:=state
 next.Revision++
 next.LastOrdinal=int64(cycle.Ordinal)
 if uint64(len(events))>math.MaxUint64-uint64(next.NextLogicalSequence){return empty,errors.New("WORK logical sequence overflow")}
 next.NextLogicalSequence+=domain.LogicalSequence(len(events))
 originalClock,err:=clock.Encode(state);if err!=nil{return empty,err}
 nextClock,err:=clock.Encode(next);if err!=nil{return empty,err}
 revision,err:=codec.CanonicalPayload([]any{uint64(state.Revision),typed,hash[:],
  uint64(next.Revision),typed,hash[:]});if err!=nil{return empty,err}
 journalBody:=[]any{runID[:],cycle.ID[:],uint64(cycle.Attempt),int64(cycle.Instant),
  uint64(cycle.Ordinal),[]any{uint64(0)},uint64(cycle.BaseRevision),hash[:],
  uint64(next.Revision),hash[:],fence.Digest[:],admitted,fence.InputDigest[:],
  policies["cycle-coordinate"],policies["event-order"],eventIDs,ids,decisionDigest[:],
  []any{},taskDigest[:],policies["perception"],policies["perception-identity"],
  uint64(next.NextLogicalSequence)}
 batchHash,_,err:=d.Sum("cote.csf.digest.batch","cote.csf.schema.batch-preimage",1,
  []any{journalBody,canonicalEvents});if err!=nil{return empty,err}
 commitID,_,err:=d.Sum("cote.csf.id.cycle-commit","cote.csf.schema.cycle-commit-id-preimage",1,
  []any{runID[:],cycle.ID[:],uint64(cycle.Attempt)});if err!=nil{return empty,err}
 journalBytes,err:=codec.CanonicalPayload(append(cloneAny(journalBody),batchHash[:]))
 if err!=nil{return empty,err}
 journal:=ports.CanonicalRecord{Identity:[32]byte(commitID),Canonical:journalBytes}
 batch:=ports.WorkBatch{RunID:[32]byte(runID),CycleID:[32]byte(cycle.ID),
  FenceID:[32]byte(fence.ID),ExpectedClock:originalClock,ResultClock:nextClock,
  ExpectedInputPrefix:prefix,Journal:journal,Decisions:decisions,Events:events,
  BaseStateHash:hash,RevisionCanonical:revision,FailAfterStep:c.failAfterStep}
 if c.tamperWorkBatch!=nil{c.tamperWorkBatch(&batch)}
 if err:=c.store.CommitWork(ctx,batch);err!=nil{return empty,fmt.Errorf("NoProposal WORK transaction rejected: %w",err)}
 return journal,nil
}

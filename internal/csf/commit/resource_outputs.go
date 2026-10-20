package commit

import (
 "bytes"
 "errors"
 "math"
 "sort"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/clock"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/decision"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/reducer"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/settlement"
)

// ResourceOutput is a PURE preparatory working set, not a durable settlement.
// Only the single CommitWork SQLite transaction may publish its events and
// post-state after independently checking the pinned fence and source roots.
type ResourceOutput struct{
 Events []ports.CanonicalRecord
 ResultWorld []any
 ResultHash domain.Digest
}

// MaterializeResource derives the V1 synthetic resource event and matching
// round lifecycle for the exact provisional decision. No LLM, store, time,
// input arrival order or mutable resource state is consulted.
func MaterializeResource(input decision.Input,result decision.Result,before []any,
 state clock.State)(ResourceOutput,error){
 var out ResourceOutput
 stock,err:=reducer.DecodeStock(before);if err!=nil{return out,err}
 if state.Revision!=input.Snapshot.BaseRevision||stock.Available>math.MaxInt64||
  int64(stock.Available)!=input.Snapshot.Available||!input.Snapshot.Known{
  return out,errors.New("resource snapshot does not equal authoritative typed stock")
 }
 verified,err:=decision.Run(input)
 if err!=nil||verified.Failure!=""||!bytes.Equal(verified.Canonical,result.Canonical){
  return out,errors.New("resource provisional result not reconstructible from pinned inputs")
 }
 bySource:=map[domain.ID]decision.Unit{}
 for _,unit:=range input.Units{
  if _,exists:=bySource[unit.Response.ID];exists{return out,errors.New("duplicate materialization source")}
  bySource[unit.Response.ID]=unit
 }
 if len(result.Dispositions)!=len(input.Admitted)||len(bySource)!=len(input.Admitted){
  return out,errors.New("resource disposition/source partition incomplete")
 }
 byCandidate:=map[domain.ID]decision.Candidate{}
 for _,candidate:=range result.Candidates{
  if _,duplicate:=byCandidate[candidate.ID];duplicate{return out,errors.New("duplicate candidate")}
  byCandidate[candidate.ID]=candidate
 }
 type ref struct{slot string;kind uint64;id domain.ID;digest domain.Digest}
 roundSources:=map[domain.ID][]ref{}
 winners:=make([]decision.Candidate,0,1)
 var consumed uint64
 for i,disposition:=range result.Dispositions{
  sourceID:=input.Admitted[i]
  if disposition.Subject!=sourceID{return out,errors.New("disposition not in fenced order")}
  source,found:=bySource[sourceID]
  if !found||source.Response.Kind!=0{return out,errors.New("resource handler cannot settle non-proposals")}
  roundSources[source.Response.RoundID]=append(roundSources[source.Response.RoundID],
   ref{slot:source.Response.SlotID,kind:0,id:sourceID,digest:source.Response.Digest})
  candidate,found:=byCandidate[disposition.CandidateID]
  if !found||len(candidate.SourceUnitIDs)!=1||candidate.SourceUnitIDs[0]!=sourceID{
   return out,errors.New("resource disposition not backed by candidate")
  }
  if disposition.Status=="COMMIT"{
   claims,ok:=candidate.Fields[9].([]any);if !ok||len(claims)!=1{return out,errors.New("resource winning claim missing")}
   claim,ok:=claims[0].([]any);if !ok||len(claim)!=4||
    claim[0]!="inventory"||claim[1]!="last.resource"||claim[2]!=uint64(1){
    return out,errors.New("resource winner claim outside registered stock")
   }
   amount,ok:=claim[3].(int64)
   if !ok{v,valid:=claim[3].(uint64);if valid&&v<=math.MaxInt64{amount=int64(v);ok=true}}
   if !ok||amount!=1{return out,errors.New("synthetic resource reducer requires exactly one unit")}
   if uint64(amount)>math.MaxUint64-consumed{return out,errors.New("resource claim overflow")}
   consumed+=uint64(amount)
   winners=append(winners,candidate)
  }else if disposition.Status!="REJECT"{
   return out,errors.New("unsupported resource disposition without lifecycle handler")
  }
 }
 if consumed!=1||len(winners)!=1{return out,errors.New("single last resource requires exactly one winning candidate")}
 after,afterHash,err:=reducer.Consume(before,consumed);if err!=nil{return out,err}
 afterStock,err:=reducer.DecodeStock(after);if err!=nil||afterStock.Available>math.MaxInt64||
  int64(afterStock.Available)!=result.PostAvailable{
  return out,errors.New("resource provisional post-state differs from reducer")
 }
 winner:=winners[0]
 drafts,ok:=winner.Fields[8].([]any);if !ok||len(drafts)!=1{return out,errors.New("winning candidate missing EventDraft")}
 draft,ok:=drafts[0].([]any);if !ok{return out,errors.New("winning EventDraft malformed")}
 sourceID:=winner.SourceUnitIDs[0]
 source:=bySource[sourceID]
 resourceEvent,err:=settlement.ResourceConsumedEvent(input.RunID,input.CycleID,winner.ID,sourceID,
  source.Response.Digest,state.Instant,state.Revision,state.NextLogicalSequence,draft)
 if err!=nil{return out,err}
 out.Events=append(out.Events,resourceEvent)
 rounds:=make([]domain.ID,0,len(roundSources))
 for id:=range roundSources{rounds=append(rounds,id)}
 sort.Slice(rounds,func(i,j int)bool{return bytes.Compare(rounds[i][:],rounds[j][:])<0})
 for i,id:=range rounds{
  items:=roundSources[id]
  sort.Slice(items,func(i,j int)bool{return items[i].slot<items[j].slot})
  refs:=make([]any,len(items))
  for j,v:=range items{
   if j>0&&items[j-1].slot==v.slot{return out,errors.New("duplicate slot in terminal round")}
   refs[j]=[]any{uint64(0),v.kind,v.id[:],v.digest[:]}
  }
  if uint64(i)+1>math.MaxUint64-uint64(state.NextLogicalSequence){
   return out,errors.New("resource event logical-sequence overflow")
  }
  event,err:=settlement.ConsumedRoundEvent(input.RunID,input.CycleID,id,
   state.Instant,state.Revision,state.NextLogicalSequence+domain.LogicalSequence(i)+1,refs)
  if err!=nil{return out,err}
  out.Events=append(out.Events,event)
 }
 out.ResultWorld=after;out.ResultHash=afterHash
 return out,nil
}

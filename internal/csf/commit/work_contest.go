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
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/reducer"
)

// CommitTwoResourceContest closes the first genuinely contested special-exam
// resource. The deterministic kernel resolves two otherwise-eligible actors;
// only the authenticated SQLite CommitWork publishes their final verdicts.
func (c *Coordinator) CommitTwoResourceContest(ctx context.Context,ledger *inputledger.Ledger,
 cycle admission.Cycle,fence admission.Fence)(ports.CanonicalRecord,error){
 var empty ports.CanonicalRecord
 if c==nil||c.store==nil||c.bundle==nil||ledger==nil||!ledger.SharesFenceAuthority(c.store){
  return empty,errors.New("contest requires shared durable causal authority")
 }
 view,prefix,err:=ledger.AdmissionView(ctx,c.bundle);if err!=nil{return empty,err}
 if err:=admission.VerifyFence(c.bundle,view,cycle,c.admissionPolicies,fence);err!=nil{return empty,err}
 persisted,err:=c.store.GetFence(ctx,[32]byte(fence.ID))
 if err!=nil||!bytes.Equal(persisted,fence.Canonical){return empty,errors.New("contest fence not durably sealed")}
 _,_,_,_,slots,run:=view.Snapshot()
 state,err:=c.store.LoadClock(ctx,[32]byte(run));if err!=nil{return empty,err}
 if state.Revision!=cycle.BaseRevision||state.Instant!=cycle.Instant||
  cycle.Ordinal>math.MaxInt64||state.LastOrdinal>=int64(cycle.Ordinal)||
  state.Revision==domain.WorldRevision(math.MaxUint64){return empty,errors.New("stale contest coordinate")}
 if abort,hit,err:=c.abortIfCascadeExceeded(ctx,run,cycle,fence);hit||err!=nil{return abort,err}
 before,hash,err:=c.store.LoadWorldState(ctx,uint64(state.Revision));if err!=nil{return empty,err}
 if hash!=cycle.BaseStateHash{return empty,errors.New("contest WorldState base differs from fence")}
 stock,err:=reducer.DecodeStock(before);if err!=nil||stock.Available!=1{return empty,errors.New("two contenders require precisely one real inventory unit")}
 units,ok:=fence.Fields[11].([]any)
 if !ok||len(units)<2{return empty,errors.New("contest requires two admitted slot responses")}
 responses:=map[domain.ID]inputledger.Response{}
 for _,unit:=range slots{for _,response:=range unit{responses[response.ID]=response}}
 actors,forbidden:=map[string]bool{},map[string]bool{}
 slotCount:=0
 for _,raw:=range units{
  fields,ok:=raw.([]any);if !ok||len(fields)!=5{return empty,errors.New("malformed source in contest fence")}
  if fields[1]!=uint64(0){
   if fields[1]!=uint64(2)&&fields[1]!=uint64(3){return empty,errors.New("unregistered co-admitted contest source")}
   continue
  }
  slotCount++
  idBytes,ok:=fields[3].([]byte);if !ok||len(idBytes)!=32{return empty,errors.New("contest source identity malformed")}
  var id domain.ID;copy(id[:],idBytes)
  response,exists:=responses[id]
  if !exists||response.Kind!=0||response.Actor[0]!="school"{return empty,errors.New("contest source not authentic school ActionProposal")}
  actor:=response.Actor[0]+":"+response.Actor[1]
  if actors[actor]{return empty,errors.New("contest duplicates an actor")}
  actors[actor]=true;forbidden[actor]=false
 }
 if slotCount!=2{return empty,errors.New("contest requires exactly two authenticated school slot responses")}
 manifest,err:=c.store.ValidatedGenesis(ctx,[32]byte(run),c.bundle);if err!=nil{return empty,err}
 assemble:=decision.FromPersistedFence
 if len(units)>2{assemble=decision.FromPersistedFenceWithSystemHandlers}
 input,err:=assemble(ctx,c.bundle,view,cycle,c.admissionPolicies,fence,c.store,
  decision.Snapshot{BaseRevision:state.Revision,ResourceKind:"inventory",ResourceID:"last.resource",
   Available:1,Known:true,AccessibleActors:actors,ForbiddenActors:forbidden},
  []decision.ActionSchema{{Type:"resource.consume",Version:1,Operation:"resource.consume",Producer:"resource.consume"}},
  manifest.WorldSeed,nil)
 if err!=nil{return empty,err}
 tentative,err:=decision.Run(input);if err!=nil{return empty,err}
 if len(tentative.Candidates)!=2||len(tentative.Conflicts)!=1||
  len(tentative.Draws)!=1||len(tentative.Assessments)!=2||len(tentative.Dispositions)!=2{
  return empty,errors.New("contested resource outcome missing canonical evidence")
 }
 output,err:=MaterializeResource(input,tentative,before,state);if err!=nil{return empty,err}
 if len(output.Events)!=3{return empty,errors.New("two-contender resource event count invalid")}
 deriver:=identity.New(c.bundle)
 policies,err:=policiesByRole(c.bundle);if err!=nil{return empty,err}
 candidates:=map[domain.ID]decision.Candidate{}
 for _,candidate:=range tentative.Candidates{candidates[candidate.ID]=candidate}
 conflict:=tentative.Conflicts[0]
 draw:=tentative.Draws[0]
 conflictRef:=[]any{uint64(9),conflict.ID[:],conflict.Digest[:]}
 drawRef:=[]any{uint64(24),draw.ID[:],draw.Digest[:]}
 decisions:=make([]ports.CanonicalRecord,0,2)
 decisionIDs,pairs,admitted:=make([]any,0,2),make([]any,0,2),make([]any,0,2)
 wins:=0
 for i,source:=range input.Admitted{
  disp:=tentative.Dispositions[i]
  response:=responses[source]
  candidate,ok:=candidates[disp.CandidateID]
  if !ok||disp.Subject!=source||len(candidate.SourceUnitIDs)!=1||candidate.SourceUnitIDs[0]!=source{
   return empty,errors.New("unbound contested terminal decision")
  }
  status,reason:=uint64(1),"resource.conflict"
  produced:=[]any{}
  if disp.Status=="COMMIT"{
   status,reason=0,"resource.allocated"
   produced=[]any{output.Events[0].Identity[:]}
   wins++
  }else if disp.Status!="REJECT"||disp.Reason!="resource.conflict"{
   return empty,errors.New("resource loser not rejected by authenticated conflict")
  }
  decisionID,_,err:=deriver.Sum("cote.csf.id.decision","cote.csf.schema.decision-id-preimage",1,
   []any{cycle.ID[:],uint64(cycle.Attempt),source[:]});if err!=nil{return empty,err}
  body:=[]any{decisionID[:],run[:],cycle.ID[:],uint64(cycle.Attempt),
   []any{uint64(0),uint64(0),source[:],response.Digest[:]},status,
   []any{uint64(1),candidate.ID[:]},reason,[]any{conflictRef},
   policies["validators"],policies["conflict-resolvers"],[]any{drawRef},
   produced,[]any{uint64(0)},[]any{uint64(0)}}
  h,_,err:=deriver.Sum("cote.csf.digest.decision-record","cote.csf.schema.decision-record.body",1,body)
  if err!=nil{return empty,err}
  encoded,err:=codec.CanonicalPayload(append(cloneAny(body),h[:]));if err!=nil{return empty,err}
  decisions=append(decisions,ports.CanonicalRecord{Identity:[32]byte(decisionID),Canonical:encoded})
  decisionIDs=append(decisionIDs,decisionID[:])
  pairs=append(pairs,[]any{source[:],decisionID[:],h[:]})
  admitted=append(admitted,source[:])
 }
 if wins!=1{return empty,errors.New("contested resource selected other than one winner")}
 dd,_,err:=deriver.Sum("cote.csf.digest.decision","cote.csf.schema.decision-pair-list",1,pairs)
 if err!=nil{return empty,err}
 taskHash,_,err:=deriver.Sum("cote.csf.digest.perception-task-list","cote.csf.schema.perception-task-id-list",1,[]any{})
 if err!=nil{return empty,err}
 next:=state
 next.Revision++;next.LastOrdinal=int64(cycle.Ordinal)
 if uint64(len(output.Events))>math.MaxUint64-uint64(next.NextLogicalSequence){
  return empty,errors.New("contest logical-sequence overflow")
 }
 next.NextLogicalSequence+=domain.LogicalSequence(len(output.Events))
 oldClock,err:=clock.Encode(state);if err!=nil{return empty,err}
 newClock,err:=clock.Encode(next);if err!=nil{return empty,err}
 revision,err:=codec.CanonicalPayload([]any{uint64(state.Revision),before,hash[:],
  uint64(next.Revision),output.ResultWorld,output.ResultHash[:]})
 if err!=nil{return empty,err}
 eventIDs,eventBytes:=make([]any,0,len(output.Events)),make([]any,0,len(output.Events))
 for _,event:=range output.Events{eventIDs=append(eventIDs,event.Identity[:]);eventBytes=append(eventBytes,event.Canonical)}
 body:=[]any{run[:],cycle.ID[:],uint64(cycle.Attempt),int64(cycle.Instant),
  uint64(cycle.Ordinal),[]any{uint64(0)},uint64(cycle.BaseRevision),hash[:],
  uint64(next.Revision),output.ResultHash[:],fence.Digest[:],admitted,fence.InputDigest[:],
  policies["cycle-coordinate"],policies["event-order"],eventIDs,decisionIDs,
  dd[:],[]any{},taskHash[:],policies["perception"],
  policies["perception-identity"],uint64(next.NextLogicalSequence)}
 batchHash,_,err:=deriver.Sum("cote.csf.digest.batch","cote.csf.schema.batch-preimage",1,
  []any{body,eventBytes});if err!=nil{return empty,err}
 cid,_,err:=deriver.Sum("cote.csf.id.cycle-commit","cote.csf.schema.cycle-commit-id-preimage",1,
  []any{run[:],cycle.ID[:],uint64(cycle.Attempt)});if err!=nil{return empty,err}
 journalBytes,err:=codec.CanonicalPayload(append(cloneAny(body),batchHash[:]))
 if err!=nil{return empty,err}
 proof:=make([]ports.CanonicalRecord,0,6)
 for _,v:=range tentative.Candidates{proof=append(proof,ports.CanonicalRecord{Identity:[32]byte(v.ID),Canonical:v.Canonical})}
 for _,v:=range tentative.Assessments{proof=append(proof,ports.CanonicalRecord{Identity:[32]byte(v.Digest),Canonical:v.Canonical})}
 proof=append(proof,ports.CanonicalRecord{Identity:[32]byte(conflict.ID),Canonical:conflict.Canonical},
  ports.CanonicalRecord{Identity:[32]byte(draw.ID),Canonical:draw.Canonical})
 sort.Slice(proof,func(i,j int)bool{return bytes.Compare(proof[i].Identity[:],proof[j].Identity[:])<0})
 batch:=ports.WorkBatch{Mode:2,ContestProofs:proof,ResultStateHash:[32]byte(output.ResultHash),
  RunID:[32]byte(run),CycleID:[32]byte(cycle.ID),FenceID:[32]byte(fence.ID),
  ExpectedClock:oldClock,ResultClock:newClock,ExpectedInputPrefix:prefix,
  Journal:ports.CanonicalRecord{Identity:[32]byte(cid),Canonical:journalBytes},
  Decisions:decisions,Events:output.Events,BaseStateHash:[32]byte(hash),
  RevisionCanonical:revision,FailAfterStep:c.failAfterStep}
 if len(units)>2{return c.commitResourceSystem(ctx,ledger,cycle,fence,batch)}
 if err:=c.commitPreparedWork(ctx,&batch);err!=nil{return empty,err}
 return batch.Journal,nil
}

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
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/genesis"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/inputledger"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/reducer"
)

// CommitSingleResource is deliberately narrower than the future full resource
// competition: one admitted ActionProposal, one registered candidate and one
// claimed unit. It still uses the existing CommitWork transaction authority;
// it cannot bypass the persisted fence or write any gameplay state by itself.
func (c *Coordinator) CommitSingleResource(ctx context.Context,ledger *inputledger.Ledger,
 cycle admission.Cycle,fence admission.Fence)(ports.CanonicalRecord,error){
 var empty ports.CanonicalRecord
 if c==nil||c.store==nil||c.bundle==nil||ledger==nil||!ledger.SharesFenceAuthority(c.store){
  return empty,errors.New("resource commit requires common authority")
 }
 view,prefix,err:=ledger.AdmissionView(ctx,c.bundle);if err!=nil{return empty,err}
 if err:=admission.VerifyFence(c.bundle,view,cycle,c.admissionPolicies,fence);err!=nil{return empty,err}
 persisted,err:=c.store.GetFence(ctx,[32]byte(fence.ID))
 if err!=nil||!bytes.Equal(persisted,fence.Canonical){return empty,errors.New("resource fence not published")}
 _,_,_,_,slots,run:=view.Snapshot()
 state,err:=c.store.LoadClock(ctx,[32]byte(run));if err!=nil{return empty,err}
 if state.Revision!=cycle.BaseRevision||state.Instant!=cycle.Instant||
  state.LastOrdinal>=int64(cycle.Ordinal)||state.Revision==domain.WorldRevision(math.MaxUint64)||
  cycle.Ordinal>math.MaxInt64{return empty,errors.New("stale resource cycle coordinate")}
 if abort,hit,err:=c.abortIfCascadeExceeded(ctx,run,cycle,fence);hit||err!=nil{return abort,err}
 before,hash,err:=c.store.LoadWorldState(ctx,uint64(state.Revision));if err!=nil{return empty,err}
 if hash!=cycle.BaseStateHash{return empty,errors.New("resource base hash differs from fence")}
 stock,err:=reducer.DecodeStock(before);if err!=nil{return empty,err}
 if stock.Available>math.MaxInt64{return empty,errors.New("resource stock too large for registered signed claim")}
 units,ok:=fence.Fields[11].([]any)
 if !ok||len(units)==0{return empty,errors.New("resource handler requires admitted proposal")}
 entry,ok:=units[0].([]any);if !ok||len(entry)!=5||entry[1]!=uint64(0){
  return empty,errors.New("resource handler cannot silently consume occurrence or trigger")
 }
 idBytes,ok:=entry[3].([]byte);if !ok||len(idBytes)!=32{return empty,errors.New("resource source ID malformed")}
 var sourceID domain.ID;copy(sourceID[:],idBytes)
 var response inputledger.Response
 matched:=false
 for _,items:=range slots{
  for _,unit:=range items{if unit.ID==sourceID{response=unit;matched=true}}
 }
 if !matched||response.Kind!=0||response.Actor[0]!="school"{
  return empty,errors.New("resource source is not a school ActionProposal")
 }
 genRaw,err:=c.store.LoadGenesis(ctx,[32]byte(run));if err!=nil{return empty,err}
 manifest,err:=genesis.ValidateCanonical(genRaw,c.bundle);if err!=nil{return empty,err}
 actor:=response.Actor[0]+":"+response.Actor[1]
 assemble:=decision.FromPersistedFence
 if len(units)>1{assemble=decision.FromPersistedFenceWithSystemHandlers}
 input,err:=assemble(ctx,c.bundle,view,cycle,c.admissionPolicies,fence,c.store,
  decision.Snapshot{BaseRevision:state.Revision,ResourceKind:"inventory",ResourceID:"last.resource",
   Available:int64(stock.Available),Known:true,
   AccessibleActors:map[string]bool{actor:true},ForbiddenActors:map[string]bool{actor:false}},
  []decision.ActionSchema{{Type:"resource.consume",Version:1,Operation:"resource.consume",Producer:"resource.consume"}},
  manifest.WorldSeed,nil)
 if err!=nil{return empty,err}
 result,err:=decision.Run(input);if err!=nil{return empty,err}
 if len(result.Candidates)!=1||len(result.Dispositions)!=1||result.Dispositions[0].Status!="COMMIT"{
  return empty,errors.New("resource resolver did not uniquely commit source")
 }
 output,err:=MaterializeResource(input,result,before,state);if err!=nil{return empty,err}
 if len(output.Events)!=2{return empty,errors.New("resource handler emitted unexpected number of lifecycle events")}
 d:=identity.New(c.bundle)
 policyRefs,err:=policiesByRole(c.bundle);if err!=nil{return empty,err}
 candidate:=result.Candidates[0]
 decisionID,_,err:=d.Sum("cote.csf.id.decision","cote.csf.schema.decision-id-preimage",1,
  []any{cycle.ID[:],uint64(cycle.Attempt),sourceID[:]})
 if err!=nil{return empty,err}
 body:=[]any{decisionID[:],run[:],cycle.ID[:],uint64(cycle.Attempt),
  []any{uint64(0),uint64(0),sourceID[:],response.Digest[:]},
  uint64(0),[]any{uint64(1),candidate.ID[:]},"resource.allocated",[]any{},
  policyRefs["validators"],policyRefs["conflict-resolvers"],[]any{},
  []any{output.Events[0].Identity[:]},[]any{uint64(0)},[]any{uint64(0)}}
 recordDigest,_,err:=d.Sum("cote.csf.digest.decision-record","cote.csf.schema.decision-record.body",1,body)
 if err!=nil{return empty,err}
 encoded,err:=codec.CanonicalPayload(append(cloneAny(body),recordDigest[:]))
 if err!=nil{return empty,err}
 record:=ports.CanonicalRecord{Identity:[32]byte(decisionID),Canonical:encoded}
 pairs:=[]any{[]any{sourceID[:],decisionID[:],recordDigest[:]}}
 dd,_,err:=d.Sum("cote.csf.digest.decision","cote.csf.schema.decision-pair-list",1,pairs)
 if err!=nil{return empty,err}
 tasks,_,err:=d.Sum("cote.csf.digest.perception-task-list","cote.csf.schema.perception-task-id-list",1,[]any{})
 if err!=nil{return empty,err}
 next:=state
 next.Revision++
 next.LastOrdinal=int64(cycle.Ordinal)
 if uint64(len(output.Events))>math.MaxUint64-uint64(next.NextLogicalSequence){
  return empty,errors.New("logical sequence overflow in resource commit")
 }
 next.NextLogicalSequence+=domain.LogicalSequence(len(output.Events))
 beforeClock,err:=clock.Encode(state);if err!=nil{return empty,err}
 afterClock,err:=clock.Encode(next);if err!=nil{return empty,err}
 worldRevision,err:=codec.CanonicalPayload([]any{uint64(state.Revision),before,hash[:],
  uint64(next.Revision),output.ResultWorld,output.ResultHash[:]})
 if err!=nil{return empty,err}
 eventIDs,canonicalEvents:=make([]any,0,len(output.Events)),make([]any,0,len(output.Events))
 for _,ev:=range output.Events{eventIDs=append(eventIDs,ev.Identity[:]);canonicalEvents=append(canonicalEvents,ev.Canonical)}
 journalBody:=[]any{run[:],cycle.ID[:],uint64(cycle.Attempt),int64(cycle.Instant),
  uint64(cycle.Ordinal),[]any{uint64(0)},uint64(cycle.BaseRevision),hash[:],
  uint64(next.Revision),output.ResultHash[:],fence.Digest[:],
  []any{sourceID[:]},fence.InputDigest[:],policyRefs["cycle-coordinate"],policyRefs["event-order"],
  eventIDs,[]any{decisionID[:]},dd[:],[]any{},tasks[:],policyRefs["perception"],
  policyRefs["perception-identity"],uint64(next.NextLogicalSequence)}
 batchDigest,_,err:=d.Sum("cote.csf.digest.batch","cote.csf.schema.batch-preimage",1,
  []any{journalBody,canonicalEvents});if err!=nil{return empty,err}
 commitID,_,err:=d.Sum("cote.csf.id.cycle-commit","cote.csf.schema.cycle-commit-id-preimage",1,
  []any{run[:],cycle.ID[:],uint64(cycle.Attempt)});if err!=nil{return empty,err}
 canonical,err:=codec.CanonicalPayload(append(cloneAny(journalBody),batchDigest[:]))
 if err!=nil{return empty,err}
 journal:=ports.CanonicalRecord{Identity:[32]byte(commitID),Canonical:canonical}
 batch:=ports.WorkBatch{Mode:1,ResourceCandidate:ports.CanonicalRecord{
   Identity:[32]byte(candidate.ID),Canonical:candidate.Canonical},ResultStateHash:[32]byte(output.ResultHash),
  RunID:[32]byte(run),CycleID:[32]byte(cycle.ID),FenceID:[32]byte(fence.ID),
  ExpectedClock:beforeClock,ResultClock:afterClock,ExpectedInputPrefix:prefix,
  Journal:journal,Decisions:[]ports.CanonicalRecord{record},Events:output.Events,
  BaseStateHash:[32]byte(hash),RevisionCanonical:worldRevision,FailAfterStep:c.failAfterStep}
 if len(units)>1{
  return c.commitResourceSystem(ctx,ledger,cycle,fence,batch)
 }
 if err:=c.commitPreparedWork(ctx,&batch);err!=nil{return empty,err}
 return batch.Journal,nil
}

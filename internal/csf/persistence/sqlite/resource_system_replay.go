package sqlite

import (
 "context"
 "errors"
 "math"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/clock"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
)

// verifyPublishedResourceSystem rehydrates resource and co-admitted
// occurrences/triggers exclusively from committed durable records. Neither
// a caller-provided batch nor the mutable pending clock projection is used.
func verifyPublishedResourceSystem(ctx context.Context,q queryOne,run,cycle,commit [32]byte,
 journal,fence []any,eventRaw []any)([]domain.ID,error){
 units,ok:=fence[11].([]any)
 if !ok||len(units)<2{return nil,errors.New("resource-system replay requires full mixed fence")}
 slotCount:=0
 for _,raw:=range units{
  entry,ok:=raw.([]any)
  if !ok||len(entry)!=5{return nil,errors.New("resource-system replay admitted source malformed")}
  if entry[1]!=uint64(0){break}
  slotCount++
 }
 if slotCount<1||slotCount>2||slotCount>=len(units){
  return nil,errors.New("resource-system replay needs one or two contestant slots and pending system sources")
 }
 names,ok:=journal[16].([]any)
 if !ok||len(names)!=len(units){return nil,errors.New("resource-system decision roster incomplete")}
 decisions:=make([]ports.CanonicalRecord,len(units))
 candidates:=make([]ports.CanonicalRecord,0)
 successors:=make([]ports.CanonicalRecord,0)
 sources:=make([]domain.ID,len(units))
 var resourceCandidate ports.CanonicalRecord
 for i,entryRaw:=range units{
  entry,ok:=entryRaw.([]any)
  if !ok||len(entry)!=5{return nil,errors.New("replayed resource-system unit malformed")}
  id,ok:=entry[3].([]byte)
  if !ok||len(id)!=32{return nil,errors.New("replayed resource-system source ID invalid")}
  copy(sources[i][:],id)
  did,ok:=names[i].([]byte)
  if !ok||len(did)!=32{return nil,errors.New("replayed resource-system DecisionRecord ID malformed")}
  var raw []byte
  if err:=q.QueryRowContext(ctx,"SELECT canonical FROM decision_ledger WHERE id=?",did).Scan(&raw);err!=nil{return nil,err}
  parsed,err:=codec.StrictDecodePayload(raw);if err!=nil{return nil,err}
  body,ok:=parsed.([]any)
  if !ok||len(body)!=16{return nil,errors.New("replayed resource-system decision malformed")}
  var key [32]byte;copy(key[:],did)
  decisions[i]=ports.CanonicalRecord{Identity:key,Canonical:raw}
  choice,ok:=body[6].([]any)
  if !ok||len(choice)!=2||choice[0]!=uint64(1){
   return nil,errors.New("resource-system decision lost authenticated candidate")
  }
  cid,ok:=choice[1].([]byte)
  if !ok||len(cid)!=32{return nil,errors.New("resource-system candidate ID invalid")}
  var candRaw []byte
  if err:=q.QueryRowContext(ctx,"SELECT canonical FROM decision_ledger WHERE id=?",cid).Scan(&candRaw);err!=nil{return nil,err}
  var ckey [32]byte;copy(ckey[:],cid)
  candidate:=ports.CanonicalRecord{Identity:ckey,Canonical:candRaw}
  if i==0{resourceCandidate=candidate}
  if i<slotCount{continue}
  candidates=append(candidates,candidate)
  option,ok:=body[13].([]any)
  if !ok{return nil,errors.New("resource-system deferred successor option malformed")}
  if len(option)==2&&option[0]==uint64(1){
   childID,ok:=option[1].([]byte)
   if !ok||len(childID)!=32{return nil,errors.New("resource-system deferred child ID missing")}
   var childRaw []byte
   if err:=q.QueryRowContext(ctx,"SELECT canonical FROM scheduled_occurrences WHERE run_id=? AND id=?",run[:],childID).Scan(&childRaw);err!=nil{return nil,err}
   var childKey [32]byte;copy(childKey[:],childID)
   successors=append(successors,ports.CanonicalRecord{Identity:childKey,Canonical:childRaw})
  }else if len(option)!=1||option[0]!=uint64(0){
   return nil,errors.New("resource-system unregistered successor option")
  }
 }
 if len(candidates)==0{return nil,errors.New("resource-system replay lacks system candidate")}
 eventIDs,ok:=journal[15].([]any)
 if !ok||len(eventIDs)!=len(eventRaw){return nil,errors.New("resource-system replay event roster incomplete")}
 events:=make([]ports.CanonicalRecord,len(eventRaw))
 for i,v:=range eventIDs{
  eventID,ok:=v.([]byte)
  raw,valid:=eventRaw[i].([]byte)
  if !ok||!valid||len(eventID)!=32{return nil,errors.New("resource-system replay malformed Event ID")}
  var k [32]byte;copy(k[:],eventID)
  events[i]=ports.CanonicalRecord{Identity:k,Canonical:raw}
 }
 base,ok:=journal[6].(uint64)
 if !ok||base==math.MaxUint64{return nil,errors.New("resource-system predecessor revision malformed")}
 var oldRaw,newRaw []byte
 if err:=q.QueryRowContext(ctx,"SELECT canonical FROM world_revisions WHERE revision=?",base).Scan(&oldRaw);err!=nil{return nil,err}
 if err:=q.QueryRowContext(ctx,"SELECT canonical FROM world_revisions WHERE revision=?",base+1).Scan(&newRaw);err!=nil{return nil,err}
 decodedBase,err:=codec.StrictDecodePayload(oldRaw);if err!=nil{return nil,err}
 decodedNext,err:=codec.StrictDecodePayload(newRaw);if err!=nil{return nil,err}
 predecessor,ok:=decodedBase.([]any)
 if !ok||len(predecessor)!=6{return nil,errors.New("resource-system predecessor WorldRevision malformed")}
 nextRevision,ok:=decodedNext.([]any)
 if !ok||len(nextRevision)!=6{return nil,errors.New("resource-system next WorldRevision malformed")}
 stateHash,ok:=predecessor[5].([]byte)
 if !ok||len(stateHash)!=32{return nil,errors.New("resource-system predecessor hash invalid")}
 var genesisRaw []byte
 if err:=q.QueryRowContext(ctx,"SELECT canonical FROM genesis_records WHERE run_id=?",run[:]).Scan(&genesisRaw);err!=nil{return nil,err}
 envelope,err:=codec.StrictDecodeEnvelope(genesisRaw);if err!=nil{return nil,err}
 manifest,ok:=envelope.Value.([]any)
 if !ok||len(manifest)!=18{return nil,errors.New("resource-system Genesis missing")}
 refs,ok:=manifest[14].([]any)
 if !ok{return nil,errors.New("resource-system Genesis policy refs missing")}
 allowed:=map[string]any{}
 for _,entry:=range refs{
  ref,ok:=entry.([]any)
  if !ok||len(ref)!=3{return nil,errors.New("resource-system Genesis policy malformed")}
  name,ok:=ref[0].(string)
  if !ok||allowed[name]!=nil{return nil,errors.New("resource-system duplicate Genesis policy")}
  allowed[name]=ref
 }
 instant,ok:=asSigned64(journal[3])
 if !ok{return nil,errors.New("resource-system logical instant missing")}
 ordinal,ok:=journal[4].(uint64)
 if !ok||ordinal>math.MaxInt64{return nil,errors.New("resource-system ordinal overflow")}
 end,ok:=journal[22].(uint64)
 if !ok||end<uint64(len(events)){return nil,errors.New("resource-system event sequence underflow")}
 before:=clock.State{Instant:domain.SimulationInstant(instant),Revision:domain.WorldRevision(base),
  NextLogicalSequence:domain.LogicalSequence(end-uint64(len(events))),LastOrdinal:int64(ordinal)-1}
 after:=before;after.Revision++;after.LastOrdinal=int64(ordinal);after.NextLogicalSequence=domain.LogicalSequence(end)
 var hash [32]byte;copy(hash[:],stateHash)
 mode:=uint8(5)
 if slotCount==2{mode=6}
 batch:=ports.WorkBatch{Mode:mode,RunID:run,CycleID:cycle,ResourceCandidate:resourceCandidate,
  SystemCandidate:candidates[0],SystemExtraCandidates:candidates[1:],SystemSuccessors:successors,
  Events:events,Decisions:decisions,RevisionCanonical:newRaw,
  BaseStateHash:hash,Journal:ports.CanonicalRecord{Identity:commit}}
 resultHash,ok:=nextRevision[5].([]byte)
 if !ok||len(resultHash)!=32{return nil,errors.New("resource-system result hash invalid")}
 copy(batch.ResultStateHash[:],resultHash)
 // Rebuild each complete resource RoundDeclaration from its authenticated
 // ActionProposal; it is not safe to infer the round from the decision list.
 expectedSlots:=map[domain.ID]map[string]bool{}
 for i:=0;i<slotCount;i++{
  var sourceRaw []byte
  if err:=q.QueryRowContext(ctx,"SELECT canonical FROM input_ledger WHERE id=?",sources[i][:]).Scan(&sourceRaw);err!=nil{return nil,err}
  rawProposal,err:=codec.StrictDecodePayload(sourceRaw);if err!=nil{return nil,err}
  proposal,ok:=rawProposal.([]any)
  if !ok||len(proposal)!=19{return nil,errors.New("resource-system source not ActionProposal")}
  roundID,ok:=proposal[13].([]byte)
  if !ok||len(roundID)!=32{return nil,errors.New("resource-system proposal RoundDeclaration missing")}
  slotName,ok:=proposal[14].(string)
  if !ok||slotName==""{return nil,errors.New("resource-system ActionProposal slot missing")}
  var roundRaw []byte
  if err:=q.QueryRowContext(ctx,"SELECT canonical FROM input_ledger WHERE id=?",roundID).Scan(&roundRaw);err!=nil{return nil,err}
  rawRound,err:=codec.StrictDecodePayload(roundRaw);if err!=nil{return nil,err}
  roundFields,ok:=rawRound.([]any)
  if !ok||len(roundFields)!=11{return nil,errors.New("resource-system RoundDeclaration malformed")}
  slots,ok:=roundFields[4].([]any)
  if !ok||len(slots)!=1||slots[0]!=slotName{return nil,errors.New("resource-system RoundDeclaration did not authorize exact sole slot")}
  var rid domain.ID;copy(rid[:],roundID)
  if expectedSlots[rid]!=nil{return nil,errors.New("resource-system contest repeats a RoundDeclaration")}
  expectedSlots[rid]=map[string]bool{slotName:true}
 }
 if slotCount==2{
  proof,err:=replayContestProofSet(ctx,q,run,cycle,sources[:2])
  if err!=nil{return nil,err}
  batch.ContestProofs=proof
 }
 if err:=verifyResourceSystemTerminal(ctx,q,batch,before,after,fence,journal,
  predecessor,nextRevision,manifest,allowed,expectedSlots);err!=nil{return nil,err}
 return sources,nil
}

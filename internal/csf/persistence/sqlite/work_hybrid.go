package sqlite

import (
 "bytes"
 "context"
 "errors"
 "sort"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/clock"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/settlement"
)

type hybridSlotProof struct{slot string;id,digest []byte}

// verifyHybridTerminal is a re-derivation of every slot NoProposal and each
// system handler in one shared fence. The EventOrderKeys and logical sequences
// belong to the WHOLE batch, not independently replayed sub-transactions.
func verifyHybridTerminal(ctx context.Context,q queryOne,b ports.WorkBatch,
 prior,next clock.State,fence,journal,before,revision []any,allowed map[string]any,
 expectedSlots map[domain.ID]map[string]bool)error{
 units,ok:=fence[11].([]any)
 if !ok||len(units)!=len(b.Decisions){return errors.New("hybrid decision count differs from admission")}
 if !sameCanonical(revision[4],before[4])||!sameCanonical(revision[5],before[5]){
  return errors.New("hybrid source-only workload changed WorldState")
 }
 refs,ok:=journal[15].([]any)
 if !ok||len(refs)!=len(b.Events){return errors.New("hybrid event references missing")}
 eventOf:=map[string]int{}
 previous:=[]byte(nil)
 for i,event:=range b.Events{
  fields,err:=strictFull(event,19);if err!=nil{return err}
  key,ok:=fields[2].([]any)
  if !ok||len(key)!=6||key[0]!=uint64(2){return errors.New("hybrid SOURCE_LIFECYCLE EventOrderKey malformed")}
  origin,ok:=key[1].([]byte)
  if !ok||len(origin)!=32{return errors.New("hybrid Event owner ID malformed")}
  if i>0&&bytes.Compare(previous,origin)>=0{return errors.New("hybrid event order is not strict canonical origin ID")}
  previous=origin
  sequence,ok:=fields[6].(uint64)
  if !ok||sequence!=uint64(prior.NextLogicalSequence)+uint64(i){
   return errors.New("hybrid logical event sequences are not contiguous")
  }
  if !sameCanonical(refs[i],event.Identity[:]){return errors.New("hybrid journal EventID order differs from validated Event")}
  if _,found:=eventOf[string(origin)];found{return errors.New("duplicate hybrid Event owner")}
  eventOf[string(origin)]=i
 }
 pairs:=make([]any,len(units))
 covered:=map[domain.ID]map[string]bool{}
 byRound:=map[domain.ID][]hybridSlotProof{}
 var systems,slots int
 usedEvents:=map[int]bool{}
 usedChildren:=map[[32]byte]bool{}
 candidates:=append([]ports.CanonicalRecord{b.SystemCandidate},b.SystemExtraCandidates...)
 sysIndex:=0
 for i,unitRaw:=range units{
  unit,ok:=unitRaw.([]any)
  if !ok||len(unit)!=5{return errors.New("hybrid admitted source malformed")}
  kind,ok:=unit[1].(uint64);if !ok{return errors.New("hybrid source kind malformed")}
  rawID,ok:=unit[3].([]byte);if !ok||len(rawID)!=32{return errors.New("hybrid source ID malformed")}
  digest,ok:=unit[4].([]byte);if !ok||len(digest)!=32{return errors.New("hybrid source digest malformed")}
  dr,err:=strictFull(b.Decisions[i],16);if err!=nil{return err}
  if !sameCanonical(journal[16].([]any)[i],b.Decisions[i].Identity[:]){
   return errors.New("hybrid DecisionRecord list order changed")
  }
  pairHash,err:=registeredDigest("cote.csf.digest.decision-record","cote.csf.schema.decision-record.body",dr[:15])
  if err!=nil||!equalBytes(dr[15],pairHash[:]){return errors.New("hybrid DecisionRecord digest not authentic")}
  pairs[i]=[]any{rawID,b.Decisions[i].Identity[:],pairHash[:]}
  if kind==0{
   slots++
   var raw []byte
   if err:=q.QueryRowContext(ctx,"SELECT canonical FROM input_ledger WHERE id=?",rawID).Scan(&raw);err!=nil{return err}
   decoded,err:=codec.StrictDecodePayload(raw);if err!=nil{return err}
   response,ok:=decoded.([]any)
   if !ok||len(response)!=11||!equalBytes(response[0],rawID)||
    !equalBytes(response[2],b.RunID[:])||!equalBytes(response[3],b.CycleID[:])||
    response[8]!=uint64(prior.Revision){return errors.New("hybrid NoProposal source malformed")}
   roundRaw,ok:=response[4].([]byte);if !ok||len(roundRaw)!=32{return errors.New("hybrid round ID invalid")}
   slot,ok:=response[5].(string);if !ok{return errors.New("hybrid slot ID invalid")}
   var round domain.ID;copy(round[:],roundRaw)
   if !expectedSlots[round][slot]{return errors.New("hybrid NoProposal belongs to unadmitted round slot")}
   if covered[round]==nil{covered[round]=map[string]bool{}}
   if covered[round][slot]{return errors.New("hybrid round slot settled twice")}
   covered[round][slot]=true
   byRound[round]=append(byRound[round],hybridSlotProof{slot:slot,id:rawID,digest:digest})
   wantDigest,err:=registeredDigest("cote.csf.digest.no-proposal-unit",
    "cote.csf.schema.no-proposal.body",[]any{response[:10]});if err!=nil{return err}
   wantID,err:=registeredDigest("cote.csf.id.no-proposal",
    "cote.csf.schema.slot-response-id-preimage",[]any{response[1],uint64(1)});if err!=nil{return err}
   if !bytes.Equal(wantID[:],rawID)||!bytes.Equal(wantDigest[:],digest)||!equalBytes(response[10],digest){
    return errors.New("hybrid NoProposal identity or digest mismatches fence")
   }
   expectedID,err:=registeredDigest("cote.csf.id.decision","cote.csf.schema.decision-id-preimage",
    []any{b.CycleID[:],journal[2],rawID});if err!=nil{return err}
   want:=[]any{expectedID[:],b.RunID[:],b.CycleID[:],journal[2],
    []any{uint64(0),uint64(1),rawID,digest},uint64(3),noRef(),
    "slot.no-proposal",[]any{},allowed["validators-v1"],allowed["conflict-resolvers-v1"],
    []any{},[]any{},noRef(),noRef()}
   if !sameCanonical(dr[:15],want)||b.Decisions[i].Identity!=[32]byte(expectedID){
    return errors.New("hybrid slot DecisionRecord differs from authenticated NoProposal")
   }
   continue
  }
  if kind!=2&&kind!=3{return errors.New("hybrid handler cannot claim unregistered source kind")}
  systems++
  index,found:=eventOf[string(rawID)]
  if !found{return errors.New("hybrid system source has no lifecycle Event")}
  usedEvents[index]=true
  if sysIndex>=len(candidates){return errors.New("hybrid system missing candidate evidence")}
  opt,ok:=dr[13].([]any);if !ok{return errors.New("hybrid successor ref malformed")}
  successors:=[]ports.CanonicalRecord{}
  if len(opt)==2&&opt[0]==uint64(1){
   id,ok:=opt[1].([]byte);if !ok||len(id)!=32{return errors.New("hybrid successor identity invalid")}
   for _,record:=range b.SystemSuccessors{
    if bytes.Equal(record.Identity[:],id){
     if usedChildren[record.Identity]{return errors.New("hybrid successor referenced twice")}
     usedChildren[record.Identity]=true;successors=append(successors,record)
    }
   }
   if len(successors)!=1{return errors.New("hybrid DEFER successor missing")}
  }else if len(opt)!=1||opt[0]!=uint64(0){return errors.New("hybrid successor option unregistered")}
  subFence:=append([]any(nil),fence...)
  subFence[11]=[]any{unit}
  subJournal:=append([]any(nil),journal...)
  subJournal[15]=[]any{b.Events[index].Identity[:]}
  subJournal[16]=[]any{b.Decisions[i].Identity[:]}
  subPair,err:=registeredDigest("cote.csf.digest.decision","cote.csf.schema.decision-pair-list",
   []any{pairs[i]});if err!=nil{return err}
  subJournal[17]=subPair[:]
  eventFields,err:=strictFull(b.Events[index],19);if err!=nil{return err}
  sequence:=eventFields[6].(uint64)
  subPrior,subNext:=prior,next
  subPrior.NextLogicalSequence=domain.LogicalSequence(sequence)
  subNext.NextLogicalSequence=domain.LogicalSequence(sequence+1)
  sub:=b
  sub.SystemCandidate=candidates[sysIndex]
  sub.SystemExtraCandidates=nil
  sub.SystemSuccessors=successors
  sub.Decisions=[]ports.CanonicalRecord{b.Decisions[i]}
  sub.Events=[]ports.CanonicalRecord{b.Events[index]}
  if err:=verifySystemTerminal(ctx,q,sub,subPrior,subNext,subFence,subJournal,
   before,revision,allowed,map[domain.ID]map[string]bool{});err!=nil{return err}
  sysIndex++
 }
 if systems==0||slots==0||sysIndex!=len(candidates){
  return errors.New("hybrid terminal must close both source classes and every candidate")
 }
 if len(usedChildren)!=len(b.SystemSuccessors){return errors.New("hybrid surplus successor without DEFER decision")}
 for round,members:=range expectedSlots{
  if len(covered[round])!=len(members){return errors.New("hybrid round was only partially settled")}
  refs:=byRound[round]
  sort.Slice(refs,func(i,j int)bool{return refs[i].slot<refs[j].slot})
  list:=make([]any,len(refs))
  for i,v:=range refs{list[i]=[]any{uint64(0),uint64(1),v.id,v.digest}}
  index,ok:=eventOf[string(round[:])]
  if !ok{return errors.New("hybrid round has no lifecycle Event")}
  usedEvents[index]=true
  expected,err:=settlement.ConsumedRoundEvent(domain.ID(b.RunID),domain.ID(b.CycleID),round,
   prior.Instant,prior.Revision,prior.NextLogicalSequence+domain.LogicalSequence(index),list)
  if err!=nil{return err}
  if expected.Identity!=b.Events[index].Identity||!bytes.Equal(expected.Canonical,b.Events[index].Canonical){
   return errors.New("hybrid round lifecycle Event does not match admitted slot source roots")
  }
 }
 if len(usedEvents)!=len(b.Events){return errors.New("hybrid published orphaned Event")}
 wantPairs,err:=registeredDigest("cote.csf.digest.decision","cote.csf.schema.decision-pair-list",pairs)
 if err!=nil||!equalBytes(wantPairs[:],journal[17]){return errors.New("hybrid decision digest differs from validated causal partition")}
 tasks,err:=registeredDigest("cote.csf.digest.perception-task-list","cote.csf.schema.perception-task-id-list",[]any{})
 if err!=nil||!equalBytes(tasks[:],journal[19]){return errors.New("hybrid private events unexpectedly created visible tasks")}
 return nil
}

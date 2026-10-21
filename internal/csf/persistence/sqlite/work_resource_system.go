package sqlite

import (
 "bytes"
 "context"
 "errors"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/clock"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
)

// verifyResourceSystemTerminal reduces exactly one resource and consumes
// every co-admitted scheduled/trigger source within one immutable fence.
// Each sub-proof reuses the existing independent SQL verifiers, while the
// global journal enforces complete one-to-one source/event/decision partition.
func verifyResourceSystemTerminal(ctx context.Context,q queryOne,b ports.WorkBatch,
 prior,next clock.State,fence,journal,predecessor,revision []any,
 allowed map[string]any,expectedSlots map[domain.ID]map[string]bool)error{
 units,ok:=fence[11].([]any)
 if !ok||len(units)<2||len(b.Decisions)!=len(units)||
  len(b.Events)!=len(units)+1||
  len(b.SystemExtraCandidates)!=len(units)-2||
  len(expectedSlots)!=1{return errors.New("mixed resource/system source/decision/event bijection invalid")}
 primary,ok:=units[0].([]any)
 if !ok||len(primary)!=5||primary[1]!=uint64(0){return errors.New("resource-system first admitted unit must be resource slot")}
 primaryID,ok:=primary[3].([]byte)
 if !ok||len(primaryID)!=32{return errors.New("resource-system proposal ID malformed")}
 ids,ok:=journal[15].([]any)
 if !ok||len(ids)!=len(b.Events){return errors.New("resource-system Event IDs differ from journal")}
 var prev []byte
 indexed:=map[string]int{}
 for i,event:=range b.Events{
  full,err:=strictFull(event,19);if err!=nil{return err}
  key,ok:=full[2].([]any)
  if !ok||len(key)!=6{return errors.New("resource-system EventOrderKey malformed")}
  phase,ok:=key[0].(uint64)
  if !ok||((i==0&&phase!=1)||(i>0&&phase!=2)){
   return errors.New("resource-system Events not sorted by causal phase")
  }
  source,ok:=key[1].([]byte)
  if !ok||len(source)!=32{return errors.New("resource-system lifecycle origin malformed")}
  if i>1&&bytes.Compare(prev,source)>=0{
   return errors.New("resource-system lifecycle order is not strict by canonical origin")
  }
  prev=source
  if i>0{if _,duplicate:=indexed[string(source)];duplicate{return errors.New("resource-system duplicate lifecycle owner")};indexed[string(source)]=i}
  seq,ok:=full[6].(uint64)
  if !ok||seq!=uint64(prior.NextLogicalSequence)+uint64(i){
   return errors.New("resource-system Event logical sequences not contiguous")
  }
  if !sameCanonical(ids[i],event.Identity[:]){return errors.New("resource-system journal order differs from event batch")}
 }
 resourceChoice,err:=strictFull(b.Decisions[0],16);if err!=nil{return err}
 resourceRound:=map[domain.ID]bool{}
 for round:=range expectedSlots{resourceRound[round]=true}
 if len(resourceRound)!=1{return errors.New("resource-system expected one complete resource round")}
 var round domain.ID
 for key:=range resourceRound{round=key}
 roundIndex,ok:=indexed[string(round[:])]
 if !ok{return errors.New("resource-system omitted consumed resource RoundDeclaration")}
 subFence:=append([]any(nil),fence...)
 subFence[11]=[]any{primary}
 subJournal:=append([]any(nil),journal...)
 subJournal[11]=[]any{primaryID}
 subJournal[15]=[]any{b.Events[0].Identity[:],b.Events[roundIndex].Identity[:]}
 subJournal[16]=[]any{b.Decisions[0].Identity[:]}
 h,err:=registeredDigest("cote.csf.digest.decision-record","cote.csf.schema.decision-record.body",resourceChoice[:15])
 if err!=nil{return err}
 pairing:=[]any{[]any{primaryID,b.Decisions[0].Identity[:],h[:]}}
 digest,err:=registeredDigest("cote.csf.digest.decision","cote.csf.schema.decision-pair-list",pairing)
 if err!=nil{return err}
 subJournal[17]=digest[:]
 resourceBatch:=b
 resourceBatch.Events=[]ports.CanonicalRecord{b.Events[0],b.Events[roundIndex]}
 resourceBatch.Decisions=[]ports.CanonicalRecord{b.Decisions[0]}
 resourceNext:=next
 resourceNext.NextLogicalSequence=prior.NextLogicalSequence+2
 if err:=verifySingleResourceTerminal(ctx,q,resourceBatch,prior,resourceNext,
  subFence,subJournal,predecessor,revision,allowed,expectedSlots);err!=nil{return err}
 candidates:=append([]ports.CanonicalRecord{b.SystemCandidate},b.SystemExtraCandidates...)
 pairs:=make([]any,len(units))
 pairs[0]=pairing[0]
 usedEvents:=map[int]bool{0:true,roundIndex:true}
 usedSuccessors:=map[[32]byte]bool{}
 for i:=1;i<len(units);i++{
  unit,ok:=units[i].([]any)
  if !ok||len(unit)!=5{return errors.New("resource-system unit malformed")}
  kind,ok:=unit[1].(uint64)
  if !ok||(kind!=2&&kind!=3){return errors.New("resource-system cannot silently consume non-system slot")}
  sourceID,ok:=unit[3].([]byte)
  if !ok||len(sourceID)!=32{return errors.New("resource-system source ID malformed")}
  eventIndex,ok:=indexed[string(sourceID)]
  if !ok||usedEvents[eventIndex]{return errors.New("resource-system missing or multiply assigned lifecycle Event")}
  usedEvents[eventIndex]=true
  decisionFields,err:=strictFull(b.Decisions[i],16);if err!=nil{return err}
  opt,ok:=decisionFields[13].([]any)
  if !ok{return errors.New("resource-system successor option invalid")}
  children:=[]ports.CanonicalRecord{}
  if len(opt)==2&&opt[0]==uint64(1){
   childID,ok:=opt[1].([]byte)
   if !ok||len(childID)!=32{return errors.New("resource-system successor ID invalid")}
   for _,c:=range b.SystemSuccessors{
    if bytes.Equal(c.Identity[:],childID){
     if usedSuccessors[c.Identity]{return errors.New("resource-system duplicate child")}
     usedSuccessors[c.Identity]=true
     children=append(children,c)
    }
   }
   if len(children)!=1{return errors.New("resource-system DEFER child absent")}
  }else if len(opt)!=1||opt[0]!=uint64(0){return errors.New("resource-system unsupported child option")}
  dh,err:=registeredDigest("cote.csf.digest.decision-record","cote.csf.schema.decision-record.body",decisionFields[:15])
  if err!=nil{return err}
  pairs[i]=[]any{sourceID,b.Decisions[i].Identity[:],dh[:]}
  pd,err:=registeredDigest("cote.csf.digest.decision","cote.csf.schema.decision-pair-list",[]any{pairs[i]})
  if err!=nil{return err}
  oneFence:=append([]any(nil),fence...)
  oneFence[11]=[]any{unit}
  oneJournal:=append([]any(nil),journal...)
  oneJournal[11]=[]any{sourceID}
  oneJournal[15]=[]any{b.Events[eventIndex].Identity[:]}
  oneJournal[16]=[]any{b.Decisions[i].Identity[:]}
  oneJournal[17]=pd[:]
  single:=b
  single.SystemCandidate=candidates[i-1]
  single.SystemExtraCandidates=nil
  single.SystemSuccessors=children
  single.Events=[]ports.CanonicalRecord{b.Events[eventIndex]}
  single.Decisions=[]ports.CanonicalRecord{b.Decisions[i]}
  prevState,nextState:=prior,next
  prevState.NextLogicalSequence=prior.NextLogicalSequence+domain.LogicalSequence(eventIndex)
  nextState.NextLogicalSequence=prevState.NextLogicalSequence+1
  if err:=verifySystemTerminal(ctx,q,single,prevState,nextState,oneFence,
   oneJournal,predecessor,revision,allowed,map[domain.ID]map[string]bool{});err!=nil{return err}
 }
 if len(usedEvents)!=len(b.Events)||len(usedSuccessors)!=len(b.SystemSuccessors){
  return errors.New("resource-system omitted an event or invented successor")
 }
 total,err:=registeredDigest("cote.csf.digest.decision","cote.csf.schema.decision-pair-list",pairs)
 if err!=nil||!sameCanonical(journal[17],total[:]){
  return errors.New("resource-system total decision digest differs from complete fence")
 }
 if next.NextLogicalSequence!=prior.NextLogicalSequence+domain.LogicalSequence(len(b.Events)){
  return errors.New("resource-system terminal did not advance contiguous global event sequence")
 }
 return nil
}

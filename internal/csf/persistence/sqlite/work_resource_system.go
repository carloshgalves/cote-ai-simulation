package sqlite

import (
 "bytes"
 "sort"
 "context"
 "errors"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/clock"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
)

// verifyResourceSystemTerminal reduces exactly one resource and consumes
// every co-admitted scheduled/trigger source within one immutable fence.
// Each sub-proof reuses the existing independent SQL verifiers, while the
// global journal enforces complete one-to-one source/event/decision partition.
func verifyResourceSystemTerminal(ctx context.Context,q queryOne,b ports.WorkBatch,
 prior,next clock.State,fence,journal,predecessor,revision []any,
 genesis []any,allowed map[string]any,expectedSlots map[domain.ID]map[string]bool)error{
 units,ok:=fence[11].([]any)
 if !ok||len(units)<2{return errors.New("mixed resource/system source list invalid")}
 slots:=1
 if b.Mode==6{slots=2}else if b.Mode!=5{return errors.New("unregistered resource-system terminal mode")}
 if len(units)<=slots||len(b.Decisions)!=len(units)||
  len(b.Events)!=len(units)+1||
  len(b.SystemExtraCandidates)!=len(units)-slots-1||
  len(expectedSlots)!=slots{return errors.New("mixed resource/system source/decision/event bijection invalid")}
 for i:=0;i<slots;i++{
  unit,ok:=units[i].([]any)
  if !ok||len(unit)!=5||unit[1]!=uint64(0){return errors.New("resource-system first units must be resource slots")}
 }
 ids,ok:=journal[15].([]any)
 if !ok||len(ids)!=len(b.Events){return errors.New("resource-system Event IDs differ from journal")}
 decisions,ok:=journal[16].([]any)
 if !ok||len(decisions)!=len(b.Decisions){return errors.New("resource-system DecisionRecord references missing")}
 for i,item:=range b.Decisions{
  if !sameCanonical(decisions[i],item.Identity[:]){
   return errors.New("resource-system journal omitted or reordered terminal decision")
  }
 }
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
 roundIDs:=make([]domain.ID,0,len(expectedSlots))
 for round:=range expectedSlots{roundIDs=append(roundIDs,round)}
 sort.Slice(roundIDs,func(i,j int)bool{return bytes.Compare(roundIDs[i][:],roundIDs[j][:])<0})
 roundEvents:=make([]ports.CanonicalRecord,0,slots)
 resourceIDs:=make([]any,slots)
 resourceDecisionIDs:=make([]any,slots)
 pairing:=make([]any,slots)
 usedEvents:=map[int]bool{0:true}
 for i:=0;i<slots;i++{
  index,ok:=indexed[string(roundIDs[i][:])]
  if !ok||usedEvents[index]{return errors.New("resource-system omitted or duplicated consumed resource round")}
  usedEvents[index]=true
  roundEvents=append(roundEvents,b.Events[index])
  unit:=units[i].([]any)
  source,ok:=unit[3].([]byte)
  if !ok||len(source)!=32{return errors.New("resource-system slot ID invalid")}
  decision,err:=strictFull(b.Decisions[i],16)
  if err!=nil{return err}
  h,err:=registeredDigest("cote.csf.digest.decision-record","cote.csf.schema.decision-record.body",decision[:15])
  if err!=nil{return err}
  resourceIDs[i]=source
  resourceDecisionIDs[i]=b.Decisions[i].Identity[:]
  pairing[i]=[]any{source,b.Decisions[i].Identity[:],h[:]}
 }
 subFence:=append([]any(nil),fence...)
 subFence[11]=append([]any(nil),units[:slots]...)
 subJournal:=append([]any(nil),journal...)
 subJournal[11]=resourceIDs
 subJournal[15]=make([]any,slots+1)
 subJournal[15].([]any)[0]=b.Events[0].Identity[:]
 for i,event:=range roundEvents{subJournal[15].([]any)[i+1]=event.Identity[:]}
 subJournal[16]=resourceDecisionIDs
 digest,err:=registeredDigest("cote.csf.digest.decision","cote.csf.schema.decision-pair-list",pairing)
 if err!=nil{return err}
 subJournal[17]=digest[:]
 subJournal[22]=uint64(prior.NextLogicalSequence)+uint64(slots+1)
 resourceBatch:=b
 resourceBatch.Events=append([]ports.CanonicalRecord{b.Events[0]},roundEvents...)
 resourceBatch.Decisions=append([]ports.CanonicalRecord(nil),b.Decisions[:slots]...)
 resourceNext:=next
 resourceNext.NextLogicalSequence=prior.NextLogicalSequence+domain.LogicalSequence(slots+1)
 if slots==1{
  if err:=verifySingleResourceTerminal(ctx,q,resourceBatch,prior,resourceNext,
   subFence,subJournal,predecessor,revision,allowed,expectedSlots);err!=nil{return err}
 }else{
  if err:=verifyTwoResourceTerminal(ctx,q,resourceBatch,prior,resourceNext,
   subFence,subJournal,predecessor,revision,genesis,allowed,expectedSlots);err!=nil{return err}
 }
 candidates:=append([]ports.CanonicalRecord{b.SystemCandidate},b.SystemExtraCandidates...)
 pairs:=make([]any,len(units))
 copy(pairs,pairing)
 usedSuccessors:=map[[32]byte]bool{}
 for i:=slots;i<len(units);i++{
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
  single.SystemCandidate=candidates[i-slots]
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

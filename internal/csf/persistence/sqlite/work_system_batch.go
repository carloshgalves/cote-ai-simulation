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

func verifySystemBatch(ctx context.Context,q queryOne,b ports.WorkBatch,
 prior,next clock.State,fence,journal,predecessor,revision []any,
 allowed map[string]any,expectedSlots map[domain.ID]map[string]bool)error{
 if len(expectedSlots)!=0{return errors.New("mixed slot/system terminals require explicit combined handler")}
 units,ok:=fence[11].([]any)
 if !ok||len(units)==0{return errors.New("system WORK missing source units")}
 if len(units)==1{
  if len(b.SystemExtraCandidates)!=0{return errors.New("single system WORK invented extra candidate")}
  return verifySystemTerminal(ctx,q,b,prior,next,fence,journal,predecessor,revision,allowed,expectedSlots)
 }
 n:=len(units)
 if len(b.Events)!=n||len(b.Decisions)!=n||len(b.SystemExtraCandidates)!=n-1{
  return errors.New("multi-source WORK is not bijective with candidate, Event and DecisionRecord")
 }
 candidates:=append([]ports.CanonicalRecord{b.SystemCandidate},b.SystemExtraCandidates...)
 events:=map[string]int{}
 var previous []byte
 for j,item:=range b.Events{
  fields,err:=strictFull(item,19);if err!=nil{return err}
  key,ok:=fields[2].([]any)
  if !ok||len(key)!=6||key[0]!=uint64(2){return errors.New("system EventOrderKey malformed")}
  source,ok:=key[1].([]byte)
  if !ok||len(source)!=32{return errors.New("system event origin ID missing")}
  if j>0&&bytes.Compare(previous,source)>=0{return errors.New("system EventOrderKeys not in strict canonical source order")}
  previous=source
  events[string(source)]=j
 }
 pairs:=make([]any,0,n)
 usedSuccessor:=map[[32]byte]bool{}
 for i,unitRaw:=range units{
  unit,ok:=unitRaw.([]any)
  if !ok||len(unit)!=5{return errors.New("system admitted source malformed")}
  sourceID,ok:=unit[3].([]byte)
  if !ok||len(sourceID)!=32{return errors.New("system source ID missing")}
  eventIndex,ok:=events[string(sourceID)]
  if !ok{return errors.New("missing lifecycle Event for admitted system source")}
  decision,err:=strictFull(b.Decisions[i],16);if err!=nil{return err}
  ref,ok:=decision[4].([]any)
  if !ok||len(ref)!=3||!equalBytes(ref[1],sourceID)||!equalBytes(ref[2],unit[4].([]byte)){
   return errors.New("multi-system DecisionRecord subject not equal to fence unit")
  }
  childOpt,ok:=decision[13].([]any)
  if !ok{return errors.New("multi-system successor ref malformed")}
  var child []ports.CanonicalRecord
  if len(childOpt)==2&&childOpt[0]==uint64(1){
   id,ok:=childOpt[1].([]byte);if !ok||len(id)!=32{return errors.New("multi-system successor ID invalid")}
   for _,candidate:=range b.SystemSuccessors{
    if bytes.Equal(candidate.Identity[:],id){
     if usedSuccessor[candidate.Identity]{return errors.New("multi-system successor duplicated")}
     usedSuccessor[candidate.Identity]=true
     child=append(child,candidate)
    }
   }
   if len(child)!=1{return errors.New("missing successor referenced by DEFER DecisionRecord")}
  }else if len(childOpt)!=1||childOpt[0]!=uint64(0){return errors.New("multi-system successor option invalid")}
  pair,err:=registeredDigest("cote.csf.digest.decision","cote.csf.schema.decision-pair-list",
   []any{[]any{sourceID,b.Decisions[i].Identity[:],decision[15]}})
  if err!=nil{return err}
  subFence:=append([]any(nil),fence...)
  subFence[11]=[]any{unit}
  subJournal:=append([]any(nil),journal...)
  subJournal[15]=[]any{b.Events[eventIndex].Identity[:]}
  subJournal[16]=[]any{b.Decisions[i].Identity[:]}
  subJournal[17]=pair[:]
  sub:=b
  sub.SystemCandidate=candidates[i]
  sub.SystemExtraCandidates=nil
  sub.SystemSuccessors=child
  sub.Events=[]ports.CanonicalRecord{b.Events[eventIndex]}
  sub.Decisions=[]ports.CanonicalRecord{b.Decisions[i]}
  subPrior,subNext:=prior,next
  subPrior.NextLogicalSequence+=domain.LogicalSequence(eventIndex)
  subNext.NextLogicalSequence=subPrior.NextLogicalSequence+1
  if err:=verifySystemTerminal(ctx,q,sub,subPrior,subNext,subFence,subJournal,
   predecessor,revision,allowed,expectedSlots);err!=nil{return err}
  pairs=append(pairs,[]any{sourceID,b.Decisions[i].Identity[:],decision[15]})
 }
 if len(usedSuccessor)!=len(b.SystemSuccessors){return errors.New("unreferenced generated schedule successors")}
 expected,err:=registeredDigest("cote.csf.digest.decision","cote.csf.schema.decision-pair-list",pairs)
 if err!=nil{return err}
 if !equalBytes(journal[17],expected[:]){return errors.New("multi-system decision pair-list digest invalid")}
 return nil
}

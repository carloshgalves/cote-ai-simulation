package communication

import (
 "bytes"
 "testing"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
)

// The registered first-run WORK handler admits one message at a time. This
// property tests that independent producer outcomes are content-addressed,
// not dependent on iteration order, retry count or operational worker order.
func TestCSFRV17IndependentChannelPermutationDeterminism(t *testing.T){
 bundle,claim,tx:=fixture(t)
 const n=24
 var run domain.ID;run[0]=91
 specs:=make([]SendSpec,n)
 for i:=0;i<n;i++{
  origin:=claim.Origin
  origin.Kind=5
  origin.ID[1]=byte(i+1)
  origin.Digest[1]=byte(i+2)
  ch:="school.message.v1"
  if i%4==0{ch="school.drop.v1"}
  specs[i]=SendSpec{Run:run,Source:origin,ActualSender:tx.ActualSender,
   PresentedSender:"C",Recipient:Actor{"school","b"},SentAt:4,
   DueAt:domain.SimulationInstant(12+i%3),Channel:ch,Proposition:claim.Proposition}
 }
 compute:=func(order []int)map[int][]byte{
  fingerprints:=map[int][]byte{}
  for _,index:=range order{
   spec:=specs[index]
   plan,err:=PrepareSend(bundle,spec)
   if err!=nil{t.Fatal(err)}
   due,err:=ValidateOccurrence(bundle,plan)
   if err!=nil{t.Fatal(err)}
   result,err:=PlanDueDelivery(bundle,plan.Transmission,plan.Claim,
    domain.ID(plan.Occurrence.Identity),due,due)
   if err!=nil{t.Fatal(err)}
   var cycle domain.ID;cycle[0]=77;cycle[1]=byte(index)
   sent,err:=NewSentEvent(bundle,EventContext{
    Run:run,Cycle:cycle,At:4,Sequence:0,BaseRevision:0},plan,spec.Source)
   if err!=nil{t.Fatal(err)}
   cycle[0]=78
   var occDomain domain.Digest
   parsed,err:=codec.StrictDecodePayload(plan.Occurrence.Canonical)
   if err!=nil{t.Fatal(err)}
   copy(occDomain[:],parsed.([]any)[14].([]byte))
   terminal,err:=NewDueEvent(bundle,EventContext{
    Run:run,Cycle:cycle,At:due,Sequence:1,BaseRevision:1},plan,result,
    Origin{Kind:6,ID:domain.ID(plan.Occurrence.Identity),Digest:occDomain})
   if err!=nil{t.Fatal(err)}
   total:=append([]byte(nil),plan.Claim.Canonical...)
   total=append(total,plan.Transmission.Canonical...)
   total=append(total,plan.Occurrence.Canonical...)
   total=append(total,sent.Canonical...)
   total=append(total,terminal.Canonical...)
   fingerprints[index]=total
  }
  return fingerprints
 }
 straight:=make([]int,n);reverse:=make([]int,n)
 for i:=0;i<n;i++{straight[i]=i;reverse[i]=n-i-1}
 want:=compute(straight)
 for round,ordering:=range [][]int{reverse,straight,reverse}{
  got:=compute(ordering)
  for i:=0;i<n;i++{
   if !bytes.Equal(want[i],got[i]){
    t.Fatalf("permuted message #%d on round %d changed causal bytes",i,round)
   }
  }
 }
}

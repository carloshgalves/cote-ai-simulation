package communication

import (
 "testing"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
)

func TestCSFRV17TracePartialReplayVsTerminal(t *testing.T){
 for _,tc:=range []struct{channel string;status uint8;hasInput bool}{
  {"school.message.v1",1,true},
  {"school.drop.v1",2,false},
 }{
  t.Run(tc.channel,func(t *testing.T){
   bundle,claim,tx:=fixture(t)
   var run,cycle domain.ID;run[0],cycle[0]=0x31,0x32
   source:=Origin{Kind:5,ID:claim.Origin.ID,Digest:claim.Origin.Digest}
   plan,err:=PrepareSend(bundle,SendSpec{Run:run,Source:source,
    ActualSender:tx.ActualSender,PresentedSender:"C",Recipient:Actor{"school","b"},
    Channel:tc.channel,SentAt:4,DueAt:12,Proposition:claim.Proposition})
   if err!=nil{t.Fatal(err)}
   first:=EventContext{Run:run,Cycle:cycle,BaseRevision:0,At:4,Sequence:0}
   send,err:=NewSentEvent(bundle,first,plan,source);if err!=nil{t.Fatal(err)}
   before,err:=ReplayCommunication(bundle,plan,[]ports.CanonicalRecord{send})
   if err!=nil||before.Status!=0||before.DeliveredAt!=nil||before.Notice!=nil{
    t.Fatal("pause before due leaked a communicated claim",err)
   }
   second:=first
   second.Cycle[0]++
   second.At=12;second.BaseRevision=2;second.Sequence=3
   outcome,err:=PlanDueDelivery(bundle,plan.Transmission,plan.Claim,
    domain.ID(plan.Occurrence.Identity),12,second.At)
   if err!=nil{t.Fatal(err)}
   occRaw,err:=codec.StrictDecodePayload(plan.Occurrence.Canonical);if err!=nil{t.Fatal(err)}
   fields:=occRaw.([]any)
   var digest domain.Digest;copy(digest[:],fields[14].([]byte))
   due,err:=NewDueEvent(bundle,second,plan,outcome,Origin{Kind:6,ID:domain.ID(plan.Occurrence.Identity),Digest:digest})
   if err!=nil{t.Fatal(err)}
   trace,err:=ReplayCommunication(bundle,plan,[]ports.CanonicalRecord{send,due})
   if err!=nil||trace.Status!=tc.status||(trace.Notice!=nil)!=tc.hasInput||
    trace.DeliveredAt==nil||*trace.DeliveredAt!=12{
    t.Fatal("due delivery/failure replay diverged",err,trace)
   }
   for _,bad:=range [][]ports.CanonicalRecord{{send,due,due},{due,send}}{
    if _,err:=ReplayCommunication(bundle,plan,bad);err==nil{
     t.Fatal("duplicate/out-of-order terminal Event accepted")
    }
   }
   tampered:=due
   tampered.Canonical=append([]byte(nil),due.Canonical...)
   tampered.Canonical[len(tampered.Canonical)-1]^=1
   if _,err:=ReplayCommunication(bundle,plan,[]ports.CanonicalRecord{send,tampered});err==nil{
    t.Fatal("semantic terminal Event corruption accepted")
   }
  })
 }
}

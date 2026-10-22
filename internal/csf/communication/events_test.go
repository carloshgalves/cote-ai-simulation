package communication

import (
 "testing"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
)

func TestCSFRV17EventDraftsKeepFalseClaimSeparateFromWorldTruth(t *testing.T){
 bundle,claim,tx:=fixture(t)
 var run,cycle domain.ID;run[0]=0x71;cycle[0]=0x72
 source:=Origin{Kind:5,ID:claim.Origin.ID,Digest:claim.Origin.Digest}
 plan,err:=PrepareSend(bundle,SendSpec{Run:run,Source:source,
  ActualSender:tx.ActualSender,PresentedSender:"C",Recipient:Actor{"school","b"},
  SentAt:4,DueAt:12,Channel:"school.message.v1",Proposition:claim.Proposition})
 if err!=nil{t.Fatal(err)}
 ctx:=EventContext{Run:run,Cycle:cycle,At:4,Sequence:0,BaseRevision:0}
 sent,err:=NewSentEvent(bundle,ctx,plan,source);if err!=nil{t.Fatal(err)}
 val,err:=codec.StrictDecodePayload(sent.Canonical);if err!=nil{t.Fatal(err)}
 fields:=val.([]any)
 if fields[3]!="communication.sent"||fields[15]!=uint64(2)||len(fields[12].([]any))!=0{
  t.Fatal("sending leaked recipient observation or forged a theft fact")
 }
 ctx.Cycle[0]++;ctx.At=12;ctx.Sequence=1;ctx.BaseRevision=1
 delivered,err:=PlanDueDelivery(bundle,plan.Transmission,plan.Claim,domain.ID(plan.Occurrence.Identity),12,12,true)
 if err!=nil{t.Fatal(err)}
 occ,err:=codec.StrictDecodePayload(plan.Occurrence.Canonical);if err!=nil{t.Fatal(err)}
 f:=occ.([]any)
 var digest domain.Digest;copy(digest[:],f[14].([]byte))
 origin:=Origin{Kind:6,ID:domain.ID(plan.Occurrence.Identity),Digest:digest}
 // The Event materializer must not trust a worker-supplied recipient,
 // presentation or proposition even if the worker guesses a valid event kind.
 forgedNotice:=delivered
 copied:=*delivered.Notice
 copied.PresentedSender="A"
 forgedNotice.Notice=&copied
 if _,err:=NewDueEvent(bundle,ctx,plan,forgedNotice,origin);err==nil{
  t.Fatal("worker forged presented sender at terminal delivery")
 }
 forgedOrigin:=origin;forgedOrigin.Digest[0]^=0x01
 if _,err:=NewDueEvent(bundle,ctx,plan,delivered,forgedOrigin);err==nil{
  t.Fatal("delivery accepted forged ScheduledOccurrence digest")
 }
 recorded,err:=NewDueEvent(bundle,ctx,plan,delivered,origin);if err!=nil{t.Fatal(err)}
 result,err:=codec.StrictDecodePayload(recorded.Canonical);if err!=nil{t.Fatal(err)}
 ef:=result.([]any)
 if ef[3]!="communication.delivered"||ef[15]!=uint64(1)||
  len(ef[12].([]any))!=1||ef[12].([]any)[0].([]any)[1]!="b"{
  t.Fatal("delivery Event not addressed solely to B")
 }
 fail,err:=PlanDueDelivery(bundle,plan.Transmission,plan.Claim,origin.ID,12,12,false)
 if err!=nil{t.Fatal(err)}
 denied,err:=NewDueEvent(bundle,ctx,plan,fail,origin);if err!=nil{t.Fatal(err)}
 rejected,err:=codec.StrictDecodePayload(denied.Canonical);if err!=nil{t.Fatal(err)}
 rf:=rejected.([]any)
 if rf[3]!="communication.delivery_failed"||len(rf[12].([]any))!=0||
  rf[15]!=uint64(2){t.Fatal("channel failure made B an observer")}
 if recorded.Identity!=denied.Identity{
  t.Fatal("same scheduled occurrence must retain causal event role ID independent of channel outcome")
 }
 if _,err:=NewDueEvent(bundle,EventContext{Run:run,Cycle:cycle,At:11},plan,delivered,origin);err==nil{
  t.Fatal("delivery Event backdated to before due")
 }
}

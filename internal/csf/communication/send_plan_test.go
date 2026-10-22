package communication

import (
 "bytes"
 "testing"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
)

func TestCSFRV17PrepareSendProducesNoEarlyRecipientInput(t *testing.T){
 bundle,claim,transmission:=fixture(t)
 var run domain.ID;run[0]=0x77
 plan,err:=PrepareSend(bundle,SendSpec{Run:run,Source:Origin{Kind:5,ID:claim.Origin.ID,Digest:claim.Origin.Digest},
  ActualSender:transmission.ActualSender,PresentedSender:"C",Recipient:Actor{"school","b"},
  SentAt:4,DueAt:12,Channel:"school.message.v1",Proposition:claim.Proposition})
 if err!=nil{t.Fatal(err)}
 same,err:=PrepareSend(bundle,SendSpec{Run:run,Source:Origin{Kind:5,ID:claim.Origin.ID,Digest:claim.Origin.Digest},
  ActualSender:transmission.ActualSender,PresentedSender:"C",Recipient:Actor{"school","b"},
  SentAt:4,DueAt:12,Channel:"school.message.v1",Proposition:claim.Proposition})
 if err!=nil{t.Fatal(err)}
 for i,records:=range [][2][]byte{{plan.Claim.Canonical,same.Claim.Canonical},
  {plan.Transmission.Canonical,same.Transmission.Canonical},{plan.Occurrence.Canonical,same.Occurrence.Canonical}}{
  if !bytes.Equal(records[0],records[1]){t.Fatalf("source permutation changed canonical record #%d",i)}
 }
 due,err:=ValidateOccurrence(bundle,plan);if err!=nil||due!=12{t.Fatal("invalid due plan",err)}
 decoded,err:=codec.StrictDecodePayload(plan.Transmission.Canonical);if err!=nil{t.Fatal(err)}
 fields:=decoded.([]any)
 if fields[12]!=uint64(0){t.Fatal("sending terminalized before scheduled delivery")}
 if _,err:=PlanDueDelivery(bundle,plan.Transmission,plan.Claim,
  domain.ID(plan.Occurrence.Identity),due,due-1);err==nil{
  t.Fatal("uncommitted early knowledge formed from an in-transit transmission")
 }
 result,err:=PlanDueDelivery(bundle,plan.Transmission,plan.Claim,
  domain.ID(plan.Occurrence.Identity),due,due)
 if err!=nil||result.Notice==nil||result.Notice.PresentedSender!="C"{
  t.Fatal("committed delivery projection missing presented identity",err)
 }
 droppedSpec:=SendSpec{Run:run,Source:Origin{Kind:5,ID:claim.Origin.ID,Digest:claim.Origin.Digest},
  ActualSender:transmission.ActualSender,PresentedSender:"C",Recipient:Actor{"school","b"},
  SentAt:4,DueAt:12,Channel:"school.drop.v1",Proposition:claim.Proposition}
 dropped,err:=PrepareSend(bundle,droppedSpec);if err!=nil{t.Fatal(err)}
 failure,err:=PlanDueDelivery(bundle,dropped.Transmission,dropped.Claim,
  domain.ID(dropped.Occurrence.Identity),due,due)
 if err!=nil||failure.Notice!=nil||failure.NextStatus!=2{
  t.Fatal("failed channel produced recipient knowledge",err)
 }
}
func TestCSFRV17DeliveryRequiresExactOccurrenceDigest(t *testing.T){
 bundle,claim,transmission:=fixture(t)
 var run domain.ID;run[0]=0x77
 spec:=SendSpec{Run:run,Source:Origin{Kind:5,ID:claim.Origin.ID,Digest:claim.Origin.Digest},
  ActualSender:transmission.ActualSender,PresentedSender:"C",Recipient:Actor{"school","b"},
  SentAt:4,DueAt:12,Channel:"school.message.v1",Proposition:claim.Proposition}
 plan,err:=PrepareSend(bundle,spec);if err!=nil{t.Fatal(err)}
 parsed,err:=codec.StrictDecodePayload(plan.Occurrence.Canonical);if err!=nil{t.Fatal(err)}
 fields:=append([]any(nil),parsed.([]any)...)
 fields[3]=int64(99)
 forged,err:=codec.CanonicalPayload(fields);if err!=nil{t.Fatal(err)}
 plan.Occurrence.Canonical=forged
 if _,err:=ValidateOccurrence(bundle,plan);err==nil{
  t.Fatal("changed semantic delivery time kept a valid occurrence signature")
 }
 spec.DueAt=4
 if _,err:=PrepareSend(bundle,spec);err==nil{t.Fatal("zero-delay in same cycle accepted")}
 spec.DueAt=12
 spec.Source.Kind=0
 if _,err:=PrepareSend(bundle,spec);err==nil{t.Fatal("non-action origin authorized send")}
}

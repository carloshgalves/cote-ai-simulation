package communication

import (
 "bytes"
 "testing"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
)

func TestCSFRV17FalseForgedDelayedAndFailurePlans(t *testing.T){
 bundle,claimSpec,txSpec:=fixture(t)
 claim,err:=NewClaim(bundle,claimSpec);if err!=nil{t.Fatal(err)}
 tx,err:=NewTransmission(bundle,txSpec);if err!=nil{t.Fatal(err)}
 due:=*txSpec.DeliveryOccurrence
 if _,err:=PlanDueDelivery(bundle,tx,claim,due,12,11);err==nil{
  t.Fatal("B obtained knowledge before causal due")
 }
 result,err:=PlanDueDelivery(bundle,tx,claim,due,12,12)
 if err!=nil{t.Fatal(err)}
 if result.EventType!="communication.delivered"||result.NextStatus!=1||result.Notice==nil{
  t.Fatal("missing delivery projection")
 }
 if result.Notice.Recipient!=(Actor{"school","b"})||result.Notice.PresentedSender!="C"{
  t.Fatal("the addressed delivery changed the presented identity")
 }
 payload,err:=result.Notice.RedactedValue();if err!=nil{t.Fatal(err)}
 raw,err:=codec.CanonicalPayload(payload);if err!=nil{t.Fatal(err)}
 if len(payload)!=6||result.Notice.Recipient==txSpec.ActualSender||
  bytes.Contains(raw,[]byte("school/a"))||bytes.Contains(raw,[]byte("actual_sender")){
  t.Fatal("actual sender leaked to B")
 }
 same,err:=PlanDueDelivery(bundle,tx,claim,due,12,12)
 if err!=nil{t.Fatal(err)}
 sameRaw,err:=same.Notice.RedactedValue();if err!=nil{t.Fatal(err)}
 norm1,_:=codec.CanonicalPayload(payload);norm2,_:=codec.CanonicalPayload(sameRaw)
 if !bytes.Equal(norm1,norm2){t.Fatal("worker order changed causal observation bytes")}
 txSpec.Channel="school.drop.v1"
 failedTx,err:=NewTransmission(bundle,txSpec);if err!=nil{t.Fatal(err)}
 failed,err:=PlanDueDelivery(bundle,failedTx,claim,due,12,12)
 if err!=nil{t.Fatal(err)}
 if failed.EventType!="communication.delivery_failed"||failed.Notice!=nil||failed.NextStatus!=2{
  t.Fatal("failed delivery leaked a KnowledgeInput candidate")
 }
 var bad domain.ID;bad[0]=0xff
 if _,err:=PlanDueDelivery(bundle,tx,claim,bad,12,12);err==nil{t.Fatal("unrelated occurrence delivered the claim")}
 txSpec.ActualSender=Actor{"school","c"}
 forged,err:=NewTransmission(bundle,txSpec);if err!=nil{t.Fatal(err)}
 if _,err:=PlanDueDelivery(bundle,forged,claim,due,12,12);err==nil{
  t.Fatal("presented sender supplied by caller was promoted to authoritative author")
 }
}

func TestCSFRV17TamperedClaimOrTransmissionRejectedBeforeProjection(t *testing.T){
 bundle,claimSpec,txSpec:=fixture(t)
 claim,err:=NewClaim(bundle,claimSpec);if err!=nil{t.Fatal(err)}
 tx,err:=NewTransmission(bundle,txSpec);if err!=nil{t.Fatal(err)}
 due:=*txSpec.DeliveryOccurrence
 decoded,err:=codec.StrictDecodePayload(tx.Canonical);if err!=nil{t.Fatal(err)}
 f:=append([]any(nil),decoded.([]any)...)
 f[6]="A"
 mutated,err:=codec.CanonicalPayload(f);if err!=nil{t.Fatal(err)}
 tx.Canonical=mutated
 if _,err:=PlanDueDelivery(bundle,tx,claim,due,12,12);err==nil{
  t.Fatal("modified presentation without recomputed digest was delivered")
 }
 tx,err=NewTransmission(bundle,txSpec);if err!=nil{t.Fatal(err)}
 v,err:=codec.StrictDecodePayload(claim.Canonical);if err!=nil{t.Fatal(err)}
 parts:=append([]any(nil),v.([]any)...)
 parts[6]=int64(20)
 claim.Canonical,err=codec.CanonicalPayload(parts);if err!=nil{t.Fatal(err)}
 if _,err:=PlanDueDelivery(bundle,tx,claim,due,12,12);err==nil{
  t.Fatal("changed claim causal time was accepted")
 }
}

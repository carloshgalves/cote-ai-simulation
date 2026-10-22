package communication

import (
 "bytes"
 "testing"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

func fixture(t *testing.T)(*policy.Bundle,ClaimSpec,TransmissionSpec){
 t.Helper()
 bundle,err:=policy.Load("../../../docs/architecture/canonical-codec-v1-bundle")
 if err!=nil{t.Fatal(err)}
 var origin Origin
 origin.Kind=0
 origin.ID[0],origin.Digest[0]=1,2
 scope,err:=MakeTyped("cote.csf.communication.scope.v1","cote.csf.schema.typed-value",[]any{uint64(0)})
 if err!=nil{t.Fatal(err)}
 content,err:=MakeTyped("cote.csf.communication.allegation.v1","cote.csf.schema.typed-value",
  []any{"resource.stolen",[]any{"school","c"},"last.resource"})
 if err!=nil{t.Fatal(err)}
 claim:=ClaimSpec{Origin:origin,Role:0,Ordinal:0,AssertedBy:Actor{"school","a"},
  AssertedAt:4,Proposition:Proposition{
   Type:"resource.stolen",Polarity:0,TemporalScope:scope,Content:content}}
 record,err:=NewClaim(bundle,claim);if err!=nil{t.Fatal(err)}
 parsed,err:=codec.StrictDecodePayload(record.Canonical);if err!=nil{t.Fatal(err)}
 parts:=parsed.([]any)
 var claimDigest domain.Digest;copy(claimDigest[:],parts[9].([]byte))
 var due domain.ID;due[0]=7
 tx:=TransmissionSpec{Origin:origin,Role:0,Ordinal:0,DeliveryOccurrence:&due,
  ActualSender:Actor{"school","a"},PresentedSender:"C",Recipients:[]Actor{{"school","b"}},
  Channel:"school.message.v1",SentAt:4,Status:0,
  ClaimRefs:[]domain.CausalRef{{Kind:18,ID:domain.ID(record.Identity),Digest:claimDigest}}}
 return bundle,claim,tx
}
func TestCSFRV17CanonicalFalseClaimIsNotWorldTruth(t *testing.T){
 bundle,claim,tx:=fixture(t)
 a,err:=NewClaim(bundle,claim);if err!=nil{t.Fatal(err)}
 b,err:=NewClaim(bundle,claim);if err!=nil{t.Fatal(err)}
 if a.Identity!=b.Identity||!bytes.Equal(a.Canonical,b.Canonical){t.Fatal("claim not deterministic")}
 parsed,err:=codec.StrictDecodePayload(a.Canonical);if err!=nil{t.Fatal(err)}
 fields:=parsed.([]any)
 if len(fields)!=10||len(fields[4].([]any))!=4||
  fields[4].([]any)[0]!="resource.stolen"||fields[4].([]any)[1]!=uint64(0){
  t.Fatal("false assertion not encoded as truth-neutral proposition")
 }
 if bytes.Contains(a.Canonical,[]byte("truth_flag")){t.Fatal("truth oracle leaked into claim")}
 transmission,err:=NewTransmission(bundle,tx);if err!=nil{t.Fatal(err)}
 repeated,err:=NewTransmission(bundle,tx)
 if err!=nil||transmission.Identity!=repeated.Identity||!bytes.Equal(transmission.Canonical,repeated.Canonical){
  t.Fatal("transmission identity/bytes not deterministic",err)
 }
 v,err:=codec.StrictDecodePayload(transmission.Canonical);if err!=nil{t.Fatal(err)}
 f:=v.([]any)
 if len(f)!=14||f[6]!="C"||f[12]!=uint64(0)||f[5].([]any)[1]!="a"||
  f[8]!="school.message.v1"{
  t.Fatal("sender, presented identity or lifecycle malformed")
 }
}
func TestCSFRV17CannotPretendToBecomeAnotherFact(t *testing.T){
 bundle,claim,tx:=fixture(t)
 claim.Proposition.Polarity=3
 if _,err:=NewClaim(bundle,claim);err==nil{t.Fatal("unregistered polarity accepted")}
 _,claim,tx=fixture(t)
 tx.Recipients=[]Actor{{"school","b"},{"school","b"}}
 if _,err:=NewTransmission(bundle,tx);err==nil{t.Fatal("duplicated recipients accepted")}
 tx.Recipients=[]Actor{{"school","b"}}
 tx.ClaimRefs[0].Kind=0
 if _,err:=NewTransmission(bundle,tx);err==nil{t.Fatal("Event used as Claim without distinction")}
 tx.ClaimRefs[0].Kind=18
 tx.Status=9
 if _,err:=NewTransmission(bundle,tx);err==nil{t.Fatal("unknown transmission status")}
 tx.Status=0
 tx.Facts=[]any{[]any{"actual-sender-leak",uint64(1)}}
 if _,err:=NewTransmission(bundle,tx);err==nil{t.Fatal("unverified metadata facts accepted")}
 claim.Origin.ID=[32]byte{}
 if _,err:=NewClaim(bundle,claim);err==nil{t.Fatal("unattributed assertion accepted")}
}

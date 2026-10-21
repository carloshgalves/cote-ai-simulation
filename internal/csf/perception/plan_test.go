package perception

import (
 "bytes"
 "testing"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/settlement"
)

func TestPotentialPerceptibilityHasNoObserverOrPayloadProjection(t *testing.T){
 bundle,err:=policy.Load("../../../docs/architecture/canonical-codec-v1-bundle")
 if err!=nil{t.Fatal(err)}
 refs,err:=bundle.GenesisPolicyRefs();if err!=nil{t.Fatal(err)}
 byID:=map[string][]any{}
 for _,ref:=range refs{byID[ref[0].(string)]=ref}
 pins:=Pins{Classification:byID["perception-v1"],Resolver:byID["validators-v1"],
  Identity:byID["perception-identity-v1"]}
 if pins.Classification==nil||pins.Resolver==nil||pins.Identity==nil{t.Fatal("perception pins missing")}
 var run,cycle,round,source domain.ID
 run[0],cycle[0],round[0],source[0]=1,2,3,4
 var digest domain.Digest;digest[0]=9
 event,err:=settlement.ConsumedRoundEvent(run,cycle,round,100,0,0,
  []any{[]any{uint64(0),uint64(1),source[:],digest[:]}})
 if err!=nil{t.Fatal(err)}
 tasks,err:=Plan(bundle,run,cycle,0,1,[]ports.CanonicalRecord{event},pins)
 if err!=nil||len(tasks)!=1{t.Fatalf("registered ordinal-zero event should make one potential task: %d %v",len(tasks),err)}
 parsed,err:=codec.StrictDecodePayload(tasks[0].Canonical);if err!=nil{t.Fatal(err)}
 fields:=parsed.([]any)
 if len(fields)!=10||!bytes.Equal(fields[3].([]byte),event.Identity[:])||
  fields[4]!=uint64(0)||fields[5]!=uint64(1)||fields[9]!=uint64(0){
  t.Fatal("perception task missing canonical event coordinate")
 }
 if bytes.Contains(tasks[0].Canonical,source[:]){t.Fatal("task leaked private slot-response ID")}
 other,err:=Plan(bundle,run,cycle,0,1,[]ports.CanonicalRecord{event},pins)
 if err!=nil||!bytes.Equal(other[0].Canonical,tasks[0].Canonical){t.Fatal("perception task not deterministic",err)}
 forged:=event
 forged.Identity[0]^=0xff
 if _,err:=Plan(bundle,run,cycle,0,1,[]ports.CanonicalRecord{forged},pins);err==nil{
  t.Fatal("planner accepted self-inconsistent Event identity")
 }
 bad:=event
 bad.Canonical=[]byte{0x80}
 if _,err:=Plan(bundle,run,cycle,0,1,[]ports.CanonicalRecord{bad},pins);err==nil{
  t.Fatal("planner accepted unauthenticated Event bytes")
 }
 decoded,err:=codec.StrictDecodePayload(event.Canonical);if err!=nil{t.Fatal(err)}
 fields2:=append([]any(nil),decoded.([]any)...)
 key:=append([]any(nil),fields2[2].([]any)...)
 key[4]=uint64(1)
 d:=identity.New(bundle)
 alternative,_,err:=d.Sum("cote.csf.id.event","cote.csf.schema.event-id-preimage",1,
  []any{run[:],cycle[:],key});if err!=nil{t.Fatal(err)}
 fields2[0],fields2[2]=alternative[:],key
 h,_,err:=d.Sum("cote.csf.digest.event","cote.csf.schema.event.body",1,fields2[:18])
 if err!=nil{t.Fatal(err)}
 fields2[18]=h[:]
 alt,err:=codec.CanonicalPayload(fields2);if err!=nil{t.Fatal(err)}
 skipped,err:=Plan(bundle,run,cycle,0,1,[]ports.CanonicalRecord{{Identity:[32]byte(alternative),Canonical:alt}},pins)
 if err!=nil||len(skipped)!=0{t.Fatal("planner created task for event-local-ordinal 1",err)}
}

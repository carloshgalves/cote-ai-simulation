package contextboundary

import (
 "context"
 "testing"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
)

type testInboxReader struct {
 observations []ports.CanonicalRecord
 inputs []ports.CanonicalRecord
}
func (f testInboxReader)ReadAddressedEvidence(_ context.Context,_ [2]string)(
 []ports.CanonicalRecord,[]ports.CanonicalRecord,error){
 return f.observations,f.inputs,nil
}

func testAddressedPair(t *testing.T,domain string,notRedacted bool,claims bool,late bool)(
 ports.CanonicalRecord,ports.CanonicalRecord){
 t.Helper()
 var task,event,round,eventDigest [32]byte
 task[0],event[0],round[0],eventDigest[0]=1,2,3,4
 actor:=[]any{"school","a"}
 content:=[]any{round[:],uint64(2),[]any{}}
 if notRedacted{content[2]=[]any{[]any{uint64(0),event[:],eventDigest[:]}}}
 encoded,err:=codec.CanonicalBytes(domain,"cote.csf.schema.typed-value",1,content)
 if err!=nil{t.Fatal(err)}
 digest:=identity.OpaqueContentHash(encoded)
 percept:=[]any{"cote.csf.schema.typed-value",uint64(1),encoded,digest[:]}
 source:=[]any{[]any{event[:],eventDigest[:]}}
 evidence:=[]any{[]any{uint64(0),event[:],eventDigest[:]}}
 obsID,err:=sum("cote.csf.id.observation","cote.csf.schema.observation-id-preimage",
  []any{task[:],actor,uint64(0),uint64(0)})
 if err!=nil{t.Fatal(err)}
 obsClaims:=[]any{}
 if claims{obsClaims=append(obsClaims,[]any{uint64(18),event[:],eventDigest[:]})}
 body:=[]any{obsID[:],task[:],actor,uint64(0),uint64(0),int64(10),
  source,uint64(5),[]any{uint64(0)},[]any{percept},obsClaims,
  []any{"source-inputs-redacted"},evidence,
  []any{"validators-v1",uint64(1),make([]byte,32)}}
 obsDigest,err:=sum("cote.csf.digest.observation","cote.csf.schema.observation.body",body)
 if err!=nil{t.Fatal(err)}
 raw,err:=codec.CanonicalPayload(append(body,obsDigest[:]))
 if err!=nil{t.Fatal(err)}
 obs:=ports.CanonicalRecord{Identity:obsID,Canonical:raw}
 inputID,err:=sum("cote.csf.id.knowledge-input","cote.csf.schema.knowledge-input-id-preimage",
  []any{obsID[:],actor,uint64(0),uint64(0),uint64(0)})
 if err!=nil{t.Fatal(err)}
 at:=int64(10)
 if late{at=11}
 inBody:=[]any{inputID[:],uint64(0),uint64(0),actor,at,
  uint64(0),"school.round.lifecycle.v1",
  []any{uint64(16),obsID[:],obsDigest[:]},[]any{},evidence}
 inputDigest,err:=sum("cote.csf.digest.knowledge-input",
  "cote.csf.schema.knowledge-input.body",inBody)
 if err!=nil{t.Fatal(err)}
 inputRaw,err:=codec.CanonicalPayload(append(inBody,inputDigest[:]))
 if err!=nil{t.Fatal(err)}
 return obs,ports.CanonicalRecord{Identity:inputID,Canonical:inputRaw}
}

func TestKnowledgeBoundaryRejectsUnauthorizedOrigins(t *testing.T){
 ctx:=context.Background()
 for _,tc:=range []struct{
  name,domain string
  unredacted,claims,late bool
 }{
  {"VIP-secret","cote.csf.exam.vip-secret.v1",false,false,false},
  {"White-Room","cote.csf.background.white-room.v1",false,false,false},
  {"canon-future","cote.csf.canon.future-after-divergence.v1",false,false,false},
  {"physical-telemetry","cote.csf.body.synthetic-advance.v1",false,false,false},
  {"Observatory","cote.csf.observatory.privileged.v1",false,false,false},
  {"unredacted-source","cote.csf.round.consumed.v1",true,false,false},
  {"arbitrary-claim","cote.csf.round.consumed.v1",false,true,false},
  {"worker-latency","cote.csf.round.consumed.v1",false,false,true},
 }{
  t.Run(tc.name,func(t *testing.T){
   observation,input:=testAddressedPair(t,tc.domain,tc.unredacted,tc.claims,tc.late)
   reader:=testInboxReader{[]ports.CanonicalRecord{observation},[]ports.CanonicalRecord{input}}
   if _,err:=Build(ctx,reader,[2]string{"school","a"});err==nil{
    t.Fatal("forbidden information entered actor context")
   }
  })
 }
}

func TestKnowledgeBoundaryRequiresOwnDurableReceipts(t *testing.T){
 ctx:=context.Background()
 observation,input:=testAddressedPair(t,"cote.csf.round.consumed.v1",false,false,false)
 reader:=testInboxReader{[]ports.CanonicalRecord{observation},[]ports.CanonicalRecord{input}}
 view,err:=Build(ctx,reader,[2]string{"school","a"})
 if err!=nil||len(view.Inbox)!=1{t.Fatal("valid addressed evidence refused",err)}
 if _,err:=Build(ctx,reader,[2]string{"school","b"});err==nil{
  t.Fatal("A's private evidence entered B context")
 }
 reader.inputs=nil
 if _,err:=Build(ctx,reader,[2]string{"school","a"});err==nil{
  t.Fatal("Observation without durable KnowledgeInput entered context")
 }
 reader.observations=nil
 reader.inputs=[]ports.CanonicalRecord{input}
 if _,err:=Build(ctx,reader,[2]string{"school","a"});err==nil{
  t.Fatal("KnowledgeInput without Observation entered context")
 }
}

package contextboundary

import (
 "context"
 "testing"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
)

func buildCommunicationTestPair(t *testing.T)(ports.CanonicalRecord,ports.CanonicalRecord){
 t.Helper()
 var task,event,eventDigest,claimID,claimDigest [32]byte
 task[0],event[0],eventDigest[0],claimID[0],claimDigest[0]=1,2,3,4,5
 actor:=[]any{"school","b"}
 typed:=func(tag string,body any)[]any{
  raw,err:=codec.CanonicalBytes(tag,"cote.csf.schema.typed-value",1,body)
  if err!=nil{t.Fatal(err)}
  digest:=identity.OpaqueContentHash(raw)
  return []any{"cote.csf.schema.typed-value",uint64(1),raw,digest[:]}
 }
 proposition:=[]any{"resource.stolen",uint64(0),
  typed("cote.csf.communication.scope.v1",[]any{uint64(0)}),
  typed("cote.csf.communication.allegation.v1",
   []any{"resource.stolen",[]any{"school","c"},"last.resource"})}
 percept:=typed("cote.csf.communication.delivered.v1",
  []any{actor,"C",claimID[:],proposition,int64(10),"school.message.v1"})
 claims:=[]any{[]any{uint64(18),claimID[:],claimDigest[:]}}
 evidence:=[]any{[]any{uint64(0),event[:],eventDigest[:]}}
 id,err:=sum("cote.csf.id.observation","cote.csf.schema.observation-id-preimage",
  []any{task[:],actor,uint64(0),uint64(0)})
 if err!=nil{t.Fatal(err)}
 body:=[]any{id[:],task[:],actor,uint64(0),uint64(0),int64(10),
  []any{[]any{event[:],eventDigest[:]}},uint64(5),[]any{uint64(0)},
  []any{percept},claims,[]any{"actual-sender-redacted","claim-unverified"},
  evidence,[]any{"validators-v1",uint64(1),make([]byte,32)}}
 digest,err:=sum("cote.csf.digest.observation","cote.csf.schema.observation.body",body)
 if err!=nil{t.Fatal(err)}
 obsBytes,err:=codec.CanonicalPayload(append(body,digest[:]))
 if err!=nil{t.Fatal(err)}
 iid,err:=sum("cote.csf.id.knowledge-input",
  "cote.csf.schema.knowledge-input-id-preimage",
  []any{id[:],actor,uint64(1),uint64(0),uint64(0)})
 if err!=nil{t.Fatal(err)}
 inputBody:=[]any{iid[:],uint64(0),uint64(0),actor,int64(10),
  uint64(1),"school.message.v1",
  []any{uint64(16),id[:],digest[:]},claims,evidence}
 idigest,err:=sum("cote.csf.digest.knowledge-input",
  "cote.csf.schema.knowledge-input.body",inputBody)
 if err!=nil{t.Fatal(err)}
 inputBytes,err:=codec.CanonicalPayload(append(inputBody,idigest[:]))
 if err!=nil{t.Fatal(err)}
 return ports.CanonicalRecord{Identity:id,Canonical:obsBytes},
  ports.CanonicalRecord{Identity:iid,Canonical:inputBytes}
}

func TestCSFRV17KnowledgeBoundaryAllowsClaimButNotWorldTruth(t *testing.T){
 ctx:=context.Background()
 obs,input:=buildCommunicationTestPair(t)
 reader:=testInboxReader{[]ports.CanonicalRecord{obs},[]ports.CanonicalRecord{input}}
 view,err:=Build(ctx,reader,[2]string{"school","b"})
 if err!=nil||len(view.Inbox)!=1||view.Inbox[0].ReceivedAt!=10{
  t.Fatalf("received false claim cannot enter B's allow-listed context: %v",err)
 }
 if _,err:=Build(ctx,reader,[2]string{"school","a"});err==nil{
  t.Fatal("hidden actual sender A could read B's claim")
 }
 reader.inputs=nil
 if _,err:=Build(ctx,reader,[2]string{"school","b"});err==nil{
  t.Fatal("observation without KnowledgeInput visible to B")
 }
 reader.inputs=[]ports.CanonicalRecord{input}
 reader.observations=nil
 if _,err:=Build(ctx,reader,[2]string{"school","b"});err==nil{
  t.Fatal("unbacked KnowledgeInput visible to B")
 }
}

func TestCSFRV17KnowledgeBoundaryRejectsClaimForging(t *testing.T){
 ctx:=context.Background()
 obs,input:=buildCommunicationTestPair(t)
 for _,tc:=range []struct{
  name string
  obs func([]any)
  in func([]any)
 }{
  {"hidden-actual-author",func(v []any){v[11]=[]any{"actual-sender-revealed","claim-unverified"}},nil},
  {"pretend-is-true",func(v []any){v[11]=[]any{"actual-sender-redacted","claim-verified"}},nil},
  {"extra-world-claim",func(v []any){v[10]=[]any{}},nil},
  {"physical-modality",nil,func(v []any){v[5]=uint64(0)}},
  {"unregistered-channel",nil,func(v []any){v[6]="school.drop.v1"}},
  {"backdated-inbox",nil,func(v []any){v[4]=int64(9)}},
 }{
  t.Run(tc.name,func(t *testing.T){
   o,i:=obs,input
   if tc.obs!=nil{
    v,err:=codec.StrictDecodePayload(obs.Canonical);if err!=nil{t.Fatal(err)}
    f:=v.([]any);tc.obs(f)
    o.Canonical,err=codec.CanonicalPayload(f);if err!=nil{t.Fatal(err)}
   }
   if tc.in!=nil{
    v,err:=codec.StrictDecodePayload(input.Canonical);if err!=nil{t.Fatal(err)}
    f:=v.([]any);tc.in(f)
    i.Canonical,err=codec.CanonicalPayload(f);if err!=nil{t.Fatal(err)}
   }
   reader:=testInboxReader{[]ports.CanonicalRecord{o},[]ports.CanonicalRecord{i}}
   if _,err:=Build(ctx,reader,[2]string{"school","b"});err==nil{
    t.Fatal("unregistered or modified communication payload accepted")
   }
  })
 }
}

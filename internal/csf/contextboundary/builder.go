// Package contextboundary constructs agent context from a strict evidence
// allow-list. It cannot read world truth, EventStore, PerceptionTask, global
// observations, RAG or privileged Observatory state.
package contextboundary

import (
 "bytes"
 "context"
 "errors"
 "sort"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
)

type AddressedReader interface {
 ReadAddressedEvidence(context.Context,[2]string)([]ports.CanonicalRecord,[]ports.CanonicalRecord,error)
}

type Entry struct {
 Observation ports.CanonicalRecord
 Input ports.CanonicalRecord
 ReceivedAt int64
}
type View struct {
 Actor [2]string
 Inbox []Entry
 // No background, cognition or RAG is returned before source gate
 // contracts exist. Silence is preferable to an unchecked global prompt.
}

func signedTime(value any)(int64,bool){
 switch v:=value.(type){
 case int64:return v,true
 case uint64:
  if v<=uint64(^uint64(0)>>1){return int64(v),true}
 }
 return 0,false
}

func Build(ctx context.Context,reader AddressedReader,actor [2]string)(View,error){
 out:=View{Actor:actor,Inbox:[]Entry{}}
 if reader==nil||actor[0]==""||actor[1]==""{return out,errors.New("context requires authenticated addressed reader")}
 observations,inputs,err:=reader.ReadAddressedEvidence(ctx,actor)
 if err!=nil{return out,err}
 byID:=map[[32]byte]ports.CanonicalRecord{}
 byTime:=map[[32]byte]int64{}
 for _,obs:=range observations{
  v,err:=codec.StrictDecodePayload(obs.Canonical);if err!=nil{return out,err}
  f,ok:=v.([]any)
  if !ok||len(f)!=15||!bytes.Equal(f[0].([]byte),obs.Identity[:])||
   !canonicalActor(f[2],actor){return out,errors.New("foreign or malformed Observation")}
  time,valid:=signedTime(f[5])
  if !valid{return out,errors.New("Observation time invalid")}
  percepts,ok:=f[9].([]any)
  if !ok||len(percepts)!=1{return out,errors.New("unknown/unchecked context percept")}
  percept,ok:=percepts[0].([]any)
  if !ok||len(percept)!=4{return out,errors.New("invalid context percept")}
  encoded,ok:=percept[2].([]byte)
  if !ok||percept[0]!="cote.csf.schema.typed-value"||percept[1]!=uint64(1){
   return out,errors.New("context percept unregistered")
  }
  payload,err:=codec.StrictDecodeEnvelope(encoded)
  if err!=nil||payload.DomainTag!="cote.csf.round.consumed.v1"||
   payload.SchemaID!="cote.csf.schema.typed-value"||payload.SchemaVersion!=1{
   return out,errors.New("context rejected non-addressed percept (canon/physical/privileged)")
  }
  parts,ok:=payload.Value.([]any)
  if !ok||len(parts)!=3||parts[1]!=uint64(2){return out,errors.New("context notice malformed")}
  confidential,ok:=parts[2].([]any)
  if !ok||len(confidential)!=0{return out,errors.New("context includes unredacted event sources")}
  h:=identity.OpaqueContentHash(encoded)
  if !bytes.Equal(h[:],percept[3].([]byte)){
   return out,errors.New("context percept digest mismatch")
  }
  if len(f[10].([]any))!=0||!sameTextList(f[11],[]string{"source-inputs-redacted"}){
   return out,errors.New("context observation leaked claim/raw payload")
  }
  if _,exists:=byID[obs.Identity];exists{return out,errors.New("duplicate observation")}
  byID[obs.Identity]=obs;byTime[obs.Identity]=time
 }
 used:=map[[32]byte]bool{}
 for _,record:=range inputs{
  v,err:=codec.StrictDecodePayload(record.Canonical);if err!=nil{return out,err}
  f,ok:=v.([]any)
  if !ok||len(f)!=11||!canonicalActor(f[3],actor)||
   f[5]!=uint64(0)||f[6]!="school.round.lifecycle.v1"{
   return out,errors.New("unregistered or foreign KnowledgeInput")
  }
  ref,ok:=f[7].([]any)
  if !ok||len(ref)!=3||ref[0]!=uint64(16){return out,errors.New("input observation ref invalid")}
  id,ok:=ref[1].([]byte)
  if !ok||len(id)!=32{return out,errors.New("input observation id malformed")}
  var key [32]byte;copy(key[:],id)
  obs,exists:=byID[key]
  if !exists||used[key]{return out,errors.New("missing/duplicate addressed Observation")}
  decoded,err:=codec.StrictDecodePayload(obs.Canonical);if err!=nil{return out,err}
  of:=decoded.([]any)
  digest,err:=sum("cote.csf.digest.observation","cote.csf.schema.observation.body",of[:14])
  if err!=nil||!bytes.Equal(ref[2].([]byte),digest[:]){
   return out,errors.New("KnowledgeInput references wrong Observation digest")
  }
  received,valid:=signedTime(f[4])
  if !valid||received!=byTime[key]{return out,errors.New("epistemic time differs from causal event")}
  if len(f[8].([]any))!=0||!sameCBOR(f[9],of[12]){
   return out,errors.New("KnowledgeInput extends disclosed claims/evidence")
  }
  if !bytes.Equal(f[0].([]byte),record.Identity[:]){return out,errors.New("KnowledgeInput ID mismatch")}
  used[key]=true
  out.Inbox=append(out.Inbox,Entry{Observation:obs,Input:record,ReceivedAt:received})
 }
 if len(used)!=len(byID){return out,errors.New("unconfirmed Observation cannot enter context")}
 sort.Slice(out.Inbox,func(i,j int)bool{
  a,z:=out.Inbox[i],out.Inbox[j]
  if a.ReceivedAt!=z.ReceivedAt{return a.ReceivedAt<z.ReceivedAt}
  return bytes.Compare(a.Input.Identity[:],z.Input.Identity[:])<0
 })
 return out,nil
}
func sum(tag,schema string,value any)([32]byte,error){
 raw,err:=codec.CanonicalBytes(tag,schema,1,value)
 if err!=nil{return [32]byte{},err}
 return [32]byte(identity.OpaqueContentHash(raw)),nil
}
func canonicalActor(raw any,actor [2]string)bool{
 v,ok:=raw.([]any)
 return ok&&len(v)==2&&v[0]==actor[0]&&v[1]==actor[1]
}
func sameCBOR(a,b any)bool{
 x,err:=codec.CanonicalPayload(a);if err!=nil{return false}
 y,err:=codec.CanonicalPayload(b)
 return err==nil&&bytes.Equal(x,y)
}
func sameTextList(value any,want []string)bool{
 v,ok:=value.([]any);if !ok||len(v)!=len(want){return false}
 for i:=range want{if v[i]!=want[i]{return false}}
 return true
}

// Package perception derives potential perceptibility from pinned Event
// authority. A PerceptionTask contains no observation, observer list, claim,
// Event payload, or recipient knowledge; task processing belongs downstream.
package perception

import (
 "bytes"
 "errors"
 "math"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
)

type Pins struct {
 Classification []any
 Resolver []any
 Identity []any
}

// Plan is pure and stable under worker/event insertion permutations once
// passed EventOrderKey-sorted canonical Events. The V1 registered predicate
// is event-local-ordinal==0, not Event confidentiality. A task never grants
// an observation or bypasses observer/channel authorization.
func Plan(bundle *policy.Bundle,genesisPins [][]any,run,cycle domain.ID,base,result domain.WorldRevision,
 events []ports.CanonicalRecord,pins Pins)([]ports.CanonicalRecord,error){
 if bundle==nil{return nil,errors.New("perception needs authenticated canonical bundle")}
 return PlanPinned(genesisPins,run,cycle,base,result,events,pins)
}

// PlanPinned is the SQL-verifiable pure classifier. Genesis policy refs
// must already have been validated against the canonical bundle. It derives
// IDs through the registered canonical domain operation rather than reading
// any machine-local fixture at replay time.
func PlanPinned(genesisPins [][]any,run,cycle domain.ID,base,result domain.WorldRevision,
 events []ports.CanonicalRecord,pins Pins)([]ports.CanonicalRecord,error){
 for _,ref:=range []any{pins.Classification,pins.Resolver,pins.Identity}{
  v,ok:=ref.([]any)
  if !ok||len(v)!=3{return nil,errors.New("perception policy/ref missing")}
  id,ok:=v[0].(string)
  if !ok||id==""{return nil,errors.New("perception policy identity invalid")}
  _,ok=v[1].(uint64)
  h,good:=v[2].([]byte)
  if !ok||!good||len(h)!=32{return nil,errors.New("perception policy version/hash invalid")}
 }
 if uint64(base)==math.MaxUint64||uint64(result)!=uint64(base)+1{return nil,errors.New("perception task revisions not adjacent")}
 if pins.Classification[0]!="perception-v1"||pins.Classification[1]!=uint64(1)||
  pins.Identity[0]!="perception-identity-v1"||pins.Identity[1]!=uint64(1){
  return nil,errors.New("unregistered perception classifier/identity policy version")
 }
 // The caller supplies refs from a validated Genesis; task planning never
 // reads fixture files or trusts a machine-local policy lookup.
 allowed:=map[string][]any{}
 for _,ref:=range genesisPins{allowed[ref[0].(string)]=ref}
 for _,ref:=range [][]any{pins.Classification,pins.Resolver,pins.Identity}{
  pinned,found:=allowed[ref[0].(string)]
  if !found||pinned[1]!=ref[1]||!bytes.Equal(pinned[2].([]byte),ref[2].([]byte)){
   return nil,errors.New("perception policy ref not bound to canonical Genesis pins")
  }
 }
 d:=func(tag,schema string,value any)(domain.Digest,error){
  data,err:=codec.CanonicalBytes(tag,schema,1,value)
  if err!=nil{return domain.Digest{},err}
  return identity.OpaqueContentHash(data),nil
 }
 resultRecords:=make([]ports.CanonicalRecord,0)
 seen:=map[domain.ID]bool{}
 for _,event:=range events{
  decoded,err:=codec.StrictDecodePayload(event.Canonical);if err!=nil{return nil,err}
  fields,ok:=decoded.([]any)
  if !ok||len(fields)!=19{return nil,errors.New("perception source Event not canonical")}
  eventID,ok:=fields[0].([]byte)
  eventRun,validRun:=fields[1].([]byte)
  eventCycle,validCycle:=fields[7].([]byte)
  if !ok||!validRun||!validCycle||len(eventID)!=32||
   !bytes.Equal(eventID,event.Identity[:])||!bytes.Equal(eventRun,run[:])||
   !bytes.Equal(eventCycle,cycle[:]){
   return nil,errors.New("perception task event/run/cycle identity differs from authority")
  }
  if seen[domain.ID(event.Identity)]{return nil,errors.New("duplicate perceptible Event identity")}
  seen[domain.ID(event.Identity)]=true
  key,ok:=fields[2].([]any)
  if !ok||len(key)!=6{return nil,errors.New("event source has malformed EventOrderKey")}
  ordinal,ok:=key[4].(uint64)
  if !ok{return nil,errors.New("event local ordinal malformed")}
  h,err:=d("cote.csf.digest.event","cote.csf.schema.event.body",fields[:18])
  if err!=nil{return nil,err}
  digest,ok:=fields[18].([]byte)
  if !ok||!bytes.Equal(digest,h[:]){return nil,errors.New("perception task event digest not verified")}
  id,err:=d("cote.csf.id.event","cote.csf.schema.event-id-preimage",
   []any{run[:],cycle[:],key})
  if err!=nil||id!=domain.Digest(event.Identity){return nil,errors.New("perception source EventOrderKey identity invalid")}
  if ordinal!=0{continue}
  task,err:=d("cote.csf.id.perception-task","cote.csf.schema.perception-task-id-preimage",
   []any{run[:],event.Identity[:],pins.Classification,pins.Resolver,pins.Identity})
  if err!=nil{return nil,err}
  body:=[]any{task[:],run[:],cycle[:],event.Identity[:],uint64(base),uint64(result),
   pins.Classification,pins.Resolver,pins.Identity,uint64(0)}
  raw,err:=codec.CanonicalPayload(body);if err!=nil{return nil,err}
  resultRecords=append(resultRecords,ports.CanonicalRecord{Identity:[32]byte(task),Canonical:raw})
 }
 return resultRecords,nil
}

// Package communication defines canonical, truth-neutral communication
// artefacts. It cannot publish events, mutate world truth, grant observer
// access, or choose the simulation clock.
package communication

import (
 "bytes"
 "errors"
 "fmt"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
)

type Actor [2]string
type Origin struct {Kind uint16; ID domain.ID; Digest domain.Digest}
type Proposition struct {
 Type string
 Polarity uint8 // 0=affirmed, 1=negated, 2=unknown; not a truth flag
 TemporalScope []any
 Content []any
}
type ClaimSpec struct {
 Origin Origin
 Role uint16
 Ordinal uint32
 Proposition Proposition
 AssertedBy Actor
 AssertedAt domain.SimulationInstant
 PriorClaims []domain.CausalRef
 Evidence []domain.CausalRef
}
type TransmissionSpec struct {
 Origin Origin
 Role uint16
 Ordinal uint32
 ClaimRefs []domain.CausalRef
 ActualSender Actor
 PresentedSender string
 Recipients []Actor
 Channel string
 SentAt domain.SimulationInstant
 DeliveryOccurrence *domain.ID
 Facts []any
 Status uint8 // 0 in transit, 1 delivered, 2 failed, 3 cancelled
}

func actorValue(actor Actor)([]any,error){
 if err:=domain.ValidateMachineID(actor[0]);err!=nil{return nil,err}
 if err:=domain.ValidateMachineID(actor[1]);err!=nil{return nil,err}
 return []any{actor[0],actor[1]},nil
}
func originValue(origin Origin)([]any,error){
 if origin.Kind>27{return nil,errors.New("unregistered causal origin kind")}
 if origin.ID==([32]byte{})||origin.Digest==([32]byte{}){return nil,errors.New("origin missing causal ID/digest")}
 return []any{uint64(origin.Kind),origin.ID[:],origin.Digest[:]},nil
}
func refs(values []domain.CausalRef,kind int)([]any,error){
 if len(values)>1024{return nil,errors.New("too many communication references")}
 out:=make([]any,0,len(values));seen:=map[domain.ID]bool{}
 for _,v:=range values{
  if kind>=0&&v.Kind!=uint16(kind){return nil,errors.New("wrong claim ref kind")}
  if v.ID==([32]byte{})||v.Digest==([32]byte{})||seen[v.ID]{return nil,errors.New("duplicate or empty claim/evidence reference")}
  seen[v.ID]=true;out=append(out,[]any{uint64(v.Kind),v.ID[:],v.Digest[:]})
 }
 return out,nil
}
func checkedTyped(value []any)error{
 if len(value)!=4{return errors.New("typed communication value must be [schema,version,envelope,digest]")}
 schema,ok:=value[0].(string);if !ok{return errors.New("typed schema absent")}
 version,ok:=value[1].(uint64);if !ok||version!=1{return errors.New("typed version must be 1")}
 encoded,ok:=value[2].([]byte);if !ok{return errors.New("typed canonical envelope absent")}
 stored,ok:=value[3].([]byte);if !ok||len(stored)!=32{return errors.New("typed digest invalid")}
 env,err:=codec.StrictDecodeEnvelope(encoded);if err!=nil{return err}
 digest:=identity.OpaqueContentHash(encoded)
 if schema!=env.SchemaID||env.SchemaVersion!=1||!bytes.Equal(digest[:],stored){
  return errors.New("typed value descriptor/digest mismatch")
 }
 return nil
}
func MakeTyped(tag,schema string,content any)([]any,error){
 raw,err:=codec.CanonicalBytes(tag,schema,1,content);if err!=nil{return nil,err}
 h:=identity.OpaqueContentHash(raw)
 return []any{schema,uint64(1),raw,h[:]},nil
}
func NewClaim(bundle *policy.Bundle,s ClaimSpec)(ports.CanonicalRecord,error){
 var zero ports.CanonicalRecord
 if bundle==nil{return zero,errors.New("claim requires canonical policy bundle")}
 origin,err:=originValue(s.Origin);if err!=nil{return zero,err}
 if err:=domain.ValidateMachineID(s.Proposition.Type);err!=nil{return zero,err}
 if s.Proposition.Polarity>2{return zero,errors.New("claim polarity invalid")}
 if err:=checkedTyped(s.Proposition.TemporalScope);err!=nil{return zero,err}
 if err:=checkedTyped(s.Proposition.Content);err!=nil{return zero,err}
 actor,err:=actorValue(s.AssertedBy);if err!=nil{return zero,err}
 prev,err:=refs(s.PriorClaims,18);if err!=nil{return zero,err}
 evidence,err:=refs(s.Evidence,-1);if err!=nil{return zero,err}
 if len(evidence)>0{return zero,errors.New("first slice does not authenticate external evidence chains")}
 d:=identity.New(bundle)
 id,_,err:=d.Sum("cote.csf.id.claim","cote.csf.schema.claim-id-preimage",1,
  []any{origin,uint64(s.Role),uint64(s.Ordinal)})
 if err!=nil{return zero,err}
 proposition:=[]any{s.Proposition.Type,uint64(s.Proposition.Polarity),s.Proposition.TemporalScope,s.Proposition.Content}
 body:=[]any{id[:],origin,uint64(s.Role),uint64(s.Ordinal),proposition,
  []any{uint64(1),actor},int64(s.AssertedAt),prev,evidence}
 digest,_,err:=d.Sum("cote.csf.digest.claim","cote.csf.schema.claim.body",1,body)
 if err!=nil{return zero,err}
 raw,err:=codec.CanonicalPayload(append(body,digest[:]));if err!=nil{return zero,err}
 return ports.CanonicalRecord{Identity:[32]byte(id),Canonical:raw},nil
}
func NewTransmission(bundle *policy.Bundle,s TransmissionSpec)(ports.CanonicalRecord,error){
 var zero ports.CanonicalRecord
 if bundle==nil{return zero,errors.New("transmission requires canonical policy bundle")}
 origin,err:=originValue(s.Origin);if err!=nil{return zero,err}
 sender,err:=actorValue(s.ActualSender);if err!=nil{return zero,err}
 if len(s.PresentedSender)==0||len(s.PresentedSender)>512{return zero,errors.New("presented sender missing or excessive")}
 if _,err:=registeredChannelResult(s.Channel);err!=nil{return zero,err}
 if s.Status!=0{return zero,errors.New("cannot mint terminal Transmission outside a causal Event")}
 if len(s.Recipients)!=1{return zero,errors.New("V1-7 communication must have exactly one recipient")}
 actors:=make([]any,0,len(s.Recipients))
 prior:=""
 for _,item:=range s.Recipients{
  v,err:=actorValue(item);if err!=nil{return zero,err}
  key:=item[0]+"/"+item[1]
  if prior!=""&&key<=prior{return zero,errors.New("recipient list must be sorted and duplicate-free")}
  prior=key;actors=append(actors,v)
 }
 claims,err:=refs(s.ClaimRefs,18);if err!=nil{return zero,err}
 if len(claims)==0{return zero,errors.New("transmission requires claim refs")}
 if len(s.Facts)!=0{return zero,errors.New("unverified private transmission facts forbidden")}
 var due any=[]any{uint64(0)}
 if s.DeliveryOccurrence!=nil{
  if *s.DeliveryOccurrence==([32]byte{}){return zero,errors.New("empty due occurrence ID")}
  due=[]any{uint64(1),s.DeliveryOccurrence[:]}
 }
 id,_,err:=identity.New(bundle).Sum("cote.csf.id.transmission","cote.csf.schema.transmission-id-preimage",1,
  []any{origin,uint64(s.Role),uint64(s.Ordinal)})
 if err!=nil{return zero,err}
 body:=[]any{id[:],origin,uint64(s.Role),uint64(s.Ordinal),[]any{uint64(0),claims},sender,
  s.PresentedSender,actors,s.Channel,int64(s.SentAt),due,[]any{},uint64(s.Status)}
 digest,_,err:=identity.New(bundle).Sum("cote.csf.digest.transmission","cote.csf.schema.transmission.body",1,body)
 if err!=nil{return zero,err}
 raw,err:=codec.CanonicalPayload(append(body,digest[:]));if err!=nil{return zero,err}
 return ports.CanonicalRecord{Identity:[32]byte(id),Canonical:raw},nil
}
func (p Proposition) String()string{return fmt.Sprintf("%s/%d (unverified)",p.Type,p.Polarity)}

func registeredChannelResult(channel string)(bool,error){
 switch channel{
 case "school.message.v1":return true,nil
 case "school.drop.v1":return false,nil
 default:return false,errors.New("channel lacks registered deterministic delivery policy")
 }
}

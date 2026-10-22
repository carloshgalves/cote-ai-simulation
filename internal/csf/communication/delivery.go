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

// VerifiedClaims are canonical *allegations*; verification of their envelope
// never entails truth of the proposition in the authoritative world.
type VerifiedClaim struct{
 ID domain.ID
 Digest domain.Digest
 Origin domain.CausalRef
 Proposition []any
 AssertedBy Actor
 AssertedAt domain.SimulationInstant
}

type VerifiedTransmission struct{
 ID domain.ID
 Digest domain.Digest
 Origin domain.CausalRef
 ClaimRefs []domain.CausalRef
 ActualSender Actor
 PresentedSender string
 Recipient Actor
 Channel string
 SentAt domain.SimulationInstant
 Occurrence domain.ID
 Status uint8
}

func bytesID(v any)(domain.ID,bool){
 var result domain.ID
 b,ok:=v.([]byte);if !ok||len(b)!=32{return result,false}
 copy(result[:],b);return result,true
}
func asRef(v any)(domain.CausalRef,bool){
 var result domain.CausalRef
 a,ok:=v.([]any);if !ok||len(a)!=3{return result,false}
 kind,ok:=a[0].(uint64);if !ok||kind>27{return result,false}
 id,ok:=bytesID(a[1]);if !ok||id==([32]byte{}){return result,false}
 dg,ok:=bytesID(a[2]);if !ok||dg==([32]byte{}){return result,false}
 result.Kind=uint16(kind);result.ID=id;result.Digest=domain.Digest(dg)
 return result,true
}
func asActor(v any)(Actor,bool){
 var a Actor
 x,ok:=v.([]any);if !ok||len(x)!=2{return a,false}
 for i:=0;i<2;i++{
  s,ok:=x[i].(string);if !ok||domain.ValidateMachineID(s)!=nil{return a,false}
  a[i]=s
 }
 return a,true
}
func AuthenticateClaim(bundle *policy.Bundle,record ports.CanonicalRecord)(VerifiedClaim,error){
 var zero VerifiedClaim
 if bundle==nil{return zero,errors.New("missing policy bundle")}
 decoded,err:=codec.StrictDecodePayload(record.Canonical);if err!=nil{return zero,err}
 f,ok:=decoded.([]any);if !ok||len(f)!=10{return zero,errors.New("Claim shape invalid")}
 if !bytes.Equal(rawBytes(f[0]),record.Identity[:]){return zero,errors.New("Claim identity bytes wrong")}
 origin,ok:=asRef(f[1]);if !ok{return zero,errors.New("Claim origin invalid")}
 role,roleOK:=f[2].(uint64);ordinal,ordinalOK:=f[3].(uint64)
 if !roleOK||!ordinalOK||role>65535||ordinal>4294967295{return zero,errors.New("Claim role invalid")}
 proposition,ok:=f[4].([]any);if !ok||len(proposition)!=4{return zero,errors.New("Proposition malformed")}
 typ,ok:=proposition[0].(string);if !ok||domain.ValidateMachineID(typ)!=nil{return zero,errors.New("Proposition type invalid")}
 polarity,ok:=proposition[1].(uint64);if !ok||polarity>2{return zero,errors.New("Proposition polarity invalid")}
 scope,ok:=proposition[2].([]any);if !ok{return zero,errors.New("Proposition time scope malformed")}
 content,ok:=proposition[3].([]any);if !ok{return zero,errors.New("Proposition content malformed")}
 if err:=checkedTyped(scope);err!=nil{return zero,err}
 if err:=checkedTyped(content);err!=nil{return zero,err}
 present,ok:=f[5].([]any);if !ok||len(present)!=2||present[0]!=uint64(1){return zero,errors.New("Claim actual author required")}
 actor,ok:=asActor(present[1]);if !ok{return zero,errors.New("Claim author invalid")}
 time,ok:=causalTime(f[6]);if !ok{return zero,errors.New("Claim asserted-at invalid")}
 prior,ok:=f[7].([]any);if !ok{return zero,errors.New("Claim priors malformed")}
 if _,err:=validateRefArray(prior,18);err!=nil{return zero,err}
 evidence,ok:=f[8].([]any);if !ok||len(evidence)!=0{return zero,errors.New("unverified Claim evidence refused")}
 id,_,err:=identity.New(bundle).Sum("cote.csf.id.claim","cote.csf.schema.claim-id-preimage",1,
  []any{f[1],f[2],f[3]});if err!=nil{return zero,err}
 if [32]byte(id)!=record.Identity{return zero,errors.New("Claim forged origin/role identity")}
 digest,_,err:=identity.New(bundle).Sum("cote.csf.digest.claim","cote.csf.schema.claim.body",1,f[:9])
 if err!=nil{return zero,err}
 if !bytes.Equal(rawBytes(f[9]),digest[:]){return zero,errors.New("Claim body tampered")}
 return VerifiedClaim{ID:domain.ID(id),Digest:digest,Origin:origin,Proposition:proposition,AssertedBy:actor,AssertedAt:time},nil
}
func validateRefArray(values []any,kind int)([]domain.CausalRef,error){
 seen:=map[domain.ID]bool{}
 out:=make([]domain.CausalRef,0,len(values))
 if len(values)>1024{return nil,errors.New("reference array excessive")}
 for _,v:=range values{
  ref,ok:=asRef(v)
  if !ok||kind>=0&&ref.Kind!=uint16(kind)||seen[ref.ID]{return nil,errors.New("invalid or duplicate claim reference")}
  seen[ref.ID]=true;out=append(out,ref)
 }
 return out,nil
}
func rawBytes(v any)[]byte{x,_:=v.([]byte);return x}
func causalTime(v any)(domain.SimulationInstant,bool){
 switch x:=v.(type){
 case int64:return domain.SimulationInstant(x),true
 case uint64:
  if x<=uint64(^uint64(0)>>1){return domain.SimulationInstant(x),true}
 }
 return 0,false
}
func AuthenticateTransmission(bundle *policy.Bundle,record ports.CanonicalRecord)(VerifiedTransmission,error){
 var zero VerifiedTransmission
 if bundle==nil{return zero,errors.New("missing policy bundle")}
 decoded,err:=codec.StrictDecodePayload(record.Canonical);if err!=nil{return zero,err}
 f,ok:=decoded.([]any);if !ok||len(f)!=14{return zero,errors.New("Transmission shape invalid")}
 if !bytes.Equal(rawBytes(f[0]),record.Identity[:]){return zero,errors.New("Transmission identity differs from stored record")}
 origin,ok:=asRef(f[1]);if !ok{return zero,errors.New("Transmission origin absent")}
 role,roleOK:=f[2].(uint64);ordinal,ordOK:=f[3].(uint64)
 if !roleOK||!ordOK||role>65535||ordinal>4294967295{return zero,errors.New("Transmission role invalid")}
 content,ok:=f[4].([]any);if !ok||len(content)!=2||content[0]!=uint64(0){return zero,errors.New("unregistered transmission content variant")}
 items,ok:=content[1].([]any);if !ok||len(items)==0{return zero,errors.New("Transmission claim links missing")}
 claims,err:=validateRefArray(items,18);if err!=nil{return zero,err}
 actual,ok:=asActor(f[5]);if !ok{return zero,errors.New("actual sender invalid")}
 presented,ok:=f[6].(string);if !ok||presented==""||len(presented)>512{return zero,errors.New("presented sender missing")}
 rec,ok:=f[7].([]any);if !ok||len(rec)!=1{return zero,errors.New("current V1-7 route must target exactly one actor")}
 recipient,ok:=asActor(rec[0]);if !ok{return zero,errors.New("intended recipient invalid")}
 channel,ok:=f[8].(string);if !ok||domain.ValidateMachineID(channel)!=nil{return zero,errors.New("transmission channel invalid")}
 sent,ok:=causalTime(f[9]);if !ok{return zero,errors.New("Transmission sent-at invalid")}
 due,ok:=f[10].([]any);if !ok||len(due)!=2||due[0]!=uint64(1){return zero,errors.New("delivery must be backed by ScheduledOccurrence")}
 occur,ok:=bytesID(due[1]);if !ok{return zero,errors.New("due occurrence id invalid")}
 facts,ok:=f[11].([]any);if !ok||len(facts)!=0{return zero,errors.New("unexpected private channel facts")}
 status,ok:=f[12].(uint64);if !ok||status>3{return zero,errors.New("unregistered Transmission state")}
 id,_,err:=identity.New(bundle).Sum("cote.csf.id.transmission","cote.csf.schema.transmission-id-preimage",1,
  []any{f[1],f[2],f[3]});if err!=nil{return zero,err}
 if [32]byte(id)!=record.Identity{return zero,errors.New("Transmission forged identity")}
 digest,_,err:=identity.New(bundle).Sum("cote.csf.digest.transmission","cote.csf.schema.transmission.body",1,f[:13])
 if err!=nil{return zero,err}
 if !bytes.Equal(rawBytes(f[13]),digest[:]){return zero,errors.New("Transmission forged digest")}
 return VerifiedTransmission{ID:domain.ID(id),Digest:digest,Origin:origin,ClaimRefs:claims,ActualSender:actual,
  PresentedSender:presented,Recipient:recipient,Channel:channel,SentAt:sent,
  Occurrence:occur,Status:uint8(status)},nil
}

// DeliveryNotice is a *projection*, not the EventStore or a KnowledgeInput.
// Only a CommitCoordinator may publish an actual Event. It deliberately has
// no field capable of carrying actual_sender or truth classification.
type DeliveryNotice struct{
 Recipient Actor
 PresentedSender string
 Proposition []any
 ClaimID domain.ID
 At domain.SimulationInstant
 Channel string
}
type DeliveryResult struct{
 EventType string
 Notice *DeliveryNotice
 NextStatus uint8
}

// PlanDueDelivery is deterministic over a causally admitted delivery
// occurrence and an authenticated fixed channel result. It never uses
// worker elapsed time, opens EventStore or directly writes recipient inbox.
func PlanDueDelivery(bundle *policy.Bundle,txRecord,claimRecord ports.CanonicalRecord,
 dueOccurrence domain.ID,dueAt,now domain.SimulationInstant)(DeliveryResult,error){
 var zero DeliveryResult
 transmission,err:=AuthenticateTransmission(bundle,txRecord);if err!=nil{return zero,err}
 claim,err:=AuthenticateClaim(bundle,claimRecord);if err!=nil{return zero,err}
 if transmission.Status!=0{return zero,errors.New("terminal transmission cannot be re-delivered")}
 if transmission.Occurrence!=dueOccurrence||dueOccurrence==([32]byte{}){
  return zero,errors.New("delivery occurrence not bound to transmission")
 }
 if now<dueAt||dueAt<transmission.SentAt{return zero,errors.New("delivery before its causal due time")}
 matched:=false
 for _,ref:=range transmission.ClaimRefs{
  if ref.ID==claim.ID&&ref.Digest==claim.Digest{matched=true}
 }
 if !matched{return zero,errors.New("Transmission references a different Claim")}
 if claim.Origin!=transmission.Origin{return zero,errors.New("Claim and transmission have different causal origins")}
 if claim.AssertedBy!=transmission.ActualSender||claim.AssertedAt!=transmission.SentAt{
  return zero,errors.New("authoritative claim author/time mismatch")
 }
 delivered,err:=registeredChannelResult(transmission.Channel)
 if err!=nil{return zero,err}
 if !delivered{return DeliveryResult{EventType:"communication.delivery_failed",NextStatus:2},nil}
 notice:=DeliveryNotice{Recipient:transmission.Recipient,PresentedSender:transmission.PresentedSender,
  ClaimID:claim.ID,Proposition:claim.Proposition,At:now,Channel:transmission.Channel}
 return DeliveryResult{EventType:"communication.delivered",Notice:&notice,NextStatus:1},nil
}
func (n DeliveryNotice) RedactedValue()([]any,error){
 actor,err:=actorValue(n.Recipient);if err!=nil{return nil,err}
 if len(n.Proposition)!=4{return nil,fmt.Errorf("noncanonical proposition")}
 // Typed content is just a reported assertion, not authoritative truth.
 return []any{actor,n.PresentedSender,n.ClaimID[:],n.Proposition,int64(n.At),n.Channel},nil
}

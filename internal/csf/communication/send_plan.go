package communication

import (
 "errors"
 "math"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
)

// SendSpec is an *unpublished* deterministic preparation request. The
// CommitCoordinator must validate its source against an admitted and sealed
// ActionProposal, then publish ALL records in one terminal transaction.
// No constructor here has a database or clock writer.
type SendSpec struct{
 Run domain.ID
 Source Origin // a verified ActionProposal; caller may not grant authority
 ActualSender Actor
 PresentedSender string
 Recipient Actor
 SentAt domain.SimulationInstant
 DueAt domain.SimulationInstant
 Channel string
 Proposition Proposition
}
type SendPlan struct{
 Run domain.ID
 Claim ports.CanonicalRecord
 Transmission ports.CanonicalRecord
 Occurrence ports.CanonicalRecord
}

// PrepareSend creates the exact canonical Claim, IN_TRANSIT Transmission and
// delivery ScheduledOccurrence. The ID of the due occurrence derives from
// the original ActionProposal input, not a wall-clock worker request.
// The input-based origin avoids a cycle between send Event, occurrence,
// transmission ref and Event payload digest.
func PrepareSend(bundle *policy.Bundle,s SendSpec)(SendPlan,error){
 var zero SendPlan
 if bundle==nil||s.Run==([32]byte{})||s.Source.Kind!=5{return zero,errors.New("send requires run and admitted ActionProposal origin")}
 if s.DueAt<=s.SentAt||s.DueAt==domain.SimulationInstant(math.MaxInt64){
  return zero,errors.New("delivery occurrence must be after send")
 }
 sender,err:=actorValue(s.ActualSender);if err!=nil{return zero,err}
 _=sender
 if _,err:=actorValue(s.Recipient);err!=nil{return zero,err}
 // ActionProposal is the slot-response kind 0 in canonical input-ref.
 createdBy:=[]any{uint64(1),[]any{uint64(0),uint64(0),s.Source.ID[:],s.Source.Digest[:]}}
 d:=identity.New(bundle)
 occID,_,err:=d.Sum("cote.csf.id.occurrence","cote.csf.schema.occurrence-id-preimage",1,
  []any{createdBy,uint64(0),uint64(0)})
 if err!=nil{return zero,err}
 claim,err:=NewClaim(bundle,ClaimSpec{Origin:s.Source,Role:0,Ordinal:0,AssertedBy:s.ActualSender,
  AssertedAt:s.SentAt,Proposition:s.Proposition})
 if err!=nil{return zero,err}
 verified,err:=AuthenticateClaim(bundle,claim);if err!=nil{return zero,err}
 dueID:=domain.ID(occID)
 tx,err:=NewTransmission(bundle,TransmissionSpec{Origin:s.Source,Role:0,Ordinal:0,
  ClaimRefs:[]domain.CausalRef{{Kind:18,ID:domain.ID(claim.Identity),Digest:verified.Digest}},
  ActualSender:s.ActualSender,PresentedSender:s.PresentedSender,
  Recipients:[]Actor{s.Recipient},Channel:s.Channel,SentAt:s.SentAt,
  DeliveryOccurrence:&dueID,Status:0})
 if err!=nil{return zero,err}
 parsed,err:=AuthenticateTransmission(bundle,tx);if err!=nil{return zero,err}
 payload,err:=MakeTyped("cote.csf.communication.delivery.v1","cote.csf.schema.typed-value",
  []any{tx.Identity[:],parsed.Digest[:],claim.Identity[:],verified.Digest[:]})
 if err!=nil{return zero,err}
 // Every delivery has one fixed due date and cannot self-recur.
 body:=[]any{occID[:],s.Run[:],"communication",int64(s.DueAt),
  []any{int64(s.DueAt),uint64(0)},"communication.delivery",uint64(1),payload,
  uint64(0),createdBy,uint64(0),uint64(0),[]any{uint64(0)},[]any{uint64(0)}}
 digest,_,err:=d.Sum("cote.csf.digest.occurrence-unit","cote.csf.schema.scheduled-occurrence.body",1,body)
 if err!=nil{return zero,err}
 raw,err:=codec.CanonicalPayload(append(body,digest[:]));if err!=nil{return zero,err}
 return SendPlan{Run:s.Run,Claim:claim,Transmission:tx,
  Occurrence:ports.CanonicalRecord{Identity:[32]byte(occID),Canonical:raw}},nil
}

// ValidateOccurrence connects a due source back to both communication
// artefacts, checking canonical identity, its source ActionProposal origin,
// its due coordinate, and embedded transmission+claim digest refs. The
// scheduler's actual admission/fence still belongs to its existing owner.
func ValidateOccurrence(bundle *policy.Bundle,p SendPlan)(domain.SimulationInstant,error){
 if bundle==nil{return 0,errors.New("missing bundle")}
 c,err:=AuthenticateClaim(bundle,p.Claim);if err!=nil{return 0,err}
 tx,err:=AuthenticateTransmission(bundle,p.Transmission);if err!=nil{return 0,err}
 if tx.Occurrence!=domain.ID(p.Occurrence.Identity)||p.Run==([32]byte{}){
  return 0,errors.New("transmission due occurrence mismatch")
 }
 if c.Origin!=tx.Origin||tx.Origin.Kind!=5{return 0,errors.New("transmission and claim origin mismatch")}
 createdBy:=[]any{uint64(1),[]any{uint64(0),uint64(0),tx.Origin.ID[:],tx.Origin.Digest[:]}}
 expectedID,_,err:=identity.New(bundle).Sum("cote.csf.id.occurrence","cote.csf.schema.occurrence-id-preimage",1,
  []any{createdBy,uint64(0),uint64(0)})
 if err!=nil||domain.ID(expectedID)!=tx.Occurrence{return 0,errors.New("occurrence not derived from source ActionProposal")}
 value,err:=codec.StrictDecodePayload(p.Occurrence.Canonical);if err!=nil{return 0,err}
 fields,ok:=value.([]any)
 if !ok||len(fields)!=15||!bytesEqualID(fields[0],domain.ID(p.Occurrence.Identity))||
  !bytesEqualID(fields[1],p.Run)||fields[2]!="communication"||fields[5]!="communication.delivery"||
  fields[6]!=uint64(1)||fields[8]!=uint64(0)||fields[10]!=uint64(0)||fields[11]!=uint64(0)||
  !sameCBOR(fields[9],createdBy)||!sameCBOR(fields[12],[]any{uint64(0)})||
  !sameCBOR(fields[13],[]any{uint64(0)}){
  return 0,errors.New("delivery occurrence forged")
 }
 due,ok:=causalTime(fields[3]);if !ok||due<=tx.SentAt{return 0,errors.New("delivery due invalid")}
 coordinate,ok:=fields[4].([]any)
 if !ok||len(coordinate)!=2||coordinate[1]!=uint64(0){return 0,errors.New("delivery eligibility malformed")}
 eligible,ok:=causalTime(coordinate[0]);if !ok||eligible!=due{return 0,errors.New("delivery eligibility differs from due")}
 typed,ok:=fields[7].([]any);if !ok{return 0,errors.New("delivery payload invalid")}
 if err:=checkedTyped(typed);err!=nil{return 0,err}
 env,err:=codec.StrictDecodeEnvelope(rawBytes(typed[2]));if err!=nil{return 0,err}
 if env.DomainTag!="cote.csf.communication.delivery.v1"{return 0,errors.New("delivery domain wrong")}
 refs,ok:=env.Value.([]any)
 if !ok||len(refs)!=4||!bytesEqualID(refs[0],tx.ID)||!bytesEqualID(refs[2],c.ID)||
  !bytesEqualDigest(refs[1],tx.Digest)||!bytesEqualDigest(refs[3],c.Digest){
  return 0,errors.New("delivery occurrence references forged claim/transmission")
 }
 digest,_,err:=identity.New(bundle).Sum("cote.csf.digest.occurrence-unit",
  "cote.csf.schema.scheduled-occurrence.body",1,fields[:14])
 if err!=nil||!bytesEqualDigest(fields[14],digest){return 0,errors.New("delivery occurrence digest altered")}
 return due,nil
}
func bytesEqualID(v any,want domain.ID)bool{x,ok:=v.([]byte);return ok&&len(x)==32&&string(x)==string(want[:])}
func bytesEqualDigest(v any,want domain.Digest)bool{x,ok:=v.([]byte);return ok&&len(x)==32&&string(x)==string(want[:])}

func sameCBOR(a,b any)bool{
 x,e1:=codec.CanonicalPayload(a)
 y,e2:=codec.CanonicalPayload(b)
 return e1==nil&&e2==nil&&string(x)==string(y)
}

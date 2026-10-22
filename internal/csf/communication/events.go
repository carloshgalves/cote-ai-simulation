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

// EventContext is supplied by the terminal CommitCoordinator, never by a
// perception worker or messaging adapter. It must be matched to persisted
// CycleCommit/Clock/AdmissionFence before any Event is published.
type EventContext struct{
 Run domain.ID
 Cycle domain.ID
 BaseRevision domain.WorldRevision
 At domain.SimulationInstant
 Sequence domain.LogicalSequence
}
func NewSentEvent(bundle *policy.Bundle,ctx EventContext,plan SendPlan,proposal Origin)(
 ports.CanonicalRecord,error){
 var zero ports.CanonicalRecord
 if bundle==nil||ctx.Run!=plan.Run||proposal.Kind!=5{return zero,errors.New("send Event missing causal origin")}
 tx,err:=AuthenticateTransmission(bundle,plan.Transmission);if err!=nil{return zero,err}
 claim,err:=AuthenticateClaim(bundle,plan.Claim);if err!=nil{return zero,err}
 due,err:=ValidateOccurrence(bundle,plan)
 if err!=nil||due<=ctx.At||tx.SentAt!=ctx.At||tx.Origin.Kind!=5||tx.Origin.ID!=proposal.ID||
  tx.Origin.Digest!=proposal.Digest||claim.Origin!=tx.Origin{
  return zero,errors.New("sent Event and authored message do not resolve to admitted proposal")
 }
 typed,err:=MakeTyped("cote.csf.communication.sent.v1","cote.csf.schema.typed-value",
  []any{tx.ID[:],tx.Digest[:],claim.ID[:],claim.Digest[:],
   plan.Occurrence.Identity[:],int64(due)})
 if err!=nil{return zero,err}
 key:=[]any{uint64(1),proposal.ID[:],"communication.send",uint64(1),uint64(0),uint64(0)}
 input:=[]any{uint64(0),uint64(0),proposal.ID[:],proposal.Digest[:]}
 // A sending action is private and grants no recipient a percept.
 return makeEvent(bundle,ctx,key,"communication.sent",typed,[]any{input},[]any{},2,"communication.send",nil)
}
func NewDueEvent(bundle *policy.Bundle,ctx EventContext,plan SendPlan,result DeliveryResult,occurrence Origin)(
 ports.CanonicalRecord,error){
 var zero ports.CanonicalRecord
 if bundle==nil||ctx.Run!=plan.Run||occurrence.Kind!=6||
  occurrence.ID!=domain.ID(plan.Occurrence.Identity){
  return zero,errors.New("delivery Event must consume its authenticated occurrence")
 }
 due,err:=ValidateOccurrence(bundle,plan);if err!=nil{return zero,err}
 if ctx.At<due{return zero,errors.New("delivery Event cannot precede due date")}
 var typed []any
 var actorRefs []any
 var confidentiality uint64=2
 if result.NextStatus==1&&result.EventType=="communication.delivered"&&result.Notice!=nil{
  if result.Notice.At!=ctx.At{return zero,errors.New("delivery notice timestamp not causal")}
  v,err:=result.Notice.RedactedValue();if err!=nil{return zero,err}
  typed,err=MakeTyped("cote.csf.communication.delivered.v1","cote.csf.schema.typed-value",v)
  if err!=nil{return zero,err}
  a,err:=actorValue(result.Notice.Recipient);if err!=nil{return zero,err}
  actorRefs=[]any{a};confidentiality=1
 }else if result.NextStatus==2&&result.EventType=="communication.delivery_failed"&&result.Notice==nil{
  tx,err:=AuthenticateTransmission(bundle,plan.Transmission);if err!=nil{return zero,err}
  typed,err=MakeTyped("cote.csf.communication.failure.v1","cote.csf.schema.typed-value",
   []any{tx.ID[:],plan.Occurrence.Identity[:],int64(ctx.At)})
  if err!=nil{return zero,err}
 }else{return zero,errors.New("unregistered communication terminal outcome")}
 key:=[]any{uint64(1),occurrence.ID[:],"communication.delivery",uint64(1),uint64(0),uint64(0)}
 source:=[]any{uint64(2),occurrence.ID[:],occurrence.Digest[:]}
 return makeEvent(bundle,ctx,key,result.EventType,typed,[]any{source},actorRefs,
  confidentiality,"communication.delivery",nil)
}
func makeEvent(bundle *policy.Bundle,ctx EventContext,key []any,kind string,typed []any,
 sources,actors []any,confidentiality uint64,producer string,parents []any)(
 ports.CanonicalRecord,error){
 var zero ports.CanonicalRecord
 if ctx.Run==([32]byte{})||ctx.Cycle==([32]byte{})||
  ctx.Sequence==domain.LogicalSequence(math.MaxUint64){return zero,errors.New("causal Event context invalid")}
 id,_,err:=identity.New(bundle).Sum("cote.csf.id.event","cote.csf.schema.event-id-preimage",1,
  []any{ctx.Run[:],ctx.Cycle[:],key})
 if err!=nil{return zero,err}
 if parents==nil{parents=[]any{}}
 body:=[]any{id[:],ctx.Run[:],key,kind,uint64(1),int64(ctx.At),uint64(ctx.Sequence),
  ctx.Cycle[:],uint64(ctx.BaseRevision),typed,parents,sources,actors,[]any{},
  []any{uint64(0)},confidentiality,producer,uint64(1)}
 digest,_,err:=identity.New(bundle).Sum("cote.csf.digest.event","cote.csf.schema.event.body",1,body)
 if err!=nil{return zero,err}
 raw,err:=codec.CanonicalPayload(append(body,digest[:]));if err!=nil{return zero,err}
 return ports.CanonicalRecord{Identity:[32]byte(id),Canonical:raw},nil
}

package communication

import (
 "bytes"
 "errors"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
)

// Trace is a read-only, truth-neutral projection from canonically proven
// Event records. It does not publish EventStore, update a transmission, infer
// belief, or bypass the CommitCoordinator's admission and ledger integrity.
type Trace struct{
 Status uint8 // 0 in transit, 1 delivered, 2 failed
 SentAt domain.SimulationInstant
 DeliveredAt *domain.SimulationInstant
 Notice *DeliveryNotice
}

func eventContext(record ports.CanonicalRecord)(EventContext,[]any,error){
 var zero EventContext
 parsed,err:=codec.StrictDecodePayload(record.Canonical)
 if err!=nil{return zero,nil,err}
 fields,ok:=parsed.([]any)
 if !ok||len(fields)!=19||!bytes.Equal(rawBytes(fields[0]),record.Identity[:]){
  return zero,nil,errors.New("communication Event CBOR shape/key invalid")
 }
 run,ok:=bytesID(fields[1]);if !ok{return zero,nil,errors.New("Event run invalid")}
 cycle,ok:=bytesID(fields[7]);if !ok{return zero,nil,errors.New("Event cycle invalid")}
 base,ok:=fields[8].(uint64);if !ok{return zero,nil,errors.New("Event world revision invalid")}
 seq,ok:=fields[6].(uint64);if !ok{return zero,nil,errors.New("Event logical sequence invalid")}
 at,ok:=causalTime(fields[5]);if !ok{return zero,nil,errors.New("Event instant invalid")}
 return EventContext{Run:run,Cycle:cycle,BaseRevision:domain.WorldRevision(base),
  At:at,Sequence:domain.LogicalSequence(seq)},fields,nil
}

// ReplayCommunication is strict over an already-authenticated world event
// subset. A missing delivery is IN_TRANSIT, not a failed read, and a delivery
// is visible only after a later due occurrence Event. Duplicate/forked
// terminals cannot produce a second addressed receipt. The full replay
// additionally verifies that each Event belongs to an actual CycleCommit.
func ReplayCommunication(bundle *policy.Bundle,plan SendPlan,events []ports.CanonicalRecord)(Trace,error){
 var trace Trace
 if bundle==nil{return trace,errors.New("communication replay missing bundle")}
 tx,err:=AuthenticateTransmission(bundle,plan.Transmission)
 if err!=nil{return trace,err}
 due,err:=ValidateOccurrence(bundle,plan)
 if err!=nil{return trace,err}
 if len(events)<1||len(events)>2{return trace,errors.New("communication trace has absent/duplicate Event")}
 first,firstFields,err:=eventContext(events[0])
 if err!=nil{return trace,err}
 source:=Origin{Kind:tx.Origin.Kind,ID:tx.Origin.ID,Digest:tx.Origin.Digest}
 sent,err:=NewSentEvent(bundle,first,plan,source)
 if err!=nil||sent.Identity!=events[0].Identity||!bytes.Equal(sent.Canonical,events[0].Canonical){
  return trace,errors.New("trace sent Event differs from authoritative claim")
 }
 if firstFields[3]!="communication.sent"||first.At!=tx.SentAt{return trace,errors.New("wrong source Event order")}
 trace=Trace{Status:0,SentAt:first.At}
 if len(events)==1{return trace,nil}
 second,secondFields,err:=eventContext(events[1]);if err!=nil{return Trace{},err}
 if second.Run!=first.Run||second.Cycle==first.Cycle||
  second.BaseRevision<=first.BaseRevision||second.Sequence<=first.Sequence||
  second.At<due {
  return Trace{},errors.New("due communication cannot run before source commit/occurrence")
 }
 planned,err:=PlanDueDelivery(bundle,plan.Transmission,plan.Claim,
  domain.ID(plan.Occurrence.Identity),due,second.At)
 if err!=nil{return Trace{},err}
 parsed,err:=codec.StrictDecodePayload(plan.Occurrence.Canonical);if err!=nil{return Trace{},err}
 occ:=parsed.([]any)
 dig,ok:=bytesID(occ[14]);if !ok{return Trace{},errors.New("due occurrence digest missing")}
 dueOrigin:=Origin{Kind:6,ID:domain.ID(plan.Occurrence.Identity),Digest:domain.Digest(dig)}
 expected,err:=NewDueEvent(bundle,second,plan,planned,dueOrigin)
 if err!=nil||expected.Identity!=events[1].Identity||
  !bytes.Equal(expected.Canonical,events[1].Canonical)||
  secondFields[3]!=planned.EventType {
  return Trace{},errors.New("due Event forged, missing or incorrectly addressed")
 }
 at:=second.At
 trace.Status=planned.NextStatus
 trace.DeliveredAt=&at
 trace.Notice=planned.Notice
 return trace,nil
}

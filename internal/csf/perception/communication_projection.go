package perception

import (
 "bytes"
 "errors"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/communication"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
)

// ProjectCommunicationNotice is an addressed claim-evidence producer, not a
// world-truth reducer. A caller MUST load task, event and communication roots
// from independently verified durable stores and reconcile the Event with
// its CycleCommit before persisting this pure deterministic projection.
func ProjectCommunicationNotice(bundle *policy.Bundle,task,event ports.CanonicalRecord,
 plan communication.SendPlan)(RoundProjection,error){
 var zero RoundProjection
 if bundle==nil{return zero,errors.New("communication projection requires pinned policy")}
 tv,err:=codec.StrictDecodePayload(task.Canonical);if err!=nil{return zero,err}
 t,ok:=tv.([]any)
 if !ok||len(t)!=10||!bytes.Equal(asBytes(t[0]),task.Identity[:])||
  t[9]!=uint64(0){return zero,errors.New("communication perception task not PENDING")}
 ev,err:=codec.StrictDecodePayload(event.Canonical);if err!=nil{return zero,err}
 e,ok:=ev.([]any)
 if !ok||len(e)!=19||e[3]!="communication.delivered"||
  !bytes.Equal(asBytes(e[0]),event.Identity[:])||
  !bytes.Equal(asBytes(e[0]),asBytes(t[3]))||
  !bytes.Equal(asBytes(e[1]),asBytes(t[1]))||
  !bytes.Equal(asBytes(e[7]),asBytes(t[2]))||
  e[16]!="communication.delivery"||e[15]!=uint64(1){
  return zero,errors.New("communication Event does not authorize addressed claim")
 }
 due,err:=communication.ValidateOccurrence(bundle,plan);if err!=nil{return zero,err}
 occurred,ok:=communicationInstant(e[5]);if !ok||occurred<due{return zero,errors.New("communicated claim predates causal delivery")}
 result,err:=communication.PlanDueDelivery(bundle,plan.Transmission,plan.Claim,
  domain.ID(plan.Occurrence.Identity),due,occurred)
 if err!=nil||result.NextStatus!=1||result.Notice==nil{
  return zero,errors.New("delivery channel did not send a claim")
 }
 cr,err:=communication.AuthenticateClaim(bundle,plan.Claim);if err!=nil{return zero,err}
 trans,err:=communication.AuthenticateTransmission(bundle,plan.Transmission);if err!=nil{return zero,err}
 actors,ok:=e[12].([]any)
 expectedActor:=[]any{trans.Recipient[0],trans.Recipient[1]}
 if !ok||len(actors)!=1||!canonicalEqual(actors[0],expectedActor){
  return zero,errors.New("claimed observer was not intended recipient")
 }
 sourceDigest,err:=communicationOccurrenceDigest(plan.Occurrence)
 if err!=nil{return zero,err}
 cctx:=communication.EventContext{Run:plan.Run,
  Cycle:domain.ID(readID(e[7])),BaseRevision:domain.WorldRevision(readUint(e[8])),
  Sequence:domain.LogicalSequence(readUint(e[6])),At:occurred}
 expectedEvent,err:=communication.NewDueEvent(bundle,cctx,plan,result,
  communication.Origin{Kind:6,ID:domain.ID(plan.Occurrence.Identity),Digest:sourceDigest})
 if err!=nil||expectedEvent.Identity!=event.Identity||!bytes.Equal(expectedEvent.Canonical,event.Canonical){
  return zero,errors.New("observer Event differs from canonical due delivery")
 }
 policy,ok:=t[7].([]any)
 if !ok||len(policy)!=3||policy[0]!="validators-v1"||policy[1]!=uint64(1)||
  len(asBytes(policy[2]))!=32{return zero,errors.New("communication resolver lacks Genesis pin")}
 eventDigest,err:=roundSum("cote.csf.digest.event","cote.csf.schema.event.body",e[:18])
 if err!=nil||!bytes.Equal(eventDigest[:],asBytes(e[18])){return zero,errors.New("communication Event digest invalid")}
 observer:=expectedActor
 eventEvidence:=[]any{[]any{uint64(0),event.Identity[:],eventDigest[:]}}
 origin:=[]any{event.Identity[:],eventDigest[:]}
 claimRefs:=[]any{[]any{uint64(18),cr.ID[:],cr.Digest[:]}}
 // Typed Event content has only C's presented identity, B's recipient and
 // the unverified proposition. The hidden actual sender is not copied here.
 percept:=e[9]
 id,err:=roundSum("cote.csf.id.observation","cote.csf.schema.observation-id-preimage",
  []any{task.Identity[:],observer,uint64(0),uint64(0)})
 if err!=nil{return zero,err}
 body:=[]any{id[:],task.Identity[:],observer,uint64(0),uint64(0),
  e[5],[]any{origin},uint64(5),[]any{uint64(0)},
  []any{percept},claimRefs,[]any{"actual-sender-redacted","claim-unverified"},
  eventEvidence,policy}
 hash,err:=roundSum("cote.csf.digest.observation","cote.csf.schema.observation.body",body)
 if err!=nil{return zero,err}
 obs,err:=codec.CanonicalPayload(append(body,hash[:]));if err!=nil{return zero,err}
 inputID,err:=roundSum("cote.csf.id.knowledge-input","cote.csf.schema.knowledge-input-id-preimage",
  []any{id[:],observer,uint64(1),uint64(0),uint64(0)})
 if err!=nil{return zero,err}
 inputBody:=[]any{inputID[:],uint64(0),uint64(0),observer,
  e[5],uint64(1),trans.Channel,[]any{uint64(16),id[:],hash[:]},claimRefs,eventEvidence}
 inputDigest,err:=roundSum("cote.csf.digest.knowledge-input","cote.csf.schema.knowledge-input.body",inputBody)
 if err!=nil{return zero,err}
 inputBytes,err:=codec.CanonicalPayload(append(inputBody,inputDigest[:]))
 if err!=nil{return zero,err}
 completionID,err:=roundSum("cote.csf.id.perception-completion",
  "cote.csf.schema.perception-completion-id-preimage",[]any{task.Identity[:]})
 if err!=nil{return zero,err}
 completionBody:=[]any{completionID[:],task.Identity[:],[]any{id[:]},[]any{inputID[:]}}
 completionDigest,err:=roundSum("cote.csf.digest.perception-completion",
  "cote.csf.schema.perception-task-completion.body",completionBody)
 if err!=nil{return zero,err}
 completionBytes,err:=codec.CanonicalPayload(append(completionBody,completionDigest[:]))
 if err!=nil{return zero,err}
 return RoundProjection{Actor:[2]string{trans.Recipient[0],trans.Recipient[1]},
  Observation:ports.CanonicalRecord{Identity:[32]byte(id),Canonical:obs},
  Input:ports.CanonicalRecord{Identity:[32]byte(inputID),Canonical:inputBytes},
  Completion:ports.CanonicalRecord{Identity:[32]byte(completionID),Canonical:completionBytes}},nil
}
func canonicalEqual(a,b any)bool{
 x,e1:=codec.CanonicalPayload(a);y,e2:=codec.CanonicalPayload(b)
 return e1==nil&&e2==nil&&bytes.Equal(x,y)
}
func communicationInstant(v any)(domain.SimulationInstant,bool){
 switch n:=v.(type){case uint64:if n<=uint64(^uint64(0)>>1){return domain.SimulationInstant(n),true}
 case int64:return domain.SimulationInstant(n),true}
 return 0,false
}
func communicationOccurrenceDigest(record ports.CanonicalRecord)(domain.Digest,error){
 v,err:=codec.StrictDecodePayload(record.Canonical);if err!=nil{return domain.Digest{},err}
 f,ok:=v.([]any);if !ok||len(f)!=15{return domain.Digest{},errors.New("occurrence digest unavailable")}
 b,ok:=f[14].([]byte);if !ok||len(b)!=32{return domain.Digest{},errors.New("occurrence digest malformed")}
 var result domain.Digest;copy(result[:],b);return result,nil
}
func readID(v any)[32]byte{var id [32]byte;copy(id[:],asBytes(v));return id}
func readUint(v any)uint64{n,_:=v.(uint64);return n}

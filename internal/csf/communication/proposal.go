package communication

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

type AuthoredSend struct {
 Run domain.ID
 Cycle domain.ID
 Source Origin
 Actor Actor
 Round domain.ID
 Slot string
 Revision domain.WorldRevision
 Spec SendSpec
}

// AuthoredAction verifies a complete, stored ActionProposal rather than a
// worker-supplied SendSpec. Only the closed communication.send schema can
// authorize a message publication. Caller must prove admission/fence/dispatch.
func AuthoredAction(bundle *policy.Bundle,response ports.CanonicalRecord)(AuthoredSend,error){
 var zero AuthoredSend
 if bundle==nil{return zero,errors.New("communication ActionProposal requires policy bundle")}
 root,err:=codec.StrictDecodePayload(response.Canonical);if err!=nil{return zero,err}
 f,ok:=root.([]any)
 if !ok||len(f)!=19||!bytes.Equal(rawBytes(f[0]),response.Identity[:]){
  return zero,errors.New("ActionProposal envelope invalid")
 }
 if f[4]!="communication.send"||f[5]!=uint64(1){
  return zero,errors.New("unregistered communication ActionProposal type/version")
 }
 run,ok:=bytesID(f[2]);if !ok{return zero,errors.New("ActionProposal run invalid")}
 cycle,ok:=bytesID(f[3]);if !ok{return zero,errors.New("ActionProposal cycle invalid")}
 dispatch,ok:=bytesID(f[1]);if !ok{return zero,errors.New("ActionProposal dispatch invalid")}
 round,ok:=bytesID(f[13]);if !ok{return zero,errors.New("ActionProposal round invalid")}
 slot,ok:=f[14].(string);if !ok||domain.ValidateMachineID(slot)!=nil{return zero,errors.New("ActionProposal slot invalid")}
 actor,ok:=asActor(f[6]);if !ok||actor[0]!="school"{return zero,errors.New("ActionProposal author invalid")}
 timestamp,ok:=causalTime(f[11]);if !ok{return zero,errors.New("ActionProposal effective time invalid")}
 revision,ok:=f[15].(uint64);if !ok||revision==math.MaxUint64{return zero,errors.New("ActionProposal revision invalid")}
 target,ok:=f[7].([]any);if !ok||len(target)!=0{return zero,errors.New("unsupported communication target-references")}
 claims,ok:=f[9].([]any);if !ok||len(claims)!=0{return zero,errors.New("communication may not claim resources")}
 id,_,err:=identity.New(bundle).Sum("cote.csf.id.action-proposal",
  "cote.csf.schema.slot-response-id-preimage",1,[]any{dispatch[:],uint64(0)})
 if err!=nil||domain.ID(id)!=domain.ID(response.Identity){return zero,errors.New("ActionProposal identity not derived from SlotDispatch")}
 digest,_,err:=identity.New(bundle).Sum("cote.csf.digest.action-proposal-unit",
  "cote.csf.schema.action-proposal.body",1,f[:18])
 if err!=nil||!bytes.Equal(rawBytes(f[18]),digest[:]){
  return zero,errors.New("ActionProposal body digest altered")
 }
 typed,ok:=f[8].([]any);if !ok{return zero,errors.New("communication parameters malformed")}
 if err:=checkedTyped(typed);err!=nil{return zero,err}
 env,err:=codec.StrictDecodeEnvelope(rawBytes(typed[2]))
 if err!=nil||env.DomainTag!="cote.csf.communication.send-params.v1"{
  return zero,errors.New("communication parameters not registered")
 }
 args,ok:=env.Value.([]any)
 if !ok||len(args)!=8{return zero,errors.New("communication parameter arity invalid")}
 recipient,ok:=asActor(args[0]);if !ok{return zero,errors.New("communication recipient invalid")}
 presented,ok:=args[1].(string);if !ok||presented==""||len(presented)>512{return zero,errors.New("sender display invalid")}
 channel,ok:=args[2].(string);if !ok{return zero,errors.New("communication channel missing")}
 if _,err:=registeredChannelResult(channel);err!=nil{return zero,err}
 due,ok:=causalTime(args[3]);if !ok||due<=timestamp{return zero,errors.New("communication due time invalid")}
 propositionType,ok:=args[4].(string);if !ok{return zero,errors.New("proposition type invalid")}
 pol,ok:=args[5].(uint64);if !ok||pol>2{return zero,errors.New("proposition polarity invalid")}
 scope,ok:=args[6].([]any);if !ok{return zero,errors.New("proposition temporal scope absent")}
 content,ok:=args[7].([]any);if !ok{return zero,errors.New("proposition reported content absent")}
 if err:=checkedRegisteredProposition(scope,"cote.csf.communication.scope.v1");err!=nil{return zero,err}
 if err:=checkedRegisteredProposition(content,"cote.csf.communication.allegation.v1");err!=nil{return zero,err}
 spec:=SendSpec{Run:run,Source:Origin{Kind:5,ID:domain.ID(response.Identity),Digest:digest},
  ActualSender:actor,PresentedSender:presented,Recipient:recipient,Channel:channel,
  SentAt:timestamp,DueAt:due,
  Proposition:Proposition{Type:propositionType,Polarity:uint8(pol),
   TemporalScope:scope,Content:content}}
 return AuthoredSend{Run:run,Cycle:cycle,Source:spec.Source,Actor:actor,
  Round:round,Slot:slot,Revision:domain.WorldRevision(revision),Spec:spec},nil
}
func SendParameters(spec SendSpec)([]any,error){
 if _,err:=registeredChannelResult(spec.Channel);err!=nil{return nil,err}
 actor,err:=actorValue(spec.Recipient);if err!=nil{return nil,err}
 if err:=checkedRegisteredProposition(spec.Proposition.TemporalScope,"cote.csf.communication.scope.v1");err!=nil{return nil,err}
 if err:=checkedRegisteredProposition(spec.Proposition.Content,"cote.csf.communication.allegation.v1");err!=nil{return nil,err}
 return MakeTyped("cote.csf.communication.send-params.v1",
  "cote.csf.schema.typed-value",[]any{actor,spec.PresentedSender,spec.Channel,
   int64(spec.DueAt),spec.Proposition.Type,uint64(spec.Proposition.Polarity),
   spec.Proposition.TemporalScope,spec.Proposition.Content})
}

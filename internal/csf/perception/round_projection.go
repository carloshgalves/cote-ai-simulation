package perception

import (
 "bytes"
 "errors"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
)

// RoundProjection contains only observer-addressed, redacted evidence.
// World Event and producer's source-inputs are NOT present in actor output.
// The first registered route is a school's terminal round notice, addressed
// to the actor of the authenticated RoundDeclaration, never to all actors.
type RoundProjection struct {
 Actor [2]string
 Observation ports.CanonicalRecord
 Input ports.CanonicalRecord
 Completion ports.CanonicalRecord
}

func roundSum(tag,schema string,value any)([32]byte,error){
 raw,err:=codec.CanonicalBytes(tag,schema,1,value)
 if err!=nil{return [32]byte{},err}
 return [32]byte(identity.OpaqueContentHash(raw)),nil
}

// ProjectRoundNotice is pure. Its caller must load the three records from
// independent persisted roots, reconcile task against genesis-pinned Event,
// and reconcile the Event against its CycleCommit. The round is never supplied
// by the worker. The authorized actor is derived from its durable signed owner.
func ProjectRoundNotice(task,event,round ports.CanonicalRecord)(RoundProjection,error){
 var out RoundProjection
 tv,err:=codec.StrictDecodePayload(task.Canonical);if err!=nil{return out,err}
 t,ok:=tv.([]any)
 if !ok||len(t)!=10||!bytes.Equal(asBytes(t[0]),task.Identity[:])||
  t[9]!=uint64(0){return out,errors.New("invalid PENDING perception task")}
 ev,err:=codec.StrictDecodePayload(event.Canonical);if err!=nil{return out,err}
 e,ok:=ev.([]any)
 if !ok||len(e)!=19||e[3]!="round.consumed"||e[4]!=uint64(1)||
  e[15]!=uint64(2)||e[16]!="round.lifecycle"||e[17]!=uint64(1)||
  !bytes.Equal(asBytes(e[0]),event.Identity[:])||
  !bytes.Equal(asBytes(e[0]),asBytes(t[3]))||
  !bytes.Equal(asBytes(e[1]),asBytes(t[1]))||
  !bytes.Equal(asBytes(e[7]),asBytes(t[2])){
  return out,errors.New("unregistered or unrelated source Event")
 }
 key,ok:=e[2].([]any)
 if !ok||len(key)!=6||key[0]!=uint64(2)||key[2]!="round.lifecycle"||
  key[3]!=uint64(1)||key[4]!=uint64(0){
  return out,errors.New("round EventOrderKey does not identify registered lifecycle route")
 }
 if !bytes.Equal(asBytes(key[1]),round.Identity[:]) {return out,errors.New("round source identity mismatch")}
 if _,ok:=e[12].([]any);!ok{return out,errors.New("event actor list malformed")}
 if actors:=e[12].([]any);len(actors)!=0{
  return out,errors.New("round event supplied unsolicited observers")
 }
 eventID,err:=roundSum("cote.csf.id.event","cote.csf.schema.event-id-preimage",
  []any{e[1],e[7],key})
 if err!=nil||!bytes.Equal(eventID[:],event.Identity[:]){
  return out,errors.New("round Event ID invalid")
 }
 eventDigest,err:=roundSum("cote.csf.digest.event","cote.csf.schema.event.body",e[:18])
 if err!=nil||!bytes.Equal(eventDigest[:],asBytes(e[18])){
  return out,errors.New("round Event digest mismatch")
 }
 rv,err:=codec.StrictDecodePayload(round.Canonical);if err!=nil{return out,err}
 r,ok:=rv.([]any)
 if !ok||len(r)!=11||!bytes.Equal(asBytes(r[0]),round.Identity[:])||
  !bytes.Equal(asBytes(r[1]),asBytes(t[1])){return out,errors.New("round registry identity/shape malformed")}
 actor,ok:=r[3].([]any)
 if !ok||len(actor)!=2{return out,errors.New("round owner actor malformed")}
 ns,nsOK:=actor[0].(string);name,nameOK:=actor[1].(string)
 if !nsOK||!nameOK||ns==""||name==""{return out,errors.New("round owner invalid")}
 roundID,err:=roundSum("cote.csf.id.decision-round",
  "cote.csf.schema.decision-round-id-preimage",[]any{r[6],r[7],r[8]})
 if err!=nil||!bytes.Equal(roundID[:],round.Identity[:]){
  return out,errors.New("round declaration causal ID mismatch")
 }
 roundDigest,err:=roundSum("cote.csf.digest.round-declaration",
  "cote.csf.schema.round-declaration",r)
 if err!=nil{return out,err}
 // The event's lifecycle payload is authenticated. It is NEVER copied to
 // the observation, because it contains private slot-response roots.
 typed,ok:=e[9].([]any)
 if !ok||len(typed)!=4{return out,errors.New("round typed payload missing")}
 payload,ok:=typed[2].([]byte)
 if !ok{return out,errors.New("round event typed payload malformed")}
 decoded,err:=codec.StrictDecodeEnvelope(payload)
 if err!=nil{return out,err}
 body,ok:=decoded.Value.([]any)
 if !ok||len(body)!=3||!bytes.Equal(asBytes(body[0]),round.Identity[:])||
  body[1]!=uint64(2){return out,errors.New("round lifecycle payload differs from source")}
 sourceRefs,ok:=body[2].([]any)
 if !ok||len(sourceRefs)==0{return out,errors.New("round lifecycle lacks source closure")}
 if decoded.DomainTag!="cote.csf.round.consumed.v1"||
  decoded.SchemaID!="cote.csf.schema.typed-value"||
  decoded.SchemaVersion!=1||
  typed[0]!="cote.csf.schema.typed-value"||
  typed[1]!=uint64(1)||
  !sameBytesOrDigest(typed[3],identity.OpaqueContentHash(payload)){
  return out,errors.New("round Event payload digest mismatch")
 }
 // Only a fixed, minimal notice is visible: this round closed. Source refs,
 // proposal bodies, resource outcomes and global state never reach the actor.
 noticeBytes,err:=codec.CanonicalBytes("cote.csf.round.consumed.v1",
  "cote.csf.schema.typed-value",1,[]any{round.Identity[:],uint64(2),[]any{}})
 if err!=nil{return out,err}
 noticeHash:=identity.OpaqueContentHash(noticeBytes)
 percept:=[]any{"cote.csf.schema.typed-value",uint64(1),noticeBytes,noticeHash[:]}
 evidence:=[]any{[]any{uint64(0),event.Identity[:],eventDigest[:]}}
 origin:=[]any{event.Identity[:],eventDigest[:]}
 resolver,ok:=t[7].([]any)
 if !ok||len(resolver)!=3||resolver[0]!="validators-v1"||resolver[1]!=uint64(1)||
  len(asBytes(resolver[2]))!=32{
  return out,errors.New("round observer route lacks pinned resolver")
 }
 observationID,err:=roundSum("cote.csf.id.observation",
  "cote.csf.schema.observation-id-preimage",[]any{task.Identity[:],actor,uint64(0),uint64(0)})
 if err!=nil{return out,err}
 obsBody:=[]any{observationID[:],task.Identity[:],actor,uint64(0),uint64(0),
  e[5],[]any{origin},uint64(5),[]any{uint64(0)},[]any{percept},
  []any{},[]any{"source-inputs-redacted"},evidence,resolver}
 obsDigest,err:=roundSum("cote.csf.digest.observation",
  "cote.csf.schema.observation.body",obsBody)
 if err!=nil{return out,err}
 obsRaw,err:=codec.CanonicalPayload(append(obsBody,obsDigest[:]))
 if err!=nil{return out,err}
 inputID,err:=roundSum("cote.csf.id.knowledge-input",
  "cote.csf.schema.knowledge-input-id-preimage",[]any{observationID[:],actor,uint64(0),uint64(0),uint64(0)})
 if err!=nil{return out,err}
 inputBody:=[]any{inputID[:],uint64(0),uint64(0),actor,e[5],uint64(0),
  "school.round.lifecycle.v1",[]any{uint64(16),observationID[:],obsDigest[:]},
  []any{},evidence}
 inputDigest,err:=roundSum("cote.csf.digest.knowledge-input",
  "cote.csf.schema.knowledge-input.body",inputBody)
 if err!=nil{return out,err}
 inputRaw,err:=codec.CanonicalPayload(append(inputBody,inputDigest[:]))
 if err!=nil{return out,err}
 completionID,err:=roundSum("cote.csf.id.perception-completion",
  "cote.csf.schema.perception-completion-id-preimage",[]any{task.Identity[:]})
 if err!=nil{return out,err}
 completionBody:=[]any{completionID[:],task.Identity[:],[]any{observationID[:]},[]any{inputID[:]}}
 completionDigest,err:=roundSum("cote.csf.digest.perception-completion",
  "cote.csf.schema.perception-task-completion.body",completionBody)
 if err!=nil{return out,err}
 completionRaw,err:=codec.CanonicalPayload(append(completionBody,completionDigest[:]))
 if err!=nil{return out,err}
 _=roundDigest // SQLite must independently verify the immutable round_integrity_seals hash
 out.Actor=[2]string{ns,name}
 out.Observation=ports.CanonicalRecord{Identity:observationID,Canonical:obsRaw}
 out.Input=ports.CanonicalRecord{Identity:inputID,Canonical:inputRaw}
 out.Completion=ports.CanonicalRecord{Identity:completionID,Canonical:completionRaw}
 return out,nil
}
func sameBytesOrDigest(value any,hash domain.Digest)bool{
 return bytes.Equal(asBytes(value),hash[:])
}

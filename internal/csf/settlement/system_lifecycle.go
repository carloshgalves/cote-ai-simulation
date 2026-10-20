package settlement

import (
 "errors"
 "math"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
)

// SystemLifecycleEvent is private engine truth. Trigger handling is a
// registered identity-only transition; no observable agent output is implied.
func SystemLifecycleEvent(run,cycle,source domain.ID,sourceDigest domain.Digest,
 kind uint64,disposition uint64,instant domain.SimulationInstant,
 revision domain.WorldRevision,sequence domain.LogicalSequence)(ports.CanonicalRecord,error){
 var result ports.CanonicalRecord
 if (kind!=2&&kind!=3)||(kind==3&&disposition!=0)||
  (kind==2&&disposition!=1&&disposition!=2){
  return result,errors.New("unsupported system-unit lifecycle disposition")
 }
 producer,eventType:="schedule","occurrence.rejected"
 if kind==2&&disposition==2{eventType="occurrence.deferred"}
 if kind==3{producer,eventType="trigger","trigger.activation.consumed"}
 key:=[]any{uint64(2),source[:],producer,uint64(1),uint64(0),uint64(0)}
 id,err:=root("cote.csf.id.event","cote.csf.schema.event-id-preimage",[]any{run[:],cycle[:],key})
 if err!=nil{return result,err}
 env,err:=codec.CanonicalBytes("cote.csf.system.lifecycle.v1","cote.csf.schema.typed-value",1,
  []any{kind,source[:],sourceDigest[:],disposition})
 if err!=nil{return result,err}
 hash:=identity.OpaqueContentHash(env)
 typed:=[]any{"cote.csf.schema.typed-value",uint64(1),env,hash[:]}
 ref:=[]any{kind,source[:],sourceDigest[:]}
 fields:=[]any{id[:],run[:],key,eventType,uint64(1),int64(instant),
  uint64(sequence),cycle[:],uint64(revision),typed,
  []any{},[]any{ref},[]any{},[]any{},[]any{uint64(0)},uint64(2),producer,uint64(1)}
 dg,err:=root("cote.csf.digest.event","cote.csf.schema.event.body",fields)
 if err!=nil{return result,err}
 encoded,err:=codec.CanonicalPayload(append(append([]any(nil),fields...),dg[:]))
 if err!=nil{return result,err}
 return ports.CanonicalRecord{Identity:[32]byte(id),Canonical:encoded},nil
}

func signedSystem(value any)(int64,bool){
 switch v:=value.(type){
 case int64:return v,true
 case uint64:if v<=math.MaxInt64{return int64(v),true}
 }
 return 0,false
}

// OccurrenceOutcome derives the only registered synthetic scheduling handler.
// A nonrecurring test.occurrence is REJECT (1). A fixed-interval recurrence
// with remaining work is DEFER (2) with an event-origin successor.
func OccurrenceOutcome(run domain.ID,fields []any,at domain.SimulationInstant)(uint64,int64,any,error){
 if len(fields)!=15||fields[5]!="test.occurrence"||fields[6]!=uint64(1)||
  fields[8]!=uint64(0){return 0,0,nil,errors.New("unregistered or terminal occurrence handler")}
 option,ok:=fields[12].([]any);if !ok{return 0,0,nil,errors.New("recurrence option malformed")}
 if len(option)==1&&option[0]==uint64(0){return 1,0,[]any{uint64(0)},nil}
 if len(option)!=2||option[0]!=uint64(1){return 0,0,nil,errors.New("recurrence variant unsupported")}
 recurrence,ok:=option[1].([]any)
 if !ok||len(recurrence)!=3||recurrence[0]!=uint64(0){
  return 0,0,nil,errors.New("only fixed-interval recurrence is registered")
 }
 every,ok:=signedSystem(recurrence[1])
 if !ok||every<=0{return 0,0,nil,errors.New("nonpositive recurrence interval")}
 remaining,ok:=recurrence[2].([]any)
 if !ok||len(remaining)==0{return 0,0,nil,errors.New("recurrence remaining-count option malformed")}
 nextRemaining:=any([]any{uint64(0)})
 if remaining[0]==uint64(1){
  if len(remaining)!=2{return 0,0,nil,errors.New("remaining-count missing")}
  count,ok:=remaining[1].(uint64)
  if !ok||count==0||count>math.MaxUint32{return 0,0,nil,errors.New("remaining-count invalid")}
  if count<=1{return 1,0,[]any{uint64(0)},nil}
  nextRemaining=[]any{uint64(1),count-1}
 }else if remaining[0]!=uint64(0)||len(remaining)!=1{
  return 0,0,nil,errors.New("remaining-count invalid")
 }
 due,ok:=signedSystem(fields[3])
 if !ok{return 0,0,nil,errors.New("occurrence due timestamp not signed")}
 if due>math.MaxInt64-every{return 0,0,nil,errors.New("successor timestamp overflow")}
 newDue:=due+every
 if newDue<=int64(at){return 0,0,nil,errors.New("deferred successor must be in the future")}
 return 2,newDue,[]any{uint64(1),[]any{uint64(0),every,nextRemaining}},nil
}

func OccurrenceSuccessor(run domain.ID,source []any,at domain.SimulationInstant,
 event ports.CanonicalRecord,newDue int64,recurrence any)(ports.CanonicalRecord,error){
 var result ports.CanonicalRecord
 if len(source)!=15||newDue<=int64(at){return result,errors.New("invalid occurrence successor")}
 envelope,err:=codec.StrictDecodePayload(event.Canonical);if err!=nil{return result,err}
 eventFields,ok:=envelope.([]any)
 if !ok||len(eventFields)!=19{return result,errors.New("lifecycle Event is not canonical")}
 eventDigest,ok:=eventFields[18].([]byte)
 if !ok||len(eventDigest)!=32{return result,errors.New("lifecycle Event digest invalid")}
 origin:=[]any{uint64(0),[]any{event.Identity[:],eventDigest}}
 role:=source[10]
 ordinal:=uint64(0)
 id,err:=root("cote.csf.id.occurrence","cote.csf.schema.occurrence-id-preimage",
  []any{origin,role,ordinal});if err!=nil{return result,err}
 fields:=[]any{id[:],run[:],source[2],newDue,[]any{newDue,uint64(0)},
  source[5],source[6],source[7],uint64(0),origin,role,ordinal,recurrence,source[13]}
 digest,err:=root("cote.csf.digest.occurrence-unit","cote.csf.schema.scheduled-occurrence.body",fields)
 if err!=nil{return result,err}
 canonical,err:=codec.CanonicalPayload(append(append([]any(nil),fields...),digest[:]))
 if err!=nil{return result,err}
 return ports.CanonicalRecord{Identity:[32]byte(id),Canonical:canonical},nil
}

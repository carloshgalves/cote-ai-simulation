// Package settlement builds internal, deterministic lifecycle events. They
// are never delivered to an agent as an observation and do not alter the
// gameplay WorldState. Event publication is owned by the terminal commit.
package settlement

import (
 "errors"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
)

func root(tag,schema string,value any)(domain.Digest,error){
 raw,err:=codec.CanonicalBytes(tag,schema,1,value)
 if err!=nil{return domain.Digest{},err}
 return identity.OpaqueContentHash(raw),nil
}

// ConsumedRoundEvent is a SOURCE_LIFECYCLE (phase=2) event ordered by the
// canonical round ID. Its payload and source-input refs contain exactly the
// slot response units that close every declared slot, not observer-visible
// source material or arbitrarily supplied author metadata.
func ConsumedRoundEvent(run,cycle,round domain.ID,
 instant domain.SimulationInstant,revision domain.WorldRevision,
 sequence domain.LogicalSequence,sourceRefs []any)(ports.CanonicalRecord,error){
 var out ports.CanonicalRecord
 if len(sourceRefs)==0{return out,errors.New("cannot consume a round without committed slot roots")}
 for _,v:=range sourceRefs{
  r,ok:=v.([]any)
  if !ok||len(r)!=4||r[0]!=uint64(0){
   return out,errors.New("round lifecycle requires exact slot response references")
  }
  kind,valid:=r[1].(uint64)
  if !valid||kind>1{return out,errors.New("round lifecycle slot response kind invalid")}
  id,ok:=r[2].([]byte);if !ok||len(id)!=32{return out,errors.New("invalid source unit ID")}
  h,ok:=r[3].([]byte);if !ok||len(h)!=32{return out,errors.New("invalid source unit digest")}
 }
 key:=[]any{uint64(2),round[:],"round.lifecycle",uint64(1),uint64(0),uint64(0)}
 eventID,err:=root("cote.csf.id.event","cote.csf.schema.event-id-preimage",[]any{run[:],cycle[:],key})
 if err!=nil{return out,err}
 // The reducer for this event is the registered identity transition:
 // source-lifecycle ownership changes, not user-facing WorldState.
 envelope,err:=codec.CanonicalBytes("cote.csf.round.consumed.v1","cote.csf.schema.typed-value",1,
  []any{round[:],uint64(2),sourceRefs})
 if err!=nil{return out,err}
 h:=identity.OpaqueContentHash(envelope)
 typed:=[]any{"cote.csf.schema.typed-value",uint64(1),envelope,h[:]}
 fields:=[]any{eventID[:],run[:],key,"round.consumed",uint64(1),int64(instant),
  uint64(sequence),cycle[:],uint64(revision),typed,
  []any{},sourceRefs,[]any{},[]any{},[]any{uint64(0)},uint64(2),"round.lifecycle",uint64(1)}
 dg,err:=root("cote.csf.digest.event","cote.csf.schema.event.body",fields)
 if err!=nil{return out,err}
 canonical,err:=codec.CanonicalPayload(append(append([]any(nil),fields...),dg[:]))
 if err!=nil{return out,err}
 return ports.CanonicalRecord{Identity:[32]byte(eventID),Canonical:canonical},nil
}

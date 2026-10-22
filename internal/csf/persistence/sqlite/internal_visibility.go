package sqlite

import "errors"

// registeredInvisibleEngineEvent encodes the *closed* set of already supported
// CSFRV1-5 secret engine lifecycle producers. A confidentiality bit alone is
// never authorization to make a task empty: type, originating subsystem,
// EventOrderKey phase/ordinal and actor-free contract must all agree.
// Any new event producer needs a named resolver, not a default fallback.
func registeredInvisibleEngineEvent(event []any) error {
 if len(event)!=19||event[4]!=uint64(1)||event[15]!=uint64(2)||
  event[17]!=uint64(1){
  return errors.New("engine-only Event version/privacy/producer contract invalid")
 }
 actors,ok:=event[12].([]any)
 if !ok||len(actors)!=0{return errors.New("engine-only Event has addressable actor")}
 key,ok:=event[2].([]any)
 if !ok||len(key)!=6||key[3]!=uint64(1)||key[4]!=uint64(0)||key[5]!=uint64(0){
  return errors.New("engine-only EventOrderKey is not a registered task phase")
 }
 phase,ok:=key[0].(uint64)
 if !ok{return errors.New("internal event causal phase missing")}
 originProducer,ok:=key[2].(string)
 if !ok{return errors.New("internal event producer missing")}
 declaredProducer,ok:=event[16].(string)
 if !ok||declaredProducer!=originProducer{
  return errors.New("internal Event producer disagrees with causal key")
 }
 kind,ok:=event[3].(string)
 if !ok{return errors.New("internal Event type malformed")}
 switch kind{
 case "communication.sent":
  if phase==1&&originProducer=="communication.send"{return nil}
 case "communication.delivery_failed":
  if phase==1&&originProducer=="communication.delivery"{return nil}
 case "resource.consumed":
  if phase==1&&originProducer=="resource.consume"{return nil}
 case "clock.advanced":
  if phase==0&&originProducer=="clock"{return nil}
 case "occurrence.rejected","occurrence.deferred":
  if phase==2&&originProducer=="schedule"{return nil}
 case "trigger.activation.consumed":
  if phase==2&&originProducer=="trigger"{return nil}
 case "trigger.runtime.transition":
  if phase==3&&originProducer=="trigger"{return nil}
 case "trigger.activation.created":
  if phase==4&&originProducer=="trigger"{return nil}
 }
 return errors.New("internal event has no registered zero-observer resolver")
}

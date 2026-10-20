package trigger

import (
 "bytes"
 "encoding/hex"
 "errors"
 "fmt"
 "sort"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

// Runtime-facing definitions are not supplied to each cycle. The initial
// registry is sealed to Genesis during bootstrap; replay loads its bytes.
type Registered struct {
 Definition Definition
 Runtime Runtime
 RuntimeCanonical []byte
 DefinitionCanonical []byte
 DefinitionDigest domain.Digest
}

const PredicateSchemaID = "cote.csf.schema.typed-value"
const PredicateTag = "cote.csf.trigger.clock-threshold.v1"

// DefinitionBytes returns the CDDL trigger-definition tuple, not an ad-hoc
// parallel record format. HandlerRef is a pinned genesis policy reference.
func DefinitionBytes(d Definition, handler []any) ([]byte,error) {
 if err:=Validate(d);err!=nil{return nil,err}
 if len(handler)!=3{return nil,errors.New("handler policy reference required")}
 digest,ok:=handler[2].([]byte)
 if !ok||len(digest)!=32{return nil,errors.New("handler hash malformed")}
 payload,err:=codec.CanonicalBytes(PredicateTag,PredicateSchemaID,1,[]any{int64(d.PredicateThreshold)})
 if err!=nil{return nil,err}
 h:=identity.OpaqueContentHash(payload)
 repeat:=[]any{uint64(0)}
 if d.Policy==RepeatWhileTrue{repeat=[]any{uint64(1),d.RepeatEvery}}
 deps:=make([]any,len(d.Dependencies))
 for i,x:=range d.Dependencies{deps[i]=x}
 return codec.CanonicalPayload([]any{d.ID[:],uint64(d.Version),
  []any{PredicateSchemaID,uint64(1),payload,h[:]},uint64(d.Policy),
  d.FireOnInitialTrue,repeat,deps,handler})
}

func DecodeDefinition(raw []byte,allowedHandlers [][]any,bundle *policy.Bundle)(Definition,[]any,error){
 var d Definition
 v,err:=codec.StrictDecodePayload(raw);if err!=nil{return d,nil,err}
 fields,ok:=v.([]any)
 if !ok||len(fields)!=8{return d,nil,errors.New("invalid trigger-definition schema")}
 b,ok:=fields[0].([]byte);if !ok||len(b)!=32{return d,nil,errors.New("invalid definition id")}
 copy(d.ID[:],b)
 version,ok:=fields[1].(uint64);if !ok||version==0||version>uint64(^uint32(0)){return d,nil,errors.New("invalid definition version")}
 d.Version=uint32(version)
 typed,ok:=fields[2].([]any)
 if !ok||len(typed)!=4||typed[0]!=PredicateSchemaID||typed[1]!=uint64(1){return d,nil,errors.New("invalid predicate typed-value")}
 bytesValue,ok:=typed[2].([]byte);if !ok{return d,nil,errors.New("predicate payload missing")}
 digest:=identity.OpaqueContentHash(bytesValue)
 provided,ok:=typed[3].([]byte);if !ok||!bytes.Equal(digest[:],provided){return d,nil,errors.New("predicate bytes tampered")}
 env,err:=codec.StrictDecodeEnvelope(bytesValue)
 if err!=nil||env.DomainTag!=PredicateTag||env.SchemaID!=PredicateSchemaID||env.SchemaVersion!=1{return d,nil,errors.New("predicate version/tag mismatch")}
 args,ok:=env.Value.([]any);if !ok||len(args)!=1{return d,nil,errors.New("invalid clock predicate args")}
 threshold,ok:=asSigned(args[0]);if !ok{return d,nil,errors.New("invalid clock threshold")}
 d.PredicateThreshold=domain.SimulationInstant(threshold)
 kind,ok:=fields[3].(uint64);if !ok||kind>uint64(RepeatWhileTrue){return d,nil,errors.New("invalid activation policy")}
 d.Policy=Policy(kind)
 d.FireOnInitialTrue,ok=fields[4].(bool);if !ok{return d,nil,errors.New("invalid initial fire")}
 repeat,ok:=fields[5].([]any);if !ok{return d,nil,errors.New("invalid repeat option")}
 if len(repeat)==2&&repeat[0]==uint64(1){every,ok:=asSigned(repeat[1]);if !ok{return d,nil,errors.New("repeat cadence invalid")};d.RepeatEvery=every
 }else if len(repeat)!=1||repeat[0]!=uint64(0){return d,nil,errors.New("repeat option invalid")}
 selectors,ok:=fields[6].([]any);if !ok{return d,nil,errors.New("invalid selectors")}
 d.Dependencies=make([]string,len(selectors))
 for i,selector:=range selectors{x,ok:=selector.(string);if !ok{return d,nil,errors.New("invalid selector")};d.Dependencies[i]=x}
 ref,ok:=fields[7].([]any);if !ok||len(ref)!=3{return d,nil,errors.New("handler ref invalid")}
 permitted:=false
 for _,grant:=range allowedHandlers{if samePolicy(grant,ref){permitted=true;break}}
 if !permitted{return d,nil,errors.New("handler is not admitted by genesis policy")}
 if err:=Validate(d);err!=nil{return d,nil,err}
 rebuilt,err:=DefinitionBytes(d,ref);if err!=nil{return d,nil,err}
 if !bytes.Equal(rebuilt,raw){return d,nil,errors.New("non-canonical definition projection")}
 _=bundle // Already verified by genesis policy binding.
 return d,ref,nil
}

func samePolicy(a,b []any)bool{
 if len(a)!=3||len(b)!=3||a[0]!=b[0]||a[1]!=b[1]{return false}
 x,ok:=a[2].([]byte);if !ok{return false}
 y,ok:=b[2].([]byte);return ok&&bytes.Equal(x,y)
}
func asSigned(v any)(int64,bool){
 switch n:=v.(type) {
 case int64:return n,true
 case uint64:if n<=uint64(^uint64(0)>>1){return int64(n),true}
 }
 return 0,false
}
func CanonicalDefinitions(in []Definition)([]Definition,error){
 out:=append([]Definition(nil),in...)
 sort.Slice(out,func(i,j int)bool{return bytes.Compare(out[i].ID[:],out[j].ID[:])<0})
 for i,d:=range out{
  if err:=Validate(d);err!=nil{return nil,fmt.Errorf("trigger %d: %w",i,err)}
  if i>0&&out[i-1].ID==d.ID{return nil,errors.New("duplicate trigger ID")}
 }
 return out,nil
}

// The registry does not accept unversioned selectors or arbitrary world paths.
// Clock and synthetic body advancements are the only CSFRV1-3 footprints.
func EventFootprint(kind string)([]string,error){
 switch kind{
 case "clock.advanced":return []string{"clock"},nil
 case "body.advanced":return []string{"body"},nil
 default:return nil,fmt.Errorf("unregistered synthetic event footprint %q",kind)
 }
}

// CycleActivation derives every authenticated field of an activation from
// the pinned trigger definition and causal cycle, without host authority.
func CycleActivation(bundle *policy.Bundle,runID domain.ID,def Definition,r Runtime,
 cycleID domain.ID,revision domain.WorldRevision,instant domain.SimulationInstant,
 causes []domain.ID)(domain.ID,[]byte,error){
 var zero domain.ID
 d:=identity.New(bundle)
 id,_,err:=d.Sum("cote.csf.id.trigger-activation","cote.csf.schema.trigger-activation-id-preimage",1,
  []any{runID[:],def.ID[:],uint64(def.Version),uint64(revision),r.ActivationCount})
 if err!=nil{return zero,nil,err}
 predicate:=PredicateValue(def,instant)
 pbytes,err:=codec.CanonicalBytes("cote.csf.trigger.predicate","cote.csf.schema.typed-value",1,predicate)
 if err!=nil{return zero,nil,err}
 digest:=identity.OpaqueContentHash(pbytes)
 pd,_,err:=d.Sum("cote.csf.digest.trigger-predicate-result","cote.csf.schema.typed-value",1,
  []any{"cote.csf.schema.typed-value",uint64(1),pbytes,digest[:]})
 if err!=nil{return zero,nil,err}
 causalIDs:=make([]any,len(causes));for i,c:=range causes{causalIDs[i]=c[:]}
 origin:=[]any{uint64(0),cycleID[:],uint64(revision),causalIDs}
 fields:=[]any{id[:],runID[:],"trigger:"+hex.EncodeToString(def.ID[:]),def.ID[:],uint64(def.Version),origin,
  []any{int64(instant),uint64(0)},r.ActivationCount,pd[:],uint64(0),[]any{uint64(0)}}
 fullDigest,_,err:=d.Sum("cote.csf.digest.trigger-activation-unit","cote.csf.schema.trigger-activation.body",1,fields)
 if err!=nil{return zero,nil,err}
 raw,err:=codec.CanonicalPayload(append(fields,fullDigest[:]));if err!=nil{return zero,nil,err}
 return domain.ID(id),raw,nil
}

// LifecycleEvent makes the temporal trigger transition observable to causal
// replay without invoking an action handler. Only the two closed V1 phases
// are admitted; these are internal, non-perceptible lifecycle events.
func LifecycleEvent(bundle *policy.Bundle,runID,cycleID domain.ID,
 from domain.SimulationInstant,base domain.WorldRevision,
 sequence domain.LogicalSequence,phase uint64,origin domain.ID,
 payload []byte)(domain.ID,[]byte,error){
 var zero domain.ID
 var kind,tag string
 switch phase{
 case 3:kind="trigger.runtime.transition";tag="cote.csf.trigger.runtime-transition.v1"
 case 4:kind="trigger.activation.created";tag="cote.csf.trigger.activation-created.v1"
 default:return zero,nil,errors.New("unknown trigger lifecycle phase")
 }
 deriver:=identity.New(bundle)
 key:=[]any{phase,origin[:],"trigger",uint64(1),uint64(0),uint64(0)}
 id,_,err:=deriver.Sum("cote.csf.id.event","cote.csf.schema.event-id-preimage",1,[]any{runID[:],cycleID[:],key})
 if err!=nil{return zero,nil,err}
 env,err:=codec.CanonicalBytes(tag,"cote.csf.schema.typed-value",1,[]any{origin[:],payload})
 if err!=nil{return zero,nil,err}
 h:=identity.OpaqueContentHash(env)
 typed:=[]any{"cote.csf.schema.typed-value",uint64(1),env,h[:]}
 body:=[]any{id[:],runID[:],key,kind,uint64(1),int64(from),uint64(sequence),cycleID[:],uint64(base),typed,
  []any{},[]any{},[]any{},[]any{},[]any{uint64(0)},uint64(2),"trigger",uint64(1)}
 digest,_,err:=deriver.Sum("cote.csf.digest.event","cote.csf.schema.event.body",1,body)
 if err!=nil{return zero,nil,err}
 raw,err:=codec.CanonicalPayload(append(body,digest[:]));if err!=nil{return zero,nil,err}
 return domain.ID(id),raw,nil
}

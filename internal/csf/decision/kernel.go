// Package decision derives non-authoritative CSFRV1-4 decision evidence.
// Nothing in this package accepts a mutable store, writes WorldState or emits Events.
package decision

import (
 "bytes"
 "errors"
 "fmt"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/inputledger"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

var (
 ErrProvenance=errors.New("PROVENANCE_FAILURE")
 ErrIdempotency=errors.New("IDEMPOTENCY_CONFLICT")
 ErrIndeterminate=errors.New("INDETERMINATE_FACET")
)

type Unit struct {
 Response inputledger.Response
 ProducerScope string
}
type Snapshot struct {
 BaseRevision domain.WorldRevision
 ResourceKind string
 ResourceID string
 Available int64
 Known bool
 ForbiddenActors map[string]bool
}
type HistoryBinding struct {
 Namespace string
 LogicalDigest domain.Digest
 CanonicalUnit domain.ID
}
type Input struct {
 RunID domain.ID
 CycleID domain.ID
 Attempt uint32
 Units []Unit
 Admitted []domain.ID // the exact fence order; NOT completion order
 Snapshot Snapshot
 History []HistoryBinding
 Seed domain.Digest
 Bundle *policy.Bundle
}
type Assessment struct{ Subject domain.ID; Fields []any; Digest domain.Digest; Canonical []byte }
type Candidate struct{
 ID domain.ID
 SourceUnitIDs []domain.ID
 Fields []any
 Digest domain.Digest
 Canonical []byte
}
type Conflict struct{ ID domain.ID; CandidateIDs []domain.ID; Fields []any; Digest domain.Digest; Canonical []byte }
type Draw struct{ ID domain.ID; Fields []any; Digest domain.Digest; Canonical []byte }
type Indeterminate struct{ Subject domain.ID; Facet uint8; Validator []any; Reason string; Evidence []any }
type Disposition struct{ Subject domain.ID; CandidateID domain.ID; Status string; Reason string; CanonicalUnit domain.ID }
type Result struct {
 Assessments []Assessment
 Candidates []Candidate
 Conflicts []Conflict
 Draws []Draw
 Indeterminate []Indeterminate
 Dispositions []Disposition
 PostAvailable int64
 Failure string
 // Canonical is provisional audit material only; never an Event, CycleCommit,
 // DecisionRecord, SourceClosure or WorldRevision.
 Canonical []byte
}

func policyRefs(b *policy.Bundle)(map[string][]any,error){
 if b==nil{return nil,errors.New("pinned bundle required")}
 roles,err:=b.RequiredPolicyRoles();if err!=nil{return nil,err}
 refs,err:=b.GenesisPolicyRefs();if err!=nil{return nil,err}
 if len(roles)!=len(refs){return nil,errors.New("genesis policy registry inconsistent")}
 result:=map[string][]any{}
 for i,k:=range roles{result[k]=refs[i]}
 for _,k:=range []string{"validators","conflict-resolvers","rng","idempotency"}{
  if result[k]==nil{return nil,fmt.Errorf("missing pinned %s policy",k)}
 }
 return result,nil
}
func refsEqual(a,b any)bool{
 x,e:=codec.CanonicalPayload(a);if e!=nil{return false}
 y,e:=codec.CanonicalPayload(b);return e==nil&&bytes.Equal(x,y)
}
func derive(d identity.Deriver,tag,schema string,v any)(domain.ID,error){
 h,_,e:=d.Sum(tag,schema,1,v);return domain.ID(h),e
}
func digest(d identity.Deriver,tag,schema string,v any)(domain.Digest,error){
 h,_,e:=d.Sum(tag,schema,1,v);return h,e
}
func encode(fields []any,hash domain.Digest)([]byte,error){
 full:=append(append([]any(nil),fields...),hash[:])
 return codec.CanonicalPayload(full)
}
func bytesIDs(ids []domain.ID)[]any{
 out:=make([]any,len(ids));for i,v:=range ids{out[i]=v[:]}
 return out
}
func canonicalIDLess(a,b domain.ID)bool{return bytes.Compare(a[:],b[:])<0}

type prepared struct{
 unit Unit
 id domain.ID
 logical domain.Digest
 ns string
 alias domain.ID
 aliasFound bool
 resource string
 amount int64
 blocked bool
 indeterminate bool
 noProposal bool
}

func validateResponse(d identity.Deriver,req Input,u Unit)(prepared,error){
 p:=prepared{unit:u,id:u.Response.ID,noProposal:u.Response.Kind==1}
 x:=u.Response
 if x.Kind>1||x.RunID!=req.RunID||x.CycleID!=req.CycleID||x.SubmittedAgainstRevision!=req.Snapshot.BaseRevision {
  return p,fmt.Errorf("%w: response identity or base revision differs from frozen cycle",ErrProvenance)
 }
 if err:=domain.ValidateMachineID(u.ProducerScope);err!=nil{return p,err}
 tag,sch:="cote.csf.id.action-proposal","cote.csf.schema.action-proposal.body"
 dtag:="cote.csf.digest.action-proposal-unit"
 arity:=18
 if p.noProposal{tag="cote.csf.id.no-proposal";sch="cote.csf.schema.no-proposal.body";dtag="cote.csf.digest.no-proposal-unit";arity=10}
 if len(x.Body)!=arity{return p,fmt.Errorf("%w: response body arity",ErrProvenance)}
 wanted,e:=derive(d,tag,"cote.csf.schema.slot-response-id-preimage",[]any{x.DispatchID[:],uint64(x.Kind)})
 if e!=nil||wanted!=x.ID{return p,fmt.Errorf("%w: response preimage",ErrProvenance)}
 dg,e:=digest(d,dtag,sch,[]any{x.Body})
 if e!=nil||dg!=x.Digest{return p,fmt.Errorf("%w: response digest",ErrProvenance)}
 raw,e:=encode(x.Body,dg);if e!=nil||!bytes.Equal(raw,x.Canonical){return p,fmt.Errorf("%w: response canonical bytes",ErrProvenance)}
 if !sameBytes(x.Body[0],x.ID[:])||!sameBytes(x.Body[1],x.DispatchID[:])||!sameBytes(x.Body[2],req.RunID[:])||!sameBytes(x.Body[3],req.CycleID[:]){
  return p,fmt.Errorf("%w: response body references",ErrProvenance)
 }
 if p.noProposal {
  if !sameBytes(x.Body[4],x.RoundID[:])||x.Body[5]!=x.SlotID||!sameActor(x.Body[6],x.Actor)||x.Body[8]!=uint64(x.SubmittedAgainstRevision)||!sameInstant(x.Body[7],x.EffectiveAt){
   return p,fmt.Errorf("%w: no-proposal fields",ErrProvenance)
  }
  return p,nil
 }
 if !sameActor(x.Body[6],x.Actor)||!sameBytes(x.Body[13],x.RoundID[:])||x.Body[14]!=x.SlotID||x.Body[15]!=uint64(x.SubmittedAgainstRevision){
  return p,fmt.Errorf("%w: action fields",ErrProvenance)
 }
 if !sameInstant(x.Body[11],x.EffectiveAt) {
  return p,fmt.Errorf("%w: mismatched effective instant",ErrProvenance)
 }
 if targets,ok:=x.Body[7].([]any);!ok||len(targets)!=0{
  return p,fmt.Errorf("%w: test resource action forbids target injection",ErrProvenance)
 }
 // The synthetic extension cannot accept an opaque or mismatched typed-value
 // payload that the handler has never validated.
 params,ok:=x.Body[8].([]any)
 if !ok||len(params)!=4||params[0]!="cote.csf.schema.typed-value"||params[1]!=uint64(1){
  return p,fmt.Errorf("%w: invalid typed parameters",ErrProvenance)
 }
 encoded,ok:=params[2].([]byte)
 if !ok||!validTypedHash(encoded,params[3]){
  return p,fmt.Errorf("%w: typed parameter digest",ErrProvenance)
 }
 env,err:=codec.StrictDecodeEnvelope(encoded)
 if err!=nil||env.SchemaID!="cote.csf.schema.typed-value"||env.SchemaVersion!=1{
  return p,fmt.Errorf("%w: opaque typed parameters",ErrProvenance)
 }
 value,ok:=env.Value.([]any);if !ok||len(value)!=0{
  return p,fmt.Errorf("%w: unregistered resource parameters",ErrProvenance)
 }
 if !refsEqual(x.Body[10],[]any{uint64(0)})||!refsEqual(x.Body[12],[]any{uint64(0)}){
  return p,fmt.Errorf("%w: resource action location/duration not supported",ErrProvenance)
 }
 if x.Body[4]!="resource.consume"||x.Body[5]!=uint64(1){p.indeterminate=true;return p,nil}
 claims,ok:=x.Body[9].([]any)
 if !ok||len(claims)!=1 {return p,fmt.Errorf("%w: expected one resource claim",ErrProvenance)}
 claim,ok:=claims[0].([]any)
 if !ok||len(claim)!=4||claim[0]!=req.Snapshot.ResourceKind||claim[1]!=req.Snapshot.ResourceID||claim[2]!=uint64(1) {
  return p,fmt.Errorf("%w: resource claim shape/scope",ErrProvenance)
 }
 n,ok:=claim[3].(int64)
 if !ok {v,isU:=claim[3].(uint64);if isU&&v<=uint64(^uint64(0)>>1){n=int64(v);ok=true}}
 if !ok||n<=0{return p,fmt.Errorf("%w: nonpositive resource claim",ErrProvenance)}
 p.amount=n;p.resource=req.Snapshot.ResourceID
 return p,nil
}
func sameBytes(value any,want []byte)bool{a,ok:=value.([]byte);return ok&&bytes.Equal(a,want)}
func sameActor(value any,actor [2]string)bool{
 a,ok:=value.([]any);return ok&&len(a)==2&&a[0]==actor[0]&&a[1]==actor[1]
}
func sameInstant(v any,expected domain.SimulationInstant)bool{
 switch n:=v.(type){
 case int64: return n==int64(expected)
 case uint64: return expected>=0&&n==uint64(expected)
 }
 return false
}

func namespace(p prepared,key string)string{
 x:=p.unit.Response
 return fmt.Sprintf("%x/%d/%s/%s/%s/%s",x.RunID,uint64(0),p.unit.ProducerScope,x.Actor[0],x.Actor[1],key)
}
func resolveAliases(d identity.Deriver,units []prepared,history []HistoryBinding,ref []any)([]prepared,error){
 binding:=map[string]HistoryBinding{}
 for _,h:=range history{
  if h.Namespace==""||h.CanonicalUnit==(domain.ID{}) {return nil,fmt.Errorf("%w: malformed historical alias",ErrProvenance)}
  if old,ok:=binding[h.Namespace];ok&&(old.CanonicalUnit!=h.CanonicalUnit||old.LogicalDigest!=h.LogicalDigest){
   return nil,ErrIdempotency
  }
  binding[h.Namespace]=h
 }
 for i:=range units{
  p:=&units[i]
  if p.noProposal||p.indeterminate{continue}
  body:=p.unit.Response.Body
  opt,ok:=body[17].([]any);if !ok||len(opt)==0 {return nil,fmt.Errorf("%w: invalid optional idempotency key",ErrProvenance)}
  if opt[0]==uint64(0){if len(opt)!=1{return nil,ErrProvenance};continue}
  if opt[0]!=uint64(1)||len(opt)!=2{return nil,ErrProvenance}
  key,ok:=opt[1].(string);if !ok||domain.ValidateMachineID(key)!=nil{return nil,ErrProvenance}
  p.ns=namespace(*p,key)
  // Logical operation excludes proposal/dispatch, delivery, admission
  // coordinate, and transport-specific metadata.
  logical:=[]any{body[4],body[5],body[6],body[7],body[8],body[9],body[10],body[11],body[12],body[16]}
  preimage:=[]any{p.unit.Response.RunID[:],uint64(0),p.unit.ProducerScope,[]any{uint64(1),[]any{p.unit.Response.Actor[0],p.unit.Response.Actor[1]}},key,logical,ref}
  h,err:=digest(d,"cote.csf.digest.idempotency","cote.csf.schema.idempotency-preimage",preimage)
  if err!=nil{return nil,err}
  p.logical=h
  if prev,exists:=binding[p.ns];exists{
   if prev.LogicalDigest!=h{return nil,ErrIdempotency}
   p.alias=prev.CanonicalUnit;p.aliasFound=true
  }else{
   binding[p.ns]=HistoryBinding{Namespace:p.ns,LogicalDigest:h,CanonicalUnit:p.id}
  }
 }
 return units,nil
}

func MakeCandidate(d identity.Deriver,run,cycle domain.ID,sourceIDs []domain.ID,producer string,version uint32,role uint16,ordinal uint32,claims []any,provenance []any,priority int64,resolver []any)(Candidate,error){
 var out Candidate
 if len(sourceIDs)==0||len(provenance)==0{return out,ErrProvenance}
 if err:=domain.ValidateMachineID(producer);err!=nil{return out,err}
 for i,id:=range sourceIDs{if id==(domain.ID{})||(i>0&&sourceIDs[i-1]==id){return out,ErrProvenance}}
 id,e:=derive(d,"cote.csf.id.commit-candidate","cote.csf.schema.candidate-id-preimage",
  []any{run[:],cycle[:],bytesIDs(sourceIDs),producer,uint64(version),uint64(role),uint64(ordinal)})
 if e!=nil{return out,e}
 fields:=[]any{id[:],run[:],cycle[:],bytesIDs(sourceIDs),producer,uint64(version),uint64(role),uint64(ordinal),
  []any{},claims,[]any{},[]any{},[]any{"resource.nonnegative"},provenance,priority,resolver}
 hash,e:=digest(d,"cote.csf.digest.commit-candidate","cote.csf.schema.commit-candidate.body",fields)
 if e!=nil{return out,e}
 canonical,e:=encode(fields,hash)
 return Candidate{ID:id,SourceUnitIDs:append([]domain.ID(nil),sourceIDs...),Fields:fields,Digest:hash,Canonical:canonical},e
}

func CheckPartition(admitted []domain.ID,noProposal,aliases map[domain.ID]bool,candidates []Candidate)error{
 expected:=map[domain.ID]bool{}
 for _,id:=range admitted{
  if expected[id] {return fmt.Errorf("%w: duplicate admitted unit",ErrProvenance)}
  expected[id]=true
 }
 for id:=range noProposal{if !expected[id]{return fmt.Errorf("%w: foreign NoProposal",ErrProvenance)}}
 for id:=range aliases{if !expected[id]||noProposal[id]{return fmt.Errorf("%w: foreign/overlap alias",ErrProvenance)}}
 seen:=map[domain.ID]bool{}
 for _,c:=range candidates{
  if len(c.SourceUnitIDs)==0{return ErrProvenance}
  for _,id:=range c.SourceUnitIDs{
   if !expected[id]||noProposal[id]||aliases[id]||seen[id]{return fmt.Errorf("%w: overlap or external candidate unit",ErrProvenance)}
   seen[id]=true
  }
 }
 for id:=range expected{if !seen[id]&&!noProposal[id]&&!aliases[id]{return fmt.Errorf("%w: missing candidate producer",ErrProvenance)}}
 return nil
}

func assess(d identity.Deriver,p prepared,known,blocked,forbidden bool,validator []any)(Assessment,error){
 status:=uint64(0);reason:="resource.available"
 if !known{status=3;reason="resource.missing"}
 if blocked{status=1;reason="resource.insufficient"}
 facets:=[]any{
  []any{uint64(0),uint64(0),uint64(2),"physical.possible",[]any{},validator},
  []any{uint64(1),uint64(0),uint64(2),"access.granted",[]any{},validator},
  []any{uint64(2),status,uint64(0),reason,[]any{},validator},
  []any{uint64(3),uint64(2),uint64(2),"temporal.not-applicable",[]any{},validator},
 }
 if forbidden{
  facets=append(facets,[]any{uint64(4),uint64(1),uint64(1),"institution.prohibited",[]any{},validator})
 }else{
  facets=append(facets,[]any{uint64(4),uint64(0),uint64(2),"institution.allowed",[]any{},validator})
 }
 facets=append(facets,[]any{uint64(5),uint64(2),uint64(2),"legal.not-applicable",[]any{},validator})
 subj:=[]any{uint64(0),p.id[:],p.unit.Response.Digest[:]}
 fields:=[]any{subj,facets}
 dg,e:=digest(d,"cote.csf.digest.affordance-assessment","cote.csf.schema.affordance-assessment.body",fields)
 if e!=nil{return Assessment{},e}
 raw,e:=encode(fields,dg)
 return Assessment{Subject:p.id,Fields:fields,Digest:dg,Canonical:raw},e
}

func namedDraw(d identity.Deriver,input Input,ids []domain.ID,ref []any)(Draw,uint64,error){
 var out Draw
 if len(ids)<2{return out,0,errors.New("named conflict draw requires competitors")}
 subsystem,decisionKey,purpose:="resource","last-unit","resolve"
 id,e:=derive(d,"cote.csf.id.rng-draw","cote.csf.schema.rng-draw-id-preimage",
  []any{input.RunID[:],input.CycleID[:],uint64(input.Attempt),subsystem,decisionKey,bytesIDs(ids),purpose,ref})
 if e!=nil{return out,0,e}
 bits,e:=identity.NamedSubstreamV1(input.Seed,subsystem,decisionKey,bytesIDs(ids),purpose,ref)
 if e!=nil{return out,0,e}
 // Two contenders: one unbiased bit; never derive a stream from worker order.
 selected:=uint64(bits[0]&1)
 envelope,e:=codec.CanonicalBytes("cote.csf.test.resource-rng-result","cote.csf.schema.typed-value",1,selected)
 if e!=nil{return out,0,e}
 h:=identity.OpaqueContentHash(envelope)
 result:=[]any{"cote.csf.schema.typed-value",uint64(1),envelope,h[:]}
 fields:=[]any{id[:],input.RunID[:],input.CycleID[:],uint64(input.Attempt),subsystem,decisionKey,bytesIDs(ids),purpose,ref,result}
 dg,e:=digest(d,"cote.csf.digest.rng-draw","cote.csf.schema.rng-draw.body",fields)
 if e!=nil{return out,0,e}
 raw,e:=encode(fields,dg)
 return Draw{ID:id,Fields:fields,Digest:dg,Canonical:raw},selected,e
}

func validTypedHash(raw []byte,witness any)bool {
 h:=identity.OpaqueContentHash(raw)
 return sameBytes(witness,h[:])
}

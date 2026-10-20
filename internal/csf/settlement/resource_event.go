package settlement

import (
 "bytes"
 "errors"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
)

// ResourceConsumedEvent materializes ONLY a previously signed winning
// CommitCandidate's exact event draft. It is SECRET engine truth, not
// an observation; no PerceptionTask may be inferred without an observer
// resolver in a later causal slice.
func ResourceConsumedEvent(run,cycle,candidate,source domain.ID,
 sourceDigest domain.Digest,from domain.SimulationInstant,revision domain.WorldRevision,
 sequence domain.LogicalSequence,draft []any)(ports.CanonicalRecord,error){
 var out ports.CanonicalRecord
 if len(draft)!=5||draft[0]!=uint64(0)||draft[1]!=uint64(0)||
  draft[2]!="resource.consumed"{return out,errors.New("unregistered resource event draft")}
 parents,ok:=draft[4].([]any);if !ok||len(parents)!=0{return out,errors.New("resource event has illegal same-cycle parents")}
 typed,ok:=draft[3].([]any);if !ok||len(typed)!=4{return out,errors.New("resource event payload absent")}
 env,ok:=typed[2].([]byte);if !ok{return out,errors.New("resource event payload not an envelope")}
 parsed,err:=codec.StrictDecodeEnvelope(env);if err!=nil{return out,err}
 if parsed.SchemaID!="cote.csf.schema.typed-value"||parsed.SchemaVersion!=1{
  return out,errors.New("unregistered resource event payload schema")
 }
 digest:=identity.OpaqueContentHash(env)
 if claimed,ok:=typed[3].([]byte);!ok||!bytes.Equal(claimed,digest[:]){
  return out,errors.New("resource event payload digest invalid")
 }
 key:=[]any{uint64(1),candidate[:],"resource.consume",uint64(1),uint64(0),uint64(0)}
 id,err:=root("cote.csf.id.event","cote.csf.schema.event-id-preimage",[]any{run[:],cycle[:],key})
 if err!=nil{return out,err}
 sourceRef:=[]any{uint64(0),uint64(0),source[:],sourceDigest[:]}
 fields:=[]any{id[:],run[:],key,"resource.consumed",uint64(1),
  int64(from),uint64(sequence),cycle[:],uint64(revision),typed,[]any{},
  []any{sourceRef},[]any{},[]any{},[]any{uint64(0)},uint64(2),"resource.consume",uint64(1)}
 dg,err:=root("cote.csf.digest.event","cote.csf.schema.event.body",fields)
 if err!=nil{return out,err}
 canonical,err:=codec.CanonicalPayload(append(append([]any(nil),fields...),dg[:]))
 if err!=nil{return out,err}
 return ports.CanonicalRecord{Identity:[32]byte(id),Canonical:canonical},nil
}

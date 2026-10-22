package sqlite

import (
 "bytes"
 "context"
 "errors"
 "fmt"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/genesis"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/trigger"
)

type forkTriggerDefinition struct{
 ID [32]byte
 Canonical,ContentDigest,Runtime []byte
}
type forkTriggerActivation struct{
 ID [32]byte
 Instant int64
 Ordinal uint64
 Canonical []byte
}
type forkTriggerMigration struct{
 Definitions []forkTriggerDefinition
 Activations []forkTriggerActivation
 Seal [32]byte
}

// migrateGenesisPendingTriggers rekeys initial Genesis-owned TriggerActivation
// IDs while preserving the exact snapshot-time runtime state and pinned
// definitions. Later Event-owned activations require a separate, explicit
// per-producer migration and are fail-closed here.
func migrateGenesisPendingTriggers(ctx context.Context,s *Store,
 parent,child *genesis.Genesis,b *policy.Bundle,runtimes,activationRefs []any)(
 forkTriggerMigration,error){
 var result forkTriggerMigration
 registered,err:=s.LoadRegisteredTriggers(ctx,[32]byte(parent.RunID),b);if err!=nil{return result,err}
 if len(registered)!=len(runtimes){return result,errors.New("fork trigger runtime roster not fully covered")}
 // The V1 Genesis trigger migrator has no authority to replay a non-Genesis
 // transition at a different clock origin. Reject those cases instead of
 // publishing a child which cannot causally reconstruct its trigger runtime.
 if len(registered)>0 && child.InitialInstant!=parent.InitialInstant{
  return result,errors.New("fork advanced trigger runtime requires a versioned transition migrator")
 }
 runtimeByID:=map[string][]byte{}
 for _,raw:=range runtimes{
  fields,ok:=raw.([]any)
  if !ok||len(fields)==0{return result,errors.New("fork trigger runtime projection malformed")}
  encoded,err:=codec.CanonicalPayload(fields);if err!=nil{return result,err}
  rt,err:=trigger.DecodeRuntime(encoded);if err!=nil{return result,err}
  runtimeByID[string(rt.DefinitionID[:])]=encoded
 }
 refs:=[]any{}
 for _,item:=range registered{
  def:=item.Definition
  raw,ok:=runtimeByID[string(def.ID[:])]
  if !ok{return result,errors.New("fork trigger definition missing snapshot-time runtime")}
  state,err:=trigger.DecodeRuntime(raw)
  if err!=nil||state.DefinitionID!=def.ID||state.Version!=def.Version{
   return result,errors.New("fork trigger runtime incompatible with source definition")
  }
  digest:=item.DefinitionDigest
  result.Definitions=append(result.Definitions,forkTriggerDefinition{
   ID:[32]byte(def.ID),Canonical:bytes.Clone(item.DefinitionCanonical),
   ContentDigest:bytes.Clone(digest[:]),Runtime:raw,
  })
  refs=append(refs,[]any{def.ID[:],digest[:]})
 }
 sealRaw,err:=codec.CanonicalPayload([]any{child.ID[:],child.Digest[:],refs})
 if err!=nil{return result,err}
 result.Seal=[32]byte(identity.OpaqueContentHash(sealRaw))
 for _,item:=range activationRefs{
  ref,ok:=item.([]any)
  if !ok||len(ref)!=3||ref[0]!=uint64(7){
   return result,errors.New("fork trigger activation ref malformed")
  }
  originalID,ok:=ref[1].([]byte)
  if !ok||len(originalID)!=32{return result,errors.New("fork trigger activation ID absent")}
  checkpointDigest,ok:=ref[2].([]byte)
  if !ok||len(checkpointDigest)!=32{return result,errors.New("fork trigger activation checkpoint digest malformed")}
  var raw []byte
  if err:=s.db.QueryRowContext(ctx,"SELECT canonical FROM trigger_activations WHERE id=? AND run_id=?",
   originalID,parent.RunID[:]).Scan(&raw);err!=nil{return result,err}
  decoded,err:=codec.StrictDecodePayload(raw);if err!=nil{return result,err}
  f,ok:=decoded.([]any)
  if !ok||len(f)!=12||!equalBytes(f[0],originalID)||!equalBytes(f[1],parent.RunID[:])||
   !equalBytes(f[11],checkpointDigest){return result,errors.New("fork trigger activation not covered by snapshot")}
  origin,ok:=f[5].([]any)
  if !ok||len(origin)!=2||origin[0]!=uint64(1){
   return result,errors.New("fork trigger activation not Genesis-owned")
  }
  root,ok:=origin[1].([]any)
  if !ok||len(root)!=2||!equalBytes(root[0],parent.ID[:])||!equalBytes(root[1],parent.Digest[:]){
   return result,errors.New("fork trigger activation origin differs from parent Genesis")
  }
  definition,defOK:=f[3].([]byte)
  version,verOK:=f[4].(uint64)
  count,countOK:=f[7].(uint64)
  coordinate,atOK:=f[6].([]any)
  if !defOK||len(definition)!=32||!verOK||!countOK||!atOK||len(coordinate)!=2{
   return result,errors.New("fork trigger activation fields malformed")
  }
  exists:=false
  for _,def:=range result.Definitions{if bytes.Equal(def.ID[:],definition){exists=true}}
  if !exists{return result,errors.New("fork activation references unregistered trigger")}
  newID,_,err:=identity.New(b).Sum("cote.csf.id.trigger-activation",
   "cote.csf.schema.trigger-activation-id-preimage",1,
   []any{child.RunID[:],definition,version,uint64(0),count})
  if err!=nil{return result,err}
  body:=append([]any(nil),f[:11]...)
  body[0]=newID[:]
  body[1]=child.RunID[:]
  body[5]=[]any{uint64(1),[]any{child.ID[:],child.Digest[:]}}
  digest,_,err:=identity.New(b).Sum("cote.csf.digest.trigger-activation-unit",
   "cote.csf.schema.trigger-activation.body",1,body)
  if err!=nil{return result,err}
  canonical,err:=codec.CanonicalPayload(append(body,digest[:]));if err!=nil{return result,err}
  instant,valid:=asSigned64(coordinate[0])
  ordinal,validOrd:=coordinate[1].(uint64)
  if !valid||!validOrd||instant<int64(child.InitialInstant){
   return result,errors.New("fork trigger activation violates child clock floor")
  }
  var id [32]byte;copy(id[:],newID[:])
  for _,prior:=range result.Activations{
   if prior.ID==id{return result,errors.New("fork duplicate child trigger activation")}
  }
  result.Activations=append(result.Activations,forkTriggerActivation{
   ID:id,Instant:instant,Ordinal:ordinal,Canonical:canonical})
 }
 if len(registered)==0&&len(activationRefs)>0{return result,fmt.Errorf("activation without trigger definitions")}
 return result,nil
}

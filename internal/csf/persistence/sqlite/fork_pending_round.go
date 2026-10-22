package sqlite

import (
 "bytes"
 "context"
 "errors"
 "fmt"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/genesis"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

type forkGenesisRound struct {
 ID [32]byte
 Canonical []byte
 Seal [32]byte
}

// migrateGenesisPendingRounds reconstructs only unstarted Genesis-owned
// RoundDeclarations. Each causal ID and full record is derived under the new
// Genesis. A round with a parent Event origin, existing dispatch, or a
// response requires an independent, versioned work migrator and is rejected
// by the fork's pending partition checks.
func migrateGenesisPendingRounds(ctx context.Context, s *Store,
 parent, child *genesis.Genesis, refs []any, b *policy.Bundle)([]forkGenesisRound,error){
 out:=make([]forkGenesisRound,0,len(refs))
 seen:=map[[32]byte]bool{}
 // A round may remain pending even after one of its slots was dispatched,
 // answered, or revoked. Rebuilding only its declaration would silently
 // erase that operational history and risk a second response. A typed V2
 // dispatch/response migrator is required for every such round.
 touched:=map[string]bool{}
 rows,err:=s.db.QueryContext(ctx,"SELECT canonical FROM input_ledger")
 if err!=nil{return nil,err}
 for rows.Next(){
  var encoded []byte
  if err:=rows.Scan(&encoded);err!=nil{rows.Close();return nil,err}
  value,err:=codec.StrictDecodePayload(encoded)
  if err!=nil{
   if _,envelopeErr:=codec.StrictDecodeEnvelope(encoded);envelopeErr==nil{continue}
   rows.Close();return nil,errors.New("fork cannot classify a parent input record")
  }
  fields,ok:=value.([]any)
  if !ok{rows.Close();return nil,errors.New("fork parent input record is not canonical array")}
  if len(fields)!=11||!equalBytes(fields[1],parent.RunID[:]){continue}
  if _,cycleOK:=fields[2].([]byte);!cycleOK{continue}
  roundID,roundOK:=fields[3].([]byte)
  if _,slotOK:=fields[4].(string);roundOK&&slotOK&&len(roundID)==32{
   touched[string(roundID)]=true
  }
 }
 if err:=rows.Err();err!=nil{rows.Close();return nil,err}
 if err:=rows.Close();err!=nil{return nil,err}
 for _,rawRef:=range refs{
  ref,ok:=rawRef.([]any)
  if !ok||len(ref)!=3||ref[0]!=uint64(3){
   return nil,errors.New("fork pending round reference not canonical")
  }
  parentID,ok:=ref[1].([]byte)
  if !ok||len(parentID)!=32{return nil,errors.New("fork parent round identity invalid")}
  if touched[string(parentID)]{
   return nil,errors.New("fork round has prior dispatch history without a versioned migrator")
  }
  registeredDigest,ok:=ref[2].([]byte)
  if !ok||len(registeredDigest)!=32{return nil,errors.New("fork parent round digest invalid")}
  var raw []byte
  if err:=s.db.QueryRowContext(ctx,"SELECT canonical FROM input_ledger WHERE id=?",parentID).Scan(&raw);err!=nil{
   return nil,fmt.Errorf("fork pending round is not stored: %w",err)
  }
  value,err:=codec.StrictDecodePayload(raw)
  if err!=nil{return nil,err}
  f,ok:=value.([]any)
  if !ok||len(f)!=11||!equalBytes(f[0],parentID)||!equalBytes(f[1],parent.RunID[:]){
   return nil,errors.New("fork pending round source schema/run invalid")
  }
  digest,_,err:=identity.New(b).Sum("cote.csf.digest.round-declaration",
   "cote.csf.schema.round-declaration",1,f)
  if err!=nil||!bytes.Equal(digest[:],registeredDigest){
   return nil,errors.New("fork round checkpoint digest differs from source record")
  }
  role,roleOK:=f[7].(uint64)
  ordinal,ordinalOK:=f[8].(uint64)
  lifecycle,lifeOK:=f[9].(uint64)
  coordinate,coordOK:=f[5].([]any)
  origin,originOK:=f[6].([]any)
  if !roleOK||role>65535||!ordinalOK||ordinal>uint64(^uint32(0))||
   !lifeOK||lifecycle!=0||!coordOK||len(coordinate)!=2||!originOK||len(origin)!=2||
   origin[0]!=uint64(2) {
   return nil,errors.New("fork round has unsupported lifecycle or non-Genesis origin")
  }
  parentRoot,ok:=origin[1].([]any)
  if !ok||len(parentRoot)!=2||!equalBytes(parentRoot[0],parent.ID[:])||
   !equalBytes(parentRoot[1],parent.Digest[:]){
   return nil,errors.New("fork round origin is not authenticated parent Genesis")
  }
  instant,valid:=asSigned64(coordinate[0])
  _,validOrdinal:=coordinate[1].(uint64)
  if !valid||!validOrdinal||instant<int64(child.InitialInstant){
   return nil,errors.New("fork round eligibility precedes child Genesis clock")
  }
  source,sourceOK:=f[2].(string)
  actor,actorOK:=f[3].([]any)
  slots,slotsOK:=f[4].([]any)
  if !sourceOK||domain.ValidateMachineID(source)!=nil||!actorOK||len(actor)!=2||
   !slotsOK||len(slots)==0||len(slots)>1024{
   return nil,errors.New("fork Genesis round source/roster malformed")
  }
  for _,a:=range actor {
   s,ok:=a.(string);if !ok||domain.ValidateMachineID(s)!=nil{
    return nil,errors.New("fork round actor identity malformed")
   }
  }
  var previous string
  for i,item:=range slots{
   slot,ok:=item.(string)
   if !ok||domain.ValidateMachineID(slot)!=nil||(i>0&&slot<=previous){
    return nil,errors.New("fork round slots must form a canonical set")
   }
   previous=slot
  }
  // Reject forged source ID even if an adversary arranged a matching
  // snapshot digest; a round's ID is derived from its Genesis origin.
  wantParent,_,err:=identity.New(b).Sum("cote.csf.id.decision-round",
   "cote.csf.schema.decision-round-id-preimage",1,[]any{f[6],role,ordinal})
  if err!=nil||!bytes.Equal(wantParent[:],parentID){
   return nil,errors.New("fork parent round causal identity mismatched")
  }
  childOrigin:=[]any{uint64(2),[]any{child.ID[:],child.Digest[:]}}
  childID,_,err:=identity.New(b).Sum("cote.csf.id.decision-round",
   "cote.csf.schema.decision-round-id-preimage",1,[]any{childOrigin,role,ordinal})
  if err!=nil{return nil,err}
  newFields:=append([]any(nil),f...)
  newFields[0]=childID[:]
  newFields[1]=child.RunID[:]
  newFields[6]=childOrigin
  newCanonical,err:=codec.CanonicalPayload(newFields)
  if err!=nil{return nil,err}
  id:=[32]byte(childID)
  if seen[id]{return nil,errors.New("fork two rounds alias the same child identity")}
  seen[id]=true
  out=append(out,forkGenesisRound{
   ID:id,Canonical:newCanonical,Seal:[32]byte(identity.OpaqueContentHash(newCanonical)),
  })
 }
 return out,nil
}

package sqlite

import (
 "bytes"
 "context"
 "errors"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
)

// verifiedPerceptionRevision loads the authoritative pre/post WorldState
// revisions through the same SQL read transaction as the addressed resolver.
// There is no worker-supplied state and no mutable host callback.
func verifiedPerceptionRevision(ctx context.Context,q queryOne,revision uint64)([]any,error){
 var raw,storedHash []byte
 if err:=q.QueryRowContext(ctx,
  "SELECT canonical,state_hash FROM world_revisions WHERE revision=?",revision).
  Scan(&raw,&storedHash);err!=nil{return nil,err}
 v,err:=codec.StrictDecodePayload(raw)
 if err!=nil{return nil,err}
 fields,ok:=v.([]any)
 if !ok||len(fields)!=6||fields[3]!=revision||
  len(storedHash)!=32||!bytes.Equal(storedHash,asRevisionBytes(fields[5])){
  return nil,errors.New("perception revision root/hash malformed")
 }
 typed,ok:=fields[4].([]any)
 if !ok||len(typed)!=4{return nil,errors.New("perception WorldState typed envelope malformed")}
 encoded,ok:=typed[2].([]byte)
 if !ok{return nil,errors.New("perception WorldState missing envelope")}
 env,err:=codec.StrictDecodeEnvelope(encoded)
 if err!=nil{return nil,err}
 if !sameCanonical([]any{typed[0],typed[1]},[]any{env.SchemaID,uint64(env.SchemaVersion)}){
  return nil,errors.New("perception WorldState schema differs from embedded envelope")
 }
 h:=identity.OpaqueContentHash(encoded)
 if !bytes.Equal(h[:],storedHash)||!bytes.Equal(h[:],asRevisionBytes(typed[3])){
  return nil,errors.New("perception WorldState content hash mismatch")
 }
 return fields,nil
}
func asRevisionBytes(v any)[]byte{raw,_:=v.([]byte);return raw}

// verifyTaskStateTransition requires exact pre/result state roots for the
// commit that emitted the event. The notice is currently state-independent
// (round lifecycle), but its resolver input is still an authenticated,
// replayable pre/post transition rather than caller-supplied state.
func verifyTaskStateTransition(ctx context.Context,q queryOne,base,result uint64)error{
 if result!=base+1||result<=base{
  return errors.New("perception task revisions must be adjacent")
 }
 before,err:=verifiedPerceptionRevision(ctx,q,base)
 if err!=nil{return err}
 after,err:=verifiedPerceptionRevision(ctx,q,result)
 if err!=nil{return err}
 if after[0]!=base||!sameCanonical(after[1],before[4])||
  !sameCanonical(after[2],before[5]){
  return errors.New("perception task pre-state is not its committed base revision")
 }
 return nil
}

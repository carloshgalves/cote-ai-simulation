package sqlite

import (
 "bytes"
 "context"
 "database/sql"
 "errors"
 "fmt"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
)

// VerifyCycleControl rechecks every projected control reference against the
// immutable decision journal. It does not trust cached process status or
// a lone SQL status flag to authorize execution after crash or reconnect.
// Subsequent phases may extend the replay fold with new terminal variants.
func (s *Store) VerifyCycleControl(ctx context.Context,runID [32]byte)error{
 tx,err:=s.db.BeginTx(ctx,&sql.TxOptions{ReadOnly:true})
 if err!=nil{return err}
 defer tx.Rollback()
 ctl,err:=cycleControl(ctx,tx,runID)
 if err!=nil{return err}
 if ctl==nil{return errors.New("causal run missing authoritative CycleControlState")}
 status,ok:=ctl[0].(uint64)
 if !ok||status>3{return errors.New("unknown persisted cycle control status")}
 next,ok:=ctl[6].(uint64)
 if !ok{return errors.New("invalid persisted next attempt ordinal")}
 optional:=func(item any)(any,bool,error){
  v,ok:=item.([]any)
  if !ok||(len(v)!=1&&len(v)!=2){return nil,false,errors.New("malformed cycle control optional")}
  kind,ok:=v[0].(uint64);if !ok{return nil,false,errors.New("non-integer control optional")}
  if kind==0&&len(v)==1{return nil,false,nil}
  if kind!=1||len(v)!=2{return nil,false,errors.New("invalid control optional variant")}
  return v[1],true,nil
 }
 cycleValue,hasCycle,err:=optional(ctl[1]);if err!=nil{return err}
 ordinalValue,hasOrdinal,err:=optional(ctl[2]);if err!=nil{return err}
 var cycle [32]byte
 if hasCycle{
  raw,ok:=cycleValue.([]byte);if !ok||len(raw)!=32{return errors.New("control cycle ID invalid")}
  copy(cycle[:],raw)
 }
 var attempt uint64
 if hasOrdinal{
  attempt,ok=ordinalValue.(uint64);if !ok||attempt>uint64(^uint32(0)){return errors.New("control attempt invalid")}
 }
 validate:=func(field any,wantKind uint64)([]any,[]any,error){
  value,yes,err:=optional(field);if err!=nil{return nil,nil,err}
  if !yes{return nil,nil,nil}
  ref,ok:=value.([]any)
  if !ok||len(ref)!=3||ref[0]!=wantKind{return nil,nil,errors.New("control references wrong record kind")}
  id,ok:=ref[1].([]byte);if !ok||len(id)!=32{return nil,nil,errors.New("control ID invalid")}
  digest,ok:=ref[2].([]byte);if !ok||len(digest)!=32{return nil,nil,errors.New("control digest invalid")}
  var canonical []byte
  if err:=tx.QueryRowContext(ctx,"SELECT canonical FROM decision_ledger WHERE id=?",id).Scan(&canonical);err!=nil{return nil,nil,err}
  payload,err:=codec.StrictDecodePayload(canonical);if err!=nil{return nil,nil,err}
  fields,ok:=payload.([]any);if !ok{return nil,nil,errors.New("control references nonrecord")}
  switch wantKind{
  case 11:
   if len(fields)!=18{return nil,nil,errors.New("control fence arity mismatch")}
   got,err:=registeredDigest("cote.csf.digest.fence","cote.csf.schema.admission-fence.body",fields[:17]);if err!=nil{return nil,nil,err}
   if !bytes.Equal(got[:],digest)||!equalBytes(fields[17],digest){return nil,nil,errors.New("control fence digest invalid")}
  case 13:
   if len(fields)!=15{return nil,nil,errors.New("control abort arity mismatch")}
   got,err:=registeredDigest("cote.csf.digest.abort","cote.csf.schema.cycle-abort.body",fields[:14]);if err!=nil{return nil,nil,err}
   if !bytes.Equal(got[:],digest)||!equalBytes(fields[14],digest){return nil,nil,errors.New("control abort digest invalid")}
  case 14:
   if len(fields)!=7{return nil,nil,errors.New("control retry arity mismatch")}
   got,err:=registeredDigest("cote.csf.digest.attempt-retry","cote.csf.schema.attempt-retry",fields);if err!=nil{return nil,nil,err}
   if !bytes.Equal(got[:],digest){return nil,nil,errors.New("control retry digest invalid")}
  default:return nil,nil,errors.New("unknown control record kind")
  }
  if !equalBytes(fields[0],runID[:])||!equalBytes(fields[1],cycle[:]){
   return nil,nil,errors.New("control root foreign to run/cycle")
  }
  return ref,fields,nil
 }
 fenceRef,fence,err:=validate(ctl[3],11);if err!=nil{return err}
 retryRef,retry,err:=validate(ctl[4],14);if err!=nil{return err}
 abortRef,abort,err:=validate(ctl[5],13);if err!=nil{return err}
 if status==0{
  if next!=0||hasCycle||hasOrdinal||fence!=nil||retry!=nil||abort!=nil{
   return errors.New("IDLE control retained an unfinished terminal reference")
  }
  return tx.Commit()
 }
 if !hasCycle||!hasOrdinal||fence==nil{
  return errors.New("active control missing cycle/attempt/fence")
 }
 if !sameCanonical(fence[2],attempt)&&status!=2{
  return errors.New("active control attempt not bound to fence")
 }
 if !equalBytes(fenceRef[2],fence[17].([]byte)){
  return errors.New("control fence ref differs from persisted fence digest")
 }
 if status==1{
  if next!=attempt{return errors.New("in-flight control next attempt differs from attempt")}
  if attempt==0{
   if retry!=nil||abort!=nil||!sameCanonical(fence[3],noRef()){
    return errors.New("first in-flight attempt has forged retry/abort history")
   }
  }else{
   if retry==nil||abort==nil||!sameCanonical(fence[3],ctl[4]){
    return errors.New("retried in-flight fence missing authenticated retry")
   }
  }
 }else if status==3{
  if retry!=nil||abort==nil||next!=attempt+1||
   !sameCanonical(abort[2],attempt)||!equalBytes(abort[8],fence[17].([]byte)){
   return errors.New("HALTED_ON_ABORT diverges from journal")
  }
 }else if status==2{
  if retry==nil||abort==nil||next!=attempt+1||
   retry[2]!=attempt||retry[3]!=next||
   !sameCanonical(retry[4],abortRef)||!sameCanonical(abort[2],attempt){
   return errors.New("RETRY_AUTHORIZED diverges from journal")
  }
 }else{return fmt.Errorf("unsupported control lifecycle: %d",status)}
 _=retryRef
 return tx.Commit()
}

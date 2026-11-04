package sqlite

import (
 "bytes"
 "context"
 "database/sql"
 "errors"
 "fmt"
 "math"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/clock"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
)

// verifyDispatchAuthority rejects a dispatch that bypasses the inputledger
// owner's checks. All authority is read from this transaction's clock and
// immutable, sealed RoundDeclaration: caller-supplied actor, slot, coordinate
// and revision cannot authorize themselves.
func verifyDispatchAuthority(ctx context.Context,tx *sql.Tx,fields []any)error{
 if len(fields)!=11{return errors.New("SlotDispatch must have 11 fields")}
 run,runOK:=idField(fields[1])
 _,cycleOK:=idField(fields[2])
 roundID,roundOK:=idField(fields[3])
 if !runOK||!cycleOK||!roundOK{return errors.New("dispatch causal references malformed")}
 slot,ok:=fields[4].(string)
 if !ok||domain.ValidateMachineID(slot)!=nil{return errors.New("dispatch slot invalid")}
 ordinal,ok:=fields[5].(uint64)
 if !ok||ordinal>math.MaxUint32{return errors.New("dispatch ordinal invalid")}
 actor,ok:=fields[6].([]any)
 if !ok||len(actor)!=2{return errors.New("dispatch actor invalid")}
 for _,a:=range actor{
  value,ok:=a.(string)
  if !ok||domain.ValidateMachineID(value)!=nil{return errors.New("dispatch actor identifier invalid")}
 }
 coord,ok:=fields[7].([]any)
 if !ok||len(coord)!=2{return errors.New("dispatch coordinate malformed")}
 if _,ok:=decodedInstant(coord[0]);!ok{return errors.New("dispatch eligible instant invalid")}
 if _,ok:=coord[1].(uint64);!ok{return errors.New("dispatch eligible ordinal invalid")}
 base,ok:=fields[8].(uint64)
 if !ok{return errors.New("dispatch base revision invalid")}
 effective,ok:=decodedInstant(fields[9])
 if !ok{return errors.New("dispatch effective instant invalid")}
 if data,ok:=fields[10].([]byte);!ok||len(data)!=32{
  return errors.New("dispatch request digest invalid")
 }
 var current []byte
 if err:=tx.QueryRowContext(ctx,"SELECT canonical FROM clock_states WHERE run_id=?",run[:]).Scan(&current);err!=nil{
  return fmt.Errorf("dispatch has no authoritative run clock: %w",err)
 }
 state,err:=clock.Decode(current)
 if err!=nil{return err}
 if uint64(state.Revision)!=base||int64(state.Instant)!=effective{
  return errors.New("dispatch base revision or effective instant differs from current authoritative clock")
 }
 if state.LastOrdinal==math.MaxInt64{
  return errors.New("dispatch cycle ordinal exhausted")
 }
 eligibleInstant,_:=decodedInstant(coord[0])
 if eligibleInstant>int64(state.Instant)||
  eligibleInstant==int64(state.Instant)&&coord[1].(uint64)>uint64(state.LastOrdinal+1){
  return errors.New("dispatch round is not eligible in current WORK cohort")
 }
 expectedCycle,err:=registeredDigest("cote.csf.id.cycle",
  "cote.csf.schema.cycle-id-preimage",
  []any{run[:],[]any{int64(state.Instant),uint64(state.LastOrdinal+1)}})
 if err!=nil||!equalBytes(fields[2],expectedCycle[:]){
  return errors.New("dispatch CycleID does not match current causal coordinate")
 }
 var roundRaw,sealed []byte
 if err:=tx.QueryRowContext(ctx,"SELECT canonical FROM input_ledger WHERE id=?",roundID[:]).Scan(&roundRaw);err!=nil{
  return errors.New("dispatch has no registered RoundDeclaration")
 }
 if err:=tx.QueryRowContext(ctx,"SELECT canonical_hash FROM round_integrity_seals WHERE id=? AND run_id=?",
  roundID[:],run[:]).Scan(&sealed);err!=nil{
  return errors.New("dispatch RoundDeclaration lacks an immutable seal")
 }
 hash:=identity.OpaqueContentHash(roundRaw)
 if !bytes.Equal(hash[:],sealed){return errors.New("dispatch RoundDeclaration seal differs from durable bytes")}
 decoded,err:=codec.StrictDecodePayload(roundRaw)
 if err!=nil{return err}
 round,ok:=decoded.([]any)
 if !ok||len(round)!=11||!equalBytes(round[0],roundID[:])||!equalBytes(round[1],run[:]){
  return errors.New("dispatch RoundDeclaration ID/run invalid")
 }
 if !sameCanonical(round[3],actor)||!sameCanonical(round[5],coord){
  return errors.New("dispatch actor/coordinate differs from sealed RoundDeclaration")
 }
 slots,ok:=round[4].([]any)
 if !ok||len(slots)==0{return errors.New("dispatch has invalid declared slots")}
 found:=false
 for _,candidate:=range slots{if candidate==slot{found=true;break}}
 if !found{return errors.New("dispatch slot not in declared round")}
 // Durable dispatch sequence: a new first dispatch has ordinal zero;
 // retries of a revoked dispatch must link to an earlier ordinal and a
 // durable revocation receipt, never to an unregistered new slot identity.
 prior,err:=tx.QueryContext(ctx,"SELECT id,canonical FROM input_ledger")
 if err!=nil{return err}
 dispatched:=map[uint64][]byte{}
 revoked:=map[string]bool{}
 for prior.Next(){
  var id,raw []byte
  if err:=prior.Scan(&id,&raw);err!=nil{prior.Close();return err}
  value,err:=codec.StrictDecodePayload(raw)
  if err!=nil{continue} // an envelope input record
  parts,ok:=value.([]any)
  if !ok{continue}
  if len(parts)==11&&equalBytes(parts[3],roundID[:])&&parts[4]==slot{
   n,ok:=parts[5].(uint64)
   if !ok{prior.Close();return errors.New("invalid persisted dispatch ordinal")}
   if _,exists:=dispatched[n];exists{prior.Close();return errors.New("two active dispatches for same round slot ordinal")}
   dispatched[n]=id
  }
  if len(parts)==3{
   ref,ok:=parts[0].([]byte)
   if ok&&len(ref)==32{
    digest:=identity.OpaqueContentHash(raw)
    if bytes.Equal(digest[:],id){revoked[string(ref)]=true}
   }
  }
 }
 if err:=prior.Err();err!=nil{prior.Close();return err}
 if err:=prior.Close();err!=nil{return err}
 if ordinal==0{
  if len(dispatched)!=0{return errors.New("first round-slot dispatch already persisted")}
 }else{
  prev,ok:=dispatched[ordinal-1]
  if !ok||!revoked[string(prev)]||len(dispatched)!=int(ordinal){
   return errors.New("dispatch retry ordinal has no valid revoked predecessor chain")
  }
 }
 return nil
}

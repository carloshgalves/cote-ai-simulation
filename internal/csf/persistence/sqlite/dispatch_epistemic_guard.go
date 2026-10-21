package sqlite

import (
 "bytes"
 "context"
 "database/sql"
 "errors"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
)

// isSlotDispatchRecord identifies the native eleven-field SlotDispatch
// (not the equally sized NoProposal, whose field 4 is a round ID).
// The barrier runs from the actual Store.Append authority so callers of
// inputledger.RegisterDispatch cannot skip a separate preflight check.
func isSlotDispatchRecord(record ports.CanonicalRecord) bool {
 value,err:=codec.StrictDecodePayload(record.Canonical)
 if err!=nil{return false}
 f,ok:=value.([]any)
 if !ok||len(f)!=11{return false}
 _,slot:=f[4].(string)
 _,actor:=f[6].([]any)
 _,coordinate:=f[7].([]any)
 _,revision:=f[8].(uint64)
 _,instant:=decodedInstant(f[9])
 return slot&&actor&&coordinate&&revision&&instant
}

// appendDispatchGuarded serializes the barrier check and the actual dispatch
// append in one SQLite transaction. A crash or concurrent commit cannot
// permit a newer response to race ahead of an older uncompleted task.
func (s *Store) appendDispatchGuarded(ctx context.Context,record ports.CanonicalRecord)error{
 v,err:=codec.StrictDecodePayload(record.Canonical);if err!=nil{return err}
 f,ok:=v.([]any)
 if !ok||len(f)!=11{return errors.New("invalid SlotDispatch record")}
 run,ok:=idField(f[1]);if !ok{return errors.New("dispatch run malformed")}
 base,ok:=f[8].(uint64);if !ok{return errors.New("dispatch revision malformed")}
 tx,err:=s.db.BeginTx(ctx,nil);if err!=nil{return err}
 defer tx.Rollback()
 if err:=verifyPriorPerceptionEmpty(ctx,tx,run,base);err!=nil{return err}
 // Dispatch idempotency uses the same byte collision semantics as the
 // input-ledger append; never use INSERT OR REPLACE.
 insert,err:=tx.ExecContext(ctx,
  "INSERT OR IGNORE INTO input_ledger(id,canonical) VALUES(?,?)",
  record.Identity[:],record.Canonical)
 if err!=nil{return err}
 affected,err:=insert.RowsAffected();if err!=nil{return err}
 if affected==0{
  var old []byte
  if err:=tx.QueryRowContext(ctx,"SELECT canonical FROM input_ledger WHERE id=?",record.Identity[:]).Scan(&old);err!=nil{return err}
  if !bytes.Equal(old,record.Canonical){return errors.New("dispatch record identity collision")}
 }
 return tx.Commit()
}

// verifyPriorPerceptionEmpty proves complete, *empty* results only. It must
// fail closed, rather than treating an unimplemented positive projection as
// a completed task. The full addressed/positive extension will widen this
// gate with exact Observation/KnowledgeInput verification.
func verifyPriorPerceptionEmpty(ctx context.Context,tx *sql.Tx,run [32]byte,through uint64)error{
 rows,err:=tx.QueryContext(ctx,"SELECT d.canonical FROM committed_cycles c JOIN decision_ledger d ON d.id=c.commit_id WHERE c.run_id=?",run[:])
 if err!=nil{return err}
 type expectedTask struct{id [32]byte;revision uint64}
 tasks:=make([]expectedTask,0)
 seen:=map[[32]byte]bool{}
 for rows.Next(){
  var raw []byte
  if err:=rows.Scan(&raw);err!=nil{rows.Close();return err}
  decoded,err:=codec.StrictDecodePayload(raw);if err!=nil{rows.Close();return err}
  f,ok:=decoded.([]any)
  if !ok||len(f)!=24||!equalBytes(f[0],run[:]){rows.Close();return errors.New("barrier invalid CycleCommit")}
  revision,ok:=f[8].(uint64)
  if !ok{rows.Close();return errors.New("barrier invalid revision")}
  if revision>through{continue}
  listed,ok:=f[18].([]any)
  if !ok{rows.Close();return errors.New("barrier task roster missing")}
  for _,v:=range listed{
   id,valid:=idField(v)
   if !valid||seen[id]{rows.Close();return errors.New("barrier duplicate/bad task identity")}
   seen[id]=true;tasks=append(tasks,expectedTask{id,revision})
  }
 }
 if err:=rows.Err();err!=nil{rows.Close();return err}
 if err:=rows.Close();err!=nil{return err}
 for _,task:=range tasks{
  var raw []byte
  if err:=tx.QueryRowContext(ctx,"SELECT canonical FROM causal_outbox WHERE id=?",task.id[:]).Scan(&raw);err!=nil{
   return errors.New("dispatch held: prior committed task missing")
  }
  decoded,err:=codec.StrictDecodePayload(raw);if err!=nil{return err}
  f,ok:=decoded.([]any)
  if !ok||len(f)!=10||!equalBytes(f[0],task.id[:])||!equalBytes(f[1],run[:])||f[5]!=task.revision||f[9]!=uint64(0){
   return errors.New("dispatch held: task disagrees with committed revision")
  }
  id,err:=registeredDigest("cote.csf.id.perception-completion",
   "cote.csf.schema.perception-completion-id-preimage",[]any{task.id[:]})
  if err!=nil{return err}
  var completionRaw []byte
  if err:=tx.QueryRowContext(ctx,"SELECT canonical FROM causal_outbox WHERE id=?",id[:]).Scan(&completionRaw);err!=nil{
   return errors.New("dispatch held: prior perception has no durable completion")
  }
  body:=[]any{id[:],task.id[:],[]any{},[]any{}}
  digest,err:=registeredDigest("cote.csf.digest.perception-completion",
   "cote.csf.schema.perception-task-completion.body",body)
  if err!=nil{return err}
  canonical,err:=codec.CanonicalPayload(append(body,digest[:]))
  if err!=nil{return err}
  if !bytes.Equal(completionRaw,canonical){
   return errors.New("dispatch held: incomplete or mismatched perception receipt")
  }
 }
 return nil
}

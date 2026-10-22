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
 // A malformed dispatch candidate must be rejected by the guarded path,
 // not disguised as a harmless generic input by changing its coordinate,
 // actor, revision or timestamp type. Other native 11-field inputs have a
 // non-string field 4.
 _,slot:=f[4].(string)
 return slot
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
 wantID,err:=registeredDigest("cote.csf.id.dispatch",
  "cote.csf.schema.dispatch-id-preimage",[]any{f[1],f[2],f[3],f[4],f[5]})
 if err!=nil||!bytes.Equal(wantID[:],record.Identity[:]){
  return errors.New("dispatch identity differs from canonical preimage")
 }
 tx,err:=s.db.BeginTx(ctx,nil);if err!=nil{return err}
 defer tx.Rollback()
 var already []byte
 lookupErr:=tx.QueryRowContext(ctx,"SELECT canonical FROM input_ledger WHERE id=?",record.Identity[:]).Scan(&already)
 if lookupErr==nil{
  if !bytes.Equal(already,record.Canonical){return errors.New("dispatch identity collision")}
  // A retry is a fresh authorization to hand off this request, even when
  // its immutable bytes were appended earlier. A deleted prior completion
  // must not turn this path into a fail-open bypass of the causal barrier.
  // Current-clock equality is intentionally not required for historical
  // idempotent retries; the task audit is against the recorded base.
  if err:=verifyPriorPerceptionEmpty(ctx,tx,run,base);err!=nil{return err}
  return tx.Commit()
 }
 if lookupErr!=sql.ErrNoRows{return lookupErr}
 if err:=verifyDispatchAuthority(ctx,tx,f);err!=nil{return err}
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

// verifyPriorPerceptionEmpty retains its original function name for the
// existing storage boundary. It now proves complete *empty or addressed*
// results, reconciling every prior committed task against its exact terminal
// Observation/KnowledgeInput projection, under the dispatch transaction.
// An unknown producer or missing receipt always fails closed.
func verifyPriorPerceptionEmpty(ctx context.Context,tx *sql.Tx,run [32]byte,through uint64)error{
 rows,err:=tx.QueryContext(ctx,"SELECT d.canonical FROM committed_cycles c JOIN decision_ledger d ON d.id=c.commit_id WHERE c.run_id=?",run[:])
 if err!=nil{return err}
 type expectedTask struct{id [32]byte;revision uint64}
 tasks:=make([]expectedTask,0)
 seen:=map[[32]byte]bool{}
 coveredRevisions:=map[uint64]bool{}
 for rows.Next(){
  var raw []byte
  if err:=rows.Scan(&raw);err!=nil{rows.Close();return err}
  decoded,err:=codec.StrictDecodePayload(raw);if err!=nil{rows.Close();return err}
  f,ok:=decoded.([]any)
  if !ok||len(f)!=24||!equalBytes(f[0],run[:]){rows.Close();return errors.New("barrier invalid CycleCommit")}
  revision,ok:=f[8].(uint64)
  if !ok||revision==0{rows.Close();return errors.New("barrier invalid revision")}
  if revision>through{continue}
  base,validBase:=f[6].(uint64)
  if !validBase||base!=revision-1||coveredRevisions[revision]{
   rows.Close()
   return errors.New("barrier duplicate or discontinuous CycleCommit revision")
  }
  coveredRevisions[revision]=true
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
 // Exactly the revisions 1..through must be present. Every key is nonzero
 // and <= through, unique by construction, so matching cardinality rules
 // out missing intermediate or terminal commits without an unbounded loop.
 if uint64(len(coveredRevisions))!=through{
  return errors.New("dispatch held: committed causal revision missing")
 }
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
  var eventRaw []byte
  eventID,ok:=idField(f[3])
  if !ok{return errors.New("dispatch task Event link invalid")}
  if err:=tx.QueryRowContext(ctx,"SELECT canonical FROM event_store WHERE id=?",eventID[:]).Scan(&eventRaw);err!=nil{return err}
  decodedEvent,err:=codec.StrictDecodePayload(eventRaw);if err!=nil{return err}
  eventFields,ok:=decodedEvent.([]any)
  if !ok||len(eventFields)!=19{return errors.New("dispatch task Event invalid")}
  if eventFields[3]=="round.consumed"{
   projection,err:=loadVerifiedRoundProjection(ctx,tx,task.id)
   if err!=nil{return err}
   if !bytes.Equal(completionRaw,projection.Completion.Canonical){
    return errors.New("dispatch held: addressed completion differs from expected projection")
   }
   if err:=checkProjectionEvidence(ctx,tx,run,task.id,projection);err!=nil{return err}
  }else{
   if err:=registeredInvisibleEngineEvent(eventFields);err!=nil{
    return errors.New("dispatch held: secret producer/phase not registered: "+err.Error())
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
   if err:=noTaskEvidence(ctx,tx,task.id);err!=nil{return err}
  }
 }
 return nil
}


// CheckPerceptionBarrier is the non-mutating counterpart of the actual
// transactional SlotDispatch guard. It verifies that all committed tasks
// causally preceding the target revision have exact durable completions.
// It does not admit a slot or change a round's membership/coordinate.
func (s *Store) CheckPerceptionBarrier(ctx context.Context,run [32]byte,throughRevision uint64)error{
 if s==nil{return errors.New("epistemic barrier requires Store")}
 tx,err:=s.db.BeginTx(ctx,&sql.TxOptions{ReadOnly:true})
 if err!=nil{return err}
 defer tx.Rollback()
 if err:=verifyPriorPerceptionEmpty(ctx,tx,run,throughRevision);err!=nil{return err}
 return tx.Commit()
}

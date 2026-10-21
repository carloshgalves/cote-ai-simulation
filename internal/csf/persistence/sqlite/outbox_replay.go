package sqlite

import (
 "bytes"
 "context"
 "errors"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
)

// verifyPendingOutboxExact is the causal outbox replay fold. A task is
// published by exactly one CycleCommit, then remains PENDING until an
// authenticated, exact completion is appended. Completed and pending tasks
// are both legal after a restart; orphan receipts, missing tasks, forged
// completion lists and evidence divergence are corruption.
func verifyPendingOutboxExact(ctx context.Context,q queryOne,run [32]byte)error{
 rows,err:=q.QueryContext(ctx,"SELECT commit_id FROM committed_cycles WHERE run_id=?",run[:])
 if err!=nil{return err}
 var commits [][]byte
 for rows.Next(){
  var id []byte
  if err:=rows.Scan(&id);err!=nil{rows.Close();return err}
  if len(id)!=32{rows.Close();return errors.New("outbox journal commit ID malformed")}
  commits=append(commits,id)
 }
 if err:=rows.Err();err!=nil{rows.Close();return err}
 if err:=rows.Close();err!=nil{return err}
 expected:=map[[32]byte]bool{}
 for _,commit:=range commits{
  var raw []byte
  if err:=q.QueryRowContext(ctx,"SELECT canonical FROM decision_ledger WHERE id=?",commit).Scan(&raw);err!=nil{return err}
  decoded,err:=codec.StrictDecodePayload(raw);if err!=nil{return err}
  journal,ok:=decoded.([]any)
  if !ok||len(journal)!=24||!equalBytes(journal[0],run[:]){return errors.New("outbox journal shape/run malformed")}
  refs,ok:=journal[18].([]any)
  if !ok{return errors.New("outbox journal task ID list missing")}
  for _,v:=range refs{
   id,ok:=idField(v)
   if !ok{return errors.New("outbox journal task ID invalid")}
   if expected[id]{return errors.New("PerceptionTask identity published in more than one CycleCommit")}
   expected[id]=true
  }
 }
 rows,err=q.QueryContext(ctx,"SELECT id,canonical FROM causal_outbox")
 if err!=nil{return err}
 tasks:=map[[32]byte][]byte{}
 completions:=map[[32]byte][]byte{}
 for rows.Next(){
  var id,raw []byte
  if err:=rows.Scan(&id,&raw);err!=nil{rows.Close();return err}
  key,ok:=idField(id)
  if !ok{rows.Close();return errors.New("invalid outbox identity")}
  decoded,err:=codec.StrictDecodePayload(raw)
  if err!=nil{rows.Close();return err}
  fields,ok:=decoded.([]any)
  if !ok{rows.Close();return errors.New("outbox root malformed")}
  switch len(fields){
  case 10: // PerceptionTask
   if !equalBytes(fields[0],id){rows.Close();return errors.New("causal_outbox task identity differs from key")}
   if !equalBytes(fields[1],run[:]){continue} // another run in the same SQLite authority
   if !expected[key]||tasks[key]!=nil||fields[9]!=uint64(0){
    rows.Close();return errors.New("unauthorized or duplicate PENDING outbox task")
   }
   tasks[key]=raw
  case 5: // PerceptionTaskCompletion
   if !equalBytes(fields[0],id){rows.Close();return errors.New("completion key differs from canonical record")}
   taskID,ok:=idField(fields[1])
   if !ok{rows.Close();return errors.New("completion task link malformed")}
   if !expected[taskID]{continue} // other runs' completions validated by their replay
   if completions[taskID]!=nil{
    rows.Close();return errors.New("duplicate PerceptionTask completion")
   }
   wantID,err:=registeredDigest("cote.csf.id.perception-completion",
    "cote.csf.schema.perception-completion-id-preimage",[]any{taskID[:]})
   if err!=nil||wantID!=key{rows.Close();return errors.New("perception completion ID mismatch")}
   wantDigest,err:=registeredDigest("cote.csf.digest.perception-completion",
    "cote.csf.schema.perception-task-completion.body",fields[:4])
   if err!=nil||!equalBytes(fields[4],wantDigest[:]){
    rows.Close();return errors.New("perception completion digest mismatch")
   }
   completions[taskID]=raw
  default:
   rows.Close();return errors.New("unregistered causal outbox record")
  }
 }
 if err:=rows.Err();err!=nil{rows.Close();return err}
 if err:=rows.Close();err!=nil{return err}
 if len(tasks)!=len(expected){return errors.New("outbox is missing a committed PerceptionTask")}
 for taskID,taskRaw:=range tasks{
  completionRaw:=completions[taskID]
  if completionRaw==nil{continue}
  parsed,err:=codec.StrictDecodePayload(taskRaw);if err!=nil{return err}
  task:=parsed.([]any)
  eventID,ok:=idField(task[3]);if !ok{return errors.New("task source ID invalid")}
  var eventRaw []byte
  if err:=q.QueryRowContext(ctx,"SELECT canonical FROM event_store WHERE id=?",eventID[:]).Scan(&eventRaw);err!=nil{return err}
  decoded,err:=codec.StrictDecodePayload(eventRaw);if err!=nil{return err}
  ev,ok:=decoded.([]any)
  if !ok||len(ev)!=19{return errors.New("task Event malformed")}
  switch ev[3]{
  case "round.consumed":
   projected,err:=loadVerifiedRoundProjection(ctx,q,taskID)
   if err!=nil{return err}
   if !bytes.Equal(completionRaw,projected.Completion.Canonical){
    return errors.New("round task completion differs from deterministic addressed projection")
   }
   if err:=checkProjectionEvidence(ctx,q,taskID,projected);err!=nil{return err}
  case "clock.advanced","resource.consumed":
   if ev[15]!=uint64(2){return errors.New("empty completion source is not secret")}
   actors,ok:=ev[12].([]any)
   if !ok||len(actors)!=0{return errors.New("empty completion source contains observers")}
   id,err:=registeredDigest("cote.csf.id.perception-completion",
    "cote.csf.schema.perception-completion-id-preimage",[]any{taskID[:]})
   if err!=nil{return err}
   body:=[]any{id[:],taskID[:],[]any{},[]any{}}
   digest,err:=registeredDigest("cote.csf.digest.perception-completion",
    "cote.csf.schema.perception-task-completion.body",body)
   if err!=nil{return err}
   want,err:=codec.CanonicalPayload(append(body,digest[:]))
   if err!=nil{return err}
   if !bytes.Equal(want,completionRaw){return errors.New("unobserved task has nonempty/altered completion")}
  default:return errors.New("completion has no registered perception resolver")
  }
 }
 return nil
}

package sqlite

import (
 "context"
 "errors"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
)

// verifyPendingOutboxExact ensures the outbox has neither missing nor
// unattached PerceptionTasks. The CSFRV1-5 pre-processing runtime supports
// only PENDING task roots; later completion roots require their own fold.
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
 expected:=map[string]bool{}
 for _,commit:=range commits{
  var raw []byte
  if err:=q.QueryRowContext(ctx,"SELECT canonical FROM decision_ledger WHERE id=?",commit).Scan(&raw);err!=nil{return err}
  decoded,err:=codec.StrictDecodePayload(raw);if err!=nil{return err}
  journal,ok:=decoded.([]any)
  if !ok||len(journal)!=24{return errors.New("outbox journal shape malformed")}
  refs,ok:=journal[18].([]any)
  if !ok{return errors.New("outbox journal task ID list missing")}
  for _,v:=range refs{
   id,ok:=v.([]byte)
   if !ok||len(id)!=32{return errors.New("outbox journal task ID invalid")}
   if expected[string(id)]{return errors.New("PerceptionTask identity published in more than one CycleCommit")}
   expected[string(id)]=true
  }
 }
 rows,err=q.QueryContext(ctx,"SELECT id,canonical FROM causal_outbox")
 if err!=nil{return err}
 found:=map[string]bool{}
 for rows.Next(){
  var id,raw []byte
  if err:=rows.Scan(&id,&raw);err!=nil{rows.Close();return err}
  if len(id)!=32||!expected[string(id)]||found[string(id)]{
   rows.Close();return errors.New("unauthorized or duplicate causal_outbox entry has no terminal CycleCommit")
  }
  decoded,err:=codec.StrictDecodePayload(raw)
  if err!=nil{rows.Close();return err}
  fields,ok:=decoded.([]any)
  if !ok||len(fields)!=10||fields[9]!=uint64(0){
   rows.Close();return errors.New("unregistered non-PENDING causal_outbox entry")
  }
  if !equalBytes(fields[0],id)||!equalBytes(fields[1],run[:]){
   rows.Close();return errors.New("causal_outbox record ID/run differs from committed authority")
  }
  found[string(id)]=true
 }
 if err:=rows.Err();err!=nil{rows.Close();return err}
 if err:=rows.Close();err!=nil{return err}
 if len(found)!=len(expected){return errors.New("outbox is missing a committed PerceptionTask")}
 return nil
}

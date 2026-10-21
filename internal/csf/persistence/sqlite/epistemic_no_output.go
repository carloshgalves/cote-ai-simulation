package sqlite

import (
 "context"
 "errors"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
)

// noTaskEvidence rejects evidence/route leftovers for a secret or still
// pending task. The current V1-6 writer commits both receipts atomically;
// therefore a task without its positive completion has no legitimate partial
// publication to expose. It also catches unindexed raw evidence rows.
func noTaskEvidence(ctx context.Context,q queryOne,task [32]byte)error{
 var routes int
 if err:=q.QueryRowContext(ctx,"SELECT COUNT(*) FROM evidence_routes WHERE task_id=?",task[:]).Scan(&routes);err!=nil{return err}
 if routes!=0{return errors.New("uncompleted/secret task has addressed routes")}
 rows,err:=q.QueryContext(ctx,"SELECT canonical FROM evidence_ledger")
 if err!=nil{return err}
 observations:=map[[32]byte]bool{}
 inputs:=make([][32]byte,0)
 for rows.Next(){
  var raw []byte
  if err:=rows.Scan(&raw);err!=nil{rows.Close();return err}
  decoded,err:=codec.StrictDecodePayload(raw);if err!=nil{rows.Close();return err}
  fields,ok:=decoded.([]any)
  if !ok{rows.Close();return errors.New("invalid evidence root")}
  if len(fields)==15{
   id,ok:=idField(fields[0])
   if !ok{rows.Close();return errors.New("invalid Observation ID")}
   if equalBytes(fields[1],task[:]){rows.Close();return errors.New("secret/pending task has unindexed Observation")}
   observations[id]=true
  }else if len(fields)==11{
   ref,ok:=fields[7].([]any)
   if !ok||len(ref)!=3{rows.Close();return errors.New("KnowledgeInput has invalid observation ref")}
   id,ok:=idField(ref[1])
   if !ok{rows.Close();return errors.New("KnowledgeInput has invalid observation ID")}
   inputs=append(inputs,id)
  }else{
   rows.Close();return errors.New("unregistered evidence root in V1-6")
  }
 }
 if err:=rows.Err();err!=nil{rows.Close();return err}
 if err:=rows.Close();err!=nil{return err}
 for _,id:=range inputs{
  if !observations[id]{return errors.New("KnowledgeInput has no durable Observation")}
 }
 return nil
}

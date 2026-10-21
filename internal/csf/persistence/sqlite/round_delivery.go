package sqlite

import (
 "bytes"
 "context"
 "database/sql"
 "errors"
 "fmt"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/perception"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
)

func insertExactEvidence(ctx context.Context,tx *sql.Tx,record ports.CanonicalRecord)error{
 res,err:=tx.ExecContext(ctx,"INSERT OR IGNORE INTO evidence_ledger(id,canonical) VALUES(?,?)",
  record.Identity[:],record.Canonical)
 if err!=nil{return err}
 count,err:=res.RowsAffected();if err!=nil{return err}
 if count==1{return nil}
 var prior []byte
 if err:=tx.QueryRowContext(ctx,"SELECT canonical FROM evidence_ledger WHERE id=?",
  record.Identity[:]).Scan(&prior);err!=nil{return err}
 if !bytes.Equal(prior,record.Canonical){return errors.New("evidence identity collision")}
 return nil
}

func insertExactRoute(ctx context.Context,tx *sql.Tx,id,task [32]byte,actor [2]string,kind int)error{
 res,err:=tx.ExecContext(ctx,"INSERT OR IGNORE INTO evidence_routes "+
  "(record_id,task_id,holder_namespace,holder_id,kind) VALUES(?,?,?,?,?)",
  id[:],task[:],actor[0],actor[1],kind)
 if err!=nil{return err}
 changed,err:=res.RowsAffected();if err!=nil{return err}
 if changed==1{return nil}
 var taskID []byte
 var ns,name string
 var savedKind int
 if err:=tx.QueryRowContext(ctx,"SELECT task_id,holder_namespace,holder_id,kind "+
 "FROM evidence_routes WHERE record_id=?",id[:]).Scan(&taskID,&ns,&name,&savedKind);err!=nil{return err}
 if !bytes.Equal(taskID,task[:])||ns!=actor[0]||name!=actor[1]||savedKind!=kind{
  return errors.New("addressed evidence route identity collision")
 }
 return nil
}

// checkProjectionEvidence enforces a two-way ledger/index reconciliation.
// It rejects extra, missing, tampered or misaddressed evidence even when
// the completion lists the two expected IDs.
func checkProjectionEvidence(ctx context.Context,q queryOne,task [32]byte,
 p perception.RoundProjection)error{
 rows,err:=q.QueryContext(ctx,"SELECT record_id,holder_namespace,holder_id,kind "+
 "FROM evidence_routes WHERE task_id=? ORDER BY kind,record_id",task[:])
 if err!=nil{return err}
 seen:=map[[32]byte]bool{}
 count:=0
 for rows.Next(){
  var id []byte;var ns,name string;var kind int
  if err:=rows.Scan(&id,&ns,&name,&kind);err!=nil{rows.Close();return err}
  recordID,ok:=idField(id)
  if !ok||seen[recordID]||ns!=p.Actor[0]||name!=p.Actor[1]{
   rows.Close();return errors.New("evidence route duplicate/foreign holder")
  }
  want:=p.Observation
  if kind==1{want=p.Input}
  if kind!=0&&kind!=1{rows.Close();return errors.New("invalid epistemic route kind")}
  if want.Identity!=recordID{rows.Close();return errors.New("evidence route has extra record")}
  seen[recordID]=true
  count++
 }
 if err:=rows.Err();err!=nil{rows.Close();return err}
 if err:=rows.Close();err!=nil{return err}
 if count!=2||!seen[p.Observation.Identity]||!seen[p.Input.Identity]{
  return errors.New("addressed evidence incomplete")
 }
 for _,want:=range []ports.CanonicalRecord{p.Observation,p.Input}{
  var durable []byte
  if err:=q.QueryRowContext(ctx,"SELECT canonical FROM evidence_ledger WHERE id=?",want.Identity[:]).Scan(&durable);err!=nil{
   return errors.New("addressed evidence referenced but not durable")
  }
  if !bytes.Equal(durable,want.Canonical){return errors.New("addressed evidence canonical mismatch")}
 }
 // An injected ledger row without its indexed authorization is also invalid.
 unindexed,err:=q.QueryContext(ctx,"SELECT id,canonical FROM evidence_ledger")
 if err!=nil{return err}
 defer unindexed.Close()
 for unindexed.Next(){
  var id,raw []byte
  if err:=unindexed.Scan(&id,&raw);err!=nil{return err}
  decoded,err:=codec.StrictDecodePayload(raw);if err!=nil{return err}
  f,ok:=decoded.([]any)
  if !ok{return errors.New("unregistered evidence payload")}
  if len(f)==15&&equalBytes(f[1],task[:])&&!bytes.Equal(id,p.Observation.Identity[:]){
   return errors.New("unindexed extra Observation for task")
  }
  if len(f)==11{
   ref,ok:=f[7].([]any)
   if ok&&len(ref)==3&&equalBytes(ref[1],p.Observation.Identity[:])&&
    !bytes.Equal(id,p.Input.Identity[:]){
    return errors.New("unindexed extra KnowledgeInput for task")
   }
  }
 }
 return unindexed.Err()
}

// DeliverRoundPerception owns the addressed evidence writer. Observer,
// semantic time, modality, percept, channel and redaction are all recomputed
// from authenticated durable authority, never from a worker payload. The
// two receipts and completion use one SQLite CAS. A competing worker can
// only discover the same canonical bytes.
func (s *Store) DeliverRoundPerception(ctx context.Context,taskID [32]byte)(
 perception.RoundProjection,error){
 return s.deliverRoundPerception(ctx,taskID,0)
}

// Test failpoint injects rollback after each logical append. No partial
// delivery or partial completion may be published.
func (s *Store) deliverRoundPerception(ctx context.Context,taskID [32]byte,failAfter uint8)(
 perception.RoundProjection,error){
 var zero perception.RoundProjection
 if s==nil{return zero,errors.New("nil evidence store")}
 tx,err:=s.db.BeginTx(ctx,nil);if err!=nil{return zero,err}
 defer tx.Rollback()
 p,err:=loadVerifiedRoundProjection(ctx,tx,taskID)
 if err!=nil{return zero,err}
 if err:=insertExactEvidence(ctx,tx,p.Observation);err!=nil{return zero,err}
 if err:=insertExactRoute(ctx,tx,p.Observation.Identity,taskID,p.Actor,0);err!=nil{return zero,err}
 if failAfter==1{return zero,errors.New("injected failure after observation")}
 if err:=insertExactEvidence(ctx,tx,p.Input);err!=nil{return zero,err}
 if err:=insertExactRoute(ctx,tx,p.Input.Identity,taskID,p.Actor,1);err!=nil{return zero,err}
 if failAfter==2{return zero,errors.New("injected failure after knowledge input")}
 if err:=checkProjectionEvidence(ctx,tx,taskID,p);err!=nil{return zero,err}
 res,err:=tx.ExecContext(ctx,"INSERT OR IGNORE INTO causal_outbox(id,canonical) VALUES(?,?)",
  p.Completion.Identity[:],p.Completion.Canonical)
 if err!=nil{return zero,err}
 affected,err:=res.RowsAffected();if err!=nil{return zero,err}
 if affected==0{
  var old []byte
  if err:=tx.QueryRowContext(ctx,"SELECT canonical FROM causal_outbox WHERE id=?",
   p.Completion.Identity[:]).Scan(&old);err!=nil{return zero,err}
  if !bytes.Equal(old,p.Completion.Canonical){return zero,errors.New("perception completion CAS collision")}
 }
 if failAfter==3{return zero,errors.New("injected failure after completion")}
 if err:=tx.Commit();err!=nil{return zero,fmt.Errorf("addressed evidence commit: %w",err)}
 return p,nil
}

// ReadAddressedEvidence returns only the holder's own observations/inputs.
// This read port never queries EventStore or WorldState and cannot return
// raw PerceptionTask, resource state, Observatory data, or other holders.
// It will fail closed if route bytes and record recipient disagree.
func (s *Store) ReadAddressedEvidence(ctx context.Context,actor [2]string)(
 []ports.CanonicalRecord,[]ports.CanonicalRecord,error){
 if s==nil||actor[0]==""||actor[1]==""{return nil,nil,errors.New("invalid holder")}
 rows,err:=s.db.QueryContext(ctx,"SELECT r.record_id,r.kind,e.canonical "+
 "FROM evidence_routes r JOIN evidence_ledger e ON e.id=r.record_id "+
 "WHERE r.holder_namespace=? AND r.holder_id=? "+
 "ORDER BY r.kind,r.task_id,r.record_id",actor[0],actor[1])
 if err!=nil{return nil,nil,err}
 defer rows.Close()
 var observations,inputs []ports.CanonicalRecord
 for rows.Next(){
  var id,raw []byte;var kind int
  if err:=rows.Scan(&id,&kind,&raw);err!=nil{return nil,nil,err}
  identity,ok:=idField(id);if !ok{return nil,nil,errors.New("evidence identity corrupt")}
  v,err:=codec.StrictDecodePayload(raw);if err!=nil{return nil,nil,err}
  f,ok:=v.([]any);if !ok{return nil,nil,errors.New("invalid addressed evidence")}
  record:=ports.CanonicalRecord{Identity:identity,Canonical:raw}
  switch kind{
  case 0:
   if len(f)!=15||!sameCanonical(f[2],[]any{actor[0],actor[1]})||
    !equalBytes(f[0],id){return nil,nil,errors.New("cross-holder Observation")}
   observations=append(observations,record)
  case 1:
   if len(f)!=11||!sameCanonical(f[3],[]any{actor[0],actor[1]})||
    !equalBytes(f[0],id){return nil,nil,errors.New("cross-holder KnowledgeInput")}
   inputs=append(inputs,record)
  default:return nil,nil,errors.New("unregistered evidence kind")
  }
 }
 return observations,inputs,rows.Err()
}

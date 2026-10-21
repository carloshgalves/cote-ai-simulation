package sqlite

import (
 "bytes"
 "context"
 "database/sql"
 "errors"
 "fmt"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/genesis"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
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

func insertExactRoute(ctx context.Context,tx *sql.Tx,record ports.CanonicalRecord,run,task [32]byte,actor [2]string,kind int)error{
 id:=record.Identity
 seal:=identity.OpaqueContentHash(record.Canonical)
 res,err:=tx.ExecContext(ctx,"INSERT OR IGNORE INTO evidence_routes "+
  "(record_id,run_id,task_id,holder_namespace,holder_id,kind,canonical_hash) VALUES(?,?,?,?,?,?,?)",
  id[:],run[:],task[:],actor[0],actor[1],kind,seal[:])
 if err!=nil{return err}
 changed,err:=res.RowsAffected();if err!=nil{return err}
 if changed==1{return nil}
 var taskID,savedHash,savedRun []byte
 var ns,name string
 var savedKind int
 if err:=tx.QueryRowContext(ctx,"SELECT run_id,task_id,holder_namespace,holder_id,kind,canonical_hash "+
 "FROM evidence_routes WHERE record_id=?",id[:]).Scan(&savedRun,&taskID,&ns,&name,&savedKind,&savedHash);err!=nil{return err}
 if !bytes.Equal(savedRun,run[:])||!bytes.Equal(taskID,task[:])||ns!=actor[0]||name!=actor[1]||savedKind!=kind||!bytes.Equal(savedHash,seal[:]){
  return errors.New("addressed evidence route identity collision")
 }
 return nil
}

// checkProjectionEvidence enforces a two-way ledger/index reconciliation.
// It rejects extra, missing, tampered or misaddressed evidence even when
// the completion lists the two expected IDs.
func checkProjectionEvidence(ctx context.Context,q queryOne,run,task [32]byte,
 p perception.RoundProjection)error{
 rows,err:=q.QueryContext(ctx,"SELECT record_id,run_id,holder_namespace,holder_id,kind "+
 "FROM evidence_routes WHERE task_id=? ORDER BY kind,record_id",task[:])
 if err!=nil{return err}
 seen:=map[[32]byte]bool{}
 count:=0
 for rows.Next(){
  var id,routeRun []byte;var ns,name string;var kind int
  if err:=rows.Scan(&id,&routeRun,&ns,&name,&kind);err!=nil{rows.Close();return err}
  if !bytes.Equal(routeRun,run[:]){rows.Close();return errors.New("addressed route crosses timelines")}
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
func (s *Store) DeliverRoundPerception(ctx context.Context,taskID [32]byte,bundle *policy.Bundle)(
 perception.RoundProjection,error){
 return s.deliverRoundPerception(ctx,taskID,bundle,0)
}

// Test failpoint injects rollback after each logical append. No partial
// delivery or partial completion may be published.
func (s *Store) deliverRoundPerception(ctx context.Context,taskID [32]byte,bundle *policy.Bundle,failAfter uint8)(
 perception.RoundProjection,error){
 var zero perception.RoundProjection
 if s==nil||bundle==nil{return zero,errors.New("perception delivery requires validated policy bundle")}
 tx,err:=s.db.BeginTx(ctx,nil);if err!=nil{return zero,err}
 defer tx.Rollback()
 var taskRaw []byte
 if err:=tx.QueryRowContext(ctx,"SELECT canonical FROM causal_outbox WHERE id=?",taskID[:]).Scan(&taskRaw);err!=nil{return zero,err}
 parsed,err:=codec.StrictDecodePayload(taskRaw);if err!=nil{return zero,err}
 taskFields,ok:=parsed.([]any)
 if !ok||len(taskFields)!=10{return zero,errors.New("perception task shape invalid")}
 run,ok:=idField(taskFields[1]);if !ok{return zero,errors.New("perception run missing")}
 var genesisRaw []byte
 if err:=tx.QueryRowContext(ctx,"SELECT canonical FROM genesis_records WHERE run_id=?",run[:]).Scan(&genesisRaw);err!=nil{return zero,err}
 if _,err:=genesis.ValidateCanonical(genesisRaw,bundle);err!=nil{
  return zero,fmt.Errorf("perception delivery Genesis pin mismatch: %w",err)
 }
 p,err:=loadVerifiedRoundProjection(ctx,tx,taskID)
 if err!=nil{return zero,err}
 if err:=insertExactEvidence(ctx,tx,p.Observation);err!=nil{return zero,err}
 if err:=insertExactRoute(ctx,tx,p.Observation,run,taskID,p.Actor,0);err!=nil{return zero,err}
 if failAfter==1{return zero,errors.New("injected failure after observation")}
 if err:=insertExactEvidence(ctx,tx,p.Input);err!=nil{return zero,err}
 if err:=insertExactRoute(ctx,tx,p.Input,run,taskID,p.Actor,1);err!=nil{return zero,err}
 if failAfter==2{return zero,errors.New("injected failure after knowledge input")}
 if err:=checkProjectionEvidence(ctx,tx,run,taskID,p);err!=nil{return zero,err}
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

// readAddressedEvidence returns only the holder's own observations/inputs.
// This read port never queries EventStore or WorldState and cannot return
// raw PerceptionTask, resource state, Observatory data, or other holders.
// It will fail closed if route bytes and record recipient disagree.
func (s *Store) readAddressedEvidence(ctx context.Context,run [32]byte,actor [2]string)(
 []ports.CanonicalRecord,[]ports.CanonicalRecord,error){
 if s==nil||actor[0]==""||actor[1]==""{return nil,nil,errors.New("invalid holder")}
 rows,err:=s.db.QueryContext(ctx,"SELECT r.record_id,r.kind,e.canonical,r.canonical_hash "+
 "FROM evidence_routes r JOIN evidence_ledger e ON e.id=r.record_id "+
 "WHERE r.run_id=? AND r.holder_namespace=? AND r.holder_id=? "+
 "ORDER BY r.kind,r.task_id,r.record_id",run[:],actor[0],actor[1])
 if err!=nil{return nil,nil,err}
 defer rows.Close()
 var observations,inputs []ports.CanonicalRecord
 for rows.Next(){
  var id,raw,storedHash []byte;var kind int
  if err:=rows.Scan(&id,&kind,&raw,&storedHash);err!=nil{return nil,nil,err}
  computed:=identity.OpaqueContentHash(raw)
  if !bytes.Equal(computed[:],storedHash){return nil,nil,errors.New("addressed evidence tampered after delivery")}
  identity,ok:=idField(id);if !ok{return nil,nil,errors.New("evidence identity corrupt")}
  v,err:=codec.StrictDecodePayload(raw);if err!=nil{return nil,nil,err}
  f,ok:=v.([]any);if !ok{return nil,nil,errors.New("invalid addressed evidence")}
  record:=ports.CanonicalRecord{Identity:identity,Canonical:raw}
  switch kind{
  case 0:
   if len(f)!=15||!sameCanonical(f[2],[]any{actor[0],actor[1]})||
    !equalBytes(f[0],id){return nil,nil,errors.New("cross-holder Observation")}
   computedID,err:=registeredDigest("cote.csf.id.observation",
    "cote.csf.schema.observation-id-preimage",[]any{f[1],f[2],f[3],f[4]})
   if err!=nil||computedID!=identity{return nil,nil,errors.New("addressed Observation ID invalid")}
   digest,err:=registeredDigest("cote.csf.digest.observation",
    "cote.csf.schema.observation.body",f[:14])
   if err!=nil||!equalBytes(f[14],digest[:]){return nil,nil,errors.New("addressed Observation digest invalid")}
   observations=append(observations,record)
  case 1:
   if len(f)!=11||!sameCanonical(f[3],[]any{actor[0],actor[1]})||
    !equalBytes(f[0],id){return nil,nil,errors.New("cross-holder KnowledgeInput")}
   ref,ok:=f[7].([]any)
   if !ok||len(ref)!=3||ref[0]!=uint64(16){return nil,nil,errors.New("addressed input Observation reference invalid")}
   computedID,err:=registeredDigest("cote.csf.id.knowledge-input",
    "cote.csf.schema.knowledge-input-id-preimage",
    []any{ref[1],f[3],f[5],f[1],f[2]})
   if err!=nil||computedID!=identity{return nil,nil,errors.New("addressed KnowledgeInput ID invalid")}
   digest,err:=registeredDigest("cote.csf.digest.knowledge-input",
    "cote.csf.schema.knowledge-input.body",f[:10])
   if err!=nil||!equalBytes(f[10],digest[:]){return nil,nil,errors.New("addressed KnowledgeInput digest invalid")}
   inputs=append(inputs,record)
  default:return nil,nil,errors.New("unregistered evidence kind")
  }
 }
 if err:=rows.Err();err!=nil{return nil,nil,err}
 if err:=rows.Close();err!=nil{return nil,nil,err}
 // A holder must never see an Observation before its KnowledgeInput and
 // the task's terminal receipt are durable. The inbox reads no EventStore.
 byObservation:=map[[32]byte]ports.CanonicalRecord{}
 for _,item:=range inputs{
  parsed,err:=codec.StrictDecodePayload(item.Canonical);if err!=nil{return nil,nil,err}
  fields:=parsed.([]any)
  ref,ok:=fields[7].([]any)
  if !ok||len(ref)!=3{return nil,nil,errors.New("inbox input observation link malformed")}
  id,ok:=idField(ref[1])
  if !ok{return nil,nil,errors.New("inbox input observation ID malformed")}
  if _,duplicate:=byObservation[id];duplicate{return nil,nil,errors.New("duplicate addressed input for Observation")}
  byObservation[id]=item
 }
 if len(byObservation)!=len(observations){
  return nil,nil,errors.New("inbox contains unmatched Observation or KnowledgeInput")
 }
 for _,obs:=range observations{
  linked,ok:=byObservation[obs.Identity]
  if !ok{return nil,nil,errors.New("inbox Observation not durably delivered")}
  decoded,err:=codec.StrictDecodePayload(obs.Canonical);if err!=nil{return nil,nil,err}
  fields:=decoded.([]any)
  task,ok:=idField(fields[1])
  if !ok{return nil,nil,errors.New("inbox Observation missing task identity")}
  completionID,err:=registeredDigest("cote.csf.id.perception-completion",
   "cote.csf.schema.perception-completion-id-preimage",[]any{task[:]})
  if err!=nil{return nil,nil,err}
  body:=[]any{completionID[:],task[:],[]any{obs.Identity[:]},[]any{linked.Identity[:]}}
  receiptHash,err:=registeredDigest("cote.csf.digest.perception-completion",
   "cote.csf.schema.perception-task-completion.body",body)
  if err!=nil{return nil,nil,err}
  expected,err:=codec.CanonicalPayload(append(body,receiptHash[:]))
  if err!=nil{return nil,nil,err}
  var actual []byte
  if err:=s.db.QueryRowContext(ctx,
   "SELECT canonical FROM causal_outbox WHERE id=?",completionID[:]).Scan(&actual);err!=nil{
   return nil,nil,errors.New("inbox cannot expose evidence without durable completion")
  }
  if !bytes.Equal(expected,actual){
   return nil,nil,errors.New("inbox completion differs from addressed receipt list")
  }
 }
 return observations,inputs,nil
}

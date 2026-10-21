package sqlite

import (
 "bytes"
 "context"
 "errors"
 "fmt"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
)

// verifyEvidenceRegistryExact is the global terminal replay fold. Checking
// only the tasks referenced by one run cannot detect an orphan Observation
// whose task is absent from every CycleCommit. This audit reconciles *all*
// evidence rows and *all* addressed route rows, then checks each causal
// task and completion. It never sends the global collection to an agent.
func verifyEvidenceRegistryExact(ctx context.Context,q queryOne) error {
 type entry struct {
  id [32]byte
  raw []byte
  fields []any
  kind int
  task [32]byte
  actor [2]string
  obs [32]byte
 }
 records:=map[[32]byte]entry{}
 rows,err:=q.QueryContext(ctx,"SELECT id,canonical FROM evidence_ledger")
 if err!=nil{return err}
 for rows.Next(){
  var rawID,raw []byte
  if err:=rows.Scan(&rawID,&raw);err!=nil{rows.Close();return err}
  id,ok:=idField(rawID)
  if !ok{rows.Close();return errors.New("evidence replay malformed ledger key")}
  decoded,err:=codec.StrictDecodePayload(raw)
  if err!=nil{rows.Close();return err}
  f,ok:=decoded.([]any)
  if !ok||len(f)==0||!equalBytes(f[0],id[:]){rows.Close();return errors.New("evidence replay identity/shape malformed")}
  rec:=entry{id:id,raw:bytes.Clone(raw),fields:f}
  switch len(f){
  case 15:
   rec.kind=0
   rec.task,ok=idField(f[1])
   if !ok{rows.Close();return errors.New("Observation missing causal task")}
   observer,valid:=f[2].([]any)
   if !valid||len(observer)!=2{rows.Close();return errors.New("Observation observer invalid")}
   ns,nsok:=observer[0].(string);actor,actorOK:=observer[1].(string)
   if !nsok||!actorOK||ns==""||actor==""{rows.Close();return errors.New("Observation actor invalid")}
   rec.actor=[2]string{ns,actor}
   expected,err:=registeredDigest("cote.csf.id.observation",
    "cote.csf.schema.observation-id-preimage",[]any{f[1],f[2],f[3],f[4]})
   if err!=nil||expected!=id{rows.Close();return errors.New("Observation identity not canonical")}
   digest,err:=registeredDigest("cote.csf.digest.observation",
    "cote.csf.schema.observation.body",f[:14])
   if err!=nil||!equalBytes(f[14],digest[:]){
    rows.Close();return errors.New("Observation digest invalid")
   }
  case 11:
   rec.kind=1
   recipient,valid:=f[3].([]any)
   if !valid||len(recipient)!=2{rows.Close();return errors.New("KnowledgeInput recipient invalid")}
   ns,nsok:=recipient[0].(string);actor,actorOK:=recipient[1].(string)
   if !nsok||!actorOK||ns==""||actor==""{rows.Close();return errors.New("KnowledgeInput actor invalid")}
   rec.actor=[2]string{ns,actor}
   ref,valid:=f[7].([]any)
   if !valid||len(ref)!=3||ref[0]!=uint64(16){rows.Close();return errors.New("KnowledgeInput Observation ref invalid")}
   rec.obs,ok=idField(ref[1])
   if !ok{rows.Close();return errors.New("KnowledgeInput Observation id invalid")}
   expected,err:=registeredDigest("cote.csf.id.knowledge-input",
    "cote.csf.schema.knowledge-input-id-preimage",[]any{ref[1],f[3],f[5],f[1],f[2]})
   if err!=nil||expected!=id{rows.Close();return errors.New("KnowledgeInput identity not canonical")}
   digest,err:=registeredDigest("cote.csf.digest.knowledge-input",
    "cote.csf.schema.knowledge-input.body",f[:10])
   if err!=nil||!equalBytes(f[10],digest[:]){
    rows.Close();return errors.New("KnowledgeInput digest invalid")
   }
  default:
   rows.Close();return errors.New("unregistered evidence type in CSFRV1-6 replay")
  }
  if _,exists:=records[id];exists{rows.Close();return errors.New("duplicate evidence identity")}
  records[id]=rec
 }
 if err:=rows.Err();err!=nil{rows.Close();return err}
 if err:=rows.Close();err!=nil{return err}
 // KnowledgeInput must be the exact typed projection of its Observation.
 inputsForObs:=map[[32]byte][]entry{}
 for _,rec:=range records{
  if rec.kind!=1{continue}
  obs,ok:=records[rec.obs]
  if !ok||obs.kind!=0{return errors.New("orphan KnowledgeInput lacks Observation")}
  ref:=rec.fields[7].([]any)
  expectedDigest,validDigest:=obs.fields[14].([]byte)
  if !validDigest||len(expectedDigest)!=32{
   return errors.New("Observation digest reference malformed")
  }
  if !equalBytes(ref[2],expectedDigest)||
   rec.actor!=obs.actor||
   !sameCanonical(rec.fields[4],obs.fields[5])||
   !sameCanonical(rec.fields[8],obs.fields[10])||
   !sameCanonical(rec.fields[9],obs.fields[12]){
   return errors.New("KnowledgeInput extends or misaddresses its Observation")
  }
  rec.task=obs.task
  records[rec.id]=rec
  inputsForObs[rec.obs]=append(inputsForObs[rec.obs],rec)
 }
 for _,rec:=range records{
  if rec.kind==0&&len(inputsForObs[rec.id])!=1{
   return errors.New("Observation has incomplete or duplicate KnowledgeInput projection")
  }
 }
 type route struct{
  task,run [32]byte
  actor [2]string
  kind int
  hash []byte
 }
 routes:=map[[32]byte]route{}
 routeRows,err:=q.QueryContext(ctx,"SELECT record_id,run_id,task_id,holder_namespace,holder_id,kind,canonical_hash FROM evidence_routes")
 if err!=nil{return err}
 for routeRows.Next(){
  var rid,ridRun,ridTask,hash []byte
  var ns,actor string
  var kind int
  if err:=routeRows.Scan(&rid,&ridRun,&ridTask,&ns,&actor,&kind,&hash);err!=nil{routeRows.Close();return err}
  id,ok:=idField(rid)
  if !ok{routeRows.Close();return errors.New("addressed route record_id malformed")}
  run,runOK:=idField(ridRun)
  task,taskOK:=idField(ridTask)
  if !runOK||!taskOK{routeRows.Close();return errors.New("addressed route provenance malformed")}
  if _,exists:=routes[id];exists{routeRows.Close();return errors.New("duplicate evidence route")}
  routes[id]=route{task:task,run:run,actor:[2]string{ns,actor},kind:kind,hash:bytes.Clone(hash)}
 }
 if err:=routeRows.Err();err!=nil{routeRows.Close();return err}
 if err:=routeRows.Close();err!=nil{return err}
 if len(routes)!=len(records){
  return fmt.Errorf("evidence ledger/route cardinality mismatch: %d records, %d routes",len(records),len(routes))
 }
 taskRoots:=map[[32]byte][32]byte{}
 taskOutputs:=map[[32]byte][]entry{}
 for id,rec:=range records{
  route,ok:=routes[id]
  if !ok{return errors.New("evidence record missing exact addressed route")}
  if route.task!=rec.task||route.actor!=rec.actor||route.kind!=rec.kind{
   return errors.New("evidence route is not the recipient/role/task projection")
  }
  h:=identity.OpaqueContentHash(rec.raw)
  if !bytes.Equal(h[:],route.hash){
   return errors.New("evidence route and canonical bytes hash differ")
  }
  // Resolve the source task from the durable outbox, not a worker list.
  run,known:=taskRoots[rec.task]
  if !known{
   var raw []byte
   if err:=q.QueryRowContext(ctx,"SELECT canonical FROM causal_outbox WHERE id=?",rec.task[:]).Scan(&raw);err!=nil{
    return errors.New("orphan Observation/KnowledgeInput references missing PerceptionTask")
   }
   parsed,err:=codec.StrictDecodePayload(raw)
   if err!=nil{return err}
   fields,ok:=parsed.([]any)
   if !ok||len(fields)!=10||!equalBytes(fields[0],rec.task[:])||fields[9]!=uint64(0){
    return errors.New("evidence source task identity/status invalid")
   }
   run,ok=idField(fields[1])
   if !ok{return errors.New("evidence source task run malformed")}
   taskRoots[rec.task]=run
  }
  if route.run!=run{return errors.New("evidence route crosses causal run")}
  taskOutputs[rec.task]=append(taskOutputs[rec.task],rec)
 }
 // V1-6 currently has only one registered positive projection, a single
 // round notice to a single actor, so each addressed task must have exactly
 // one Observation and its one KnowledgeInput, with an exact terminal root.
 for task,outputs:=range taskOutputs{
  if len(outputs)!=2{return errors.New("addressed task has incomplete/extra evidence outputs")}
  var obs,input *entry
  for i:=range outputs{
   switch outputs[i].kind{
   case 0:obs=&outputs[i]
   case 1:input=&outputs[i]
   }
  }
  if obs==nil||input==nil||input.obs!=obs.id{
   return errors.New("addressed task output pairing invalid")
  }
  completionID,err:=registeredDigest("cote.csf.id.perception-completion",
   "cote.csf.schema.perception-completion-id-preimage",[]any{task[:]})
  if err!=nil{return err}
  body:=[]any{completionID[:],task[:],[]any{obs.id[:]},[]any{input.id[:]}}
  digest,err:=registeredDigest("cote.csf.digest.perception-completion",
   "cote.csf.schema.perception-task-completion.body",body)
  if err!=nil{return err}
  expected,err:=codec.CanonicalPayload(append(body,digest[:]))
  if err!=nil{return err}
  var persisted []byte
  if err:=q.QueryRowContext(ctx,"SELECT canonical FROM causal_outbox WHERE id=?",completionID[:]).Scan(&persisted);err!=nil{
   return errors.New("addressed output has no completed task")
  }
  if !bytes.Equal(expected,persisted){
   return errors.New("addressed output not in canonical terminal completion")
  }
 }
 return nil
}

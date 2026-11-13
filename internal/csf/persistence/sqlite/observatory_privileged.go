package sqlite

import (
 "bytes"
 "context"
 "errors"
 "fmt"
 "reflect"
 "sort"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/observatory"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

// PrivilegedReader deliberately does not implement observatory.Queries.
// The privileged audit result is not a permissible actor KnowledgeInput.
type PrivilegedReader struct{ base *ObservatoryReader }

var _ observatory.PrivilegedQueries = (*PrivilegedReader)(nil)

func OpenPrivilegedObservatory(ctx context.Context,path string,bundle *policy.Bundle)(*PrivilegedReader,error){
 base,err:=OpenObservatory(ctx,path,bundle)
 if err!=nil{return nil,err}
 return &PrivilegedReader{base:base},nil
}
func (q *PrivilegedReader) Close()error{
 if q==nil||q.base==nil{return errors.New("nil privileged Observatory")}
 return q.base.Close()
}

type o2Record struct{
 id [32]byte
 bytes []byte
 fields []any
}
type o2CommitRow struct{cycle,commit [32]byte}
type o2Snapshot struct{
 decisions map[[32]byte]o2Record
 committed []o2CommitRow
}

func o2Bytes(v any)([32]byte,error){
 var id [32]byte
 raw,ok:=v.([]byte)
 if !ok||len(raw)!=32{return id,errors.New("expected 32-byte causal identity or digest")}
 copy(id[:],raw)
 return id,nil
}
func o2Uint(v any)(uint64,error){
 n,ok:=v.(uint64);if !ok{return 0,errors.New("expected uint64 causal ordinal/revision")}
 return n,nil
}
func o2Instant(v any)(int64,error){
 n,ok:=asSigned64(v);if !ok{return 0,errors.New("expected signed simulation instant")}
 return n,nil
}
func o2Ref(table string,id [32]byte,raw []byte)observatory.AuthorityRef{
 return observatory.AuthorityRef{Ledger:table,ID:id,ContentHash:[32]byte(identity.OpaqueContentHash(raw))}
}

// Snapshot copies candidate immutable ledger rows without any command-port,
// shared mutable Store object or schema initialization. Results are checked
// against normative replay and a connection-relative data_version fence.
func(q *PrivilegedReader) snapshot(ctx context.Context,run [32]byte)(o2Snapshot,error){
 var out o2Snapshot
 out.decisions=map[[32]byte]o2Record{}
 db:=q.base.store.db
 rows,err:=db.QueryContext(ctx,"SELECT id,canonical FROM decision_ledger ORDER BY id")
 if err!=nil{return out,err}
 for rows.Next(){
  var id,raw []byte
  if err:=rows.Scan(&id,&raw);err!=nil{rows.Close();return out,err}
  if len(id)!=32{rows.Close();return out,errors.New("decision_ledger has invalid record identity")}
  var key [32]byte;copy(key[:],id)
  value,err:=codec.StrictDecodePayload(raw)
  if err!=nil{rows.Close();return out,fmt.Errorf("decision_ledger record %x: %w",key,err)}
  fields,ok:=value.([]any)
  if !ok{rows.Close();return out,fmt.Errorf("decision_ledger record %x is not canonical record",key)}
  out.decisions[key]=o2Record{id:key,bytes:raw,fields:fields}
 }
 if err:=rows.Err();err!=nil{rows.Close();return out,err}
 if err:=rows.Close();err!=nil{return out,err}
 rows,err=db.QueryContext(ctx,"SELECT cycle_id,commit_id FROM committed_cycles WHERE run_id=?",run[:])
 if err!=nil{return out,err}
 for rows.Next(){
  var cycle,commit []byte
  if err:=rows.Scan(&cycle,&commit);err!=nil{rows.Close();return out,err}
  cid,err:=o2Bytes(cycle);if err!=nil{rows.Close();return out,err}
  id,err:=o2Bytes(commit);if err!=nil{rows.Close();return out,err}
  out.committed=append(out.committed,o2CommitRow{cycle:cid,commit:id})
 }
 if err:=rows.Err();err!=nil{rows.Close();return out,err}
 if err:=rows.Close();err!=nil{return out,err}
 return out,nil
}

// ReadPrivilegedReport is all-or-error. An incomplete/malformed historical
// chain never returns a partially credible report. On concurrent mutations,
// it fails closed rather than combining mismatched SQLite snapshots.
func(q *PrivilegedReader) ReadPrivilegedReport(ctx context.Context,run [32]byte)(observatory.PrivilegedReport,error){
 var empty observatory.PrivilegedReport
 if q==nil||q.base==nil{return empty,errors.New("privileged Observatory not initialized")}
 version,err:=q.base.dataVersion(ctx)
 if err!=nil{return empty,err}
 state,err:=q.base.ReadRun(ctx,run)
 if err!=nil{return empty,fmt.Errorf("privileged causal replay: %w",err)}
 snap,err:=q.snapshot(ctx,run)
 if err!=nil{return empty,err}
 attempts,fences,err:=o2ReadAttempts(run,snap.decisions)
 if err!=nil{return empty,err}
 commits,err:=q.readCommits(ctx,run,snap.committed,snap.decisions,fences,attempts)
 if err!=nil{return empty,err}
 finalized,err:=o2FinalizeAttempts(state,attempts)
 if err!=nil{return empty,err}
 pending,err:=q.base.store.PendingClock(ctx,run)
 if err!=nil{return empty,fmt.Errorf("privileged pending source proof: %w",err)}
 after,err:=q.base.ReadRun(ctx,run)
 if err!=nil{return empty,err}
 versionNow,err:=q.base.dataVersion(ctx)
 if err!=nil{return empty,err}
 if version!=versionNow||!reflect.DeepEqual(state,after){
  return empty,errors.New("privileged report authorities changed during verification")
 }
 return observatory.PrivilegedReport{
  State:state, Commits:commits, Attempts:finalized,
  PendingUnits:len(pending),RejectedIngress:"NOT_PERSISTED",
 },nil
}

func(q *PrivilegedReader) raw(ctx context.Context,table string,id [32]byte)([]byte,error){
 // The caller must select a table exclusively from this closed set.
 switch table{
 case "input_ledger","event_store","causal_outbox","scheduled_occurrences","trigger_activations":
 default:return nil,fmt.Errorf("unapproved Observatory authority table %q",table)
 }
 var raw []byte
 err:=q.base.store.db.QueryRowContext(ctx,"SELECT canonical FROM "+table+" WHERE id=?",id[:]).Scan(&raw)
 if err!=nil{return nil,fmt.Errorf("%s %x unavailable: %w",table,id,err)}
 return raw,nil
}
func(q *PrivilegedReader) readRef(ctx context.Context,table string,id [32]byte)(observatory.AuthorityRef,[]any,error){
 var zero observatory.AuthorityRef
 raw,err:=q.raw(ctx,table,id)
 if err!=nil{return zero,nil,err}
 decoded,err:=codec.StrictDecodePayload(raw)
 if err!=nil{return zero,nil,fmt.Errorf("%s %x canonical decode: %w",table,id,err)}
 fields,ok:=decoded.([]any)
 if !ok{return zero,nil,fmt.Errorf("%s %x malformed record body",table,id)}
 return o2Ref(table,id,raw),fields,nil
}

func(q *PrivilegedReader) readCommits(ctx context.Context,run [32]byte,
 rows []o2CommitRow,records map[[32]byte]o2Record,
 fences map[o2AttemptKey]o2Record,attempts map[o2AttemptKey]*observatory.CausalAttempt)([]observatory.CausalCommit,error){
 out:=make([]observatory.CausalCommit,0,len(rows))
 for _,entry:=range rows{
  record,ok:=records[entry.commit]
  if !ok{return nil,fmt.Errorf("CycleCommit %x absent from decision_ledger",entry.commit)}
  v:=record.fields
  if len(v)!=24||!equalBytes(v[0],run[:])||!equalBytes(v[1],entry.cycle[:]){
   return nil,fmt.Errorf("CycleCommit %x run/cycle not authenticated",entry.commit)
  }
  attempt,err:=o2Uint(v[2]);if err!=nil{return nil,err}
  k:=o2AttemptKey{cycle:entry.cycle,attempt:attempt}
  fence,ok:=fences[k]
  if !ok{return nil,fmt.Errorf("CycleCommit %x missing authoritative fence",entry.commit)}
  id,err:=registeredDigest("cote.csf.id.cycle-commit","cote.csf.schema.cycle-commit-id-preimage",[]any{run[:],entry.cycle[:],attempt})
  if err!=nil||[32]byte(id)!=entry.commit{return nil,fmt.Errorf("CycleCommit %x identity forged",entry.commit)}
  instant,err:=o2Instant(v[3]);if err!=nil{return nil,err}
  ordinal,err:=o2Uint(v[4]);if err!=nil{return nil,err}
  base,err:=o2Uint(v[6]);if err!=nil{return nil,err}
  next,err:=o2Uint(v[8]);if err!=nil{return nil,err}
  bh,err:=o2Bytes(v[7]);if err!=nil{return nil,err}
  rh,err:=o2Bytes(v[9]);if err!=nil{return nil,err}
  input,err:=o2Bytes(v[12]);if err!=nil{return nil,err}
  decision,err:=o2Bytes(v[17]);if err!=nil{return nil,err}
  if !sameCanonical(v[10],fence.fields[17])||!sameCanonical(v[12],fence.fields[12])||
   !sameCanonical(v[3],fence.fields[4])||!sameCanonical(v[4],fence.fields[5])||
   !sameCanonical(v[5],fence.fields[6])||!sameCanonical(v[6],fence.fields[7])||
   !sameCanonical(v[7],fence.fields[8])||next!=base+1{
   return nil,fmt.Errorf("CycleCommit %x differs from AdmissionFence causal cut",entry.commit)
  }
  plan,ok:=v[5].([]any)
  if !ok{return nil,fmt.Errorf("CycleCommit %x plan missing",entry.commit)}
  kind:="WORK"
  if len(plan)==4&&plan[0]==uint64(1){kind="CLOCK_ADVANCE"
  }else if len(plan)!=1||plan[0]!=uint64(0){return nil,fmt.Errorf("CycleCommit %x plan unknown",entry.commit)}
  c:=observatory.CausalCommit{
   CycleID:entry.cycle,Instant:instant,Ordinal:ordinal,Attempt:attempt,
   Plan:kind,BaseRevision:base,ResultRevision:next,
   BaseWorldHash:bh,ResultWorldHash:rh,InputDigest:input,
   DecisionDigest:decision,
   Fence:o2Ref("decision_ledger",fence.id,fence.bytes),
   Commit:o2Ref("decision_ledger",entry.commit,record.bytes),
  }
  c.Closures,err=q.o2Closures(ctx,run,fence.fields)
  if err!=nil{return nil,fmt.Errorf("CycleCommit %x closure proof: %w",entry.commit,err)}
  c.Admitted,err=q.o2Admitted(ctx,run,fence.fields)
  if err!=nil{return nil,fmt.Errorf("CycleCommit %x admitted proof: %w",entry.commit,err)}
  evIDs,ok:=v[15].([]any)
  if !ok{return nil,fmt.Errorf("CycleCommit %x event list malformed",entry.commit)}
  rawEvents:=make([]any,0,len(evIDs))
  for _,anyID:=range evIDs{
   eid,err:=o2Bytes(anyID);if err!=nil{return nil,err}
   ref,fields,err:=q.readRef(ctx,"event_store",eid)
   if err!=nil{return nil,fmt.Errorf("CycleCommit %x event: %w",entry.commit,err)}
   if len(fields)!=19||!equalBytes(fields[0],eid[:])||!equalBytes(fields[1],run[:])||!equalBytes(fields[7],entry.cycle[:]){
    return nil,fmt.Errorf("CycleCommit %x event %x foreign/malformed",entry.commit,eid)
   }
   dg,err:=registeredDigest("cote.csf.digest.event","cote.csf.schema.event.body",fields[:18])
   if err!=nil||!equalBytes(fields[18],dg[:]){return nil,fmt.Errorf("CycleCommit %x event %x digest invalid",entry.commit,eid)}
   raw,err:=q.raw(ctx,"event_store",eid);if err!=nil{return nil,err}
   rawEvents=append(rawEvents,raw)
   c.Events=append(c.Events,ref)
  }
  batch,err:=registeredDigest("cote.csf.digest.batch","cote.csf.schema.batch-preimage",[]any{v[:23],rawEvents})
  if err!=nil||!equalBytes(v[23],batch[:]){return nil,fmt.Errorf("CycleCommit %x batch hash invalid",entry.commit)}
  decisions,ok:=v[16].([]any)
  if !ok{return nil,fmt.Errorf("CycleCommit %x decisions malformed",entry.commit)}
  for _,item:=range decisions{
   did,err:=o2Bytes(item);if err!=nil{return nil,err}
   rec,exists:=records[did]
   if !exists||len(rec.fields)!=16||!equalBytes(rec.fields[0],did[:])||
    !equalBytes(rec.fields[1],run[:])||!equalBytes(rec.fields[2],entry.cycle[:]){
    return nil,fmt.Errorf("CycleCommit %x decision %x missing or foreign",entry.commit,did)
   }
   dg,err:=registeredDigest("cote.csf.digest.decision-record","cote.csf.schema.decision-record.body",rec.fields[:15])
   if err!=nil||!equalBytes(rec.fields[15],dg[:]){return nil,fmt.Errorf("CycleCommit %x decision %x digest invalid",entry.commit,did)}
   c.Decisions=append(c.Decisions,o2Ref("decision_ledger",did,rec.bytes))
  }
  tasks,ok:=v[18].([]any)
  if !ok{return nil,fmt.Errorf("CycleCommit %x task list malformed",entry.commit)}
  for _,item:=range tasks{
   task,err:=o2Bytes(item);if err!=nil{return nil,err}
   ref,_,err:=q.readRef(ctx,"causal_outbox",task)
   if err!=nil{return nil,fmt.Errorf("CycleCommit %x perception task: %w",entry.commit,err)}
   c.Tasks=append(c.Tasks,ref)
  }
  var stateHash []byte
  if err:=q.base.store.db.QueryRowContext(ctx,"SELECT state_hash FROM world_revisions WHERE revision=?",next).Scan(&stateHash);err!=nil{return nil,err}
  if !bytes.Equal(stateHash,rh[:]){return nil,fmt.Errorf("CycleCommit %x world revision hash differs",entry.commit)}
  if a:=attempts[k];a!=nil{
   if a.Commit!=nil||a.Abort!=nil{return nil,fmt.Errorf("CycleCommit %x duplicate terminal attempt",entry.commit)}
   ref:=c.Commit;a.Commit=&ref
  }else{return nil,fmt.Errorf("CycleCommit %x was not admitted",entry.commit)}
  out=append(out,c)
 }
 sort.Slice(out,func(i,j int)bool{
  if out[i].ResultRevision!=out[j].ResultRevision{return out[i].ResultRevision<out[j].ResultRevision}
  return bytes.Compare(out[i].CycleID[:],out[j].CycleID[:])<0
 })
 for i,c:=range out{
  if c.ResultRevision!=uint64(i)+1||c.BaseRevision!=uint64(i){
   return nil,fmt.Errorf("privileged causal revision sequence not contiguous at %d",i)
  }
  if i>0&&c.BaseWorldHash!=out[i-1].ResultWorldHash{
   return nil,fmt.Errorf("privileged causal world hash chain diverged at %d",i)
  }
 }
 return out,nil
}

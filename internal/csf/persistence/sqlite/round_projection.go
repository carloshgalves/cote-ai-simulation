package sqlite

import (
 "bytes"
 "context"
 "errors"
 "fmt"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/perception"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
)

// loadVerifiedRoundProjection derives a recipient from immutable authorities
// only: the committed task and event, authenticated Genesis policy pins,
// and the atomically sealed RoundDeclaration. Never accept an observer,
// channel, or projection supplied by the worker or dispatch caller.
func loadVerifiedRoundProjection(ctx context.Context,q queryOne,taskID [32]byte)(perception.RoundProjection,error){
 var empty perception.RoundProjection
 var taskRaw []byte
 if err:=q.QueryRowContext(ctx,"SELECT canonical FROM causal_outbox WHERE id=?",taskID[:]).Scan(&taskRaw);err!=nil{
  return empty,fmt.Errorf("perception task absent: %w",err)
 }
 tv,err:=codec.StrictDecodePayload(taskRaw);if err!=nil{return empty,err}
 t,ok:=tv.([]any)
 if !ok||len(t)!=10||!equalBytes(t[0],taskID[:])||t[9]!=uint64(0){
  return empty,errors.New("non-PENDING or forged perception task")
 }
 run,runOK:=idField(t[1]);cycle,cycOK:=idField(t[2]);eventID,evtOK:=idField(t[3])
 if !runOK||!cycOK||!evtOK{return empty,errors.New("task run/cycle/event malformed")}
 base,baseOK:=t[4].(uint64)
 result,resultOK:=t[5].(uint64)
 if !baseOK||!resultOK{return empty,errors.New("task base/result state revision malformed")}
 if err:=verifyTaskStateTransition(ctx,q,base,result);err!=nil{
  return empty,fmt.Errorf("perception pre/post authority mismatch: %w",err)
 }
 for _,idx:=range []int{6,7,8}{
  ref,ok:=t[idx].([]any)
  if !ok||len(ref)!=3{return empty,errors.New("malformed pinned perception policy")}
  h,ok:=ref[2].([]byte)
  if !ok||len(h)!=32{return empty,errors.New("perception hash absent")}
 }
 expectedTaskID,err:=registeredDigest("cote.csf.id.perception-task",
  "cote.csf.schema.perception-task-id-preimage",
  []any{run[:],eventID[:],t[6],t[7],t[8]})
 if err!=nil||expectedTaskID!=taskID{return empty,errors.New("task not derivable from pinned components")}
 var genesisRaw []byte
 if err:=q.QueryRowContext(ctx,"SELECT canonical FROM genesis_records WHERE run_id=?",run[:]).Scan(&genesisRaw);err!=nil{return empty,err}
 env,err:=codec.StrictDecodeEnvelope(genesisRaw);if err!=nil{return empty,err}
 genesis,ok:=env.Value.([]any)
 if !ok||len(genesis)!=18||!equalBytes(genesis[0],run[:]){return empty,errors.New("genesis authority malformed")}
 policies,ok:=genesis[14].([]any)
 if !ok{return empty,errors.New("genesis policies missing")}
 for _,idx:=range []int{6,7,8}{
  found:=false
  for _,entry:=range policies{
   if sameCanonical(entry,t[idx]){found=true;break}
  }
  if !found{return empty,errors.New("task policy not pinned by genesis")}
 }
 var eventRaw,journalRaw []byte
 if err:=q.QueryRowContext(ctx,"SELECT canonical FROM event_store WHERE id=?",eventID[:]).Scan(&eventRaw);err!=nil{return empty,err}
 if err:=q.QueryRowContext(ctx,
  "SELECT d.canonical FROM committed_cycles c JOIN decision_ledger d ON c.commit_id=d.id WHERE c.run_id=? AND c.cycle_id=?",
  run[:],cycle[:]).Scan(&journalRaw);err!=nil{return empty,err}
 j,err:=codec.StrictDecodePayload(journalRaw);if err!=nil{return empty,err}
 journal,ok:=j.([]any)
 if !ok||len(journal)!=24||!equalBytes(journal[0],run[:])||!equalBytes(journal[1],cycle[:])||
  !sameCanonical(journal[6],t[4])||!sameCanonical(journal[8],t[5])||
  !containsRoot(journal[15],eventID)||!containsRoot(journal[18],taskID){
  return empty,errors.New("task/event cannot resolve to terminal CycleCommit")
 }
 ev,err:=codec.StrictDecodePayload(eventRaw);if err!=nil{return empty,err}
 e,ok:=ev.([]any)
 if !ok||len(e)!=19{return empty,errors.New("source Event malformed")}
 key,ok:=e[2].([]any)
 if !ok||len(key)!=6{return empty,errors.New("EventOrderKey malformed")}
 roundID,ok:=idField(key[1]);if !ok{return empty,errors.New("round lifecycle key missing")}
 var roundRaw,sealed []byte
 if err:=q.QueryRowContext(ctx,"SELECT canonical FROM input_ledger WHERE id=?",roundID[:]).Scan(&roundRaw);err!=nil{
  return empty,errors.New("round route missing authenticated declaration")
 }
 if err:=q.QueryRowContext(ctx,
  "SELECT canonical_hash FROM round_integrity_seals WHERE id=? AND run_id=?",
  roundID[:],run[:]).Scan(&sealed);err!=nil{
  return empty,errors.New("round route missing immutability seal")
 }
 digest:=identity.OpaqueContentHash(roundRaw)
 if !bytes.Equal(digest[:],sealed){return empty,errors.New("round route canonical seal changed")}
 return perception.ProjectRoundNotice(
  ports.CanonicalRecord{Identity:taskID,Canonical:taskRaw},
  ports.CanonicalRecord{Identity:eventID,Canonical:eventRaw},
  ports.CanonicalRecord{Identity:roundID,Canonical:roundRaw},
 )
}

package sqlite

import (
 "context"
 "database/sql"
 "errors"
 "fmt"
 "sort"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/clock"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/observatory"
)

// o2ClosureGaps diagnoses only the next deterministic eligibility coordinate.
// A source lacking authoritative covering SourceClosure is reported explicitly;
// absence of an Event is NEVER used to infer source completion or rejection.
func(q *PrivilegedReader) o2ClosureGaps(ctx context.Context,run [32]byte,
 state observatory.RunState,pending []clock.Pending)([]string,error){
 plan,exists,err:=clock.Next(state.Clock,pending)
 if err!=nil{return nil,fmt.Errorf("observatory next causal coordinate: %w",err)}
 if !exists{return nil,nil}
 target:=plan.Target
 rows,err:=q.base.store.db.QueryContext(ctx,"SELECT canonical FROM input_ledger ORDER BY id")
 if err!=nil{return nil,err}
 sources:=map[string]bool{}
 closures:=map[string]bool{}
 for rows.Next(){
  var raw []byte
  if err:=rows.Scan(&raw);err!=nil{rows.Close();return nil,err}
  decoded,err:=codec.StrictDecodePayload(raw)
  if err!=nil{rows.Close();return nil,fmt.Errorf("observatory closure input ledger: %w",err)}
  fields,ok:=decoded.([]any)
  if !ok{continue}
  if len(fields)==3&&fields[0]=="source-declaration-v1"{
   if !equalBytes(fields[1],run[:]){rows.Close();return nil,errors.New("foreign source declaration in run")}
   source,ok:=fields[2].(string)
   if !ok||source==""{rows.Close();return nil,errors.New("malformed source declaration")}
   sources[source]=true
  }else if len(fields)==7&&equalBytes(fields[1],run[:]){
   source,ok:=fields[2].(string)
   if !ok||source==""{continue}
   final,ok:=fields[5].(bool)
   if !ok{continue}
   pair,ok:=fields[4].([]any)
   if !ok||len(pair)!=2{rows.Close();return nil,errors.New("source closure coordinate malformed")}
   instant,err:=o2Instant(pair[0]);if err!=nil{rows.Close();return nil,err}
   ordinal,err:=o2Uint(pair[1]);if err!=nil{rows.Close();return nil,err}
   at:=domain.EligibilityCoordinate{NotBeforeInstant:domain.SimulationInstant(instant),NotBeforeCycleOrdinal:domain.CycleOrdinal(ordinal)}
   if final||at.Compare(target)>=0{closures[source]=true}
  }
 }
 if err:=rows.Err();err!=nil{rows.Close();return nil,err}
 if err:=rows.Close();err!=nil{return nil,err}
 var missing []string
 for source:=range sources{if !closures[source]{missing=append(missing,source)}}
 sort.Strings(missing)
 return missing,nil
}

// A canonical task is pending until its registered completion ID appears in
// causal_outbox. No inference from a knowledge receipt or a narrative output.
func(q *PrivilegedReader) o2PendingTasks(ctx context.Context,run [32]byte,
 commits []observatory.CausalCommit,revision uint64)(int,error){
 var missing int
 for _,commit:=range commits{
  for _,task:=range commit.Tasks{
   completion,err:=registeredDigest("cote.csf.id.perception-completion",
    "cote.csf.schema.perception-completion-id-preimage",[]any{task.ID[:]})
   if err!=nil{return 0,err}
   var raw []byte
   err=q.base.store.db.QueryRowContext(ctx,"SELECT canonical FROM causal_outbox WHERE id=?",completion[:]).Scan(&raw)
   if err==sql.ErrNoRows{missing++;continue}
   if err!=nil{return 0,err}
   if _,err:=codec.StrictDecodePayload(raw);err!=nil{return 0,fmt.Errorf("task completion %x malformed: %w",completion,err)}
  }
 }
 if missing==0{
  if err:=q.base.store.CheckPerceptionBarrier(ctx,run,revision);err!=nil{
   return 0,fmt.Errorf("prior perception barrier verification: %w",err)
  }
 }
 return missing,nil
}

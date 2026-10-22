package sqlite

import (
 "bytes"
 "context"
 "errors"
 "fmt"
 "sort"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

// DrainPendingPerceptionV1 deterministically completes previously committed
// perception tasks before the next agent dispatch. Each handler derives
// recipients and proof exclusively from the persisted event and journal.
// A crash may split this across transactions: rerun is idempotent because
// terminal completion records are immutable and uniquely indexed.
func (s *Store) DrainPendingPerceptionV1(ctx context.Context,run,snapshotID [32]byte,b *policy.Bundle)(ResumeCheckpointV1,error){
 var zero ResumeCheckpointV1
 if s==nil||b==nil{return zero,errors.New("perception resume requires authority and pinned policy")}
 before,err:=s.VerifySnapshotV1(ctx,run,snapshotID,b)
 if err!=nil{return zero,fmt.Errorf("pre-drain causal replay: %w",err)}
 // HALTED_ON_ABORT is stronger than a pending perception barrier. It must
 // remain halted; completing past tasks must not create retry authorization.
 if before.ControlStatus==3{return before,nil}
 rows,err:=s.db.QueryContext(ctx,"SELECT id,canonical FROM causal_outbox ORDER BY id")
 if err!=nil{return zero,err}
 type item struct{id [32]byte;event [32]byte;run [32]byte}
 tasks:=map[[32]byte]item{}
 complete:=map[[32]byte]bool{}
 for rows.Next(){
  var id,raw []byte
  if err:=rows.Scan(&id,&raw);err!=nil{rows.Close();return zero,err}
  if len(id)!=32{rows.Close();return zero,errors.New("malformed perception outbox row ID")}
  parsed,err:=codec.StrictDecodePayload(raw);if err!=nil{rows.Close();return zero,err}
  f,ok:=parsed.([]any);if !ok||len(f)==0||!equalBytes(f[0],id){rows.Close();return zero,errors.New("outbox row ID mismatch")}
  switch len(f){
  case 10:
   taskRun,ok:=idField(f[1]);if !ok{rows.Close();return zero,errors.New("perception task missing run")}
   if taskRun!=run{continue}
   event,ok:=idField(f[3]);if !ok{rows.Close();return zero,errors.New("perception task missing event")}
   var key [32]byte;copy(key[:],id)
   if _,exists:=tasks[key];exists{rows.Close();return zero,errors.New("duplicate perception task")}
   tasks[key]=item{id:key,event:event,run:taskRun}
  case 5:
   task,ok:=idField(f[1]);if !ok{rows.Close();return zero,errors.New("completion missing task")}
   complete[task]=true
  default:
   rows.Close();return zero,errors.New("unregistered perception outbox type")
  }
 }
 if err:=rows.Err();err!=nil{rows.Close();return zero,err}
 if err:=rows.Close();err!=nil{return zero,err}
 keys:=make([][32]byte,0,len(tasks))
 for id:=range tasks{keys=append(keys,id)}
 sort.Slice(keys,func(i,j int)bool{return bytes.Compare(keys[i][:],keys[j][:])<0})
 for _,id:=range keys{
  if complete[id]{continue}
  task:=tasks[id]
  var raw []byte
  if err:=s.db.QueryRowContext(ctx,"SELECT canonical FROM event_store WHERE id=?",task.event[:]).Scan(&raw);err!=nil{
   return zero,fmt.Errorf("pending perception event missing: %w",err)
  }
  event,err:=codec.StrictDecodePayload(raw);if err!=nil{return zero,err}
  fields,ok:=event.([]any)
  if !ok||len(fields)<4{return zero,errors.New("perception source event malformed")}
  eventType,ok:=fields[3].(string);if !ok{return zero,errors.New("perception event type malformed")}
  switch eventType{
  case "round.consumed":
   if _,err:=s.DeliverRoundPerception(ctx,id,b);err!=nil{return zero,fmt.Errorf("round perception %x: %w",id,err)}
  case "communication.delivered":
   if _,err:=s.DeliverCommunicationPerception(ctx,id);err!=nil{return zero,fmt.Errorf("communication perception %x: %w",id,err)}
  default:
   // CompleteInternalPerception independently rejects events whose policy
   // demands an addressed observation: unknown visibility never defaults
   // to "private claim" or silently creates world truth.
   if _,err:=s.CompleteInternalPerception(ctx,id,b);err!=nil{return zero,fmt.Errorf("internal perception %x: %w",id,err)}
  }
 }
 after,err:=s.VerifySnapshotV1(ctx,run,snapshotID,b)
 if err!=nil{return zero,fmt.Errorf("post-drain replay and integrity: %w",err)}
 if after.PendingTasks!=0{return zero,errors.New("perception barrier not yet clear; concurrent work may have arrived")}
 if err:=s.CheckPerceptionBarrier(ctx,run,uint64(after.Clock.Revision));err!=nil{return zero,err}
 return after,nil
}

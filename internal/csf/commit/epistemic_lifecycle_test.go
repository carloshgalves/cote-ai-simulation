package commit

import (
 "context"
 "database/sql"
 "testing"

 _ "modernc.org/sqlite"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
)

// V1-5 already publishes these lifecycle Events. Each ordinal-0 event
// receives a PerceptionTask even though no actor may see the engine transition.
// Without a registered *empty* resolver, the next guarded dispatch deadlocks.
func TestCSFRV16ScheduledAndTriggerLifecycleCompleteWithoutDisclosure(t *testing.T){
 cases:=[]struct{fixture,wantEvent string}{
  {"reject","occurrence.rejected"},
  {"defer","occurrence.deferred"},
  {"trigger","trigger.activation.consumed"},
 }
 for _,tc:=range cases{
  t.Run(tc.fixture,func(t *testing.T){
   ctx:=context.Background()
   c,store,ledger,cycle,fence,path:=systemWorkFixture(t,tc.fixture)
   if _,err:=c.CommitSystemUnit(ctx,ledger,cycle,fence);err!=nil{t.Fatal(err)}
   _,_,_,_,_,run:=ledger.Snapshot()
   db,err:=sql.Open("sqlite","file:"+path);if err!=nil{t.Fatal(err)}
   defer db.Close()
   rows,err:=db.QueryContext(ctx,"SELECT id,canonical FROM causal_outbox")
   if err!=nil{t.Fatal(err)}
   count:=0
   for rows.Next(){
    var id,raw []byte
    if err:=rows.Scan(&id,&raw);err!=nil{t.Fatal(err)}
    decoded,err:=codec.StrictDecodePayload(raw);if err!=nil{t.Fatal(err)}
    fields,ok:=decoded.([]any)
    if !ok||len(fields)!=10{t.Fatal("unexpected outbox root before perception")}
    var eventRaw []byte
    if err:=db.QueryRowContext(ctx,"SELECT canonical FROM event_store WHERE id=?",fields[3]).Scan(&eventRaw);err!=nil{t.Fatal(err)}
    event,err:=codec.StrictDecodePayload(eventRaw);if err!=nil{t.Fatal(err)}
    if event.([]any)[3]!=tc.wantEvent{t.Fatalf("wrong lifecycle Event %v",event.([]any)[3])}
    var task [32]byte;copy(task[:],id)
    if _,err:=store.CompleteInternalPerception(ctx,task,c.bundle);err!=nil{
     t.Fatal("registered internal lifecycle task has no empty resolver:",err)
    }
    if _,err:=store.DeliverRoundPerception(ctx,task,c.bundle);err==nil{
     t.Fatal("private system transition was exposed as a round observation")
    }
    count++
   }
   if err:=rows.Err();err!=nil{t.Fatal(err)}
   rows.Close()
   if count!=1{t.Fatalf("expected one internal lifecycle perception task, got %d",count)}
   if err:=store.CheckPerceptionBarrier(ctx,[32]byte(run),1);err!=nil{
    t.Fatal("internal completion did not release barrier:",err)
   }
   var n int
   if err:=db.QueryRowContext(ctx,"SELECT COUNT(*) FROM evidence_ledger").Scan(&n);err!=nil{t.Fatal(err)}
   if n!=0{t.Fatal("secret lifecycle produced addressed evidence")}
   reopened,err:=sqlite.Open(ctx,path);if err!=nil{t.Fatal(err)}
   defer reopened.Close()
   report,err:=reopened.ReplayClock(ctx,[32]byte(run),c.bundle)
   if err!=nil||report.ReplayedCommits!=1{
    t.Fatalf("internal lifecycle failed replay: %+v %v",report,err)
   }
  })
 }
}


func TestCSFRV16TemporalTriggerEventsArePrivateAndCompletionReplayable(t *testing.T){
 ctx:=context.Background()
 definition:=activeTrigger()
 c,store,ledger,run,bundle,path:=testFixture(t,definition)
 output,err:=c.Advance(ctx,ledger,run);if err!=nil{t.Fatal(err)}
 if len(output.Events)!=4{t.Fatalf("expected CLOCK/BODY plus two trigger lifecycle Events, got %d",len(output.Events))}
 db,err:=sql.Open("sqlite","file:"+path);if err!=nil{t.Fatal(err)}
 defer db.Close()
 rows,err:=db.QueryContext(ctx,"SELECT id,canonical FROM causal_outbox")
 if err!=nil{t.Fatal(err)}
 found:=map[string]bool{}
 for rows.Next(){
  var id,raw []byte
  if err:=rows.Scan(&id,&raw);err!=nil{t.Fatal(err)}
  parsed,err:=codec.StrictDecodePayload(raw);if err!=nil{t.Fatal(err)}
  task,ok:=parsed.([]any)
  if !ok||len(task)!=10{t.Fatal("unexpected temporal outbox root")}
  var eventRaw []byte
  if err:=db.QueryRowContext(ctx,"SELECT canonical FROM event_store WHERE id=?",task[3]).Scan(&eventRaw);err!=nil{t.Fatal(err)}
  event,err:=codec.StrictDecodePayload(eventRaw);if err!=nil{t.Fatal(err)}
  kind,ok:=event.([]any)[3].(string);if !ok{t.Fatal("missing trigger event kind")}
  if found[kind]{t.Fatal("duplicate temporal perception producer:",kind)}
  found[kind]=true
  var taskID [32]byte;copy(taskID[:],id)
  if _,err:=store.CompleteInternalPerception(ctx,taskID,bundle);err!=nil{
   t.Fatalf("registered internal producer %s could not terminate: %v",kind,err)
  }
 }
 if err:=rows.Err();err!=nil{t.Fatal(err)}
 rows.Close()
 for _,name:=range []string{"clock.advanced","trigger.runtime.transition","trigger.activation.created"}{
  if !found[name]{t.Fatal("missing registered private temporal task:",name)}
 }
 if found["body.advanced"]{t.Fatal("BODY local ordinal 1 cannot produce a V1-6 task")}
 if err:=store.CheckPerceptionBarrier(ctx,[32]byte(run),1);err!=nil{
  t.Fatal("private temporal tasks were not all completed:",err)
 }
 var n int
 if err:=db.QueryRowContext(ctx,"SELECT COUNT(*) FROM evidence_ledger").Scan(&n);err!=nil{t.Fatal(err)}
 if n!=0{t.Fatal("private temporal transition leaked knowledge to an actor")}
 reopened,err:=sqlite.Open(ctx,path);if err!=nil{t.Fatal(err)}
 defer reopened.Close()
 report,err:=reopened.ReplayClock(ctx,[32]byte(run),bundle)
 if err!=nil||report.ReplayedCommits!=1||report.ReplayedEvents!=4{
  t.Fatalf("temporal trigger completion not replayable: %+v %v",report,err)
 }
}

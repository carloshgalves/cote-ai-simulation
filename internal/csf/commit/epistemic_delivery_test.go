package commit

import (
 "bytes"
 "context"
 "database/sql"
 "sync"
 "testing"

 _ "modernc.org/sqlite"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/perception"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
)

func TestCSFRV16ResourceSecretAndAddressedRoundEndToEnd(t *testing.T){
 ctx:=context.Background()
 c,store,ledger,cycle,fence,path:=singleResourceFixture(t)
 if _,err:=c.CommitSingleResource(ctx,ledger,cycle,fence);err!=nil{t.Fatal(err)}
 _,_,_,_,_,run:=ledger.Snapshot()
 reader,err:=sql.Open("sqlite","file:"+path);if err!=nil{t.Fatal(err)}
 rows,err:=reader.QueryContext(ctx,"SELECT id,canonical FROM causal_outbox ORDER BY id")
 if err!=nil{t.Fatal(err)}
 var internalTask,addressedTask [32]byte
 var privateSource []byte
 var occurred any
 for rows.Next(){
  var id,raw []byte
  if err:=rows.Scan(&id,&raw);err!=nil{t.Fatal(err)}
  parsed,err:=codec.StrictDecodePayload(raw);if err!=nil{t.Fatal(err)}
  task:=parsed.([]any)
  var eventRaw []byte
  if err:=reader.QueryRowContext(ctx,"SELECT canonical FROM event_store WHERE id=?",
   task[3]).Scan(&eventRaw);err!=nil{t.Fatal(err)}
  parsed,err=codec.StrictDecodePayload(eventRaw);if err!=nil{t.Fatal(err)}
  ev:=parsed.([]any)
  switch ev[3]{
  case "round.consumed":
   copy(addressedTask[:],id);occurred=ev[5]
   source:=ev[11].([]any)[0].([]any)
   privateSource=bytes.Clone(source[2].([]byte))
  case "resource.consumed":copy(internalTask[:],id)
  default:t.Fatal("unexpected event type in perception outbox")
  }
 }
 if err:=rows.Err();err!=nil{t.Fatal(err)}
 rows.Close();reader.Close()
 if addressedTask==([32]byte{})||internalTask==([32]byte{}){
  t.Fatal("mixed cycle lacks addressed round and secret resource tasks")
 }
 if _,err:=store.CompleteInternalPerception(ctx,addressedTask,c.bundle);err==nil{
  t.Fatal("internal empty resolver erased legitimate addressed evidence")
 }
 if _,err:=store.DeliverRoundPerception(ctx,internalTask);err==nil{
  t.Fatal("secret resource event delivered as actor knowledge")
 }
 if _,err:=store.CompleteInternalPerception(ctx,internalTask,c.bundle);err!=nil{
  t.Fatal("invisible source must complete empty",err)
 }
 var workers sync.WaitGroup
 results:=make(chan perception.RoundProjection,2)
 errs:=make(chan error,2)
 for i:=0;i<2;i++{
  workers.Add(1)
  go func(){
   defer workers.Done()
   p,err:=store.DeliverRoundPerception(ctx,addressedTask)
   results<-p;errs<-err
  }()
 }
 workers.Wait();close(results);close(errs)
 for err:=range errs{if err!=nil{t.Fatal("concurrent addressed delivery:",err)}}
 var first *perception.RoundProjection
 for p:=range results{
  if first==nil{copyP:=p;first=&copyP;continue}
  if !bytes.Equal(first.Observation.Canonical,p.Observation.Canonical)||
   !bytes.Equal(first.Input.Canonical,p.Input.Canonical)||
   !bytes.Equal(first.Completion.Canonical,p.Completion.Canonical){
   t.Fatal("worker permutation changed canonical addressed evidence")
  }
 }
 if first==nil||first.Actor!=([2]string{"school","a"}){t.Fatal("invalid recipient policy")}
 obs,inputs,err:=store.ReadAddressedEvidence(ctx,[2]string{"school","a"})
 if err!=nil||len(obs)!=1||len(inputs)!=1{t.Fatal("A must receive exactly one observation/input",err,len(obs),len(inputs))}
 bObs,bInputs,err:=store.ReadAddressedEvidence(ctx,[2]string{"school","b"})
 if err!=nil||len(bObs)!=0||len(bInputs)!=0{t.Fatal("B must never see A's evidence",err)}
 if bytes.Contains(obs[0].Canonical,privateSource)||bytes.Contains(inputs[0].Canonical,privateSource){
  t.Fatal("private response source leaked into A's redacted evidence")
 }
 observed,err:=codec.StrictDecodePayload(obs[0].Canonical);if err!=nil{t.Fatal(err)}
 received,err:=codec.StrictDecodePayload(inputs[0].Canonical);if err!=nil{t.Fatal(err)}
 if !sameTestCanonical(observed.([]any)[5],received.([]any)[4])||
  !sameTestCanonical(observed.([]any)[5],occurred){
  t.Fatal("worker wall-time changed epistemic timestamp")
 }
 reopened,err:=sqlite.Open(ctx,path);if err!=nil{t.Fatal(err)}
 defer reopened.Close()
 report,err:=reopened.ReplayClock(ctx,[32]byte(run),c.bundle)
 if err!=nil||report.ReplayedCommits!=1||report.ReconstructedClock.Revision!=1{
  t.Fatalf("addressed/secret task completion failed causal replay: %+v %v",report,err)
 }
 a2,i2,err:=reopened.ReadAddressedEvidence(ctx,[2]string{"school","a"})
 if err!=nil||len(a2)!=1||len(i2)!=1||!bytes.Equal(a2[0].Canonical,obs[0].Canonical){
  t.Fatal("reopen did not preserve addressed bytes",err)
 }
}
func sameTestCanonical(a,b any)bool{
 ra,ea:=codec.CanonicalPayload(a);rb,eb:=codec.CanonicalPayload(b)
 return ea==nil&&eb==nil&&bytes.Equal(ra,rb)
}

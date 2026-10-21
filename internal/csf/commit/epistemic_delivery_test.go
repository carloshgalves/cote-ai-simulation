package commit

import (
 "bytes"
 "context"
 "database/sql"
 "sync"
 "testing"

 _ "modernc.org/sqlite"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
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
 for rows.Next(){
  var id,raw []byte
  if err:=rows.Scan(&id,&raw);err!=nil{t.Fatal(err)}
  fields,err:=codec.StrictDecodePayload(raw);if err!=nil{t.Fatal(err)}
  task:=fields.([]any)
  var eventRaw []byte
  if err:=reader.QueryRowContext(ctx,"SELECT canonical FROM event_store WHERE id=?",
   task[3]).Scan(&eventRaw);err!=nil{t.Fatal(err)}
  parsed,err:=codec.StrictDecodePayload(eventRaw);if err!=nil{t.Fatal(err)}
  ev:=parsed.([]any)
  switch ev[3]{
  case "round.consumed":copy(addressedTask[:],id)
  case "resource.consumed":copy(internalTask[:],id)
  default:t.Fatal("unexpected event type in perception outbox")
  }
 }
 if err:=rows.Err();err!=nil{t.Fatal(err)}
 rows.Close();reader.Close()
 if addressedTask==([32]byte{})||internalTask==([32]byte{}){
  t.Fatal("mixed cycle lacks one addressed and one secret task")
 }
 if _,err:=store.CompleteInternalPerception(ctx,addressedTask,c.bundle);err==nil{
  t.Fatal("secret-default internal resolver erased legitimate addressed observation")
 }
 if _,err:=store.DeliverRoundPerception(ctx,internalTask);err==nil{
  t.Fatal("secret resource event was delivered as a round notice")
 }
 if _,err:=store.CompleteInternalPerception(ctx,internalTask,c.bundle);err!=nil{
  t.Fatal("invisible source must complete empty",err)
 }
 for step:=uint8(1);step<=3;step++{
  if _,err:=store.DeliverRoundPerception(ctx,addressedTask);err==nil{
   t.Fatal("expected manual delivery?") // overwritten by next loop helper
  }
  _=step
 }
 _=run
 _=sql.ErrNoRows
 _=sqlite.Store{}
 _=bytes.Equal
 _=sync.WaitGroup{}
}

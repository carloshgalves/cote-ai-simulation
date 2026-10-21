package commit

import (
 "context"
 "database/sql"
 "testing"

 _ "modernc.org/sqlite"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
)

func TestCSFRV16ExistingDispatchRetryRechecksEpistemicBarrier(t *testing.T){
 ctx:=context.Background()
 c,store,ledger,cycle,fence,path:=singleResourceFixture(t)
 if _,err:=c.CommitSingleResource(ctx,ledger,cycle,fence);err!=nil{t.Fatal(err)}
 db,err:=sql.Open("sqlite","file:"+path)
 if err!=nil{t.Fatal(err)}
 defer db.Close()
 rows,err:=db.QueryContext(ctx,"SELECT id,canonical FROM causal_outbox")
 if err!=nil{t.Fatal(err)}
 var secret,visible [32]byte
 for rows.Next(){
  var id,raw []byte
  if err:=rows.Scan(&id,&raw);err!=nil{t.Fatal(err)}
  decoded,err:=codec.StrictDecodePayload(raw);if err!=nil{t.Fatal(err)}
  f,ok:=decoded.([]any);if !ok||len(f)!=10{t.Fatal("unexpected outbox entry")}
  var eventRaw []byte
  if err:=db.QueryRowContext(ctx,"SELECT canonical FROM event_store WHERE id=?",f[3]).Scan(&eventRaw);err!=nil{t.Fatal(err)}
  event,err:=codec.StrictDecodePayload(eventRaw);if err!=nil{t.Fatal(err)}
  switch event.([]any)[3]{
  case "resource.consumed":copy(secret[:],id)
  case "round.consumed":copy(visible[:],id)
  default:t.Fatal("unexpected perception task type")
  }
 }
 if err:=rows.Err();err!=nil{t.Fatal(err)}
 rows.Close()
 if secret==([32]byte{})||visible==([32]byte{}){t.Fatal("missing mixed perceptible cycle")}
 if _,err:=store.CompleteInternalPerception(ctx,secret,c.bundle);err!=nil{t.Fatal(err)}
 p,err:=store.DeliverRoundPerception(ctx,visible,c.bundle);if err!=nil{t.Fatal(err)}
 _=resourceActorInbox(t,ctx,store,path,ledger,c.bundle) // publishes next real and guarded dispatch

 rows,err=db.QueryContext(ctx,"SELECT id,canonical FROM input_ledger")
 if err!=nil{t.Fatal(err)}
 var retry ports.CanonicalRecord
 for rows.Next(){
  var id,raw []byte
  if err:=rows.Scan(&id,&raw);err!=nil{t.Fatal(err)}
  decoded,err:=codec.StrictDecodePayload(raw);if err!=nil{continue}
  f,ok:=decoded.([]any)
  if !ok||len(f)!=11||f[4]!="act"||f[8]!=uint64(1){continue}
  var key [32]byte;copy(key[:],id)
  retry=ports.CanonicalRecord{Identity:key,Canonical:raw}
 }
 if err:=rows.Err();err!=nil{t.Fatal(err)}
 rows.Close()
 if len(retry.Canonical)==0{t.Fatal("missing next revision's guarded dispatch")}
 if err:=store.Append(ctx,retry);err!=nil{t.Fatal("unmodified exact retry must be idempotent",err)}
 if _,err:=db.ExecContext(ctx,"DELETE FROM causal_outbox WHERE id=?",p.Completion.Identity[:]);err!=nil{t.Fatal(err)}
 if err:=store.Append(ctx,retry);err==nil{
  t.Fatal("exact dispatch retry escaped missing prior addressed completion")
 }
}

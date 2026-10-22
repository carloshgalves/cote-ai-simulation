package commit

import (
 "bytes"
 "context"
 "database/sql"
 "sync"
 "testing"

 _ "modernc.org/sqlite"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
)

// Property-style exhaustive permutations for the only registered V1-6
// positive route. Worker scheduling, order of secret/visible completion and
// delivery retries must not alter any canonical EvidenceLedger/outbox bytes.
func TestCSFRV16DeliveryPermutationsPreserveCanonicalRoots(t *testing.T) {
 scenarios:=[]struct{name string; secretFirst bool; workers int}{
  {"visible-then-secret-single",false,1},
  {"secret-then-visible-single",true,1},
  {"visible-then-secret-eight-workers",false,8},
  {"secret-then-visible-eight-workers",true,8},
  {"visible-then-secret-two-workers",false,2},
  {"secret-then-visible-two-workers",true,2},
 }
 var baseline []byte
 for _,tc:=range scenarios {
  t.Run(tc.name,func(t *testing.T){
   ctx:=context.Background()
   c,store,ledger,cycle,fence,path:=singleResourceFixture(t)
   if _,err:=c.CommitSingleResource(ctx,ledger,cycle,fence);err!=nil{t.Fatal(err)}
   db,err:=sql.Open("sqlite","file:"+path)
   if err!=nil{t.Fatal(err)}
   defer db.Close()
   tasks,err:=db.QueryContext(ctx,"SELECT id,canonical FROM causal_outbox ORDER BY id")
   if err!=nil{t.Fatal(err)}
   var visible,secret [32]byte
   for tasks.Next(){
    var id,raw []byte
    if err:=tasks.Scan(&id,&raw);err!=nil{t.Fatal(err)}
    parsed,err:=codec.StrictDecodePayload(raw);if err!=nil{t.Fatal(err)}
    fields,ok:=parsed.([]any);if !ok||len(fields)!=10{t.Fatal("unexpected outbox root")}
    var eventRaw []byte
    if err:=db.QueryRowContext(ctx,"SELECT canonical FROM event_store WHERE id=?",fields[3]).Scan(&eventRaw);err!=nil{t.Fatal(err)}
    parsed,err=codec.StrictDecodePayload(eventRaw);if err!=nil{t.Fatal(err)}
    event:=parsed.([]any)
    switch event[3]{
    case "round.consumed":copy(visible[:],id)
    case "resource.consumed":copy(secret[:],id)
    default:t.Fatalf("unknown event %v",event[3])
    }
   }
   if err:=tasks.Err();err!=nil{t.Fatal(err)}
   tasks.Close()
   if visible==([32]byte{})||secret==([32]byte{}){t.Fatal("permutation fixture lacks both event types")}
   deliverVisible:=func(){
    var wg sync.WaitGroup
    errc:=make(chan error,tc.workers)
    for i:=0;i<tc.workers;i++{
     wg.Add(1)
     go func(){
      defer wg.Done()
      _,err:=store.DeliverRoundPerception(ctx,visible,c.bundle)
      errc<-err
     }()
    }
    wg.Wait();close(errc)
    for err:=range errc{if err!=nil{t.Error(err)}}
   }
   deliverSecret:=func(){
    if _,err:=store.CompleteInternalPerception(ctx,secret,c.bundle);err!=nil{t.Fatal(err)}
    if _,err:=store.CompleteInternalPerception(ctx,secret,c.bundle);err!=nil{t.Fatal("secret retry changed output:",err)}
   }
   if tc.secretFirst{deliverSecret();deliverVisible()}else{deliverVisible();deliverSecret()}
   _,_,_,_,_,run:=ledger.Snapshot()
   if err:=store.CheckPerceptionBarrier(ctx,[32]byte(run),1);err!=nil{t.Fatal("permutation left incomplete causal receipts:",err)}
   var observed []byte
   for _,query:=range []string{
    "SELECT id,canonical FROM evidence_ledger ORDER BY id",
    "SELECT id,canonical FROM causal_outbox ORDER BY id",
    "SELECT record_id,canonical_hash FROM evidence_routes ORDER BY record_id",
   }{
    rows,err:=db.QueryContext(ctx,query);if err!=nil{t.Fatal(err)}
    for rows.Next(){
     var id,raw []byte
     if err:=rows.Scan(&id,&raw);err!=nil{t.Fatal(err)}
     observed=append(observed,id...)
     observed=append(observed,raw...)
    }
    if err:=rows.Err();err!=nil{t.Fatal(err)}
    rows.Close()
   }
   if baseline==nil{baseline=bytes.Clone(observed)}
   if !bytes.Equal(baseline,observed){t.Fatal("permuted workers/retries changed IDs or canonical bytes")}
  })
 }
}

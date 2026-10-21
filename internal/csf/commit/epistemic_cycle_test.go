package commit

import (
 "context"
 "database/sql"
 "testing"

 _ "modernc.org/sqlite"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
)

func TestCSFRV16StorageRejectsForgedCycleForAuthorizedRound(t *testing.T){
 ctx:=context.Background()
 c,store,ledger,cycle,fence,path:=singleResourceFixture(t)
 if _,err:=c.CommitSingleResource(ctx,ledger,cycle,fence);err!=nil{t.Fatal(err)}
 db,err:=sql.Open("sqlite","file:"+path);if err!=nil{t.Fatal(err)}
 defer db.Close()
 rows,err:=db.QueryContext(ctx,"SELECT id,canonical FROM causal_outbox")
 if err!=nil{t.Fatal(err)}
 var secret,visible [32]byte
 for rows.Next(){
  var id,raw []byte
  if err:=rows.Scan(&id,&raw);err!=nil{t.Fatal(err)}
  value,err:=codec.StrictDecodePayload(raw);if err!=nil{t.Fatal(err)}
  task,ok:=value.([]any);if !ok||len(task)!=10{t.Fatal("unknown outbox root")}
  var eventRaw []byte
  if err:=db.QueryRowContext(ctx,"SELECT canonical FROM event_store WHERE id=?",task[3]).Scan(&eventRaw);err!=nil{t.Fatal(err)}
  parsed,err:=codec.StrictDecodePayload(eventRaw);if err!=nil{t.Fatal(err)}
  switch parsed.([]any)[3]{
  case "resource.consumed":copy(secret[:],id)
  case "round.consumed":copy(visible[:],id)
  default:t.Fatal("unregistered event")
  }
 }
 if err:=rows.Err();err!=nil{t.Fatal(err)}
 rows.Close()
 if _,err:=store.CompleteInternalPerception(ctx,secret,c.bundle);err!=nil{t.Fatal(err)}
 if _,err:=store.DeliverRoundPerception(ctx,visible,c.bundle);err!=nil{t.Fatal(err)}
 _,_,_,_,_,run:=ledger.Snapshot()
 state,err:=store.LoadClock(ctx,[32]byte(run));if err!=nil{t.Fatal(err)}
 if state.LastOrdinal<0{t.Fatal("fixture did not commit first cycle")}
 ordinal:=uint64(state.LastOrdinal+1)
 genesis:=validGenesis(t,c.bundle)
 origin:=[]any{uint64(2),[]any{genesis.ID[:],genesis.Digest[:]}}
 coordinate:=domain.EligibilityCoordinate{
  NotBeforeInstant:state.Instant,
  NotBeforeCycleOrdinal:domain.CycleOrdinal(ordinal),
 }
 round,err:=ledger.DeclareRound(ctx,"schedule",[2]string{"school","a"},[]string{"act"},
  coordinate,origin,0,1,0,[]any{uint64(0)})
 if err!=nil{t.Fatal("cannot seal next legitimate round:",err)}
 deriver:=identity.New(c.bundle)
 actualCycle,_,err:=deriver.Sum("cote.csf.id.cycle",
  "cote.csf.schema.cycle-id-preimage",1,
  []any{run[:],[]any{int64(state.Instant),ordinal}})
 if err!=nil{t.Fatal(err)}
 var fakeCycle [32]byte
 fakeCycle[0]=0xaa
 newDispatch:=func(cycleID [32]byte)ports.CanonicalRecord{
  t.Helper()
  id,_,err:=deriver.Sum("cote.csf.id.dispatch",
   "cote.csf.schema.dispatch-id-preimage",1,
   []any{run[:],cycleID[:],round.ID[:],"act",uint64(0)})
  if err!=nil{t.Fatal(err)}
  body:=[]any{id[:],run[:],cycleID[:],round.ID[:],"act",uint64(0),
   []any{"school","a"},[]any{int64(state.Instant),ordinal},
   uint64(state.Revision),int64(state.Instant),make([]byte,32)}
  raw,err:=codec.CanonicalPayload(body);if err!=nil{t.Fatal(err)}
  return ports.CanonicalRecord{Identity:[32]byte(id),Canonical:raw}
 }
 if err:=store.Append(ctx,newDispatch(fakeCycle));err==nil{
  t.Fatal("forged CycleID bypassed durable dispatch authority")
 }
 if err:=store.Append(ctx,newDispatch([32]byte(actualCycle)));err!=nil{
  t.Fatal("valid, causally-next SlotDispatch rejected by authority:",err)
 }
}

package sqlite

import (
 "context"
 "testing"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
)

func TestDispatchAppendRequiresDurablePriorPerception(t *testing.T){
 ctx:=context.Background()
 s,err:=Open(ctx,t.TempDir()+"/barrier.db")
 if err!=nil{t.Fatal(err)}
 defer s.Close()
 var run,cycle,task,event,commit [32]byte
 run[0],cycle[0],task[0],event[0],commit[0]=1,2,3,4,5
 // This fixture creates a committed roster, not a fabricated event observer.
 journal:=make([]any,24)
 for i:=range journal{journal[i]=[]any{}}
 journal[0],journal[1],journal[8],journal[18]=run[:],cycle[:],uint64(1),[]any{task[:]}
 journalRaw,err:=codec.CanonicalPayload(journal)
 if err!=nil{t.Fatal(err)}
 if _,err:=s.db.ExecContext(ctx,
   "INSERT INTO decision_ledger(id,canonical) VALUES(?,?)",commit[:],journalRaw);err!=nil{t.Fatal(err)}
 if _,err:=s.db.ExecContext(ctx,
   "INSERT INTO committed_cycles(run_id,cycle_id,commit_id) VALUES(?,?,?)",
   run[:],cycle[:],commit[:]);err!=nil{t.Fatal(err)}
 var targetCycle,round [32]byte
 targetCycle[0],round[0]=6,7
 fields:=[]any{[]byte{},run[:],targetCycle[:],round[:],"slot",uint64(0),
  []any{"school","agent-a"},[]any{int64(5),uint64(0)},uint64(1),int64(5),make([]byte,32)}
 did,err:=registeredDigest("cote.csf.id.dispatch","cote.csf.schema.dispatch-id-preimage",
  []any{fields[1],fields[2],fields[3],fields[4],fields[5]})
 if err!=nil{t.Fatal(err)}
 fields[0]=did[:]
 dispatchRaw,err:=codec.CanonicalPayload(fields)
 if err!=nil{t.Fatal(err)}
 dispatch:=portsRecord(did[:],dispatchRaw)
 if err:=s.Append(ctx,dispatch);err==nil{t.Fatal("missing committed task released dispatch")}
 var count int
 if err:=s.db.QueryRow("SELECT COUNT(*) FROM input_ledger WHERE id=?",did[:]).Scan(&count);err!=nil||count!=0{
  t.Fatal("blocked dispatch was persisted",err,count)
 }
 taskFields:=[]any{task[:],run[:],cycle[:],event[:],uint64(0),uint64(1),
  []any{"perception-v1",uint64(1),make([]byte,32)},
  []any{"validators-v1",uint64(1),make([]byte,32)},
  []any{"perception-identity-v1",uint64(1),make([]byte,32)},uint64(0)}
 taskRaw,err:=codec.CanonicalPayload(taskFields)
 if err!=nil{t.Fatal(err)}
 if _,err:=s.db.ExecContext(ctx,"INSERT INTO causal_outbox(id,canonical) VALUES(?,?)",
  task[:],taskRaw);err!=nil{t.Fatal(err)}
 if err:=s.Append(ctx,dispatch);err==nil{t.Fatal("pending perception task released dispatch")}
 cid,err:=registeredDigest("cote.csf.id.perception-completion",
  "cote.csf.schema.perception-completion-id-preimage",[]any{task[:]})
 if err!=nil{t.Fatal(err)}
 body:=[]any{cid[:],task[:],[]any{},[]any{}}
 digest,err:=registeredDigest("cote.csf.digest.perception-completion",
  "cote.csf.schema.perception-task-completion.body",body)
 if err!=nil{t.Fatal(err)}
 canonical,err:=codec.CanonicalPayload(append(body,digest[:]))
 if err!=nil{t.Fatal(err)}
 if _,err:=s.db.ExecContext(ctx,"INSERT INTO causal_outbox(id,canonical) VALUES(?,?)",
  cid[:],canonical);err!=nil{t.Fatal(err)}
 if err:=s.Append(ctx,dispatch);err!=nil{t.Fatal("durable completed task must allow dispatch",err)}
 if err:=s.Append(ctx,dispatch);err!=nil{t.Fatal("same dispatch bytes should be idempotent",err)}
 if err:=s.db.QueryRow("SELECT COUNT(*) FROM input_ledger WHERE id=?",did[:]).Scan(&count);err!=nil||count!=1{
  t.Fatal("dispatch not persisted exactly once",err,count)
 }
 if _,err:=s.db.ExecContext(ctx,"UPDATE causal_outbox SET canonical=? WHERE id=?",
  []byte{0x80},cid[:]);err!=nil{t.Fatal(err)}
 var next [32]byte;next[0]=9
 fields[2]=next[:]
 derived,err:=registeredDigest("cote.csf.id.dispatch","cote.csf.schema.dispatch-id-preimage",
   []any{fields[1],fields[2],fields[3],fields[4],fields[5]})
 if err!=nil{t.Fatal(err)}
 fields[0]=derived[:]
 nextRaw,err:=codec.CanonicalPayload(fields);if err!=nil{t.Fatal(err)}
 if err:=s.Append(ctx,portsRecord(derived[:],nextRaw));err==nil{
  t.Fatal("corrupted durable completion allowed another dispatch")
 }
}

func portsRecord(id []byte,raw []byte) ports.CanonicalRecord{
 var key [32]byte;copy(key[:],id)
 return ports.CanonicalRecord{Identity:key,Canonical:raw}
}

package commit

import (
 "bytes"
 "context"
 "database/sql"
 "sync"
 "testing"

 _ "modernc.org/sqlite"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/contextboundary"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
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
 if _,err:=store.DeliverRoundPerception(ctx,internalTask,c.bundle);err==nil{
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
   p,err:=store.DeliverRoundPerception(ctx,addressedTask,c.bundle)
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
 actorInbox:=resourceActorInbox(t,ctx,store,path)
 obs,inputs,err:=actorInbox.ReadAddressedEvidence(ctx,[2]string{"school","a"})
 if err!=nil||len(obs)!=1||len(inputs)!=1{t.Fatal("A must receive exactly one observation/input",err,len(obs),len(inputs))}
 if _,_,err:=actorInbox.ReadAddressedEvidence(ctx,[2]string{"school","b"});err==nil{
  t.Fatal("B accessed A's capability/partition")
 }
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
 reopenedInbox:=resourceActorInbox(t,ctx,reopened,path)
 a2,i2,err:=reopenedInbox.ReadAddressedEvidence(ctx,[2]string{"school","a"})
 if err!=nil||len(a2)!=1||len(i2)!=1||!bytes.Equal(a2[0].Canonical,obs[0].Canonical){
  t.Fatal("reopen did not preserve addressed bytes",err)
 }
 viewA,err:=contextboundary.Build(ctx,reopenedInbox,[2]string{"school","a"})
 if err!=nil||len(viewA.Inbox)!=1{
  t.Fatal("allow-list context lost addressed evidence",err)
 }
 _,err=contextboundary.Build(ctx,reopenedInbox,[2]string{"school","b"})
 if err==nil{t.Fatal("B obtained a context through A's scoped inbox")}
}
func sameTestCanonical(a,b any)bool{
 ra,ea:=codec.CanonicalPayload(a);rb,eb:=codec.CanonicalPayload(b)
 return ea==nil&&eb==nil&&bytes.Equal(ra,rb)
}


func findTerminalRoundTask(t *testing.T,ctx context.Context,db *sql.DB)[32]byte{
 t.Helper()
 rows,err:=db.QueryContext(ctx,"SELECT id,canonical FROM causal_outbox")
 if err!=nil{t.Fatal(err)}
 defer rows.Close()
 for rows.Next(){
  var id,raw []byte
  if err:=rows.Scan(&id,&raw);err!=nil{t.Fatal(err)}
  v,err:=codec.StrictDecodePayload(raw);if err!=nil{t.Fatal(err)}
  f,ok:=v.([]any)
  if !ok||len(f)!=10{continue}
  var eventRaw []byte
  if err:=db.QueryRowContext(ctx,"SELECT canonical FROM event_store WHERE id=?",f[3]).Scan(&eventRaw);err!=nil{t.Fatal(err)}
  value,err:=codec.StrictDecodePayload(eventRaw);if err!=nil{t.Fatal(err)}
  if value.([]any)[3]=="round.consumed"{
   var task [32]byte;copy(task[:],id);return task
  }
 }
 if err:=rows.Err();err!=nil{t.Fatal(err)}
 t.Fatal("missing addressed round task")
 return [32]byte{}
}

func TestCSFRV16DeliveryFaultsRollbackEveryReceipt(t *testing.T){
 ctx:=context.Background()
 c,store,ledger,cycle,fence,path:=singleResourceFixture(t)
 if _,err:=c.CommitSingleResource(ctx,ledger,cycle,fence);err!=nil{t.Fatal(err)}
 db,err:=sql.Open("sqlite","file:"+path);if err!=nil{t.Fatal(err)}
 defer db.Close()
 task:=findTerminalRoundTask(t,ctx,db)
 for _,scenario:=range []struct{name,trigger string}{
  {"after-observation",
   "CREATE TRIGGER block_evidence_route BEFORE INSERT ON evidence_routes WHEN NEW.kind=0 BEGIN SELECT RAISE(ABORT,'fault-observation'); END"},
  {"after-input",
   "CREATE TRIGGER block_evidence_route BEFORE INSERT ON evidence_routes WHEN NEW.kind=1 BEGIN SELECT RAISE(ABORT,'fault-input'); END"},
  {"before-completion",
   "CREATE TRIGGER block_evidence_route BEFORE INSERT ON causal_outbox BEGIN SELECT RAISE(ABORT,'fault-completion'); END"},
 }{
  if _,err:=db.ExecContext(ctx,scenario.trigger);err!=nil{t.Fatal(err)}
  if _,err:=store.DeliverRoundPerception(ctx,task,c.bundle);err==nil{
   t.Fatalf("worker crash %s allowed partial publication",scenario.name)
  }
  if _,err:=db.ExecContext(ctx,"DROP TRIGGER block_evidence_route");err!=nil{t.Fatal(err)}
  for _,table:=range []string{"evidence_ledger","evidence_routes"}{
   var count int
   if err:=db.QueryRowContext(ctx,"SELECT COUNT(*) FROM "+table).Scan(&count);err!=nil{t.Fatal(err)}
   if count!=0{t.Fatalf("%s survived worker crash %s: %d",table,scenario.name,count)}
  }
  var remaining int
  if err:=db.QueryRowContext(ctx,"SELECT COUNT(*) FROM causal_outbox").Scan(&remaining);err!=nil{t.Fatal(err)}
  if remaining!=2{t.Fatalf("completion survived crash %s",scenario.name)}
 }
 if _,err:=store.DeliverRoundPerception(ctx,task,c.bundle);err!=nil{t.Fatal("retry failed",err)}
 _,_,_,_,_,run:=ledger.Snapshot()
 replay,err:=sqlite.Open(ctx,path);if err!=nil{t.Fatal(err)}
 defer replay.Close()
 if _,err:=replay.ReplayClock(ctx,[32]byte(run),c.bundle);err!=nil{t.Fatal("replay after injected failures:",err)}
}

func TestCSFRV16TamperedAddressedEvidenceFailsClosed(t *testing.T){
 ctx:=context.Background()
 c,store,ledger,cycle,fence,path:=singleResourceFixture(t)
 if _,err:=c.CommitSingleResource(ctx,ledger,cycle,fence);err!=nil{t.Fatal(err)}
 db,err:=sql.Open("sqlite","file:"+path);if err!=nil{t.Fatal(err)}
 defer db.Close()
 task:=findTerminalRoundTask(t,ctx,db)
 if _,err:=store.DeliverRoundPerception(ctx,task,c.bundle);err!=nil{t.Fatal(err)}
 var obsID,raw []byte
 if err:=db.QueryRowContext(ctx,"SELECT record_id,canonical FROM evidence_ledger "+
 "JOIN evidence_routes ON evidence_routes.record_id=evidence_ledger.id "+
 "WHERE evidence_routes.kind=0 AND evidence_routes.task_id=?",task[:]).Scan(&obsID,&raw);err!=nil{t.Fatal(err)}
 modified:=bytes.Clone(raw)
 modified[len(modified)-1]^=1
 if _,err:=db.ExecContext(ctx,"UPDATE evidence_ledger SET canonical=? WHERE id=?",modified,obsID);err!=nil{t.Fatal(err)}
 actorInbox:=resourceActorInbox(t,ctx,store,path)
 if _,_,err:=actorInbox.ReadAddressedEvidence(ctx,[2]string{"school","a"});err==nil{
  t.Fatal("tampered Observation passed the holder-only read seal")
 }
 _,_,_,_,_,run:=ledger.Snapshot()
 replay,err:=sqlite.Open(ctx,path);if err!=nil{t.Fatal(err)}
 defer replay.Close()
 if _,err:=replay.ReplayClock(ctx,[32]byte(run),c.bundle);err==nil{
  t.Fatal("causal replay accepted a tampered private observation")
 }
}


func resourceActorInbox(t *testing.T,ctx context.Context,store *sqlite.Store,path string)*sqlite.ActorInbox{
 t.Helper()
 db,err:=sql.Open("sqlite","file:"+path);if err!=nil{t.Fatal(err)}
 defer db.Close()
 rows,err:=db.QueryContext(ctx,"SELECT id,canonical FROM input_ledger")
 if err!=nil{t.Fatal(err)}
 defer rows.Close()
 for rows.Next(){
  var id,raw []byte
  if err:=rows.Scan(&id,&raw);err!=nil{t.Fatal(err)}
  v,err:=codec.StrictDecodePayload(raw);if err!=nil{continue}
  fields,ok:=v.([]any)
  if !ok||len(fields)!=11||fields[4]!="act"{continue}
  actor,ok:=fields[6].([]any)
  if !ok||len(actor)!=2||actor[0]!="school"||actor[1]!="a"{continue}
  var dispatch [32]byte;copy(dispatch[:],id)
  inbox,err:=store.BindActorInbox(ctx,dispatch,[2]string{"school","a"})
  if err!=nil{t.Fatal(err)}
  if _,err:=store.BindActorInbox(ctx,dispatch,[2]string{"school","b"});err==nil{
   t.Fatal("B can mint a capability with A's dispatch")
  }
  return inbox
 }
 if err:=rows.Err();err!=nil{t.Fatal(err)}
 t.Fatal("cannot find A's authenticated dispatch for inbox binding")
 return nil
}


func TestCSFRV16ReplayRejectsIncompleteOrExtraDelivery(t *testing.T){
 for _,tc:=range []struct{name string; mutate func(*testing.T,*sql.DB,perception.RoundProjection)}{
  {"missing-input",func(t *testing.T,db *sql.DB,p perception.RoundProjection){
   if _,err:=db.Exec("DELETE FROM evidence_ledger WHERE id=?",p.Input.Identity[:]);err!=nil{t.Fatal(err)}
  }},
  {"foreign-recipient-route",func(t *testing.T,db *sql.DB,p perception.RoundProjection){
   if _,err:=db.Exec("UPDATE evidence_routes SET holder_id='b' WHERE record_id=?",p.Input.Identity[:]);err!=nil{t.Fatal(err)}
  }},
  {"missing-completion",func(t *testing.T,db *sql.DB,p perception.RoundProjection){
   if _,err:=db.Exec("DELETE FROM causal_outbox WHERE id=?",p.Completion.Identity[:]);err!=nil{t.Fatal(err)}
  }},
  {"extra-unindexed-observation",func(t *testing.T,db *sql.DB,p perception.RoundProjection){
   value,err:=codec.StrictDecodePayload(p.Observation.Canonical);if err!=nil{t.Fatal(err)}
   fields:=append([]any(nil),value.([]any)...)
   fields[3]=uint64(1)
   id,err:=codec.CanonicalBytes("cote.csf.id.observation",
    "cote.csf.schema.observation-id-preimage",1,
    []any{fields[1],fields[2],fields[3],fields[4]})
   if err!=nil{t.Fatal(err)}
   hash:=identity.OpaqueContentHash(id)
   fields[0]=hash[:]
   digestData,err:=codec.CanonicalBytes("cote.csf.digest.observation",
    "cote.csf.schema.observation.body",1,fields[:14])
   if err!=nil{t.Fatal(err)}
   digest:=identity.OpaqueContentHash(digestData)
   fields[14]=digest[:]
   raw,err:=codec.CanonicalPayload(fields);if err!=nil{t.Fatal(err)}
   if _,err:=db.Exec("INSERT INTO evidence_ledger(id,canonical) VALUES(?,?)",hash[:],raw);err!=nil{t.Fatal(err)}
  }},
 }{
  t.Run(tc.name,func(t *testing.T){
   ctx:=context.Background()
   c,store,ledger,cycle,fence,path:=singleResourceFixture(t)
   if _,err:=c.CommitSingleResource(ctx,ledger,cycle,fence);err!=nil{t.Fatal(err)}
   db,err:=sql.Open("sqlite","file:"+path);if err!=nil{t.Fatal(err)}
   defer db.Close()
   task:=findTerminalRoundTask(t,ctx,db)
   p,err:=store.DeliverRoundPerception(ctx,task,c.bundle);if err!=nil{t.Fatal(err)}
   tc.mutate(t,db,p)
   _,_,_,_,_,run:=ledger.Snapshot()
   reopened,err:=sqlite.Open(ctx,path);if err!=nil{t.Fatal(err)}
   defer reopened.Close()
   if _,err:=reopened.ReplayClock(ctx,[32]byte(run),c.bundle);err==nil{
    t.Fatal("replay accepted missing, extra or misaddressed causal evidence")
   }
  })
 }
}


func TestCSFRV16ActorInboxCannotReadAnotherRunSameActor(t *testing.T){
 ctx:=context.Background()
 c,store,ledger,cycle,fence,path:=singleResourceFixture(t)
 if _,err:=c.CommitSingleResource(ctx,ledger,cycle,fence);err!=nil{t.Fatal(err)}
 db,err:=sql.Open("sqlite","file:"+path);if err!=nil{t.Fatal(err)}
 defer db.Close()
 task:=findTerminalRoundTask(t,ctx,db)
 p,err:=store.DeliverRoundPerception(ctx,task,c.bundle);if err!=nil{t.Fatal(err)}
 inbox:=resourceActorInbox(t,ctx,store,path)
 var foreignRun,foreignTask [32]byte
 foreignRun[0],foreignTask[0]=0xfa,0xfb
 decoded,err:=codec.StrictDecodePayload(p.Observation.Canonical);if err!=nil{t.Fatal(err)}
 f:=append([]any(nil),decoded.([]any)...)
 f[1]=foreignTask[:]
 derived,_,err:=identity.New(c.bundle).Sum("cote.csf.id.observation",
  "cote.csf.schema.observation-id-preimage",1,[]any{f[1],f[2],f[3],f[4]})
 if err!=nil{t.Fatal(err)}
 f[0]=derived[:]
 digest,_,err:=identity.New(c.bundle).Sum("cote.csf.digest.observation",
  "cote.csf.schema.observation.body",1,f[:14]);if err!=nil{t.Fatal(err)}
 f[14]=digest[:]
 raw,err:=codec.CanonicalPayload(f);if err!=nil{t.Fatal(err)}
 hash:=identity.OpaqueContentHash(raw)
 if _,err:=db.ExecContext(ctx,"INSERT INTO evidence_ledger(id,canonical) VALUES(?,?)",derived[:],raw);err!=nil{t.Fatal(err)}
 if _,err:=db.ExecContext(ctx,"INSERT INTO evidence_routes(record_id,run_id,task_id,holder_namespace,holder_id,kind,canonical_hash) VALUES(?,?,?,?,?,?,?)",
  derived[:],foreignRun[:],foreignTask[:],"school","a",0,hash[:]);err!=nil{t.Fatal(err)}
 obs,inputs,err:=inbox.ReadAddressedEvidence(ctx,[2]string{"school","a"})
 if err!=nil||len(obs)!=1||len(inputs)!=1||
  !bytes.Equal(obs[0].Canonical,p.Observation.Canonical){
  t.Fatal("same actor in another run crossed the timeline boundary",err,len(obs),len(inputs))
 }
}

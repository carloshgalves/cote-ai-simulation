package commit

import (
 "bytes"
 "context"
 "database/sql"
 "testing"

 _ "modernc.org/sqlite"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/admission"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/clock"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/extensions/testresourcev1"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/inputledger"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/reducer"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/trigger"
)
func singleResourceFixture(t *testing.T)(*Coordinator,*sqlite.Store,*inputledger.Ledger,admission.Cycle,admission.Fence,string){
 return singleResourceFixtureWithFuture(t,false)
}
func singleResourceFixtureWithFuture(t *testing.T,future bool)(*Coordinator,*sqlite.Store,*inputledger.Ledger,admission.Cycle,admission.Fence,string){
 return singleResourceFixtureWithOccurrence(t,future,false)
}
func singleResourceFixtureWithOccurrence(t *testing.T,future,dueNow bool)(*Coordinator,*sqlite.Store,*inputledger.Ledger,admission.Cycle,admission.Fence,string){
 return singleResourceFixtureWithSystem(t,future,dueNow,false,false)
}
func singleResourceFixtureWithSystem(t *testing.T,future,dueNow,activeTrigger,recurring bool)(*Coordinator,*sqlite.Store,*inputledger.Ledger,admission.Cycle,admission.Fence,string){
 t.Helper();ctx:=context.Background()
 bundle,err:=policy.Load(bundlePath);if err!=nil{t.Fatal(err)}
 g:=stockGenesis(t,bundle)
 path:=t.TempDir()+"/single-stock.db"
 store,err:=sqlite.Open(ctx,path);if err!=nil{t.Fatal(err)};t.Cleanup(func(){store.Close()})
 if err:=store.PersistGenesis(ctx,g,bundle);err!=nil{t.Fatal(err)}
 if err:=store.BootstrapClock(ctx,g,bundle);err!=nil{t.Fatal(err)}
 definitions:=[]trigger.Definition(nil)
 if activeTrigger{
  var id domain.ID;id[0]=0xa4
  definitions=append(definitions,trigger.Definition{ID:id,Version:1,Policy:trigger.RisingEdge,
   FireOnInitialTrue:true,Dependencies:[]string{"clock"},PredicateThreshold:g.InitialInstant})
 }
 if err:=store.BootstrapGenesisTriggers(ctx,g,bundle,definitions);err!=nil{t.Fatal(err)}
 if future||dueNow{
  d:=identity.New(bundle)
  origin:=[]any{uint64(2),[]any{g.ID[:],g.Digest[:]}}
  id,_,err:=d.Sum("cote.csf.id.occurrence","cote.csf.schema.occurrence-id-preimage",1,
   []any{origin,uint64(0),uint64(0)});if err!=nil{t.Fatal(err)}
  due:=g.InitialInstant
  if future{due+=100}
  recurrence:=[]any{uint64(0)}
  if recurring{recurrence=[]any{uint64(1),[]any{uint64(0),int64(100),[]any{uint64(0)}}}}
  fields:=[]any{id[:],g.RunID[:],"schedule",int64(due),[]any{int64(due),uint64(0)},
   "test.occurrence",uint64(1),g.Payload[16],uint64(0),origin,uint64(0),uint64(0),
   recurrence,[]any{uint64(0)}}
  dg,_,err:=d.Sum("cote.csf.digest.occurrence-unit","cote.csf.schema.scheduled-occurrence.body",1,fields)
  if err!=nil{t.Fatal(err)}
  canonical,err:=codec.CanonicalPayload(append(cloneAny(fields),dg[:]))
  if err!=nil{t.Fatal(err)}
  if err:=store.SeedGenesisOccurrence(ctx,domain.ID(id),g.RunID,
   domain.EligibilityCoordinate{NotBeforeInstant:due},canonical);err!=nil{t.Fatal(err)}
 }
 ledger,err:=inputledger.New(ctx,g.RunID,bundle,store);if err!=nil{t.Fatal(err)}
 if err:=ledger.DeclareSource(inputledger.Source{ID:"driver"});err!=nil{t.Fatal(err)}
 at:=domain.EligibilityCoordinate{NotBeforeInstant:g.InitialInstant}
 closed:=at
 if future{closed.NotBeforeInstant+=100}
 if _,err:=ledger.CloseSource(ctx,"driver",inputledger.Closure{ProducerKey:"source.closed",ClosedThrough:closed});err!=nil{t.Fatal(err)}
 d:=identity.New(bundle)
 cycleID,_,err:=d.Sum("cote.csf.id.cycle","cote.csf.schema.cycle-id-preimage",1,
  []any{g.RunID[:],[]any{int64(g.InitialInstant),uint64(0)}});if err!=nil{t.Fatal(err)}
 created:=[]any{uint64(2),[]any{g.ID[:],g.Digest[:]}}
 round,err:=ledger.DeclareRound(ctx,"schedule",[2]string{"school","a"},[]string{"act"},at,created,0,0,0,[]any{uint64(0)})
 if err!=nil{t.Fatal(err)}
 dispatchID,_,err:=d.Sum("cote.csf.id.dispatch","cote.csf.schema.dispatch-id-preimage",1,
  []any{g.RunID[:],cycleID[:],round.ID[:],"act",uint64(0)});if err!=nil{t.Fatal(err)}
 dispatch:=inputledger.Dispatch{ID:domain.ID(dispatchID),RunID:g.RunID,CycleID:domain.ID(cycleID),RoundID:round.ID,
  SlotID:"act",Actor:round.Actor,Eligibility:round.Eligibility,BaseRevision:0,EffectiveAt:g.InitialInstant}
 if err:=ledger.RegisterDispatch(ctx,dispatch,inputledger.CycleBinding{
  ID:domain.ID(cycleID),BaseRevision:0,Instant:g.InitialInstant});err!=nil{t.Fatal(err)}
 response,err:=testresourcev1.ProposalForDispatch(bundle,dispatch);if err!=nil{t.Fatal(err)}
 if err:=ledger.Respond(ctx,response);err!=nil{t.Fatal(err)}
 var base domain.Digest;copy(base[:],g.Payload[16].([]any)[3].([]byte))
 cycle:=admission.Cycle{ID:domain.ID(cycleID),Attempt:0,Instant:g.InitialInstant,Ordinal:0,
  BaseRevision:0,BaseStateHash:base,RetryRef:[]any{uint64(0)}}
 refs,err:=policiesByRole(bundle);if err!=nil{t.Fatal(err)}
 policies:=admission.Policies{AdmissionOrder:refs["admission-order"],
  CausalIdentity:refs["causal-identity"],Fence:refs["admission-fence"],Rules:[][]any{}}
 coordinator:=New(store,bundle,policies)
 fence,err:=admission.NewCoordinator(admission.NewFenceLog(store)).AdmitThenEvaluate(ctx,bundle,ledger,cycle,policies,nil)
 if err!=nil{t.Fatal(err)}
 return coordinator,store,ledger,cycle,fence,path
}
func TestSingleResourceCommitAtomicAndReplayable(t *testing.T){
 ctx:=context.Background()
 c,store,ledger,cycle,fence,path:=singleResourceFixture(t)
 _,_,_,_,_,run:=ledger.Snapshot()
 before,err:=store.LoadClock(ctx,[32]byte(run));if err!=nil{t.Fatal(err)}
 for step:=uint8(1);step<=7;step++{
  c.failAfterStep=step
  if _,err:=c.CommitSingleResource(ctx,ledger,cycle,fence);err==nil{t.Fatalf("resource crash point %d accepted",step)}
  clockAfter,err:=store.LoadClock(ctx,[32]byte(run))
  if err!=nil||clockAfter!=before{t.Fatalf("resource crash changed clock: %v %+v",err,clockAfter)}
  stock,_,err:=store.LoadWorldState(ctx,0)
  if err!=nil{t.Fatal(err)}
  current,err:=reducer.DecodeStock(stock);if err!=nil||current.Available!=1{t.Fatal("crash consumed authoritative stock",err)}
 }
 c.failAfterStep=0
 commit,err:=c.CommitSingleResource(ctx,ledger,cycle,fence)
 if err!=nil{t.Fatal(err)}
 stored,err:=store.GetFence(ctx,commit.Identity)
 if err!=nil||!bytes.Equal(stored,commit.Canonical){t.Fatal("resource CycleCommit missing")}
 successor,err:=store.LoadClock(ctx,[32]byte(run))
 if err!=nil||successor.Revision!=1||successor.LastOrdinal!=0||successor.NextLogicalSequence!=2{
  t.Fatalf("resource state clock invalid: %+v %v",successor,err)
 }
 world,_,err:=store.LoadWorldState(ctx,1)
 if err!=nil{t.Fatal(err)}
 stock,err:=reducer.DecodeStock(world);if err!=nil||stock.Available!=0{t.Fatal("successful cycle failed to consume stock",err)}
 db,err:=sql.Open("sqlite","file:"+path);if err!=nil{t.Fatal(err)};defer db.Close()
 for _,item:=range []struct{table string;want int}{{"event_store",2},{"decision_ledger",4},{"world_revisions",2},{"committed_cycles",1},{"causal_outbox",2},{"evidence_ledger",0}}{
  var got int
  if err:=db.QueryRowContext(ctx,"SELECT count(*) FROM "+item.table).Scan(&got);err!=nil{t.Fatal(err)}
  if got!=item.want{t.Fatalf("resource terminal %s contains %d, want %d",item.table,got,item.want)}
 }
 events,err:=db.QueryContext(ctx,"SELECT canonical FROM event_store")
 if err!=nil{t.Fatal(err)}
 for events.Next(){
  var raw []byte
  if err:=events.Scan(&raw);err!=nil{events.Close();t.Fatal(err)}
  decoded,err:=codec.StrictDecodePayload(raw);if err!=nil{events.Close();t.Fatal(err)}
  fields,ok:=decoded.([]any)
  if !ok||len(fields)!=19||fields[15]!=uint64(2){
   events.Close();t.Fatal("resource/lifecycle event entered observable agent route")
  }
 }
 if err:=events.Err();err!=nil{events.Close();t.Fatal(err)}
 events.Close()
 reopened,err:=sqlite.Open(ctx,path);if err!=nil{t.Fatal(err)};defer reopened.Close()
 report,err:=reopened.ReplayClock(ctx,[32]byte(run),c.bundle)
 if err!=nil||report.ReplayedCommits!=1||report.ReplayedEvents!=2||
  report.ReconstructedClock!=successor{
  t.Fatalf("resource terminal replay diverged: %+v %v",report,err)
 }
 nextPending,err:=reopened.PendingClock(ctx,[32]byte(run))
 if err!=nil{t.Fatal(err)}
 if _,available,err:=clock.Next(successor,nextPending);err!=nil||available{
  t.Fatalf("committed resource round became pending again: %v %v",available,err)
 }
}

func TestSingleResourceCommitRejectsForgedProofsWithoutPublishing(t *testing.T){
 cases:=[]struct{name string;mutate func(*ports.WorkBatch)}{
  {"candidate-digest",func(b *ports.WorkBatch){b.ResourceCandidate.Canonical=[]byte{0x80}}},
  {"resource-event",func(b *ports.WorkBatch){b.Events[0].Canonical=[]byte{0x80}}},
  {"lifecycle-event-order",func(b *ports.WorkBatch){b.Events[0],b.Events[1]=b.Events[1],b.Events[0]}},
  {"forged-result-hash",func(b *ports.WorkBatch){b.ResultStateHash[0]^=0xff}},
  {"world-revision",func(b *ports.WorkBatch){b.RevisionCanonical=[]byte{0x80}}},
  {"forged-decision",func(b *ports.WorkBatch){b.Decisions[0].Canonical=[]byte{0x80}}},
  {"truncated-admission-prefix",func(b *ports.WorkBatch){
   if len(b.ExpectedInputPrefix)>0{b.ExpectedInputPrefix=b.ExpectedInputPrefix[:len(b.ExpectedInputPrefix)-1]}
  }},
 }
 for _,tc:=range cases{
  t.Run(tc.name,func(t *testing.T){
   ctx:=context.Background()
   c,store,ledger,cycle,fence,path:=singleResourceFixture(t)
   c.tamperWorkBatch=tc.mutate
   if _,err:=c.CommitSingleResource(ctx,ledger,cycle,fence);err==nil{
    t.Fatal("SQLite accepted a forged or truncated resource terminal batch")
   }
   _,_,_,_,_,run:=ledger.Snapshot()
   state,err:=store.LoadClock(ctx,[32]byte(run))
   if err!=nil||state.Revision!=0||state.NextLogicalSequence!=0{t.Fatal("failed terminal mutated clock",err)}
   before,_,err:=store.LoadWorldState(ctx,0)
   if err!=nil{t.Fatal(err)}
   stock,err:=reducer.DecodeStock(before)
   if err!=nil||stock.Available!=1{t.Fatal("failed terminal mutated world",err)}
   db,err:=sql.Open("sqlite","file:"+path);if err!=nil{t.Fatal(err)};defer db.Close()
   for _,table:=range []string{"event_store","committed_cycles"}{
    var n int
    if err:=db.QueryRowContext(ctx,"SELECT count(*) FROM "+table).Scan(&n);err!=nil{t.Fatal(err)}
    if n!=0{t.Fatalf("failed resource terminal emitted %d rows to %s",n,table)}
   }
   c.tamperWorkBatch=nil
   if _,err:=c.CommitSingleResource(ctx,ledger,cycle,fence);err!=nil{
    t.Fatalf("valid retry of unchanged in-flight fence failed: %v",err)
   }
  })
 }
}

func TestResourceConsumedThenClockAdvancePreservesStockAndReplays(t *testing.T){
 ctx:=context.Background()
 c,store,ledger,cycle,fence,path:=singleResourceFixtureWithFuture(t,true)
 if _,err:=c.CommitSingleResource(ctx,ledger,cycle,fence);err!=nil{t.Fatal(err)}
 _,_,_,_,_,run:=ledger.Snapshot()
 before,_,err:=store.LoadWorldState(ctx,1);if err!=nil{t.Fatal(err)}
 used,err:=reducer.DecodeStock(before);if err!=nil||used.Available!=0{t.Fatal("resource reducer missed unit",err)}
 advance,err:=c.Advance(ctx,ledger,run)
 if err!=nil{t.Fatalf("CLOCK_ADVANCE could not advance a mutated world: %v",err)}
 if advance.ResultClock.Revision!=2||advance.ResultClock.Instant<=cycle.Instant{
  t.Fatal("resource successor did not advance time",advance.ResultClock)
 }
 after,_,err:=store.LoadWorldState(ctx,2);if err!=nil{t.Fatal(err)}
 remaining,err:=reducer.DecodeStock(after)
 if err!=nil||remaining.Available!=0{t.Fatal("temporal advance replenished spent resource",err)}
 reopened,err:=sqlite.Open(ctx,path);if err!=nil{t.Fatal(err)};defer reopened.Close()
 report,err:=reopened.ReplayClock(ctx,[32]byte(run),c.bundle)
 if err!=nil||report.ReplayedCommits!=2||report.ReplayedEvents!=4||
  report.ReconstructedClock!=advance.ResultClock{
  t.Fatalf("resource WORK + CLOCK_ADVANCE replay failed: %+v %v",report,err)
 }
}

func TestPerceptionTaskOutboxIsAtomicPrivateAndReplayAuthenticated(t *testing.T){
 ctx:=context.Background()
 c,store,ledger,cycle,fence,path:=singleResourceFixture(t)
 _,_,_,_,_,run:=ledger.Snapshot()
 before,err:=store.LoadClock(ctx,[32]byte(run));if err!=nil{t.Fatal(err)}
 db,err:=sql.Open("sqlite","file:"+path);if err!=nil{t.Fatal(err)}
 defer db.Close()
 for step:=uint8(1);step<=7;step++{
  c.failAfterStep=step
  if _,err:=c.CommitSingleResource(ctx,ledger,cycle,fence);err==nil{
   t.Fatalf("perception task WORK failpoint %d bypassed",step)
  }
  var tasks int
  if err:=db.QueryRowContext(ctx,"SELECT count(*) FROM causal_outbox").Scan(&tasks);err!=nil{t.Fatal(err)}
  after,err:=store.LoadClock(ctx,[32]byte(run))
  if err!=nil||after!=before||tasks!=0{
   t.Fatalf("failpoint %d leaked task or advanced clock: %v %+v tasks=%d",step,err,after,tasks)
  }
 }
 c.failAfterStep=0
 commit,err:=c.CommitSingleResource(ctx,ledger,cycle,fence)
 if err!=nil{t.Fatal(err)}
 decoded,err:=codec.StrictDecodePayload(commit.Canonical);if err!=nil{t.Fatal(err)}
 fields:=decoded.([]any)
 ids:=fields[18].([]any)
 if len(ids)!=2{t.Fatalf("registered visible-first V1 events require 2 pending tasks, got %d",len(ids))}
 for _,item:=range ids{
  id,ok:=item.([]byte);if !ok||len(id)!=32{t.Fatal("task identity malformed")}
  var raw []byte
  if err:=db.QueryRowContext(ctx,"SELECT canonical FROM causal_outbox WHERE id=?",id).Scan(&raw);err!=nil{t.Fatal(err)}
  task,err:=codec.StrictDecodePayload(raw);if err!=nil{t.Fatal(err)}
  body,ok:=task.([]any)
  if !ok||len(body)!=10||body[9]!=uint64(0)||body[4]!=uint64(0)||body[5]!=uint64(1){
   t.Fatal("outbox projected an unregistered/non-PENDING task")
  }
  // Tasks contain only event/revision/policies. They must not contain an
  // observation, allow-list, KnowledgeInput, or source response payload.
  if len(raw)>1200{t.Fatal("PENDING task appears to carry unauthorized context")}
 }
 reopened,err:=sqlite.Open(ctx,path);if err!=nil{t.Fatal(err)}
 if _,err:=reopened.ReplayClock(ctx,[32]byte(run),c.bundle);err!=nil{
  reopened.Close();t.Fatal("complete causal outbox failed replay",err)
 }
 reopened.Close()
 if _,err:=db.ExecContext(ctx,"DELETE FROM causal_outbox WHERE id=?",ids[0].([]byte));err!=nil{t.Fatal(err)}
 reopened,err=sqlite.Open(ctx,path);if err!=nil{t.Fatal(err)}
 defer reopened.Close()
 if _,err:=reopened.ReplayClock(ctx,[32]byte(run),c.bundle);err==nil{
  t.Fatal("replay accepted committed Event without its Genesis-pinned PerceptionTask")
 }
}

func TestUnreferencedPerceptionTaskCannotAppearAsAuthorizedCausalOutbox(t *testing.T){
 ctx:=context.Background()
 c,_,ledger,cycle,fence,path:=singleResourceFixture(t)
 if _,err:=c.CommitSingleResource(ctx,ledger,cycle,fence);err!=nil{t.Fatal(err)}
 _,_,_,_,_,run:=ledger.Snapshot()
 db,err:=sql.Open("sqlite","file:"+path);if err!=nil{t.Fatal(err)}
 var id,canonical []byte
 if err:=db.QueryRowContext(ctx,"SELECT id,canonical FROM causal_outbox LIMIT 1").Scan(&id,&canonical);err!=nil{t.Fatal(err)}
 orphan:=append([]byte(nil),id...)
 orphan[0]^=0x8f
 if _,err:=db.ExecContext(ctx,"INSERT INTO causal_outbox(id,canonical) VALUES(?,?)",orphan,canonical);err!=nil{t.Fatal(err)}
 db.Close()
 reopened,err:=sqlite.Open(ctx,path);if err!=nil{t.Fatal(err)}
 defer reopened.Close()
 if _,err:=reopened.ReplayClock(ctx,[32]byte(run),c.bundle);err==nil{
  t.Fatal("replay admitted an extra uncommitted task that could become an unauthorized observation")
 }
}

func TestResourceAndDueOccurrenceMustShareOneTerminalCommit(t *testing.T){
 ctx:=context.Background()
 c,store,ledger,cycle,fence,path:=singleResourceFixtureWithOccurrence(t,false,true)
 if len(fence.Fields[11].([]any))!=2{t.Fatal("resource/occurrence co-admission not frozen together")}
 _,_,_,_,_,run:=ledger.Snapshot()
 initial,err:=store.LoadClock(ctx,[32]byte(run));if err!=nil{t.Fatal(err)}
 for step:=uint8(1);step<=7;step++{
  c.failAfterStep=step
  if _,err:=c.CommitSingleResource(ctx,ledger,cycle,fence);err==nil{t.Fatalf("mixed resource WORK failpoint %d bypassed",step)}
  state,err:=store.LoadClock(ctx,[32]byte(run))
  if err!=nil||state!=initial{t.Fatalf("mixed resource crash half-mutated world at %d",step)}
 }
 c.failAfterStep=0
 if _,err:=c.CommitSingleResource(ctx,ledger,cycle,fence);err!=nil{t.Fatalf("co-eligible resource+occurrence must settle atomically: %v",err)}
 state,err:=store.LoadClock(ctx,[32]byte(run))
 if err!=nil||state.Revision!=1||state.NextLogicalSequence!=3{t.Fatal("resource+system committed incomplete event sequence",err)}
 world,_,err:=store.LoadWorldState(ctx,1)
 if err!=nil{t.Fatal(err)}
 stock,err:=reducer.DecodeStock(world)
 if err!=nil||stock.Available!=0{t.Fatal("resource consumption discarded when scheduled source present",err)}
 pending,err:=store.PendingClock(ctx,[32]byte(run))
 if err!=nil||len(pending)!=0{t.Fatal("due scheduled occurrence left pending after resource commit",err)}
 reopened,err:=sqlite.Open(ctx,path);if err!=nil{t.Fatal(err)}
 report,err:=reopened.ReplayClock(ctx,[32]byte(run),c.bundle)
 reopened.Close()
 if err!=nil||report.ReplayedCommits!=1||report.ReplayedEvents!=3{t.Fatalf("resource+system replay failed: %+v %v",report,err)}
}

func TestResourceWithSimultaneousTriggerAndDeferredOccurrence(t *testing.T){
 for _,tc:=range []struct{name string;trigger,due,recurring bool;events,remaining int}{
  {"trigger-only",true,false,false,3,0},
  {"deferred-occurrence",false,true,true,3,1},
  {"trigger-and-deferred-occurrence",true,true,true,4,1},
 }{t.Run(tc.name,func(t *testing.T){
  ctx:=context.Background()
  c,store,ledger,cycle,fence,path:=singleResourceFixtureWithSystem(t,false,tc.due,tc.trigger,tc.recurring)
  _,_,_,_,_,run:=ledger.Snapshot()
  if len(fence.Fields[11].([]any))!=tc.events-1{t.Fatal("incomplete mixed system resource admission")}
  before,err:=store.LoadClock(ctx,[32]byte(run));if err!=nil{t.Fatal(err)}
  for failure:=uint8(1);failure<=7;failure++{
   c.failAfterStep=failure
   if _,err:=c.CommitSingleResource(ctx,ledger,cycle,fence);err==nil{t.Fatalf("resource+system crash point %d committed",failure)}
   after,err:=store.LoadClock(ctx,[32]byte(run))
   if err!=nil||after!=before{t.Fatal("crash published partial resource/system outcome",err)}
  }
  c.failAfterStep=0
  if _,err:=c.CommitSingleResource(ctx,ledger,cycle,fence);err!=nil{t.Fatal(err)}
  after,err:=store.LoadClock(ctx,[32]byte(run))
  if err!=nil||after.Revision!=1||uint64(after.NextLogicalSequence)!=uint64(tc.events){
   t.Fatal("mixed terminal omitted lifecycle Event",err)
  }
  typed,_,err:=store.LoadWorldState(ctx,1);if err!=nil{t.Fatal(err)}
  stock,err:=reducer.DecodeStock(typed);if err!=nil||stock.Available!=0{
   t.Fatal("resource+system lost registered reducer output",err)
  }
  pending,err:=store.PendingClock(ctx,[32]byte(run))
  if err!=nil||len(pending)!=tc.remaining{t.Fatal("resource+system left incorrect system pending membership",err)}
  reopened,err:=sqlite.Open(ctx,path);if err!=nil{t.Fatal(err)}
  report,err:=reopened.ReplayClock(ctx,[32]byte(run),c.bundle)
  reopened.Close()
  if err!=nil||report.ReplayedCommits!=1||report.ReplayedEvents!=tc.events{
   t.Fatalf("mixed resource/system replay failed: %+v %v",report,err)
  }
 })}
}

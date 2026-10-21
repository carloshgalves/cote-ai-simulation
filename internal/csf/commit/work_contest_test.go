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
)
func twoResourceContestFixture(t *testing.T,reverse bool)(*Coordinator,*sqlite.Store,*inputledger.Ledger,admission.Cycle,admission.Fence,string){
 return twoResourceContestFixtureWithOccurrence(t,reverse,false)
}
func twoResourceContestFixtureWithOccurrence(t *testing.T,reverse,dueNow bool)(*Coordinator,*sqlite.Store,*inputledger.Ledger,admission.Cycle,admission.Fence,string){
 t.Helper()
 ctx:=context.Background()
 bundle,err:=policy.Load(bundlePath);if err!=nil{t.Fatal(err)}
 g:=stockGenesis(t,bundle)
 path:=t.TempDir()+"/contest.db"
 store,err:=sqlite.Open(ctx,path);if err!=nil{t.Fatal(err)}
 t.Cleanup(func(){store.Close()})
 if err:=store.PersistGenesis(ctx,g,bundle);err!=nil{t.Fatal(err)}
 if err:=store.BootstrapClock(ctx,g,bundle);err!=nil{t.Fatal(err)}
 if err:=store.BootstrapGenesisTriggers(ctx,g,bundle,nil);err!=nil{t.Fatal(err)}
 if dueNow{
  d:=identity.New(bundle)
  origin:=[]any{uint64(2),[]any{g.ID[:],g.Digest[:]}}
  id,_,err:=d.Sum("cote.csf.id.occurrence","cote.csf.schema.occurrence-id-preimage",1,
   []any{origin,uint64(0),uint64(0)});if err!=nil{t.Fatal(err)}
  due:=int64(g.InitialInstant)
  fields:=[]any{id[:],g.RunID[:],"schedule",due,[]any{due,uint64(0)},
   "test.occurrence",uint64(1),g.Payload[16],uint64(0),origin,uint64(0),uint64(0),
   []any{uint64(0)},[]any{uint64(0)}}
  dg,_,err:=d.Sum("cote.csf.digest.occurrence-unit","cote.csf.schema.scheduled-occurrence.body",1,fields)
  if err!=nil{t.Fatal(err)}
  raw,err:=codec.CanonicalPayload(append(cloneAny(fields),dg[:]));if err!=nil{t.Fatal(err)}
  if err:=store.SeedGenesisOccurrence(ctx,domain.ID(id),g.RunID,
   domain.EligibilityCoordinate{NotBeforeInstant:g.InitialInstant},raw);err!=nil{t.Fatal(err)}
 }
 ledger,err:=inputledger.New(ctx,g.RunID,bundle,store);if err!=nil{t.Fatal(err)}
 if err:=ledger.DeclareSource(inputledger.Source{ID:"driver"});err!=nil{t.Fatal(err)}
 at:=domain.EligibilityCoordinate{NotBeforeInstant:g.InitialInstant}
 if _,err:=ledger.CloseSource(ctx,"driver",inputledger.Closure{ProducerKey:"ready",ClosedThrough:at});err!=nil{t.Fatal(err)}
 d:=identity.New(bundle)
 cycleHash,_,err:=d.Sum("cote.csf.id.cycle","cote.csf.schema.cycle-id-preimage",1,
  []any{g.RunID[:],[]any{int64(g.InitialInstant),uint64(0)}});if err!=nil{t.Fatal(err)}
 id:=domain.ID(cycleHash)
 created:=[]any{uint64(2),[]any{g.ID[:],g.Digest[:]}}
 responses:=make([]inputledger.Response,0,2)
 for i,actor:=range []string{"a","b"}{
  round,err:=ledger.DeclareRound(ctx,"schedule",[2]string{"school",actor},[]string{"act"},
   at,created,0,uint32(i),0,[]any{uint64(0)});if err!=nil{t.Fatal(err)}
  dispatchHash,_,err:=d.Sum("cote.csf.id.dispatch","cote.csf.schema.dispatch-id-preimage",1,
   []any{g.RunID[:],id[:],round.ID[:],"act",uint64(0)});if err!=nil{t.Fatal(err)}
  dispatch:=inputledger.Dispatch{ID:domain.ID(dispatchHash),RunID:g.RunID,CycleID:id,RoundID:round.ID,
   SlotID:"act",Actor:round.Actor,Eligibility:round.Eligibility,BaseRevision:0,EffectiveAt:g.InitialInstant}
  if err:=ledger.RegisterDispatch(ctx,dispatch,inputledger.CycleBinding{ID:id,BaseRevision:0,Instant:g.InitialInstant});err!=nil{t.Fatal(err)}
  response,err:=testresourcev1.ProposalForDispatch(bundle,dispatch);if err!=nil{t.Fatal(err)}
  responses=append(responses,response)
 }
 if reverse{responses[0],responses[1]=responses[1],responses[0]}
 for _,v:=range responses{if err:=ledger.Respond(ctx,v);err!=nil{t.Fatal(err)}}
 var base domain.Digest;copy(base[:],g.Payload[16].([]any)[3].([]byte))
 cycle:=admission.Cycle{ID:id,Attempt:0,Instant:g.InitialInstant,Ordinal:0,BaseRevision:0,
  BaseStateHash:base,RetryRef:[]any{uint64(0)}}
 refs,err:=policiesByRole(bundle);if err!=nil{t.Fatal(err)}
 policies:=admission.Policies{AdmissionOrder:refs["admission-order"],
  CausalIdentity:refs["causal-identity"],Fence:refs["admission-fence"],Rules:[][]any{}}
 c:=New(store,bundle,policies)
 fence,err:=admission.NewCoordinator(admission.NewFenceLog(store)).AdmitThenEvaluate(ctx,bundle,ledger,cycle,policies,nil)
 if err!=nil{t.Fatal(err)}
 return c,store,ledger,cycle,fence,path
}
func TestTwoResourceContestCommitReplaysAndDoesNotDependOnArrival(t *testing.T){
 ctx:=context.Background()
 var firstFence,firstCommit []byte
 var firstEvents [][]byte
 for _,reverse:=range []bool{false,true}{
  c,store,ledger,cycle,fence,path:=twoResourceContestFixture(t,reverse)
  journal,err:=c.CommitTwoResourceContest(ctx,ledger,cycle,fence)
  if err!=nil{t.Fatal("contested resource did not publish: ",err)}
  _,_,_,_,_,run:=ledger.Snapshot()
  state,err:=store.LoadClock(ctx,[32]byte(run))
  if err!=nil||state.Revision!=1||state.NextLogicalSequence!=3{t.Fatalf("contest clock wrong: %+v %v",state,err)}
  world,_,err:=store.LoadWorldState(ctx,1);if err!=nil{t.Fatal(err)}
  stock,err:=reducer.DecodeStock(world)
  if err!=nil||stock.Available!=0{t.Fatal("winner did not consume exactly one stock unit",err)}
  db,err:=sql.Open("sqlite","file:"+path);if err!=nil{t.Fatal(err)}
  var records,events int
  if err:=db.QueryRowContext(ctx,"SELECT count(*) FROM decision_ledger").Scan(&records);err!=nil{t.Fatal(err)}
  if err:=db.QueryRowContext(ctx,"SELECT count(*) FROM event_store").Scan(&events);err!=nil{t.Fatal(err)}
  if records!=10||events!=3{t.Fatalf("wrong conflict/assessment/RNG/decision journal cardinality: %d records %d events",records,events)}
  var all [][]byte
  rows,err:=db.QueryContext(ctx,"SELECT canonical FROM event_store ORDER BY id");if err!=nil{t.Fatal(err)}
  for rows.Next(){var raw []byte;if err:=rows.Scan(&raw);err!=nil{t.Fatal(err)};all=append(all,raw)}
  if err:=rows.Err();err!=nil{t.Fatal(err)};rows.Close();db.Close()
  if !reverse{firstFence=fence.Canonical;firstCommit=journal.Canonical;firstEvents=all}else{
   if !bytes.Equal(firstFence,fence.Canonical)||!bytes.Equal(firstCommit,journal.Canonical){
    t.Fatal("worker response permutation changed conflict resolution journal")
   }
   for i:=range firstEvents{if !bytes.Equal(firstEvents[i],all[i]){t.Fatal("worker order changed event bytes")}}
  }
  reopened,err:=sqlite.Open(ctx,path);if err!=nil{t.Fatal(err)}
  report,err:=reopened.ReplayClock(ctx,[32]byte(run),c.bundle)
  reopened.Close()
  if err!=nil||report.ReplayedCommits!=1||report.ReplayedEvents!=3{
   t.Fatalf("replay of disputed last stock failed: %+v %v",report,err)
  }
  pending,err:=store.PendingClock(ctx,[32]byte(run));if err!=nil{t.Fatal(err)}
  if _,exists,err:=clock.Next(state,pending);err!=nil||exists{
   t.Fatal("settled losing contender still pending",exists,err)
  }
 }
}

func TestTwoResourceContestAtomicAcrossSevenCrashPoints(t *testing.T){
 ctx:=context.Background()
 c,store,ledger,cycle,fence,path:=twoResourceContestFixture(t,false)
 _,_,_,_,_,run:=ledger.Snapshot()
 original,err:=store.LoadClock(ctx,[32]byte(run));if err!=nil{t.Fatal(err)}
 for step:=uint8(1);step<=7;step++{
  c.failAfterStep=step
  if _,err:=c.CommitTwoResourceContest(ctx,ledger,cycle,fence);err==nil{
   t.Fatalf("contested resource failpoint %d was bypassed",step)
  }
  current,err:=store.LoadClock(ctx,[32]byte(run))
  if err!=nil||current!=original{t.Fatalf("crash %d advanced world clock: %+v %v",step,current,err)}
  state,_,err:=store.LoadWorldState(ctx,0);if err!=nil{t.Fatal(err)}
  stock,err:=reducer.DecodeStock(state)
  if err!=nil||stock.Available!=1{t.Fatalf("failpoint %d consumed uncommitted resource",step)}
  db,err:=sql.Open("sqlite","file:"+path);if err!=nil{t.Fatal(err)}
  for _,test:=range []struct{table string;expected int}{
   {"event_store",0},{"committed_cycles",0},{"decision_ledger",1},{"world_revisions",1},
  }{
   var count int
   if err:=db.QueryRowContext(ctx,"SELECT count(*) FROM "+test.table).Scan(&count);err!=nil{t.Fatal(err)}
   if count!=test.expected{t.Fatalf("failpoint %d leaked %s: got %d expected %d",step,test.table,count,test.expected)}
  }
  db.Close()
 }
 c.failAfterStep=0
 if _,err:=c.CommitTwoResourceContest(ctx,ledger,cycle,fence);err!=nil{t.Fatal(err)}
 if _,err:=c.CommitTwoResourceContest(ctx,ledger,cycle,fence);err==nil{
  t.Fatal("duplicate contested resource allocation accepted")
 }
}
func TestTwoResourceContestRejectsForgedEventAndRNGProof(t *testing.T){
 cases:=[]struct{name string;mutate func(*ports.WorkBatch)}{
  {"event",func(b *ports.WorkBatch){b.Events[0].Canonical=[]byte{0x80}}},
  {"winner-decision",func(b *ports.WorkBatch){b.Decisions[0].Canonical=[]byte{0x80}}},
  {"candidate-proof",func(b *ports.WorkBatch){b.ContestProofs[0].Canonical=[]byte{0x80}}},
  {"proof-order",func(b *ports.WorkBatch){b.ContestProofs[0],b.ContestProofs[1]=b.ContestProofs[1],b.ContestProofs[0]}},
  {"stock-result",func(b *ports.WorkBatch){b.ResultStateHash[0]^=0xff}},
  {"lifecycle-event-order",func(b *ports.WorkBatch){b.Events[1],b.Events[2]=b.Events[2],b.Events[1]}},
  {"input-cut",func(b *ports.WorkBatch){b.ExpectedInputPrefix=b.ExpectedInputPrefix[:len(b.ExpectedInputPrefix)-1]}},
 }
 for _,tc:=range cases{t.Run(tc.name,func(t *testing.T){
  ctx:=context.Background()
  c,store,ledger,cycle,fence,_:=twoResourceContestFixture(t,false)
  c.tamperWorkBatch=tc.mutate
  if _,err:=c.CommitTwoResourceContest(ctx,ledger,cycle,fence);err==nil{
   t.Fatal("forged contested terminal batch accepted by SQLite")
  }
  _,_,_,_,_,run:=ledger.Snapshot()
  state,err:=store.LoadClock(ctx,[32]byte(run))
  if err!=nil||state.Revision!=0{t.Fatal("rejected contested batch advanced clock",err)}
  c.tamperWorkBatch=nil
  if _,err:=c.CommitTwoResourceContest(ctx,ledger,cycle,fence);err!=nil{
   t.Fatalf("clean contested batch cannot succeed after rollback: %v",err)
  }
 })}
}

func TestTwoResourceContestReplayRejectsMissingAffordanceProof(t *testing.T){
 ctx:=context.Background()
 c,_,ledger,cycle,fence,path:=twoResourceContestFixture(t,false)
 if _,err:=c.CommitTwoResourceContest(ctx,ledger,cycle,fence);err!=nil{t.Fatal(err)}
 _,_,_,_,_,run:=ledger.Snapshot()
 db,err:=sql.Open("sqlite","file:"+path);if err!=nil{t.Fatal(err)}
 rows,err:=db.QueryContext(ctx,"SELECT id,canonical FROM decision_ledger")
 if err!=nil{t.Fatal(err)}
 var target []byte
 for rows.Next(){
  var id,raw []byte
  if err:=rows.Scan(&id,&raw);err!=nil{t.Fatal(err)}
  decoded,err:=codec.StrictDecodePayload(raw);if err!=nil{t.Fatal(err)}
  fields,ok:=decoded.([]any)
  if !ok||len(fields)!=3{continue}
  subject,ok:=fields[0].([]any)
  if ok&&len(subject)==3&&subject[0]==uint64(5){target=append([]byte(nil),id...);break}
 }
 rows.Close()
 if len(target)!=32{t.Fatal("contested assessment proof absent from durable journal")}
 if _,err:=db.ExecContext(ctx,"DELETE FROM decision_ledger WHERE id=?",target);err!=nil{t.Fatal(err)}
 db.Close()
 reopened,err:=sqlite.Open(ctx,path);if err!=nil{t.Fatal(err)};defer reopened.Close()
 if _,err:=reopened.ReplayClock(ctx,[32]byte(run),c.bundle);err==nil{
  t.Fatal("replay accepted a contested settlement with one missing affordance assessment")
 }
}

func TestTwoResourceContendersWithScheduledOccurrenceOneAtomicCommit(t *testing.T){
 ctx:=context.Background()
 c,store,ledger,cycle,fence,path:=twoResourceContestFixtureWithOccurrence(t,false,true)
 if len(fence.Fields[11].([]any))!=3{t.Fatal("contest/system fence omitted source")}
 _,_,_,_,_,run:=ledger.Snapshot()
 original,err:=store.LoadClock(ctx,[32]byte(run));if err!=nil{t.Fatal(err)}
 for crash:=uint8(1);crash<=7;crash++{
  c.failAfterStep=crash
  if _,err:=c.CommitTwoResourceContest(ctx,ledger,cycle,fence);err==nil{
   t.Fatalf("contest/system failpoint %d committed",crash)
  }
  after,err:=store.LoadClock(ctx,[32]byte(run))
  if err!=nil||after!=original{t.Fatal("contest/system partially changed clock",err)}
 }
 c.failAfterStep=0
 if _,err:=c.CommitTwoResourceContest(ctx,ledger,cycle,fence);err!=nil{
  t.Fatalf("two contenders and schedule cannot settle same fence: %v",err)
 }
 now,err:=store.LoadClock(ctx,[32]byte(run))
 if err!=nil||now.Revision!=1||now.NextLogicalSequence!=4{t.Fatal("contest/system event batch incomplete",err)}
 typed,_,err:=store.LoadWorldState(ctx,1);if err!=nil{t.Fatal(err)}
 stock,err:=reducer.DecodeStock(typed)
 if err!=nil||stock.Available!=0{t.Fatal("contest winner did not consume last resource",err)}
 pending,err:=store.PendingClock(ctx,[32]byte(run))
 if err!=nil||len(pending)!=0{t.Fatal("contest/system left admitted source pending",err)}
 reopened,err:=sqlite.Open(ctx,path);if err!=nil{t.Fatal(err)}
 report,err:=reopened.ReplayClock(ctx,[32]byte(run),c.bundle)
 reopened.Close()
 if err!=nil||report.ReplayedCommits!=1||report.ReplayedEvents!=4{
  t.Fatalf("contest/system replay failed: %+v %v",report,err)
 }
}

func TestContestedResourceAndScheduleWorkerPermutationPreservesTerminalBytes(t *testing.T){
 ctx:=context.Background()
 var previous,previousFence []byte
 var previousEvents [][]byte
 for index,reverse:=range []bool{false,true}{
  c,_,ledger,cycle,fence,path:=twoResourceContestFixtureWithOccurrence(t,reverse,true)
  journal,err:=c.CommitTwoResourceContest(ctx,ledger,cycle,fence)
  if err!=nil{t.Fatal("contest+schedule does not commit under worker response permutation",err)}
  db,err:=sql.Open("sqlite","file:"+path);if err!=nil{t.Fatal(err)}
  rows,err:=db.QueryContext(ctx,"SELECT canonical FROM event_store ORDER BY id");if err!=nil{t.Fatal(err)}
  var events [][]byte
  for rows.Next(){
   var raw []byte
   if err:=rows.Scan(&raw);err!=nil{t.Fatal(err)}
   events=append(events,append([]byte(nil),raw...))
  }
  if err:=rows.Err();err!=nil{t.Fatal(err)}
  rows.Close();db.Close()
  if index==0{
   previous=append([]byte(nil),journal.Canonical...)
   previousFence=append([]byte(nil),fence.Canonical...)
   previousEvents=events
  }else{
   if !bytes.Equal(previous,journal.Canonical)||!bytes.Equal(previousFence,fence.Canonical)||len(previousEvents)!=len(events){
    t.Fatal("contest plus schedule depends on worker arrival order")
   }
   for i,raw:=range events{
    if !bytes.Equal(previousEvents[i],raw){t.Fatalf("contested source lifecycle Event %d changed by arrival order",i)}
   }
  }
 }
}

func TestContestWithScheduledSourceRejectsForgedResourceAndLifecycleProofs(t *testing.T){
 for _,variant:=range []struct{name string;mutate func(*ports.WorkBatch)}{
  {"conflict-rng-proof",func(b *ports.WorkBatch){b.ContestProofs[0].Canonical=[]byte{0x80}}},
  {"resource-winner",func(b *ports.WorkBatch){b.Decisions[0].Canonical=[]byte{0x80}}},
  {"resource-loser",func(b *ports.WorkBatch){b.Decisions[1].Canonical=[]byte{0x80}}},
  {"schedule-decision",func(b *ports.WorkBatch){b.Decisions[2].Canonical=[]byte{0x80}}},
  {"schedule-candidate",func(b *ports.WorkBatch){b.SystemCandidate.Canonical=[]byte{0x80}}},
  {"event-order",func(b *ports.WorkBatch){b.Events[1],b.Events[2]=b.Events[2],b.Events[1]}},
  {"world-hash",func(b *ports.WorkBatch){b.ResultStateHash[0]^=1}},
  {"outbox-task",func(b *ports.WorkBatch){b.PerceptionTasks[0].Canonical=[]byte{0x80}}},
 }{t.Run(variant.name,func(t *testing.T){
  ctx:=context.Background()
  c,store,ledger,cycle,fence,_:=twoResourceContestFixtureWithOccurrence(t,false,true)
  _,_,_,_,_,run:=ledger.Snapshot()
  original,err:=store.LoadClock(ctx,[32]byte(run));if err!=nil{t.Fatal(err)}
  c.tamperWorkBatch=variant.mutate
  if _,err:=c.CommitTwoResourceContest(ctx,ledger,cycle,fence);err==nil{
   t.Fatal("forged resource conflict/system evidence reached terminal publication")
  }
  now,err:=store.LoadClock(ctx,[32]byte(run))
  if err!=nil||now!=original{t.Fatal("failed contested system transaction changed world",err)}
  c.tamperWorkBatch=nil
  if _,err:=c.CommitTwoResourceContest(ctx,ledger,cycle,fence);err!=nil{
   t.Fatal("valid contested source batch cannot retry cleanly after rejected evidence",err)
  }
 })}
}

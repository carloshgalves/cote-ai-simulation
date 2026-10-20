package commit

import (
 "bytes"
 "context"
 "database/sql"
 "testing"

 _ "modernc.org/sqlite"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/admission"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/clock"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/extensions/testresourcev1"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/inputledger"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/reducer"
)
func twoResourceContestFixture(t *testing.T,reverse bool)(*Coordinator,*sqlite.Store,*inputledger.Ledger,admission.Cycle,admission.Fence,string){
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

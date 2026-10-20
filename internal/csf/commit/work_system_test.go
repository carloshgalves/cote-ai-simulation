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
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/inputledger"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/trigger"
)

func systemWorkFixture(t *testing.T,kind string)(*Coordinator,*sqlite.Store,*inputledger.Ledger,
 admission.Cycle,admission.Fence,string){
 t.Helper();ctx:=context.Background()
 bundle,err:=policy.Load(bundlePath);if err!=nil{t.Fatal(err)}
 g:=validGenesis(t,bundle)
 path:=t.TempDir()+"/system-work.db"
 store,err:=sqlite.Open(ctx,path);if err!=nil{t.Fatal(err)}
 t.Cleanup(func(){store.Close()})
 if err:=store.PersistGenesis(ctx,g,bundle);err!=nil{t.Fatal(err)}
 if err:=store.BootstrapClock(ctx,g,bundle);err!=nil{t.Fatal(err)}
 if kind=="trigger"{
  var triggerID domain.ID;triggerID[0]=0x83
  def:=trigger.Definition{ID:triggerID,Version:1,Policy:trigger.RisingEdge,
   FireOnInitialTrue:true,Dependencies:[]string{"clock"},PredicateThreshold:g.InitialInstant}
  if err:=store.BootstrapGenesisTriggers(ctx,g,bundle,[]trigger.Definition{def});err!=nil{t.Fatal(err)}
 }else{
  if err:=store.BootstrapGenesisTriggers(ctx,g,bundle,nil);err!=nil{t.Fatal(err)}
  origin:=[]any{uint64(2),[]any{g.ID[:],g.Digest[:]}}
  d:=identity.New(bundle)
  id,_,err:=d.Sum("cote.csf.id.occurrence","cote.csf.schema.occurrence-id-preimage",1,
   []any{origin,uint64(0),uint64(0)});if err!=nil{t.Fatal(err)}
  recurrence:=[]any{uint64(0)}
  if kind=="defer"{recurrence=[]any{uint64(1),[]any{uint64(0),int64(100),[]any{uint64(0)}}}}
  fields:=[]any{id[:],g.RunID[:],"schedule",int64(g.InitialInstant),
   []any{int64(g.InitialInstant),uint64(0)},"test.occurrence",uint64(1),
   g.Payload[16],uint64(0),origin,uint64(0),uint64(0),recurrence,[]any{uint64(0)}}
  dg,_,err:=d.Sum("cote.csf.digest.occurrence-unit","cote.csf.schema.scheduled-occurrence.body",1,fields)
  if err!=nil{t.Fatal(err)}
  raw,err:=codec.CanonicalPayload(append(cloneAny(fields),dg[:]));if err!=nil{t.Fatal(err)}
  if err:=store.SeedGenesisOccurrence(ctx,domain.ID(id),g.RunID,
   domain.EligibilityCoordinate{NotBeforeInstant:g.InitialInstant},raw);err!=nil{t.Fatal(err)}
 }
 ledger,err:=inputledger.New(ctx,g.RunID,bundle,store);if err!=nil{t.Fatal(err)}
 roles,err:=policiesByRole(bundle);if err!=nil{t.Fatal(err)}
 policies:=admission.Policies{AdmissionOrder:roles["admission-order"],
  CausalIdentity:roles["causal-identity"],Fence:roles["admission-fence"],Rules:[][]any{}}
 c:=New(store,bundle,policies)
 d:=identity.New(bundle)
 cid,_,err:=d.Sum("cote.csf.id.cycle","cote.csf.schema.cycle-id-preimage",1,
  []any{g.RunID[:],[]any{int64(g.InitialInstant),uint64(0)}});if err!=nil{t.Fatal(err)}
 _,hash,err:=store.LoadWorldState(ctx,0);if err!=nil{t.Fatal(err)}
 cycle:=admission.Cycle{ID:domain.ID(cid),Instant:g.InitialInstant,Ordinal:0,
  BaseRevision:0,BaseStateHash:domain.Digest(hash),RetryRef:[]any{uint64(0)}}
 fence,err:=admission.NewCoordinator(admission.NewFenceLog(store)).AdmitThenEvaluate(
  ctx,bundle,ledger,cycle,policies,nil);if err!=nil{t.Fatal(err)}
 return c,store,ledger,cycle,fence,path
}
func TestSystemUnitRejectAndDeferAreAtomicAndReplayable(t *testing.T){
 for _,kind:=range []string{"reject","defer"}{t.Run(kind,func(t *testing.T){
  ctx:=context.Background()
  c,store,ledger,cycle,fence,path:=systemWorkFixture(t,kind)
  if n:=len(fence.Fields[11].([]any));n!=1{t.Fatalf("unexpected admitted source count %d",n)}
  _,_,_,_,_,run:=ledger.Snapshot()
  initial,err:=store.LoadClock(ctx,[32]byte(run));if err!=nil{t.Fatal(err)}
  for fail:=uint8(1);fail<=7;fail++{
   c.failAfterStep=fail
   if _,err:=c.CommitSystemUnit(ctx,ledger,cycle,fence);err==nil{t.Fatalf("system crash point %d returned success",fail)}
   current,err:=store.LoadClock(ctx,[32]byte(run))
   if err!=nil||current!=initial{t.Fatalf("system crash left half clock at %d",fail)}
  }
  c.failAfterStep=0
  committed,err:=c.CommitSystemUnit(ctx,ledger,cycle,fence);if err!=nil{t.Fatal(err)}
  if len(committed.Canonical)==0{t.Fatal("system CycleCommit bytes missing")}
  current,err:=store.LoadClock(ctx,[32]byte(run))
  if err!=nil||current.Revision!=1||current.NextLogicalSequence!=1{t.Fatalf("system successor invalid: %+v %v",current,err)}
  pending,err:=store.PendingClock(ctx,[32]byte(run));if err!=nil{t.Fatal(err)}
  plan,exists,err:=clock.Next(current,pending)
  if err!=nil{t.Fatal(err)}
  if kind=="reject"&&exists{t.Fatal("REJECT left original occurrence eligible")}
  if kind=="defer"&&(!exists||plan.Kind!=clock.ClockAdvance||plan.To<=current.Instant){
   t.Fatal("DEFER failed to produce independent future successor",plan,exists)
  }
  reopened,err:=sqlite.Open(ctx,path);if err!=nil{t.Fatal(err)}
  report,err:=reopened.ReplayClock(ctx,[32]byte(run),c.bundle)
  reopened.Close()
  if err!=nil||report.ReplayedCommits!=1||report.ReplayedEvents!=1||
   report.ReconstructedClock!=current{
   t.Fatalf("system %s not replayable: %+v %v",kind,report,err)
  }
 })}
}
func TestPendingTriggerActivationCommitsThroughSameTerminalAuthority(t *testing.T){
 ctx:=context.Background()
 c,store,ledger,cycle,fence,path:=systemWorkFixture(t,"trigger")
 entries:=fence.Fields[11].([]any)
 if len(entries)!=1||entries[0].([]any)[1]!=uint64(3){t.Fatal("pending trigger omitted from fence")}
 _,_,_,_,_,run:=ledger.Snapshot()
 journal,err:=c.CommitSystemUnit(ctx,ledger,cycle,fence)
 if err!=nil{t.Fatal(err)}
 state,err:=store.LoadClock(ctx,[32]byte(run))
 if err!=nil||state.Revision!=1||state.NextLogicalSequence!=1{t.Fatal("trigger not terminal",err)}
 pending,err:=store.PendingClock(ctx,[32]byte(run))
 if err!=nil||len(pending)!=0{t.Fatal("trigger activation remained pending",err)}
 db,err:=sql.Open("sqlite","file:"+path);if err!=nil{t.Fatal(err)}
 var events,commits,decisions int
 for _,item:=range []struct{table string;count *int}{
  {"event_store",&events},{"committed_cycles",&commits},{"decision_ledger",&decisions},
 }{
  if err:=db.QueryRowContext(ctx,"SELECT count(*) FROM "+item.table).Scan(item.count);err!=nil{t.Fatal(err)}
 }
 db.Close()
 if events!=1||commits!=1||decisions!=4{t.Fatalf("trigger incomplete atomic terminal: events %d, commits %d, decisions %d",events,commits,decisions)}
 reopened,err:=sqlite.Open(ctx,path);if err!=nil{t.Fatal(err)}
 report,err:=reopened.ReplayClock(ctx,[32]byte(run),c.bundle)
 reopened.Close()
 if err!=nil||report.ReplayedCommits!=1||report.ReplayedEvents!=1{
  t.Fatalf("trigger lifecycle cannot replay: %+v %v",report,err)
 }
 if bytes.Equal(journal.Canonical,[]byte{0x80}){t.Fatal("noncanonical trigger CycleCommit")}
}

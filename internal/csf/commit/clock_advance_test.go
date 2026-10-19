package commit

import (
 "bytes"
 "context"
 "database/sql"
 "encoding/hex"
 "encoding/json"
 "os"
 "testing"
 _ "modernc.org/sqlite"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/clock"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/genesis"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/inputledger"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/trigger"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/admission"
)

const bundlePath="../../../docs/architecture/canonical-codec-v1-bundle"

func validGenesis(t *testing.T,b *policy.Bundle)*genesis.Genesis{
 t.Helper()
 data,err:=os.ReadFile(bundlePath+"/fixtures.json")
 if err!=nil{t.Fatal(err)}
 var fixtures struct{Semantic []struct{CaseID string `json:"case_id"`;GenesisEnvelopeHex string `json:"genesis_envelope_hex"`} `json:"semantic"`}
 if err:=json.Unmarshal(data,&fixtures);err!=nil{t.Fatal(err)}
 for _,item:=range fixtures.Semantic{
  if item.CaseID!="genesis.pinning"{continue}
  encoded,err:=hex.DecodeString(item.GenesisEnvelopeHex);if err!=nil{t.Fatal(err)}
  env,err:=codec.StrictDecodeEnvelope(encoded);if err!=nil{t.Fatal(err)}
  p:=env.Value.([]any)
  p[15]=[]any{[]any{"cote.csf.schema.snapshot-pending-state",uint64(1)}}
  state:=make([]any,10);for i:=range state{state[i]=[]any{}}
  stateEnv,err:=codec.CanonicalBytes("cote.csf.snapshot.pending-state","cote.csf.schema.snapshot-pending-state",1,state)
  if err!=nil{t.Fatal(err)}
  digest:=identity.OpaqueContentHash(stateEnv)
  p[16]=[]any{"cote.csf.schema.snapshot-pending-state",uint64(1),stateEnv,digest[:]}
  encoded,err=codec.CanonicalBytes("cote.csf.digest.genesis-manifest",env.SchemaID,env.SchemaVersion,p)
  if err!=nil{t.Fatal(err)}
  g,err:=genesis.ValidateCanonical(encoded,b)
  if err!=nil{t.Fatal(err)}
  return g
 }
 t.Fatal("canonical genesis fixture missing")
 return nil
}

func testFixture(t *testing.T,definitions ...trigger.Definition)(*Coordinator,*sqlite.Store,*inputledger.Ledger,domain.ID,*policy.Bundle,string) {
 t.Helper()
 ctx:=context.Background()
 bundle,err:=policy.Load(bundlePath);if err!=nil{t.Fatal(err)}
 g:=validGenesis(t,bundle)
 path:=t.TempDir()+"/clock.db"
 store,err:=sqlite.Open(ctx,path);if err!=nil{t.Fatal(err)}
 t.Cleanup(func(){store.Close()})
 if err:=store.PersistGenesis(ctx,g,bundle);err!=nil{t.Fatal(err)}
 if err:=store.BootstrapClock(ctx,g,bundle);err!=nil{t.Fatal(err)}
 d:=identity.New(bundle)
 due:=g.InitialInstant+100
 for i:=range definitions {definitions[i].PredicateThreshold=due}
 if err:=store.BootstrapGenesisTriggers(ctx,g,bundle,definitions);err!=nil{t.Fatal(err)}
 origin:=[]any{uint64(2),[]any{g.ID[:],g.Digest[:]}}
 occID,_,err:=d.Sum("cote.csf.id.occurrence","cote.csf.schema.occurrence-id-preimage",1,[]any{origin,uint64(0),uint64(0)})
 if err!=nil{t.Fatal(err)}
 fields:=[]any{occID[:],g.RunID[:],"schedule",int64(due),[]any{int64(due),uint64(1)},"test.occurrence",uint64(1),
 g.Payload[16],uint64(0),origin,uint64(0),uint64(0),[]any{uint64(0)},[]any{uint64(0)}}
 dig,_,err:=d.Sum("cote.csf.digest.occurrence-unit","cote.csf.schema.scheduled-occurrence.body",1,fields)
 if err!=nil{t.Fatal(err)}
 raw,err:=codec.CanonicalPayload(append(fields,dig[:]));if err!=nil{t.Fatal(err)}
 if err:=store.SeedGenesisOccurrence(ctx,domain.ID(occID),g.RunID,domain.EligibilityCoordinate{NotBeforeInstant:due,NotBeforeCycleOrdinal:1},raw);err!=nil{t.Fatal(err)}
 ledger,err:=inputledger.New(ctx,g.RunID,bundle,store);if err!=nil{t.Fatal(err)}
 if err:=ledger.DeclareSource(inputledger.Source{ID:"driver"});err!=nil{t.Fatal(err)}
 closedAt:=domain.EligibilityCoordinate{NotBeforeInstant:due,NotBeforeCycleOrdinal:1}
 if _,err:=ledger.CloseSource(ctx,"driver",inputledger.Closure{ProducerKey:"closed-through-next",ClosedThrough:closedAt});err!=nil{t.Fatal(err)}
 roles,err:=policiesByRole(bundle);if err!=nil{t.Fatal(err)}
 policies:=admission.Policies{AdmissionOrder:roles["admission-order"],CausalIdentity:roles["causal-identity"],Fence:roles["admission-fence"],Rules:[][]any{}}
 return New(store,bundle,policies),store,ledger,g.RunID,bundle,path
}

func TestClockAdvancePublishesCycleAndPendingActivation(t *testing.T){
 ctx:=context.Background()
 var triggerID domain.ID;triggerID[0]=0x80
 definition:=trigger.Definition{ID:triggerID,Version:1,Policy:trigger.RisingEdge,FireOnInitialTrue:true,Dependencies:[]string{"clock"}}
 coordinator,store,ledger,runID,_,_:=testFixture(t,definition)
 before,err:=store.LoadClock(ctx,[32]byte(runID));if err!=nil{t.Fatal(err)}
 result,err:=coordinator.Advance(ctx,ledger,runID)
 if err!=nil{t.Fatal(err)}
 if result.Plan.Kind!=clock.ClockAdvance||result.ResultClock.Instant<=before.Instant||result.ResultClock.LastOrdinal!=-1{t.Fatal(result.Plan,result.ResultClock)}
 if len(result.Events)!=4||len(result.Activations)!=1{t.Fatalf("events=%d activations=%d",len(result.Events),len(result.Activations))}
 if result.Commit.Identity==([32]byte{}){t.Fatal("zero commit id")}
 again,err:=store.LoadClock(ctx,[32]byte(runID));if err!=nil{t.Fatal(err)}
 if again!=result.ResultClock||again.Revision!=1||again.NextLogicalSequence!=4{t.Fatal(again)}
 pending,err:=store.PendingClock(ctx,[32]byte(runID));if err!=nil{t.Fatal(err)}
 next,ok,err:=clock.Next(again,pending)
 if err!=nil||!ok||next.Kind!=clock.Work||next.OriginOrdinal!=0 {t.Fatal(next,err)}
 if _,err:=coordinator.Advance(ctx,ledger,runID);err==nil {t.Fatal("advance repeated while WORK activation is pending")}
 fence,err:=store.GetFence(ctx,result.Fence.ID)
 if err!=nil||!bytes.Equal(fence,result.Fence.Canonical){t.Fatal("fence changed")}
}

func activeTrigger() trigger.Definition {
 var id domain.ID;id[0]=0x81
 d:=trigger.Definition{ID:id,Version:1,Policy:trigger.RisingEdge,FireOnInitialTrue:true,Dependencies:[]string{"clock"}}
 r:=trigger.Runtime{DefinitionID:id,Version:1,Armed:true}
 _=r
 return d
}

// A failure anywhere inside the one SQLite transaction must roll back every
// authoritative table, including the journal (decision, not input ledger).
// Fence publication precedes the commit transaction by design and remains.
func TestAdvanceAtomicRollbackAtEveryWriteBoundary(t *testing.T){
 for step:=uint8(1);step<=6;step++ {
  t.Run(string(rune('0'+step)),func(t *testing.T) {
   ctx:=context.Background()
   coordinator,store,ledger,runID,_,_:=testFixture(t,activeTrigger())
   initial,err:=store.LoadClock(ctx,[32]byte(runID));if err!=nil{t.Fatal(err)}
   before,err:=store.AuditClock(ctx,[32]byte(runID));if err!=nil{t.Fatal(err)}
   coordinator.failAfterStep=step
   if _,err:=coordinator.Advance(ctx,ledger,runID);err==nil{t.Fatal("injected failure was ignored")}
   state,err:=store.LoadClock(ctx,[32]byte(runID));if err!=nil||state!=initial{t.Fatalf("clock changed after rollback: %+v %v",state,err)}
   after,err:=store.AuditClock(ctx,[32]byte(runID));if err!=nil{t.Fatal(err)}
   if after!=before {t.Fatalf("incomplete transaction leaked to authorities: before=%+v after=%+v",before,after)}
   // The persisted fence is not a terminal commit: repeat from the same
   // closed cohort after simulated process interruption.
   coordinator.failAfterStep=0
   result,err:=coordinator.Advance(ctx,ledger,runID)
   if err!=nil{t.Fatalf("retry after rollback: %v",err)}
   audit,err:=store.AuditClock(ctx,[32]byte(runID));if err!=nil{t.Fatal(err)}
   if audit.CommitCount!=1||audit.EventCount!=4||audit.RuntimeCount!=1||audit.ActivationCount!=1||audit.RevisionCount!=2||audit.InputCount!=before.InputCount{
    t.Fatalf("incomplete committed state: %+v",audit)
   }
   journal,err:=store.ReadCycleCommit(ctx,result.Commit.Identity)
   if err!=nil||!bytes.Equal(journal,result.Commit.Canonical){t.Fatal("CycleCommit missing from decision ledger")}
   event,err:=store.ReadClockEvent(ctx,result.Events[0].Identity)
   if err!=nil||!bytes.Equal(event,result.Events[0].Canonical){t.Fatal("clock event missing from event store")}
  })
 }
}

func TestAdvanceReopenAndWorkerPermutationPreserveAuthoritativeBytes(t *testing.T) {
 ctx:=context.Background()
 var baselineJournal,baselineEvent,baselineActivation []byte
 for iteration:=0;iteration<2;iteration++{
  one:=activeTrigger()
  two:=activeTrigger()
  two.ID[0]=0x82
  work:=[]trigger.Definition{one,two}
  if iteration==1{work[0],work[1]=work[1],work[0]}
  coordinator,store,ledger,runID,bundle,path:=testFixture(t,work...)
  result,err:=coordinator.Advance(ctx,ledger,runID)
  if err!=nil{t.Fatal(err)}
  if len(result.Events)!=6||len(result.Activations)!=2{t.Fatal("missing trigger activations")}
  if iteration==0 {
   baselineJournal=append([]byte(nil),result.Commit.Canonical...)
   baselineEvent=append([]byte(nil),result.Events[0].Canonical...)
   baselineActivation=append([]byte(nil),result.Activations[0].Canonical...)
  }else{
   if !bytes.Equal(baselineJournal,result.Commit.Canonical)||!bytes.Equal(baselineEvent,result.Events[0].Canonical)||!bytes.Equal(baselineActivation,result.Activations[0].Canonical){
    t.Fatal("worker completion order changed canonical bytes")
   }
  }
  reopened,err:=sqlite.Open(ctx,path);if err!=nil{t.Fatal(err)}
  state,err:=reopened.LoadClock(ctx,[32]byte(runID));if err!=nil||state!=result.ResultClock{reopened.Close();t.Fatal("clock did not survive reopen")}
  actualJournal,err:=reopened.ReadCycleCommit(ctx,result.Commit.Identity)
  if err!=nil||!bytes.Equal(actualJournal,result.Commit.Canonical){reopened.Close();t.Fatal("commit did not survive reopen")}
  actualEvent,err:=reopened.ReadClockEvent(ctx,result.Events[0].Identity)
  if err!=nil||!bytes.Equal(actualEvent,result.Events[0].Canonical){reopened.Close();t.Fatal("event did not survive reopen")}
  pending,err:=reopened.PendingClock(ctx,[32]byte(runID))
  if err!=nil||len(pending)!=3 {reopened.Close();t.Fatalf("lost pending trigger/occurrence: %v %+v",err,pending)}
  report,err:=reopened.ReplayClock(ctx,[32]byte(runID),bundle)
  if err!=nil{reopened.Close();t.Fatalf("durable-only replay failed: %v",err)}
  if report.ReconstructedClock!=result.ResultClock || report.ReplayedCommits!=1 ||
    report.ReplayedEvents!=6 || report.ReplayedTriggerTransitions!=2 || report.VerifiedActivations!=2 {
    reopened.Close();t.Fatalf("incomplete replay: %+v",report)
  }
  reopened.Close()
  _=store
 }
}

// A post-fence Q <= the advance destination must be rejected at the input
// authority, or the same cycle/fence identity becomes permanently un-retryable.
func TestPostFenceEarlierRoundIsRejectedAndClockCanCommit(t *testing.T){
 ctx:=context.Background()
 coordinator,store,ledger,runID,_,_:=testFixture(t)
 state,err:=store.LoadClock(ctx,[32]byte(runID));if err!=nil{t.Fatal(err)}
 var attempted bool
 var rejected error
 coordinator.beforeCommit=func(){
  attempted=true
  g:=validGenesis(t,coordinator.bundle)
  origin:=[]any{uint64(2),[]any{g.ID[:],g.Digest[:]}}
  q:=domain.EligibilityCoordinate{NotBeforeInstant:state.Instant+50,NotBeforeCycleOrdinal:0}
  _,rejected=ledger.DeclareRound(ctx,"schedule",[2]string{"school","actor-a"},[]string{"decision"},q,origin,0,57,0,[]any{uint64(0)})
 }
 result,err:=coordinator.Advance(ctx,ledger,runID)
 if err!=nil||!attempted||rejected==nil{t.Fatalf("post-fence Q must fail while the original advance commits: attempt=%v rejected=%v outcome=%+v commit=%v",attempted,rejected,result,err)}
 audit,err:=store.AuditClock(ctx,[32]byte(runID));if err!=nil{t.Fatal(err)}
 if audit.CommitCount!=1||audit.RevisionCount!=2||audit.EventCount!=2{t.Fatalf("fenced advance was stranded: %+v",audit)}
 pending,err:=store.PendingClock(ctx,[32]byte(runID));if err!=nil{t.Fatal(err)}
 next,has,err:=clock.Next(result.ResultClock,pending)
 if err!=nil||!has||next.Kind!=clock.Work{t.Fatalf("clock did not resume at admitted P: %+v %v",next,err)}
}

// Q arriving BEFORE the admission fence must change the minimum legitimately.
func TestEarlierRoundBeforeFenceBecomesClockTarget(t *testing.T){
 ctx:=context.Background()
 coordinator,store,ledger,runID,bundle,_:=testFixture(t)
 state,err:=store.LoadClock(ctx,[32]byte(runID));if err!=nil{t.Fatal(err)}
 g:=validGenesis(t,bundle)
 origin:=[]any{uint64(2),[]any{g.ID[:],g.Digest[:]}}
 q:=domain.EligibilityCoordinate{NotBeforeInstant:state.Instant+50,NotBeforeCycleOrdinal:0}
 if _,err:=ledger.DeclareRound(ctx,"schedule",[2]string{"school","actor-a"},[]string{"decision"},q,origin,0,57,0,[]any{uint64(0)});err!=nil{t.Fatal(err)}
 advance,err:=coordinator.Advance(ctx,ledger,runID)
 if err!=nil||advance.ResultClock.Instant!=q.NotBeforeInstant{t.Fatalf("pre-fence Q did not determine target: %+v %v",advance,err)}
}

func TestRepeatCadenceIsPersistedAsFuturePendingWork(t *testing.T){
 ctx:=context.Background()
 w:=activeTrigger()
 w.Policy=trigger.RepeatWhileTrue
 w.FireOnInitialTrue=false
 w.RepeatEvery=25
 coordinator,store,ledger,runID,_,_:=testFixture(t,w)
 outcome,err:=coordinator.Advance(ctx,ledger,runID)
 if err!=nil{t.Fatal(err)}
 if len(outcome.Activations)!=1{t.Fatal("repeat did not activate at initial true")}
 pending,err:=store.PendingClock(ctx,[32]byte(runID));if err!=nil{t.Fatal(err)}
 var hasCadence bool
 for _,item:=range pending{
  if item.ID==w.ID {
   hasCadence=true
   if item.Eligibility.NotBeforeInstant!=outcome.ResultClock.Instant+25 {t.Fatalf("repeat cadence was not durable: %+v",item)}
  }
 }
 if !hasCadence{t.Fatal("persisted repeat runtime did not contribute to pending minimum")}
}

func TestSQLiteRejectsTamperedClockBatchWithoutPartialState(t *testing.T) {
 tests:=[]struct{
  name string
  alter func(t *testing.T,b *ports.ClockAdvanceBatch)
 }{
  {"journal-batch-digest",func(t *testing.T,b *ports.ClockAdvanceBatch){
    decoded,err:=codec.StrictDecodePayload(b.Journal.Canonical);if err!=nil{t.Fatal(err)}
    fields:=decoded.([]any);fields[23]=make([]byte,32)
    b.Journal.Canonical,err=codec.CanonicalPayload(fields);if err!=nil{t.Fatal(err)}
  }},
  {"event-content-digest",func(t *testing.T,b *ports.ClockAdvanceBatch){
    decoded,err:=codec.StrictDecodePayload(b.Events[0].Canonical);if err!=nil{t.Fatal(err)}
    fields:=decoded.([]any);fields[18]=make([]byte,32)
    b.Events[0].Canonical,err=codec.CanonicalPayload(fields);if err!=nil{t.Fatal(err)}
  }},
  {"activation-provenance",func(t *testing.T,b *ports.ClockAdvanceBatch){
    if len(b.Activations)==0{t.Fatal("missing activation")}
    decoded,err:=codec.StrictDecodePayload(b.Activations[0].Canonical);if err!=nil{t.Fatal(err)}
    fields:=decoded.([]any);fields[2]="trigger:forged-source"
    b.Activations[0].Canonical,err=codec.CanonicalPayload(fields);if err!=nil{t.Fatal(err)}
  }},
  {"missing-registered-trigger",func(t *testing.T,b *ports.ClockAdvanceBatch){ b.TriggerRuntime=nil }},
   {"omitted-due-activation",func(t *testing.T,b *ports.ClockAdvanceBatch){ b.Activations=nil }},
   {"modified-trigger-runtime",func(t *testing.T,b *ports.ClockAdvanceBatch){
      decoded,err:=codec.StrictDecodePayload(b.TriggerRuntime[0].After);if err!=nil{t.Fatal(err)}
      fields:=decoded.([]any);fields[5]=uint64(99)
      b.TriggerRuntime[0].After,err=codec.CanonicalPayload(fields);if err!=nil{t.Fatal(err)}
   }},
   {"changed-immutable-definition",func(t *testing.T,b *ports.ClockAdvanceBatch){
      decoded,err:=codec.StrictDecodePayload(b.TriggerRuntime[0].DefinitionCanonical);if err!=nil{t.Fatal(err)}
      fields:=decoded.([]any);fields[4]=false
      b.TriggerRuntime[0].DefinitionCanonical,err=codec.CanonicalPayload(fields);if err!=nil{t.Fatal(err)}
   }},
   {"revision-hash",func(t *testing.T,b *ports.ClockAdvanceBatch){
    decoded,err:=codec.StrictDecodePayload(b.RevisionCanonical);if err!=nil{t.Fatal(err)}
    fields:=decoded.([]any);fields[5]=make([]byte,32)
    b.RevisionCanonical,err=codec.CanonicalPayload(fields);if err!=nil{t.Fatal(err)}
  }},
 }
 for _,test:=range tests {
  t.Run(test.name,func(t *testing.T){
   ctx:=context.Background()
   coordinator,store,ledger,runID,_,_:=testFixture(t,activeTrigger())
   before,err:=store.AuditClock(ctx,[32]byte(runID));if err!=nil{t.Fatal(err)}
   coordinator.tamperBatch=func(b *ports.ClockAdvanceBatch){test.alter(t,b)}
   if _,err:=coordinator.Advance(ctx,ledger,runID);err==nil {
    t.Fatal("corrupted causal batch was accepted")
   }
   after,err:=store.AuditClock(ctx,[32]byte(runID));if err!=nil{t.Fatal(err)}
   if after!=before{t.Fatalf("corrupted batch leaked partial publication: before=%+v after=%+v",before,after)}
  })
 }
}

func TestGenesisBoundRegistryCannotChangeAndNeverNeedsHostTriggerWork(t *testing.T) {
 ctx:=context.Background()
 def:=activeTrigger()
 coord,store,ledger,runID,bundle,_:=testFixture(t,def)
 records,err:=store.LoadRegisteredTriggers(ctx,[32]byte(runID),bundle)
 if err!=nil||len(records)!=1||records[0].Definition.ID!=def.ID ||len(records[0].RuntimeCanonical)==0{
  t.Fatalf("registered trigger is not durable: %v %+v",err,records)
 }
 root:=validGenesis(t,bundle)
 changed:=def
 changed.PredicateThreshold+=1
 if err:=store.BootstrapGenesisTriggers(ctx,root,bundle,[]trigger.Definition{changed});err==nil{
  t.Fatal("same genesis admitted modified trigger definition")
 }
 before,err:=store.ReplayClock(ctx,[32]byte(runID),bundle)
 if err!=nil||before.ReplayedCommits!=0||before.ReplayedTriggerTransitions!=0{
  t.Fatalf("genesis-only trigger replay invalid: %+v %v",before,err)
 }
 // No supplied definitions/runtime on invocation. The persisted registry is
 // the only source from which the coordinator can evaluate predicates.
 outcome,err:=coord.Advance(ctx,ledger,runID)
 if err!=nil||len(outcome.Activations)!=1{t.Fatalf("registered trigger not executed: %v",err)}
 after,err:=store.ReplayClock(ctx,[32]byte(runID),bundle)
 if err!=nil||after.ReplayedTriggerTransitions!=1||after.VerifiedActivations!=1{
  t.Fatalf("durable registry replay failed: %+v %v",after,err)
 }
}

func TestSyntheticBodyDependencyHasSeparateCanonicalCause(t *testing.T) {
 ctx:=context.Background()
 clockDef:=activeTrigger()
 bodyDef:=activeTrigger()
 bodyDef.ID[0]=0x83
 bodyDef.Dependencies=[]string{"body"}
 coord,store,ledger,runID,bundle,_:=testFixture(t,clockDef,bodyDef)
 result,err:=coord.Advance(ctx,ledger,runID)
 if err!=nil{t.Fatal(err)}
 if len(result.Events)!=6||len(result.Activations)!=2{t.Fatal("missing synthetic events, lifecycle events or triggered activations")}
 expected:=map[domain.ID]domain.ID{
  clockDef.ID:domain.ID(result.Events[0].Identity),
  bodyDef.ID:domain.ID(result.Events[1].Identity),
 }
 for _,activation:=range result.Activations {
  raw,err:=codec.StrictDecodePayload(activation.Canonical)
  if err!=nil{t.Fatal(err)}
  fields:=raw.([]any)
  rawDefinition:=fields[3].([]byte)
  var defID domain.ID
  copy(defID[:],rawDefinition)
  origin:=fields[5].([]any)
  causing:=origin[3].([]any)
  if len(causing)!=1||!bytes.Equal(causing[0].([]byte),func()[]byte{id:=expected[defID];return id[:]}()){
   t.Fatalf("wrong synthetic dependency cause for %x: %#v",defID,causing)
  }
 }
 replay,err:=store.ReplayClock(ctx,[32]byte(runID),bundle)
 if err!=nil||replay.VerifiedActivations!=2{
  t.Fatalf("synthetic footprint replay diverged: %+v %v",replay,err)
 }
}

func TestRearmRequiresExplicitCausalProvenance(t *testing.T){
 d:=activeTrigger()
 d.Policy=trigger.OnceWhenTrue
 exhausted:=trigger.Runtime{DefinitionID:d.ID,Version:1,Exhausted:true,HasLast:true,Last:true,ActivationCount:1}
 if _,err:=trigger.RearmPure(d,exhausted,domain.ID{});err==nil{
  t.Fatal("an uncaused rearm was allowed")
 }
 var event domain.ID
 event[0]=19
 changed,err:=trigger.RearmPure(d,exhausted,event)
 if err!=nil||changed.Exhausted||!changed.Armed||changed.HasLast||changed.ActivationCount!=exhausted.ActivationCount{
  t.Fatalf("rearm transition invalid: %+v %v",changed,err)
 }
 // RearmPure does NOT persist anything: a WORK CycleCommit with the causal
 // lifecycle event is required to publish it.
}

func TestInitiallyTrueTriggerIsBootstrappedAtGenesisAndReplayed(t *testing.T) {
 ctx:=context.Background()
 bundle,err:=policy.Load(bundlePath);if err!=nil{t.Fatal(err)}
 root:=validGenesis(t,bundle)
 store,err:=sqlite.Open(ctx,t.TempDir()+"/initial-trigger.db");if err!=nil{t.Fatal(err)};defer store.Close()
 if err:=store.PersistGenesis(ctx,root,bundle);err!=nil{t.Fatal(err)}
 if err:=store.BootstrapClock(ctx,root,bundle);err!=nil{t.Fatal(err)}
 d:=activeTrigger()
 d.PredicateThreshold=root.InitialInstant-1
 d.FireOnInitialTrue=true
 if err:=store.BootstrapGenesisTriggers(ctx,root,bundle,[]trigger.Definition{d});err!=nil{t.Fatal(err)}
 pending,err:=store.PendingClock(ctx,[32]byte(root.RunID));if err!=nil{t.Fatal(err)}
 if len(pending)!=1||pending[0].Eligibility.NotBeforeInstant!=root.InitialInstant||pending[0].Eligibility.NotBeforeCycleOrdinal!=0 {
  t.Fatalf("initially true trigger was not committed to genesis pending state: %+v",pending)
 }
 replay,err:=store.ReplayClock(ctx,[32]byte(root.RunID),bundle)
 if err!=nil||replay.VerifiedActivations!=1||replay.ReplayedCommits!=0{
  t.Fatalf("initial activation not reconstructible from persisted registry: %+v %v",replay,err)
 }
 initial,err:=store.LoadClock(ctx,[32]byte(root.RunID));if err!=nil{t.Fatal(err)}
 p,ok,err:=clock.Next(initial,pending)
 if err!=nil||!ok||p.Kind!=clock.Work||p.OriginOrdinal!=0{t.Fatalf("genesis trigger should be WORK (t,0): %+v %v",p,err)}
}

func TestClockAdvanceCannotSmuggleUncommittedManualRearm(t *testing.T) {
 ctx:=context.Background()
 def:=activeTrigger()
 def.Policy=trigger.OnceWhenTrue
 coordinator,store,ledger,runID,_,_:=testFixture(t,def)
 before,err:=store.AuditClock(ctx,[32]byte(runID));if err!=nil{t.Fatal(err)}
 coordinator.tamperBatch=func(b *ports.ClockAdvanceBatch){
  if len(b.TriggerRuntime)!=1{t.Fatal("one trigger expected")}
  next,err:=trigger.DecodeRuntime(b.TriggerRuntime[0].After)
  if err!=nil{t.Fatal(err)}
  if !next.Exhausted {t.Fatal("ONCE predicate should be exhausted")}
  forged:=next
  forged.Exhausted=false
  forged.Armed=true
  forged.HasLast=false
  b.TriggerRuntime[0].After,err=trigger.RuntimeBytes(forged)
  if err!=nil{t.Fatal(err)}
 }
 if _,err:=coordinator.Advance(ctx,ledger,runID);err==nil {
  t.Fatal("CLOCK_ADVANCE accepted a manual rearm without WORK event")
 }
 after,err:=store.AuditClock(ctx,[32]byte(runID));if err!=nil{t.Fatal(err)}
 if after!=before {t.Fatalf("manual rearm polluted causal tables: before=%+v after=%+v",before,after)}
}

// A malicious or damaged SQLite row must be detected by replay from
// genesis, immutable definitions and journal. This test intentionally
// corrupts disposable test databases; no production write port is exposed.
func TestReplayRejectsCorruptedDurableAuthority(t *testing.T) {
 tests:=[]struct{
  name string
  statement string
  key func(Outcome,[32]byte)[]byte
 }{
  {"definition","UPDATE trigger_definitions SET canonical=? WHERE run_id=?",
   func(_ Outcome,run [32]byte)[]byte{return run[:]}},
  {"runtime","UPDATE trigger_runtime SET canonical=? WHERE run_id=?",
   func(_ Outcome,run [32]byte)[]byte{return run[:]}},
  {"activation","UPDATE trigger_activations SET canonical=? WHERE run_id=?",
   func(_ Outcome,run [32]byte)[]byte{return run[:]}},
  {"journal","UPDATE decision_ledger SET canonical=? WHERE id=?",
   func(o Outcome,_ [32]byte)[]byte{return o.Commit.Identity[:]}},
  {"event","UPDATE event_store SET canonical=? WHERE id=?",
   func(o Outcome,_ [32]byte)[]byte{return o.Events[0].Identity[:]}},
  {"clock","UPDATE clock_states SET canonical=? WHERE run_id=?",
   func(_ Outcome,run [32]byte)[]byte{return run[:]}},
 }
 for _,tc:=range tests{
  t.Run(tc.name,func(t *testing.T){
   ctx:=context.Background()
   coordinator,_,ledger,run,bundle,path:=testFixture(t,activeTrigger())
   o,err:=coordinator.Advance(ctx,ledger,run)
   if err!=nil{t.Fatal(err)}
   // The unmodified persisted state must be fully replayable first.
   clean,err:=sqlite.Open(ctx,path)
   if err!=nil{t.Fatal(err)}
   _,err=clean.ReplayClock(ctx,[32]byte(run),bundle)
   clean.Close()
   if err!=nil{t.Fatalf("precondition replay failure: %v",err)}
   db,err:=sql.Open("sqlite",path);if err!=nil{t.Fatal(err)}
   result,err:=db.ExecContext(ctx,tc.statement,[]byte{0x80},tc.key(o,[32]byte(run)))
   db.Close()
   if err!=nil{t.Fatal(err)}
   count,err:=result.RowsAffected()
   if err!=nil||count!=1{t.Fatalf("expected one corrupted authority row, got %d: %v",count,err)}
   reopened,err:=sqlite.Open(ctx,path);if err!=nil{t.Fatal(err)}
   _,err=reopened.ReplayClock(ctx,[32]byte(run),bundle)
   reopened.Close()
   if err==nil{t.Fatal("replay accepted corrupted durable causal authority")}
  })
 }
}

func TestReplayRejectsWorldRevisionHashNotAnchoredToGenesis(t *testing.T){
 ctx:=context.Background()
 coordinator,_,ledger,runID,bundle,path:=testFixture(t,activeTrigger())
 if _,err:=coordinator.Advance(ctx,ledger,runID);err!=nil{t.Fatal(err)}
 db,err:=sql.Open("sqlite",path);if err!=nil{t.Fatal(err)}
 result,err:=db.ExecContext(ctx,"UPDATE world_revisions SET state_hash=? WHERE revision=1",bytes.Repeat([]byte{0xF1},32))
 db.Close()
 if err!=nil{t.Fatal(err)}
 affected,err:=result.RowsAffected();if err!=nil||affected!=1{t.Fatalf("expected one revision change, got %d %v",affected,err)}
 reopened,err:=sqlite.Open(ctx,path);if err!=nil{t.Fatal(err)}
 defer reopened.Close()
 if _,err:=reopened.ReplayClock(ctx,[32]byte(runID),bundle);err==nil{
  t.Fatal("replay accepted world revision hash differing from genesis")
 }
}

// Regression finding #1: runtime and creation are persisted in the same
// CycleCommit event set, after the two pre-trigger temporal events.
func TestTriggerLifecycleEventsBelongToClockCommit(t *testing.T){
 ctx:=context.Background()
 coordinator,store,ledger,run,bundle,_:=testFixture(t,activeTrigger())
 result,err:=coordinator.Advance(ctx,ledger,run);if err!=nil{t.Fatal(err)}
 if len(result.Events)!=4{t.Fatalf("expected 2 temporal + runtime + activation events, got %d",len(result.Events))}
 for i,event:=range result.Events {
  v,err:=codec.StrictDecodePayload(event.Canonical);if err!=nil{t.Fatal(err)}
  fields:=v.([]any);key:=fields[2].([]any)
  want:=uint64(0);if i==2{want=3};if i==3{want=4}
  if key[0]!=want{t.Fatalf("event %d phase %v, want %d",i,key[0],want)}
  if fields[15]!=uint64(2){t.Fatal("lifecycle event improperly perceptible")}
 }
 report,err:=store.ReplayClock(ctx,[32]byte(run),bundle)
 if err!=nil||report.ReplayedEvents!=4||report.ReplayedTriggerTransitions!=1||report.VerifiedActivations!=1{
  t.Fatalf("replay could not reproduce trigger lifecycle: %+v %v",report,err)
 }
}

// Regression finding #2: an index column is not an alternate causal authority.
// Changing only the index must fail both live planning and a persisted replay.
func TestPendingProjectionMismatchFailsClosed(t *testing.T){
 ctx:=context.Background()
 coordinator,store,ledger,run,bundle,path:=testFixture(t,activeTrigger())
 if _,err:=coordinator.Advance(ctx,ledger,run);err!=nil{t.Fatal(err)}
 db,err:=sql.Open("sqlite",path);if err!=nil{t.Fatal(err)}
 if _,err:=db.ExecContext(ctx,"UPDATE trigger_activations SET lifecycle=2 WHERE run_id=?",run[:]);err!=nil{t.Fatal(err)}
 db.Close()
 if _,err:=store.PendingClock(ctx,[32]byte(run));err==nil{t.Fatal("index-only lifecycle corruption hid an activation")}
 opened,err:=sqlite.Open(ctx,path);if err!=nil{t.Fatal(err)};defer opened.Close()
 if _,err:=opened.ReplayClock(ctx,[32]byte(run),bundle);err==nil{t.Fatal("replay overlooked index-only lifecycle corruption")}
}

// Regression finding #3: terminal cycles cannot exist without the admitted
// fence, even when their journal and event digests remain self-consistent.
func TestReplayRejectsMissingPersistedAdmissionFence(t *testing.T){
 ctx:=context.Background()
 coordinator,_,ledger,run,bundle,path:=testFixture(t,activeTrigger())
 outcome,err:=coordinator.Advance(ctx,ledger,run);if err!=nil{t.Fatal(err)}
 db,err:=sql.Open("sqlite",path);if err!=nil{t.Fatal(err)}
 if _,err:=db.ExecContext(ctx,"DELETE FROM decision_ledger WHERE id=?",outcome.Fence.ID[:]);err!=nil{t.Fatal(err)}
 db.Close()
 opened,err:=sqlite.Open(ctx,path);if err!=nil{t.Fatal(err)};defer opened.Close()
 if _,err:=opened.ReplayClock(ctx,[32]byte(run),bundle);err==nil{t.Fatal("replay accepted CycleCommit with missing AdmissionFence")}
}

// Regression finding #4: unlike a broken CBOR byte, this mutation preserves
// the entire record's own recomputed digest and indexed SQL coordinate.
func TestReplayRejectsAuthenticallyRehashedActivationAtWrongCoordinate(t *testing.T){
 ctx:=context.Background()
 coordinator,_,ledger,run,bundle,path:=testFixture(t,activeTrigger())
 outcome,err:=coordinator.Advance(ctx,ledger,run);if err!=nil{t.Fatal(err)}
 if len(outcome.Activations)!=1{t.Fatal("fixture has no activation")}
 id:=outcome.Activations[0].Identity
 fieldsAny,err:=codec.StrictDecodePayload(outcome.Activations[0].Canonical);if err!=nil{t.Fatal(err)}
 fields:=fieldsAny.([]any)
 coord:=fields[6].([]any);to,_:=coord[0].(int64);if to==0{to=int64(outcome.ResultClock.Instant)}
 fields[6]=[]any{to-1,uint64(0)}
 dig,_,err:=identity.New(bundle).Sum("cote.csf.digest.trigger-activation-unit","cote.csf.schema.trigger-activation.body",1,fields[:11]);if err!=nil{t.Fatal(err)}
 fields[11]=dig[:]
 raw,err:=codec.CanonicalPayload(fields);if err!=nil{t.Fatal(err)}
 db,err:=sql.Open("sqlite",path);if err!=nil{t.Fatal(err)}
 if _,err:=db.ExecContext(ctx,"UPDATE trigger_activations SET canonical=?,eligibility_instant=? WHERE id=?",raw,to-1,id[:]);err!=nil{t.Fatal(err)}
 db.Close()
 opened,err:=sqlite.Open(ctx,path);if err!=nil{t.Fatal(err)};defer opened.Close()
 if _,err:=opened.ReplayClock(ctx,[32]byte(run),bundle);err==nil{t.Fatal("replay accepted rehashed activation with forged eligibility")}
}

// Regression finding #5: state hash columns are insufficient to authenticate
// the nested pre/post typed-values persisted in WorldRevision transitions.
func TestReplayRejectsRevisionStateMutationWithUntouchedHash(t *testing.T){
 ctx:=context.Background()
 coordinator,_,ledger,run,bundle,path:=testFixture(t,activeTrigger())
 if _,err:=coordinator.Advance(ctx,ledger,run);err!=nil{t.Fatal(err)}
 db,err:=sql.Open("sqlite",path);if err!=nil{t.Fatal(err)}
 var raw []byte
 if err:=db.QueryRowContext(ctx,"SELECT canonical FROM world_revisions WHERE revision=1").Scan(&raw);err!=nil{t.Fatal(err)}
 v,err:=codec.StrictDecodePayload(raw);if err!=nil{t.Fatal(err)}
 fields:=v.([]any);typed:=append([]any(nil),fields[1].([]any)...)
 typed[0]="cote.csf.schema.tampered-state"
 fields[1]=typed
 changed,err:=codec.CanonicalPayload(fields);if err!=nil{t.Fatal(err)}
 if _,err:=db.ExecContext(ctx,"UPDATE world_revisions SET canonical=? WHERE revision=1",changed);err!=nil{t.Fatal(err)}
 db.Close()
 opened,err:=sqlite.Open(ctx,path);if err!=nil{t.Fatal(err)};defer opened.Close()
 if _,err:=opened.ReplayClock(ctx,[32]byte(run),bundle);err==nil{t.Fatal("replay accepted forged typed world state")}
}

// Regression finding #6: no initial schedule entry may choose its own
// identity or digest just because its indexed eligibility matches.
// Negative occurrence seeding is exercised on genesis-only fixtures to
// distinguish validation failure from a pre-existing fence or duplicate ID.
func TestGenesisOccurrenceRejectsForgedUnitDigestAndID(t *testing.T){
 ctx:=context.Background()
 bundle,err:=policy.Load(bundlePath);if err!=nil{t.Fatal(err)}
 g:=validGenesis(t,bundle)
 store,err:=sqlite.Open(ctx,t.TempDir()+"/seed-validation.db");if err!=nil{t.Fatal(err)};defer store.Close()
 if err:=store.PersistGenesis(ctx,g,bundle);err!=nil{t.Fatal(err)}
 if err:=store.BootstrapClock(ctx,g,bundle);err!=nil{t.Fatal(err)}
 origin:=[]any{uint64(2),[]any{g.ID[:],g.Digest[:]}}
 d:=identity.New(bundle)
 id,_,err:=d.Sum("cote.csf.id.occurrence","cote.csf.schema.occurrence-id-preimage",1,[]any{origin,uint64(0),uint64(0)});if err!=nil{t.Fatal(err)}
 due:=g.InitialInstant+100
 eligibility:=domain.EligibilityCoordinate{NotBeforeInstant:due,NotBeforeCycleOrdinal:0}
 fields:=[]any{id[:],g.RunID[:],"schedule",int64(due),[]any{int64(due),uint64(0)},"seed.test",uint64(1),
  g.Payload[16],uint64(0),origin,uint64(0),uint64(0),[]any{uint64(0)},[]any{uint64(0)}}
 digest,_,err:=d.Sum("cote.csf.digest.occurrence-unit","cote.csf.schema.scheduled-occurrence.body",1,fields);if err!=nil{t.Fatal(err)}
 canonical,err:=codec.CanonicalPayload(append(append([]any(nil),fields...),digest[:]));if err!=nil{t.Fatal(err)}
 corruptDigest:=append([]any(nil),fields...)
 badDigest,err:=codec.CanonicalPayload(append(corruptDigest,bytes.Repeat([]byte{0xab},32)));if err!=nil{t.Fatal(err)}
 if err:=store.SeedGenesisOccurrence(ctx,domain.ID(id),g.RunID,eligibility,badDigest);err==nil{t.Fatal("bootstrap accepted forged occurrence digest")}
 corruptID:=append([]any(nil),fields...)
 forgedID:=id;forgedID[0]^=0x80;corruptID[0]=forgedID[:]
 badID,err:=codec.CanonicalPayload(append(corruptID,digest[:]));if err!=nil{t.Fatal(err)}
 if err:=store.SeedGenesisOccurrence(ctx,domain.ID(forgedID),g.RunID,eligibility,badID);err==nil{t.Fatal("bootstrap accepted occurrence with forged ID")}
 backdated:=append([]any(nil),fields...)
 backdated[3]=int64(g.InitialInstant)-1
 backdated[4]=[]any{int64(g.InitialInstant)-1,uint64(0)}
 digestBackdated,_,err:=d.Sum("cote.csf.digest.occurrence-unit","cote.csf.schema.scheduled-occurrence.body",1,backdated);if err!=nil{t.Fatal(err)}
 retro,err:=codec.CanonicalPayload(append(backdated,digestBackdated[:]));if err!=nil{t.Fatal(err)}
 if err:=store.SeedGenesisOccurrence(ctx,domain.ID(id),g.RunID,domain.EligibilityCoordinate{NotBeforeInstant:g.InitialInstant-1},retro);err==nil{t.Fatal("retroactive pending occurrence was admitted at genesis")}

 if err:=store.SeedGenesisOccurrence(ctx,domain.ID(id),g.RunID,eligibility,canonical);err!=nil{t.Fatalf("valid genesis occurrence rejected: %v",err)}
}

// A self-consistent rehash is required here: simple broken-digest fixtures
// would not exercise the closure/policy/provenance semantic validators.
func TestReplayRejectsRehashedAuthorityForgeries(t *testing.T){
 cases:=[]struct{
  name string
  mutate func(context.Context,*testing.T,*inputledger.Ledger,Outcome,[]any,[]any,[]ports.CanonicalRecord,*policy.Bundle)
 }{
  {"missing-declared-source-closure",func(_ context.Context,_ *testing.T,_ *inputledger.Ledger,_ Outcome,f,_ []any,_ []ports.CanonicalRecord,_ *policy.Bundle){
   f[9]=[]any{}
  }},
  {"later-covering-closure-instead-of-first",func(ctx context.Context,t *testing.T,l *inputledger.Ledger,out Outcome,f,_ []any,_ []ports.CanonicalRecord,_ *policy.Bundle){
   newer,err:=l.CloseSource(ctx,"driver",inputledger.Closure{
    ProducerKey:"later-coverage",ClosedThrough:domain.EligibilityCoordinate{NotBeforeInstant:out.Plan.To+1},
   });if err!=nil{t.Fatal(err)}
   f[9]=[]any{[]any{"driver",[]any{uint64(2),newer.ID[:],newer.Digest[:]},newer.Receipt.Sequence,newer.Fields[4],newer.Fields[5]}}
  }},
  {"different-versioned-policy-ref",func(_ context.Context,_ *testing.T,_ *inputledger.Ledger,_ Outcome,_,j []any,_ []ports.CanonicalRecord,_ *policy.Bundle){
   j[13]=j[14] // event-order is not a cycle-coordinate policy.
  }},
  {"synthetic-event-forged-source-provenance",func(_ context.Context,t *testing.T,_ *inputledger.Ledger,_ Outcome,_,_ []any,events []ports.CanonicalRecord,b *policy.Bundle){
   decoded,err:=codec.StrictDecodePayload(events[0].Canonical);if err!=nil{t.Fatal(err)}
   fields:=decoded.([]any)
   fields[11]=[]any{bytes.Repeat([]byte{0x77},32)}
   digest,_,err:=identity.New(b).Sum("cote.csf.digest.event","cote.csf.schema.event.body",1,fields[:18]);if err!=nil{t.Fatal(err)}
   fields[18]=digest[:]
   events[0].Canonical,err=codec.CanonicalPayload(fields);if err!=nil{t.Fatal(err)}
  }},
 }
 for _,tc:=range cases{
  t.Run(tc.name,func(t *testing.T){
   ctx:=context.Background()
   coordinator,store,ledger,run,bundle,path:=testFixture(t,activeTrigger())
   out,err:=coordinator.Advance(ctx,ledger,run);if err!=nil{t.Fatal(err)}
   fAny,err:=codec.StrictDecodePayload(out.Fence.Canonical);if err!=nil{t.Fatal(err)}
   fence:=fAny.([]any)
   jAny,err:=codec.StrictDecodePayload(out.Commit.Canonical);if err!=nil{t.Fatal(err)}
   journal:=jAny.([]any)
   events:=append([]ports.CanonicalRecord(nil),out.Events...)
   tc.mutate(ctx,t,ledger,out,fence,journal,events,bundle)
   d:=identity.New(bundle)
   fd,_,err:=d.Sum("cote.csf.digest.fence","cote.csf.schema.admission-fence.body",1,fence[:17]);if err!=nil{t.Fatal(err)}
   fence[17]=fd[:]
   journal[10]=fd[:]
   eventBytes:=make([]any,len(events))
   for i,event:=range events{eventBytes[i]=event.Canonical}
   jd,_,err:=d.Sum("cote.csf.digest.batch","cote.csf.schema.batch-preimage",1,[]any{journal[:23],eventBytes});if err!=nil{t.Fatal(err)}
   journal[23]=jd[:]
   sealed,err:=codec.CanonicalPayload(fence);if err!=nil{t.Fatal(err)}
   committed,err:=codec.CanonicalPayload(journal);if err!=nil{t.Fatal(err)}
   db,err:=sql.Open("sqlite",path);if err!=nil{t.Fatal(err)}
   if _,err:=db.ExecContext(ctx,"UPDATE decision_ledger SET canonical=? WHERE id=?",sealed,out.Fence.ID[:]);err!=nil{t.Fatal(err)}
   if _,err:=db.ExecContext(ctx,"UPDATE decision_ledger SET canonical=? WHERE id=?",committed,out.Commit.Identity[:]);err!=nil{t.Fatal(err)}
   for _,event:=range events{
    if _,err:=db.ExecContext(ctx,"UPDATE event_store SET canonical=? WHERE id=?",event.Canonical,event.Identity[:]);err!=nil{t.Fatal(err)}
   }
   if err:=db.Close();err!=nil{t.Fatal(err)}
   if _,err:=store.ReplayClock(ctx,[32]byte(run),bundle);err==nil{
    t.Fatal("replay accepted rehashed but causally forged authoritative records")
   }
  })
 }
}

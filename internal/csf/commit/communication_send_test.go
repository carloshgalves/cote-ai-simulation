package commit

import (
 "bytes"
 "context"
 "database/sql"
 "testing"

 _ "modernc.org/sqlite"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/admission"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/clock"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/communication"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/inputledger"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/trigger"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/extensions/testresourcev1"
)

func communicationFixture(t *testing.T)(*Coordinator,*sqlite.Store,*inputledger.Ledger,
 admission.Cycle,admission.Fence,string){
 t.Helper()
 ctx:=context.Background()
 bundle,err:=policy.Load(bundlePath);if err!=nil{t.Fatal(err)}
 g:=stockGenesis(t,bundle)
 path:=t.TempDir()+"/communication.db"
 store,err:=sqlite.Open(ctx,path);if err!=nil{t.Fatal(err)}
 t.Cleanup(func(){store.Close()})
 if err:=store.PersistGenesis(ctx,g,bundle);err!=nil{t.Fatal(err)}
 if err:=store.BootstrapClock(ctx,g,bundle);err!=nil{t.Fatal(err)}
 if err:=store.BootstrapGenesisTriggers(ctx,g,bundle,[]trigger.Definition{});err!=nil{t.Fatal(err)}
 ledger,err:=inputledger.New(ctx,g.RunID,bundle,store);if err!=nil{t.Fatal(err)}
 if err:=ledger.DeclareSource(inputledger.Source{ID:"driver"});err!=nil{t.Fatal(err)}
 at:=domain.EligibilityCoordinate{NotBeforeInstant:g.InitialInstant}
 until:=at;until.NotBeforeInstant+=100
 if _,err:=ledger.CloseSource(ctx,"driver",inputledger.Closure{
  ProducerKey:"source.closed",ClosedThrough:until});err!=nil{t.Fatal(err)}
 d:=identity.New(bundle)
 cycleID,_,err:=d.Sum("cote.csf.id.cycle","cote.csf.schema.cycle-id-preimage",1,
  []any{g.RunID[:],[]any{int64(g.InitialInstant),uint64(0)}})
 if err!=nil{t.Fatal(err)}
 origin:=[]any{uint64(2),[]any{g.ID[:],g.Digest[:]}}
 round,err:=ledger.DeclareRound(ctx,"schedule",[2]string{"school","a"},
  []string{"act"},at,origin,0,0,0,[]any{uint64(0)})
 if err!=nil{t.Fatal(err)}
 dispatchID,_,err:=d.Sum("cote.csf.id.dispatch","cote.csf.schema.dispatch-id-preimage",1,
  []any{g.RunID[:],cycleID[:],round.ID[:],"act",uint64(0)})
 if err!=nil{t.Fatal(err)}
 dispatch:=inputledger.Dispatch{ID:domain.ID(dispatchID),RunID:g.RunID,
  CycleID:domain.ID(cycleID),RoundID:round.ID,SlotID:"act",Actor:round.Actor,
  Eligibility:round.Eligibility,BaseRevision:0,EffectiveAt:g.InitialInstant}
 if err:=ledger.RegisterDispatch(ctx,dispatch,inputledger.CycleBinding{
  ID:domain.ID(cycleID),BaseRevision:0,Instant:g.InitialInstant});err!=nil{t.Fatal(err)}
 proposal,err:=testresourcev1.ProposalForDispatch(bundle,dispatch);if err!=nil{t.Fatal(err)}
 scope,err:=communication.MakeTyped("cote.csf.communication.scope.v1","cote.csf.schema.typed-value",[]any{uint64(0)})
 if err!=nil{t.Fatal(err)}
 content,err:=communication.MakeTyped("cote.csf.communication.allegation.v1","cote.csf.schema.typed-value",
  []any{"resource.stolen",[]any{"school","c"},"last.resource"})
 if err!=nil{t.Fatal(err)}
 params,err:=communication.SendParameters(communication.SendSpec{
  Recipient:communication.Actor{"school","b"},PresentedSender:"C",
  Channel:"school.message.v1",DueAt:g.InitialInstant+25,
  Proposition:communication.Proposition{
   Type:"resource.stolen",Polarity:0,TemporalScope:scope,Content:content}})
 if err!=nil{t.Fatal(err)}
 proposal.Body[4]="communication.send"
 proposal.Body[5]=uint64(1)
 proposal.Body[7]=[]any{}
 proposal.Body[8]=params
 proposal.Body[9]=[]any{}
 proposal.Body[10]=[]any{uint64(0)}
 digest,_,err:=d.Sum("cote.csf.digest.action-proposal-unit",
  "cote.csf.schema.action-proposal.body",1,[]any{proposal.Body})
 if err!=nil{t.Fatal(err)}
 proposal.Digest=digest
 proposal.Canonical,err=codec.CanonicalPayload(append(cloneAny(proposal.Body),digest[:]))
 if err!=nil{t.Fatal(err)}
 if err:=ledger.Respond(ctx,proposal);err!=nil{t.Fatal(err)}
 var base domain.Digest
 copy(base[:],g.Payload[16].([]any)[3].([]byte))
 cycle:=admission.Cycle{ID:domain.ID(cycleID),Attempt:0,Instant:g.InitialInstant,
  Ordinal:0,BaseRevision:0,BaseStateHash:base,RetryRef:[]any{uint64(0)}}
 roles,err:=policiesByRole(bundle);if err!=nil{t.Fatal(err)}
 policies:=admission.Policies{AdmissionOrder:roles["admission-order"],
  CausalIdentity:roles["causal-identity"],Fence:roles["admission-fence"],Rules:[][]any{}}
 c:=New(store,bundle,policies)
 fence,err:=admission.NewCoordinator(admission.NewFenceLog(store)).
  AdmitThenEvaluate(ctx,bundle,ledger,cycle,policies,nil)
 if err!=nil{t.Fatal(err)}
 return c,store,ledger,cycle,fence,path
}

func TestCSFRV17CommunicationSendAtomicAndTruthNeutral(t *testing.T){
 ctx:=context.Background()
 c,store,ledger,cycle,fence,path:=communicationFixture(t)
 _,_,_,_,_,run:=ledger.Snapshot()
 for fail:=uint8(1);fail<=7;fail++{
  c.failAfterStep=fail
  if _,err:=c.CommitCommunicationSend(ctx,ledger,cycle,fence);err==nil{
   t.Fatalf("communication atomic terminal survived injected crash at step %d",fail)
  }
  db,err:=sql.Open("sqlite","file:"+path);if err!=nil{t.Fatal(err)}
  for _,table:=range []string{"communication_claims","communication_transmissions",
   "event_store","committed_cycles"}{
   var n int
   if err:=db.QueryRowContext(ctx,"SELECT COUNT(*) FROM "+table).Scan(&n);err!=nil{t.Fatal(err)}
   if n!=0{t.Fatalf("partial %s survived rollback at %d",table,fail)}
  }
  db.Close()
 }
 c.failAfterStep=0
 commit,err:=c.CommitCommunicationSend(ctx,ledger,cycle,fence)
 if err!=nil{t.Fatal(err)}
 if commit.Identity==([32]byte{}){t.Fatal("missing communication CycleCommit")}
 db,err:=sql.Open("sqlite","file:"+path);if err!=nil{t.Fatal(err)}
 defer db.Close()
 for _,want:=range []struct{table string;count int}{
  {"communication_claims",1},{"communication_transmissions",1},
  {"scheduled_occurrences",1},{"event_store",2},
  {"committed_cycles",1},{"causal_outbox",2},{"evidence_ledger",0},
 }{
  var n int
  if err:=db.QueryRowContext(ctx,"SELECT COUNT(*) FROM "+want.table).Scan(&n);err!=nil{t.Fatal(err)}
  if n!=want.count{t.Fatalf("communication commit %s count %d, want %d",want.table,n,want.count)}
 }
 world,hash,err:=store.LoadWorldState(ctx,1);if err!=nil{t.Fatal(err)}
 before,oldHash,err:=store.LoadWorldState(ctx,0);if err!=nil{t.Fatal(err)}
 if hash!=oldHash||!bytes.Equal(world[2].([]byte),before[2].([]byte)){
  t.Fatal("false claim modified authoritative stock state")
 }
 next,err:=store.LoadClock(ctx,[32]byte(run))
 if err!=nil||next.Revision!=1||next.NextLogicalSequence!=2{
  t.Fatal("message did not advance causal revision atomically",err,next)
 }
}

func TestCSFRV17DelayedDeliveryExactlyOnceAndCausalReplay(t *testing.T){
 ctx:=context.Background()
 c,store,ledger,sendCycle,sendFence,path:=communicationFixture(t)
 _,_,_,_,_,run:=ledger.Snapshot()
 if _,err:=c.CommitCommunicationSend(ctx,ledger,sendCycle,sendFence);err!=nil{t.Fatal("send:",err)}
 db,err:=sql.Open("sqlite","file:"+path);if err!=nil{t.Fatal(err)}
 defer db.Close()
 var status,count int
 if err:=db.QueryRowContext(ctx,"SELECT status FROM communication_transmissions").Scan(&status);err!=nil||status!=0{
  t.Fatal("send did not leave a pending transmission",err,status)
 }
 if err:=db.QueryRowContext(ctx,"SELECT count(*) FROM evidence_ledger").Scan(&count);err!=nil||count!=0{
  t.Fatal("B saw a non-delivered claim",err,count)
 }
 advance,err:=c.Advance(ctx,ledger,run)
 if err!=nil{t.Fatal("expected causal CLOCK_ADVANCE until scheduled delivery:",err)}
 clockNow:=advance.ResultClock
 pending,err:=store.PendingClock(ctx,[32]byte(run));if err!=nil{t.Fatal(err)}
 plan,ok,err:=clock.Next(clockNow,pending)
 if err!=nil||!ok||plan.Kind!=clock.Work{t.Fatal("expected due WORK after advance",plan,err)}
 id,_,err:=identity.New(c.bundle).Sum("cote.csf.id.cycle","cote.csf.schema.cycle-id-preimage",1,
  []any{run[:],[]any{int64(clockNow.Instant),uint64(plan.OriginOrdinal)}})
 if err!=nil{t.Fatal(err)}
 _,hash,err:=store.LoadWorldState(ctx,uint64(clockNow.Revision));if err!=nil{t.Fatal(err)}
 cycle:=admission.Cycle{ID:domain.ID(id),Attempt:0,Instant:clockNow.Instant,
  Ordinal:plan.OriginOrdinal,BaseRevision:clockNow.Revision,
  BaseStateHash:domain.Digest(hash),RetryRef:[]any{uint64(0)}}
 fence,err:=admission.NewCoordinator(admission.NewFenceLog(store)).
  AdmitThenEvaluate(ctx,c.bundle,ledger,cycle,c.admissionPolicies,nil)
 if err!=nil{t.Fatal("due admission:",err)}
 for step:=uint8(1);step<=7;step++{
  c.failAfterStep=step
  if _,err:=c.CommitCommunicationDue(ctx,ledger,cycle,fence);err==nil{
   t.Fatalf("due transaction survived failpoint %d",step)
  }
  var s,n int
  if e:=db.QueryRowContext(ctx,"SELECT status FROM communication_transmissions").Scan(&s);e!=nil||s!=0{
   t.Fatal("rollback consumed channel",step,s,e)
  }
  if e:=db.QueryRowContext(ctx,"SELECT count(*) FROM event_store").Scan(&n);e!=nil||n!=4{
   t.Fatal("rollback published delivery Event",step,n,e)
  }
 }
 c.failAfterStep=0
 if _,err:=c.CommitCommunicationDue(ctx,ledger,cycle,fence);err!=nil{t.Fatal("delivery:",err)}
 if _,err:=c.CommitCommunicationDue(ctx,ledger,cycle,fence);err==nil{t.Fatal("due source delivered twice")}
 if err:=db.QueryRowContext(ctx,"SELECT status FROM communication_transmissions").Scan(&status);err!=nil||status!=1{
  t.Fatal("transmission remained IN_TRANSIT",status,err)
 }
 if err:=db.QueryRowContext(ctx,"SELECT count(*) FROM event_store").Scan(&count);err!=nil||count!=5{
  t.Fatal("exact event history changed",count,err)
 }
 pending,err=store.PendingClock(ctx,[32]byte(run))
 if err!=nil{t.Fatal(err)}
 next,err:=store.LoadClock(ctx,[32]byte(run));if err!=nil{t.Fatal(err)}
 if _,exists,err:=clock.Next(next,pending);err!=nil||exists{t.Fatal("due occurrence remained pending",exists,err)}
}

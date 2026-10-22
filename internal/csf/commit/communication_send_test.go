package commit

import (
 "bytes"
 "context"
 "database/sql"
 "testing"

 _ "modernc.org/sqlite"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/admission"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
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

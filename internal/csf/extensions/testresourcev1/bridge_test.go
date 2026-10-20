package testresourcev1

import (
 "bytes"
 "context"
 "errors"
 "testing"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/admission"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/decision"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/inputledger"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/memory"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

func TestDecisionInputMustResolvePersistedAdmissionFence(t *testing.T){
 ctx:=context.Background()
 bundle,err:=policy.Load("../../../../docs/architecture/canonical-codec-v1-bundle");if err!=nil{t.Fatal(err)}
 var run,cycleID domain.ID;run[0]=0x71;cycleID[0]=0x72
 store:=memory.New()
 ledger,err:=inputledger.New(ctx,run,bundle,store);if err!=nil{t.Fatal(err)}
 var gid,gd domain.ID;gid[0]=1;gd[0]=2
 created:=[]any{uint64(2),[]any{gid[:],gd[:]}}
 deriver:=identity.New(bundle)
 rounds:=[]inputledger.Round{}
 for i,actor:=range []string{"a","b"} {
  round,err:=ledger.DeclareRound(ctx,"schedule",[2]string{"school",actor},[]string{"act"},
   domain.EligibilityCoordinate{NotBeforeInstant:100},created,0,uint32(i),0,[]any{uint64(0)})
  if err!=nil{t.Fatal(err)}
  rounds=append(rounds,round)
 }
 for _,round:=range rounds{
  did,_,err:=deriver.Sum("cote.csf.id.dispatch","cote.csf.schema.dispatch-id-preimage",1,
   []any{run[:],cycleID[:],round.ID[:],"act",uint64(0)})
  if err!=nil{t.Fatal(err)}
  dispatch:=inputledger.Dispatch{ID:domain.ID(did),RunID:run,CycleID:cycleID,RoundID:round.ID,
   SlotID:"act",Actor:round.Actor,Eligibility:round.Eligibility,BaseRevision:7,EffectiveAt:100}
  if err:=ledger.RegisterDispatch(ctx,dispatch,inputledger.CycleBinding{ID:cycleID,BaseRevision:7,Instant:100});err!=nil{t.Fatal(err)}
  response,err:=ProposalForDispatch(bundle,dispatch);if err!=nil{t.Fatal(err)}
  if err:=ledger.Respond(ctx,response);err!=nil{t.Fatal(err)}
 }
 roles,err:=bundle.RequiredPolicyRoles();if err!=nil{t.Fatal(err)}
 refs,err:=bundle.GenesisPolicyRefs();if err!=nil{t.Fatal(err)}
 r:=map[string][]any{}
 for i,role:=range roles{r[role]=refs[i]}
 pol:=admission.Policies{AdmissionOrder:r["admission-order"],CausalIdentity:r["causal-identity"],Fence:r["admission-fence"],Rules:[][]any{}}
 var hash domain.Digest;hash[0]=0x77
 cycle:=admission.Cycle{ID:cycleID,Instant:100,Ordinal:0,BaseRevision:7,BaseStateHash:hash,RetryRef:[]any{uint64(0)}}
 fence,err:=admission.NewCoordinator(admission.NewFenceLog(store)).AdmitThenEvaluate(ctx,bundle,ledger,cycle,pol,
  func(admission.Fence)error{return nil})
 if err!=nil{t.Fatal(err)}
 snapshot:=decision.Snapshot{BaseRevision:7,ResourceKind:"inventory",ResourceID:"last.resource",Available:1,Known:true,
  AccessibleActors:map[string]bool{"school:a":true,"school:b":true},
  ForbiddenActors:map[string]bool{"school:a":false,"school:b":false}}
 config,err:=LastUnit(bundle);if err!=nil{t.Fatal(err)}
 req,err:=decision.FromPersistedFence(ctx,bundle,ledger,cycle,pol,fence,store,snapshot,config.Schemas,config.Seed,nil)
 if err!=nil{t.Fatal(err)}
 if len(req.Admitted)!=2{t.Fatal("admission fence did not bind exactly two recorded slot responses")}
 result,err:=decision.Run(req);if err!=nil{t.Fatal(err)}
 if len(result.Candidates)!=2||len(result.Conflicts)!=1||result.PostAvailable!=0{
  t.Fatal("fenced resource contest did not resolve exactly one last unit")
 }
 req.Units[0],req.Units[1]=req.Units[1],req.Units[0]
 again,err:=decision.Run(req)
 if err!=nil||!bytes.Equal(result.Canonical,again.Canonical){t.Fatalf("workers changed persisted-fence decision: %v",err)}
 corrupt:=fence
 corrupt.Canonical=[]byte{0x80}
 if _,err:=decision.FromPersistedFence(ctx,bundle,ledger,cycle,pol,corrupt,store,snapshot,config.Schemas,config.Seed,nil);!errors.Is(err,decision.ErrProvenance){
  t.Fatalf("unpublished/adulterated fence was accepted: %v",err)
 }
}

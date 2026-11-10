package commit

import (
 "context"
 "errors"
 "strings"
 "testing"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/inputledger"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

func TestCSFRV18V2SettlesFencedWorkBeforeChildFork(t *testing.T){
 ctx:=context.Background()
 coordinator,parent,ledger,_,_,sourcePath:=noProposalWorkFixture(t)
 _,_,_,_,_,run:=ledger.Snapshot()
 before,err:=parent.LoadClock(ctx,[32]byte(run));if err!=nil{t.Fatal(err)}
 // The source's remaining external ingress must be finalized before moving
 // ownership to the child; it cannot silently be forgotten at handoff.
 if _,err:=ledger.CloseSource(ctx,"driver",inputledger.Closure{
  ProducerKey:"fork-final-after-fence",
  ClosedThrough:domain.EligibilityCoordinate{NotBeforeInstant:before.Instant+1000},
  Final:true,
 });err!=nil{t.Fatal(err)}
 sealed,err:=parent.CreateSnapshotV1(ctx,[32]byte(run),coordinator.bundle,nil)
 if err!=nil{t.Fatal(err)}
 v2,err:=policy.LoadV2("../../../docs/architecture/canonical-codec-v2-bundle")
 if err!=nil{t.Fatal(err)}
 childID:=[32]byte(run);childID[0]^=0x6a
 path:=t.TempDir()+"/recovered-fence-v2.db"
 child,err:=sqlite.Open(ctx,path);if err!=nil{t.Fatal(err)}
 defer child.Close()
 provider:=func(ctx context.Context,s *sqlite.Store)([]sqlite.SnapshotCheckpoint,error){
  state,err:=s.LoadClock(ctx,[32]byte(run))
  if err!=nil{return nil,err}
  return csfrv18SyntheticActorCheckpoints(t,sourcePath,state.Instant,int64(state.Revision)),nil
 }
 if _,err:=coordinator.SettleFencedThenForkV2(ctx,ledger,[32]byte(run),
  sealed.ID,childID,child,coordinator.bundle,provider);err==nil{
  t.Fatal("uninstalled V2 policy passed preflight")
 }
 unchanged,err:=parent.LoadClock(ctx,[32]byte(run))
 if err!=nil||unchanged!=before{
  t.Fatal("failed child policy preflight mutated V1 fenced parent:",err)
 }
 root,err:=coordinator.SettleFencedThenForkV2(ctx,ledger,[32]byte(run),
  sealed.ID,childID,child,v2,provider)
 if err!=nil{t.Fatal("recovered fence handoff did not complete:",err)}
 if root.Payload[5]!=policy.CodecPolicyV2ID{t.Fatal("child policy downgraded")}
 parentAfter,err:=parent.LoadClock(ctx,[32]byte(run))
 if err!=nil||parentAfter.Revision!=before.Revision+1{
  t.Fatalf("fenced parent did not commit exactly once: %+v %v",parentAfter,err)
 }
 if _,err:=child.ReplayClock(ctx,childID,v2);err!=nil{
  t.Fatal("recovered-fence V2 child is not replayable:",err)
 }
 if _,err:=parent.VerifySnapshotV1(ctx,[32]byte(run),sealed.ID,coordinator.bundle);err!=nil{
  t.Fatal("historical frozen checkpoint lost:",err)
 }
 preserved,err:=provider(ctx,parent)
 if err!=nil{t.Fatal(err)}
 childSnap,err:=child.CreateSnapshotV1(ctx,childID,v2,preserved)
 if err!=nil{t.Fatal(err)}
 if _,err:=child.VerifySnapshotV1(ctx,childID,childSnap.ID,v2);err!=nil{t.Fatal(err)}
}

func TestCSFRV18R22FencedHandoffRetryAfterCheckpointProviderFailure(t *testing.T) {
 ctx:=context.Background()
 coordinator,parent,ledger,_,_,sourcePath:=noProposalWorkFixture(t)
 _,_,_,_,_,run:=ledger.Snapshot()
 before,err:=parent.LoadClock(ctx,[32]byte(run))
 if err!=nil{t.Fatal(err)}
 if _,err:=ledger.CloseSource(ctx,"driver",inputledger.Closure{
  ProducerKey:"frozen-retry-source-final",
  ClosedThrough:domain.EligibilityCoordinate{NotBeforeInstant:before.Instant+1000},
  Final:true,
 });err!=nil{t.Fatal(err)}
 frozen,err:=parent.CreateSnapshotV1(ctx,[32]byte(run),coordinator.bundle,nil)
 if err!=nil{t.Fatal(err)}
 v2,err:=policy.LoadV2("../../../docs/architecture/canonical-codec-v2-bundle")
 if err!=nil{t.Fatal(err)}
 childRun:=[32]byte(run)
 childRun[0]^=0x69
 child,err:=sqlite.Open(ctx,t.TempDir()+"/retry-after-parent-commit.db")
 if err!=nil{t.Fatal(err)}
 defer child.Close()
 failed:=false
 provider:=func(ctx context.Context,s *sqlite.Store)([]sqlite.SnapshotCheckpoint,error){
  if !failed {
   failed=true
   return nil,errors.New("injected cognitive checkpoint provider outage")
  }
  state,err:=s.LoadClock(ctx,[32]byte(run))
  if err!=nil{return nil,err}
  return csfrv18SyntheticActorCheckpoints(t,sourcePath,state.Instant,int64(state.Revision)),nil
 }
 if _,err:=coordinator.SettleFencedThenForkV2(ctx,ledger,[32]byte(run),
  frozen.ID,childRun,child,v2,provider);err==nil||
  !strings.Contains(err.Error(),"injected cognitive checkpoint provider outage"){
  t.Fatal("expected checkpoint acquisition failure after source commit:",err)
 }
 committed,err:=parent.LoadClock(ctx,[32]byte(run))
 if err!=nil||committed.Revision!=before.Revision+1{
  t.Fatalf("source terminal not exactly once: %+v %v",committed,err)
 }
 // Re-use the SAME immutable frozen checkpoint; no re-dispatch of the V1
 // terminal is permitted. A successful second attempt proves that missing
 // external checkpoint bytes cannot strand a durable parent commit.
 childGenesis,err:=coordinator.SettleFencedThenForkV2(ctx,ledger,[32]byte(run),
  frozen.ID,childRun,child,v2,provider)
 if err!=nil{t.Fatal("retry after committed source must finish:",err)}
 if childGenesis.Payload[5]!=policy.CodecPolicyV2ID{
  t.Fatal("retry downgraded child policy")
 }
 after,err:=parent.LoadClock(ctx,[32]byte(run))
 if err!=nil||after.Revision!=committed.Revision{
  t.Fatalf("retry committed V1 source twice: %+v %v",after,err)
 }
 if _,err:=child.ReplayClock(ctx,childRun,v2);err!=nil{
  t.Fatal("recovered child is not replayable:",err)
 }
 if _,err:=parent.VerifySnapshotV1(ctx,[32]byte(run),frozen.ID,coordinator.bundle);err!=nil{
  t.Fatal("frozen source snapshot changed:",err)
 }
}

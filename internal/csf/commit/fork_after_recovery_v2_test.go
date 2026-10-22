package commit

import (
 "context"
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
 root,err:=coordinator.SettleFencedThenForkV2(ctx,ledger,[32]byte(run),
  sealed.ID,childID,child,v2,func(ctx context.Context,s *sqlite.Store)([]sqlite.SnapshotCheckpoint,error){
   state,err:=s.LoadClock(ctx,[32]byte(run))
   if err!=nil{return nil,err}
   return csfrv18SyntheticActorCheckpoints(t,sourcePath,state.Instant,int64(state.Revision)),nil
  })
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
 childSnap,err:=child.CreateSnapshotV1(ctx,childID,v2,nil)
 if err!=nil{t.Fatal(err)}
 if _,err:=child.VerifySnapshotV1(ctx,childID,childSnap.ID,v2);err!=nil{t.Fatal(err)}
}

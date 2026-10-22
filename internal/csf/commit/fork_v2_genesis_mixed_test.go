package commit

import (
 "context"
 "testing"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/inputledger"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/trigger"
)

// The source holds both a pending Genesis schedule and a registered trigger.
// Neither child may inherit its parent's live source ID or trigger seal.
func TestCSFRV18V2MigratesGenesisScheduleAndInitialTrigger(t *testing.T){
 ctx:=context.Background()
 _,source,ledger,run,v1,_:=testFixture(t,activeTrigger())
 state,err:=source.LoadClock(ctx,[32]byte(run));if err!=nil{t.Fatal(err)}
 if _,err:=ledger.CloseSource(ctx,"driver",inputledger.Closure{
  ProducerKey:"v2-closed-driver",Final:true,
  ClosedThrough:domain.EligibilityCoordinate{NotBeforeInstant:state.Instant+100,
   NotBeforeCycleOrdinal:1},
 });err!=nil{t.Fatal(err)}
 before,err:=source.PendingClock(ctx,[32]byte(run))
 if err!=nil||len(before)!=1{t.Fatalf("bad pending parent Genesis schedule: %+v %v",before,err)}
 snap,err:=source.CreateSnapshotV1(ctx,[32]byte(run),v1,nil)
 if err!=nil{t.Fatal(err)}
 v2,err:=policy.LoadV2("../../../docs/architecture/canonical-codec-v2-bundle")
 if err!=nil{t.Fatal(err)}
 path:=t.TempDir()+"/schedule-trigger-v2.db"
 child,err:=sqlite.Open(ctx,path);if err!=nil{t.Fatal(err)}
 childID:=[32]byte(run);childID[0]^=0x65
 root,err:=source.ForkIdleSnapshotV1ToV2(ctx,[32]byte(run),snap.ID,childID,child,v1,v2)
 if err!=nil{t.Fatal("Genesis schedule + initial trigger V2 migration:",err)}
 after,err:=child.PendingClock(ctx,childID)
 if err!=nil||len(after)!=1||after[0].ID==before[0].ID{
  t.Fatalf("V2 child schedule not rekeyed: %+v %v",after,err)
 }
 if triggers,err:=child.LoadRegisteredTriggers(ctx,childID,v2);err!=nil||len(triggers)!=1{
  t.Fatalf("V2 trigger registry not installed: %d %v",len(triggers),err)
 }
 if _,err:=child.ReplayClock(ctx,childID,v2);err!=nil{t.Fatal(err)}
 childSnap,err:=child.CreateSnapshotV1(ctx,childID,v2,nil);if err!=nil{t.Fatal(err)}
 if _,err:=child.VerifySnapshotV1(ctx,childID,childSnap.ID,v2);err!=nil{t.Fatal(err)}
 if err:=child.Close();err!=nil{t.Fatal(err)}
 reopened,err:=sqlite.Open(ctx,path);if err!=nil{t.Fatal(err)}
 defer reopened.Close()
 if _,err:=reopened.ReplayClock(ctx,childID,v2);err!=nil{t.Fatal(err)}
 if _,err:=source.VerifySnapshotV1(ctx,[32]byte(run),snap.ID,v1);err!=nil{t.Fatal(err)}
 if root.Payload[5]!=policy.CodecPolicyV2ID{t.Fatal("V2 Genesis policy missing")}
}

func TestCSFRV18V2RekeysInitialGenesisTriggerActivation(t *testing.T){
 ctx:=context.Background()
 v1,err:=policy.Load(bundlePath);if err!=nil{t.Fatal(err)}
 g:=validGenesis(t,v1)
 parent,err:=sqlite.Open(ctx,t.TempDir()+"/parent-trigger.db");if err!=nil{t.Fatal(err)}
 defer parent.Close()
 if err:=parent.PersistGenesis(ctx,g,v1);err!=nil{t.Fatal(err)}
 if err:=parent.BootstrapClock(ctx,g,v1);err!=nil{t.Fatal(err)}
 def:=activeTrigger()
 def.PredicateThreshold=g.InitialInstant
 if err:=parent.BootstrapGenesisTriggers(ctx,g,v1,[]trigger.Definition{def});err!=nil{t.Fatal(err)}
 before,err:=parent.PendingClock(ctx,[32]byte(g.RunID))
 if err!=nil||len(before)!=1{t.Fatalf("no parent activation: %v",err)}
 snap,err:=parent.CreateSnapshotV1(ctx,[32]byte(g.RunID),v1,nil);if err!=nil{t.Fatal(err)}
 v2,err:=policy.LoadV2("../../../docs/architecture/canonical-codec-v2-bundle");if err!=nil{t.Fatal(err)}
 path:=t.TempDir()+"/child-trigger-v2.db"
 child,err:=sqlite.Open(ctx,path);if err!=nil{t.Fatal(err)}
 defer child.Close()
 id:=[32]byte(g.RunID);id[0]^=0x42
 if _,err:=parent.ForkIdleSnapshotV1ToV2(ctx,[32]byte(g.RunID),snap.ID,id,child,v1,v2);err!=nil{
  t.Fatal("initial trigger activation V2 migration:",err)
 }
 after,err:=child.PendingClock(ctx,id)
 if err!=nil||len(after)!=1||after[0].ID==before[0].ID{
  t.Fatalf("trigger activation not child-owned: %+v %v",after,err)
 }
 if _,err:=child.LoadRegisteredTriggers(ctx,id,v2);err!=nil{t.Fatal(err)}
 if _,err:=child.ReplayClock(ctx,id,v2);err!=nil{t.Fatal(err)}
 if _,err:=parent.VerifySnapshotV1(ctx,[32]byte(g.RunID),snap.ID,v1);err!=nil{t.Fatal(err)}
}

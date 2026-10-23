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

func TestCSFRV18V2ForkSiblingInputTailsDivergeWithoutCrossRunAuthority(t *testing.T){
 ctx:=context.Background()
 source,ledger,v1,run,origin:=csfrv18GenesisRoundSource(t)
 state,err:=source.LoadClock(ctx,[32]byte(run));if err!=nil{t.Fatal(err)}
 due:=domain.EligibilityCoordinate{NotBeforeInstant:state.Instant+80}
 if _,err:=ledger.DeclareRound(ctx,"genesis.schedule",[2]string{"school","a"},
  []string{"choice"},due,origin,2,1,0,[]any{uint64(0)});err!=nil{t.Fatal(err)}
 snap,err:=source.CreateSnapshotV1(ctx,[32]byte(run),v1,nil);if err!=nil{t.Fatal(err)}
 v2,err:=policy.LoadV2("../../../docs/architecture/canonical-codec-v2-bundle")
 if err!=nil{t.Fatal(err)}
 a,err:=sqlite.Open(ctx,t.TempDir()+"/a.db");if err!=nil{t.Fatal(err)}
 defer a.Close()
 b,err:=sqlite.Open(ctx,t.TempDir()+"/b.db");if err!=nil{t.Fatal(err)}
 defer b.Close()
 aid:=[32]byte(run);aid[0]^=0x24
 bid:=[32]byte(run);bid[0]^=0x35
 rootA,err:=source.ForkIdleSnapshotV1ToV2(ctx,[32]byte(run),snap.ID,aid,a,v1,v2)
 if err!=nil{t.Fatal(err)}
 rootB,err:=source.ForkIdleSnapshotV1ToV2(ctx,[32]byte(run),snap.ID,bid,b,v1,v2)
 if err!=nil{t.Fatal(err)}
 if rootA.ID==rootB.ID||rootA.Digest==rootB.Digest{
  t.Fatal("V2 sibling Genesis identities aliased")
 }
 beforeB,err:=b.CreateSnapshotV1(ctx,bid,v2,nil);if err!=nil{t.Fatal(err)}
 ledgerA,err:=inputledger.Open(ctx,domain.ID(aid),v2,a);if err!=nil{t.Fatal(err)}
 childOrigin:=[]any{uint64(2),[]any{rootA.ID[:],rootA.Digest[:]}}
 newRound,err:=ledgerA.DeclareRound(ctx,"child.schedule",
  [2]string{"school","a"},[]string{"new"},due,childOrigin,3,7,0,[]any{uint64(0)})
 if err!=nil{t.Fatal("child A tail append:",err)}
 if _,err:=a.ReplayClock(ctx,aid,v2);err!=nil{t.Fatal("child A replay:",err)}
 afterA,err:=a.CreateSnapshotV1(ctx,aid,v2,nil);if err!=nil{t.Fatal(err)}
 if afterA.ID==beforeB.ID{t.Fatal("divergent child tails produced same snapshot ID")}
 if _,err:=b.VerifySnapshotV1(ctx,bid,beforeB.ID,v2);err!=nil{
  t.Fatal("append in child A changed sibling B:",err)
 }
 ledgerB,err:=inputledger.Open(ctx,domain.ID(bid),v2,b);if err!=nil{t.Fatal(err)}
 _,_,_,roundsB,_,_:=ledgerB.Snapshot()
 if len(roundsB)!=1||roundsB[0].ID==newRound.ID{
  t.Fatal("child B adopted new child A round authority")
 }
 if _,err:=source.VerifySnapshotV1(ctx,[32]byte(run),snap.ID,v1);err!=nil{
  t.Fatal("V2 child append changed V1 origin:",err)
 }
}

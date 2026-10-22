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

// A live future occurrence and a sealed Genesis trigger definition are
// preserved as child-owned runtime authority, never as shared parent rows.
func TestCSFRV18ForkMigratesGenesisScheduleAndTriggerRuntime(t *testing.T) {
 ctx := context.Background()
 _, parent, ledger, run, bundle, _ := testFixture(t, activeTrigger())
 before, err := parent.PendingClock(ctx, [32]byte(run))
 if err != nil { t.Fatal(err) }
 if len(before) != 1 { t.Fatalf("expected one pending Genesis schedule, got %d",len(before)) }
 state, err := parent.LoadClock(ctx, [32]byte(run))
 if err != nil { t.Fatal(err) }
 if _, err := ledger.CloseSource(ctx, "driver", inputledger.Closure{
  ProducerKey: "fork-completed-driver", Final: true,
  ClosedThrough: domain.EligibilityCoordinate{NotBeforeInstant: state.Instant+100,NotBeforeCycleOrdinal: 1},
 }); err != nil { t.Fatal(err) }
 parentCheckpoint, err := parent.CreateSnapshotV1(ctx, [32]byte(run), bundle, nil)
 if err != nil { t.Fatal(err) }
 child, err := sqlite.Open(ctx, t.TempDir()+"/child.db")
 if err != nil { t.Fatal(err) }
 defer child.Close()
 childRun := [32]byte(run)
 childRun[0] ^= 0x67
 if _, err := parent.ForkIdleSnapshotV1(ctx, [32]byte(run), parentCheckpoint.ID, childRun, child, bundle); err != nil {
  t.Fatal("supported Genesis pending fork rejected:", err)
 }
 after, err := child.PendingClock(ctx, childRun)
 if err != nil { t.Fatal(err) }
 if len(after) != len(before) || after[0].ID == before[0].ID {
  t.Fatal("fork lost a pending occurrence or aliased parent's ID")
 }
 if _, err := child.LoadRegisteredTriggers(ctx,childRun,bundle); err != nil {
  t.Fatal("fork failed to register immutable child trigger authority:", err)
 }
 if _, err := child.ReplayClock(ctx,childRun,bundle); err != nil {
  t.Fatal("migrated child cannot be causally replayed:", err)
 }
 checkpoint, err := child.CreateSnapshotV1(ctx, childRun, bundle, nil)
 if err != nil { t.Fatal("migrated child cannot snapshot:", err) }
 if _, err := child.VerifySnapshotV1(ctx, childRun, checkpoint.ID, bundle); err != nil { t.Fatal(err) }
 if _, err := parent.VerifySnapshotV1(ctx, [32]byte(run), parentCheckpoint.ID, bundle); err != nil {
  t.Fatal("fork mutated parent checkpoint:", err)
 }
}

func TestCSFRV18ForkRekeysPendingGenesisTriggerActivation(t *testing.T) {
 ctx := context.Background()
 bundle, err := policy.Load(bundlePath)
 if err != nil { t.Fatal(err) }
 g := validGenesis(t, bundle)
 source, err := sqlite.Open(ctx, t.TempDir()+"/activation-parent.db")
 if err != nil { t.Fatal(err) }
 defer source.Close()
 if err := source.PersistGenesis(ctx,g,bundle); err != nil { t.Fatal(err) }
 if err := source.BootstrapClock(ctx,g,bundle); err != nil { t.Fatal(err) }
 def := activeTrigger()
 def.PredicateThreshold = g.InitialInstant
 if err := source.BootstrapGenesisTriggers(ctx,g,bundle,[]trigger.Definition{def}); err != nil { t.Fatal(err) }
 parentPending, err := source.PendingClock(ctx, [32]byte(g.RunID))
 if err != nil { t.Fatal(err) }
 if len(parentPending) != 1 { t.Fatalf("expected one Genesis activation; got %d",len(parentPending)) }
 snapshot, err := source.CreateSnapshotV1(ctx,[32]byte(g.RunID),bundle,nil)
 if err != nil { t.Fatal(err) }
 child, err := sqlite.Open(ctx,t.TempDir()+"/activation-child.db")
 if err != nil { t.Fatal(err) }
 defer child.Close()
 id := [32]byte(g.RunID); id[0] ^= 0x79
 if _, err := source.ForkIdleSnapshotV1(ctx,[32]byte(g.RunID),snapshot.ID,id,child,bundle); err != nil {
  t.Fatal("genesis trigger activation rejected by registered V1 migrator:",err)
 }
 pending, err := child.PendingClock(ctx,id)
 if err != nil { t.Fatal(err) }
 if len(pending) != 1 || pending[0].ID == parentPending[0].ID {
  t.Fatal("pending activation not rekeyed to isolated child authority")
 }
 if _, err := child.LoadRegisteredTriggers(ctx,id,bundle); err != nil { t.Fatal(err) }
 if _, err := child.ReplayClock(ctx,id,bundle); err != nil { t.Fatal(err) }
 if _, err := source.VerifySnapshotV1(ctx,[32]byte(g.RunID),snapshot.ID,bundle); err != nil {
  t.Fatal("fork changed parent activation history:",err)
 }
}

func TestCSFRV18ForkSiblingsHaveIndependentCommittedTails(t *testing.T) {
 ctx := context.Background()
 c, parent, ledger, run, bundle, _ := testFixture(t,activeTrigger())
 state,err := parent.LoadClock(ctx,[32]byte(run))
 if err != nil { t.Fatal(err) }
 if _,err=ledger.CloseSource(ctx,"driver",inputledger.Closure{
  ProducerKey:"fork-terminal-driver",Final:true,
  ClosedThrough:domain.EligibilityCoordinate{NotBeforeInstant:state.Instant+100,NotBeforeCycleOrdinal:1},
 });err!=nil{t.Fatal(err)}
 baseline,err:=parent.CreateSnapshotV1(ctx,[32]byte(run),bundle,nil)
 if err!=nil{t.Fatal(err)}
 a,err:=sqlite.Open(ctx,t.TempDir()+"/fork-a.db")
 if err!=nil{t.Fatal(err)}
 defer a.Close()
 b,err:=sqlite.Open(ctx,t.TempDir()+"/fork-b.db")
 if err!=nil{t.Fatal(err)}
 defer b.Close()
 aID:=[32]byte(run);aID[0]^=0x26
 bID:=[32]byte(run);bID[0]^=0x39
 if _,err:=parent.ForkIdleSnapshotV1(ctx,[32]byte(run),baseline.ID,aID,a,bundle);err!=nil{t.Fatal(err)}
 if _,err:=parent.ForkIdleSnapshotV1(ctx,[32]byte(run),baseline.ID,bID,b,bundle);err!=nil{t.Fatal(err)}
 beforeB,err:=b.CreateSnapshotV1(ctx,bID,bundle,nil)
 if err!=nil{t.Fatal(err)}
 ledgerA,err:=inputledger.New(ctx,domain.ID(aID),bundle,a)
 if err!=nil{t.Fatal(err)}
 runnerA:=New(a,bundle,c.admissionPolicies)
 if _,err:=runnerA.Advance(ctx,ledgerA,domain.ID(aID));err!=nil{
  t.Fatal("fork child cannot execute its own migrated pending schedule:",err)
 }
 afterA,err:=a.CreateSnapshotV1(ctx,aID,bundle,nil)
 if err!=nil{t.Fatal(err)}
 if afterA.Revision!=1{t.Fatal("fork child did not produce independent causal tail")}
 verifiedB,err:=b.VerifySnapshotV1(ctx,bID,beforeB.ID,bundle)
 if err!=nil{t.Fatal("sibling lost its original checkpoint after another tail:",err)}
 if verifiedB.Clock.Revision!=0{t.Fatal("sibling adopted another child's event tail")}
 if _,err:=parent.VerifySnapshotV1(ctx,[32]byte(run),baseline.ID,bundle);err!=nil{
  t.Fatal("parent history changed when child advanced:",err)
 }
 parentState,err:=parent.LoadClock(ctx,[32]byte(run))
 if err!=nil||parentState.Revision!=0{t.Fatal("child mutated parent clock",err)}
}

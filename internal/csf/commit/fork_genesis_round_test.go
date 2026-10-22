package commit

import (
 "context"
 "testing"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/inputledger"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

// Both positive and negative cases exercise real SQLite snapshot/lineage
// authority, not a fabricated pending projection.
func csfrv18GenesisRoundSource(t *testing.T)(*sqlite.Store,*inputledger.Ledger,*policy.Bundle,domain.ID,[]any){
 t.Helper()
 ctx:=context.Background()
 b,err:=policy.Load(bundlePath);if err!=nil{t.Fatal(err)}
 g:=validGenesis(t,b)
 store,err:=sqlite.Open(ctx,t.TempDir()+"/origin-round.db");if err!=nil{t.Fatal(err)}
 t.Cleanup(func(){store.Close()})
 if err:=store.PersistGenesis(ctx,g,b);err!=nil{t.Fatal(err)}
 if err:=store.BootstrapClock(ctx,g,b);err!=nil{t.Fatal(err)}
 if err:=store.BootstrapGenesisTriggers(ctx,g,b,nil);err!=nil{t.Fatal(err)}
 ledger,err:=inputledger.Open(ctx,g.RunID,b,store)
 if err!=nil{t.Fatal(err)}
 return store,ledger,b,g.RunID,[]any{uint64(2),[]any{g.ID[:],g.Digest[:]}}
}

func TestCSFRV18ForkMigratesPendingUnstartedGenesisRound(t *testing.T){
 ctx:=context.Background()
 source,ledger,b,run,origin:=csfrv18GenesisRoundSource(t)
 state,err:=source.LoadClock(ctx,[32]byte(run));if err!=nil{t.Fatal(err)}
 due:=domain.EligibilityCoordinate{NotBeforeInstant:state.Instant+55,NotBeforeCycleOrdinal:0}
 prior,err:=ledger.DeclareRound(ctx,"genesis.schedule",[2]string{"school","b"},
  []string{"write","read"},due,origin,1,2,0,[]any{uint64(0)})
 if err!=nil{t.Fatal(err)}
 snap,err:=source.CreateSnapshotV1(ctx,[32]byte(run),b,nil);if err!=nil{t.Fatal(err)}
 child,err:=sqlite.Open(ctx,t.TempDir()+"/child-round.db");if err!=nil{t.Fatal(err)}
 defer child.Close()
 childID:=[32]byte(run);childID[0]^=0x54
 childGenesis,err:=source.ForkIdleSnapshotV1(ctx,[32]byte(run),snap.ID,childID,child,b)
 if err!=nil{t.Fatal("unstarted Genesis-owned round migration:",err)}
 if [32]byte(childGenesis.RunID)!=childID{t.Fatal("fork child run mismatch")}
 restored,err:=inputledger.Open(ctx,domain.ID(childID),b,child);if err!=nil{t.Fatal(err)}
 _,_,_,rounds,_,gotRun:=restored.Snapshot()
 if [32]byte(gotRun)!=childID||len(rounds)!=1{t.Fatalf("migrated round is not live in child: %d",len(rounds))}
 rebuilt:=rounds[0]
 if rebuilt.ID==prior.ID||rebuilt.Eligibility!=prior.Eligibility||
  rebuilt.Actor!=prior.Actor||rebuilt.SourceID!=prior.SourceID||
  rebuilt.Role!=prior.Role||rebuilt.LocalOrdinal!=prior.LocalOrdinal {
  t.Fatal("fork round reused parent ID or changed semantic membership")
 }
 childPending,err:=child.PendingClock(ctx,childID);if err!=nil{t.Fatal(err)}
 if len(childPending)!=1||childPending[0].ID!=rebuilt.ID{
  t.Fatalf("unstarted round not recoverable as child pending WORK: %+v",childPending)
 }
 if _,err:=child.ReplayClock(ctx,childID,b);err!=nil{t.Fatal("round fork not replayable:",err)}
 childSnap,err:=child.CreateSnapshotV1(ctx,childID,b,nil);if err!=nil{t.Fatal(err)}
 if _,err:=child.VerifySnapshotV1(ctx,childID,childSnap.ID,b);err!=nil{t.Fatal(err)}
 if _,err:=source.VerifySnapshotV1(ctx,[32]byte(run),snap.ID,b);err!=nil{
  t.Fatal("fork changed the parent checkpoint:",err)
 }
}

func TestCSFRV18ForkRejectsEventOwnedPendingRoundWithoutMigrator(t *testing.T){
 ctx:=context.Background()
 source,ledger,b,run,_:=csfrv18GenesisRoundSource(t)
 state,err:=source.LoadClock(ctx,[32]byte(run));if err!=nil{t.Fatal(err)}
 fake:=[32]byte(run);fake[0]^=0x87
 eventOrigin:=[]any{uint64(0),[]any{fake[:],fake[:]}}
 _,err=ledger.DeclareRound(ctx,"event.schedule",[2]string{"school","b"},
  []string{"choice"},domain.EligibilityCoordinate{NotBeforeInstant:state.Instant+44},
  eventOrigin,1,0,0,[]any{uint64(0)})
 if err!=nil{t.Fatal(err)}
 snap,err:=source.CreateSnapshotV1(ctx,[32]byte(run),b,nil);if err!=nil{t.Fatal(err)}
 path:=t.TempDir()+"/rejected-event-round.db"
 child,err:=sqlite.Open(ctx,path);if err!=nil{t.Fatal(err)}
 defer child.Close()
 id:=[32]byte(run);id[0]^=0x53
 if _,err:=source.ForkIdleSnapshotV1(ctx,[32]byte(run),snap.ID,id,child,b);err==nil{
  t.Fatal("event-owned pending round migrated without its causal event")
 }
 assertCSFRV18EmptyRejectedFork(t,path)
}

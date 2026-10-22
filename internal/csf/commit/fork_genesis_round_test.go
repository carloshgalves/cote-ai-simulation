package commit

import (
 "context"
 "database/sql"
 "testing"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/inputledger"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
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
 childOrigin:=[]any{uint64(2),[]any{childGenesis.ID[:],childGenesis.Digest[:]}}
 retried,err:=restored.DeclareRound(ctx,rebuilt.SourceID,rebuilt.Actor,
  []string{"read","write"},due,childOrigin,1,2,0,[]any{uint64(0)})
 if err!=nil||retried.ID!=rebuilt.ID{
  t.Fatalf("migrated child round not idempotently reconstructible: %v",err)
 }
 if _,err:=restored.DeclareRound(ctx,rebuilt.SourceID,rebuilt.Actor,
  []string{"tampered-slot"},due,childOrigin,1,2,0,[]any{uint64(0)});err==nil{
  t.Fatal("child round accepted identity collision with altered slot roster")
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

func TestCSFRV18ForkRejectsGenesisRoundWithRevokedDispatchHistory(t *testing.T){
 ctx:=context.Background()
 source,ledger,b,run,origin:=csfrv18GenesisRoundSource(t)
 state,err:=source.LoadClock(ctx,[32]byte(run));if err!=nil{t.Fatal(err)}
 due:=domain.EligibilityCoordinate{NotBeforeInstant:state.Instant+15}
 round,err:=ledger.DeclareRound(ctx,"genesis.schedule",[2]string{"school","a"},
  []string{"choice","reply"},due,origin,1,0,0,[]any{uint64(0)})
 if err!=nil{t.Fatal(err)}
 cid,_,err:=identity.New(b).Sum("cote.csf.id.cycle","cote.csf.schema.cycle-id-preimage",
  1,[]any{run[:],[]any{int64(state.Instant),uint64(state.LastOrdinal+1)}})
 if err!=nil{t.Fatal(err)}
 dispatchID,_,err:=identity.New(b).Sum("cote.csf.id.dispatch",
  "cote.csf.schema.dispatch-id-preimage",1,
  []any{run[:],cid[:],round.ID[:],"choice",uint64(0)})
 if err!=nil{t.Fatal(err)}
 dispatch:=inputledger.Dispatch{ID:domain.ID(dispatchID),RunID:run,
  CycleID:domain.ID(cid),RoundID:round.ID,SlotID:"choice",Actor:round.Actor,
  Eligibility:due,BaseRevision:state.Revision,EffectiveAt:state.Instant}
 if err:=ledger.RegisterDispatch(ctx,dispatch,inputledger.CycleBinding{
  ID:domain.ID(cid),BaseRevision:state.Revision,Instant:state.Instant});err!=nil{t.Fatal(err)}
 if err:=ledger.RevokeDispatch(ctx,round.ID,"choice",0,[]any{uint64(0)});err!=nil{t.Fatal(err)}
 snap,err:=source.CreateSnapshotV1(ctx,[32]byte(run),b,nil)
 if err!=nil{t.Fatal(err)}
 path:=t.TempDir()+"/revoked-child.db"
 child,err:=sqlite.Open(ctx,path);if err!=nil{t.Fatal(err)}
 defer child.Close()
 childID:=[32]byte(run);childID[0]^=0x33
 if _,err:=source.ForkIdleSnapshotV1(ctx,[32]byte(run),snap.ID,childID,child,b);err==nil{
  t.Fatal("fork silently discarded revoked dispatch history from a pending Genesis round")
 }
 assertCSFRV18EmptyRejectedFork(t,path)
}

func TestCSFRV18ForkRoundSealFaultRollsBackAllChildAuthority(t *testing.T){
 ctx:=context.Background()
 source,ledger,b,run,origin:=csfrv18GenesisRoundSource(t)
 state,err:=source.LoadClock(ctx,[32]byte(run));if err!=nil{t.Fatal(err)}
 _,err=ledger.DeclareRound(ctx,"genesis.schedule",[2]string{"school","a"},
  []string{"choice"},domain.EligibilityCoordinate{NotBeforeInstant:state.Instant+21},
  origin,1,0,0,[]any{uint64(0)})
 if err!=nil{t.Fatal(err)}
 snap,err:=source.CreateSnapshotV1(ctx,[32]byte(run),b,nil);if err!=nil{t.Fatal(err)}
 path:=t.TempDir()+"/failed-round-seal.db"
 child,err:=sqlite.Open(ctx,path);if err!=nil{t.Fatal(err)}
 defer child.Close()
 raw,err:=sql.Open("sqlite",path);if err!=nil{t.Fatal(err)}
 defer raw.Close()
 if _,err:=raw.ExecContext(ctx, `CREATE TRIGGER csfrv18_fault_round_seal
  BEFORE INSERT ON round_integrity_seals
  BEGIN SELECT RAISE(ABORT,'injected failure after child round insertion'); END;`);err!=nil{t.Fatal(err)}
 id:=[32]byte(run);id[0]^=0x91
 if _,err:=source.ForkIdleSnapshotV1(ctx,[32]byte(run),snap.ID,id,child,b);err==nil{
  t.Fatal("fault during migrated round seal did not abort child bootstrap")
 }
 assertCSFRV18EmptyRejectedFork(t,path)
 if _,err:=source.VerifySnapshotV1(ctx,[32]byte(run),snap.ID,b);err!=nil{
  t.Fatal("failed child transaction changed parental snapshot:",err)
 }
}

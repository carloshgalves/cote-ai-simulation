package commit

import (
 "context"
 "testing"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/inputledger"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

func TestCSFRV18V2IdleForkExecutesAndReplaysAcrossReopen(t *testing.T){
 ctx:=context.Background()
 parent,_,v1,run,_:=csfrv18GenesisRoundSource(t)
 checkpoint,err:=parent.CreateSnapshotV1(ctx,[32]byte(run),v1,nil)
 if err!=nil{t.Fatal(err)}
 v2,err:=policy.LoadV2("../../../docs/architecture/canonical-codec-v2-bundle")
 if err!=nil{t.Fatal(err)}
 path:=t.TempDir()+"/v2-child.db"
 child,err:=sqlite.Open(ctx,path);if err!=nil{t.Fatal(err)}
 childID:=[32]byte(run);childID[0]^=0x81
 if childID==([32]byte{}){childID[0]=1}
 root,err:=parent.ForkIdleSnapshotV1ToV2(ctx,[32]byte(run),checkpoint.ID,childID,child,v1,v2)
 if err!=nil{t.Fatal("cross-policy V1-to-V2 fork:",err)}
 if [32]byte(root.RunID)!=childID||root.Payload[5]!=policy.CodecPolicyV2ID{
  t.Fatal("V2 fork created wrong child identity or policy")
 }
 if verified,err:=child.ValidatedGenesis(ctx,childID,v2);err!=nil||verified.Digest!=root.Digest{
  t.Fatalf("V2 child Genesis lineage not independently verifiable: %v",err)
 }
 replay,err:=child.ReplayClock(ctx,childID,v2)
 if err!=nil||replay.ReplayedCommits!=0{t.Fatalf("V2 child replay invalid: %+v %v",replay,err)}
 snap,err:=child.CreateSnapshotV1(ctx,childID,v2,nil)
 if err!=nil{t.Fatal("V2 child cannot create signed snapshot:",err)}
 if _,err:=child.VerifySnapshotV1(ctx,childID,snap.ID,v2);err!=nil{
  t.Fatal("V2 snapshot cannot verify its own policy hash:",err)
 }
 if err:=child.Close();err!=nil{t.Fatal(err)}
 reopened,err:=sqlite.Open(ctx,path);if err!=nil{t.Fatal(err)}
 defer reopened.Close()
 if _,err:=reopened.ReplayClock(ctx,childID,v2);err!=nil{
  t.Fatal("cross-policy origin lost across SQLite reopen:",err)
 }
 if _,err:=parent.VerifySnapshotV1(ctx,[32]byte(run),checkpoint.ID,v1);err!=nil{
  t.Fatal("child fork altered parent checkpoint:",err)
 }
}

func TestCSFRV18V2MigratesUnstartedGenesisRoundAndReplays(t *testing.T){
 ctx:=context.Background()
 parent,ledger,v1,run,origin:=csfrv18GenesisRoundSource(t)
 state,err:=parent.LoadClock(ctx,[32]byte(run));if err!=nil{t.Fatal(err)}
 due:=domain.EligibilityCoordinate{NotBeforeInstant:state.Instant+50}
 sourceRound,err:=ledger.DeclareRound(ctx,"genesis.schedule",[2]string{"school","b"},
  []string{"choice"},due,origin,0,1,0,[]any{uint64(0)})
 if err!=nil{t.Fatal(err)}
 checkpoint,err:=parent.CreateSnapshotV1(ctx,[32]byte(run),v1,nil);if err!=nil{t.Fatal(err)}
 v2,err:=policy.LoadV2("../../../docs/architecture/canonical-codec-v2-bundle")
 if err!=nil{t.Fatal(err)}
 path:=t.TempDir()+"/v2-round.db"
 child,err:=sqlite.Open(ctx,path);if err!=nil{t.Fatal(err)}
 childID:=[32]byte(run);childID[0]^=0x79
 root,err:=parent.ForkIdleSnapshotV1ToV2(ctx,[32]byte(run),checkpoint.ID,childID,child,v1,v2)
 if err!=nil{t.Fatal("V2 unstarted round migration:",err)}
 restored,err:=inputledger.Open(ctx,domain.ID(childID),v2,child)
 if err!=nil{t.Fatal(err)}
 _,_,_,rounds,_,gotRun:=restored.Snapshot()
 if [32]byte(gotRun)!=childID||len(rounds)!=1{t.Fatalf("V2 child round missing: %d",len(rounds))}
 if rounds[0].ID==sourceRound.ID||rounds[0].Actor!=sourceRound.Actor||
  rounds[0].SourceID!=sourceRound.SourceID||
  rounds[0].Eligibility!=sourceRound.Eligibility{
  t.Fatal("V2 round reused V1 identity or changed semantic membership")
 }
 pending,err:=child.PendingClock(ctx,childID)
 if err!=nil||len(pending)!=1||pending[0].ID!=rounds[0].ID{
  t.Fatalf("V2 round not pending after fork: %+v %v",pending,err)
 }
 if _,err:=child.ReplayClock(ctx,childID,v2);err!=nil{t.Fatal(err)}
 childSnapshot,err:=child.CreateSnapshotV1(ctx,childID,v2,nil);if err!=nil{t.Fatal(err)}
 if _,err:=child.VerifySnapshotV1(ctx,childID,childSnapshot.ID,v2);err!=nil{t.Fatal(err)}
 if err:=child.Close();err!=nil{t.Fatal(err)}
 reopened,err:=sqlite.Open(ctx,path);if err!=nil{t.Fatal(err)}
 defer reopened.Close()
 if _,err:=reopened.ReplayClock(ctx,childID,v2);err!=nil{t.Fatal(err)}
 if _,err:=parent.VerifySnapshotV1(ctx,[32]byte(run),checkpoint.ID,v1);err!=nil{t.Fatal(err)}
 if root.Payload[5]!=policy.CodecPolicyV2ID{t.Fatal("child Genesis lost V2 policy")}
}

func TestCSFRV18V2MigratesPartiallyDispatchedRoundWithCausalHistory(t *testing.T){
 ctx:=context.Background()
 parent,ledger,v1,run,origin:=csfrv18GenesisRoundSource(t)
 state,err:=parent.LoadClock(ctx,[32]byte(run));if err!=nil{t.Fatal(err)}
 due:=domain.EligibilityCoordinate{NotBeforeInstant:state.Instant}
 round,err:=ledger.DeclareRound(ctx,"genesis.schedule",[2]string{"school","a"},
  []string{"choice","reply"},due,origin,1,0,0,[]any{uint64(0)})
 if err!=nil{t.Fatal(err)}
 // A dispatch is an externally consumed capability; it cannot be replaced
 // by an unstarted child round with new IDs while hiding the historical slot.
 // The V2 migrator now preserves and verifies that capability history.
 cid,_,err:=identity.New(v1).Sum("cote.csf.id.cycle","cote.csf.schema.cycle-id-preimage",1,
  []any{run[:],[]any{int64(state.Instant),uint64(state.LastOrdinal+1)}})
 if err!=nil{t.Fatal(err)}
 did,_,err:=identity.New(v1).Sum("cote.csf.id.dispatch",
  "cote.csf.schema.dispatch-id-preimage",1,
  []any{run[:],cid[:],round.ID[:],"choice",uint64(0)})
 if err!=nil{t.Fatal(err)}
 dispatch:=inputledger.Dispatch{ID:domain.ID(did),RunID:run,
  CycleID:domain.ID(cid),RoundID:round.ID,SlotID:"choice",Actor:round.Actor,
  Eligibility:due,BaseRevision:state.Revision,EffectiveAt:state.Instant}
 if err:=ledger.RegisterDispatch(ctx,dispatch,inputledger.CycleBinding{
  ID:domain.ID(cid),BaseRevision:state.Revision,Instant:state.Instant});err!=nil{t.Fatal(err)}
 checkpoint,err:=parent.CreateSnapshotV1(ctx,[32]byte(run),v1,nil);if err!=nil{t.Fatal(err)}
 v2,err:=policy.LoadV2("../../../docs/architecture/canonical-codec-v2-bundle");if err!=nil{t.Fatal(err)}
 path:=t.TempDir()+"/partial-v2.db"
 child,err:=sqlite.Open(ctx,path);if err!=nil{t.Fatal(err)}
 defer child.Close()
 childID:=[32]byte(run);childID[0]^=0x71
 if _,err:=parent.ForkIdleSnapshotV1ToV2(ctx,[32]byte(run),checkpoint.ID,childID,child,v1,v2);err!=nil{
  t.Fatal("V2 partial dispatch migration:",err)
 }
 if _,err:=child.ReplayClock(ctx,childID,v2);err!=nil{t.Fatal(err)}
}

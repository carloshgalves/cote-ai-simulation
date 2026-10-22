package commit

import (
 "context"
 "testing"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
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

func TestCSFRV18V2RejectsPendingRoundWithNoV2Migrator(t *testing.T){
 ctx:=context.Background()
 parent,ledger,v1,run,origin:=csfrv18GenesisRoundSource(t)
 clock,err:=parent.LoadClock(ctx,[32]byte(run));if err!=nil{t.Fatal(err)}
 _,err=ledger.DeclareRound(ctx,"genesis.schedule",[2]string{"school","b"},
  []string{"choice"},domain.EligibilityCoordinate{NotBeforeInstant:clock.Instant+50},
  origin,0,1,0,[]any{uint64(0)})
 if err!=nil{t.Fatal(err)}
 checkpoint,err:=parent.CreateSnapshotV1(ctx,[32]byte(run),v1,nil);if err!=nil{t.Fatal(err)}
 v2,err:=policy.LoadV2("../../../docs/architecture/canonical-codec-v2-bundle")
 if err!=nil{t.Fatal(err)}
 path:=t.TempDir()+"/rejected-v2.db"
 child,err:=sqlite.Open(ctx,path);if err!=nil{t.Fatal(err)}
 defer child.Close()
 childID:=[32]byte(run);childID[0]^=0x79
 if _,err:=parent.ForkIdleSnapshotV1ToV2(ctx,[32]byte(run),checkpoint.ID,childID,child,v1,v2);err==nil{
  t.Fatal("V2 silently dropped active pending RoundDeclaration")
 }
 assertCSFRV18EmptyRejectedFork(t,path)
}

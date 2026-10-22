package commit

import (
 "bytes"
 "context"
 "database/sql"
 "testing"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/communication"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
 _ "modernc.org/sqlite"
)

func TestCSFRV18V2ForkImportsInTransitAsChildOwnedAuthorities(t *testing.T){
 ctx:=context.Background()
 c,source,ledger,cycle,fence,_:=communicationFixture(t)
 _,_,_,_,_,run:=ledger.Snapshot()
 if _,err:=c.CommitCommunicationSend(ctx,ledger,cycle,fence);err!=nil{t.Fatal(err)}
 snap,err:=source.CreateSnapshotV1(ctx,[32]byte(run),c.bundle,nil)
 if err!=nil{t.Fatal(err)}
 v2,err:=policy.LoadV2("../../../docs/architecture/canonical-codec-v2-bundle")
 if err!=nil{t.Fatal(err)}
 path:=t.TempDir()+"/imported-message-v2.db"
 child,err:=sqlite.Open(ctx,path);if err!=nil{t.Fatal(err)}
 id:=[32]byte(run);id[0]^=0x94
 root,err:=source.ForkIdleSnapshotV1ToV2(ctx,[32]byte(run),snap.ID,id,child,c.bundle,v2)
 if err!=nil{t.Fatal("atomic imported V2 communication fork:",err)}
 if root.Payload[5]!=policy.CodecPolicyV2ID{
  t.Fatal("import child still uses origin V1 policy")
 }
 pending,err:=child.PendingClock(ctx,id)
 if err!=nil||len(pending)!=1{t.Fatalf("imported communication not pending in child clock: %+v %v",pending,err)}
 dueID:=[32]byte(pending[0].ID)
 publication,err:=child.LoadCommunicationForOccurrence(ctx,id,dueID)
 if err!=nil{t.Fatal("imported child communication not live:",err)}
 if _,err:=communication.ValidateMigratedOccurrenceV2(v2,communication.SendPlan{
  Run:domain.ID(id),Claim:publication.Claim,Transmission:publication.Transmission,
  Occurrence:publication.Occurrence,
 },root.ID,root.Digest);err!=nil{
  t.Fatal("child Claim/Transmission/due not rekeyed and authentic:",err)
 }
 original,err:=source.PrepareInTransitDependenciesV1(ctx,[32]byte(run),snap.ID,c.bundle)
 if err!=nil||len(original)!=1{t.Fatal("parent transmission disappeared:",err)}
 if bytes.Equal(publication.Claim.Identity[:],original[0].ClaimID[:])||
  bytes.Equal(publication.Transmission.Identity[:],original[0].TransmissionID[:])||
  bytes.Equal(publication.Occurrence.Identity[:],original[0].OccurrenceID[:]){
  t.Fatal("import kept a parental causal identity")
 }
 if _,err:=child.ReplayClock(ctx,id,v2);err!=nil{t.Fatal("child after imported fork not replayable:",err)}
 childSnap,err:=child.CreateSnapshotV1(ctx,id,v2,nil)
 if err!=nil{t.Fatal("V2 imported child snapshot:",err)}
 if _,err:=child.VerifySnapshotV1(ctx,id,childSnap.ID,v2);err!=nil{t.Fatal(err)}
 if err:=child.Close();err!=nil{t.Fatal(err)}
 reopened,err:=sqlite.Open(ctx,path);if err!=nil{t.Fatal(err)}
 defer reopened.Close()
 if _,err:=reopened.ReplayClock(ctx,id,v2);err!=nil{
  t.Fatal("V2 transmission birth failed replay after process restart:",err)
 }
 if _,err:=source.VerifySnapshotV1(ctx,[32]byte(run),snap.ID,c.bundle);err!=nil{
  t.Fatal("child import mutated historical parent authority:",err)
 }
}

func TestCSFRV18V2ForkImportFaultRollsBackEveryChildTable(t *testing.T){
 ctx:=context.Background()
 c,source,ledger,cycle,fence,_:=communicationFixture(t)
 _,_,_,_,_,run:=ledger.Snapshot()
 if _,err:=c.CommitCommunicationSend(ctx,ledger,cycle,fence);err!=nil{t.Fatal(err)}
 snap,err:=source.CreateSnapshotV1(ctx,[32]byte(run),c.bundle,nil)
 if err!=nil{t.Fatal(err)}
 v2,err:=policy.LoadV2("../../../docs/architecture/canonical-codec-v2-bundle")
 if err!=nil{t.Fatal(err)}
 path:=t.TempDir()+"/fail-import.db"
 child,err:=sqlite.Open(ctx,path);if err!=nil{t.Fatal(err)}
 defer child.Close()
 db,err:=sql.Open("sqlite",path);if err!=nil{t.Fatal(err)}
 defer db.Close()
 if _,err:=db.ExecContext(ctx,`CREATE TRIGGER reject_v2_import
  BEFORE INSERT ON fork_communication_imports
  BEGIN SELECT RAISE(ABORT,'V2 injected migration birth interruption'); END;`);err!=nil{t.Fatal(err)}
 id:=[32]byte(run);id[0]^=0x95
 if _,err:=source.ForkIdleSnapshotV1ToV2(ctx,[32]byte(run),snap.ID,id,child,c.bundle,v2);err==nil{
  t.Fatal("injected import failure yielded a partially initialized child")
 }
 assertCSFRV18EmptyRejectedFork(t,path)
}

package commit

import (
 "bytes"
 "context"
 "database/sql"
 "testing"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/communication"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/inputledger"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
 _ "modernc.org/sqlite"
)

// Complete parent-internal perception of its send/round lifecycle before
// crossing policy boundaries. Unfinished perception tasks are not allowed
// to disappear merely because the delivery itself is still IN_TRANSIT.
func csfrv18SettleSendOutbox(t *testing.T,store *sqlite.Store,b *policy.Bundle,path string){
 t.Helper();ctx:=context.Background()
 db,err:=sql.Open("sqlite","file:"+path);if err!=nil{t.Fatal(err)}
 defer db.Close()
 rows,err:=db.QueryContext(ctx,"SELECT canonical FROM causal_outbox")
 if err!=nil{t.Fatal(err)}
 tasks:=map[string][32]byte{}
 for rows.Next(){
  var raw []byte
  if err:=rows.Scan(&raw);err!=nil{rows.Close();t.Fatal(err)}
  value,err:=codec.StrictDecodePayload(raw);if err!=nil{rows.Close();t.Fatal(err)}
  f,ok:=value.([]any);if !ok||len(f)!=10{continue}
  id,ok:=f[0].([]byte);if !ok||len(id)!=32{rows.Close();t.Fatal("bad task ID")}
  var event []byte
  if err:=db.QueryRowContext(ctx,"SELECT canonical FROM event_store WHERE id=?",f[3]).Scan(&event);err!=nil{
   rows.Close();t.Fatal(err)
  }
  ev,err:=codec.StrictDecodePayload(event);if err!=nil{rows.Close();t.Fatal(err)}
  kind,ok:=ev.([]any)[3].(string);if !ok{rows.Close();t.Fatal("task Event kind invalid")}
  var taskID [32]byte;copy(taskID[:],id);tasks[kind]=taskID
 }
 if err:=rows.Err();err!=nil{rows.Close();t.Fatal(err)}
 if err:=rows.Close();err!=nil{t.Fatal(err)}
 if _,ok:=tasks["communication.sent"];!ok{t.Fatal("missing sent Event perception task")}
 if _,ok:=tasks["round.consumed"];!ok{t.Fatal("missing round closure perception task")}
 if _,err:=store.CompleteInternalPerception(ctx,tasks["communication.sent"],b);err!=nil{t.Fatal(err)}
 if _,err:=store.DeliverRoundPerception(ctx,tasks["round.consumed"],b);err!=nil{t.Fatal(err)}
}

func TestCSFRV18V2ForkImportsInTransitAsChildOwnedAuthorities(t *testing.T){
 ctx:=context.Background()
 c,source,ledger,cycle,fence,sourcePath:=communicationFixture(t)
 _,_,_,_,_,run:=ledger.Snapshot()
 if _,err:=c.CommitCommunicationSend(ctx,ledger,cycle,fence);err!=nil{t.Fatal(err)}
 csfrv18SettleSendOutbox(t,source,c.bundle,sourcePath)
 clockState,err:=source.LoadClock(ctx,[32]byte(run))
 if err!=nil{t.Fatal(err)}
 // The original fixture leaves its exogenous driver intentionally open;
 // a fork must not lose that capability. Explicitly seal a newer FINAL
 // SourceClosure with a distinct producer key before exporting the cut.
 if _,err:=ledger.CloseSource(ctx,"driver",inputledger.Closure{
  ProducerKey:"v2-fork-final",ClosedThrough:domain.EligibilityCoordinate{
   NotBeforeInstant:clockState.Instant+1000},Final:true,
 });err!=nil{t.Fatal("finalize imported external source:",err)}
 checkpoints:=csfrv18SyntheticActorCheckpoints(t,sourcePath,clockState.Instant,int64(clockState.Revision))
 if len(checkpoints)==0{t.Fatal("no source cognitive actor to migrate")}
 snap,err:=source.CreateSnapshotV1(ctx,[32]byte(run),c.bundle,checkpoints)
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
 childSnap,err:=child.CreateSnapshotV1(ctx,id,v2,checkpoints)
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
 c,source,ledger,cycle,fence,sourcePath:=communicationFixture(t)
 _,_,_,_,_,run:=ledger.Snapshot()
 if _,err:=c.CommitCommunicationSend(ctx,ledger,cycle,fence);err!=nil{t.Fatal(err)}
 csfrv18SettleSendOutbox(t,source,c.bundle,sourcePath)
 clockState,err:=source.LoadClock(ctx,[32]byte(run))
 if err!=nil{t.Fatal(err)}
 // The original fixture leaves its exogenous driver intentionally open;
 // a fork must not lose that capability. Explicitly seal a newer FINAL
 // SourceClosure with a distinct producer key before exporting the cut.
 if _,err:=ledger.CloseSource(ctx,"driver",inputledger.Closure{
  ProducerKey:"v2-fork-final",ClosedThrough:domain.EligibilityCoordinate{
   NotBeforeInstant:clockState.Instant+1000},Final:true,
 });err!=nil{t.Fatal("finalize imported external source:",err)}
 checkpoints:=csfrv18SyntheticActorCheckpoints(t,sourcePath,clockState.Instant,int64(clockState.Revision))
 if len(checkpoints)==0{t.Fatal("no source cognitive actor to migrate")}
 snap,err:=source.CreateSnapshotV1(ctx,[32]byte(run),c.bundle,checkpoints)
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

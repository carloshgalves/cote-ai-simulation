package commit

import (
 "bytes"
 "context"
 "database/sql"
 "testing"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/communication"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/admission"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/clock"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
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
 // The imported message must actually be deliverable through the SAME
 // admission/CommitWork stack. Provenance is an archived V1 sent Event,
 // not a fabricated V2 send event.
 reloaded,err:=inputledger.Open(ctx,domain.ID(id),v2,reopened)
 if err!=nil{t.Fatal(err)}
 coordinator:=New(reopened,v2,c.admissionPolicies)
 advance,err:=coordinator.Advance(ctx,reloaded,domain.ID(id))
 if err!=nil{t.Fatal("V2 imported delivery cannot advance to due clock:",err)}
 pendingDue,err:=reopened.PendingClock(ctx,id);if err!=nil{t.Fatal(err)}
 selected,exists,err:=clock.Next(advance.ResultClock,pendingDue)
 if err!=nil||!exists||selected.Kind!=clock.Work{
  t.Fatalf("V2 imported due not scheduled as WORK: %+v %v",selected,err)
 }
 state:=advance.ResultClock
 cycleID,_,err:=identity.New(v2).Sum("cote.csf.id.cycle",
  "cote.csf.schema.cycle-id-preimage",1,
  []any{id[:],[]any{int64(state.Instant),uint64(selected.OriginOrdinal)}})
 if err!=nil{t.Fatal(err)}
 _,worldHash,err:=reopened.LoadWorldState(ctx,uint64(state.Revision))
 if err!=nil{t.Fatal(err)}
 dueCycle:=admission.Cycle{ID:domain.ID(cycleID),Attempt:0,
  Instant:state.Instant,Ordinal:selected.OriginOrdinal,BaseRevision:state.Revision,
  BaseStateHash:domain.Digest(worldHash),RetryRef:[]any{uint64(0)}}
 admitted,err:=admission.NewCoordinator(admission.NewFenceLog(reopened)).
  AdmitThenEvaluate(ctx,v2,reloaded,dueCycle,c.admissionPolicies,nil)
 if err!=nil{t.Fatal("imported V2 due not admitted:",err)}
 if _,err:=coordinator.CommitCommunicationDue(ctx,reloaded,dueCycle,admitted);err!=nil{
  t.Fatal("imported V2 due did not use authenticated terminal:",err)
 }
 if _,err:=coordinator.CommitCommunicationDue(ctx,reloaded,dueCycle,admitted);err==nil{
  t.Fatal("imported transmission delivered twice")
 }
 if _,err:=reopened.ReplayClock(ctx,id,v2);err!=nil{
  t.Fatal("V2 imported delivery terminal not replayable:",err)
 }
 // Adversarial re-review: a live child Transmission without its immutable
 // signed import birth must fail complete replay, even after delivery.
 tamper,err:=sql.Open("sqlite",path);if err!=nil{t.Fatal(err)}
 if _,err:=tamper.ExecContext(ctx,"DELETE FROM fork_communication_imports");err!=nil{
  tamper.Close();t.Fatal(err)
 }
 if err:=tamper.Close();err!=nil{t.Fatal(err)}
 if _,err:=reopened.ReplayClock(ctx,id,v2);err==nil{
  t.Fatal("replay accepted an imported Transmission after deletion of its V2 birth")
 }
 if _,err:=source.VerifySnapshotV1(ctx,[32]byte(run),snap.ID,c.bundle);err!=nil{
  t.Fatal("child import mutated historical parent authority:",err)
 }
 // Replay must fail if the signed migration birth disappears while its
 // rekeyed Transmission remains live/terminal; checking the remaining
 // receipt rows alone would falsely accept the orphan.
 probe,err:=sql.Open("sqlite","file:"+path)
 if err!=nil{t.Fatal(err)}
 defer probe.Close()
 if _,err:=probe.ExecContext(ctx,
  "DELETE FROM fork_communication_imports WHERE child_run_id=?",id[:]);err!=nil{t.Fatal(err)}
 if _,err:=reopened.ReplayClock(ctx,id,v2);err==nil{
  t.Fatal("V2 child replay accepted a transmission after its import proof was deleted")
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

package commit

import (
 "context"
 "testing"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
)

// An idle run with an unfinished perception outbox does not have a frozen WORK
// terminal to dispatch. Recovery must drain it without requiring a callback.
func TestCSFRV18RunnerDrainsIdlePerceptionWithoutWorkHandler(t *testing.T) {
 ctx:=context.Background()
 c,store,ledger,cycle,fence,_:=communicationFixture(t)
 _,_,_,_,_,run:=ledger.Snapshot()
 if _,err:=c.CommitCommunicationSend(ctx,ledger,cycle,fence);err!=nil{t.Fatal(err)}
 snapshot,err:=store.CreateSnapshotV1(ctx,[32]byte(run),c.bundle,nil)
 if err!=nil{t.Fatal(err)}
 initial,err:=store.VerifySnapshotV1(ctx,[32]byte(run),snapshot.ID,c.bundle)
 if err!=nil{t.Fatal(err)}
 if initial.ControlStatus!=0||initial.Action.Kind!=sqlite.ResumeDrainPerception||initial.PendingTasks==0{
  t.Fatalf("fixture is not idle with pending causal perception: %+v",initial)
 }
 for attempt:=0;attempt<2;attempt++{
  result,err:=c.ResumeRunV1(ctx,ledger,[32]byte(run),snapshot.ID,nil)
  if err!=nil{t.Fatalf("idle no-handler recovery attempt %d: %v",attempt,err)}
  if result.Continued||result.Publication.Identity!=([32]byte{})||
   result.Checkpoint.Action.Kind!=sqlite.ResumeIdle||result.Checkpoint.PendingTasks!=0{
   t.Fatalf("idle recovery emitted work or left perceptual barrier on attempt %d: %+v",attempt,result)
  }
 }
 if _,err:=store.VerifySnapshotV1(ctx,[32]byte(run),snapshot.ID,c.bundle);err!=nil{
  t.Fatalf("recovery changed the immutable snapshot history: %v",err)
 }
}

// In contrast, a persisted in-flight WORK always needs a terminal handler,
// before any draining or attempt transition. Never invent a handler on reboot.
func TestCSFRV18RunnerNilHandlerRejectsFrozenWork(t *testing.T) {
 ctx:=context.Background()
 c,store,ledger,_,run,_,_:=abortFixture(t)
 snapshot,err:=store.CreateSnapshotV1(ctx,[32]byte(run),c.bundle,nil)
 if err!=nil{t.Fatal(err)}
 before,err:=store.VerifySnapshotV1(ctx,[32]byte(run),snapshot.ID,c.bundle)
 if err!=nil{t.Fatal(err)}
 if before.ControlStatus!=1{t.Fatal("fixture is not in-flight")}
 if _,err:=c.ResumeRunV1(ctx,ledger,[32]byte(run),snapshot.ID,nil);err==nil{
  t.Fatal("in-flight cycle recovered without registered terminal handler")
 }
 after,err:=store.VerifySnapshotV1(ctx,[32]byte(run),snapshot.ID,c.bundle)
 if err!=nil{t.Fatal(err)}
 if after.ControlStatus!=before.ControlStatus||after.PendingTasks!=before.PendingTasks||
  after.Action.FenceID!=before.Action.FenceID{
  t.Fatal("missing handler changed the frozen attempt")
 }
}

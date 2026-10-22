package commit

import (
 "bytes"
 "context"
 "database/sql"
 "fmt"
 "testing"
 _ "modernc.org/sqlite"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/admission"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/clock"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/inputledger"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/trigger"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/extensions/testresourcev1"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/communication"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/fork"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
)

// These tests run the V8 snapshot path on the real V3-V7 commit journal.
// They deliberately reopen SQLite rather than trusting a process cache.
func TestCSFRV18SnapshotReopenClockAdvanceAndCanonicalBytes(t *testing.T){
 ctx:=context.Background()
 coordinator,store,ledger,run,bundle,path:=testFixture(t,activeTrigger())
 before,err:=store.CreateSnapshotV1(ctx,[32]byte(run),bundle,nil)
 if err!=nil{t.Fatal(err)}
 if len(before.Canonical)==0||before.ID==([32]byte{}){t.Fatal("snapshot must be durable and canonical")}
 verified,err:=store.VerifySnapshotV1(ctx,[32]byte(run),before.ID,bundle)
 if err!=nil{t.Fatal(err)}
 if !bytes.Equal(before.Canonical,verified.Snapshot.Canonical){t.Fatal("initial snapshot differs from verified bytes")}
 if _,err:=coordinator.Advance(ctx,ledger,run);err!=nil{t.Fatal(err)}
 after,err:=store.CreateSnapshotV1(ctx,[32]byte(run),bundle,nil)
 if err!=nil{t.Fatal(err)}
 if before.ID==after.ID||after.Revision!=before.Revision+1{t.Fatal("new revision reused snapshot identity")}
 historical,err:=store.VerifySnapshotV1(ctx,[32]byte(run),before.ID,bundle)
 if err!=nil{t.Fatalf("valid historical snapshot and tail failed verification: %v",err)}
 if historical.SnapshotClock.Revision!=0||historical.Clock.Revision!=1||historical.TailCommits!=1{
  t.Fatalf("snapshot + tail changed causal clock history: %+v",historical)
 }
 reopened,err:=sqlite.Open(ctx,path);if err!=nil{t.Fatal(err)};defer reopened.Close()
 result,err:=reopened.VerifySnapshotV1(ctx,[32]byte(run),after.ID,bundle)
 if err!=nil{t.Fatal(err)}
 if !bytes.Equal(result.Snapshot.Canonical,after.Canonical)||result.Clock.Revision!=1{
  t.Fatal("reopen failed to restore the same canonical checkpoint")
 }
 again,err:=reopened.CreateSnapshotV1(ctx,[32]byte(run),bundle,nil)
 if err!=nil{t.Fatal(err)}
 if !bytes.Equal(again.Canonical,after.Canonical)||again.Digest!=after.Digest{
  t.Fatal("idempotent same-revision snapshot was not byte-identical")
 }
}

func TestCSFRV18SnapshotTamperFailClosed(t *testing.T){
 ctx:=context.Background()
 _,store,_,run,bundle,path:=testFixture(t)
 snapshot,err:=store.CreateSnapshotV1(ctx,[32]byte(run),bundle,nil)
 if err!=nil{t.Fatal(err)}
 other,err:=sql.Open("sqlite",path)
 if err!=nil{t.Fatal(err)}
 defer other.Close()
 corrupted:=append([]byte(nil),snapshot.Canonical...)
 corrupted[len(corrupted)-1]^=1
 if _,err:=other.ExecContext(ctx,"UPDATE snapshots SET canonical=? WHERE id=?",corrupted,snapshot.ID[:]);err!=nil{t.Fatal(err)}
 if _,err:=store.VerifySnapshotV1(ctx,[32]byte(run),snapshot.ID,bundle);err==nil{
  t.Fatal("accepted corrupt persisted checkpoint")
 }
 if _,err:=other.ExecContext(ctx,"UPDATE snapshots SET canonical=? WHERE id=?",snapshot.Canonical,snapshot.ID[:]);err!=nil{t.Fatal(err)}
 if _,err:=store.VerifySnapshotV1(ctx,[32]byte(run),snapshot.ID,bundle);err!=nil{t.Fatal(err)}
 if _,err:=other.ExecContext(ctx,"DELETE FROM snapshots WHERE id=?",snapshot.ID[:]);err!=nil{t.Fatal(err)}
 if _,err:=store.VerifySnapshotV1(ctx,[32]byte(run),snapshot.ID,bundle);err==nil{
  t.Fatal("missing snapshot was treated as resumable")
 }
}

func TestCSFRV18RejectStaleAndDuplicateEpistemicRefs(t *testing.T){
 ctx:=context.Background()
 _,store,_,run,bundle,_:=testFixture(t)
 stale:=sqlite.SnapshotCheckpoint{ActorNamespace:"test",ActorID:"agent",StoreID:"memory",SchemaVersion:1,
  Content:[]byte("opaque"),Producer:"memory-owner",ProducerVersion:1,
  CoversInstant:-999999,CoversRevision:0,CoversEvidenceCursor:0}
 if _,err:=store.CreateSnapshotV1(ctx,[32]byte(run),bundle,[]sqlite.SnapshotCheckpoint{stale});err==nil{
  t.Fatal("stale checkpoint coverage accepted")
 }
 stale.CoversInstant=1<<50
 if _,err:=store.CreateSnapshotV1(ctx,[32]byte(run),bundle,[]sqlite.SnapshotCheckpoint{stale,stale});err==nil{
  t.Fatal("duplicate actor checkpoint accepted")
 }
}

func TestCSFRV18SnapshotProjectsRegisteredPendingState(t *testing.T){
 ctx:=context.Background()
 coordinator,store,ledger,run,bundle,_:=testFixture(t,activeTrigger())
 check:=func(s sqlite.SnapshotV1,expectActivation bool){
  t.Helper()
  decoded,err:=codec.StrictDecodePayload(s.Canonical);if err!=nil{t.Fatal(err)}
  fields,ok:=decoded.([]any);if !ok||len(fields)!=19{t.Fatal("snapshot missing fields")}
  typed,ok:=fields[11].([]any);if !ok||len(typed)!=4{t.Fatal("pending typed record absent")}
  pending,err:=codec.StrictDecodeEnvelope(typed[2].([]byte));if err!=nil{t.Fatal(err)}
  lists,ok:=pending.Value.([]any);if !ok||len(lists)!=10{t.Fatal("wrong number of pending partitions")}
  wantActivations:=0;if expectActivation{wantActivations=1}
  if len(lists[0].([]any))!=1||len(lists[1].([]any))!=1||
   len(lists[2].([]any))!=wantActivations||len(lists[3].([]any))!=1 {
    t.Fatalf("missing occurrence, runtime, activation or closure: %v %v %v %v",
     len(lists[0].([]any)),len(lists[1].([]any)),len(lists[2].([]any)),len(lists[3].([]any)))
  }
  occ:=lists[0].([]any)[0].([]any)
  closure:=lists[3].([]any)[0].([]any)
  if occ[0]!=uint64(6)||closure[0]!=uint64(2)||
   len(occ[2].([]byte))!=32||len(closure[2].([]byte))!=32{
   t.Fatal("snapshot uses unregistered unit ref or empty digest")
  }
  units:=lists[7].([]any)
  if len(units)==0{t.Fatal("pending input ref list unexpectedly empty")}
  for _,unit:=range units{
   ref,ok:=unit.([]any);if !ok||len(ref)!=3||
    len(ref[1].([]byte))!=32||len(ref[2].([]byte))!=32 {
    t.Fatal("snapshot pending input refs have wrong canonical shape")
   }
  }
 }
 before,err:=store.CreateSnapshotV1(ctx,[32]byte(run),bundle,nil);if err!=nil{t.Fatal(err)}
 check(before,false)
 if _,err:=coordinator.Advance(ctx,ledger,run);err!=nil{t.Fatal(err)}
 after,err:=store.CreateSnapshotV1(ctx,[32]byte(run),bundle,nil);if err!=nil{t.Fatal(err)}
 check(after,true)
 restored,err:=store.VerifySnapshotV1(ctx,[32]byte(run),after.ID,bundle)
 if err!=nil{t.Fatal(err)}
 if restored.PendingTasks==0{t.Fatal("uncompleted perception task lost on resume")}
}

func TestCSFRV18SelfHashedForgedSnapshotPrefixStillFailsClosed(t *testing.T){
 ctx:=context.Background()
 _,store,_,run,bundle,path:=testFixture(t)
 snapshot,err:=store.CreateSnapshotV1(ctx,[32]byte(run),bundle,nil);if err!=nil{t.Fatal(err)}
 fieldsRaw,err:=codec.StrictDecodePayload(snapshot.Canonical);if err!=nil{t.Fatal(err)}
 fields:=fieldsRaw.([]any)
 cursors:=fields[12].([]any)
 first:=cursors[0].([]any)
 first[1]=first[1].(uint64)+1 // caller forges a wrong prefix cursor
 d,_,err:=identity.New(bundle).Sum("cote.csf.hash.snapshot","cote.csf.schema.snapshot.body",1,fields[:18])
 if err!=nil{t.Fatal(err)}
 fields[18]=d[:]
 forged,err:=codec.CanonicalPayload(fields);if err!=nil{t.Fatal(err)}
 q,err:=sql.Open("sqlite",path);if err!=nil{t.Fatal(err)};defer q.Close()
 if _,err:=q.ExecContext(ctx,"UPDATE snapshots SET canonical=? WHERE id=?",forged,snapshot.ID[:]);err!=nil{t.Fatal(err)}
 if _,err:=store.VerifySnapshotV1(ctx,[32]byte(run),snapshot.ID,bundle);err==nil{
  t.Fatal("accepted rehashed snapshot with false ledger prefix")
 }
}

func TestCSFRV18MissingSealAndCorruptedHistoricalMembershipFailClosed(t *testing.T){
 ctx:=context.Background()
 _,store,_,run,bundle,path:=testFixture(t)
 s,err:=store.CreateSnapshotV1(ctx,[32]byte(run),bundle,nil);if err!=nil{t.Fatal(err)}
 q,err:=sql.Open("sqlite",path);if err!=nil{t.Fatal(err)};defer q.Close()
 if _,err:=q.ExecContext(ctx,"UPDATE snapshot_prefix_members SET canonical_hash=zeroblob(32) WHERE snapshot_id=? AND ledger_name='input_ledger'",s.ID[:]);err!=nil{t.Fatal(err)}
 if _,err:=store.VerifySnapshotV1(ctx,[32]byte(run),s.ID,bundle);err==nil{t.Fatal("historical member digest tampering accepted")}
 if _,err:=q.ExecContext(ctx,"DELETE FROM snapshot_seals WHERE snapshot_id=?",s.ID[:]);err!=nil{t.Fatal(err)}
 if _,err:=store.VerifySnapshotV1(ctx,[32]byte(run),s.ID,bundle);err==nil{t.Fatal("deleted immutable snapshot seal accepted")}
}

func TestCSFRV18SnapshotPublicationIsTransactionalUnderFailure(t *testing.T){
 ctx:=context.Background()
 _,store,_,run,bundle,path:=testFixture(t)
 q,err:=sql.Open("sqlite",path);if err!=nil{t.Fatal(err)};defer q.Close()
 if _,err:=q.ExecContext(ctx,`CREATE TRIGGER csfrv18_fail_seal BEFORE INSERT ON snapshot_seals BEGIN SELECT RAISE(ABORT,'injected seal crash'); END`);err!=nil{t.Fatal(err)}
 if _,err:=store.CreateSnapshotV1(ctx,[32]byte(run),bundle,nil);err==nil{t.Fatal("ignored injected seal failure")}
 var snapshots,members int
 if err:=q.QueryRowContext(ctx,"SELECT count(*) FROM snapshots").Scan(&snapshots);err!=nil{t.Fatal(err)}
 if err:=q.QueryRowContext(ctx,"SELECT count(*) FROM snapshot_prefix_members").Scan(&members);err!=nil{t.Fatal(err)}
 if snapshots!=0||members!=0{t.Fatalf("non-atomic failed publication: snapshots=%d members=%d",snapshots,members)}
 if _,err:=q.ExecContext(ctx,"DROP TRIGGER csfrv18_fail_seal");err!=nil{t.Fatal(err)}
 created,err:=store.CreateSnapshotV1(ctx,[32]byte(run),bundle,nil);if err!=nil{t.Fatal(err)}
 if _,err:=store.VerifySnapshotV1(ctx,[32]byte(run),created.ID,bundle);err!=nil{t.Fatalf("reopen after injected failure: %v",err)}
}

func TestCSFRV18HistoricalSnapshotRejectsCorruptedTail(t *testing.T){
 ctx:=context.Background()
 coordinator,store,ledger,run,bundle,path:=testFixture(t,activeTrigger())
 origin,err:=store.CreateSnapshotV1(ctx,[32]byte(run),bundle,nil);if err!=nil{t.Fatal(err)}
 result,err:=coordinator.Advance(ctx,ledger,run);if err!=nil{t.Fatal(err)}
 if _,err:=store.VerifySnapshotV1(ctx,[32]byte(run),origin.ID,bundle);err!=nil{t.Fatal(err)}
 q,err:=sql.Open("sqlite",path);if err!=nil{t.Fatal(err)};defer q.Close()
 event:=result.Events[0]
 damaged:=append([]byte(nil),event.Canonical...)
 damaged[len(damaged)-1]^=1
 if _,err:=q.ExecContext(ctx,"UPDATE event_store SET canonical=? WHERE id=?",damaged,event.Identity[:]);err!=nil{t.Fatal(err)}
 if _,err:=store.VerifySnapshotV1(ctx,[32]byte(run),origin.ID,bundle);err==nil{
  t.Fatal("historical prefix resumed over a corrupted terminal event tail")
 }
}

func TestCSFRV18ForkIdleSamePolicyEndToEnd(t *testing.T) {
 ctx:=context.Background()
 bundle,err:=policy.Load(bundlePath);if err!=nil{t.Fatal(err)}
 root:=validGenesis(t,bundle)
 parent,err:=sqlite.Open(ctx,t.TempDir()+"/parent.db");if err!=nil{t.Fatal(err)};defer parent.Close()
 if err:=parent.PersistGenesis(ctx,root,bundle);err!=nil{t.Fatal(err)}
 if err:=parent.BootstrapClock(ctx,root,bundle);err!=nil{t.Fatal(err)}
 if err:=parent.BootstrapGenesisTriggers(ctx,root,bundle,nil);err!=nil{t.Fatal(err)}
 checkpoint,err:=parent.CreateSnapshotV1(ctx,[32]byte(root.RunID),bundle,nil)
 if err!=nil{t.Fatal(err)}
 child,err:=sqlite.Open(ctx,t.TempDir()+"/child.db");if err!=nil{t.Fatal(err)};defer child.Close()
 childID:=[32]byte(root.RunID);childID[0]^=0x5a
 branch,err:=parent.ForkIdleSnapshotV1(ctx,[32]byte(root.RunID),checkpoint.ID,childID,child,bundle)
 if err!=nil{t.Fatal(err)}
 if [32]byte(branch.RunID)!=childID||branch.Digest==root.Digest{t.Fatal("fork inherited parent identity")}
 if _,err:=child.ValidatedGenesis(ctx,childID,bundle);err!=nil{t.Fatalf("child Genesis unverifiable: %v",err)}
 if _,err:=child.ReplayClock(ctx,childID,bundle);err!=nil{t.Fatalf("fork causal replay failed: %v",err)}
 branchSnap,err:=child.CreateSnapshotV1(ctx,childID,bundle,nil)
 if err!=nil{t.Fatal(err)}
 if branchSnap.ID==checkpoint.ID{t.Fatal("fork shares snapshot identity with parent")}
 _,err=child.VerifySnapshotV1(ctx,childID,branchSnap.ID,bundle)
 if err!=nil{t.Fatalf("fork resume failed: %v",err)}
 if _,err:=parent.VerifySnapshotV1(ctx,[32]byte(root.RunID),checkpoint.ID,bundle);err!=nil{t.Fatalf("child mutated parent: %v",err)}
 // Cross-workspace history has the parent's exact bytes, not its live ledgers.
 parentHistory:=branch.Payload[4].([]any)
 ref,err:=fork.Decode(parentHistory[1].([]byte));if err!=nil{t.Fatal(err)}
 if err:=fork.VerifyOriginSnapshot(ref,checkpoint.Canonical,bundle);err!=nil{t.Fatal(err)}
}

func TestCSFRV18RejectForkWithPendingScheduleAndReusedIdentity(t *testing.T) {
 ctx:=context.Background()
 _,parent,_,run,bundle,_:=testFixture(t,activeTrigger())
 checkpoint,err:=parent.CreateSnapshotV1(ctx,[32]byte(run),bundle,nil)
 if err!=nil{t.Fatal(err)}
 child,err:=sqlite.Open(ctx,t.TempDir()+"/child.db");if err!=nil{t.Fatal(err)};defer child.Close()
 if _,err:=parent.ForkIdleSnapshotV1(ctx,[32]byte(run),checkpoint.ID,[32]byte(run),child,bundle);err==nil{t.Fatal("same-run fork accepted")}
 other:=[32]byte(run);other[0]^=1
 if _,err:=parent.ForkIdleSnapshotV1(ctx,[32]byte(run),checkpoint.ID,other,child,bundle);err==nil{t.Fatal("fork dropped pending work without migration")}
}

func TestCSFRV18ForkRollbackAndArchiveTamperReject(t *testing.T){
 ctx:=context.Background()
 bundle,err:=policy.Load(bundlePath);if err!=nil{t.Fatal(err)}
 g:=validGenesis(t,bundle)
 parent,err:=sqlite.Open(ctx,t.TempDir()+"/parent.db");if err!=nil{t.Fatal(err)};defer parent.Close()
 if err:=parent.PersistGenesis(ctx,g,bundle);err!=nil{t.Fatal(err)}
 if err:=parent.BootstrapClock(ctx,g,bundle);err!=nil{t.Fatal(err)}
 if err:=parent.BootstrapGenesisTriggers(ctx,g,bundle,nil);err!=nil{t.Fatal(err)}
 snapshot,err:=parent.CreateSnapshotV1(ctx,[32]byte(g.RunID),bundle,nil);if err!=nil{t.Fatal(err)}
 childPath:=t.TempDir()+"/child.db"
 child,err:=sqlite.Open(ctx,childPath);if err!=nil{t.Fatal(err)};defer child.Close()
 raw,err:=sql.Open("sqlite",childPath);if err!=nil{t.Fatal(err)};defer raw.Close()
 childID:=[32]byte(g.RunID);childID[0]^=0x75
 if _,err:=raw.ExecContext(ctx,`CREATE TRIGGER fail_fork BEFORE INSERT ON fork_lineage
 BEGIN SELECT RAISE(ABORT,'injected fork lineage failure'); END`);err!=nil{t.Fatal(err)}
 if _,err:=parent.ForkIdleSnapshotV1(ctx,[32]byte(g.RunID),snapshot.ID,childID,child,bundle);err==nil{
  t.Fatal("fork ignored failure between genesis and lineage")
 }
 for _,table:=range []string{"genesis_records","world_revisions","clock_states","fork_lineage","fork_history_records"}{
  var count int
  if err:=raw.QueryRowContext(ctx,"SELECT count(*) FROM "+table).Scan(&count);err!=nil{t.Fatal(err)}
  if count!=0{t.Fatalf("fork crash partially wrote %s: %d",table,count)}
 }
 if _,err:=raw.ExecContext(ctx,"DROP TRIGGER fail_fork");err!=nil{t.Fatal(err)}
 if _,err:=parent.ForkIdleSnapshotV1(ctx,[32]byte(g.RunID),snapshot.ID,childID,child,bundle);err!=nil{t.Fatal(err)}
 if _,err:=child.ValidatedGenesis(ctx,childID,bundle);err!=nil{t.Fatal(err)}
 if _,err:=raw.ExecContext(ctx,"UPDATE fork_lineage SET reference_canonical=x'00' WHERE child_run=?",childID[:]);err!=nil{t.Fatal(err)}
 if _,err:=child.ValidatedGenesis(ctx,childID,bundle);err==nil{
  t.Fatal("fork accepted forged archived parent reference")
 }
}

func TestCSFRV18TwoForksNeverAliasChildRunIDs(t *testing.T){
 ctx:=context.Background()
 bundle,err:=policy.Load(bundlePath);if err!=nil{t.Fatal(err)}
 g:=validGenesis(t,bundle)
 source,err:=sqlite.Open(ctx,t.TempDir()+"/source.db");if err!=nil{t.Fatal(err)};defer source.Close()
 if err:=source.PersistGenesis(ctx,g,bundle);err!=nil{t.Fatal(err)}
 if err:=source.BootstrapClock(ctx,g,bundle);err!=nil{t.Fatal(err)}
 if err:=source.BootstrapGenesisTriggers(ctx,g,bundle,nil);err!=nil{t.Fatal(err)}
 checkpoint,err:=source.CreateSnapshotV1(ctx,[32]byte(g.RunID),bundle,nil);if err!=nil{t.Fatal(err)}
 idA:=[32]byte(g.RunID);idA[0]^=0x55
 idB:=[32]byte(g.RunID);idB[0]^=0x33
 storeA,err:=sqlite.Open(ctx,t.TempDir()+"/a.db");if err!=nil{t.Fatal(err)};defer storeA.Close()
 storeB,err:=sqlite.Open(ctx,t.TempDir()+"/b.db");if err!=nil{t.Fatal(err)};defer storeB.Close()
 a,err:=source.ForkIdleSnapshotV1(ctx,[32]byte(g.RunID),checkpoint.ID,idA,storeA,bundle);if err!=nil{t.Fatal(err)}
 b,err:=source.ForkIdleSnapshotV1(ctx,[32]byte(g.RunID),checkpoint.ID,idB,storeB,bundle);if err!=nil{t.Fatal(err)}
 if bytes.Equal(a.CanonicalBytes,b.CanonicalBytes)||a.ID==b.ID{t.Fatal("fork Genesis identity aliased")}
 snapA,err:=storeA.CreateSnapshotV1(ctx,idA,bundle,nil);if err!=nil{t.Fatal(err)}
 snapB,err:=storeB.CreateSnapshotV1(ctx,idB,bundle,nil);if err!=nil{t.Fatal(err)}
 if snapA.ID==snapB.ID||bytes.Equal(snapA.Canonical,snapB.Canonical){
  t.Fatal("divergent child runs share causal snapshot identities")
 }
 if _,err:=source.VerifySnapshotV1(ctx,[32]byte(g.RunID),checkpoint.ID,bundle);err!=nil{
  t.Fatal("parent prefix changed when two children were created:",err)
 }
}

func TestCSFRV18SnapshotRealInFlightHaltedRetryRecovery(t *testing.T){
 for _,stage:=range []string{"fence","halted","retry"}{
  t.Run(stage,func(t *testing.T){
   ctx:=context.Background()
   coordinator,store,_,fence,run,cycle,path:=abortFixture(t)
   var expected uint64=1
   if stage!="fence"{
    abort,err:=coordinator.Abort(ctx,run,cycle,fence,4,nil)
    if err!=nil{t.Fatal(err)}
    expected=3
    if stage=="retry"{
     if _,err:=coordinator.Retry(ctx,run,cycle,abort,"operator.retry");err!=nil{t.Fatal(err)}
     expected=2
    }
   }
   snap,err:=store.CreateSnapshotV1(ctx,[32]byte(run),coordinator.bundle,nil)
   if err!=nil{t.Fatalf("snapshot at %s failed: %v",stage,err)}
   reopened,err:=sqlite.Open(ctx,path);if err!=nil{t.Fatal(err)};defer reopened.Close()
   restored,err:=reopened.VerifySnapshotV1(ctx,[32]byte(run),snap.ID,coordinator.bundle)
   if err!=nil{t.Fatalf("resume at %s failed: %v",stage,err)}
   if restored.ControlStatus!=expected||restored.SnapshotControlStatus!=expected{
    t.Fatalf("restore changed terminal control at %s: %+v",stage,restored)
   }
   if expected==3&&restored.Action.Kind!=sqlite.ResumeHalted{
    t.Fatal("HALTED_ON_ABORT resumed without explicit operator authorization")
   }
   if expected==1&&restored.Action.Kind!=sqlite.ResumeReexecuteFencedAttempt&&
    restored.Action.Kind!=sqlite.ResumeDrainPerception{
    t.Fatal("in-flight fence incorrectly reopened a new cycle")
   }
   if expected==2&&restored.Action.Kind!=sqlite.ResumeFenceAuthorizedRetry&&
    restored.Action.Kind!=sqlite.ResumeDrainPerception{
    t.Fatal("authorized retry did not preserve same cycle")
   }
   if (expected==1||expected==2)&&restored.Action.CycleID!=[32]byte(cycle){
    t.Fatal("resume discarded the frozen cycle identity")
   }
  })
 }
}

func TestCSFRV18CommunicationSnapshotInTransitReplayAndDeliveryOnce(t *testing.T){
 ctx:=context.Background()
 c,store,ledger,sendCycle,sendFence,path:=communicationFixture(t)
 _,_,_,_,_,run:=ledger.Snapshot()
 if _,err:=c.CommitCommunicationSend(ctx,ledger,sendCycle,sendFence);err!=nil{t.Fatal(err)}
 checkpoint,err:=store.CreateSnapshotV1(ctx,[32]byte(run),c.bundle,nil)
 if err!=nil{t.Fatalf("snapshot while allegation IN_TRANSIT: %v",err)}
 restarted,err:=sqlite.Open(ctx,path);if err!=nil{t.Fatal(err)};defer restarted.Close()
 tip,err:=restarted.VerifySnapshotV1(ctx,[32]byte(run),checkpoint.ID,c.bundle)
 if err!=nil{t.Fatalf("resuming in-transit transmission: %v",err)}
 if tip.Clock.Revision!=1||tip.PendingTasks==0{
  t.Fatal("snapshot lost uncompleted perception tasks at send boundary")
 }
 var status int
 raw,err:=sql.Open("sqlite","file:"+path);if err!=nil{t.Fatal(err)};defer raw.Close()
 if err:=raw.QueryRowContext(ctx,"SELECT status FROM communication_transmissions").Scan(&status);err!=nil||status!=0{
  t.Fatal("checkpoint consumed or lost an undelivered claim")
 }
 advanced,err:=c.Advance(ctx,ledger,run);if err!=nil{t.Fatal(err)}
 pending,err:=store.PendingClock(ctx,[32]byte(run));if err!=nil{t.Fatal(err)}
 selected,ok,err:=clock.Next(advanced.ResultClock,pending)
 if err!=nil||!ok||selected.Kind!=clock.Work{t.Fatal("due transmission not selected",err)}
 nextCycleID,_,err:=identity.New(c.bundle).Sum("cote.csf.id.cycle","cote.csf.schema.cycle-id-preimage",1,
  []any{run[:],[]any{int64(advanced.ResultClock.Instant),uint64(selected.OriginOrdinal)}})
 if err!=nil{t.Fatal(err)}
 _,hash,err:=store.LoadWorldState(ctx,uint64(advanced.ResultClock.Revision));if err!=nil{t.Fatal(err)}
 nextCycle:=admission.Cycle{ID:domain.ID(nextCycleID),Attempt:0,Instant:advanced.ResultClock.Instant,
  Ordinal:selected.OriginOrdinal,BaseRevision:advanced.ResultClock.Revision,
  BaseStateHash:domain.Digest(hash),RetryRef:[]any{uint64(0)}}
 dueFence,err:=admission.NewCoordinator(admission.NewFenceLog(store)).
  AdmitThenEvaluate(ctx,c.bundle,ledger,nextCycle,c.admissionPolicies,nil)
 if err!=nil{t.Fatal(err)}
 if _,err:=c.CommitCommunicationDue(ctx,ledger,nextCycle,dueFence);err!=nil{t.Fatal(err)}
 if _,err:=c.CommitCommunicationDue(ctx,ledger,nextCycle,dueFence);err==nil{t.Fatal("duplicate delivery accepted after replay")}
 if err:=raw.QueryRowContext(ctx,"SELECT status FROM communication_transmissions").Scan(&status);err!=nil||status!=1{
  t.Fatal("channel no longer delivered exactly once",status,err)
 }
 recovered,err:=restarted.VerifySnapshotV1(ctx,[32]byte(run),checkpoint.ID,c.bundle)
 if err!=nil{t.Fatalf("historical in-transit checkpoint plus delivery tail: %v",err)}
 if recovered.TailCommits!=2||recovered.Clock.Revision!=3{
  t.Fatalf("snapshot plus delivered tail lost causal transitions: %+v",recovered)
 }
 _,beforeHash,err:=store.LoadWorldState(ctx,0);if err!=nil{t.Fatal(err)}
 _,afterHash,err:=store.LoadWorldState(ctx,3);if err!=nil{t.Fatal(err)}
 if beforeHash!=afterHash{t.Fatal("replayed allegation promoted false claim to world truth")}
 tipSnapshot,err:=store.CreateSnapshotV1(ctx,[32]byte(run),c.bundle,nil)
 if err!=nil{t.Fatal("tip snapshot after channel delivery:",err)}
 data,err:=codec.StrictDecodePayload(tipSnapshot.Canonical);if err!=nil{t.Fatal(err)}
 parts:=data.([]any)[11].([]any)
 pendingEnvelope,err:=codec.StrictDecodeEnvelope(parts[2].([]byte));if err!=nil{t.Fatal(err)}
 partitions:=pendingEnvelope.Value.([]any)
 if len(partitions[0].([]any))!=0{
  t.Fatal("already delivered ScheduledOccurrence remained as pending checkpoint authority")
 }
}

func TestCSFRV18ForkCanArchiveFinalSourceClosureWithoutReopeningIngress(t *testing.T){
 ctx:=context.Background()
 bundle,err:=policy.Load(bundlePath);if err!=nil{t.Fatal(err)}
 g:=validGenesis(t,bundle)
 source,err:=sqlite.Open(ctx,t.TempDir()+"/origin.db");if err!=nil{t.Fatal(err)};defer source.Close()
 if err:=source.PersistGenesis(ctx,g,bundle);err!=nil{t.Fatal(err)}
 if err:=source.BootstrapClock(ctx,g,bundle);err!=nil{t.Fatal(err)}
 if err:=source.BootstrapGenesisTriggers(ctx,g,bundle,nil);err!=nil{t.Fatal(err)}
 ledger,err:=inputledger.New(ctx,g.RunID,bundle,source);if err!=nil{t.Fatal(err)}
 if err:=ledger.DeclareSource(inputledger.Source{ID:"closed-origin"});err!=nil{t.Fatal(err)}
 at:=domain.EligibilityCoordinate{NotBeforeInstant:g.InitialInstant+50}
 if _,err:=ledger.CloseSource(ctx,"closed-origin",inputledger.Closure{
  ProducerKey:"last-closure",ClosedThrough:at,Final:true});err!=nil{t.Fatal(err)}
 origin,err:=source.CreateSnapshotV1(ctx,[32]byte(g.RunID),bundle,nil);if err!=nil{t.Fatal(err)}
 childPath:=t.TempDir()+"/child.db"
 child,err:=sqlite.Open(ctx,childPath);if err!=nil{t.Fatal(err)};defer child.Close()
 id:=[32]byte(g.RunID);id[0]^=0x71
 if _,err:=source.ForkIdleSnapshotV1(ctx,[32]byte(g.RunID),origin.ID,id,child,bundle);err!=nil{
  t.Fatalf("terminal source closure is historical, not pending work: %v",err)
 }
 if _,err:=child.ReplayClock(ctx,id,bundle);err!=nil{t.Fatal(err)}
 forkInputs,err:=inputledger.New(ctx,domain.ID(id),bundle,child);if err!=nil{t.Fatal(err)}
 if err:=forkInputs.DeclareSource(inputledger.Source{ID:"closed-origin"});err==nil{
  t.Fatal("final exogenous source was reopened through a child timeline")
 }
 if err:=forkInputs.DeclareSource(inputledger.Source{ID:"fresh-child"});err!=nil{
  t.Fatal("fork wrongly blocked an independent new child source:",err)
 }
 if _,err:=source.VerifySnapshotV1(ctx,[32]byte(g.RunID),origin.ID,bundle);err!=nil{
  t.Fatal("fork modified immutable parental source history",err)
 }
 childRaw,err:=sql.Open("sqlite",childPath);if err!=nil{t.Fatal(err)};defer childRaw.Close()
 if _,err:=childRaw.ExecContext(ctx,
  "UPDATE fork_history_records SET canonical=x'01' WHERE child_run=? AND ledger_name='input_ledger' AND ordinal=0",id[:]);err!=nil{t.Fatal(err)}
 if _,err:=child.ValidatedGenesis(ctx,id,bundle);err==nil{
  t.Fatal("tampered historical parent prefix was accepted by child fork")
 }
 if err:=forkInputs.DeclareSource(inputledger.Source{ID:"closed-origin"});err==nil{
  t.Fatal("malformed archived source bytes reopened a previously finalized source")
 }
}

func TestCSFRV18SnapshotDeterministicAcrossWorkerAndTriggerOrder(t *testing.T){
 ctx:=context.Background()
 var baseline []byte
 var baseDigest [32]byte
 for seed:=0;seed<4;seed++{
  first:=activeTrigger();second:=activeTrigger();second.ID[0]=0x82
  defs:=[]trigger.Definition{first,second}
  if seed%2==1{defs[0],defs[1]=defs[1],defs[0]}
  c,store,ledger,run,bundle,_:=testFixture(t,defs...)
  if _,err:=c.Advance(ctx,ledger,run);err!=nil{t.Fatal(err)}
  snap,err:=store.CreateSnapshotV1(ctx,[32]byte(run),bundle,nil)
  if err!=nil{t.Fatal(err)}
  if seed==0{baseline=bytes.Clone(snap.Canonical);baseDigest=snap.Digest;continue}
  if !bytes.Equal(baseline,snap.Canonical)||baseDigest!=snap.Digest{
   t.Fatalf("same semantic run changed snapshot bytes under trigger permutation %d",seed)
  }
 }
}

func TestCSFRV18ContinuousVersusReopenedDeliverySnapshotByteEquivalence(t *testing.T){
 ctx:=context.Background()
 var baseline,baselineLedgers []byte
 for mode:=0;mode<4;mode++{
  t.Run(fmt.Sprintf("reopen-after-step-%d",mode),func(t *testing.T){
   c,store,ledger,cycle,fence,path:=communicationFixture(t)
   _,_,_,_,_,run:=ledger.Snapshot()
   if _,err:=c.CommitCommunicationSend(ctx,ledger,cycle,fence);err!=nil{t.Fatal(err)}
   initial,err:=store.CreateSnapshotV1(ctx,[32]byte(run),c.bundle,nil)
   if err!=nil{t.Fatal(err)}
   reopen:=func(){
    fresh,err:=sqlite.Open(ctx,path);if err!=nil{t.Fatal(err)}
    t.Cleanup(func(){fresh.Close()})
    restored,err:=inputledger.New(ctx,run,c.bundle,fresh);if err!=nil{t.Fatal(err)}
    c=New(fresh,c.bundle,c.admissionPolicies)
    store=fresh
    ledger=restored
    if _,err:=store.VerifySnapshotV1(ctx,[32]byte(run),initial.ID,c.bundle);err!=nil{
     t.Fatal("snapshot+tail proof diverged after independent SQLite reopen:",err)
    }
   }
   if mode==1{reopen()}
   advanced,err:=c.Advance(ctx,ledger,run);if err!=nil{t.Fatal(err)}
   if mode==2{reopen()}
   state:=advanced.ResultClock
   pending,err:=store.PendingClock(ctx,[32]byte(run));if err!=nil{t.Fatal(err)}
   plan,ok,err:=clock.Next(state,pending);if err!=nil||!ok||plan.Kind!=clock.Work{t.Fatal("due work missing",err)}
   cycleID,_,err:=identity.New(c.bundle).Sum("cote.csf.id.cycle","cote.csf.schema.cycle-id-preimage",1,
    []any{run[:],[]any{int64(state.Instant),uint64(plan.OriginOrdinal)}})
   if err!=nil{t.Fatal(err)}
   _,worldHash,err:=store.LoadWorldState(ctx,uint64(state.Revision));if err!=nil{t.Fatal(err)}
   due:=admission.Cycle{ID:domain.ID(cycleID),Attempt:0,Instant:state.Instant,
    Ordinal:plan.OriginOrdinal,BaseRevision:state.Revision,BaseStateHash:domain.Digest(worldHash),
    RetryRef:[]any{uint64(0)}}
   dueFence,err:=admission.NewCoordinator(admission.NewFenceLog(store)).
    AdmitThenEvaluate(ctx,c.bundle,ledger,due,c.admissionPolicies,nil)
   if err!=nil{t.Fatal(err)}
   if mode==3{reopen()}
   if _,err:=c.CommitCommunicationDue(ctx,ledger,due,dueFence);err!=nil{t.Fatal(err)}
   finished,err:=store.CreateSnapshotV1(ctx,[32]byte(run),c.bundle,nil)
   if err!=nil{t.Fatal(err)}
   if _,err:=store.VerifySnapshotV1(ctx,[32]byte(run),finished.ID,c.bundle);err!=nil{t.Fatal(err)}
   rawLedgers,err:=csfrv18ExactFiveLedgers(path)
   if err!=nil{t.Fatal(err)}
   if mode==0{
    baseline=bytes.Clone(finished.Canonical)
    baselineLedgers=bytes.Clone(rawLedgers)
   }else{
    if !bytes.Equal(baseline,finished.Canonical){
     t.Fatal("continuous and restarted delivery produced divergent canonical snapshot/ledger prefix")
    }
    if !bytes.Equal(baselineLedgers,rawLedgers){
     t.Fatal("continuous and resumed execution differ in authoritative five-ledger canonical bytes")
    }
   }
  })
 }
}

func TestCSFRV18ForkRejectsSourceDeclaredWithoutFinalClosure(t *testing.T){
 ctx:=context.Background()
 b,err:=policy.Load(bundlePath);if err!=nil{t.Fatal(err)}
 g:=validGenesis(t,b)
 parent,err:=sqlite.Open(ctx,t.TempDir()+"/open-parent.db");if err!=nil{t.Fatal(err)};defer parent.Close()
 if err:=parent.PersistGenesis(ctx,g,b);err!=nil{t.Fatal(err)}
 if err:=parent.BootstrapClock(ctx,g,b);err!=nil{t.Fatal(err)}
 if err:=parent.BootstrapGenesisTriggers(ctx,g,b,nil);err!=nil{t.Fatal(err)}
 l,err:=inputledger.New(ctx,g.RunID,b,parent);if err!=nil{t.Fatal(err)}
 if err:=l.DeclareSource(inputledger.Source{ID:"unfinished"});err!=nil{t.Fatal(err)}
 snap,err:=parent.CreateSnapshotV1(ctx,[32]byte(g.RunID),b,nil);if err!=nil{t.Fatal(err)}
 child,err:=sqlite.Open(ctx,t.TempDir()+"/child.db");if err!=nil{t.Fatal(err)};defer child.Close()
 id:=[32]byte(g.RunID);id[0]^=0x19
 if _,err:=parent.ForkIdleSnapshotV1(ctx,[32]byte(g.RunID),snap.ID,id,child,b);err==nil{
  t.Fatal("fork discarded declared but never final-closed source")
 }
}

func TestCSFRV18CognitiveRegistryRequiresCheckpointBeforeFirstObservation(t *testing.T){
 ctx:=context.Background()
 _,store,_,run,bundle,path:=testFixture(t)
 if err:=store.RegisterCognitiveActor(ctx,[32]byte(run),"school","quiet-agent",bundle);err!=nil{t.Fatal(err)}
 if _,err:=store.CreateSnapshotV1(ctx,[32]byte(run),bundle,nil);err==nil{
  t.Fatal("snapshot silently omitted cognitive actor with no evidence")
 }
 state,err:=store.LoadClock(ctx,[32]byte(run));if err!=nil{t.Fatal(err)}
 checkpoint:=sqlite.SnapshotCheckpoint{
  ActorNamespace:"school",ActorID:"quiet-agent",StoreID:"cognition-checkpoint",
  SchemaVersion:1,Content:[]byte{1,2,3,4},Producer:"cognition-owner",ProducerVersion:1,
  CoversInstant:int64(state.Instant),CoversRevision:uint64(state.Revision),CoversEvidenceCursor:0,
 }
 snap,err:=store.CreateSnapshotV1(ctx,[32]byte(run),bundle,[]sqlite.SnapshotCheckpoint{checkpoint})
 if err!=nil{t.Fatal(err)}
 reopened,err:=sqlite.Open(ctx,path);if err!=nil{t.Fatal(err)};defer reopened.Close()
 if _,err:=reopened.VerifySnapshotV1(ctx,[32]byte(run),snap.ID,bundle);err!=nil{
  t.Fatalf("checkpoint cannot recover actor without prior evidence: %v",err)
 }
 if err:=store.RegisterCognitiveActor(ctx,[32]byte(run),"school","new-agent",bundle);err==nil{
  t.Fatal("late same-revision enrollment rewrote an already published roster")
 }
}

func TestCSFRV18CorruptActorRosterFailsSnapshotResume(t *testing.T){
 ctx:=context.Background()
 _,store,_,run,bundle,path:=testFixture(t)
 if err:=store.RegisterCognitiveActor(ctx,[32]byte(run),"school","a",bundle);err!=nil{t.Fatal(err)}
 state,err:=store.LoadClock(ctx,[32]byte(run));if err!=nil{t.Fatal(err)}
 checkpoint:=sqlite.SnapshotCheckpoint{
  ActorNamespace:"school",ActorID:"a",StoreID:"opaque-store",SchemaVersion:1,
  Content:[]byte("verified cognition content"),Producer:"checkpoint-writer",ProducerVersion:1,
  CoversInstant:int64(state.Instant),CoversRevision:0,CoversEvidenceCursor:0,
 }
 snap,err:=store.CreateSnapshotV1(ctx,[32]byte(run),bundle,[]sqlite.SnapshotCheckpoint{checkpoint})
 if err!=nil{t.Fatal(err)}
 raw,err:=sql.Open("sqlite",path);if err!=nil{t.Fatal(err)};defer raw.Close()
 if _,err:=raw.ExecContext(ctx,"DELETE FROM snapshot_cognitive_actors WHERE snapshot_id=?",snap.ID[:]);err!=nil{t.Fatal(err)}
 if _,err:=store.VerifySnapshotV1(ctx,[32]byte(run),snap.ID,bundle);err==nil{
  t.Fatal("missing frozen actor roster survived snapshot verification")
 }
}

func TestCSFRV18SameRevisionFenceAbortRetryJournalTails(t *testing.T){
 ctx:=context.Background()
 coordinator,store,_,fence,run,cycle,path:=abortFixture(t)
 frozen,err:=store.CreateSnapshotV1(ctx,[32]byte(run),coordinator.bundle,nil)
 if err!=nil{t.Fatal(err)}
 first,err:=store.VerifySnapshotV1(ctx,[32]byte(run),frozen.ID,coordinator.bundle)
 if err!=nil{t.Fatal(err)}
 if first.ControlStatus!=1||first.Action.FenceID!=[32]byte(fence.ID)||
 (first.Action.Kind!=sqlite.ResumeReexecuteFencedAttempt&&first.Action.Kind!=sqlite.ResumeDrainPerception){
  t.Fatal("initial snapshot lost its frozen in-flight fence or perception barrier")
 }
 abort,err:=coordinator.Abort(ctx,run,cycle,fence,4,nil);if err!=nil{t.Fatal(err)}
 fromAbort,err:=store.VerifySnapshotV1(ctx,[32]byte(run),frozen.ID,coordinator.bundle)
 if err!=nil{t.Fatalf("valid same-revision abort tail rejected: %v",err)}
 if fromAbort.Clock.Revision!=first.Clock.Revision||fromAbort.TailCommits!=0||
  fromAbort.Action.Kind!=sqlite.ResumeHalted{
  t.Fatalf("same-revision abort tail did not preserve HALTED state: %+v",fromAbort)
 }
 if _,err:=coordinator.Retry(ctx,run,cycle,abort,"operator.retry");err!=nil{t.Fatal(err)}
 reopened,err:=sqlite.Open(ctx,path);if err!=nil{t.Fatal(err)};defer reopened.Close()
 fromRetry,err:=reopened.VerifySnapshotV1(ctx,[32]byte(run),frozen.ID,coordinator.bundle)
 if err!=nil{t.Fatalf("same-revision retry tail failed restart: %v",err)}
 if fromRetry.TailCommits!=0||fromRetry.ControlStatus!=2||
 (fromRetry.Action.Kind!=sqlite.ResumeFenceAuthorizedRetry&&fromRetry.Action.Kind!=sqlite.ResumeDrainPerception){
  t.Fatalf("same-revision retry changed causal state or reopened work: %+v",fromRetry)
 }
 if _,err:=reopened.CreateSnapshotV1(ctx,[32]byte(run),coordinator.bundle,nil);err==nil{
  t.Fatal("V1 snapshot identity collision in same revision was silently overwritten")
 }
}

func TestCSFRV18ResumeDrainsCausallyCommittedPerceptionExactlyOnce(t *testing.T){
 ctx:=context.Background()
 coordinator,store,ledger,cycle,fence,path:=communicationFixture(t)
 _,_,_,_,_,run:=ledger.Snapshot()
 if _,err:=coordinator.CommitCommunicationSend(ctx,ledger,cycle,fence);err!=nil{t.Fatal(err)}
 frozen,err:=store.CreateSnapshotV1(ctx,[32]byte(run),coordinator.bundle,nil)
 if err!=nil{t.Fatal(err)}
 before,err:=store.VerifySnapshotV1(ctx,[32]byte(run),frozen.ID,coordinator.bundle)
 if err!=nil{t.Fatal(err)}
 if before.PendingTasks!=2{t.Fatalf("expected two uncompleted perception tasks, got %d",before.PendingTasks)}
 if before.Action.Kind!=sqlite.ResumeDrainPerception{t.Fatal("resume skipped barrier")}
 reconnected,err:=sqlite.Open(ctx,path);if err!=nil{t.Fatal(err)};defer reconnected.Close()
 recovered,err:=reconnected.DrainPendingPerceptionV1(ctx,[32]byte(run),frozen.ID,coordinator.bundle)
 if err!=nil{t.Fatal(err)}
 if recovered.PendingTasks!=0||recovered.Action.Kind!=sqlite.ResumeIdle{
  t.Fatalf("perception drain failed to release authenticated barrier: %+v",recovered)
 }
 reader,err:=sql.Open("sqlite",path);if err!=nil{t.Fatal(err)};defer reader.Close()
 var count int
 if err:=reader.QueryRowContext(ctx,"SELECT count(*) FROM causal_outbox").Scan(&count);err!=nil{t.Fatal(err)}
 if count!=4{t.Fatalf("expected two immutable tasks and two completions, got %d",count)}
 again,err:=reconnected.DrainPendingPerceptionV1(ctx,[32]byte(run),frozen.ID,coordinator.bundle)
 if err!=nil{t.Fatal("idempotent retry of perception drain:",err)}
 if again.PendingTasks!=0{t.Fatal("resumed completion was repeated as new work")}
 if err:=reader.QueryRowContext(ctx,"SELECT count(*) FROM causal_outbox").Scan(&count);err!=nil{t.Fatal(err)}
 if count!=4{t.Fatal("replay duplicated perception receipts")}
 if _,err:=reconnected.VerifySnapshotV1(ctx,[32]byte(run),frozen.ID,coordinator.bundle);err!=nil{
  t.Fatal("historical snapshot lost provenance after partial perception tail:",err)
 }
}

func csfrv18ExactFiveLedgers(path string)([]byte,error){
 db,err:=sql.Open("sqlite","file:"+path);if err!=nil{return nil,err}
 defer db.Close()
 var out bytes.Buffer
 for _,name:=range []string{"input_ledger","decision_ledger","event_store","evidence_ledger","causal_outbox"}{
  rows,err:=db.Query("SELECT id,canonical FROM "+name+" ORDER BY id")
  if err!=nil{return nil,err}
  out.WriteString(name)
  for rows.Next(){
   var id,canonical []byte
   if err:=rows.Scan(&id,&canonical);err!=nil{rows.Close();return nil,err}
   fmt.Fprintf(&out,"%08x%x%08x%x",len(id),id,len(canonical),canonical)
  }
  if err:=rows.Err();err!=nil{rows.Close();return nil,err}
  if err:=rows.Close();err!=nil{return nil,err}
 }
 return out.Bytes(),nil
}

func TestCSFRV18RehydrateFrozenFenceInsteadOfReselectingCoordinate(t *testing.T){
 ctx:=context.Background()
 original,store,_,fence,run,cycleID,path:=abortFixture(t)
 snap,err:=store.CreateSnapshotV1(ctx,[32]byte(run),original.bundle,nil)
 if err!=nil{t.Fatal(err)}
 reopened,err:=sqlite.Open(ctx,path);if err!=nil{t.Fatal(err)};defer reopened.Close()
 resumed:=New(reopened,original.bundle,original.admissionPolicies)
 recoveredCycle,recoveredFence,result,err:=resumed.ReopenFencedCycle(ctx,[32]byte(run),snap.ID)
 if err!=nil{t.Fatal(err)}
 if recoveredCycle.ID!=cycleID||recoveredCycle.Attempt!=0||
  recoveredFence.ID!=fence.ID||!bytes.Equal(recoveredFence.Canonical,fence.Canonical)||
  recoveredFence.InputDigest!=fence.InputDigest{
  t.Fatal("restored fence or cycle differs from exact admitted immutable plan")
 }
 if result.Action.Kind!=sqlite.ResumeReexecuteFencedAttempt||result.PendingTasks!=0{
  t.Fatal("frozen fence resumed before causal perception drain")
 }
 if recoveredCycle.Plan!=nil{t.Fatal("WORK replay must retain nil cycle plan")}
 if _,err:=reopened.VerifySnapshotV1(ctx,[32]byte(run),snap.ID,original.bundle);err!=nil{
  t.Fatal("checkpoint lost same-revision completed-perception tail:",err)
 }
}

func TestCSFRV18PartialPerceptionBeforeCrashResumesWithoutDuplicateReceipt(t *testing.T){
 ctx:=context.Background()
 coordinator,store,ledger,cycle,fence,path:=communicationFixture(t)
 _,_,_,_,_,run:=ledger.Snapshot()
 if _,err:=coordinator.CommitCommunicationSend(ctx,ledger,cycle,fence);err!=nil{t.Fatal(err)}
 snapshot,err:=store.CreateSnapshotV1(ctx,[32]byte(run),coordinator.bundle,nil)
 if err!=nil{t.Fatal(err)}
 q,err:=sql.Open("sqlite","file:"+path);if err!=nil{t.Fatal(err)};defer q.Close()
 rows,err:=q.QueryContext(ctx,"SELECT id,canonical FROM causal_outbox ORDER BY id")
 if err!=nil{t.Fatal(err)}
 var internal [32]byte
 for rows.Next(){
  var id,raw []byte
  if err:=rows.Scan(&id,&raw);err!=nil{rows.Close();t.Fatal(err)}
  decoded,err:=codec.StrictDecodePayload(raw);if err!=nil{rows.Close();t.Fatal(err)}
  fields:=decoded.([]any)
  if len(fields)!=10{continue}
  var eventRaw []byte
  if err:=q.QueryRowContext(ctx,"SELECT canonical FROM event_store WHERE id=?",fields[3]).Scan(&eventRaw);err!=nil{rows.Close();t.Fatal(err)}
  ev,err:=codec.StrictDecodePayload(eventRaw);if err!=nil{rows.Close();t.Fatal(err)}
  if ev.([]any)[3]=="communication.sent"{copy(internal[:],id)}
 }
 if err:=rows.Err();err!=nil{rows.Close();t.Fatal(err)}
 if err:=rows.Close();err!=nil{t.Fatal(err)}
 if internal==([32]byte{}){t.Fatal("internal transmission send task not found")}
 if _,err:=store.CompleteInternalPerception(ctx,internal,coordinator.bundle);err!=nil{t.Fatal(err)}
 var receipts int
 if err:=q.QueryRowContext(ctx,"SELECT count(*) FROM causal_outbox").Scan(&receipts);err!=nil{t.Fatal(err)}
 if receipts!=3{t.Fatalf("one partial completion expected, got %d outbox rows",receipts)}
 reopened,err:=sqlite.Open(ctx,path);if err!=nil{t.Fatal(err)};defer reopened.Close()
 healed,err:=reopened.DrainPendingPerceptionV1(ctx,[32]byte(run),snapshot.ID,coordinator.bundle)
 if err!=nil{t.Fatal(err)}
 if healed.PendingTasks!=0{t.Fatal("replayed partial completion did not drain remaining work")}
 if err:=q.QueryRowContext(ctx,"SELECT count(*) FROM causal_outbox").Scan(&receipts);err!=nil{t.Fatal(err)}
 if receipts!=4{t.Fatalf("duplicate/absent completion after restart: %d",receipts)}
 if _,err:=reopened.DrainPendingPerceptionV1(ctx,[32]byte(run),snapshot.ID,coordinator.bundle);err!=nil{t.Fatal(err)}
 if err:=q.QueryRowContext(ctx,"SELECT count(*) FROM causal_outbox").Scan(&receipts);err!=nil{t.Fatal(err)}
 if receipts!=4{t.Fatal("repeat recovery changed settled outbox")}
}

func TestCSFRV18ReopenRetryFromVerifiedFrozenSemanticCut(t *testing.T){
 ctx:=context.Background()
 c,store,_,fence,run,cycleID,path:=abortFixture(t)
 abort,err:=c.Abort(ctx,run,cycleID,fence,4,nil);if err!=nil{t.Fatal(err)}
 if _,err:=c.Retry(ctx,run,cycleID,abort,"operator.retry");err!=nil{t.Fatal(err)}
 snapshot,err:=store.CreateSnapshotV1(ctx,[32]byte(run),c.bundle,nil)
 if err!=nil{t.Fatal(err)}
 reopened,err:=sqlite.Open(ctx,path);if err!=nil{t.Fatal(err)};defer reopened.Close()
 ledger,err:=inputledger.Open(ctx,run,c.bundle,reopened);if err!=nil{t.Fatal(err)}
 resumer:=New(reopened,c.bundle,c.admissionPolicies)
 next,newFence,resume,err:=resumer.ReopenAuthorizedRetry(ctx,ledger,[32]byte(run),snapshot.ID)
 if err!=nil{t.Fatal(err)}
 if resume.ControlStatus!=2||resume.PendingTasks!=0||next.ID!=cycleID||next.Attempt!=1||
  newFence.ID==fence.ID||newFence.InputDigest!=fence.InputDigest{
  t.Fatal("authorized retry resumed different cycle or changed previous semantic cut")
 }
 if _,_,_,err:=resumer.ReopenAuthorizedRetry(ctx,ledger,[32]byte(run),snapshot.ID);err==nil{
  t.Fatal("second recovery admitted duplicate attempt after state became in-flight")
 }
 ctl,err:=reopened.LoadCycleControl(ctx,[32]byte(run));if err!=nil{t.Fatal(err)}
 if ctl[0]!=uint64(1){t.Fatal("retry was not fenced into a durable in-flight attempt")}
 if _,err:=reopened.VerifySnapshotV1(ctx,[32]byte(run),snapshot.ID,c.bundle);err!=nil{
  t.Fatal("snapshot cannot verify same-revision successor fence:",err)
 }
}

func TestCSFRV18HaltedNeverAutoResumesOnReconnect(t *testing.T){
 ctx:=context.Background()
 c,store,_,fence,run,cycle,path:=abortFixture(t)
 if _,err:=c.Abort(ctx,run,cycle,fence,4,nil);err!=nil{t.Fatal(err)}
 snapshot,err:=store.CreateSnapshotV1(ctx,[32]byte(run),c.bundle,nil)
 if err!=nil{t.Fatal(err)}
 reopened,err:=sqlite.Open(ctx,path);if err!=nil{t.Fatal(err)};defer reopened.Close()
 l,err:=inputledger.Open(ctx,run,c.bundle,reopened);if err!=nil{t.Fatal(err)}
 runner:=New(reopened,c.bundle,c.admissionPolicies)
 if _,_,_,err:=runner.ReopenFencedCycle(ctx,[32]byte(run),snapshot.ID);err==nil{
  t.Fatal("halted run unlawfully reopened its aborted fence")
 }
 if _,_,_,err:=runner.ReopenAuthorizedRetry(ctx,l,[32]byte(run),snapshot.ID);err==nil{
  t.Fatal("halted run without signed AttemptRetryRecord fabricated retry authority")
 }
 status,err:=reopened.LoadCycleControl(ctx,[32]byte(run));if err!=nil{t.Fatal(err)}
 if status[0]!=uint64(3){t.Fatal("resume mutated HALTED_ON_ABORT into an executable status")}
}

func TestCSFRV18ResumeRunnerDispatchesFrozenTerminalWithoutResampling(t *testing.T){
 ctx:=context.Background()
 previous,store,original,cycle,fence,path:=twoResourceContestFixtureWithSystem(t,false,true,true)
 _,_,_,_,_,run:=original.Snapshot()
 snapshot,err:=store.CreateSnapshotV1(ctx,[32]byte(run),previous.bundle,nil)
 if err!=nil{t.Fatal(err)}
 restarted,err:=sqlite.Open(ctx,path);if err!=nil{t.Fatal(err)};defer restarted.Close()
 ledger,err:=inputledger.Open(ctx,run,previous.bundle,restarted);if err!=nil{t.Fatal(err)}
 resumed:=New(restarted,previous.bundle,previous.admissionPolicies)
 ran:=0
 handler:=func(ctx context.Context,actual *inputledger.Ledger,c admission.Cycle,f admission.Fence)(ports.CanonicalRecord,error){
  ran++
  if c.ID!=cycle.ID||f.ID!=fence.ID||!bytes.Equal(f.Canonical,fence.Canonical){
   t.Fatal("resume runner substituted a different cycle/fence")
  }
  return resumed.CommitTwoResourceContest(ctx,actual,c,f)
 }
 if _,err:=resumed.ResumeRunV1(ctx,ledger,[32]byte(run),snapshot.ID,nil);err==nil{
  t.Fatal("runner executed perception or fence without terminal handler")
 }
 result,err:=resumed.ResumeRunV1(ctx,ledger,[32]byte(run),snapshot.ID,handler)
 if err!=nil{t.Fatal(err)}
 if !result.Continued||ran!=1||result.Publication.Identity==([32]byte{}){
  t.Fatal("recovered in-flight cycle was not committed exactly once")
 }
 if _,err:=resumed.ResumeRunV1(ctx,ledger,[32]byte(run),snapshot.ID,handler);err!=nil{
  t.Fatalf("idle recovery after settled cycle must be safe: %v",err)
 }
 if ran!=1{t.Fatal("terminal handler was repeated after successful replay")}
 if _,err:=restarted.ReplayClock(ctx,[32]byte(run),previous.bundle);err!=nil{
  t.Fatal("resumed terminal did not persist an independently replayable journal:",err)
 }
}

func TestCSFRV18ResourceContestTriggerScheduleContinuityCrashMatrix(t *testing.T){
 ctx:=context.Background()
 var expectedSnapshot,expectedLedger []byte
 for _,mode:=range []struct{name string;reverse bool;reopen bool;crash uint8}{
  {"continuous",false,false,0},
  {"reopened",false,true,0},
  {"reverse-workers",true,true,0},
  {"crash-before-journal",false,true,1},
  {"crash-mid-terminal",true,true,4},
  {"crash-near-terminal",false,true,7},
 }{
  t.Run(mode.name,func(t *testing.T){
   c,store,ledger,cycle,fence,path:=twoResourceContestFixtureWithSystem(t,mode.reverse,true,true)
   _,_,_,_,_,run:=ledger.Snapshot()
   saved,err:=store.CreateSnapshotV1(ctx,[32]byte(run),c.bundle,nil)
   if err!=nil{t.Fatal("joint scenario pre-commit checkpoint:",err)}
   if mode.crash!=0{
    c.failAfterStep=mode.crash
    if _,err:=c.CommitTwoResourceContest(ctx,ledger,cycle,fence);err==nil{
     t.Fatal("failure injection did not roll back atomic contest/trigger/occurrence")
    }
    c.failAfterStep=0
   }
   if mode.reopen{
    fresh,err:=sqlite.Open(ctx,path);if err!=nil{t.Fatal(err)}
    defer fresh.Close()
    opened,err:=inputledger.Open(ctx,run,c.bundle,fresh);if err!=nil{t.Fatal(err)}
    coordinator:=New(fresh,c.bundle,c.admissionPolicies)
    handler:=func(ctx context.Context,ledger *inputledger.Ledger,
     cy admission.Cycle,fr admission.Fence)(ports.CanonicalRecord,error){
     return coordinator.CommitTwoResourceContest(ctx,ledger,cy,fr)
    }
    recovered,err:=coordinator.ResumeRunV1(ctx,opened,[32]byte(run),saved.ID,handler)
    if err!=nil{t.Fatal("snapshot + terminal recovery:",err)}
    if !recovered.Continued||recovered.Cycle.ID!=cycle.ID||
     !bytes.Equal(recovered.Fence.Canonical,fence.Canonical){
     t.Fatal("recovery displaced trigger/resource admission cut")
    }
    store=fresh
   }else{
    if _,err:=c.CommitTwoResourceContest(ctx,ledger,cycle,fence);err!=nil{t.Fatal(err)}
   }
   final,err:=store.CreateSnapshotV1(ctx,[32]byte(run),c.bundle,nil)
   if err!=nil{t.Fatal(err)}
   if _,err:=store.VerifySnapshotV1(ctx,[32]byte(run),saved.ID,c.bundle);err!=nil{
    t.Fatal("historical checkpoint lost full terminal tail:",err)
   }
   if _,err:=store.ReplayClock(ctx,[32]byte(run),c.bundle);err!=nil{
    t.Fatal("independent replay failed:",err)
   }
   ledgerBytes,err:=csfrv18ExactFiveLedgers(path);if err!=nil{t.Fatal(err)}
   if mode.name=="continuous"{
    expectedSnapshot=bytes.Clone(final.Canonical)
    expectedLedger=bytes.Clone(ledgerBytes)
   }else if !bytes.Equal(expectedSnapshot,final.Canonical)||
    !bytes.Equal(expectedLedger,ledgerBytes){
    t.Fatal("snapshot/ledger bytes diverged after worker permutation, crash or restore")
   }
  })
 }
}

func TestCSFRV18OriginPolicyResolvedWithoutConsultingChildCodec(t *testing.T){
 ctx:=context.Background()
 _,store,_,run,bundle,_:=testFixture(t)
 snapshot,err:=store.CreateSnapshotV1(ctx,[32]byte(run),bundle,nil);if err!=nil{t.Fatal(err)}
 origin,err:=fork.NewSnapshotReference(bundle,snapshot.Canonical);if err!=nil{t.Fatal(err)}
 encoded,err:=fork.Encode(origin);if err!=nil{t.Fatal(err)}
 requests:=0
 resolver:=fork.ResolveOriginFunc(func(_ context.Context,id string,hash [32]byte)(*policy.Bundle,error){
  requests++
  if id!=origin.OriginPolicyID||hash!=origin.OriginPolicyHash{
   t.Fatal("resolver accepted child-dependent policy lookup")
  }
  return bundle,nil
 })
 resolved,used,err:=fork.VerifyOriginWithResolver(ctx,encoded,snapshot.Canonical,resolver)
 if err!=nil{t.Fatal(err)}
 if resolved!=origin||used!=bundle||requests!=1{t.Fatal("origin policy lookup changed pinned checkpoint reference")}
 broken:=append([]byte(nil),snapshot.Canonical...)
 broken[len(broken)-1]^=1
 if _,_,err:=fork.VerifyOriginWithResolver(ctx,encoded,broken,resolver);err==nil{
  t.Fatal("origin policy resolver accepted changed checkpoint bytes")
 }
 if _,_,err:=fork.VerifyOriginWithResolver(ctx,encoded,snapshot.Canonical,nil);err==nil{
  t.Fatal("unregistered historic origin policy incorrectly trusted child codec")
 }
 wrong:=fork.ResolveOriginFunc(func(context.Context,string,[32]byte)(*policy.Bundle,error){
  fake:=*bundle;fake.CodecPolicyHash[0]^=1;return &fake,nil
 })
 if _,_,err:=fork.VerifyOriginWithResolver(ctx,encoded,snapshot.Canonical,wrong);err==nil{
  t.Fatal("different policy hash accepted for historic canonical snapshot")
 }
}

func TestCSFRV18RunnerRejectsFakeCompletionWithoutJournalCommit(t *testing.T){
 ctx:=context.Background()
 source,store,original,cycle,_,path:=twoResourceContestFixtureWithSystem(t,false,true,true)
 _,_,_,_,_,run:=original.Snapshot()
 snapshot,err:=store.CreateSnapshotV1(ctx,[32]byte(run),source.bundle,nil)
 if err!=nil{t.Fatal(err)}
 reopened,err:=sqlite.Open(ctx,path);if err!=nil{t.Fatal(err)};defer reopened.Close()
 ledger,err:=inputledger.Open(ctx,run,source.bundle,reopened);if err!=nil{t.Fatal(err)}
 c:=New(reopened,source.bundle,source.admissionPolicies)
 fake:=func(_ context.Context,_ *inputledger.Ledger,_ admission.Cycle,_ admission.Fence)(ports.CanonicalRecord,error){
  return ports.CanonicalRecord{Identity:[32]byte(cycle.ID),Canonical:[]byte{0x80}},nil
 }
 if _,err:=c.ResumeRunV1(ctx,ledger,[32]byte(run),snapshot.ID,fake);err==nil{
  t.Fatal("runner accepted uncommitted fabricated terminal")
 }
 state,err:=reopened.LoadClock(ctx,[32]byte(run));if err!=nil{t.Fatal(err)}
 if state.Revision!=0{t.Fatal("invalid completion altered world revision")}
 committed,err:=c.ResumeRunV1(ctx,ledger,[32]byte(run),snapshot.ID,
  func(ctx context.Context,l *inputledger.Ledger,cy admission.Cycle,f admission.Fence)(ports.CanonicalRecord,error){
   return c.CommitTwoResourceContest(ctx,l,cy,f)
  })
 if err!=nil{t.Fatal("false callback poisoned legitimate recovery:",err)}
 if !committed.Continued||committed.Publication.Identity==([32]byte{}){
  t.Fatal("valid terminal did not recover after forged callback")
 }
}

func TestCSFRV18IntegratedResourceTriggerCommunicationInTransit(t *testing.T){
 ctx:=context.Background()
 var baseline,allLedgers []byte
 for _,variant:=range []struct{name string;reverse,restore,failed bool}{
  {"continuous",false,false,false},
  {"reverse-workers",true,false,false},
  {"resume-before-communication",false,true,false},
  {"crash-in-communication",true,true,true},
 }{
  t.Run(variant.name,func(t *testing.T){
   c,store,ledger,contest,fence,path:=twoResourceContestFixtureWithSystem(t,variant.reverse,true,true)
   _,_,_,_,_,run:=ledger.Snapshot()
   pre,err:=store.CreateSnapshotV1(ctx,[32]byte(run),c.bundle,nil);if err!=nil{t.Fatal(err)}
   if _,err:=c.CommitTwoResourceContest(ctx,ledger,contest,fence);err!=nil{t.Fatal(err)}
   post,err:=store.CreateSnapshotV1(ctx,[32]byte(run),c.bundle,nil);if err!=nil{t.Fatal(err)}
   if _,err:=store.DrainPendingPerceptionV1(ctx,[32]byte(run),post.ID,c.bundle);err!=nil{t.Fatal(err)}
   if variant.restore{
    independent,err:=sqlite.Open(ctx,path);if err!=nil{t.Fatal(err)}
    defer independent.Close()
    ledger,err=inputledger.Open(ctx,run,c.bundle,independent);if err!=nil{t.Fatal(err)}
    c=New(independent,c.bundle,c.admissionPolicies)
    store=independent
    if _,err:=store.VerifySnapshotV1(ctx,[32]byte(run),pre.ID,c.bundle);err!=nil{t.Fatal(err)}
   }
   g:=stockGenesis(t,c.bundle)
   state,err:=store.LoadClock(ctx,[32]byte(run));if err!=nil{t.Fatal(err)}
   if state.Revision!=1{t.Fatal("mixed resource+trigger commit not published first")}
   ordinal:=domain.CycleOrdinal(state.LastOrdinal+1)
   at:=domain.EligibilityCoordinate{NotBeforeInstant:state.Instant,NotBeforeCycleOrdinal:ordinal}
   if _,err:=ledger.CloseSource(ctx,"driver",inputledger.Closure{
    ProducerKey:"communication.closed",ClosedThrough:at});err!=nil{t.Fatal(err)}
   d:=identity.New(c.bundle)
   cycleID,_,err:=d.Sum("cote.csf.id.cycle","cote.csf.schema.cycle-id-preimage",1,
    []any{run[:],[]any{int64(state.Instant),uint64(ordinal)}});if err!=nil{t.Fatal(err)}
   origin:=[]any{uint64(2),[]any{g.ID[:],g.Digest[:]}}
   round,err:=ledger.DeclareRound(ctx,"schedule",[2]string{"school","c"},
    []string{"act"},at,origin,1,0,0,[]any{uint64(0)})
   if err!=nil{t.Fatal(err)}
   dispatchID,_,err:=d.Sum("cote.csf.id.dispatch","cote.csf.schema.dispatch-id-preimage",1,
    []any{run[:],cycleID[:],round.ID[:],"act",uint64(0)});if err!=nil{t.Fatal(err)}
   dispatch:=inputledger.Dispatch{ID:domain.ID(dispatchID),RunID:run,
    CycleID:domain.ID(cycleID),RoundID:round.ID,SlotID:"act",Actor:round.Actor,
    Eligibility:at,BaseRevision:state.Revision,EffectiveAt:state.Instant}
   if err:=ledger.RegisterDispatch(ctx,dispatch,inputledger.CycleBinding{
    ID:domain.ID(cycleID),BaseRevision:state.Revision,Instant:state.Instant});err!=nil{t.Fatal(err)}
   proposal,err:=testresourcev1.ProposalForDispatch(c.bundle,dispatch);if err!=nil{t.Fatal(err)}
   scope,err:=communication.MakeTyped("cote.csf.communication.scope.v1","cote.csf.schema.typed-value",[]any{uint64(0)})
   if err!=nil{t.Fatal(err)}
   content,err:=communication.MakeTyped("cote.csf.communication.allegation.v1","cote.csf.schema.typed-value",
    []any{"resource.stolen",[]any{"school","a"},"last.resource"})
   if err!=nil{t.Fatal(err)}
   params,err:=communication.SendParameters(communication.SendSpec{
    Recipient:communication.Actor{"school","b"},PresentedSender:"C",
    Channel:"school.message.v1",DueAt:state.Instant+25,
    Proposition:communication.Proposition{Type:"resource.stolen",Polarity:0,
     TemporalScope:scope,Content:content}})
   if err!=nil{t.Fatal(err)}
   proposal.Body[4]="communication.send"
   proposal.Body[5]=uint64(1)
   proposal.Body[7]=[]any{}
   proposal.Body[8]=params
   proposal.Body[9]=[]any{}
   proposal.Body[10]=[]any{uint64(0)}
   digest,_,err:=d.Sum("cote.csf.digest.action-proposal-unit","cote.csf.schema.action-proposal.body",1,
    []any{proposal.Body});if err!=nil{t.Fatal(err)}
   proposal.Digest=digest
   proposal.Canonical,err=codec.CanonicalPayload(append(cloneAny(proposal.Body),digest[:]));if err!=nil{t.Fatal(err)}
   if err:=ledger.Respond(ctx,proposal);err!=nil{t.Fatal(err)}
   _,hash,err:=store.LoadWorldState(ctx,1);if err!=nil{t.Fatal(err)}
   messageCycle:=admission.Cycle{ID:domain.ID(cycleID),Attempt:0,Instant:state.Instant,
    Ordinal:ordinal,BaseRevision:state.Revision,BaseStateHash:domain.Digest(hash),RetryRef:[]any{uint64(0)}}
   messageFence,err:=admission.NewCoordinator(admission.NewFenceLog(store)).
    AdmitThenEvaluate(ctx,c.bundle,ledger,messageCycle,c.admissionPolicies,nil)
   if err!=nil{t.Fatal(err)}
   if variant.failed{
    c.failAfterStep=4
    if _,err:=c.CommitCommunicationSend(ctx,ledger,messageCycle,messageFence);err==nil{
     t.Fatal("communication failpoint failed to roll back mixed-timeline transmission")
    }
    c.failAfterStep=0
   }
   if _,err:=c.CommitCommunicationSend(ctx,ledger,messageCycle,messageFence);err!=nil{t.Fatal(err)}
   checkpoints:=csfrv18SyntheticActorCheckpoints(t,path,state.Instant,int64(state.Revision)+1)
   final,err:=store.CreateSnapshotV1(ctx,[32]byte(run),c.bundle,checkpoints);if err!=nil{t.Fatal(err)}
   var status int
   q,err:=sql.Open("sqlite",path);if err!=nil{t.Fatal(err)}
   if err:=q.QueryRowContext(ctx,"SELECT status FROM communication_transmissions").Scan(&status);err!=nil{q.Close();t.Fatal(err)}
   q.Close()
   if status!=0{t.Fatal("claim delivered too early instead of remaining IN_TRANSIT")}
   if _,err:=store.ReplayClock(ctx,[32]byte(run),c.bundle);err!=nil{t.Fatal(err)}
   all,err:=csfrv18ExactFiveLedgers(path);if err!=nil{t.Fatal(err)}
   if variant.name=="continuous"{baseline=bytes.Clone(final.Canonical);allLedgers=bytes.Clone(all)}else if
    !bytes.Equal(baseline,final.Canonical)||!bytes.Equal(allLedgers,all){
    t.Fatal("mixed resource+trigger+in-transit communication diverged across restored routes")
   }
  })
 }
}

func csfrv18SyntheticActorCheckpoints(t *testing.T,path string,at domain.SimulationInstant,revision int64)[]sqlite.SnapshotCheckpoint{
 t.Helper()
 db,err:=sql.Open("sqlite",path);if err!=nil{t.Fatal(err)}
 defer db.Close()
 var cursor uint64
 if err:=db.QueryRow("SELECT count(*) FROM evidence_ledger").Scan(&cursor);err!=nil{t.Fatal(err)}
 rows,err:=db.Query("SELECT DISTINCT holder_namespace,holder_id FROM evidence_routes ORDER BY holder_namespace,holder_id")
 if err!=nil{t.Fatal(err)}
 defer rows.Close()
 refs:=[]sqlite.SnapshotCheckpoint{}
 for rows.Next(){
  var ns,id string
  if err:=rows.Scan(&ns,&id);err!=nil{t.Fatal(err)}
  refs=append(refs,sqlite.SnapshotCheckpoint{
   ActorNamespace:ns,ActorID:id,StoreID:"test-opaque-cognition",
   SchemaVersion:1,Content:[]byte("versioned synthetic checkpoint"),
   Producer:"test-checkpoint-owner",ProducerVersion:1,
   CoversInstant:int64(at),CoversRevision:uint64(revision),CoversEvidenceCursor:cursor,
  })
 }
 if err:=rows.Err();err!=nil{t.Fatal(err)}
 return refs
}

func TestCSFRV18ForkMigratesPendingGenesisScheduleIntoChildIDs(t *testing.T){
 ctx:=context.Background()
 b,err:=policy.Load(bundlePath);if err!=nil{t.Fatal(err)}
 g:=validGenesis(t,b)
 source,err:=sqlite.Open(ctx,t.TempDir()+"/genesis-parent.db");if err!=nil{t.Fatal(err)};defer source.Close()
 if err:=source.PersistGenesis(ctx,g,b);err!=nil{t.Fatal(err)}
 if err:=source.BootstrapClock(ctx,g,b);err!=nil{t.Fatal(err)}
 if err:=source.BootstrapGenesisTriggers(ctx,g,b,nil);err!=nil{t.Fatal(err)}
 deriver:=identity.New(b)
 origin:=[]any{uint64(2),[]any{g.ID[:],g.Digest[:]}}
 parentID,_,err:=deriver.Sum("cote.csf.id.occurrence","cote.csf.schema.occurrence-id-preimage",1,
  []any{origin,uint64(0),uint64(0)});if err!=nil{t.Fatal(err)}
 due:=int64(g.InitialInstant)+100
 body:=[]any{parentID[:],g.RunID[:],"schedule",due,[]any{due,uint64(0)},
  "test.occurrence",uint64(1),g.Payload[16],uint64(0),origin,uint64(0),uint64(0),
  []any{uint64(0)},[]any{uint64(0)}}
 digest,_,err:=deriver.Sum("cote.csf.digest.occurrence-unit","cote.csf.schema.scheduled-occurrence.body",1,body)
 if err!=nil{t.Fatal(err)}
 canonical,err:=codec.CanonicalPayload(append(cloneAny(body),digest[:]));if err!=nil{t.Fatal(err)}
 if err:=source.SeedGenesisOccurrence(ctx,domain.ID(parentID),g.RunID,
  domain.EligibilityCoordinate{NotBeforeInstant:domain.SimulationInstant(due)},canonical);err!=nil{t.Fatal(err)}
 originSnapshot,err:=source.CreateSnapshotV1(ctx,[32]byte(g.RunID),b,nil);if err!=nil{t.Fatal(err)}
 target,err:=sqlite.Open(ctx,t.TempDir()+"/genesis-fork.db");if err!=nil{t.Fatal(err)};defer target.Close()
 childID:=[32]byte(g.RunID);childID[0]^=0x96
 childGenesis,err:=source.ForkIdleSnapshotV1(ctx,[32]byte(g.RunID),originSnapshot.ID,childID,target,b)
 if err!=nil{t.Fatalf("pending genesis schedule was not migrated: %v",err)}
 if childGenesis.RunID!=domain.ID(childID){t.Fatal("fork run ID was not derived")}
 parentPending,err:=source.PendingClock(ctx,[32]byte(g.RunID));if err!=nil{t.Fatal(err)}
 childPending,err:=target.PendingClock(ctx,childID);if err!=nil{t.Fatal(err)}
 if len(parentPending)!=1||len(childPending)!=1||childPending[0].ID==parentPending[0].ID||
  childPending[0].Eligibility!=parentPending[0].Eligibility{
  t.Fatalf("scheduled fork changed semantic due coordinate or reused parent identity: %v %v",parentPending,childPending)
 }
 if _,err:=target.ValidatedGenesis(ctx,childID,b);err!=nil{t.Fatal(err)}
 if _,err:=target.ReplayClock(ctx,childID,b);err!=nil{t.Fatal(err)}
 childSnap,err:=target.CreateSnapshotV1(ctx,childID,b,nil);if err!=nil{t.Fatal(err)}
 if _,err:=target.VerifySnapshotV1(ctx,childID,childSnap.ID,b);err!=nil{t.Fatal(err)}
 if _,err:=source.VerifySnapshotV1(ctx,[32]byte(g.RunID),originSnapshot.ID,b);err!=nil{
  t.Fatal("fork altered parent schedule or prefix",err)
 }
}

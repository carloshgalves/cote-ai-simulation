package commit

import (
 "bytes"
 "context"
 "database/sql"
 "testing"
 _ "modernc.org/sqlite"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/admission"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/clock"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/inputledger"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/fork"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
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
 if _,err:=source.VerifySnapshotV1(ctx,[32]byte(g.RunID),origin.ID,bundle);err!=nil{
  t.Fatal("fork modified immutable parental source history",err)
 }
 childRaw,err:=sql.Open("sqlite",childPath);if err!=nil{t.Fatal(err)};defer childRaw.Close()
 if _,err:=childRaw.ExecContext(ctx,
  "UPDATE fork_history_records SET canonical=x'01' WHERE child_run=? AND ledger_name='input_ledger' AND ordinal=0",id[:]);err!=nil{t.Fatal(err)}
 if _,err:=child.ValidatedGenesis(ctx,id,bundle);err==nil{
  t.Fatal("tampered historical parent prefix was accepted by child fork")
 }
}

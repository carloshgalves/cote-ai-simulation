package commit

import (
 "context"
 "errors"
 "strings"
 "testing"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/inputledger"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

func TestCSFRV18V2SettlesFencedWorkBeforeChildFork(t *testing.T){
 ctx:=context.Background()
 coordinator,parent,ledger,_,_,sourcePath:=noProposalWorkFixture(t)
 _,_,_,_,_,run:=ledger.Snapshot()
 before,err:=parent.LoadClock(ctx,[32]byte(run));if err!=nil{t.Fatal(err)}
 // The source's remaining external ingress must be finalized before moving
 // ownership to the child; it cannot silently be forgotten at handoff.
 if _,err:=ledger.CloseSource(ctx,"driver",inputledger.Closure{
  ProducerKey:"fork-final-after-fence",
  ClosedThrough:domain.EligibilityCoordinate{NotBeforeInstant:before.Instant+1000},
  Final:true,
 });err!=nil{t.Fatal(err)}
 sealed,err:=parent.CreateSnapshotV1(ctx,[32]byte(run),coordinator.bundle,nil)
 if err!=nil{t.Fatal(err)}
 v2,err:=policy.LoadV2("../../../docs/architecture/canonical-codec-v2-bundle")
 if err!=nil{t.Fatal(err)}
 childID:=[32]byte(run);childID[0]^=0x6a
 path:=t.TempDir()+"/recovered-fence-v2.db"
 child,err:=sqlite.Open(ctx,path);if err!=nil{t.Fatal(err)}
 defer child.Close()
 provider:=func(ctx context.Context,s *sqlite.Store)([]sqlite.SnapshotCheckpoint,error){
  state,err:=s.LoadClock(ctx,[32]byte(run))
  if err!=nil{return nil,err}
  return csfrv18SyntheticActorCheckpoints(t,sourcePath,state.Instant,int64(state.Revision)),nil
 }
 if _,err:=coordinator.SettleFencedThenForkV2(ctx,ledger,[32]byte(run),
  sealed.ID,childID,child,coordinator.bundle,provider);err==nil{
  t.Fatal("uninstalled V2 policy passed preflight")
 }
 unchanged,err:=parent.LoadClock(ctx,[32]byte(run))
 if err!=nil||unchanged!=before{
  t.Fatal("failed child policy preflight mutated V1 fenced parent:",err)
 }
 root,err:=coordinator.SettleFencedThenForkV2(ctx,ledger,[32]byte(run),
  sealed.ID,childID,child,v2,provider)
 if err!=nil{t.Fatal("recovered fence handoff did not complete:",err)}
 if root.Payload[5]!=policy.CodecPolicyV2ID{t.Fatal("child policy downgraded")}
 parentAfter,err:=parent.LoadClock(ctx,[32]byte(run))
 if err!=nil||parentAfter.Revision!=before.Revision+1{
  t.Fatalf("fenced parent did not commit exactly once: %+v %v",parentAfter,err)
 }
 if _,err:=child.ReplayClock(ctx,childID,v2);err!=nil{
  t.Fatal("recovered-fence V2 child is not replayable:",err)
 }
 if _,err:=parent.VerifySnapshotV1(ctx,[32]byte(run),sealed.ID,coordinator.bundle);err!=nil{
  t.Fatal("historical frozen checkpoint lost:",err)
 }
 preserved,err:=provider(ctx,parent)
 if err!=nil{t.Fatal(err)}
 childSnap,err:=child.CreateSnapshotV1(ctx,childID,v2,preserved)
 if err!=nil{t.Fatal(err)}
 if _,err:=child.VerifySnapshotV1(ctx,childID,childSnap.ID,v2);err!=nil{t.Fatal(err)}
}

func TestCSFRV18R22FencedHandoffRetryAfterCheckpointProviderFailure(t *testing.T) {
 ctx:=context.Background()
 coordinator,parent,ledger,_,_,sourcePath:=noProposalWorkFixture(t)
 _,_,_,_,_,run:=ledger.Snapshot()
 before,err:=parent.LoadClock(ctx,[32]byte(run))
 if err!=nil{t.Fatal(err)}
 if _,err:=ledger.CloseSource(ctx,"driver",inputledger.Closure{
  ProducerKey:"frozen-retry-source-final",
  ClosedThrough:domain.EligibilityCoordinate{NotBeforeInstant:before.Instant+1000},
  Final:true,
 });err!=nil{t.Fatal(err)}
 frozen,err:=parent.CreateSnapshotV1(ctx,[32]byte(run),coordinator.bundle,nil)
 if err!=nil{t.Fatal(err)}
 v2,err:=policy.LoadV2("../../../docs/architecture/canonical-codec-v2-bundle")
 if err!=nil{t.Fatal(err)}
 childRun:=[32]byte(run)
 childRun[0]^=0x69
 childPath:=t.TempDir()+"/retry-after-parent-commit.db"
 child,err:=sqlite.Open(ctx,childPath)
 if err!=nil{t.Fatal(err)}
 defer child.Close()
 failed:=false
 provider:=func(ctx context.Context,s *sqlite.Store)([]sqlite.SnapshotCheckpoint,error){
  if !failed {
   failed=true
   return nil,errors.New("injected cognitive checkpoint provider outage")
  }
  state,err:=s.LoadClock(ctx,[32]byte(run))
  if err!=nil{return nil,err}
  return csfrv18SyntheticActorCheckpoints(t,sourcePath,state.Instant,int64(state.Revision)),nil
 }
 if _,err:=coordinator.SettleFencedThenForkV2(ctx,ledger,[32]byte(run),
  frozen.ID,childRun,child,v2,provider);err==nil||
  !strings.Contains(err.Error(),"injected cognitive checkpoint provider outage"){
  t.Fatal("expected checkpoint acquisition failure after source commit:",err)
 }
 committed,err:=parent.LoadClock(ctx,[32]byte(run))
 if err!=nil||committed.Revision!=before.Revision+1{
  t.Fatalf("source terminal not exactly once: %+v %v",committed,err)
 }
 // Re-use the SAME immutable frozen checkpoint; no re-dispatch of the V1
 // terminal is permitted. A successful second attempt proves that missing
 // external checkpoint bytes cannot strand a durable parent commit.
 childGenesis,err:=coordinator.SettleFencedThenForkV2(ctx,ledger,[32]byte(run),
  frozen.ID,childRun,child,v2,provider)
 if err!=nil{t.Fatal("retry after committed source must finish:",err)}
 if childGenesis.Payload[5]!=policy.CodecPolicyV2ID{
  t.Fatal("retry downgraded child policy")
 }
 after,err:=parent.LoadClock(ctx,[32]byte(run))
 if err!=nil||after.Revision!=committed.Revision{
  t.Fatalf("retry committed V1 source twice: %+v %v",after,err)
 }
 if _,err:=child.ReplayClock(ctx,childRun,v2);err!=nil{
  t.Fatal("recovered child is not replayable:",err)
 }
 if _,err:=parent.VerifySnapshotV1(ctx,[32]byte(run),frozen.ID,coordinator.bundle);err!=nil{
  t.Fatal("frozen source snapshot changed:",err)
 }
 // Simulate a crash AFTER child SQLite bootstrap committed, before its
 // Genesis reached the orchestrator. Reopen the child's real SQLite file,
 // then retry with the exact immutable parent cut and child ID.
 if err:=child.Close();err!=nil{t.Fatal(err)}
 reopened,err:=sqlite.Open(ctx,childPath)
 if err!=nil{t.Fatal("child reopen after injected result-loss:",err)}
 defer reopened.Close()
 // The checkpoint owner is now OFFLINE: re-contacting it would fail
 // even though an authenticated child has already durably committed.
 unexpectedProviderCalls:=0
 offlineProvider:=func(context.Context,*sqlite.Store)([]sqlite.SnapshotCheckpoint,error){
  unexpectedProviderCalls++
  return nil,errors.New("owner unavailable after child commit")
 }
 repeated,err:=coordinator.SettleFencedThenForkV2(ctx,ledger,[32]byte(run),
  frozen.ID,childRun,reopened,v2,offlineProvider)
 if unexpectedProviderCalls!=0{
  t.Fatalf("post-bootstrap recovery invoked unavailable external owner %d times",
   unexpectedProviderCalls)
 }
 if err!=nil{t.Fatal("post-bootstrap retry failed:",err)}
 if repeated.ID!=childGenesis.ID||repeated.Digest!=childGenesis.Digest{
  t.Fatal("post-bootstrap retry replaced the already published child")
 }
 parentFinal,err:=parent.LoadClock(ctx,[32]byte(run))
 if err!=nil||parentFinal.Revision!=after.Revision{
  t.Fatal("post-bootstrap retry committed another source terminal:",err)
 }
 wrongParentSnapshot:=frozen.ID
 wrongParentSnapshot[0]^=0x21
 if _,err:=reopened.RecoverPublishedForkV1ToV2(ctx,childRun,[32]byte(run),
  wrongParentSnapshot,v2);err==nil{
  t.Fatal("fork recover treated unrelated parent cut as the same child")
 }
 wrongChildID:=childRun
 wrongChildID[0]^=0x11
 if _,err:=reopened.RecoverPublishedForkV1ToV2(ctx,wrongChildID,[32]byte(run),
  frozen.ID,v2);err==nil{
  t.Fatal("fork recover silently accepted an occupied destination under another child ID")
 }
}

func TestCSFRV18R22HandoffSnapshotWriterLockRequiresFrozenRevisionAndIdle(t *testing.T){
 ctx:=context.Background()
 _,idle,ledger,run,bundle,_:=testFixture(t)
 _=ledger
 snapshotClock,err:=idle.LoadClock(ctx,[32]byte(run));if err!=nil{t.Fatal(err)}
 if _,err:=idle.CreateSnapshotV1AtRevision(ctx,[32]byte(run),bundle,nil,
  uint64(snapshotClock.Revision)+1);err==nil{
  t.Fatal("frozen handoff accepted a different revision")
 }
 signed,err:=idle.CreateSnapshotV1AtRevision(ctx,[32]byte(run),bundle,nil,
  uint64(snapshotClock.Revision))
 if err!=nil{t.Fatal("exact IDLE revision rejected:",err)}
 if _,err:=idle.VerifySnapshotV1(ctx,[32]byte(run),signed.ID,bundle);err!=nil{t.Fatal(err)}

 coordinator,source,inFlight,_,_,_:=noProposalWorkFixture(t)
 _,_,_,_,_,flightRun:=inFlight.Snapshot()
 state,err:=source.LoadClock(ctx,[32]byte(flightRun));if err!=nil{t.Fatal(err)}
 if _,err:=source.CreateSnapshotV1AtRevision(ctx,[32]byte(flightRun),
  coordinator.bundle,nil,uint64(state.Revision));err==nil{
  t.Fatal("handoff published new snapshot despite an in-flight WORK fence")
 }
 // The general V1 snapshot API deliberately retains WORK checkpoint support.
 // Only a versioned handoff to a new run requires an IDLE writer-lock fence.
 work,err:=source.CreateSnapshotV1(ctx,[32]byte(flightRun),coordinator.bundle,nil)
 if err!=nil{t.Fatal("legacy frozen WORK checkpoint was regressed:",err)}
 if _,err:=source.VerifySnapshotV1(ctx,[32]byte(flightRun),work.ID,coordinator.bundle);err!=nil{
  t.Fatal("general frozen WORK snapshot no longer resumable:",err)
 }
}

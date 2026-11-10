package commit

import (
 "bytes"
 "context"
 "crypto/ed25519"
 "crypto/rand"
 "database/sql"
 "testing"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
 _ "modernc.org/sqlite"
)

func r22Sign(t *testing.T,run [32]byte,cp sqlite.SnapshotCheckpoint,locator string,private ed25519.PrivateKey)[]byte{
 t.Helper()
 raw,err:=sqlite.CognitiveCheckpointSigningBytesV1(run,cp,locator);if err!=nil{t.Fatal(err)}
 return ed25519.Sign(private,raw)
}

// The verified owner receipt is a durable prerequisite; merely passing opaque
// bytes to snapshot creation must not bypass a pinned owner's signature.
func TestCSFRV18R22SignedOwnerReceiptDurableBeforeSnapshot(t *testing.T) {
 ctx:=context.Background()
 _,store,_,run,bundle,path:=testFixture(t)
 ns,actor:="cognition","agent"
 if err:=store.RegisterCognitiveActor(ctx,[32]byte(run),ns,actor,bundle);err!=nil{t.Fatal(err)}
 pub,private,err:=ed25519.GenerateKey(rand.Reader);if err!=nil{t.Fatal(err)}
 if err:=store.RegisterCognitiveOwnerKeyV1(ctx,[32]byte(run),ns,actor,pub,bundle);err!=nil{t.Fatal(err)}
 clock,err:=store.LoadClock(ctx,[32]byte(run));if err!=nil{t.Fatal(err)}
 cp:=sqlite.SnapshotCheckpoint{
  ActorNamespace:ns,ActorID:actor,StoreID:"memory.v1",SchemaVersion:1,
  Content:[]byte("opaque private cognition; never interpreted by CSF"),
  Producer:"memory-owner",ProducerVersion:1,
  CoversInstant:int64(clock.Instant),CoversRevision:uint64(clock.Revision),
  CoversEvidenceCursor:0,
 }
 if _,err:=store.CreateSnapshotV1(ctx,[32]byte(run),bundle,[]sqlite.SnapshotCheckpoint{cp});err==nil{
  t.Fatal("snapshot accepted unsigned owner checkpoint")
 }
 if _,err:=store.PersistCognitiveCheckpointV1(ctx,[32]byte(run),cp,"owner-checkpoint-01",
  bytes.Repeat([]byte{0x17},ed25519.SignatureSize),bundle);err==nil{
  t.Fatal("forged owner signature accepted")
 }
 signature:=r22Sign(t,[32]byte(run),cp,"owner-checkpoint-01",private)
 first,err:=store.PersistCognitiveCheckpointV1(ctx,[32]byte(run),cp,"owner-checkpoint-01",signature,bundle)
 if err!=nil{t.Fatal("owner receipt persist:",err)}
 repeat,err:=store.PersistCognitiveCheckpointV1(ctx,[32]byte(run),cp,"owner-checkpoint-01",signature,bundle)
 if err!=nil||repeat!=first{t.Fatal("owner receipt retry not idempotent:",err)}
 snap,err:=store.CreateSnapshotV1(ctx,[32]byte(run),bundle,[]sqlite.SnapshotCheckpoint{cp})
 if err!=nil{t.Fatal("durable signed receipt was not accepted:",err)}
 if _,err:=store.VerifySnapshotV1(ctx,[32]byte(run),snap.ID,bundle);err!=nil{t.Fatal(err)}
 reopened,err:=sqlite.Open(ctx,path);if err!=nil{t.Fatal(err)}
 defer reopened.Close()
 if _,err:=reopened.VerifySnapshotV1(ctx,[32]byte(run),snap.ID,bundle);err!=nil{
  t.Fatal("reopened owner receipt lost durability:",err)
 }
 restored,locator,err:=reopened.LoadCognitiveCheckpointV1(ctx,[32]byte(run),first,ns,actor,bundle)
 if err!=nil||locator!="owner-checkpoint-01"||!bytes.Equal(restored.Content,cp.Content){
  t.Fatal("replay cannot rehydrate exact independently signed owner bytes:",err)
 }
 if _,_,err:=reopened.LoadCognitiveCheckpointV1(ctx,[32]byte(run),first,ns,"other-actor",bundle);err==nil{
  t.Fatal("receipt of one actor was disclosed as another actor's checkpoint")
 }
 db,err:=sql.Open("sqlite",path);if err!=nil{t.Fatal(err)}
 defer db.Close()
 if _,err:=db.ExecContext(ctx,"UPDATE cognitive_owner_receipts SET content=? WHERE receipt_id=?",
  []byte("forged state"),first[:]);err!=nil{t.Fatal(err)}
 if _,err:=reopened.VerifySnapshotV1(ctx,[32]byte(run),snap.ID,bundle);err==nil{
  t.Fatal("tampered opaque owner bytes verified")
 }
 if _,err:=db.ExecContext(ctx,"UPDATE cognitive_owner_receipts SET content=? WHERE receipt_id=?",
  cp.Content,first[:]);err!=nil{t.Fatal(err)}
 if _,err:=reopened.VerifySnapshotV1(ctx,[32]byte(run),snap.ID,bundle);err!=nil{
  t.Fatal("restored owner receipt was not recoverable:",err)
 }
 mutated:=cp;mutated.Content=[]byte("different same-cut private state")
 if _,err:=store.PersistCognitiveCheckpointV1(ctx,[32]byte(run),mutated,
  "owner-checkpoint-01",r22Sign(t,[32]byte(run),mutated,"owner-checkpoint-01",private),bundle);err==nil{
  t.Fatal("same-coordinate conflicting owner state was accepted")
 }
 // Deleting BOTH signed owner authority rows must not turn a previously
 // signed snapshot into a valid legacy inline checkpoint.
 if _,err:=db.ExecContext(ctx,"DELETE FROM cognitive_owner_receipts");err!=nil{t.Fatal(err)}
 if _,err:=db.ExecContext(ctx,
  "DELETE FROM cognitive_owner_keys WHERE run_id=? AND actor_namespace=? AND actor_id=?",
  run[:],ns,actor);err!=nil{t.Fatal(err)}
 if _,err:=reopened.VerifySnapshotV1(ctx,[32]byte(run),snap.ID,bundle);err==nil{
  t.Fatal("signed snapshot was downgraded to unsigned legacy by deleting owner key and receipt")
 }
}

func TestCSFRV18R22OwnerTrustCannotBeRetrofittedAfterSnapshot(t *testing.T){
 ctx:=context.Background()
 _,store,_,run,bundle,_:=testFixture(t)
 if err:=store.RegisterCognitiveActor(ctx,[32]byte(run),"cognition","late-owner",bundle);err!=nil{t.Fatal(err)}
 clock,err:=store.LoadClock(ctx,[32]byte(run));if err!=nil{t.Fatal(err)}
 cp:=sqlite.SnapshotCheckpoint{ActorNamespace:"cognition",ActorID:"late-owner",
  StoreID:"memory.v1",SchemaVersion:1,Content:[]byte("legacy"),
  Producer:"memory-owner",ProducerVersion:1,CoversInstant:int64(clock.Instant),
  CoversRevision:uint64(clock.Revision),CoversEvidenceCursor:0}
 if _,err:=store.CreateSnapshotV1(ctx,[32]byte(run),bundle,[]sqlite.SnapshotCheckpoint{cp});err!=nil{t.Fatal(err)}
 pub,_,err:=ed25519.GenerateKey(rand.Reader);if err!=nil{t.Fatal(err)}
 if err:=store.RegisterCognitiveOwnerKeyV1(ctx,[32]byte(run),"cognition","late-owner",pub,bundle);err==nil{
  t.Fatal("historical unsigned actor was retroactively promoted to trusted signer")
 }
}

func TestCSFRV18R22CrashBetweenOwnerReceiptAndSnapshotPublication(t *testing.T) {
 ctx:=context.Background()
 _,source,_,run,bundle,path:=testFixture(t)
 ns,actor:="cognition","agent-retry"
 if err:=source.RegisterCognitiveActor(ctx,[32]byte(run),ns,actor,bundle);err!=nil{t.Fatal(err)}
 pub,key,err:=ed25519.GenerateKey(rand.Reader);if err!=nil{t.Fatal(err)}
 if err:=source.RegisterCognitiveOwnerKeyV1(ctx,[32]byte(run),ns,actor,pub,bundle);err!=nil{t.Fatal(err)}
 clock,err:=source.LoadClock(ctx,[32]byte(run));if err!=nil{t.Fatal(err)}
 cp:=sqlite.SnapshotCheckpoint{
  ActorNamespace:ns,ActorID:actor,StoreID:"memory.v1",SchemaVersion:1,
  Content:[]byte("owner state persisted before snapshot crash"),
  Producer:"memory-owner",ProducerVersion:1,
  CoversInstant:int64(clock.Instant),CoversRevision:uint64(clock.Revision),
 }
 signed:=r22Sign(t,[32]byte(run),cp,"owner-cut-01",key)
 receipt,err:=source.PersistCognitiveCheckpointV1(ctx,[32]byte(run),cp,"owner-cut-01",signed,bundle)
 if err!=nil{t.Fatal(err)}
 db,err:=sql.Open("sqlite",path);if err!=nil{t.Fatal(err)}
 defer db.Close()
 if _,err:=db.ExecContext(ctx,"CREATE TRIGGER stop_snapshot BEFORE INSERT ON snapshot_seals BEGIN SELECT RAISE(ABORT,'injected snapshot crash'); END");err!=nil{t.Fatal(err)}
 if _,err:=source.CreateSnapshotV1(ctx,[32]byte(run),bundle,[]sqlite.SnapshotCheckpoint{cp});err==nil{
  t.Fatal("injected snapshot publication crash was ignored")
 }
 var snapshots,seals,escrow int
 for _,item:=range []struct{table string;out *int}{
  {"snapshots",&snapshots},{"snapshot_seals",&seals},{"cognitive_owner_receipts",&escrow},
 } {
  if err:=db.QueryRowContext(ctx,"SELECT count(*) FROM "+item.table).Scan(item.out);err!=nil{t.Fatal(err)}
 }
 if snapshots!=0||seals!=0||escrow!=1{
  t.Fatalf("owner-before-ref atomicity lost: snapshots=%d seals=%d escrow=%d",snapshots,seals,escrow)
 }
 if _,err:=db.ExecContext(ctx,"DROP TRIGGER stop_snapshot");err!=nil{t.Fatal(err)}
 reopened,err:=sqlite.Open(ctx,path);if err!=nil{t.Fatal(err)}
 defer reopened.Close()
 restored,_,err:=reopened.LoadCognitiveCheckpointV1(ctx,[32]byte(run),receipt,ns,actor,bundle)
 if err!=nil||!bytes.Equal(restored.Content,cp.Content){t.Fatal("owner exact restore after crash:",err)}
 result,err:=reopened.CreateSnapshotV1(ctx,[32]byte(run),bundle,[]sqlite.SnapshotCheckpoint{restored})
 if err!=nil{t.Fatal("post-crash retry of same signed cut:",err)}
 if _,err:=reopened.VerifySnapshotV1(ctx,[32]byte(run),result.ID,bundle);err!=nil{
  t.Fatal("recovered snapshot not replayable:",err)
 }
}

func TestCSFRV18R22V2ForkPreservesPinnedOwnerProofAndRequiresChildReceipt(t *testing.T){
 ctx:=context.Background()
 v1,err:=policy.Load(bundlePath);if err!=nil{t.Fatal(err)}
 g:=validGenesis(t,v1)
 parent,err:=sqlite.Open(ctx,t.TempDir()+"/signed-parent.db");if err!=nil{t.Fatal(err)}
 defer parent.Close()
 if err:=parent.PersistGenesis(ctx,g,v1);err!=nil{t.Fatal(err)}
 if err:=parent.BootstrapClock(ctx,g,v1);err!=nil{t.Fatal(err)}
 if err:=parent.BootstrapGenesisTriggers(ctx,g,v1,nil);err!=nil{t.Fatal(err)}
 srcRun:=[32]byte(g.RunID)
 if err:=parent.RegisterCognitiveActor(ctx,srcRun,"cognition","agent",v1);err!=nil{t.Fatal(err)}
 pub,key,err:=ed25519.GenerateKey(rand.Reader);if err!=nil{t.Fatal(err)}
 if err:=parent.RegisterCognitiveOwnerKeyV1(ctx,srcRun,"cognition","agent",pub,v1);err!=nil{t.Fatal(err)}
 state,err:=parent.LoadClock(ctx,srcRun);if err!=nil{t.Fatal(err)}
 cp:=sqlite.SnapshotCheckpoint{
  ActorNamespace:"cognition",ActorID:"agent",StoreID:"memory.v1",SchemaVersion:1,
  Content:[]byte("source epistemic state"),Producer:"owner",ProducerVersion:1,
  CoversInstant:int64(state.Instant),CoversRevision:uint64(state.Revision),
 }
 _,err=parent.PersistCognitiveCheckpointV1(ctx,srcRun,cp,"owner-source-cut",
  r22Sign(t,srcRun,cp,"owner-source-cut",key),v1)
 if err!=nil{t.Fatal(err)}
 snap,err:=parent.CreateSnapshotV1(ctx,srcRun,v1,[]sqlite.SnapshotCheckpoint{cp})
 if err!=nil{t.Fatal(err)}
 v2,err:=policy.LoadV2("../../../docs/architecture/canonical-codec-v2-bundle")
 if err!=nil{t.Fatal(err)}
 childPath:=t.TempDir()+"/signed-v2-child.db"
 child,err:=sqlite.Open(ctx,childPath);if err!=nil{t.Fatal(err)}
 defer child.Close()
 childRun:=srcRun;childRun[0]^=0x48
 if _,err:=parent.ForkIdleSnapshotV1ToV2(ctx,srcRun,snap.ID,childRun,child,v1,v2);err!=nil{
  t.Fatal("signed source checkpoint failed V2 migration:",err)
 }
 if _,err:=child.ReplayClock(ctx,childRun,v2);err!=nil{t.Fatal(err)}
 // The historical parent's signature binds SOURCE run only. The child may
 // re-use the exact opaque bytes but must first obtain an OWNER signature
 // explicitly covering its new run identity.
 if _,err:=child.CreateSnapshotV1(ctx,childRun,v2,[]sqlite.SnapshotCheckpoint{cp});err==nil{
  t.Fatal("V2 child downgraded parent's signing requirement to legacy inline")
 }
 signedChild:=r22Sign(t,childRun,cp,"owner-child-cut",key)
 if _,err:=child.PersistCognitiveCheckpointV1(ctx,childRun,cp,"owner-child-cut",signedChild,v2);err!=nil{
  t.Fatal("new-run owner re-attestation rejected:",err)
 }
 // R22 composition: after fork, an independently enrolled child-native
 // cognitive actor must not be confused with the frozen parent's roster.
 if err:=child.RegisterCognitiveActor(ctx,childRun,"cognition","new-agent",v2);err!=nil{
  t.Fatal("V2 child could not enroll a new actor:",err)
 }
 newPub,newKey,err:=ed25519.GenerateKey(rand.Reader);if err!=nil{t.Fatal(err)}
 if err:=child.RegisterCognitiveOwnerKeyV1(ctx,childRun,
  "cognition","new-agent",newPub,v2);err!=nil{
  t.Fatal("V2 child could not pin a new native signer:",err)
 }
 added:=cp
 added.ActorID="new-agent"
 added.Content=[]byte("child-native signed memory")
 added.CoversRevision=0
 if _,err:=child.PersistCognitiveCheckpointV1(ctx,childRun,added,
  "new-agent-child-cut",r22Sign(t,childRun,added,"new-agent-child-cut",newKey),v2);err!=nil{
  t.Fatal("child-native owner receipt missing:",err)
 }
 if _,err:=child.ReplayClock(ctx,childRun,v2);err!=nil{
  t.Fatal("child-native owner broke authenticated historical fork lineage:",err)
 }
 childSnap,err:=child.CreateSnapshotV1(ctx,childRun,v2,[]sqlite.SnapshotCheckpoint{cp,added})
 if err!=nil{t.Fatal(err)}
 if _,err:=child.VerifySnapshotV1(ctx,childRun,childSnap.ID,v2);err!=nil{t.Fatal(err)}
 probe,err:=sql.Open("sqlite",childPath);if err!=nil{t.Fatal(err)}
 defer probe.Close()
 if _,err:=probe.ExecContext(ctx,"UPDATE fork_cognitive_owner_proofs SET signature=zeroblob(64)");err!=nil{t.Fatal(err)}
 if _,err:=child.ReplayClock(ctx,childRun,v2);err==nil{
  t.Fatal("corrupted origin owner's imported proof accepted by child replay")
 }
 if _,err:=parent.VerifySnapshotV1(ctx,srcRun,snap.ID,v1);err!=nil{
  t.Fatal("child signer tamper invalidated original source snapshot:",err)
 }
 // Restore signature and then DELETE BOTH live key and historical proof.
 // The durable fork proof roster seal must detect this trust downgrade.
 sourceSig:=r22Sign(t,srcRun,cp,"owner-source-cut",key)
 if _,err:=probe.ExecContext(ctx,
  "UPDATE fork_cognitive_owner_proofs SET signature=?",sourceSig);err!=nil{t.Fatal(err)}
 if _,err:=child.ReplayClock(ctx,childRun,v2);err!=nil{
  t.Fatal("restored exact source proof did not replay:",err)
 }
 if _,err:=probe.ExecContext(ctx,
  "DELETE FROM fork_cognitive_owner_proofs");err!=nil{t.Fatal(err)}
 if _,err:=probe.ExecContext(ctx,
  "DELETE FROM cognitive_owner_receipts");err!=nil{t.Fatal(err)}
 if _,err:=probe.ExecContext(ctx,
  "DELETE FROM cognitive_owner_keys");err!=nil{t.Fatal(err)}
 if _,err:=child.ReplayClock(ctx,childRun,v2);err==nil{
  t.Fatal("deleting both signer and proof bypassed child trust gate")
 }
}

func TestCSFRV18R2401LateEnrolledActorCanSignWithoutRewritingOlderTrust(t *testing.T) {
 ctx:=context.Background()
 coordinator,store,ledger,run,bundle,path:=testFixture(t,activeTrigger())
 initial,err:=store.CreateSnapshotV1(ctx,[32]byte(run),bundle,nil)
 if err!=nil{t.Fatal(err)}
 if _,err:=coordinator.Advance(ctx,ledger,run);err!=nil{t.Fatal("advance to later revision:",err)}
 state,err:=store.LoadClock(ctx,[32]byte(run));if err!=nil{t.Fatal(err)}
 if state.Revision!=1{t.Fatalf("unexpected post-snapshot revision: %d",state.Revision)}
 if err:=store.RegisterCognitiveActor(ctx,[32]byte(run),"cognition","late-signer",bundle);err!=nil{
  t.Fatal("new actor enrollment after old snapshot:",err)
 }
 pub,key,err:=ed25519.GenerateKey(rand.Reader);if err!=nil{t.Fatal(err)}
 if err:=store.RegisterCognitiveOwnerKeyV1(ctx,[32]byte(run),"cognition","late-signer",pub,bundle);err!=nil{
  t.Fatal("R24-01: later actor could not register a signed owner:",err)
 }
 cp:=sqlite.SnapshotCheckpoint{
  ActorNamespace:"cognition",ActorID:"late-signer",StoreID:"memory.v1",
  SchemaVersion:1,Content:[]byte("late but signed"),
  Producer:"memory-owner",ProducerVersion:1,CoversInstant:int64(state.Instant),
  CoversRevision:uint64(state.Revision),CoversEvidenceCursor:0,
 }
 if _,err:=store.CreateSnapshotV1(ctx,[32]byte(run),bundle,[]sqlite.SnapshotCheckpoint{cp});err==nil{
  t.Fatal("newly enrolled signed actor was accepted without owner receipt")
 }
 signature:=r22Sign(t,[32]byte(run),cp,"late-actor-cut",key)
 if _,err:=store.PersistCognitiveCheckpointV1(ctx,[32]byte(run),cp,
  "late-actor-cut",signature,bundle);err!=nil{t.Fatal(err)}
 later,err:=store.CreateSnapshotV1(ctx,[32]byte(run),bundle,[]sqlite.SnapshotCheckpoint{cp})
 if err!=nil{t.Fatal("new snapshot rejected signed later actor:",err)}
 if _,err:=store.VerifySnapshotV1(ctx,[32]byte(run),initial.ID,bundle);err!=nil{
  t.Fatal("older snapshot trust roster changed after valid future enrollment:",err)
 }
 if _,err:=store.VerifySnapshotV1(ctx,[32]byte(run),later.ID,bundle);err!=nil{
  t.Fatal("later signed actor checkpoint invalid:",err)
 }
 if err:=store.RegisterCognitiveOwnerKeyV1(ctx,[32]byte(run),
  "cognition","late-signer",pub,bundle);err!=nil{t.Fatal("idempotent signer key changed:",err)}
 another,_,err:=ed25519.GenerateKey(rand.Reader);if err!=nil{t.Fatal(err)}
 if err:=store.RegisterCognitiveOwnerKeyV1(ctx,[32]byte(run),
  "cognition","late-signer",another,bundle);err==nil{
  t.Fatal("signed owner key was replaced after snapshot")
 }
 reopened,err:=sqlite.Open(ctx,path);if err!=nil{t.Fatal(err)}
 defer reopened.Close()
 for _,snap:=range []sqlite.SnapshotV1{initial,later}{
  if _,err:=reopened.VerifySnapshotV1(ctx,[32]byte(run),snap.ID,bundle);err!=nil{
   t.Fatal("snapshot changed after reopen and future actor registration:",err)
  }
 }
}

func TestCSFRV18R2501EvidenceHolderSignedLaterKeepsHistoricalLegacySnapshot(t *testing.T) {
 ctx:=context.Background()
 // A real addressed round creates validated evidence_ledger and
 // evidence_routes entries for school/a BEFORE cognitive enrollment.
 c,store,ledger,cycle,fence,path:=singleResourceFixtureWithFuture(t,true)
 _,_,_,_,_,runID:=ledger.Snapshot()
 run:=[32]byte(runID)
 frozen,err:=store.CreateSnapshotV1(ctx,run,c.bundle,nil)
 if err!=nil{t.Fatal("initial in-flight snapshot:",err)}
 if _,err:=c.CommitSingleResource(ctx,ledger,cycle,fence);err!=nil{t.Fatal(err)}
 if _,err:=store.DrainPendingPerceptionV1(ctx,run,frozen.ID,c.bundle);err!=nil{
  t.Fatal("draining real addressed evidence:",err)
 }
 probe,err:=sql.Open("sqlite",path);if err!=nil{t.Fatal(err)}
 defer probe.Close()
 var routes,registered int
 if err:=probe.QueryRowContext(ctx,
  "SELECT count(*) FROM evidence_routes WHERE run_id=? AND holder_namespace=? AND holder_id=?",
  run[:],"school","a").Scan(&routes);err!=nil{t.Fatal(err)}
 if routes!=2{t.Fatalf("expected real observation and knowledge input for school/a, got %d",routes)}
 if err:=probe.QueryRowContext(ctx,
  "SELECT count(*) FROM cognitive_actor_registry WHERE run_id=? AND actor_namespace=? AND actor_id=?",
  run[:],"school","a").Scan(&registered);err!=nil{t.Fatal(err)}
 if registered!=0{t.Fatal("fixture already registered evidence-only holder")}
 prior,err:=store.LoadClock(ctx,run);if err!=nil{t.Fatal(err)}
 if prior.Revision!=1{t.Fatalf("expected evidence revision 1, got %d",prior.Revision)}
 legacy:=csfrv18SyntheticActorCheckpoints(t,path,prior.Instant,int64(prior.Revision))
 if len(legacy)!=1||legacy[0].ActorNamespace!="school"||legacy[0].ActorID!="a"{
  t.Fatalf("expected a single legacy evidence holder: %+v",legacy)
 }
 historical,err:=store.CreateSnapshotV1(ctx,run,c.bundle,legacy)
 if err!=nil{t.Fatal("persist legacy evidence snapshot:",err)}
 if _,err:=store.VerifySnapshotV1(ctx,run,historical.ID,c.bundle);err!=nil{
  t.Fatal("legacy evidence snapshot invalid before enrollment:",err)
 }
 advance,err:=c.Advance(ctx,ledger,runID)
 if err!=nil{t.Fatal("advance from evidence R1 to R2:",err)}
 if advance.ResultClock.Revision!=2{t.Fatal("future advance did not create revision 2")}
 if err:=store.RegisterCognitiveActor(ctx,run,"school","a",c.bundle);err!=nil{
  t.Fatal("register previously addressed holder at later revision:",err)
 }
 public,private,err:=ed25519.GenerateKey(rand.Reader);if err!=nil{t.Fatal(err)}
 if err:=store.RegisterCognitiveOwnerKeyV1(ctx,run,"school","a",public,c.bundle);err!=nil{
  t.Fatal("new evidence holder must be signable in R2:",err)
 }
 // R25-01: before this fix, VerifySnapshotV1 rejected this *authentic*
 // historical legacy checkpoint for missing a non-existent old signature.
 if _,err:=store.VerifySnapshotV1(ctx,run,historical.ID,c.bundle);err!=nil{
  t.Fatal("historical unsigned evidence cut retroactively required new signature:",err)
 }
 later:=csfrv18SyntheticActorCheckpoints(t,path,advance.ResultClock.Instant,
  int64(advance.ResultClock.Revision))
 if len(later)!=1||later[0].ActorID!="a"{t.Fatal("late signed owner checkpoint missing")}
 if _,err:=store.CreateSnapshotV1(ctx,run,c.bundle,later);err==nil{
  t.Fatal("future signed snapshot accepted a legacy unsigned checkpoint")
 }
 receipt,err:=store.PersistCognitiveCheckpointV1(ctx,run,later[0],"later-evidence-owner",
  r22Sign(t,run,later[0],"later-evidence-owner",private),c.bundle)
 if err!=nil{t.Fatal("new owner signed receipt:",err)}
 current,err:=store.CreateSnapshotV1(ctx,run,c.bundle,later)
 if err!=nil{t.Fatal("publish newly authenticated R2:",err)}
 reopened,err:=sqlite.Open(ctx,path);if err!=nil{t.Fatal(err)}
 defer reopened.Close()
 for _,snap:=range []sqlite.SnapshotV1{historical,current}{
  if _,err:=reopened.VerifySnapshotV1(ctx,run,snap.ID,c.bundle);err!=nil{
   t.Fatalf("snapshot R%d not replayable after new signer: %v",snap.Revision,err)
  }
 }
 if _,_,err:=reopened.LoadCognitiveCheckpointV1(ctx,run,receipt,"school","a",c.bundle);err!=nil{
  t.Fatal("signed owner receipt cannot be restored:",err)
 }
 // Deleting a key that WAS part of the later snapshot's sealed authority
 // must fail closed. The earlier legacy cut remains authentic.
 if _,err:=probe.ExecContext(ctx,
  "DELETE FROM cognitive_owner_receipts WHERE run_id=? AND actor_namespace=? AND actor_id=?",
  run[:],"school","a");err!=nil{t.Fatal(err)}
 if _,err:=probe.ExecContext(ctx,
  "DELETE FROM cognitive_owner_keys WHERE run_id=? AND actor_namespace=? AND actor_id=?",
  run[:],"school","a");err!=nil{t.Fatal(err)}
 if _,err:=reopened.VerifySnapshotV1(ctx,run,current.ID,c.bundle);err==nil{
  t.Fatal("late signed cut was downgraded after deleting its key and receipt")
 }
 if _,err:=reopened.VerifySnapshotV1(ctx,run,historical.ID,c.bundle);err!=nil{
  t.Fatal("legitimate earlier legacy snapshot was invalidated by new-owner deletion:",err)
 }
}

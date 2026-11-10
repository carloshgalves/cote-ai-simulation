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
 childSnap,err:=child.CreateSnapshotV1(ctx,childRun,v2,[]sqlite.SnapshotCheckpoint{cp})
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
}

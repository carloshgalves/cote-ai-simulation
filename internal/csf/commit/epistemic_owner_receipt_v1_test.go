package commit

import (
 "bytes"
 "context"
 "crypto/ed25519"
 "crypto/rand"
 "database/sql"
 "testing"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
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

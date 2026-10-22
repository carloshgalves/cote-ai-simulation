package commit

import (
 "bytes"
 "context"
 "database/sql"
 "testing"
 _ "modernc.org/sqlite"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
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
 if _,err:=store.VerifySnapshotV1(ctx,[32]byte(run),before.ID,bundle);err==nil{
  t.Fatal("stale snapshot was silently accepted without replay of its tails")
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

package commit

import (
 "bytes"
 "context"
 "database/sql"
 "testing"
 _ "modernc.org/sqlite"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
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

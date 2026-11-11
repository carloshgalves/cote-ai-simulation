package commit

import (
 "context"
 "database/sql"
 "testing"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
 _ "modernc.org/sqlite"
)

// A destination containing untracked claims is NOT an empty fork workspace.
// The bootstrap must reject it before any Genesis or lineage row is created,
// even though communication_claims is not one of the five canonical ledgers.
func TestCSFRV18ForkRejectsDirtyCommunicationDestination(t *testing.T) {
 ctx:=context.Background()
 bundle,err:=policy.Load(bundlePath);if err!=nil{t.Fatal(err)}
 root:=validGenesis(t,bundle)
 parent,err:=sqlite.Open(ctx,t.TempDir()+"/root.db");if err!=nil{t.Fatal(err)}
 defer parent.Close()
 if err:=parent.PersistGenesis(ctx,root,bundle);err!=nil{t.Fatal(err)}
 if err:=parent.BootstrapClock(ctx,root,bundle);err!=nil{t.Fatal(err)}
 if err:=parent.BootstrapGenesisTriggers(ctx,root,bundle,nil);err!=nil{t.Fatal(err)}
 snap,err:=parent.CreateSnapshotV1(ctx,[32]byte(root.RunID),bundle,nil);if err!=nil{t.Fatal(err)}
 path:=t.TempDir()+"/dirty.db"
 child,err:=sqlite.Open(ctx,path);if err!=nil{t.Fatal(err)}
 defer child.Close()
 direct,err:=sql.Open("sqlite",path);if err!=nil{t.Fatal(err)}
 defer direct.Close()
 bogus:=make([]byte,32);bogus[0]=0xa7
 if _,err:=direct.ExecContext(ctx,
  "INSERT INTO communication_claims(id,run_id,canonical) VALUES(?,?,?)",
  bogus,bogus,[]byte{0x80});err!=nil{t.Fatal(err)}
 childRun:=[32]byte(root.RunID);childRun[0]^=0x2b
 if _,err:=parent.ForkIdleSnapshotV1(ctx,[32]byte(root.RunID),snap.ID,childRun,child,bundle);err==nil{
  t.Fatal("fork accepted a destination with alien communication authority")
 }
 var count int
 if err:=direct.QueryRowContext(ctx,"SELECT count(*) FROM genesis_records").Scan(&count);err!=nil{t.Fatal(err)}
 if count!=0{t.Fatal("rejected dirty destination partially received Genesis")}
 if err:=direct.QueryRowContext(ctx,"SELECT count(*) FROM communication_claims").Scan(&count);err!=nil{t.Fatal(err)}
 if count!=1{t.Fatal("rejected fork altered unrelated existing authority")}
}

func TestCSFRV18ForkRejectsOrphanSignedSnapshotSealDestination(t *testing.T) {
 ctx:=context.Background()
 b,err:=policy.Load(bundlePath);if err!=nil{t.Fatal(err)}
 g:=validGenesis(t,b)
 source,err:=sqlite.Open(ctx,t.TempDir()+"/source.db");if err!=nil{t.Fatal(err)}
 defer source.Close()
 if err:=source.PersistGenesis(ctx,g,b);err!=nil{t.Fatal(err)}
 if err:=source.BootstrapClock(ctx,g,b);err!=nil{t.Fatal(err)}
 if err:=source.BootstrapGenesisTriggers(ctx,g,b,nil);err!=nil{t.Fatal(err)}
 snapshot,err:=source.CreateSnapshotV1(ctx,[32]byte(g.RunID),b,nil);if err!=nil{t.Fatal(err)}
 dstPath:=t.TempDir()+"/orphan-owner-seal.db"
 dest,err:=sqlite.Open(ctx,dstPath);if err!=nil{t.Fatal(err)}
 defer dest.Close()
 tamper,err:=sql.Open("sqlite",dstPath);if err!=nil{t.Fatal(err)}
 defer tamper.Close()
 if _,err:=tamper.ExecContext(ctx,"PRAGMA foreign_keys=OFF");err!=nil{t.Fatal(err)}
 orphan:=make([]byte,32);orphan[0]=0xbe
 if _,err:=tamper.ExecContext(ctx,
  "INSERT INTO snapshot_owner_key_seals(snapshot_id,run_id,owner_count,owner_roster_hash) VALUES(?,?,?,?)",
  orphan,orphan,0,orphan);err!=nil{t.Fatal(err)}
 childRun:=[32]byte(g.RunID);childRun[0]^=0x5d
 if _,err:=source.ForkIdleSnapshotV1(ctx,[32]byte(g.RunID),snapshot.ID,
  childRun,dest,b);err==nil{
  t.Fatal("fork accepted preexisting orphan snapshot signer seal in destination")
 }
 var count int
 if err:=tamper.QueryRowContext(ctx,"SELECT count(*) FROM genesis_records").Scan(&count);err!=nil{t.Fatal(err)}
 if count!=0{t.Fatal("rejected signer-seal workspace received a partial child Genesis")}
 if err:=tamper.QueryRowContext(ctx,"SELECT count(*) FROM snapshot_owner_key_seals").Scan(&count);err!=nil{t.Fatal(err)}
 if count!=1{t.Fatal("rejected fork mutated the preexisting trust authority")}
 // After rejection, remove only the foreign authority and prove that the
 // original destination can be bootstrapped, reopened, and causally replayed.
 if _,err:=tamper.ExecContext(ctx,"DELETE FROM snapshot_owner_key_seals");err!=nil{t.Fatal(err)}
 if _,err:=source.ForkIdleSnapshotV1(ctx,[32]byte(g.RunID),snapshot.ID,
  childRun,dest,b);err!=nil{t.Fatalf("clean retry of rejected fork: %v",err)}
 if err:=dest.Close();err!=nil{t.Fatal(err)}
 reopened,err:=sqlite.Open(ctx,dstPath);if err!=nil{t.Fatal(err)}
 defer reopened.Close()
 if _,err:=reopened.ValidatedGenesis(ctx,childRun,b);err!=nil{
  t.Fatalf("reopen of recovered fork lineage: %v",err)
 }
 if _,err:=reopened.ReplayClock(ctx,childRun,b);err!=nil{
  t.Fatalf("replay of recovered fork after prior rejection: %v",err)
 }
}

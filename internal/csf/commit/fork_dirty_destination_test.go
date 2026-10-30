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

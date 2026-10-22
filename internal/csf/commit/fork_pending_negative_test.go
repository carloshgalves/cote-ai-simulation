package commit

import (
 "context"
 "database/sql"
 "testing"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
 _ "modernc.org/sqlite"
)

// Unmigrated parent-owned communication and round IDs can never become live
// child authority merely because a history prefix has been archived. Until a
// verified and atomic migrator exists, fork must reject and leave no child.
func TestCSFRV18UnsupportedOpenCommunicationAndRoundForkFailClosed(t *testing.T){
 ctx:=context.Background()
 t.Run("communication-in-transit",func(t *testing.T){
  c,parent,ledger,cycle,fence,_:=communicationFixture(t)
  _,_,_,_,_,run:=ledger.Snapshot()
  if _,err:=c.CommitCommunicationSend(ctx,ledger,cycle,fence);err!=nil{t.Fatal(err)}
  snap,err:=parent.CreateSnapshotV1(ctx,[32]byte(run),c.bundle,nil);if err!=nil{t.Fatal(err)}
  childPath:=t.TempDir()+"/transit-child.db"
  child,err:=sqlite.Open(ctx,childPath);if err!=nil{t.Fatal(err)}
  defer child.Close()
  id:=[32]byte(run);id[0]^=0x51
  if _,err:=parent.ForkIdleSnapshotV1(ctx,[32]byte(run),snap.ID,id,child,c.bundle);err==nil{
   t.Fatal("fork silently migrated IN_TRANSIT claim without dependent event/round authority")
  }
  assertCSFRV18EmptyRejectedFork(t,childPath)
 })
 t.Run("fenced-unfinished-round",func(t *testing.T){
  c,parent,ledger,_,_,_:=noProposalWorkFixture(t)
  _,_,_,_,_,run:=ledger.Snapshot()
  snap,err:=parent.CreateSnapshotV1(ctx,[32]byte(run),c.bundle,nil);if err!=nil{t.Fatal(err)}
  childPath:=t.TempDir()+"/round-child.db"
  child,err:=sqlite.Open(ctx,childPath);if err!=nil{t.Fatal(err)}
  defer child.Close()
  id:=[32]byte(run);id[0]^=0x62
  if _,err:=parent.ForkIdleSnapshotV1(ctx,[32]byte(run),snap.ID,id,child,c.bundle);err==nil{
   t.Fatal("fork silently discarded an admitted in-flight round and immutable fence")
  }
  assertCSFRV18EmptyRejectedFork(t,childPath)
 })
}

func assertCSFRV18EmptyRejectedFork(t *testing.T,path string){
 t.Helper()
 db,err:=sql.Open("sqlite",path);if err!=nil{t.Fatal(err)}
 defer db.Close()
 // Query the actual runtime schema rather than a hand-maintained subset:
 // a rejected fork must publish zero rows in EVERY authority table, including
 // newly added round/dispatch, claim, outbox, trigger and cognitive tables.
 rows,err:=db.Query("SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name")
 if err!=nil{t.Fatal(err)}
 var names []string
 for rows.Next(){var name string;if err:=rows.Scan(&name);err!=nil{rows.Close();t.Fatal(err)};names=append(names,name)}
 if err:=rows.Err();err!=nil{rows.Close();t.Fatal(err)}
 if err:=rows.Close();err!=nil{t.Fatal(err)}
 if len(names)<30{t.Fatalf("fork destination schema incomplete: %d authority tables",len(names))}
 for _,table:=range names{
  var count int
  // sqlite_master names are the trusted identifiers emitted by Open/createSchema.
  if err:=db.QueryRow("SELECT count(*) FROM "+table).Scan(&count);err!=nil{t.Fatal(err)}
  if count!=0{t.Fatalf("rejected fork wrote %d rows to %s",count,table)}
 }
}

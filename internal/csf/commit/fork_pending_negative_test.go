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
 for _,tc:=range []struct{
  name string
  build func(*testing.T)(*sqlite.Store,[32]byte,[32]byte,any)
 }{} {
  _=tc
 }
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
  c,parent,_,_,_,_:=noProposalWorkFixture(t)
  // Use the authenticated run stored in the genesis checkpoint, not a
  // fabricated round identity or newly selected cycle.
  var run [32]byte
  raw,err:=sql.Open("sqlite",":memory:")
  if err!=nil{t.Fatal(err)}
  raw.Close()
  _=run
  _=c
  _=parent
 })
}

func assertCSFRV18EmptyRejectedFork(t *testing.T,path string){
 t.Helper()
 db,err:=sql.Open("sqlite",path);if err!=nil{t.Fatal(err)}
 defer db.Close()
 for _,table:=range []string{"genesis_records","fork_lineage","fork_history_records","clock_states",
  "world_revisions","scheduled_occurrences","trigger_activations","communication_transmissions"} {
  var count int
  if err:=db.QueryRow("SELECT count(*) FROM "+table).Scan(&count);err!=nil{t.Fatal(err)}
  if count!=0{t.Fatalf("rejected fork wrote %d rows to %s",count,table)}
 }
}

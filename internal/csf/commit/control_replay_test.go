package commit

import (
 "context"
 "database/sql"
 "testing"

 _ "modernc.org/sqlite"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
)

func TestCycleControlReplayDistinguishesGenesisFromMissingHistory(t *testing.T){
 ctx:=context.Background()
 c,store,ledger,run,_,path:=freshResourceWorkFixture(t)
 // No fence exists at genesis; this is the only legitimately absent row.
 if err:=store.VerifyCycleControl(ctx,[32]byte(run));err!=nil{t.Fatal(err)}
 c2,store2,ledger2,cycle,fence,path2:=noProposalWorkFixture(t)
 _,_,_,_,_,run2:=ledger2.Snapshot()
 if _,err:=c2.CommitNoProposal(ctx,ledger2,cycle,fence);err!=nil{t.Fatal(err)}
 if err:=store2.VerifyCycleControl(ctx,[32]byte(run2));err!=nil{t.Fatal(err)}
 db,err:=sql.Open("sqlite","file:"+path2);if err!=nil{t.Fatal(err)}
 if _,err:=db.ExecContext(ctx,"DELETE FROM cycle_control_state WHERE run_id=?",run2[:]);err!=nil{t.Fatal(err)}
 db.Close()
 reopened,err:=sqlite.Open(ctx,path2);if err!=nil{t.Fatal(err)}
 defer reopened.Close()
 if err:=reopened.VerifyCycleControl(ctx,[32]byte(run2));err==nil{
  t.Fatal("missing control projection after durable CycleCommit accepted as genesis")
 }
 if _,err:=reopened.ReplayClock(ctx,[32]byte(run2),c2.bundle);err==nil{
  t.Fatal("replay accepted erased post-commit control")
 }
 // A pristine reference cannot be forged merely by appending a journal root.
 db2,err:=sql.Open("sqlite","file:"+path);if err!=nil{t.Fatal(err)}
 if _,err:=db2.ExecContext(ctx,"INSERT INTO decision_ledger(id,canonical) VALUES(?,?)",
  make([]byte,32),[]byte{0x80});err!=nil{t.Fatal(err)}
 db2.Close()
 if err:=store.VerifyCycleControl(ctx,[32]byte(run));err==nil{
  t.Fatal("unbound decision record bypassed missing genesis control")
 }
 _=c;_=ledger
}

func TestCycleControlReplayChecksAbortAndRetryJournalReferences(t *testing.T){
 ctx:=context.Background()
 c,store,ledger,fence,run,cycle,path:=abortFixture(t)
 if err:=store.VerifyCycleControl(ctx,[32]byte(run));err!=nil{t.Fatal(err)}
 aborted,err:=c.Abort(ctx,run,cycle,fence,4,nil);if err!=nil{t.Fatal(err)}
 if err:=store.VerifyCycleControl(ctx,[32]byte(run));err!=nil{t.Fatal(err)}
 if _,err:=c.Retry(ctx,run,cycle,aborted,"operator.retry");err!=nil{t.Fatal(err)}
 if err:=store.VerifyCycleControl(ctx,[32]byte(run));err!=nil{t.Fatal(err)}
 db,err:=sql.Open("sqlite","file:"+path);if err!=nil{t.Fatal(err)}
 if _,err:=db.ExecContext(ctx,"DELETE FROM decision_ledger WHERE id=?",aborted.Identity[:]);err!=nil{t.Fatal(err)}
 db.Close()
 if err:=store.VerifyCycleControl(ctx,[32]byte(run));err==nil{
  t.Fatal("RETRY_AUTHORIZED survived removal of its referenced abort")
 }
 _=ledger
}

package commit

import (
 "bytes"
 "context"
 "database/sql"
 "fmt"
 "testing"
 _ "modernc.org/sqlite"
)

// Integrated resource contest produces a full journal and perceptual outputs.
// Each authoritative ledger must be independently covered by the original
// checkpoint plus authenticated tail; changing a record and keeping its ID
// cannot be accepted by a subsequent recovery attempt.
func TestCSFRV18IntegratedFiveLedgerTamperFailsClosed(t *testing.T) {
 ctx:=context.Background()
 c,store,ledger,cycle,fence,path:=twoResourceContestFixtureWithSystem(t,false,true,true)
 _,_,_,_,_,run:=ledger.Snapshot()
 sealed,err:=store.CreateSnapshotV1(ctx,[32]byte(run),c.bundle,nil)
 if err!=nil{t.Fatal(err)}
 if _,err:=c.CommitTwoResourceContest(ctx,ledger,cycle,fence);err!=nil{t.Fatal(err)}
 if _,err:=store.DrainPendingPerceptionV1(ctx,[32]byte(run),sealed.ID,c.bundle);err!=nil{t.Fatal(err)}
 if _,err:=store.VerifySnapshotV1(ctx,[32]byte(run),sealed.ID,c.bundle);err!=nil{
  t.Fatal("legitimate integrated contest+perception tail did not replay:",err)
 }
 direct,err:=sql.Open("sqlite",path);if err!=nil{t.Fatal(err)}
 defer direct.Close()
 for _,table:=range []string{"input_ledger","decision_ledger","event_store","evidence_ledger","causal_outbox"}{
  t.Run(table,func(t *testing.T){
   query:=fmt.Sprintf("SELECT id,canonical FROM %s ORDER BY id LIMIT 1",table)
   var id,raw []byte
   if err:=direct.QueryRowContext(ctx,query).Scan(&id,&raw);err!=nil{
    t.Fatalf("integrated scenario lacks authoritative %s records: %v",table,err)
   }
   damaged:=bytes.Clone(raw)
   if len(damaged)==0{t.Fatal("empty canonical record")}
   damaged[len(damaged)-1]^=0x01
   update:=fmt.Sprintf("UPDATE %s SET canonical=? WHERE id=?",table)
   if _,err:=direct.ExecContext(ctx,update,damaged,id);err!=nil{t.Fatal(err)}
   if _,err:=store.VerifySnapshotV1(ctx,[32]byte(run),sealed.ID,c.bundle);err==nil{
    t.Fatal("recovery accepted byte-corrupt authoritative record")
   }
   if _,err:=direct.ExecContext(ctx,update,raw,id);err!=nil{t.Fatal(err)}
   if _,err:=store.VerifySnapshotV1(ctx,[32]byte(run),sealed.ID,c.bundle);err!=nil{
    t.Fatal("restoring original authoritative bytes did not recover:",err)
   }
  })
 }
}

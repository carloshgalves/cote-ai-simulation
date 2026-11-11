package commit

import (
 "bytes"
 "context"
 "database/sql"
 "fmt"
 "path/filepath"
 "testing"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
 _ "modernc.org/sqlite"
)

// A read-only consumer cannot bypass the same lineage proof checked in the
// publisher before commit. Each mutation is a real persisted SQLite byte
// change, not a mocked validator or a re-encoded CBOR approximation.
func TestCSFRV18P4LineageGateSharedByReplayAndRecovery(t *testing.T){
 ctx:=context.Background()
 _,parent,_,run,v1,snap,_:=r21EvolvedTriggerSource(t)
 defer parent.Close()
 v2,err:=policy.LoadV2("../../../docs/architecture/canonical-codec-v2-bundle")
 if err!=nil{t.Fatal(err)}
 path:=filepath.Join(t.TempDir(),"published-child.db")
 child,err:=sqlite.Open(ctx,path);if err!=nil{t.Fatal(err)}
 defer child.Close()
 childRun:=[32]byte(run);childRun[0]^=0x34
 if _,err:=parent.ForkIdleSnapshotV1ToV2(ctx,[32]byte(run),snap.ID,childRun,child,v1,v2);err!=nil{
  t.Fatal("P4 healthy cross-policy publish:",err)
 }
 if _,err:=child.ReplayClock(ctx,childRun,v2);err!=nil{t.Fatal("P4 healthy replay:",err)}
 if _,err:=child.RecoverPublishedForkV1ToV2(ctx,childRun,[32]byte(run),snap.ID,v2);err!=nil{
  t.Fatal("P4 healthy committed recovery:",err)
 }
 db,err:=sql.Open("sqlite",path);if err!=nil{t.Fatal(err)}
 defer db.Close()
 type target struct{
  label,table,column,where string
  args []any
 }
 cases:=[]target{
  {"origin-policy","fork_lineage","origin_policy_hash","child_run=?",[]any{childRun[:]}},
  {"sealed-parent-snapshot","fork_lineage","parent_snapshot_canonical","child_run=?",[]any{childRun[:]}},
  {"five-ledger-history","fork_history_records","canonical",
   "rowid=(SELECT rowid FROM fork_history_records WHERE child_run=? ORDER BY ledger_name,ordinal LIMIT 1)",[]any{childRun[:]}},
  {"migrated-trigger-manifest","fork_trigger_imports","canonical","child_run=?",[]any{childRun[:]}},
 }
 for _,tc:=range cases {
  t.Run(tc.label,func(t *testing.T){
   var original []byte
   selectSQL:=fmt.Sprintf("SELECT %s FROM %s WHERE %s",tc.column,tc.table,tc.where)
   if err:=db.QueryRowContext(ctx,selectSQL,tc.args...).Scan(&original);err!=nil{
    t.Fatalf("fixture lacks %s proof: %v",tc.label,err)
   }
   if len(original)==0{t.Fatalf("%s proof unexpectedly empty",tc.label)}
   corrupt:=bytes.Clone(original)
   corrupt[len(corrupt)-1]^=1
   updateSQL:=fmt.Sprintf("UPDATE %s SET %s=? WHERE %s",tc.table,tc.column,tc.where)
   args:=append([]any{corrupt},tc.args...)
   result,err:=db.ExecContext(ctx,updateSQL,args...)
   if err!=nil{t.Fatal(err)}
   affected,err:=result.RowsAffected();if err!=nil||affected!=1{
    t.Fatalf("%s did not mutate exactly one lineage row: %d %v",tc.label,affected,err)
   }
   if _,err:=child.ValidatedGenesis(ctx,childRun,v2);err==nil{
    t.Fatalf("ValidatedGenesis accepted modified %s",tc.label)
   }
   if _,err:=child.ReplayClock(ctx,childRun,v2);err==nil{
    t.Fatalf("ReplayClock bypassed modified %s",tc.label)
   }
   if recovered,err:=child.RecoverPublishedForkV1ToV2(ctx,childRun,[32]byte(run),snap.ID,v2);err==nil||recovered!=nil{
    t.Fatalf("recovery bypassed modified %s: recovered=%v err=%v",tc.label,recovered,err)
   }
   args[0]=original
   result,err=db.ExecContext(ctx,updateSQL,args...)
   if err!=nil{t.Fatal(err)}
   affected,err=result.RowsAffected();if err!=nil||affected!=1{
    t.Fatalf("%s restored %d rows: %v",tc.label,affected,err)
   }
   if _,err:=child.ValidatedGenesis(ctx,childRun,v2);err!=nil{
    t.Fatalf("restored lineage %s rejected: %v",tc.label,err)
   }
   if _,err:=child.ReplayClock(ctx,childRun,v2);err!=nil{
    t.Fatalf("restored replay %s rejected: %v",tc.label,err)
   }
   if _,err:=child.RecoverPublishedForkV1ToV2(ctx,childRun,[32]byte(run),snap.ID,v2);err!=nil{
    t.Fatalf("restored recovery %s rejected: %v",tc.label,err)
   }
  })
 }
 if _,err:=parent.VerifySnapshotV1(ctx,[32]byte(run),snap.ID,v1);err!=nil{
  t.Fatal("P4 read-only verification mutated frozen parent:",err)
 }
}

// The same P4 proof is evaluated while the P3 child SQL transaction remains
// open. A SQL trigger corrupting archived history immediately after INSERT
// must cause the WHOLE child fork to roll back, not publish a Genesis stub.
func TestCSFRV18P4PrecommitHistoryCorruptionRollsBackAndRetries(t *testing.T){
 ctx:=context.Background()
 _,parent,_,run,v1,snap,_:=r21EvolvedTriggerSource(t)
 defer parent.Close()
 v2,err:=policy.LoadV2("../../../docs/architecture/canonical-codec-v2-bundle")
 if err!=nil{t.Fatal(err)}
 path:=filepath.Join(t.TempDir(),"child.db")
 child,err:=sqlite.Open(ctx,path);if err!=nil{t.Fatal(err)}
 defer child.Close()
 db,err:=sql.Open("sqlite",path);if err!=nil{t.Fatal(err)}
 defer db.Close()
 if _,err:=db.ExecContext(ctx,`CREATE TRIGGER p4_mutate_archive
 AFTER INSERT ON fork_history_records
 BEGIN UPDATE fork_history_records SET canonical=x'00' WHERE rowid=NEW.rowid; END;`);err!=nil{t.Fatal(err)}
 id:=[32]byte(run);id[0]^=0x48
 if _,err:=parent.ForkIdleSnapshotV1ToV2(ctx,[32]byte(run),snap.ID,id,child,v1,v2);err==nil{
  t.Fatal("P4 did not reject history poisoned inside uncommitted child SQL")
 }
 assertCSFRV18EmptyRejectedFork(t,path)
 if _,err:=db.ExecContext(ctx,"DROP TRIGGER p4_mutate_archive");err!=nil{t.Fatal(err)}
 if _,err:=parent.ForkIdleSnapshotV1ToV2(ctx,[32]byte(run),snap.ID,id,child,v1,v2);err!=nil{
  t.Fatal("P4 retry of rollback rejected:",err)
 }
 if err:=child.Close();err!=nil{t.Fatal(err)}
 reopened,err:=sqlite.Open(ctx,path);if err!=nil{t.Fatal(err)}
 defer reopened.Close()
 if _,err:=reopened.ReplayClock(ctx,id,v2);err!=nil{t.Fatal("P4 reopened replay after recovery:",err)}
 if _,err:=reopened.RecoverPublishedForkV1ToV2(ctx,id,[32]byte(run),snap.ID,v2);err!=nil{
  t.Fatal("P4 reopened recovery after retry:",err)
 }
}

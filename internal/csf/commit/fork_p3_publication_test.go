package commit

import (
 "context"
 "database/sql"
 "fmt"
 "path/filepath"
 "strings"
 "testing"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
 _ "modernc.org/sqlite"
)

// P3 targets the unconditional publication boundaries; conditional V1->V2
// branches have their own fault matrices in fork_v2_import_test,
// fork_v2_trigger_runtime_test and fork_v2_round_history_test.
// Every injected SQL failure must roll back the ENTIRE installed authority
// inventory and permit a clean retry in the SAME workspace.
func TestCSFRV18P3ChildPublicationRollbackAtEveryBaseInsert(t *testing.T) {
 ctx:=context.Background()
 bundle,err:=policy.Load(bundlePath);if err!=nil{t.Fatal(err)}
 root:=validGenesis(t,bundle)
 parentPath:=filepath.Join(t.TempDir(),"parent.db")
 parent,err:=sqlite.Open(ctx,parentPath);if err!=nil{t.Fatal(err)}
 defer parent.Close()
 if err:=parent.PersistGenesis(ctx,root,bundle);err!=nil{t.Fatal(err)}
 if err:=parent.BootstrapClock(ctx,root,bundle);err!=nil{t.Fatal(err)}
 if err:=parent.BootstrapGenesisTriggers(ctx,root,bundle,nil);err!=nil{t.Fatal(err)}
 snapshot,err:=parent.CreateSnapshotV1(ctx,[32]byte(root.RunID),bundle,nil)
 if err!=nil{t.Fatal(err)}

 // All these tables get an INSERT even for a child without pending work.
 // Others are conditional and are covered by the V2-specific matrices.
 for _,table:=range []string{
  "genesis_records",
  "fork_lineage",
  "clock_states",
  "world_revisions",
  "trigger_registry_seals",
 }{
  t.Run(table,func(t *testing.T){
   childPath:=filepath.Join(t.TempDir(),"child.db")
   child,err:=sqlite.Open(ctx,childPath);if err!=nil{t.Fatal(err)}
   // The same child Store must survive failed bootstrap and accept retry.
   defer child.Close()
   inspector,err:=sql.Open("sqlite",childPath);if err!=nil{t.Fatal(err)}
   defer inspector.Close()
   // Whitelist above is static; interpolation never includes user data.
   query:=fmt.Sprintf(`CREATE TRIGGER p3_abort BEFORE INSERT ON %s
BEGIN SELECT RAISE(ABORT, 'P3 injected insert failure'); END;`,table)
   if _,err:=inspector.ExecContext(ctx,query);err!=nil{t.Fatal(err)}
   childRun:=[32]byte(root.RunID);childRun[0]^=0x5a
   if _,err:=parent.ForkIdleSnapshotV1(ctx,[32]byte(root.RunID),snapshot.ID,childRun,child,bundle);
    err==nil||!strings.Contains(err.Error(),"P3 injected insert failure"){
    t.Fatalf("fault not reached at %s: %v",table,err)
   }
   assertCSFRV18EmptyRejectedFork(t,childPath)
   if _,err:=inspector.ExecContext(ctx,"DROP TRIGGER p3_abort");err!=nil{t.Fatal(err)}
   if _,err:=parent.ForkIdleSnapshotV1(ctx,[32]byte(root.RunID),snapshot.ID,childRun,child,bundle);
    err!=nil{t.Fatalf("clean retry of %s: %v",table,err)}
   if err:=child.Close();err!=nil{t.Fatal(err)}
   reopened,err:=sqlite.Open(ctx,childPath);if err!=nil{t.Fatal(err)}
   defer reopened.Close()
   if _,err:=reopened.ValidatedGenesis(ctx,childRun,bundle);err!=nil{t.Fatalf("reopen after %s: %v",table,err)}
   if _,err:=reopened.ReplayClock(ctx,childRun,bundle);err!=nil{t.Fatalf("replay after %s: %v",table,err)}
  })
 }
 if _,err:=parent.VerifySnapshotV1(ctx,[32]byte(root.RunID),snapshot.ID,bundle);err!=nil{
  t.Fatal("P3 publish tests mutated the immutable parent cut:",err)
 }
}

// The pre-commit validation must reject corruption introduced by SQLite
// itself after INSERT but before COMMIT. The tamper trigger is transactional,
// so both the altered lineage and the earlier Genesis/clock rows disappear.
func TestCSFRV18P3LineageTamperBeforeCommitRollsBackAllAuthorities(t *testing.T){
 ctx:=context.Background()
 b,err:=policy.Load(bundlePath);if err!=nil{t.Fatal(err)}
 g:=validGenesis(t,b)
 parent,err:=sqlite.Open(ctx,filepath.Join(t.TempDir(),"source.db"));if err!=nil{t.Fatal(err)}
 defer parent.Close()
 if err:=parent.PersistGenesis(ctx,g,b);err!=nil{t.Fatal(err)}
 if err:=parent.BootstrapClock(ctx,g,b);err!=nil{t.Fatal(err)}
 if err:=parent.BootstrapGenesisTriggers(ctx,g,b,nil);err!=nil{t.Fatal(err)}
 snap,err:=parent.CreateSnapshotV1(ctx,[32]byte(g.RunID),b,nil);if err!=nil{t.Fatal(err)}
 path:=filepath.Join(t.TempDir(),"target.db")
 child,err:=sqlite.Open(ctx,path);if err!=nil{t.Fatal(err)}
 defer child.Close()
 probe,err:=sql.Open("sqlite",path);if err!=nil{t.Fatal(err)}
 defer probe.Close()
 if _,err:=probe.ExecContext(ctx,`CREATE TRIGGER p3_corrupt_lineage
 AFTER INSERT ON fork_lineage
 BEGIN
 UPDATE fork_lineage SET reference_canonical=x'00'
 WHERE child_run=NEW.child_run;
 END;`);err!=nil{t.Fatal(err)}
 id:=[32]byte(g.RunID);id[0]^=0x61
 if _,err:=parent.ForkIdleSnapshotV1(ctx,[32]byte(g.RunID),snap.ID,id,child,b);err==nil{
  t.Fatal("lineage verification accepted a corrupted uncommitted reference")
 }
 assertCSFRV18EmptyRejectedFork(t,path)
 if _,err:=probe.ExecContext(ctx,"DROP TRIGGER p3_corrupt_lineage");err!=nil{t.Fatal(err)}
 if _,err:=parent.ForkIdleSnapshotV1(ctx,[32]byte(g.RunID),snap.ID,id,child,b);err!=nil{
  t.Fatal("clean retry after failed pre-commit lineage:",err)
 }
 if _,err:=child.ReplayClock(ctx,id,b);err!=nil{t.Fatal("child not replayable after recovery:",err)}
}

// A populated parent ledger must be archived before a child transaction can
// commit. This specifically aborts the history INSERT (not an empty-row case).
func TestCSFRV18P3NonemptyHistoryArchiveFaultRollsBack(t *testing.T){
 ctx:=context.Background()
 _,parent,_,run,v1,snap,_:=r21EvolvedTriggerSource(t)
 v2,err:=policy.LoadV2("../../../docs/architecture/canonical-codec-v2-bundle")
 if err!=nil{t.Fatal(err)}
 childPath:=filepath.Join(t.TempDir(),"history.db")
 child,err:=sqlite.Open(ctx,childPath);if err!=nil{t.Fatal(err)}
 defer child.Close()
 inspector,err:=sql.Open("sqlite",childPath);if err!=nil{t.Fatal(err)}
 defer inspector.Close()
 if _,err:=inspector.ExecContext(ctx,`CREATE TRIGGER p3_fail_history
 BEFORE INSERT ON fork_history_records
 BEGIN SELECT RAISE(ABORT,'P3 history archive interruption'); END;`);err!=nil{t.Fatal(err)}
 childRun:=[32]byte(run);childRun[0]^=0x4b
 if _,err:=parent.ForkIdleSnapshotV1ToV2(ctx,[32]byte(run),snap.ID,childRun,child,v1,v2);
  err==nil||!strings.Contains(err.Error(),"P3 history archive interruption"){
  t.Fatalf("history fault not reached: %v",err)
 }
 assertCSFRV18EmptyRejectedFork(t,childPath)
 if _,err:=inspector.ExecContext(ctx,"DROP TRIGGER p3_fail_history");err!=nil{t.Fatal(err)}
 if _,err:=parent.ForkIdleSnapshotV1ToV2(ctx,[32]byte(run),snap.ID,childRun,child,v1,v2);
  err!=nil{t.Fatal("clean retry after history failure:",err)}
 if _,err:=child.ReplayClock(ctx,childRun,v2);err!=nil{t.Fatal("replay after history failure:",err)}
 if _,err:=parent.VerifySnapshotV1(ctx,[32]byte(run),snap.ID,v1);err!=nil{
  t.Fatal("history retry mutated authenticated parent:",err)
 }
}

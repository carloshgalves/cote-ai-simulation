package commit

import (
 "context"
 "database/sql"
 "testing"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/trigger"
 _ "modernc.org/sqlite"
)

// The fork must publish its root, full trigger registry, migration rows and
// archive together. A crash while inserting a rekeyed activation may not leave
// an apparently usable child with half its causal authorities.
func TestCSFRV18ForkGenesisActivationFailureIsAtomic(t *testing.T) {
 ctx:=context.Background()
 bundle,err:=policy.Load(bundlePath);if err!=nil{t.Fatal(err)}
 root:=validGenesis(t,bundle)
 origin,err:=sqlite.Open(ctx,t.TempDir()+"/origin.db");if err!=nil{t.Fatal(err)}
 defer origin.Close()
 if err:=origin.PersistGenesis(ctx,root,bundle);err!=nil{t.Fatal(err)}
 if err:=origin.BootstrapClock(ctx,root,bundle);err!=nil{t.Fatal(err)}
 definition:=activeTrigger()
 definition.PredicateThreshold=root.InitialInstant
 if err:=origin.BootstrapGenesisTriggers(ctx,root,bundle,[]trigger.Definition{definition});err!=nil{t.Fatal(err)}
 baseline,err:=origin.PendingClock(ctx,[32]byte(root.RunID));if err!=nil{t.Fatal(err)}
 if len(baseline)!=1{t.Fatalf("expected Genesis-owned pending activation, got %d",len(baseline))}
 sealed,err:=origin.CreateSnapshotV1(ctx,[32]byte(root.RunID),bundle,nil);if err!=nil{t.Fatal(err)}
 childPath:=t.TempDir()+"/child.db"
 child,err:=sqlite.Open(ctx,childPath);if err!=nil{t.Fatal(err)}
 defer child.Close()
 direct,err:=sql.Open("sqlite",childPath);if err!=nil{t.Fatal(err)}
 defer direct.Close()
 if _,err:=direct.ExecContext(ctx,`CREATE TRIGGER stop_activation BEFORE INSERT ON trigger_activations
  BEGIN SELECT RAISE(ABORT,'injected child activation crash'); END`);err!=nil{t.Fatal(err)}
 next:=[32]byte(root.RunID);next[0]^=0x28
 if _,err:=origin.ForkIdleSnapshotV1(ctx,[32]byte(root.RunID),sealed.ID,next,child,bundle);err==nil{
  t.Fatal("fork partially committed when activation write crashed")
 }
 for _,table:=range []string{"genesis_records","fork_lineage","fork_history_records",
  "clock_states","world_revisions","trigger_registry_seals","trigger_definitions",
  "trigger_runtime","trigger_activations","scheduled_occurrences"}{
  var count int
  if err:=direct.QueryRowContext(ctx,"SELECT count(*) FROM "+table).Scan(&count);err!=nil{t.Fatal(err)}
  if count!=0{t.Fatalf("failed fork left %d rows in %s",count,table)}
 }
 if _,err:=direct.ExecContext(ctx,"DROP TRIGGER stop_activation");err!=nil{t.Fatal(err)}
 if _,err:=origin.ForkIdleSnapshotV1(ctx,[32]byte(root.RunID),sealed.ID,next,child,bundle);err!=nil{
  t.Fatal("recoverable fork after transactional rollback:",err)
 }
 if _,err:=child.ReplayClock(ctx,next,bundle);err!=nil{t.Fatal(err)}
 if _,err:=origin.VerifySnapshotV1(ctx,[32]byte(root.RunID),sealed.ID,bundle);err!=nil{
  t.Fatal("failed fork changed archived parent:",err)
 }
}

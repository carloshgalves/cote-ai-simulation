package sqlite

import (
 "bytes"
 "context"
 "database/sql"
 "errors"
 "fmt"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/genesis"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

// RecoverPublishedForkV1ToV2 proves that a previously committed V2 child
// was created from this EXACT authenticated parent (run and snapshot ID).
// It is a read-only recovery path for a crash after child SQLite commit but
// before the caller received the Genesis. An unrelated or tampered destination
// never becomes an idempotent success.
func (s *Store) RecoverPublishedForkV1ToV2(ctx context.Context,
 childRun,expectedParentRun,expectedParentSnapshot [32]byte,
 b *policy.Bundle)(*genesis.Genesis,error){
 if s==nil||b==nil{return nil,errors.New("fork recovery needs destination and installed policy")}
 id,version,_,_,err:=b.CodecIdentity()
 if err!=nil||id!=policy.CodecPolicyV2ID||version!=2{
  return nil,errors.New("fork recovery requires installed V2 policy")
 }
 var count int
 if err:=s.db.QueryRowContext(ctx,
  "SELECT count(*) FROM genesis_records").Scan(&count);err!=nil{return nil,err}
 if count==0{return nil,nil} // clean target; caller must bootstrap new fork
 if count!=1{return nil,errors.New("fork recovery destination contains unexpected genesis authorities")}
 var parentRun,parentSnapshot []byte
 err=s.db.QueryRowContext(ctx,
  "SELECT parent_run,parent_snapshot_id FROM fork_lineage WHERE child_run=?",
  childRun[:]).Scan(&parentRun,&parentSnapshot)
 if err==sql.ErrNoRows{return nil,errors.New("fork destination already contains unrelated genesis")}
 if err!=nil{return nil,fmt.Errorf("fork recovery lineage absent or corrupted: %w",err)}
 if !bytes.Equal(parentRun,expectedParentRun[:])||
  !bytes.Equal(parentSnapshot,expectedParentSnapshot[:]){
  return nil,errors.New("existing fork was not born from the requested exact frozen parent cut")
 }
 child,err:=s.ValidatedGenesis(ctx,childRun,b)
 if err!=nil{return nil,fmt.Errorf("existing fork genesis failed independent lineage check: %w",err)}
 if _,err:=s.ReplayClock(ctx,childRun,b);err!=nil{
  return nil,fmt.Errorf("existing fork replay diverged: %w",err)
 }
 return child,nil
}

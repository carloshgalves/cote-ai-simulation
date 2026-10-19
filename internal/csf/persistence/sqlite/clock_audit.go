package sqlite

import (
 "context"
 "fmt"
)

// ClockAudit is read-only evidence of the actual SQLite transactional
// authorities. It is not a shadow store or an alternate publisher.
type ClockAudit struct {
 CommitCount int
 EventCount int
 RuntimeCount int
 ActivationCount int
 RevisionCount int
 InputCount int
}
func (s *Store) AuditClock(ctx context.Context,runID [32]byte)(ClockAudit,error){
 var a ClockAudit
 queries:=[]struct{sql string;dst *int}{
  {"SELECT count(*) FROM committed_cycles WHERE run_id=?",&a.CommitCount},
  {"SELECT count(*) FROM event_store WHERE id IN (SELECT id FROM event_store)",&a.EventCount},
  {"SELECT count(*) FROM trigger_runtime WHERE run_id=?",&a.RuntimeCount},
  {"SELECT count(*) FROM trigger_activations WHERE run_id=?",&a.ActivationCount},
  {"SELECT count(*) FROM world_revisions",&a.RevisionCount},
  {"SELECT count(*) FROM input_ledger",&a.InputCount},
 }
 for i,q:=range queries {
  var err error
  if i==0 || i==2 || i==3 {err=s.db.QueryRowContext(ctx,q.sql,runID[:]).Scan(q.dst)
  } else {err=s.db.QueryRowContext(ctx,q.sql).Scan(q.dst)}
  if err!=nil{return a,fmt.Errorf("audit clock table %d: %w",i,err)}
 }
 return a,nil
}
func (s *Store) ReadCycleCommit(ctx context.Context,id [32]byte)([]byte,error){
 var raw []byte
 err:=s.db.QueryRowContext(ctx,"SELECT canonical FROM decision_ledger WHERE id=?",id[:]).Scan(&raw)
 return raw,err
}
func (s *Store) ReadClockEvent(ctx context.Context,id [32]byte)([]byte,error){
 var raw []byte
 err:=s.db.QueryRowContext(ctx,"SELECT canonical FROM event_store WHERE id=?",id[:]).Scan(&raw)
 return raw,err
}

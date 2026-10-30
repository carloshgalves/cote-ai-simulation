package sqlite

import (
 "bytes"
 "context"
 "errors"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

// VerifyRecoveredTerminal attests the exact terminal returned by a runner
// callback against the independently persisted commit journal. A callback
// cannot satisfy ResumeRunV1 by returning a fabricated nonempty record,
// by writing unrelated state, or by leaving CycleControl in-flight.
func (s *Store) VerifyRecoveredTerminal(ctx context.Context,run,cycle,terminal [32]byte,
 raw []byte,expectedRevision uint64,b *policy.Bundle)error{
 if s==nil||b==nil||len(raw)==0{return errors.New("terminal verification requires persisted record and policy")}
 if _,err:=s.ReplayClock(ctx,run,b);err!=nil{return err}
 var storedID,stored []byte
 if err:=s.db.QueryRowContext(ctx,
  "SELECT c.commit_id,d.canonical FROM committed_cycles c "+
   "JOIN decision_ledger d ON d.id=c.commit_id "+
   "WHERE c.run_id=? AND c.cycle_id=?",run[:],cycle[:]).Scan(&storedID,&stored);err!=nil{return err}
 if !bytes.Equal(storedID,terminal[:])||!bytes.Equal(stored,raw){
  return errors.New("resume handler returned a different or uncommitted CycleCommit")
 }
 value,err:=codec.StrictDecodePayload(stored);if err!=nil{return err}
 fields,ok:=value.([]any)
 if !ok||len(fields)!=24{return errors.New("recovered CycleCommit schema invalid")}
 rawRun,runOK:=fields[0].([]byte);rawCycle,cycleOK:=fields[1].([]byte)
 if !runOK||!cycleOK||!bytes.Equal(rawRun,run[:])||!bytes.Equal(rawCycle,cycle[:]){
  return errors.New("recovered CycleCommit does not match resumed run/cycle")
 }
 committedRevision,ok:=fields[8].(uint64)
 if !ok||committedRevision!=expectedRevision+1{
  return errors.New("recovered terminal advanced wrong world revision")
 }
 current,err:=s.LoadClock(ctx,run)
 if err!=nil{return err}
 if uint64(current.Revision)!=committedRevision{
  return errors.New("committed terminal not reflected by authoritative clock")
 }
 if err:=s.VerifyCycleControl(ctx,run);err!=nil{return err}
 ctl,err:=s.LoadCycleControl(ctx,run)
 if err!=nil{return err}
 if len(ctl)!=7||ctl[0]!=uint64(0){
  return errors.New("recovered terminal did not settle cycle to IDLE")
 }
 return nil
}

package sqlite

import (
 "bytes"
 "context"
 "database/sql"
 "errors"
 "fmt"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/clock"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
)

// The WORK abort/retry path uses the same Store, pinned SQLite runtime and
// serial, FULL/WAL transaction authority as CommitClockAdvance. Neither an
// abort nor a retry publishes a world revision, event or source settlement.

// workFence authenticates the complete pinned fence identity and digest. The
// journal and retry publisher must never accept a caller-selected fence.
func workFence(ctx context.Context,tx *sql.Tx,runID,cycleID,fenceID [32]byte)([]any,error){
 var canonical []byte
 if err:=tx.QueryRowContext(ctx,"SELECT canonical FROM decision_ledger WHERE id=?",fenceID[:]).Scan(&canonical);err!=nil{
  return nil,fmt.Errorf("persisted WORK AdmissionFence required: %w",err)
 }
 decoded,err:=codec.StrictDecodePayload(canonical);if err!=nil{return nil,err}
 fields,ok:=decoded.([]any)
 if !ok||len(fields)!=18||!equalBytes(fields[0],runID[:])||!equalBytes(fields[1],cycleID[:]){
  return nil,errors.New("foreign/malformed WORK AdmissionFence")
 }
 attempt,ok:=fields[2].(uint64);if !ok||attempt>uint64(^uint32(0)){return nil,errors.New("invalid fence attempt")}
 id,err:=registeredDigest("cote.csf.id.admission-fence","cote.csf.schema.admission-fence-id-preimage",
  []any{runID[:],cycleID[:],attempt});if err!=nil{return nil,err}
 digest,err:=registeredDigest("cote.csf.digest.fence","cote.csf.schema.admission-fence.body",fields[:17])
 if err!=nil{return nil,err}
 plan,ok:=fields[6].([]any)
 if id!=fenceID||!equalBytes(fields[17],digest[:])||!ok||len(plan)!=1||plan[0]!=uint64(0){
  return nil,errors.New("unaligned WORK fence identity/digest/plan")
 }
 return fields,nil
}
func exactCurrentClock(ctx context.Context,tx *sql.Tx,runID [32]byte,expected []byte)(clock.State,error){
 var saved []byte
 if err:=tx.QueryRowContext(ctx,"SELECT canonical FROM clock_states WHERE run_id=?",runID[:]).Scan(&saved);err!=nil{return clock.State{},err}
 if !bytes.Equal(saved,expected){return clock.State{},errors.New("clock changed before terminal action")}
 state,err:=clock.Decode(saved);if err!=nil{return state,err}
 var hash []byte
 if err:=tx.QueryRowContext(ctx,"SELECT state_hash FROM world_revisions WHERE revision=?",uint64(state.Revision)).Scan(&hash);err!=nil{return state,err}
 if len(hash)!=32{return state,errors.New("world revision missing state hash")}
 return state,nil
}
func cycleControl(ctx context.Context,tx *sql.Tx,runID [32]byte)([]any,error){
 var raw []byte
 err:=tx.QueryRowContext(ctx,"SELECT canonical FROM cycle_control_state WHERE run_id=?",runID[:]).Scan(&raw)
 if err==sql.ErrNoRows{return nil,nil}
 if err!=nil{return nil,err}
 v,err:=codec.StrictDecodePayload(raw);if err!=nil{return nil,err}
 fields,ok:=v.([]any);if !ok||len(fields)!=7{return nil,errors.New("corrupted CycleControlState")}
 return fields,nil
}
func storeCycleControl(ctx context.Context,tx *sql.Tx,runID [32]byte,fields []any)error{
 raw,err:=codec.CanonicalPayload(fields);if err!=nil{return err}
 _,err=tx.ExecContext(ctx,`INSERT INTO cycle_control_state(run_id,canonical) VALUES(?,?)
 ON CONFLICT(run_id) DO UPDATE SET canonical=excluded.canonical`,runID[:],raw)
 return err
}
func present(x any,want any)bool{
 opt,ok:=x.([]any);return ok&&len(opt)==2&&opt[0]==uint64(1)&&sameCanonical(opt[1],want)
}
func fullRef(kind uint64,id,hash []byte)[]any{return []any{kind,id,hash}}
func noRef()[]any{return []any{uint64(0)}}
func someRef(x any)[]any{return []any{uint64(1),x}}
func rejectTerminalDuplicate(ctx context.Context,tx *sql.Tx,runID,cycleID [32]byte)error{
 var count int
 err:=tx.QueryRowContext(ctx,"SELECT count(*) FROM committed_cycles WHERE run_id=? AND cycle_id=?",runID[:],cycleID[:]).Scan(&count)
 if err!=nil{return err}
 if count!=0{return errors.New("cycle already has authoritative CycleCommit")}
 return nil
}
func (s *Store) CommitCycleAbort(ctx context.Context,b ports.TerminalAbortBatch)error{
 if b.RunID==([32]byte{})||b.CycleID==([32]byte{})||len(b.Envelope.Canonical)==0{return errors.New("incomplete cycle abort")}
 decoded,err:=codec.StrictDecodePayload(b.Envelope.Canonical);if err!=nil{return err}
 body,ok:=decoded.([]any)
 if !ok||len(body)!=15{return errors.New("invalid CycleAbortRecord body")}
 attempt,ok:=body[2].(uint64);if !ok||attempt>uint64(^uint32(0)){return errors.New("invalid abort attempt")}
 desired,err:=registeredDigest("cote.csf.id.cycle-abort","cote.csf.schema.cycle-abort-id-preimage",
  []any{b.RunID[:],b.CycleID[:],attempt})
 if err!=nil{return err}
 dg,err:=registeredDigest("cote.csf.digest.abort","cote.csf.schema.cycle-abort.body",body[:14])
 if err!=nil{return err}
 reason,ok:=body[10].(uint64)
 if desired!=b.Envelope.Identity||!equalBytes(body[14],dg[:])||reason>6||!ok||
  !equalBytes(body[0],b.RunID[:])||!equalBytes(body[1],b.CycleID[:]){
  return errors.New("CycleAbortRecord ID/digest/reason invalid")
 }
 ind,ok:=body[11].([]any);if !ok{return errors.New("abort indeterminate list missing")}
 evidence,ok:=body[12].([]any);if !ok||len(evidence)!=0{return errors.New("unauthenticated abort evidence refs")}
 if reason==0&&len(ind)==0{return errors.New("INDETERMINATE_FACET missing per-subject evidence")}
 for _,v:=range ind{
  fields,ok:=v.([]any)
  if !ok||len(fields)!=9||!sameCanonical(fields[8],[]any{fields[2],fields[3],fields[4]}){
   return errors.New("unbound indeterminate policy evidence")
  }
  subj,ok:=fields[0].([]any);if !ok||len(subj)!=3{return errors.New("invalid indeterminate validation subject")}
 }
 tx,err:=s.db.BeginTx(ctx,nil);if err!=nil{return err}
 defer tx.Rollback()
 state,err:=exactCurrentClock(ctx,tx,b.RunID,b.ExpectedClock);if err!=nil{return err}
 if err:=rejectTerminalDuplicate(ctx,tx,b.RunID,b.CycleID);err!=nil{return err}
 fence,err:=workFence(ctx,tx,b.RunID,b.CycleID,b.FenceID);if err!=nil{return err}
 if fence[2]!=attempt{return errors.New("abort attempt differs from fence")}
 for _,pair:=range [][2]int{{4,3},{5,4},{6,5},{7,6},{8,7},{12,9},{16,13}}{
  if !sameCanonical(fence[pair[0]],body[pair[1]]){return errors.New("abort differs from frozen fence")}
 }
 if !equalBytes(fence[17],body[8].([]byte)){return errors.New("abort fence digest invalid")}
 if state.Revision!=0 && uint64(state.Revision)!=fence[7]{return errors.New("abort base revision stale")}
 if uint64(state.Revision)!=fence[7]{return errors.New("abort revision mismatch")}
 if int64(state.Instant)!=func()int64{n,_:=asSigned64(fence[4]);return n}(){return errors.New("abort instant differs from clock")}
 var baseHash []byte
 if err:=tx.QueryRowContext(ctx,"SELECT state_hash FROM world_revisions WHERE revision=?",uint64(state.Revision)).Scan(&baseHash);err!=nil{return err}
 if !equalBytes(fence[8],baseHash){return errors.New("abort base world hash mismatch")}
 previous,err:=cycleControl(ctx,tx,b.RunID);if err!=nil{return err}
 if attempt==0{
  if previous!=nil&&previous[0]!=uint64(0)&&previous[0]!=uint64(1){return errors.New("run halted or retry already authorized")}
 }else{
  if previous==nil||previous[0]!=uint64(2)||!present(previous[1],b.CycleID[:])||previous[6]!=attempt{
   return errors.New("retry authorization missing for abort attempt")
  }
 }
 if _,err:=tx.ExecContext(ctx,"INSERT INTO decision_ledger(id,canonical) VALUES(?,?)",b.Envelope.Identity[:],b.Envelope.Canonical);err!=nil{return err}
 if b.FailAfterStep==1{return errors.New("injected abort journal failure")}
 ctl:=[]any{uint64(3),someRef(b.CycleID[:]),someRef(attempt),someRef(fullRef(11,b.FenceID[:],body[8].([]byte))),
  noRef(),someRef(fullRef(13,b.Envelope.Identity[:],dg[:])),attempt+1}
 if err:=storeCycleControl(ctx,tx,b.RunID,ctl);err!=nil{return err}
 if b.FailAfterStep==2{return errors.New("injected abort control failure")}
 return tx.Commit()
}
func (s *Store) AuthorizeAttemptRetry(ctx context.Context,b ports.AttemptRetryBatch)error{
 if b.RunID==([32]byte{})||b.CycleID==([32]byte{})||len(b.Record.Canonical)==0{return errors.New("incomplete retry")}
 raw,err:=codec.StrictDecodePayload(b.Record.Canonical);if err!=nil{return err}
 fields,ok:=raw.([]any)
 if !ok||len(fields)!=7||!equalBytes(fields[0],b.RunID[:])||!equalBytes(fields[1],b.CycleID[:]){
  return errors.New("invalid AttemptRetryRecord body")
 }
 from,ok:=fields[2].(uint64);if !ok||from>=uint64(^uint32(0)){return errors.New("invalid retry source")}
 to,ok:=fields[3].(uint64);if !ok||to!=from+1{return errors.New("retry must authorize exactly attempt + 1")}
 expected,err:=registeredDigest("cote.csf.id.attempt-retry","cote.csf.schema.attempt-retry-id-preimage",
  []any{b.RunID[:],b.CycleID[:],from,to})
 if err!=nil{return err}
 if expected!=b.Record.Identity{return errors.New("AttemptRetryRecord ID preimage invalid")}
 reason,ok:=fields[5].(string);if !ok||reason=="" {return errors.New("retry reason missing")}
 tx,err:=s.db.BeginTx(ctx,nil);if err!=nil{return err}
 defer tx.Rollback()
 if _,err:=exactCurrentClock(ctx,tx,b.RunID,b.ExpectedClock);err!=nil{return err}
 if err:=rejectTerminalDuplicate(ctx,tx,b.RunID,b.CycleID);err!=nil{return err}
 aborted,err:=cycleControl(ctx,tx,b.RunID)
 if err!=nil{return err}
 if aborted==nil||aborted[0]!=uint64(3)||!present(aborted[1],b.CycleID[:])||
  !present(aborted[2],from)||aborted[6]!=to{return errors.New("retry not authorized from HALTED_ON_ABORT")}
 var canonical []byte
 if err:=tx.QueryRowContext(ctx,"SELECT canonical FROM decision_ledger WHERE id=?",b.AbortID[:]).Scan(&canonical);err!=nil{return err}
 decoded,err:=codec.StrictDecodePayload(canonical);if err!=nil{return err}
 abort,ok:=decoded.([]any)
 if !ok||len(abort)!=15||abort[2]!=from||!equalBytes(abort[0],b.RunID[:])||
  !equalBytes(abort[1],b.CycleID[:]){return errors.New("retry references a different abort")}
 h,err:=registeredDigest("cote.csf.digest.abort","cote.csf.schema.cycle-abort.body",abort[:14])
 if err!=nil{return err}
 wantRef:=fullRef(13,b.AbortID[:],h[:])
 if !present(fields[4],wantRef)||!present(aborted[5],wantRef){
  return errors.New("retry must cite the exact authoritative abort")
 }
 if !sameCanonical(fields[6],abort[13]){return errors.New("retry rule versions differ from aborted cycle")}
 if _,err:=tx.ExecContext(ctx,"INSERT INTO decision_ledger(id,canonical) VALUES(?,?)",b.Record.Identity[:],b.Record.Canonical);err!=nil{return err}
 if b.FailAfterStep==1{return errors.New("injected retry journal failure")}
 retryDigest,err:=registeredDigest("cote.csf.digest.attempt-retry","cote.csf.schema.attempt-retry",fields)
 if err!=nil{return err}
 next:=[]any{uint64(2),someRef(b.CycleID[:]),someRef(from),aborted[3],
  someRef(fullRef(14,b.Record.Identity[:],retryDigest[:])),aborted[5],to}
 if err:=storeCycleControl(ctx,tx,b.RunID,next);err!=nil{return err}
 if b.FailAfterStep==2{return errors.New("injected retry control failure")}
 return tx.Commit()
}
func (s *Store) LoadCycleControl(ctx context.Context,runID [32]byte)([]any,error){
 tx,err:=s.db.BeginTx(ctx,&sql.TxOptions{ReadOnly:true});if err!=nil{return nil,err}
 defer tx.Rollback()
 fields,err:=cycleControl(ctx,tx,runID);if err!=nil{return nil,err}
 if fields==nil{return []any{uint64(0),noRef(),noRef(),noRef(),noRef(),noRef(),uint64(0)},nil}
 return fields,nil
}

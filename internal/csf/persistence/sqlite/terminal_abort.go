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
// Abort subjects are authenticated against the exact sealed source set or
// canonical candidate records committed in the *same* terminal transaction.
// A standalone candidate digest is insufficient without the source partition.
func verifyAbortSubjects(fence []any,items []any,candidates []ports.CanonicalRecord,run,cycle [32]byte)error{
 admitted,ok:=fence[11].([]any);if !ok{return errors.New("fence admitted unit list missing")}
 sourceDigests:=map[[32]byte][32]byte{}
 for _,v:=range admitted{
  entry,ok:=v.([]any);if !ok||len(entry)!=5{return errors.New("fence unit malformed")}
  idBytes,ok:=entry[3].([]byte);if !ok||len(idBytes)!=32{return errors.New("fence unit ID malformed")}
  digBytes,ok:=entry[4].([]byte);if !ok||len(digBytes)!=32{return errors.New("fence unit digest malformed")}
  var id,digest [32]byte;copy(id[:],idBytes);copy(digest[:],digBytes)
  if _,dup:=sourceDigests[id];dup{return errors.New("duplicate admitted unit root")}
  sourceDigests[id]=digest
 }
 candidateDigests:=map[[32]byte][32]byte{}
 for _,c:=range candidates{
  fields,err:=strictFull(c,17);if err!=nil{return err}
  if !equalBytes(fields[1],run[:])||!equalBytes(fields[2],cycle[:]){
   return errors.New("candidate not derived in the aborted cycle")
  }
  sourceIDs,ok:=fields[3].([]any);if !ok||len(sourceIDs)==0{return errors.New("candidate source set missing")}
  provenance,ok:=fields[13].([]any)
  if !ok||len(provenance)!=len(sourceIDs){return errors.New("candidate lacks exact source provenance")}
  var prior []byte
  for i,v:=range sourceIDs{
   source,ok:=v.([]byte);if !ok||len(source)!=32{return errors.New("candidate source ID malformed")}
   var id [32]byte;copy(id[:],source)
   digest,exists:=sourceDigests[id]
   if !exists{return errors.New("candidate uses unit outside AdmissionFence")}
   ref,ok:=provenance[i].([]any)
   if !ok||len(ref)!=3||!equalBytes(ref[1],source)||!equalBytes(ref[2],digest[:]){
    return errors.New("candidate source provenance digest differs from sealed unit")
   }
   kind,ok:=ref[0].(uint64);if !ok||kind>27{return errors.New("candidate source reference kind invalid")}
   if prior!=nil&&bytes.Compare(prior,source)>=0{return errors.New("candidate sources are not canonical")}
   prior=source
  }
  expected,err:=registeredDigest("cote.csf.id.commit-candidate","cote.csf.schema.candidate-id-preimage",
   []any{fields[1],fields[2],fields[3],fields[4],fields[5],fields[6],fields[7]})
  if err!=nil||expected!=c.Identity{return errors.New("candidate ID preimage does not match source identity")}
  hash,err:=registeredDigest("cote.csf.digest.commit-candidate","cote.csf.schema.commit-candidate.body",fields[:16])
  if err!=nil||!equalBytes(fields[16],hash[:]){return errors.New("candidate body digest invalid")}
  if _,dup:=candidateDigests[c.Identity];dup{return errors.New("duplicate candidate audit root")}
  candidateDigests[c.Identity]=hash
 }
 used:=map[[32]byte]bool{}
 previousKind:=uint64(0)
 var previous [32]byte
 for index,raw:=range items{
  entry,ok:=raw.([]any);if !ok||len(entry)!=8{return errors.New("invalid abort evidence shape")}
  subject,ok:=entry[0].([]any);if !ok||len(subject)!=3{return errors.New("invalid abort subject ref")}
  kind,ok:=subject[0].(uint64);if !ok||kind>1{return errors.New("unknown abort subject kind")}
  idBytes,ok:=subject[1].([]byte);if !ok||len(idBytes)!=32{return errors.New("abort subject ID missing")}
  digestBytes,ok:=subject[2].([]byte);if !ok||len(digestBytes)!=32{return errors.New("abort subject digest missing")}
  var id,digest [32]byte;copy(id[:],idBytes);copy(digest[:],digestBytes)
  if index>0&&(kind<previousKind||(kind==previousKind&&bytes.Compare(id[:],previous[:])<=0)){
   return errors.New("abort subjects unordered or duplicated")
  }
  previousKind=kind;previous=id
  var expected [32]byte;var exists bool
  if kind==0{expected,exists=sourceDigests[id]}else{expected,exists=candidateDigests[id];used[id]=true}
  if !exists||expected!=digest{return errors.New("abort evidence subject not authenticated by fenced source/candidate")}
 }
 if len(used)!=len(candidateDigests){return errors.New("extraneous candidate audit material")}
 return nil
}

func (s *Store) CommitCycleAbort(ctx context.Context,b ports.TerminalAbortBatch)error{
 if len(b.Envelope.Canonical)==0{return errors.New("incomplete cycle abort")}
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
 if reason==6&&len(ind)!=0{return errors.New("CASCADE_LIMIT is not a fabricated validator failure")}
 for _,v:=range ind{
  fields,ok:=v.([]any)
  if !ok||len(fields)!=8||!sameCanonical(fields[7],[]any{fields[2],fields[3],fields[4]}){
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
 if err:=verifyAbortSubjects(fence,ind,b.CandidateEvidence,b.RunID,b.CycleID);err!=nil{return err}
 for _,pair:=range [][2]int{{4,3},{5,4},{6,5},{7,6},{8,7},{12,9},{16,13}}{
  if !sameCanonical(fence[pair[0]],body[pair[1]]){return errors.New("abort differs from frozen fence")}
 }
 if !equalBytes(fence[17],func()[]byte{v,_:=body[8].([]byte);return v}()){return errors.New("abort fence digest invalid")}
 if uint64(state.Revision)!=fence[7]{return errors.New("abort revision mismatch")}
 if int64(state.Instant)!=func()int64{n,_:=asSigned64(fence[4]);return n}(){return errors.New("abort instant differs from clock")}
 var baseHash []byte
 if err:=tx.QueryRowContext(ctx,"SELECT state_hash FROM world_revisions WHERE revision=?",uint64(state.Revision)).Scan(&baseHash);err!=nil{return err}
 if !equalBytes(fence[8],baseHash){return errors.New("abort base world hash mismatch")}
 previous,err:=cycleControl(ctx,tx,b.RunID);if err!=nil{return err}
 if reason==6{
  count,err:=countWorkAt(ctx,tx,b.RunID,int64(state.Instant))
  if err!=nil{return err}
  if !clock.CascadeExceeded(count){
   return errors.New("CASCADE_LIMIT abort claimed before versioned threshold")
  }
 }
 if previous==nil||previous[0]!=uint64(1)||
  !present(previous[1],b.CycleID[:])||!present(previous[2],attempt)||
  !present(previous[3],fullRef(11,b.FenceID[:],body[8].([]byte))){
  return errors.New("abort requires the same persisted in-flight fence")
 }
 if attempt==0{
  if !sameCanonical(previous[4],noRef()){return errors.New("first attempt has forged retry ref")}
 }else if !sameCanonical(previous[4],fence[3]){
  return errors.New("retrial abort lost its retry ref")
 }
 for _,candidate:=range b.CandidateEvidence{
  if _,err:=tx.ExecContext(ctx,"INSERT INTO decision_ledger(id,canonical) VALUES(?,?)",candidate.Identity[:],candidate.Canonical);err!=nil{return err}
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
 if len(b.Record.Canonical)==0{return errors.New("incomplete retry")}
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
 if !sameCanonical(fields[4],wantRef)||!present(aborted[5],wantRef){
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

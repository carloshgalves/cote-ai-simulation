package sqlite

import (
 "bytes"
 "context"
 "errors"
 "math"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/clock"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/reducer"
)

// replayNoProposalCommit is the zero-event WORK fold. Every decision and its
// source/fence link are independently checked by settledNoProposalUnits. The
// fold checks revision, ordinal, world-state transition and journal authority
// instead of interpreting an eventless CycleCommit as CLOCK_ADVANCE.
func (s *Store) replayNoProposalCommit(ctx context.Context,runID,cycleID,commitID [32]byte,
 journal []any,prior clock.State)(clock.State,error){
 if len(journal)!=24||prior.Revision==math.MaxUint64{return clock.State{},errors.New("invalid WORK journal/revision")}
 attempt,ok:=journal[2].(uint64);if !ok||attempt>math.MaxUint32{return clock.State{},errors.New("WORK replay attempt invalid")}
 commit,err:=registeredDigest("cote.csf.id.cycle-commit","cote.csf.schema.cycle-commit-id-preimage",
  []any{runID[:],cycleID[:],attempt});if err!=nil{return clock.State{},err}
 events,err:=loadJournalEvents(ctx,s.db,journal)
 if err!=nil{return clock.State{},err}
 batch,err:=registeredDigest("cote.csf.digest.batch","cote.csf.schema.batch-preimage",
  []any{journal[:23],events});if err!=nil{return clock.State{},err}
 if commit!=commitID||!equalBytes(journal[23],batch[:]){return clock.State{},errors.New("WORK replay journal identity/hash differs")}
 // Read the predecessor outside the SQLite transaction; the store may
 // run with one connection, so a nested s.db query would deadlock here.
 typed,hash,err:=s.LoadWorldState(ctx,uint64(prior.Revision))
 if err!=nil{return clock.State{},err}
 // The terminal journal cannot arbitrarily pick a later round or ordinal.
 // Replay the historical minimum, not today's already-settled projection.
 oldPending,err:=s.replayAdvanceInputs(ctx,runID,uint64(prior.Revision))
 if err!=nil{return clock.State{},err}
 selected,exists,err:=clock.Next(prior,oldPending)
 if err!=nil{return clock.State{},err}
 claimedOrdinal,valid:=journal[4].(uint64)
 if !valid||!exists||selected.Kind!=clock.Work||selected.OriginOrdinal!=domain.CycleOrdinal(claimedOrdinal){
  return clock.State{},errors.New("WORK replay did not select the earliest pending coordinate")
 }
 tx,err:=s.db.BeginTx(ctx,nil);if err!=nil{return clock.State{},err}
 defer tx.Rollback()
 fenceID,err:=registeredDigest("cote.csf.id.admission-fence","cote.csf.schema.admission-fence-id-preimage",
  []any{runID[:],cycleID[:],attempt});if err!=nil{return clock.State{},err}
 fence,err:=workFence(ctx,tx,runID,cycleID,[32]byte(fenceID));if err!=nil{return clock.State{},err}
 if err:=verifyJournalPolicyPins(ctx,tx,runID,fence,journal);err!=nil{return clock.State{},err}
 if !sameCanonical(journal[10],fence[17])||!sameCanonical(journal[12],fence[12])||
  !sameCanonical(journal[11],noProposalIDs(fence))||journal[6]!=uint64(prior.Revision){
  return clock.State{},errors.New("WORK replay fence cut/revision changed")
 }
 if !sameCanonical(journal[5],[]any{uint64(0)})||!sameCanonical(fence[6],[]any{uint64(0)}){
  return clock.State{},errors.New("WORK replay plan malformed")
 }
 timestamp,ok:=asSigned64(journal[3]);if !ok||timestamp!=int64(prior.Instant){
  return clock.State{},errors.New("WORK replay instant differs from clock")
 }
 ordinal,ok:=journal[4].(uint64)
 if !ok||ordinal>math.MaxInt64||int64(ordinal)<=prior.LastOrdinal{
  return clock.State{},errors.New("WORK replay ordinal not strictly forward")
 }
 if fence[5]!=ordinal||fence[7]!=uint64(prior.Revision)||
  !sameCanonical(fence[4],journal[3]){
  return clock.State{},errors.New("WORK replay coordinate differs from frozen fence")
 }
 expectedCycle,err:=registeredDigest("cote.csf.id.cycle","cote.csf.schema.cycle-id-preimage",
  []any{runID[:],[]any{int64(prior.Instant),ordinal}})
 if err!=nil||[32]byte(expectedCycle)!=cycleID{
  return clock.State{},errors.New("WORK replay cycle identity not derived from authoritative coordinate")
 }
 next:=prior
 next.Revision++
 next.LastOrdinal=int64(ordinal)
 if uint64(len(events))>math.MaxUint64-uint64(next.NextLogicalSequence){
  return clock.State{},errors.New("WORK replay logical sequence overflow")
 }
 next.NextLogicalSequence+=domain.LogicalSequence(len(events))
 if journal[8]!=uint64(next.Revision)||journal[22]!=uint64(next.NextLogicalSequence){
  return clock.State{},errors.New("WORK replay successor clock invalid")
 }
 tasks,ok:=journal[18].([]any)
 if !ok||len(tasks)!=0{return clock.State{},errors.New("NoProposal cannot replay visible tasks")}
 resultTyped,resultHash:=typed,hash
 if len(events)>=2{
  first,ok:=events[0].([]byte)
  if !ok{return clock.State{},errors.New("WORK event proof must contain canonical bytes")}
  eventValue,err:=codec.StrictDecodePayload(first);if err!=nil{return clock.State{},err}
  fields,ok:=eventValue.([]any)
  if ok&&len(fields)==19&&fields[3]=="resource.consumed"{
   if len(events)!=2&&len(events)!=3{return clock.State{},errors.New("unsupported resource event batch cardinality")}
   resultTyped,resultHash,err=reducer.Consume(typed,1)
   if err!=nil{return clock.State{},err}
  }
 }
 if !equalBytes(journal[7],hash[:])||!equalBytes(journal[9],resultHash[:]){
  return clock.State{},errors.New("WORK replay WorldState hashes not from registered reducer")
 }
 var raw []byte
 if err:=tx.QueryRowContext(ctx,"SELECT canonical FROM world_revisions WHERE revision=?",uint64(next.Revision)).Scan(&raw);err!=nil{return clock.State{},err}
 want,err:=codec.CanonicalPayload([]any{uint64(prior.Revision),typed,hash[:],
  uint64(next.Revision),resultTyped,resultHash[:]});if err!=nil{return clock.State{},err}
 if !bytes.Equal(raw,want){return clock.State{},errors.New("WORK replay world transition differs from registered reducer")}
 // This verifies every NoProposal input-ref, DecisionRecord identity/hash,
 // ordered decision pair digest, and the exact committed fence root.
 if _,err:=settledNoProposalUnits(ctx,tx,runID);err!=nil{return clock.State{},err}
 if err:=tx.Commit();err!=nil{return clock.State{},err}
 return next,nil
}

// historicalWorkRounds recovers the original pending round set at a replay
// revision. A fully-settled round disappears from the live PendingClock, but
// was pending before its terminal WORK. Its immutable RoundDeclaration and
// committed revision/AdmissionFence reproduce that historical interval.
func (s *Store) historicalWorkRounds(ctx context.Context,runID [32]byte,replayRevision uint64)([]clock.Pending,error){
 type committed struct{cycleID,commitID [32]byte}
 rows,err:=s.db.QueryContext(ctx,"SELECT cycle_id,commit_id FROM committed_cycles WHERE run_id=? ORDER BY rowid",runID[:])
 if err!=nil{return nil,err}
 var commits []committed
 for rows.Next(){
  var a,b []byte
  if err:=rows.Scan(&a,&b);err!=nil{rows.Close();return nil,err}
  if len(a)!=32||len(b)!=32{rows.Close();return nil,errors.New("replay committed cycle key malformed")}
  var pair committed;copy(pair.cycleID[:],a);copy(pair.commitID[:],b);commits=append(commits,pair)
 }
 if err:=rows.Err();err!=nil{rows.Close();return nil,err}
 if err:=rows.Close();err!=nil{return nil,err}
 settledAt:=map[[32]byte]uint64{}
 for _,item:=range commits{
  var raw []byte
  if err:=s.db.QueryRowContext(ctx,"SELECT canonical FROM decision_ledger WHERE id=?",item.commitID[:]).Scan(&raw);err!=nil{return nil,err}
  v,err:=codec.StrictDecodePayload(raw);if err!=nil{return nil,err}
  journal,ok:=v.([]any);if !ok||len(journal)!=24{return nil,errors.New("replay journal invalid")}
  plan,ok:=journal[5].([]any);if !ok||len(plan)==0{return nil,errors.New("replay historical plan missing")}
  if len(plan)==4&&plan[0]==uint64(1){continue}
  if len(plan)!=1||plan[0]!=uint64(0){return nil,errors.New("replay unsupported WORK handler")}
  at,ok:=journal[8].(uint64);if !ok{return nil,errors.New("WORK resulting revision missing")}
  attempt,ok:=journal[2].(uint64);if !ok{return nil,errors.New("WORK attempt missing")}
  fid,err:=registeredDigest("cote.csf.id.admission-fence","cote.csf.schema.admission-fence-id-preimage",
   []any{runID[:],item.cycleID[:],attempt});if err!=nil{return nil,err}
  var fenceRaw []byte
  if err:=s.db.QueryRowContext(ctx,"SELECT canonical FROM decision_ledger WHERE id=?",fid[:]).Scan(&fenceRaw);err!=nil{return nil,err}
  rawFence,err:=codec.StrictDecodePayload(fenceRaw);if err!=nil{return nil,err}
  fence,ok:=rawFence.([]any);if !ok||len(fence)!=18{return nil,errors.New("historical round fence corrupt")}
  cohort,ok:=fence[10].([]any);if !ok||len(cohort)!=2{return nil,errors.New("historical round cohort malformed")}
  rounds,ok:=cohort[1].([]any);if !ok{return nil,errors.New("historical round members missing")}
  for _,v:=range rounds{
   r,ok:=v.([]any);if !ok||len(r)!=2{return nil,errors.New("historical member invalid")}
   id,ok:=r[0].([]byte);if !ok||len(id)!=32{return nil,errors.New("historical round ID invalid")}
   var key [32]byte;copy(key[:],id)
   if _,dup:=settledAt[key];dup{return nil,errors.New("round committed more than once")}
   settledAt[key]=at
  }
 }
 list,err:=s.db.QueryContext(ctx,"SELECT id,canonical FROM input_ledger ORDER BY rowid")
 if err!=nil{return nil,err}
 result:=make([]clock.Pending,0)
 for list.Next(){
  var id,raw []byte
  if err:=list.Scan(&id,&raw);err!=nil{list.Close();return nil,err}
  fieldsAny,err:=codec.StrictDecodePayload(raw);if err!=nil{continue}
  fields,ok:=fieldsAny.([]any)
  if !ok||len(fields)!=11||len(id)!=32||!equalBytes(fields[0],id)||
   !equalBytes(fields[1],runID[:]){continue}
  if _,ok:=fields[3].([]any);!ok{continue}
  if _,ok:=fields[4].([]any);!ok{continue}
  if fields[9]!=uint64(0){continue}
  var key [32]byte;copy(key[:],id)
  if terminal,ok:=settledAt[key];ok&&terminal<=replayRevision{continue}
  coord,ok:=fields[5].([]any);if !ok||len(coord)!=2{list.Close();return nil,errors.New("round eligibility malformed")}
  instant,ok:=asSigned64(coord[0]);if !ok{list.Close();return nil,errors.New("round instant malformed")}
  ordinal,ok:=coord[1].(uint64);if !ok{list.Close();return nil,errors.New("round ordinal malformed")}
  result=append(result,clock.Pending{ID:domain.ID(key),
   Eligibility:domain.EligibilityCoordinate{NotBeforeInstant:domain.SimulationInstant(instant),
    NotBeforeCycleOrdinal:domain.CycleOrdinal(ordinal)}})
 }
 if err:=list.Err();err!=nil{list.Close();return nil,err}
 if err:=list.Close();err!=nil{return nil,err}
 return result,nil
}

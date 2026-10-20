package sqlite

import (
 "bytes"
 "context"
 "errors"
 "math"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/clock"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
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
 batch,err:=registeredDigest("cote.csf.digest.batch","cote.csf.schema.batch-preimage",
  []any{journal[:23],[]any{}});if err!=nil{return clock.State{},err}
 if commit!=commitID||!equalBytes(journal[23],batch[:]){return clock.State{},errors.New("WORK replay journal identity/hash differs")}
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
 next:=prior
 next.Revision++
 next.LastOrdinal=int64(ordinal)
 if journal[8]!=uint64(next.Revision)||journal[22]!=uint64(next.NextLogicalSequence){
  return clock.State{},errors.New("WORK replay successor clock invalid")
 }
 for _,index:=range []int{15,18}{
  items,ok:=journal[index].([]any)
  if !ok||len(items)!=0{return clock.State{},errors.New("NoProposal cannot replay unregistered events/tasks")}
 }
 typed,hash,err:=s.LoadWorldState(ctx,uint64(prior.Revision))
 if err!=nil{return clock.State{},err}
 if !equalBytes(journal[7],hash[:])||!equalBytes(journal[9],hash[:]){
  return clock.State{},errors.New("WORK replay changed world-state hash without a reducer")
 }
 var raw []byte
 if err:=tx.QueryRowContext(ctx,"SELECT canonical FROM world_revisions WHERE revision=?",uint64(next.Revision)).Scan(&raw);err!=nil{return clock.State{},err}
 want,err:=codec.CanonicalPayload([]any{uint64(prior.Revision),typed,hash[:],
  uint64(next.Revision),typed,hash[:]});if err!=nil{return clock.State{},err}
 if !bytes.Equal(raw,want){return clock.State{},errors.New("WORK replay world transition differs from derived zero-effect reducer")}
 // This verifies every NoProposal input-ref, DecisionRecord identity/hash,
 // ordered decision pair digest, and the exact committed fence root.
 if _,err:=settledNoProposalUnits(ctx,tx,runID);err!=nil{return clock.State{},err}
 if err:=tx.Commit();err!=nil{return clock.State{},err}
 return next,nil
}

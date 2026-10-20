package sqlite

import (
 "bytes"
 "context"
 "errors"
 "math"
 "sort"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/clock"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/settlement"
)

// CommitWork is the WORK arm of the SAME pinned SQLite commit authority as
// CommitClockAdvance. Its first registered, non-mutating variant accepts only
// genuine NoProposal slot responses, one terminal decision each, and no event,
// outbox, trigger or reducer effect. Unsupported eventful WORK fails CLOSED.
func (s *Store) CommitWork(ctx context.Context,b ports.WorkBatch)error{
 if b.Mode>3{return errors.New("unregistered WORK terminal variant")}
 if len(b.ExpectedClock)==0||len(b.ResultClock)==0||len(b.Journal.Canonical)==0{
  return errors.New("incomplete WORK publication batch")
 }
 prior,err:=clock.Decode(b.ExpectedClock);if err!=nil{return err}
 next,err:=clock.Decode(b.ResultClock);if err!=nil{return err}
 if uint64(prior.Revision)==math.MaxUint64||
  next.Revision!=prior.Revision+1||next.Instant!=prior.Instant||
  uint64(len(b.Events))>math.MaxUint64-uint64(prior.NextLogicalSequence)||
  uint64(next.NextLogicalSequence)!=uint64(prior.NextLogicalSequence)+uint64(len(b.Events))||
  next.LastOrdinal<0{
  return errors.New("zero-event WORK changes clock or logical sequence illegally")
 }
 raw,err:=codec.StrictDecodePayload(b.Journal.Canonical);if err!=nil{return err}
 journal,ok:=raw.([]any);if !ok||len(journal)!=24{return errors.New("invalid WORK CycleCommit schema")}
 attempt,ok:=journal[2].(uint64);if !ok||attempt>math.MaxUint32{return errors.New("WORK attempt invalid")}
 id,err:=registeredDigest("cote.csf.id.cycle-commit","cote.csf.schema.cycle-commit-id-preimage",
  []any{b.RunID[:],b.CycleID[:],attempt});if err!=nil{return err}
 canonicalEvents:=make([]any,len(b.Events))
 for i,item:=range b.Events{canonicalEvents[i]=item.Canonical}
 batchHash,err:=registeredDigest("cote.csf.digest.batch","cote.csf.schema.batch-preimage",
  []any{journal[:23],canonicalEvents});if err!=nil{return err}
 if id!=b.Journal.Identity||!equalBytes(journal[23],batchHash[:])||
  !equalBytes(journal[0],b.RunID[:])||!equalBytes(journal[1],b.CycleID[:]){
  return errors.New("unauthenticated WORK batch ID/digest/run")
 }
 // The only registered WORK events in this first slice are internal
 // SOURCE_LIFECYCLE (round.consumed) transitions; no domain event or
 // perception task may be forged through this gate.
 eventIDs,ok:=journal[15].([]any)
 if !ok||len(eventIDs)!=len(b.Events){return errors.New("WORK event batch differs from journal")}
 taskIDs,ok:=journal[18].([]any)
 if !ok||len(taskIDs)!=0{return errors.New("unregistered WORK perception tasks")}
 if len(b.Decisions)==0{return errors.New("WORK has no settled source units")}
 tx,err:=s.db.BeginTx(ctx,nil);if err!=nil{return err}
 defer tx.Rollback()
 state,err:=exactCurrentClock(ctx,tx,b.RunID,b.ExpectedClock);if err!=nil{return err}
 if state!=prior{return errors.New("WORK stored clock differs from proposed predecessor")}
 if err:=rejectTerminalDuplicate(ctx,tx,b.RunID,b.CycleID);err!=nil{return err}
 fid,err:=registeredDigest("cote.csf.id.admission-fence","cote.csf.schema.admission-fence-id-preimage",
  []any{b.RunID[:],b.CycleID[:],attempt});if err!=nil{return err}
 if [32]byte(fid)!=b.FenceID{return errors.New("WORK fence ID not derivable from attempt")}
 fence,err:=workFence(ctx,tx,b.RunID,b.CycleID,b.FenceID);if err!=nil{return err}
 if fence[2]!=attempt{return errors.New("WORK attempt differs from persisted fence")}
 if err:=verifyJournalPolicyPins(ctx,tx,b.RunID,fence,journal);err!=nil{return err}
 var genesisRaw []byte
 if err:=tx.QueryRowContext(ctx,"SELECT canonical FROM genesis_records WHERE run_id=?",b.RunID[:]).Scan(&genesisRaw);err!=nil{return err}
 genesisEnv,err:=codec.StrictDecodeEnvelope(genesisRaw);if err!=nil{return err}
 genesisFields,ok:=genesisEnv.Value.([]any)
 if !ok||len(genesisFields)!=18{return errors.New("genesis policy registry malformed")}
 genesisRefs,ok:=genesisFields[14].([]any)
 if !ok{return errors.New("genesis policy refs absent")}
 allowed:=map[string]any{}
 for _,v:=range genesisRefs{
  ref,ok:=v.([]any)
  if !ok||len(ref)!=3{return errors.New("malformed genesis policy ref")}
  name,ok:=ref[0].(string);if !ok{return errors.New("genesis policy name malformed")}
  allowed[name]=ref
 }
 for _,name:=range []string{"validators-v1","conflict-resolvers-v1"}{
  if allowed[name]==nil{return errors.New("NoProposal decision policy unbound by genesis")}
 }
 for _,pair:=range [][2]int{{4,3},{5,4},{6,5},{7,6},{8,7},{12,12}}{
  if !sameCanonical(fence[pair[0]],journal[pair[1]]){return errors.New("WORK journal differs from admission cut")}
 }
 if !sameCanonical(journal[10],fence[17])||!sameCanonical(journal[11],noProposalIDs(fence)){
  return errors.New("WORK journal does not match sealed source membership")
 }
 if !sameCanonical(journal[7],b.BaseStateHash[:])||!sameCanonical(journal[9],func()[]byte{if b.Mode!=0{return b.ResultStateHash[:]}else{return b.BaseStateHash[:]}}())||
  journal[8]!=uint64(next.Revision)||journal[22]!=uint64(next.NextLogicalSequence){
  return errors.New("WORK revision or next sequence not from current state")
 }
 if timestamp,ok:=asSigned64(journal[3]);!ok||timestamp!=int64(prior.Instant)||
  uint64(journal[4].(uint64))!=uint64(next.LastOrdinal){
  return errors.New("WORK coordinate inconsistent with clock")
 }
 if prior.LastOrdinal>=next.LastOrdinal{return errors.New("WORK ordinal must move forward")}
 prefix,err:=tx.QueryContext(ctx,"SELECT id,canonical FROM input_ledger ORDER BY rowid")
 if err!=nil{return err}
 index:=0
 for prefix.Next(){
  var cid,canonical []byte
  if err:=prefix.Scan(&cid,&canonical);err!=nil{prefix.Close();return err}
  if index>=len(b.ExpectedInputPrefix)||!bytes.Equal(cid,b.ExpectedInputPrefix[index].Identity[:])||
   !bytes.Equal(canonical,b.ExpectedInputPrefix[index].Canonical){
   prefix.Close();return errors.New("WORK input prefix changed since fence evaluation")
  }
  index++
 }
 if err:=prefix.Err();err!=nil{prefix.Close();return err}
 if err:=prefix.Close();err!=nil{return err}
 if index!=len(b.ExpectedInputPrefix){return errors.New("WORK input prefix was truncated")}
 pending,err:=pendingClock(ctx,tx,b.RunID);if err!=nil{return err}
 plan,exists,err:=clock.Next(state,pending)
 if err!=nil{return err}
 if !exists||plan.Kind!=clock.Work||int64(plan.From)!=int64(prior.Instant)||
  uint64(plan.OriginOrdinal)!=uint64(next.LastOrdinal){
  return errors.New("WORK source coordinate not selected by canonical clock")
 }
 committed,err:=countWorkAt(ctx,tx,b.RunID,int64(prior.Instant))
 if err!=nil{return err}
 if clock.CascadeExceeded(committed){
  return errors.New("CASCADE_LIMIT: terminal WORK cannot exceed pinned same-instant count; publish CycleAbortRecord")
 }
 cohort,ok:=fence[10].([]any)
 if !ok||len(cohort)!=2{return errors.New("WORK fence cohort missing")}
 rounds,ok:=cohort[1].([]any)
 if !ok{return errors.New("WORK round cohort missing")}
 admittedRounds:=map[domain.ID]bool{}
 for _,v:=range rounds{
  item,ok:=v.([]any);if !ok||len(item)!=2{return errors.New("malformed round cohort item")}
  idBytes,ok:=item[0].([]byte);if !ok||len(idBytes)!=32{return errors.New("round cohort identity invalid")}
  var id domain.ID;copy(id[:],idBytes)
  if admittedRounds[id]{return errors.New("duplicate cohort round")}
  admittedRounds[id]=true
 }
 // An unrelated due occurrence, trigger activation or exogenous input
 // cannot be silently skipped by settling only the visible NoProposal
 // rounds. Until those handlers exist, the whole attempt fails closed.
 due:=domain.EligibilityCoordinate{NotBeforeInstant:prior.Instant,NotBeforeCycleOrdinal:domain.CycleOrdinal(next.LastOrdinal)}
 seenRounds:=map[domain.ID]bool{}
 for _,p:=range pending{
  if p.Eligibility.Compare(due)>0{continue}
  if b.Mode==3{
   sys,ok:=fence[11].([]any)
   if !ok||len(sys)!=1{return errors.New("system WORK source cardinality invalid")}
   entry,ok:=sys[0].([]any);if !ok||len(entry)!=5{return errors.New("system fence unit malformed")}
   source,ok:=entry[3].([]byte)
   if !ok||len(source)!=32||!equalBytes(source,p.ID[:]){return errors.New("system WORK cannot skip another due unit")}
  }else if !admittedRounds[p.ID]{return errors.New("WORK has pending non-slot causal work without a source lifecycle handler")}
  seenRounds[p.ID]=true
 }
 if b.Mode!=3&&len(seenRounds)!=len(admittedRounds){return errors.New("WORK cohort does not equal due authoritative rounds")}
 if b.Mode==3&&len(admittedRounds)!=0{return errors.New("system-only handler cannot omit a due decision round")}
 // A selected RoundDeclaration is indivisible: a subset of its slots
 // cannot become a terminal cycle, since the stranded slot would then
 // remain eligible at a clock ordinal that has already advanced.
 expectedSlots:=map[domain.ID]map[string]bool{}
 for roundID:=range admittedRounds{
  var raw []byte
  if err:=tx.QueryRowContext(ctx,"SELECT canonical FROM input_ledger WHERE id=?",roundID[:]).Scan(&raw);err!=nil{return err}
  parsed,err:=codec.StrictDecodePayload(raw);if err!=nil{return err}
  fields,ok:=parsed.([]any)
  if !ok||len(fields)!=11||!equalBytes(fields[0],roundID[:])||!equalBytes(fields[1],b.RunID[:])||
   fields[9]!=uint64(0){return errors.New("WORK cohort contains non-pending RoundDeclaration")}
  slots,ok:=fields[4].([]any)
  if !ok||len(slots)==0{return errors.New("selected round has no slots")}
  membership:=map[string]bool{}
  for _,v:=range slots{
   slot,ok:=v.(string);if !ok||slot==""||membership[slot]{return errors.New("selected round has malformed or repeated slot")}
   membership[slot]=true
  }
  expectedSlots[roundID]=membership
 }
 var prevHash []byte
 var priorWorld []byte
 if err:=tx.QueryRowContext(ctx,"SELECT state_hash,canonical FROM world_revisions WHERE revision=?",
  uint64(prior.Revision)).Scan(&prevHash,&priorWorld);err!=nil{return err}
 if !bytes.Equal(prevHash,b.BaseStateHash[:]){return errors.New("WORK world predecessor hash mismatch")}
 worldBody,err:=codec.StrictDecodePayload(priorWorld);if err!=nil{return err}
 predecessor,ok:=worldBody.([]any)
 if !ok||len(predecessor)!=6||!sameCanonical(predecessor[5],prevHash){
  return errors.New("invalid authoritative previous world revision")
 }
 rv,err:=codec.StrictDecodePayload(b.RevisionCanonical);if err!=nil{return err}
 revision,ok:=rv.([]any)
 if !ok||len(revision)!=6||revision[0]!=uint64(prior.Revision)||
  revision[3]!=uint64(next.Revision)||!sameCanonical(revision[1],predecessor[4])||
  !sameCanonical(revision[2],prevHash){
  return errors.New("WORK WorldRevision does not match authenticated predecessor")
 }
 if b.Mode==0&&(!sameCanonical(revision[4],predecessor[4])||!sameCanonical(revision[5],prevHash)){
  return errors.New("NoProposal cannot mutate canonical WorldState")
 }
 // Decisions correspond exactly to the authenticated fence units in order.
 units,ok:=fence[11].([]any);if !ok||len(units)!=len(b.Decisions){
  return errors.New("terminal decision count does not biject to admitted sources")
 }
 list,ok:=journal[16].([]any);if !ok||len(list)!=len(units){
  return errors.New("CycleCommit omits terminal decision references")
 }
 if b.Mode==1{
  if err:=verifySingleResourceTerminal(ctx,tx,b,prior,next,fence,journal,predecessor,revision,allowed,expectedSlots);err!=nil{return err}
 }else if b.Mode==3{
  if err:=verifySystemTerminal(ctx,tx,b,prior,next,fence,journal,predecessor,revision,allowed,expectedSlots);err!=nil{return err}
 }else if b.Mode==2{
  if err:=verifyTwoResourceTerminal(ctx,tx,b,prior,next,fence,journal,predecessor,revision,genesisFields,allowed,expectedSlots);err!=nil{return err}
 }else{
 pairing:=make([]any,0,len(units))
 coveredSlots:=map[domain.ID]map[string]bool{}
 type slotProof struct{slot string;id,digest []byte}
 eventSources:=map[domain.ID][]slotProof{}
 for i,unit:=range units{
  source,ok:=unit.([]any)
  if !ok||len(source)!=5||source[1]!=uint64(0){
   return errors.New("unsupported non-slot source in NoProposal WORK")
  }
  sourceID,ok:=source[3].([]byte);if !ok||len(sourceID)!=32{return errors.New("invalid source ID")}
  sourceHash,ok:=source[4].([]byte);if !ok||len(sourceHash)!=32{return errors.New("invalid source digest")}
  var saved []byte
  if err:=tx.QueryRowContext(ctx,"SELECT canonical FROM input_ledger WHERE id=?",sourceID).Scan(&saved);err!=nil{return err}
  sr,err:=codec.StrictDecodePayload(saved);if err!=nil{return err}
  src,ok:=sr.([]any)
  if !ok||len(src)!=11||!equalBytes(src[0],sourceID)||!equalBytes(src[2],b.RunID[:])||
   !equalBytes(src[3],b.CycleID[:])||src[8]!=uint64(prior.Revision){
   return errors.New("WORK source is not an authentic NoProposal")
  }
  roundBytes,ok:=src[4].([]byte);if !ok||len(roundBytes)!=32{return errors.New("NoProposal round root invalid")}
  slot,ok:=src[5].(string);if !ok{return errors.New("NoProposal slot ID invalid")}
  var roundID domain.ID;copy(roundID[:],roundBytes)
  if !expectedSlots[roundID][slot]{
   return errors.New("NoProposal source belongs to no selected round slot")
  }
  if coveredSlots[roundID]==nil{coveredSlots[roundID]=map[string]bool{}}
  if coveredSlots[roundID][slot]{return errors.New("two NoProposal records settle the same round slot")}
  coveredSlots[roundID][slot]=true
  eventSources[roundID]=append(eventSources[roundID],slotProof{slot:slot,id:sourceID,digest:sourceHash})
  responseDigest,err:=registeredDigest("cote.csf.digest.no-proposal-unit",
   "cote.csf.schema.no-proposal.body",[]any{src[:10]});if err!=nil{return err}
  responseID,err:=registeredDigest("cote.csf.id.no-proposal",
   "cote.csf.schema.slot-response-id-preimage",[]any{src[1],uint64(1)});if err!=nil{return err}
  if !bytes.Equal(responseID[:],sourceID)||!bytes.Equal(responseDigest[:],sourceHash)||
   !equalBytes(src[10],sourceHash){
   return errors.New("NoProposal source ID or digest differs from sealed unit")
  }
  dr,err:=strictFull(b.Decisions[i],16);if err!=nil{return err}
  did,err:=registeredDigest("cote.csf.id.decision","cote.csf.schema.decision-id-preimage",
   []any{b.CycleID[:],attempt,sourceID});if err!=nil{return err}
  hash,err:=registeredDigest("cote.csf.digest.decision-record",
   "cote.csf.schema.decision-record.body",dr[:15]);if err!=nil{return err}
  subject:=[]any{uint64(0),uint64(1),sourceID,sourceHash}
  if did!=b.Decisions[i].Identity||!equalBytes(dr[15],hash[:])||
   !equalBytes(dr[1],b.RunID[:])||!equalBytes(dr[2],b.CycleID[:])||dr[3]!=attempt||
   !sameCanonical(dr[4],subject)||dr[5]!=uint64(3)||dr[7]!="slot.no-proposal"{
   return errors.New("invalid terminal NoProposal DecisionRecord")
  }
  if !sameCanonical(dr[9],allowed["validators-v1"])||
   !sameCanonical(dr[10],allowed["conflict-resolvers-v1"]){
   return errors.New("NoProposal decision uses a policy not pinned at genesis")
  }
  for _,opt:=range []int{6,13,14}{
   if !sameCanonical(dr[opt],noRef()){return errors.New("NoProposal may not claim candidate or successor")}
  }
  for _,empty:=range []int{8,11,12}{
   if !sameCanonical(dr[empty],[]any{}){return errors.New("NoProposal cannot settle conflict, RNG or event")}
  }
  if !sameCanonical(list[i],b.Decisions[i].Identity[:]){
   return errors.New("WORK decision IDs not in admitted order")
  }
  pairing=append(pairing,[]any{sourceID,b.Decisions[i].Identity[:],hash[:]})
 }
 for roundID,slots:=range expectedSlots{
  if len(coveredSlots[roundID])!=len(slots){
   return errors.New("terminal WORK cannot settle a strict subset of round slots")
  }
 }
 if len(b.Events)!=len(expectedSlots){
  return errors.New("SOURCE_LIFECYCLE event count differs from terminal rounds")
 }
 roundIDs:=make([]domain.ID,0,len(expectedSlots))
 for id:=range expectedSlots{roundIDs=append(roundIDs,id)}
 sort.Slice(roundIDs,func(i,j int)bool{return bytes.Compare(roundIDs[i][:],roundIDs[j][:])<0})
 for i,roundID:=range roundIDs{
  parts:=eventSources[roundID]
  sort.Slice(parts,func(i,j int)bool{return parts[i].slot<parts[j].slot})
  refs:=make([]any,len(parts))
  for j,v:=range parts{refs[j]=[]any{uint64(0),uint64(1),v.id,v.digest}}
  derived,err:=settlement.ConsumedRoundEvent(domain.ID(b.RunID),domain.ID(b.CycleID),roundID,
   prior.Instant,prior.Revision,prior.NextLogicalSequence+domain.LogicalSequence(i),refs)
  if err!=nil{return err}
  if derived.Identity!=b.Events[i].Identity||!bytes.Equal(derived.Canonical,b.Events[i].Canonical)||
   !sameCanonical(eventIDs[i],derived.Identity[:]){
   return errors.New("SOURCE_LIFECYCLE event differs from sealed round settlement")
  }
 }
 dd,err:=registeredDigest("cote.csf.digest.decision","cote.csf.schema.decision-pair-list",pairing)
 if err!=nil{return err}
 tasks,err:=registeredDigest("cote.csf.digest.perception-task-list","cote.csf.schema.perception-task-id-list",[]any{})
 if err!=nil{return err}
 if !equalBytes(journal[17],dd[:])||!equalBytes(journal[19],tasks[:]){
  return errors.New("WORK decision/task digest not bound to authoritative output")
 }
 }
 control,err:=cycleControl(ctx,tx,b.RunID);if err!=nil{return err}
 if control==nil||control[0]!=uint64(1)||
  !present(control[1],b.CycleID[:])||!present(control[2],attempt)||
  !present(control[3],fullRef(11,b.FenceID[:],fence[17].([]byte))){
  return errors.New("WORK not current in-flight authorized attempt")
 }
 if b.Mode==1{
  if _,err:=tx.ExecContext(ctx,"INSERT INTO decision_ledger(id,canonical) VALUES(?,?)",b.ResourceCandidate.Identity[:],b.ResourceCandidate.Canonical);err!=nil{return err}
 }
 if b.Mode==3{
  if _,err:=tx.ExecContext(ctx,"INSERT INTO decision_ledger(id,canonical) VALUES(?,?)",b.SystemCandidate.Identity[:],b.SystemCandidate.Canonical);err!=nil{return err}
  for _,item:=range b.SystemSuccessors{
   fields,err:=strictFull(item,15);if err!=nil{return err}
   if len(fields)!=15{return errors.New("invalid deferred occurrence successor")}
   eligibility,life,err:=validateScheduledOccurrence(b.RunID,item.Identity,item.Canonical)
   if err!=nil||life!=0{return errors.New("invalid non-pending deferred successor")}
   if _,err:=tx.ExecContext(ctx,"INSERT INTO scheduled_occurrences(id,run_id,eligibility_instant,eligibility_ordinal,lifecycle,canonical) VALUES(?,?,?,?,0,?)",
    item.Identity[:],b.RunID[:],int64(eligibility.NotBeforeInstant),uint64(eligibility.NotBeforeCycleOrdinal),item.Canonical);err!=nil{return err}
  }
 }
 if b.Mode==2{
  for _,proof:=range b.ContestProofs{
   if _,err:=tx.ExecContext(ctx,"INSERT INTO decision_ledger(id,canonical) VALUES(?,?)",proof.Identity[:],proof.Canonical);err!=nil{return err}
  }
 }
 for _,item:=range b.Decisions{
  if _,err:=tx.ExecContext(ctx,"INSERT INTO decision_ledger(id,canonical) VALUES(?,?)",item.Identity[:],item.Canonical);err!=nil{return err}
 }
 if err:=failClock(b.FailAfterStep,1);err!=nil{return err}
 for _,item:=range b.Events{
  if _,err:=strictFull(item,19);err!=nil{return err}
  if err:=appendToTx(ctx,tx,"event_store",item);err!=nil{return err}
 }
 if err:=failClock(b.FailAfterStep,2);err!=nil{return err}
 if _,err:=tx.ExecContext(ctx,"INSERT INTO decision_ledger(id,canonical) VALUES(?,?)",b.Journal.Identity[:],b.Journal.Canonical);err!=nil{return err}
 if err:=failClock(b.FailAfterStep,3);err!=nil{return err}
 outputHash:=b.BaseStateHash
 if b.Mode!=0{outputHash=b.ResultStateHash}
 if _,err:=tx.ExecContext(ctx,"INSERT INTO world_revisions(revision,state_hash,canonical) VALUES(?,?,?)",uint64(next.Revision),outputHash[:],b.RevisionCanonical);err!=nil{return err}
 if err:=failClock(b.FailAfterStep,4);err!=nil{return err}
 updated,err:=tx.ExecContext(ctx,"UPDATE clock_states SET canonical=? WHERE run_id=? AND canonical=?",b.ResultClock,b.RunID[:],b.ExpectedClock)
 if err!=nil{return err}
 n,err:=updated.RowsAffected();if err!=nil||n!=1{return errors.New("WORK clock CAS rejected")}
 if err:=failClock(b.FailAfterStep,5);err!=nil{return err}
 if _,err:=tx.ExecContext(ctx,"INSERT INTO committed_cycles(run_id,cycle_id,commit_id) VALUES(?,?,?)",b.RunID[:],b.CycleID[:],b.Journal.Identity[:]);err!=nil{return err}
 if err:=failClock(b.FailAfterStep,6);err!=nil{return err}
 if err:=storeCycleControl(ctx,tx,b.RunID,[]any{uint64(0),noRef(),noRef(),noRef(),noRef(),noRef(),uint64(0)});err!=nil{return err}
 if err:=failClock(b.FailAfterStep,7);err!=nil{return err}
 return tx.Commit()
}
func noProposalIDs(fence []any)[]any{
 units,ok:=fence[11].([]any);if !ok{return nil}
 out:=make([]any,0,len(units))
 for _,item:=range units{
  fields,ok:=item.([]any);if !ok||len(fields)!=5{return nil}
  out=append(out,fields[3])
 }
 return out
}

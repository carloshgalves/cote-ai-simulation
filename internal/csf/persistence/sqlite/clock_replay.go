package sqlite

import (
 "bytes"
 "context"
 "errors"
 "fmt"
 "sort"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/clock"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/genesis"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/inputledger"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/trigger"
)

type ClockReplayReport struct{
 GenesisID domain.ID
 ReconstructedClock clock.State
 ReplayedCommits int
 ReplayedEvents int
 ReplayedTriggerTransitions int
 VerifiedActivations int
}

// Reconstruct the pending set that motivated the first CLOCK_ADVANCE. No
// WORK or schedule settlement is implemented in CSFRV1-3, so original
// occurrences/rounds/inputs remain durable. Exclude only the activations and
// repeat cadences materialized by the advance itself, not source work.
func (s *Store) replayAdvanceInputs(ctx context.Context,runID [32]byte)([]clock.Pending,error){
 pending,err:=pendingClock(ctx,s.db,runID)
 if err!=nil{return nil,err}
 generated:=map[domain.ID]bool{}
 rows,err:=s.db.QueryContext(ctx,"SELECT id,canonical FROM trigger_activations WHERE run_id=?",runID[:])
 if err!=nil{return nil,err}
 for rows.Next(){
  var id,raw []byte
  if err:=rows.Scan(&id,&raw);err!=nil{rows.Close();return nil,err}
  if len(id)!=32{rows.Close();return nil,errors.New("invalid replay activation ID")}
  value,err:=codec.StrictDecodePayload(raw);if err!=nil{rows.Close();return nil,err}
  fields,ok:=value.([]any);if !ok||len(fields)!=12{rows.Close();return nil,errors.New("invalid replay activation body")}
  origin,ok:=fields[5].([]any);if !ok||len(origin)==0{rows.Close();return nil,errors.New("activation has no causal origin")}
  kind,ok:=origin[0].(uint64);if !ok{rows.Close();return nil,errors.New("invalid activation origin tag")}
  if kind==0 {
   var key domain.ID;copy(key[:],id);generated[key]=true
  }else if kind!=1{rows.Close();return nil,errors.New("unexpected activation origin tag")}
 }
 if err:=rows.Err();err!=nil{rows.Close();return nil,err}
 if err:=rows.Close();err!=nil{return nil,err}
 // A post-advance repeat cadence is likewise not a pre-fence pending unit.
 runtimeRows,err:=s.db.QueryContext(ctx,"SELECT definition_id FROM trigger_runtime WHERE run_id=?",runID[:])
 if err!=nil{return nil,err}
 for runtimeRows.Next(){
  var id []byte
  if err:=runtimeRows.Scan(&id);err!=nil{runtimeRows.Close();return nil,err}
  if len(id)!=32{runtimeRows.Close();return nil,errors.New("invalid replay trigger definition ID")}
  var key domain.ID;copy(key[:],id);generated[key]=true
 }
 if err:=runtimeRows.Err();err!=nil{runtimeRows.Close();return nil,err}
 if err:=runtimeRows.Close();err!=nil{return nil,err}
 original:=make([]clock.Pending,0,len(pending))
 for _,item:=range pending{if !generated[item.ID]{original=append(original,item)}}
 return original,nil
}

// ReplayClock rebuilds temporal state, triggers and activation provenance
// from only durable Genesis/definitions, CycleCommit, event and revision bytes.
// It does not accept predicate callbacks, worker results or caller definitions.
func (s *Store) ReplayClock(ctx context.Context,runID [32]byte,bundle *policy.Bundle)(ClockReplayReport,error){
 var report ClockReplayReport
 if bundle==nil{return report,errors.New("replay requires pinned bundle")}
 graw,err:=s.LoadGenesis(ctx,runID);if err!=nil{return report,err}
 genesisRoot,err:=genesis.ValidateCanonical(graw,bundle);if err!=nil{return report,err}
 if [32]byte(genesisRoot.RunID)!=runID{return report,errors.New("genesis run mismatch")}
 report.GenesisID=genesisRoot.ID
 // CLOCK_ADVANCE in this slice changes logical time, not physical world.
 // All revision hashes must remain anchored to the verified genesis state.
 genesisTyped,ok:=genesisRoot.Payload[16].([]any)
 if !ok||len(genesisTyped)!=4{return report,errors.New("genesis state typed-value malformed")}
 initialStateHash,ok:=genesisTyped[3].([]byte)
 if !ok||len(initialStateHash)!=32{return report,errors.New("genesis state hash malformed")}
 var revZeroHash,revZeroCanonical []byte
 if err:=s.db.QueryRowContext(ctx,"SELECT state_hash,canonical FROM world_revisions WHERE revision=0").Scan(&revZeroHash,&revZeroCanonical);err!=nil{return report,err}
 if !bytes.Equal(revZeroHash,initialStateHash){return report,errors.New("genesis state differs from first durable revision")}
 canonicalZero,err:=codec.CanonicalPayload([]any{uint64(0),genesisTyped,initialStateHash,uint64(0),genesisTyped,initialStateHash})
 if err!=nil{return report,err}
 if !bytes.Equal(revZeroCanonical,canonicalZero){return report,errors.New("genesis revision bytes disagree with typed world state")}
 // Validate the complete durable InputLedger, not just the minimum used
 // for clock selection: identity, source, unit digests, receipt association
 // and gapless source ingress sequences are restored from canonical bytes.
 restored,err:=inputledger.Open(ctx,domain.ID(runID),bundle,s)
 if err!=nil{return report,fmt.Errorf("replay input ledger integrity: %w",err)}
 _,_,_,rounds,_,_:=restored.Snapshot()
 for _,round:=range rounds{
  hash:=identity.OpaqueContentHash(round.Canonical)
  var pinned []byte
  if err:=s.db.QueryRowContext(ctx,"SELECT canonical_hash FROM round_integrity_seals WHERE id=? AND run_id=?",round.ID[:],runID[:]).Scan(&pinned);err!=nil{
   return report,fmt.Errorf("round missing atomic canonical seal: %w",err)
  }
  if !bytes.Equal(pinned,hash[:]){return report,errors.New("replay round actor/slots changed after admission")}
 }
 var seals int
 if err:=s.db.QueryRowContext(ctx,"SELECT count(*) FROM round_integrity_seals WHERE run_id=?",runID[:]).Scan(&seals);err!=nil{return report,err}
 if seals!=len(rounds){return report,errors.New("replay round integrity seal set diverges from declarations")}
 definitions,err:=s.LoadRegisteredTriggers(ctx,runID,bundle);if err!=nil{return report,err}
 current:=clock.State{Instant:genesisRoot.InitialInstant,LastOrdinal:-1}
 runtime:=make(map[domain.ID]trigger.Runtime,len(definitions))
 for _,registered:=range definitions{
  d:=registered.Definition
  seed:=trigger.Runtime{DefinitionID:d.ID,Version:d.Version,Armed:true}
  initial,err:=trigger.Evaluate(d,seed,genesisRoot.InitialInstant,0,trigger.PredicateValue(d,genesisRoot.InitialInstant),nil)
  if err!=nil{return report,err}
  runtime[d.ID]=initial.Runtime
  if initial.Fired{
   activation,err:=genesisActivation(bundle,genesisRoot,d,initial.Runtime);if err!=nil{return report,err}
   var recorded []byte
   if err:=s.db.QueryRowContext(ctx,"SELECT canonical FROM trigger_activations WHERE id=? AND run_id=?",activation.id[:],runID[:]).Scan(&recorded);err!=nil{return report,err}
   if !bytes.Equal(recorded,activation.canonical){return report,errors.New("genesis activation diverges from replay")}
   report.VerifiedActivations++
  }
 }
 rows,err:=s.db.QueryContext(ctx,"SELECT cycle_id,commit_id FROM committed_cycles WHERE run_id=? ORDER BY rowid",runID[:])
 if err!=nil{return report,err}
 type cycleEntry struct{cycleID,commitID [32]byte}
 var entries []cycleEntry
 for rows.Next(){
  var c,id []byte
  if err:=rows.Scan(&c,&id);err!=nil{rows.Close();return report,err}
  if len(c)!=32||len(id)!=32{rows.Close();return report,errors.New("cycle row is malformed")}
  var v cycleEntry
  copy(v.cycleID[:],c);copy(v.commitID[:],id)
  entries=append(entries,v)
 }
 if err:=rows.Err();err!=nil{rows.Close();return report,err}
 if err:=rows.Close();err!=nil{return report,err}
 var lastEventCounter int
 d:=identity.New(bundle)
 for _,entry:=range entries{
  raw,err:=s.ReadCycleCommit(ctx,entry.commitID);if err!=nil{return report,err}
  v,err:=codec.StrictDecodePayload(raw);if err!=nil{return report,err}
  journal,ok:=v.([]any);if !ok||len(journal)!=24{return report,errors.New("replay CycleCommit not canonical")}
  if !equalBytes(journal[0],runID[:])||!equalBytes(journal[1],entry.cycleID[:]){return report,errors.New("replay cycle reference mismatch")}
 if _,err:=verifyCommittedFence(ctx,s.db,runID,entry.cycleID,journal);err!=nil{return report,err}
  planFields,ok:=journal[5].([]any)
  if !ok||len(planFields)!=4||planFields[0]!=uint64(1){return report,errors.New("replay only supports CLOCK_ADVANCE in this slice")}
  from,ok:=asSigned64(planFields[1]);if !ok||from!=int64(current.Instant){return report,errors.New("replay clock origin mismatch")}
  to,ok:=asSigned64(planFields[2]);if !ok||to<=from{return report,errors.New("replay destination invalid")}
  coord,ok:=planFields[3].([]any);if !ok||len(coord)!=2{return report,errors.New("replay target coordinate invalid")}
  targetInstant,ok:=asSigned64(coord[0]);if !ok||targetInstant!=to{return report,errors.New("replay target instant differs")}
  targetOrdinal,ok:=coord[1].(uint64);if !ok{return report,errors.New("replay target ordinal invalid")}
  plan:=clock.Plan{Kind:clock.ClockAdvance,From:current.Instant,To:domain.SimulationInstant(to),OriginOrdinal:domain.CycleOrdinal(current.LastOrdinal+1),
   Target:domain.EligibilityCoordinate{NotBeforeInstant:domain.SimulationInstant(to),NotBeforeCycleOrdinal:domain.CycleOrdinal(targetOrdinal)}}
  // A well-formed journal cannot assert a destination that was not
  // the earliest persisted pending before its terminal advance.
  original,err:=s.replayAdvanceInputs(ctx,runID)
  if err!=nil{return report,err}
  selected,has,err:=clock.Next(current,original)
  if err!=nil{return report,fmt.Errorf("replay has stale pre-advance pending: %w",err)}
  if !has||selected!=plan{return report,errors.New("replay target differs from authoritative pending minimum")}
  next,err:=clock.ApplyAdvance(current,plan);if err!=nil{return report,err}
  eventIDs,ok:=journal[15].([]any);if !ok||len(eventIDs)<2{return report,errors.New("replay temporal event IDs incomplete")}
  events:=make([]ports.CanonicalRecord,0,len(eventIDs))
  triggerEvents:=make([]trigger.Event,0,len(eventIDs))
  for eventIndex,rawID:=range eventIDs{
   id,ok:=rawID.([]byte);if !ok||len(id)!=32{return report,errors.New("invalid replay event id")}
   var eventID [32]byte;copy(eventID[:],id)
   encoded,err:=s.ReadClockEvent(ctx,eventID);if err!=nil{return report,err}
   events=append(events,ports.CanonicalRecord{Identity:eventID,Canonical:encoded})
   ev,err:=codec.StrictDecodePayload(encoded);if err!=nil{return report,err}
   fields,ok:=ev.([]any);if !ok||len(fields)!=19{return report,errors.New("invalid event schema in replay")}
   kind,ok:=fields[3].(string);if !ok{return report,errors.New("replay event kind missing")}
   if eventIndex<2 {
    footprint,err:=trigger.EventFootprint(kind);if err!=nil{return report,err}
    orderKey,err:=codec.CanonicalPayload(fields[2]);if err!=nil{return report,err}
    triggerEvents=append(triggerEvents,trigger.Event{ID:domain.ID(eventID),OrderKey:orderKey,Footprint:footprint})
   }
  }
  next.NextLogicalSequence=current.NextLogicalSequence+domain.LogicalSequence(len(events))
  expectedClock,err:=clock.Encode(current);if err!=nil{return report,err}
  resultClock,err:=clock.Encode(next);if err!=nil{return report,err}
  revBytes:=[]byte{}
  if err:=s.db.QueryRowContext(ctx,"SELECT canonical FROM world_revisions WHERE revision=?",uint64(next.Revision)).Scan(&revBytes);err!=nil{return report,err}
  baseHash,ok:=journal[7].([]byte);if !ok||len(baseHash)!=32{return report,errors.New("replay base hash malformed")}
  resultHash,ok:=journal[9].([]byte);if !ok||len(resultHash)!=32{return report,errors.New("replay resulting hash malformed")}
  if !bytes.Equal(baseHash,initialStateHash)||!bytes.Equal(resultHash,initialStateHash){
   return report,errors.New("CLOCK_ADVANCE changed genesis world state hash")
  }
  var storedRevisionHash []byte
  if err:=s.db.QueryRowContext(ctx,"SELECT state_hash FROM world_revisions WHERE revision=?",uint64(next.Revision)).Scan(&storedRevisionHash);err!=nil{return report,err}
  if !bytes.Equal(storedRevisionHash,initialStateHash){return report,errors.New("world revision state hash conflicts with genesis")}
  var bh,rh [32]byte;copy(bh[:],baseHash);copy(rh[:],resultHash)
  record:=ports.ClockAdvanceBatch{RunID:runID,CycleID:entry.cycleID,ExpectedRevision:uint64(current.Revision),ResultRevision:uint64(next.Revision),
   BaseStateHash:bh,ResultStateHash:rh,ExpectedClock:expectedClock,ResultClock:resultClock,
   Journal:ports.CanonicalRecord{Identity:entry.commitID,Canonical:raw},Events:events,RevisionCanonical:revBytes}
  if err:=verifyAdvanceEnvelope(record,journal,current,next,genesisTyped);err!=nil{return report,fmt.Errorf("replay commit integrity: %w",err)}
  runtimeEvents:=make([]ports.CanonicalRecord,0,len(definitions))
  activationPayloads:=make([]ports.CanonicalRecord,0)
  for _,registered:=range definitions{
   def:=registered.Definition
   old:=runtime[def.ID]
   decision,err:=trigger.Evaluate(def,old,next.Instant,next.Revision,trigger.PredicateValue(def,next.Instant),triggerEvents)
   if err!=nil{return report,err}
   runtime[def.ID]=decision.Runtime
   report.ReplayedTriggerTransitions++
   beforeRaw,err:=trigger.RuntimeBytes(old);if err!=nil{return report,err}
   afterRaw,err:=trigger.RuntimeBytes(decision.Runtime);if err!=nil{return report,err}
   delta,err:=codec.CanonicalPayload([]any{def.ID[:],beforeRaw,afterRaw});if err!=nil{return report,err}
   runtimeID,runtimeEvent,err:=trigger.LifecycleEvent(bundle,domain.ID(runID),domain.ID(entry.cycleID),
    current.Instant,current.Revision,current.NextLogicalSequence+2+domain.LogicalSequence(len(runtimeEvents)),3,def.ID,delta)
   if err!=nil{return report,err}
   runtimeEvents=append(runtimeEvents,ports.CanonicalRecord{Identity:[32]byte(runtimeID),Canonical:runtimeEvent})
   if decision.Fired{
    expected,_,err:=d.Sum("cote.csf.id.trigger-activation","cote.csf.schema.trigger-activation-id-preimage",1,
     []any{runID[:],def.ID[:],uint64(def.Version),uint64(next.Revision),decision.Runtime.ActivationCount})
    if err!=nil{return report,err}
    var rawActivation []byte
    if err:=s.db.QueryRowContext(ctx,"SELECT canonical FROM trigger_activations WHERE id=? AND run_id=?",expected[:],runID[:]).Scan(&rawActivation);err!=nil{return report,err}
    expectedID,expectedBytes,err:=trigger.CycleActivation(bundle,domain.ID(runID),def,decision.Runtime,domain.ID(entry.cycleID),next.Revision,next.Instant,decision.CausingEventIDs)
    if err!=nil{return report,err}
    if expectedID!=domain.ID(expected)||!bytes.Equal(rawActivation,expectedBytes){return report,errors.New("replay activation body differs from deterministic transition")}
    activationPayloads=append(activationPayloads,ports.CanonicalRecord{Identity:[32]byte(expectedID),Canonical:rawActivation})
    decoded,err:=codec.StrictDecodePayload(rawActivation);if err!=nil{return report,err}
    fields,ok:=decoded.([]any);if !ok||len(fields)!=12{return report,errors.New("activation replay schema malformed")}
    origin,ok:=fields[5].([]any);if !ok||len(origin)!=4{return report,errors.New("activation replay origin malformed")}
    if origin[0]!=uint64(0)||!equalBytes(origin[1],entry.cycleID[:])||origin[2]!=uint64(next.Revision){
     return report,errors.New("activation replay origin differs from committed cycle")
    }
    causing,ok:=origin[3].([]any);if !ok||len(causing)!=len(decision.CausingEventIDs){
     return report,errors.New("activation replay cause set diverged")
    }
    for i,id:=range decision.CausingEventIDs{
     if !equalBytes(causing[i],id[:]){return report,errors.New("activation replay cause order diverged")}
    }
    digest,err:=registeredDigest("cote.csf.digest.trigger-activation-unit","cote.csf.schema.trigger-activation.body",fields[:11]);if err!=nil{return report,err}
    if !equalBytes(fields[11],digest[:]){return report,errors.New("replay activation digest mismatch")}
    report.VerifiedActivations++
   }
  }
  // Reconstruct both lifecycle phases rather than trusting self-hashed
  // persisted records; every runtime revision and activation is an event.
  sort.Slice(activationPayloads,func(i,j int)bool{return bytes.Compare(activationPayloads[i].Identity[:],activationPayloads[j].Identity[:])<0})
  lifecycles:=append([]ports.CanonicalRecord(nil),runtimeEvents...)
  for _,activation:=range activationPayloads{
   eventID,raw,err:=trigger.LifecycleEvent(bundle,domain.ID(runID),domain.ID(entry.cycleID),
    current.Instant,current.Revision,current.NextLogicalSequence+2+domain.LogicalSequence(len(lifecycles)),4,domain.ID(activation.Identity),activation.Canonical)
   if err!=nil{return report,err}
   lifecycles=append(lifecycles,ports.CanonicalRecord{Identity:[32]byte(eventID),Canonical:raw})
  }
  if len(events)!=2+len(lifecycles){return report,errors.New("replay lifecycle event set incomplete")}
  for i,expected:=range lifecycles{
   actual:=events[2+i]
   if actual.Identity!=expected.Identity||!bytes.Equal(actual.Canonical,expected.Canonical){return report,errors.New("replay lifecycle event differs from causal trigger transition")}
  }
  current=next
  report.ReplayedCommits++
  lastEventCounter+=len(events)
 }
 report.ReplayedEvents=lastEventCounter
 // Validate every indexed pending record, including rows whose projected
 // lifecycle was changed to hide an otherwise durable pending activation.
 if _,err:=pendingClock(ctx,s.db,runID);err!=nil{return report,err}
 persisted,err:=s.LoadClock(ctx,runID);if err!=nil{return report,err}
 if current!=persisted{return report,errors.New("replayed clock differs from durable clock")}
 report.ReconstructedClock=current
 for _,registered:=range definitions{
  actual,err:=trigger.RuntimeBytes(runtime[registered.Definition.ID]);if err!=nil{return report,err}
  if !bytes.Equal(actual,registered.RuntimeCanonical){return report,errors.New("replayed trigger runtime differs from stored state")}
 }
 var count int
 if err:=s.db.QueryRowContext(ctx,"SELECT count(*) FROM trigger_activations WHERE run_id=?",runID[:]).Scan(&count);err!=nil{return report,err}
 if count!=report.VerifiedActivations{return report,errors.New("unreplayable extra trigger activation")}
 // All temporal events in this slice belong to replayed CycleCommits.
 if err:=s.db.QueryRowContext(ctx,"SELECT count(*) FROM event_store").Scan(&count);err!=nil{return report,err}
 if count!=report.ReplayedEvents{return report,errors.New("event store diverged from CycleCommit history")}
 // WorldRevision is an authoritative ledger: every row must have exactly
 // one terminal CycleCommit, with revision zero as the sole genesis row.
 // Checking only the revisions referenced by known commits misses orphans.
 revisions,err:=s.db.QueryContext(ctx,"SELECT revision FROM world_revisions ORDER BY revision")
 if err!=nil{return report,err}
 var expectedRevision uint64
 for revisions.Next(){
  var revision int64
  if err:=revisions.Scan(&revision);err!=nil{revisions.Close();return report,err}
  if revision<0||uint64(revision)!=expectedRevision{
   revisions.Close();return report,errors.New("world revisions contain a missing or orphaned causal revision")
  }
  expectedRevision++
 }
 if err:=revisions.Err();err!=nil{revisions.Close();return report,err}
 if err:=revisions.Close();err!=nil{return report,err}
 if expectedRevision!=uint64(current.Revision)+1{
  return report,errors.New("world revision count differs from replayed CycleCommit history")
 }
 return report,nil
}

func sortedIDs(rows []trigger.Event)[]domain.ID{
 result:=append([]trigger.Event(nil),rows...)
 sort.Slice(result,func(i,j int)bool{return bytes.Compare(result[i].OrderKey,result[j].OrderKey)<0})
 ids:=make([]domain.ID,len(result));for i,r:=range result{ids[i]=r.ID}
 return ids
}

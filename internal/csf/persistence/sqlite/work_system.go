package sqlite

import (
 "bytes"
 "context"
 "errors"
 "math"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/clock"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/settlement"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/trigger"
)

func verifySystemTerminal(ctx context.Context,q queryOne,b ports.WorkBatch,
 prior,next clock.State,fence,journal,predecessor,revision []any,
 allowed map[string]any,expectedSlots map[domain.ID]map[string]bool)error{
 if len(expectedSlots)!=0||len(b.Decisions)!=1||len(b.Events)!=1||
  len(b.SystemSuccessors)>1||len(b.ContestProofs)!=0{
  return errors.New("system WORK does not completely partition its one source")
 }
 units,ok:=fence[11].([]any)
 if !ok||len(units)!=1{return errors.New("system fence must admit exactly one unit")}
 unit,ok:=units[0].([]any)
 if !ok||len(unit)!=5{return errors.New("system admitted unit malformed")}
 kind,ok:=unit[1].(uint64)
 if !ok||(kind!=2&&kind!=3){return errors.New("system handler only accepts occurrence or trigger")}
 id,ok:=unit[3].([]byte);if !ok||len(id)!=32{return errors.New("system source identity invalid")}
 digest,ok:=unit[4].([]byte);if !ok||len(digest)!=32{return errors.New("system source digest invalid")}
 var source domain.ID;copy(source[:],id)
 var sourceDigest domain.Digest;copy(sourceDigest[:],digest)
 table:="scheduled_occurrences"
 if kind==3{table="trigger_activations"}
 var raw []byte
 var life,at,ordinal int64
 if err:=q.QueryRowContext(ctx,"SELECT canonical,lifecycle,eligibility_instant,eligibility_ordinal FROM "+table+
  " WHERE run_id=? AND id=?",b.RunID[:],id).Scan(&raw,&life,&at,&ordinal);err!=nil{return err}
 if life!=0||ordinal<0{return errors.New("system unit not pending at terminal commit")}
 var eligibility domain.EligibilityCoordinate
 var computedLife uint64
 var err error
 // The source identity comes from the fence, never the candidate.
 if kind==2{eligibility,computedLife,err=validateScheduledOccurrence(b.RunID,[32]byte(source),raw)
 }else{eligibility,computedLife,err=validateTriggerActivation(b.RunID,[32]byte(source),raw)}
 if err!=nil||computedLife!=0||int64(eligibility.NotBeforeInstant)!=at||
  uint64(eligibility.NotBeforeCycleOrdinal)!=uint64(ordinal){
  return errors.New("system source lifecycle/coordinate not authentic")
 }
 decoded,err:=codec.StrictDecodePayload(raw);if err!=nil{return err}
 fields,ok:=decoded.([]any)
 if !ok||!equalBytes(fields[0],id)||!equalBytes(fields[1],b.RunID[:])||
  !equalBytes(fields[len(fields)-1],digest)||fields[2]!=unit[2]||
  !sameCanonical(unit[0],[]any{int64(eligibility.NotBeforeInstant),uint64(eligibility.NotBeforeCycleOrdinal)}){
  return errors.New("fence system-unit fields differ from indexed source")
 }
 var disposition uint64
 var reason,producer string
 var due int64
 var recurrence any
 if kind==2{
  disposition,due,recurrence,err=settlement.OccurrenceOutcome(domain.ID(b.RunID),fields,prior.Instant)
  if err!=nil{return err}
  producer="schedule.lifecycle"
  if disposition==1{reason="schedule.rejected"}else{reason="schedule.deferred"}
 }else{
  if len(fields)!=12{return errors.New("invalid trigger source arity")}
  definitionID,ok:=fields[3].([]byte);if !ok||len(definitionID)!=32{return errors.New("trigger definition ID missing")}
  var definitionBytes []byte
  if err:=q.QueryRowContext(ctx,"SELECT canonical FROM trigger_definitions WHERE run_id=? AND definition_id=?",
   b.RunID[:],definitionID).Scan(&definitionBytes);err!=nil{return err}
  policy,ok:=allowed["dependency-footprints-v1"].([]any)
  if !ok{return errors.New("trigger handler policy missing from genesis")}
  registered,handler,err:=trigger.DecodeDefinition(definitionBytes,[][]any{policy},nil)
  if err!=nil||!sameCanonical(handler,policy)||!equalBytes(registered.ID[:],definitionID)||
   fields[4]!=uint64(registered.Version){
   return errors.New("trigger handler differs from pinned definition")
  }
  var runtimeBytes []byte
  if err:=q.QueryRowContext(ctx,"SELECT canonical FROM trigger_runtime WHERE run_id=? AND definition_id=?",
   b.RunID[:],definitionID).Scan(&runtimeBytes);err!=nil{return err}
  state,err:=trigger.DecodeRuntime(runtimeBytes)
  if err!=nil{return err}
  count,ok:=fields[7].(uint64)
  if !ok||state.ActivationCount<count{return errors.New("trigger activation exceeds pinned runtime")}
  disposition=0;reason="trigger.consumed";producer="trigger.lifecycle"
 }
 event,err:=settlement.SystemLifecycleEvent(domain.ID(b.RunID),domain.ID(b.CycleID),
  source,sourceDigest,kind,disposition,prior.Instant,prior.Revision,prior.NextLogicalSequence)
 if err!=nil{return err}
 if event.Identity!=b.Events[0].Identity||!bytes.Equal(event.Canonical,b.Events[0].Canonical)||
  !sameCanonical(journal[15],[]any{event.Identity[:]}){return errors.New("system lifecycle Event not reproducible")}
 successorRef:=[]any{uint64(0)}
 if kind==2&&disposition==2{
  if len(b.SystemSuccessors)!=1{return errors.New("deferred occurrence missing successor")}
  successor,err:=settlement.OccurrenceSuccessor(domain.ID(b.RunID),fields,
   prior.Instant,event,due,recurrence)
  if err!=nil{return err}
  if successor.Identity!=b.SystemSuccessors[0].Identity||
   !bytes.Equal(successor.Canonical,b.SystemSuccessors[0].Canonical){
   return errors.New("deferred occurrence successor not derived from committed lifecycle event")
  }
  successorRef=[]any{uint64(1),successor.Identity[:]}
 }else if len(b.SystemSuccessors)!=0{return errors.New("terminal source invented a successor")}
 sourceRef:=[]any{kind,id,digest}
 candidate,err:=strictFull(b.SystemCandidate,17);if err!=nil{return err}
 expectedCandidate,err:=registeredDigest("cote.csf.id.commit-candidate",
  "cote.csf.schema.candidate-id-preimage",
  []any{b.RunID[:],b.CycleID[:],[]any{id},producer,uint64(1),uint64(0),uint64(0)})
 if err!=nil{return err}
 eventValue,err:=codec.StrictDecodePayload(event.Canonical);if err!=nil{return err}
 payload:=eventValue.([]any)[9]
 if expectedCandidate!=b.SystemCandidate.Identity||!equalBytes(candidate[0],expectedCandidate[:])||
  !equalBytes(candidate[1],b.RunID[:])||!equalBytes(candidate[2],b.CycleID[:])||
  !sameCanonical(candidate[3],[]any{id})||candidate[4]!=producer||
  candidate[5]!=uint64(1)||candidate[6]!=uint64(0)||candidate[7]!=uint64(0)||
  !sameCanonical(candidate[8],[]any{[]any{uint64(0),uint64(0),reason,payload,[]any{}}})||
  !sameCanonical(candidate[9],[]any{})||!sameCanonical(candidate[10],[]any{})||
  !sameCanonical(candidate[11],[]any{})||!sameCanonical(candidate[12],[]any{"source.lifecycle"})||
  !sameCanonical(candidate[13],[]any{sourceRef})||
  !sameCanonical(candidate[14],uint64(0))||
  !sameCanonical(candidate[15],allowed["conflict-resolvers-v1"]){
  return errors.New("system candidate contains unsupported source, policy or handler output")
 }
 ch,err:=registeredDigest("cote.csf.digest.commit-candidate","cote.csf.schema.commit-candidate.body",candidate[:16])
 if err!=nil||!equalBytes(candidate[16],ch[:]){return errors.New("system candidate digest invalid")}
 record,err:=strictFull(b.Decisions[0],16);if err!=nil{return err}
 decisionID,err:=registeredDigest("cote.csf.id.decision","cote.csf.schema.decision-id-preimage",
  []any{b.CycleID[:],journal[2],id});if err!=nil{return err}
 recordHash,err:=registeredDigest("cote.csf.digest.decision-record","cote.csf.schema.decision-record.body",record[:15])
 if err!=nil{return err}
 expectedFields:=[]any{decisionID[:],b.RunID[:],b.CycleID[:],journal[2],sourceRef,
  disposition,[]any{uint64(1),expectedCandidate[:]},reason,[]any{},
  allowed["validators-v1"],allowed["conflict-resolvers-v1"],[]any{},
  []any{event.Identity[:]},successorRef,noRef()}
 if !sameCanonical(record[:15],expectedFields)||!equalBytes(record[15],recordHash[:])||
  b.Decisions[0].Identity!=[32]byte(decisionID)||
  !sameCanonical(journal[16],[]any{decisionID[:]}){return errors.New("system DecisionRecord does not prove exact disposition")}
 pair,err:=registeredDigest("cote.csf.digest.decision","cote.csf.schema.decision-pair-list",
  []any{[]any{id,decisionID[:],recordHash[:]}});if err!=nil{return err}
 task,err:=registeredDigest("cote.csf.digest.perception-task-list",
  "cote.csf.schema.perception-task-id-list",journal[18]);if err!=nil{return err}
 if !equalBytes(journal[17],pair[:])||!equalBytes(journal[19],task[:]){
  return errors.New("system decision/task digest does not match journal")
 }
 if (b.Mode!=5&&(!sameCanonical(revision[4],predecessor[4])||
  !sameCanonical(revision[5],predecessor[5])||
  !sameCanonical(journal[9],predecessor[5])))||
  uint64(prior.NextLogicalSequence)==math.MaxUint64||
  next.NextLogicalSequence!=prior.NextLogicalSequence+1{
  return errors.New("source lifecycle changed world outside resource reducer or sequence")
 }
 return nil
}

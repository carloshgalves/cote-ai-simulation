package commit

import (
 "bytes"
 "context"
 "errors"
 "math"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/admission"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/clock"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/communication"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/inputledger"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/settlement"
)

// CommitCommunicationSend is a registered one-ActionProposal WORK terminal.
// The entire action, two Events, Claim/Transmission, due ScheduledOccurrence,
// decision, round settlement, outbox, world revision and clock are committed
// only by the shared SQLite CommitWork authority. Never append these records
// independently. Unsupported mixed cohorts fail closed in this first slice.
func (c *Coordinator) CommitCommunicationSend(ctx context.Context,ledger *inputledger.Ledger,
 cycle admission.Cycle,fence admission.Fence)(ports.CanonicalRecord,error){
 var zero ports.CanonicalRecord
 if c==nil||c.bundle==nil||c.store==nil||ledger==nil||!ledger.SharesFenceAuthority(c.store){
  return zero,errors.New("communication send requires one authoritative ledger/commit coordinator")
 }
 view,prefix,err:=ledger.AdmissionView(ctx,c.bundle);if err!=nil{return zero,err}
 if err:=admission.VerifyFence(c.bundle,view,cycle,c.admissionPolicies,fence);err!=nil{return zero,err}
 stored,err:=c.store.GetFence(ctx,[32]byte(fence.ID))
 if err!=nil||!bytes.Equal(stored,fence.Canonical){return zero,errors.New("communication fence differs from durable authority")}
 _,_,_,rounds,slots,run:=view.Snapshot()
 state,err:=c.store.LoadClock(ctx,[32]byte(run));if err!=nil{return zero,err}
 if state.Revision!=cycle.BaseRevision||state.Instant!=cycle.Instant||
  state.LastOrdinal>=int64(cycle.Ordinal)||cycle.Ordinal>math.MaxInt64||
  uint64(state.Revision)==math.MaxUint64||uint64(state.NextLogicalSequence)>math.MaxUint64-2{
  return zero,errors.New("stale communication send cycle or logical sequence overflow")
 }
 if abort,hit,err:=c.abortIfCascadeExceeded(ctx,run,cycle,fence);hit||err!=nil{return abort,err}
 before,hash,err:=c.store.LoadWorldState(ctx,uint64(state.Revision))
 if err!=nil||hash!=cycle.BaseStateHash{return zero,errors.New("communication base world hash differs from fence")}
 units,ok:=fence.Fields[11].([]any)
 if !ok||len(units)!=1{return zero,errors.New("communication send requires exactly one fenced ActionProposal")}
 entry,ok:=units[0].([]any)
 if !ok||len(entry)!=5||entry[1]!=uint64(0){return zero,errors.New("communication send cannot skip system/other units")}
 source,ok:=entry[3].([]byte)
 if !ok||len(source)!=32{return zero,errors.New("fenced communication source malformed")}
 sourceDigest,ok:=entry[4].([]byte)
 if !ok||len(sourceDigest)!=32{return zero,errors.New("fenced source digest missing")}
 var response inputledger.Response
 seen:=0
 for _,round:=range rounds{
  for _,v:=range slots[round.ID]{
   if bytes.Equal(v.ID[:],source){response=v;seen++}
  }
 }
 if seen!=1||response.Kind!=0||!bytes.Equal(response.Digest[:],sourceDigest){
  return zero,errors.New("communication proposal not authenticated by frozen slot")
 }
 authored,err:=communication.AuthoredAction(c.bundle,ports.CanonicalRecord{
  Identity:[32]byte(response.ID),Canonical:response.Canonical})
 if err!=nil{return zero,err}
 if authored.Cycle!=cycle.ID||authored.Run!=run||authored.Actor!=communication.Actor(response.Actor)||
  authored.Round!=response.RoundID||authored.Slot!=response.SlotID||
  authored.Revision!=state.Revision||authored.Spec.SentAt!=state.Instant{
  return zero,errors.New("communication ActionProposal owner, base or cycle differs from fence")
 }
 plan,err:=communication.PrepareSend(c.bundle,authored.Spec);if err!=nil{return zero,err}
 d:=identity.New(c.bundle)
 sourceOrigin:=authored.Source
 sent,err:=communication.NewSentEvent(c.bundle,communication.EventContext{
  Run:run,Cycle:cycle.ID,At:state.Instant,BaseRevision:state.Revision,
  Sequence:state.NextLogicalSequence},plan,sourceOrigin)
 if err!=nil{return zero,err}
 refs:=[]any{[]any{uint64(0),uint64(0),response.ID[:],response.Digest[:]}}
 roundEvent,err:=settlement.ConsumedRoundEvent(run,cycle.ID,response.RoundID,
  state.Instant,state.Revision,state.NextLogicalSequence+1,refs)
 if err!=nil{return zero,err}
 events:=[]ports.CanonicalRecord{sent,roundEvent}
 policies,err:=policiesByRole(c.bundle);if err!=nil{return zero,err}
 sourceRef:=[]any{uint64(5),response.ID[:],response.Digest[:]}
 candidateID,_,err:=d.Sum("cote.csf.id.commit-candidate",
  "cote.csf.schema.candidate-id-preimage",1,
  []any{run[:],cycle.ID[:],[]any{response.ID[:]},"communication.send",uint64(1),uint64(0),uint64(0)})
 if err!=nil{return zero,err}
 sentPayload,err:=codec.StrictDecodePayload(sent.Canonical);if err!=nil{return zero,err}
 eventFields:=sentPayload.([]any)
 candidateFields:=[]any{candidateID[:],run[:],cycle.ID[:],[]any{response.ID[:]},
  "communication.send",uint64(1),uint64(0),uint64(0),
  []any{[]any{uint64(0),uint64(0),"communication.sent",eventFields[9],[]any{}}},
  []any{},[]any{},[]any{},[]any{"communication.authorized"},
  []any{sourceRef},int64(0),policies["conflict-resolvers"]}
 candidateDigest,_,err:=d.Sum("cote.csf.digest.commit-candidate",
  "cote.csf.schema.commit-candidate.body",1,candidateFields)
 if err!=nil{return zero,err}
 candidateRaw,err:=codec.CanonicalPayload(append(cloneAny(candidateFields),candidateDigest[:]))
 if err!=nil{return zero,err}
 candidate:=ports.CanonicalRecord{Identity:[32]byte(candidateID),Canonical:candidateRaw}
 decisionID,_,err:=d.Sum("cote.csf.id.decision","cote.csf.schema.decision-id-preimage",1,
  []any{cycle.ID[:],uint64(cycle.Attempt),response.ID[:]})
 if err!=nil{return zero,err}
 decisionFields:=[]any{decisionID[:],run[:],cycle.ID[:],uint64(cycle.Attempt),
  []any{uint64(0),uint64(0),response.ID[:],response.Digest[:]},
  uint64(0),[]any{uint64(1),candidateID[:]},"communication.sent",[]any{},
  policies["validators"],policies["conflict-resolvers"],[]any{},
  []any{sent.Identity[:]},[]any{uint64(0)},[]any{uint64(0)}}
 decisionDigest,_,err:=d.Sum("cote.csf.digest.decision-record",
  "cote.csf.schema.decision-record.body",1,decisionFields)
 if err!=nil{return zero,err}
 decisionRaw,err:=codec.CanonicalPayload(append(cloneAny(decisionFields),decisionDigest[:]))
 if err!=nil{return zero,err}
 decision:=ports.CanonicalRecord{Identity:[32]byte(decisionID),Canonical:decisionRaw}
 pairs:=[]any{[]any{response.ID[:],decisionID[:],decisionDigest[:]}}
 pairDigest,_,err:=d.Sum("cote.csf.digest.decision","cote.csf.schema.decision-pair-list",1,pairs)
 if err!=nil{return zero,err}
 taskDigest,_,err:=d.Sum("cote.csf.digest.perception-task-list",
  "cote.csf.schema.perception-task-id-list",1,[]any{})
 if err!=nil{return zero,err}
 next:=state;next.Revision++;next.LastOrdinal=int64(cycle.Ordinal);next.NextLogicalSequence+=2
 oldClock,err:=clock.Encode(state);if err!=nil{return zero,err}
 afterClock,err:=clock.Encode(next);if err!=nil{return zero,err}
 revision,err:=codec.CanonicalPayload([]any{uint64(state.Revision),before,hash[:],
  uint64(next.Revision),before,hash[:]});if err!=nil{return zero,err}
 eventIDs:=[]any{sent.Identity[:],roundEvent.Identity[:]}
 journalFields:=[]any{run[:],cycle.ID[:],uint64(cycle.Attempt),int64(cycle.Instant),
  uint64(cycle.Ordinal),[]any{uint64(0)},uint64(cycle.BaseRevision),hash[:],
  uint64(next.Revision),hash[:],fence.Digest[:],[]any{response.ID[:]},fence.InputDigest[:],
  policies["cycle-coordinate"],policies["event-order"],eventIDs,[]any{decisionID[:]},
  pairDigest[:],[]any{},taskDigest[:],policies["perception"],policies["perception-identity"],
  uint64(next.NextLogicalSequence)}
 batchHash,_,err:=d.Sum("cote.csf.digest.batch","cote.csf.schema.batch-preimage",1,
  []any{journalFields,[]any{sent.Canonical,roundEvent.Canonical}})
 if err!=nil{return zero,err}
 commitID,_,err:=d.Sum("cote.csf.id.cycle-commit","cote.csf.schema.cycle-commit-id-preimage",1,
  []any{run[:],cycle.ID[:],uint64(cycle.Attempt)})
 if err!=nil{return zero,err}
 journalRaw,err:=codec.CanonicalPayload(append(cloneAny(journalFields),batchHash[:]))
 if err!=nil{return zero,err}
 batch:=ports.WorkBatch{Mode:7,RunID:[32]byte(run),CycleID:[32]byte(cycle.ID),
  FenceID:[32]byte(fence.ID),ExpectedClock:oldClock,ResultClock:afterClock,
  ExpectedInputPrefix:prefix,BaseStateHash:[32]byte(hash),ResultStateHash:[32]byte(hash),
  RevisionCanonical:revision,
  Journal:ports.CanonicalRecord{Identity:[32]byte(commitID),Canonical:journalRaw},
  Decisions:[]ports.CanonicalRecord{decision},Events:events,
  CommunicationCandidate:candidate,CommunicationBundle:c.bundle,
  Communication:&ports.CommunicationPublication{Claim:plan.Claim,Transmission:plan.Transmission,
   Occurrence:plan.Occurrence},FailAfterStep:c.failAfterStep}
 if err:=c.commitPreparedWork(ctx,&batch);err!=nil{return zero,err}
 return batch.Journal,nil
}

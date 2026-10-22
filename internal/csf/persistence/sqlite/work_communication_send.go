package sqlite

import (
 "bytes"
 "context"
 "database/sql"
 "errors"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/clock"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/communication"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/genesis"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/settlement"
)

// verifyCommunicationSendTerminal derives the complete proposed send from
// the frozen admitted input. No recipient, claim, occurrence or Event body
// from the batch is accepted without exact canonical rederivation. The
// surrounding CommitWork transaction owns its event/clock/WorldRevision CAS.
func verifyCommunicationSendTerminal(ctx context.Context,tx *sql.Tx,
 b ports.WorkBatch,prior,next clock.State,fence,journal,predecessor,revision []any,
 expectedSlots map[domain.ID]map[string]bool)error{
 if b.Mode!=7||b.Communication==nil||b.CommunicationBundle==nil||
  len(b.Decisions)!=1||len(b.Events)!=2||len(expectedSlots)!=1||
  len(b.SystemExtraCandidates)!=0||len(b.SystemSuccessors)!=0||
  len(b.ContestProofs)!=0||b.ResourceCandidate.Canonical!=nil{
  return errors.New("communication terminal has unsupported mixed cohort/partial records")
 }
 if !sameCanonical(revision[4],predecessor[4])||
  !sameCanonical(revision[5],b.BaseStateHash[:])||
  b.ResultStateHash!=b.BaseStateHash{
  return errors.New("communication allegation cannot mutate authoritative WorldState")
 }
 var manifest []byte
 if err:=tx.QueryRowContext(ctx,
  "SELECT canonical FROM genesis_records WHERE run_id=?",b.RunID[:]).Scan(&manifest);err!=nil{return err}
 root,err:=genesis.ValidateCanonical(manifest,b.CommunicationBundle)
 if err!=nil||[32]byte(root.RunID)!=b.RunID{return errors.New("communication policy not pinned by Genesis")}
 units,ok:=fence[11].([]any)
 if !ok||len(units)!=1{return errors.New("communication send requires one admitted slot")}
 u,ok:=units[0].([]any)
 if !ok||len(u)!=5||u[1]!=uint64(0){return errors.New("communication source unit not a slot response")}
 id,ok:=idField(u[3]);if !ok{return errors.New("communication ActionProposal ID invalid")}
 var responseRaw []byte
 if err:=tx.QueryRowContext(ctx,"SELECT canonical FROM input_ledger WHERE id=?",id[:]).Scan(&responseRaw);err!=nil{
  return errors.New("communication source absent from frozen input ledger")
 }
 authored,err:=communication.AuthoredAction(b.CommunicationBundle,
  ports.CanonicalRecord{Identity:id,Canonical:responseRaw})
 if err!=nil{return err}
 if !equalBytes(u[4],authored.Source.Digest[:])||
  [32]byte(authored.Run)!=b.RunID||[32]byte(authored.Cycle)!=b.CycleID||
  uint64(authored.Revision)!=uint64(prior.Revision)||
  authored.Spec.SentAt!=prior.Instant{
  return errors.New("communication ActionProposal not admitted at current frozen instant")
 }
 if len(expectedSlots[authored.Round])!=1||!expectedSlots[authored.Round][authored.Slot]{
  return errors.New("communication ActionProposal round/slot outside sealed cohort")
 }
 plan,err:=communication.PrepareSend(b.CommunicationBundle,authored.Spec)
 if err!=nil{return err}
 for _,record:=range [][2]ports.CanonicalRecord{
  {plan.Claim,b.Communication.Claim},{plan.Transmission,b.Communication.Transmission},
  {plan.Occurrence,b.Communication.Occurrence},
 }{
  if record[0].Identity!=record[1].Identity||
   !bytes.Equal(record[0].Canonical,record[1].Canonical){
   return errors.New("forged or omitted communication Claim/Transmission/ScheduledOccurrence")
  }
 }
 sent,err:=communication.NewSentEvent(b.CommunicationBundle,
  communication.EventContext{Run:authored.Run,Cycle:authored.Cycle,At:prior.Instant,
   BaseRevision:prior.Revision,Sequence:prior.NextLogicalSequence},plan,authored.Source)
 if err!=nil{return err}
 refs:=[]any{[]any{uint64(0),uint64(0),id[:],authored.Source.Digest[:]}}
 roundEvent,err:=settlement.ConsumedRoundEvent(authored.Run,authored.Cycle,
  authored.Round,prior.Instant,prior.Revision,prior.NextLogicalSequence+1,refs)
 if err!=nil{return err}
 for i,want:=range []ports.CanonicalRecord{sent,roundEvent}{
  if b.Events[i].Identity!=want.Identity||!bytes.Equal(b.Events[i].Canonical,want.Canonical){
   return errors.New("communication Event or round closure not derived from admitted proposal")
  }
 }
 candidateID,err:=registeredDigest("cote.csf.id.commit-candidate",
  "cote.csf.schema.candidate-id-preimage",[]any{
   b.RunID[:],b.CycleID[:],[]any{id[:]},"communication.send",uint64(1),uint64(0),uint64(0)})
 if err!=nil||candidateID!=b.CommunicationCandidate.Identity{
  return errors.New("unregistered communication candidate ID")
 }
 candidate,err:=strictFull(b.CommunicationCandidate,17)
 if err!=nil{return err}
 cd,err:=registeredDigest("cote.csf.digest.commit-candidate",
  "cote.csf.schema.commit-candidate.body",candidate[:16])
 if err!=nil||!equalBytes(candidate[16],cd[:])||
  !equalBytes(candidate[0],candidateID[:])||
  !sameCanonical(candidate[3],[]any{id[:]})||
  candidate[4]!="communication.send"||candidate[5]!=uint64(1)||
  !sameCanonical(candidate[13],[]any{[]any{uint64(5),id[:],authored.Source.Digest[:]}}){
  return errors.New("communication candidate not provably bound to source")
 }
 eventValue,err:=codec.StrictDecodePayload(sent.Canonical);if err!=nil{return err}
 sentFields:=eventValue.([]any)
 expectedDraft:=[]any{uint64(0),uint64(0),"communication.sent",sentFields[9],[]any{}}
 if !sameCanonical(candidate[8],[]any{expectedDraft})||
  !sameCanonical(candidate[9],[]any{})||
  !sameCanonical(candidate[10],[]any{})||
  !sameCanonical(candidate[11],[]any{})||
  !sameCanonical(candidate[12],[]any{"communication.authorized"})||
  !sameCanonical(candidate[14],int64(0)){
  return errors.New("communication candidate declared noncanonical drafts/resource claims")
 }
 decisionID,err:=registeredDigest("cote.csf.id.decision","cote.csf.schema.decision-id-preimage",
  []any{b.CycleID[:],journal[2],id[:]})
 if err!=nil||decisionID!=b.Decisions[0].Identity{return errors.New("communication decision identity invalid")}
 decision,err:=strictFull(b.Decisions[0],16);if err!=nil{return err}
 decisionDigest,err:=registeredDigest("cote.csf.digest.decision-record",
  "cote.csf.schema.decision-record.body",decision[:15])
 if err!=nil||!equalBytes(decision[15],decisionDigest[:])||
  !equalBytes(decision[0],decisionID[:])||!sameCanonical(decision[4],
   []any{uint64(0),uint64(0),id[:],authored.Source.Digest[:]})||
  decision[5]!=uint64(0)||decision[7]!="communication.sent"||
  !sameCanonical(decision[6],[]any{uint64(1),candidateID[:]})||
  !sameCanonical(decision[12],[]any{sent.Identity[:]}){
  return errors.New("communication commit DecisionRecord invalid")
 }
 pairs:=[]any{[]any{id[:],decisionID[:],decisionDigest[:]}}
 digest,err:=registeredDigest("cote.csf.digest.decision",
  "cote.csf.schema.decision-pair-list",pairs)
 if err!=nil||!equalBytes(journal[17],digest[:])||
  !sameCanonical(journal[16],[]any{decisionID[:]})||
  !sameCanonical(journal[15],[]any{sent.Identity[:],roundEvent.Identity[:]})||
  !sameCanonical(journal[11],[]any{id[:]})||
  !sameCanonical(journal[9],b.BaseStateHash[:])||
  !sameCanonical(journal[8],uint64(next.Revision)){
  return errors.New("communication journal missing its exact source/decision/event closure")
 }
 return nil
}

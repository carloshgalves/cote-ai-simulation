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
)

// LoadCommunicationForOccurrence reads only immutable records from a successful
// send; callers still must prove the occurrence is the exact admitted unit.
func (s *Store) LoadCommunicationForOccurrence(ctx context.Context,run,occurrence [32]byte)(
 ports.CommunicationPublication,error){
 var out ports.CommunicationPublication
 if s==nil{return out,errors.New("nil communication store")}
 var transmissionID,claimID,sentCommit []byte
 var rawTransmission,rawClaim,rawOccurrence []byte
 if err:=s.db.QueryRowContext(ctx,"SELECT id,claim_id,sent_commit_id,canonical FROM communication_transmissions WHERE run_id=? AND occurrence_id=? AND status=0",
  run[:],occurrence[:]).Scan(&transmissionID,&claimID,&sentCommit,&rawTransmission);err!=nil{return out,err}
 if err:=s.db.QueryRowContext(ctx,"SELECT canonical FROM communication_claims WHERE run_id=? AND id=?",run[:],claimID).Scan(&rawClaim);err!=nil{return out,err}
 if err:=s.db.QueryRowContext(ctx,"SELECT canonical FROM scheduled_occurrences WHERE run_id=? AND id=? AND lifecycle=0",
  run[:],occurrence[:]).Scan(&rawOccurrence);err!=nil{return out,err}
 if len(transmissionID)!=32||len(claimID)!=32||len(sentCommit)!=32{return out,errors.New("durable transmission metadata malformed")}
 copy(out.Claim.Identity[:],claimID);out.Claim.Canonical=rawClaim
 copy(out.Transmission.Identity[:],transmissionID);out.Transmission.Canonical=rawTransmission
 out.Occurrence.Identity=occurrence;out.Occurrence.Canonical=rawOccurrence
 return out,nil
}

// verifyCommunicationDueTerminal is the sole mode-8 authorization. It
// reconstructs the terminal Event and every canonical source proof from
// committed run history, never from worker-provided channel results.
func verifyCommunicationDueTerminal(ctx context.Context,tx *sql.Tx,b ports.WorkBatch,
 prior,next clock.State,fence,journal,predecessor,revision []any,
 expectedSlots map[domain.ID]map[string]bool)error{
 if b.Mode!=8||b.Communication==nil||b.CommunicationBundle==nil||
  b.CommunicationStatus<1||b.CommunicationStatus>2||
  len(b.Decisions)!=1||len(b.Events)!=1||len(expectedSlots)!=0||
  len(b.SystemSuccessors)!=0||len(b.SystemExtraCandidates)!=0||
  len(b.ContestProofs)!=0||len(b.PerceptionTasks)>1||
  b.ResourceCandidate.Canonical!=nil{
  return errors.New("due communication has unsupported partial/mixed terminal output")
 }
 if b.ResultStateHash!=b.BaseStateHash||!sameCanonical(revision[4],predecessor[4])||
  !sameCanonical(revision[5],b.BaseStateHash[:]){
  return errors.New("delivering a claim may not mutate WorldState")
 }
 var genesisRaw []byte
 if err:=tx.QueryRowContext(ctx,"SELECT canonical FROM genesis_records WHERE run_id=?",b.RunID[:]).Scan(&genesisRaw);err!=nil{return err}
 manifest,err:=genesis.ValidateCanonical(genesisRaw,b.CommunicationBundle)
 if err!=nil||[32]byte(manifest.RunID)!=b.RunID{return errors.New("communication due Genesis policy mismatch")}
 bindings,ok:=manifest.Payload[14].([]any)
 if !ok{return errors.New("Genesis communication policy roster invalid")}
 pinned:=map[string]any{}
 for _,v:=range bindings{
  ref,ok:=v.([]any);if !ok||len(ref)!=3{return errors.New("Genesis communication policy reference malformed")}
  name,ok:=ref[0].(string);if !ok{return errors.New("Genesis communication policy identity malformed")}
  pinned[name]=ref
 }
 if pinned["validators-v1"]==nil||pinned["conflict-resolvers-v1"]==nil{
  return errors.New("Genesis communication decision policy unregistered")
 }
 units,ok:=fence[11].([]any)
 if !ok||len(units)!=1{return errors.New("due delivery requires exactly one admitted scheduled unit")}
 entry,ok:=units[0].([]any)
 if !ok||len(entry)!=5||entry[1]!=uint64(2){return errors.New("due source is not scheduled occurrence")}
 id,ok:=idField(entry[3]);if !ok||id!=b.Communication.Occurrence.Identity{return errors.New("due occurrence identity differs from fence")}
 dg,ok:=idField(entry[4]);if !ok{return errors.New("due occurrence digest absent")}
 var durableOccurrence []byte
 var eligibilityInstant,eligibilityOrdinal,life int64
 if err:=tx.QueryRowContext(ctx,"SELECT canonical,eligibility_instant,eligibility_ordinal,lifecycle FROM scheduled_occurrences WHERE run_id=? AND id=?",
  b.RunID[:],id[:]).Scan(&durableOccurrence,&eligibilityInstant,&eligibilityOrdinal,&life);err!=nil{return err}
 if life!=0||!bytes.Equal(durableOccurrence,b.Communication.Occurrence.Canonical){
  return errors.New("due ScheduledOccurrence changed or terminal")
 }
 due,lifeCheck,err:=validateScheduledOccurrence(b.RunID,id,durableOccurrence)
 if err!=nil||lifeCheck!=0||int64(due.NotBeforeInstant)!=eligibilityInstant||
  uint64(due.NotBeforeCycleOrdinal)!=uint64(eligibilityOrdinal){
  return errors.New("due occurrence indexed coordinate invalid")
 }
 decoded,err:=codec.StrictDecodePayload(durableOccurrence);if err!=nil{return err}
 occurrenceFields:=decoded.([]any)
 if !equalBytes(occurrenceFields[14],dg[:]){
  return errors.New("fenced due occurrence digest mismatch")
 }
 if due.NotBeforeInstant>prior.Instant||due.NotBeforeCycleOrdinal>domain.CycleOrdinal(next.LastOrdinal){
  return errors.New("delivery before selected due coordinate")
 }
 var durableTransmission,durableClaim,sentCommit []byte
 var claimID,storedTXID []byte
 var status int64
 if err:=tx.QueryRowContext(ctx,
  "SELECT id,claim_id,sent_commit_id,status,canonical FROM communication_transmissions WHERE run_id=? AND occurrence_id=?",
  b.RunID[:],id[:]).Scan(&storedTXID,&claimID,&sentCommit,&status,&durableTransmission);err!=nil{return err}
 if status!=0||!equalBytes(storedTXID,b.Communication.Transmission.Identity[:])||
  !bytes.Equal(durableTransmission,b.Communication.Transmission.Canonical)||
  !equalBytes(claimID,b.Communication.Claim.Identity[:]){
  return errors.New("terminal/stale or forged transmission record")
 }
 if err:=tx.QueryRowContext(ctx,"SELECT canonical FROM communication_claims WHERE run_id=? AND id=?",
  b.RunID[:],claimID).Scan(&durableClaim);err!=nil{return err}
 if !bytes.Equal(durableClaim,b.Communication.Claim.Canonical){return errors.New("communication Claim no longer immutable")}
 if len(sentCommit)!=32{return errors.New("transmission send commit reference absent")}
 var sourceJournalRaw []byte
 if err:=tx.QueryRowContext(ctx,
  "SELECT j.canonical FROM committed_cycles c JOIN decision_ledger j ON j.id=c.commit_id WHERE c.run_id=? AND c.commit_id=?",
  b.RunID[:],sentCommit).Scan(&sourceJournalRaw);err!=nil{return errors.New("delivery has no durable earlier send CycleCommit")}
 earlier,err:=codec.StrictDecodePayload(sourceJournalRaw);if err!=nil{return err}
 priorCommit,ok:=earlier.([]any)
 if !ok||len(priorCommit)!=24{return errors.New("earlier send journal corrupt")}
 sentAtRevision,ok:=priorCommit[8].(uint64)
 if !ok||sentAtRevision>uint64(prior.Revision){return errors.New("delivery claims nonexistent future sent message")}
 trans,err:=communication.AuthenticateTransmission(b.CommunicationBundle,b.Communication.Transmission)
 if err!=nil{return err}
 if !containsRoot(priorCommit[11],[32]byte(trans.Origin.ID)){
  return errors.New("transmission born outside its claimed send CycleCommit")
 }
 plan:=communication.SendPlan{Run:domain.ID(b.RunID),Claim:b.Communication.Claim,
  Transmission:b.Communication.Transmission,Occurrence:b.Communication.Occurrence}
 dueAt,err:=communication.ValidateOccurrence(b.CommunicationBundle,plan)
 if err!=nil||dueAt>prior.Instant{return errors.New("communication due cryptographic origin invalid")}
 result,err:=communication.PlanDueDelivery(b.CommunicationBundle,plan.Transmission,plan.Claim,
  domain.ID(id),dueAt,prior.Instant)
 if err!=nil||result.NextStatus!=b.CommunicationStatus{return errors.New("channel outcome not registered or mismatched")}
 dueEvent,err:=communication.NewDueEvent(b.CommunicationBundle,communication.EventContext{
  Run:domain.ID(b.RunID),Cycle:domain.ID(b.CycleID),BaseRevision:prior.Revision,
  At:prior.Instant,Sequence:prior.NextLogicalSequence},
  plan,result,communication.Origin{Kind:6,ID:domain.ID(id),Digest:domain.Digest(dg)})
 if err!=nil||dueEvent.Identity!=b.Events[0].Identity||
  !bytes.Equal(dueEvent.Canonical,b.Events[0].Canonical){
  return errors.New("due delivery Event differs from registered deterministic channel")
 }
 candidateID,err:=registeredDigest("cote.csf.id.commit-candidate","cote.csf.schema.candidate-id-preimage",
  []any{b.RunID[:],b.CycleID[:],[]any{id[:]},"communication.delivery",uint64(1),uint64(0),uint64(0)})
 if err!=nil||candidateID!=b.CommunicationCandidate.Identity{return errors.New("communication due candidate ID invalid")}
 candidate,err:=strictFull(b.CommunicationCandidate,17);if err!=nil{return err}
 candidateDG,err:=registeredDigest("cote.csf.digest.commit-candidate","cote.csf.schema.commit-candidate.body",candidate[:16])
 if err!=nil||!equalBytes(candidate[16],candidateDG[:])||
  !sameCanonical(candidate[3],[]any{id[:]})||candidate[4]!="communication.delivery"||
  candidate[5]!=uint64(1)||!sameCanonical(candidate[13],[]any{[]any{uint64(2),id[:],dg[:]}})||
  !sameCanonical(candidate[8],[]any{[]any{uint64(0),uint64(0),result.EventType,
   eventPayload(dueEvent),[]any{}}})||
  !sameCanonical(candidate[9],[]any{})||!sameCanonical(candidate[10],[]any{})||
  !sameCanonical(candidate[11],[]any{})||!sameCanonical(candidate[12],[]any{"source.lifecycle"})||
  !sameCanonical(candidate[14],int64(0))||
  !sameCanonical(candidate[15],pinned["conflict-resolvers-v1"]){
  return errors.New("communication due candidate fields or provenance invalid")
 }
 did,err:=registeredDigest("cote.csf.id.decision","cote.csf.schema.decision-id-preimage",
  []any{b.CycleID[:],journal[2],id[:]})
 if err!=nil||did!=b.Decisions[0].Identity{return errors.New("communication due decision ID invalid")}
 decision,err:=strictFull(b.Decisions[0],16);if err!=nil{return err}
 decisionDG,err:=registeredDigest("cote.csf.digest.decision-record","cote.csf.schema.decision-record.body",decision[:15])
 if err!=nil||!equalBytes(decision[15],decisionDG[:])||
  !sameCanonical(decision[4],[]any{uint64(2),id[:],dg[:]})||
  decision[5]!=uint64(0)||decision[7]!=result.EventType||
  !sameCanonical(decision[6],[]any{uint64(1),candidateID[:]})||
  !sameCanonical(decision[12],[]any{dueEvent.Identity[:]})||
  !sameCanonical(decision[13],[]any{uint64(0)})||
  !sameCanonical(decision[14],[]any{uint64(0)})||
  !equalBytes(decision[0],did[:])||!equalBytes(decision[1],b.RunID[:])||
  !equalBytes(decision[2],b.CycleID[:])||decision[3]!=journal[2]||
  !sameCanonical(decision[8],[]any{})||!sameCanonical(decision[11],[]any{})||
  !sameCanonical(decision[9],pinned["validators-v1"])||
  !sameCanonical(decision[10],pinned["conflict-resolvers-v1"]){
  return errors.New("communication due DecisionRecord not canonical")
 }
 pairHash,err:=registeredDigest("cote.csf.digest.decision","cote.csf.schema.decision-pair-list",
  []any{[]any{id[:],did[:],decisionDG[:]}})
 if err!=nil||!equalBytes(journal[17],pairHash[:])||
  !sameCanonical(journal[11],[]any{id[:]})||
  !sameCanonical(journal[16],[]any{did[:]})||
  !sameCanonical(journal[15],[]any{dueEvent.Identity[:]})||
  !sameCanonical(journal[9],b.BaseStateHash[:]){
  return errors.New("communication due journal missing exact source closure")
 }
 return nil
}
func eventPayload(record ports.CanonicalRecord)any{
 f,err:=codec.StrictDecodePayload(record.Canonical)
 if err!=nil{return nil}
 v,ok:=f.([]any);if !ok||len(v)!=19{return nil}
 return v[9]
}

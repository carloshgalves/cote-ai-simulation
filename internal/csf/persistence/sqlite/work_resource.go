package sqlite

import (
 "bytes"
 "context"
 "errors"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/clock"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/reducer"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/settlement"
)

// verifySingleResourceTerminal runs under the *same* SQLite transaction as
// the source settlement, EventStore, journal, world revision and clock CAS.
// It never delegates world truth to the provisional decision output.
func verifySingleResourceTerminal(ctx context.Context,tx queryOne,b ports.WorkBatch,
 prior,next clock.State,fence,journal,predecessor,revision []any,
 allowed map[string]any,expectedSlots map[domain.ID]map[string]bool)error{
 if len(b.Decisions)!=1||len(b.Events)!=2||len(expectedSlots)!=1{
  return errors.New("first resource WORK requires one source, one winner and two events")
 }
 sources,ok:=fence[11].([]any);if !ok||len(sources)!=1{return errors.New("resource fence must admit one source")}
 unit,ok:=sources[0].([]any);if !ok||len(unit)!=5||unit[1]!=uint64(0){return errors.New("resource source is not a slot")}
 sourceID,ok:=unit[3].([]byte);if !ok||len(sourceID)!=32{return errors.New("resource input id invalid")}
 sourceDigest,ok:=unit[4].([]byte);if !ok||len(sourceDigest)!=32{return errors.New("resource input digest invalid")}
 var source domain.ID;copy(source[:],sourceID)
 var dig domain.Digest;copy(dig[:],sourceDigest)
 var raw []byte
 if err:=tx.QueryRowContext(ctx,"SELECT canonical FROM input_ledger WHERE id=?",sourceID).Scan(&raw);err!=nil{return err}
 value,err:=codec.StrictDecodePayload(raw);if err!=nil{return err}
 proposal,ok:=value.([]any)
 if !ok||len(proposal)!=19||!equalBytes(proposal[0],sourceID)||
  !equalBytes(proposal[2],b.RunID[:])||!equalBytes(proposal[3],b.CycleID[:])||
  proposal[4]!="resource.consume"||proposal[5]!=uint64(1)||
  proposal[15]!=uint64(prior.Revision){return errors.New("unregistered source ActionProposal")}
 refID,err:=registeredDigest("cote.csf.id.action-proposal","cote.csf.schema.slot-response-id-preimage",
  []any{proposal[1],uint64(0)});if err!=nil{return err}
 sourceHash,err:=registeredDigest("cote.csf.digest.action-proposal-unit",
  "cote.csf.schema.action-proposal.body",[]any{proposal[:18]});if err!=nil{return err}
 if !equalBytes(refID[:],sourceID)||!equalBytes(sourceHash[:],sourceDigest)||
  !equalBytes(proposal[18],sourceDigest){return errors.New("ActionProposal ID/digest not sealed")}
 actor,ok:=proposal[6].([]any)
 if !ok||len(actor)!=2||actor[0]!="school"{return errors.New("resource producer actor not registered in test school scope")}
 target,ok:=proposal[7].([]any);if !ok||len(target)!=0{return errors.New("resource target injection")}
 roundBytes,ok:=proposal[13].([]byte);if !ok||len(roundBytes)!=32{return errors.New("resource round ID invalid")}
 slot,ok:=proposal[14].(string);if !ok{return errors.New("resource slot ID invalid")}
 var roundID domain.ID;copy(roundID[:],roundBytes)
 if !expectedSlots[roundID][slot]||len(expectedSlots[roundID])!=1{
  return errors.New("resource source did not settle complete selected round")
 }
 claims,ok:=proposal[9].([]any);if !ok||len(claims)!=1{return errors.New("resource proposal claim cardinality invalid")}
 claim,ok:=claims[0].([]any)
 if !ok||len(claim)!=4||claim[0]!="inventory"||claim[1]!="last.resource"||
  claim[2]!=uint64(1)||!sameCanonical(claim[3],uint64(1)){
  return errors.New("resource event has no authenticated one-unit claim")
 }
 candidate,err:=strictFull(b.ResourceCandidate,17);if err!=nil{return err}
 srcList:=[]any{sourceID}
 if !equalBytes(candidate[1],b.RunID[:])||!equalBytes(candidate[2],b.CycleID[:])||
  !sameCanonical(candidate[3],srcList)||candidate[4]!="resource.consume"||
  candidate[5]!=uint64(1)||candidate[6]!=uint64(0)||candidate[7]!=uint64(0)||
  !sameCanonical(candidate[9],claims){
  return errors.New("resource candidate not bound to authenticated source and claim")
 }
 provenance:=[]any{[]any{uint64(5),sourceID,sourceDigest}}
 if !sameCanonical(candidate[13],provenance)||!sameCanonical(candidate[12],[]any{"resource.nonnegative"}){
  return errors.New("resource candidate provenance/invariant mismatch")
 }
 identity,err:=registeredDigest("cote.csf.id.commit-candidate","cote.csf.schema.candidate-id-preimage",
  []any{b.RunID[:],b.CycleID[:],srcList,candidate[4],candidate[5],candidate[6],candidate[7]})
 if err!=nil{return err}
 hash,err:=registeredDigest("cote.csf.digest.commit-candidate","cote.csf.schema.commit-candidate.body",candidate[:16])
 if err!=nil{return err}
 if identity!=b.ResourceCandidate.Identity||!equalBytes(candidate[16],hash[:]){
  return errors.New("resource candidate identity or digest wrong")
 }
 drafts,ok:=candidate[8].([]any);if !ok||len(drafts)!=1{return errors.New("resource candidate has no single EventDraft")}
 draft,ok:=drafts[0].([]any)
 if !ok||len(draft)!=5||!sameCanonical(draft[3],proposal[8]){
  return errors.New("resource EventDraft not signed by ActionProposal parameters")
 }
 resourceEvent,err:=settlement.ResourceConsumedEvent(domain.ID(b.RunID),domain.ID(b.CycleID),
  domain.ID(b.ResourceCandidate.Identity),source,dig,prior.Instant,prior.Revision,
  prior.NextLogicalSequence,draft)
 if err!=nil{return err}
 refs:=[]any{[]any{uint64(0),uint64(0),sourceID,sourceDigest}}
 lifecycleEvent,err:=settlement.ConsumedRoundEvent(domain.ID(b.RunID),domain.ID(b.CycleID),
  roundID,prior.Instant,prior.Revision,prior.NextLogicalSequence+1,refs)
 if err!=nil{return err}
 for i,expect:=range []ports.CanonicalRecord{resourceEvent,lifecycleEvent}{
  if b.Events[i].Identity!=expect.Identity||!bytes.Equal(b.Events[i].Canonical,expect.Canonical){
   return errors.New("resource event ordering/payload/provenance not from candidate and round")
  }
 }
 records,ok:=journal[15].([]any)
 if !ok||len(records)!=2||!sameCanonical(records[0],resourceEvent.Identity[:])||
  !sameCanonical(records[1],lifecycleEvent.Identity[:]){
  return errors.New("resource event references differ from canonical event batch")
 }
 beforeStock,err:=reducer.DecodeStock(predecessor[4]);if err!=nil{return err}
 if beforeStock.Available!=1{return errors.New("single-consumer resource fixture must have one available unit")}
 reduced,digest,err:=reducer.Consume(predecessor[4].([]any),1);if err!=nil{return err}
 if !sameCanonical(revision[4],reduced)||!equalBytes(revision[5],digest[:])||
  !equalBytes(b.ResultStateHash[:],digest[:])||!equalBytes(journal[9],digest[:]){
  return errors.New("resource WorldRevision not produced by pure versioned reducer")
 }
 decision,err:=strictFull(b.Decisions[0],16);if err!=nil{return err}
 decisionID,err:=registeredDigest("cote.csf.id.decision","cote.csf.schema.decision-id-preimage",
  []any{b.CycleID[:],journal[2],sourceID});if err!=nil{return err}
 decisionDigest,err:=registeredDigest("cote.csf.digest.decision-record","cote.csf.schema.decision-record.body",decision[:15])
 if err!=nil{return err}
 if decisionID!=b.Decisions[0].Identity||!equalBytes(decision[15],decisionDigest[:])||
  !equalBytes(decision[1],b.RunID[:])||!equalBytes(decision[2],b.CycleID[:])||
  !sameCanonical(decision[3],journal[2])||
  !sameCanonical(decision[4],[]any{uint64(0),uint64(0),sourceID,sourceDigest})||
  decision[5]!=uint64(0)||decision[7]!="resource.allocated"||
  !sameCanonical(decision[6],[]any{uint64(1),b.ResourceCandidate.Identity[:]})||
  !sameCanonical(decision[12],[]any{resourceEvent.Identity[:]})||
  !sameCanonical(decision[9],allowed["validators-v1"])||
  !sameCanonical(decision[10],allowed["conflict-resolvers-v1"]){
  return errors.New("resource DecisionRecord not bound to selected winning candidate")
 }
 for _,index:=range []int{8,11}{
  if !sameCanonical(decision[index],[]any{}){return errors.New("single resource winner may not claim conflict or RNG")}
 }
 for _,index:=range []int{13,14}{
  if !sameCanonical(decision[index],noRef()){return errors.New("resource decision claimed unsupported successor or alias")}
 }
 if !sameCanonical(journal[16],[]any{b.Decisions[0].Identity[:]}){return errors.New("resource decision missing from CycleCommit")}
 pair:=[]any{[]any{sourceID,b.Decisions[0].Identity[:],decisionDigest[:]}}
 pairHash,err:=registeredDigest("cote.csf.digest.decision","cote.csf.schema.decision-pair-list",pair)
 if err!=nil||!equalBytes(journal[17],pairHash[:]){return errors.New("resource decision digest mismatch")}
 tasksHash,err:=registeredDigest("cote.csf.digest.perception-task-list","cote.csf.schema.perception-task-id-list",journal[18])
 if err!=nil||!equalBytes(journal[19],tasksHash[:]){return errors.New("secret-only resource events must have empty task digest")}
 if next.NextLogicalSequence!=prior.NextLogicalSequence+2{return errors.New("resource logical sequence is not contiguous")}
 return nil
}

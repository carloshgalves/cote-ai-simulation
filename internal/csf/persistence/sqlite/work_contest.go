package sqlite

import (
 "bytes"
 "context"
 "encoding/hex"
 "errors"
 "fmt"
 "math"
 "sort"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/clock"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/reducer"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/settlement"
)

// verifyTwoResourceTerminal is shared by live CommitWork and replay. It
// rederives the RNG substream from the PINNED genesis seed, candidate IDs
// from authenticated sources and the named conflict, rather than trusting
// any claimant's chosen winning DecisionRecord.
func verifyTwoResourceTerminal(ctx context.Context,q queryOne,b ports.WorkBatch,
 prior,next clock.State,fence,journal,predecessor,revision []any,
 genesis []any,allowed map[string]any,expectedSlots map[domain.ID]map[string]bool)error{
 if len(b.Decisions)!=2||len(b.Events)!=3||len(b.ContestProofs)!=6||
  len(expectedSlots)!=2{return errors.New("two-agent contest requires two decisions and three events")}
 rawSources,ok:=fence[11].([]any);if !ok||len(rawSources)!=2{return errors.New("two-agent contest source set invalid")}
 type competitor struct{source,round,candidate domain.ID;digest domain.Digest;slot string;candidateBody,proposal []any}
 competitors:=make([]competitor,0,2)
 seenActors:=map[string]bool{}
 for _,item:=range rawSources{
  fields,ok:=item.([]any)
  if !ok||len(fields)!=5||fields[1]!=uint64(0){return errors.New("contest admits non-slot input")}
  source,ok:=fields[3].([]byte);if !ok||len(source)!=32{return errors.New("contest source missing ID")}
  sourceHash,ok:=fields[4].([]byte);if !ok||len(sourceHash)!=32{return errors.New("contest source missing digest")}
  var raw []byte
  if err:=q.QueryRowContext(ctx,"SELECT canonical FROM input_ledger WHERE id=?",source).Scan(&raw);err!=nil{return err}
  decoded,err:=codec.StrictDecodePayload(raw);if err!=nil{return err}
  prop,ok:=decoded.([]any)
  if !ok||len(prop)!=19||!equalBytes(prop[0],source)||!equalBytes(prop[2],b.RunID[:])||
   !equalBytes(prop[3],b.CycleID[:])||prop[4]!="resource.consume"||prop[5]!=uint64(1)||
   prop[15]!=uint64(prior.Revision){return errors.New("contest source is not registered ActionProposal")}
  claimList,ok:=prop[9].([]any);if !ok||len(claimList)!=1{return errors.New("contest source claims invalid")}
  claim,ok:=claimList[0].([]any)
  if !ok||len(claim)!=4||claim[0]!="inventory"||claim[1]!="last.resource"||
   claim[2]!=uint64(1)||!sameCanonical(claim[3],uint64(1)){
   return errors.New("unregistered contest resource claim")
  }
  actor,ok:=prop[6].([]any);if !ok||len(actor)!=2||actor[0]!="school"{
   return errors.New("contest action actor not eligible for synthetic resource")
  }
  actorID,ok:=actor[1].(string)
  if !ok||actorID==""||seenActors[actorID]{return errors.New("contest actor duplicated or invalid")}
  seenActors[actorID]=true
  targets,ok:=prop[7].([]any)
  if !ok||len(targets)!=0||!sameCanonical(prop[10],noRef())||
   !sameCanonical(prop[12],noRef())||!sameCanonical(prop[17],noRef()){
   return errors.New("contest proposal references unregistered target, location, duration or idempotency")
  }
  effective,ok:=asSigned64(prop[11])
  if !ok||effective!=int64(prior.Instant){return errors.New("contest proposal execution instant does not match sealed clock")}
  params,ok:=prop[8].([]any)
  if !ok||len(params)!=4||params[0]!="cote.csf.schema.typed-value"||params[1]!=uint64(1){
   return errors.New("contest proposal typed payload malformed")
  }
  payload,ok:=params[2].([]byte)
  if !ok{return errors.New("contest proposal typed payload missing")}
  envelope,err:=codec.StrictDecodeEnvelope(payload)
  if err!=nil||envelope.DomainTag!="cote.csf.testresource.params"||
   envelope.SchemaID!="cote.csf.schema.typed-value"||envelope.SchemaVersion!=1||
   !sameCanonical(envelope.Value,[]any{}){
   return errors.New("contest proposal parameters are not registered resource input")
  }
  paramHash:=identity.OpaqueContentHash(payload)
  if !equalBytes(params[3],paramHash[:]){
   return errors.New("contest proposal parameters have invalid typed digest")
  }
  id,err:=registeredDigest("cote.csf.id.action-proposal","cote.csf.schema.slot-response-id-preimage",
   []any{prop[1],uint64(0)});if err!=nil{return err}
  dg,err:=registeredDigest("cote.csf.digest.action-proposal-unit",
   "cote.csf.schema.action-proposal.body",[]any{prop[:18]});if err!=nil{return err}
  if !equalBytes(id[:],source)||!equalBytes(dg[:],sourceHash)||
   !equalBytes(prop[18],sourceHash){return errors.New("contest proposal ID/digest invalid")}
  round,ok:=prop[13].([]byte);if !ok||len(round)!=32{return errors.New("contest source round ID malformed")}
  slot,ok:=prop[14].(string);if !ok{return errors.New("contest slot malformed")}
  var r,s domain.ID;copy(r[:],round);copy(s[:],source)
  if !expectedSlots[r][slot]||len(expectedSlots[r])!=1{
   return errors.New("contest did not cover every selected round slot")
  }
  candidateID,err:=registeredDigest("cote.csf.id.commit-candidate",
   "cote.csf.schema.candidate-id-preimage",
   []any{b.RunID[:],b.CycleID[:],[]any{source},"resource.consume",uint64(1),uint64(0),uint64(0)})
  if err!=nil{return err}
  competitors=append(competitors,competitor{source:s,round:r,candidate:domain.ID(candidateID),digest:domain.Digest(dg),slot:slot,proposal:prop})
 }
 if competitors[0].round==competitors[1].round||competitors[0].source==competitors[1].source{
  return errors.New("contest sources or rounds duplicate")
 }
 proofByID:=map[[32]byte][]any{}
 for i,proof:=range b.ContestProofs{
  if i>0&&bytes.Compare(b.ContestProofs[i-1].Identity[:],proof.Identity[:])>=0{
   return errors.New("contest proof identities not strictly sorted")
  }
  decoded,err:=codec.StrictDecodePayload(proof.Canonical);if err!=nil{return err}
  fields,ok:=decoded.([]any);if !ok{return errors.New("contest audit proof malformed")}
  if _,duplicate:=proofByID[proof.Identity];duplicate{return errors.New("duplicate contest audit proof")}
  proofByID[proof.Identity]=fields
 }
 for i:=range competitors{
  competitor:=&competitors[i]
  candidate,ok:=proofByID[[32]byte(competitor.candidate)]
  if !ok||len(candidate)!=17{return errors.New("missing contestant candidate audit root")}
  wanted:=[]any{competitor.source[:]}
  checks:=[]struct{name string;valid bool}{
   {"candidate-id",sameCanonical(candidate[0],competitor.candidate[:])},
   {"run",equalBytes(candidate[1],b.RunID[:])},
   {"cycle",equalBytes(candidate[2],b.CycleID[:])},
   {"sources",sameCanonical(candidate[3],wanted)},
   {"producer",candidate[4]=="resource.consume"},
   {"version",candidate[5]==uint64(1)},
   {"role",candidate[6]==uint64(0)},
   {"ordinal",candidate[7]==uint64(0)},
   {"priority",sameCanonical(candidate[14],uint64(0))},
   {"resolver",sameCanonical(candidate[15],allowed["conflict-resolvers-v1"])},
  }
  for _,item:=range checks{if !item.valid{return fmt.Errorf("contest candidate %s does not match registered causal evidence",item.name)}}
  provenance:=[]any{[]any{uint64(5),competitor.source[:],competitor.digest[:]}}
  wantDraft:=[]any{[]any{uint64(0),uint64(0),"resource.consumed",competitor.proposal[8],[]any{}}}
  if !sameCanonical(candidate[8],wantDraft)||!sameCanonical(candidate[9],competitor.proposal[9])||
   !sameCanonical(candidate[10],[]any{})||!sameCanonical(candidate[11],[]any{})||
   !sameCanonical(candidate[13],provenance)||!sameCanonical(candidate[12],[]any{"resource.nonnegative"}){
   return errors.New("contest candidate lacks sealed provenance")
  }
  digest,err:=registeredDigest("cote.csf.digest.commit-candidate","cote.csf.schema.commit-candidate.body",candidate[:16])
  if err!=nil||!equalBytes(candidate[16],digest[:]){return errors.New("contest candidate digest invalid")}
  competitor.candidateBody=candidate
 }
 // Each affordance assessment is itself a signed, content-addressed
 // validation of the admitted ActionProposal. Require both exact roots,
 // not just a proof-list length that could hide arbitrary material.
 validator:=allowed["validators-v1"]
 for _,contender:=range competitors{
  facets:=[]any{
   []any{uint64(0),uint64(0),uint64(2),"physical.possible",[]any{},validator},
   []any{uint64(1),uint64(0),uint64(2),"access.granted",[]any{},validator},
   []any{uint64(2),uint64(0),uint64(0),"resource.available",[]any{},validator},
   []any{uint64(3),uint64(2),uint64(2),"temporal.not-applicable",[]any{},validator},
   []any{uint64(4),uint64(0),uint64(2),"institution.allowed",[]any{},validator},
   []any{uint64(5),uint64(2),uint64(2),"legal.not-applicable",[]any{},validator},
  }
  subject:=[]any{uint64(5),contender.source[:],contender.digest[:]}
  expectedBody:=[]any{subject,facets}
  digest,err:=registeredDigest("cote.csf.digest.affordance-assessment",
   "cote.csf.schema.affordance-assessment.body",expectedBody)
  if err!=nil{return err}
  saved,found:=proofByID[[32]byte(digest)]
  if !found||len(saved)!=3||!sameCanonical(saved[:2],expectedBody)||
   !equalBytes(saved[2],digest[:]){
   return errors.New("contest affordance assessment missing or inconsistent with admitted actor")
  }
 }
 sorted:=[]domain.ID{competitors[0].candidate,competitors[1].candidate}
 sort.Slice(sorted,func(i,j int)bool{return bytes.Compare(sorted[i][:],sorted[j][:])<0})
 conflictID,err:=registeredDigest("cote.csf.id.conflict-set","cote.csf.schema.conflict-set-id-preimage",
  []any{b.CycleID[:],[]any{sorted[0][:],sorted[1][:]},uint64(0),"last.resource"})
 if err!=nil{return err}
 conflict,found:=proofByID[conflictID]
 if !found||len(conflict)!=7||!equalBytes(conflict[0],conflictID[:])||
  !sameCanonical(conflict[1],[]any{sorted[0][:],sorted[1][:]})||
  conflict[2]!=uint64(0)||conflict[3]!="last.resource"||
  !sameCanonical(conflict[4],allowed["validators-v1"])||
  !sameCanonical(conflict[5],allowed["conflict-resolvers-v1"]){
  return errors.New("resource conflict not derived from exact contender set")
 }
 conflictDigest,err:=registeredDigest("cote.csf.digest.conflict-set","cote.csf.schema.conflict-set.body",conflict[:6])
 if err!=nil||!equalBytes(conflict[6],conflictDigest[:]){return errors.New("contest conflict digest invalid")}
 decisionKey:="conflict."+hex.EncodeToString(conflictID[:])
 drawID,err:=registeredDigest("cote.csf.id.rng-draw","cote.csf.schema.rng-draw-id-preimage",
  []any{b.RunID[:],b.CycleID[:],journal[2],"resource",decisionKey,[]any{},"resolve",allowed["rng-v1"]})
 if err!=nil{return err}
 draw,found:=proofByID[drawID]
 if !found||len(draw)!=11||!equalBytes(draw[0],drawID[:])||
  !equalBytes(draw[1],b.RunID[:])||!equalBytes(draw[2],b.CycleID[:])||
  draw[3]!=journal[2]||draw[4]!="resource"||draw[5]!=decisionKey||
  !sameCanonical(draw[6],[]any{})||draw[7]!="resolve"||
  !sameCanonical(draw[8],allowed["rng-v1"]){
  return errors.New("resource RNG proof not bound to pinned conflict")
 }
 drawDigest,err:=registeredDigest("cote.csf.digest.rng-draw","cote.csf.schema.rng-draw.body",draw[:10])
 if err!=nil||!equalBytes(draw[10],drawDigest[:]){return errors.New("resource RNG digest invalid")}
 seed,ok:=genesis[1].([]byte);if !ok||len(seed)!=32{return errors.New("genesis seed unavailable")}
 var worldSeed domain.Digest;copy(worldSeed[:],seed)
 bits,err:=identity.NamedSubstreamV1(worldSeed,"resource",decisionKey,[]any{},"resolve",allowed["rng-v1"].([]any))
 if err!=nil{return err}
 chosen:=int(bits[0]&1)
 result,ok:=draw[9].([]any);if !ok||len(result)!=4{return errors.New("RNG draw result typed value malformed")}
 encoded,ok:=result[2].([]byte);if !ok{return errors.New("RNG draw typed envelope missing")}
 env,err:=codec.StrictDecodeEnvelope(encoded)
 if err!=nil||env.DomainTag!="cote.csf.test.resource-rng-result"||
  env.SchemaID!="cote.csf.schema.typed-value"||env.SchemaVersion!=1||
  env.Value!=uint64(chosen){return errors.New("recorded RNG draw does not reproduce named substream")}
 digest:=identity.OpaqueContentHash(encoded)
 if !equalBytes(result[3],digest[:]){return errors.New("RNG result digest invalid")}
 conflictRef:=[]any{uint64(9),conflictID[:],conflictDigest[:]}
 drawRef:=[]any{uint64(24),drawID[:],drawDigest[:]}
 winners:=0
 var winning competitor
 pairs:=make([]any,0,2)
 for i,competitor:=range competitors{
  expectedID,err:=registeredDigest("cote.csf.id.decision","cote.csf.schema.decision-id-preimage",
   []any{b.CycleID[:],journal[2],competitor.source[:]});if err!=nil{return err}
  decision,err:=strictFull(b.Decisions[i],16);if err!=nil{return err}
  recordHash,err:=registeredDigest("cote.csf.digest.decision-record","cote.csf.schema.decision-record.body",decision[:15])
  if err!=nil{return err}
  subject:=[]any{uint64(0),uint64(0),competitor.source[:],competitor.digest[:]}
  if expectedID!=b.Decisions[i].Identity||!equalBytes(decision[0],expectedID[:])||
   !equalBytes(decision[1],b.RunID[:])||!equalBytes(decision[2],b.CycleID[:])||
   decision[3]!=journal[2]||!sameCanonical(decision[4],subject)||
   !sameCanonical(decision[6],[]any{uint64(1),competitor.candidate[:]})||
   !sameCanonical(decision[8],[]any{conflictRef})||
   !sameCanonical(decision[9],allowed["validators-v1"])||
   !sameCanonical(decision[10],allowed["conflict-resolvers-v1"])||
   !sameCanonical(decision[11],[]any{drawRef})||
   !sameCanonical(decision[13],noRef())||!sameCanonical(decision[14],noRef())||
   !equalBytes(decision[15],recordHash[:]){
   return errors.New("contest DecisionRecord not derived from sealed source/conflict/RNG")
  }
  isWinner:=competitor.candidate==sorted[chosen]
  if isWinner{
   winners++
   if decision[5]!=uint64(0)||decision[7]!="resource.allocated"||
    !sameCanonical(decision[12],[]any{b.Events[0].Identity[:]}){
    return errors.New("randomly selected resource winner not COMMIT")
   }
   winning=competitor
  }else if decision[5]!=uint64(1)||decision[7]!="resource.conflict"||
   !sameCanonical(decision[12],[]any{}){
   return errors.New("resource loser was not rejected")
  }
  list,ok:=journal[16].([]any)
  if !ok||len(list)!=2||!sameCanonical(list[i],expectedID[:]){
   return errors.New("contest terminal decision list order differs from fence")
  }
  pairs=append(pairs,[]any{competitor.source[:],expectedID[:],recordHash[:]})
 }
 if winners!=1{return errors.New("named RNG winner not uniquely settled")}
 expectedPairHash,err:=registeredDigest("cote.csf.digest.decision","cote.csf.schema.decision-pair-list",pairs)
 if err!=nil||!equalBytes(journal[17],expectedPairHash[:]){
  return errors.New("contest decision pair digest wrong")
 }
 draftList,ok:=winning.candidateBody[8].([]any)
 if !ok||len(draftList)!=1{return errors.New("winning candidate has no EventDraft")}
 draft,ok:=draftList[0].([]any);if !ok{return errors.New("winning EventDraft invalid")}
 expectedEvent,err:=settlement.ResourceConsumedEvent(domain.ID(b.RunID),domain.ID(b.CycleID),
  winning.candidate,winning.source,winning.digest,
  prior.Instant,prior.Revision,prior.NextLogicalSequence,draft)
 if err!=nil{return err}
 if b.Events[0].Identity!=expectedEvent.Identity||
  !bytes.Equal(b.Events[0].Canonical,expectedEvent.Canonical){
  return errors.New("winner's event not derived from named RNG candidate")
 }
 roundIDs:=[]domain.ID{competitors[0].round,competitors[1].round}
 sort.Slice(roundIDs,func(i,j int)bool{return bytes.Compare(roundIDs[i][:],roundIDs[j][:])<0})
 for i,round:=range roundIDs{
  var contestant competitor
  if competitors[0].round==round{contestant=competitors[0]}else{contestant=competitors[1]}
  refs:=[]any{[]any{uint64(0),uint64(0),contestant.source[:],contestant.digest[:]}}
  sequence:=prior.NextLogicalSequence+domain.LogicalSequence(i)+1
  if b.Mode==6{
   decoded,err:=codec.StrictDecodePayload(b.Events[i+1].Canonical)
   if err!=nil{return err}
   fields,ok:=decoded.([]any)
   if !ok||len(fields)!=19{return errors.New("contest-system lifecycle Event malformed")}
   value,ok:=fields[6].(uint64)
   if !ok||value<=uint64(prior.NextLogicalSequence){return errors.New("contest-system lifecycle logical sequence invalid")}
   sequence=domain.LogicalSequence(value)
  }
  expected,err:=settlement.ConsumedRoundEvent(domain.ID(b.RunID),domain.ID(b.CycleID),round,
   prior.Instant,prior.Revision,sequence,refs)
  if err!=nil{return err}
  if b.Events[i+1].Identity!=expected.Identity||!bytes.Equal(b.Events[i+1].Canonical,expected.Canonical){
   return errors.New("loser or winner round lifecycle not consumed")
  }
 }
 for i,rawID:=range journal[15].([]any){
  if !sameCanonical(rawID,b.Events[i].Identity[:]){return errors.New("contest event journal ID order invalid")}
 }
 priorStock,err:=reducer.DecodeStock(predecessor[4]);if err!=nil||priorStock.Available!=1{
  return errors.New("contest did not begin with pinned single resource")
 }
 typed,hash,err:=reducer.Consume(predecessor[4].([]any),1)
 if err!=nil||!sameCanonical(revision[4],typed)||
  !equalBytes(revision[5],hash[:])||!equalBytes(journal[9],hash[:])||
  !equalBytes(b.ResultStateHash[:],hash[:]){
  return errors.New("contest state was not derived by registered reducer")
 }
 taskHash,err:=registeredDigest("cote.csf.digest.perception-task-list","cote.csf.schema.perception-task-id-list",journal[18])
 if err!=nil||!equalBytes(journal[19],taskHash[:]){
  return errors.New("secret-only contest must not produce perception tasks")
 }
 if uint64(prior.NextLogicalSequence)>math.MaxUint64-3||next.NextLogicalSequence!=prior.NextLogicalSequence+3{
  return errors.New("contest event sequences invalid")
 }
 return nil
}

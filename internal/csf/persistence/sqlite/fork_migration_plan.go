package sqlite

import (
 "bytes"
 "context"
 "errors"
 "fmt"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/genesis"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
)

// forkMigrationPlan is a proposed, uncommitted child authority set.
// Producing a plan is NOT publication. Every generated record still has to
// pass the unchanged destination guard, transaction and pre-commit lineage.
type forkMigrationPlan struct {
 childGenesis *genesis.Genesis
 migrated []forkGenesisOccurrence
 importBirths []V2CommunicationBirth
 occurrenceRoots []ports.CanonicalRecord
 migratedRounds []forkGenesisRound
 migratedSlots []forkSlotRecord
 historicalRoots []forkHistoricalRoot
 cognitiveActors []forkCognitiveActor
 archived [5][]forkArchiveRecord // populated at the existing post-migration read boundary
 triggerMigration forkTriggerMigration
 triggerManifest []byte
}

// prepareForkMigrationPlan is the read-aware P2 coordinator. The *pure*
// derivations below deliberately accept no Store or context. Existing
// authenticated readers (round history, source triggers and signed owner
// sources) remain adapters for now: they are not mislabeled as pure, and
// retain original read order until independently extracted/proven. Critically,
// no child transaction is opened here and no source database is written.
func (s *Store) prepareForkMigrationPlan(ctx context.Context,
 sourceRun,parentSnapshotID,childRun [32]byte,cut verifiedForkCut,
 origin,childPolicy *policy.Bundle,crossPolicy bool)(forkMigrationPlan,error){
 var zero forkMigrationPlan
 parent,snapshot,lists:=cut.parent,cut.snapshot,cut.pending
 childGenesis,err:=deriveForkChildGenesis(cut,childRun,childPolicy)
 if err!=nil{return zero,err}
 var migrated []forkGenesisOccurrence
 var importBirths []V2CommunicationBirth
 var occurrenceRoots []ports.CanonicalRecord
 archive:=forkHistoryArchive{}
 if crossPolicy{
  for _,name:=range snapshotLedgerNames{
   records,err:=s.snapshotRoundLedgerRecords(ctx,parentSnapshotID,name);if err!=nil{return zero,err}
   archive[name]=records
  }
  if len(lists[0])>0{
   importBirths,err=s.PrepareV2CommunicationBirths(ctx,sourceRun,parentSnapshotID,
    childGenesis,origin,childPolicy)
   if err!=nil{return zero,fmt.Errorf("V1-to-V2 pending transmission migration: %w",err)}
  }
  ordinaryRefs,err:=partitionV2PendingOccurrenceRefs(lists[0],importBirths)
  if err!=nil{return zero,err}
  migrated,err=remintV2CommunicationOccurrences(importBirths)
  if err!=nil{return zero,err}
  occurrenceRoots,err=prepareForkOccurrenceRoots(ctx,s,parent,ordinaryRefs)
  if err!=nil{return zero,err}
  generated,err:=migrateOccurrenceBirthsV2(parent,childGenesis,snapshot,ordinaryRefs,occurrenceRoots,migrated,archive,origin,childPolicy)
  if err!=nil{return zero,fmt.Errorf("V2 occurrence migration: %w",err)}
  migrated=append(migrated,generated...)
 }else{
  migrated,err=migrateGenesisPendingOccurrences(ctx,s,parent,childGenesis,lists[0],origin,childPolicy)
  if err!=nil{return zero,err}
 }
 var migratedRounds []forkGenesisRound
 var migratedSlots []forkSlotRecord
 var historicalRoots []forkHistoricalRoot
 if crossPolicy{
  historicalRoots,err=prepareNativeRoundIntentions(ctx,s,parent,snapshot,archive,origin)
  if err!=nil{return zero,fmt.Errorf("V2 native intention history: %w",err)}
  migratedRounds,migratedSlots,err=migrateRoundHistoryV2(parent,childGenesis,snapshot,archive,origin,childPolicy)
  if err!=nil{return zero,fmt.Errorf("V2 round history migration: %w",err)}
 }else{
  migratedRounds,err=migrateGenesisPendingRounds(ctx,s,parent,childGenesis,lists[4],origin,childPolicy)
  if err!=nil{return zero,fmt.Errorf("fork Genesis round migration: %w",err)}
 }
 var triggerMigration forkTriggerMigration
 var triggerManifest []byte
 if crossPolicy {
  triggerManifest,err=prepareTriggerManifestV2(ctx,s,parent,snapshot,origin)
  if err!=nil{return zero,err}
  triggerMigration,err=migrateTriggerSeedsV2(parent,childGenesis,snapshot,triggerManifest,archive,origin,childPolicy)
 }else{
  triggerMigration,err=migrateGenesisPendingTriggers(ctx,s,parent,childGenesis,origin,childPolicy,
   lists[1],lists[2])
 }
 if err!=nil{return zero,fmt.Errorf("fork trigger migration: %w",err)}

 actors,err:=deriveForkCognitiveActors(cut.epistemic,crossPolicy)
 if err!=nil{return zero,err}
 return forkMigrationPlan{
  cognitiveActors:actors,
  childGenesis:childGenesis,migrated:migrated,importBirths:importBirths,
  occurrenceRoots:occurrenceRoots,migratedRounds:migratedRounds,migratedSlots:migratedSlots,
  historicalRoots:historicalRoots,triggerMigration:triggerMigration,
  triggerManifest:triggerManifest,
 },nil
}


// forkCognitiveActor is an authenticated owner/recipient namespace pair.
// Signed source receipts remain replay-verified in the atomic P3 publisher;
// this P2 projection is only a roster, never owner signing authority.
type forkCognitiveActor struct{
 namespace,id string
}

func deriveForkCognitiveActors(refs []any,crossPolicy bool)([]forkCognitiveActor,error){
 if !crossPolicy{return nil,nil}
 actors:=make([]forkCognitiveActor,0,len(refs))
 seen:=map[string]bool{}
 for _,raw:=range refs{
  entry,ok:=raw.([]any)
  if !ok||len(entry)!=8{return nil,errors.New("V2 cognition checkpoint ref malformed")}
  actor,ok:=entry[0].([]any)
  if !ok||len(actor)!=2{return nil,errors.New("V2 cognition actor malformed")}
  ns,okNS:=actor[0].(string);id,okID:=actor[1].(string)
  if !okNS||!okID||domain.ValidateMachineID(ns)!=nil||domain.ValidateMachineID(id)!=nil{
   return nil,errors.New("V2 cognition actor identifier invalid")
  }
  key:=ns+"\\x00"+id
  if seen[key]{return nil,errors.New("duplicate V2 cognitive actor")}
  seen[key]=true
  actors=append(actors,forkCognitiveActor{namespace:ns,id:id})
 }
 return actors,nil
}

// deriveForkChildGenesis is deterministic: exact verified source bytes and
// pinned child policy in, canonical V2 (or same-policy V1) Genesis out.
func deriveForkChildGenesis(cut verifiedForkCut,childRun [32]byte,childPolicy *policy.Bundle)(*genesis.Genesis,error){
 parent,snapshot,resume,refRaw:=cut.parent,cut.snapshot,cut.resume,cut.reference
 payload:=append([]any(nil),parent.Payload...)
 payload[0]=bytes.Clone(childRun[:])
 payload[3]=int64(resume.SnapshotClock.Instant)
 payload[4]=[]any{uint64(1),refRaw}
 targetID,targetVersion,codecHash,schemaHash,err:=childPolicy.CodecIdentity()
 if err!=nil{return nil,err}
 payload[5]=targetID
 payload[6]=targetVersion
 payload[10]=codecHash[:]
 payload[11]=schemaHash[:]
 payload[16]=snapshot[6]
 payload[17]=snapshot[14]
 canonical,err:=codec.CanonicalBytes("cote.csf.digest.genesis-manifest","cote.csf.schema.genesis-manifest",1,payload)
 if err!=nil{return nil,err}
 childGenesis,err:=genesis.ValidateForkCanonical(canonical,childPolicy);if err!=nil{return nil,err}
 if [32]byte(childGenesis.RunID)!=childRun{return nil,errors.New("fork generated wrong run")}

 return childGenesis,nil
}

// partitionV2PendingOccurrenceRefs preserves the signed source-roster order.
// An in-transit migration consumes exactly one parent occurrence ID, and no
// extra/duplicate imported occurrence may disappear silently.
func partitionV2PendingOccurrenceRefs(pendingRefs []any, births []V2CommunicationBirth)([]any,error){
 importedRefs:=map[string]bool{}
 for _,birth:=range births {
  if importedRefs[string(birth.Parent.OccurrenceID[:])]{return nil,errors.New("duplicate V2 imported communication source")}
  importedRefs[string(birth.Parent.OccurrenceID[:])]=true
 }
 ordinaryRefs:=make([]any,0,len(pendingRefs))
 for _,raw:=range pendingRefs{
  ref,ok:=raw.([]any)
  if !ok||len(ref)!=3||ref[0]!=uint64(6){return nil,errors.New("invalid cross-policy pending occurrence ref")}
  parentID,ok:=ref[1].([]byte)
  if !ok||len(parentID)!=32{return nil,errors.New("invalid cross-policy pending occurrence ID")}
  if importedRefs[string(parentID)]{delete(importedRefs,string(parentID));continue}
  ordinaryRefs=append(ordinaryRefs,raw)
 }
 if len(importedRefs)!=0{return nil,errors.New("in-transit transmission is absent from source checkpoint")}
 return ordinaryRefs,nil
}

// remintV2CommunicationOccurrences turns already authenticated communication
// births into typed pending occurrences. It has no Store, SQL, or side effects.
func remintV2CommunicationOccurrences(births []V2CommunicationBirth)([]forkGenesisOccurrence,error){
 result:=make([]forkGenesisOccurrence,0,len(births))
 for _,birth:=range births {
  value,err:=codec.StrictDecodePayload(birth.ChildOccurrence.Canonical)
  if err!=nil{return nil,err}
  fields,ok:=value.([]any);if !ok||len(fields)!=15{return nil,errors.New("V2 imported occurrence malformed")}
  coordinate,ok:=fields[4].([]any);if !ok||len(coordinate)!=2{return nil,errors.New("V2 migrated coordinate malformed")}
  when,valid:=asSigned64(coordinate[0]);ordinal,validOrdinal:=coordinate[1].(uint64)
  if !valid||!validOrdinal{return nil,errors.New("V2 migrated coordinate invalid")}
  result=append(result,forkGenesisOccurrence{
   ID:birth.ChildOccurrence.Identity,EligibilityInstant:when,
   EligibilityOrdinal:ordinal,Canonical:bytes.Clone(birth.ChildOccurrence.Canonical),
  })
 }
 return result,nil
}

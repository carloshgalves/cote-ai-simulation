package sqlite

import (
 "context"
 "errors"
 "fmt"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/clock"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/genesis"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

// ForkIdleSnapshotV1 starts a new run from a causally verified checkpoint in
// a distinct SQLite workspace. Its bounded V1 migrators reconstruct pending
// Genesis-owned occurrences, initial activations, and UNSTARTED Genesis-owned
// rounds, deriving child identities and sealing all records atomically.
// Dispatches (including revoked), slot responses, Event/Claim-owned work,
// in-transit communication, evolved trigger runtime and cognition checkpoints
// require separate versioned migrators and fail closed; none may be dropped.
// The immutable parental prefix remains archived, never child ledger authority.
func (s *Store) ForkIdleSnapshotV1(ctx context.Context,sourceRun,parentSnapshotID,childRun [32]byte,
 child *Store,b *policy.Bundle)(*genesis.Genesis,error){
 return s.forkSnapshotWithInstalledPolicies(ctx,sourceRun,parentSnapshotID,childRun,child,b,b,false)
}

// ForkIdleSnapshotV1ToV2 is the positive cross-policy bootstrap for verified
// IDLE cuts supported by the registered causal migrators. Parent V1 history is preserved
// byte-for-byte, while child Genesis, clock and future authorities are V2.
// Pending communication, Event/Genesis occurrences and round history are
// authenticated migration births. V2 also imports snapshot-time trigger seeds
// and their pending activation proofs; unsupported source classes fail closed.
func (s *Store) ForkIdleSnapshotV1ToV2(ctx context.Context,sourceRun,parentSnapshotID,childRun [32]byte,
 child *Store,origin,version2 *policy.Bundle)(*genesis.Genesis,error){
 if origin==nil||version2==nil{return nil,errors.New("cross-policy fork requires both installed bundles")}
 id,v,_,_,err:=origin.CodecIdentity()
 if err!=nil||id!=policy.CodecPolicyID||v!=1{return nil,errors.New("cross-policy fork origin must be installed V1")}
 id,v,_,_,err=version2.CodecIdentity()
 if err!=nil||id!=policy.CodecPolicyV2ID||v!=2{return nil,errors.New("cross-policy fork child must be installed V2")}
 historic,err:=version2.ResolveHistoricalOrigin(policy.CodecPolicyID,origin.CodecPolicyHash)
 if err!=nil||historic==version2{return nil,errors.New("V2 lacks exactly pinned V1 origin")}
 return s.forkSnapshotWithInstalledPolicies(ctx,sourceRun,parentSnapshotID,childRun,child,origin,version2,true)
}

func (s *Store) forkSnapshotWithInstalledPolicies(ctx context.Context,sourceRun,parentSnapshotID,childRun [32]byte,
 child *Store,origin,childPolicy *policy.Bundle,crossPolicy bool)(*genesis.Genesis,error){
 cut,err:=s.prepareVerifiedForkCut(ctx,sourceRun,parentSnapshotID,childRun,child,origin,childPolicy,crossPolicy)
 if err!=nil{return nil,err}
 resume,parent,snapshot,epistemic,refRaw:=cut.resume,cut.parent,cut.snapshot,cut.epistemic,cut.reference
 draft,err:=s.prepareForkMigrationDraft(ctx,sourceRun,parentSnapshotID,childRun,cut,origin,childPolicy,crossPolicy)
 if err!=nil{return nil,err}
 // Re-verify and copy the exact parent ledger prefixes at the ORIGINAL
 // read-transaction boundary. Until it succeeds, only a draft exists:
 // no complete plan may be supplied to the child SQL publisher.
 archived,err:=s.verifiedForkArchive(ctx,parentSnapshotID,snapshot,origin)
 if err!=nil{return nil,err}
 plan:=forkMigrationPlan{forkMigrationDraft:draft,archived:archived}
 childGenesis,migrated,importBirths,occurrenceRoots:=plan.childGenesis,plan.migrated,plan.importBirths,plan.occurrenceRoots
 migratedRounds,migratedSlots,historicalRoots:=plan.migratedRounds,plan.migratedSlots,plan.historicalRoots
 triggerMigration,triggerManifest:=plan.triggerMigration,plan.triggerManifest
 tx,err:=child.db.BeginTx(ctx,nil);if err!=nil{return nil,err}
 defer tx.Rollback()
 // A fork never shares mutable authority columns with the source run.
 // Validate the COMPLETE installed schema, then require every known authority
 // to be empty before the first child write. Unknown tables fail closed.
 if err:=ensureForkDestinationEmpty(ctx,tx);err!=nil{return nil,err}
 if _,err:=tx.ExecContext(ctx,"INSERT INTO genesis_records(run_id,genesis_id,digest,canonical) VALUES(?,?,?,?)",
  childRun[:],childGenesis.ID[:],childGenesis.Digest[:],childGenesis.CanonicalBytes);err!=nil{return nil,err}
 if crossPolicy{
  // The signed parent snapshot carries the exact complete cognition roster.
  // Preserve membership under child run/revision zero, while the opaque
  // inline checkpoint itself stays a Genesis-pinned historical owner blob.
  for _,actor:=range plan.cognitiveActors {
   if _,err:=tx.ExecContext(ctx,
    "INSERT INTO cognitive_actor_registry(run_id,actor_namespace,actor_id,registered_revision) VALUES(?,?,?,0)",
    childRun[:],actor.namespace,actor.id);err!=nil{return nil,err}
  }
 if err:=importCognitiveOwnerProofsV2(ctx,s,tx,sourceRun,childRun,epistemic);err!=nil{
  return nil,fmt.Errorf("V2 signed cognitive owner migration: %w",err)
 }
 }
 if _,err:=tx.ExecContext(ctx,
  "INSERT INTO fork_lineage(child_run,parent_run,parent_snapshot_id,origin_policy_hash,reference_canonical,parent_genesis_canonical,parent_snapshot_canonical) "+
  "VALUES(?,?,?,?,?,?,?)",childRun[:],sourceRun[:],parentSnapshotID[:],origin.CodecPolicyHash[:],
  refRaw,parent.CanonicalBytes,resume.Snapshot.Canonical);err!=nil{return nil,err}
 for i,name:=range snapshotLedgerNames{
  for ordinal,record:=range plan.archived[i]{
   if _,err:=tx.ExecContext(ctx,"INSERT INTO fork_history_records(child_run,ledger_name,ordinal,record_id,canonical) VALUES(?,?,?,?,?)",
    childRun[:],name,ordinal,record.id[:],record.canonical);err!=nil{return nil,err}
  }
 }
 for _,root:=range historicalRoots{
  if _,err:=tx.ExecContext(ctx,"INSERT INTO fork_history_roots(child_run,reference_kind,record_id,canonical) VALUES(?,?,?,?)",
   childRun[:],root.Kind,root.Identity[:],root.Canonical);err!=nil{return nil,err}
 }
 emptyClock:=clock.State{Instant:childGenesis.InitialInstant,LastOrdinal:-1}
 clockRaw,err:=clock.Encode(emptyClock);if err!=nil{return nil,err}
 if _,err:=tx.ExecContext(ctx,"INSERT INTO clock_states(run_id,canonical) VALUES(?,?)",childRun[:],clockRaw);err!=nil{return nil,err}
 // Persist child RoundDeclarations with their own canonical seals inside
 // the same all-or-nothing child bootstrap transaction.
 for _,round:=range migratedRounds{
  if _,err:=tx.ExecContext(ctx,"INSERT INTO input_ledger(id,canonical) VALUES(?,?)",
   round.ID[:],round.Canonical);err!=nil{return nil,err}
  if _,err:=tx.ExecContext(ctx,
   "INSERT INTO round_integrity_seals(id,run_id,canonical_hash) VALUES(?,?,?)",
   round.ID[:],childRun[:],round.Seal[:]);err!=nil{return nil,err}
 }
 if err:=publishRoundHistoryV2(ctx,tx,migratedSlots);err!=nil{return nil,err}
 for _,record:=range occurrenceRoots{
  if _,err:=tx.ExecContext(ctx,"INSERT INTO fork_occurrence_imports(child_run,parent_occurrence_id,canonical) VALUES(?,?,?)",childRun[:],record.Identity[:],record.Canonical);err!=nil{return nil,err}
 }
 for _,o:=range migrated{
  if _,err:=tx.ExecContext(ctx,"INSERT INTO scheduled_occurrences "+
   "(id,run_id,eligibility_instant,eligibility_ordinal,lifecycle,canonical) VALUES(?,?,?,?,0,?)",
   o.ID[:],childRun[:],o.EligibilityInstant,o.EligibilityOrdinal,o.Canonical);err!=nil{return nil,err}
 }
 if err:=ensureGenesisScheduleSeal(ctx,tx,childRun);err!=nil{return nil,err}
 world:=childGenesis.Payload[16].([]any)
 hash,ok:=world[3].([]byte);if !ok||len(hash)!=32{return nil,errors.New("fork world state hash missing")}
 revisionZero,err:=codec.CanonicalPayload([]any{uint64(0),world,hash,uint64(0),world,hash})
 if err!=nil{return nil,err}
 if _,err:=tx.ExecContext(ctx,"INSERT INTO world_revisions(revision,state_hash,canonical) VALUES(?,?,?)",
  0,hash,revisionZero);err!=nil{return nil,err}
 // All supported pending triggers are migrated and sealed atomically.
 if triggerManifest!=nil{
  seal:=identity.OpaqueContentHash(triggerManifest)
  if _,err:=tx.ExecContext(ctx,"INSERT INTO fork_trigger_imports(child_run,canonical,canonical_hash) VALUES(?,?,?)",childRun[:],triggerManifest,seal[:]);err!=nil{return nil,err}
 }
 for _,def:=range triggerMigration.Definitions{
  if _,err:=tx.ExecContext(ctx,
   "INSERT INTO trigger_definitions(definition_id,run_id,genesis_id,genesis_digest,content_digest,canonical) VALUES(?,?,?,?,?,?)",
   def.ID[:],childRun[:],childGenesis.ID[:],childGenesis.Digest[:],def.ContentDigest,def.Canonical);err!=nil{return nil,err}
  if _,err:=tx.ExecContext(ctx,
   "INSERT INTO trigger_runtime(definition_id,run_id,canonical) VALUES(?,?,?)",
   def.ID[:],childRun[:],def.Runtime);err!=nil{return nil,err}
 }
 for _,act:=range triggerMigration.Activations{
  if _,err:=tx.ExecContext(ctx,
   "INSERT INTO trigger_activations(id,run_id,eligibility_instant,eligibility_ordinal,lifecycle,canonical) VALUES(?,?,?,?,0,?)",
   act.ID[:],childRun[:],act.Instant,act.Ordinal,act.Canonical);err!=nil{return nil,err}
 }
 if _,err:=tx.ExecContext(ctx,"INSERT INTO trigger_registry_seals(run_id,genesis_id,genesis_digest,registry_hash) VALUES(?,?,?,?)",
  childRun[:],childGenesis.ID[:],childGenesis.Digest[:],triggerMigration.Seal[:]);err!=nil{return nil,err}
 if len(importBirths)>0 {
  if err:=publishV2CommunicationBirths(ctx,tx,childRun,importBirths,
   sourceRun,parentSnapshotID,origin,childGenesis);err!=nil{return nil,err}
 }
 if err:=validateForkLineageTx(ctx,tx,childGenesis,childPolicy);err!=nil{return nil,err}
 if err:=tx.Commit();err!=nil{return nil,err}
 return childGenesis,nil
}

// Compile-time guarantees for the run identity and the parent/child
// responsibility boundary; no shared world truth authority is imported.
var _ domain.ID = domain.ID{}

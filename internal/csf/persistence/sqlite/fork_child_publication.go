package sqlite

import (
 "context"
 "errors"
 "fmt"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/clock"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

// publishForkChildTransactional is the sole owner of the CHILD write
// transaction. The parent remains read-only. The complete typed plan must
// already contain all five verified archive prefixes before this entrypoint.
//
// BEGIN -> complete destination guard -> ordered INSERTs / seals ->
// validateForkLineageTx -> COMMIT. Any error rolls back ALL child authorities,
// including rows created by helper publishers; no Genesis-only partial fork.
//
// Historical owner signatures are intentionally looked up in the parent at
// their original point inside the child transaction; moving them to an
// earlier cut would change concurrent-source semantics (P2/P3 boundary).
// This method never writes to the parent or reissues external work.
func (child *Store) publishForkChildTransactional(ctx context.Context,source *Store,
 sourceRun,parentSnapshotID,childRun [32]byte,cut verifiedForkCut,
 plan forkMigrationPlan,origin,childPolicy *policy.Bundle,crossPolicy bool)error{
 resume,parent,epistemic,refRaw:=cut.resume,cut.parent,cut.epistemic,cut.reference
 childGenesis,migrated,importBirths,occurrenceRoots:=plan.childGenesis,plan.migrated,plan.importBirths,plan.occurrenceRoots
 migratedRounds,migratedSlots,historicalRoots:=plan.migratedRounds,plan.migratedSlots,plan.historicalRoots
 triggerMigration,triggerManifest:=plan.triggerMigration,plan.triggerManifest
 tx,err:=child.db.BeginTx(ctx,nil);if err!=nil{return err}
 defer tx.Rollback()
 // A fork never shares mutable authority columns with the source run.
 // Validate the COMPLETE installed schema, then require every known authority
 // to be empty before the first child write. Unknown tables fail closed.
 if err:=ensureForkDestinationEmpty(ctx,tx);err!=nil{return err}
 if _,err:=tx.ExecContext(ctx,"INSERT INTO genesis_records(run_id,genesis_id,digest,canonical) VALUES(?,?,?,?)",
  childRun[:],childGenesis.ID[:],childGenesis.Digest[:],childGenesis.CanonicalBytes);err!=nil{return err}
 if crossPolicy{
  // The signed parent snapshot carries the exact complete cognition roster.
  // Preserve membership under child run/revision zero, while the opaque
  // inline checkpoint itself stays a Genesis-pinned historical owner blob.
  for _,actor:=range plan.cognitiveActors {
   if _,err:=tx.ExecContext(ctx,
    "INSERT INTO cognitive_actor_registry(run_id,actor_namespace,actor_id,registered_revision) VALUES(?,?,?,0)",
    childRun[:],actor.namespace,actor.id);err!=nil{return err}
  }
 if err:=importCognitiveOwnerProofsV2(ctx,source,tx,sourceRun,childRun,epistemic);err!=nil{
  return fmt.Errorf("V2 signed cognitive owner migration: %w",err)
 }
 }
 if _,err:=tx.ExecContext(ctx,
  "INSERT INTO fork_lineage(child_run,parent_run,parent_snapshot_id,origin_policy_hash,reference_canonical,parent_genesis_canonical,parent_snapshot_canonical) "+
  "VALUES(?,?,?,?,?,?,?)",childRun[:],sourceRun[:],parentSnapshotID[:],origin.CodecPolicyHash[:],
  refRaw,parent.CanonicalBytes,resume.Snapshot.Canonical);err!=nil{return err}
 for i,name:=range snapshotLedgerNames{
  for ordinal,record:=range plan.archived[i]{
   if _,err:=tx.ExecContext(ctx,"INSERT INTO fork_history_records(child_run,ledger_name,ordinal,record_id,canonical) VALUES(?,?,?,?,?)",
    childRun[:],name,ordinal,record.id[:],record.canonical);err!=nil{return err}
  }
 }
 for _,root:=range historicalRoots{
  if _,err:=tx.ExecContext(ctx,"INSERT INTO fork_history_roots(child_run,reference_kind,record_id,canonical) VALUES(?,?,?,?)",
   childRun[:],root.Kind,root.Identity[:],root.Canonical);err!=nil{return err}
 }
 emptyClock:=clock.State{Instant:childGenesis.InitialInstant,LastOrdinal:-1}
 clockRaw,err:=clock.Encode(emptyClock);if err!=nil{return err}
 if _,err:=tx.ExecContext(ctx,"INSERT INTO clock_states(run_id,canonical) VALUES(?,?)",childRun[:],clockRaw);err!=nil{return err}
 // Persist child RoundDeclarations with their own canonical seals inside
 // the same all-or-nothing child bootstrap transaction.
 for _,round:=range migratedRounds{
  if _,err:=tx.ExecContext(ctx,"INSERT INTO input_ledger(id,canonical) VALUES(?,?)",
   round.ID[:],round.Canonical);err!=nil{return err}
  if _,err:=tx.ExecContext(ctx,
   "INSERT INTO round_integrity_seals(id,run_id,canonical_hash) VALUES(?,?,?)",
   round.ID[:],childRun[:],round.Seal[:]);err!=nil{return err}
 }
 if err:=publishRoundHistoryV2(ctx,tx,migratedSlots);err!=nil{return err}
 for _,record:=range occurrenceRoots{
  if _,err:=tx.ExecContext(ctx,"INSERT INTO fork_occurrence_imports(child_run,parent_occurrence_id,canonical) VALUES(?,?,?)",childRun[:],record.Identity[:],record.Canonical);err!=nil{return err}
 }
 for _,o:=range migrated{
  if _,err:=tx.ExecContext(ctx,"INSERT INTO scheduled_occurrences "+
   "(id,run_id,eligibility_instant,eligibility_ordinal,lifecycle,canonical) VALUES(?,?,?,?,0,?)",
   o.ID[:],childRun[:],o.EligibilityInstant,o.EligibilityOrdinal,o.Canonical);err!=nil{return err}
 }
 if err:=ensureGenesisScheduleSeal(ctx,tx,childRun);err!=nil{return err}
 world,ok:=childGenesis.Payload[16].([]any)
 if !ok||len(world)!=4{return errors.New("fork world state projection malformed")}
 hash,ok:=world[3].([]byte);if !ok||len(hash)!=32{return errors.New("fork world state hash missing")}
 revisionZero,err:=codec.CanonicalPayload([]any{uint64(0),world,hash,uint64(0),world,hash})
 if err!=nil{return err}
 if _,err:=tx.ExecContext(ctx,"INSERT INTO world_revisions(revision,state_hash,canonical) VALUES(?,?,?)",
  0,hash,revisionZero);err!=nil{return err}
 // All supported pending triggers are migrated and sealed atomically.
 if triggerManifest!=nil{
  seal:=identity.OpaqueContentHash(triggerManifest)
  if _,err:=tx.ExecContext(ctx,"INSERT INTO fork_trigger_imports(child_run,canonical,canonical_hash) VALUES(?,?,?)",childRun[:],triggerManifest,seal[:]);err!=nil{return err}
 }
 for _,def:=range triggerMigration.Definitions{
  if _,err:=tx.ExecContext(ctx,
   "INSERT INTO trigger_definitions(definition_id,run_id,genesis_id,genesis_digest,content_digest,canonical) VALUES(?,?,?,?,?,?)",
   def.ID[:],childRun[:],childGenesis.ID[:],childGenesis.Digest[:],def.ContentDigest,def.Canonical);err!=nil{return err}
  if _,err:=tx.ExecContext(ctx,
   "INSERT INTO trigger_runtime(definition_id,run_id,canonical) VALUES(?,?,?)",
   def.ID[:],childRun[:],def.Runtime);err!=nil{return err}
 }
 for _,act:=range triggerMigration.Activations{
  if _,err:=tx.ExecContext(ctx,
   "INSERT INTO trigger_activations(id,run_id,eligibility_instant,eligibility_ordinal,lifecycle,canonical) VALUES(?,?,?,?,0,?)",
   act.ID[:],childRun[:],act.Instant,act.Ordinal,act.Canonical);err!=nil{return err}
 }
 if _,err:=tx.ExecContext(ctx,"INSERT INTO trigger_registry_seals(run_id,genesis_id,genesis_digest,registry_hash) VALUES(?,?,?,?)",
  childRun[:],childGenesis.ID[:],childGenesis.Digest[:],triggerMigration.Seal[:]);err!=nil{return err}
 if len(importBirths)>0 {
  if err:=publishV2CommunicationBirths(ctx,tx,childRun,importBirths,
   sourceRun,parentSnapshotID,origin,childGenesis);err!=nil{return err}
 }
 if err:=validateForkLineageTx(ctx,tx,childGenesis,childPolicy);err!=nil{return err}
 if err:=tx.Commit();err!=nil{return err}
 return nil
}

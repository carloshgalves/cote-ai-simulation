package sqlite

import (
 "bytes"
 "context"
 "database/sql"
 "errors"
 "fmt"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/clock"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/fork"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/genesis"
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
// IDLE cuts WITHOUT unsettled migration units. Parent V1 history is preserved
// byte-for-byte, while child Genesis, clock and future authorities are V2.
// The in-transit path requires an additional migration birth executor and is
// intentionally not implicitly enabled by this bounded operation.
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
 if s==nil||child==nil||origin==nil||childPolicy==nil||s==child{return nil,errors.New("fork requires distinct parent/child stores and policy")}
 if sourceRun==childRun||childRun==([32]byte{}){return nil,errors.New("fork must allocate a new nonzero run ID")}
 resume,err:=s.VerifySnapshotV1(ctx,sourceRun,parentSnapshotID,origin)
 if err!=nil{return nil,fmt.Errorf("fork source checkpoint replay: %w",err)}
 if resume.SnapshotControlStatus!=0{return nil,errors.New("fork origin checkpoint must be IDLE")}
 parent,err:=s.ValidatedGenesis(ctx,sourceRun,origin)
 if err!=nil{return nil,err}
 if len(parent.Payload[4].([]any))!=1{
  return nil,errors.New("recursive fork origin requires a transitive lineage archive")
 }
 snapRaw,err:=codec.StrictDecodePayload(resume.Snapshot.Canonical)
 if err!=nil{return nil,err}
 snapshot,ok:=snapRaw.([]any);if !ok||len(snapshot)!=19{return nil,errors.New("source snapshot schema invalid")}
 epistemic,ok:=snapshot[14].([]any)
 if !ok{return nil,errors.New("fork epistemic snapshot roster malformed")}
 if len(epistemic)>0&&!crossPolicy{
  return nil,errors.New("V1 fork requires versioned migration of opaque cognitive checkpoints")
 }
 // V2 imports only exact inlined, signed parent checkpoint bytes. The
 // ownership/eligibility roster is reconstructed below, and lineage replay
 // binds it to the immutable archived parent checkpoint.
 if crossPolicy{
  for _,raw:=range epistemic{
   entry,ok:=raw.([]any)
   if !ok||len(entry)!=8{return nil,errors.New("V2 cognition checkpoint ref malformed")}
   inline,ok:=entry[3].([]any)
   if !ok||len(inline)!=2||inline[0]!=uint64(1){return nil,errors.New("V2 cannot migrate external cognition checkpoint")}
  }
 }
 typed,ok:=snapshot[11].([]any);if !ok||len(typed)!=4{return nil,errors.New("source pending envelope absent")}
 penv,err:=codec.StrictDecodeEnvelope(typed[2].([]byte));if err!=nil{return nil,err}
 lists,ok:=penv.Value.([]any);if !ok||len(lists)!=10{return nil,errors.New("unsupported pending-state schema")}
 if crossPolicy {
  // The only currently admitted positive V2 migration unit is a complete
  // in-transit communication (pending occurrence and its input ref).
  // All other live partitions, including dispatches, trigger runtimes,
  // perception tasks and unfinished rounds, still fail closed.
  for i,raw:=range lists {
   if i==0||i==3||i==6||i==7||i==9{continue} // completed perception receipts remain archived
   items,ok:=raw.([]any)
   if !ok||len(items)!=0{return nil,fmt.Errorf("V1-to-V2 fork needs migrator for pending partition %d",i)}
  }
 }
 for i,raw:=range lists{
  items,ok:=raw.([]any)
  if !ok{return nil,fmt.Errorf("fork pending partition %d is malformed",i)}
  // Latest SourceClosure, settled slot responses and completed perception
  // receipts are historical proofs, not pending work. Keep them in the
  // frozen parent prefix rather than transplanting their run-bound IDs.
  if i==3||i==6||i==9||i==0||i==1||i==2||i==4{continue}
  // An explicitly migrated genesis schedule contributes one pending input
  // ref (kind=2) for each immutable occurrence. Unknown source kinds fail
  // closed, rather than discarding the parent pending work.
  if i==7{
   pending,ok:=lists[0].([]any)
   if !ok{return nil,errors.New("fork pending schedule roster malformed")}
   activations,ok:=lists[2].([]any)
   if !ok{return nil,errors.New("fork activation roster malformed")}
   rounds,ok:=lists[4].([]any)
   if !ok||len(items)!=len(pending)+len(activations)+len(rounds){
    return nil,errors.New("fork pending input roster not fully supported")
   }
   expected:=map[string]bool{}
   for _,v:=range pending{
    ref,ok:=v.([]any);if !ok||len(ref)!=3{return nil,errors.New("fork genesis ref malformed")}
    id,ok:=ref[1].([]byte);if !ok||len(id)!=32{return nil,errors.New("fork genesis ref missing ID")}
    expected[string(id)]=true
   }
   for _,v:=range activations{
    ref,ok:=v.([]any)
    if !ok||len(ref)!=3||ref[0]!=uint64(7){return nil,errors.New("fork activation ref malformed")}
    id,ok:=ref[1].([]byte)
    if !ok||len(id)!=32||expected[string(id)]{return nil,errors.New("fork duplicate activation ref")}
    expected[string(id)]=true
   }
   for _,v:=range rounds{
    ref,ok:=v.([]any)
    if !ok||len(ref)!=3||ref[0]!=uint64(3){return nil,errors.New("fork pending round ref malformed")}
    id,ok:=ref[1].([]byte)
    if !ok||len(id)!=32||expected[string(id)]{return nil,errors.New("fork duplicate round input ref")}
    expected[string(id)]=true
   }
   for _,v:=range items{
    ref,ok:=v.([]any);if !ok||len(ref)!=3||
     (ref[0]!=uint64(2)&&ref[0]!=uint64(3)&&ref[0]!=uint64(4)){
     return nil,errors.New("fork cannot rebind unsupported pending input")
    }
    id,ok:=ref[1].([]byte)
    if !ok||len(id)!=32||!expected[string(id)]{return nil,errors.New("fork pending input does not match Genesis migration sources")}
    delete(expected,string(id))
   }
   if len(expected)!=0{return nil,errors.New("fork pending input omits a Genesis source")}
   continue
  }
  if len(items)!=0{
   return nil,fmt.Errorf("fork requires a versioned migrator for pending partition %d",i)
  }
 }
 // Every source declared in the historical input prefix must have a
 // final SourceClosure. A source declared but never closed is NOT represented
 // by lists[3], so inspecting closure refs alone would silently discard it.
 finalSourceNames:=map[string]bool{}
 // Every source cursor inherited only as archived history must be final:
 // otherwise a child would silently drop unfinished exogenous input ingress.
 for _,item:=range lists[3].([]any){
  ref,ok:=item.([]any)
  if !ok||len(ref)!=3||ref[0]!=uint64(2){return nil,errors.New("fork source closure reference malformed")}
  id,ok:=ref[1].([]byte)
  if !ok||len(id)!=32{return nil,errors.New("fork source closure ID malformed")}
  var canonical []byte
  if err:=s.db.QueryRowContext(ctx,"SELECT canonical FROM input_ledger WHERE id=?",id).Scan(&canonical);err!=nil{
   return nil,fmt.Errorf("fork source closure absent from origin history: %w",err)
  }
  decoded,err:=codec.StrictDecodePayload(canonical);if err!=nil{return nil,err}
  record,ok:=decoded.([]any)
  if !ok||len(record)!=7||record[5]!=true||!equalBytes(record[0],id)||
   !equalBytes(record[1],sourceRun[:]){
   return nil,errors.New("fork would lose an unfinished or invalid source cursor")
  }
  name,ok:=record[2].(string);if !ok||name==""{return nil,errors.New("fork source id malformed")}
  finalSourceNames[name]=true
 }
 sourceRows,err:=s.db.QueryContext(ctx,
  "SELECT t.canonical FROM snapshot_prefix_members m "+
   "JOIN input_ledger t ON t.id=m.record_id WHERE m.snapshot_id=? "+
   "AND m.ledger_name='input_ledger' ORDER BY m.ordinal",parentSnapshotID[:])
 if err!=nil{return nil,err}
 for sourceRows.Next(){
  var canonical []byte
  if err:=sourceRows.Scan(&canonical);err!=nil{sourceRows.Close();return nil,err}
  record,err:=codec.StrictDecodePayload(canonical)
  if err!=nil{continue} // an enveloped SourceIngressReceipt cannot declare a new source
  fields,ok:=record.([]any)
  if !ok||len(fields)!=3||fields[0]!="source-declaration-v1"{continue}
  sourceID,ok:=fields[2].(string)
  if !ok||!finalSourceNames[sourceID]{
   sourceRows.Close();return nil,errors.New("fork has a declared but non-final exogenous source")
  }
 }
 if err:=sourceRows.Err();err!=nil{sourceRows.Close();return nil,err}
 if err:=sourceRows.Close();err!=nil{return nil,err}
 // Parent's registered snapshot body hash is distinct from the exact-byte
 // digest in CSFHREF. The latter binds immutable archive content across
 // origin/child policy versions without rehashing historical identities.
 href,err:=fork.NewSnapshotReference(origin,resume.Snapshot.Canonical)
 if err!=nil{return nil,err}
 refRaw,err:=fork.Encode(href);if err!=nil{return nil,err}
 // Always resolve the immutable origin by its own id+hash before
 // interpreting its checkpoint, even for the same-policy V1 subset.
 installed,err:=fork.NewInstalledPolicyRegistry(origin)
 if err!=nil{return nil,err}
 _,verifiedOrigin,err:=fork.VerifyOriginWithResolver(ctx,refRaw,resume.Snapshot.Canonical,installed)
 if err!=nil{return nil,err}
 if verifiedOrigin!=origin{return nil,errors.New("fork origin policy unexpectedly switched during validation")}
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
 var migrated []forkGenesisOccurrence
 var importBirths []V2CommunicationBirth
 if crossPolicy{
  importBirths,err=s.PrepareV2CommunicationBirths(ctx,sourceRun,parentSnapshotID,
   childGenesis,origin,childPolicy)
  if err!=nil{return nil,fmt.Errorf("V1-to-V2 pending transmission migration: %w",err)}
  if len(importBirths)!=len(lists[0].([]any)){
   return nil,errors.New("V2 communication migration omits pending non-communication occurrence")
  }
  for _,birth:=range importBirths {
   value,err:=codec.StrictDecodePayload(birth.ChildOccurrence.Canonical)
   if err!=nil{return nil,err}
   fields,ok:=value.([]any);if !ok||len(fields)!=15{return nil,errors.New("V2 imported occurrence malformed")}
   coordinate,ok:=fields[4].([]any);if !ok||len(coordinate)!=2{return nil,errors.New("V2 migrated coordinate malformed")}
   when,valid:=asSigned64(coordinate[0]);ordinal,validOrdinal:=coordinate[1].(uint64)
   if !valid||!validOrdinal{return nil,errors.New("V2 migrated coordinate invalid")}
   migrated=append(migrated,forkGenesisOccurrence{
    ID:birth.ChildOccurrence.Identity,EligibilityInstant:when,
    EligibilityOrdinal:ordinal,Canonical:birth.ChildOccurrence.Canonical,
   })
  }
 }else{
  migrated,err=migrateGenesisPendingOccurrences(ctx,s,parent,childGenesis,lists[0].([]any),origin)
  if err!=nil{return nil,err}
 }
 migratedRounds,err:=migrateGenesisPendingRounds(ctx,s,parent,childGenesis,lists[4].([]any),origin)
 if err!=nil{return nil,fmt.Errorf("fork Genesis round migration: %w",err)}
 triggerMigration,err:=migrateGenesisPendingTriggers(ctx,s,parent,childGenesis,origin,
  lists[1].([]any),lists[2].([]any))
 if err!=nil{return nil,fmt.Errorf("fork trigger migration: %w",err)}
 // The parent workspace must remain verifiably immutable: copy the exact
 // members under a read transaction instead of trusting a worker's proposal.
 originTx,err:=s.db.BeginTx(ctx,&sql.TxOptions{ReadOnly:true});if err!=nil{return nil,err}
 defer originTx.Rollback()
 cursors:=snapshot[12].([]any)
 type member struct{id,canonical []byte}
 archived:=make([][]member,len(snapshotLedgerNames))
 for i,name:=range snapshotLedgerNames{
  proof,err:=snapshotVerifiedPrefix(ctx,originTx,parentSnapshotID,name,origin);if err!=nil{return nil,err}
  if !sameCanonical(cursors[i],proof){return nil,errors.New("origin prefix changed while preparing fork")}
  rows,err:=originTx.QueryContext(ctx,
   "SELECT m.ordinal,m.record_id,t.canonical FROM snapshot_prefix_members m "+
    "JOIN "+name+" t ON t.id=m.record_id WHERE m.snapshot_id=? AND m.ledger_name=? ORDER BY m.ordinal",
   parentSnapshotID[:],name)
  if err!=nil{return nil,err}
  for rows.Next(){
   var ordinal int64
   var id,raw []byte
   if err:=rows.Scan(&ordinal,&id,&raw);err!=nil{rows.Close();return nil,err}
   if ordinal!=int64(len(archived[i]))||len(id)!=32{rows.Close();return nil,errors.New("origin archive order malformed")}
   archived[i]=append(archived[i],member{bytes.Clone(id),bytes.Clone(raw)})
  }
  if err:=rows.Err();err!=nil{rows.Close();return nil,err}
  if err:=rows.Close();err!=nil{return nil,err}
 }
 if err:=originTx.Commit();err!=nil{return nil,err}
 tx,err:=child.db.BeginTx(ctx,nil);if err!=nil{return nil,err}
 defer tx.Rollback()
 // A fork never shares mutable authority columns with the source run.
 for _,name:=range []string{"genesis_records","world_revisions","clock_states",
  "input_ledger","decision_ledger","event_store","evidence_ledger","causal_outbox",
  "input_source_cursors","source_ingress_receipts","input_slot_responses","round_integrity_seals",
  "cognitive_actor_registry","snapshot_cognitive_actors","evidence_routes",
  "communication_claims","communication_transmissions","fork_communication_imports","cycle_control_state",
  "committed_cycles","fork_lineage","fork_history_records","snapshots","snapshot_seals",
  "snapshot_prefix_members","genesis_schedule_seals","scheduled_occurrences",
  "trigger_registry_seals","trigger_definitions","trigger_runtime","trigger_activations"}{
  var count int
  if err:=tx.QueryRowContext(ctx,"SELECT count(*) FROM "+name).Scan(&count);err!=nil{return nil,err}
  if count!=0{return nil,fmt.Errorf("fork destination contains previous authority in %s",name)}
 }
 if _,err:=tx.ExecContext(ctx,"INSERT INTO genesis_records(run_id,genesis_id,digest,canonical) VALUES(?,?,?,?)",
  childRun[:],childGenesis.ID[:],childGenesis.Digest[:],childGenesis.CanonicalBytes);err!=nil{return nil,err}
 if crossPolicy{
  // The signed parent snapshot carries the exact complete cognition roster.
  // Preserve membership under child run/revision zero, while the opaque
  // inline checkpoint itself stays a Genesis-pinned historical owner blob.
  for _,raw:=range epistemic {
   entry:=raw.([]any)
   actor,ok:=entry[0].([]any)
   if !ok||len(actor)!=2{return nil,errors.New("V2 cognition actor malformed")}
   ns,okNS:=actor[0].(string);id,okID:=actor[1].(string)
   if !okNS||!okID||domain.ValidateMachineID(ns)!=nil||
    domain.ValidateMachineID(id)!=nil{
    return nil,errors.New("V2 cognition actor identifier invalid")
   }
   if _,err:=tx.ExecContext(ctx,
    "INSERT INTO cognitive_actor_registry(run_id,actor_namespace,actor_id,registered_revision) VALUES(?,?,?,0)",
    childRun[:],ns,id);err!=nil{return nil,err}
  }
 }
 if _,err:=tx.ExecContext(ctx,
  "INSERT INTO fork_lineage(child_run,parent_run,parent_snapshot_id,origin_policy_hash,reference_canonical,parent_genesis_canonical,parent_snapshot_canonical) "+
  "VALUES(?,?,?,?,?,?,?)",childRun[:],sourceRun[:],parentSnapshotID[:],origin.CodecPolicyHash[:],
  refRaw,parent.CanonicalBytes,resume.Snapshot.Canonical);err!=nil{return nil,err}
 for i,name:=range snapshotLedgerNames{
  for ordinal,record:=range archived[i]{
   if _,err:=tx.ExecContext(ctx,"INSERT INTO fork_history_records(child_run,ledger_name,ordinal,record_id,canonical) VALUES(?,?,?,?,?)",
    childRun[:],name,ordinal,record.id,record.canonical);err!=nil{return nil,err}
  }
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

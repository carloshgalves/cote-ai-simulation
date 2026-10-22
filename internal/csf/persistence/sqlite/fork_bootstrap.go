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
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

// ForkIdleSnapshotV1 starts a different, initially empty run from an archived,
// causally verified snapshot in a different SQLite workspace. The V1 safe
// subset requires no pending schedule, runtime, source closure, round, dispatch,
// or perception completion to migrate. Unsupported live cutover fails closed
// instead of silently dropping work or reopening final exogenous sources.
// New post-fork inputs/commits remain exclusively in the child's five ledgers.
func (s *Store) ForkIdleSnapshotV1(ctx context.Context,sourceRun,parentSnapshotID,childRun [32]byte,
 child *Store,b *policy.Bundle)(*genesis.Genesis,error){
 if s==nil||child==nil||b==nil||s==child{return nil,errors.New("fork requires distinct parent/child stores and policy")}
 if sourceRun==childRun||childRun==([32]byte{}){return nil,errors.New("fork must allocate a new nonzero run ID")}
 resume,err:=s.VerifySnapshotV1(ctx,sourceRun,parentSnapshotID,b)
 if err!=nil{return nil,fmt.Errorf("fork source checkpoint replay: %w",err)}
 if resume.SnapshotControlStatus!=0{return nil,errors.New("fork origin checkpoint must be IDLE")}
 parent,err:=s.ValidatedGenesis(ctx,sourceRun,b)
 if err!=nil{return nil,err}
 if len(parent.Payload[4].([]any))!=1{
  return nil,errors.New("recursive fork origin requires a transitive lineage archive")
 }
 snapRaw,err:=codec.StrictDecodePayload(resume.Snapshot.Canonical)
 if err!=nil{return nil,err}
 snapshot,ok:=snapRaw.([]any);if !ok||len(snapshot)!=19{return nil,errors.New("source snapshot schema invalid")}
 typed,ok:=snapshot[11].([]any);if !ok||len(typed)!=4{return nil,errors.New("source pending envelope absent")}
 penv,err:=codec.StrictDecodeEnvelope(typed[2].([]byte));if err!=nil{return nil,err}
 lists,ok:=penv.Value.([]any);if !ok||len(lists)!=10{return nil,errors.New("unsupported pending-state schema")}
 for i,raw:=range lists{
  items,ok:=raw.([]any)
  if !ok{return nil,fmt.Errorf("fork pending partition %d is malformed",i)}
  // Latest SourceClosure, settled slot responses and completed perception
  // receipts are historical proofs, not pending work. Keep them in the
  // frozen parent prefix rather than transplanting their run-bound IDs.
  if i==3||i==6||i==9||i==0||i==1||i==2{continue}
  // An explicitly migrated genesis schedule contributes one pending input
  // ref (kind=2) for each immutable occurrence. Unknown source kinds fail
  // closed, rather than discarding the parent pending work.
  if i==7{
   pending,ok:=lists[0].([]any)
   if !ok||len(items)!=len(pending){return nil,errors.New("fork pending input roster not fully supported")}
   expected:=map[string]bool{}
   for _,v:=range pending{
    ref,ok:=v.([]any);if !ok||len(ref)!=3{return nil,errors.New("fork genesis ref malformed")}
    id,ok:=ref[1].([]byte);if !ok||len(id)!=32{return nil,errors.New("fork genesis ref missing ID")}
    expected[string(id)]=true
   }
   for _,v:=range items{
    ref,ok:=v.([]any);if !ok||len(ref)!=3||ref[0]!=uint64(2){
     return nil,errors.New("fork cannot rebind non-genesis pending input")
    }
    id,ok:=ref[1].([]byte)
    if !ok||len(id)!=32||!expected[string(id)]{return nil,errors.New("fork pending input does not match genesis schedule")}
   }
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
 href,err:=fork.NewSnapshotReference(b,resume.Snapshot.Canonical)
 if err!=nil{return nil,err}
 refRaw,err:=fork.Encode(href);if err!=nil{return nil,err}
 if err:=fork.VerifyOriginSnapshot(href,resume.Snapshot.Canonical,b);err!=nil{return nil,err}
 payload:=append([]any(nil),parent.Payload...)
 payload[0]=bytes.Clone(childRun[:])
 payload[3]=int64(resume.SnapshotClock.Instant)
 payload[4]=[]any{uint64(1),refRaw}
 payload[16]=snapshot[6]
 payload[17]=snapshot[14]
 canonical,err:=codec.CanonicalBytes("cote.csf.digest.genesis-manifest","cote.csf.schema.genesis-manifest",1,payload)
 if err!=nil{return nil,err}
 childGenesis,err:=genesis.ValidateForkCanonical(canonical,b);if err!=nil{return nil,err}
 if [32]byte(childGenesis.RunID)!=childRun{return nil,errors.New("fork generated wrong run")}
 migrated,err:=migrateGenesisPendingOccurrences(ctx,s,parent,childGenesis,lists[0].([]any),b)
 if err!=nil{return nil,err}
 triggerMigration,err:=migrateGenesisPendingTriggers(ctx,s,parent,childGenesis,b,
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
  proof,err:=snapshotVerifiedPrefix(ctx,originTx,parentSnapshotID,name,b);if err!=nil{return nil,err}
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
  "fork_lineage","snapshots","scheduled_occurrences","trigger_runtime","trigger_activations"}{
  var count int
  if err:=tx.QueryRowContext(ctx,"SELECT count(*) FROM "+name).Scan(&count);err!=nil{return nil,err}
  if count!=0{return nil,fmt.Errorf("fork destination contains previous authority in %s",name)}
 }
 if _,err:=tx.ExecContext(ctx,"INSERT INTO genesis_records(run_id,genesis_id,digest,canonical) VALUES(?,?,?,?)",
  childRun[:],childGenesis.ID[:],childGenesis.Digest[:],childGenesis.CanonicalBytes);err!=nil{return nil,err}
 if _,err:=tx.ExecContext(ctx,
  "INSERT INTO fork_lineage(child_run,parent_run,parent_snapshot_id,origin_policy_hash,reference_canonical,parent_genesis_canonical,parent_snapshot_canonical) "+
  "VALUES(?,?,?,?,?,?,?)",childRun[:],sourceRun[:],parentSnapshotID[:],b.CodecPolicyHash[:],
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
 // An initially empty trigger registry is part of the fork transaction.
 // A separate post-commit initializer could crash and strand an unusable run.
 emptyRoster:=[]any{}
 sealBytes,err:=codec.CanonicalPayload([]any{childGenesis.ID[:],childGenesis.Digest[:],emptyRoster})
 if err!=nil{return nil,err}
 sealHash:=identity.OpaqueContentHash(sealBytes)
 if _,err:=tx.ExecContext(ctx,"INSERT INTO trigger_registry_seals(run_id,genesis_id,genesis_digest,registry_hash) VALUES(?,?,?,?)",
  childRun[:],childGenesis.ID[:],childGenesis.Digest[:],sealHash[:]);err!=nil{return nil,err}
 if err:=validateForkLineageTx(ctx,tx,childGenesis,b);err!=nil{return nil,err}
 if err:=tx.Commit();err!=nil{return nil,err}
 return childGenesis,nil
}

// Compile-time guarantees for the run identity and the parent/child
// responsibility boundary; no shared world truth authority is imported.
var _ domain.ID = domain.ID{}

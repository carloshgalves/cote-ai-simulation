package sqlite

import (
 "bytes"
 "context"
 "database/sql"
 "errors"
 "fmt"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/fork"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/genesis"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

// verifiedForkLineage is ephemeral, authenticated evidence—not a new
// persistent authority. Its fields are only exposed after the exact archived
// Genesis/snapshot and the installed historical origin policy agree.
// Each subsequent verifier runs against the SAME *sql.Tx, including before
// the publisher commits and whenever a child Genesis is read for replay.
type verifiedForkLineage struct {
 run [32]byte
 parentRun,parentSnapshotID []byte
 parent,child *genesis.Genesis
 origin *policy.Bundle
 snapshot []any
}

// verifyForkLineageOriginTx uses no live parent Store. All provenance is
// authenticated from immutable rows of the CHILD SQLite transaction.
func verifyForkLineageOriginTx(ctx context.Context,tx *sql.Tx,
 child *genesis.Genesis,bundle *policy.Bundle)(verifiedForkLineage,error){
 var zero verifiedForkLineage
 run:=[32]byte(child.RunID)
 var parentRun,parentSnapshotID,originHash,refRaw,parentGenesisRaw,parentSnapshotRaw []byte
 if err:=tx.QueryRowContext(ctx,
  "SELECT parent_run,parent_snapshot_id,origin_policy_hash,reference_canonical,parent_genesis_canonical,parent_snapshot_canonical "+
   "FROM fork_lineage WHERE child_run=?",run[:]).
  Scan(&parentRun,&parentSnapshotID,&originHash,&refRaw,&parentGenesisRaw,&parentSnapshotRaw);err!=nil{
  return zero,fmt.Errorf("fork parent provenance not durably published: %w",err)
 }
 if len(parentRun)!=32||len(parentSnapshotID)!=32||len(originHash)!=32{
  return zero,errors.New("fork lineage identity columns malformed")
 }
 if bytes.Equal(parentRun,run[:]){return zero,errors.New("fork cannot parent itself")}
 option,ok:=child.Payload[4].([]any)
 if !ok||len(option)!=2||option[0]!=uint64(1)||!equalBytes(option[1],refRaw){
  return zero,errors.New("Genesis parent ref differs from archived lineage")
 }
 reference,err:=fork.Decode(refRaw);if err!=nil{return zero,err}
 origin,err:=bundle.ResolveHistoricalOrigin(reference.OriginPolicyID,reference.OriginPolicyHash)
 if err!=nil{return zero,fmt.Errorf("fork origin not installed by a verified manifest: %w",err)}
 if reference.OriginPolicyHash!=origin.CodecPolicyHash||!bytes.Equal(originHash,origin.CodecPolicyHash[:]){
  return zero,errors.New("fork origin codec differs from exact installed historical bundle")
 }
 // Always authenticate old snapshot and Genesis with their OWN policy.
 if err:=fork.VerifyOriginSnapshot(reference,parentSnapshotRaw,origin);err!=nil{return zero,err}
 parent,err:=genesis.ValidateCanonical(parentGenesisRaw,origin)
 if err!=nil{return zero,fmt.Errorf("origin Genesis requires missing lineage resolver: %w",err)}
 if !bytes.Equal(parent.RunID[:],parentRun){return zero,errors.New("fork origin Genesis run mismatch")}
 snapValue,err:=codec.StrictDecodePayload(parentSnapshotRaw);if err!=nil{return zero,err}
 snap,ok:=snapValue.([]any)
 if !ok||len(snap)!=19||!equalBytes(snap[0],parentRun){return zero,errors.New("origin snapshot has wrong run")}
 if !sameCanonical(snap[9],[]any{parent.ID[:],parent.Digest[:]})||
  !sameCanonical(snap[8],parent.WorldSeed[:]){
  return zero,errors.New("fork origin Genesis ref or world seed mismatch")
 }
 if !sameCanonical(child.Payload[16],snap[6])||!sameCanonical(child.Payload[17],snap[14])||
  !sameCanonical(child.Payload[3],snap[2]){
  return zero,errors.New("fork initial world, epistemic refs or instant differ from sealed parent")
 }

 return verifiedForkLineage{
  run:run,parentRun:bytes.Clone(parentRun),parentSnapshotID:bytes.Clone(parentSnapshotID),
  parent:parent,child:child,origin:origin,snapshot:snap,
 },nil
}

// verifyForkCognitiveLineageTx proves that the V2 child contains the frozen
// parent cognition roster and authentic signed source-owner receipts. It is a
// no-op for a V1 child, preserving the old policy gate and query order.
func verifyForkCognitiveLineageTx(ctx context.Context,tx *sql.Tx,
 proof verifiedForkLineage,bundle *policy.Bundle)error{
 run,parentRun,snap:=proof.run,proof.parentRun,proof.snapshot
 if id,_,_,_,err:=bundle.CodecIdentity();err!=nil{return err}else if id==policy.CodecPolicyV2ID{
  // The exact signed parent checkpoint roster is the sole authority for
  // which opaque cognitive actors survive the migration. A child may not
  // drop or invent a memory owner even if the world-state hash still matches.
  expected:=map[string]bool{}
  entries,ok:=snap[14].([]any)
  if !ok{return errors.New("V2 cognition snapshot roster malformed")}
  for _,raw:=range entries{
   f,ok:=raw.([]any)
   if !ok||len(f)!=8{return errors.New("V2 checkpoint tuple malformed")}
   actor,ok:=f[0].([]any)
   if !ok||len(actor)!=2{return errors.New("V2 checkpoint actor malformed")}
   ns,okNS:=actor[0].(string);name,okName:=actor[1].(string)
   if !okNS||!okName||domain.ValidateMachineID(ns)!=nil||domain.ValidateMachineID(name)!=nil{
    return errors.New("V2 checkpoint actor invalid")
   }
   key:=ns+"\x00"+name
   if expected[key]{return errors.New("V2 repeated imported cognitive actor")}
   expected[key]=true
  }
  actual,err:=snapshotCognitiveActorRoster(ctx,tx,run,nil)
  if err!=nil{return err}
  // Every imported owner survives. Child-native owners may be enrolled
  // later; their keys and checkpoint bytes are governed by the child-run
  // authority and must NOT be mistaken for unimported parent records.
  for key:=range expected{if !actual[key]{return errors.New("V2 imported cognitive owner missing")}}
  if err:=verifyCognitiveOwnerProofsV2(ctx,tx,run,parentRun,entries);err!=nil{return err}
 }
 return nil
}

// verifyForkHistoricalPrefixesTx checks the bound parent snapshot ID and
// exactly the five canonical, ordered ledger prefixes without re-decoding or
// re-encoding the source's archived bytes.
func verifyForkHistoricalPrefixesTx(ctx context.Context,tx *sql.Tx,
 proof verifiedForkLineage)error{
 run,parentRun,parentSnapshotID:=proof.run,proof.parentRun,proof.parentSnapshotID
 origin,snap:=proof.origin,proof.snapshot
 rev,ok:=snap[3].(uint64);if !ok{return errors.New("fork origin revision invalid")}
 snapID,_,err:=identity.New(origin).Sum("cote.csf.id.snapshot","cote.csf.schema.snapshot-id-preimage",1,
  []any{parentRun,uint64(1),rev})
 if err!=nil{return err}
 if !bytes.Equal(parentSnapshotID,snapID[:]){return errors.New("origin snapshot reference is not ID-bound")}
 cursors,ok:=snap[12].([]any);if !ok||len(cursors)!=len(snapshotLedgerNames){
  return errors.New("fork must archive all five historical prefix cursors")
 }
 for i,name:=range snapshotLedgerNames{
  rows,err:=tx.QueryContext(ctx,
   "SELECT ordinal,record_id,canonical FROM fork_history_records WHERE child_run=? AND ledger_name=? ORDER BY ordinal",
   run[:],name)
  if err!=nil{return err}
  records:=make([]any,0)
  var prior []byte
  for rows.Next(){
   var ordinal int64
   var id,raw []byte
   if err:=rows.Scan(&ordinal,&id,&raw);err!=nil{rows.Close();return err}
   if ordinal!=int64(len(records))||len(id)!=32||len(raw)==0||
    (prior!=nil&&bytes.Compare(prior,id)>=0){
    rows.Close();return errors.New("fork history record gap or reordering")
   }
   records=append(records,bytes.Clone(raw));prior=bytes.Clone(id)
  }
  if err:=rows.Err();err!=nil{rows.Close();return err}
  if err:=rows.Close();err!=nil{return err}
  digest,_,err:=identity.New(origin).Sum("cote.csf.digest.ledger-prefix",
   "cote.csf.schema.ledger-prefix-digest-preimage",1,[]any{name,records})
  if err!=nil{return err}
  actual:=[]any{name,uint64(len(records)),digest[:]}
  if !sameCanonical(cursors[i],actual){return fmt.Errorf("origin immutable %s prefix changed",name)}
 }
 return nil
}

// verifyForkMigratedAuthoritiesTx delegates to the existing per-domain
// verifiers (round/slot, communication, occurrence, trigger), in their
// unchanged order and on the SAME read/write transaction.
func verifyForkMigratedAuthoritiesTx(ctx context.Context,tx *sql.Tx,
 proof verifiedForkLineage,bundle *policy.Bundle)error{
 run,parent,child,snap,origin:=proof.run,proof.parent,proof.child,proof.snapshot,proof.origin
 if err:=verifyRoundHistoryV2(ctx,tx,parent,child,snap,origin,bundle);err!=nil{
  return fmt.Errorf("fork V2 round history verification: %w",err)
 }
 // Every migrated V2 communication requires an independently rederived
 // child Claim/Transmission/due and an exact archived V1 send proof.
 if err:=verifyAllV2ImportsTx(ctx,tx,run,bundle);err!=nil{
  return fmt.Errorf("fork V2 imported birth verification: %w",err)
 }
 if err:=verifyOccurrenceBirthsV2(ctx,tx,parent,child,snap,origin,bundle);err!=nil{
  return fmt.Errorf("fork V2 occurrence birth verification: %w",err)
 }
 if err:=verifyTriggerMigrationV2(ctx,tx,parent,child,snap,origin,bundle);err!=nil{
  return fmt.Errorf("fork V2 trigger seed verification: %w",err)
 }
 // A child ledger must not contain the parent history: post-fork event,
 // input and causal identities are always derived in the child run.
 return nil
}

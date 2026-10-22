package sqlite

import (
 "bytes"
 "context"
 "database/sql"
 "errors"
 "fmt"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/fork"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/genesis"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

// ValidatedGenesis resolves a local fork's immutable, archived parent prefix
// before granting it runtime authority. Base runs retain their old stricter
// validation; a framed but unresolved fork can never run.
func (s *Store) ValidatedGenesis(ctx context.Context,run [32]byte,bundle *policy.Bundle)(*genesis.Genesis,error){
 if s==nil||bundle==nil{return nil,errors.New("genesis requires store and policy")}
 tx,err:=s.db.BeginTx(ctx,&sql.TxOptions{ReadOnly:true});if err!=nil{return nil,err}
 defer tx.Rollback()
 root,err:=validatedGenesisTx(ctx,tx,run,bundle);if err!=nil{return nil,err}
 if err:=tx.Commit();err!=nil{return nil,err}
 return root,nil
}

// validatedGenesisTx is safe within existing terminal or snapshot transactions,
// avoiding nested locks when fork lineage needs the same SQLite read view.
func validatedGenesisTx(ctx context.Context,tx *sql.Tx,run [32]byte,bundle *policy.Bundle)(*genesis.Genesis,error){
 var raw []byte
 if err:=tx.QueryRowContext(ctx,"SELECT canonical FROM genesis_records WHERE run_id=?",run[:]).Scan(&raw);err!=nil{return nil,err}
 if root,err:=genesis.ValidateCanonical(raw,bundle);err==nil{
  if [32]byte(root.RunID)!=run{return nil,errors.New("base Genesis run mismatch")}
  return root,nil
 }
 root,err:=genesis.ValidateForkCanonical(raw,bundle);if err!=nil{return nil,err}
 if [32]byte(root.RunID)!=run{return nil,errors.New("fork Genesis run mismatch")}
 if err:=validateForkLineageTx(ctx,tx,root,bundle);err!=nil{return nil,err}
 return root,nil
}

func validateForkLineageTx(ctx context.Context,tx *sql.Tx,child *genesis.Genesis,bundle *policy.Bundle)error{
 run:=[32]byte(child.RunID)
 var parentRun,parentSnapshotID,originHash,refRaw,parentGenesisRaw,parentSnapshotRaw []byte
 if err:=tx.QueryRowContext(ctx,
  "SELECT parent_run,parent_snapshot_id,origin_policy_hash,reference_canonical,parent_genesis_canonical,parent_snapshot_canonical "+
   "FROM fork_lineage WHERE child_run=?",run[:]).
  Scan(&parentRun,&parentSnapshotID,&originHash,&refRaw,&parentGenesisRaw,&parentSnapshotRaw);err!=nil{
  return fmt.Errorf("fork parent provenance not durably published: %w",err)
 }
 if len(parentRun)!=32||len(parentSnapshotID)!=32||len(originHash)!=32{
  return errors.New("fork lineage identity columns malformed")
 }
 if bytes.Equal(parentRun,run[:]){return errors.New("fork cannot parent itself")}
 option,ok:=child.Payload[4].([]any)
 if !ok||len(option)!=2||option[0]!=uint64(1)||!equalBytes(option[1],refRaw){
  return errors.New("Genesis parent ref differs from archived lineage")
 }
 reference,err:=fork.Decode(refRaw);if err!=nil{return err}
 origin,err:=bundle.ResolveHistoricalOrigin(reference.OriginPolicyID,reference.OriginPolicyHash)
 if err!=nil{return fmt.Errorf("fork origin not installed by a verified manifest: %w",err)}
 if reference.OriginPolicyHash!=origin.CodecPolicyHash||!bytes.Equal(originHash,origin.CodecPolicyHash[:]){
  return errors.New("fork origin codec differs from exact installed historical bundle")
 }
 // Always authenticate old snapshot and Genesis with their OWN policy.
 if err:=fork.VerifyOriginSnapshot(reference,parentSnapshotRaw,origin);err!=nil{return err}
 parent,err:=genesis.ValidateCanonical(parentGenesisRaw,origin)
 if err!=nil{return fmt.Errorf("origin Genesis requires missing lineage resolver: %w",err)}
 if !bytes.Equal(parent.RunID[:],parentRun){return errors.New("fork origin Genesis run mismatch")}
 snapValue,err:=codec.StrictDecodePayload(parentSnapshotRaw);if err!=nil{return err}
 snap,ok:=snapValue.([]any)
 if !ok||len(snap)!=19||!equalBytes(snap[0],parentRun){return errors.New("origin snapshot has wrong run")}
 if !sameCanonical(snap[9],[]any{parent.ID[:],parent.Digest[:]})||
  !sameCanonical(snap[8],parent.WorldSeed[:]){
  return errors.New("fork origin Genesis ref or world seed mismatch")
 }
 if !sameCanonical(child.Payload[16],snap[6])||!sameCanonical(child.Payload[17],snap[14])||
  !sameCanonical(child.Payload[3],snap[2]){
  return errors.New("fork initial world, epistemic refs or instant differ from sealed parent")
 }
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
 // Every migrated V2 communication requires an independently rederived
 // child Claim/Transmission/due and an exact archived V1 send proof.
 if err:=verifyAllV2ImportsTx(ctx,tx,run,bundle);err!=nil{
  return fmt.Errorf("fork V2 imported birth verification: %w",err)
 }
 // A child ledger must not contain the parent history: post-fork event,
 // input and causal identities are always derived in the child run.
 return nil
}

// A fork's final external source cannot be reopened under the same source
// identifier in its child run. This decision is made under the admission
// transaction's SQLite lock, including when callers try generic Append().
func rejectForkFinalSourceReopening(ctx context.Context,tx *sql.Tx,run [32]byte,source string)error{
 rows,err:=tx.QueryContext(ctx,
  "SELECT canonical FROM fork_history_records WHERE child_run=? AND ledger_name='input_ledger' ORDER BY ordinal",run[:])
 if err!=nil{return err}
 defer rows.Close()
 for rows.Next(){
  var raw []byte
  if err:=rows.Scan(&raw);err!=nil{return err}
  if envelope,err:=codec.StrictDecodeEnvelope(raw);err==nil{
   if envelope.DomainTag!="cote.csf.record.source-ingress-receipt"{
    return errors.New("fork historical envelope is not a SourceIngressReceipt")
   }
   continue
  }
  value,err:=codec.StrictDecodePayload(raw)
  if err!=nil{return fmt.Errorf("fork history corrupt while guarding source finality: %w",err)}
  f,ok:=value.([]any)
  if !ok{return errors.New("fork archived input record is not a canonical array")}
  switch len(f){
  case 3,11,13,19:continue // source declaration, round, dispatch, response or input
  case 7:
  default:return errors.New("unknown fork historical input record shape")
  }
  name,ok:=f[2].(string);if !ok{return errors.New("fork SourceClosure id malformed")}
  if name!=source{continue}
  terminal,ok:=f[5].(bool)
  if !ok{return errors.New("fork SourceClosure final marker malformed")}
  if terminal{return fmt.Errorf("source %q finalized by immutable fork history",source)}
 }
 return rows.Err()
}

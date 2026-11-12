package sqlite

import (
 "context"
 "database/sql"
 "errors"
 "fmt"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/genesis"
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

// validateForkLineageTx is the sole lineage/replay authentication gate.
// The child publisher calls it before COMMIT; ValidatedGenesis calls the
// exact same gate in a read-only transaction, and ReplayClock necessarily
// re-enters via ValidatedGenesis before any runtime state is reconstructed.
func validateForkLineageTx(ctx context.Context,tx *sql.Tx,child *genesis.Genesis,bundle *policy.Bundle)error{
 proof,err:=verifyForkLineageOriginTx(ctx,tx,child,bundle)
 if err!=nil{return err}
 if err:=verifyForkCognitiveLineageTx(ctx,tx,proof,bundle);err!=nil{return err}
 if err:=verifyForkHistoricalPrefixesTx(ctx,tx,proof);err!=nil{return err}
 return verifyForkMigratedAuthoritiesTx(ctx,tx,proof,bundle)
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

package sqlite

import (
 "bytes"
 "context"
 "database/sql"
 "errors"
 "fmt"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
)

const snapshotOwnerTrustDomainV1="cote.csf.snapshot-owner-key-roster.v1"

// This seal is a PRIVATE SQLite authority alongside snapshot_seals, not a new
// field in the normative canonical snapshot V1. Both empty and nonempty
// owner rosters have exact domain-separated digests.
func snapshotOwnerRoster(ctx context.Context,tx *sql.Tx,run [32]byte)(uint64,[32]byte,error){
 var zero [32]byte
 rows,err:=tx.QueryContext(ctx,
  "SELECT actor_namespace,actor_id,public_key FROM cognitive_owner_keys "+
  "WHERE run_id=? ORDER BY actor_namespace,actor_id",run[:])
 if err!=nil{return 0,zero,err}
 records:=[]any{}
 for rows.Next(){
  var ns,id string
  var key []byte
  if err:=rows.Scan(&ns,&id,&key);err!=nil{rows.Close();return 0,zero,err}
  if len(key)!=32{rows.Close();return 0,zero,errors.New("invalid sealed memory owner public key")}
  records=append(records,[]any{ns,id,bytes.Clone(key)})
 }
 if err:=rows.Err();err!=nil{rows.Close();return 0,zero,err}
 if err:=rows.Close();err!=nil{return 0,zero,err}
 canonical,err:=codec.CanonicalPayload([]any{
  snapshotOwnerTrustDomainV1,uint64(1),run[:],records,
 })
 if err!=nil{return 0,zero,err}
 return uint64(len(records)),identity.OpaqueContentHash(canonical),nil
}

func sealSnapshotOwnerTrust(ctx context.Context,tx *sql.Tx,run,snapshotID [32]byte)error{
 count,digest,err:=snapshotOwnerRoster(ctx,tx,run)
 if err!=nil{return err}
 _,err=tx.ExecContext(ctx,
  "INSERT INTO snapshot_owner_key_seals(snapshot_id,run_id,owner_count,owner_roster_hash) "+
  "VALUES(?,?,?,?)",snapshotID[:],run[:],count,digest[:])
 return err
}

func verifySnapshotOwnerTrust(ctx context.Context,tx *sql.Tx,run,snapshotID [32]byte)error{
 var storedRun,storedDigest []byte
 var storedCount uint64
 err:=tx.QueryRowContext(ctx,
  "SELECT run_id,owner_count,owner_roster_hash FROM snapshot_owner_key_seals "+
  "WHERE snapshot_id=?",snapshotID[:]).Scan(&storedRun,&storedCount,&storedDigest)
 if err!=nil{return fmt.Errorf("snapshot owner trust seal absent: %w",err)}
 count,digest,err:=snapshotOwnerRoster(ctx,tx,run)
 if err!=nil{return err}
 if !bytes.Equal(storedRun,run[:])||storedCount!=count||
  !bytes.Equal(storedDigest,digest[:]){
  return errors.New("signed snapshot owner-key trust roster differs from frozen seal")
 }
 return nil
}

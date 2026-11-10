package sqlite

import (
 "bytes"
 "context"
 "crypto/ed25519"
 "crypto/sha256"
 "database/sql"
 "errors"
 "fmt"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

// A cognitive owner must durably persist the exact opaque bytes in their own
// store BEFORE signing. CSF verifies that attestation and escrows those bytes
// before publishing a causal snapshot reference. No memory content is decoded.
const cognitiveOwnerReceiptDomainV1="cote.csf.cognitive-owner-receipt.v1"

func CognitiveCheckpointSigningBytesV1(run [32]byte,cp SnapshotCheckpoint,locator string)([]byte,error){
 if locator==""||domain.ValidateMachineID(locator)!=nil{return nil,errors.New("owner receipt requires stable locator ID")}
 for _,name:=range []string{cp.ActorNamespace,cp.ActorID,cp.StoreID,cp.Producer}{
  if err:=domain.ValidateMachineID(name);err!=nil{return nil,err}
 }
 if cp.SchemaVersion==0||cp.ProducerVersion==0{return nil,errors.New("owner receipt version missing")}
 hash:=identity.OpaqueContentHash(cp.Content)
 return codec.CanonicalPayload([]any{
  cognitiveOwnerReceiptDomainV1,uint64(1),run[:],cp.ActorNamespace,cp.ActorID,
  cp.StoreID,uint64(cp.SchemaVersion),cp.Producer,uint64(cp.ProducerVersion),
  cp.CoversInstant,cp.CoversRevision,cp.CoversEvidenceCursor,hash[:],locator,
 })
}

func (s *Store) RegisterCognitiveOwnerKeyV1(ctx context.Context,run [32]byte,
 namespace,actor string,publicKey ed25519.PublicKey,b *policy.Bundle)error{
 if s==nil||b==nil||len(publicKey)!=ed25519.PublicKeySize{return errors.New("invalid cognitive owner registration")}
 for _,name:=range []string{namespace,actor}{if err:=domain.ValidateMachineID(name);err!=nil{return err}}
 if _,err:=s.ValidatedGenesis(ctx,run,b);err!=nil{return err}
 tx,err:=s.db.BeginTx(ctx,nil);if err!=nil{return err};defer tx.Rollback()
 if _,err:=tx.ExecContext(ctx,"UPDATE clock_states SET canonical=canonical WHERE run_id=?",run[:]);err!=nil{return err}
 var count int
 if err:=tx.QueryRowContext(ctx,
  "SELECT count(*) FROM cognitive_actor_registry WHERE run_id=? AND actor_namespace=? AND actor_id=?",
  run[:],namespace,actor).Scan(&count);err!=nil{return err}
 if count!=1{return errors.New("cognitive owner is not in durable actor registry")}
 var prev []byte
 err=tx.QueryRowContext(ctx,
  "SELECT public_key FROM cognitive_owner_keys WHERE run_id=? AND actor_namespace=? AND actor_id=?",
  run[:],namespace,actor).Scan(&prev)
 if err==nil{
  if !bytes.Equal(prev,publicKey){return errors.New("cognitive owner signing key is immutable")}
  return tx.Commit()
 }
 if err!=sql.ErrNoRows{return err}
 if err:=tx.QueryRowContext(ctx,
  "SELECT count(*) FROM snapshot_seals WHERE run_id=?",run[:]).Scan(&count);err!=nil{return err}
 if count!=0{return errors.New("cannot introduce owner trust after first snapshot")}
 if _,err:=tx.ExecContext(ctx,
  "INSERT INTO cognitive_owner_keys(run_id,actor_namespace,actor_id,public_key) VALUES(?,?,?,?)",
  run[:],namespace,actor,bytes.Clone(publicKey));err!=nil{return err}
 return tx.Commit()
}

// PersistCognitiveCheckpointV1 is an idempotent SQLite FULL-sync escrow of an
// owner-signed opaque checkpoint. The external owner is accountable for the
// preceding remote durable commit that its signature attests.
func (s *Store) PersistCognitiveCheckpointV1(ctx context.Context,run [32]byte,
 cp SnapshotCheckpoint,locator string,signature []byte,b *policy.Bundle)([32]byte,error){
 var zero [32]byte
 if s==nil||b==nil||len(signature)!=ed25519.SignatureSize{return zero,errors.New("missing signer receipt or policy")}
 if _,err:=s.ValidatedGenesis(ctx,run,b);err!=nil{return zero,err}
 payload,err:=CognitiveCheckpointSigningBytesV1(run,cp,locator);if err!=nil{return zero,err}
 tx,err:=s.db.BeginTx(ctx,nil);if err!=nil{return zero,err};defer tx.Rollback()
 var key []byte
 if err:=tx.QueryRowContext(ctx,
  "SELECT public_key FROM cognitive_owner_keys WHERE run_id=? AND actor_namespace=? AND actor_id=?",
  run[:],cp.ActorNamespace,cp.ActorID).Scan(&key);err!=nil{return zero,fmt.Errorf("unknown cognitive owner: %w",err)}
 if len(key)!=ed25519.PublicKeySize||!ed25519.Verify(ed25519.PublicKey(key),payload,signature){
  return zero,errors.New("invalid cognitive owner receipt signature")
 }
 receipt:=sha256.Sum256(payload)
 digest:=identity.OpaqueContentHash(cp.Content)
 _,err=tx.ExecContext(ctx,
  "INSERT OR IGNORE INTO cognitive_owner_receipts("+
  "receipt_id,run_id,actor_namespace,actor_id,store_id,schema_version,"+
  "content,content_hash,producer,producer_version,covers_instant,covers_revision,"+
  "covers_evidence_cursor,locator,signature) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)",
  receipt[:],run[:],cp.ActorNamespace,cp.ActorID,cp.StoreID,cp.SchemaVersion,
  bytes.Clone(cp.Content),digest[:],cp.Producer,cp.ProducerVersion,
  cp.CoversInstant,cp.CoversRevision,cp.CoversEvidenceCursor,locator,bytes.Clone(signature))
 if err!=nil{return zero,err}
 var stored,signed []byte
 if err:=tx.QueryRowContext(ctx,
  "SELECT content,signature FROM cognitive_owner_receipts WHERE receipt_id=?",receipt[:]).
  Scan(&stored,&signed);err!=nil{return zero,fmt.Errorf("owner escrow collision: %w",err)}
 if !bytes.Equal(stored,cp.Content)||!bytes.Equal(signed,signature){
  return zero,errors.New("conflicting immutable signed owner checkpoint")
 }
 if err:=tx.Commit();err!=nil{return zero,err}
 return receipt,nil
}

// Called by both snapshot publication and VerifySnapshotV1. Legacy actors
// without an enrolled signing owner retain existing inline V1 compatibility.
func verifyPreparedOwnerCheckpointTx(ctx context.Context,tx *sql.Tx,run [32]byte,
 cp SnapshotCheckpoint)error{
 var key []byte
 err:=tx.QueryRowContext(ctx,
  "SELECT public_key FROM cognitive_owner_keys WHERE run_id=? AND actor_namespace=? AND actor_id=?",
  run[:],cp.ActorNamespace,cp.ActorID).Scan(&key)
 if err==sql.ErrNoRows{return nil}
 if err!=nil{return err}
 if len(key)!=ed25519.PublicKeySize{return errors.New("corrupt cognitive owner public key")}
 var receipt,stored,storedHash,signature []byte
 var locator string
 err=tx.QueryRowContext(ctx,
  "SELECT receipt_id,content,content_hash,locator,signature FROM cognitive_owner_receipts "+
  "WHERE run_id=? AND actor_namespace=? AND actor_id=? AND store_id=? AND schema_version=? "+
  "AND producer=? AND producer_version=? AND covers_instant=? AND covers_revision=? "+
  "AND covers_evidence_cursor=?",
  run[:],cp.ActorNamespace,cp.ActorID,cp.StoreID,cp.SchemaVersion,cp.Producer,
  cp.ProducerVersion,cp.CoversInstant,cp.CoversRevision,cp.CoversEvidenceCursor).
  Scan(&receipt,&stored,&storedHash,&locator,&signature)
 if err!=nil{return fmt.Errorf("required durable cognitive owner receipt missing: %w",err)}
 digest:=identity.OpaqueContentHash(cp.Content)
 if !bytes.Equal(stored,cp.Content)||!bytes.Equal(storedHash,digest[:]){
  return errors.New("owner escrow differs from opaque snapshot bytes")
 }
 payload,err:=CognitiveCheckpointSigningBytesV1(run,cp,locator);if err!=nil{return err}
 expected:=sha256.Sum256(payload)
 if !bytes.Equal(expected[:],receipt)||len(signature)!=ed25519.SignatureSize||
  !ed25519.Verify(ed25519.PublicKey(key),payload,signature){
  return errors.New("owner receipt identity or signature invalid")
 }
 return nil
}

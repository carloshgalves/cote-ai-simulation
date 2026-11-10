package sqlite

import (
 "bytes"
 "context"
 "crypto/ed25519"
 "database/sql"
 "errors"
 "fmt"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
)

// Parse only the already authenticated parent checkpoint tuples. The memory
// bytes remain opaque even while their owner signature is inspected.
func checkpointFromSnapshotRefV1(raw any)(SnapshotCheckpoint,error){
 var out SnapshotCheckpoint
 f,ok:=raw.([]any);if !ok||len(f)!=8{return out,errors.New("signed fork checkpoint shape invalid")}
 actor,ok:=f[0].([]any);if !ok||len(actor)!=2{return out,errors.New("signed fork actor invalid")}
 ns,nsOK:=actor[0].(string);who,whoOK:=actor[1].(string)
 store,storeOK:=f[1].(string);sv,svOK:=f[2].(uint64)
 content,contentOK:=f[3].([]any)
 producer,producerOK:=f[5].(string);pv,pvOK:=f[6].(uint64)
 cover,coverOK:=f[7].([]any)
 if !nsOK||!whoOK||!storeOK||!svOK||sv==0||sv>uint64(^uint32(0))||
  !contentOK||len(content)!=2||content[0]!=uint64(1)||
  !producerOK||!pvOK||pv==0||pv>uint64(^uint32(0))||
  !coverOK||len(cover)!=3{return out,errors.New("signed fork checkpoint type invalid")}
 rawBytes,bytesOK:=content[1].([]byte);instant,instantOK:=asSigned64(cover[0])
 revision,revisionOK:=cover[1].(uint64);cursor,cursorOK:=cover[2].(uint64)
 if !bytesOK||!instantOK||!revisionOK||!cursorOK{return out,errors.New("signed fork checkpoint coverage invalid")}
 hash:=identity.OpaqueContentHash(rawBytes)
 if !equalBytes(f[4],hash[:]){return out,errors.New("signed fork checkpoint digest invalid")}
 return SnapshotCheckpoint{ActorNamespace:ns,ActorID:who,StoreID:store,
  SchemaVersion:uint32(sv),Content:bytes.Clone(rawBytes),Producer:producer,
  ProducerVersion:uint32(pv),CoversInstant:instant,CoversRevision:revision,
  CoversEvidenceCursor:cursor},nil
}

// Source owner keys are migrated with their signed SOURCE receipts. The child
// still requires a fresh receipt over its NEW run ID before a child snapshot.
// Historic signatures cannot authorize new, child-scoped memory contents.
func importCognitiveOwnerProofsV2(ctx context.Context,source *Store,tx *sql.Tx,
 sourceRun,childRun [32]byte,refs []any)error{
 for _,raw:=range refs{
  cp,err:=checkpointFromSnapshotRefV1(raw);if err!=nil{return err}
  var key []byte
  err=source.db.QueryRowContext(ctx,
   "SELECT public_key FROM cognitive_owner_keys WHERE run_id=? AND actor_namespace=? AND actor_id=?",
   sourceRun[:],cp.ActorNamespace,cp.ActorID).Scan(&key)
  if err==sql.ErrNoRows{continue}
  if err!=nil{return err}
  if len(key)!=ed25519.PublicKeySize{return errors.New("source cognitive owner public key invalid")}
  var receipt,signature []byte
  var locator string
  err=source.db.QueryRowContext(ctx,
   "SELECT receipt_id,locator,signature FROM cognitive_owner_receipts WHERE "+
    "run_id=? AND actor_namespace=? AND actor_id=? AND store_id=? AND schema_version=? "+
    "AND producer=? AND producer_version=? AND covers_instant=? AND covers_revision=? "+
    "AND covers_evidence_cursor=?",
    sourceRun[:],cp.ActorNamespace,cp.ActorID,cp.StoreID,cp.SchemaVersion,
    cp.Producer,cp.ProducerVersion,cp.CoversInstant,cp.CoversRevision,
    cp.CoversEvidenceCursor).Scan(&receipt,&locator,&signature)
  if err!=nil{return fmt.Errorf("missing source owner's durable checkpoint proof: %w",err)}
  payload,err:=CognitiveCheckpointSigningBytesV1(sourceRun,cp,locator);if err!=nil{return err}
  digest:=identity.OpaqueContentHash(payload)
  if !bytes.Equal(receipt,digest[:])||!ed25519.Verify(ed25519.PublicKey(key),payload,signature){
   return errors.New("source cognitive owner proof cannot be authenticated")
  }
  if _,err:=tx.ExecContext(ctx,
   "INSERT INTO cognitive_owner_keys(run_id,actor_namespace,actor_id,public_key) VALUES(?,?,?,?)",
   childRun[:],cp.ActorNamespace,cp.ActorID,key);err!=nil{return err}
  if _,err:=tx.ExecContext(ctx,
   "INSERT INTO fork_cognitive_owner_proofs(child_run,actor_namespace,actor_id,"+
   "source_run,receipt_id,public_key,locator,signature) VALUES(?,?,?,?,?,?,?,?)",
   childRun[:],cp.ActorNamespace,cp.ActorID,sourceRun[:],receipt,key,locator,signature);err!=nil{return err}
 }
 count,digest,err:=ownerProofRosterSealV2(ctx,tx,childRun)
 if err!=nil{return err}
 if _,err:=tx.ExecContext(ctx,
  "INSERT INTO fork_owner_proof_seals(child_run,proof_count,roster_digest) VALUES(?,?,?)",
  childRun[:],count,digest[:]);err!=nil{return err}
 return nil
}

// Reauthenticate the exact signed parent content and public-key equality at
// every child replay. Missing/altered proof or live owner key rejects lineage.
func verifyCognitiveOwnerProofsV2(ctx context.Context,tx *sql.Tx,childRun [32]byte,
 sourceRun []byte,refs []any)error{
 if len(sourceRun)!=32{return errors.New("fork owner proof source run invalid")}
 var parent [32]byte;copy(parent[:],sourceRun)
 roster:=map[string]SnapshotCheckpoint{}
 for _,raw:=range refs{
  cp,err:=checkpointFromSnapshotRefV1(raw);if err!=nil{return err}
  key:=cp.ActorNamespace+"\x00"+cp.ActorID
  if _,found:=roster[key];found{return errors.New("duplicate parent cognitive owner")}
  roster[key]=cp
 }
 rows,err:=tx.QueryContext(ctx,
  "SELECT actor_namespace,actor_id,source_run,receipt_id,public_key,locator,signature "+
  "FROM fork_cognitive_owner_proofs WHERE child_run=? ORDER BY actor_namespace,actor_id",
  childRun[:])
 if err!=nil{return err}
 type proof struct{ns,actor,locator string;src,receipt,key,signature []byte}
 proofs:=[]proof{}
 for rows.Next(){
  var p proof
  if err:=rows.Scan(&p.ns,&p.actor,&p.src,&p.receipt,&p.key,&p.locator,&p.signature);err!=nil{
   rows.Close();return err
  }
  proofs=append(proofs,p)
 }
 if err:=rows.Err();err!=nil{rows.Close();return err}
 if err:=rows.Close();err!=nil{return err}
 for _,p:=range proofs{
  cp,found:=roster[p.ns+"\x00"+p.actor]
  if !found{return errors.New("imported signer not in frozen parent actor roster")}
  var liveKey []byte
  if err:=tx.QueryRowContext(ctx,
   "SELECT public_key FROM cognitive_owner_keys WHERE run_id=? AND actor_namespace=? AND actor_id=?",
   childRun[:],p.ns,p.actor).Scan(&liveKey);err!=nil{return err}
  if !bytes.Equal(liveKey,p.key)||len(p.key)!=ed25519.PublicKeySize||
   !bytes.Equal(p.src,sourceRun){return errors.New("migrated cognitive owner key/source changed")}
  payload,err:=CognitiveCheckpointSigningBytesV1(parent,cp,p.locator)
  if err!=nil{return err}
  expected:=identity.OpaqueContentHash(payload)
  if !bytes.Equal(p.receipt,expected[:])||len(p.signature)!=ed25519.SignatureSize||
   !ed25519.Verify(ed25519.PublicKey(p.key),payload,p.signature){
   return errors.New("migrated signed cognitive owner proof corrupted")
  }
 }
 var live int
 if err:=tx.QueryRowContext(ctx,
  "SELECT count(*) FROM cognitive_owner_keys WHERE run_id=?",childRun[:]).Scan(&live);err!=nil{return err}
 if live!=len(proofs){return errors.New("migrated cognitive owner key/proof bijection broken")}
 var sealedCount uint64
 var sealedDigest []byte
 if err:=tx.QueryRowContext(ctx,
  "SELECT proof_count,roster_digest FROM fork_owner_proof_seals WHERE child_run=?",
  childRun[:]).Scan(&sealedCount,&sealedDigest);err!=nil{
  return fmt.Errorf("missing immutable fork owner proof roster seal: %w",err)
 }
 count,hash,err:=ownerProofRosterSealV2(ctx,tx,childRun)
 if err!=nil{return err}
 if count!=sealedCount||!bytes.Equal(hash[:],sealedDigest){
  return errors.New("fork owner proof roster differs from signed bootstrap seal")
 }
 return nil
}

// ownerProofRosterSealV2 seals the complete signer roster, including zero
// entries, at bootstrap. A later deletion of BOTH proof and signer rows must
// not silently downgrade a signed child to legacy unauthenticated memory.
func ownerProofRosterSealV2(ctx context.Context,tx *sql.Tx,childRun [32]byte)(uint64,[32]byte,error){
 var zero [32]byte
 rows,err:=tx.QueryContext(ctx,
  "SELECT actor_namespace,actor_id,source_run,receipt_id,public_key,locator,signature "+
  "FROM fork_cognitive_owner_proofs WHERE child_run=? ORDER BY actor_namespace,actor_id",
  childRun[:])
 if err!=nil{return 0,zero,err}
 records:=[]any{}
 for rows.Next(){
  var ns,actor,loc string
  var origin,receipt,key,sig []byte
  if err:=rows.Scan(&ns,&actor,&origin,&receipt,&key,&loc,&sig);err!=nil{
   rows.Close();return 0,zero,err
  }
  records=append(records,[]any{ns,actor,origin,receipt,key,loc,sig})
 }
 if err:=rows.Err();err!=nil{rows.Close();return 0,zero,err}
 if err:=rows.Close();err!=nil{return 0,zero,err}
 serialized,err:=codec.CanonicalPayload([]any{
  "cote.csf.fork-owner-proof-roster.v1",uint64(1),childRun[:],records,
 })
 if err!=nil{return 0,zero,err}
 return uint64(len(records)),identity.OpaqueContentHash(serialized),nil
}

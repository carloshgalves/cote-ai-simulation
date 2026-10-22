package sqlite

import (
 "bytes"
 "context"
 "database/sql"
 "errors"
 "fmt"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

// sealSnapshotPrefix stores the exact canonical membership visible when the
// snapshot was produced. Sorting future rows cannot retroactively change
// which immutable records formed an earlier prefix.
func sealSnapshotPrefix(ctx context.Context,tx *sql.Tx,snapshotID [32]byte)error{
 for _,name:=range snapshotLedgerNames{
  rows,err:=tx.QueryContext(ctx,"SELECT id,canonical FROM "+name+" ORDER BY id")
  if err!=nil{return err}
  var ordinal uint64
  for rows.Next(){
   var id,raw []byte
   if err:=rows.Scan(&id,&raw);err!=nil{rows.Close();return err}
   if len(id)!=32||len(raw)==0{rows.Close();return errors.New("invalid ledger member for snapshot")}
   hash:=identity.OpaqueContentHash(raw)
   if _,err:=tx.ExecContext(ctx, "INSERT INTO snapshot_prefix_members(snapshot_id,ledger_name,ordinal,record_id,canonical_hash) VALUES(?,?,?,?,?)",
    snapshotID[:],name,ordinal,id,hash[:]);err!=nil{
    rows.Close();return err
   }
   ordinal++
  }
  if err:=rows.Err();err!=nil{rows.Close();return err}
  if err:=rows.Close();err!=nil{return err}
 }
 return nil
}

func sealSnapshot(ctx context.Context,tx *sql.Tx,checkpoint SnapshotV1,run [32]byte)error{
 if _,err:=tx.ExecContext(ctx,
  "INSERT INTO snapshot_seals(snapshot_id,run_id,revision,digest) VALUES(?,?,?,?)",
  checkpoint.ID[:],run[:],checkpoint.Revision,checkpoint.Digest[:]);err!=nil{return err}
 return sealSnapshotPrefix(ctx,tx,checkpoint.ID)
}

// snapshotVerifiedPrefix proves every original member is still present with
// its exact bytes. A new record with a lexicographically earlier ID does not
// invalidate this immutable checkpoint, but deleting or editing one does.
func snapshotVerifiedPrefix(ctx context.Context,tx *sql.Tx,snapID [32]byte,table string,b *policy.Bundle)([]any,error){
 valid:=false;for _,v:=range snapshotLedgerNames{if v==table{valid=true}}
 if !valid{return nil,errors.New("snapshot ledger table not allowlisted")}
 rows,err:=tx.QueryContext(ctx,"SELECT ordinal,record_id,canonical_hash FROM snapshot_prefix_members WHERE snapshot_id=? AND ledger_name=? ORDER BY ordinal",snapID[:],table)
 if err!=nil{return nil,err}
 type member struct{id,hash []byte}
 members:=[]member{}
 for rows.Next(){
  var ordinal int64
  var id,hash []byte
  if err:=rows.Scan(&ordinal,&id,&hash);err!=nil{rows.Close();return nil,err}
  if ordinal!=int64(len(members))||len(id)!=32||len(hash)!=32{rows.Close();return nil,errors.New("snapshot prefix membership gap or invalid identity")}
  members=append(members,member{id,hash})
 }
 if err:=rows.Err();err!=nil{rows.Close();return nil,err}
 if err:=rows.Close();err!=nil{return nil,err}
 canonical:=make([]any,0,len(members))
 prev:=[]byte(nil)
 for _,m:=range members{
  if prev!=nil&&bytes.Compare(prev,m.id)>=0{return nil,errors.New("snapshot ledger prefix not in canonical ID order")}
  prev=m.id
  var raw []byte
  err:=tx.QueryRowContext(ctx,"SELECT canonical FROM "+table+" WHERE id=?",m.id).Scan(&raw)
  if err!=nil{return nil,fmt.Errorf("snapshot historical member missing: %w",err)}
  h:=identity.OpaqueContentHash(raw)
  if !bytes.Equal(h[:],m.hash){return nil,errors.New("historical snapshot member bytes changed")}
  canonical=append(canonical,raw)
 }
 d,_,err:=identity.New(b).Sum("cote.csf.digest.ledger-prefix","cote.csf.schema.ledger-prefix-digest-preimage",1,
  []any{table,canonical})
 if err!=nil{return nil,err}
 return []any{table,uint64(len(canonical)),d[:]},nil
}

func verifySnapshotSeal(ctx context.Context,tx *sql.Tx,id,run,digest [32]byte,revision uint64)error{
 var sealRun,sealDigest []byte
 var sealRevision int64
 if err:=tx.QueryRowContext(ctx,"SELECT run_id,revision,digest FROM snapshot_seals WHERE snapshot_id=?",id[:]).
  Scan(&sealRun,&sealRevision,&sealDigest);err!=nil{return fmt.Errorf("snapshot seal absent: %w",err)}
 if !bytes.Equal(sealRun,run[:])||sealRevision<0||uint64(sealRevision)!=revision||
  !bytes.Equal(sealDigest,digest[:]){return errors.New("immutable checkpoint seal mismatch")}
 var count int
 if err:=tx.QueryRowContext(ctx,"SELECT count(DISTINCT ledger_name) FROM snapshot_prefix_members WHERE snapshot_id=?",id[:]).Scan(&count);err!=nil{return err}
 // An entirely empty five-ledger snapshot has zero membership rows. The
 // authoritative per-ledger cursors in the snapshot still prove five digests.
 if count>len(snapshotLedgerNames){return errors.New("extra ledger partition in snapshot membership")}
 return nil
}

// snapshotHistoricalClock derives the clock at an older revision from the
// already replay-validated terminal journals. No current clock state or
// worker/controller state is substituted for the historical coordinate.
func snapshotHistoricalClock(ctx context.Context,tx *sql.Tx,run [32]byte,initialInstant int64,target uint64)(snapshotClock,error){
 result:=snapshotClock{Instant:initialInstant,LastOrdinal:-1}
 if target==0{return result,nil}
 rows,err:=tx.QueryContext(ctx, "SELECT d.canonical FROM committed_cycles c JOIN decision_ledger d ON d.id=c.commit_id WHERE c.run_id=? ORDER BY c.rowid",run[:])
 if err!=nil{return result,err}
 defer rows.Close()
 for rows.Next(){
  var raw []byte
  if err:=rows.Scan(&raw);err!=nil{return result,err}
  decoded,err:=codec.StrictDecodePayload(raw);if err!=nil{return result,err}
  v,ok:=decoded.([]any);if !ok||len(v)!=24{return result,errors.New("historical CycleCommit schema mismatch")}
  after,ok:=v[8].(uint64);if !ok{return result,errors.New("historical revision malformed")}
  if after>target{break}
  if after!=result.Revision+1{return result,errors.New("historical terminal revision gap")}
  plan,ok:=v[5].([]any);if !ok{return result,errors.New("historical plan malformed")}
  switch{
  case len(plan)==4&&plan[0]==uint64(1):
   instant,ok:=asSigned64(plan[2]);if !ok{return result,errors.New("historical CLOCK_ADVANCE target invalid")}
   result.Instant=instant;result.LastOrdinal=-1
  case len(plan)==1&&plan[0]==uint64(0):
   instant,ok:=asSigned64(v[3]);if !ok||instant!=result.Instant{return result,errors.New("historical WORK instant mismatch")}
   ordinal,ok:=v[4].(uint64);if !ok||ordinal>uint64(^uint64(0)>>1){return result,errors.New("historical WORK ordinal invalid")}
   result.LastOrdinal=int64(ordinal)
  default:return result,errors.New("historical cycle kind unsupported")
  }
  seq,ok:=v[22].(uint64);if !ok{return result,errors.New("historical next sequence missing")}
  result.NextSequence=seq
  result.Revision=after
 }
 if err:=rows.Err();err!=nil{return result,err}
 if result.Revision!=target{return result,errors.New("historical checkpoint not covered by complete CycleCommit journal")}
 return result,nil
}

type snapshotClock struct{
 Instant int64
 Revision uint64
 NextSequence uint64
 LastOrdinal int64
}

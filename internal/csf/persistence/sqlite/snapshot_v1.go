package sqlite

import (
 "bytes"
 "context"
 "database/sql"
 "errors"
 "fmt"
 "sort"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/clock"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/genesis"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

// SnapshotV1 is a canonical, content-addressed checkpoint. It does not own world
// state: the persisted Genesis, journal, outbox and revisions remain authoritative.
type SnapshotV1 struct {
 ID [32]byte
 Digest [32]byte
 Canonical []byte
 Revision uint64
}

// SnapshotCheckpoint is an opaque epistemic checkpoint. The core validates
// content and coverage, but never interprets it or invokes an agent/model.
type SnapshotCheckpoint struct {
 ActorNamespace string
 ActorID string
 StoreID string
 SchemaVersion uint32
 Content []byte
 Producer string
 ProducerVersion uint32
 CoversInstant int64
 CoversRevision uint64
 CoversEvidenceCursor uint64
}

var snapshotLedgerNames = [...]string{
 "input_ledger", "decision_ledger", "event_store", "evidence_ledger", "causal_outbox",
}

func snapshotLedgerPrefix(ctx context.Context, tx *sql.Tx, name string, b *policy.Bundle)([]any,error){
 allowed:=false
 for _,n:=range snapshotLedgerNames {if name==n {allowed=true}}
 if !allowed {return nil,errors.New("unregistered snapshot ledger")}
 rows,err:=tx.QueryContext(ctx,"SELECT canonical FROM "+name+" ORDER BY id")
 if err!=nil{return nil,err}
 defer rows.Close()
 records:=make([]any,0)
 for rows.Next(){
  var raw []byte
  if err:=rows.Scan(&raw);err!=nil{return nil,err}
  if len(raw)==0{return nil,errors.New("empty authoritative ledger record")}
  records=append(records,bytes.Clone(raw))
 }
 if err:=rows.Err();err!=nil{return nil,err}
 digest,_,err:=identity.New(b).Sum("cote.csf.digest.ledger-prefix","cote.csf.schema.ledger-prefix-digest-preimage",1,[]any{name,records})
 if err!=nil{return nil,err}
 return []any{name,uint64(len(records)),digest[:]},nil
}

func snapshotPending(ctx context.Context,tx *sql.Tx,run [32]byte,b *policy.Bundle)([]any,error){
 // Derived in authoritative, repeatable, byte-sorted order. Caller cannot
 // forge a roster of pending schedule, runtime, tasks or completions.
 var parts [10][]any
 for i:=range parts {parts[i]=[]any{}}
 pick:=func(table,sqlQuery string,part int,refKind uint64,allPayloads bool)error{
  rows,err:=tx.QueryContext(ctx,sqlQuery,run[:]);if err!=nil{return err}
  defer rows.Close()
  for rows.Next(){
   var id,raw []byte
   if err:=rows.Scan(&id,&raw);err!=nil{return err}
   if len(id)!=32{return errors.New("pending source ID invalid")}
   if allPayloads{
    v,err:=codec.StrictDecodePayload(raw);if err!=nil{return err}
    parts[part]=append(parts[part],v)
   }else{
    d:=identity.OpaqueContentHash(raw)
    parts[part]=append(parts[part],[]any{refKind,bytes.Clone(id),d[:]})
   }
  }
  return rows.Err()
 }
 if err:=pick("scheduled_occurrences","SELECT id,canonical FROM scheduled_occurrences WHERE run_id=? AND lifecycle=0 ORDER BY id",0,0,false);err!=nil{return nil,err}
 if err:=pick("trigger_runtime","SELECT definition_id,canonical FROM trigger_runtime WHERE run_id=? ORDER BY definition_id",1,0,true);err!=nil{return nil,err}
 if err:=pick("trigger_activations","SELECT id,canonical FROM trigger_activations WHERE run_id=? AND lifecycle=0 ORDER BY id",2,0,false);err!=nil{return nil,err}
 // Source receipts, rounds and dispatches are immutable; their terminal
 // liveness must be verified from journal before snapshot publication.
 // Full source/slot projections are kept by the five authenticated prefixes.
 pending,err:=pendingClock(ctx,tx,run);if err!=nil{return nil,err}
 for _,p:=range pending {
  parts[7]=append(parts[7],[]any{uint64(4),p.ID[:],[]byte{}})
 }
 // Completion payloads are included as opaque authoritative receipt bytes.
 rows,err:=tx.QueryContext(ctx,"SELECT canonical FROM causal_outbox ORDER BY id")
 if err!=nil{return nil,err}
 for rows.Next(){
  var raw []byte
  if err:=rows.Scan(&raw);err!=nil{rows.Close();return nil,err}
  v,err:=codec.StrictDecodePayload(raw);if err!=nil{rows.Close();return nil,err}
  f,ok:=v.([]any);if !ok{rows.Close();return nil,errors.New("invalid outbox record")}
  if len(f)==10&&equalBytes(f[1],run[:]){hash:=identity.OpaqueContentHash(raw);parts[8]=append(parts[8],[]any{uint64(15),f[0],hash[:]})}
  if len(f)==8 {parts[9]=append(parts[9],f)}
 }
 if err:=rows.Err();err!=nil{rows.Close();return nil,err}
 if err:=rows.Close();err!=nil{return nil,err}
 fields:=make([]any,len(parts))
 for i,v:=range parts {fields[i]=v}
 hash,encoded,err:=identity.New(b).Sum("cote.csf.snapshot.pending-state","cote.csf.schema.snapshot-pending-state",1,fields)
 if err!=nil{return nil,err}
 return []any{"cote.csf.schema.snapshot-pending-state",uint64(1),encoded,hash[:]},nil
}

func snapshotCheckpointRefs(checkpoints []SnapshotCheckpoint,instant int64,rev,cursor uint64)([]any,error){
 items:=append([]SnapshotCheckpoint(nil),checkpoints...)
 sort.Slice(items,func(i,j int)bool{
  if items[i].ActorNamespace!=items[j].ActorNamespace{return items[i].ActorNamespace<items[j].ActorNamespace}
  return items[i].ActorID<items[j].ActorID
 })
 refs:=make([]any,0,len(items))
 seen:=map[string]bool{}
 for _,c:=range items{
  key:=c.ActorNamespace+"\x00"+c.ActorID
  if seen[key]{return nil,errors.New("duplicated epistemic checkpoint actor")}
  seen[key]=true
  for _,name:=range []string{c.ActorNamespace,c.ActorID,c.StoreID,c.Producer}{
   if err:=domain.ValidateMachineID(name);err!=nil{return nil,fmt.Errorf("checkpoint identifier: %w",err)}
  }
  if c.SchemaVersion==0||c.ProducerVersion==0||c.CoversInstant<instant||
   c.CoversRevision<rev||c.CoversEvidenceCursor<cursor {
   return nil,errors.New("epistemic checkpoint is stale or versionless")
  }
  hash:=identity.OpaqueContentHash(c.Content)
  refs=append(refs,[]any{[]any{c.ActorNamespace,c.ActorID},c.StoreID,uint64(c.SchemaVersion),
   []any{uint64(1),bytes.Clone(c.Content)},hash[:],c.Producer,uint64(c.ProducerVersion),
   []any{c.CoversInstant,c.CoversRevision,c.CoversEvidenceCursor}})
 }
 return refs,nil
}

// CreateSnapshotV1 publishes an immutable checkpoint using the registered
// snapshot ID/hash and the exact five-ledger cursor order. A replay audit runs
// before publication; a single SQLite writer lock freezes subsequent changes.
func (s *Store) CreateSnapshotV1(ctx context.Context,run [32]byte,b *policy.Bundle,checkpoints []SnapshotCheckpoint)(SnapshotV1,error){
 var zero SnapshotV1
 if s==nil||b==nil{return zero,errors.New("snapshot requires store and pinned policy")}
 if _,err:=s.ReplayClock(ctx,run,b);err!=nil{return zero,fmt.Errorf("snapshot replay preflight: %w",err)}
 if err:=s.VerifyCycleControl(ctx,run);err!=nil{return zero,err}
 tx,err:=s.db.BeginTx(ctx,nil);if err!=nil{return zero,err}
 defer tx.Rollback()
 // Acquiring the RESERVED lock before reading prevents a concurrent terminal
 // append through another SQLite connection from altering this checkpoint.
 if _,err:=tx.ExecContext(ctx,"UPDATE clock_states SET canonical=canonical WHERE run_id=?",run[:]);err!=nil{return zero,err}
 var raw []byte
 if err:=tx.QueryRowContext(ctx,"SELECT canonical FROM genesis_records WHERE run_id=?",run[:]).Scan(&raw);err!=nil{return zero,err}
 g,err:=genesis.ValidateCanonical(raw,b);if err!=nil{return zero,err}
 if [32]byte(g.RunID)!=run{return zero,errors.New("snapshot genesis run mismatch")}
 var clockRaw []byte
 if err:=tx.QueryRowContext(ctx,"SELECT canonical FROM clock_states WHERE run_id=?",run[:]).Scan(&clockRaw);err!=nil{return zero,err}
 state,err:=clock.Decode(clockRaw);if err!=nil{return zero,err}
 var revRaw,revHash []byte
 if err:=tx.QueryRowContext(ctx,"SELECT canonical,state_hash FROM world_revisions WHERE revision=?",uint64(state.Revision)).Scan(&revRaw,&revHash);err!=nil{return zero,err}
 world,err:=codec.StrictDecodePayload(revRaw);if err!=nil{return zero,err}
 wf,ok:=world.([]any);if !ok||len(wf)!=6||!equalBytes(wf[5],revHash){return zero,errors.New("snapshot world revision malformed")}
 typed,ok:=wf[4].([]any);if !ok||len(typed)!=4{return zero,errors.New("snapshot world typed-state malformed")}
 cursors:=make([]any,0,5)
 for _,name:=range snapshotLedgerNames{
  c,err:=snapshotLedgerPrefix(ctx,tx,name,b);if err!=nil{return zero,err}
  cursors=append(cursors,c)
 }
 evidenceCount:=cursors[3].([]any)[1].(uint64)
 refs,err:=snapshotCheckpointRefs(checkpoints,int64(state.Instant),uint64(state.Revision),evidenceCount)
 if err!=nil{return zero,err}
 pending,err:=snapshotPending(ctx,tx,run,b);if err!=nil{return zero,err}
 control:=[]any{uint64(0),[]any{uint64(0)},[]any{uint64(0)},[]any{uint64(0)},[]any{uint64(0)},[]any{uint64(0)},uint64(0)}
 var ctlRaw []byte
 switch err:=tx.QueryRowContext(ctx,"SELECT canonical FROM cycle_control_state WHERE run_id=?",run[:]).Scan(&ctlRaw);err{
 case nil:
  parsed,e:=codec.StrictDecodePayload(ctlRaw);if e!=nil{return zero,e}
  c,ok:=parsed.([]any);if !ok||len(c)!=7{return zero,errors.New("snapshot control malformed")}
  control=c
 case sql.ErrNoRows:
 default:return zero,err
 }
 fields:=[]any{
  run[:],uint64(1),int64(state.Instant),uint64(state.Revision),uint64(state.NextLogicalSequence),state.LastOrdinal,
  typed,bytes.Clone(revHash),g.WorldSeed[:],[]any{g.ID[:],g.Digest[:]},g.Payload[14],
  pending,cursors,control,refs,policy.CodecPolicyID,b.CodecPolicyHash[:],b.SchemaHash[:],
 }
 hash,_,err:=identity.New(b).Sum("cote.csf.hash.snapshot","cote.csf.schema.snapshot.body",1,fields)
 if err!=nil{return zero,err}
 id,_,err:=identity.New(b).Sum("cote.csf.id.snapshot","cote.csf.schema.snapshot-id-preimage",1,
  []any{run[:],uint64(1),uint64(state.Revision)})
 if err!=nil{return zero,err}
 canonical,err:=codec.CanonicalPayload(append(fields,hash[:]));if err!=nil{return zero,err}
 res,err:=tx.ExecContext(ctx,"INSERT OR IGNORE INTO snapshots(id,canonical) VALUES(?,?)",id[:],canonical)
 if err!=nil{return zero,err}
 n,err:=res.RowsAffected();if err!=nil{return zero,err}
 if n==0{
  var existing []byte
  if err:=tx.QueryRowContext(ctx,"SELECT canonical FROM snapshots WHERE id=?",id[:]).Scan(&existing);err!=nil{return zero,err}
  if !bytes.Equal(existing,canonical){return zero,errors.New("snapshot same-revision identity collision")}
 }
 if err:=tx.Commit();err!=nil{return zero,err}
 return SnapshotV1{ID:id,Digest:hash,Canonical:canonical,Revision:uint64(state.Revision)},nil
}

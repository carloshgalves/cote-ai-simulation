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

// snapshotPending is a deterministic projection of the complete linked
// pending-state model. Ref digests use the registered record operation or
// the persisted unit digest, never a surrogate hash of arbitrary bytes.
func snapshotPending(ctx context.Context,tx *sql.Tx,run [32]byte,b *policy.Bundle)([]any,error){
 var parts [10][]any
 for i:=range parts {parts[i]=[]any{}}
 deriver:=identity.New(b)
 digestRef:=func(tag,schema string,fields []any)([]byte,error){
  d,_,err:=deriver.Sum(tag,schema,1,fields)
  if err!=nil{return nil,err}
  return d[:],nil
 }
 typed:=func(table,query string,index int,kind uint64,recordDigest func([]any)([]byte,error))error{
  rows,err:=tx.QueryContext(ctx,query,run[:]);if err!=nil{return err}
  defer rows.Close()
  for rows.Next(){
   var id,raw []byte
   if err:=rows.Scan(&id,&raw);err!=nil{return err}
   if len(id)!=32{return errors.New("snapshot authority id wrong length")}
   decoded,err:=codec.StrictDecodePayload(raw);if err!=nil{return err}
   fields,ok:=decoded.([]any);if !ok||len(fields)==0||!equalBytes(fields[0],id){return errors.New("snapshot authority id and payload differ")}
   if recordDigest==nil {
    parts[index]=append(parts[index],fields)
   }else{
    d,err:=recordDigest(fields);if err!=nil{return err}
    if len(d)!=32{return errors.New("snapshot reference digest invalid")}
    parts[index]=append(parts[index],[]any{kind,bytes.Clone(id),d})
   }
  }
  return rows.Err()
 }
 // Pending occurrence and activation membership is checked by pendingClock.
 // Their unit digests live at the tail of the canonical immutable record.
 lastDigest:=func(f []any)([]byte,error){
  d,ok:=f[len(f)-1].([]byte)
  if !ok||len(d)!=32{return nil,errors.New("invalid scheduled unit digest")}
  return bytes.Clone(d),nil
 }
 if err:=typed("scheduled_occurrences","SELECT id,canonical FROM scheduled_occurrences WHERE run_id=? AND lifecycle=0 ORDER BY id",0,6,lastDigest);err!=nil{return nil,err}
 if err:=typed("trigger_runtime","SELECT definition_id,canonical FROM trigger_runtime WHERE run_id=? ORDER BY definition_id",1,0,nil);err!=nil{return nil,err}
 if err:=typed("trigger_activations","SELECT id,canonical FROM trigger_activations WHERE run_id=? AND lifecycle=0 ORDER BY id",2,7,lastDigest);err!=nil{return nil,err}
 pending,err:=pendingClock(ctx,tx,run);if err!=nil{return nil,err}
 live:=map[[32]byte]bool{}
 for _,item:=range pending{live[[32]byte(item.ID)]=true}
 // Input authorities: closure finality, RoundDeclaration, active dispatch,
 // response and exogenous unit are independently pinned by InputLedger replay.
 rows,err:=tx.QueryContext(ctx,"SELECT id,canonical FROM input_ledger ORDER BY id")
 if err!=nil{return nil,err}
 type closure struct{source string;seq uint64;ref any}
 closings:=map[string]closure{}
 dispatches:=map[[32]byte]any{}
 answered:=map[[32]byte]bool{}
 revoked:=map[[32]byte]bool{}
 for rows.Next(){
  var id,raw []byte
  if err:=rows.Scan(&id,&raw);err!=nil{rows.Close();return nil,err}
  if len(id)!=32{rows.Close();return nil,errors.New("snapshot input id malformed")}
  var key [32]byte;copy(key[:],id)
  if env,err:=codec.StrictDecodeEnvelope(raw);err==nil{
   if env.DomainTag!="cote.csf.record.source-ingress-receipt"{rows.Close();return nil,errors.New("unexpected enveloped input record")}
   continue
  }
  data,err:=codec.StrictDecodePayload(raw);if err!=nil{rows.Close();return nil,err}
  f,ok:=data.([]any);if !ok||len(f)==0{rows.Close();return nil,errors.New("unrecognised snapshot input record")}
  if len(f)==3&&f[0]=="source-declaration-v1"{continue}
  if len(f)==3{
   rawDispatch,ok:=f[0].([]byte);if !ok||len(rawDispatch)!=32{rows.Close();return nil,errors.New("bad dispatch revocation")}
   var dkey [32]byte;copy(dkey[:],rawDispatch);revoked[dkey]=true
   continue
  }
  if !equalBytes(f[0],id){rows.Close();return nil,errors.New("input record id mismatch")}
  if len(f)==7{
   source,ok:=f[2].(string);seq,okSeq:=f[6].(uint64)
   if !ok||!okSeq||!equalBytes(f[1],run[:]){rows.Close();return nil,errors.New("source closure malformed")}
   d,err:=digestRef("cote.csf.digest.source-closure","cote.csf.schema.source-closure",f)
   if err!=nil{rows.Close();return nil,err}
   if prev,exists:=closings[source];!exists||seq>prev.seq{closings[source]=closure{source,seq,[]any{uint64(2),bytes.Clone(id),d}}}
   continue
  }
  if len(f)==11{
   if source,ok:=f[2].(string);ok{
    if !equalBytes(f[1],run[:]){rows.Close();return nil,errors.New("cross-run pending round")}
    if live[key]{
     d,err:=digestRef("cote.csf.digest.round-declaration","cote.csf.schema.round-declaration",f)
     if err!=nil{rows.Close();return nil,err}
     parts[4]=append(parts[4],[]any{uint64(3),bytes.Clone(id),d})
     parts[7]=append(parts[7],[]any{uint64(4),bytes.Clone(id),d})
    }
    _=source;continue
   }
   if _,ok:=f[4].(string);ok{
    if !equalBytes(f[1],run[:]){rows.Close();return nil,errors.New("cross-run dispatch")}
    d,err:=digestRef("cote.csf.digest.slot-dispatch","cote.csf.schema.slot-dispatch",f)
    if err!=nil{rows.Close();return nil,err}
    dispatches[key]=[]any{uint64(4),bytes.Clone(id),d}
    continue
   }
   // NoProposal slot response (the final field is its unit digest).
   if !equalBytes(f[2],run[:]){rows.Close();return nil,errors.New("cross-run NoProposal")}
   d,err:=lastDigest(f);if err!=nil{rows.Close();return nil,err}
   parts[6]=append(parts[6],[]any{uint64(23),bytes.Clone(id),d})
   rawDispatch,ok:=f[1].([]byte);if !ok||len(rawDispatch)!=32{rows.Close();return nil,errors.New("NoProposal dispatch invalid")}
   var k [32]byte;copy(k[:],rawDispatch);answered[k]=true
   continue
  }
  if len(f)==19{
   if !equalBytes(f[2],run[:]){rows.Close();return nil,errors.New("cross-run proposal")}
   d,err:=lastDigest(f);if err!=nil{rows.Close();return nil,err}
   parts[6]=append(parts[6],[]any{uint64(5),bytes.Clone(id),d})
   rawDispatch,ok:=f[1].([]byte);if !ok||len(rawDispatch)!=32{rows.Close();return nil,errors.New("proposal dispatch invalid")}
   var k [32]byte;copy(k[:],rawDispatch);answered[k]=true
   continue
  }
  if len(f)==13{
   if !equalBytes(f[1],run[:]){rows.Close();return nil,errors.New("cross-run exogenous input")}
   if live[key]{
    d,err:=lastDigest(f);if err!=nil{rows.Close();return nil,err}
    parts[7]=append(parts[7],[]any{uint64(1),bytes.Clone(id),d})
   }
   continue
  }
  rows.Close();return nil,errors.New("unknown input record shape in snapshot")
 }
 if err:=rows.Err();err!=nil{rows.Close();return nil,err}
 if err:=rows.Close();err!=nil{return nil,err}
 keys:=make([]string,0,len(closings))
 for key:=range closings{keys=append(keys,key)}
 sort.Strings(keys)
 for _,key:=range keys{parts[3]=append(parts[3],closings[key].ref)}
 dispatchKeys:=make([][32]byte,0,len(dispatches))
 for key:=range dispatches{dispatchKeys=append(dispatchKeys,key)}
 sort.Slice(dispatchKeys,func(i,j int)bool{return bytes.Compare(dispatchKeys[i][:],dispatchKeys[j][:])<0})
 for _,key:=range dispatchKeys{if !answered[key]&&!revoked[key]{parts[5]=append(parts[5],dispatches[key])}}
 // Scheduled/trigger work contributes input-ref, distinct from causal-ref.
 for _,p:=range []int{0,2}{
  for _,raw:=range parts[p]{
   ref:=raw.([]any)
   kind:=uint64(2);if p==2{kind=3}
   if id,ok:=idField(ref[1]);ok&&live[id]{
    parts[7]=append(parts[7],[]any{kind,ref[1],ref[2]})
   }
  }
 }
 // Outbox tasks are not evidence. Only tasks without a committed completion
 // remain pending; completed receipts are included in full canonical form.
 out,err:=tx.QueryContext(ctx,"SELECT id,canonical FROM causal_outbox ORDER BY id")
 if err!=nil{return nil,err}
 tasks:=map[[32]byte]any{}
 completed:=map[[32]byte]bool{}
 for out.Next(){
  var id,raw []byte
  if err:=out.Scan(&id,&raw);err!=nil{out.Close();return nil,err}
  if len(id)!=32{out.Close();return nil,errors.New("outbox identity wrong length")}
  decoded,err:=codec.StrictDecodePayload(raw);if err!=nil{out.Close();return nil,err}
  f,ok:=decoded.([]any);if !ok||len(f)==0||!equalBytes(f[0],id){out.Close();return nil,errors.New("outbox canonical id changed")}
  if len(f)==10 {
   if !equalBytes(f[1],run[:]){out.Close();return nil,errors.New("foreign outbox task")}
   digest,err:=digestRef("cote.csf.digest.perception-task","cote.csf.schema.perception-task",f)
   if err!=nil{out.Close();return nil,err}
   var key [32]byte;copy(key[:],id);tasks[key]=[]any{uint64(15),bytes.Clone(id),digest}
  } else if len(f)==5 {
   rawTask,ok:=f[1].([]byte);if !ok||len(rawTask)!=32{out.Close();return nil,errors.New("completion task ref invalid")}
   d,err:=digestRef("cote.csf.digest.perception-completion","cote.csf.schema.perception-task-completion.body",f[:4])
   if err!=nil{out.Close();return nil,err}
   if !equalBytes(f[4],d){out.Close();return nil,errors.New("completion digest altered")}
   var k [32]byte;copy(k[:],rawTask);completed[k]=true
   parts[9]=append(parts[9],f)
  }else{out.Close();return nil,errors.New("unregistered outbox record")}
 }
 if err:=out.Err();err!=nil{out.Close();return nil,err}
 if err:=out.Close();err!=nil{return nil,err}
 orderedTaskIDs:=make([][32]byte,0,len(tasks))
 for k:=range tasks{orderedTaskIDs=append(orderedTaskIDs,k)}
 sort.Slice(orderedTaskIDs,func(i,j int)bool{return bytes.Compare(orderedTaskIDs[i][:],orderedTaskIDs[j][:])<0})
 for _,k:=range orderedTaskIDs{if !completed[k]{parts[8]=append(parts[8],tasks[k])}}
 for k:=range completed{if _,ok:=tasks[k];!ok{return nil,errors.New("orphan perception completion in snapshot")}}
 fields:=make([]any,len(parts))
 for i,v:=range parts {fields[i]=v}
 hash,encoded,err:=deriver.Sum("cote.csf.snapshot.pending-state","cote.csf.schema.snapshot-pending-state",1,fields)
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
 clockBefore,err:=s.LoadClock(ctx,run);if err!=nil{return zero,err}
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
 if state!=clockBefore{return zero,errors.New("clock changed between replay audit and snapshot writer lock")}
 if err:=verifyPendingOutboxExact(ctx,tx,run);err!=nil{return zero,err}
 if err:=verifyEvidenceRegistryExact(ctx,tx);err!=nil{return zero,err}
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
 if err:=verifySnapshotCheckpointRefs(ctx,tx,run,refs,int64(state.Instant),uint64(state.Revision),evidenceCount);err!=nil{return zero,err}
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

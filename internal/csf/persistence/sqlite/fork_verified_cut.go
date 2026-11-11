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
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

// verifiedForkCut is an immutable-by-contract view of the authenticated parent
// cut. Canonical CBOR structures remain private, decoded exactly once, and
// are only READ by later phases. Partition values are checked before exposure;
// fuller semantic typing belongs to #57 rather than to this phase.
type verifiedForkCut struct {
 resume ResumeCheckpointV1
 parent *genesis.Genesis
 snapshot []any
 epistemic []any
 pending [10][]any
 reference []byte
}

// prepareVerifiedForkCut preserves the pre-migration validation/read order.
// It never writes to the parent or child, and it does not attempt to capture
// the five ledgers early: the exact-prefix archive is verified at its existing
// later read-transaction boundary (verifiedForkArchive below).
func (s *Store) prepareVerifiedForkCut(ctx context.Context,sourceRun,parentSnapshotID,childRun [32]byte,
 child *Store,origin,childPolicy *policy.Bundle,crossPolicy bool)(verifiedForkCut,error){
 var zero verifiedForkCut
 if s==nil||child==nil||origin==nil||childPolicy==nil||s==child{return zero,errors.New("fork requires distinct parent/child stores and policy")}
 if sourceRun==childRun||childRun==([32]byte{}){return zero,errors.New("fork must allocate a new nonzero run ID")}
 resume,err:=s.VerifySnapshotV1(ctx,sourceRun,parentSnapshotID,origin)
 if err!=nil{return zero,fmt.Errorf("fork source checkpoint replay: %w",err)}
 if resume.SnapshotControlStatus!=0{return zero,errors.New("fork origin checkpoint must be IDLE")}
 parent,err:=s.ValidatedGenesis(ctx,sourceRun,origin)
 if err!=nil{return zero,err}
 lineage,ok:=parent.Payload[4].([]any)
 if !ok||len(lineage)!=1{
  return zero,errors.New("recursive fork origin requires a transitive lineage archive")
 }
 snapRaw,err:=codec.StrictDecodePayload(resume.Snapshot.Canonical)
 if err!=nil{return zero,err}
 snapshot,ok:=snapRaw.([]any);if !ok||len(snapshot)!=19{return zero,errors.New("source snapshot schema invalid")}
 epistemic,ok:=snapshot[14].([]any)
 if !ok{return zero,errors.New("fork epistemic snapshot roster malformed")}
 if len(epistemic)>0&&!crossPolicy{
  return zero,errors.New("V1 fork requires versioned migration of opaque cognitive checkpoints")
 }
 // V2 imports only exact inlined, signed parent checkpoint bytes. The
 // ownership/eligibility roster is reconstructed below, and lineage replay
 // binds it to the immutable archived parent checkpoint.
 if crossPolicy{
  for _,raw:=range epistemic{
   entry,ok:=raw.([]any)
   if !ok||len(entry)!=8{return zero,errors.New("V2 cognition checkpoint ref malformed")}
   inline,ok:=entry[3].([]any)
   if !ok||len(inline)!=2||inline[0]!=uint64(1){return zero,errors.New("V2 cannot migrate external cognition checkpoint")}
  }
 }
 typed,ok:=snapshot[11].([]any);if !ok||len(typed)!=4{return zero,errors.New("source pending envelope absent")}
 pendingEnvelope,ok:=typed[2].([]byte)
 if !ok{return zero,errors.New("source pending envelope malformed")}
 penv,err:=codec.StrictDecodeEnvelope(pendingEnvelope);if err!=nil{return zero,err}
 lists,ok:=penv.Value.([]any);if !ok||len(lists)!=10{return zero,errors.New("unsupported pending-state schema")}
 if crossPolicy {
  // These partitions are handled by typed migrators below. Incomplete
  // perception work still requires parent settlement before the fork.
  for i,raw:=range lists {
   if i==0||i==1||i==2||i==3||i==4||i==5||i==6||i==7||i==9{continue} // migrated rounds and completed receipts
   items,ok:=raw.([]any)
   if !ok||len(items)!=0{return zero,fmt.Errorf("V1-to-V2 fork needs migrator for pending partition %d",i)}
  }
 }
 for i,raw:=range lists{
  items,ok:=raw.([]any)
  if !ok{return zero,fmt.Errorf("fork pending partition %d is malformed",i)}
  // Latest SourceClosure, historical slot responses and completed perception
  // receipts are historical proofs, not pending work. Keep them in the
  // frozen parent prefix rather than transplanting their run-bound IDs.
  if i==3||i==6||i==9||i==0||i==1||i==2||i==4||(crossPolicy&&i==5){continue}
  // An explicitly migrated genesis schedule contributes one pending input
  // ref (kind=2) for each immutable occurrence. Unknown source kinds fail
  // closed, rather than discarding the parent pending work.
  if i==7{
   pending,ok:=lists[0].([]any)
   if !ok{return zero,errors.New("fork pending schedule roster malformed")}
   activations,ok:=lists[2].([]any)
   if !ok{return zero,errors.New("fork activation roster malformed")}
   rounds,ok:=lists[4].([]any)
   if !ok||len(items)!=len(pending)+len(activations)+len(rounds){
    return zero,errors.New("fork pending input roster not fully supported")
   }
   expected:=map[string]bool{}
   for _,v:=range pending{
    ref,ok:=v.([]any);if !ok||len(ref)!=3{return zero,errors.New("fork genesis ref malformed")}
    id,ok:=ref[1].([]byte);if !ok||len(id)!=32{return zero,errors.New("fork genesis ref missing ID")}
    expected[string(id)]=true
   }
   for _,v:=range activations{
    ref,ok:=v.([]any)
    if !ok||len(ref)!=3||ref[0]!=uint64(7){return zero,errors.New("fork activation ref malformed")}
    id,ok:=ref[1].([]byte)
    if !ok||len(id)!=32||expected[string(id)]{return zero,errors.New("fork duplicate activation ref")}
    expected[string(id)]=true
   }
   for _,v:=range rounds{
    ref,ok:=v.([]any)
    if !ok||len(ref)!=3||ref[0]!=uint64(3){return zero,errors.New("fork pending round ref malformed")}
    id,ok:=ref[1].([]byte)
    if !ok||len(id)!=32||expected[string(id)]{return zero,errors.New("fork duplicate round input ref")}
    expected[string(id)]=true
   }
   for _,v:=range items{
    ref,ok:=v.([]any);if !ok||len(ref)!=3||
     (ref[0]!=uint64(2)&&ref[0]!=uint64(3)&&ref[0]!=uint64(4)){
     return zero,errors.New("fork cannot rebind unsupported pending input")
    }
    id,ok:=ref[1].([]byte)
    if !ok||len(id)!=32||!expected[string(id)]{return zero,errors.New("fork pending input does not match Genesis migration sources")}
    delete(expected,string(id))
   }
   if len(expected)!=0{return zero,errors.New("fork pending input omits a Genesis source")}
   continue
  }
  if len(items)!=0{
   return zero,fmt.Errorf("fork requires a versioned migrator for pending partition %d",i)
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
  if !ok||len(ref)!=3||ref[0]!=uint64(2){return zero,errors.New("fork source closure reference malformed")}
  id,ok:=ref[1].([]byte)
  if !ok||len(id)!=32{return zero,errors.New("fork source closure ID malformed")}
  var canonical []byte
  if err:=s.db.QueryRowContext(ctx,"SELECT canonical FROM input_ledger WHERE id=?",id).Scan(&canonical);err!=nil{
   return zero,fmt.Errorf("fork source closure absent from origin history: %w",err)
  }
  decoded,err:=codec.StrictDecodePayload(canonical);if err!=nil{return zero,err}
  record,ok:=decoded.([]any)
  if !ok||len(record)!=7||record[5]!=true||!equalBytes(record[0],id)||
   !equalBytes(record[1],sourceRun[:]){
   return zero,errors.New("fork would lose an unfinished or invalid source cursor")
  }
  name,ok:=record[2].(string);if !ok||name==""{return zero,errors.New("fork source id malformed")}
  finalSourceNames[name]=true
 }
 sourceRows,err:=s.db.QueryContext(ctx,
  "SELECT t.canonical FROM snapshot_prefix_members m "+
   "JOIN input_ledger t ON t.id=m.record_id WHERE m.snapshot_id=? "+
   "AND m.ledger_name='input_ledger' ORDER BY m.ordinal",parentSnapshotID[:])
 if err!=nil{return zero,err}
 for sourceRows.Next(){
  var canonical []byte
  if err:=sourceRows.Scan(&canonical);err!=nil{sourceRows.Close();return zero,err}
  record,err:=codec.StrictDecodePayload(canonical)
  if err!=nil{continue} // an enveloped SourceIngressReceipt cannot declare a new source
  fields,ok:=record.([]any)
  if !ok||len(fields)!=3||fields[0]!="source-declaration-v1"{continue}
  sourceID,ok:=fields[2].(string)
  if !ok||!finalSourceNames[sourceID]{
   sourceRows.Close();return zero,errors.New("fork has a declared but non-final exogenous source")
  }
 }
 if err:=sourceRows.Err();err!=nil{sourceRows.Close();return zero,err}
 if err:=sourceRows.Close();err!=nil{return zero,err}
 // Parent's registered snapshot body hash is distinct from the exact-byte
 // digest in CSFHREF. The latter binds immutable archive content across
 // origin/child policy versions without rehashing historical identities.
 href,err:=fork.NewSnapshotReference(origin,resume.Snapshot.Canonical)
 if err!=nil{return zero,err}
 refRaw,err:=fork.Encode(href);if err!=nil{return zero,err}
 // Always resolve the immutable origin by its own id+hash before
 // interpreting its checkpoint, even for the same-policy V1 subset.
 installed,err:=fork.NewInstalledPolicyRegistry(origin)
 if err!=nil{return zero,err}
 _,verifiedOrigin,err:=fork.VerifyOriginWithResolver(ctx,refRaw,resume.Snapshot.Canonical,installed)
 if err!=nil{return zero,err}
 if verifiedOrigin!=origin{return zero,errors.New("fork origin policy unexpectedly switched during validation")}

 var partitions [10][]any
 for i,raw:=range lists{
  items,ok:=raw.([]any)
  if !ok{return zero,fmt.Errorf("fork pending partition %d is malformed",i)}
  partitions[i]=items
 }
 return verifiedForkCut{
  resume:resume,parent:parent,snapshot:snapshot,epistemic:epistemic,
  pending:partitions,reference:bytes.Clone(refRaw),
 },nil
}

// forkArchiveRecord holds the exact old ledger identity/canonical bytes. It
// is not a live authority of the child. The record ID gets an explicitly
// checked 32-byte representation instead of an opaque unchecked slice.
type forkArchiveRecord struct{
 id [32]byte
 canonical []byte
}

// verifiedForkArchive runs at the ORIGINAL read-only boundary after the
// migrators have prepared their births. Moving it before planning would change
// concurrent-parent semantics, so this P1 refactor deliberately does not.
func (s *Store) verifiedForkArchive(ctx context.Context,parentSnapshotID [32]byte,
 snapshot []any,origin *policy.Bundle)([5][]forkArchiveRecord,error){
 var archived [5][]forkArchiveRecord
 if len(snapshotLedgerNames)!=len(archived){return archived,errors.New("fork ledger inventory differs from five-prefix contract")}
 originTx,err:=s.db.BeginTx(ctx,&sql.TxOptions{ReadOnly:true})
 if err!=nil{return archived,err}
 defer originTx.Rollback()
 cursors,ok:=snapshot[12].([]any)
 if !ok||len(cursors)!=len(archived){return archived,errors.New("fork snapshot cursor roster malformed")}
 for i,name:=range snapshotLedgerNames{
  proof,err:=snapshotVerifiedPrefix(ctx,originTx,parentSnapshotID,name,origin)
  if err!=nil{return archived,err}
  if !sameCanonical(cursors[i],proof){return archived,errors.New("origin prefix changed while preparing fork")}
  rows,err:=originTx.QueryContext(ctx,
   "SELECT m.ordinal,m.record_id,t.canonical FROM snapshot_prefix_members m "+
    "JOIN "+name+" t ON t.id=m.record_id WHERE m.snapshot_id=? AND m.ledger_name=? ORDER BY m.ordinal",
   parentSnapshotID[:],name)
  if err!=nil{return archived,err}
  for rows.Next(){
   var ordinal int64
   var id,raw []byte
   if err:=rows.Scan(&ordinal,&id,&raw);err!=nil{rows.Close();return archived,err}
   if ordinal!=int64(len(archived[i]))||len(id)!=32{
    rows.Close();return archived,errors.New("origin archive order malformed")
   }
   var recordID [32]byte
   copy(recordID[:],id)
   archived[i]=append(archived[i],forkArchiveRecord{recordID,bytes.Clone(raw)})
  }
  if err:=rows.Err();err!=nil{rows.Close();return archived,err}
  if err:=rows.Close();err!=nil{return archived,err}
 }
 if err:=originTx.Commit();err!=nil{return archived,err}
 return archived,nil
}

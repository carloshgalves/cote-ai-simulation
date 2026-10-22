package sqlite

import (
 "bytes"
 "context"
 "database/sql"
 "errors"
 "fmt"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/clock"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

// ResumeCheckpointV1 audits persisted causal authority before a caller may
// resume. It never invokes a model, reissues a dispatch or changes control state.
// The returned clock/control are verified, not synthesized from an external hint.
type ResumeCheckpointV1 struct{
 Snapshot SnapshotV1
 // SnapshotClock is the terminal clock at the sealed revision; Clock is the
 // causally replayed *current* tip after applying every later tail.
 SnapshotClock clock.State
 Clock clock.State
 TailCommits uint64
 ControlStatus uint64
 PendingTasks int
}

// VerifySnapshotV1 fails closed if a snapshot, ledger prefix, pending runtime,
// genesis policy, epistemic checkpoint or journal has been corrupted. V1
// verifies the sealed snapshot prefix and complete subsequent journal tails.
// It never rewinds authoritative SQLite tables or discards later commits.
func (s *Store) VerifySnapshotV1(ctx context.Context,run,id [32]byte,b *policy.Bundle)(ResumeCheckpointV1,error){
 var zero ResumeCheckpointV1
 if s==nil||b==nil{return zero,errors.New("resume requires store and pinned policy")}
 if _,err:=s.ReplayClock(ctx,run,b);err!=nil{return zero,fmt.Errorf("resume causal replay: %w",err)}
 if err:=s.VerifyCycleControl(ctx,run);err!=nil{return zero,err}
 tx,err:=s.db.BeginTx(ctx,&sql.TxOptions{ReadOnly:true});if err!=nil{return zero,err}
 defer tx.Rollback()
 var canonical []byte
 if err:=tx.QueryRowContext(ctx,"SELECT canonical FROM snapshots WHERE id=?",id[:]).Scan(&canonical);err!=nil{return zero,fmt.Errorf("snapshot missing: %w",err)}
 raw,err:=codec.StrictDecodePayload(canonical);if err!=nil{return zero,err}
 all,ok:=raw.([]any);if !ok||len(all)!=19{return zero,errors.New("snapshot has unknown canonical schema")}
 if !equalBytes(all[0],run[:])||all[1]!=uint64(1) {return zero,errors.New("snapshot run or schema version mismatch")}
 rev,ok:=all[3].(uint64);if !ok{return zero,errors.New("snapshot revision invalid")}
 wantedID,_,err:=identity.New(b).Sum("cote.csf.id.snapshot","cote.csf.schema.snapshot-id-preimage",1,[]any{run[:],uint64(1),rev})
 if err!=nil{return zero,err}
 if id!=wantedID{return zero,errors.New("snapshot ID preimage mismatch")}
 hash,_,err:=identity.New(b).Sum("cote.csf.hash.snapshot","cote.csf.schema.snapshot.body",1,all[:18])
 if err!=nil{return zero,err}
 if !equalBytes(all[18],hash[:]){return zero,errors.New("snapshot content hash mismatch")}
 if err:=verifySnapshotSeal(ctx,tx,id,run,[32]byte(hash),rev);err!=nil{return zero,err}
 if all[15]!=policy.CodecPolicyID||!equalBytes(all[16],b.CodecPolicyHash[:])||!equalBytes(all[17],b.SchemaHash[:]){
  return zero,errors.New("snapshot codec/schema policy unavailable or changed")
 }
 var genesisRaw,clockRaw,worldRaw,worldHash,controlRaw []byte
 if err:=tx.QueryRowContext(ctx,"SELECT canonical FROM genesis_records WHERE run_id=?",run[:]).Scan(&genesisRaw);err!=nil{return zero,err}
 g,err:=genesisValidateForSnapshot(genesisRaw,b);if err!=nil{return zero,err}
 if !equalBytes(all[8],g.Seed[:])||!sameCanonical(all[9],[]any{g.ID[:],g.Digest[:]})||!sameCanonical(all[10],g.Policies){
  return zero,errors.New("snapshot genesis or policy refs mismatch")
 }
 if err:=tx.QueryRowContext(ctx,"SELECT canonical FROM clock_states WHERE run_id=?",run[:]).Scan(&clockRaw);err!=nil{return zero,err}
 c,err:=clock.Decode(clockRaw);if err!=nil{return zero,err}
 if rev>uint64(c.Revision){return zero,errors.New("snapshot revision is in the future")}
 hist,err:=snapshotHistoricalClock(ctx,tx,run,g.InitialInstant,rev)
 if err!=nil{return zero,err}
 if !sameCanonical(all[2],hist.Instant)||!sameCanonical(all[4],hist.NextSequence)||!sameCanonical(all[5],hist.LastOrdinal){
  return zero,errors.New("snapshot clock differs from independently replayed historical journal")
 }
 if err:=tx.QueryRowContext(ctx,"SELECT canonical,state_hash FROM world_revisions WHERE revision=?",rev).Scan(&worldRaw,&worldHash);err!=nil{return zero,err}
 world,err:=codec.StrictDecodePayload(worldRaw);if err!=nil{return zero,err}
 wf,ok:=world.([]any);if !ok||len(wf)!=6||!sameCanonical(all[6],wf[4])||!equalBytes(all[7],worldHash){
  return zero,errors.New("snapshot world state differs from world revision")
 }
 cursors,ok:=all[12].([]any);if !ok||len(cursors)!=5{return zero,errors.New("snapshot must contain exactly five ledger prefixes")}
 for i,name:=range snapshotLedgerNames{
  sealed,err:=snapshotVerifiedPrefix(ctx,tx,id,name,b);if err!=nil{return zero,err}
  if !sameCanonical(sealed,cursors[i]){return zero,fmt.Errorf("snapshot %s historical prefix proof changed",name)}
  if rev==uint64(c.Revision){
   current,err:=snapshotLedgerPrefix(ctx,tx,name,b);if err!=nil{return zero,err}
   if !sameCanonical(current,sealed){return zero,fmt.Errorf("tip snapshot %s cursor or prefix changed",name)}
  }
 }
 snapshotPendingValue,ok:=all[11].([]any)
 if !ok||len(snapshotPendingValue)!=4||snapshotPendingValue[0]!="cote.csf.schema.snapshot-pending-state"||
  snapshotPendingValue[1]!=uint64(1){return zero,errors.New("historical pending-state schema unsupported")}
 snapshotRaw,ok:=snapshotPendingValue[2].([]byte);if !ok{return zero,errors.New("historical pending envelope missing")}
 snapshotDigest:=identity.OpaqueContentHash(snapshotRaw)
 if !equalBytes(snapshotPendingValue[3],snapshotDigest[:]){return zero,errors.New("historical pending envelope hash mismatch")}
 snapshotEnv,err:=codec.StrictDecodeEnvelope(snapshotRaw);if err!=nil{return zero,err}
 snapshotLists,ok:=snapshotEnv.Value.([]any)
 if !ok||len(snapshotLists)!=10||snapshotEnv.SchemaID!="cote.csf.schema.snapshot-pending-state"||
  snapshotEnv.SchemaVersion!=1{return zero,errors.New("historical pending-state envelope incompatible")}
 currentPending,err:=snapshotPending(ctx,tx,run,b);if err!=nil{return zero,err}
 if rev==uint64(c.Revision)&&!sameCanonical(currentPending,all[11]){return zero,errors.New("tip snapshot pending state differs from authority")}
 controlStatus:=uint64(0)
 err=tx.QueryRowContext(ctx,"SELECT canonical FROM cycle_control_state WHERE run_id=?",run[:]).Scan(&controlRaw)
 if err!=nil&&err!=sql.ErrNoRows{return zero,err}
 if err==sql.ErrNoRows{
  idle:=[]any{uint64(0),[]any{uint64(0)},[]any{uint64(0)},[]any{uint64(0)},[]any{uint64(0)},[]any{uint64(0)},uint64(0)}
  if rev==uint64(c.Revision)&&!sameCanonical(all[13],idle){return zero,errors.New("tip snapshot wrongly claims a non-IDLE control state")}
 }else{
  control,err:=codec.StrictDecodePayload(controlRaw);if err!=nil{return zero,err}
  ctl,ok:=control.([]any);if !ok||len(ctl)!=7{return zero,errors.New("cycle control state malformed")}
  if rev==uint64(c.Revision)&&!sameCanonical(all[13],ctl){return zero,errors.New("tip snapshot control state diverged")}
 }
 sealedControl,ok:=all[13].([]any);if !ok||len(sealedControl)!=7{return zero,errors.New("historical control schema malformed")}
 controlStatus,ok=sealedControl[0].(uint64);if !ok||controlStatus>3{return zero,errors.New("historical control status invalid")}
 refs,ok:=all[14].([]any);if !ok{return zero,errors.New("snapshot checkpoint roster invalid")}
 count,ok:=cursors[3].([]any);if !ok||len(count)!=3{return zero,errors.New("evidence cursor malformed")}
 evidenceCursor,ok:=count[1].(uint64);if !ok{return zero,errors.New("evidence cursor invalid")}
 if err:=verifySnapshotCheckpointRefs(ctx,tx,run,refs,hist.Instant,rev,evidenceCursor,&id);err!=nil{return zero,err}
 pfields,ok:=currentPending[2].([]byte);if !ok{return zero,errors.New("pending state envelope missing")}
 penv,err:=codec.StrictDecodeEnvelope(pfields);if err!=nil{return zero,err}
 memberships,ok:=penv.Value.([]any);if !ok||len(memberships)!=10{return zero,errors.New("invalid pending state typed projection")}
 outstanding,ok:=memberships[8].([]any);if !ok{return zero,errors.New("invalid outstanding task list")}
 tasks:=len(outstanding)
 if err:=tx.Commit();err!=nil{return zero,err}
 return ResumeCheckpointV1{
  Snapshot:SnapshotV1{ID:id,Digest:hash,Canonical:bytes.Clone(canonical),Revision:rev},
  SnapshotClock:clock.State{Instant:domain.SimulationInstant(hist.Instant),Revision:domain.WorldRevision(hist.Revision),
    NextLogicalSequence:domain.LogicalSequence(hist.NextSequence),LastOrdinal:hist.LastOrdinal},
  Clock:c,TailCommits:uint64(c.Revision)-rev,ControlStatus:controlStatus,PendingTasks:tasks,
 },nil
}

type snapshotGenesis struct{ID,Digest [32]byte;Seed [32]byte;Policies any;InitialInstant int64}
func genesisValidateForSnapshot(raw []byte,b *policy.Bundle)(snapshotGenesis,error){
 var out snapshotGenesis
 env,err:=codec.StrictDecodeEnvelope(raw);if err!=nil{return out,err}
 p,ok:=env.Value.([]any);if !ok||len(p)!=18{return out,errors.New("snapshot genesis missing")}
 // The full genesis authority is independently replay-validated before use.
 id,_,err:=identity.New(b).Sum("cote.csf.id.genesis","cote.csf.schema.genesis-id-preimage",1,[]any{p[0]});if err!=nil{return out,err}
 dig,_,err:=identity.New(b).Sum("cote.csf.digest.genesis-manifest","cote.csf.schema.genesis-manifest",1,p);if err!=nil{return out,err}
 out.ID=[32]byte(id);out.Digest=[32]byte(dig)
 seed,ok:=p[1].([]byte);if !ok||len(seed)!=32{return out,errors.New("genesis seed malformed")}
 copy(out.Seed[:],seed);out.Policies=p[14]
 initial,ok:=asSigned64(p[3]);if !ok{return out,errors.New("genesis initial instant invalid")}
 out.InitialInstant=initial
 return out,nil
}

// The roster is bound to evidence holders, not to a caller-selected list.
// Inline checkpoint bytes are hashed, never interpreted.
func verifySnapshotCheckpointRefs(ctx context.Context,tx *sql.Tx,run [32]byte,refs []any,instant int64,rev,cursor uint64)error{
 holders,err:=tx.QueryContext(ctx,"SELECT DISTINCT holder_namespace,holder_id FROM evidence_routes WHERE run_id=? ORDER BY holder_namespace,holder_id",run[:])
 if err!=nil{return err}
 expected:=map[string]bool{}
 for holders.Next(){
  var ns,id string
  if err:=holders.Scan(&ns,&id);err!=nil{holders.Close();return err}
  expected[ns+"\x00"+id]=true
 }
 if err:=holders.Err();err!=nil{holders.Close();return err}
 if err:=holders.Close();err!=nil{return err}
 got:=map[string]bool{}
 prior:=""
 for _,v:=range refs{
  f,ok:=v.([]any);if !ok||len(f)!=8{return errors.New("checkpoint reference malformed")}
  actor,ok:=f[0].([]any);if !ok||len(actor)!=2{return errors.New("checkpoint actor malformed")}
  ns,ok1:=actor[0].(string);id,ok2:=actor[1].(string)
  if !ok1||!ok2{return errors.New("checkpoint actor identifier malformed")}
  key:=ns+"\x00"+id
  if prior!=""&&key<=prior{return errors.New("checkpoint roster unordered or duplicated")}
  prior=key;got[key]=true
  content,ok:=f[3].([]any);if !ok||len(content)!=2||content[0]!=uint64(1){return errors.New("external checkpoint cannot be verified without an owner resolver")}
  inline,ok:=content[1].([]byte);if !ok{return errors.New("checkpoint content malformed")}
  h:=identity.OpaqueContentHash(inline)
  if !equalBytes(f[4],h[:]){return errors.New("checkpoint bytes missing or corrupted")}
  covers,ok:=f[7].([]any);if !ok||len(covers)!=3{return errors.New("checkpoint coverage malformed")}
  at,ok:=asSigned64(covers[0]);if !ok||at<instant{return errors.New("checkpoint instant stale")}
  revision,ok:=covers[1].(uint64);if !ok||revision<rev{return errors.New("checkpoint revision stale")}
  evidence,ok:=covers[2].(uint64);if !ok||evidence<cursor{return errors.New("checkpoint evidence cursor stale")}
  ver,ok:=f[2].(uint64);if !ok||ver==0||ver>uint64(^uint32(0)){return errors.New("checkpoint schema version invalid")}
  producerVer,ok:=f[6].(uint64);if !ok||producerVer==0||producerVer>uint64(^uint32(0)){return errors.New("checkpoint producer version invalid")}
  storeID,storeOK:=f[1].(string);producer,producerOK:=f[5].(string)
  if !storeOK||!producerOK{return errors.New("checkpoint store or producer absent")}
  for _,name:=range []string{ns,id,storeID,producer}{if domain.ValidateMachineID(name)!=nil{return errors.New("invalid checkpoint machine ID")}}
 }
 if len(expected)!=len(got){return errors.New("missing or extra epistemic checkpoint holder")}
 for k:=range expected{if !got[k]{return errors.New("missing checkpoint for evidence holder")}}
 return nil
}

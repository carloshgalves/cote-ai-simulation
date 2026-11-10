package commit

import (
 "context"
 "errors"
 "fmt"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/admission"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/genesis"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/inputledger"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
)

// SettleFencedThenForkV2 is the versioned boundary for a V1 run that has a
// durable WORK/retry fence at the requested checkpoint. It does NOT migrate
// an in-flight fence as if it were a fresh V2 authority. Instead it finishes
// the exact previously admitted parent terminal with ResumeRegisteredWorkV1,
// drains the resulting perception obligations, seals a NEW idle V1 checkpoint,
// and only then performs the authenticated V1->V2 fork.
//
// IMPORTANT: the source terminal commit is durable before the child SQLite
// fork begins. The two databases do not share a transaction. If the V2 fork
// rejects unsupported remaining work, the V1 source has still advanced.
// This is safe and retryable from its new idle checkpoint, but not an
// all-or-nothing transfer across two independent databases.
func (c *Coordinator) SettleFencedThenForkV2(ctx context.Context,
 ledger *inputledger.Ledger,sourceRun,sourceSnapshot,childRun [32]byte,
 destination *sqlite.Store,childPolicy *policy.Bundle,
 buildCheckpoints func(context.Context,*sqlite.Store)([]sqlite.SnapshotCheckpoint,error))(*genesis.Genesis,error){
 if c==nil||ledger==nil||destination==nil||c.bundle==nil||buildCheckpoints==nil{
  return nil,errors.New("versioned fence handoff requires registered parent and V2 destination")
 }
 // Reject an invalid target before making any irreversible V1 parent commit.
 if childPolicy==nil||childRun==([32]byte{})||childRun==sourceRun{
  return nil,errors.New("fence handoff requires a distinct nonzero V2 child run")
 }
 id,version,_,_,policyErr:=childPolicy.CodecIdentity()
 if policyErr!=nil||id!=policy.CodecPolicyV2ID||version!=2{
  return nil,errors.New("fence handoff requires an independently installed normative V2 policy")
 }
 origin,ok:=c.store.(*sqlite.Store)
 if !ok||origin==destination{
  return nil,errors.New("V1 fence handoff requires distinct SQLite authorities")
 }
 if id,version,_,_,err:=c.bundle.CodecIdentity();err!=nil||
  id!=policy.CodecPolicyID||version!=1{
  return nil,errors.New("fenced terminal source is not an installed V1 policy")
 }
 proof,err:=origin.VerifySnapshotV1(ctx,sourceRun,sourceSnapshot,c.bundle)
 if err!=nil{return nil,err}
 // A retry is always anchored in the FROZEN WORK/retry cut, never in
 // the live control status: the parent may have admitted other WORK later.
 if proof.SnapshotControlStatus!=1&&proof.SnapshotControlStatus!=2{
  return nil,errors.New("fence handoff requires a frozen WORK or retry checkpoint")
 }
 // Cognitive actor history must never be dropped when taking the follow-up
 // checkpoint. The caller supplies signed snapshots for these owners.
 parsed,err:=codec.StrictDecodePayload(proof.Snapshot.Canonical)
 if err!=nil{return nil,err}
 fields,ok:=parsed.([]any)
 if !ok||len(fields)!=19{return nil,errors.New("fence checkpoint schema invalid")}
 owners,ok:=fields[14].([]any)
 if !ok{return nil,errors.New("fence checkpoint cognitive roster malformed")}
 expectedActors:=map[string]bool{}
 for _,raw:=range owners{
  item,ok:=raw.([]any)
  if !ok||len(item)!=8{return nil,errors.New("fence cognitive owner tuple malformed")}
  actor,ok:=item[0].([]any)
  if !ok||len(actor)!=2{return nil,errors.New("fence cognitive actor malformed")}
  ns,nsOK:=actor[0].(string);name,nameOK:=actor[1].(string)
  if !nsOK||!nameOK{return nil,errors.New("fence cognitive actor ID malformed")}
  key:=ns+"\\x00"+name
  if expectedActors[key]{return nil,errors.New("fence contains duplicate cognitive owner")}
  expectedActors[key]=true
 }
 // The frozen cycle ID is immutable. Never dispatch a newer, unrelated
 // live WORK after a crash or after another actor advances the parent.
 frozenControl,ok:=fields[13].([]any)
 if !ok||len(frozenControl)!=7{return nil,errors.New("frozen fence control malformed")}
 frozenCycleRef,ok:=frozenControl[1].([]any)
 if !ok||len(frozenCycleRef)!=2||frozenCycleRef[0]!=uint64(1){
  return nil,errors.New("frozen fence cycle reference absent")
 }
 frozenCycleBytes,ok:=frozenCycleRef[1].([]byte)
 if !ok||len(frozenCycleBytes)!=32{return nil,errors.New("frozen fence cycle ID malformed")}
 var frozenCycle [32]byte
 copy(frozenCycle[:],frozenCycleBytes)
 frozenRevision:=uint64(proof.SnapshotClock.Revision)
 if frozenRevision==^uint64(0){return nil,errors.New("frozen world revision overflow")}
 expectedRevision:=frozenRevision+1
 expectedID,_,err:=identity.New(c.bundle).Sum(
  "cote.csf.id.snapshot","cote.csf.schema.snapshot-id-preimage",1,
  []any{sourceRun[:],uint64(1),expectedRevision})
 if err!=nil{return nil,err}
 idleID:=[32]byte(expectedID)
 // An already committed V2 child is a READ-ONLY retry. Probe it before
 // touching a provider or dispatching any live parent cycle, regardless
 // of the current parent revision or control status.
 child,err:=destination.RecoverPublishedForkV1ToV2(
  ctx,childRun,sourceRun,idleID,childPolicy)
 if err!=nil{return nil,fmt.Errorf("V2 committed child recovery refused: %w",err)}
 if child!=nil {
  if err:=origin.VerifyFrozenFenceCommittedOnce(ctx,sourceRun,sourceSnapshot,c.bundle);err!=nil{
   return nil,fmt.Errorf("published child's frozen terminal not authenticated: %w",err)
  }
  idle,err:=origin.VerifySnapshotV1(ctx,sourceRun,idleID,c.bundle)
  if err!=nil{return nil,fmt.Errorf("V1 idle source checkpoint missing: %w",err)}
  // Only the IMMUTABLE IDLE cut at R+1 is pertinent. Its live tip may
  // legitimately be R+2 or an independent in-flight WORK.
  if idle.SnapshotControlStatus!=0||
   uint64(idle.SnapshotClock.Revision)!=expectedRevision {
   return nil,errors.New("committed child does not match the authenticated V1 idle source cut")
  }
  return child,nil
 }
 // With no published child, only the original frozen cycle (at R) or
 // its immediate settled IDLE successor (R+1) can make progress.
 // An unrelated live WORK at R+1 must never be resumed by this retry.
 switch {
 case uint64(proof.Clock.Revision)==frozenRevision &&
  (proof.ControlStatus==1||proof.ControlStatus==2):
  recovered,err:=c.ResumeRunV1(ctx,ledger,sourceRun,sourceSnapshot,
   func(ctx context.Context,l *inputledger.Ledger,cy admission.Cycle,f admission.Fence)(ports.CanonicalRecord,error){
    if [32]byte(cy.ID)!=frozenCycle||uint64(cy.BaseRevision)!=frozenRevision{
     return ports.CanonicalRecord{},errors.New("handoff refuses unrelated parent WORK after frozen cut")
    }
    return c.DispatchFencedWorkV1(ctx,l,cy,f)
   })
  if err!=nil{return nil,fmt.Errorf("V1 frozen terminal recovery: %w",err)}
  if !recovered.Continued{return nil,errors.New("frozen fence did not publish one verified V1 terminal")}
 case uint64(proof.Clock.Revision)==expectedRevision && proof.ControlStatus==0:
  if err:=origin.VerifyFrozenFenceCommittedOnce(ctx,sourceRun,sourceSnapshot,c.bundle);err!=nil{
   return nil,fmt.Errorf("frozen fence handoff retry not authenticated: %w",err)
  }
 default:
  return nil,errors.New("handoff source has unrelated WORK or revision without a published child")
 }
 // The terminal may create new eligible actors, so these checkpoint
 // bytes must be supplied and validated AFTER V1 terminal publication.
 acquire:=func()([]sqlite.SnapshotCheckpoint,error){
  items,err:=buildCheckpoints(ctx,origin)
  if err!=nil{return nil,err}
  supplied:=map[string]bool{}
  for _,cp:=range items{
   key:=cp.ActorNamespace+"\\x00"+cp.ActorID
   if supplied[key]{return nil,errors.New("handoff duplicated a cognitive actor")}
   supplied[key]=true
  }
  for old:=range expectedActors{
   if !supplied[old]{return nil,errors.New("handoff dropped pre-fence cognitive actor")}
  }
  return items,nil
 }
 // A snapshot ID is unique per (run, world revision). Perception
 // completion can append a ledger tail WITHOUT advancing that revision.
 // Therefore do not publish an intermediate snapshot and collide with the
 // eventual final checkpoint at the same revision. Recovery can inspect
 // the original frozen checkpoint plus its authenticated committed tail.
 // Drain only previously committed perception work. Calling the generic
 // resume/dispatcher here can execute an unrelated WORK cycle admitted by
 // another actor after the first source terminal. A handoff must never
 // perform that second, unrequested world mutation.
 drained,err:=origin.DrainPendingPerceptionV1(ctx,sourceRun,sourceSnapshot,c.bundle)
 if err!=nil{return nil,fmt.Errorf("V1 terminal outbox drain: %w",err)}
 if drained.ControlStatus!=0 || drained.Action.Kind!=sqlite.ResumeIdle {
  return nil,errors.New("new V1 WORK appeared during handoff; refuse to dispatch it")
 }
 // A second unrelated terminal can land while the parent is between
 // transactions. Do not silently choose a later world cut for the V2 child.
 // Only the one authenticated frozen CycleCommit may advance source revision.
 if drained.Clock.Revision!=proof.SnapshotClock.Revision+1 {
  return nil,errors.New("source revision changed after frozen terminal; cannot fork an unrequested later cut")
 }
 finalCheckpoints,err:=acquire()
 if err!=nil{return nil,err}
 idle,err:=origin.CreateSnapshotV1AtRevision(ctx,sourceRun,c.bundle,finalCheckpoints,
  uint64(proof.SnapshotClock.Revision)+1)
 if err!=nil{return nil,err}
 verified,err:=origin.VerifySnapshotV1(ctx,sourceRun,idle.ID,c.bundle)
 if err!=nil{return nil,err}
 if verified.ControlStatus!=0{
  return nil,errors.New("recovered V1 fence did not produce an IDLE checkpoint")
 }
 // A process may crash after the V2 bootstrap transaction commits but
 // before the caller receives its Genesis. Authenticate an existing child
 // against the EXACT parent snapshot, policy, lineage and replay; never
 // treat a merely nonempty destination as an idempotent success.
 existing,err:=destination.RecoverPublishedForkV1ToV2(ctx,childRun,sourceRun,idle.ID,childPolicy)
 if err!=nil{return nil,fmt.Errorf("existing V2 child cannot be authenticated: %w",err)}
 if existing!=nil{return existing,nil}
 created,err:=origin.ForkIdleSnapshotV1ToV2(ctx,sourceRun,idle.ID,childRun,
  destination,c.bundle,childPolicy)
 if err==nil{return created,nil}
 // Another caller may have completed the same atomic bootstrap between our
 // read-only probe and INSERT. Verify it rather than publishing twice.
 verifiedChild,recoverErr:=destination.RecoverPublishedForkV1ToV2(
  ctx,childRun,sourceRun,idle.ID,childPolicy)
 if recoverErr==nil&&verifiedChild!=nil{return verifiedChild,nil}
 if recoverErr!=nil{return nil,fmt.Errorf("V2 fork failed: %w; destination rejected: %v",err,recoverErr)}
 return nil,err
}

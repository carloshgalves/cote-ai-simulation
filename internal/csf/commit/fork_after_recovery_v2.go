package commit

import (
 "context"
 "errors"
 "fmt"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/genesis"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/inputledger"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
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
 checkpoints []sqlite.SnapshotCheckpoint)(*genesis.Genesis,error){
 if c==nil||ledger==nil||destination==nil||c.bundle==nil{
  return nil,errors.New("versioned fence handoff requires registered parent and V2 destination")
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
 if proof.ControlStatus!=1&&proof.ControlStatus!=2{
  return nil,errors.New("fence handoff requires a frozen WORK or retry checkpoint")
 }
 // Cognitive actor history must never be dropped when taking the follow-up
 // checkpoint. The caller supplies signed snapshots for these owners.
 parsed,err:=codec.StrictDecodePayload(proof.Snapshot.Canonical)
 if err!=nil{return nil,err}
 fields,ok:=parsed.([]any)
 if !ok||len(fields)!=19{return nil,errors.New("fence checkpoint schema invalid")}
 owners,ok:=fields[14].([]any)
 if !ok||len(owners)!=len(checkpoints){
  return nil,errors.New("fence handoff omits cognitive owner snapshots")
 }
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
 for _,cp:=range checkpoints{
  key:=cp.ActorNamespace+"\\x00"+cp.ActorID
  if !expectedActors[key]{
   return nil,errors.New("fence handoff would replace or duplicate a cognitive owner")
  }
  delete(expectedActors,key)
 }
 if len(expectedActors)!=0{return nil,errors.New("fence handoff lost cognitive actor")}
 recovered,err:=c.ResumeRegisteredWorkV1(ctx,ledger,sourceRun,sourceSnapshot)
 if err!=nil{return nil,fmt.Errorf("V1 fenced terminal recovery: %w",err)}
 if !recovered.Continued {
  return nil,errors.New("frozen fence did not publish one verified V1 terminal")
 }
 // This intermediate checkpoint is only a recovery handle, never the
 // authoritative fork cut; pending causal perception may still exist.
 intermediate,err:=origin.CreateSnapshotV1(ctx,sourceRun,c.bundle,checkpoints)
 if err!=nil{return nil,err}
 drained,err:=c.ResumeRegisteredWorkV1(ctx,ledger,sourceRun,intermediate.ID)
 if err!=nil{return nil,fmt.Errorf("V1 terminal outbox drain: %w",err)}
 if drained.Continued {
  return nil,errors.New("unexpected additional V1 WORK terminal during fence outbox drain")
 }
 idle,err:=origin.CreateSnapshotV1(ctx,sourceRun,c.bundle,checkpoints)
 if err!=nil{return nil,err}
 verified,err:=origin.VerifySnapshotV1(ctx,sourceRun,idle.ID,c.bundle)
 if err!=nil{return nil,err}
 if verified.ControlStatus!=0{
  return nil,errors.New("recovered V1 fence did not produce an IDLE checkpoint")
 }
 return origin.ForkIdleSnapshotV1ToV2(ctx,sourceRun,idle.ID,childRun,
  destination,c.bundle,childPolicy)
}

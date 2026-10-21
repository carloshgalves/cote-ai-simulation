package decision

import (
 "bytes"
 "context"
 "errors"
 "fmt"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/admission"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/inputledger"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

// FenceReader is the read-only authority available to pre-evaluation
// assembly. It is deliberately NOT passed to action handlers/validators.
type FenceReader interface{
 GetFence(context.Context,[32]byte)([]byte,error)
}

// FromPersistedFence authenticates the exact admitted source units against
// the canonical ledger, the rederived fence and the already persisted fence.
// It admits only slot response units in this first resource experiment;
// other producers need their own CSFRV1-4 handler extension.
func FromPersistedFence(
 ctx context.Context,bundle *policy.Bundle,ledger *inputledger.Ledger,
 cycle admission.Cycle,policies admission.Policies,
 fence admission.Fence,reader FenceReader,snapshot Snapshot,
 schemas []ActionSchema,seed domain.Digest,history []HistoryBinding,
)(Input,error){
 return fromPersistedFence(ctx,bundle,ledger,cycle,policies,fence,reader,snapshot,schemas,seed,history,false)
}

// FromPersistedFenceWithSystemHandlers projects only authenticated slot units
// to the *resource* decision kernel while leaving schedule/trigger sources
// untouched for a separate registered lifecycle handler inside the SAME
// CycleCommit. Never use this projection without terminal SQL co-settlement.
func FromPersistedFenceWithSystemHandlers(
 ctx context.Context,bundle *policy.Bundle,ledger *inputledger.Ledger,
 cycle admission.Cycle,policies admission.Policies,
 fence admission.Fence,reader FenceReader,snapshot Snapshot,
 schemas []ActionSchema,seed domain.Digest,history []HistoryBinding,
)(Input,error){
 return fromPersistedFence(ctx,bundle,ledger,cycle,policies,fence,reader,snapshot,schemas,seed,history,true)
}
func fromPersistedFence(
 ctx context.Context,bundle *policy.Bundle,ledger *inputledger.Ledger,
 cycle admission.Cycle,policies admission.Policies,
 fence admission.Fence,reader FenceReader,snapshot Snapshot,
 schemas []ActionSchema,seed domain.Digest,history []HistoryBinding,
 allowSystem bool,
)(Input,error){
 var result Input
 if ledger==nil||reader==nil||bundle==nil{return result,ErrProvenance}
 persisted,err:=reader.GetFence(ctx,[32]byte(fence.ID))
 if err!=nil{return result,fmt.Errorf("%w: admission fence not durably published: %v",ErrProvenance,err)}
 if !bytes.Equal(persisted,fence.Canonical){return result,fmt.Errorf("%w: persisted fence bytes differ",ErrProvenance)}
 if err:=admission.VerifyFence(bundle,ledger,cycle,policies,fence);err!=nil{
  return result,fmt.Errorf("%w: admission fence differs from source ledger: %v",ErrProvenance,err)
 }
 if snapshot.BaseRevision!=cycle.BaseRevision{return result,fmt.Errorf("%w: snapshot not at fenced base revision",ErrProvenance)}
 _,_,_,rounds,slots,runID:=ledger.Snapshot()
 responses:=map[domain.ID]Unit{}
 for _,round:=range rounds{
  for _,r:=range slots[round.ID]{
   if _,exists:=responses[r.ID];exists{return result,fmt.Errorf("%w: duplicate source response identity",ErrProvenance)}
   responses[r.ID]=Unit{Response:r,ProducerScope:round.SourceID}
  }
 }
 if len(fence.Fields)<12{return result,ErrProvenance}
 rawUnits,ok:=fence.Fields[11].([]any);if !ok{return result,ErrProvenance}
 result=Input{RunID:runID,CycleID:cycle.ID,Attempt:cycle.Attempt,
  Snapshot:snapshot,Schemas:schemas,Seed:seed,History:history,Bundle:bundle}
 for _,raw:=range rawUnits{
  unit,ok:=raw.([]any)
  if !ok||len(unit)!=5{return Input{},ErrProvenance}
  kind,ok:=unit[1].(uint64)
  if !ok{return Input{},ErrProvenance}
  if kind!=0{
   if allowSystem&&(kind==2||kind==3){continue}
   return Input{},fmt.Errorf("%w: non-slot admitted unit needs a registered source handler",ErrIndeterminate)
  }
  source,ok:=unit[2].(string);if !ok{return Input{},ErrProvenance}
  idBytes,ok:=unit[3].([]byte);if !ok||len(idBytes)!=32{return Input{},ErrProvenance}
  digBytes,ok:=unit[4].([]byte);if !ok||len(digBytes)!=32{return Input{},ErrProvenance}
  var id domain.ID;copy(id[:],idBytes)
  stored,exists:=responses[id]
  if !exists||source!=stored.ProducerScope||!bytes.Equal(digBytes,stored.Response.Digest[:]){
   return Input{},fmt.Errorf("%w: fenced unit does not resolve to immutable response",ErrProvenance)
  }
  result.Admitted=append(result.Admitted,id)
  result.Units=append(result.Units,stored)
 }
 if len(result.Admitted)==0{return Input{},errors.New("resource decision requires at least one fenced slot response")}
 return result,nil
}

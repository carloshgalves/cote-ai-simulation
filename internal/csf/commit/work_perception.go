package commit

import (
 "context"
 "errors"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/perception"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
)

// commitPreparedWork seals a PerceptionTask for each Event selected by
// the Genesis-pinned potential-perceptibility predicate. The task contains
// no observation nor recipient list. A full Event remains authoritative
// only in EventStore; only a PENDING task reference enters causal_outbox.
func (c *Coordinator) commitPreparedWork(ctx context.Context,b *ports.WorkBatch) error{
 if c==nil||c.bundle==nil||c.store==nil||b==nil{
  return errors.New("cannot classify events without authenticated terminal coordinator")
 }
 refs,err:=c.bundle.GenesisPolicyRefs();if err!=nil{return err}
 roles,err:=policiesByRole(c.bundle);if err!=nil{return err}
 raw,err:=codec.StrictDecodePayload(b.Journal.Canonical)
 if err!=nil{return err}
 fields,ok:=raw.([]any)
 if !ok||len(fields)!=24{return errors.New("WORK journal missing task publication fields")}
 prior,ok:=fields[6].(uint64)
 if !ok{return errors.New("WORK task base revision missing")}
 after,ok:=fields[8].(uint64)
 if !ok{return errors.New("WORK task result revision missing")}
 genesisPins:=make([][]any,len(refs))
 for i,ref:=range refs{genesisPins[i]=ref}
 tasks,err:=perception.PlanPinned(genesisPins,domain.ID(b.RunID),domain.ID(b.CycleID),
  domain.WorldRevision(prior),domain.WorldRevision(after),b.Events,
  perception.Pins{Classification:roles["perception"],Resolver:roles["validators"],Identity:roles["perception-identity"]})
 if err!=nil{return err}
 d:=identity.New(c.bundle)
 ids:=make([]any,len(tasks))
 for i,item:=range tasks{ids[i]=item.Identity[:]}
 taskHash,_,err:=d.Sum("cote.csf.digest.perception-task-list",
  "cote.csf.schema.perception-task-id-list",1,ids)
 if err!=nil{return err}
 fields[18]=ids;fields[19]=taskHash[:]
 rawEvents:=make([]any,len(b.Events))
 for i,item:=range b.Events{rawEvents[i]=item.Canonical}
 batchHash,_,err:=d.Sum("cote.csf.digest.batch","cote.csf.schema.batch-preimage",1,
  []any{fields[:23],rawEvents})
 if err!=nil{return err}
 fields[23]=batchHash[:]
 canonical,err:=codec.CanonicalPayload(fields)
 if err!=nil{return err}
 b.Journal.Canonical=canonical
 b.PerceptionTasks=tasks
 if c.tamperWorkBatch!=nil{c.tamperWorkBatch(b)}
 return c.store.CommitWork(ctx,*b)
}

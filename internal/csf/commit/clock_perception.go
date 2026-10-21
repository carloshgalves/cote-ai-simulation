package commit

import (
 "errors"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/perception"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
)

// prepareClockPerception uses the exact same registered task classification
// as WORK. CLOCK_ADVANCE events with local ordinal zero are not exempt from
// publishing a PENDING task simply because the resulting observation may be
// restricted by downstream knowledge/access policy.
func (c *Coordinator) prepareClockPerception(b *ports.ClockAdvanceBatch)error{
 if c==nil||c.bundle==nil||b==nil{return errors.New("missing clock perception authority")}
 refs,err:=c.bundle.GenesisPolicyRefs();if err!=nil{return err}
 roles,err:=policiesByRole(c.bundle);if err!=nil{return err}
 tasks,err:=perception.PlanPinned(refs,domain.ID(b.RunID),domain.ID(b.CycleID),
  domain.WorldRevision(b.ExpectedRevision),domain.WorldRevision(b.ResultRevision),b.Events,
  perception.Pins{Classification:roles["perception"],Resolver:roles["validators"],Identity:roles["perception-identity"]})
 if err!=nil{return err}
 decoded,err:=codec.StrictDecodePayload(b.Journal.Canonical);if err!=nil{return err}
 fields,ok:=decoded.([]any)
 if !ok||len(fields)!=24{return errors.New("CLOCK_ADVANCE task journal shape invalid")}
 ids:=make([]any,len(tasks))
 for i,task:=range tasks{ids[i]=task.Identity[:]}
 d:=identity.New(c.bundle)
 hash,_,err:=d.Sum("cote.csf.digest.perception-task-list","cote.csf.schema.perception-task-id-list",1,ids)
 if err!=nil{return err}
 fields[18],fields[19]=ids,hash[:]
 events:=make([]any,len(b.Events))
 for i,event:=range b.Events{events[i]=event.Canonical}
 batchHash,_,err:=d.Sum("cote.csf.digest.batch","cote.csf.schema.batch-preimage",1,
  []any{fields[:23],events});if err!=nil{return err}
 fields[23]=batchHash[:]
 raw,err:=codec.CanonicalPayload(fields);if err!=nil{return err}
 b.Journal.Canonical=raw
 b.PerceptionTasks=tasks
 return nil
}

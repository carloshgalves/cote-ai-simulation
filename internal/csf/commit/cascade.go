package commit

import (
 "context"
 "errors"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/admission"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/clock"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
)

var ErrCascadeLimit=errors.New("versioned same-instant WORK cascade limit exceeded; authoritative CycleAbortRecord published")

// abortIfCascadeExceeded is called after publishing a trusted fence and
// before deriving any terminal decision or event. The store independently
// repeats this guard under CommitWork's atomic CAS, closing interleaving races.
func (c *Coordinator) abortIfCascadeExceeded(ctx context.Context,run domain.ID,
 cycle admission.Cycle,fence admission.Fence)(ports.CanonicalRecord,bool,error){
 var empty ports.CanonicalRecord
 reader,ok:=c.store.(interface{
  CountSameInstantWork(context.Context,[32]byte,int64)(uint64,error)
 })
 if !ok{return empty,false,errors.New("runtime missing authoritative cascade count")}
 count,err:=reader.CountSameInstantWork(ctx,[32]byte(run),int64(cycle.Instant))
 if err!=nil{return empty,false,err}
 if !clock.CascadeExceeded(count){return empty,false,nil}
 abort,err:=c.Abort(ctx,run,cycle.ID,fence,6,nil)
 if err!=nil{return empty,true,err}
 return abort,true,ErrCascadeLimit
}

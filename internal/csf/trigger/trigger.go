// Package trigger is the pure, versioned trigger runtime. The only publishing
// authority is the CycleCommit transaction owned by the CSF coordinator.
package trigger

import (
 "bytes"
 "errors"
 "sort"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
)

type Policy uint8
const (
 RisingEdge Policy=iota
 FallingEdge
 OnceWhenTrue
 RepeatWhileTrue
)

type Definition struct {
 ID domain.ID
 // Pure clock-only predicate for CSFRV1-3. Future domain predicates require
 // a version-pinned registry; arbitrary callbacks are never accepted.
 PredicateThreshold domain.SimulationInstant
 Version uint32
 Policy Policy
 FireOnInitialTrue bool
 RepeatEvery int64
 Dependencies []string
}

type Runtime struct {
 DefinitionID domain.ID
 Version uint32
 HasLast bool
 Last bool
 Armed bool
 Exhausted bool
 ActivationCount uint64
 HasNextRepeat bool
 NextRepeat domain.SimulationInstant
 EvaluatedThrough domain.WorldRevision
}

type Event struct {
 ID domain.ID
 // EventOrderKey is a canonical byte vector derived from EventOrderKey.
 OrderKey []byte
 Footprint []string
}

type Result struct {
 Runtime Runtime
 Fired bool
 CausingEventIDs []domain.ID
}

// PredicateValue is wholly derived from the persisted logical instant.
// No caller-provided predicate function, host clock, or knowledge API.
func PredicateValue(d Definition, instant domain.SimulationInstant) bool {
 return instant >= d.PredicateThreshold
}

func Validate(d Definition) error {
 if d.Version==0||d.Policy>RepeatWhileTrue{return errors.New("invalid trigger identity or policy")}
 if d.Policy==RepeatWhileTrue {
  if d.RepeatEvery<=0{return errors.New("repeat trigger requires positive cadence")}
 }else if d.RepeatEvery!=0{return errors.New("repeat cadence forbidden for nonrepeat trigger")}
 for i,x:=range d.Dependencies {if x==""||(i>0&&d.Dependencies[i-1]>=x){return errors.New("dependencies must be unique and sorted")}}
 return nil
}

// Causes collects *every* matching pre-trigger event, in EventOrderKey order,
// independent of the order in which footprint workers returned.
func Causes(dependencies []string, events []Event) ([]domain.ID,error) {
 result:=append([]Event(nil),events...)
 sort.Slice(result,func(i,j int)bool{return bytes.Compare(result[i].OrderKey,result[j].OrderKey)<0})
 dep:=map[string]bool{}
 for _,s:=range dependencies{dep[s]=true}
 ids:=make([]domain.ID,0)
 for i,e:=range result{
  if i>0 && bytes.Equal(e.OrderKey,result[i-1].OrderKey){return nil,errors.New("duplicate event order key")}
  for _,f:=range e.Footprint{if dep[f]{ids=append(ids,e.ID);break}}
 }
 return ids,nil
}

// Evaluate is a pure state transition over a predicate result already computed
// from a pinned revision and its declared dependencies. No clock/LLM/I/O reads.
func Evaluate(def Definition, old Runtime, now domain.SimulationInstant, revision domain.WorldRevision, predicate bool, events []Event) (Result,error) {
 if err:=Validate(def);err!=nil{return Result{},err}
 if old.DefinitionID!=def.ID || old.Version!=def.Version {return Result{},errors.New("trigger version mismatch")}
 if revision<=old.EvaluatedThrough && old.HasLast {return Result{},errors.New("trigger revision already evaluated")}
 if old.Exhausted && def.Policy!=OnceWhenTrue{return Result{},errors.New("unexpected exhausted state")}
 next:=old
 next.EvaluatedThrough=revision
 fired:=false
 switch def.Policy{
 case RisingEdge:
  fired=predicate && ((!old.HasLast && def.FireOnInitialTrue)||(old.HasLast && !old.Last))
  next.Armed=!predicate
 case FallingEdge:
  fired=old.HasLast && old.Last && !predicate
  next.Armed=predicate
 case OnceWhenTrue:
  fired=predicate&&!old.Exhausted
  if fired{next.Exhausted=true}
  next.Armed=!next.Exhausted
 case RepeatWhileTrue:
  if !predicate {next.HasNextRepeat=false;next.Armed=true
  } else if !old.HasLast || !old.Last || !old.HasNextRepeat {
   fired=true
  } else if now>=old.NextRepeat {
   fired=true
  }
  if fired {
   if int64(now)>int64(^uint64(0)>>1)-def.RepeatEvery{return Result{},errors.New("repeat cadence overflows instant")}
   next.NextRepeat=now+domain.SimulationInstant(def.RepeatEvery)
   next.HasNextRepeat=true
  }
 }
 next.HasLast=true
 next.Last=predicate
 if fired {
  if old.ActivationCount==^uint64(0){return Result{},errors.New("activation count exhausted")}
  next.ActivationCount++
 }
 ids,err:=Causes(def.Dependencies,events)
 if err!=nil{return Result{},err}
 return Result{Runtime:next,Fired:fired,CausingEventIDs:ids},nil
}

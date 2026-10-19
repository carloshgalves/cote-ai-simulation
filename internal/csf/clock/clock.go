// Package clock owns the causal scheduling coordinate and explicit clock advance.
// All decisions use SimulationInstant, never wall clock or worker arrival order.
package clock

import (
    "errors"
    "math"

    "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
)

type State struct {
    Instant domain.SimulationInstant
    Revision domain.WorldRevision
    NextLogicalSequence domain.LogicalSequence
    // -1 means that no cycle was committed at Instant.
    LastOrdinal int64
}

type Kind uint8
const (
    Work Kind = iota
    ClockAdvance
)

type Plan struct {
    Kind Kind
    From domain.SimulationInstant
    To domain.SimulationInstant
    OriginOrdinal domain.CycleOrdinal
    Target domain.EligibilityCoordinate
}

// Pending is an authoritative, not-yet-consumed occurrence, trigger activation,
// round or source input. Callers must fetch a consistent prefix before planning.
type Pending struct {
    ID domain.ID
    Eligibility domain.EligibilityCoordinate
}

var ErrStalePending = errors.New("pending causal work at or before committed clock")
var ErrInvalidClock = errors.New("invalid clock state or cycle coordinate")
var ErrInvalidAdvance = errors.New("invalid clock advance")

func coordinateAfterState(state State) (domain.EligibilityCoordinate, error) {
    if state.LastOrdinal < -1 { return domain.EligibilityCoordinate{},ErrInvalidClock }
    if state.LastOrdinal >= math.MaxInt64 {return domain.EligibilityCoordinate{},ErrInvalidClock}
    return domain.EligibilityCoordinate{NotBeforeInstant:state.Instant,NotBeforeCycleOrdinal:domain.CycleOrdinal(state.LastOrdinal+1)},nil
}

// Next is total: no pending -> idle; same-instant work jumps ordinal holes;
// future work requires a CLOCK_ADVANCE at the next ordinal of the source instant.
func Next(state State, pending []Pending) (Plan,bool,error) {
    next,err:=coordinateAfterState(state)
    if err!=nil{return Plan{},false,err}
    if len(pending)==0{return Plan{},false,nil}
    smallest:=pending[0].Eligibility
    for _,p:=range pending[1:] {
        if p.Eligibility.Compare(smallest)<0{smallest=p.Eligibility}
    }
    if smallest.Compare(next)<0 {return Plan{},false,ErrStalePending}
    if smallest.NotBeforeInstant==state.Instant {
        return Plan{Kind:Work,From:state.Instant,To:state.Instant,OriginOrdinal:smallest.NotBeforeCycleOrdinal,Target:smallest},true,nil
    }
    if smallest.NotBeforeInstant<state.Instant{return Plan{},false,ErrStalePending}
    return Plan{Kind:ClockAdvance,From:state.Instant,To:smallest.NotBeforeInstant,OriginOrdinal:next.NotBeforeCycleOrdinal,Target:smallest},true,nil
}

// Confirm rechecks minimum pending and the origin clock at publication time.
// A Q that arrives ahead of the chosen target rejects the provisional plan.
func Confirm(state State, pending []Pending, proposal Plan) error {
    recomputed,ok,err:=Next(state,pending)
    if err!=nil{return err}
    if !ok||recomputed!=proposal{return ErrInvalidAdvance}
    return nil
}

func SuccessorFloor(plan Plan) (domain.EligibilityCoordinate,error) {
    if plan.Kind==ClockAdvance {
        if plan.To<=plan.From || plan.Target.NotBeforeInstant!=plan.To {
            return domain.EligibilityCoordinate{},ErrInvalidAdvance
        }
        return domain.EligibilityCoordinate{NotBeforeInstant:plan.To},nil
    }
    if plan.Kind!=Work || plan.OriginOrdinal==math.MaxUint64{return domain.EligibilityCoordinate{},ErrInvalidAdvance}
    return domain.EligibilityCoordinate{NotBeforeInstant:plan.From,NotBeforeCycleOrdinal:plan.OriginOrdinal+1},nil
}

// ApplyAdvance is the only time transition in this slice. No work-cycle or
// trigger activation is consumed here; the next WORK cycle is selected later.
func ApplyAdvance(state State, plan Plan) (State,error) {
    if plan.Kind!=ClockAdvance || plan.From!=state.Instant {return State{},ErrInvalidAdvance}
    if plan.To<=plan.From {return State{},ErrInvalidAdvance}
    return State{Instant:plan.To,Revision:state.Revision+1,NextLogicalSequence:state.NextLogicalSequence+2,LastOrdinal:-1},nil
}

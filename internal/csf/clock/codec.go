package clock

import (
    "errors"

    "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
)

// Encode stores the value object in the same canonical CBOR profile as the
// rest of the CSF. It is clock-state, not an alternative CycleCommit record.
func Encode(state State) ([]byte,error) {
    if state.LastOrdinal < -1 {return nil,ErrInvalidClock}
    return codec.CanonicalPayload([]any{int64(state.Instant),uint64(state.Revision),uint64(state.NextLogicalSequence),state.LastOrdinal})
}
func Decode(data []byte) (State,error) {
    var state State
    raw,err:=codec.StrictDecodePayload(data)
    if err!=nil{return state,err}
    fields,ok:=raw.([]any)
    if !ok||len(fields)!=4 {return state,errors.New("invalid clock-state record")}
    instant,ok:=signed(fields[0]);if !ok {return state,ErrInvalidClock}
    revision,ok:=fields[1].(uint64);if !ok{return state,ErrInvalidClock}
    next,ok:=fields[2].(uint64);if !ok{return state,ErrInvalidClock}
    ordinal,ok:=signed(fields[3]);if !ok||ordinal< -1{return state,ErrInvalidClock}
    return State{Instant:domain.SimulationInstant(instant),Revision:domain.WorldRevision(revision),NextLogicalSequence:domain.LogicalSequence(next),LastOrdinal:ordinal},nil
}
func signed(value any)(int64,bool){
    switch n:=value.(type){
    case int64:return n,true
    case uint64:if n<=uint64(^uint64(0)>>1){return int64(n),true}
    }
    return 0,false
}
func EncodePlan(p Plan)([]any,error){
    if p.Kind==Work{return []any{uint64(0)},nil}
    if _,err:=SuccessorFloor(p);err!=nil{return nil,err}
    return []any{uint64(1),int64(p.From),int64(p.To),[]any{int64(p.Target.NotBeforeInstant),uint64(p.Target.NotBeforeCycleOrdinal)}},nil
}

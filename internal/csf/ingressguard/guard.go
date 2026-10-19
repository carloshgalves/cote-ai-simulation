// Package ingressguard validates the causal coordinate of persisted ingress
// against an already published AdmissionFence. Storage adapters use this while
// holding their transaction/lock, never after publishing the input receipt.
package ingressguard

import (
    "bytes"
    "errors"
    "fmt"

    "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
)

type Entry struct {
    RunID [32]byte
    Coordinate domain.EligibilityCoordinate
}

func asRunID(v any) ([32]byte,error) {
    var id [32]byte
    b,ok:=v.([]byte)
    if !ok || len(b)!=32 {return id, errors.New("invalid ingress/fence run id")}
    copy(id[:],b)
    return id,nil
}
func asInstant(v any) (int64,error) {
    switch n:=v.(type) {
    case int64: return n,nil
    case uint64:
        if n<=uint64(^uint64(0)>>1) {return int64(n),nil}
    }
    return 0,errors.New("invalid causal instant")
}
func coord(v any) (domain.EligibilityCoordinate,error) {
    parts,ok:=v.([]any)
    if !ok || len(parts)!=2 {return domain.EligibilityCoordinate{},errors.New("invalid eligibility coordinate")}
    instant,err:=asInstant(parts[0])
    if err!=nil {return domain.EligibilityCoordinate{},err}
    ordinal,ok:=parts[1].(uint64)
    if !ok {return domain.EligibilityCoordinate{},errors.New("invalid causal ordinal")}
    return domain.EligibilityCoordinate{
        NotBeforeInstant:domain.SimulationInstant(instant),
        NotBeforeCycleOrdinal:domain.CycleOrdinal(ordinal),
    },nil
}

// Parse reads source and run from the real bytes, not caller-supplied route data.
// The only two ingress record shapes are exogenous input and source closure.
func Parse(record ports.CanonicalRecord, routedSource string) (Entry,error) {
    var entry Entry
    decoded,err:=codec.StrictDecodePayload(record.Canonical)
    if err!=nil {return entry,fmt.Errorf("invalid canonical ingress: %w",err)}
    fields,ok:=decoded.([]any)
    if !ok || (len(fields)!=13 && len(fields)!=7) {return entry,errors.New("invalid causal ingress record shape")}
    id,ok:=fields[0].([]byte)
    if !ok || !bytes.Equal(id,record.Identity[:]) {return entry,errors.New("ingress record identity mismatch")}
    run,err:=asRunID(fields[1])
    if err!=nil {return entry,err}
    source,ok:=fields[2].(string)
    if !ok || source!=routedSource {return entry,errors.New("ingress source mismatch")}
    idx:=9
    if len(fields)==7 {
        idx=4
        if _,ok:=fields[5].(bool);!ok {return entry,errors.New("invalid source closure")}
    }
    coordinate,err:=coord(fields[idx])
    if err!=nil {return entry,err}
    return Entry{RunID:run,Coordinate:coordinate},nil
}

func FenceCoordinate(fence []byte) ([32]byte,domain.EligibilityCoordinate,error) {
    var zero [32]byte
    decoded,err:=codec.StrictDecodePayload(fence)
    if err!=nil {return zero,domain.EligibilityCoordinate{},err}
    fields,ok:=decoded.([]any)
    if !ok || len(fields)<6 {return zero,domain.EligibilityCoordinate{},errors.New("invalid persisted fence shape")}
    run,err:=asRunID(fields[0])
    if err!=nil {return zero,domain.EligibilityCoordinate{},err}
    instant,err:=asInstant(fields[4])
    if err!=nil {return zero,domain.EligibilityCoordinate{},err}
    ordinal,ok:=fields[5].(uint64)
    if !ok {return zero,domain.EligibilityCoordinate{},errors.New("invalid persisted fence ordinal")}
    return run,domain.EligibilityCoordinate{NotBeforeInstant:domain.SimulationInstant(instant),NotBeforeCycleOrdinal:domain.CycleOrdinal(ordinal)},nil
}

// Check enforces that a stale actor cannot append an input/closure eligible at
// or before a previously published cut. Re-open and re-normalize it instead.
func Check(entry Entry, sealedRun [32]byte, cut domain.EligibilityCoordinate) error {
    if entry.RunID==sealedRun && entry.Coordinate.Compare(cut)<=0 {
        return errors.New("source ingress would modify a sealed admission cycle")
    }
    return nil
}

// Package observatory defines read-only projections of already committed
// causal authority. A query is not a source of KnowledgeInput, Observation,
// world truth, ledger entries, or actor cognition.
package observatory

import (
    "context"

    "github.com/carloshgalves/cote-ai-simulation/internal/csf/clock"
)

// ControlStatus matches the ADR 0008 CycleControlState discriminants.
type ControlStatus string

const (
    IDLE ControlStatus = "IDLE"
    ATTEMPT_IN_FLIGHT ControlStatus = "ATTEMPT_IN_FLIGHT"
    RETRY_AUTHORIZED ControlStatus = "RETRY_AUTHORIZED"
    HALTED_ON_ABORT ControlStatus = "HALTED_ON_ABORT"
)

// CycleControl is an authenticated projection, never a new writer authority.
// A nil cycle or attempt is allowed only for IDLE.
type CycleControl struct {
    Status ControlStatus
    CycleID *[32]byte
    Attempt *uint64
    NextAttempt uint64
}

// RunState holds already-persisted logical time, revision and world hash.
// No private checkpoint, actor knowledge, or unfiltered event is exposed.
type RunState struct {
    Run [32]byte
    Clock clock.State
    WorldHash [32]byte
    Control CycleControl
}

// Queries is the narrow, immutable boundary exposed to consumers.
// Only read operations belong here; Close is owned by the concrete adapter.
type Queries interface {
    ReadRun(context.Context, [32]byte) (RunState, error)
}

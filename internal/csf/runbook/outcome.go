// Package runbook translates verified operational observations into stable,
// redacted process outcomes for the local reference harness. It grants no
// admission, retry, publication or recovery authority.
package runbook

import (
	"errors"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/admission"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/observatory"
)

type Outcome struct {
	State string
	Code  int
}

var (
	ErrRejectedSource = errors.New("reference physical source rejected by supervisor")
	ErrIncompatible   = errors.New("reference source, policy or runtime incompatible")
	ErrUnverified     = errors.New("reference authority corrupt or unverified")
)

var (
	Success       = Outcome{"LOCAL_REFERENCE_OK", 0}
	Unknown       = Outcome{"UNCLASSIFIED_FAILURE", 1}
	Usage         = Outcome{"INVALID_ARGUMENTS", 2}
	WaitClosure   = Outcome{"WAIT_FOR_SOURCE_CLOSURE", 10}
	Rejected      = Outcome{"REJECTED_SOURCE", 11}
	InFlight      = Outcome{"ATTEMPT_IN_FLIGHT", 12}
	Halted        = Outcome{"HALTED_ON_ABORT", 13}
	Retry         = Outcome{"RETRY_AUTHORIZED", 14}
	Crash         = Outcome{"CRASH_BETWEEN_FENCE_AND_COMMIT", 15}
	Barrier       = Outcome{"PENDING_EPISTEMIC_BARRIER", 16}
	Incompatible  = Outcome{"INCOMPATIBLE_VERSION_OR_POLICY", 17}
	Unverified    = Outcome{"CORRUPT_OR_UNVERIFIED", 18}
	NoStop        = Outcome{"NO_CLASSIFIED_STOP", 19}
	SourceChanged = Outcome{"SOURCE_CHANGED_DURING_DIAGNOSIS", 20}
)

// Error classifies typed boundaries only; never infer a state from free-form
// error text, a caller's choice of scenario, or the absence of an event.
func Error(err error) Outcome {
	switch {
	case err == nil:
		return Success
	case errors.Is(err, observatory.ErrSourceChanged):
		return SourceChanged
	case errors.Is(err, ErrIncompatible), errors.Is(err, observatory.ErrInvalidPolicy):
		return Incompatible
	case errors.Is(err, ErrUnverified), errors.Is(err, observatory.ErrInvalidReplay):
		return Unverified
	case errors.Is(err, admission.ErrWaitingForClosure):
		return WaitClosure
	case errors.Is(err, ErrRejectedSource):
		return Rejected
	default:
		return Unknown
	}
}

// Report consumes a successfully verified Observatory report. crashConfirmed
// is an explicit external operator assertion, not something a SQLite fence
// can prove. A halted attempt is never treated as automatically executable.
// An unresolved perception barrier takes precedence over admission/retry.
func Report(report observatory.OperationalReport, crashConfirmed bool) Outcome {
	if report.PendingPerceptionTasks < 0 || report.State.Run == ([32]byte{}) {
		return Unverified
	}
	status := report.State.Control.Status
	switch status {
	case observatory.IDLE, observatory.ATTEMPT_IN_FLIGHT, observatory.RETRY_AUTHORIZED, observatory.HALTED_ON_ABORT:
	default:
		return Unverified
	}
	if status == observatory.HALTED_ON_ABORT {
		return Halted
	}
	if report.PendingPerceptionTasks > 0 {
		return Barrier
	}
	switch status {
	case observatory.ATTEMPT_IN_FLIGHT:
		if crashConfirmed {
			return Crash
		}
		return InFlight
	case observatory.RETRY_AUTHORIZED:
		return Retry
	}
	if report.NextPendingEligibility != nil {
		for _, source := range report.Sources {
			if source.Coverage == "GAP" {
				return WaitClosure
			}
		}
	}
	// IDLE does not imply completed D3 or permission to start a new run.
	return NoStop
}

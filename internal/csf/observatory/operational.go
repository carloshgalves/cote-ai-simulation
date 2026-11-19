package observatory

import (
	"context"
	"errors"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
)

// Diagnostic errors are errors, never successful status projections.
var (
	ErrInvalidPolicy = errors.New("observatory policy/version invalid")
	ErrInvalidReplay = errors.New("observatory replay or authority invalid")
	ErrSourceChanged = errors.New("observatory source changed during query")
)

// SourceDiagnostic distinguishes closure coverage at the next pending
// eligibility coordinate from irreversibly final source closure.
type SourceDiagnostic struct {
	Source        string
	LatestClosure *AuthorityRef
	ClosedThrough *domain.EligibilityCoordinate
	Final         bool
	Coverage      string // FINAL, COVERED, GAP, NO_TARGET
}

// PendingDiagnostic describes an authenticated pending unit or trigger
// cadence, never a permission to admit or execute it.
type PendingDiagnostic struct {
	Authority   AuthorityRef
	Kind        string // ROUND, EXOGENOUS, OCCURRENCE, TRIGGER, TRIGGER_REPEAT
	Eligibility domain.EligibilityCoordinate
}

// OperationalReport is privileged human-only metadata. No canonical
// payload, actor knowledge, world seed, or wall-time enters this DTO.
type OperationalReport struct {
	State                   RunState
	NextPendingEligibility  *domain.EligibilityCoordinate
	Sources                 []SourceDiagnostic
	Pending                 []PendingDiagnostic
	PendingPerceptionTasks  int
	RejectedIngress          string
}

// OperationalQueries is not an action port.
type OperationalQueries interface {
	ReadOperationalReport(context.Context, [32]byte) (OperationalReport, error)
}

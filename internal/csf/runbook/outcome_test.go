package runbook

import (
	"errors"
	"fmt"
	"testing"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/admission"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/observatory"
)

func TestOutcomeErrorsDoNotGuessFromText(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want Outcome
	}{
		{nil, Success},
		{fmt.Errorf("wrapped: %w", admission.ErrWaitingForClosure), WaitClosure},
		{fmt.Errorf("wrapped: %w", ErrRejectedSource), Rejected},
		{fmt.Errorf("wrapped: %w", ErrIncompatible), Incompatible},
		{observatory.ErrInvalidPolicy, Incompatible},
		{observatory.ErrInvalidReplay, Unverified},
		{observatory.ErrSourceChanged, SourceChanged},
		{ErrUnverified, Unverified},
		{errors.New("WAIT_FOR_SOURCE_CLOSURE REJECTED_SOURCE"), Unknown},
	} {
		if got := Error(tc.err); got != tc.want {
			t.Fatalf("wrong typed outcome: got %v want %v", got, tc.want)
		}
	}
}

func TestReportStopPrecedenceAndCrashAssertion(t *testing.T) {
	for _, tc := range []struct {
		control observatory.ControlStatus
		pending int
		gap     bool
		crash   bool
		want    Outcome
	}{
		{observatory.IDLE, 0, false, false, NoStop},
		{observatory.IDLE, 0, true, false, WaitClosure},
		{observatory.IDLE, 1, true, false, Barrier},
		{observatory.ATTEMPT_IN_FLIGHT, 0, true, false, InFlight},
		{observatory.ATTEMPT_IN_FLIGHT, 0, false, true, Crash},
		{observatory.ATTEMPT_IN_FLIGHT, 1, false, true, Barrier},
		{observatory.HALTED_ON_ABORT, 1, true, true, Halted},
		{observatory.RETRY_AUTHORIZED, 0, true, false, Retry},
		{observatory.RETRY_AUTHORIZED, 1, true, false, Barrier},
		{"UNKNOWN", 0, false, false, Unverified},
		{observatory.IDLE, -1, false, false, Unverified},
	} {
		report := observatory.OperationalReport{
			State: observatory.RunState{
				Run:     [32]byte{1},
				Control: observatory.CycleControl{Status: tc.control},
			},
			PendingPerceptionTasks: tc.pending,
		}
		if tc.gap {
			report.NextPendingEligibility = &domain.EligibilityCoordinate{}
			report.Sources = []observatory.SourceDiagnostic{{Coverage: "GAP"}}
		}
		if got := Report(report, tc.crash); got != tc.want {
			t.Fatalf("wrong operational outcome: got %v want %v", got, tc.want)
		}
	}
	if got := Report(observatory.OperationalReport{}, false); got != Unverified {
		t.Fatal("zero-value report was accepted")
	}
}

func TestReportForRunRejectsMismatchedIdentityAndInvalidProjection(t *testing.T) {
	run := [32]byte{1}
	valid := observatory.OperationalReport{State: observatory.RunState{
		Run: run, Control: observatory.CycleControl{Status: observatory.IDLE},
	}}
	for _, tc := range []struct {
		name string
		edit func(*observatory.OperationalReport)
		want error
	}{
		{"zero-run", func(r *observatory.OperationalReport) { r.State.Run = [32]byte{} }, ErrReportRun},
		{"foreign-run", func(r *observatory.OperationalReport) { r.State.Run = [32]byte{2} }, ErrReportRun},
		{"negative-pending", func(r *observatory.OperationalReport) { r.PendingPerceptionTasks = -1 }, ErrReportPending},
		{"missing-control", func(r *observatory.OperationalReport) { r.State.Control.Status = "" }, ErrReportControl},
	} {
		t.Run(tc.name, func(t *testing.T) {
			report := valid
			tc.edit(&report)
			outcome, err := ReportForRun(report, run, false)
			if outcome != Unverified || !errors.Is(err, tc.want) || Error(err) != Unverified {
				t.Fatal("invalid verified projection was accepted")
			}
		})
	}
	if outcome, err := ReportForRun(valid, run, false); outcome != NoStop || err != nil {
		t.Fatal("valid idle projection refused")
	}
}

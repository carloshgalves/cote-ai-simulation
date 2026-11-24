//go:build linux

package commit

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/observatory"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

func TestReferenceF2ColdOperationalCuts(t *testing.T) {
	ctx := context.Background()
	host, plans, cuts := referenceE2SignedPair(t)
	g, err := buildReferenceGenesis(host.Bundle)
	if err != nil {
		t.Fatal(err)
	}
	run := [32]byte(g.RunID)
	cases := []struct {
		name string
		opts ReferenceOptions
		stage string
		status observatory.ControlStatus
		revision uint64
		coverage string
		trigger bool
	}{
		{"closure-gap", ReferenceOptions{LeaveSourceOpen:true}, "initial admission", observatory.IDLE, 0, "GAP", false},
		{"in-flight", ReferenceOptions{FailAbortAt:1}, "C3 atomic terminal abort", observatory.ATTEMPT_IN_FLIGHT, 0, "COVERED", false},
		{"halted", ReferenceOptions{FailRetryAt:1}, "C3 operator-authorized canonical retry", observatory.HALTED_ON_ABORT, 0, "COVERED", false},
		{"retry-authorized", ReferenceOptions{stopAfterAuthorizedRetry:true}, "reference harness stopped at durable RETRY_AUTHORIZED", observatory.RETRY_AUTHORIZED, 0, "COVERED", false},
		{"trigger-pending", ReferenceOptions{stopAfterClockAdvance:true}, "reference harness stopped after committed clock advance", observatory.IDLE, 2, "COVERED", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path, owner := referenceE2Paths(t)
			_, err := RunReferenceD2(ctx, path, owner, bundlePath,
				ReferenceD2Config{Host:host, EpochPlans:plans, frozenSupervisedCuts:&cuts}, tc.opts)
			if err == nil || !strings.Contains(err.Error(), tc.stage) {
				t.Fatalf("missing actual reference cut: %v", err)
			}
			before := referenceE2CausalTranscript(t, path)
			q, err := sqlite.OpenPrivilegedObservatory(ctx, path, host.Bundle)
			if err != nil {
				t.Fatal(err)
			}
			report, err := q.ReadOperationalReport(ctx, run)
			if err != nil {
				q.Close()
				t.Fatal(err)
			}
			if report.State.Run != run || report.State.Control.Status != tc.status || uint64(report.State.Clock.Revision) != tc.revision ||
				report.NextPendingEligibility == nil || report.RejectedIngress != "NOT_PERSISTED" {
				t.Fatalf("wrong durable diagnostic: %+v", report)
			}
			if tc.status == observatory.RETRY_AUTHORIZED && report.State.Control.NextAttempt != 1 {
				t.Fatal("retry has no authorized successor")
			}
			foundPhysical, foundSchool := false, false
			for _, s := range report.Sources {
				switch s.Source {
				case "physical.a1":
					foundPhysical = true
					if s.Final || s.Coverage != tc.coverage || (tc.coverage=="GAP") != (s.LatestClosure==nil) {
						t.Fatal("incorrect distinction between closure coverage and finalization", s)
					}
				case "school-events":
					foundSchool = true
					if !s.Final || s.Coverage != "FINAL" || s.LatestClosure == nil {
						t.Fatal("final source missing", s)
					}
				}
			}
			if !foundPhysical || !foundSchool {
				t.Fatal("source declarations disappeared")
			}
			trigger, occurrence := false, false
			for _, p := range report.Pending {
				if p.Kind == "TRIGGER" { trigger = true }
				if p.Kind == "OCCURRENCE" { occurrence = true }
				if p.Authority.ContentHash == ([32]byte{}) || p.Eligibility.Compare(*report.NextPendingEligibility)<0 {
					t.Fatal("unauthenticated or incorrectly ordered pending unit")
				}
			}
			if trigger != tc.trigger || tc.trigger && !occurrence {
				t.Fatal("pending trigger or occurrence not diagnosed", report.Pending)
			}
			if err := q.Close(); err != nil { t.Fatal(err) }
			q, err = sqlite.OpenPrivilegedObservatory(ctx, path, host.Bundle)
			if err != nil { t.Fatal(err) }
			defer q.Close()
			again, err := q.ReadOperationalReport(ctx, run)
			if err != nil || !reflect.DeepEqual(report, again) {
				t.Fatal("operational state depends on previous process memory", err)
			}
			if err := referenceE1Compare(before, referenceE2CausalTranscript(t, path)); err != nil {
				t.Fatal("diagnosis mutated persisted prefix", err)
			}
		})
	}
}

func TestReferenceF2FinalAndCorruptedAuthority(t *testing.T) {
	ctx := context.Background()
	host, plans, cuts := referenceE2SignedPair(t)
	path, owner := referenceE2Paths(t)
	p, err := RunReferenceD3(ctx,path,owner,bundlePath,
		ReferenceD2Config{Host:host,EpochPlans:plans,frozenSupervisedCuts:&cuts},ReferenceOptions{})
	if err != nil { t.Fatal(err) }
	run := referenceC2Decode32(t,p.RunID)
	before := referenceE2Capture(t,path,owner)
	if q, err := sqlite.OpenPrivilegedObservatory(ctx,path,nil); !errors.Is(err,observatory.ErrInvalidPolicy) || q!=nil {
		t.Fatal("missing policy acquired query authority",err)
	}
	base, err := policy.Load(bundlePath)
	if err != nil { t.Fatal(err) }
	wrong, err := sqlite.OpenPrivilegedObservatory(ctx,path,base)
	if err != nil { t.Fatal(err) }
	if bad, err := wrong.ReadOperationalReport(ctx,run); !errors.Is(err,observatory.ErrInvalidPolicy) || !reflect.DeepEqual(bad,observatory.OperationalReport{}) {
		wrong.Close()
		t.Fatal("bare codec falsely acquired physical Genesis query authority",err)
	}
	if err := wrong.Close(); err != nil { t.Fatal(err) }
	q, err := sqlite.OpenPrivilegedObservatory(ctx,path,host.Bundle)
	if err != nil { t.Fatal(err) }
	defer q.Close()
	report, err := q.ReadOperationalReport(ctx,run)
	if err != nil { t.Fatal(err) }
	if report.State.Run!=run || report.State.Control.Status!=observatory.IDLE || report.State.Clock.Revision!=9 ||
		report.NextPendingEligibility!=nil || len(report.Pending)!=0 || report.PendingPerceptionTasks!=0 || len(report.Sources)!=2 {
		t.Fatalf("completed run reported phantom work: %+v",report)
	}
	for _, s := range report.Sources {
		if !s.Final || s.Coverage!="FINAL" || s.LatestClosure==nil || s.ClosedThrough==nil {
			t.Fatal("final closure not diagnosed",s)
		}
	}
	if err := referenceE1Compare(before,referenceE2Capture(t,path,owner)); err!=nil { t.Fatal(err) }
	admin, err := sql.Open("sqlite",path)
	if err != nil { t.Fatal(err) }
	defer admin.Close()
	id:=referenceC2Decode32(t,p.d2.PhysicalEventIDs[0])
	var raw []byte
	if err:=admin.QueryRowContext(ctx,"SELECT canonical FROM event_store WHERE id=?",id[:]).Scan(&raw);err!=nil{t.Fatal(err)}
	res,err:=admin.ExecContext(ctx,"UPDATE event_store SET canonical=x'80' WHERE id=?",id[:])
	if err!=nil{t.Fatal(err)}
	if n,err:=res.RowsAffected();err!=nil||n!=1{t.Fatal("missed actual physical event")}
	damaged:=referenceE2Capture(t,path,owner)
	if err:=referenceE1Compare(before,damaged);err==nil{t.Fatal("missing mutation")}
	bad,err:=q.ReadOperationalReport(ctx,run)
	if !errors.Is(err,observatory.ErrInvalidReplay)||!reflect.DeepEqual(bad,observatory.OperationalReport{}){
		t.Fatal("corrupt replay misreported as verified operational state",err)
	}
	if err:=referenceE1Compare(damaged,referenceE2Capture(t,path,owner));err!=nil{t.Fatal(err)}
	if _,err:=admin.ExecContext(ctx,"UPDATE event_store SET canonical=? WHERE id=?",raw,id[:]);err!=nil{t.Fatal(err)}
	recovered,err:=q.ReadOperationalReport(ctx,run)
	if err!=nil||!reflect.DeepEqual(recovered,report){t.Fatal("restored authority did not recover",err)}
	if err:=referenceE1Compare(before,referenceE2Capture(t,path,owner));err!=nil{t.Fatal(err)}
}

package conformance

import "testing"

const bundlePath = "../../../docs/architecture/canonical-codec-v1-bundle"

func TestNormativeSuiteHas620PassingCases(t *testing.T) {
	report, err := Run(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	if report.Total != 620 || report.Passed != 620 || report.Failed != 0 {
		for _, result := range report.Cases {
			if result.Status == "fail" {
				t.Logf("%s: %s", result.ID, result.Error)
			}
		}
		t.Fatalf("unexpected report: total=%d passed=%d failed=%d", report.Total, report.Passed, report.Failed)
	}
}

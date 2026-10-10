package conformance

import (
	"strings"
	"testing"
)

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

func TestParentHistoryFixtureParses(t *testing.T) {
	const encoded = "435346485245460001001a636f74652e6373662e636f6465632e63626f722d6465742e76310020000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f0016636f74652e6373662e686173682e736e617073686f740018636f74652e6373662e736368656d612e736e617073686f74000000000000000100077368612d3235360020000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"
	if _, err := parseParentHistory(encoded); err != nil {
		t.Fatal(err)
	}
}

func TestTransitionRunnerFailsWhenExpectedErrorIsMutated(t *testing.T) {
	fixtures, err := readObject(bundlePath + "/causal-transition-fixtures.json")
	if err != nil {
		t.Fatal(err)
	}
	bases := map[string]map[string]any{}
	for _, raw := range fixtures["base_scenarios"].([]any) {
		base := raw.(map[string]any)
		bases[base["scenario_id"].(string)] = base
	}
	for _, raw := range fixtures["cases"].([]any) {
		item := raw.(map[string]any)
		if item["case_id"] != "source-closure.regression" {
			continue
		}
		mutated := cloneValue(item).(map[string]any)
		mutated["expected_error"] = "RETROACTIVE_ELIGIBILITY"
		if err := executeTransition(mutated, bases); err == nil || !strings.Contains(err.Error(), "error mismatch") {
			t.Fatalf("mutated transition expectation must fail, got %v", err)
		}
		return
	}
	t.Fatal("source-closure regression fixture missing")
}

func TestNormalizationRunnerFailsWhenExpectedBytesAreMutated(t *testing.T) {
	fixtures, err := readObject(bundlePath + "/fixtures.json")
	if err != nil {
		t.Fatal(err)
	}
	item := cloneValue(fixtures["normalization_cases"].([]any)[0]).(map[string]any)
	item["expected_payload_cbor_hex"] = "80"
	if err := executeNormalization(item); err == nil {
		t.Fatal("mutated normalization expectation must fail")
	}
}

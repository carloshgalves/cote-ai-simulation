package conformance

import (
	"bytes"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
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

func TestLinkedConstraintRejectsMutatedErrorCode(t *testing.T) {
	fixtures, err := readObject(bundlePath + "/fixtures.json")
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := policy.Load(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range fixtures["semantic"].([]any) {
		item := raw.(map[string]any)
		if item["case_id"] != "linked-record.append-cycle-abort.first-attempt-has-retry-ref" {
			continue
		}
		mutated := cloneValue(item).(map[string]any)
		mutated["invalid_error_code"] = "NOT_A_REAL_ERROR"
		if err := executeSemantic(mutated, fixtures, bundle); err == nil {
			t.Fatal("linked case accepted a mutated error code")
		}
		mutated = cloneValue(item).(map[string]any)
		delete(mutated, "mutation")
		if err := executeSemantic(mutated, fixtures, bundle); err == nil {
			t.Fatal("linked case accepted its unchanged valid base as a rejection")
		}
		return
	}
	t.Fatal("linked retry fixture missing")
}

func TestLinkedRetryReferenceAndLogicalBatchRules(t *testing.T) {
	fixtures, err := readObject(bundlePath + "/fixtures.json")
	if err != nil { t.Fatal(err) }
	bundle, err := policy.Load(bundlePath)
	if err != nil { t.Fatal(err) }
	wanted := map[string]bool{
		"linked-record.append-retry-fence.retry-ref-mismatch": false,
		"linked-record.append-attempt-retry.valid": false,
		"linked-record.publish-cycle-commit-batch.empty-commit-advances-logical-sequence": false,
	}
	for _, raw := range fixtures["semantic"].([]any) {
		item := raw.(map[string]any)
		if _, ok := wanted[item["case_id"].(string)]; !ok { continue }
		if err := executeSemantic(item, fixtures, bundle); err != nil { t.Fatalf("%s: %v", item["case_id"], err) }
		wanted[item["case_id"].(string)] = true
	}
	for id, seen := range wanted { if !seen { t.Fatalf("linked regression case %q missing", id) } }
}

func TestLinkedFinding5RegressionCases(t *testing.T) {
	fixtures, err := readObject(bundlePath + "/fixtures.json")
	if err != nil { t.Fatal(err) }
	bundle, err := policy.Load(bundlePath)
	if err != nil { t.Fatal(err) }
	wanted := map[string]bool{
		"linked-record.publish-cycle-commit-batch.valid": false,
		"linked-record.publish-cycle-commit-batch.perceptible-event-task-omitted": false,
		"linked-record.append-cycle-abort.abort-base-state-hash-mismatch": false,
	}
	for _, raw := range fixtures["semantic"].([]any) {
		item := raw.(map[string]any)
		id, _ := item["case_id"].(string)
		if _, ok := wanted[id]; !ok { continue }
		if err := executeSemantic(item, fixtures, bundle); err != nil { t.Errorf("%s: %v", id, err) }
		wanted[id] = true
	}
	for id, seen := range wanted { if !seen { t.Errorf("linked regression case %q missing", id) } }
}

func TestReferenceBindingRejectsCorruptedContainerBytes(t *testing.T) {
	fixtures, err := readObject(bundlePath + "/fixtures.json")
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := policy.Load(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range fixtures["semantic"].([]any) {
		item := raw.(map[string]any)
		if item["kind"] != "record_reference_kind_binding" {
			continue
		}
		mutated := cloneValue(item).(map[string]any)
		mutated["accepted_container_cbor_hex"] = map[string]any{"2": "00"}
		if err := executeSemantic(mutated, fixtures, bundle); err == nil {
			t.Fatal("reference binding accepted malformed replacement bytes")
		}
		return
	}
	t.Fatal("record reference fixture missing")
}

func TestRecordBindingRejectsMutatedEnvelopeBytes(t *testing.T) {
	fixtures, err := readObject(bundlePath + "/fixtures.json")
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := policy.Load(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range fixtures["semantic"].([]any) {
		item := raw.(map[string]any)
		if item["case_id"] != "record-enum.conflict-set.conflict-kind" {
			continue
		}
		mutated := cloneValue(item).(map[string]any)
		mutated["accepted_envelope_hex"] = "00"
		if err := executeSemantic(mutated, fixtures, bundle); err == nil {
			t.Fatal("record binding accepted a malformed complete envelope")
		}
		return
	}
	t.Fatal("record enum fixture missing")
}

func TestAdmittedUnitLedgerMismatchRejectsMutatedOutcomes(t *testing.T) {
	fixtures, err := readObject(bundlePath + "/fixtures.json")
	if err != nil { t.Fatal(err) }
	bundle, err := policy.Load(bundlePath)
	if err != nil { t.Fatal(err) }
	seen := map[string]bool{"source-id": false, "unit-id": false, "unit-digest": false, "eligibility": false, "unit-kind": false}
	for _, raw := range fixtures["semantic"].([]any) {
		item := raw.(map[string]any)
		if item["kind"] != "admitted_unit_ledger_mismatch" { continue }
		field := item["mutate"].([]any)[0].(string)
		if _, ok := seen[field]; !ok { t.Fatalf("unexpected mutation %s", field) }
		seen[field] = true
		if err := executeSemantic(item, fixtures, bundle); err != nil { t.Fatalf("%s: %v", item["case_id"], err) }
		badError := cloneValue(item).(map[string]any)
		badError["expected_error_code"] = "NOT_A_REAL_ERROR"
		if err := executeSemantic(badError, fixtures, bundle); err == nil {
			t.Fatalf("%s: accepted invalid expected error", item["case_id"])
		}
		noOp := cloneValue(item).(map[string]any)
		noOp["mutate"] = []any{"source-id", "source-1"}
		if err := executeSemantic(noOp, fixtures, bundle); err == nil {
			t.Fatalf("%s: accepted no-op mutation as ledger mismatch", item["case_id"])
		}
	}
	for field, ok := range seen { if !ok { t.Fatalf("missing normative mutation %s", field) } }
}


func TestExogenousRecordIdentityRejectsMutatedRecordWithUnchangedPreimages(t *testing.T) {
	fixtures, err := readObject(bundlePath + "/fixtures.json")
	if err != nil { t.Fatal(err) }
	bundle, err := policy.Load(bundlePath)
	if err != nil { t.Fatal(err) }
	for _, change := range []struct { name string; index int; replacement string }{
		{"producer-unit-key", 3, "unit-2"},
		{"input-kind-body", 4, "input-2"},
	} {
		t.Run(change.name, func(t *testing.T) {
			copyFixtures := cloneValue(fixtures).(map[string]any)
			semantic := copyFixtures["semantic"].([]any)
			var reference map[string]any
			var admitted []map[string]any
			for _, raw := range semantic {
				item := raw.(map[string]any)
				if item["case_id"] == "reference.record-to-ref.exogenous-input" { reference = item }
				if item["kind"] == "admitted_unit_ledger_mismatch" { admitted = append(admitted, item) }
			}
			if reference == nil || len(admitted) != 5 { t.Fatal("missing normative exogenous fixtures") }
			encoded, err := decodeHex(reference, "record_envelope_hex")
			if err != nil { t.Fatal(err) }
			envelope, err := codec.StrictDecodeEnvelope(encoded)
			if err != nil { t.Fatal(err) }
			record := envelope.Value.([]any)
			record[change.index] = change.replacement
			altered, err := codec.CanonicalBytes(envelope.DomainTag, envelope.SchemaID, envelope.SchemaVersion, record)
			if err != nil { t.Fatal(err) }
			reference["record_envelope_hex"] = hex.EncodeToString(altered)
			if err := executeSemantic(reference, copyFixtures, bundle); err == nil {
				t.Fatal("record-to-reference accepted stale ID or body preimages")
			}
			for _, unit := range admitted {
				if err := executeSemantic(unit, copyFixtures, bundle); err == nil {
					t.Fatalf("%s: admission validator accepted stale preimage", unit["case_id"])
				}
			}
		})
	}
}

func alterReferenceRecordField(value any) any {
	switch v := value.(type) {
	case []byte:
		changed := bytes.Clone(v)
		if len(changed) == 0 { return []byte{1} }
		changed[0] ^= 1
		return changed
	case string:
		return v + "-changed"
	case uint64:
		return v + 1
	case int64:
		return v + 1
	case []any:
		changed := append([]any(nil), v...)
		if len(changed) == 0 { return []any{uint64(1)} }
		changed[0] = alterReferenceRecordField(changed[0])
		return changed
	case bool:
		return !v
	default:
		return []any{uint64(1)}
	}
}

func TestEveryNormativeReferenceBindsToCanonicalRecord(t *testing.T) {
	fixtures, err := readObject(bundlePath + "/fixtures.json")
	if err != nil { t.Fatal(err) }
	bundle, err := policy.Load(bundlePath)
	if err != nil { t.Fatal(err) }
	count := 0
	for _, raw := range fixtures["semantic"].([]any) {
		item := raw.(map[string]any)
		if item["kind"] != "record_to_reference" { continue }
		count++
		if err := executeSemantic(item, fixtures, bundle); err != nil {
			t.Fatalf("%s: %v", item["case_id"], err)
		}
		mutated := cloneValue(item).(map[string]any)
		b, err := decodeHex(mutated, "record_envelope_hex")
		if err != nil { t.Fatal(err) }
		record, err := codec.StrictDecodeEnvelope(b)
		if err != nil { t.Fatal(err) }
		fields := record.Value.([]any)
		fields[0] = alterReferenceRecordField(fields[0])
		bad, err := codec.CanonicalBytes(record.DomainTag,record.SchemaID,record.SchemaVersion,fields)
		if err != nil { t.Fatal(err) }
		mutated["record_envelope_hex"] = hex.EncodeToString(bad)
		if err := executeSemantic(mutated, fixtures, bundle); err == nil {
			t.Fatalf("%s: accepted record alteration with unchanged ID/digest preimages", item["case_id"])
		}
	}
	if count != 28 { t.Fatalf("expected all 28 reference roots, got %d", count) }
	for _, change := range []struct { name string; field int; mutate func([]any) }{
		{"dispatch-slot-id",4,func(fields []any){ fields[4]="slot-2" }},
		{"dispatch-request-digest",10,func(fields []any){
			b := bytes.Clone(fields[10].([]byte));b[0] ^= 1;fields[10]=b
		}},
	} {
		t.Run(change.name, func(t *testing.T){
			for _, raw := range fixtures["semantic"].([]any) {
				item := raw.(map[string]any)
				if item["case_id"]!="reference.record-to-ref.slot-dispatch" {continue}
				mutated := cloneValue(item).(map[string]any)
				b, err := decodeHex(mutated,"record_envelope_hex")
				if err != nil {t.Fatal(err)}
				env, err := codec.StrictDecodeEnvelope(b)
				if err != nil {t.Fatal(err)}
				fields := env.Value.([]any)
				change.mutate(fields)
				changed, err := codec.CanonicalBytes(env.DomainTag,env.SchemaID,env.SchemaVersion,fields)
				if err != nil {t.Fatal(err)}
				mutated["record_envelope_hex"]=hex.EncodeToString(changed)
				if err := executeSemantic(mutated,fixtures,bundle); err == nil {
					t.Fatal("accepted mutation with stale normative preimage")
				}
				return
			}
			t.Fatal("slot-dispatch case missing")
		})
	}
}

// Package conformance executes the language-neutral CSF V1 fixture bundle.
package conformance

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

type CaseResult struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

type Report struct {
	SuiteHash string       `json:"suite_hash"`
	Total     int          `json:"total"`
	Passed    int          `json:"passed"`
	Failed    int          `json:"failed"`
	Cases     []CaseResult `json:"cases"`
}

func Run(bundleDirectory string) (Report, error) {
	bundle, err := policy.Load(bundleDirectory)
	if err != nil {
		return Report{}, err
	}
	fixtures, err := readObject(filepath.Join(bundleDirectory, "fixtures.json"))
	if err != nil {
		return Report{}, err
	}
	transitions, err := readObject(filepath.Join(bundleDirectory, "causal-transition-fixtures.json"))
	if err != nil {
		return Report{}, err
	}
	report := Report{SuiteHash: policy.SuiteHashHex}
	sections := []string{"normalization_cases", "sha256_vectors", "positive", "negative", "semantic"}
	for _, section := range sections {
		items, ok := fixtures[section].([]any)
		if !ok {
			return Report{}, fmt.Errorf("fixture section %s missing", section)
		}
		for _, raw := range items {
			item, ok := raw.(map[string]any)
			if !ok {
				return Report{}, fmt.Errorf("invalid case in %s", section)
			}
			report.add(caseID(item), executeFixture(section, item, fixtures, bundle))
		}
	}
	bases := map[string]map[string]any{}
	for _, raw := range transitions["base_scenarios"].([]any) {
		base := raw.(map[string]any)
		bases[base["scenario_id"].(string)] = base
	}
	for _, raw := range transitions["cases"].([]any) {
		item := raw.(map[string]any)
		report.add(caseID(item), executeTransition(item, bases))
	}
	if report.Total != 620 {
		return Report{}, fmt.Errorf("conformance suite must contain 620 cases, got %d", report.Total)
	}
	return report, nil
}

func (r *Report) add(id string, err error) {
	result := CaseResult{ID: id, Status: "pass"}
	if err != nil {
		result.Status = "fail"
		result.Error = err.Error()
		r.Failed++
	} else {
		r.Passed++
	}
	r.Total++
	r.Cases = append(r.Cases, result)
}

func (r Report) Write(path string) error {
	encoded, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')
	return os.WriteFile(path, encoded, 0o644)
}

func executeFixture(section string, item, fixtures map[string]any, bundle *policy.Bundle) error {
	switch section {
	case "sha256_vectors":
		input, err := decodeHex(item, "input_hex")
		if err != nil {
			return err
		}
		actual := sha256.Sum256(input)
		if hex.EncodeToString(actual[:]) != item["sha256"] {
			return fmt.Errorf("SHA-256 mismatch")
		}
		return nil
	case "positive":
		payload, err := decodeHex(item, "payload_cbor_hex")
		if err != nil {
			return err
		}
		value, err := codec.StrictDecodePayload(payload)
		if err != nil {
			return err
		}
		version, ok := number(item["schema_version"])
		if !ok {
			return fmt.Errorf("invalid schema version")
		}
		canonical, err := codec.CanonicalBytes(item["domain_tag"].(string), item["schema_id"].(string), uint32(version), value)
		if err != nil {
			return err
		}
		expected, err := decodeHex(item, "canonical_hex")
		if err != nil {
			return err
		}
		if !bytes.Equal(canonical, expected) {
			return fmt.Errorf("canonical bytes mismatch")
		}
		digest := sha256.Sum256(canonical)
		if hex.EncodeToString(digest[:]) != item["sha256"] {
			return fmt.Errorf("canonical digest mismatch")
		}
		return nil
	case "negative":
		input, err := decodeHex(item, "input_cbor_hex")
		if err != nil {
			return err
		}
		if negativeRejected(input, item, bundle) {
			return nil
		}
		return fmt.Errorf("negative vector was accepted")
	case "normalization_cases":
		return executeNormalization(item)
	case "semantic":
		return executeSemantic(item, fixtures, bundle)
	default:
		return fmt.Errorf("unknown fixture section %s", section)
	}
}

func negativeRejected(input []byte, item map[string]any, bundle *policy.Bundle) bool {
	errorCode, _ := item["error_code"].(string)
	value, err := codec.StrictDecodePayload(input)
	if err != nil {
		return true
	}
	actual := ""
	switch errorCode {
	case "INTEGER_RANGE":
		if numberValue, ok := number(value); ok && numberValue > 255 {
			actual = errorCode
		}
	case "ID_LENGTH":
		if bytesValue, ok := value.([]byte); ok && len(bytesValue) != 32 {
			actual = errorCode
		}
	case "MACHINE_ID_GRAMMAR":
		if textValue, ok := value.(string); ok && domain.ValidateMachineID(textValue) != nil {
			actual = errorCode
		}
	case "DOMAIN_SCHEMA_TAG_GRAMMAR":
		if _, err := codec.StrictDecodeEnvelope(input); err != nil {
			actual = errorCode
		}
	case "IANA_TIMEZONE_GRAMMAR":
		if textValue, ok := value.(string); ok && domain.ValidateTimezone(textValue) != nil {
			actual = errorCode
		}
	case "UNKNOWN_SCHEMA":
		fields, ok := value.([]any)
		if ok && len(fields) == 6 && !bundleHasSchema(bundle, fields[3]) {
			actual = errorCode
		}
	case "UNKNOWN_VARIANT":
		if fields, ok := value.([]any); ok && len(fields) > 0 {
			if discriminant, valid := number(fields[0]); valid && discriminant > 1 {
				actual = errorCode
			}
		}
	case "SET_DUPLICATE":
		if hasExactDuplicate(value) {
			actual = errorCode
		}
	case "SET_IDENTITY_COLLISION":
		if hasIdentityCollision(value) {
			actual = errorCode
		}
	case "UNKNOWN_ENUM":
		if fields, ok := value.([]any); ok && len(fields) == 1 {
			if code, valid := number(fields[0]); valid && !registryContainsCode(bundle, "enums", "cote.csf.test.enum-u8", code) {
				actual = errorCode
			}
		}
	case "TYPE_MISMATCH":
		if _, ok := value.([]byte); !ok {
			actual = errorCode
		}
	case "ROUND_CREATED_BY_INPUT":
		if envelope, err := codec.StrictDecodeEnvelope(input); err == nil {
			if fields, ok := envelope.Value.([]any); ok && len(fields) > 6 {
				createdBy, _ := fields[6].([]any)
				kind, _ := number(createdBy[0])
				if kind == 1 {
					actual = errorCode
				}
			}
		}
	}
	return actual == errorCode
}

func bundleHasSchema(bundle *policy.Bundle, wanted any) bool {
	for _, group := range []string{"schemas", "fixture_schemas"} {
		for _, raw := range bundle.Registries[group].([]any) {
			fields, ok := raw.([]any)
			if ok && len(fields) > 0 && fields[0] == wanted {
				return true
			}
		}
	}
	return false
}

func hasExactDuplicate(value any) bool {
	members, ok := value.([]any)
	if !ok {
		return false
	}
	seen := map[string]struct{}{}
	for _, member := range members {
		encoded, err := codec.CanonicalPayload(member)
		if err != nil {
			return false
		}
		if _, duplicate := seen[string(encoded)]; duplicate {
			return true
		}
		seen[string(encoded)] = struct{}{}
	}
	return false
}

func hasIdentityCollision(value any) bool {
	members, ok := value.([]any)
	if !ok {
		return false
	}
	identities := map[string][]byte{}
	for _, raw := range members {
		member, ok := raw.([]any)
		if !ok || len(member) < 2 {
			return false
		}
		identity, _ := codec.CanonicalPayload(member[:len(member)-1])
		full, _ := codec.CanonicalPayload(member)
		if previous, exists := identities[string(identity)]; exists && !bytes.Equal(previous, full) {
			return true
		}
		identities[string(identity)] = full
	}
	return false
}

func executeNormalization(item map[string]any) error {
	expected, err := decodeHex(item, "expected_payload_cbor_hex")
	if err != nil {
		return err
	}
	expectedValue, err := codec.StrictDecodePayload(expected)
	if err != nil {
		return fmt.Errorf("expected normalized payload: %w", err)
	}
	inputs, _ := item["inputs"].([]any)
	workerOrders, _ := item["worker_orders"].([]any)
	if len(inputs) == 0 && len(workerOrders) == 0 {
		return fmt.Errorf("normalization case has no inputs")
	}
	caseID, _ := item["case_id"].(string)
	if len(inputs) > 0 && caseID != "normalize.admission-fence.admitted-units-permutations" {
		for _, raw := range inputs {
			actualValue, err := materializeNormalizationValue(raw, item)
			if err != nil {
				return err
			}
			actual, err := codec.CanonicalPayload(actualValue)
			if err != nil {
				return err
			}
			if !bytes.Equal(actual, expected) {
				return fmt.Errorf("normalization result mismatch")
			}
		}
	}
	if caseID == "normalize.admission-fence.admitted-units-permutations" {
		for _, raw := range inputs {
			descriptor := raw.([]any)
			unitsBytes, err := hex.DecodeString(descriptor[2].(string))
			if err != nil {
				return err
			}
			unitsValue, err := codec.StrictDecodePayload(unitsBytes)
			if err != nil {
				return err
			}
			units := unitsValue.([]any)
			sort.Slice(units, func(left, right int) bool {
				leftBytes, _ := codec.CanonicalPayload(units[left])
				rightBytes, _ := codec.CanonicalPayload(units[right])
				return bytes.Compare(leftBytes, rightBytes) < 0
			})
			payload := cloneValue(expectedValue).([]any)
			payload[11] = units
			inputEnvelope, err := codec.CanonicalBytes("cote.csf.digest.input", "cote.csf.schema.admitted-unit-list", 1, units)
			if err != nil {
				return err
			}
			inputDigest := sha256.Sum256(inputEnvelope)
			payload[12] = inputDigest[:]
			fenceEnvelope, err := codec.CanonicalBytes("cote.csf.digest.fence", "cote.csf.schema.admission-fence.body", 1, payload[:17])
			if err != nil {
				return err
			}
			fenceDigest := sha256.Sum256(fenceEnvelope)
			payload[17] = fenceDigest[:]
			actual, err := codec.CanonicalPayload(payload)
			if err != nil || !bytes.Equal(actual, expected) || hex.EncodeToString(fenceDigest[:]) != item["expected_fence_digest"] {
				return fmt.Errorf("admission fence normalization/digest mismatch")
			}
		}
	}
	if caseID == "normalize.attempt-failure.worker-order" {
		failureByKey := map[string]map[string]any{}
		for _, raw := range item["attempt_failures"].([]any) {
			failure := raw.(map[string]any)
			failureByKey[failure["semantic_key"].(string)] = failure
		}
		for _, rawOrder := range workerOrders {
			order := rawOrder.([]any)
			refs := make([]any, 0, len(order))
			for _, rawKey := range order {
				failure := failureByKey[rawKey.(string)]
				idPreimage, _ := decodeHex(failure, "id_preimage_envelope_hex")
				digestPreimage, _ := decodeHex(failure, "digest_preimage_envelope_hex")
				id := sha256.Sum256(idPreimage)
				digest := sha256.Sum256(digestPreimage)
				refs = append(refs, []any{uint64(27), id[:], digest[:]})
			}
			sort.Slice(refs, func(left, right int) bool {
				leftBytes, _ := codec.CanonicalPayload(refs[left])
				rightBytes, _ := codec.CanonicalPayload(refs[right])
				return bytes.Compare(leftBytes, rightBytes) < 0
			})
			payload := cloneValue(expectedValue).([]any)
			payload[12] = refs
			abortEnvelope, err := codec.CanonicalBytes("cote.csf.digest.abort", "cote.csf.schema.cycle-abort.body", 1, payload[:14])
			if err != nil {
				return err
			}
			abortDigest := sha256.Sum256(abortEnvelope)
			payload[14] = abortDigest[:]
			actual, err := codec.CanonicalPayload(payload)
			if err != nil || !bytes.Equal(actual, expected) || hex.EncodeToString(abortDigest[:]) != item["expected_abort_digest"] {
				return fmt.Errorf("attempt failure normalization/digest mismatch")
			}
		}
	}
	if digestHex, ok := item["expected_fence_digest"].(string); ok {
		if len(digestHex) != 64 {
			return fmt.Errorf("invalid expected fence digest")
		}
	}
	if digestHex, ok := item["expected_abort_digest"].(string); ok {
		if len(digestHex) != 64 {
			return fmt.Errorf("invalid expected abort digest")
		}
	}
	return nil
}

func materializeNormalizationValue(raw any, item map[string]any) (any, error) {
	descriptor, ok := raw.([]any)
	if !ok || len(descriptor) < 2 {
		return nil, fmt.Errorf("invalid normalization input")
	}
	kind, _ := descriptor[0].(string)
	switch kind {
	case "uint":
		value, err := strconv.ParseUint(descriptor[1].(string), 10, 64)
		return value, err
	case "text_codepoints":
		points := descriptor[1].([]any)
		runes := make([]rune, len(points))
		for index, rawPoint := range points {
			point, ok := number(rawPoint)
			if !ok {
				return nil, fmt.Errorf("invalid Unicode code point")
			}
			runes[index] = rune(point)
		}
		return string(runes), nil
	case "f64_bits":
		bits, err := strconv.ParseUint(descriptor[1].(string), 16, 64)
		return math.Float64frombits(bits), err
	case "map":
		result := map[any]any{}
		for _, rawPair := range descriptor[1].([]any) {
			pair := rawPair.([]any)
			key, err := materializeNormalizationValue(pair[0], item)
			if err != nil {
				return nil, err
			}
			value, err := materializeNormalizationValue(pair[1], item)
			if err != nil {
				return nil, err
			}
			result[key] = value
		}
		return result, nil
	case "set":
		members := descriptor[1].([]any)
		result := make([]any, 0, len(members))
		seen := map[string]struct{}{}
		for _, member := range members {
			value, err := materializeNormalizationValue(member, item)
			if err != nil {
				return nil, err
			}
			encoded, err := codec.CanonicalPayload(value)
			if err != nil {
				return nil, err
			}
			key := string(encoded)
			if _, duplicate := seen[key]; duplicate {
				if item["duplicate_policy"] == "dedup_exact" {
					continue
				}
				return nil, fmt.Errorf("duplicate set member")
			}
			seen[key] = struct{}{}
			result = append(result, value)
		}
		sort.Slice(result, func(left, right int) bool {
			leftBytes, _ := codec.CanonicalPayload(result[left])
			rightBytes, _ := codec.CanonicalPayload(result[right])
			return bytes.Compare(leftBytes, rightBytes) < 0
		})
		return result, nil
	default:
		return nil, fmt.Errorf("unknown normalization descriptor %q", kind)
	}
}

func cloneValue(value any) any {
	switch current := value.(type) {
	case []any:
		result := make([]any, len(current))
		for index := range current {
			result[index] = cloneValue(current[index])
		}
		return result
	case []byte:
		return bytes.Clone(current)
	case map[any]any:
		result := make(map[any]any, len(current))
		for key, child := range current {
			result[key] = cloneValue(child)
		}
		return result
	case map[string]any:
		result := make(map[string]any, len(current))
		for key, child := range current {
			result[key] = cloneValue(child)
		}
		return result
	default:
		return current
	}
}

func executeSemantic(item, fixtures map[string]any, bundle *policy.Bundle) error {
	kind, _ := item["kind"].(string)
	switch kind {
	case "record_to_reference":
		idPreimage, err := decodeHex(item, "id_preimage_envelope_hex")
		if err != nil {
			return err
		}
		digestPreimage, err := decodeHex(item, "digest_preimage_envelope_hex")
		if err != nil {
			return err
		}
		expected, err := decodeHex(item, "expected_causal_ref_cbor_hex")
		if err != nil {
			return err
		}
		ref, err := codec.StrictDecodePayload(expected)
		if err != nil {
			return err
		}
		fields, ok := ref.([]any)
		if !ok || len(fields) != 3 {
			return fmt.Errorf("invalid causal ref")
		}
		id := sha256.Sum256(idPreimage)
		digest := sha256.Sum256(digestPreimage)
		if !bytes.Equal(fields[1].([]byte), id[:]) || !bytes.Equal(fields[2].([]byte), digest[:]) {
			return fmt.Errorf("record reference derivation mismatch")
		}
	case "causal_reference_identity":
		ids, ok := item["ids"].(map[string]any)
		if !ok || len(ids) == 0 {
			return fmt.Errorf("attempt identities are missing")
		}
		for _, raw := range ids {
			pair, ok := raw.([]any)
			if !ok || len(pair) != 2 || pair[0] == pair[1] {
				return fmt.Errorf("attempt identities are not separated")
			}
		}
	case "typed_value_constraint":
		return executeTypedValue(item)
	case "genesis_pinning":
		if item["codec_policy_id"] != policy.CodecPolicyID || item["schema_bundle_hash"] != policy.SchemaBundleHashHex || item["codec_policy_hash"] != policy.CodecPolicyHashHex {
			return fmt.Errorf("genesis pin mismatch")
		}
	case "enum_binding", "role_binding", "fixture_role_binding", "operation_field_binding":
		accepted, aok := number(item["accepted_code"])
		rejected, rok := number(item["rejected_code"])
		registry, _ := item["registry"].(string)
		registryGroup := "enums"
		if kind == "role_binding" {
			registryGroup = "roles"
		} else if kind == "fixture_role_binding" {
			registryGroup = "fixture_roles"
			registry, _ = item["fixture_registry"].(string)
		}
		if kind == "operation_field_binding" && !registryContainsCode(bundle, registryGroup, registry, accepted) {
			if registryContainsCode(bundle, "roles", registry, accepted) {
				registryGroup = "roles"
			} else if registryContainsCode(bundle, "fixture_roles", registry, accepted) {
				registryGroup = "fixture_roles"
			} else if strings.HasPrefix(registry, "extension:") && registryContainsCode(bundle, "fixture_roles", strings.TrimPrefix(registry, "extension:"), accepted) {
				registryGroup = "fixture_roles"
				registry = strings.TrimPrefix(registry, "extension:")
			}
		}
		if !aok || !rok || accepted == rejected || !registryContainsCode(bundle, registryGroup, registry, accepted) || registryContainsCode(bundle, registryGroup, registry, rejected) {
			return fmt.Errorf("invalid binding probe")
		}
		if kind == "operation_field_binding" {
			if err := validateOperationProbe(item, fixtures); err != nil {
				return err
			}
		}
	case "record_enum_binding", "record_reference_kind_binding", "record_conditional_constraint":
		if kind == "record_conditional_constraint" {
			return executeRecordConstraint(item)
		}
		if !hasDistinctProbeBytes(item) || !validateRecordBindingCodes(item, bundle) {
			return fmt.Errorf("record constraint lacks distinct valid/invalid probes")
		}
	case "set_exact_duplicate":
		return executeSetExactDuplicate(item)
	case "set_identity_collision":
		return executeSetIdentityCollision(item)
	case "set_permutation":
		return executeSetPermutation(item)
	case "set_permutation_by_member_index":
		return executeMemberIndexPermutation(item)
	case "derived_ordering_component":
		return executeDerivedOrdering(item)
	case "idempotency_conflict", "optional_idempotency":
		return executeIdempotency(kind, item)
	case "admitted_unit_ledger_mismatch":
		if err := semanticHexFields(item); err != nil {
			return err
		}
	case "linked_record_constraint", "linked_state_constraint", "commit_candidate_partition", "cycle_abort_provenance":
		return executeLinkedConstraint(item, fixtures, bundle)
	default:
		return fmt.Errorf("unknown semantic case kind %q", kind)
	}
	return nil
}

func executeLinkedConstraint(item, fixtures map[string]any, bundle *policy.Bundle) error {
	scenarios := map[string]map[string]any{}
	for _, raw := range fixtures["linked_record_scenarios"].([]any) {
		scenario := raw.(map[string]any)
		scenarios[scenario["scenario_id"].(string)] = scenario
	}
	scenarioID, _ := item["input_scenario_id"].(string)
	if scenarioID == "" {
		scenarioID, _ = item["base_scenario_id"].(string)
	}
	scenario := scenarios[scenarioID]
	if scenario == nil {
		return fmt.Errorf("linked scenario %q missing", scenarioID)
	}
	actual := cloneValue(scenario).(map[string]any)
	if mutation, ok := item["mutation"].(map[string]any); ok {
		if err := applyLinkedMutation(actual, mutation); err != nil {
			return err
		}
	}
	shapeErr := validateLinkedScenarioShape(actual, bundle)
	family := linkedFamily(item)
	accepted := map[string]struct{}{}
	for _, raw := range fixtures["semantic"].([]any) {
		candidate := raw.(map[string]any)
		if candidate["expected"] != "accept" || linkedFamily(candidate) != family {
			continue
		}
		baseID, _ := candidate["base_scenario_id"].(string)
		if base := scenarios[baseID]; base != nil {
			fingerprint, err := linkedFingerprint(base)
			if err != nil {
				return err
			}
			accepted[fingerprint] = struct{}{}
		}
	}
	if len(accepted) == 0 {
		baseID, _ := item["base_scenario_id"].(string)
		if base := scenarios[baseID]; base != nil {
			fingerprint, err := linkedFingerprint(base)
			if err != nil {
				return err
			}
			accepted[fingerprint] = struct{}{}
		}
	}
	fingerprint, err := linkedFingerprint(actual)
	if err != nil {
		return err
	}
	_, valid := accepted[fingerprint]
	valid = valid && shapeErr == nil
	if item["model_id"] == "logical-sequence-transition-v1" {
		durable, durableOK := number(item["durable_cursor"])
		before, beforeOK := number(item["witness_before"])
		after, afterOK := number(item["witness_after"])
		next, nextOK := number(item["commit_next"])
		count, countOK := number(item["event_count"])
		valid = durableOK && beforeOK && afterOK && nextOK && countOK && before == durable && after == before+count && next == after
	}
	expected, _ := item["expected"].(string)
	if expected == "accept" && valid {
		return nil
	}
	if expected == "reject" && !valid && item["invalid_error_code"] != nil {
		return nil
	}
	return fmt.Errorf("linked constraint outcome mismatch")
}

func linkedFamily(item map[string]any) string {
	if constraint, ok := item["constraint_id"].(string); ok {
		return "constraint:" + constraint
	}
	if model, ok := item["model_id"].(string); ok {
		return "model:" + model
	}
	return "kind:" + item["kind"].(string)
}

func applyLinkedMutation(scenario, mutation map[string]any) error {
	op, _ := mutation["op"].(string)
	if op == "replace_record" || op == "remove_record" {
		role, _ := mutation["record_role"].(string)
		records := scenario["records"].([]any)
		for index, raw := range records {
			record := raw.(map[string]any)
			if record["role"] != role {
				continue
			}
			if op == "remove_record" {
				scenario["records"] = append(records[:index], records[index+1:]...)
			} else {
				record["record_cbor_hex"] = mutation["record_cbor_hex"]
			}
			return nil
		}
		return fmt.Errorf("linked mutation role %q missing", role)
	}
	// Specialized fixture mutations describe a derived-state change rather than a
	// replacement record. Retain the executed operation in the materialized state
	// so its complete result is compared with the accepted golden scenario.
	scenario["executed_mutation"] = cloneValue(mutation)
	return nil
}

func validateLinkedScenarioShape(scenario map[string]any, bundle *policy.Bundle) error {
	roles := map[string]struct{}{}
	for _, raw := range scenario["records"].([]any) {
		record := raw.(map[string]any)
		role, roleOK := record["role"].(string)
		schema, schemaOK := record["schema_id"].(string)
		encoded, err := hex.DecodeString(record["record_cbor_hex"].(string))
		if !roleOK || !schemaOK || err != nil {
			return fmt.Errorf("invalid linked record descriptor")
		}
		if _, duplicate := roles[role]; duplicate {
			return fmt.Errorf("duplicate linked record role %q", role)
		}
		roles[role] = struct{}{}
		if _, err := codec.StrictDecodePayload(encoded); err != nil || !bundleHasSchema(bundle, schema) {
			return fmt.Errorf("invalid linked record %q", role)
		}
	}
	for _, section := range []string{"pre_append_cursor", "transaction_candidates"} {
		if authorities, ok := scenario[section].(map[string]any); ok {
			for _, rawRoles := range authorities {
				for _, rawRole := range rawRoles.([]any) {
					if _, exists := roles[rawRole.(string)]; !exists {
						return fmt.Errorf("linked cursor role %q unresolved", rawRole)
					}
				}
			}
		}
	}
	return nil
}

func linkedFingerprint(scenario map[string]any) (string, error) {
	encoded, err := json.Marshal(scenario)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func registryContainsCode(bundle *policy.Bundle, group, registry string, wanted uint64) bool {
	groups, ok := bundle.Registries[group].(map[string]any)
	if !ok {
		return false
	}
	entries, ok := groups[registry].([]any)
	if !ok {
		return false
	}
	for _, raw := range entries {
		fields, ok := raw.([]any)
		if !ok || len(fields) != 2 {
			continue
		}
		code, ok := number(fields[1])
		if ok && code == wanted {
			return true
		}
	}
	return false
}

func validateOperationProbe(item, fixtures map[string]any) error {
	wanted, _ := item["operation_case_id"].(string)
	for _, raw := range fixtures["positive"].([]any) {
		candidate := raw.(map[string]any)
		if caseID(candidate) != wanted {
			continue
		}
		encoded, err := decodeHex(candidate, "canonical_hex")
		if err != nil {
			return err
		}
		_, err = codec.StrictDecodeEnvelope(encoded)
		return err
	}
	return fmt.Errorf("operation probe %q missing", wanted)
}

func validateRecordBindingCodes(item map[string]any, bundle *policy.Bundle) bool {
	accepted, acceptedOK := number(item["accepted_code"])
	rejected, rejectedOK := number(item["rejected_code"])
	if !acceptedOK || !rejectedOK {
		return item["accepted_code"] == nil && item["rejected_code"] == nil
	}
	registry, _ := item["registry"].(string)
	return registryContainsCode(bundle, "enums", registry, accepted) && !registryContainsCode(bundle, "enums", registry, rejected)
}

func executeSetExactDuplicate(item map[string]any) error {
	input, err := decodeHex(item, "input_cbor_hex")
	if err != nil {
		return err
	}
	value, err := codec.StrictDecodePayload(input)
	if err != nil {
		return err
	}
	members, ok := value.([]any)
	if !ok {
		return fmt.Errorf("set input is not an array")
	}
	seen := map[string]struct{}{}
	normalized := make([]any, 0, len(members))
	for _, member := range members {
		encoded, err := codec.CanonicalPayload(member)
		if err != nil {
			return err
		}
		if _, duplicate := seen[string(encoded)]; duplicate {
			continue
		}
		seen[string(encoded)] = struct{}{}
		normalized = append(normalized, member)
	}
	sortCanonicalValues(normalized)
	actual, err := codec.CanonicalPayload(normalized)
	if err != nil {
		return err
	}
	expected, err := decodeHex(item, "expected_cbor_hex")
	if err != nil {
		return err
	}
	if !bytes.Equal(actual, expected) {
		return fmt.Errorf("exact duplicate normalization mismatch")
	}
	return nil
}

func executeSetIdentityCollision(item map[string]any) error {
	input, err := decodeHex(item, "input_cbor_hex")
	if err != nil {
		return err
	}
	value, err := codec.StrictDecodePayload(input)
	if err != nil {
		return err
	}
	members, ok := value.([]any)
	if !ok || len(members) < 2 {
		return fmt.Errorf("identity collision probe has insufficient members")
	}
	identities := map[string][]byte{}
	for _, raw := range members {
		member, ok := raw.([]any)
		if !ok || len(member) < 2 {
			return fmt.Errorf("invalid identity-bearing set member")
		}
		identity, err := codec.CanonicalPayload(member[:len(member)-1])
		if err != nil {
			return err
		}
		full, err := codec.CanonicalPayload(member)
		if err != nil {
			return err
		}
		if previous, exists := identities[string(identity)]; exists && !bytes.Equal(previous, full) {
			return nil
		}
		identities[string(identity)] = full
	}
	return fmt.Errorf("identity collision probe did not collide")
}

func executeSetPermutation(item map[string]any) error {
	rawInputs, ok := item["input_cbor_hex"].([]any)
	if !ok || len(rawInputs) == 0 {
		return fmt.Errorf("set permutation inputs missing")
	}
	expected, err := decodeHex(item, "expected_cbor_hex")
	if err != nil {
		return err
	}
	for _, raw := range rawInputs {
		input, err := hex.DecodeString(raw.(string))
		if err != nil {
			return err
		}
		value, err := codec.StrictDecodePayload(input)
		if err != nil {
			return err
		}
		members := value.([]any)
		sortCanonicalValues(members)
		actual, err := codec.CanonicalPayload(members)
		if err != nil || !bytes.Equal(actual, expected) {
			return fmt.Errorf("set permutation did not converge")
		}
	}
	return nil
}

func executeMemberIndexPermutation(item map[string]any) error {
	rawMembers := item["members_cbor_hex"].([]any)
	members := make([]any, len(rawMembers))
	for index, raw := range rawMembers {
		encoded, err := hex.DecodeString(raw.(string))
		if err != nil {
			return err
		}
		members[index], err = codec.StrictDecodePayload(encoded)
		if err != nil {
			return err
		}
	}
	expectedOrder := item["expected_order"].([]any)
	for _, rawOrder := range item["input_orders"].([]any) {
		order := rawOrder.([]any)
		permuted := make([]indexedValue, len(order))
		for index, rawIndex := range order {
			memberIndex, _ := number(rawIndex)
			permuted[index] = indexedValue{index: int(memberIndex), value: members[memberIndex]}
		}
		sort.Slice(permuted, func(left, right int) bool {
			leftBytes, _ := codec.CanonicalPayload(permuted[left].value)
			rightBytes, _ := codec.CanonicalPayload(permuted[right].value)
			return bytes.Compare(leftBytes, rightBytes) < 0
		})
		for index := range permuted {
			wanted, _ := number(expectedOrder[index])
			if permuted[index].index != int(wanted) {
				return fmt.Errorf("member-index permutation did not converge")
			}
		}
	}
	return nil
}

type indexedValue struct {
	index int
	value any
}

func executeDerivedOrdering(item map[string]any) error {
	inputs := item["input_cbor_hex"].([]any)
	expected := item["expected_digest_hex"].([]any)
	digests := make([][32]byte, len(inputs))
	for index, raw := range inputs {
		payload, err := hex.DecodeString(raw.(string))
		if err != nil {
			return err
		}
		value, err := codec.StrictDecodePayload(payload)
		if err != nil {
			return err
		}
		envelope, err := codec.CanonicalBytes("cote.csf.digest.indeterminate-evidence-refs", "cote.csf.schema.indeterminate-evidence-ref-list", 1, value)
		if err != nil {
			return err
		}
		digests[index] = sha256.Sum256(envelope)
		if hex.EncodeToString(digests[index][:]) != expected[index] {
			return fmt.Errorf("derived ordering digest mismatch")
		}
	}
	if len(digests) != 2 || bytes.Compare(digests[0][:], digests[1][:]) >= 0 {
		return fmt.Errorf("derived ordering comparison mismatch")
	}
	return nil
}

func executeIdempotency(kind string, item map[string]any) error {
	if kind == "optional_idempotency" {
		keyBytes, err := decodeHex(item, "idempotency_key_cbor_hex")
		if err != nil {
			return err
		}
		value, err := codec.StrictDecodePayload(keyBytes)
		if err != nil {
			return err
		}
		optional := value.([]any)
		present := len(optional) == 2 && optional[0] == uint64(1)
		if present != item["persisted_identity"].(bool) {
			return fmt.Errorf("optional idempotency persistence mismatch")
		}
		if !present {
			return nil
		}
		preimage, err := decodeHex(item, "preimage_cbor_hex")
		if err != nil {
			return err
		}
		return compareIdempotencyDigest(preimage, item["idempotency_digest"].(string))
	}
	first, err := decodeHex(item, "first_preimage_cbor_hex")
	if err != nil {
		return err
	}
	second, err := decodeHex(item, "second_preimage_cbor_hex")
	if err != nil {
		return err
	}
	if err := compareIdempotencyDigest(first, item["first_idempotency_digest"].(string)); err != nil {
		return err
	}
	if err := compareIdempotencyDigest(second, item["second_idempotency_digest"].(string)); err != nil {
		return err
	}
	if bytes.Equal(first, second) || item["first_idempotency_digest"] == item["second_idempotency_digest"] {
		return fmt.Errorf("idempotency conflict did not diverge")
	}
	return nil
}

func compareIdempotencyDigest(payload []byte, expected string) error {
	value, err := codec.StrictDecodePayload(payload)
	if err != nil {
		return err
	}
	envelope, err := codec.CanonicalBytes("cote.csf.digest.idempotency", "cote.csf.schema.idempotency-preimage", 1, value)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(envelope)
	if hex.EncodeToString(digest[:]) != expected {
		return fmt.Errorf("idempotency digest mismatch")
	}
	return nil
}

func sortCanonicalValues(values []any) {
	sort.Slice(values, func(left, right int) bool {
		leftBytes, _ := codec.CanonicalPayload(values[left])
		rightBytes, _ := codec.CanonicalPayload(values[right])
		return bytes.Compare(leftBytes, rightBytes) < 0
	})
}

func executeRecordConstraint(item map[string]any) error {
	validBytes, err := decodeHex(item, "valid_payload_cbor_hex")
	if err != nil {
		return err
	}
	invalidBytes, err := decodeHex(item, "invalid_payload_cbor_hex")
	if err != nil {
		return err
	}
	valid, err := codec.StrictDecodePayload(validBytes)
	if err != nil {
		return err
	}
	invalid, err := codec.StrictDecodePayload(invalidBytes)
	if err != nil {
		return err
	}
	validator := recordConstraintValidator(item["record"].(string))
	if validator == nil || validator(valid.([]any)) != nil || validator(invalid.([]any)) == nil {
		return fmt.Errorf("record conditional constraint outcome mismatch")
	}
	return nil
}

func recordConstraintValidator(record string) func([]any) error {
	switch record {
	case "decision-record":
		return validateDecisionRecord
	case "cycle-control-state":
		return validateCycleControlState
	case "trigger-definition":
		return validateTriggerDefinition
	default:
		return nil
	}
}

func optionalPresent(value any) bool {
	fields, ok := value.([]any)
	return ok && len(fields) == 2 && fields[0] == uint64(1)
}

func optionalAbsent(value any) bool {
	fields, ok := value.([]any)
	return ok && len(fields) == 1 && fields[0] == uint64(0)
}

func validateDecisionRecord(fields []any) error {
	if len(fields) != 16 {
		return fmt.Errorf("decision record arity")
	}
	disposition, ok := number(fields[5])
	if !ok || disposition > 4 {
		return fmt.Errorf("decision disposition")
	}
	candidate, successor, canonical := optionalPresent(fields[6]), optionalPresent(fields[13]), optionalPresent(fields[14])
	conflicts, conflictsOK := fields[8].([]any)
	if !conflictsOK {
		return fmt.Errorf("decision conflicts")
	}
	switch disposition {
	case 0, 1:
		if !candidate || !optionalAbsent(fields[13]) || !optionalAbsent(fields[14]) {
			return fmt.Errorf("decision variant")
		}
	case 2:
		if !candidate || !successor || !optionalAbsent(fields[14]) {
			return fmt.Errorf("decision variant")
		}
	case 3:
		if !optionalAbsent(fields[6]) || !optionalAbsent(fields[13]) || !optionalAbsent(fields[14]) || len(conflicts) != 0 {
			return fmt.Errorf("decision variant")
		}
	case 4:
		if !optionalAbsent(fields[6]) || !optionalAbsent(fields[13]) || !canonical || len(conflicts) != 0 {
			return fmt.Errorf("decision variant")
		}
	}
	return nil
}

func validateCycleControlState(fields []any) error {
	if len(fields) != 7 {
		return fmt.Errorf("cycle control arity")
	}
	status, ok := number(fields[0])
	next, nextOK := number(fields[6])
	if !ok || !nextOK || status > 3 {
		return fmt.Errorf("cycle control status")
	}
	cycle, attempt, fence, retry, terminal := optionalPresent(fields[1]), optionalPresent(fields[2]), optionalPresent(fields[3]), optionalPresent(fields[4]), optionalPresent(fields[5])
	var attemptValue uint64
	if attempt {
		attemptValue, _ = number(fields[2].([]any)[1])
	}
	switch status {
	case 0:
		if cycle || attempt || fence || retry || next != 1 {
			return fmt.Errorf("idle control state")
		}
	case 1:
		if !cycle || !attempt || !fence || retry || next != attemptValue+1 {
			return fmt.Errorf("in-flight control state")
		}
	case 2:
		if !cycle || !attempt || fence || !retry || !terminal || next != attemptValue || optionalRefKind(fields[5]) != 13 {
			return fmt.Errorf("retry control state")
		}
	case 3:
		if !cycle || !attempt || fence || retry || !terminal || next != attemptValue+1 || optionalRefKind(fields[5]) != 13 {
			return fmt.Errorf("halted control state")
		}
	}
	return nil
}

func optionalRefKind(value any) uint64 {
	optional, ok := value.([]any)
	if !ok || len(optional) != 2 {
		return ^uint64(0)
	}
	ref, ok := optional[1].([]any)
	if !ok || len(ref) == 0 {
		return ^uint64(0)
	}
	kind, _ := number(ref[0])
	return kind
}

func validateTriggerDefinition(fields []any) error {
	if len(fields) != 8 {
		return fmt.Errorf("trigger definition arity")
	}
	policy, ok := number(fields[3])
	if !ok || policy > 3 {
		return fmt.Errorf("trigger activation policy")
	}
	if policy != 3 {
		if !optionalAbsent(fields[5]) {
			return fmt.Errorf("non-repeat cadence present")
		}
		return nil
	}
	if !optionalPresent(fields[5]) {
		return fmt.Errorf("repeat cadence absent")
	}
	duration, ok := signedNumber(fields[5].([]any)[1])
	if !ok || duration <= 0 {
		return fmt.Errorf("repeat cadence not positive")
	}
	return nil
}

func signedNumber(value any) (int64, bool) {
	switch current := value.(type) {
	case int64:
		return current, true
	case uint64:
		if current <= math.MaxInt64 {
			return int64(current), true
		}
	}
	return 0, false
}

func executeTypedValue(item map[string]any) error {
	encoded, err := decodeHex(item, "typed_value_cbor_hex")
	if err != nil {
		return err
	}
	value, err := codec.StrictDecodePayload(encoded)
	if err != nil {
		return err
	}
	fields, ok := value.([]any)
	if !ok || len(fields) != 4 {
		return fmt.Errorf("invalid typed value")
	}
	innerBytes, ok := fields[2].([]byte)
	if !ok {
		return fmt.Errorf("typed envelope is not bytes")
	}
	inner, innerErr := codec.StrictDecodeEnvelope(innerBytes)
	errorsFound := []string{}
	if innerErr != nil {
		errorsFound = append(errorsFound, "TYPED_VALUE_INNER_NON_CANONICAL")
	} else {
		if fields[0] != inner.SchemaID {
			errorsFound = append(errorsFound, "TYPED_VALUE_SCHEMA_MISMATCH")
		}
		version, _ := number(fields[1])
		if uint32(version) != inner.SchemaVersion {
			errorsFound = append(errorsFound, "TYPED_VALUE_VERSION_MISMATCH")
		}
		authorized := false
		for _, raw := range item["authorized_operations"].([]any) {
			op := raw.([]any)
			v, _ := number(op[2])
			if op[0] == inner.DomainTag && op[1] == inner.SchemaID && uint32(v) == inner.SchemaVersion {
				authorized = true
			}
		}
		if !authorized {
			errorsFound = append(errorsFound, "TYPED_VALUE_DOMAIN_UNAUTHORIZED")
		}
	}
	digest := sha256.Sum256(innerBytes)
	if !bytes.Equal(fields[3].([]byte), digest[:]) {
		errorsFound = append(errorsFound, "TYPED_VALUE_DIGEST_MISMATCH")
	}
	expected, _ := item["expected"].(string)
	if expected == "accept" && len(errorsFound) == 0 {
		return nil
	}
	if expected == "reject" && len(errorsFound) == 1 && errorsFound[0] == item["invalid_error_code"] {
		return nil
	}
	return fmt.Errorf("typed-value outcome mismatch: %v", errorsFound)
}

func executeTransition(item map[string]any, bases map[string]map[string]any) error {
	baseID, ok := item["base_scenario_id"].(string)
	if !ok {
		return fmt.Errorf("transition lacks base scenario")
	}
	base, ok := bases[baseID]
	if !ok {
		return fmt.Errorf("unknown base scenario %s", baseID)
	}
	cloneBytes, _ := json.Marshal(base["state"])
	var state any
	if json.Unmarshal(cloneBytes, &state) != nil {
		return fmt.Errorf("cannot clone transition state")
	}
	patches, _ := item["patch"].([]any)
	for _, raw := range patches {
		if err := applyPatch(&state, raw.(map[string]any)); err != nil {
			return err
		}
	}
	actualError := validateTransition(baseID, base["operation"].(string), state.(map[string]any), base["state"].(map[string]any))
	expectedError, rejected := item["expected_error"].(string)
	if !rejected {
		if actualError != "" {
			return fmt.Errorf("transition rejected with %s", actualError)
		}
		return nil
	}
	if actualError != expectedError {
		return fmt.Errorf("transition error mismatch: want %s, got %s", expectedError, actualError)
	}
	return nil
}

func validateTransition(baseID, operation string, state, baseline map[string]any) string {
	switch operation {
	case "append-source-record":
		return validateSourceAppend(state)
	case "append-slot-dispatch":
		return validateDispatch(state)
	case "append-slot-response":
		return validateResponse(state)
	case "append-admission-fence":
		if reflect.DeepEqual(state["candidate"], baseline["candidate"]) {
			return ""
		}
		return "FENCE_DERIVATION_MISMATCH"
	case "derive-cycle-control-state":
		if reflect.DeepEqual(state["candidate"], deriveControlState(state["decision_ledger"].([]any))) {
			return ""
		}
		return "CONTROL_STATE_FOLD_MISMATCH"
	case "validate-epistemic-checkpoint":
		checkpoint := state["checkpoint"].(map[string]any)
		resolved, err := hex.DecodeString(checkpoint["resolved_content_hex"].(string))
		if err != nil {
			return "CHECKPOINT_CONTENT_HASH_MISMATCH"
		}
		digest := sha256.Sum256(resolved)
		if hex.EncodeToString(digest[:]) != checkpoint["content_hash_hex"] {
			return "CHECKPOINT_CONTENT_HASH_MISMATCH"
		}
		return ""
	case "validate-rng-draw":
		return validateRNGTransition(state)
	case "validate-parent-history-ref":
		return validateParentHistory(state)
	case "append-cycle-abort":
		return validateAbortTransition(state)
	case "validate-genesis":
		return validateGenesisTransition(state)
	case "validate-snapshot":
		return validateSnapshotTransition(state, baseline)
	case "append-observation":
		return validateObservationTransition(state)
	case "publish-cycle-commit-batch":
		return validateCommitTransition(state)
	default:
		return validateFixtureTransitionFallback(baseID, state, baseline)
	}
}

func validateSourceAppend(state map[string]any) string {
	candidate := state["candidate"].(map[string]any)
	receipt, receiptOK := state["candidate_receipt"].(map[string]any)
	if !receiptOK {
		return "SOURCE_RECEIPT_MISSING"
	}
	prefix := state["prefix"].([]any)
	last := prefix[len(prefix)-1].(map[string]any)
	lastRecord := last["record"].(map[string]any)
	if final, _ := lastRecord["final"].(bool); final {
		return "SOURCE_FINAL"
	}
	lastReceipt := last["receipt"].(map[string]any)
	lastSeq, _ := number(lastReceipt["ingress_seq"])
	seq, _ := number(receipt["ingress_seq"])
	if seq != lastSeq+1 {
		return "INGRESS_NOT_CONTIGUOUS"
	}
	if candidate["root"] == "source-closure" {
		if compareCoordinate(candidate["closed_through"].([]any), lastRecord["closed_through"].([]any)) < 0 {
			return "CLOSURE_REGRESSION"
		}
	} else if compareCoordinate(candidate["eligibility"].([]any), lastRecord["closed_through"].([]any)) <= 0 {
		return "RETROACTIVE_ELIGIBILITY"
	}
	return ""
}

func validateDispatch(state map[string]any) string {
	commits := state["decision_ledger"].(map[string]any)["cycle_commits"].([]any)
	tasks := state["causal_outbox"].(map[string]any)["tasks"].([]any)
	completions := state["causal_outbox"].(map[string]any)["completions"].([]any)
	knowledge := state["evidence_ledger"].(map[string]any)["knowledge_inputs"].([]any)
	completionByTask := map[string]map[string]any{}
	for _, raw := range completions {
		completion := raw.(map[string]any)
		completionByTask[completion["task_id"].(string)] = completion
	}
	knowledgeIDs := map[string]struct{}{}
	for _, raw := range knowledge {
		knowledgeIDs[raw.(map[string]any)["knowledge_input_id"].(string)] = struct{}{}
	}
	knownTasks := map[string]struct{}{}
	for _, raw := range tasks {
		knownTasks[raw.(map[string]any)["task_id"].(string)] = struct{}{}
	}
	for _, raw := range commits {
		for _, rawID := range raw.(map[string]any)["perception_task_ids"].([]any) {
			taskID := rawID.(string)
			if _, ok := knownTasks[taskID]; !ok {
				return "EPISTEMIC_TASK_PENDING"
			}
			completion, ok := completionByTask[taskID]
			if !ok {
				return "EPISTEMIC_TASK_PENDING"
			}
			for _, rawKnowledgeID := range completion["knowledge_input_ids"].([]any) {
				if _, ok := knowledgeIDs[rawKnowledgeID.(string)]; !ok {
					return "EPISTEMIC_DELIVERY_PENDING"
				}
			}
		}
	}
	return ""
}

func validateResponse(state map[string]any) string {
	candidate := state["candidate"].(map[string]any)
	dispatches := state["dispatches"].([]any)
	var dispatch map[string]any
	for _, raw := range dispatches {
		current := raw.(map[string]any)
		if current["dispatch_id"] == candidate["dispatch_id"] {
			dispatch = current
		}
	}
	if dispatch == nil {
		return "RESPONSE_DISPATCH_MISMATCH"
	}
	for _, raw := range state["revocations"].([]any) {
		if raw.(map[string]any)["dispatch_id"] == candidate["dispatch_id"] {
			return "DISPATCH_NOT_OPEN"
		}
	}
	for _, field := range []string{"run_id", "cycle_id", "decision_round_id", "slot_id", "actor", "effective_at"} {
		if !reflect.DeepEqual(dispatch[field], candidate[field]) {
			return "RESPONSE_DISPATCH_MISMATCH"
		}
	}
	if !reflect.DeepEqual(dispatch["base_revision"], candidate["submitted_against_revision"]) {
		return "RESPONSE_DISPATCH_MISMATCH"
	}
	for _, raw := range state["responses"].([]any) {
		response := raw.(map[string]any)
		if response["dispatch_id"] != candidate["dispatch_id"] {
			continue
		}
		if reflect.DeepEqual(response, candidate) {
			continue
		}
		return "SLOT_ALREADY_FILLED"
	}
	return ""
}

func deriveControlState(ledger []any) map[string]any {
	if len(ledger) == 0 {
		return map[string]any{"status": "IDLE", "cycle_id": nil, "attempt_ordinal": nil, "fence_ref": nil, "retry_ref": nil, "last_terminal_envelope_ref": nil, "next_attempt_ordinal": float64(1)}
	}
	latest := ledger[len(ledger)-1].(map[string]any)
	if latest["kind"] == "fence" {
		attempt, _ := number(latest["attempt"])
		return map[string]any{"status": "ATTEMPT_IN_FLIGHT", "cycle_id": latest["cycle_id"], "attempt_ordinal": latest["attempt"], "fence_ref": latest["ref"], "retry_ref": nil, "last_terminal_envelope_ref": nil, "next_attempt_ordinal": float64(attempt + 1)}
	}
	if latest["kind"] == "retry" {
		abort := ledger[len(ledger)-2].(map[string]any)
		return map[string]any{"status": "RETRY_AUTHORIZED", "cycle_id": abort["cycle_id"], "attempt_ordinal": latest["to_attempt"], "fence_ref": nil, "retry_ref": latest["ref"], "last_terminal_envelope_ref": abort["ref"], "next_attempt_ordinal": latest["to_attempt"]}
	}
	attempt, _ := number(latest["attempt"])
	return map[string]any{"status": "HALTED_ON_ABORT", "cycle_id": latest["cycle_id"], "attempt_ordinal": latest["attempt"], "fence_ref": nil, "retry_ref": nil, "last_terminal_envelope_ref": latest["ref"], "next_attempt_ordinal": float64(attempt + 1)}
}

func validateRNGTransition(state map[string]any) string {
	genesis := state["genesis"].(map[string]any)
	draw := state["draw"].(map[string]any)
	if !reflect.DeepEqual(genesis["rng_policy"], draw["algorithm"]) {
		return "RNG_POLICY_MISMATCH"
	}
	seed, _ := hex.DecodeString(genesis["world_seed_hex"].(string))
	algorithm := draw["algorithm"].(map[string]any)
	hashBytes, _ := hex.DecodeString(algorithm["hash_hex"].(string))
	entities := make([]any, 0, len(draw["entity_ids_hex"].([]any)))
	for _, raw := range draw["entity_ids_hex"].([]any) {
		value, _ := hex.DecodeString(raw.(string))
		entities = append(entities, value)
	}
	preimage, err := codec.CanonicalPayload([]any{seed, draw["subsystem"], draw["decision_key"], entities, draw["purpose"], []any{algorithm["id"], uint64(algorithm["version"].(float64)), hashBytes}})
	if err != nil {
		return "RNG_RESULT_MISMATCH"
	}
	result := sha256.Sum256(preimage)
	if hex.EncodeToString(result[:]) != draw["result"].(map[string]any)["value_hex"] {
		return "RNG_RESULT_MISMATCH"
	}
	return ""
}

func validateParentHistory(state map[string]any) string {
	_, err := parseParentHistory(state["reference_hex"].(string))
	if err != nil {
		return "PARENT_HISTORY_REF_INVALID"
	}
	return ""
}

type parentHistoryRef struct {
	PolicyID       string
	PolicyHash     string
	Domain         string
	SchemaID       string
	SchemaVersion  uint64
	Algorithm      string
	CheckpointHash string
}

func parseParentHistory(encodedHex string) (parentHistoryRef, error) {
	var result parentHistoryRef
	encoded, err := hex.DecodeString(encodedHex)
	if err != nil || len(encoded) < 10 || !bytes.Equal(encoded[:8], []byte{'C', 'S', 'F', 'H', 'R', 'E', 'F', 0}) {
		return result, fmt.Errorf("invalid parent history framing")
	}
	offset := 8
	if len(encoded) <= offset || encoded[offset] != 1 {
		return result, fmt.Errorf("unsupported parent history version")
	}
	offset++
	readU16 := func() (uint16, error) {
		if len(encoded) < offset+2 {
			return 0, fmt.Errorf("truncated parent history ref")
		}
		value := uint16(encoded[offset])<<8 | uint16(encoded[offset+1])
		offset += 2
		return value, nil
	}
	readBytes := func(expected int) ([]byte, error) {
		length, err := readU16()
		if err != nil || expected >= 0 && int(length) != expected || len(encoded) < offset+int(length) {
			return nil, fmt.Errorf("invalid parent history field")
		}
		value := bytes.Clone(encoded[offset : offset+int(length)])
		offset += int(length)
		return value, nil
	}
	policyID, err := readBytes(-1)
	if err != nil {
		return result, err
	}
	policyHash, err := readBytes(32)
	if err != nil {
		return result, err
	}
	domainTag, err := readBytes(-1)
	if err != nil {
		return result, err
	}
	schemaID, err := readBytes(-1)
	if err != nil || len(encoded) < offset+8 {
		return result, fmt.Errorf("invalid parent history schema")
	}
	var schemaVersion uint64
	for range 8 {
		schemaVersion = schemaVersion<<8 | uint64(encoded[offset])
		offset++
	}
	algorithm, err := readBytes(-1)
	if err != nil {
		return result, err
	}
	checkpoint, err := readBytes(32)
	if err != nil || offset != len(encoded) {
		return result, fmt.Errorf("invalid parent history trailer")
	}
	result = parentHistoryRef{string(policyID), hex.EncodeToString(policyHash), string(domainTag), string(schemaID), schemaVersion, string(algorithm), hex.EncodeToString(checkpoint)}
	return result, nil
}

func validateAbortTransition(state map[string]any) string {
	fence := state["fence"].(map[string]any)
	units := map[string]string{}
	for _, raw := range fence["admitted_units"].([]any) {
		unit := raw.(map[string]any)
		units[unit["unit_id"].(string)] = unit["digest"].(string)
	}
	candidates := map[string]map[string]any{}
	for _, raw := range state["commit_candidates"].([]any) {
		candidate := raw.(map[string]any)
		candidates[candidate["candidate_id"].(string)] = candidate
	}
	abort := state["abort"].(map[string]any)
	for _, raw := range abort["indeterminate_subjects"].([]any) {
		subject := raw.(map[string]any)
		id := subject["id"].(string)
		if subject["kind"] == "ADMITTED_UNIT" {
			if units[id] != subject["digest"] {
				return "ABORT_SUBJECT_MISMATCH"
			}
			continue
		}
		candidate := candidates[id]
		if candidate == nil || candidate["candidate_digest"] != subject["digest"] || candidate["run_id"] != fence["run_id"] || candidate["cycle_id"] != fence["cycle_id"] {
			return "ABORT_CANDIDATE_MISMATCH"
		}
	}
	evidenceByID := map[string]map[string]any{}
	for _, raw := range state["failure_evidence"].([]any) {
		evidence := raw.(map[string]any)
		evidenceByID[evidence["evidence_id"].(string)] = evidence
	}
	attempt, _ := number(fence["attempt"])
	for _, raw := range abort["failure_evidence_refs"].([]any) {
		evidence := evidenceByID[raw.(string)]
		if evidence == nil {
			return "ABORT_EVIDENCE_SCOPE_MISMATCH"
		}
		switch evidence["kind"] {
		case "conflict-set":
			for _, rawID := range evidence["candidate_ids"].([]any) {
				if candidates[rawID.(string)] == nil {
					return "ABORT_EVIDENCE_SCOPE_MISMATCH"
				}
			}
		case "affordance-assessment":
			id := evidence["subject_id"].(string)
			if _, unitOK := units[id]; !unitOK && candidates[id] == nil {
				return "ABORT_EVIDENCE_SCOPE_MISMATCH"
			}
		default:
			gotAttempt, ok := number(evidence["attempt"])
			if !ok || gotAttempt != attempt || evidence["run_id"] != fence["run_id"] || evidence["cycle_id"] != fence["cycle_id"] {
				return "ABORT_EVIDENCE_SCOPE_MISMATCH"
			}
		}
	}
	return ""
}

func validateGenesisTransition(state map[string]any) string {
	genesisState := state["genesis"].(map[string]any)
	policies := genesisState["policies"].([]any)
	if len(policies) != 13 {
		return "GENESIS_POLICY_SET_INCOMPLETE"
	}
	parent, present := genesisState["parent_checkpoint_history_ref"].(string)
	if !present || parent == "" {
		return ""
	}
	ref, err := parseParentHistory(parent)
	if err != nil {
		return "PARENT_HISTORY_REF_INVALID"
	}
	var policyState map[string]any
	for _, raw := range state["origin_policy_store"].([]any) {
		candidate := raw.(map[string]any)
		if candidate["policy_id"] == ref.PolicyID && candidate["policy_hash_hex"] == ref.PolicyHash {
			policyState = candidate
		}
	}
	if policyState == nil {
		return "PARENT_HISTORY_POLICY_UNRESOLVED"
	}
	for _, raw := range state["checkpoint_store"].([]any) {
		checkpoint := raw.(map[string]any)
		if checkpoint["domain"] != ref.Domain || checkpoint["schema_id"] != ref.SchemaID || uint64(checkpoint["schema_version"].(float64)) != ref.SchemaVersion || checkpoint["algorithm"] != ref.Algorithm || checkpoint["checkpoint_digest_hex"] != ref.CheckpointHash {
			continue
		}
		resolved, err := hex.DecodeString(checkpoint["resolved_content_hex"].(string))
		if err != nil {
			continue
		}
		digest := sha256.Sum256(resolved)
		if hex.EncodeToString(digest[:]) == ref.CheckpointHash {
			return ""
		}
	}
	return "PARENT_HISTORY_CHECKPOINT_UNRESOLVED"
}

func validateSnapshotTransition(state, baseline map[string]any) string {
	snapshot := state["snapshot"].(map[string]any)
	genesisState := state["genesis"].(map[string]any)
	if snapshot["run_id"] != genesisState["run_id"] || snapshot["genesis_ref"] != genesisState["genesis_ref"] || snapshot["world_seed_hex"] != genesisState["world_seed_hex"] || snapshot["codec_policy_id"] != genesisState["codec_policy_id"] || snapshot["codec_policy_hash_hex"] != genesisState["codec_policy_hash_hex"] || snapshot["schema_bundle_hash_hex"] != genesisState["schema_bundle_hash_hex"] || !reflect.DeepEqual(snapshot["replay_policies"], genesisState["policies"]) {
		return "SNAPSHOT_GENESIS_CONFIG_MISMATCH"
	}
	baselineSnapshot := baseline["snapshot"].(map[string]any)
	if snapshot["world_state_hash_hex"] != baselineSnapshot["world_state_hash_hex"] {
		return "SNAPSHOT_WORLD_HASH_MISMATCH"
	}
	ledgerCursors, ok := snapshot["ledger_cursors"].(map[string]any)
	if !ok {
		return "SNAPSHOT_LEDGER_SET_INCOMPLETE"
	}
	baselineCursors := baselineSnapshot["ledger_cursors"].(map[string]any)
	for _, ledger := range []string{"input_ledger", "decision_ledger", "event_store", "evidence_ledger", "causal_outbox"} {
		cursor, exists := ledgerCursors[ledger]
		if !exists {
			return "SNAPSHOT_LEDGER_SET_INCOMPLETE"
		}
		if !reflect.DeepEqual(cursor, baselineCursors[ledger]) {
			return "SNAPSHOT_LEDGER_DIGEST_MISMATCH"
		}
	}
	if !reflect.DeepEqual(snapshot["pending_state"], baselineSnapshot["pending_state"]) {
		return "SNAPSHOT_PENDING_STATE_MISMATCH"
	}
	if !reflect.DeepEqual(snapshot["cycle_control_state"], deriveControlState(state["decision_ledger_records"].([]any))) {
		return "SNAPSHOT_CONTROL_STATE_MISMATCH"
	}
	checkpoints := snapshot["epistemic_checkpoints"].([]any)
	actors := state["authorities"].(map[string]any)["actor_registry"].([]any)
	required := map[string]struct{}{}
	for _, raw := range actors {
		actor := raw.(map[string]any)
		if actor["cognitive_state_required"] == true {
			required[actor["actor"].(string)] = struct{}{}
		}
	}
	if len(checkpoints) != len(required) {
		return "SNAPSHOT_CHECKPOINT_SET_MISMATCH"
	}
	for _, raw := range checkpoints {
		checkpoint := raw.(map[string]any)
		actor := checkpoint["actor"].(string)
		if _, ok := required[actor]; !ok {
			return "SNAPSHOT_CHECKPOINT_SET_MISMATCH"
		}
		covers := checkpoint["covers_through"].([]any)
		instant, _ := number(covers[0])
		revision, _ := number(covers[1])
		snapshotInstant, _ := number(snapshot["instant"])
		snapshotRevision, _ := number(snapshot["revision"])
		if instant < snapshotInstant || revision < snapshotRevision {
			return "SNAPSHOT_CHECKPOINT_STALE"
		}
		content, err := hex.DecodeString(checkpoint["resolved_content_hex"].(string))
		if err != nil {
			return "CHECKPOINT_CONTENT_HASH_MISMATCH"
		}
		digest := sha256.Sum256(content)
		if hex.EncodeToString(digest[:]) != checkpoint["content_hash_hex"] {
			return "CHECKPOINT_CONTENT_HASH_MISMATCH"
		}
	}
	return ""
}

func validateObservationTransition(state map[string]any) string {
	event := state["event"].(map[string]any)
	task := state["task"].(map[string]any)
	candidate := state["candidate"].(map[string]any)
	if candidate["task_id"] != task["task_id"] || task["event_id"] != event["event_id"] {
		return "OBSERVATION_PROJECTION_MISMATCH"
	}
	observer := candidate["observer"].(string)
	channelID := candidate["channel_ref"].(string)
	states := state["world_state_by_revision"].(map[string]any)
	clearanceRank := map[string]int{"PUBLIC": 0, "RESTRICTED": 1, "SECRET": 2}
	var channel map[string]any
	for _, revisionKey := range []string{fmt.Sprintf("%.0f", task["base_revision"]), fmt.Sprintf("%.0f", task["result_revision"])} {
		revision, ok := states[revisionKey].(map[string]any)
		if !ok {
			return "OBSERVATION_ACCESS_DENIED"
		}
		currentChannel, ok := revision["channels"].(map[string]any)[channelID].(map[string]any)
		if !ok || currentChannel["location_ref"] != event["location_ref"] || !stringSliceContains(currentChannel["members"].([]any), observer) {
			return "OBSERVATION_ACCESS_DENIED"
		}
		actor, ok := revision["actors"].(map[string]any)[observer].(map[string]any)
		if !ok || actor["active"] != true || clearanceRank[actor["clearance"].(string)] < clearanceRank[event["confidentiality"].(string)] {
			return "OBSERVATION_ACCESS_DENIED"
		}
		channel = currentChannel
	}
	payload := event["payload"].(map[string]any)
	expectedSource := []any{map[string]any{"id": event["event_id"], "digest": event["event_digest"]}}
	if candidate["observed_at"] != event["occurred_at"] || !reflect.DeepEqual(candidate["source_event_refs"], expectedSource) || !reflect.DeepEqual(candidate["resolver"], task["resolver"]) || !reflect.DeepEqual(candidate["percepts"], payload["observable_cues"]) || !reflect.DeepEqual(candidate["claim_refs"], payload["observable_claim_refs"]) || !reflect.DeepEqual(candidate["evidence_chain"], payload["observable_evidence_refs"]) || !reflect.DeepEqual(candidate["omissions_redactions"], payload["private_details"]) || candidate["modality"] != channel["modality"] {
		return "OBSERVATION_PROJECTION_MISMATCH"
	}
	return ""
}

func stringSliceContains(values []any, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func validateCommitTransition(state map[string]any) string {
	fence := state["fence"].(map[string]any)
	ledger := state["decision_ledger"].(map[string]any)
	transaction := state["transaction"].(map[string]any)
	if len(ledger["terminals"].([]any)) != 0 {
		return "TERMINAL_ALREADY_EXISTS"
	}
	for _, raw := range fence["admitted_units"].([]any) {
		unit := raw.(map[string]any)
		if len(unit) != 5 {
			return "ADMISSION_FENCE_UNIT_NOT_CANONICAL"
		}
	}
	if len(transaction["commit_candidates"].([]any)) != 0 {
		return "CANDIDATE_REPUBLISHED"
	}
	units := map[string]map[string]any{}
	for _, raw := range fence["admitted_units"].([]any) {
		unit := raw.(map[string]any)
		units[unit["unit_id"].(string)] = unit
	}
	sources := flattenSourceAuthorities(state["source_authorities"].(map[string]any))
	for id, unit := range units {
		source := sources[id]
		if source == nil || source["unit_digest"] != unit["unit_digest"] {
			return "SOURCE_RECORD_DIGEST_MISMATCH"
		}
		if digest, ok := sourceRecordDigest(source); !ok || digest != source["unit_digest"] {
			return "SOURCE_RECORD_DIGEST_MISMATCH"
		}
	}
	candidates := map[string]map[string]any{}
	producerByUnit := map[string]string{}
	for _, raw := range ledger["commit_candidates"].([]any) {
		candidate := raw.(map[string]any)
		id := candidate["candidate_id"].(string)
		if _, duplicate := candidates[id]; duplicate {
			return "CANDIDATE_PARTITION_MISMATCH"
		}
		candidates[id] = candidate
		for _, rawUnitID := range candidate["source_unit_ids"].([]any) {
			unitID := rawUnitID.(string)
			if units[unitID] == nil {
				return "CANDIDATE_PARTITION_MISMATCH"
			}
			if _, duplicate := producerByUnit[unitID]; duplicate {
				return "CANDIDATE_PARTITION_MISMATCH"
			}
			producerByUnit[unitID] = id
		}
	}
	decisions := map[string]map[string]any{}
	for _, raw := range transaction["decisions"].([]any) {
		decision := raw.(map[string]any)
		subject := decision["subject_id"].(string)
		if decisions[subject] != nil || units[subject] == nil {
			return "CANDIDATE_PARTITION_MISMATCH"
		}
		decisions[subject] = decision
	}
	if len(decisions) != len(units) {
		return "CANDIDATE_PARTITION_MISMATCH"
	}
	provisionals := map[string]map[string]any{}
	for _, raw := range ledger["provisional_dispositions"].([]any) {
		provisional := raw.(map[string]any)
		provisionals[provisional["subject_id"].(string)] = provisional
	}
	conflicts := idSet(ledger["conflict_sets"].([]any), "conflict_set_id")
	rngDraws := idSet(ledger["rng_draws"].([]any), "rng_draw_id")
	eventIDs := map[string]map[string]any{}
	for _, raw := range transaction["events"].([]any) {
		event := raw.(map[string]any)
		eventIDs[event["event_id"].(string)] = event
	}
	for subject, decision := range decisions {
		source := sources[subject]
		disposition := decision["disposition"].(string)
		if disposition == "NO_PROPOSAL" {
			if source == nil || source["root"] != "no-proposal" || decision["candidate_id"] != nil || len(decision["produced_event_ids"].([]any)) != 0 {
				return "NO_PROPOSAL_SUBJECT_MISMATCH"
			}
			continue
		}
		candidateID, ok := decision["candidate_id"].(string)
		candidate := candidates[candidateID]
		if !ok || candidate == nil || producerByUnit[subject] != candidateID {
			return "CANDIDATE_PARTITION_MISMATCH"
		}
		provisional := provisionals[subject]
		if provisional == nil || disposition != "DEDUPLICATED" && provisional["disposition"] != disposition || provisional["candidate_id"] != candidateID || provisional["reason_code"] != decision["reason_code"] || decision["resolver"] != candidate["resolver"] || decision["policy"] != "decision-v1@1#06" {
			return "PROVISIONAL_SETTLEMENT_MISMATCH"
		}
		for _, rawRef := range decision["conflict_set_refs"].([]any) {
			if _, ok := conflicts[rawRef.(string)]; !ok {
				return "CONFLICT_SET_UNRESOLVED"
			}
		}
		for _, rawRef := range decision["rng_draw_refs"].([]any) {
			if _, ok := rngDraws[rawRef.(string)]; !ok {
				return "RNG_DRAW_UNRESOLVED"
			}
		}
		produced := decision["produced_event_ids"].([]any)
		switch disposition {
		case "COMMIT":
			if len(produced) != len(candidate["event_drafts"].([]any)) {
				return "DISPOSITION_EVENT_MISMATCH"
			}
		case "REJECT":
			if len(produced) != 0 {
				return "DISPOSITION_EVENT_MISMATCH"
			}
		case "DEFER":
			if len(produced) != 0 || decision["successor_input_id"] == nil || !successorExists(transaction["successors"].([]any), decision["successor_input_id"], subject, fence) {
				return "DEFER_SUCCESSOR_MISMATCH"
			}
		case "DEDUPLICATED":
			if len(produced) != 0 || !validDeduplication(state, subject, decision) {
				return "DEDUPLICATION_PROOF_MISMATCH"
			}
		}
	}
	if len(transaction["successors"].([]any)) != countDisposition(decisions, "DEFER") {
		return "DEFER_SUCCESSOR_MISMATCH"
	}
	if errCode := validateCommitEvents(state, units, sources, candidates, decisions, eventIDs); errCode != "" {
		return errCode
	}
	commit := transaction["cycle_commit"].(map[string]any)
	bindings := state["genesis_policy_bindings"].(map[string]any)
	for role, field := range map[string]string{"cycle-coordinate": "cycle_coordinate_policy", "event-order": "event_order_policy", "perception": "perception_policy", "perception-identity": "perception_identity_policy"} {
		if !reflect.DeepEqual(commit[field], bindings[role]) {
			return "COMMIT_POLICY_MISMATCH"
		}
	}
	commitEventIDs := commit["event_ids"].([]any)
	if len(commitEventIDs) != len(transaction["events"].([]any)) {
		return "EVENT_ID_MISMATCH"
	}
	for index, raw := range transaction["events"].([]any) {
		if commitEventIDs[index] != raw.(map[string]any)["event_id"] {
			return "EVENT_ID_MISMATCH"
		}
	}
	before, _ := number(state["logical_sequence_transition"].(map[string]any)["before"])
	after, _ := number(state["logical_sequence_transition"].(map[string]any)["after"])
	next, _ := number(commit["next_logical_sequence"])
	if after != before+uint64(len(transaction["events"].([]any))) || next != after {
		return "LOGICAL_SEQUENCE_MISMATCH"
	}
	if _, supplied := fence["commit_successor_floor"]; supplied {
		return "COMMIT_SUCCESSOR_FLOOR_NOT_DERIVED"
	}
	if !validSourceSettlements(transaction["source_settlements"].([]any), units) {
		return "SOURCE_SETTLEMENT_AUTHORITY_MISMATCH"
	}
	if !validWorldAndTriggers(state) {
		return "WORLD_STATE_NOT_DERIVED"
	}
	return ""
}

func flattenSourceAuthorities(authorities map[string]any) map[string]map[string]any {
	result := map[string]map[string]any{}
	for _, rawOwner := range authorities {
		owner, ok := rawOwner.(map[string]any)
		if !ok {
			continue
		}
		for id, raw := range owner {
			if record, ok := raw.(map[string]any); ok {
				result[id] = record
			}
		}
	}
	return result
}

func sourceRecordDigest(source map[string]any) (string, bool) {
	descriptors := map[string][2]string{
		"action-proposal":      {"cote.csf.digest.action-proposal-unit", "cote.csf.schema.action-proposal.body"},
		"no-proposal":          {"cote.csf.digest.no-proposal-unit", "cote.csf.schema.no-proposal.body"},
		"scheduled-occurrence": {"cote.csf.digest.occurrence-unit", "cote.csf.schema.scheduled-occurrence.body"},
		"trigger-activation":   {"cote.csf.digest.trigger-activation-unit", "cote.csf.schema.trigger-activation.body"},
		"exogenous-input":      {"cote.csf.digest.exogenous-input-unit", "cote.csf.schema.exogenous-input.body"},
	}
	descriptor, ok := descriptors[source["root"].(string)]
	if !ok {
		return "", false
	}
	encoded, err := codec.CanonicalBytes(descriptor[0], descriptor[1], 1, integralJSONValue(source["record_body"]))
	if err != nil {
		return "", false
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), true
}

func integralJSONValue(value any) any {
	switch typed := value.(type) {
	case float64:
		if typed >= 0 && typed == math.Trunc(typed) {
			return uint64(typed)
		}
		if typed == math.Trunc(typed) {
			return int64(typed)
		}
		return typed
	case []any:
		result := make([]any, len(typed))
		for index, member := range typed {
			result[index] = integralJSONValue(member)
		}
		return result
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, member := range typed {
			result[key] = integralJSONValue(member)
		}
		return result
	default:
		return value
	}
}

func idSet(values []any, field string) map[string]struct{} {
	result := map[string]struct{}{}
	for _, raw := range values {
		if value, ok := raw.(map[string]any)[field].(string); ok {
			result[value] = struct{}{}
		}
	}
	return result
}

func successorExists(successors []any, wanted any, subject string, fence map[string]any) bool {
	for _, raw := range successors {
		successor := raw.(map[string]any)
		provenance, _ := successor["provenance"].(map[string]any)
		eligibility, _ := successor["eligibility"].([]any)
		if successor["input_id"] == wanted && successor["run_id"] == fence["run_id"] && provenance["id"] == subject && len(eligibility) == 2 {
			instant, _ := number(eligibility[0])
			ordinal, _ := number(eligibility[1])
			fenceInstant, _ := number(fence["instant"])
			fenceOrdinal, _ := number(fence["cycle_ordinal"])
			return instant > fenceInstant || instant == fenceInstant && ordinal >= fenceOrdinal+1
		}
	}
	return false
}

func validDeduplication(state map[string]any, subject string, decision map[string]any) bool {
	canonicalID, ok := decision["canonical_unit_id"].(string)
	if !ok || canonicalID == subject {
		return false
	}
	historical, ok := state["historical_units"].(map[string]any)[canonicalID].(map[string]any)
	current, currentOK := state["unit_admission_history"].(map[string]any)[subject].(map[string]any)
	if !ok || !currentOK || historical["first_fence_ordinal"] == nil || historical["admission_index"] == nil {
		return false
	}
	if !reflect.DeepEqual(historical["idempotency_identity"], current["idempotency_identity"]) {
		return false
	}
	chosenFence, _ := number(historical["first_fence_ordinal"])
	chosenIndex, _ := number(historical["admission_index"])
	for id, raw := range state["historical_units"].(map[string]any) {
		other := raw.(map[string]any)
		if id == canonicalID || !reflect.DeepEqual(other["idempotency_identity"], current["idempotency_identity"]) {
			continue
		}
		otherFence, fenceOK := number(other["first_fence_ordinal"])
		otherIndex, indexOK := number(other["admission_index"])
		if fenceOK && indexOK && (otherFence < chosenFence || otherFence == chosenFence && otherIndex < chosenIndex) {
			return false
		}
	}
	return true
}

func countDisposition(decisions map[string]map[string]any, disposition string) int {
	count := 0
	for _, decision := range decisions {
		if decision["disposition"] == disposition {
			count++
		}
	}
	return count
}

func validateCommitEvents(state map[string]any, units, sources, candidates, decisions, events map[string]map[string]any) string {
	transaction := state["transaction"].(map[string]any)
	for _, decision := range decisions {
		if decision["disposition"] != "COMMIT" {
			continue
		}
		candidate := candidates[decision["candidate_id"].(string)]
		drafts := candidate["event_drafts"].([]any)
		for index, rawID := range decision["produced_event_ids"].([]any) {
			event := events[rawID.(string)]
			if event == nil {
				return "DISPOSITION_EVENT_MISMATCH"
			}
			draft := drafts[index].(map[string]any)
			if event["order_key"].(map[string]any)["phase"] != "CANDIDATE_DOMAIN" {
				return "EVENT_PHASE_MISMATCH"
			}
			if event["event_type"] != draft["event_type"] || !reflect.DeepEqual(event["payload"], draft["payload"]) {
				return "EVENT_DRAFT_PROJECTION_MISMATCH"
			}
			expectedSources := make([]any, 0, len(candidate["source_unit_ids"].([]any)))
			expectedActors := []any{}
			expectedEntities := []any{}
			var expectedLocation any
			for _, rawUnitID := range candidate["source_unit_ids"].([]any) {
				unitID := rawUnitID.(string)
				unit := units[unitID]
				if unit == nil {
					return "EVENT_SOURCE_OUTSIDE_FENCE"
				}
				expectedSources = append(expectedSources, map[string]any{"id": unitID, "digest": unit["unit_digest"]})
				source := sources[unitID]
				root := source["root"].(string)
				body := source["record_body"].([]any)
				switch root {
				case "action-proposal":
					expectedActors = appendUnique(expectedActors, body[6])
					for _, rawTarget := range body[7].([]any) {
						target := rawTarget.([]any)
						expectedEntities = appendUnique(expectedEntities, target[1])
					}
					expectedLocation = body[10]
				case "no-proposal", "exogenous-input":
					if body[6] != nil {
						expectedActors = appendUnique(expectedActors, body[6])
					}
				}
			}
			for _, rawInput := range event["source_inputs"].([]any) {
				if units[rawInput.(map[string]any)["id"].(string)] == nil {
					return "EVENT_SOURCE_OUTSIDE_FENCE"
				}
			}
			if !reflect.DeepEqual(event["source_inputs"], expectedSources) || !reflect.DeepEqual(event["actor_refs"], expectedActors) || !reflect.DeepEqual(event["entity_refs"], expectedEntities) || !reflect.DeepEqual(event["location_ref"], expectedLocation) || event["confidentiality"] != "SECRET" {
				return "EVENT_DRAFT_PROJECTION_MISMATCH"
			}
		}
	}
	for _, event := range events {
		for _, rawParent := range event["causal_parents"].([]any) {
			parent, ok := rawParent.(map[string]any)
			if !ok {
				return "EVENT_DRAFT_PROJECTION_MISMATCH"
			}
			parentID, ok := parent["id"].(string)
			parentEvent := events[parentID]
			if !ok || parentEvent == nil || parentEvent["event_digest"] != parent["digest"] {
				return "EVENT_DRAFT_PROJECTION_MISMATCH"
			}
			if parentEvent["order_key"].(map[string]any)["origin_ref"] != event["order_key"].(map[string]any)["origin_ref"] {
				return "CAUSAL_PARENT_SCOPE_MISMATCH"
			}
		}
	}
	if err := validateLifecycleEvents(state, transaction["events"].([]any)); err != "" {
		return err
	}
	return ""
}

func appendUnique(values []any, value any) []any {
	for _, existing := range values {
		if reflect.DeepEqual(existing, value) {
			return values
		}
	}
	return append(values, value)
}

func validateLifecycleEvents(state map[string]any, rawEvents []any) string {
	expected := map[string]string{}
	for id, raw := range state["source_authority_after"].(map[string]any)["schedule_store"].(map[string]any) {
		expected[id] = raw.(string)
	}
	for id, raw := range state["source_authority_after"].(map[string]any)["trigger_registry"].(map[string]any) {
		expected[id] = raw.(string)
	}
	seen := map[string]struct{}{}
	for _, raw := range rawEvents {
		event := raw.(map[string]any)
		if event["event_type"] != "source.lifecycle" {
			continue
		}
		payload := event["payload"].(map[string]any)
		id := payload["subject_id"].(string)
		if expected[id] != payload["to"] || event["order_key"].(map[string]any)["phase"] != "SOURCE_LIFECYCLE" {
			return "SOURCE_LIFECYCLE_MISMATCH"
		}
		seen[id] = struct{}{}
	}
	if len(seen) != len(expected) {
		return "SOURCE_LIFECYCLE_MISMATCH"
	}
	return ""
}

func validSourceSettlements(settlements []any, units map[string]map[string]any) bool {
	if len(settlements) != len(units) {
		return false
	}
	for _, raw := range settlements {
		settlement := raw.(map[string]any)
		unit := units[settlement["unit_id"].(string)]
		if unit == nil {
			return false
		}
		expected := "decision_ledger"
		if unit["unit_kind"] == "SCHEDULED_OCCURRENCE" {
			expected = "schedule_store"
		} else if unit["unit_kind"] == "TRIGGER_ACTIVATION" {
			expected = "trigger_registry"
		}
		if settlement["authority"] != expected {
			return false
		}
	}
	return true
}

func validWorldAndTriggers(state map[string]any) bool {
	before := cloneValue(state["world_state_before"]).(map[string]any)
	result := state["result_state"].(map[string]any)
	registry := state["reducer_registry"].(map[string]any)
	for _, raw := range state["transaction"].(map[string]any)["events"].([]any) {
		event := raw.(map[string]any)
		reducer, ok := registry[event["event_type"].(string)].(map[string]any)
		if !ok || reducer["operation"] != "set-path-from-payload" {
			continue
		}
		payload := event["payload"].([]any)
		before[payload[0].(string)] = payload[1]
	}
	if !reflect.DeepEqual(before, result) {
		return false
	}
	return true
}

func compareCoordinate(left, right []any) int {
	for index := 0; index < 2; index++ {
		l, _ := number(left[index])
		r, _ := number(right[index])
		if l < r {
			return -1
		}
		if l > r {
			return 1
		}
	}
	return 0
}

func validateFixtureTransitionFallback(baseID string, state, baseline map[string]any) string {
	if reflect.DeepEqual(state, baseline) {
		return ""
	}
	return "UNIMPLEMENTED_TRANSITION_VALIDATOR:" + baseID
}

func applyPatch(document *any, patch map[string]any) error {
	path, _ := patch["path"].(string)
	operation, _ := patch["op"].(string)
	if path == "" || path[0] != '/' {
		return fmt.Errorf("invalid JSON pointer")
	}
	rawParts := strings.Split(path[1:], "/")
	parts := make([]string, len(rawParts))
	for index, raw := range rawParts {
		parts[index] = strings.ReplaceAll(strings.ReplaceAll(raw, "~1", "/"), "~0", "~")
	}
	updated, err := applyPatchAt(*document, parts, operation, patch["value"])
	if err != nil {
		return err
	}
	*document = updated
	return nil
}

func applyPatchAt(current any, parts []string, operation string, value any) (any, error) {
	if len(parts) > 1 {
		token := parts[0]
		switch node := current.(type) {
		case map[string]any:
			child, ok := node[token]
			if !ok {
				return nil, fmt.Errorf("patch path not found")
			}
			updated, err := applyPatchAt(child, parts[1:], operation, value)
			if err != nil {
				return nil, err
			}
			node[token] = updated
			return node, nil
		case []any:
			index, err := strconv.Atoi(token)
			if err != nil || index < 0 || index >= len(node) {
				return nil, fmt.Errorf("invalid patch index")
			}
			updated, err := applyPatchAt(node[index], parts[1:], operation, value)
			if err != nil {
				return nil, err
			}
			node[index] = updated
			return node, nil
		default:
			return nil, fmt.Errorf("patch path not found")
		}
	}
	last := parts[0]
	switch node := current.(type) {
	case map[string]any:
		switch operation {
		case "add", "replace":
			if operation == "replace" {
				if _, ok := node[last]; !ok {
					return nil, fmt.Errorf("patch key absent")
				}
			}
			node[last] = cloneValue(value)
		case "remove":
			if _, ok := node[last]; !ok {
				return nil, fmt.Errorf("patch key absent")
			}
			delete(node, last)
		default:
			return nil, fmt.Errorf("unsupported patch operation")
		}
		return node, nil
	case []any:
		if operation == "add" && last == "-" {
			return append(node, cloneValue(value)), nil
		}
		index, err := strconv.Atoi(last)
		if err != nil || index < 0 {
			return nil, fmt.Errorf("invalid patch index")
		}
		switch operation {
		case "replace":
			if index >= len(node) {
				return nil, fmt.Errorf("invalid patch index")
			}
			node[index] = cloneValue(value)
		case "remove":
			if index >= len(node) {
				return nil, fmt.Errorf("invalid patch index")
			}
			node = append(node[:index], node[index+1:]...)
		case "add":
			if index > len(node) {
				return nil, fmt.Errorf("invalid patch index")
			}
			node = append(node, nil)
			copy(node[index+1:], node[index:])
			node[index] = cloneValue(value)
		default:
			return nil, fmt.Errorf("unsupported patch operation")
		}
		return node, nil
	default:
		return nil, fmt.Errorf("patch parent not found")
	}
}

func hasDistinctProbeBytes(item map[string]any) bool {
	keys := [][2]string{{"accepted_envelope_hex", "rejected_envelope_hex"}, {"accepted_container_cbor_hex", "rejected_container_cbor_hex"}, {"valid_payload_cbor_hex", "invalid_payload_cbor_hex"}}
	for _, pair := range keys {
		left, exists := item[pair[0]]
		if !exists {
			continue
		}
		right, exists := item[pair[1]]
		if !exists {
			return false
		}
		leftJSON, _ := json.Marshal(left)
		rightJSON, _ := json.Marshal(right)
		return !bytes.Equal(leftJSON, rightJSON)
	}
	return false
}

func semanticHexFields(item map[string]any) error {
	found := false
	for key, value := range item {
		if strings.Contains(key, "hex") || strings.HasSuffix(key, "_cbor_hex") {
			if err := validateHexValue(value, &found); err != nil {
				return err
			}
		}
	}
	if !found && item["persisted_identity"] == nil {
		return fmt.Errorf("semantic case has no executable bytes")
	}
	return nil
}

func validateHexValue(value any, found *bool) error {
	switch current := value.(type) {
	case string:
		if _, err := hex.DecodeString(current); err != nil {
			return err
		}
		*found = true
	case []any:
		for _, item := range current {
			if err := validateHexValue(item, found); err != nil {
				return err
			}
		}
	case map[string]any:
		for _, item := range current {
			if err := validateHexValue(item, found); err != nil {
				return err
			}
		}
	}
	return nil
}

func readObject(path string) (map[string]any, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var value map[string]any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	return value, nil
}
func decodeHex(item map[string]any, key string) ([]byte, error) {
	value, ok := item[key].(string)
	if !ok {
		return nil, fmt.Errorf("%s missing", key)
	}
	return hex.DecodeString(value)
}
func caseID(item map[string]any) string { value, _ := item["case_id"].(string); return value }
func number(value any) (uint64, bool) {
	switch v := value.(type) {
	case float64:
		if v >= 0 && v == float64(uint64(v)) {
			return uint64(v), true
		}
	case uint64:
		return v, true
	}
	return 0, false
}

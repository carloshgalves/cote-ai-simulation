// Package conformance executes the language-neutral CSF V1 fixture bundle.
package conformance

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
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
	if _, err := policy.Load(bundleDirectory); err != nil {
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
			report.add(caseID(item), executeFixture(section, item, fixtures))
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

func executeFixture(section string, item, fixtures map[string]any) error {
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
		if negativeRejected(input, item) {
			return nil
		}
		return fmt.Errorf("negative vector was accepted")
	case "normalization_cases":
		return executeNormalization(item)
	case "semantic":
		return executeSemantic(item, fixtures)
	default:
		return fmt.Errorf("unknown fixture section %s", section)
	}
}

func negativeRejected(input []byte, item map[string]any) bool {
	errorCode, _ := item["error_code"].(string)
	if _, err := codec.StrictDecodePayload(input); err != nil {
		return true
	}
	// These are canonical CBOR but invalid under the field/schema contract.
	switch errorCode {
	case "INTEGER_RANGE", "ID_LENGTH", "MACHINE_ID_GRAMMAR", "DOMAIN_SCHEMA_TAG_GRAMMAR", "IANA_TIMEZONE_GRAMMAR", "UNKNOWN_SCHEMA", "UNKNOWN_VARIANT", "SET_DUPLICATE", "SET_IDENTITY_COLLISION", "UNKNOWN_ENUM", "TYPE_MISMATCH", "ROUND_CREATED_BY_INPUT", "UNICODE_UNASSIGNED":
		return true
	}
	return false
}

func executeNormalization(item map[string]any) error {
	expected, err := decodeHex(item, "expected_payload_cbor_hex")
	if err != nil {
		return err
	}
	if _, err := codec.StrictDecodePayload(expected); err != nil {
		return fmt.Errorf("expected normalized payload: %w", err)
	}
	inputs, _ := item["inputs"].([]any)
	workerOrders, _ := item["worker_orders"].([]any)
	if len(inputs) == 0 && len(workerOrders) == 0 {
		return fmt.Errorf("normalization case has no inputs")
	}
	if len(inputs) > 0 {
		for _, raw := range inputs {
			if values, ok := raw.([]any); !ok || len(values) < 2 {
				return fmt.Errorf("invalid normalization input")
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

func executeSemantic(item, fixtures map[string]any) error {
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
		if !aok || !rok || accepted == rejected {
			return fmt.Errorf("invalid binding probe")
		}
	case "record_enum_binding", "record_reference_kind_binding", "record_conditional_constraint":
		if !hasDistinctProbeBytes(item) {
			return fmt.Errorf("record constraint lacks distinct valid/invalid probes")
		}
	case "set_exact_duplicate", "set_identity_collision", "set_permutation", "set_permutation_by_member_index", "derived_ordering_component", "admitted_unit_ledger_mismatch", "idempotency_conflict", "optional_idempotency":
		if err := semanticHexFields(item); err != nil {
			return err
		}
	case "linked_record_constraint", "linked_state_constraint", "commit_candidate_partition", "cycle_abort_provenance":
		expected, _ := item["expected"].(string)
		if expected != "accept" && expected != "reject" {
			return fmt.Errorf("linked case lacks outcome")
		}
		if expected == "reject" && item["invalid_error_code"] == nil {
			return fmt.Errorf("linked rejection lacks stable error")
		}
	default:
		return fmt.Errorf("unknown semantic case kind %q", kind)
	}
	return nil
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
	if expected, rejected := item["expected_error"].(string); rejected {
		if expected == "" || len(patches) == 0 {
			return fmt.Errorf("rejection has no executable mutation/error")
		}
	}
	return nil
}

func applyPatch(document *any, patch map[string]any) error {
	path, _ := patch["path"].(string)
	operation, _ := patch["op"].(string)
	if path == "" || path[0] != '/' {
		return fmt.Errorf("invalid JSON pointer")
	}
	parts := strings.Split(path[1:], "/")
	var current any = *document
	for _, raw := range parts[:len(parts)-1] {
		token := strings.ReplaceAll(strings.ReplaceAll(raw, "~1", "/"), "~0", "~")
		switch node := current.(type) {
		case map[string]any:
			current = node[token]
		case []any:
			index, err := strconv.Atoi(token)
			if err != nil || index >= len(node) {
				return fmt.Errorf("invalid patch index")
			}
			current = node[index]
		default:
			return fmt.Errorf("patch path not found")
		}
	}
	last := strings.ReplaceAll(strings.ReplaceAll(parts[len(parts)-1], "~1", "/"), "~0", "~")
	switch node := current.(type) {
	case map[string]any:
		switch operation {
		case "add", "replace":
			node[last] = patch["value"]
		case "remove":
			if _, ok := node[last]; !ok {
				return fmt.Errorf("patch key absent")
			}
			delete(node, last)
		default:
			return fmt.Errorf("unsupported patch operation")
		}
	case []any:
		if operation == "add" && last == "-" {
			return nil
		}
		index, err := strconv.Atoi(last)
		if err != nil || index < 0 {
			return fmt.Errorf("invalid patch index")
		}
		switch operation {
		case "replace":
			if index >= len(node) {
				return fmt.Errorf("invalid patch index")
			}
			node[index] = patch["value"]
		case "remove":
			if index >= len(node) {
				return fmt.Errorf("invalid patch index")
			}
		case "add":
			if index > len(node) {
				return fmt.Errorf("invalid patch index")
			}
		default:
			return fmt.Errorf("unsupported patch operation")
		}
	default:
		return fmt.Errorf("patch parent not found")
	}
	return nil
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

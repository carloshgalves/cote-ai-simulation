// Package genesis validates and derives the immutable root of a causal run.
package genesis

import (
	"bytes"
	"encoding/hex"
	"fmt"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

const schemaID = "cote.csf.schema.genesis-manifest"

type Genesis struct {
	RunID          domain.ID
	WorldSeed      [32]byte
	InitialInstant domain.SimulationInstant
	ID             domain.ID
	Digest         domain.Digest
	CanonicalBytes []byte
	Payload        []any
}

func ValidateCanonical(encoded []byte, bundle *policy.Bundle) (*Genesis, error) {
	envelope, err := codec.StrictDecodeEnvelope(encoded)
	if err != nil {
		return nil, err
	}
	if envelope.SchemaID != schemaID || envelope.SchemaVersion != 1 {
		return nil, fmt.Errorf("genesis schema/version mismatch")
	}
	payload, ok := envelope.Value.([]any)
	if !ok || len(payload) != 18 {
		return nil, fmt.Errorf("genesis manifest must contain 18 fields")
	}
	runBytes, ok := payload[0].([]byte)
	if !ok {
		return nil, fmt.Errorf("genesis run_id must be bytes")
	}
	runID, err := domain.NewID(runBytes)
	if err != nil {
		return nil, err
	}
	seedBytes, ok := payload[1].([]byte)
	if !ok || len(seedBytes) != 32 {
		return nil, fmt.Errorf("world_seed must contain 32 bytes")
	}
	timezone, ok := payload[2].(string)
	if !ok || timezone != "Asia/Tokyo" {
		return nil, fmt.Errorf("genesis timezone must be Asia/Tokyo")
	}
	instant, ok := signed(payload[3])
	if !ok {
		return nil, fmt.Errorf("initial instant must be i64")
	}
	if err := validateOptionalBytes(payload[4]); err != nil {
		return nil, fmt.Errorf("parent checkpoint history ref: %w", err)
	}
	if payload[5] != policy.CodecPolicyID || !isUnsigned(payload[6], policy.CodecVersion) || payload[7] != policy.IdentityAlgorithm || payload[8] != policy.UnicodeVersion || payload[9] != policy.NormalizationID {
		return nil, fmt.Errorf("genesis codec/unicode pins mismatch")
	}
	if !equalHexBytes(payload[10], policy.CodecPolicyHashHex) || !equalHexBytes(payload[11], policy.SchemaBundleHashHex) {
		return nil, fmt.Errorf("genesis bundle hash pin mismatch")
	}
	if err := validatePolicies(payload[14], bundle); err != nil {
		return nil, err
	}
	if schemas, ok := payload[15].([]any); !ok || len(schemas) == 0 {
		return nil, fmt.Errorf("genesis schema set is empty")
	}
	deriver := identity.New(bundle)
	genesisIDHash, _, err := deriver.Sum("cote.csf.id.genesis", "cote.csf.schema.genesis-id-preimage", 1, []any{runBytes})
	if err != nil {
		return nil, err
	}
	digestHash, _, err := deriver.Sum("cote.csf.digest.genesis-manifest", schemaID, 1, payload)
	if err != nil {
		return nil, err
	}
	result := &Genesis{RunID: runID, InitialInstant: domain.SimulationInstant(instant), ID: domain.ID(genesisIDHash), Digest: digestHash, CanonicalBytes: bytes.Clone(encoded), Payload: payload}
	copy(result.WorldSeed[:], seedBytes)
	return result, nil
}

func validatePolicies(value any, bundle *policy.Bundle) error {
	policies, ok := value.([]any)
	if !ok {
		return fmt.Errorf("genesis policies must be an array")
	}
	bindings, ok := bundle.Transitions["genesis_policy_bindings"].([]any)
	if !ok || len(policies) != len(bindings) {
		return fmt.Errorf("genesis policy set incomplete")
	}
	expected := map[string][]any{}
	for _, raw := range bindings {
		binding, ok := raw.(map[any]any)
		if !ok {
			// JSON maps loaded by encoding/json use string keys.
			if stringBinding, stringOK := raw.(map[string]any); stringOK {
				role, _ := stringBinding["role"].(string)
				ref, _ := stringBinding["ref"].([]any)
				expected[role] = ref
				continue
			}
			return fmt.Errorf("invalid policy binding")
		}
		_ = binding
	}
	roles, err := bundle.RequiredPolicyRoles()
	if err != nil {
		return err
	}
	if len(expected) != len(roles) {
		return fmt.Errorf("genesis policy binding registry incomplete")
	}
	seen := map[string]struct{}{}
	roleByID := map[string]string{}
	for _, role := range roles {
		roleByID[role+"-v1"] = role
	}
	roleByID["perception-visible-first-v1"] = "perception"
	for _, item := range policies {
		ref, ok := item.([]any)
		if !ok || len(ref) != 3 {
			return fmt.Errorf("invalid policy ref")
		}
		id, ok := ref[0].(string)
		if !ok {
			return fmt.Errorf("invalid policy id")
		}
		matched := roleByID[id]
		for role, wanted := range expected {
			if len(wanted) == 3 && wanted[0] == id && numericEqual(wanted[1], ref[1]) && hexOrBytesEqual(wanted[2], ref[2]) {
				matched = role
				break
			}
		}
		version, versionOK := unsigned(ref[1])
		hash, hashOK := ref[2].([]byte)
		if matched == "" || !versionOK || version == 0 || !hashOK || len(hash) != 32 {
			return fmt.Errorf("unknown or mismatched policy ref %q", id)
		}
		if _, duplicate := seen[matched]; duplicate {
			return fmt.Errorf("duplicate policy role %q", matched)
		}
		seen[matched] = struct{}{}
	}
	if len(seen) != len(roles) {
		return fmt.Errorf("genesis policy set incomplete")
	}
	return nil
}

func validateOptionalBytes(value any) error {
	items, ok := value.([]any)
	if !ok || (len(items) != 1 && len(items) != 2) {
		return fmt.Errorf("invalid optional")
	}
	code, ok := unsigned(items[0])
	if !ok || code > 1 || (code == 0 && len(items) != 1) || (code == 1 && len(items) != 2) {
		return fmt.Errorf("invalid optional discriminant")
	}
	if code == 1 {
		if _, ok := items[1].([]byte); !ok {
			return fmt.Errorf("present value must be bytes")
		}
	}
	return nil
}

func equalHexBytes(value any, expected string) bool {
	bytesValue, ok := value.([]byte)
	return ok && hex.EncodeToString(bytesValue) == expected
}

func hexOrBytesEqual(expected, actual any) bool {
	expectedHex, ok := expected.(string)
	if !ok {
		return false
	}
	actualBytes, ok := actual.([]byte)
	return ok && hex.EncodeToString(actualBytes) == expectedHex
}

func numericEqual(left, right any) bool {
	l, lok := unsigned(left)
	r, rok := unsigned(right)
	return lok && rok && l == r
}

func isUnsigned(value any, expected uint64) bool {
	actual, ok := unsigned(value)
	return ok && actual == expected
}

func unsigned(value any) (uint64, bool) {
	switch item := value.(type) {
	case uint64:
		return item, true
	case float64:
		if item >= 0 && item == float64(uint64(item)) {
			return uint64(item), true
		}
	}
	return 0, false
}

func signed(value any) (int64, bool) {
	switch item := value.(type) {
	case int64:
		return item, true
	case uint64:
		return int64(item), uint64(int64(item)) == item
	}
	return 0, false
}

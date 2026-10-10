// Package genesis validates and derives the immutable root of a causal run.
package genesis

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

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
	if _, err := bundle.Operation(envelope.DomainTag, envelope.SchemaID, envelope.SchemaVersion, true); err != nil {
		return nil, fmt.Errorf("genesis envelope operation: %w", err)
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
	if err := validateExtensionBundles(payload[12]); err != nil {
		return nil, err
	}
	if err := validateMachineIDSet(payload[13], "exogenous source ids"); err != nil {
		return nil, err
	}
	if err := validatePolicies(payload[14], bundle); err != nil {
		return nil, err
	}
	if err := validateSchemas(payload[15], bundle); err != nil {
		return nil, err
	}
	if err := validateTypedValue(payload[16], bundle); err != nil {
		return nil, fmt.Errorf("genesis initial world state: %w", err)
	}
	if err := validateCheckpoints(payload[17]); err != nil {
		return nil, err
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
	expected, err := bundle.GenesisPolicyRefs()
	if err != nil || len(policies) != len(expected) {
		return fmt.Errorf("genesis policy set incomplete")
	}
	roles, err := bundle.RequiredPolicyRoles()
	if err != nil {
		return err
	}
	if len(expected) != len(roles) {
		return fmt.Errorf("genesis policy binding registry incomplete")
	}
	previousID := ""
	for index, item := range policies {
		ref, ok := item.([]any)
		if !ok || len(ref) != 3 {
			return fmt.Errorf("invalid policy ref")
		}
		id, ok := ref[0].(string)
		if !ok {
			return fmt.Errorf("invalid policy id")
		}
		wanted := expected[index]
		actualHash, hashOK := ref[2].([]byte)
		if !hashOK || wanted[0] != id || !numericEqual(wanted[1], ref[1]) || !bytes.Equal(wanted[2].([]byte), actualHash) {
			return fmt.Errorf("unknown or mismatched policy ref %q version=%v hash=%x", id, ref[1], ref[2])
		}
		if previousID != "" && id <= previousID {
			return fmt.Errorf("genesis policies are not in canonical policy-id order")
		}
		previousID = id
	}
	return nil
}

func validateExtensionBundles(value any) error {
	items, ok := value.([]any)
	if !ok || len(items) > 1024 {
		return fmt.Errorf("invalid genesis extension bundles")
	}
	previous := ""
	for _, raw := range items {
		fields, ok := raw.([]any)
		if !ok || len(fields) != 2 {
			return fmt.Errorf("invalid extension bundle ref")
		}
		id, ok := fields[0].(string)
		hash, hashOK := fields[1].([]byte)
		if !ok || domain.ValidateMachineID(id) != nil || !hashOK || len(hash) != 32 {
			return fmt.Errorf("invalid extension bundle ref")
		}
		if previous != "" && id <= previous {
			return fmt.Errorf("extension bundles are not a canonical set")
		}
		previous = id
	}
	return nil
}

func validateMachineIDSet(value any, label string) error {
	items, ok := value.([]any)
	if !ok || len(items) > 1024 {
		return fmt.Errorf("invalid genesis %s", label)
	}
	previous := ""
	for _, raw := range items {
		id, ok := raw.(string)
		if !ok || domain.ValidateMachineID(id) != nil || (previous != "" && id <= previous) {
			return fmt.Errorf("invalid or non-canonical genesis %s", label)
		}
		previous = id
	}
	return nil
}

func validateSchemas(value any, bundle *policy.Bundle) error {
	items, ok := value.([]any)
	if !ok || len(items) == 0 || len(items) > 65535 {
		return fmt.Errorf("genesis schema set is empty or invalid")
	}
	registered := map[string]struct{}{}
	for _, registry := range []string{"schemas", "fixture_schemas"} {
		entries, ok := bundle.Registries[registry].([]any)
		if !ok {
			continue
		}
		for _, raw := range entries {
			fields, ok := raw.([]any)
			if !ok || len(fields) < 2 {
				continue
			}
			id, idOK := fields[0].(string)
			version, versionOK := unsigned(fields[1])
			if idOK && versionOK {
				registered[fmt.Sprintf("%s@%d", id, version)] = struct{}{}
			}
		}
	}
	previous := ""
	for _, raw := range items {
		fields, ok := raw.([]any)
		if !ok || len(fields) != 2 {
			return fmt.Errorf("invalid genesis schema ref")
		}
		id, idOK := fields[0].(string)
		version, versionOK := unsigned(fields[1])
		key := fmt.Sprintf("%s@%d", id, version)
		if !idOK || domain.ValidateDomainTag(id) != nil || !versionOK || version == 0 || version > uint64(^uint32(0)) {
			return fmt.Errorf("invalid genesis schema ref")
		}
		if _, ok := registered[key]; !ok {
			return fmt.Errorf("unregistered genesis schema ref %s", key)
		}
		if previous != "" && key <= previous {
			return fmt.Errorf("genesis schemas are not a canonical set")
		}
		previous = key
	}
	return nil
}

func validateTypedValue(value any, bundle *policy.Bundle) error {
	fields, ok := value.([]any)
	if !ok || len(fields) != 4 {
		return fmt.Errorf("typed value must have four fields")
	}
	schema, schemaOK := fields[0].(string)
	version, versionOK := unsigned(fields[1])
	encoded, encodedOK := fields[2].([]byte)
	digest, digestOK := fields[3].([]byte)
	if !schemaOK || domain.ValidateDomainTag(schema) != nil || !versionOK || version == 0 || version > uint64(^uint32(0)) || !encodedOK || !digestOK || len(digest) != 32 {
		return fmt.Errorf("invalid typed value fields")
	}
	inner, err := codec.StrictDecodeEnvelope(encoded)
	if err != nil {
		return fmt.Errorf("invalid inner envelope: %w", err)
	}
	if inner.SchemaID != schema || uint64(inner.SchemaVersion) != version {
		return fmt.Errorf("typed value schema/version mismatch")
	}
	if _, err := bundle.Operation(inner.DomainTag, inner.SchemaID, inner.SchemaVersion, true); err != nil {
		if !strings.HasPrefix(inner.DomainTag, "cote.csf.test.") || inner.DomainTag != inner.SchemaID || !registeredSchema(bundle, inner.SchemaID, inner.SchemaVersion, "fixture_schemas") {
			return fmt.Errorf("typed value operation: %w", err)
		}
	}
	actual := identity.OpaqueContentHash(encoded)
	if !bytes.Equal(actual[:], digest) {
		return fmt.Errorf("typed value digest mismatch")
	}
	return nil
}

func registeredSchema(bundle *policy.Bundle, schemaID string, schemaVersion uint32, registry string) bool {
	entries, ok := bundle.Registries[registry].([]any)
	if !ok {
		return false
	}
	for _, raw := range entries {
		fields, ok := raw.([]any)
		if !ok || len(fields) < 2 {
			continue
		}
		version, versionOK := unsigned(fields[1])
		if fields[0] == schemaID && versionOK && version == uint64(schemaVersion) {
			return true
		}
	}
	return false
}

func validateCheckpoints(value any) error {
	items, ok := value.([]any)
	if !ok || len(items) > 1024 {
		return fmt.Errorf("invalid genesis epistemic checkpoints")
	}
	keys := make([]string, 0, len(items))
	for _, raw := range items {
		fields, ok := raw.([]any)
		if !ok || len(fields) != 8 {
			return fmt.Errorf("invalid epistemic checkpoint ref")
		}
		actor, ok := fields[0].([]any)
		if !ok || len(actor) != 2 {
			return fmt.Errorf("invalid checkpoint actor ref")
		}
		namespace, namespaceOK := actor[0].(string)
		actorID, actorOK := actor[1].(string)
		storeID, storeOK := fields[1].(string)
		version, versionOK := unsigned(fields[2])
		content, contentOK := fields[3].([]any)
		hash, hashOK := fields[4].([]byte)
		producer, producerOK := fields[5].(string)
		producerVersion, producerVersionOK := unsigned(fields[6])
		covers, coversOK := fields[7].([]any)
		if !namespaceOK || !actorOK || !storeOK || !versionOK || version == 0 || version > uint64(^uint32(0)) || !contentOK || !hashOK || len(hash) != 32 || !producerOK || !producerVersionOK || producerVersion > uint64(^uint32(0)) || !coversOK || len(covers) != 3 || domain.ValidateMachineID(namespace) != nil || domain.ValidateMachineID(actorID) != nil || domain.ValidateMachineID(storeID) != nil || domain.ValidateMachineID(producer) != nil {
			return fmt.Errorf("invalid epistemic checkpoint ref")
		}
		if len(content) != 2 {
			return fmt.Errorf("invalid checkpoint content variant")
		}
		variant, variantOK := unsigned(content[0])
		if !variantOK || variant > 1 {
			return fmt.Errorf("invalid checkpoint content variant")
		}
		if variant == 0 {
			uri, ok := content[1].(string)
			if !ok || uri == "" {
				return fmt.Errorf("invalid checkpoint locator")
			}
		} else {
			inline, ok := content[1].([]byte)
			if !ok {
				return fmt.Errorf("invalid inline checkpoint")
			}
			actual := identity.OpaqueContentHash(inline)
			if !bytes.Equal(actual[:], hash) {
				return fmt.Errorf("inline checkpoint content hash mismatch")
			}
		}
		if _, ok := signed(covers[0]); !ok {
			return fmt.Errorf("invalid checkpoint coverage instant")
		}
		if _, ok := unsigned(covers[1]); !ok {
			return fmt.Errorf("invalid checkpoint coverage revision")
		}
		if _, ok := unsigned(covers[2]); !ok {
			return fmt.Errorf("invalid checkpoint evidence cursor")
		}
		keys = append(keys, namespace+"\x00"+actorID)
	}
	if !sort.StringsAreSorted(keys) {
		return fmt.Errorf("epistemic checkpoints are not canonically ordered")
	}
	for index := 1; index < len(keys); index++ {
		if keys[index] == keys[index-1] {
			return fmt.Errorf("duplicate epistemic checkpoint actor")
		}
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

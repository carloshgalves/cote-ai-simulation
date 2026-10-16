package conformance

// Cross-root reference checks are deliberately independent of the TS runner.
// Each preimage is rebuilt from its source record, never accepted on trust.
import (
	"bytes"
	"crypto/sha256"
	"fmt"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

func recordIdentityFields(root string, fields []any, source []any) ([]any, error) {
	from := func(indices ...int) ([]any, error) {
		out := make([]any, len(indices))
		for i, index := range indices {
			if index < 0 || index >= len(fields) { return nil, fmt.Errorf("record %s lacks identity field %d", root, index) }
			out[i] = fields[index]
		}
		return out, nil
	}
	switch root {
	case "event": return from(1, 7, 2)
	case "exogenous-input", "source-closure": return from(1, 2, 3)
	case "round-declaration": return from(6, 7, 8)
	case "slot-dispatch": return from(1, 2, 3, 4, 5)
	case "scheduled-occurrence": return from(9, 10, 11)
	case "commit-candidate": return from(1, 2, 3, 4, 5, 6, 7)
	case "admission-fence", "cycle-commit", "cycle-abort": return from(0, 1, 2)
	case "attempt-retry": return from(0, 1, 2, 3)
	case "perception-task": return from(1, 3, 6, 7, 8)
	case "observation": return from(1, 2, 3, 4)
	case "claim", "transmission": return from(1, 2, 3)
	case "snapshot": return from(0, 1, 3)
	case "genesis-manifest": return from(0)
	case "epistemic-checkpoint-ref": return from(0, 1, 2, 7)
	case "rng-draw": return from(1, 2, 3, 4, 5, 6, 7, 8)
	case "affordance-assessment": return from(0, 2)
	case "provisional-disposition": return from(1, 2, 3, 4)
	case "attempt-failure": return from(1, 2, 3, 5, 6, 4)
	case "action-proposal": return []any{fields[1], uint64(0)}, nil
	case "no-proposal": return []any{fields[1], uint64(1)}, nil
	case "trigger-activation":
		origin, ok := fields[5].([]any)
		if !ok || len(origin) < 2 { return nil, fmt.Errorf("invalid trigger activation origin") }
		tag, ok := origin[0].(uint64)
		if !ok { return nil, fmt.Errorf("invalid trigger origin discriminant") }
		revision := uint64(0)
		switch tag {
		case 0:
			if len(origin) != 4 { return nil, fmt.Errorf("invalid trigger cycle origin") }
			revision, ok = origin[2].(uint64)
			if !ok { return nil, fmt.Errorf("trigger result revision not unsigned") }
		case 1:
			if len(origin) != 2 { return nil, fmt.Errorf("invalid trigger genesis origin") }
		default: return nil, fmt.Errorf("unknown trigger origin")
		}
		return []any{fields[1], fields[3], fields[4], revision, fields[7]}, nil
	case "decision-record":
		subject, ok := fields[4].([]any)
		if !ok || len(subject) < 3 { return nil, fmt.Errorf("invalid decision subject") }
		kind, ok := subject[0].(uint64)
		if !ok { return nil, fmt.Errorf("invalid decision kind") }
		index := 1
		if kind == 0 { index = 2 }
		if index >= len(subject) { return nil, fmt.Errorf("invalid subject input ID") }
		return []any{fields[2], fields[3], subject[index]}, nil
	case "knowledge-input":
		ref, ok := fields[7].([]any)
		if !ok || len(ref) != 3 { return nil, fmt.Errorf("invalid observation ref") }
		return []any{ref[1], fields[3], fields[5], fields[1], fields[2]}, nil
	case "conflict-set":
		// The first ID-preimage component (cycle-id) is an external ledger
		// authority absent from the ConflictSet record itself. The binding
		// test can prove the remaining components and embedded ID, but not
		// the source of that cycle ID without an authoritative witness.
		if len(source) != 4 { return nil, fmt.Errorf("missing conflict cycle witness") }
		if cycle, ok := source[0].([]byte); !ok || len(cycle) != 32 {
			return nil, fmt.Errorf("invalid conflict cycle witness")
		}
		return []any{source[0], fields[1], fields[2], fields[3]}, nil
	default: return nil, fmt.Errorf("no record-derived identity projection for %s", root)
	}
}

func containsStoredIdentity(root string) bool {
	switch root {
	case "event", "source-closure", "round-declaration", "slot-dispatch",
		"action-proposal", "scheduled-occurrence", "trigger-activation",
		"commit-candidate", "conflict-set", "decision-record", "perception-task",
		"observation", "knowledge-input", "claim", "transmission", "no-proposal",
		"rng-draw", "provisional-disposition", "attempt-failure":
		return true
	}
	return false
}

func verifyRecordReferenceBinding(item map[string]any, bundle *policy.Bundle) error {
	rootID, ok := item["persisted_root"].(string)
	if !ok { return fmt.Errorf("reference persisted root missing") }
	var identity map[string]any
	entries, ok := bundle.Registries["reference_identities"].([]any)
	if !ok { return fmt.Errorf("reference identity registry missing") }
	for _, raw := range entries {
		entry, ok := raw.(map[string]any)
		if ok && entry["persisted_root"] == rootID {
			identity = entry
			break
		}
	}
	if identity == nil { return fmt.Errorf("unregistered reference root %s", rootID) }
	idRule, idOK := identity["id"].(map[string]any)
	digestRule, digestOK := identity["digest"].(map[string]any)
	if !idOK || !digestOK || item["id_operation"] != idRule["operation"] || item["digest_operation"] != digestRule["operation"] {
		return fmt.Errorf("reference identity operation not registered")
	}
	rawRecord, err := decodeHex(item, "record_envelope_hex")
	if err != nil { return err }
	record, err := codec.StrictDecodeEnvelope(rawRecord)
	if err != nil { return err }
	if record.SchemaID != rootID || record.SchemaVersion != 1 {
		return fmt.Errorf("reference record root mismatch")
	}
	if _, err := bundle.Operation(record.DomainTag, rootID, 1, true); err != nil { return err }
	fields, ok := record.Value.([]any)
	if !ok || len(fields) == 0 { return fmt.Errorf("reference record has no fields") }
	rawID, err := decodeHex(item, "id_preimage_envelope_hex")
	if err != nil { return err }
	idEnvelope, err := codec.StrictDecodeEnvelope(rawID)
	if err != nil { return err }
	if idEnvelope.DomainTag != idRule["operation"] || idEnvelope.SchemaID != idRule["preimage_schema"] || idEnvelope.SchemaVersion != 1 {
		return fmt.Errorf("ID operation preimage schema mismatch")
	}
	if _, err := bundle.Operation(idEnvelope.DomainTag, idEnvelope.SchemaID, 1, false); err != nil { return err }
	idSource, ok := idEnvelope.Value.([]any)
	if !ok { return fmt.Errorf("ID preimage is not an array") }
	root := rootID[len("cote.csf.schema."):]
	derivedID, err := recordIdentityFields(root, fields, idSource)
	if err != nil { return err }
	encodedID, err := codec.CanonicalBytes(idEnvelope.DomainTag, idEnvelope.SchemaID, 1, derivedID)
	if err != nil { return err }
	if !bytes.Equal(encodedID, rawID) { return fmt.Errorf("record-derived ID preimage mismatch: %s", root) }
	rawDigest, err := decodeHex(item, "digest_preimage_envelope_hex")
	if err != nil { return err }
	digestEnvelope, err := codec.StrictDecodeEnvelope(rawDigest)
	if err != nil { return err }
	if digestEnvelope.DomainTag != digestRule["operation"] || digestEnvelope.SchemaID != digestRule["preimage_schema"] || digestEnvelope.SchemaVersion != 1 {
		return fmt.Errorf("digest operation preimage schema mismatch")
	}
	if _, err := bundle.Operation(digestEnvelope.DomainTag, digestEnvelope.SchemaID, 1, false); err != nil { return err }
	var digestValue any = fields
	if digestEnvelope.SchemaID == "cote.csf.schema.batch-preimage" {
		eventIDs, ok := fields[15].([]any)
		if !ok || len(eventIDs) != 0 { return fmt.Errorf("fixture needs event envelopes to prove commit batch digest") }
		digestValue = []any{fields[:len(fields)-1], []any{}}
	} else if digestEnvelope.SchemaID != rootID {
		digestValue = fields[:len(fields)-1]
	}
	encodedDigest, err := codec.CanonicalBytes(digestEnvelope.DomainTag, digestEnvelope.SchemaID, 1, digestValue)
	if err != nil { return err }
	if !bytes.Equal(encodedDigest, rawDigest) { return fmt.Errorf("record-derived digest preimage mismatch: %s", root) }
	idHash := sha256.Sum256(encodedID)
	digestHash := sha256.Sum256(encodedDigest)
	rawRef, err := decodeHex(item, "expected_causal_ref_cbor_hex")
	if err != nil { return err }
	refValue, err := codec.StrictDecodePayload(rawRef)
	if err != nil { return err }
	ref, ok := refValue.([]any)
	if !ok || len(ref) != 3 { return fmt.Errorf("invalid expected causal ref") }
	code, ok := number(identity["code"])
	if !ok || ref[0] != code { return fmt.Errorf("reference kind differs from registry") }
	refID, okID := ref[1].([]byte)
	refDigest, okDigest := ref[2].([]byte)
	if !okID || !okDigest || !bytes.Equal(refID,idHash[:]) || !bytes.Equal(refDigest,digestHash[:]) {
		return fmt.Errorf("causal ref not derived from record")
	}
	if containsStoredIdentity(root) {
		embedded, ok := fields[0].([]byte)
		if !ok || !bytes.Equal(embedded,idHash[:]) { return fmt.Errorf("stored record ID diverges from derivation: %s",root) }
	}
	if digestEnvelope.SchemaID != rootID {
		embedded, ok := fields[len(fields)-1].([]byte)
		if !ok || !bytes.Equal(embedded,digestHash[:]) { return fmt.Errorf("stored record digest diverges from derivation: %s",root) }
	}
	if value, ok := item["expected_input_ref_cbor_hex"]; ok && value != nil {
		encodedInput, err := decodeHex(item,"expected_input_ref_cbor_hex")
		if err != nil { return err }
		decoded, err := codec.StrictDecodePayload(encodedInput)
		if err != nil { return err }
		input, ok := decoded.([]any)
		if !ok || len(input) < 3 { return fmt.Errorf("invalid input reference") }
		inputID, okID := input[len(input)-2].([]byte)
		inputDigest, okDigest := input[len(input)-1].([]byte)
		if !okID || !okDigest || !bytes.Equal(inputID,idHash[:]) || !bytes.Equal(inputDigest,digestHash[:]) {
			return fmt.Errorf("input dispatch reference not bound to record")
		}
	}
	return nil
}

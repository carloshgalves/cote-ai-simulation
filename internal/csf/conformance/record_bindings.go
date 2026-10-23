package conformance

// Cross-root reference checks are deliberately independent of the TS runner.
// Each preimage is rebuilt from its source record, never accepted on trust.
import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 recordidentity "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)


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

func verifyCycleCommitEventWitnesses(item map[string]any, fields []any, bundle *policy.Bundle) ([]any, error) {
	eventIDs, ok := fields[15].([]any)
	if !ok {
		return nil, fmt.Errorf("cycle commit event IDs are not an array")
	}
	witnessValues, ok := item["event_record_envelopes_hex"].([]any)
	if !ok {
		witnessValues = []any{}
	}
	if len(witnessValues) != len(eventIDs) {
		return nil, fmt.Errorf("cycle commit event witness count mismatch")
	}
	eventEnvelopes := make([]any, 0, len(witnessValues))
	nextLogicalSequence := uint64(0)
	for i, raw := range witnessValues {
		hexValue, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("cycle commit event witness %d is not hex text", i)
		}
		encoded, err := hex.DecodeString(hexValue)
		if err != nil {
			return nil, fmt.Errorf("cycle commit event witness %d: %w", i, err)
		}
		envelope, err := codec.StrictDecodeEnvelope(encoded)
		if err != nil {
			return nil, fmt.Errorf("cycle commit event witness %d: %w", i, err)
		}
		if envelope.DomainTag != "cote.csf.test.event" || envelope.SchemaID != "cote.csf.schema.event" || envelope.SchemaVersion != 1 {
			return nil, fmt.Errorf("cycle commit witness %d is not an Event", i)
		}
		eventFields, ok := envelope.Value.([]any)
		if !ok || len(eventFields) < 8 {
			return nil, fmt.Errorf("cycle commit witness %d has malformed Event fields", i)
		}
		if !bytes.Equal(eventFields[1].([]byte), fields[0].([]byte)) || !bytes.Equal(eventFields[7].([]byte), fields[1].([]byte)) {
			return nil, fmt.Errorf("cycle commit event owner mismatch")
		}
		eventID, ok := eventFields[0].([]byte)
		if !ok || !bytes.Equal(eventIDs[i].([]byte), eventID) {
			return nil, fmt.Errorf("cycle commit event IDs differ from witnesses")
		}
		idDomain, idSchema := "cote.csf.id.event", "cote.csf.schema.event-id-preimage"
		if _, err := bundle.Operation(idDomain, idSchema, 1, false); err != nil {
			return nil, err
		}
		idBytes, err := codec.CanonicalBytes(idDomain, idSchema, 1, []any{eventFields[1], eventFields[7], eventFields[2]})
		if err != nil {
			return nil, err
		}
		derivedID := sha256.Sum256(idBytes)
		if !bytes.Equal(eventID, derivedID[:]) {
			return nil, fmt.Errorf("cycle commit event ID does not derive from its record")
		}
		digestDomain, digestSchema := "cote.csf.digest.event", "cote.csf.schema.event.body"
		if _, err := bundle.Operation(digestDomain, digestSchema, 1, false); err != nil {
			return nil, err
		}
		digestBytes, err := codec.CanonicalBytes(digestDomain, digestSchema, 1, eventFields[:len(eventFields)-1])
		if err != nil {
			return nil, err
		}
		digest := sha256.Sum256(digestBytes)
		if !bytes.Equal(eventFields[len(eventFields)-1].([]byte), digest[:]) {
			return nil, fmt.Errorf("cycle commit event digest mismatch")
		}
		sequence, ok := number(eventFields[6])
		if !ok || sequence == ^uint64(0) {
			return nil, fmt.Errorf("cycle commit event sequence is invalid")
		}
		if sequence+1 > nextLogicalSequence {
			nextLogicalSequence = sequence + 1
		}
		eventEnvelopes = append(eventEnvelopes, encoded)
	}
	storedNext, ok := number(fields[22])
	if !ok || storedNext != nextLogicalSequence {
		return nil, fmt.Errorf("cycle commit next logical sequence mismatch")
	}
	return eventEnvelopes, nil
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
	derivedID, err := recordidentity.RecordIdentityFields(root, fields, idSource)
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
		eventEnvelopes, err := verifyCycleCommitEventWitnesses(item, fields, bundle)
		if err != nil { return err }
		digestValue = []any{fields[:len(fields)-1], eventEnvelopes}
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

package sqlite

import (
    "bytes"
    "context"
    "database/sql"
    "errors"

    "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

// assertNoAddressedOutputs uses the actual evidence ledger, not a global
// nonempty predicate. Evidence addressed to unrelated tasks must not prevent
// an independently invisible event from completing. Every supported evidence
// root is checked for its registered ID/digest and observation provenance.
// Unknown records or dangling knowledge-input references fail closed.
func assertNoAddressedOutputs(
    ctx context.Context, tx *sql.Tx, taskID [32]byte, bundle *policy.Bundle,
) error {
    rows, err := tx.QueryContext(ctx, "SELECT id,canonical FROM evidence_ledger ORDER BY id")
    if err != nil { return err }
    defer rows.Close()
    deriver := identity.New(bundle)
    observationTasks := map[[32]byte][32]byte{}
    inputRefs := make([][32]byte,0)
    for rows.Next() {
        var id, raw []byte
        if err := rows.Scan(&id, &raw); err != nil { return err }
        key, keyOK := idField(id)
        if !keyOK { return errors.New("evidence ledger key malformed") }
        payload, err := codec.StrictDecodePayload(raw)
        if err != nil { return err }
        fields, ok := payload.([]any)
        if !ok { return errors.New("evidence ledger record shape malformed") }
        switch len(fields) {
        case 15: // Observation
            owningTask, ok := idField(fields[1])
            if !ok || !equalBytes(fields[0],id) { return errors.New("Observation authority malformed") }
            projected, _, err := deriver.Sum("cote.csf.id.observation",
                "cote.csf.schema.observation-id-preimage",1,
                []any{fields[1],fields[2],fields[3],fields[4]})
            if err!=nil || !bytes.Equal(projected[:],id) {
                return errors.New("Observation identity does not follow task/observer/role")
            }
            digest, _, err := deriver.Sum("cote.csf.digest.observation",
                "cote.csf.schema.observation.body",1,fields[:14])
            if err!=nil || !equalBytes(fields[14],digest[:]) {
                return errors.New("Observation digest mismatch")
            }
            if owningTask == taskID {
                return errors.New("invisible task has addressed Observation")
            }
            observationTasks[key]=owningTask
        case 11: // KnowledgeInput
            if !equalBytes(fields[0],id) { return errors.New("KnowledgeInput authority malformed") }
            ref,ok:=fields[7].([]any)
            if !ok || len(ref)!=3 || ref[0]!=uint64(16) {
                return errors.New("KnowledgeInput observation reference malformed")
            }
            observationID, ok := idField(ref[1])
            if !ok { return errors.New("KnowledgeInput observation ID malformed") }
            projected, _, err := deriver.Sum("cote.csf.id.knowledge-input",
                "cote.csf.schema.knowledge-input-id-preimage",1,
                []any{ref[1],fields[3],fields[5],fields[1],fields[2]})
            if err!=nil || !bytes.Equal(projected[:],id) {
                return errors.New("KnowledgeInput identity mismatch")
            }
            digest, _, err := deriver.Sum("cote.csf.digest.knowledge-input",
                "cote.csf.schema.knowledge-input.body",1,fields[:10])
            if err!=nil || !equalBytes(fields[10],digest[:]) {
                return errors.New("KnowledgeInput digest mismatch")
            }
            inputRefs=append(inputRefs,observationID)
        case 10: // Claim (CSFRV1-7). No direct Observation relation.
            return errors.New("claim evidence requires registered CSFRV1-7 reconciliation")
        default:
            return errors.New("unregistered evidence ledger root")
        }
    }
    if err := rows.Err(); err != nil { return err }
    for _, observationID := range inputRefs {
        if _,ok:=observationTasks[observationID];!ok {
            return errors.New("KnowledgeInput has no durable Observation")
        }
    }
    return nil
}

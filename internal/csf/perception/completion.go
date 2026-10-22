// Package perception owns the pure, canonical perception record identities.
// It has no access to the event store, agent cognition or global world truth.
package perception

import (
    "bytes"
    "errors"

    "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
)

// EmptyCompletion is only a record constructor, not an observer/access resolver.
// Its caller MUST first prove from the committed event and pinned resolver that
// the exact authorized output projection for this task is empty.
func EmptyCompletion(bundle *policy.Bundle, task ports.CanonicalRecord) (ports.CanonicalRecord, error) {
    var zero ports.CanonicalRecord
    if bundle == nil {
        return zero, errors.New("missing authenticated policy bundle")
    }
    decoded, err := codec.StrictDecodePayload(task.Canonical)
    if err != nil { return zero, err }
    fields, ok := decoded.([]any)
    if !ok || len(fields) != 10 || !bytes.Equal(task.Identity[:], asBytes(fields[0])) {
        return zero, errors.New("invalid canonical perception task identity or shape")
    }
    if fields[9] != uint64(0) {
        return zero, errors.New("completion requires PENDING task")
    }
    d := identity.New(bundle)
    id, _, err := d.Sum("cote.csf.id.perception-completion",
        "cote.csf.schema.perception-completion-id-preimage", 1, []any{task.Identity[:]})
    if err != nil { return zero, err }
    body := []any{id[:], task.Identity[:], []any{}, []any{}}
    digest, _, err := d.Sum("cote.csf.digest.perception-completion",
        "cote.csf.schema.perception-task-completion.body", 1, body)
    if err != nil { return zero, err }
    encoded, err := codec.CanonicalPayload(append(body, digest[:]))
    if err != nil { return zero, err }
    return ports.CanonicalRecord{Identity: [32]byte(id), Canonical: encoded}, nil
}

func asBytes(v any) []byte {
    raw, _ := v.([]byte)
    return raw
}

package sqlite

import (
	"context"
	"errors"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/genesis"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

// forkGenesisOccurrence is a narrowly versioned V1 migration of a pending
// genesis-owned schedule. Birth proof/eligibility/content is authenticated
// under the parent's checkpoint; identity and run are rederived under the
// child's Genesis. Terminal Event- and Claim-owned occurrences must use their
// own versioned migrators and are rejected instead of inheriting parent IDs.
type forkGenesisOccurrence struct {
	ID                 [32]byte
	EligibilityInstant int64
	EligibilityOrdinal uint64
	Canonical          []byte
}

func migrateGenesisPendingOccurrences(ctx context.Context, s *Store,
	parent *genesis.Genesis, child *genesis.Genesis, refs []any, originPolicy, childPolicy *policy.Bundle) (
	[]forkGenesisOccurrence, error) {
	result := []forkGenesisOccurrence{}
	for _, rawRef := range refs {
		ref, ok := rawRef.([]any)
		if !ok || len(ref) != 3 || ref[0] != uint64(6) {
			return nil, errors.New("fork pending occurrence ref is not canonical")
		}
		parentID, ok := ref[1].([]byte)
		if !ok || len(parentID) != 32 {
			return nil, errors.New("fork occurrence parent ID malformed")
		}
		birthDigest, ok := ref[2].([]byte)
		if !ok || len(birthDigest) != 32 {
			return nil, errors.New("fork occurrence checkpoint digest malformed")
		}
		var original []byte
		if err := s.db.QueryRowContext(ctx, "SELECT canonical FROM scheduled_occurrences WHERE id=? AND run_id=?",
			parentID, parent.RunID[:]).Scan(&original); err != nil {
			return nil, err
		}
		parsed, err := codec.StrictDecodePayload(original)
		if err != nil {
			return nil, err
		}
		// A valid roster reference must match an authenticated parent birth
		// before deriving a different child-owned identity.
		if _, _, _, _, err := originPolicy.CodecIdentity(); err != nil {
			return nil, err
		}
		f, ok := parsed.([]any)
		if !ok || len(f) != 15 || !equalBytes(f[0], parentID) || !equalBytes(f[1], parent.RunID[:]) ||
			!equalBytes(f[14], birthDigest) {
			return nil, errors.New("fork schedule birth hash differs from checkpoint")
		}
		root, ok := f[9].([]any)
		if !ok || len(root) != 2 || root[0] != uint64(2) {
			return nil, errors.New("fork schedule has no supported genesis origin")
		}
		source, ok := root[1].([]any)
		if !ok || len(source) != 2 || !equalBytes(source[0], parent.ID[:]) ||
			!equalBytes(source[1], parent.Digest[:]) {
			return nil, errors.New("fork genesis occurrence does not belong to verified parent")
		}
		birth, err := remintForkOccurrence(child, f, childPolicy)
		if err != nil {
			return nil, err
		}
		for _, prior := range result {
			if prior.ID == birth.ID {
				return nil, errors.New("two parent occurrences map to the same child ID")
			}
		}
		result = append(result, birth)
	}
	return result, nil
}

// Reuse the established Genesis rekey/digest/validation path for each
// authenticated V2 migration birth after canonical coordinate allocation.
func remintForkOccurrence(child *genesis.Genesis, f []any, childPolicy *policy.Bundle) (forkGenesisOccurrence, error) {
	var zero forkGenesisOccurrence
	if len(f) != 15 {
		return zero, errors.New("fork occurrence birth malformed")
	}
	// A migrated schedule is an *unexecuted* child-side work unit. Preserve
	// its original due coordinate, but never its parent run/event identity.
	sourceChild := []any{uint64(2), []any{child.ID[:], child.Digest[:]}}
	nextID, _, err := identity.New(childPolicy).Sum("cote.csf.id.occurrence", "cote.csf.schema.occurrence-id-preimage", 1,
		[]any{sourceChild, f[10], f[11]})
	if err != nil {
		return zero, err
	}
	fields := append([]any(nil), f[:14]...)
	fields[0] = nextID[:]
	fields[1] = child.RunID[:]
	fields[9] = sourceChild
	hash, _, err := identity.New(childPolicy).Sum("cote.csf.digest.occurrence-unit",
		"cote.csf.schema.scheduled-occurrence.body", 1, fields)
	if err != nil {
		return zero, err
	}
	bytesOut, err := codec.CanonicalPayload(append(fields, hash[:]))
	if err != nil {
		return zero, err
	}
	var childID [32]byte
	copy(childID[:], nextID[:])
	eligibility, life, err := validateScheduledOccurrence([32]byte(child.RunID), childID, bytesOut)
	if err != nil {
		return zero, err
	}
	if life != 0 {
		return zero, errors.New("fork migrated nonpending schedule")
	}
	if eligibility.NotBeforeInstant < child.InitialInstant {
		return zero, errors.New("fork would reintroduce a schedule before fork origin")
	}

	return forkGenesisOccurrence{ID: childID, EligibilityInstant: int64(eligibility.NotBeforeInstant), EligibilityOrdinal: uint64(eligibility.NotBeforeCycleOrdinal), Canonical: bytesOut}, nil
}

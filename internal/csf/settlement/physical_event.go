package settlement

import (
	"bytes"
	"errors"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/reducer"
)

// PhysicalAdvancedEvent is engine-only physical world truth. It is derived
// solely from a registered candidate and the CSF-owned causal coordinates.
// It cannot issue PerceptionTasks or publish on its own.
func PhysicalAdvancedEvent(run, cycle, candidate, source domain.ID, sourceDigest domain.Digest,
	instant domain.SimulationInstant, revision domain.WorldRevision,
	sequence domain.LogicalSequence, completed []any) (ports.CanonicalRecord, error) {
	var zero ports.CanonicalRecord
	if _, err := reducer.DecodePhysical(completed); err != nil {
		return zero, err
	}
	envelope, ok := completed[2].([]byte)
	if !ok {
		return zero, errors.New("physical EventDraft missing typed state")
	}
	hash := identity.OpaqueContentHash(envelope)
	if dig, ok := completed[3].([]byte); !ok || !bytes.Equal(dig, hash[:]) {
		return zero, errors.New("physical EventDraft typed digest mismatch")
	}
	key := []any{uint64(1), candidate[:], "physical.a1", uint64(1), uint64(0), uint64(0)}
	id, err := root("cote.csf.id.event", "cote.csf.schema.event-id-preimage",
		[]any{run[:], cycle[:], key})
	if err != nil {
		return zero, err
	}
	ref := []any{uint64(1), source[:], sourceDigest[:]}
	fields := []any{id[:], run[:], key, "physical.advanced", uint64(1),
		int64(instant), uint64(sequence), cycle[:], uint64(revision), completed,
		[]any{}, []any{ref}, []any{}, []any{}, []any{uint64(0)},
		uint64(2), "physical.a1", uint64(1)}
	digest, err := root("cote.csf.digest.event", "cote.csf.schema.event.body", fields)
	if err != nil {
		return zero, err
	}
	canonical, err := codec.CanonicalPayload(append(append([]any(nil), fields...), digest[:]))
	if err != nil {
		return zero, err
	}
	return ports.CanonicalRecord{Identity: [32]byte(id), Canonical: canonical}, nil
}

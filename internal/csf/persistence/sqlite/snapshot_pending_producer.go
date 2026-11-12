package sqlite

import (
	"errors"
	"fmt"
)

// snapshotPendingPartitions names all ten normative rosters at the producer
// boundary. Each slice contains the EXACT canonical record/reference shape
// from its original owner. Named fields are not independent world authority;
// the five ledger prefixes and replay proof remain authoritative.
type snapshotPendingPartitions struct {
	occurrences           []any
	triggerRuntime        []any
	triggerActivations    []any
	sourceClosures        []any
	rounds                []any
	dispatches            []any
	responses             []any
	pendingInputs         []any
	perceptionTasks       []any
	perceptionCompletions []any
}

func newSnapshotPendingPartitions() snapshotPendingPartitions {
	return snapshotPendingPartitions{
		occurrences: []any{}, triggerRuntime: []any{},
		triggerActivations: []any{}, sourceClosures: []any{},
		rounds: []any{}, dispatches: []any{},
		responses: []any{}, pendingInputs: []any{},
		perceptionTasks: []any{}, perceptionCompletions: []any{},
	}
}

// canonicalOrder is the ONLY conversion from named producer partitions into
// the CDDL positional wire value. Order and numeric indices match
// pendingSnapshotPartSpecs/decoder exactly; nil is never encoded as CBOR null.
// The maximum count is the existing normative schema limit.
func (p snapshotPendingPartitions) canonicalOrder() ([]any, error) {
	rosters := [10][]any{
		p.occurrences, p.triggerRuntime, p.triggerActivations,
		p.sourceClosures, p.rounds, p.dispatches, p.responses,
		p.pendingInputs, p.perceptionTasks, p.perceptionCompletions,
	}
	ordered := make([]any, len(rosters))
	for i, roster := range rosters {
		if roster == nil {
			return nil, fmt.Errorf("missing snapshot pending partition %s", pendingSnapshotPartSpecs[i].name)
		}
		if len(roster) > pendingSnapshotPartSpecs[i].max {
			return nil, fmt.Errorf("snapshot pending partition %s exceeds normative cardinality", pendingSnapshotPartSpecs[i].name)
		}
		ordered[i] = roster
	}
	return ordered, nil
}

// retainLiveSnapshotSources is a pure deterministic filter. In particular,
// the input ordering and digest bytes of scheduled occurrences / activations
// are unchanged. Input-ref kinds 2 and 3 are NOT the causal-ref kinds 6 and 7.
func retainLiveSnapshotSources(refs []any, inputKind uint64,
	live map[[32]byte]bool) (active, pending []any, err error) {
	if inputKind != 2 && inputKind != 3 {
		return nil, nil, errors.New("unsupported pending snapshot source kind")
	}
	wantCausalKind := uint64(6)
	if inputKind == 3 {
		wantCausalKind = 7
	}
	active = make([]any, 0, len(refs))
	pending = make([]any, 0, len(refs))
	for _, raw := range refs {
		ref, ok := raw.([]any)
		if !ok || len(ref) != 3 || ref[0] != wantCausalKind {
			return nil, nil, errors.New("malformed pending snapshot source reference")
		}
		id, ok := idField(ref[1])
		digest, digestOK := ref[2].([]byte)
		if !ok || !digestOK || len(digest) != 32 {
			return nil, nil, errors.New("malformed pending source ID or digest")
		}
		if live[id] {
			active = append(active, raw)
			pending = append(pending, []any{inputKind, ref[1], ref[2]})
		}
	}
	return active, pending, nil
}

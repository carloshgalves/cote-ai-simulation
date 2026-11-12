package sqlite

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/trigger"
)

// pendingSnapshotReference is a typed, owned copy of a normative causal-ref
// (or of the input-ref variant in the pending-units partition).
// Neither IDs nor digests alias the caller's CBOR byte buffers.
type pendingSnapshotReference struct {
	Kind                uint64
	SlotResponseKind    uint64
	HasSlotResponseKind bool
	ID                  [32]byte
	Digest              [32]byte
}

type pendingSnapshotTriggerRuntime struct {
	DefinitionID [32]byte
	Version      uint64
}

type pendingSnapshotCompletion struct {
	ID, TaskID, Digest [32]byte
}

// pendingSnapshotRosters is a read-only projection of the TEN normatively
// ordered partitions. The legacy arrays are decoded from owned bytes, not
// re-serialized; migration and publications remain separate responsibilities.
type pendingSnapshotRosters struct {
	Occurrences           []pendingSnapshotReference
	TriggerRuntimes       []pendingSnapshotTriggerRuntime
	TriggerActivations    []pendingSnapshotReference
	SourceClosures        []pendingSnapshotReference
	Rounds                []pendingSnapshotReference
	Dispatches            []pendingSnapshotReference
	Responses             []pendingSnapshotReference
	PendingInputs         []pendingSnapshotReference
	PerceptionTasks       []pendingSnapshotReference
	PerceptionCompletions []pendingSnapshotCompletion
	original              [10][]any
}

func (r pendingSnapshotRosters) legacyPartitions() []any {
	out := make([]any, len(r.original))
	for i := range r.original {
		out[i] = r.original[i]
	}
	return out
}

type pendingSnapshotPartSpec struct {
	name  string
	max   int
	kinds []uint64
}

var pendingSnapshotPartSpecs = [10]pendingSnapshotPartSpec{
	{"scheduled-occurrences", 65535, []uint64{6}},
	{"trigger-runtime", 65535, nil},
	{"trigger-activations", 65535, []uint64{7}},
	{"source-closures", 1024, []uint64{2}},
	{"rounds", 65535, []uint64{3}},
	{"dispatches", 65535, []uint64{4}},
	{"responses", 65535, []uint64{5, 23}},
	{"pending-units", 65535, []uint64{0, 1, 2, 3, 4}},
	{"perception-tasks", 65535, []uint64{15}},
	{"perception-completions", 65535, nil},
}

func pendingSnapshotID(raw any) ([32]byte, error) {
	var id [32]byte
	bytesValue, ok := raw.([]byte)
	if !ok || len(bytesValue) != 32 {
		return id, errors.New("expected exact 32-byte authority identity")
	}
	copy(id[:], bytesValue)
	return id, nil
}

func pendingSnapshotRef(raw any, part int) (pendingSnapshotReference, error) {
	var zero pendingSnapshotReference
	fields, ok := raw.([]any)
	if !ok || len(fields) < 3 {
		return zero, errors.New("reference must be a canonical array")
	}
	kind, ok := fields[0].(uint64)
	if !ok {
		return zero, errors.New("reference kind must be uint")
	}
	allowed := false
	for _, candidate := range pendingSnapshotPartSpecs[part].kinds {
		if kind == candidate {
			allowed = true
			break
		}
	}
	if !allowed {
		return zero, fmt.Errorf("reference kind %d not admitted to %s", kind, pendingSnapshotPartSpecs[part].name)
	}
	idIndex := 1
	if part == 7 && kind == 0 {
		if len(fields) != 4 {
			return zero, errors.New("slot-response input-ref arity invalid")
		}
		slotKind, ok := fields[1].(uint64)
		if !ok || slotKind > 255 {
			return zero, errors.New("slot-response input-ref variant invalid")
		}
		zero.HasSlotResponseKind = true
		zero.SlotResponseKind = slotKind
		idIndex = 2
	} else if len(fields) != 3 {
		return zero, errors.New("causal/input ref arity invalid")
	}
	id, err := pendingSnapshotID(fields[idIndex])
	if err != nil {
		return zero, err
	}
	digest, err := pendingSnapshotID(fields[idIndex+1])
	if err != nil {
		return zero, fmt.Errorf("reference digest: %w", err)
	}
	zero.Kind, zero.ID, zero.Digest = kind, id, digest
	return zero, nil
}

func decodeSnapshotPendingRosters(snapshot []any) (pendingSnapshotRosters, error) {
	var out pendingSnapshotRosters
	if len(snapshot) != 19 {
		return out, errors.New("historical snapshot malformed")
	}
	typed, ok := snapshot[11].([]any)
	if !ok || len(typed) != 4 || typed[0] != "cote.csf.schema.snapshot-pending-state" || typed[1] != uint64(1) {
		return out, errors.New("historical pending typed-value malformed")
	}
	raw, ok := typed[2].([]byte)
	if !ok {
		return out, errors.New("historical pending envelope absent")
	}
	claimedDigest, ok := typed[3].([]byte)
	if !ok || len(claimedDigest) != 32 {
		return out, errors.New("historical pending digest malformed")
	}
	expected := identity.OpaqueContentHash(raw)
	if !bytes.Equal(claimedDigest, expected[:]) {
		return out, errors.New("historical pending envelope digest mismatch")
	}
	env, err := codec.StrictDecodeEnvelope(raw)
	if err != nil {
		return out, err
	}
	if env.DomainTag != "cote.csf.snapshot.pending-state" || env.SchemaID != "cote.csf.schema.snapshot-pending-state" || env.SchemaVersion != 1 {
		return out, errors.New("historical pending envelope schema mismatch")
	}
	lists, ok := env.Value.([]any)
	if !ok || len(lists) != 10 {
		return out, errors.New("historical pending partition count must equal ten")
	}
	for part, untyped := range lists {
		items, ok := untyped.([]any)
		if !ok || len(items) > pendingSnapshotPartSpecs[part].max {
			return out, fmt.Errorf("historical pending partition %s malformed or oversized", pendingSnapshotPartSpecs[part].name)
		}
		out.original[part] = items
		seen := map[[32]byte]bool{}
		for _, item := range items {
			if part == 1 {
				f, ok := item.([]any)
				if !ok || len(f) != 8 {
					return out, errors.New("trigger-runtime snapshot projection malformed")
				}
				id, err := pendingSnapshotID(f[0])
				if err != nil {
					return out, fmt.Errorf("trigger-runtime definition: %w", err)
				}
				canonical, err := codec.CanonicalPayload(f)
				if err != nil {
					return out, err
				}
				runtimeValue, err := trigger.DecodeRuntime(canonical)
				if err != nil {
					return out, fmt.Errorf("historical trigger-runtime invalid: %w", err)
				}
				if [32]byte(runtimeValue.DefinitionID) != id {
					return out, errors.New("trigger-runtime ID differs from typed representation")
				}
				if seen[id] {
					return out, errors.New("duplicate trigger-runtime definition")
				}
				seen[id] = true
				out.TriggerRuntimes = append(out.TriggerRuntimes, pendingSnapshotTriggerRuntime{DefinitionID: id, Version: uint64(runtimeValue.Version)})
				continue
			}
			if part == 9 {
				f, ok := item.([]any)
				if !ok || len(f) != 5 {
					return out, errors.New("perception-completion projection malformed")
				}
				id, err := pendingSnapshotID(f[0])
				if err != nil {
					return out, err
				}
				task, err := pendingSnapshotID(f[1])
				if err != nil {
					return out, err
				}
				digest, err := pendingSnapshotID(f[4])
				if err != nil {
					return out, err
				}
				for _, index := range []int{2, 3} {
					ids, ok := f[index].([]any)
					if !ok || len(ids) > 65535 {
						return out, errors.New("perception-completion linked ID roster malformed")
					}
					for _, rawID := range ids {
						if _, err := pendingSnapshotID(rawID); err != nil {
							return out, fmt.Errorf("perception-completion linked ID: %w", err)
						}
					}
				}
				if seen[id] {
					return out, errors.New("duplicate perception completion")
				}
				seen[id] = true
				out.PerceptionCompletions = append(out.PerceptionCompletions, pendingSnapshotCompletion{id, task, digest})
				continue
			}
			ref, err := pendingSnapshotRef(item, part)
			if err != nil {
				return out, fmt.Errorf("pending %s: %w", pendingSnapshotPartSpecs[part].name, err)
			}
			if seen[ref.ID] {
				return out, fmt.Errorf("duplicate pending %s authority", pendingSnapshotPartSpecs[part].name)
			}
			seen[ref.ID] = true
			switch part {
			case 0:
				out.Occurrences = append(out.Occurrences, ref)
			case 2:
				out.TriggerActivations = append(out.TriggerActivations, ref)
			case 3:
				out.SourceClosures = append(out.SourceClosures, ref)
			case 4:
				out.Rounds = append(out.Rounds, ref)
			case 5:
				out.Dispatches = append(out.Dispatches, ref)
			case 6:
				out.Responses = append(out.Responses, ref)
			case 7:
				out.PendingInputs = append(out.PendingInputs, ref)
			case 8:
				out.PerceptionTasks = append(out.PerceptionTasks, ref)
			}
		}
	}
	return out, nil
}

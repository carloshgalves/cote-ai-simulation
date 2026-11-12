package sqlite

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
)

// snapshotPendingProvenance is a transient set of source witnesses collected
// from the *same SQLite transaction* as snapshotPending. It does not confer
// authority to caller-selected records: only the producer's existing run-
// scoped SQL and authenticated input/outbox readers may populate it.
type snapshotPendingProvenance struct {
	run       [32]byte
	witnesses map[snapshotPendingWitnessKey]snapshotPendingWitness
	links     map[snapshotPendingWitnessKey][32]byte
}

type snapshotPendingWitnessKey struct {
	owner string
	id    [32]byte
}

type snapshotPendingWitness struct {
	run       [32]byte
	kind      uint64
	digest    [32]byte
	canonical []byte
}

const (
	snapshotOwnerOccurrence   = "occurrences"
	snapshotOwnerRuntime      = "trigger-runtime"
	snapshotOwnerActivation   = "trigger-activations"
	snapshotOwnerClosure      = "source-closures"
	snapshotOwnerRound        = "rounds"
	snapshotOwnerDispatch     = "dispatches"
	snapshotOwnerResponse     = "responses"
	snapshotOwnerExogenous    = "exogenous-inputs"
	snapshotOwnerSlotResponse = "slot-responses"
	snapshotOwnerTask         = "perception-tasks"
	snapshotOwnerCompletion   = "perception-completions"
)

func newSnapshotPendingProvenance(run [32]byte) *snapshotPendingProvenance {
	return &snapshotPendingProvenance{
		run: run, witnesses: make(map[snapshotPendingWitnessKey]snapshotPendingWitness),
		links: make(map[snapshotPendingWitnessKey][32]byte),
	}
}

// bind binds the exact owner/identity, digest and source-run together once.
// The owning source row has already been decoded by the producer. A bad run
// or duplicate witness is a corruption error, never last-write-wins.
func (p *snapshotPendingProvenance) bind(owner string, sourceRun [32]byte,
	rawID []byte, kind uint64, digest, canonical []byte) error {
	if p == nil || p.witnesses == nil {
		return errors.New("snapshot provenance index uninitialized")
	}
	id, err := pendingSnapshotID(rawID)
	if err != nil {
		return err
	}
	if sourceRun != p.run {
		return errors.New("foreign-run snapshot source witness")
	}
	if !snapshotWitnessKindAllowed(owner, kind) {
		return errors.New("snapshot source owner/kind unregistered")
	}
	canonicalOwner := owner == snapshotOwnerRuntime || owner == snapshotOwnerCompletion
	if canonicalOwner {
		if len(canonical) == 0 || len(digest) != 0 {
			return errors.New("canonical source witness malformed")
		}
	} else if len(digest) != 32 || len(canonical) != 0 {
		return errors.New("source reference witness requires exact digest")
	}
	var d [32]byte
	copy(d[:], digest)
	key := snapshotPendingWitnessKey{owner: owner, id: id}
	if _, exists := p.witnesses[key]; exists {
		return fmt.Errorf("duplicate snapshot source witness %s", owner)
	}
	p.witnesses[key] = snapshotPendingWitness{run: sourceRun, kind: kind, digest: d, canonical: bytes.Clone(canonical)}
	return nil
}

func snapshotWitnessKindAllowed(owner string, kind uint64) bool {
	switch owner {
	case snapshotOwnerOccurrence:
		return kind == 6
	case snapshotOwnerRuntime:
		return kind == 0
	case snapshotOwnerActivation:
		return kind == 7
	case snapshotOwnerClosure:
		return kind == 2
	case snapshotOwnerRound:
		return kind == 3
	case snapshotOwnerDispatch:
		return kind == 4
	case snapshotOwnerResponse:
		return kind == 5 || kind == 23
	case snapshotOwnerExogenous:
		return kind == 1
	case snapshotOwnerSlotResponse:
		return kind == 0
	case snapshotOwnerTask:
		return kind == 15
	case snapshotOwnerCompletion:
		return kind == 0
	default:
		return false
	}
}

// bindLink captures an immutable dependency decoded from an authoritative
// response/completion row. The target witness may arrive later in ID order;
// verify requires the exact target after the source scan is complete.
func (p *snapshotPendingProvenance) bindLink(owner string, rawID, targetID []byte) error {
	if p == nil || p.witnesses == nil || p.links == nil {
		return errors.New("snapshot provenance links uninitialized")
	}
	if owner != snapshotOwnerResponse && owner != snapshotOwnerCompletion {
		return errors.New("snapshot provenance link owner unregistered")
	}
	id, err := pendingSnapshotID(rawID)
	if err != nil {
		return err
	}
	target, err := pendingSnapshotID(targetID)
	if err != nil {
		return err
	}
	key := snapshotPendingWitnessKey{owner: owner, id: id}
	if _, ok := p.witnesses[key]; !ok {
		return errors.New("snapshot source link has no witness")
	}
	if _, already := p.links[key]; already {
		return errors.New("duplicate snapshot source link")
	}
	p.links[key] = target
	return nil
}

func snapshotOwnerForInputRef(kind uint64) (string, error) {
	switch kind {
	case 0:
		return snapshotOwnerSlotResponse, nil
	case 1:
		return snapshotOwnerExogenous, nil
	case 2:
		return snapshotOwnerOccurrence, nil
	case 3:
		return snapshotOwnerActivation, nil
	case 4:
		return snapshotOwnerRound, nil
	default:
		return "", errors.New("unregistered pending input provenance kind")
	}
}

func snapshotOwnerForPart(part int, kind uint64) (string, error) {
	switch part {
	case 0:
		return snapshotOwnerOccurrence, nil
	case 1:
		return snapshotOwnerRuntime, nil
	case 2:
		return snapshotOwnerActivation, nil
	case 3:
		return snapshotOwnerClosure, nil
	case 4:
		return snapshotOwnerRound, nil
	case 5:
		return snapshotOwnerDispatch, nil
	case 6:
		return snapshotOwnerResponse, nil
	case 7:
		return snapshotOwnerForInputRef(kind)
	case 8:
		return snapshotOwnerTask, nil
	case 9:
		return snapshotOwnerCompletion, nil
	default:
		return "", errors.New("unregistered snapshot provenance partition")
	}
}

// verify proves every emitted reference is bound to its source witness and
// expected run, not just to a structurally valid CBOR array. It also proves
// the exact cross-roster link for each pending round/occurrence/activation.
// Historical source records remain the authority; these witnesses are
// ephemeral and never written to the snapshot or transmitted to children.
func (p *snapshotPendingProvenance) verify(parts snapshotPendingPartitions) error {
	if p == nil {
		return errors.New("snapshot provenance index missing")
	}
	rosters, err := parts.canonicalOrder()
	if err != nil {
		return err
	}
	linked := map[snapshotPendingWitnessKey]bool{}
	required := map[snapshotPendingWitnessKey]bool{}
	pendingDispatches := map[[32]byte]bool{}
	pendingTasks := map[[32]byte]bool{}
	answeredDispatches := map[[32]byte]bool{}
	completedTasks := map[[32]byte]bool{}
	for part, rawRoster := range rosters {
		roster := rawRoster.([]any)
		seen := map[[32]byte]bool{}
		for _, raw := range roster {
			var id [32]byte
			var kind uint64
			var digest [32]byte
			var canonical []byte
			if part == 1 || part == 9 {
				fields, ok := raw.([]any)
				n := 8
				if part == 9 {
					n = 5
				}
				if !ok || len(fields) != n {
					return fmt.Errorf("snapshot provenance %s canonical shape malformed", pendingSnapshotPartSpecs[part].name)
				}
				id, err = pendingSnapshotID(fields[0])
				if err != nil {
					return err
				}
				canonical, err = codec.CanonicalPayload(fields)
				if err != nil {
					return err
				}
			} else {
				ref, e := pendingSnapshotRef(raw, part)
				if e != nil {
					return fmt.Errorf("snapshot provenance %s: %w", pendingSnapshotPartSpecs[part].name, e)
				}
				id, kind, digest = ref.ID, ref.Kind, ref.Digest
			}
			if seen[id] {
				return fmt.Errorf("snapshot provenance %s identity duplicated", pendingSnapshotPartSpecs[part].name)
			}
			seen[id] = true
			owner, e := snapshotOwnerForPart(part, kind)
			if e != nil {
				return e
			}
			key := snapshotPendingWitnessKey{owner: owner, id: id}
			witness, ok := p.witnesses[key]
			if !ok {
				return fmt.Errorf("snapshot provenance %s source witness absent", pendingSnapshotPartSpecs[part].name)
			}
			sourceKind := kind
			if part == 7 {
				// The pending-units partition uses input-ref kinds, not the causal
				// reference kind of its authenticated parent roster.
				switch kind {
				case 2:
					sourceKind = 6
				case 3:
					sourceKind = 7
				case 4:
					sourceKind = 3
				}
			}
			if witness.run != p.run || witness.kind != sourceKind {
				return fmt.Errorf("snapshot provenance %s foreign source/kind", pendingSnapshotPartSpecs[part].name)
			}
			if part == 1 || part == 9 {
				if !bytes.Equal(canonical, witness.canonical) {
					return fmt.Errorf("snapshot provenance %s canonical source differs", pendingSnapshotPartSpecs[part].name)
				}
			} else if witness.digest != digest {
				return fmt.Errorf("snapshot provenance %s source digest differs", pendingSnapshotPartSpecs[part].name)
			}
			if part == 5 {
				pendingDispatches[id] = true
			}
			if part == 8 {
				pendingTasks[id] = true
			}
			if part == 6 || part == 9 {
				target, found := p.links[key]
				if !found {
					return fmt.Errorf("snapshot provenance %s source link missing", pendingSnapshotPartSpecs[part].name)
				}
				if part == 6 {
					if pendingDispatches[target] || answeredDispatches[target] {
						return errors.New("snapshot response contradicts pending or already answered dispatch")
					}
					answeredDispatches[target] = true
				} else {
					if pendingTasks[target] || completedTasks[target] {
						return errors.New("snapshot completion contradicts pending or completed task")
					}
					completedTasks[target] = true
				}
				targetOwner := snapshotOwnerDispatch
				if part == 9 {
					targetOwner = snapshotOwnerTask
					fields := raw.([]any)
					taskID, e := pendingSnapshotID(fields[1])
					if e != nil || taskID != target {
						return errors.New("snapshot completion target differs from source task")
					}
				}
				targetWitness, ok := p.witnesses[snapshotPendingWitnessKey{owner: targetOwner, id: target}]
				if !ok || targetWitness.run != p.run {
					return fmt.Errorf("snapshot provenance %s parent source absent/foreign", pendingSnapshotPartSpecs[part].name)
				}
			}
			if part == 0 || part == 2 || part == 4 {
				required[key] = true
			}
			if part == 7 && (kind == 2 || kind == 3 || kind == 4) {
				linked[key] = true
			}
		}
	}
	if len(required) != len(linked) {
		return errors.New("snapshot pending input source-link count mismatch")
	}
	for key := range required {
		if !linked[key] {
			return fmt.Errorf("snapshot pending input missing matching %s authority", key.owner)
		}
	}
	return nil
}

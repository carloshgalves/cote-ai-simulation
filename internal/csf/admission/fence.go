// Package admission derives and persists the deterministic pre-evaluation cut.
package admission

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/inputledger"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
)

var ErrWaitingForClosure = errors.New("waiting for exogenous source closure")
var ErrWaitingForSlot = errors.New("waiting for slot response")

type Cycle struct {
	ID            domain.ID
	Attempt       uint32
	Instant       domain.SimulationInstant
	Ordinal       domain.CycleOrdinal
	BaseRevision  domain.WorldRevision
	BaseStateHash domain.Digest
	RetryRef      any // absent [0], or present [1, causal-ref]
}

type Policies struct {
	AdmissionOrder any
	CausalIdentity any
	Fence          any
	Rules          [][]any
}

type Fence struct {
	ID          domain.ID
	Digest      domain.Digest
	Fields      []any
	Canonical   []byte
	InputDigest domain.Digest
}

type FenceLog struct {
	store ports.AdmissionFenceStore
}

func NewFenceLog(store ports.AdmissionFenceStore) *FenceLog { return &FenceLog{store: store} }

func Derive(bundle *policy.Bundle, ledger *inputledger.Ledger, cycle Cycle, policies Policies) (Fence, error) {
	sources, inputs, closures, rounds, slots, runID := ledger.Snapshot()
	d := identity.New(bundle)
	if err := verifyInputs(d, inputs); err != nil {
		return Fence{}, err
	}
	if err := verifyClosures(d, closures); err != nil {
		return Fence{}, err
	}
	if err := verifyRounds(d, rounds); err != nil {
		return Fence{}, err
	}
	if err := verifyResponses(d, slots); err != nil {
		return Fence{}, err
	}
	cycleAt := domain.EligibilityCoordinate{NotBeforeInstant: cycle.Instant, NotBeforeCycleOrdinal: cycle.Ordinal}
	proofs := make([]any, 0, len(sources))
	for _, source := range sources {
		var selected *inputledger.StoredClosure
		for i := range closures {
			c := &closures[i]
			if c.Receipt.SourceID != source {
				continue
			}
			if c.Fields[5] == true || coordinate(c.Fields[4]).Compare(cycleAt) >= 0 {
				if selected == nil || c.Receipt.Sequence < selected.Receipt.Sequence {
					selected = c
				}
			}
		}
		if selected == nil {
			return Fence{}, fmt.Errorf("%w: %s", ErrWaitingForClosure, source)
		}
		proofs = append(proofs, []any{source, []any{uint64(2), selected.ID[:], selected.Digest[:]}, selected.Receipt.Sequence, selected.Fields[4], selected.Fields[5]})
	}
	eligibleRounds := make([]inputledger.Round, 0)
	for _, round := range rounds {
		if round.Lifecycle == 0 && round.Eligibility.Compare(cycleAt) <= 0 {
			eligibleRounds = append(eligibleRounds, round)
		}
	}
	sort.Slice(eligibleRounds, func(i, j int) bool { return bytes.Compare(eligibleRounds[i].ID[:], eligibleRounds[j].ID[:]) < 0 })
	roundIDs := make([]any, len(eligibleRounds))
	cohortRounds := make([]any, len(eligibleRounds))
	units := make([]unit, 0, len(inputs))
	for _, round := range eligibleRounds {
		idx := indexRound(eligibleRounds, round)
		roundIDs[idx] = round.ID[:]
		responseMap := slots[round.ID]
		cohortSlots := make([]any, 0, len(round.Slots))
		for _, slot := range round.Slots {
			response, ok := responseMap[slot]
			if !ok {
				return Fence{}, fmt.Errorf("%w: round %x slot %s", ErrWaitingForSlot, round.ID[:], slot)
			}
			if response.RunID != runID || response.CycleID != cycle.ID || response.EffectiveAt != cycle.Instant || response.SubmittedAgainstRevision != cycle.BaseRevision {
				return Fence{}, fmt.Errorf("slot response does not match admission cycle: round %x slot %s", round.ID[:], slot)
			}
			kind := uint64(5)
			if response.Kind == 1 {
				kind = 23
			}
			ref := []any{kind, response.ID[:], response.Digest[:]}
			cohortSlots = append(cohortSlots, []any{slot, ref})
			units = append(units, unit{eligibility: round.Eligibility, kind: 0, source: round.SourceID, id: response.ID, digest: response.Digest})
		}
		cohortRounds[idx] = []any{round.ID[:], cohortSlots}
	}
	cohortID, _, err := d.Sum("cote.csf.id.cohort", "cote.csf.schema.cohort-id-preimage", 1, []any{cycle.ID[:], uint64(cycle.BaseRevision), roundIDs})
	if err != nil {
		return Fence{}, err
	}
	cohort := []any{cohortID[:], cohortRounds}
	for _, input := range inputs {
		eligibility := coordinate(input.Fields[9])
		if eligibility.Compare(cycleAt) <= 0 {
			units = append(units, unit{eligibility: eligibility, kind: 1, source: input.Receipt.SourceID, id: input.ID, digest: input.Digest})
		}
	}
	sort.Slice(units, func(i, j int) bool { return units[i].less(units[j]) })
	unitValues := make([]any, len(units))
	for i, u := range units {
		unitValues[i] = []any{coordValue(u.eligibility), uint64(u.kind), u.source, u.id[:], u.digest[:]}
	}
	inputDigest, _, err := d.Sum("cote.csf.digest.input", "cote.csf.schema.admitted-unit-list", 1, unitValues)
	if err != nil {
		return Fence{}, err
	}
	cyclePlan := []any{uint64(0)} // WORK
	fields := []any{runID[:], cycle.ID[:], uint64(cycle.Attempt), cycle.RetryRef, int64(cycle.Instant), uint64(cycle.Ordinal), cyclePlan, uint64(cycle.BaseRevision), cycle.BaseStateHash[:], proofs, cohort, unitValues, inputDigest[:], policies.AdmissionOrder, policies.CausalIdentity, policies.Fence, policies.Rules}
	digest, _, err := d.Sum("cote.csf.digest.fence", "cote.csf.schema.admission-fence.body", 1, fields)
	if err != nil {
		return Fence{}, err
	}
	id, _, err := d.Sum("cote.csf.id.admission-fence", "cote.csf.schema.admission-fence-id-preimage", 1, []any{runID[:], cycle.ID[:], uint64(cycle.Attempt)})
	if err != nil {
		return Fence{}, err
	}
	full := append(append([]any{}, fields...), digest[:])
	canonical, err := codec.CanonicalPayload(full)
	if err != nil {
		return Fence{}, err
	}
	var fenceID domain.ID
	copy(fenceID[:], id[:])
	return Fence{ID: fenceID, Digest: digest, Fields: full, Canonical: canonical, InputDigest: inputDigest}, nil
}

// VerifyFence re-derives the fence from one coherent ledger prefix and compares
// every persisted byte, including all digest-bearing topology fields.
func VerifyFence(bundle *policy.Bundle, ledger *inputledger.Ledger, cycle Cycle, policies Policies, candidate Fence) error {
	derived, err := Derive(bundle, ledger, cycle, policies)
	if err != nil {
		return err
	}
	if candidate.ID != derived.ID || candidate.Digest != derived.Digest || candidate.InputDigest != derived.InputDigest || !bytes.Equal(candidate.Canonical, derived.Canonical) {
		return errors.New("AdmissionFence differs from its ledger derivation")
	}
	return nil
}

func verifyInputs(d identity.Deriver, records []inputledger.StoredInput) error {
	for _, record := range records {
		f := record.Fields
		if len(f) != 13 {
			return errors.New("malformed ExogenousInput record")
		}
		if _, ok := parseCoordinate(f[9]); !ok {
			return errors.New("malformed ExogenousInput eligibility coordinate")
		}
		id, _, err := d.Sum("cote.csf.id.exogenous-input", "cote.csf.schema.exogenous-input-id-preimage", 1, []any{f[1], f[2], f[3]})
		if err != nil {
			return err
		}
		if !matchesBytes(id[:], f[0]) || !bytes.Equal(id[:], record.ID[:]) {
			return errors.New("ExogenousInput identity mismatch")
		}
		digest, _, err := d.Sum("cote.csf.digest.exogenous-input-unit", "cote.csf.schema.exogenous-input.body", 1, []any{f[:12]})
		if err != nil {
			return err
		}
		if !matchesBytes(digest[:], f[12]) || !bytes.Equal(digest[:], record.Digest[:]) {
			return errors.New("ExogenousInput unit digest mismatch")
		}
		canonical, err := codec.CanonicalPayload(f)
		if err != nil {
			return err
		}
		if !bytes.Equal(canonical, record.Canonical) {
			return errors.New("ExogenousInput canonical bytes mismatch")
		}
		if record.Receipt.SourceID != f[2] {
			return errors.New("ExogenousInput receipt source mismatch")
		}
		if !matchesBytes(record.ID[:], record.Receipt.RecordRef[1]) || !matchesBytes(record.Digest[:], record.Receipt.RecordRef[2]) || record.Receipt.RecordRef[0] != uint64(1) {
			return errors.New("ExogenousInput receipt reference mismatch")
		}
		receipt, err := codec.CanonicalBytes("cote.csf.record.source-ingress-receipt", "cote.csf.schema.source-ingress-receipt", 1, []any{record.Receipt.SourceID, record.Receipt.Sequence, []any{uint64(1), record.ID[:], record.Digest[:]}})
		if err != nil {
			return err
		}
		if !bytes.Equal(receipt, record.Receipt.Canonical) {
			return errors.New("ExogenousInput receipt bytes mismatch")
		}
	}
	return nil
}

func verifyClosures(d identity.Deriver, records []inputledger.StoredClosure) error {
	for _, record := range records {
		f := record.Fields
		if len(f) != 7 {
			return errors.New("malformed SourceClosure record")
		}
		if _, ok := parseCoordinate(f[4]); !ok {
			return errors.New("malformed SourceClosure coordinate")
		}
		id, _, err := d.Sum("cote.csf.id.source-closure", "cote.csf.schema.source-closure-id-preimage", 1, []any{f[1], f[2], f[3]})
		if err != nil {
			return err
		}
		if !matchesBytes(id[:], f[0]) || !bytes.Equal(id[:], record.ID[:]) {
			return errors.New("SourceClosure identity mismatch")
		}
		digest, _, err := d.Sum("cote.csf.digest.source-closure", "cote.csf.schema.source-closure", 1, f)
		if err != nil {
			return err
		}
		if !bytes.Equal(digest[:], record.Digest[:]) {
			return errors.New("SourceClosure digest mismatch")
		}
		canonical, err := codec.CanonicalPayload(f)
		if err != nil {
			return err
		}
		if !bytes.Equal(canonical, record.Canonical) {
			return errors.New("SourceClosure canonical bytes mismatch")
		}
		if record.Receipt.SourceID != f[2] || record.Receipt.Sequence != f[6] {
			return errors.New("SourceClosure receipt coordinate mismatch")
		}
		if !matchesBytes(record.ID[:], record.Receipt.RecordRef[1]) || !matchesBytes(record.Digest[:], record.Receipt.RecordRef[2]) || record.Receipt.RecordRef[0] != uint64(2) {
			return errors.New("SourceClosure receipt reference mismatch")
		}
		receipt, err := codec.CanonicalBytes("cote.csf.record.source-ingress-receipt", "cote.csf.schema.source-ingress-receipt", 1, []any{record.Receipt.SourceID, record.Receipt.Sequence, []any{uint64(2), record.ID[:], record.Digest[:]}})
		if err != nil {
			return err
		}
		if !bytes.Equal(receipt, record.Receipt.Canonical) {
			return errors.New("SourceClosure receipt bytes mismatch")
		}
	}
	return nil
}

func verifyRounds(d identity.Deriver, records []inputledger.Round) error {
	for _, record := range records {
		f := record.Fields
		if len(f) != 11 {
			return errors.New("malformed RoundDeclaration record")
		}
		if _, ok := parseCoordinate(f[5]); !ok {
			return errors.New("malformed RoundDeclaration eligibility coordinate")
		}
		id, _, err := d.Sum("cote.csf.id.decision-round", "cote.csf.schema.decision-round-id-preimage", 1, []any{f[6], f[7], f[8]})
		if err != nil {
			return err
		}
		if !matchesBytes(id[:], f[0]) || !bytes.Equal(id[:], record.ID[:]) {
			return errors.New("RoundDeclaration identity mismatch")
		}
		digest, _, err := d.Sum("cote.csf.digest.round-declaration", "cote.csf.schema.round-declaration", 1, f)
		if err != nil {
			return err
		}
		if !bytes.Equal(digest[:], record.Digest[:]) {
			return errors.New("RoundDeclaration digest mismatch")
		}
		canonical, err := codec.CanonicalPayload(f)
		if err != nil {
			return err
		}
		if !bytes.Equal(canonical, record.Canonical) {
			return errors.New("RoundDeclaration canonical bytes mismatch")
		}
	}
	return nil
}

func verifyResponses(d identity.Deriver, slots map[domain.ID]map[string]inputledger.Response) error {
	for _, bySlot := range slots {
		for _, response := range bySlot {
			f := response.Body
			id, _, err := d.Sum(map[bool]string{true: "cote.csf.id.action-proposal", false: "cote.csf.id.no-proposal"}[response.Kind == 0], "cote.csf.schema.slot-response-id-preimage", 1, []any{response.DispatchID[:], uint64(response.Kind)})
			if err != nil {
				return err
			}
			if !bytes.Equal(id[:], response.ID[:]) {
				return errors.New("SlotResponse identity mismatch")
			}
			digestSchema, bodySchema := "cote.csf.digest.no-proposal-unit", "cote.csf.schema.no-proposal.body"
			if response.Kind == 0 {
				digestSchema, bodySchema = "cote.csf.digest.action-proposal-unit", "cote.csf.schema.action-proposal.body"
			}
			digest, _, err := d.Sum(digestSchema, bodySchema, 1, []any{f})
			if err != nil {
				return err
			}
			if !bytes.Equal(digest[:], response.Digest[:]) {
				return errors.New("SlotResponse digest mismatch")
			}
			canonical, err := codec.CanonicalPayload(append(append([]any(nil), f...), digest[:]))
			if err != nil {
				return err
			}
			if !bytes.Equal(canonical, response.Canonical) {
				return errors.New("SlotResponse canonical bytes mismatch")
			}
		}
	}
	return nil
}

func matchesBytes(expected []byte, value any) bool {
	actual, ok := value.([]byte)
	return ok && bytes.Equal(expected, actual)
}

type unit struct {
	eligibility domain.EligibilityCoordinate
	kind        uint8
	source      string
	id          domain.ID
	digest      domain.Digest
}

func (u unit) less(v unit) bool {
	if c := u.eligibility.Compare(v.eligibility); c != 0 {
		return c < 0
	}
	if u.kind != v.kind {
		return u.kind < v.kind
	}
	if u.source != v.source {
		return u.source < v.source
	}
	if c := bytes.Compare(u.digest[:], v.digest[:]); c != 0 {
		return c < 0
	}
	return bytes.Compare(u.id[:], v.id[:]) < 0
}
func coordValue(c domain.EligibilityCoordinate) []any {
	return []any{int64(c.NotBeforeInstant), uint64(c.NotBeforeCycleOrdinal)}
}
func coordinate(v any) domain.EligibilityCoordinate {
	c, _ := parseCoordinate(v)
	return c
}
func parseCoordinate(value any) (domain.EligibilityCoordinate, bool) {
	a, ok := value.([]any)
	if !ok || len(a) != 2 {
		return domain.EligibilityCoordinate{}, false
	}
	var instant int64
	switch n := a[0].(type) {
	case int64:
		instant = n
	case uint64:
		if n > uint64(^uint64(0)>>1) {
			return domain.EligibilityCoordinate{}, false
		}
		instant = int64(n)
	default:
		return domain.EligibilityCoordinate{}, false
	}
	var ordinal uint64
	switch n := a[1].(type) {
	case uint64:
		ordinal = n
	case int64:
		if n < 0 {
			return domain.EligibilityCoordinate{}, false
		}
		ordinal = uint64(n)
	default:
		return domain.EligibilityCoordinate{}, false
	}
	return domain.EligibilityCoordinate{NotBeforeInstant: domain.SimulationInstant(instant), NotBeforeCycleOrdinal: domain.CycleOrdinal(ordinal)}, true
}
func indexRound(rounds []inputledger.Round, target inputledger.Round) int {
	for i := range rounds {
		if rounds[i].ID == target.ID {
			return i
		}
	}
	return 0
}

// AppendIfAbsent is a compare-and-append boundary: the exact bytes are durable
// before the caller can invoke any validator or handler.
func (l *FenceLog) AppendIfAbsent(ctx context.Context, fence Fence) error {
	if l.store == nil {
		return errors.New("admission fence store is required")
	}
	return l.store.AppendFence(ctx, ports.CanonicalRecord{Identity: fence.ID, Canonical: fence.Canonical})
}

func (l *FenceLog) Get(ctx context.Context, id domain.ID) ([]byte, error) {
	if l.store == nil {
		return nil, errors.New("admission fence store is required")
	}
	return l.store.GetFence(ctx, id)
}

// Coordinator persists the fence before invoking evaluation.
type Coordinator struct{ log *FenceLog }

func NewCoordinator(log *FenceLog) *Coordinator { return &Coordinator{log: log} }
func (c *Coordinator) AdmitThenEvaluate(ctx context.Context, bundle *policy.Bundle, ledger *inputledger.Ledger, cycle Cycle, policies Policies, evaluate func(Fence) error) (Fence, error) {
	var fence Fence
	err := ledger.WithAdmissionBoundary(func() error {
		view, prefix, err := ledger.AdmissionView(ctx, bundle)
		if err != nil { return err }
		derived, err := Derive(bundle, view, cycle, policies)
		if err != nil { return err }
		if c.log == nil || c.log.store == nil { return errors.New("admission fence store is required") }
		if conditional, ok := c.log.store.(ports.PrefixConditionalFenceStore); ok {
			if err := conditional.AppendFenceIfPrefix(ctx, ports.CanonicalRecord{Identity: derived.ID, Canonical: derived.Canonical}, prefix); err != nil { return err }
		} else {
			// Compatibility with in-memory harness stores. Production SQLite
			// uses the cross-process prefix-conditional transaction above.
			if err := c.log.AppendIfAbsent(ctx, derived); err != nil { return err }
		}
		fence = derived
		return nil
	})
	if err != nil { return Fence{}, err }
	if evaluate != nil {
		if err := evaluate(fence); err != nil { return fence, err }
	}
	return fence, nil
}

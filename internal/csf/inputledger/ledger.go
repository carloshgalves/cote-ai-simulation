// Package inputledger owns exogenous ingress and slot completion. It deliberately
// keeps append order as receipt metadata; causal identities and fence ordering
// are derived from the records themselves.
package inputledger

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"sync"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
)

var (
	ErrUnknownSource            = errors.New("causal source is not declared")
	ErrStoreRequired            = errors.New("durable input ledger store is required")
	ErrConditionalStoreRequired = errors.New("input ledger requires conditional slot storage")
	ErrIngressStoreRequired     = errors.New("input ledger requires atomic source receipt storage")
	ErrFinalSource              = errors.New("causal source is final")
	ErrSlotMismatch             = errors.New("slot response does not match its open dispatch")
	ErrSlotFilled               = errors.New("slot already has a response")
	ErrSlotRevoked              = errors.New("slot dispatch was revoked")
)

type Source struct {
	ID string
}

type Input struct {
	ProducerUnitKey     string
	Kind                string
	SchemaVersion       uint32
	Actor               any // absent [0], or present [1, [namespace, actor-id]]
	Payload             any // typed-value
	DeclaredEffectiveAt domain.SimulationInstant
	Eligibility         domain.EligibilityCoordinate
	Provenance          any // absent [0], or present [1, causal-ref]
	IdempotencyKey      any // absent [0], or present [1, machine-id]
}

type Closure struct {
	ProducerKey   string
	ClosedThrough domain.EligibilityCoordinate
	Final         bool
}

type IngressReceipt struct {
	SourceID  string
	Sequence  uint64
	RecordRef [3]any
	Canonical []byte
}

type StoredInput struct {
	ID        domain.ID
	Digest    domain.Digest
	Fields    []any
	Canonical []byte
	Receipt   IngressReceipt
}

type StoredClosure struct {
	ID        domain.ID
	Digest    domain.Digest
	Fields    []any
	Canonical []byte
	Receipt   IngressReceipt
}

type Round struct {
	ID             domain.ID
	SourceID       string
	Actor          [2]string
	Slots          []string
	Eligibility    domain.EligibilityCoordinate
	CreatedBy      any
	Role           uint16
	LocalOrdinal   uint32
	Lifecycle      uint8
	IdempotencyKey any
	Digest         domain.Digest
	Fields         []any
	Canonical      []byte
}

// Ledger is the serialized owner of source prefixes and slot lifecycles. The
// optional stores are separated by authority; each causal record is canonical.
type Ledger struct {
	mu          sync.Mutex
	runID       domain.ID
	deriver     identity.Deriver
	sources     map[string]bool
	cursors     map[string]uint64
	inputs      map[string][]StoredInput
	closures    map[string][]StoredClosure
	inputByID   map[domain.ID]StoredInput
	closureByID map[domain.ID]StoredClosure
	rounds      map[domain.ID]Round
	providers   map[string]map[string]*slotState
	inputStore  ports.AppendOnlyStore
}

type slotState struct {
	Dispatch *Dispatch
	Revoked  bool
	Response *Response
}

// Dispatch is the complete normative SlotDispatch value.
type Dispatch struct {
	ID            domain.ID
	RunID         domain.ID
	CycleID       domain.ID
	RoundID       domain.ID
	SlotID        string
	Ordinal       uint32
	Actor         [2]string
	Eligibility   domain.EligibilityCoordinate
	BaseRevision  domain.WorldRevision
	EffectiveAt   domain.SimulationInstant
	RequestDigest domain.Digest
	Digest        domain.Digest
	Canonical     []byte
}

// Response holds the canonical fields (without the trailing response digest).
// Kind is 0 for ActionProposal and 1 for NoProposal.
type Response struct {
	Kind                     uint8
	ID                       domain.ID
	DispatchID               domain.ID
	RunID                    domain.ID
	CycleID                  domain.ID
	RoundID                  domain.ID
	SlotID                   string
	Actor                    [2]string
	EffectiveAt              domain.SimulationInstant
	SubmittedAgainstRevision domain.WorldRevision
	Body                     []any
	Digest                   domain.Digest
	Canonical                []byte
}

func New(runID domain.ID, bundle *policy.Bundle, inputStore ports.AppendOnlyStore) *Ledger {
	return &Ledger{runID: runID, deriver: identity.New(bundle), sources: map[string]bool{}, cursors: map[string]uint64{}, inputs: map[string][]StoredInput{}, closures: map[string][]StoredClosure{}, inputByID: map[domain.ID]StoredInput{}, closureByID: map[domain.ID]StoredClosure{}, rounds: map[domain.ID]Round{}, providers: map[string]map[string]*slotState{}, inputStore: inputStore}
}

// DeclareRound admits only a round whose origin is already causal (genesis or
// committed event). It does not create a round from an exogenous input.
func (l *Ledger) DeclareRound(ctx context.Context, sourceID string, actor [2]string, slots []string, eligibility domain.EligibilityCoordinate, createdBy any, role uint16, ordinal uint32, lifecycle uint8, idempotencyKey any) (Round, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.inputStore == nil {
		return Round{}, ErrStoreRequired
	}
	if err := domain.ValidateMachineID(sourceID); err != nil {
		return Round{}, err
	}
	if err := domain.ValidateMachineID(actor[0]); err != nil {
		return Round{}, err
	}
	if err := domain.ValidateMachineID(actor[1]); err != nil {
		return Round{}, err
	}
	if len(slots) == 0 || len(slots) > 1024 {
		return Round{}, fmt.Errorf("round must declare 1..1024 slots")
	}
	canonicalSlots := append([]string(nil), slots...)
	for _, slot := range canonicalSlots {
		if err := domain.ValidateMachineID(slot); err != nil {
			return Round{}, err
		}
	}
	sort.Strings(canonicalSlots)
	for i := 1; i < len(canonicalSlots); i++ {
		if canonicalSlots[i-1] == canonicalSlots[i] {
			return Round{}, fmt.Errorf("duplicate slot %q", canonicalSlots[i])
		}
	}
	if !validRoundOrigin(createdBy) {
		return Round{}, fmt.Errorf("round origin must be an event or genesis reference")
	}
	if lifecycle > 2 {
		return Round{}, fmt.Errorf("unknown round lifecycle")
	}
	id, _, err := l.deriver.Sum("cote.csf.id.decision-round", "cote.csf.schema.decision-round-id-preimage", 1, []any{createdBy, uint64(role), uint64(ordinal)})
	if err != nil {
		return Round{}, err
	}
	fields := []any{id[:], l.runID[:], sourceID, []any{actor[0], actor[1]}, stringList(canonicalSlots), coordValue(eligibility), createdBy, uint64(role), uint64(ordinal), uint64(lifecycle), idempotencyKey}
	digest, _, err := l.deriver.Sum("cote.csf.digest.round-declaration", "cote.csf.schema.round-declaration", 1, fields)
	if err != nil {
		return Round{}, err
	}
	canonical, err := codec.CanonicalPayload(fields)
	if err != nil {
		return Round{}, err
	}
	var roundID domain.ID
	copy(roundID[:], id[:])
	if prior, exists := l.rounds[roundID]; exists {
		if !bytes.Equal(prior.Canonical, canonical) {
			return Round{}, domain.ErrIdentityCollision
		}
		return prior, nil
	}
	if l.inputStore != nil {
		if err := l.inputStore.Append(ctx, ports.CanonicalRecord{Identity: roundID, Canonical: canonical}); err != nil {
			return Round{}, err
		}
	}
	round := Round{ID: roundID, SourceID: sourceID, Actor: actor, Slots: canonicalSlots, Eligibility: eligibility, CreatedBy: createdBy, Role: role, LocalOrdinal: ordinal, Lifecycle: lifecycle, IdempotencyKey: idempotencyKey, Digest: digest, Fields: fields, Canonical: canonical}
	l.rounds[roundID] = round
	return round, nil
}

func (l *Ledger) DeclareSource(source Source) error {
	if err := domain.ValidateMachineID(source.ID); err != nil {
		return err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, exists := l.sources[source.ID]; exists {
		return fmt.Errorf("source %q already declared", source.ID)
	}
	l.sources[source.ID] = true
	return nil
}

func (l *Ledger) AppendInput(ctx context.Context, sourceID string, input Input) (StoredInput, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.inputStore == nil {
		return StoredInput{}, ErrStoreRequired
	}
	if !l.sources[sourceID] {
		return StoredInput{}, ErrUnknownSource
	}
	if err := domain.ValidateMachineID(input.ProducerUnitKey); err != nil {
		return StoredInput{}, err
	}
	if err := domain.ValidateMachineID(input.Kind); err != nil {
		return StoredInput{}, err
	}
	for _, closure := range l.closures[sourceID] {
		if closure.Fields[5] == true {
			return StoredInput{}, ErrFinalSource
		}
	}
	// Once a non-final closure covers the declared coordinate, late arrival is
	// normalized to the first open coordinate after the greatest closed prefix.
	eligibility := input.Eligibility
	for _, closure := range l.closures[sourceID] {
		closed := coordinate(closure.Fields[4])
		next := nextOpen(closed)
		if next.Compare(eligibility) > 0 {
			eligibility = next
		}
	}
	fields := []any{l.runID[:], sourceID, input.ProducerUnitKey}
	id, _, err := l.deriver.Sum("cote.csf.id.exogenous-input", "cote.csf.schema.exogenous-input-id-preimage", 1, fields)
	if err != nil {
		return StoredInput{}, err
	}
	body := []any{id[:], l.runID[:], sourceID, input.ProducerUnitKey, input.Kind, input.SchemaVersion, input.Actor, input.Payload, int64(input.DeclaredEffectiveAt), coordValue(eligibility), input.Provenance, input.IdempotencyKey}
	digest, _, err := l.deriver.Sum("cote.csf.digest.exogenous-input-unit", "cote.csf.schema.exogenous-input.body", 1, []any{body})
	if err != nil {
		return StoredInput{}, err
	}
	full := append(append([]any{}, body...), digest[:])
	canonical, err := codec.CanonicalPayload(full)
	if err != nil {
		return StoredInput{}, err
	}
	var causalID domain.ID
	copy(causalID[:], id[:])
	if old, exists := l.inputByID[causalID]; exists {
		if !bytes.Equal(old.Canonical, canonical) {
			return StoredInput{}, domain.ErrIdentityCollision
		}
		return old, nil
	}
	_, receipt, err := l.appendIngress(ctx, sourceID, 1, causalID, digest, canonical)
	if err != nil {
		return StoredInput{}, err
	}
	result := StoredInput{ID: causalID, Digest: digest, Fields: full, Canonical: canonical, Receipt: receipt}
	l.inputs[sourceID] = append(l.inputs[sourceID], result)
	l.inputByID[causalID] = result
	return result, nil
}

func (l *Ledger) CloseSource(ctx context.Context, sourceID string, closure Closure) (StoredClosure, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.inputStore == nil {
		return StoredClosure{}, ErrStoreRequired
	}
	if !l.sources[sourceID] {
		return StoredClosure{}, ErrUnknownSource
	}
	if err := domain.ValidateMachineID(closure.ProducerKey); err != nil {
		return StoredClosure{}, err
	}
	for _, prior := range l.closures[sourceID] {
		if prior.Fields[5] == true {
			return StoredClosure{}, ErrFinalSource
		}
		if coordinate(prior.Fields[4]).Compare(closure.ClosedThrough) > 0 {
			return StoredClosure{}, fmt.Errorf("source closure cannot move backward")
		}
	}
	identityFields := []any{l.runID[:], sourceID, closure.ProducerKey}
	id, _, err := l.deriver.Sum("cote.csf.id.source-closure", "cote.csf.schema.source-closure-id-preimage", 1, identityFields)
	if err != nil {
		return StoredClosure{}, err
	}
	var causalID domain.ID
	copy(causalID[:], id[:])
	if old, exists := l.closureByID[causalID]; exists {
		if old.Fields[4] != nil && !bytes.Equal(closureCanonicalShape(old.Fields), closureCanonicalShape([]any{id[:], l.runID[:], sourceID, closure.ProducerKey, coordValue(closure.ClosedThrough), closure.Final, old.Fields[6]})) {
			return StoredClosure{}, domain.ErrIdentityCollision
		}
		return old, nil
	}
	store, ok := l.inputStore.(ports.IngressAppendStore)
	if !ok {
		return StoredClosure{}, ErrIngressStoreRequired
	}
	seq, err := store.NextIngressSequence(ctx, sourceID)
	if err != nil {
		return StoredClosure{}, err
	}
	fields := []any{id[:], l.runID[:], sourceID, closure.ProducerKey, coordValue(closure.ClosedThrough), closure.Final, seq}
	digest, _, err := l.deriver.Sum("cote.csf.digest.source-closure", "cote.csf.schema.source-closure", 1, fields)
	if err != nil {
		return StoredClosure{}, err
	}
	canonical, err := codec.CanonicalPayload(fields)
	if err != nil {
		return StoredClosure{}, err
	}
	_, receipt, err := l.appendIngress(ctx, sourceID, 2, causalID, digest, canonical)
	if err != nil {
		return StoredClosure{}, err
	}
	result := StoredClosure{ID: causalID, Digest: digest, Fields: fields, Canonical: canonical, Receipt: receipt}
	l.closures[sourceID] = append(l.closures[sourceID], result)
	l.closureByID[causalID] = result
	return result, nil
}

func (l *Ledger) appendIngress(ctx context.Context, source string, refKind uint64, id domain.ID, digest domain.Digest, canonical []byte) (ports.CanonicalRecord, IngressReceipt, error) {
	store, ok := l.inputStore.(ports.IngressAppendStore)
	if !ok {
		return ports.CanonicalRecord{}, IngressReceipt{}, ErrIngressStoreRequired
	}
	seq, err := store.NextIngressSequence(ctx, source)
	if err != nil {
		return ports.CanonicalRecord{}, IngressReceipt{}, err
	}
	ref := [3]any{refKind, id[:], digest[:]}
	receiptBytes, err := codec.CanonicalBytes("cote.csf.record.source-ingress-receipt", "cote.csf.schema.source-ingress-receipt", 1, []any{source, seq, []any{ref[0], ref[1], ref[2]}})
	if err != nil {
		return ports.CanonicalRecord{}, IngressReceipt{}, err
	}
	receiptID := sha256.Sum256(receiptBytes)
	if err := store.AppendIngress(ctx, source, seq, ports.CanonicalRecord{Identity: id, Canonical: canonical}, ports.CanonicalRecord{Identity: receiptID, Canonical: receiptBytes}); err != nil {
		return ports.CanonicalRecord{}, IngressReceipt{}, err
	}
	l.cursors[source] = seq + 1
	return ports.CanonicalRecord{Identity: id, Canonical: canonical}, IngressReceipt{SourceID: source, Sequence: seq, RecordRef: ref, Canonical: receiptBytes}, nil
}

func (l *Ledger) RegisterDispatch(ctx context.Context, d Dispatch) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.inputStore == nil {
		return ErrStoreRequired
	}
	if d.RunID != l.runID || d.SlotID == "" {
		return ErrSlotMismatch
	}
	if err := domain.ValidateMachineID(d.SlotID); err != nil {
		return err
	}
	round, exists := l.rounds[d.RoundID]
	if !exists || round.Actor != d.Actor || round.Eligibility != d.Eligibility || !contains(round.Slots, d.SlotID) {
		return ErrSlotMismatch
	}
	roundKey := idKey(d.RoundID)
	if l.providers[roundKey] == nil {
		l.providers[roundKey] = map[string]*slotState{}
	}
	state := l.providers[roundKey][d.SlotID]
	if state != nil && state.Response != nil {
		return ErrSlotFilled
	}
	if state != nil && state.Dispatch != nil && !state.Revoked {
		return fmt.Errorf("slot already has open dispatch")
	}
	if state == nil && d.Ordinal != 0 {
		return fmt.Errorf("first dispatch ordinal must be zero")
	}
	if state != nil && d.Ordinal != state.Dispatch.Ordinal+1 {
		return fmt.Errorf("dispatch ordinal must follow revocation")
	}
	derivedID, _, err := l.deriver.Sum("cote.csf.id.dispatch", "cote.csf.schema.dispatch-id-preimage", 1, []any{d.RunID[:], d.CycleID[:], d.RoundID[:], d.SlotID, uint64(d.Ordinal)})
	if err != nil {
		return err
	}
	if !bytes.Equal(derivedID[:], d.ID[:]) {
		return fmt.Errorf("dispatch id does not match its causal preimage")
	}
	fields := []any{d.ID[:], d.RunID[:], d.CycleID[:], d.RoundID[:], d.SlotID, d.Ordinal, []any{d.Actor[0], d.Actor[1]}, coordValue(d.Eligibility), uint64(d.BaseRevision), int64(d.EffectiveAt), d.RequestDigest[:]}
	digest, _, err := l.deriver.Sum("cote.csf.digest.slot-dispatch", "cote.csf.schema.slot-dispatch", 1, fields)
	if err != nil {
		return err
	}
	d.Digest = digest
	canonical, err := codec.CanonicalPayload(fields)
	if err != nil {
		return err
	}
	d.Canonical = canonical
	if l.inputStore != nil {
		if err := l.inputStore.Append(ctx, ports.CanonicalRecord{Identity: d.ID, Canonical: canonical}); err != nil {
			return err
		}
	}
	if state == nil {
		state = &slotState{}
		l.providers[roundKey][d.SlotID] = state
	}
	copyDispatch := d
	state.Dispatch = &copyDispatch
	state.Revoked = false
	return nil
}

func (l *Ledger) RevokeDispatch(ctx context.Context, roundID domain.ID, slotID string, reason uint8, policyRef any) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.inputStore == nil {
		return ErrStoreRequired
	}
	state := l.providers[idKey(roundID)][slotID]
	if state == nil || state.Dispatch == nil || state.Revoked || state.Response != nil {
		return ErrSlotMismatch
	}
	canonical, err := codec.CanonicalPayload([]any{state.Dispatch.ID[:], uint64(reason), policyRef})
	if err != nil {
		return err
	}
	if l.inputStore != nil {
		operationalID := sha256.Sum256(canonical)
		if err := l.inputStore.Append(ctx, ports.CanonicalRecord{Identity: operationalID, Canonical: canonical}); err != nil {
			return err
		}
	}
	state.Revoked = true
	return nil
}

func (l *Ledger) Respond(ctx context.Context, response Response) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.inputStore == nil {
		return ErrStoreRequired
	}
	state := l.providers[idKey(response.RoundID)][response.SlotID]
	if state == nil || state.Dispatch == nil {
		return ErrSlotMismatch
	}
	if state.Revoked {
		return ErrSlotRevoked
	}
	d := state.Dispatch
	if response.DispatchID != d.ID || response.RunID != d.RunID || response.CycleID != d.CycleID || response.RoundID != d.RoundID || response.SlotID != d.SlotID || response.Actor != d.Actor || response.EffectiveAt != d.EffectiveAt || response.SubmittedAgainstRevision != d.BaseRevision {
		return ErrSlotMismatch
	}
	if response.Kind > 1 {
		return fmt.Errorf("invalid slot response kind")
	}
	expectedID, _, err := l.deriver.Sum(map[bool]string{true: "cote.csf.id.action-proposal", false: "cote.csf.id.no-proposal"}[response.Kind == 0], "cote.csf.schema.slot-response-id-preimage", 1, []any{d.ID[:], uint64(response.Kind)})
	if err != nil {
		return err
	}
	if !bytes.Equal(expectedID[:], response.ID[:]) {
		return fmt.Errorf("slot response id does not match dispatch and response kind")
	}
	if err := validateResponseBody(response); err != nil {
		return err
	}
	var digestSchema string
	if response.Kind == 0 {
		digestSchema = "cote.csf.digest.action-proposal-unit"
	} else {
		digestSchema = "cote.csf.digest.no-proposal-unit"
	}
	digest, _, err := l.deriver.Sum(digestSchema, map[bool]string{true: "cote.csf.schema.action-proposal.body", false: "cote.csf.schema.no-proposal.body"}[response.Kind == 0], 1, []any{response.Body})
	if err != nil {
		return err
	}
	full := append(append([]any{}, response.Body...), digest[:])
	canonical, err := codec.CanonicalPayload(full)
	if err != nil {
		return err
	}
	if state.Response != nil {
		if bytes.Equal(state.Response.Canonical, canonical) {
			return nil
		}
		return ErrSlotFilled
	}
	conditional, ok := l.inputStore.(ports.ConditionalAppendStore)
	if !ok {
		return ErrConditionalStoreRequired
	}
	keyBytes, err := codec.CanonicalPayload([]any{response.RoundID[:], response.SlotID})
	if err != nil {
		return err
	}
	key := sha256.Sum256(append([]byte("csf-slot-response-v1\x00"), keyBytes...))
	inserted, existing, err := conditional.AppendIfAbsent(ctx, key, ports.CanonicalRecord{Identity: response.ID, Canonical: canonical})
	if err != nil {
		return err
	}
	if !inserted && !bytes.Equal(existing, canonical) {
		return ErrSlotFilled
	}
	response.Digest, response.Canonical = digest, canonical
	copyResponse := response
	state.Response = &copyResponse
	return nil
}

func (l *Ledger) Sources() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]string, 0, len(l.sources))
	for s := range l.sources {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

func (l *Ledger) Prefix(source string) (inputs []StoredInput, closures []StoredClosure, cursor uint64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]StoredInput(nil), l.inputs[source]...), append([]StoredClosure(nil), l.closures[source]...), l.cursors[source]
}

// Snapshot returns a coherent ledger prefix for deterministic fence derivation.
func (l *Ledger) Snapshot() (sources []string, inputs []StoredInput, closures []StoredClosure, rounds []Round, slots map[domain.ID]map[string]Response, runID domain.ID) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for source := range l.sources {
		sources = append(sources, source)
	}
	sort.Strings(sources)
	for _, list := range l.inputs {
		inputs = append(inputs, list...)
	}
	for _, list := range l.closures {
		closures = append(closures, list...)
	}
	for _, round := range l.rounds {
		rounds = append(rounds, round)
	}
	sort.Slice(inputs, func(i, j int) bool { return bytes.Compare(inputs[i].ID[:], inputs[j].ID[:]) < 0 })
	sort.Slice(closures, func(i, j int) bool {
		if closures[i].Receipt.SourceID != closures[j].Receipt.SourceID {
			return closures[i].Receipt.SourceID < closures[j].Receipt.SourceID
		}
		return closures[i].Receipt.Sequence < closures[j].Receipt.Sequence
	})
	sort.Slice(rounds, func(i, j int) bool { return bytes.Compare(rounds[i].ID[:], rounds[j].ID[:]) < 0 })
	slots = map[domain.ID]map[string]Response{}
	for roundID, bySlot := range l.providers {
		for _, round := range rounds {
			if idKey(round.ID) == roundID {
				slots[round.ID] = map[string]Response{}
				for slot, state := range bySlot {
					if state.Response != nil {
						slots[round.ID][slot] = *state.Response
					}
				}
			}
		}
	}
	return sources, inputs, closures, rounds, slots, l.runID
}

func coordValue(c domain.EligibilityCoordinate) []any {
	return []any{int64(c.NotBeforeInstant), uint64(c.NotBeforeCycleOrdinal)}
}
func stringList(values []string) []any {
	out := make([]any, len(values))
	for i, v := range values {
		out[i] = v
	}
	return out
}
func idKey(id domain.ID) string { return hex.EncodeToString(id[:]) }
func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
func validRoundOrigin(value any) bool {
	origin, ok := value.([]any)
	if !ok || len(origin) != 2 {
		return false
	}
	variant, ok := origin[0].(uint64)
	if !ok {
		return false
	}
	if variant != 0 && variant != 2 {
		return false
	}
	ref, ok := origin[1].([]any)
	if !ok || len(ref) != 2 {
		return false
	}
	id, idOK := ref[0].([]byte)
	digest, digestOK := ref[1].([]byte)
	return idOK && digestOK && len(id) == 32 && len(digest) == 32
}
func closureCanonicalShape(fields []any) []byte {
	encoded, _ := codec.CanonicalPayload(fields)
	return encoded
}

func validateResponseBody(r Response) error {
	if r.Kind == 0 {
		if len(r.Body) != 18 {
			return fmt.Errorf("ActionProposal must contain 18 fields")
		}
		if !bytesAt(r.Body, 0, r.ID) || !bytesAt(r.Body, 1, r.DispatchID) || !bytesAt(r.Body, 2, r.RunID) || !bytesAt(r.Body, 3, r.CycleID) || !bytesAt(r.Body, 13, r.RoundID) || r.Body[14] != r.SlotID || r.Body[11] != int64(r.EffectiveAt) || r.Body[15] != uint64(r.SubmittedAgainstRevision) {
			return ErrSlotMismatch
		}
		if !actorAt(r.Body, 6, r.Actor) {
			return ErrSlotMismatch
		}
	} else {
		if len(r.Body) != 10 {
			return fmt.Errorf("NoProposal must contain 10 fields")
		}
		if !bytesAt(r.Body, 0, r.ID) || !bytesAt(r.Body, 1, r.DispatchID) || !bytesAt(r.Body, 2, r.RunID) || !bytesAt(r.Body, 3, r.CycleID) || !bytesAt(r.Body, 4, r.RoundID) || r.Body[5] != r.SlotID || r.Body[7] != int64(r.EffectiveAt) || r.Body[8] != uint64(r.SubmittedAgainstRevision) {
			return ErrSlotMismatch
		}
		if !actorAt(r.Body, 6, r.Actor) {
			return ErrSlotMismatch
		}
	}
	return nil
}
func bytesAt(fields []any, index int, expected domain.ID) bool {
	value, ok := fields[index].([]byte)
	return ok && bytes.Equal(value, expected[:])
}
func actorAt(fields []any, index int, expected [2]string) bool {
	actor, ok := fields[index].([]any)
	return ok && len(actor) == 2 && actor[0] == expected[0] && actor[1] == expected[1]
}
func coordinate(v any) domain.EligibilityCoordinate {
	a := v.([]any)
	return domain.EligibilityCoordinate{NotBeforeInstant: domain.SimulationInstant(a[0].(int64)), NotBeforeCycleOrdinal: domain.CycleOrdinal(a[1].(uint64))}
}
func nextOpen(c domain.EligibilityCoordinate) domain.EligibilityCoordinate {
	if c.NotBeforeCycleOrdinal < ^domain.CycleOrdinal(0) {
		c.NotBeforeCycleOrdinal++
		return c
	}
	if c.NotBeforeInstant < domain.SimulationInstant(^uint64(0)>>1) {
		return domain.EligibilityCoordinate{NotBeforeInstant: c.NotBeforeInstant + 1}
	}
	return c
}

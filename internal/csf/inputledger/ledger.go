// Package inputledger owns exogenous ingress and slot completion. It deliberately
// keeps append order as receipt metadata; causal identities and fence ordering
// are derived from the records themselves.
package inputledger

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"reflect"
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
	ErrSourcesFrozen            = errors.New("causal source set is frozen")
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
	mu            sync.Mutex
	admissionMu   sync.RWMutex
	runID         domain.ID
	deriver       identity.Deriver
	sources       map[string]bool
	cursors       map[string]uint64
	inputs        map[string][]StoredInput
	closures      map[string][]StoredClosure
	inputByID     map[domain.ID]StoredInput
	closureByID   map[domain.ID]StoredClosure
	rounds        map[domain.ID]Round
	providers     map[string]map[string]*slotState
	inputStore    ports.AppendOnlyStore
	sourcesFrozen bool
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

// CycleBinding is the authoritative cycle tuple a slot dispatch must inherit.
type CycleBinding struct {
	ID           domain.ID
	BaseRevision domain.WorldRevision
	Instant      domain.SimulationInstant
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

func newLedger(runID domain.ID, bundle *policy.Bundle, inputStore ports.AppendOnlyStore) *Ledger {
	return &Ledger{runID: runID, deriver: identity.New(bundle), sources: map[string]bool{}, cursors: map[string]uint64{}, inputs: map[string][]StoredInput{}, closures: map[string][]StoredClosure{}, inputByID: map[domain.ID]StoredInput{}, closureByID: map[domain.ID]StoredClosure{}, rounds: map[domain.ID]Round{}, providers: map[string]map[string]*slotState{}, inputStore: inputStore}
}

// New opens an existing prefix or initializes an empty one. It never returns a
// ledger whose in-memory state silently omits records already in the store.
func New(ctx context.Context, runID domain.ID, bundle *policy.Bundle, inputStore ports.AppendOnlyStore) (*Ledger, error) {
	return Open(ctx, runID, bundle, inputStore)
}

// Open restores a ledger from its canonical input-store prefix before allowing
// callers to append or derive another fence.
func Open(ctx context.Context, runID domain.ID, bundle *policy.Bundle, inputStore ports.AppendOnlyStore) (*Ledger, error) {
	if inputStore == nil {
		return nil, ErrStoreRequired
	}
	l := newLedger(runID, bundle, inputStore)
	lister, ok := inputStore.(ports.RecordListingStore)
	if !ok {
		return nil, errors.New("input ledger store does not support prefix restoration")
	}
	records, err := lister.Records(ctx)
	if err != nil {
		return nil, err
	}
	if err := l.restore(records); err != nil {
		return nil, err
	}
	if err:=l.projectSettledNoProposalRounds(ctx);err!=nil{return nil,err}
	return l, nil
}

func (l *Ledger) restore(records []ports.CanonicalRecord) error {
	type receiptRecord struct {
		source string
		seq    uint64
		kind   uint64
		id     domain.ID
		digest domain.Digest
		bytes  []byte
	}
	receipts := map[domain.ID]receiptRecord{}
	inputs := map[domain.ID]StoredInput{}
	closures := map[domain.ID]StoredClosure{}
	dispatches := map[domain.ID]Dispatch{}
	for _, record := range records {
		if envelope, err := codec.StrictDecodeEnvelope(record.Canonical); err == nil && envelope.DomainTag == "cote.csf.record.source-ingress-receipt" {
			if [32]byte(identity.OpaqueContentHash(record.Canonical)) != record.Identity {
				return errors.New("persisted SourceIngressReceipt identity mismatch")
			}
			f, ok := envelope.Value.([]any)
			if !ok || len(f) != 3 {
				return errors.New("malformed persisted SourceIngressReceipt")
			}
			source, sourceOK := f[0].(string)
			seq, seqOK := f[1].(uint64)
			ref, refOK := f[2].([]any)
			if !sourceOK || domain.ValidateMachineID(source) != nil || !seqOK || !refOK || len(ref) != 3 {
				return errors.New("malformed persisted SourceIngressReceipt fields")
			}
			kind, kindOK := ref[0].(uint64)
			idBytes, idOK := ref[1].([]byte)
			digestBytes, digestOK := ref[2].([]byte)
			if !kindOK || (kind != 1 && kind != 2) || !idOK || len(idBytes) != 32 || !digestOK || len(digestBytes) != 32 {
				return errors.New("malformed persisted SourceIngressReceipt reference")
			}
			var id domain.ID
			var digest domain.Digest
			copy(id[:], idBytes)
			copy(digest[:], digestBytes)
            if _, exists := receipts[id]; exists {
                return errors.New("duplicate persisted SourceIngressReceipt for one causal record ID")
            }
			receipts[id] = receiptRecord{source: source, seq: seq, kind: kind, id: id, digest: digest, bytes: bytes.Clone(record.Canonical)}
			continue
		}
		f, err := codec.StrictDecodePayload(record.Canonical)
		if err != nil {
			return fmt.Errorf("decode persisted input-ledger record %x: %w", record.Identity[:], err)
		}
		fields, ok := f.([]any)
		if !ok {
			return errors.New("persisted input-ledger record is not an array")
		}
		if len(fields) == 3 && fields[0] == "source-declaration-v1" {
			if !matchesBytes(l.runID[:], fields[1]) {
				return errors.New("persisted source declaration belongs to another run")
			}
			source, ok := fields[2].(string)
			if !ok || domain.ValidateMachineID(source) != nil || [32]byte(identity.OpaqueContentHash(record.Canonical)) != record.Identity {
				return errors.New("invalid persisted source declaration")
			}
			l.sources[source] = true
			continue
		}
		if len(fields) == 13 && isString(fields[2]) && isString(fields[3]) && isString(fields[4]) {
			stored, err := l.restoreInput(record, fields)
			if err != nil {
				return err
			}
			inputs[stored.ID] = stored
			continue
		}
		if len(fields) == 7 && isString(fields[2]) && isString(fields[3]) && isBool(fields[5]) {
			stored, err := l.restoreClosure(record, fields)
			if err != nil {
				return err
			}
			closures[stored.ID] = stored
			continue
		}
		if len(fields) == 11 && isString(fields[2]) {
			round, err := l.restoreRound(record, fields)
			if err != nil {
				return err
			}
			l.rounds[round.ID] = round
			continue
		}
		if len(fields) == 11 && isBytes(fields[2]) && isString(fields[4]) {
			dispatch, err := l.restoreDispatch(record, fields)
			if err != nil {
				return err
			}
			round, exists := l.rounds[dispatch.RoundID]
			if !exists || round.Actor != dispatch.Actor || round.Eligibility != dispatch.Eligibility || !contains(round.Slots, dispatch.SlotID) {
				return errors.New("persisted SlotDispatch is not bound to its RoundDeclaration")
			}
			dispatches[dispatch.ID] = dispatch
			key := idKey(dispatch.RoundID)
			if l.providers[key] == nil {
				l.providers[key] = map[string]*slotState{}
			}
			prior := l.providers[key][dispatch.SlotID]
			if prior == nil {
				if dispatch.Ordinal != 0 {
					return errors.New("persisted first dispatch ordinal is not zero")
				}
			} else if prior.Dispatch == nil || prior.Response != nil || !prior.Revoked ||
				prior.Dispatch.Ordinal == ^uint32(0) || dispatch.Ordinal != prior.Dispatch.Ordinal+1 {
				return errors.New("persisted redispatch lacks immediate unfilled revocation")
			}
			l.providers[key][dispatch.SlotID] = &slotState{Dispatch: &dispatch}
			continue
		}
		if (len(fields) == 11 || len(fields) == 19) && isBytes(fields[1]) {
			dispatchID, ok := idFrom(fields[1])
			if ok {
				if dispatch, exists := dispatches[dispatchID]; exists {
					response, err := l.restoreResponse(record, fields, dispatch)
					if err != nil {
						return err
					}
					state := l.providers[idKey(response.RoundID)][response.SlotID]
					if state == nil || state.Dispatch == nil || state.Dispatch.ID != dispatchID || state.Revoked || state.Response != nil {
						return errors.New("persisted response does not fill the current open slot exactly once")
					}
					state.Response = &response
					continue
				}
			}
		}
		if len(fields) == 3 && isBytes(fields[0]) {
			dispatchID, ok := idFrom(fields[0])
			if ok {
				state := l.findDispatchState(dispatchID)
				if state != nil {
					reason, validReason := fields[1].(uint64)
					if state.Revoked || state.Response != nil || !validReason || reason > 255 {
						return errors.New("persisted revocation does not target an open unfilled dispatch")
					}
					if [32]byte(identity.OpaqueContentHash(record.Canonical)) != record.Identity {
						return errors.New("persisted dispatch revocation identity mismatch")
					}
					state.Revoked = true
					continue
				}
			}
		}
		return fmt.Errorf("unrecognized persisted input-ledger record %x", record.Identity[:])
	}

	bySource := map[string][]receiptRecord{}
	attachedIngress := map[domain.ID]bool{}
	for _, receipt := range receipts {
		bySource[receipt.source] = append(bySource[receipt.source], receipt)
	}
	for source, list := range bySource {
  if !l.sources[source] {return fmt.Errorf("ingress receipt references undeclared source %q",source)}
  sort.Slice(list,func(i,j int)bool{return list[i].seq<list[j].seq})
  var lastClosed domain.EligibilityCoordinate
  hasClosure:=false
  sourceFinal:=false
  for i,receipt:=range list{
   // Reopening and replay must reject malformed source histories even when
   // each individually authenticated SourceIngressReceipt is valid.
   if sourceFinal{return fmt.Errorf("source %q ingressed after a final SourceClosure",source)}
			if receipt.seq != uint64(i) {
				return fmt.Errorf("source %q ingress prefix has a gap at %d", source, i)
			}
			if input, ok := inputs[receipt.id]; ok {
				if receipt.kind != 1 || receipt.digest != input.Digest || input.Fields[2] != source {
     return errors.New("persisted input receipt does not match its record")
    }
    if hasClosure&&coordinate(input.Fields[9]).Compare(lastClosed)<=0{
     return errors.New("persisted input was not normalized above a prior source closure")
    }
    input.Receipt = IngressReceipt{SourceID: source, Sequence: receipt.seq, RecordRef: [3]any{receipt.kind, input.ID[:], input.Digest[:]}, Canonical: receipt.bytes}
				l.inputs[source] = append(l.inputs[source], input)
				l.inputByID[input.ID] = input
				attachedIngress[input.ID] = true
				continue
			}
			if closure, ok := closures[receipt.id]; ok {
				if receipt.kind != 2 || receipt.digest != closure.Digest || closure.Fields[2] != source || closure.Fields[6] != receipt.seq {
     return errors.New("persisted closure receipt does not match its record")
    }
    at:=coordinate(closure.Fields[4])
    if hasClosure&&at.Compare(lastClosed)<0{return errors.New("persisted source closure regressed")}
    lastClosed=at
    hasClosure=true
    sourceFinal=closure.Fields[5]==true
    closure.Receipt = IngressReceipt{SourceID: source, Sequence: receipt.seq, RecordRef: [3]any{receipt.kind, closure.ID[:], closure.Digest[:]}, Canonical: receipt.bytes}
				l.closures[source] = append(l.closures[source], closure)
				l.closureByID[closure.ID] = closure
				attachedIngress[closure.ID] = true
				continue
			}
			return fmt.Errorf("source %q receipt references a missing input record", source)
		}
		l.cursors[source] = uint64(len(list))
	}
	for id := range inputs {
		if !attachedIngress[id] {
			return errors.New("persisted input is missing its ingress receipt")
		}
	}
	for id := range closures {
		if !attachedIngress[id] {
			return errors.New("persisted closure is missing its ingress receipt")
		}
	}
	l.sourcesFrozen = len(records) > 0
	return nil
}

func (l *Ledger) restoreInput(record ports.CanonicalRecord, fields []any) (StoredInput, error) {
	if !matchesBytes(l.runID[:], fields[1]) {
		return StoredInput{}, errors.New("persisted ExogenousInput belongs to another run")
	}
	id, _, err := l.deriver.Sum("cote.csf.id.exogenous-input", "cote.csf.schema.exogenous-input-id-preimage", 1, []any{fields[1], fields[2], fields[3]})
	if err != nil || !matchesBytes(id[:], fields[0]) || !bytes.Equal(id[:], record.Identity[:]) {
		return StoredInput{}, errors.New("persisted ExogenousInput identity mismatch")
	}
	digest, _, err := l.deriver.Sum("cote.csf.digest.exogenous-input-unit", "cote.csf.schema.exogenous-input.body", 1, []any{fields[:12]})
	if err != nil || !matchesBytes(digest[:], fields[12]) {
		return StoredInput{}, errors.New("persisted ExogenousInput digest mismatch")
	}
	canonical, err := codec.CanonicalPayload(fields)
	if err != nil || !bytes.Equal(canonical, record.Canonical) {
		return StoredInput{}, errors.New("persisted ExogenousInput bytes are not canonical")
	}
	var idValue domain.ID
	var digestValue domain.Digest
	copy(idValue[:], id[:])
	copy(digestValue[:], digest[:])
	return StoredInput{ID: idValue, Digest: digestValue, Fields: fields, Canonical: canonical}, nil
}

func (l *Ledger) restoreClosure(record ports.CanonicalRecord, fields []any) (StoredClosure, error) {
	if !matchesBytes(l.runID[:], fields[1]) {
		return StoredClosure{}, errors.New("persisted SourceClosure belongs to another run")
	}
	id, _, err := l.deriver.Sum("cote.csf.id.source-closure", "cote.csf.schema.source-closure-id-preimage", 1, []any{fields[1], fields[2], fields[3]})
	if err != nil || !matchesBytes(id[:], fields[0]) || !bytes.Equal(id[:], record.Identity[:]) {
		return StoredClosure{}, errors.New("persisted SourceClosure identity mismatch")
	}
	digest, _, err := l.deriver.Sum("cote.csf.digest.source-closure", "cote.csf.schema.source-closure", 1, fields)
	if err != nil {
		return StoredClosure{}, errors.New("persisted SourceClosure digest cannot be verified")
	}
	canonical, err := codec.CanonicalPayload(fields)
	if err != nil || !bytes.Equal(canonical, record.Canonical) {
		return StoredClosure{}, errors.New("persisted SourceClosure bytes are not canonical")
	}
	var idValue domain.ID
	var digestValue domain.Digest
	copy(idValue[:], id[:])
	copy(digestValue[:], digest[:])
	return StoredClosure{ID: idValue, Digest: digestValue, Fields: fields, Canonical: canonical}, nil
}

func (l *Ledger) restoreRound(record ports.CanonicalRecord, fields []any) (Round, error) {
	if !matchesBytes(l.runID[:], fields[1]) {
		return Round{}, errors.New("persisted RoundDeclaration belongs to another run")
	}
	id, _, err := l.deriver.Sum("cote.csf.id.decision-round", "cote.csf.schema.decision-round-id-preimage", 1, []any{fields[6], fields[7], fields[8]})
	if err != nil || !matchesBytes(id[:], fields[0]) || !bytes.Equal(id[:], record.Identity[:]) {
		return Round{}, errors.New("persisted RoundDeclaration identity mismatch")
	}
	digest, _, err := l.deriver.Sum("cote.csf.digest.round-declaration", "cote.csf.schema.round-declaration", 1, fields)
	if err != nil {
		return Round{}, errors.New("persisted RoundDeclaration digest cannot be verified")
	}
	canonical, err := codec.CanonicalPayload(fields)
	if err != nil || !bytes.Equal(canonical, record.Canonical) {
		return Round{}, errors.New("persisted RoundDeclaration bytes are not canonical")
	}
	actor, ok := fields[3].([]any)
	slots, slotsOK := fields[4].([]any)
	eligibility, coordinateOK := parseCoordinate(fields[5])
	if !ok || len(actor) != 2 || !slotsOK || len(slots) == 0 || !coordinateOK {
		return Round{}, errors.New("malformed persisted RoundDeclaration")
	}
	var idValue domain.ID
	var digestValue domain.Digest
	copy(idValue[:], id[:])
	copy(digestValue[:], digest[:])
	role, _ := fields[7].(uint64)
	ordinal, _ := fields[8].(uint64)
	lifecycle, _ := fields[9].(uint64)
	return Round{ID: idValue, SourceID: fields[2].(string), Actor: [2]string{actor[0].(string), actor[1].(string)}, Slots: stringSlice(slots), Eligibility: eligibility, CreatedBy: fields[6], Role: uint16(role), LocalOrdinal: uint32(ordinal), Lifecycle: uint8(lifecycle), IdempotencyKey: fields[10], Digest: digestValue, Fields: fields, Canonical: canonical}, nil
}

func (l *Ledger) restoreDispatch(record ports.CanonicalRecord, fields []any) (Dispatch, error) {
	if !matchesBytes(l.runID[:], fields[1]) {
		return Dispatch{}, errors.New("persisted SlotDispatch belongs to another run")
	}
	id, _, err := l.deriver.Sum("cote.csf.id.dispatch", "cote.csf.schema.dispatch-id-preimage", 1, []any{fields[1], fields[2], fields[3], fields[4], fields[5]})
	if err != nil || !matchesBytes(id[:], fields[0]) || !bytes.Equal(id[:], record.Identity[:]) {
		return Dispatch{}, errors.New("persisted SlotDispatch identity mismatch")
	}
	digest, _, err := l.deriver.Sum("cote.csf.digest.slot-dispatch", "cote.csf.schema.slot-dispatch", 1, fields)
	if err != nil {
		return Dispatch{}, errors.New("persisted SlotDispatch digest cannot be verified")
	}
	actor, ok := fields[6].([]any)
	if !ok || len(actor) != 2 {
		return Dispatch{}, errors.New("malformed persisted SlotDispatch actor")
	}
	cycleID, ok1 := idFrom(fields[2])
	roundID, ok2 := idFrom(fields[3])
	var idValue domain.ID
	var digestValue domain.Digest
	copy(idValue[:], id[:])
	copy(digestValue[:], digest[:])
	ordinal, ordinalOK := fields[5].(uint64)
	if !ordinalOK || ordinal > uint64(^uint32(0)) {
		return Dispatch{}, errors.New("persisted dispatch ordinal is outside uint32")
	}
	revision, _ := fields[8].(uint64)
	instant, instantOK := signedInt64(fields[9])
	requestDigest, ok3 := digestFrom(fields[10])
	eligibility, eligibilityOK := parseCoordinate(fields[7])
	if !ok1 || !ok2 || !ok3 || !instantOK || !eligibilityOK {
		return Dispatch{}, errors.New("malformed persisted SlotDispatch references")
	}
	canonical, err := codec.CanonicalPayload(fields)
	if err != nil || !bytes.Equal(canonical, record.Canonical) {
		return Dispatch{}, errors.New("persisted SlotDispatch bytes are not canonical")
	}
	return Dispatch{ID: idValue, RunID: l.runID, CycleID: cycleID, RoundID: roundID, SlotID: fields[4].(string), Ordinal: uint32(ordinal), Actor: [2]string{actor[0].(string), actor[1].(string)}, Eligibility: eligibility, BaseRevision: domain.WorldRevision(revision), EffectiveAt: domain.SimulationInstant(instant), RequestDigest: requestDigest, Digest: digestValue, Canonical: canonical}, nil
}

func (l *Ledger) restoreResponse(record ports.CanonicalRecord, fields []any, dispatch Dispatch) (Response, error) {
	kind := uint8(1)
	if len(fields) == 19 {
		kind = 0
	} else if len(fields) != 11 {
		return Response{}, errors.New("malformed persisted SlotResponse")
	}
	body := fields[:len(fields)-1]
	id, _, err := l.deriver.Sum(map[bool]string{true: "cote.csf.id.action-proposal", false: "cote.csf.id.no-proposal"}[kind == 0], "cote.csf.schema.slot-response-id-preimage", 1, []any{dispatch.ID[:], uint64(kind)})
	if err != nil || !matchesBytes(id[:], fields[0]) || record.Identity != dispatchResponseID(id) {
		return Response{}, errors.New("persisted SlotResponse identity mismatch")
	}
	digestSchema, bodySchema := "cote.csf.digest.no-proposal-unit", "cote.csf.schema.no-proposal.body"
	if kind == 0 {
		digestSchema, bodySchema = "cote.csf.digest.action-proposal-unit", "cote.csf.schema.action-proposal.body"
	}
	digest, _, err := l.deriver.Sum(digestSchema, bodySchema, 1, []any{body})
	if err != nil || !matchesBytes(digest[:], fields[len(fields)-1]) {
		return Response{}, errors.New("persisted SlotResponse digest mismatch")
	}
	canonical, err := codec.CanonicalPayload(fields)
	if err != nil || !bytes.Equal(canonical, record.Canonical) {
		return Response{}, errors.New("persisted SlotResponse bytes are not canonical")
	}
	response := Response{Kind: kind, ID: dispatchResponseID(id), DispatchID: dispatch.ID, RunID: dispatch.RunID, CycleID: dispatch.CycleID, RoundID: dispatch.RoundID, SlotID: dispatch.SlotID, Actor: dispatch.Actor, EffectiveAt: dispatch.EffectiveAt, SubmittedAgainstRevision: dispatch.BaseRevision, Body: body, Digest: digest}
	if err := validateResponseBody(response); err != nil {
		return Response{}, err
	}
	response.Canonical = canonical
	return response, nil
}

func (l *Ledger) findDispatchState(id domain.ID) *slotState {
	for _, bySlot := range l.providers {
		for _, state := range bySlot {
			if state.Dispatch != nil && state.Dispatch.ID == id {
				return state
			}
		}
	}
	return nil
}

func dispatchResponseID(value domain.Digest) domain.ID {
	return domain.ID(value)
}
func isString(v any) bool { _, ok := v.(string); return ok }
func isBytes(v any) bool  { _, ok := v.([]byte); return ok }
func isBool(v any) bool   { _, ok := v.(bool); return ok }
func matchesBytes(expected []byte, value any) bool {
	actual, ok := value.([]byte)
	return ok && bytes.Equal(expected, actual)
}
func idFrom(value any) (domain.ID, bool) {
	var result domain.ID
	data, ok := value.([]byte)
	if !ok || len(data) != len(result) {
		return result, false
	}
	copy(result[:], data)
	return result, true
}
func digestFrom(value any) (domain.Digest, bool) {
	var result domain.Digest
	data, ok := value.([]byte)
	if !ok || len(data) != len(result) {
		return result, false
	}
	copy(result[:], data)
	return result, true
}
func stringSlice(values []any) []string {
	result := make([]string, len(values))
	for i, value := range values {
		result[i], _ = value.(string)
	}
	return result
}

// DeclareRound admits only a round whose origin is already causal (genesis or
// committed event). It does not create a round from an exogenous input.
func (l *Ledger) DeclareRound(ctx context.Context, sourceID string, actor [2]string, slots []string, eligibility domain.EligibilityCoordinate, createdBy any, role uint16, ordinal uint32, lifecycle uint8, idempotencyKey any) (Round, error) {
	l.admissionMu.RLock()
	defer l.admissionMu.RUnlock()
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
	guard, ok := l.inputStore.(ports.AdmissionGuardedAppendStore)
	if !ok { return Round{}, errors.New("store lacks durable admission round guard") }
	if err := guard.AppendRoundDeclaration(ctx, [32]byte(l.runID), int64(eligibility.NotBeforeInstant), uint64(eligibility.NotBeforeCycleOrdinal), ports.CanonicalRecord{Identity: roundID, Canonical: canonical}); err != nil {
		return Round{}, err
	}
	round := Round{ID: roundID, SourceID: sourceID, Actor: actor, Slots: canonicalSlots, Eligibility: eligibility, CreatedBy: createdBy, Role: role, LocalOrdinal: ordinal, Lifecycle: lifecycle, IdempotencyKey: idempotencyKey, Digest: digest, Fields: fields, Canonical: canonical}
	l.rounds[roundID] = round
	return round, nil
}

func (l *Ledger) DeclareSource(source Source) error {
	if err := domain.ValidateMachineID(source.ID); err != nil {
		return err
	}
	l.admissionMu.RLock()
	defer l.admissionMu.RUnlock()
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.inputStore == nil {
		return ErrStoreRequired
	}
	if l.sourcesFrozen {
		return ErrSourcesFrozen
	}
	if _, exists := l.sources[source.ID]; exists {
		return fmt.Errorf("source %q already declared", source.ID)
	}
	canonical, err := codec.CanonicalPayload([]any{"source-declaration-v1", l.runID[:], source.ID})
	if err != nil {
		return err
	}
	guard, ok := l.inputStore.(ports.AdmissionGuardedAppendStore)
	if !ok { return errors.New("store lacks durable admission source guard") }
	if err := guard.AppendSourceDeclaration(context.Background(), [32]byte(l.runID), ports.CanonicalRecord{Identity: [32]byte(identity.OpaqueContentHash(canonical)), Canonical: canonical}); err != nil {
		return err
	}
	l.sources[source.ID] = true
	return nil
}

func (l *Ledger) AppendInput(ctx context.Context, sourceID string, input Input) (StoredInput, error) {
	l.admissionMu.RLock()
	defer l.admissionMu.RUnlock()
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
	identityFields := []any{l.runID[:], sourceID, input.ProducerUnitKey}
	id, _, err := l.deriver.Sum("cote.csf.id.exogenous-input", "cote.csf.schema.exogenous-input-id-preimage", 1, identityFields)
	if err != nil {
		return StoredInput{}, err
	}
	var causalID domain.ID
	copy(causalID[:], id[:])
	if old, exists := l.inputByID[causalID]; exists {
		body := []any{id[:], l.runID[:], sourceID, input.ProducerUnitKey, input.Kind, input.SchemaVersion, input.Actor, input.Payload, int64(input.DeclaredEffectiveAt), old.Fields[9], input.Provenance, input.IdempotencyKey}
		digest, _, err := l.deriver.Sum("cote.csf.digest.exogenous-input-unit", "cote.csf.schema.exogenous-input.body", 1, []any{body})
		if err != nil {
			return StoredInput{}, err
		}
		canonical, err := codec.CanonicalPayload(append(append([]any{}, body...), digest[:]))
		if err != nil {
			return StoredInput{}, err
		}
		if !bytes.Equal(old.Canonical, canonical) {
			return StoredInput{}, domain.ErrIdentityCollision
		}
		return old, nil
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
	l.admissionMu.RLock()
	defer l.admissionMu.RUnlock()
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
	identityFields := []any{l.runID[:], sourceID, closure.ProducerKey}
	id, _, err := l.deriver.Sum("cote.csf.id.source-closure", "cote.csf.schema.source-closure-id-preimage", 1, identityFields)
	if err != nil {
		return StoredClosure{}, err
	}
	var causalID domain.ID
	copy(causalID[:], id[:])
	if old, exists := l.closureByID[causalID]; exists {
		if coordinate(old.Fields[4]).Compare(closure.ClosedThrough) != 0 || old.Fields[5] != closure.Final {
			return StoredClosure{}, domain.ErrIdentityCollision
		}
		return old, nil
	}
	for _, prior := range l.closures[sourceID] {
		if prior.Fields[5] == true {
			return StoredClosure{}, ErrFinalSource
		}
		if coordinate(prior.Fields[4]).Compare(closure.ClosedThrough) > 0 {
			return StoredClosure{}, fmt.Errorf("source closure cannot move backward")
		}
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
	receiptID := [32]byte(identity.OpaqueContentHash(receiptBytes))
	if err := store.AppendIngress(ctx, source, seq, ports.CanonicalRecord{Identity: id, Canonical: canonical}, ports.CanonicalRecord{Identity: receiptID, Canonical: receiptBytes}); err != nil {
		return ports.CanonicalRecord{}, IngressReceipt{}, err
	}
	l.cursors[source] = seq + 1
	return ports.CanonicalRecord{Identity: id, Canonical: canonical}, IngressReceipt{SourceID: source, Sequence: seq, RecordRef: ref, Canonical: receiptBytes}, nil
}

func (l *Ledger) RegisterDispatch(ctx context.Context, d Dispatch, cycle CycleBinding) error {
	l.admissionMu.RLock()
	defer l.admissionMu.RUnlock()
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.inputStore == nil {
		return ErrStoreRequired
	}
	if d.RunID != l.runID || d.CycleID != cycle.ID || d.BaseRevision != cycle.BaseRevision || d.EffectiveAt != cycle.Instant || d.SlotID == "" {
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
	l.admissionMu.RLock()
	defer l.admissionMu.RUnlock()
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
		operationalID := [32]byte(identity.OpaqueContentHash(canonical))
		if err := l.inputStore.Append(ctx, ports.CanonicalRecord{Identity: operationalID, Canonical: canonical}); err != nil {
			return err
		}
	}
	state.Revoked = true
	return nil
}

func (l *Ledger) Respond(ctx context.Context, response Response) error {
	l.admissionMu.RLock()
	defer l.admissionMu.RUnlock()
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
	key := [32]byte(identity.OpaqueContentHash(append([]byte("csf-slot-response-v1\x00"), keyBytes...)))
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

// SharesFenceAuthority deliberately requires the very same store instance for
// input and decision fence publication. V1 does not support cross-adapter or
// cross-database transactions; separate connections are rejected conservatively.
// No store records or locks are modified by this check.
func (l *Ledger) SharesFenceAuthority(store ports.AdmissionFenceStore) bool {
	if l == nil || l.inputStore == nil || store == nil { return false }
	input := reflect.ValueOf(l.inputStore)
	fence := reflect.ValueOf(store)
	return input.Kind() == reflect.Pointer && fence.Kind() == reflect.Pointer &&
		input.Type() == fence.Type() && !input.IsNil() && !fence.IsNil() &&
		input.Pointer() == fence.Pointer()
}

// AdmissionView restores a coherent persisted prefix and returns its exact
// records for a storage-level compare-and-append. Never derive from a stale
// ledger snapshot when another process may have appended records.
func (l *Ledger) AdmissionView(ctx context.Context, bundle *policy.Bundle) (*Ledger, []ports.CanonicalRecord, error) {
	lister, ok := l.inputStore.(ports.RecordListingStore)
	if !ok {
		return nil, nil, errors.New("input store cannot list durable prefix")
	}
	records, err := lister.Records(ctx)
	if err != nil {
		return nil, nil, err
	}
	view := newLedger(l.runID, bundle, l.inputStore)
	if err := view.restore(records); err != nil {
		return nil, nil, err
	}
	if err:=view.projectSettledNoProposalRounds(ctx);err!=nil{return nil,nil,err}
	return view, records, nil
}

// WithAdmissionBoundary serializes same-ledger writers against fence derivation
// and durable append. It does not replace cross-process database coordination.
func (l *Ledger) WithAdmissionBoundary(publish func() error) error {
	l.admissionMu.Lock()
	defer l.admissionMu.Unlock()
	return publish()
}

// FreezeSourceDeclarations marks this ledger's local cache after the durable
// fence append succeeds; stores independently enforce the persistent seal.
func (l *Ledger) FreezeSourceDeclarations() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.sourcesFrozen = true
}

// Snapshot returns a coherent ledger prefix for deterministic fence derivation.
func (l *Ledger) Snapshot() (sources []string, inputs []StoredInput, closures []StoredClosure, rounds []Round, slots map[domain.ID]map[string]Response, runID domain.ID) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.sourcesFrozen = true
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
func validateResponseBody(r Response) error {
	if r.Kind == 0 {
		if len(r.Body) != 18 {
			return fmt.Errorf("ActionProposal must contain 18 fields")
		}
		if !bytesAt(r.Body, 0, r.ID) || !bytesAt(r.Body, 1, r.DispatchID) || !bytesAt(r.Body, 2, r.RunID) || !bytesAt(r.Body, 3, r.CycleID) || !bytesAt(r.Body, 13, r.RoundID) || r.Body[14] != r.SlotID || !equalsInt64(r.Body[11], int64(r.EffectiveAt)) || !equalsUint64(r.Body[15], uint64(r.SubmittedAgainstRevision)) {
			return ErrSlotMismatch
		}
		if !actorAt(r.Body, 6, r.Actor) {
			return ErrSlotMismatch
		}
	} else {
		if len(r.Body) != 10 {
			return fmt.Errorf("NoProposal must contain 10 fields")
		}
		if !bytesAt(r.Body, 0, r.ID) || !bytesAt(r.Body, 1, r.DispatchID) || !bytesAt(r.Body, 2, r.RunID) || !bytesAt(r.Body, 3, r.CycleID) || !bytesAt(r.Body, 4, r.RoundID) || r.Body[5] != r.SlotID || !equalsInt64(r.Body[7], int64(r.EffectiveAt)) || !equalsUint64(r.Body[8], uint64(r.SubmittedAgainstRevision)) {
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
	coordinate, _ := parseCoordinate(v)
	return coordinate
}

func parseCoordinate(value any) (domain.EligibilityCoordinate, bool) {
	fields, ok := value.([]any)
	if !ok || len(fields) != 2 {
		return domain.EligibilityCoordinate{}, false
	}
	instant, instantOK := signedInt64(fields[0])
	ordinal, ordinalOK := unsigned64(fields[1])
	if !instantOK || !ordinalOK {
		return domain.EligibilityCoordinate{}, false
	}
	return domain.EligibilityCoordinate{NotBeforeInstant: domain.SimulationInstant(instant), NotBeforeCycleOrdinal: domain.CycleOrdinal(ordinal)}, true
}

func signedInt64(value any) (int64, bool) {
	switch number := value.(type) {
	case int64:
		return number, true
	case uint64:
		if number <= uint64(^uint64(0)>>1) {
			return int64(number), true
		}
	}
	return 0, false
}

func unsigned64(value any) (uint64, bool) {
	switch number := value.(type) {
	case uint64:
		return number, true
	case int64:
		if number >= 0 {
			return uint64(number), true
		}
	}
	return 0, false
}

func equalsInt64(value any, expected int64) bool {
	actual, ok := signedInt64(value)
	return ok && actual == expected
}

func equalsUint64(value any, expected uint64) bool {
	actual, ok := unsigned64(value)
	return ok && actual == expected
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

// projectSettledNoProposalRounds consumes a read-only, authoritative
// DecisionRecord+CycleCommit projection. Neither RoundDeclaration bytes nor
// any source response are mutated: this is a rebuilt lifecycle view.
func (l *Ledger) projectSettledNoProposalRounds(ctx context.Context) error{
 reader,ok:=l.inputStore.(interface{
  SettledNoProposalUnits(context.Context,[32]byte)(map[domain.ID]bool,error)
 })
 if !ok{return nil} // in-memory admission fixtures have no WORK publisher
 settled,err:=reader.SettledNoProposalUnits(ctx,[32]byte(l.runID))
 if err!=nil{return err}
 for id,round:=range l.rounds{
  if round.Lifecycle!=0{continue}
  providers:=l.providers[idKey(id)]
  complete:=len(round.Slots)>0
  for _,slot:=range round.Slots{
   state:=providers[slot]
   if state==nil||state.Response==nil||!settled[state.Response.ID]{
    complete=false;break
   }
  }
  if complete{round.Lifecycle=2;l.rounds[id]=round}
 }
 return nil
}

 // SystemUnit is a read-only authenticated pending scheduling/trigger source.
 // It does not grant mutation authority outside CommitWork.
type SystemUnit struct {
 Kind uint8 // 2 = ScheduledOccurrence, 3 = TriggerActivation
 Source string
 ID domain.ID
 Digest domain.Digest
 Eligibility domain.EligibilityCoordinate
}
type systemUnitReader interface {
 SystemUnits(context.Context,[32]byte,domain.EligibilityCoordinate)([]SystemUnit,error)
}
func (l *Ledger) SystemUnits(ctx context.Context,upTo domain.EligibilityCoordinate)([]SystemUnit,error){
 if l==nil||l.inputStore==nil{return nil,errors.New("system-unit authority unavailable")}
 reader,ok:=l.inputStore.(systemUnitReader)
 if !ok{return nil,nil} // memory stores have no schedule or trigger authority
 return reader.SystemUnits(ctx,[32]byte(l.runID),upTo)
}

package sqlite

import (
	"bytes"
	"errors"
	"sort"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/genesis"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
)

// forkRoundHistoryProjection is ephemeral, read-free migration data. Its
// pending snapshot is validated by P1, but archived cause/ID authentication
// must also succeed before any child publication.
type forkRoundHistoryProjection struct {
	records             []ports.CanonicalRecord
	rounds              []forkGenesisRound
	roundMap            map[string][]any
	parentCoordinates   map[string]any
	pendingDispatchRefs []any
	clocks              map[uint64]snapshotClock
	offset              uint64
}

// projectForkRoundHistoryV2 authenticates the pending rounds and freezes
// their eligibility against the exact source clock, without any SQL.
func projectForkRoundHistoryV2(parent, child *genesis.Genesis, snapshot []any,
	archive forkHistoryArchive, origin, target *policy.Bundle) (forkRoundHistoryProjection, error) {
	records, events, journals := archive["input_ledger"], archive["event_store"], archive["decision_ledger"]
	lists, err := forkPendingParts(snapshot)
	if err != nil {
		return forkRoundHistoryProjection{}, err
	}
	clocks, err := roundHistoryClocksV2(parent, snapshot, journals, origin)
	if err != nil {
		return forkRoundHistoryProjection{}, err
	}
	rounds, err := migratePendingRoundRecords(parent, child, lists[4].([]any), records, events, origin, target, true)
	if err != nil {
		return forkRoundHistoryProjection{}, err
	}
	roundMap := map[string][]any{}
	parentCoordinates := map[string]any{}
	last, ok := asSigned64(snapshot[5])
	if !ok || last < -1 || last == int64(^uint64(0)>>1) {
		return forkRoundHistoryProjection{}, errors.New("source round clock ordinal invalid")
	}
	offset := uint64(last + 1)
	for i, raw := range lists[4].([]any) {
		ref := raw.([]any)
		v, err := codec.StrictDecodePayload(rounds[i].Canonical)
		if err != nil {
			return forkRoundHistoryProjection{}, err
		}
		fields := v.([]any)
		oldKey := string(ref[1].([]byte))
		parentCoordinates[oldKey] = fields[5]
		at := fields[5].([]any)
		instant, valid := asSigned64(at[0])
		ordinal, validOrdinal := at[1].(uint64)
		if !valid || !validOrdinal {
			return forkRoundHistoryProjection{}, errors.New("round eligibility malformed")
		}
		if instant == int64(child.InitialInstant) {
			if ordinal < offset {
				return forkRoundHistoryProjection{}, errors.New("pending round precedes source causal cut")
			}
			fields[5] = []any{instant, ordinal - offset}
			raw, err := codec.CanonicalPayload(fields)
			if err != nil {
				return forkRoundHistoryProjection{}, err
			}
			rounds[i].Canonical = raw
			rounds[i].Seal = [32]byte(identity.OpaqueContentHash(raw))
		}
		roundMap[oldKey] = fields
	}

	return forkRoundHistoryProjection{
		records: records, rounds: rounds, roundMap: roundMap,
		parentCoordinates: parentCoordinates, pendingDispatchRefs: lists[5].([]any),
		clocks: clocks, offset: offset,
	}, nil
}

// authenticateForkSlotDispatchesV2 checks carried dispatch identities, actor
// roster/slot, and frozen historical clock before linking their receipts.
func authenticateForkSlotDispatchesV2(parent *genesis.Genesis,
	records []ports.CanonicalRecord, roundMap map[string][]any,
	parentCoordinates map[string]any, clocks map[uint64]snapshotClock,
	origin *policy.Bundle) (map[string]*forkSlotHistory, error) {
	histories := map[string]*forkSlotHistory{}
	for _, record := range records {
		v, err := codec.StrictDecodePayload(record.Canonical)
		if err != nil {
			return nil, errors.New("corrupt archived input in dispatch authentication")
		}
		f, ok := v.([]any)
		if !ok {
			return nil, errors.New("round history has non-array input")
		}
		if len(f) != 11 {
			continue
		}
		if _, ok := f[4].(string); !ok {
			continue
		}
		oldRound, ok := f[3].([]byte)
		if !ok {
			continue
		}
		round, ok := roundMap[string(oldRound)]
		if !ok {
			continue
		}
		id, _, err := identity.New(origin).Sum("cote.csf.id.dispatch", "cote.csf.schema.dispatch-id-preimage", 1,
			[]any{f[1], f[2], f[3], f[4], f[5]})
		if err != nil || !equalBytes(f[0], id[:]) || record.Identity != [32]byte(id) || !equalBytes(f[1], parent.RunID[:]) {
			return nil, errors.New("source dispatch identity differs from causal preimage")
		}
		if !sameCanonical(f[6], round[3]) || !sameCanonical(f[7], parentCoordinates[string(oldRound)]) {
			return nil, errors.New("source dispatch actor/eligibility differs from round")
		}
		member := false
		for _, slot := range round[4].([]any) {
			if slot == f[4] {
				member = true
			}
		}
		if !member {
			return nil, errors.New("source dispatch slot absent from round")
		}
		ordinal, ok := f[5].(uint64)
		if !ok || ordinal > uint64(^uint32(0)) {
			return nil, errors.New("source dispatch ordinal invalid")
		}
		revision, ok := f[8].(uint64)
		historical, exists := clocks[revision]
		if !ok || !exists || historical.LastOrdinal == int64(^uint64(0)>>1) {
			return nil, errors.New("source dispatch revision not covered by frozen journal")
		}
		cycle, _, err := identity.New(origin).Sum("cote.csf.id.cycle", "cote.csf.schema.cycle-id-preimage", 1,
			[]any{parent.RunID[:], []any{historical.Instant, uint64(historical.LastOrdinal + 1)}})
		if err != nil || !equalBytes(f[2], cycle[:]) || !sameCanonical(f[9], historical.Instant) {
			return nil, errors.New("source dispatch differs from its authenticated historical clock")
		}
		if _, duplicate := histories[string(id[:])]; duplicate {
			return nil, errors.New("duplicate authenticated historical dispatch")
		}
		histories[string(id[:])] = &forkSlotHistory{oldID: [32]byte(id), fields: f}
	}

	return histories, nil
}

// attachForkSlotOutcomesV2 links revocations and responses from the immutable
// input prefix. The history map is private to this invocation of the migrator.
func attachForkSlotOutcomesV2(records []ports.CanonicalRecord, histories map[string]*forkSlotHistory) error {
	for _, record := range records {
		v, err := codec.StrictDecodePayload(record.Canonical)
		if err != nil {
			return errors.New("corrupt archived input in slot-receipt association")
		}
		f, ok := v.([]any)
		if !ok {
			return errors.New("archived slot receipt must be canonical array")
		}
		if len(f) == 3 {
			dispatch, ok := f[0].([]byte)
			if !ok {
				continue
			}
			h, ok := histories[string(dispatch)]
			if !ok {
				continue
			}
			reason, ok := f[1].(uint64)
			if !ok || reason > 255 || h.revoked != nil || record.Identity != [32]byte(identity.OpaqueContentHash(record.Canonical)) {
				return errors.New("invalid or duplicate source dispatch revocation")
			}
			h.revoked = f
		} else if len(f) == 11 || len(f) == 19 {
			dispatch, ok := f[1].([]byte)
			if !ok {
				continue
			}
			h, ok := histories[string(dispatch)]
			if !ok {
				continue
			}
			if h.response != nil {
				return errors.New("source slot answered more than once")
			}
			if !equalBytes(f[0], record.Identity[:]) {
				return errors.New("source response record ID differs from body")
			}
			h.response = f
		}
	}

	return nil
}

// validateAndOrderForkSlotHistoriesV2 enforces precise source-cut eligibility,
// full pending-roster closure, contradictory-settlement rejection and stable
// (round,slot,ordinal) order, independent of map iteration or record arrival.
func validateAndOrderForkSlotHistoriesV2(pendingDispatchRefs []any,
	histories map[string]*forkSlotHistory, snapshot []any, child *genesis.Genesis,
	offset uint64, origin *policy.Bundle) ([]*forkSlotHistory, error) {
	// Every live source dispatch must be owned by one migrated round. Settled
	// responses belonging to other rounds remain historical archive only.
	live := map[string][]any{}
	for _, raw := range pendingDispatchRefs {
		ref := raw.([]any)
		live[string(ref[1].([]byte))] = ref
	}
	ordered := make([]*forkSlotHistory, 0, len(histories))
	for _, h := range histories {
		if h.revoked != nil && h.response != nil {
			return nil, errors.New("source dispatch both revoked and answered")
		}
		if h.revoked == nil {
			f := h.fields
			eligible := f[7].([]any)
			when, valid := asSigned64(eligible[0])
			atOrdinal, validOrdinal := eligible[1].(uint64)
			if !sameCanonical(f[8], snapshot[3]) || !sameCanonical(f[9], snapshot[2]) ||
				!valid || !validOrdinal || when != int64(child.InitialInstant) || atOrdinal != offset {
				return nil, errors.New("unrevoked dispatch requires exact eligible source cut")
			}
		}
		if h.revoked == nil && h.response == nil {
			ref, ok := live[string(h.oldID[:])]
			d, _, err := identity.New(origin).Sum("cote.csf.digest.slot-dispatch", "cote.csf.schema.slot-dispatch", 1, h.fields)
			if !ok || err != nil || !equalBytes(ref[2], d[:]) {
				return nil, errors.New("open source dispatch absent from checkpoint")
			}
			delete(live, string(h.oldID[:]))
		}
		ordered = append(ordered, h)
	}
	if len(live) != 0 {
		return nil, errors.New("checkpoint dispatch has no migrated pending round")
	}
	sort.Slice(ordered, func(i, j int) bool {
		a, b := ordered[i].fields, ordered[j].fields
		if c := bytes.Compare(a[3].([]byte), b[3].([]byte)); c != 0 {
			return c < 0
		}
		if a[4] != b[4] {
			return a[4].(string) < b[4].(string)
		}
		return a[5].(uint64) < b[5].(uint64)
	})
	// Validate causal redispatch sequence before assigning any child identity.
	var previous *forkSlotHistory
	for _, h := range ordered {
		f := h.fields
		if previous == nil || !sameCanonical(previous.fields[3], f[3]) || previous.fields[4] != f[4] {
			if f[5] != uint64(0) {
				return nil, errors.New("first source dispatch ordinal is not zero")
			}
		} else if previous.revoked == nil || f[5].(uint64) != previous.fields[5].(uint64)+1 {
			return nil, errors.New("source redispatch lacks immediate durable revocation")
		}
		previous = h
	}
	return ordered, nil
}

// remintForkSlotHistoriesV2 changes child IDs and relative time/ordinal,
// preserves historical intent and source bytes, and returns pure records
// for later transactional P3 publication.
func remintForkSlotHistoriesV2(parent, child *genesis.Genesis, snapshot []any,
	archive forkHistoryArchive, roundMap map[string][]any, ordered []*forkSlotHistory,
	origin, target *policy.Bundle) ([]forkSlotRecord, error) {
	childCycle, _, err := identity.New(target).Sum("cote.csf.id.cycle", "cote.csf.schema.cycle-id-preimage", 1,
		[]any{child.RunID[:], []any{int64(child.InitialInstant), uint64(0)}})
	if err != nil {
		return nil, err
	}
	var out []forkSlotRecord
	for _, h := range ordered {
		f := h.fields
		round := roundMap[string(f[3].([]byte))]
		fields := append([]any(nil), f...)
		fields[1] = child.RunID[:]
		fields[2] = childCycle[:]
		fields[3] = round[0]
		fields[8] = uint64(0)
		fields[9] = int64(child.InitialInstant)
		fields[7] = round[5]
		id, _, err := identity.New(target).Sum("cote.csf.id.dispatch", "cote.csf.schema.dispatch-id-preimage", 1,
			[]any{fields[1], fields[2], fields[3], fields[4], fields[5]})
		if err != nil {
			return nil, err
		}
		fields[0] = id[:]
		raw, err := codec.CanonicalPayload(fields)
		if err != nil {
			return nil, err
		}
		out = append(out, forkSlotRecord{CanonicalRecord: ports.CanonicalRecord{Identity: [32]byte(id), Canonical: raw}})
		if h.revoked != nil {
			raw, err := codec.CanonicalPayload([]any{id[:], h.revoked[1], h.revoked[2]})
			if err != nil {
				return nil, err
			}
			out = append(out, forkSlotRecord{CanonicalRecord: ports.CanonicalRecord{Identity: [32]byte(identity.OpaqueContentHash(raw)), Canonical: raw}})
		}
		if h.response != nil {
			if len(h.response) == 19 {
				if err := authenticateRoundIntentionV2(h.response[16], parent, snapshot, archive, origin); err != nil {
					return nil, err
				}
			}
			response, err := migrateSlotResponseV2(h.response, f, fields, origin, target)
			if err != nil {
				return nil, err
			}
			out = append(out, response)
		}
	}

	return out, nil
}

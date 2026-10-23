package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/genesis"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
)

type forkSlotRecord struct {
	ports.CanonicalRecord
	SlotKey *[32]byte
}

type forkSlotHistory struct {
	oldID             [32]byte
	fields            []any
	revoked, response []any
}

// Round history is a deterministic migration birth authenticated against the
// archived checkpoint. The child starts at revision/ordinal zero. Only requests
// against the exact source cut can be carried; stale requests require explicit
// source settlement. No model is called and no response is fabricated.
func migrateRoundHistoryV2(parent, child *genesis.Genesis, snapshot []any,
	archive forkHistoryArchive, origin, target *policy.Bundle) ([]forkGenesisRound, []forkSlotRecord, error) {
	records, events, journals := archive["input_ledger"], archive["event_store"], archive["decision_ledger"]
	pending := snapshot[11].([]any)
	env, err := codec.StrictDecodeEnvelope(pending[2].([]byte))
	if err != nil {
		return nil, nil, err
	}
	lists := env.Value.([]any)
	clocks, err := roundHistoryClocksV2(parent, snapshot, journals, origin)
	if err != nil {
		return nil, nil, err
	}
	rounds, err := migratePendingRoundRecords(parent, child, lists[4].([]any), records, events, origin, target, true)
	if err != nil {
		return nil, nil, err
	}
	roundMap := map[string][]any{}
	parentCoordinates := map[string]any{}
	last, ok := asSigned64(snapshot[5])
	if !ok || last < -1 || last == int64(^uint64(0)>>1) {
		return nil, nil, errors.New("source round clock ordinal invalid")
	}
	offset := uint64(last + 1)
	for i, raw := range lists[4].([]any) {
		ref := raw.([]any)
		v, err := codec.StrictDecodePayload(rounds[i].Canonical)
		if err != nil {
			return nil, nil, err
		}
		fields := v.([]any)
		oldKey := string(ref[1].([]byte))
		parentCoordinates[oldKey] = fields[5]
		at := fields[5].([]any)
		instant, valid := asSigned64(at[0])
		ordinal, validOrdinal := at[1].(uint64)
		if !valid || !validOrdinal {
			return nil, nil, errors.New("round eligibility malformed")
		}
		if instant == int64(child.InitialInstant) {
			if ordinal < offset {
				return nil, nil, errors.New("pending round precedes source causal cut")
			}
			fields[5] = []any{instant, ordinal - offset}
			raw, err := codec.CanonicalPayload(fields)
			if err != nil {
				return nil, nil, err
			}
			rounds[i].Canonical = raw
			rounds[i].Seal = [32]byte(identity.OpaqueContentHash(raw))
		}
		roundMap[oldKey] = fields
	}
	histories := map[string]*forkSlotHistory{}
	for _, record := range records {
		v, err := codec.StrictDecodePayload(record.Canonical)
		if err != nil {
			continue
		} // ingress envelopes
		f, ok := v.([]any)
		if !ok {
			return nil, nil, errors.New("round history has non-array input")
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
			return nil, nil, errors.New("source dispatch identity differs from causal preimage")
		}
		if !sameCanonical(f[6], round[3]) || !sameCanonical(f[7], parentCoordinates[string(oldRound)]) {
			return nil, nil, errors.New("source dispatch actor/eligibility differs from round")
		}
		member := false
		for _, slot := range round[4].([]any) {
			if slot == f[4] {
				member = true
			}
		}
		if !member {
			return nil, nil, errors.New("source dispatch slot absent from round")
		}
		ordinal, ok := f[5].(uint64)
		if !ok || ordinal > uint64(^uint32(0)) {
			return nil, nil, errors.New("source dispatch ordinal invalid")
		}
		revision, ok := f[8].(uint64)
		historical, exists := clocks[revision]
		if !ok || !exists || historical.LastOrdinal == int64(^uint64(0)>>1) {
			return nil, nil, errors.New("source dispatch revision not covered by frozen journal")
		}
		cycle, _, err := identity.New(origin).Sum("cote.csf.id.cycle", "cote.csf.schema.cycle-id-preimage", 1,
			[]any{parent.RunID[:], []any{historical.Instant, uint64(historical.LastOrdinal + 1)}})
		if err != nil || !equalBytes(f[2], cycle[:]) || !sameCanonical(f[9], historical.Instant) {
			return nil, nil, errors.New("source dispatch differs from its authenticated historical clock")
		}
		histories[string(id[:])] = &forkSlotHistory{oldID: [32]byte(id), fields: f}
	}
	for _, record := range records {
		v, err := codec.StrictDecodePayload(record.Canonical)
		if err != nil {
			continue
		}
		f := v.([]any)
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
				return nil, nil, errors.New("invalid or duplicate source dispatch revocation")
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
				return nil, nil, errors.New("source slot answered more than once")
			}
			if !equalBytes(f[0], record.Identity[:]) {
				return nil, nil, errors.New("source response record ID differs from body")
			}
			h.response = f
		}
	}
	// Every live source dispatch must be owned by one migrated round. Settled
	// responses belonging to other rounds remain historical archive only.
	live := map[string][]any{}
	for _, raw := range lists[5].([]any) {
		ref := raw.([]any)
		live[string(ref[1].([]byte))] = ref
	}
	ordered := make([]*forkSlotHistory, 0, len(histories))
	for _, h := range histories {
		if h.revoked != nil && h.response != nil {
			return nil, nil, errors.New("source dispatch both revoked and answered")
		}
		if h.revoked == nil {
			f := h.fields
			eligible := f[7].([]any)
			when, valid := asSigned64(eligible[0])
			atOrdinal, validOrdinal := eligible[1].(uint64)
			if !sameCanonical(f[8], snapshot[3]) || !sameCanonical(f[9], snapshot[2]) ||
				!valid || !validOrdinal || when != int64(child.InitialInstant) || atOrdinal != offset {
				return nil, nil, errors.New("unrevoked dispatch requires exact eligible source cut")
			}
		}
		if h.revoked == nil && h.response == nil {
			ref, ok := live[string(h.oldID[:])]
			d, _, err := identity.New(origin).Sum("cote.csf.digest.slot-dispatch", "cote.csf.schema.slot-dispatch", 1, h.fields)
			if !ok || err != nil || !equalBytes(ref[2], d[:]) {
				return nil, nil, errors.New("open source dispatch absent from checkpoint")
			}
			delete(live, string(h.oldID[:]))
		}
		ordered = append(ordered, h)
	}
	if len(live) != 0 {
		return nil, nil, errors.New("checkpoint dispatch has no migrated pending round")
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
	childCycle, _, err := identity.New(target).Sum("cote.csf.id.cycle", "cote.csf.schema.cycle-id-preimage", 1,
		[]any{child.RunID[:], []any{int64(child.InitialInstant), uint64(0)}})
	if err != nil {
		return nil, nil, err
	}
	var out []forkSlotRecord
	var previous *forkSlotHistory
	for _, h := range ordered {
		f := h.fields
		if previous == nil || !sameCanonical(previous.fields[3], f[3]) || previous.fields[4] != f[4] {
			if f[5] != uint64(0) {
				return nil, nil, errors.New("first source dispatch ordinal is not zero")
			}
		} else if previous.revoked == nil || f[5].(uint64) != previous.fields[5].(uint64)+1 {
			return nil, nil, errors.New("source redispatch lacks immediate durable revocation")
		}
		previous = h
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
			return nil, nil, err
		}
		fields[0] = id[:]
		raw, err := codec.CanonicalPayload(fields)
		if err != nil {
			return nil, nil, err
		}
		out = append(out, forkSlotRecord{CanonicalRecord: ports.CanonicalRecord{Identity: [32]byte(id), Canonical: raw}})
		if h.revoked != nil {
			raw, err := codec.CanonicalPayload([]any{id[:], h.revoked[1], h.revoked[2]})
			if err != nil {
				return nil, nil, err
			}
			out = append(out, forkSlotRecord{CanonicalRecord: ports.CanonicalRecord{Identity: [32]byte(identity.OpaqueContentHash(raw)), Canonical: raw}})
		}
		if h.response != nil {
			if len(h.response) == 19 {
				if err := authenticateRoundIntentionV2(h.response[16], parent, snapshot, archive, origin); err != nil {
					return nil, nil, err
				}
			}
			response, err := migrateSlotResponseV2(h.response, f, fields, origin, target)
			if err != nil {
				return nil, nil, err
			}
			out = append(out, response)
		}
	}
	return rounds, out, nil
}

// Revoked receipts may predate the source checkpoint. Reconstruct their clock
// from the frozen journals, rather than accepting caller-supplied cycle/revision
// pairs. The archive prefix and the origin snapshot authenticate these journals.
func roundHistoryClocksV2(parent *genesis.Genesis, snapshot []any, journals []ports.CanonicalRecord, bundle *policy.Bundle) (map[uint64]snapshotClock, error) {
	commits := make([][]any, 0)
	for _, record := range journals {
		value, err := codec.StrictDecodePayload(record.Canonical)
		if err != nil {
			return nil, err
		}
		f, ok := value.([]any)
		if !ok || len(f) != 24 {
			continue
		}
		id, _, err := identity.New(bundle).Sum("cote.csf.id.cycle-commit", "cote.csf.schema.cycle-commit-id-preimage", 1, f[:3])
		if err != nil || record.Identity != [32]byte(id) || !equalBytes(f[0], parent.RunID[:]) {
			return nil, errors.New("source historical journal identity invalid")
		}
		if _, ok := f[8].(uint64); !ok {
			return nil, errors.New("source historical revision malformed")
		}
		commits = append(commits, f)
	}
	sort.Slice(commits, func(i, j int) bool { return commits[i][8].(uint64) < commits[j][8].(uint64) })
	current := snapshotClock{Instant: int64(parent.InitialInstant), LastOrdinal: -1}
	clocks := map[uint64]snapshotClock{0: current}
	for _, f := range commits {
		if err := applyHistoricalClockCommit(&current, f); err != nil {
			return nil, err
		}
		clocks[current.Revision] = current
	}
	if !sameCanonical(current.Revision, snapshot[3]) || !sameCanonical(current.Instant, snapshot[2]) || !sameCanonical(current.LastOrdinal, snapshot[5]) {
		return nil, errors.New("source historical clock does not reach frozen snapshot")
	}
	return clocks, nil
}

// Intention references describe actor provenance, never a world assertion. A
// reference may remain historical across a fork only when its root is present
// in the authenticated cut. Genesis is already archived and independently
// verified by lineage replay. Other roots need their corresponding resolver.
func authenticateRoundIntentionV2(value any, parent *genesis.Genesis, snapshot []any, archive forkHistoryArchive, bundle *policy.Bundle) error {
	option, ok := value.([]any)
	if !ok || len(option) == 0 {
		return errors.New("action intention option malformed")
	}
	if len(option) == 1 && option[0] == uint64(0) {
		return nil
	}
	if len(option) != 2 || option[0] != uint64(1) {
		return errors.New("action intention option malformed")
	}
	ref, ok := option[1].([]any)
	if !ok {
		return errors.New("action intention reference malformed")
	}
	return authenticateArchivedCausalRef(ref, parent, snapshot, archive, bundle)
}

func migrateSlotResponseV2(old, source, dispatch []any, origin, target *policy.Bundle) (forkSlotRecord, error) {
	var zero forkSlotRecord
	kind := uint64(1)
	idTag, digestTag, bodySchema := "cote.csf.id.no-proposal", "cote.csf.digest.no-proposal-unit", "cote.csf.schema.no-proposal.body"
	roundIndex, slotIndex, timeIndex, revisionIndex := 4, 5, 7, 8
	if len(old) == 19 {
		kind = 0
		idTag = "cote.csf.id.action-proposal"
		digestTag = "cote.csf.digest.action-proposal-unit"
		bodySchema = "cote.csf.schema.action-proposal.body"
		roundIndex, slotIndex, timeIndex, revisionIndex = 13, 14, 11, 15
	}
	oldID, _, err := identity.New(origin).Sum(idTag, "cote.csf.schema.slot-response-id-preimage", 1, []any{source[0], kind})
	if err != nil || !equalBytes(old[0], oldID[:]) || !sameCanonical(old[2], source[1]) || !sameCanonical(old[3], source[2]) ||
		!sameCanonical(old[roundIndex], source[3]) || old[slotIndex] != source[4] || !sameCanonical(old[6], source[6]) ||
		!sameCanonical(old[timeIndex], source[9]) || !sameCanonical(old[revisionIndex], source[8]) {
		return zero, errors.New("source response does not match its dispatch")
	}
	oldDigest, _, err := identity.New(origin).Sum(digestTag, bodySchema, 1, old[:len(old)-1])
	if err != nil || !equalBytes(old[len(old)-1], oldDigest[:]) {
		return zero, errors.New("source response digest invalid")
	}
	body := append([]any(nil), old[:len(old)-1]...)
	id, _, err := identity.New(target).Sum(idTag, "cote.csf.schema.slot-response-id-preimage", 1, []any{dispatch[0], kind})
	if err != nil {
		return zero, err
	}
	body[0] = id[:]
	body[1] = dispatch[0]
	body[2] = dispatch[1]
	body[3] = dispatch[2]
	body[roundIndex] = dispatch[3]
	body[revisionIndex] = dispatch[8]
	digest, _, err := identity.New(target).Sum(digestTag, bodySchema, 1, body)
	if err != nil {
		return zero, err
	}
	raw, err := codec.CanonicalPayload(append(body, digest[:]))
	if err != nil {
		return zero, err
	}
	keyRaw, err := codec.CanonicalPayload([]any{dispatch[3], dispatch[4]})
	if err != nil {
		return zero, err
	}
	key := [32]byte(identity.OpaqueContentHash(append([]byte("csf-slot-response-v1\x00"), keyRaw...)))
	return forkSlotRecord{CanonicalRecord: ports.CanonicalRecord{Identity: [32]byte(id), Canonical: raw}, SlotKey: &key}, nil
}

func publishRoundHistoryV2(ctx context.Context, tx *sql.Tx, records []forkSlotRecord) error {
	for _, record := range records {
		if _, err := tx.ExecContext(ctx, "INSERT INTO input_ledger(id,canonical) VALUES(?,?)", record.Identity[:], record.Canonical); err != nil {
			return err
		}
		if record.SlotKey != nil {
			if _, err := tx.ExecContext(ctx, "INSERT INTO input_slot_responses(slot_key,record_id,canonical) VALUES(?,?,?)",
				record.SlotKey[:], record.Identity[:], record.Canonical); err != nil {
				return err
			}
		}
	}
	return nil
}

func verifyRoundHistoryV2(ctx context.Context, tx *sql.Tx, parent, child *genesis.Genesis, snapshot []any, origin, target *policy.Bundle) error {
	id, _, _, _, err := target.CodecIdentity()
	if err != nil {
		return err
	}
	if id != policy.CodecPolicyV2ID {
		return nil
	}
	archive, err := readForkHistoryArchive(ctx, tx, [32]byte(child.RunID))
	if err != nil {
		return err
	}
	rounds, slots, err := migrateRoundHistoryV2(parent, child, snapshot, archive, origin, target)
	if err != nil {
		return err
	}
	for _, round := range rounds {
		var raw, seal []byte
		if err := tx.QueryRowContext(ctx, "SELECT i.canonical,s.canonical_hash FROM input_ledger i JOIN round_integrity_seals s ON s.id=i.id WHERE i.id=? AND s.run_id=?", round.ID[:], child.RunID[:]).Scan(&raw, &seal); err != nil {
			return err
		}
		if !bytes.Equal(raw, round.Canonical) || !bytes.Equal(seal, round.Seal[:]) {
			return errors.New("migrated round differs from archived source")
		}
	}
	for _, record := range slots {
		var raw []byte
		if err := tx.QueryRowContext(ctx, "SELECT canonical FROM input_ledger WHERE id=?", record.Identity[:]).Scan(&raw); err != nil {
			return fmt.Errorf("missing migrated slot history: %w", err)
		}
		if !bytes.Equal(raw, record.Canonical) {
			return errors.New("migrated dispatch/response/revocation changed")
		}
		if record.SlotKey != nil {
			var id, raw []byte
			if err := tx.QueryRowContext(ctx, "SELECT record_id,canonical FROM input_slot_responses WHERE slot_key=?", record.SlotKey[:]).Scan(&id, &raw); err != nil {
				return err
			}
			if !bytes.Equal(id, record.Identity[:]) || !bytes.Equal(raw, record.Canonical) {
				return errors.New("migrated response lost atomic slot key")
			}
		}
	}
	return nil
}

// Read exactly the frozen cut, including when the source has since appended
// another response or revocation at the same world revision.
func (s *Store) snapshotInputRecords(ctx context.Context, id [32]byte) ([]ports.CanonicalRecord, error) {
	return s.snapshotRoundLedgerRecords(ctx, id, "input_ledger")
}
func (s *Store) snapshotRoundLedgerRecords(ctx context.Context, id [32]byte, ledger string) ([]ports.CanonicalRecord, error) {
	valid := false
	for _, name := range snapshotLedgerNames {
		if ledger == name {
			valid = true
		}
	}
	if !valid {
		return nil, errors.New("unsupported round migration ledger")
	}
	rows, err := s.db.QueryContext(ctx, "SELECT m.record_id,i.canonical FROM snapshot_prefix_members m JOIN "+ledger+" i ON i.id=m.record_id WHERE m.snapshot_id=? AND m.ledger_name=? ORDER BY m.ordinal", id[:], ledger)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []ports.CanonicalRecord
	for rows.Next() {
		var id, raw []byte
		if err := rows.Scan(&id, &raw); err != nil {
			return nil, err
		}
		if len(id) != 32 {
			return nil, errors.New("snapshot input ID invalid")
		}
		records = append(records, ports.CanonicalRecord{Identity: bytesToID(id), Canonical: raw})
	}
	return records, rows.Err()
}

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
// migrateRoundHistoryV2 orchestrates four independent read-free stages:
// round projection, authenticated dispatches, receipt linkage / ordering,
// and new child IDs. All SQL publication remains in the fork publisher.
func migrateRoundHistoryV2(parent, child *genesis.Genesis, snapshot []any,
	archive forkHistoryArchive, origin, target *policy.Bundle) ([]forkGenesisRound, []forkSlotRecord, error) {
	projection, err := projectForkRoundHistoryV2(parent, child, snapshot, archive, origin, target)
	if err != nil {
		return nil, nil, err
	}
	histories, err := authenticateForkSlotDispatchesV2(parent, projection.records,
		projection.roundMap, projection.parentCoordinates, projection.clocks, origin)
	if err != nil {
		return nil, nil, err
	}
	if err := attachForkSlotOutcomesV2(projection.records, histories); err != nil {
		return nil, nil, err
	}
	ordered, err := validateAndOrderForkSlotHistoriesV2(projection.pendingDispatchRefs,
		histories, snapshot, child, projection.offset, origin)
	if err != nil {
		return nil, nil, err
	}
	slots, err := remintForkSlotHistoriesV2(parent, child, snapshot, archive,
		projection.roundMap, ordered, origin, target)
	if err != nil {
		return nil, nil, err
	}
	return projection.rounds, slots, nil
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
	if err := loadNativeRoundIntentions(ctx, tx, parent, child, snapshot, archive, origin); err != nil {
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

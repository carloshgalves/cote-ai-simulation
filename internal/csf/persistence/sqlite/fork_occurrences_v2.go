package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"errors"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/genesis"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
)

func prepareForkOccurrenceRoots(ctx context.Context, s *Store, parent *genesis.Genesis, refs []any) ([]ports.CanonicalRecord, error) {
	var roots []ports.CanonicalRecord
	for _, raw := range refs {
		ref, ok := raw.([]any)
		if !ok || len(ref) != 3 || ref[0] != uint64(6) {
			return nil, errors.New("occurrence migration ref malformed")
		}
		id, ok := idField(ref[1])
		if !ok {
			return nil, errors.New("occurrence migration ID malformed")
		}
		var canonical []byte
		if err := s.db.QueryRowContext(ctx, "SELECT canonical FROM scheduled_occurrences WHERE run_id=? AND id=?", parent.RunID[:], id[:]).Scan(&canonical); err != nil {
			return nil, err
		}
		roots = append(roots, ports.CanonicalRecord{Identity: id, Canonical: canonical})
	}
	return roots, nil
}

func migrateOccurrenceBirthsV2(parent, child *genesis.Genesis, snapshot []any, refs []any, roots []ports.CanonicalRecord, occupied []forkGenesisOccurrence, archive forkHistoryArchive, origin, target *policy.Bundle) ([]forkGenesisOccurrence, error) {
	local := forkHistoryArchive{}
	for key, records := range archive {
		local[key] = records
	}
	local[nativeArchiveKey(6)] = append(append([]ports.CanonicalRecord(nil), archive[nativeArchiveKey(6)]...), roots...)
	byID := map[string][]any{}
	for _, record := range roots {
		value, err := codec.StrictDecodePayload(record.Canonical)
		if err != nil {
			return nil, err
		}
		f, ok := value.([]any)
		if !ok || len(f) != 15 || !equalBytes(f[0], record.Identity[:]) {
			return nil, errors.New("occurrence migration root malformed")
		}
		key := string(record.Identity[:])
		if _, duplicate := byID[key]; duplicate {
			return nil, errors.New("duplicate occurrence migration root")
		}
		byID[key] = f
	}
	if len(byID) != len(refs) {
		return nil, errors.New("occurrence proof roster differs from checkpoint")
	}
	used := map[[2]uint64]bool{}
	for _, birth := range occupied {
		value, err := codec.StrictDecodePayload(birth.Canonical)
		if err != nil {
			return nil, err
		}
		f, ok := value.([]any)
		if !ok || len(f) != 15 {
			return nil, errors.New("reserved occurrence birth malformed")
		}
		role, okRole := f[10].(uint64)
		ordinal, okOrdinal := f[11].(uint64)
		if !okRole || !okOrdinal {
			return nil, errors.New("reserved occurrence coordinate malformed")
		}
		key := [2]uint64{role, ordinal}
		if used[key] {
			return nil, errors.New("duplicate reserved occurrence coordinate")
		}
		used[key] = true
	}
	roles := map[string]uint64{}
	ordinals := map[string]uint64{}
	for _, raw := range refs {
		ref := raw.([]any)
		if err := authenticateArchivedCausalRef(ref, parent, snapshot, local, origin); err != nil {
			return nil, err
		}
		key := string(ref[1].([]byte))
		f, exists := byID[key]
		if !exists {
			return nil, errors.New("occurrence source proof absent")
		}
		role, okRole := f[10].(uint64)
		ordinal, okOrdinal := f[11].(uint64)
		cause, ok := f[9].([]any)
		if !okRole || role > 65535 || !okOrdinal || ordinal > uint64(^uint32(0)) || !ok || len(cause) != 2 {
			return nil, errors.New("occurrence source coordinates malformed")
		}
		switch cause[0] {
		case uint64(2):
			coordinate := [2]uint64{role, ordinal}
			if used[coordinate] {
				roles[key] = role
			} else {
				used[coordinate] = true
				ordinals[key] = ordinal
			}
		case uint64(0):
			if err := authenticateDeferredOccurrenceBirth(parent, snapshot, f, archive, origin); err != nil {
				return nil, err
			}
			roles[key] = role
		default:
			return nil, errors.New("ordinary occurrence requires Genesis or committed Event origin")
		}
	}
	allocated, err := allocateForkEventCoordinates(roles, used)
	if err != nil {
		return nil, err
	}
	for id, ordinal := range allocated {
		ordinals[id] = ordinal
	}
	last, ok := asSigned64(snapshot[5])
	if !ok || last < -1 || last == int64(^uint64(0)>>1) {
		return nil, errors.New("occurrence source clock ordinal malformed")
	}
	offset := uint64(last + 1)
	out := make([]forkGenesisOccurrence, 0, len(refs))
	for _, raw := range refs {
		ref := raw.([]any)
		key := string(ref[1].([]byte))
		f := append([]any(nil), byID[key]...)
		f[11] = ordinals[key]
		at, ok := f[4].([]any)
		if !ok || len(at) != 2 {
			return nil, errors.New("occurrence eligibility malformed")
		}
		instant, valid := asSigned64(at[0])
		ordinal, validOrdinal := at[1].(uint64)
		if !valid || !validOrdinal {
			return nil, errors.New("occurrence eligibility fields malformed")
		}
		if instant == int64(child.InitialInstant) {
			if ordinal < offset {
				return nil, errors.New("pending occurrence precedes source cut")
			}
			f[4] = []any{instant, ordinal - offset}
		}
		birth, err := remintForkOccurrence(child, f, target)
		if err != nil {
			return nil, err
		}
		out = append(out, birth)
	}
	return out, nil
}

func forkRosterContains(raw any, id any) bool {
	items, ok := raw.([]any)
	if !ok {
		return false
	}
	for _, item := range items {
		if sameCanonical(item, id) {
			return true
		}
	}
	return false
}

// A valid Event ID alone is insufficient: only its committed DEFER decision
// authorizes this successor occurrence in the frozen source history.
func authenticateDeferredOccurrenceBirth(parent *genesis.Genesis, snapshot, fields []any, archive forkHistoryArchive, b *policy.Bundle) error {
	cause := fields[9].([]any)[1].([]any)
	for _, record := range archive["decision_ledger"] {
		value, err := codec.StrictDecodePayload(record.Canonical)
		if err != nil {
			return err
		}
		f, ok := value.([]any)
		if !ok || len(f) != 16 || !equalBytes(f[1], parent.RunID[:]) || f[5] != uint64(2) || !sameCanonical(f[13], []any{uint64(1), fields[0]}) || !forkRosterContains(f[12], cause[0]) {
			continue
		}
		subject, ok := f[4].([]any)
		if !ok || len(subject) != 3 || subject[0] != uint64(2) {
			continue
		}
		if err := authenticateArchivedCausalRef([]any{uint64(10), f[0], f[15]}, parent, snapshot, archive, b); err != nil {
			return err
		}
		for _, journal := range archive["decision_ledger"] {
			value, err := codec.StrictDecodePayload(journal.Canonical)
			if err != nil {
				return err
			}
			j, ok := value.([]any)
			if !ok || len(j) != 24 || !equalBytes(j[0], parent.RunID[:]) || !sameCanonical(j[1], f[2]) {
				continue
			}
			revision, ok := j[8].(uint64)
			if ok && revision <= snapshot[3].(uint64) && forkRosterContains(j[16], f[0]) && forkRosterContains(j[15], cause[0]) && forkRosterContains(j[11], subject[1]) {
				return nil
			}
		}
	}
	return errors.New("Event-owned occurrence lacks committed DEFER authorization")
}

func verifyOccurrenceBirthsV2(ctx context.Context, tx *sql.Tx, parent, child *genesis.Genesis, snapshot []any, origin, target *policy.Bundle) error {
	policyID, _, _, _, err := target.CodecIdentity()
	if err != nil {
		return err
	}
	if policyID != policy.CodecPolicyV2ID {
		return nil
	}
	parts, err := forkPendingParts(snapshot)
	if err != nil {
		return err
	}
	expected := map[string][]any{}
	for _, raw := range parts[0].([]any) {
		ref := raw.([]any)
		expected[string(ref[1].([]byte))] = ref
	}
	var occupied []forkGenesisOccurrence
	imported := map[string]bool{}
	rows, err := tx.QueryContext(ctx, "SELECT canonical FROM fork_communication_imports WHERE child_run_id=?", child.RunID[:])
	if err != nil {
		return err
	}
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			rows.Close()
			return err
		}
		value, err := codec.StrictDecodePayload(raw)
		if err != nil {
			rows.Close()
			return err
		}
		f, ok := value.([]any)
		if !ok || len(f) != 20 {
			rows.Close()
			return errors.New("communication occurrence proof malformed")
		}
		key := string(communicationBytes(f[10]))
		if _, ok := expected[key]; !ok {
			rows.Close()
			return errors.New("communication source occurrence absent from checkpoint")
		}
		delete(expected, key)
		id, ok := idField(f[13])
		rawChild, rawOK := f[19].([]byte)
		if !ok || !rawOK {
			rows.Close()
			return errors.New("communication occurrence birth malformed")
		}
		occupied = append(occupied, forkGenesisOccurrence{ID: id, Canonical: rawChild})
		imported[string(id[:])] = true
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	var roots []ports.CanonicalRecord
	provided := map[string]bool{}
	rows, err = tx.QueryContext(ctx, "SELECT parent_occurrence_id,canonical FROM fork_occurrence_imports WHERE child_run=? ORDER BY parent_occurrence_id", child.RunID[:])
	if err != nil {
		return err
	}
	for rows.Next() {
		var id, raw []byte
		if err := rows.Scan(&id, &raw); err != nil {
			rows.Close()
			return err
		}
		key := string(id)
		if len(id) != 32 || expected[key] == nil {
			rows.Close()
			return errors.New("unowned occurrence source proof")
		}
		provided[key] = true
		roots = append(roots, ports.CanonicalRecord{Identity: bytesToID(id), Canonical: raw})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	actual := map[[32]byte][]byte{}
	rows, err = tx.QueryContext(ctx, "SELECT id,canonical FROM scheduled_occurrences WHERE run_id=? ORDER BY id", child.RunID[:])
	if err != nil {
		return err
	}
	for rows.Next() {
		var id, raw []byte
		if err := rows.Scan(&id, &raw); err != nil {
			rows.Close()
			return err
		}
		value, err := codec.StrictDecodePayload(raw)
		if err != nil {
			rows.Close()
			return err
		}
		f, ok := value.([]any)
		if !ok || len(f) != 15 || len(id) != 32 {
			rows.Close()
			return errors.New("child occurrence malformed")
		}
		cause, ok := f[9].([]any)
		if !ok || len(cause) != 2 {
			rows.Close()
			return errors.New("child occurrence origin malformed")
		}
		if cause[0] != uint64(2) {
			continue
		}
		actual[bytesToID(id)] = raw
		if imported[string(id)] {
			continue
		}
		// Compatibility with earlier Genesis-only V2 births. Matching both
		// the parent ID and exact checkpoint digest prevents Event synthesis.
		fields := append([]any(nil), f[:14]...)
		sourceCause := []any{uint64(2), []any{parent.ID[:], parent.Digest[:]}}
		oldID, _, err := identity.New(origin).Sum("cote.csf.id.occurrence", "cote.csf.schema.occurrence-id-preimage", 1, []any{sourceCause, f[10], f[11]})
		if err != nil {
			rows.Close()
			return err
		}
		key := string(oldID[:])
		ref := expected[key]
		if ref == nil || provided[key] {
			continue
		}
		fields[0] = oldID[:]
		fields[1] = parent.RunID[:]
		fields[9] = sourceCause
		digest, _, err := identity.New(origin).Sum("cote.csf.digest.occurrence-unit", "cote.csf.schema.scheduled-occurrence.body", 1, fields)
		if err != nil {
			rows.Close()
			return err
		}
		if !equalBytes(ref[2], digest[:]) {
			continue
		}
		canonical, err := codec.CanonicalPayload(append(fields, digest[:]))
		if err != nil {
			rows.Close()
			return err
		}
		roots = append(roots, ports.CanonicalRecord{Identity: [32]byte(oldID), Canonical: canonical})
		provided[key] = true
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if len(provided) != len(expected) {
		return errors.New("pending occurrence source proof missing")
	}
	refs := make([]any, 0, len(expected))
	for _, raw := range parts[0].([]any) {
		ref := raw.([]any)
		if expected[string(ref[1].([]byte))] != nil {
			refs = append(refs, raw)
		}
	}
	archive, err := readForkHistoryArchive(ctx, tx, [32]byte(child.RunID))
	if err != nil {
		return err
	}
	births, err := migrateOccurrenceBirthsV2(parent, child, snapshot, refs, roots, occupied, archive, origin, target)
	if err != nil {
		return err
	}
	for _, birth := range append(occupied, births...) {
		raw, ok := actual[birth.ID]
		if !ok || !bytes.Equal(raw, birth.Canonical) {
			return errors.New("migrated occurrence differs from source birth")
		}
		delete(actual, birth.ID)
	}
	if len(actual) != 0 {
		return errors.New("unowned initial child occurrence")
	}
	return nil
}

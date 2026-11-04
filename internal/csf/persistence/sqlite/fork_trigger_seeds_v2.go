package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/fork"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/genesis"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/trigger"
)

// The manifest preserves source proofs. Runtime seeds come exclusively from
// the snapshot authenticated by the child's parent_checkpoint_history_ref.
func prepareTriggerManifestV2(ctx context.Context, s *Store, parent *genesis.Genesis, snapshot []any, origin *policy.Bundle) ([]byte, error) {
	registered, err := s.LoadRegisteredTriggers(ctx, [32]byte(parent.RunID), origin)
	if err != nil {
		return nil, err
	}
	definitions := make([]any, 0, len(registered))
	for _, item := range registered {
		definitions = append(definitions, item.DefinitionCanonical)
	}
	var seal []byte
	if err := s.db.QueryRowContext(ctx, "SELECT registry_hash FROM trigger_registry_seals WHERE run_id=?", parent.RunID[:]).Scan(&seal); err != nil {
		return nil, err
	}
	parts, err := forkPendingParts(snapshot)
	if err != nil {
		return nil, err
	}
	activations := make([]any, 0)
	for _, raw := range parts[2].([]any) {
		ref := raw.([]any)
		var canonical []byte
		if err := s.db.QueryRowContext(ctx, "SELECT canonical FROM trigger_activations WHERE run_id=? AND id=?", parent.RunID[:], ref[1]).Scan(&canonical); err != nil {
			return nil, err
		}
		activations = append(activations, canonical)
	}
	return codec.CanonicalPayload([]any{uint64(2), seal, definitions, activations})
}

func migrateTriggerSeedsV2(parent, child *genesis.Genesis, snapshot []any, manifest []byte, archive forkHistoryArchive, origin, target *policy.Bundle) (forkTriggerMigration, error) {
	var result forkTriggerMigration
	v, err := codec.StrictDecodePayload(manifest)
	if err != nil {
		return result, err
	}
	f, ok := v.([]any)
	if !ok || len(f) != 4 || f[0] != uint64(2) {
		return result, errors.New("V2 trigger manifest malformed")
	}
	parts, err := forkPendingParts(snapshot)
	if err != nil {
		return result, err
	}
	states := map[domain.ID]trigger.Runtime{}
	for _, raw := range parts[1].([]any) {
		encoded, err := codec.CanonicalPayload(raw)
		if err != nil {
			return result, err
		}
		state, err := trigger.DecodeRuntime(encoded)
		if err != nil {
			return result, err
		}
		if _, duplicate := states[state.DefinitionID]; duplicate {
			return result, errors.New("duplicate V2 trigger seed")
		}
		if uint64(state.EvaluatedThrough) > snapshot[3].(uint64) {
			return result, errors.New("trigger seed exceeds parent cut")
		}
		states[state.DefinitionID] = state
	}
	refs, err := origin.GenesisPolicyRefs()
	if err != nil {
		return result, err
	}
	definitions, ok := f[2].([]any)
	if !ok || len(definitions) != len(states) {
		return result, errors.New("V2 trigger definition roster differs from snapshot")
	}
	digests := make([]any, 0, len(definitions))
	var previous []byte
	for _, raw := range definitions {
		canonical, ok := raw.([]byte)
		if !ok {
			return result, errors.New("V2 trigger definition bytes absent")
		}
		def, _, err := trigger.DecodeDefinition(canonical, refs, origin)
		if err != nil {
			return result, err
		}
		if previous != nil && bytes.Compare(previous, def.ID[:]) >= 0 {
			return result, errors.New("V2 trigger definitions not strictly ordered")
		}
		previous = bytes.Clone(def.ID[:])
		state, exists := states[def.ID]
		if !exists || state.Version != def.Version {
			return result, errors.New("V2 trigger definition has no matching seed")
		}
		if state.HasNextRepeat && (def.Policy != trigger.RepeatWhileTrue || state.NextRepeat <= child.InitialInstant) {
			return result, errors.New("V2 trigger cadence incompatible with source cut")
		}
		if state.Exhausted && def.Policy != trigger.OnceWhenTrue {
			return result, errors.New("V2 trigger exhaustion incompatible with definition")
		}
		fields, err := codec.StrictDecodePayload(canonical)
		if err != nil {
			return result, err
		}
		digest, _, err := identity.New(origin).Sum("cote.csf.digest.trigger-definition", "cote.csf.schema.trigger-definition", 1, fields)
		if err != nil {
			return result, err
		}
		digests = append(digests, []any{def.ID[:], digest[:]})
		state.EvaluatedThrough = 0
		seed, err := trigger.RuntimeBytes(state)
		if err != nil {
			return result, err
		}
		result.Definitions = append(result.Definitions, forkTriggerDefinition{ID: [32]byte(def.ID), Canonical: canonical, ContentDigest: bytes.Clone(digest[:]), Runtime: seed})
	}
	sourceSeal, err := codec.CanonicalPayload([]any{parent.ID[:], parent.Digest[:], digests})
	if err != nil {
		return result, err
	}
	sourceHash := identity.OpaqueContentHash(sourceSeal)
	if !equalBytes(f[1], sourceHash[:]) {
		return result, errors.New("V2 source trigger registry seal differs")
	}
	childSeal, err := codec.CanonicalPayload([]any{child.ID[:], child.Digest[:], digests})
	if err != nil {
		return result, err
	}
	result.Seal = [32]byte(identity.OpaqueContentHash(childSeal))
	proofs, ok := f[3].([]any)
	if !ok || len(proofs) != len(parts[2].([]any)) {
		return result, errors.New("V2 activation proof roster differs from snapshot")
	}
	activationArchive := make(forkHistoryArchive, len(archive)+1)
	for key, records := range archive {
		activationArchive[key] = records
	}
	for _, raw := range proofs {
		canonical, ok := raw.([]byte)
		if !ok {
			return result, errors.New("V2 activation proof bytes absent")
		}
		v, err := codec.StrictDecodePayload(canonical)
		if err != nil {
			return result, err
		}
		fields, ok := v.([]any)
		if !ok || len(fields) != 12 {
			return result, errors.New("V2 activation proof malformed")
		}
		id, ok := fields[0].([]byte)
		if !ok || len(id) != 32 {
			return result, errors.New("V2 activation proof identity malformed")
		}
		activationArchive[nativeArchiveKey(7)] = append(activationArchive[nativeArchiveKey(7)], ports.CanonicalRecord{Identity: bytesToID(id), Canonical: canonical})
	}
	seen := map[[32]byte]bool{}
	for i, raw := range parts[2].([]any) {
		ref := raw.([]any)
		// Proof order is exactly the signed source reference order, never SQL arrival.
		v, err := codec.StrictDecodePayload(proofs[i].([]byte))
		if err != nil {
			return result, err
		}
		fields := v.([]any)
		if !sameCanonical(fields[0], ref[1]) {
			return result, errors.New("V2 activation proof order differs")
		}
		if err := authenticateArchivedCausalRef(ref, parent, snapshot, activationArchive, origin); err != nil {
			return result, err
		}
		definition, ok := fields[3].([]byte)
		if !ok || len(definition) != 32 {
			return result, errors.New("V2 activation definition malformed")
		}
		state, exists := states[domain.ID(bytesToID(definition))]
		count, valid := fields[7].(uint64)
		if !exists || !valid || count == 0 || count > state.ActivationCount || fields[4] != uint64(state.Version) {
			return result, errors.New("V2 activation count/version exceeds source seed")
		}
		coord, ok := fields[6].([]any)
		if !ok || len(coord) != 2 {
			return result, errors.New("V2 activation eligibility malformed")
		}
		when, valid := asSigned64(coord[0])
		ordinal, ordinalOK := coord[1].(uint64)
		if !valid || !ordinalOK || when < int64(child.InitialInstant) {
			return result, errors.New("V2 activation precedes fork clock")
		}
		if when == int64(child.InitialInstant) {
			last, valid := asSigned64(snapshot[5])
			if !valid || last < -1 || last == int64(^uint64(0)>>1) {
				return result, errors.New("V2 source ordinal invalid")
			}
			floor := uint64(last + 1)
			if ordinal < floor {
				return result, errors.New("V2 activation precedes source ordinal")
			}
			ordinal -= floor
		}
		id, _, err := identity.New(target).Sum("cote.csf.id.trigger-activation", "cote.csf.schema.trigger-activation-id-preimage", 1, []any{child.RunID[:], definition, fields[4], uint64(0), count})
		if err != nil {
			return result, err
		}
		body := append([]any(nil), fields[:11]...)
		body[0], body[1], body[5], body[6] = id[:], child.RunID[:], []any{uint64(1), []any{child.ID[:], child.Digest[:]}}, []any{when, ordinal}
		digest, _, err := identity.New(target).Sum("cote.csf.digest.trigger-activation-unit", "cote.csf.schema.trigger-activation.body", 1, body)
		if err != nil {
			return result, err
		}
		canonical, err := codec.CanonicalPayload(append(body, digest[:]))
		if err != nil {
			return result, err
		}
		key := [32]byte(id)
		if seen[key] {
			return result, errors.New("V2 activation births collide")
		}
		seen[key] = true
		if _, _, err := validateTriggerActivation([32]byte(child.RunID), key, canonical); err != nil {
			return result, err
		}
		result.Activations = append(result.Activations, forkTriggerActivation{ID: key, Instant: when, Ordinal: ordinal, Canonical: canonical})
	}
	return result, nil
}

func readTriggerMigrationV2(ctx context.Context, tx *sql.Tx, parent, child *genesis.Genesis, snapshot []any, origin, target *policy.Bundle) (forkTriggerMigration, bool, error) {
	var empty forkTriggerMigration
	var raw, hash []byte
	err := tx.QueryRowContext(ctx, "SELECT canonical,canonical_hash FROM fork_trigger_imports WHERE child_run=?", child.RunID[:]).Scan(&raw, &hash)
	if err == sql.ErrNoRows {
		parts, e := forkPendingParts(snapshot)
		if e != nil {
			return empty, false, e
		}
		// Legacy V2 forks only supported fresh Genesis trigger state. A
		// deleted import manifest cannot opt an evolved checkpoint into it.
		for _, value := range parts[1].([]any) {
			fields, ok := value.([]any)
			if !ok || len(fields) != 8 || fields[7] != uint64(0) || !sameCanonical(snapshot[2], int64(parent.InitialInstant)) {
				return empty, false, errors.New("V2 evolved trigger seed requires its import manifest")
			}
		}
		return empty, false, nil
	}
	if err != nil {
		return empty, false, err
	}
	want := identity.OpaqueContentHash(raw)
	if !bytes.Equal(hash, want[:]) {
		return empty, false, errors.New("V2 trigger import manifest integrity differs")
	}
	archive, err := readForkHistoryArchive(ctx, tx, [32]byte(child.RunID))
	if err != nil {
		return empty, false, err
	}
	result, err := migrateTriggerSeedsV2(parent, child, snapshot, raw, archive, origin, target)
	return result, true, err
}

func verifyTriggerMigrationV2(ctx context.Context, tx *sql.Tx, parent, child *genesis.Genesis, snapshot []any, origin, target *policy.Bundle) error {
	result, found, err := readTriggerMigrationV2(ctx, tx, parent, child, snapshot, origin, target)
	if err != nil || !found {
		return err
	}
	var count int
	if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM trigger_definitions WHERE run_id=?", child.RunID[:]).Scan(&count); err != nil {
		return err
	}
	if count != len(result.Definitions) {
		return errors.New("V2 child trigger definition roster differs")
	}
	for _, item := range result.Definitions {
		var raw, digest []byte
		if err := tx.QueryRowContext(ctx, "SELECT canonical,content_digest FROM trigger_definitions WHERE run_id=? AND definition_id=?", child.RunID[:], item.ID[:]).Scan(&raw, &digest); err != nil {
			return err
		}
		if !bytes.Equal(raw, item.Canonical) || !bytes.Equal(digest, item.ContentDigest) {
			return errors.New("V2 child trigger definition differs from source proof")
		}
	}
	wanted := map[[32]byte][]byte{}
	for _, a := range result.Activations {
		wanted[a.ID] = a.Canonical
	}
	rows, err := tx.QueryContext(ctx, "SELECT id,canonical FROM trigger_activations WHERE run_id=?", child.RunID[:])
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, raw []byte
		if err := rows.Scan(&id, &raw); err != nil {
			return err
		}
		v, err := codec.StrictDecodePayload(raw)
		if err != nil {
			return err
		}
		f, ok := v.([]any)
		if !ok || len(f) != 12 {
			return errors.New("V2 child activation malformed")
		}
		cause, ok := f[5].([]any)
		if !ok || len(cause) == 0 {
			return errors.New("V2 child activation origin malformed")
		}
		if cause[0] != uint64(1) {
			continue
		}
		if len(id) != 32 || !bytes.Equal(wanted[bytesToID(id)], raw) {
			return errors.New("V2 child activation differs from migrated birth")
		}
		delete(wanted, bytesToID(id))
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(wanted) != 0 {
		return fmt.Errorf("V2 child missing %d migrated activations", len(wanted))
	}
	return nil
}

func (s *Store) triggerReplaySeedV2(ctx context.Context, child *genesis.Genesis, target *policy.Bundle) (forkTriggerMigration, bool, error) {
	var empty forkTriggerMigration
	if child.Payload[5] != policy.CodecPolicyV2ID || len(child.Payload[4].([]any)) == 1 {
		return empty, false, nil
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return empty, false, err
	}
	defer tx.Rollback()
	var parentRaw, snapshotRaw []byte
	if err := tx.QueryRowContext(ctx, "SELECT parent_genesis_canonical,parent_snapshot_canonical FROM fork_lineage WHERE child_run=?", child.RunID[:]).Scan(&parentRaw, &snapshotRaw); err != nil {
		return empty, false, err
	}
	ref, err := fork.Decode(child.Payload[4].([]any)[1].([]byte))
	if err != nil {
		return empty, false, err
	}
	origin, err := target.ResolveHistoricalOrigin(ref.OriginPolicyID, ref.OriginPolicyHash)
	if err != nil {
		return empty, false, err
	}
	parent, err := genesis.ValidateCanonical(parentRaw, origin)
	if err != nil {
		return empty, false, err
	}
	v, err := codec.StrictDecodePayload(snapshotRaw)
	if err != nil {
		return empty, false, err
	}
	snapshot, ok := v.([]any)
	if !ok || len(snapshot) != 19 {
		return empty, false, errors.New("V2 source snapshot malformed")
	}
	return readTriggerMigrationV2(ctx, tx, parent, child, snapshot, origin, target)
}

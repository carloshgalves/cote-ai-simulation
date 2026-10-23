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
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
)

type forkHistoricalRoot struct {
	Kind uint64
	ports.CanonicalRecord
}

func nativeArchiveKey(kind uint64) string { return fmt.Sprintf("native:%d", kind) }
func nativeRefKey(ref []any) string       { return fmt.Sprintf("%d:%x", ref[0], ref[1]) }

func forkPendingParts(snapshot []any) ([]any, error) {
	if len(snapshot) != 19 {
		return nil, errors.New("historical snapshot malformed")
	}
	typed, ok := snapshot[11].([]any)
	if !ok || len(typed) != 4 {
		return nil, errors.New("historical pending state malformed")
	}
	raw, ok := typed[2].([]byte)
	if !ok {
		return nil, errors.New("historical pending envelope absent")
	}
	env, err := codec.StrictDecodeEnvelope(raw)
	if err != nil {
		return nil, err
	}
	parts, ok := env.Value.([]any)
	if !ok || len(parts) != 10 {
		return nil, errors.New("historical pending partitions malformed")
	}
	return parts, nil
}

// Only intentions on responses belonging to migrated pending rounds require
// additional roots. Settled unrelated proposals stay in the frozen archive.
func nativeRoundIntentionRefs(snapshot []any, archive forkHistoryArchive) (map[string][]any, error) {
	parts, err := forkPendingParts(snapshot)
	if err != nil {
		return nil, err
	}
	pending := map[string]bool{}
	for _, raw := range parts[4].([]any) {
		ref := raw.([]any)
		pending[string(ref[1].([]byte))] = true
	}
	refs := map[string][]any{}
	for _, record := range archive["input_ledger"] {
		value, err := codec.StrictDecodePayload(record.Canonical)
		if err != nil {
			continue
		}
		f, ok := value.([]any)
		if !ok || len(f) != 19 {
			continue
		}
		round, ok := f[13].([]byte)
		if !ok || !pending[string(round)] {
			continue
		}
		option, ok := f[16].([]any)
		if !ok || len(option) == 0 {
			return nil, errors.New("historical intention option malformed")
		}
		if len(option) == 1 && option[0] == uint64(0) {
			continue
		}
		if len(option) != 2 || option[0] != uint64(1) {
			return nil, errors.New("historical intention option malformed")
		}
		ref, ok := option[1].([]any)
		if !ok || len(ref) != 3 {
			return nil, errors.New("historical intention ref malformed")
		}
		kind, ok := ref[0].(uint64)
		if !ok {
			return nil, errors.New("historical intention kind malformed")
		}
		if kind != 6 && kind != 7 && kind != 18 && kind != 19 && kind != 20 && kind != 22 {
			continue
		}
		id, ok := ref[1].([]byte)
		digest, digestOK := ref[2].([]byte)
		if !ok || len(id) != 32 || !digestOK || len(digest) != 32 {
			return nil, errors.New("native intention ID/digest malformed")
		}
		key := nativeRefKey(ref)
		if old, exists := refs[key]; exists && !sameCanonical(old, ref) {
			return nil, errors.New("native root identity has two content digests")
		}
		refs[key] = ref
	}
	// The sent Event authenticates the Claim/Transmission pair, including a
	// Transmission's referenced Claim. Preserve that closed historical pair.
	var communicationRefs [][]any
	for _, ref := range refs {
		if ref[0] == uint64(18) || ref[0] == uint64(19) {
			communicationRefs = append(communicationRefs, ref)
		}
	}
	for _, ref := range communicationRefs {
		pair, err := communicationIntentionPair(ref, archive)
		if err != nil {
			return nil, err
		}
		for _, member := range pair {
			key := nativeRefKey(member)
			if old, ok := refs[key]; ok && !sameCanonical(old, member) {
				return nil, errors.New("communication historical root has conflicting digests")
			}
			refs[key] = member
		}
	}
	return refs, nil
}

func prepareNativeRoundIntentions(ctx context.Context, s *Store, parent *genesis.Genesis, snapshot []any, archive forkHistoryArchive, b *policy.Bundle) ([]forkHistoricalRoot, error) {
	refs, err := nativeRoundIntentionRefs(snapshot, archive)
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(refs))
	for key := range refs {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	roots := make([]forkHistoricalRoot, 0, len(refs))
	for _, key := range keys {
		ref := refs[key]
		kind := ref[0].(uint64)
		table := "scheduled_occurrences"
		switch kind {
		case 7:
			table = "trigger_activations"
		case 18:
			table = "communication_claims"
		case 19:
			table = "communication_transmissions"
		}
		var raw []byte
		if kind == 20 || kind == 22 {
			raw, err = prepareHistoricalCheckpointIntention(ctx, s, parent, snapshot, ref, b)
			if err != nil {
				return nil, err
			}
		} else {
			if err := s.db.QueryRowContext(ctx, "SELECT canonical FROM "+table+" WHERE run_id=? AND id=?", parent.RunID[:], ref[1]).Scan(&raw); err != nil {
				return nil, fmt.Errorf("native intention root missing: %w", err)
			}
		}
		root := forkHistoricalRoot{Kind: kind, CanonicalRecord: ports.CanonicalRecord{Identity: bytesToID(ref[1].([]byte)), Canonical: raw}}
		archive[nativeArchiveKey(kind)] = append(archive[nativeArchiveKey(kind)], root.CanonicalRecord)
		if err := authenticateArchivedCausalRef(ref, parent, snapshot, archive, b); err != nil {
			return nil, err
		}
		roots = append(roots, root)
	}
	return roots, nil
}

func loadNativeRoundIntentions(ctx context.Context, tx *sql.Tx, parent, child *genesis.Genesis, snapshot []any, archive forkHistoryArchive, origin *policy.Bundle) error {
	expected, err := nativeRoundIntentionRefs(snapshot, archive)
	if err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, "SELECT reference_kind,record_id,canonical FROM fork_history_roots WHERE child_run=? ORDER BY reference_kind,record_id", child.RunID[:])
	if err != nil {
		return err
	}
	defer rows.Close()
	var loadedRefs [][]any
	for rows.Next() {
		var kind uint64
		var id, raw []byte
		if err := rows.Scan(&kind, &id, &raw); err != nil {
			return err
		}
		if len(id) != 32 {
			return errors.New("native archived root ID malformed")
		}
		ref := []any{kind, id, []byte{}}
		key := nativeRefKey(ref)
		expectedRef, ok := expected[key]
		if !ok {
			return errors.New("native archived root not owned by a migrated intention")
		}
		loadedRefs = append(loadedRefs, expectedRef)
		delete(expected, key)
		archive[nativeArchiveKey(kind)] = append(archive[nativeArchiveKey(kind)], ports.CanonicalRecord{Identity: bytesToID(id), Canonical: raw})
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(expected) != 0 {
		return errors.New("native historical intention root missing")
	}
	if err := rows.Close(); err != nil {
		return err
	}
	// Authenticate every member after loading the complete dependency graph.
	// A companion root need not itself be the response's intention pointer.
	for _, ref := range loadedRefs {
		if err := authenticateArchivedCausalRef(ref, parent, snapshot, archive, origin); err != nil {
			return err
		}
	}
	return nil
}

// A native record outside the ledgers must prove that it already existed at
// the cut: the pending projection or an archived fence binds its exact digest.
// This prevents a later source tail from satisfying an earlier dangling pointer.
func authenticateNativeRootAtCut(kind uint64, ref, fields []any, parent *genesis.Genesis, snapshot []any, archive forkHistoryArchive, b *policy.Bundle) error {
	if kind == 20 || kind == 22 {
		return authenticateHistoricalCheckpointIntention(kind, fields, parent, snapshot, archive, b)
	}
	if kind == 18 || kind == 19 {
		return authenticateCommunicationIntentionAtCut(kind, ref, fields, parent, snapshot, archive, b)
	}
	if len(fields) < 2 || !equalBytes(fields[1], parent.RunID[:]) {
		return errors.New("native intention root belongs to another run")
	}
	parts, err := forkPendingParts(snapshot)
	if err != nil {
		return err
	}
	partition, unitKind := 0, uint64(2)
	if kind == 7 {
		partition, unitKind = 2, 3
	}
	found := false
	for _, raw := range parts[partition].([]any) {
		if sameCanonical(raw, ref) {
			found = true
			break
		}
	}
	for _, record := range archive["decision_ledger"] {
		value, err := codec.StrictDecodePayload(record.Canonical)
		if err != nil {
			return err
		}
		f, ok := value.([]any)
		if !ok || len(f) != 18 || !equalBytes(f[0], parent.RunID[:]) {
			continue
		}
		units, ok := f[11].([]any)
		if !ok {
			continue
		}
		for _, raw := range units {
			u, ok := raw.([]any)
			if ok && len(u) == 5 && u[1] == unitKind && sameCanonical(u[3], ref[1]) && sameCanonical(u[4], ref[2]) {
				found = true
			}
		}
	}
	if !found {
		return errors.New("native intention root not covered by source causal cut")
	}
	if kind == 6 {
		if len(fields) != 15 {
			return errors.New("native occurrence root malformed")
		}
		origin, ok := fields[9].([]any)
		if !ok || len(origin) != 2 {
			return errors.New("native occurrence origin malformed")
		}
		root, ok := origin[1].([]any)
		if !ok {
			return errors.New("native occurrence cause malformed")
		}
		switch origin[0] {
		case uint64(2):
			if !sameCanonical(root, []any{parent.ID[:], parent.Digest[:]}) {
				return errors.New("native occurrence Genesis differs")
			}
		case uint64(0):
			if len(root) != 2 {
				return errors.New("native occurrence Event ref malformed")
			}
			events := map[string]ports.CanonicalRecord{}
			for _, record := range archive["event_store"] {
				events[string(record.Identity[:])] = record
			}
			if err := authenticateRoundEventOrigin(parent, root, events, b); err != nil {
				return err
			}
		case uint64(1):
			if len(root) != 4 || root[0] != uint64(0) || root[1] != uint64(0) {
				return errors.New("native input occurrence cause unsupported")
			}
			if err := authenticateArchivedCausalRef([]any{uint64(5), root[2], root[3]}, parent, snapshot, archive, b); err != nil {
				return err
			}
		default:
			return errors.New("native occurrence cause unregistered")
		}
		return nil
	}
	if kind != 7 || len(fields) != 12 {
		return errors.New("native activation root malformed")
	}
	origin, ok := fields[5].([]any)
	if !ok || len(origin) < 2 {
		return errors.New("native activation cause malformed")
	}
	if len(origin) == 2 && origin[0] == uint64(1) {
		if !sameCanonical(origin[1], []any{parent.ID[:], parent.Digest[:]}) {
			return errors.New("native activation Genesis differs")
		}
		return nil
	}
	if len(origin) != 4 || origin[0] != uint64(0) {
		return errors.New("native activation origin unregistered")
	}
	for _, record := range archive["decision_ledger"] {
		value, err := codec.StrictDecodePayload(record.Canonical)
		if err != nil {
			return err
		}
		f, ok := value.([]any)
		if ok && len(f) == 24 && bytes.Equal(parent.RunID[:], communicationBytes(f[0])) && sameCanonical(f[1], origin[1]) && sameCanonical(f[8], origin[2]) {
			return nil
		}
	}
	return errors.New("native activation origin CycleCommit missing")
}

package sqlite

import (
	"context"
	"errors"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/genesis"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

// Historical checkpoint roots are opaque provenance, never a request to restore
// the child's world or memory from an earlier cut. Resolve only real, sealed
// source snapshots; a caller cannot supply replacement bytes for a dangling ref.
func prepareHistoricalCheckpointIntention(ctx context.Context, s *Store, parent *genesis.Genesis, snapshot, ref []any, b *policy.Bundle) ([]byte, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT snapshot_id FROM snapshot_seals WHERE run_id=? AND revision<=? ORDER BY revision", parent.RunID[:], snapshot[3])
	if err != nil {
		return nil, err
	}
	var ids [][32]byte
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			rows.Close()
			return nil, err
		}
		if len(raw) != 32 {
			rows.Close()
			return nil, errors.New("historical checkpoint ID malformed")
		}
		ids = append(ids, bytesToID(raw))
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for _, id := range ids {
		if ref[0] == uint64(20) && !equalBytes(ref[1], id[:]) {
			continue
		}
		verified, err := s.VerifySnapshotV1(ctx, [32]byte(parent.RunID), id, b)
		if err != nil {
			return nil, err
		}
		if ref[0] == uint64(20) {
			if !equalBytes(ref[2], verified.Snapshot.Digest[:]) {
				return nil, errors.New("historical snapshot intention digest differs")
			}
			return verified.Snapshot.Canonical, nil
		}
		value, err := codec.StrictDecodePayload(verified.Snapshot.Canonical)
		if err != nil {
			return nil, err
		}
		for _, raw := range value.([]any)[14].([]any) {
			fields := raw.([]any)
			preimage, err := identity.RecordIdentityFields("epistemic-checkpoint-ref", fields, nil)
			if err != nil {
				return nil, err
			}
			rootID, _, err := identity.New(b).Sum("cote.csf.id.epistemic-checkpoint", "cote.csf.schema.epistemic-checkpoint-id-preimage", 1, preimage)
			if err != nil {
				return nil, err
			}
			digest, _, err := identity.New(b).Sum("cote.csf.digest.epistemic-checkpoint", "cote.csf.schema.epistemic-checkpoint-ref", 1, fields)
			if err != nil {
				return nil, err
			}
			if equalBytes(ref[1], rootID[:]) && equalBytes(ref[2], digest[:]) {
				return codec.CanonicalPayload(fields)
			}
		}
	}
	return nil, errors.New("historical checkpoint intention is absent from verified source snapshots")
}

func authenticateHistoricalCheckpointIntention(kind uint64, fields []any, parent *genesis.Genesis, snapshot []any, archive forkHistoryArchive, b *policy.Bundle) error {
	cut, ok := snapshot[3].(uint64)
	if !ok {
		return errors.New("historical checkpoint source cut malformed")
	}
	if kind == 20 {
		if len(fields) != 19 || !equalBytes(fields[0], parent.RunID[:]) || fields[1] != uint64(1) ||
			!sameCanonical(fields[9], []any{parent.ID[:], parent.Digest[:]}) || !sameCanonical(fields[8], parent.WorldSeed[:]) ||
			!sameCanonical(fields[10], parent.Payload[14]) || !sameCanonical(fields[15], snapshot[15]) ||
			!equalBytes(fields[16], b.CodecPolicyHash[:]) || !equalBytes(fields[17], b.SchemaHash[:]) {
			return errors.New("historical snapshot intention belongs to another origin")
		}
		rev, ok := fields[3].(uint64)
		if !ok || rev > cut {
			return errors.New("historical snapshot intention is after causal cut")
		}
		clocks, err := roundHistoryClocksV2(parent, snapshot, archive["decision_ledger"], b)
		if err != nil {
			return err
		}
		clock, exists := clocks[rev]
		if !exists || !sameCanonical(fields[2], clock.Instant) || !sameCanonical(fields[4], clock.NextSequence) || !sameCanonical(fields[5], clock.LastOrdinal) {
			return errors.New("historical snapshot intention clock differs from frozen journal")
		}
		return nil
	}
	if kind != 22 || len(fields) != 8 {
		return errors.New("historical epistemic root malformed")
	}
	coverage, ok := fields[7].([]any)
	if !ok || len(coverage) != 3 {
		return errors.New("historical epistemic coverage malformed")
	}
	revision, ok := coverage[1].(uint64)
	at, valid := asSigned64(coverage[0])
	instant, validCut := asSigned64(snapshot[2])
	cursor, cursorOK := coverage[2].(uint64)
	if !ok || !valid || !validCut || !cursorOK || revision > cut || at > instant || cursor > snapshot[12].([]any)[3].([]any)[1].(uint64) {
		return errors.New("historical epistemic root exceeds frozen causal cut")
	}
	owner := false
	for _, raw := range snapshot[14].([]any) {
		if sameCanonical(raw.([]any)[0], fields[0]) {
			owner = true
		}
	}
	if !owner {
		return errors.New("historical epistemic owner is absent from source roster")
	}
	content, ok := fields[3].([]any)
	if !ok || len(content) != 2 || content[0] != uint64(1) {
		return errors.New("historical external epistemic root requires owner resolver")
	}
	raw, ok := content[1].([]byte)
	if !ok {
		return errors.New("historical epistemic content missing")
	}
	hash := identity.OpaqueContentHash(raw)
	if !equalBytes(fields[4], hash[:]) {
		return errors.New("historical epistemic content hash differs")
	}
	return nil
}

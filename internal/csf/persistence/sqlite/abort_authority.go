package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
)

// Query-only access, usable through the active writer transaction and the
// cold replay store. No policy lookup supplied by an abort caller is trusted.
type abortAuthorityReader interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func verifyAbortAuthorities(ctx context.Context, q abortAuthorityReader, run [32]byte, fence, abort []any, candidates []ports.CanonicalRecord) error {
	if len(abort) != 15 || len(fence) != 18 {
		return errors.New("invalid abort authority envelope")
	}
	items, ok := abort[11].([]any)
	if !ok {
		return errors.New("abort evidence missing")
	}
	reason, ok := abort[10].(uint64)
	refs, valid := abort[12].([]any)
	if !ok || reason > 6 || !valid || len(refs) != 0 || (reason == 0 && len(items) == 0) || (reason == 6 && len(items) != 0) {
		return errors.New("unsupported abort reason or evidence")
	}
	// Authenticate the root from which the policy references were frozen.
	var raw, id, digest []byte
	if err := q.QueryRowContext(ctx, "SELECT canonical,genesis_id,digest FROM genesis_records WHERE run_id=?", run[:]).Scan(&raw, &id, &digest); err != nil {
		return err
	}
	env, err := codec.StrictDecodeEnvelope(raw)
	if err != nil {
		return err
	}
	payload, ok := env.Value.([]any)
	if !ok || len(payload) != 18 || env.SchemaID != "cote.csf.schema.genesis-manifest" || env.SchemaVersion != 1 || !equalBytes(payload[0], run[:]) {
		return errors.New("invalid abort Genesis root")
	}
	wantID, err := registeredDigest("cote.csf.id.genesis", "cote.csf.schema.genesis-id-preimage", []any{run[:]})
	if err != nil || !equalBytes(id, wantID[:]) {
		return errors.New("abort Genesis identity mismatch")
	}
	wantDigest, err := registeredDigest("cote.csf.digest.genesis-manifest", "cote.csf.schema.genesis-manifest", payload)
	if err != nil || !equalBytes(digest, wantDigest[:]) {
		return errors.New("abort Genesis digest mismatch")
	}
	pins, ok := payload[14].([]any)
	if !ok {
		return errors.New("abort Genesis policy set missing")
	}
	allowed := map[string][]any{}
	for _, raw := range pins {
		ref, ok := raw.([]any)
		if !ok || len(ref) != 3 {
			return errors.New("invalid Genesis policy ref")
		}
		name, ok := ref[0].(string)
		version, vok := ref[1].(uint64)
		hash, hok := ref[2].([]byte)
		if !ok || name == "" || !vok || version == 0 || !hok || len(hash) != 32 || allowed[name] != nil {
			return errors.New("invalid or duplicate Genesis policy ref")
		}
		allowed[name] = ref
	}
	for _, raw := range items {
		record, ok := raw.([]any)
		if !ok || len(record) != 8 {
			return errors.New("invalid indeterminate record")
		}
		subject, ok := record[0].([]any)
		if !ok || len(subject) != 3 {
			return errors.New("invalid indeterminate subject")
		}
		kind, ok := subject[0].(uint64)
		facet, fok := record[1].(uint64)
		msg, mok := record[5].(string)
		evidence, eok := record[6].([]any)
		if !ok || kind > 1 || !fok || facet > 255 || !mok || msg == "" || !eok || len(evidence) != 0 {
			return errors.New("unsupported indeterminate subject/facet/evidence")
		}
		ref := []any{record[2], record[3], record[4]}
		if !sameCanonical(record[7], ref) {
			return errors.New("inconsistent indeterminate validator reference")
		}
		// Unit validation and candidate conflict resolution are the only
		// registered V1 indeterminate producers. Do not authorize a
		// perception/clock/other policy merely because it exists in Genesis.
		pin := allowed["validators-v1"]
		if kind == 1 {
			pin = allowed["conflict-resolvers-v1"]
		}
		if pin == nil || !sameCanonical(pin, ref) {
			return errors.New("indeterminate validator not pinned for subject kind by Genesis")
		}
	}
	cycle, ok := historicalID(abort[1])
	if !ok {
		return errors.New("abort cycle identity invalid")
	}
	return verifyAbortSubjects(fence, items, candidates, run, cycle)
}

func loadAbortCandidates(ctx context.Context, q abortAuthorityReader, abort []any) ([]ports.CanonicalRecord, error) {
	items, ok := abort[11].([]any)
	if !ok {
		return nil, errors.New("invalid abort evidence list")
	}
	// An abort's candidate evidence must already exist and precede the
	// envelope in the immutable journal. Validate exact bytes below.
	run, rok := historicalID(abort[0])
	cycle, cok := historicalID(abort[1])
	attempt, aok := abort[2].(uint64)
	if !rok || !cok || !aok {
		return nil, errors.New("invalid abort origin")
	}
	abortID, err := registeredDigest("cote.csf.id.cycle-abort", "cote.csf.schema.cycle-abort-id-preimage", []any{run[:], cycle[:], attempt})
	if err != nil {
		return nil, err
	}
	var abortRow int64
	if err := q.QueryRowContext(ctx, "SELECT rowid FROM decision_ledger WHERE id=?", abortID[:]).Scan(&abortRow); err != nil {
		return nil, err
	}
	out := []ports.CanonicalRecord{}
	for _, raw := range items {
		record, ok := raw.([]any)
		if !ok || len(record) != 8 {
			return nil, errors.New("malformed abort evidence")
		}
		subject, ok := record[0].([]any)
		if !ok || len(subject) != 3 {
			return nil, errors.New("malformed abort subject")
		}
		if subject[0] != uint64(1) {
			continue
		}
		id, ok := historicalID(subject[1])
		if !ok {
			return nil, errors.New("invalid candidate identity")
		}
		var row int64
		var canonical []byte
		if err := q.QueryRowContext(ctx, "SELECT rowid,canonical FROM decision_ledger WHERE id=?", id[:]).Scan(&row, &canonical); err != nil {
			return nil, err
		}
		if row >= abortRow {
			return nil, errors.New("candidate evidence postdates abort")
		}
		out = append(out, ports.CanonicalRecord{Identity: id, Canonical: canonical})
	}
	return out, nil
}

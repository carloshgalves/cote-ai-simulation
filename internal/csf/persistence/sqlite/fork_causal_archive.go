package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/genesis"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
)

type forkHistoryArchive map[string][]ports.CanonicalRecord

func readForkHistoryArchive(ctx context.Context, tx *sql.Tx, run [32]byte) (forkHistoryArchive, error) {
	archive := forkHistoryArchive{}
	for _, ledger := range snapshotLedgerNames {
		rows, err := tx.QueryContext(ctx, "SELECT record_id,canonical FROM fork_history_records WHERE child_run=? AND ledger_name=? ORDER BY ordinal", run[:], ledger)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id, raw []byte
			if err := rows.Scan(&id, &raw); err != nil {
				rows.Close()
				return nil, err
			}
			if len(id) != 32 {
				rows.Close()
				return nil, errors.New("historical root ID malformed")
			}
			archive[ledger] = append(archive[ledger], ports.CanonicalRecord{Identity: bytesToID(id), Canonical: raw})
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		if err := rows.Close(); err != nil {
			return nil, err
		}
	}
	return archive, nil
}

// An actor's opaque intention can retain its historical reference. Resolution
// authenticates the root's complete ID preimage and digest under the ORIGIN
// policy; it grants no child-world, evidence or context authority.
func authenticateArchivedCausalRef(ref []any, parent *genesis.Genesis, snapshot []any, archive forkHistoryArchive, bundle *policy.Bundle) error {
	if len(ref) != 3 {
		return errors.New("historical causal reference malformed")
	}
	kind, ok := ref[0].(uint64)
	id, idOK := ref[1].([]byte)
	digest, digestOK := ref[2].([]byte)
	if !ok || !idOK || len(id) != 32 || !digestOK || len(digest) != 32 {
		return errors.New("historical causal reference fields malformed")
	}
	var rule map[string]any
	for _, item := range bundle.Registries["reference_identities"].([]any) {
		entry := item.(map[string]any)
		if entry["code"] == float64(kind) {
			rule = entry
			break
		}
	}
	if rule == nil {
		return errors.New("historical causal reference kind unregistered")
	}
	root := strings.TrimPrefix(rule["persisted_root"].(string), "cote.csf.schema.")
	var candidates []ports.CanonicalRecord
	for ledger, records := range archive {
		if strings.HasPrefix(ledger, "native:") && ledger != nativeArchiveKey(kind) {
			continue
		}
		for _, record := range records {
			if bytes.Equal(record.Identity[:], id) {
				candidates = append(candidates, record)
			}
		}
	}
	var fields []any
	switch kind {
	case 21:
		if !bytes.Equal(id, parent.ID[:]) || !bytes.Equal(digest, parent.Digest[:]) {
			return errors.New("historical Genesis reference differs")
		}
		return nil // independently validated canonical Genesis in lineage
	case 20:
		fields = snapshot
	case 22:
		idRule := rule["id"].(map[string]any)
		for _, raw := range snapshot[14].([]any) {
			f := raw.([]any)
			preimage, err := identity.RecordIdentityFields(root, f, nil)
			if err != nil {
				return err
			}
			derived, _, err := identity.New(bundle).Sum(idRule["operation"].(string), idRule["preimage_schema"].(string), 1, preimage)
			if err != nil {
				return err
			}
			if bytes.Equal(derived[:], id) {
				if fields != nil {
					return errors.New("ambiguous epistemic root")
				}
				fields = f
			}
		}
	default:
		var raw []byte
		for _, record := range candidates {
			if raw != nil && !bytes.Equal(raw, record.Canonical) {
				return errors.New("ambiguous historical causal root")
			}
			raw = record.Canonical
		}
		if raw != nil {
			v, err := codec.StrictDecodePayload(raw)
			if err != nil {
				return err
			}
			fields, ok = v.([]any)
			if !ok {
				return errors.New("historical causal root is not an array")
			}
		}
	}
	if fields == nil {
		return fmt.Errorf("historical causal root %d unavailable in authenticated cut", kind)
	}
	var witness []any
	if kind == 9 {
		if len(fields) != 7 {
			return errors.New("historical conflict set malformed")
		}
		// ConflictSet's cycle is external. A persisted terminal DecisionRecord
		// must bind the exact conflict ref; an arbitrary caller cannot choose it.
		for _, record := range archive["decision_ledger"] {
			v, err := codec.StrictDecodePayload(record.Canonical)
			if err != nil {
				return err
			}
			f, ok := v.([]any)
			if !ok || len(f) != 16 {
				continue
			}
			refs, ok := f[8].([]any)
			if !ok {
				continue
			}
			for _, candidate := range refs {
				if sameCanonical(candidate, ref) {
					if witness != nil && !sameCanonical(witness[0], f[2]) {
						return errors.New("ambiguous conflict cycle")
					}
					witness = []any{f[2], fields[1], fields[2], fields[3]}
				}
			}
		}
	}
	preimage, err := identity.RecordIdentityFields(root, fields, witness)
	if err != nil {
		return err
	}
	idRule := rule["id"].(map[string]any)
	want, _, err := identity.New(bundle).Sum(idRule["operation"].(string), idRule["preimage_schema"].(string), 1, preimage)
	if err != nil || !bytes.Equal(id, want[:]) {
		return errors.New("historical root ID differs from complete causal preimage")
	}
	digestRule := rule["digest"].(map[string]any)
	var body any = fields
	if digestRule["preimage_schema"] != rule["persisted_root"] {
		body = fields[:len(fields)-1]
	}
	if kind == 12 {
		if len(fields) != 24 {
			return errors.New("historical CycleCommit malformed")
		}
		ids, ok := fields[15].([]any)
		if !ok {
			return errors.New("historical journal events absent")
		}
		events := make([]any, 0, len(ids))
		for _, eid := range ids {
			found := false
			for _, record := range archive["event_store"] {
				if equalBytes(eid, record.Identity[:]) {
					events = append(events, record.Canonical)
					found = true
					break
				}
			}
			if !found {
				return errors.New("historical journal Event witness missing")
			}
		}
		body = []any{fields[:23], events}
	}
	wantDigest, _, err := identity.New(bundle).Sum(digestRule["operation"].(string), digestRule["preimage_schema"].(string), 1, body)
	if err != nil || !bytes.Equal(digest, wantDigest[:]) {
		return errors.New("historical causal root digest differs")
	}
	if digestRule["preimage_schema"] != rule["persisted_root"] && !equalBytes(fields[len(fields)-1], digest) {
		return errors.New("historical root embedded digest differs from authenticated reference")
	}
	if kind == 6 || kind == 7 {
		return authenticateNativeRootAtCut(kind, ref, fields, parent, snapshot, archive, bundle)
	}
	return nil
}

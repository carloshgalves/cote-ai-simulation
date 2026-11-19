package queryhost

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/observatory"
)

// Revisions are integer-keyed, unlike the 32-byte causal record IDs.
func (q *PrivilegedReader) o2RevisionAuthority(ctx context.Context, revision uint64,
	expectedHash [32]byte) (observatory.RevisionAuthority, error) {
	var zero observatory.RevisionAuthority
	var raw, hash []byte
	err := q.audit.QueryRowContext(ctx,
		"SELECT canonical,state_hash FROM world_revisions WHERE revision=?", revision).Scan(&raw, &hash)
	if err != nil {
		return zero, err
	}
	if !bytes.Equal(hash, expectedHash[:]) {
		return zero, errors.New("world revision hash differs from CycleCommit")
	}
	decoded, err := codec.StrictDecodePayload(raw)
	if err != nil {
		return zero, err
	}
	fields, ok := decoded.([]any)
	if !ok || len(fields) != 6 || fields[3] != revision || !equalBytes(fields[5], hash) {
		return zero, errors.New("world revision canonical record/index mismatch")
	}
	return observatory.RevisionAuthority{
		Ledger: "world_revisions", Revision: revision,
		ContentHash: [32]byte(identity.OpaqueContentHash(raw)),
	}, nil
}

// A task may still be pending. Completed tasks must resolve to their exact
// durable completion and, when addressed, to exactly the Observation and
// KnowledgeInput registered by the epistemic route index. The normative
// ReplayClock first verifies all evidence globally; these are projections
// of IDs and hashes only, never actor-facing knowledge or raw payloads.
func (q *PrivilegedReader) o2TaskEvidence(ctx context.Context, run [32]byte,
	tasks []observatory.AuthorityRef) ([]observatory.AuthorityRef, []observatory.AuthorityRef, error) {
	completions := make([]observatory.AuthorityRef, 0)
	evidence := make([]observatory.AuthorityRef, 0)
	for _, task := range tasks {
		completion, err := registeredDigest("cote.csf.id.perception-completion",
			"cote.csf.schema.perception-completion-id-preimage", []any{task.ID[:]})
		if err != nil {
			return nil, nil, err
		}
		var raw []byte
		err = q.audit.QueryRowContext(ctx,
			"SELECT canonical FROM causal_outbox WHERE id=?", completion[:]).Scan(&raw)
		hasCompletion := err == nil
		if err != nil && err != sql.ErrNoRows {
			return nil, nil, err
		}
		expected := map[[32]byte]int{}
		if hasCompletion {
			decoded, err := codec.StrictDecodePayload(raw)
			if err != nil {
				return nil, nil, fmt.Errorf("completion %x malformed: %w", completion, err)
			}
			f, ok := decoded.([]any)
			if !ok || len(f) != 5 || !equalBytes(f[0], completion[:]) || !equalBytes(f[1], task.ID[:]) {
				return nil, nil, fmt.Errorf("completion %x misbound to causal task", completion)
			}
			digest, err := registeredDigest("cote.csf.digest.perception-completion",
				"cote.csf.schema.perception-task-completion.body", f[:4])
			if err != nil || !equalBytes(f[4], digest[:]) {
				return nil, nil, fmt.Errorf("completion %x digest invalid", completion)
			}
			for kind := 0; kind < 2; kind++ {
				ids, ok := f[2+kind].([]any)
				if !ok {
					return nil, nil, errors.New("completion addressed roster malformed")
				}
				for _, value := range ids {
					id, err := o2Bytes(value)
					if err != nil {
						return nil, nil, err
					}
					if _, seen := expected[id]; seen {
						return nil, nil, errors.New("duplicated completion evidence ref")
					}
					expected[id] = kind
				}
			}
			completions = append(completions, o2Ref("causal_outbox", [32]byte(completion), raw))
		}
		// This reader has MaxOpenConns(1). Collect the bounded route index and
		// CLOSE its cursor before resolving any evidence_ledger record by ID.
		// Nested queries while rows is open would wait forever for the same conn.
		type routeRef struct {
			id   [32]byte
			hash []byte
			kind int
		}
		rows, err := q.audit.QueryContext(ctx,
			"SELECT record_id,canonical_hash,kind FROM evidence_routes WHERE run_id=? AND task_id=? ORDER BY kind,record_id",
			run[:], task.ID[:])
		if err != nil {
			return nil, nil, err
		}
		routes := make([]routeRef, 0, len(expected))
		for rows.Next() {
			var idBytes, routeHash []byte
			var kind int
			if err := rows.Scan(&idBytes, &routeHash, &kind); err != nil {
				rows.Close()
				return nil, nil, err
			}
			id, err := o2Bytes(idBytes)
			if err != nil {
				rows.Close()
				return nil, nil, err
			}
			routes = append(routes, routeRef{id: id, hash: bytes.Clone(routeHash), kind: kind})
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, nil, err
		}
		if err := rows.Close(); err != nil {
			return nil, nil, err
		}
		seen := map[[32]byte]bool{}
		for _, route := range routes {
			id, kind := route.id, route.kind
			wanted, listed := expected[id]
			if !hasCompletion || seen[id] || kind < 0 || kind > 1 || !listed || wanted != kind {
				return nil, nil, fmt.Errorf("uncompleted/foreign evidence route %x for task %x", id, task.ID)
			}
			rawEvidence, err := q.raw(ctx, "evidence_ledger", id)
			if err != nil {
				return nil, nil, err
			}
			seal := identity.OpaqueContentHash(rawEvidence)
			if !bytes.Equal(route.hash, seal[:]) {
				return nil, nil, fmt.Errorf("evidence route %x hash altered", id)
			}
			value, err := codec.StrictDecodePayload(rawEvidence)
			if err != nil {
				return nil, nil, err
			}
			fields, ok := value.([]any)
			if !ok || len(fields) == 0 || !equalBytes(fields[0], id[:]) {
				return nil, nil, fmt.Errorf("evidence record %x identity mismatch", id)
			}
			if kind == 0 {
				if len(fields) != 15 || !equalBytes(fields[1], task.ID[:]) {
					return nil, nil, fmt.Errorf("Observation %x task identity mismatch", id)
				}
			} else if len(fields) != 11 {
				return nil, nil, fmt.Errorf("KnowledgeInput %x schema wrong", id)
			}
			seen[id] = true
			evidence = append(evidence, o2Ref("evidence_ledger", id, rawEvidence))
		}
		if len(seen) != len(expected) {
			return nil, nil, fmt.Errorf("completion evidence roster/route cardinality mismatch for task %x", task.ID)
		}
	}
	return completions, evidence, nil
}

// Project one causal task at a time so evidence cannot be mistaken for
// evidence delivered by a different task in the same CycleCommit.
func (q *PrivilegedReader) o2PerceptionLinks(ctx context.Context, run [32]byte, c *observatory.CausalCommit) error {
	c.Completions = make([]observatory.AuthorityRef, 0)
	c.Evidence = make([]observatory.AuthorityRef, 0)
	for _, task := range c.Tasks {
		_, fields, err := q.readRef(ctx, "causal_outbox", task.ID)
		if err != nil || len(fields) != 10 || !equalBytes(fields[0], task.ID[:]) ||
			!equalBytes(fields[1], run[:]) || !equalBytes(fields[2], c.CycleID[:]) ||
			fields[4] != c.BaseRevision || fields[5] != c.ResultRevision {
			return errors.New("perception task does not match commit coordinate")
		}
		link := observatory.PerceptionLink{Task: task}
		found := false
		for _, event := range c.Events {
			if equalBytes(fields[3], event.ID[:]) {
				link.Event, found = event, true
			}
		}
		if !found {
			return errors.New("perception task source absent from commit")
		}
		completions, evidence, err := q.o2TaskEvidence(ctx, run, []observatory.AuthorityRef{task})
		if err != nil || len(completions) > 1 {
			return fmt.Errorf("perception completion verification: %v", err)
		}
		if len(completions) == 1 {
			completion := completions[0]
			link.Completion = &completion
			_, f, err := q.readRef(ctx, "causal_outbox", completion.ID)
			if err != nil || len(f) != 5 {
				return errors.New("invalid perception completion link")
			}
			kinds := []struct {
				at     int
				target *[]observatory.AuthorityRef
			}{{2, &link.Observations}, {3, &link.KnowledgeInputs}}
			for _, kind := range kinds {
				list, ok := f[kind.at].([]any)
				if !ok {
					return errors.New("invalid perception completion evidence roster")
				}
				for _, id := range list {
					found := false
					for _, ref := range evidence {
						if equalBytes(id, ref.ID[:]) {
							*kind.target = append(*kind.target, ref)
							found = true
						}
					}
					if !found {
						return errors.New("completion evidence not authenticated")
					}
				}
			}
		} else if len(evidence) != 0 {
			return errors.New("pending task has evidence")
		}
		c.Completions = append(c.Completions, completions...)
		c.Evidence = append(c.Evidence, evidence...)
		c.PerceptionLinks = append(c.PerceptionLinks, link)
	}
	return nil
}

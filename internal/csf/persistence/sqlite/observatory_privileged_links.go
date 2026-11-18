package sqlite

import (
	"errors"
	"fmt"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/observatory"
)

// Each source and decision is already authenticated by full ReplayClock.
// Resolve explicit links again against the exact same immutable roster;
// never infer candidate ownership from an actor's identity or event type.
func o2DecisionLink(run, cycle [32]byte, attempt uint64, rec o2Record,
	fence []any, admitted, events []observatory.AuthorityRef,
	records map[[32]byte]o2Record) (observatory.DecisionLink, error) {
	var zero observatory.DecisionLink
	f := rec.fields
	if len(f) != 16 || !equalBytes(f[0], rec.id[:]) || !equalBytes(f[1], run[:]) ||
		!equalBytes(f[2], cycle[:]) || f[3] != attempt {
		return zero, errors.New("decision link scope mismatch")
	}
	source, ok := f[4].([]any)
	if !ok || len(source) < 3 || len(source) > 4 {
		return zero, errors.New("malformed decision source")
	}
	kind, err := o2Uint(source[0])
	if err != nil || kind > 3 || (kind == 0 && len(source) != 4) || (kind != 0 && len(source) != 3) {
		return zero, errors.New("invalid decision source kind")
	}
	id, err := o2Bytes(source[len(source)-2])
	if err != nil {
		return zero, err
	}
	digest, err := o2Bytes(source[len(source)-1])
	if err != nil {
		return zero, err
	}
	unitList, ok := fence[11].([]any)
	if !ok || len(unitList) != len(admitted) {
		return zero, errors.New("decision link missing fence roster")
	}
	var origin observatory.AuthorityRef
	found := false
	for i, value := range unitList {
		unit, ok := value.([]any)
		if !ok || len(unit) != 5 {
			return zero, errors.New("invalid fenced unit")
		}
		if !equalBytes(unit[3], id[:]) {
			continue
		}
		if found || unit[1] != kind || !equalBytes(unit[4], digest[:]) || admitted[i].ID != id {
			return zero, errors.New("decision source not bound to fence authority")
		}
		origin = admitted[i]
		found = true
	}
	if !found {
		return zero, errors.New("decision source absent from fence")
	}
	expected, err := registeredDigest("cote.csf.id.decision", "cote.csf.schema.decision-id-preimage",
		[]any{cycle[:], attempt, id[:]})
	if err != nil || [32]byte(expected) != rec.id {
		return zero, errors.New("decision ID not derived from admitted source")
	}
	dg, err := o2Bytes(f[15])
	if err != nil {
		return zero, err
	}
	status, err := o2Uint(f[5])
	if err != nil {
		return zero, err
	}
	out := observatory.DecisionLink{
		Decision: o2Ref("decision_ledger", rec.id, rec.bytes), Source: origin,
		SourceDigest: digest, DecisionDigest: dg, Status: status,
	}
	option, ok := f[6].([]any)
	if !ok || len(option) == 0 {
		return zero, errors.New("candidate option malformed")
	}
	switch {
	case len(option) == 1 && option[0] == uint64(0):
		// NoProposal has no candidate. Replay verifies status compatibility.
	case len(option) == 2 && option[0] == uint64(1):
		cid, err := o2Bytes(option[1])
		if err != nil {
			return zero, err
		}
		c, ok := records[cid]
		if !ok || len(c.fields) != 17 || !equalBytes(c.fields[0], cid[:]) ||
			!equalBytes(c.fields[1], run[:]) || !equalBytes(c.fields[2], cycle[:]) {
			return zero, errors.New("candidate missing or foreign")
		}
		members, ok := c.fields[3].([]any)
		if !ok || len(members) == 0 {
			return zero, errors.New("candidate has no source roster")
		}
		contains := false
		for _, m := range members {
			if equalBytes(m, id[:]) {
				contains = true
			}
		}
		if !contains {
			return zero, errors.New("candidate does not own decision source")
		}
		want, err := registeredDigest("cote.csf.id.commit-candidate", "cote.csf.schema.candidate-id-preimage",
			[]any{run[:], cycle[:], c.fields[3], c.fields[4], c.fields[5], c.fields[6], c.fields[7]})
		if err != nil || [32]byte(want) != cid {
			return zero, errors.New("candidate derived ID mismatch")
		}
		hash, err := registeredDigest("cote.csf.digest.commit-candidate", "cote.csf.schema.commit-candidate.body", c.fields[:16])
		if err != nil || !equalBytes(c.fields[16], hash[:]) {
			return zero, errors.New("candidate digest mismatch")
		}
		ref := o2Ref("decision_ledger", cid, c.bytes)
		out.Candidate = &ref
	default:
		return zero, errors.New("invalid candidate option")
	}
	// Explicit conflict/draw references carry normative digests. Replay
	// independently rederives the registered conflict and RNG semantics.
	proofs := []struct {
		at, size int
		kind uint64
		target *[]observatory.AuthorityRef
	}{
		{8, 7, 9, &out.Conflicts}, {11, 11, 24, &out.Draws},
	}
	for _, spec := range proofs {
		list, ok := f[spec.at].([]any)
		if !ok {
			return zero, errors.New("malformed decision proof roster")
		}
		seen := map[[32]byte]bool{}
		for _, raw := range list {
			ref, ok := raw.([]any)
			if !ok || len(ref) != 3 || ref[0] != spec.kind {
				return zero, errors.New("invalid decision proof reference")
			}
			key, err := o2Bytes(ref[1])
			if err != nil {
				return zero, err
			}
			p, ok := records[key]
			if !ok || seen[key] || len(p.fields) != spec.size ||
				!equalBytes(p.fields[0], key[:]) || !sameCanonical(p.fields[spec.size-1], ref[2]) {
				return zero, errors.New("decision proof absent, duplicated or digest mismatch")
			}
			seen[key] = true
			*spec.target = append(*spec.target, o2Ref("decision_ledger", key, p.bytes))
		}
	}
	// A contested terminal persists one assessment per admitted response.
	// Single-source terminals need not persist one. Only the exact subject
	// and source digest can be linked to this decision.
	if len(out.Conflicts) > 0 || len(out.Draws) > 0 {
		for key, record := range records {
			if len(record.fields) != 3 {
				continue
			}
			sub, ok := record.fields[0].([]any)
			if !ok || len(sub) != 3 || sub[0] != uint64(5) || !equalBytes(sub[1], id[:]) {
				continue
			}
			if out.Assessment != nil || !equalBytes(sub[2], digest[:]) {
				return zero, errors.New("ambiguous or mismatched assessment subject")
			}
			hash, err := registeredDigest("cote.csf.digest.affordance-assessment",
				"cote.csf.schema.affordance-assessment.body", record.fields[:2])
			if err != nil || [32]byte(hash) != key || !equalBytes(record.fields[2], hash[:]) {
				return zero, errors.New("assessment digest mismatch")
			}
			ref := o2Ref("decision_ledger", key, record.bytes)
			out.Assessment = &ref
		}
		if out.Assessment == nil {
			return zero, errors.New("contested decision missing assessment")
		}
	}
	produced, ok := f[12].([]any)
	if !ok {
		return zero, errors.New("invalid decision event roster")
	}
	seenEvents := map[[32]byte]bool{}
	for _, raw := range produced {
		event, err := o2Bytes(raw)
		if err != nil || seenEvents[event] {
			return zero, errors.New("invalid or repeated decision event")
		}
		found := false
		for _, e := range events {
			if e.ID == event {
				out.ProducedEvents = append(out.ProducedEvents, e)
				found = true
			}
		}
		if !found {
			return zero, fmt.Errorf("decision event %x not in terminal", event)
		}
		seenEvents[event] = true
	}
	return out, nil
}

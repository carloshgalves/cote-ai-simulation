package decision

import (
	"sync"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
)

// Each worker writes a disjoint result slot; no worker commits, resolves a
// conflict or consumes shared RNG. The caller merges the slots after join.
type preparedOutcome struct {
	Result
	blocked, accessBlocked bool
}

type workerOutcome struct {
	value preparedOutcome
	err   error
}

func evaluateWorkers(units []prepared, parallelism int, evaluate func(prepared) (preparedOutcome, error)) []workerOutcome {
	out := make([]workerOutcome, len(units))
	if len(units) == 0 {
		return out
	}
	if parallelism <= 1 || len(units) == 1 {
		for i, unit := range units {
			out[i].value, out[i].err = evaluate(unit)
		}
		return out
	}
	if parallelism > len(units) {
		parallelism = len(units)
	}
	var wg sync.WaitGroup
	// Fixed disjoint strides and a bounded number of real goroutines.
	// Scheduling and completion order cannot alter output positions.
	for worker := 0; worker < parallelism; worker++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			for i := index; i < len(units); i += parallelism {
				out[i].value, out[i].err = evaluate(units[i])
			}
		}(worker)
	}
	wg.Wait()
	return out
}

// evaluateCanonicalWorkers preserves fence positions even if a worker returns
// partial evidence and an error. The caller must validate the permutation.
func evaluateCanonicalWorkers(ordered []prepared, workerOrder []int, parallelism int, evaluate func(prepared) (preparedOutcome, error)) []workerOutcome {
	if workerOrder == nil {
		return evaluateWorkers(ordered, parallelism, evaluate)
	}
	workers := make([]prepared, len(ordered))
	for i, index := range workerOrder {
		workers[i] = ordered[index]
	}
	outcomes := evaluateWorkers(workers, parallelism, evaluate)
	canonical := make([]workerOutcome, len(ordered))
	for hostIndex, fenceIndex := range workerOrder {
		canonical[fenceIndex] = outcomes[hostIndex]
	}
	return canonical
}

func evaluatePrepared(d identity.Deriver, input Input, p prepared, refs map[string][]any) (preparedOutcome, error) {
	var out preparedOutcome
	if p.noProposal || p.aliasFound {
		return out, nil
	}
	if p.indeterminate || !input.Snapshot.Known {
		out.Indeterminate = append(out.Indeterminate, Indeterminate{
			Subject: p.id, SubjectDigest: p.unit.Response.Digest, SubjectKind: 0, Facet: 2, Validator: refs["validators"], Reason: "resource.validator.missing", Evidence: []any{},
		})
		return out, nil
	}
	actor := p.unit.Response.Actor[0] + ":" + p.unit.Response.Actor[1]
	canAccess, accessKnown := input.Snapshot.AccessibleActors[actor]
	forbidden, institutionKnown := input.Snapshot.ForbiddenActors[actor]
	if !accessKnown || !institutionKnown {
		facet, reason := uint8(1), "access.validator.missing"
		if accessKnown {
			facet = 4
			reason = "institution.validator.missing"
		}
		out.Indeterminate = append(out.Indeterminate, Indeterminate{
			Subject: p.id, SubjectDigest: p.unit.Response.Digest, SubjectKind: 0, Facet: facet, Validator: refs["validators"], Reason: reason, Evidence: []any{},
		})
		return out, nil
	}
	resourceBlocked := p.amount > input.Snapshot.Available
	assessment, err := assess(d, p, resourceBlocked, canAccess, forbidden, refs["validators"])
	if err != nil {
		return out, err
	}
	out.Assessments = append(out.Assessments, assessment)
	out.blocked = resourceBlocked || !canAccess
	out.accessBlocked = !canAccess
	// Candidate provenance resolves the authentic ActionProposal unit root.
	source := []any{uint64(5), p.id[:], p.unit.Response.Digest[:]}
	claims := p.unit.Response.Body[9].([]any)
	candidate, err := MakeCandidate(d, input.RunID, input.CycleID, []domain.ID{p.id},
		p.producer, 1, 0, 0, claims, []any{source}, 0, refs["conflict-resolvers"])
	if err != nil {
		return out, err
	}
	// The synthetic V1 resource action has exactly one declared event draft.
	// Its typed payload is the already validated immutable proposal params;
	// the claim amount is independently authenticated from that proposal.
	// Loser drafts are never events unless settlement commits them.
	if p.producer == "resource.consume" {
		draft := []any{uint64(0), uint64(0), "resource.consumed", p.unit.Response.Body[8], []any{}}
		candidate.Fields[8] = []any{draft}
		h, err := digest(d, "cote.csf.digest.commit-candidate", "cote.csf.schema.commit-candidate.body", candidate.Fields)
		if err != nil {
			return out, err
		}
		candidate.Digest = h
		candidate.Canonical, err = encode(candidate.Fields, h)
		if err != nil {
			return out, err
		}
	}
	out.Candidates = append(out.Candidates, candidate)
	return out, nil
}

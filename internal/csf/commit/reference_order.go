package commit

import (
	"hash/fnv"
	"sort"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/decision"
)

// referenceOrder is host-only experiment metadata, never a normative RNG
// seed, a policy input, or a durable source. Nil retains default ordering.
// Each phase gets its own deterministic stream; adding a phase cannot
// silently change another phase's physical/causal inputs.
type referenceOrder struct {
	seed        int64
	reverse     bool
	trace       map[string][]int
	parallelism int
}

func referencePermute[T any](order *referenceOrder, phase string, values []T) {
	if order == nil {
		return
	}
	indices := make([]int, len(values))
	for i := range indices {
		indices[i] = i
	}
	if order.reverse {
		for i, j := 0, len(indices)-1; i < j; i, j = i+1, j-1 {
			indices[i], indices[j] = indices[j], indices[i]
		}
	} else {
		h := fnv.New64a()
		_, _ = h.Write([]byte(phase))
		base := h.Sum64() ^ uint64(order.seed)
		sort.Slice(indices, func(i, j int) bool {
			return referenceOrderRank(base, indices[i]) < referenceOrderRank(base, indices[j])
		})
	}
	copyOfValues := append([]T(nil), values...)
	for i, original := range indices {
		values[i] = copyOfValues[original]
	}
	if order.trace != nil {
		order.trace[phase] = indices
	}
}

// referenceOrderRank is a deterministic host scheduling rank, not a causal
// identity, random source or policy. The odd multipliers and xor shifts are
// bijections on uint64, so distinct indices have distinct ranks.
func referenceOrderRank(base uint64, index int) uint64 {
	x := base + (uint64(index)+1)*0x9e3779b97f4a7c15
	x = (x ^ (x >> 30)) * 0xbf58476d1ce4e5b9
	x = (x ^ (x >> 27)) * 0x94d049bb133111eb
	return x ^ (x >> 31)
}

func referenceWorkerOrder(order *referenceOrder, phase string, n int) []int {
	if order == nil {
		return nil
	}
	indices := make([]int, n)
	for i := range indices {
		indices[i] = i
	}
	referencePermute(order, phase, indices)
	return indices
}

// referenceDecisionRun selects only the host execution strategy, never a
// causal policy. Default/legacy callers retain the serial kernel.
func referenceDecisionRun(input decision.Input, order *referenceOrder, phase string) (decision.Result, error) {
	workers := referenceWorkerOrder(order, phase, len(input.Admitted))
	if order == nil || order.parallelism == 0 {
		return decision.RunWithWorkerOrder(input, workers)
	}
	return decision.RunWithParallelism(input, workers, order.parallelism)
}

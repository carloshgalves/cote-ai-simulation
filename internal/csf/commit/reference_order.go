package commit

import (
	"hash/fnv"
	"math/rand"
)

// referenceOrder is host-only experiment metadata, never a normative RNG
// seed, a policy input, or a durable source. Nil retains default ordering.
// Each phase gets its own deterministic stream; adding a phase cannot
// silently change another phase's physical/causal inputs.
type referenceOrder struct {
	seed    int64
	reverse bool
	trace   map[string][]int
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
		r := rand.New(rand.NewSource(order.seed ^ int64(h.Sum64())))
		r.Shuffle(len(indices), func(i, j int) { indices[i], indices[j] = indices[j], indices[i] })
	}
	copyOfValues := append([]T(nil), values...)
	for i, original := range indices {
		values[i] = copyOfValues[original]
	}
	if order.trace != nil {
		order.trace[phase] = indices
	}
}

//go:build linux

package commit

import (
	"testing"
)

// Two real signed physical A1 epochs are reused byte-for-byte; C3 and the
// resource contest execute through the same pure kernel, now with different
// actual worker counts. No fresh exogenous cut is substituted.
func TestReferenceG3ActualParallelismWholeTranscript(t *testing.T) {
	host, plans, cuts := referenceE2SignedPair(t)
	one := &referenceOrder{parallelism: 1, trace: map[string][]int{}}
	p, owner, baseline := referenceE5Run(t, host, plans, &cuts, one, 0, 0, nil)
	expected := referenceE2Capture(t, p, owner)
	for _, count := range []int{2, 4} {
		order := &referenceOrder{parallelism: count, reverse: true, trace: map[string][]int{}}
		db, private, proof := referenceE5Run(t, host, plans, &cuts, order, 0, 0, &baseline)
		if proof.Matrix.ReplayCommits != 9 || proof.RunID != baseline.RunID {
			t.Fatal("parallel run did not finish the same integrated history")
		}
		for _, phase := range []string{"abort-validator-workers", "contest-validator-workers"} {
			if len(one.trace[phase]) != 2 || len(order.trace[phase]) != 2 ||
				order.trace[phase][0] != 1 || order.trace[phase][1] != 0 {
				t.Fatal("missing real worker cohort or reversed order", phase)
			}
		}
		got := referenceE2Capture(t, db, private)
		if err := referenceE1Compare(expected, got); err != nil {
			t.Fatal("actual parallelism changed authoritative bytes", count, err)
		}
	}
}

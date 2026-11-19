package decision

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
)

// The unbuffered rendezvous requires both real workers to enter their
// evaluator before either may exit. A serial fake cannot satisfy it.
func TestG3WorkerPoolActuallyConcurrent(t *testing.T) {
	units := []prepared{{id: domain.ID{1}}, {id: domain.ID{2}}}
	entered := make(chan domain.ID, 2)
	release := make(chan struct{})
	done := make(chan []workerOutcome, 1)
	go func() {
		done <- evaluateWorkers(units, 2, func(p prepared) (preparedOutcome, error) {
			entered <- p.id
			<-release
			return preparedOutcome{Result: Result{Dispositions: []Disposition{{Subject: p.id}}}}, nil
		})
	}()
	released := false
	defer func() {
		if !released {
			close(release)
		}
	}()
	seen := map[domain.ID]bool{}
	for i := 0; i < 2; i++ {
		select {
		case id := <-entered:
			seen[id] = true
		case <-time.After(10 * time.Second):
			t.Fatal("two workers never overlapped")
		}
	}
	if len(seen) != 2 || !seen[units[0].id] || !seen[units[1].id] {
		t.Fatal("worker cohort was not actually evaluated")
	}
	close(release)
	released = true
	select {
	case out := <-done:
		if len(out) != 2 || out[0].err != nil || out[1].err != nil ||
			out[0].value.Dispositions[0].Subject != units[0].id || out[1].value.Dispositions[0].Subject != units[1].id {
			t.Fatal("parallel results did not preserve input positions")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("worker group did not join")
	}
}

func TestG3ParallelDecisionCanonicalAndInvalidCounts(t *testing.T) {
	for _, known := range []bool{true, false} {
		in, _, _ := fixture(t, known)
		base, baseErr := Run(in)
		if known && baseErr != nil || !known && !errors.Is(baseErr, ErrIndeterminate) {
			t.Fatal("invalid serial baseline", baseErr)
		}
		for _, n := range []int{1, 2, 4, 64} {
			for _, order := range [][]int{nil, {0, 1}, {1, 0}} {
				got, err := RunWithParallelism(in, order, n)
				if !errors.Is(err, baseErr) || !reflect.DeepEqual(base, got) {
					t.Fatal("parallelism changed canonical result", n, order, err)
				}
			}
		}
		for _, n := range []int{-1, 0, 65} {
			got, err := RunWithParallelism(in, nil, n)
			if !errors.Is(err, ErrProvenance) || !reflect.DeepEqual(got, Result{}) {
				t.Fatal("invalid worker count gained authority", n, err)
			}
		}
	}
}

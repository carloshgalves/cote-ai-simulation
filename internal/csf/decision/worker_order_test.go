package decision

import (
	"bytes"
	"errors"
	"reflect"
	"testing"
)

func TestWorkerOrderCanonicalFencePreserved(t *testing.T) {
	for _, known := range []bool{true, false} {
		in, _, _ := fixture(t, known)
		base, baseErr := Run(in)
		if known && baseErr != nil || !known && !errors.Is(baseErr, ErrIndeterminate) {
			t.Fatal("invalid baseline", baseErr)
		}
		for _, order := range [][]int{{0, 1}, {1, 0}} {
			got, err := RunWithWorkerOrder(in, order)
			if !errors.Is(err, baseErr) || !bytes.Equal(base.Canonical, got.Canonical) || !reflect.DeepEqual(base, got) {
				t.Fatal("worker execution changed provisional result, membership or fence order", known, order, err)
			}
		}
		for _, invalid := range [][]int{{}, {0}, {0, 0}, {0, 2}, {-1, 1}, {0, 1, 0}} {
			got, err := RunWithWorkerOrder(in, invalid)
			if !errors.Is(err, ErrProvenance) || len(got.Canonical) != 0 || len(got.Dispositions) != 0 {
				t.Fatal("invalid host permutation acquired authority", invalid, err)
			}
		}
	}
}

package queryhost

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/observatory"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

// ObservatoryReader exposes only verified run-state reads.
type ObservatoryReader struct {
	host   CoreAuthority
	bundle *policy.Bundle
}

var _ observatory.Queries = (*ObservatoryReader)(nil)

func New(host CoreAuthority, bundle *policy.Bundle) *ObservatoryReader {
	return &ObservatoryReader{host: host, bundle: bundle}
}
func (r *ObservatoryReader) Close() error {
	if r == nil || r.host == nil {
		return errors.New("nil observatory reader")
	}
	return r.host.Close()
}
func (r *ObservatoryReader) dataVersion(ctx context.Context) (int64, error) {
	return r.host.DataVersion(ctx)
}

// ReadRun reuses the existing replay verifier rather than implementing a
// second causal engine. If another connection modifies the database during
// verification, fail closed instead of returning a mixed old/new projection.
func (r *ObservatoryReader) ReadRun(ctx context.Context, run [32]byte) (observatory.RunState, error) {
	var zero observatory.RunState
	if r == nil || r.host == nil || r.bundle == nil {
		return zero, errors.New("observatory reader is not initialized")
	}
	if err := r.bundle.VerifyInstalledBundle(); err != nil {
		return zero, fmt.Errorf("%w: %v", observatory.ErrInvalidPolicy, err)
	}
	versionBefore, err := r.dataVersion(ctx)
	if err != nil {
		return zero, err
	}
	first, err := r.host.LoadRun(ctx, run)
	if err != nil {
		return zero, err
	}
	replayed, err := r.host.Replay(ctx, run, r.bundle)
	if err != nil {
		return zero, fmt.Errorf("observatory causal replay verification: %w", err)
	}
	if first.Clock != replayed {
		return zero, errors.New("observatory run clock diverged from authenticated replay")
	}
	last, err := r.host.LoadRun(ctx, run)
	if err != nil {
		return zero, err
	}
	versionAfter, err := r.dataVersion(ctx)
	if err != nil {
		return zero, err
	}
	if versionBefore != versionAfter || !reflect.DeepEqual(first, last) {
		return zero, observatory.ErrSourceChanged
	}
	return last, nil
}

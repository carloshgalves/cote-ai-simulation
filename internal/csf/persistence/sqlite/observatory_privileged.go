package sqlite

import (
	"context"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/observatory/queryhost"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

type PrivilegedReader = queryhost.PrivilegedReader

func OpenPrivilegedObservatory(ctx context.Context, path string, bundle *policy.Bundle) (*PrivilegedReader, error) {
	host, err := openObservatoryAuthority(ctx, path, bundle)
	if err != nil {
		return nil, err
	}
	return queryhost.NewPrivileged(host, bundle), nil
}

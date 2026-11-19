package sqlite

import (
	"context"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/observatory"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/observatory/queryhost"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

type POVReader = queryhost.POVReader
type PublicReader = queryhost.PublicReader

func restrictedOpenError(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return observatory.ErrViewUnavailable
}

func OpenPOVObservatory(ctx context.Context, path string, bundle *policy.Bundle, run, dispatchID [32]byte, actor [2]string) (*POVReader, error) {
	host, err := openObservatoryAuthority(ctx, path, bundle)
	if err != nil {
		return nil, restrictedOpenError(ctx)
	}
	return queryhost.NewPOV(ctx, actorAuthority{coreAuthority{host: host}}, bundle, run, dispatchID, actor)
}

func OpenPublicObservatory(ctx context.Context, path string, bundle *policy.Bundle) (*PublicReader, error) {
	host, err := openObservatoryAuthority(ctx, path, bundle)
	if err != nil {
		return nil, restrictedOpenError(ctx)
	}
	return queryhost.NewPublic(publicAuthority{coreAuthority{host: host}}, bundle), nil
}

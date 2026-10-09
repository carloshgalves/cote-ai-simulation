// Package identity is the only causal entrypoint for registered IDs and digests.
package identity

import (
	"crypto/sha256"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

type Deriver struct {
	bundle          *policy.Bundle
	includeFixtures bool
}

func New(bundle *policy.Bundle) Deriver { return Deriver{bundle: bundle} }

func NewConformance(bundle *policy.Bundle) Deriver {
	return Deriver{bundle: bundle, includeFixtures: true}
}

func (d Deriver) Sum(domainTag, schemaID string, schemaVersion uint32, value any) (domain.Digest, []byte, error) {
	operation, err := d.bundle.Operation(domainTag, schemaID, schemaVersion, d.includeFixtures)
	if err != nil {
		return domain.Digest{}, nil, err
	}
	encoded, err := codec.CanonicalBytes(operation.DomainTag, operation.SchemaID, operation.SchemaVersion, value)
	if err != nil {
		return domain.Digest{}, nil, err
	}
	return sha256.Sum256(encoded), encoded, nil
}

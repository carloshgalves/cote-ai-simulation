// Package identity is the only causal entrypoint for registered IDs and digests.
package identity

import (
	"crypto/sha256"
 "fmt"

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

// OpaqueContentHash hashes content bytes whose digest is stored as data rather
// than used as a causal ID or registered domain-operation digest.
func OpaqueContentHash(content []byte) domain.Digest { return sha256.Sum256(content) }

// NamedSubstreamV1 is the RNG adapter for the deterministic CSFRV1-4
// provisional decision fixture. A policy reference and stable semantic
// components are part of the preimage; no wall clock or completion order.
// This implementation is not a new registered causal ID/digest operation:
// its raw SHA256 bytes are the typed result of the separately registered
// RngDraw record, whose ID/digest are derived with Deriver.Sum.
func NamedSubstreamV1(seed domain.Digest,subsystem,key string,entities []any,purpose string,policyRef []any)(domain.Digest,error){
 for _,s:=range []string{subsystem,key,purpose}{
  if err:=domain.ValidateMachineID(s);err!=nil{return domain.Digest{},err}
 }
 if len(policyRef)!=3{return domain.Digest{},fmt.Errorf("rng policy ref missing")}
 data,err:=codec.CanonicalPayload([]any{seed[:],subsystem,key,entities,purpose,policyRef})
 if err!=nil{return domain.Digest{},err}
 return sha256.Sum256(data),nil
}

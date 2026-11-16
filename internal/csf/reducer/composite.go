package reducer

import (
	"bytes"
	"errors"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
)

// CompositeVariant is a closed, versioned *local reference* WorldState
// variant. Both child typed values retain their independently authenticated
// registered schemas and digests, and the outer typed value commits to their
// exact canonical order and bytes. This does not register a new physical
// producer or authorize A1: Genesis must still pin PhysicalRegistration.
//
// The array [stock, body] is deliberately fixed. Never use a map, dynamically
// typed payload, default empty body, or best-effort decoding: that would allow
// the Go engine to silently drop one subsystem when another reduces.
const CompositeVariant = "reference.stock-and-physical.v1"

var ErrInvalidComposite = errors.New("invalid stock+physical WorldState composition")

// ValidateCompositePayload checks the full nested trusted typed-value shapes,
// not just an outer hash or an asserted variant name. Nested composite values,
// reversed branches, duplicate sources and arbitrary extensions fail closed.
func ValidateCompositePayload(value any) error {
	payload, ok := value.([]any)
	if !ok || len(payload) != 3 || payload[0] != CompositeVariant || payload[1] != uint64(1) {
		return ErrInvalidComposite
	}
	components, ok := payload[2].([]any)
	if !ok || len(components) != 2 {
		return ErrInvalidComposite
	}
	for i, kind := range []string{StockVariant, PhysicalVariant} {
		typed, ok := components[i].([]any)
		if !ok || len(typed) != 4 || typed[0] != StockSchemaID || typed[1] != uint64(1) {
			return ErrInvalidComposite
		}
		raw, ok := typed[2].([]byte)
		if !ok {
			return ErrInvalidComposite
		}
		h, ok := typed[3].([]byte)
		if !ok || len(h) != 32 {
			return ErrInvalidComposite
		}
		digest := identity.OpaqueContentHash(raw)
		if !bytes.Equal(h, digest[:]) {
			return ErrInvalidComposite
		}
		env, err := codec.StrictDecodeEnvelope(raw)
		if err != nil || env.DomainTag != StockDomainTag || env.SchemaID != StockSchemaID ||
			env.SchemaVersion != 1 {
			return ErrInvalidComposite
		}
		inner, ok := env.Value.([]any)
		if !ok || len(inner) != 3 || inner[0] != kind || inner[1] != uint64(1) {
			return ErrInvalidComposite
		}
		if i == 0 {
			if ValidateStockPayload(inner) != nil {
				return ErrInvalidComposite
			}
		} else if ValidatePhysicalPayload(inner) != nil {
			return ErrInvalidComposite
		}
	}
	return nil
}

// EncodeComposite creates the only permitted canonical order of the two
// independently authenticated typed components, with a new outer digest.
// Neither producer receives authority over the other component.
func EncodeComposite(stock Stock, bodies []PhysicalBody) ([]any, domain.Digest, error) {
	var zero domain.Digest
	resource, _, err := EncodeStock(stock)
	if err != nil {
		return nil, zero, err
	}
	physical, _, err := EncodePhysical(bodies)
	if err != nil {
		return nil, zero, err
	}
	payload := []any{CompositeVariant, uint64(1), []any{resource, physical}}
	if err := ValidateCompositePayload(payload); err != nil {
		return nil, zero, err
	}
	raw, err := codec.CanonicalBytes(StockDomainTag, StockSchemaID, 1, payload)
	if err != nil {
		return nil, zero, err
	}
	digest := identity.OpaqueContentHash(raw)
	return []any{StockSchemaID, uint64(1), raw, digest[:]}, domain.Digest(digest), nil
}

// DecodeComposite rejects any standalone state or unregistered alternative.
// Distinct read APIs for stock and physical state may safely project from
// this same immutable outer WorldState without changing its authority.
func DecodeComposite(value any) (Stock, []PhysicalBody, error) {
	var none Stock
	typed, ok := value.([]any)
	if !ok || len(typed) != 4 || typed[0] != StockSchemaID || typed[1] != uint64(1) {
		return none, nil, ErrInvalidComposite
	}
	raw, ok := typed[2].([]byte)
	if !ok {
		return none, nil, ErrInvalidComposite
	}
	declared, ok := typed[3].([]byte)
	if !ok || len(declared) != 32 {
		return none, nil, ErrInvalidComposite
	}
	computed := identity.OpaqueContentHash(raw)
	if !bytes.Equal(declared, computed[:]) {
		return none, nil, ErrInvalidComposite
	}
	envelope, err := codec.StrictDecodeEnvelope(raw)
	if err != nil || envelope.DomainTag != StockDomainTag || envelope.SchemaID != StockSchemaID ||
		envelope.SchemaVersion != 1 {
		return none, nil, ErrInvalidComposite
	}
	if err := ValidateCompositePayload(envelope.Value); err != nil {
		return none, nil, err
	}
	components := envelope.Value.([]any)[2].([]any)
	stock, err := DecodeStock(components[0])
	if err != nil {
		return none, nil, ErrInvalidComposite
	}
	physical, err := DecodePhysical(components[1])
	if err != nil {
		return none, nil, ErrInvalidComposite
	}
	return stock, physical, nil
}

func isCompositeWorldState(value any) bool {
	typed, ok := value.([]any)
	if !ok || len(typed) != 4 {
		return false
	}
	raw, ok := typed[2].([]byte)
	if !ok {
		return false
	}
	env, err := codec.StrictDecodeEnvelope(raw)
	if err != nil {
		return false
	}
	payload, ok := env.Value.([]any)
	return ok && len(payload) > 0 && payload[0] == CompositeVariant
}

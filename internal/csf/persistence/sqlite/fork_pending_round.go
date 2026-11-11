package sqlite

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/genesis"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
)

type forkGenesisRound struct {
	ID        [32]byte
	Canonical []byte
	Seal      [32]byte
}

// migrateGenesisPendingRounds reconstructs Genesis-owned RoundDeclarations.
// V1 permits only unstarted rounds; V2 callers must also run the versioned
// history migrator and lineage verifier before publishing the child.
func migrateGenesisPendingRounds(ctx context.Context, s *Store,
	parent, child *genesis.Genesis, refs []any, originPolicy, childPolicy *policy.Bundle) ([]forkGenesisRound, error) {
	if len(refs) == 0 {
		return []forkGenesisRound{}, nil
	}
	records, err := s.Records(ctx)
	if err != nil {
		return nil, err
	}
	return migratePendingRoundRecords(parent, child, refs, records, nil, originPolicy, childPolicy, false)
}

// Shared by bootstrap and lineage replay: the archive is the source of truth,
// never a worker-provided round projection.
func migratePendingRoundRecords(parent, child *genesis.Genesis, refs []any,
	records, events []ports.CanonicalRecord, originPolicy, childPolicy *policy.Bundle, allowHistory bool) ([]forkGenesisRound, error) {
	out := make([]forkGenesisRound, 0, len(refs))
	seen := map[[32]byte]bool{}
	touched := map[string]bool{}
	byID := map[string][]byte{}
	for _, record := range records {
		byID[string(record.Identity[:])] = record.Canonical
		value, err := codec.StrictDecodePayload(record.Canonical)
		if err != nil {
			if _, envelopeErr := codec.StrictDecodeEnvelope(record.Canonical); envelopeErr == nil {
				continue
			}
			return nil, errors.New("fork cannot classify a parent input record")
		}
		fields, ok := value.([]any)
		if !ok {
			return nil, errors.New("fork parent input record is not canonical array")
		}
		if len(fields) != 11 || !equalBytes(fields[1], parent.RunID[:]) {
			continue
		}
		if _, cycleOK := fields[2].([]byte); !cycleOK {
			continue
		}
		roundID, roundOK := fields[3].([]byte)
		if _, slotOK := fields[4].(string); roundOK && slotOK && len(roundID) == 32 {
			touched[string(roundID)] = true
		}
	}
	childID, _, _, _, err := childPolicy.CodecIdentity()
	if err != nil {
		return nil, err
	}
	eventOrdinals, err := allocateEventRoundOrdinals(refs, byID)
	if err != nil {
		return nil, err
	}
	eventByID := map[string]ports.CanonicalRecord{}
	for _, record := range events {
		eventByID[string(record.Identity[:])] = record
	}
	for _, rawRef := range refs {
		ref, ok := rawRef.([]any)
		if !ok || len(ref) != 3 || ref[0] != uint64(3) {
			return nil, errors.New("fork pending round reference not canonical")
		}
		parentID, ok := ref[1].([]byte)
		if !ok || len(parentID) != 32 {
			return nil, errors.New("fork parent round identity invalid")
		}
		if touched[string(parentID)] && (!allowHistory || childID != policy.CodecPolicyV2ID) {
			return nil, errors.New("fork round has prior dispatch history without a versioned migrator")
		}
		registeredDigest, ok := ref[2].([]byte)
		if !ok || len(registeredDigest) != 32 {
			return nil, errors.New("fork parent round digest invalid")
		}
		raw, exists := byID[string(parentID)]
		if !exists {
			return nil, fmt.Errorf("fork pending round is not stored: %x", parentID)
		}
		value, err := codec.StrictDecodePayload(raw)
		if err != nil {
			return nil, err
		}
		f, ok := value.([]any)
		if !ok || len(f) != 11 || !equalBytes(f[0], parentID) || !equalBytes(f[1], parent.RunID[:]) {
			return nil, errors.New("fork pending round source schema/run invalid")
		}
		digest, _, err := identity.New(originPolicy).Sum("cote.csf.digest.round-declaration",
			"cote.csf.schema.round-declaration", 1, f)
		if err != nil || !bytes.Equal(digest[:], registeredDigest) {
			return nil, errors.New("fork round checkpoint digest differs from source record")
		}
		role, roleOK := f[7].(uint64)
		ordinal, ordinalOK := f[8].(uint64)
		lifecycle, lifeOK := f[9].(uint64)
		coordinate, coordOK := f[5].([]any)
		origin, originOK := f[6].([]any)
		if !roleOK || role > 65535 || !ordinalOK || ordinal > uint64(^uint32(0)) ||
			!lifeOK || lifecycle != 0 || !coordOK || len(coordinate) != 2 || !originOK || len(origin) != 2 ||
			(origin[0] != uint64(2) && (origin[0] != uint64(0) || !allowHistory || childID != policy.CodecPolicyV2ID)) {
			return nil, errors.New("fork round has unsupported lifecycle or origin")
		}
		parentRoot, ok := origin[1].([]any)
		if !ok || len(parentRoot) != 2 {
			return nil, errors.New("fork round origin reference malformed")
		}
		childOrdinal := ordinal
		if origin[0] == uint64(2) {
			if !equalBytes(parentRoot[0], parent.ID[:]) || !equalBytes(parentRoot[1], parent.Digest[:]) {
				return nil, errors.New("fork round origin is not authenticated parent Genesis")
			}
		} else {
			if err := authenticateRoundEventOrigin(parent, parentRoot, eventByID, originPolicy); err != nil {
				return nil, err
			}
			childOrdinal = eventOrdinals[string(parentID)]
		}
		instant, valid := asSigned64(coordinate[0])
		_, validOrdinal := coordinate[1].(uint64)
		if !valid || !validOrdinal || instant < int64(child.InitialInstant) {
			return nil, errors.New("fork round eligibility precedes child Genesis clock")
		}
		source, sourceOK := f[2].(string)
		actor, actorOK := f[3].([]any)
		slots, slotsOK := f[4].([]any)
		if !sourceOK || domain.ValidateMachineID(source) != nil || !actorOK || len(actor) != 2 ||
			!slotsOK || len(slots) == 0 || len(slots) > 1024 {
			return nil, errors.New("fork Genesis round source/roster malformed")
		}
		for _, a := range actor {
			s, ok := a.(string)
			if !ok || domain.ValidateMachineID(s) != nil {
				return nil, errors.New("fork round actor identity malformed")
			}
		}
		var previous string
		for i, item := range slots {
			slot, ok := item.(string)
			if !ok || domain.ValidateMachineID(slot) != nil || (i > 0 && slot <= previous) {
				return nil, errors.New("fork round slots must form a canonical set")
			}
			previous = slot
		}
		// Reject forged source ID even if an adversary arranged a matching
		// snapshot digest; a round's ID is derived from its Genesis origin.
		wantParent, _, err := identity.New(originPolicy).Sum("cote.csf.id.decision-round",
			"cote.csf.schema.decision-round-id-preimage", 1, []any{f[6], role, ordinal})
		if err != nil || !bytes.Equal(wantParent[:], parentID) {
			return nil, errors.New("fork parent round causal identity mismatched")
		}
		childOrigin := []any{uint64(2), []any{child.ID[:], child.Digest[:]}}
		childID, _, err := identity.New(childPolicy).Sum("cote.csf.id.decision-round",
			"cote.csf.schema.decision-round-id-preimage", 1, []any{childOrigin, role, childOrdinal})
		if err != nil {
			return nil, err
		}
		newFields := append([]any(nil), f...)
		newFields[0] = childID[:]
		newFields[1] = child.RunID[:]
		newFields[6] = childOrigin
		newFields[8] = childOrdinal
		newCanonical, err := codec.CanonicalPayload(newFields)
		if err != nil {
			return nil, err
		}
		id := [32]byte(childID)
		if seen[id] {
			return nil, errors.New("fork two rounds alias the same child identity")
		}
		seen[id] = true
		out = append(out, forkGenesisRound{
			ID: id, Canonical: newCanonical, Seal: [32]byte(identity.OpaqueContentHash(newCanonical)),
		})
	}
	return out, nil
}

// Genesis births keep their existing local coordinates. Event-born rounds use
// the lowest free ordinal in their role, in lexicographic parent Round ID order.
// This is a named migration output order, never append or worker order.
func allocateEventRoundOrdinals(refs []any, byID map[string][]byte) (map[string]uint64, error) {
	used := map[[2]uint64]bool{}
	roles := map[string]uint64{}
	for _, raw := range refs {
		ref, ok := raw.([]any)
		if !ok || len(ref) != 3 {
			return nil, errors.New("round allocation ref malformed")
		}
		id, ok := ref[1].([]byte)
		if !ok || len(id) != 32 {
			return nil, errors.New("round allocation ID malformed")
		}
		decoded, err := codec.StrictDecodePayload(byID[string(id)])
		if err != nil {
			return nil, err
		}
		f, ok := decoded.([]any)
		if !ok || len(f) != 11 {
			return nil, errors.New("round allocation record malformed")
		}
		role, okRole := f[7].(uint64)
		ordinal, okOrdinal := f[8].(uint64)
		origin, okOrigin := f[6].([]any)
		if !okRole || role > 65535 || !okOrdinal || ordinal > uint64(^uint32(0)) || !okOrigin || len(origin) != 2 {
			return nil, errors.New("round allocation fields malformed")
		}
		if origin[0] == uint64(2) {
			used[[2]uint64{role, ordinal}] = true
		} else if origin[0] == uint64(0) {
			roles[string(id)] = role
		}
	}
	return allocateForkEventCoordinates(roles, used)
}

// Shared canonical allocation for Event-owned rounds and occurrences.
func allocateForkEventCoordinates(roles map[string]uint64, used map[[2]uint64]bool) (map[string]uint64, error) {
	ids := make([]string, 0, len(roles))
	for id := range roles {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	result := map[string]uint64{}
	next := map[uint64]uint64{}
	for _, id := range ids {
		role := roles[id]
		ordinal := next[role]
		for used[[2]uint64{role, ordinal}] {
			ordinal++
			if ordinal > uint64(^uint32(0)) {
				return nil, errors.New("migrated birth ordinal namespace exhausted")
			}
		}
		if ordinal > uint64(^uint32(0)) {
			return nil, errors.New("migrated birth ordinal namespace exhausted")
		}
		result[id] = ordinal
		used[[2]uint64{role, ordinal}] = true
		next[role] = ordinal + 1
	}
	return result, nil
}

func authenticateRoundEventOrigin(parent *genesis.Genesis, ref []any, events map[string]ports.CanonicalRecord, b *policy.Bundle) error {
	id, ok := ref[0].([]byte)
	digest, okDigest := ref[1].([]byte)
	if !ok || len(id) != 32 || !okDigest || len(digest) != 32 {
		return errors.New("round Event origin malformed")
	}
	record, exists := events[string(id)]
	if !exists {
		return errors.New("round Event origin absent from frozen parent archive")
	}
	decoded, err := codec.StrictDecodePayload(record.Canonical)
	if err != nil {
		return err
	}
	f, ok := decoded.([]any)
	if !ok || len(f) != 19 || !equalBytes(f[0], id) || !equalBytes(f[1], parent.RunID[:]) || !equalBytes(f[18], digest) {
		return errors.New("round Event origin differs from authenticated parent")
	}
	want, _, err := identity.New(b).Sum("cote.csf.id.event", "cote.csf.schema.event-id-preimage", 1, []any{f[1], f[7], f[2]})
	if err != nil || !equalBytes(f[0], want[:]) {
		return errors.New("round source Event identity invalid")
	}
	dg, _, err := identity.New(b).Sum("cote.csf.digest.event", "cote.csf.schema.event.body", 1, f[:18])
	if err != nil || !equalBytes(f[18], dg[:]) {
		return errors.New("round source Event digest invalid")
	}
	return nil
}

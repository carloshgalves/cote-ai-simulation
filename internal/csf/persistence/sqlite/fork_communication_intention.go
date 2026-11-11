package sqlite

import (
	"errors"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/genesis"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
)

func communicationIntentionPair(ref []any, archive forkHistoryArchive) ([][]any, error) {
	var found [][]any
	for _, record := range archive["event_store"] {
		event, content, err := communicationIntentionWitness(record)
		if err != nil {
			return nil, err
		}
		if event == nil {
			continue
		}
		pair := [][]any{{uint64(18), content[2], content[3]}, {uint64(19), content[0], content[1]}}
		for _, candidate := range pair {
			if sameCanonical(candidate, ref) {
				if found != nil {
					return nil, errors.New("communication root claimed by multiple sent Events")
				}
				found = pair
			}
		}
	}
	if found == nil {
		return nil, errors.New("communication intention has no sent Event in frozen cut")
	}
	return found, nil
}

func communicationIntentionWitness(record ports.CanonicalRecord) ([]any, []any, error) {
	value, err := codec.StrictDecodePayload(record.Canonical)
	if err != nil {
		return nil, nil, err
	}
	f, ok := value.([]any)
	if !ok || len(f) != 19 || f[3] != "communication.sent" {
		return nil, nil, nil
	}
	typed, ok := f[9].([]any)
	if !ok || len(typed) != 4 || !communicationTypedDigestMatches(typed) {
		return nil, nil, errors.New("historical sent payload hash invalid")
	}
	raw, ok := typed[2].([]byte)
	if !ok {
		return nil, nil, errors.New("historical sent payload absent")
	}
	env, err := codec.StrictDecodeEnvelope(raw)
	if err != nil {
		return nil, nil, err
	}
	content, ok := env.Value.([]any)
	if !ok || len(content) != 6 || env.DomainTag != "cote.csf.communication.sent.v1" || env.SchemaID != "cote.csf.schema.typed-value" || env.SchemaVersion != 1 {
		return nil, nil, errors.New("historical sent payload schema invalid")
	}
	return f, content, nil
}

// The exact Claim/Transmission digest is attested by the frozen sent Event and
// its committed source ActionProposal. These historical roots do not stand in
// for a child send, nor do they grant actor knowledge of a Claim's assertion.
func authenticateCommunicationIntentionAtCut(kind uint64, ref, fields []any, parent *genesis.Genesis, snapshot []any, archive forkHistoryArchive, b *policy.Bundle) error {
	if kind == 18 && len(fields) != 10 || kind == 19 && len(fields) != 14 {
		return errors.New("historical communication root malformed")
	}
	origin, ok := fields[1].([]any)
	if !ok || len(origin) != 3 || origin[0] != uint64(5) {
		return errors.New("historical communication root lacks source proposal")
	}
	if err := authenticateArchivedCausalRef(origin, parent, snapshot, archive, b); err != nil {
		return err
	}
	pair, err := communicationIntentionPair(ref, archive)
	if err != nil {
		return err
	}
	if kind == 19 {
		if !sameCanonical(fields[4], []any{uint64(0), []any{pair[0]}}) {
			return errors.New("historical Transmission Claim differs from sent Event")
		}
		if err := authenticateArchivedCausalRef(pair[0], parent, snapshot, archive, b); err != nil {
			return err
		}
	}
	for _, record := range archive["event_store"] {
		event, content, err := communicationIntentionWitness(record)
		if err != nil {
			return err
		}
		if event == nil {
			continue
		}
		if !sameCanonical(content[0], pair[1][1]) || !sameCanonical(content[2], pair[0][1]) {
			continue
		}
		if !equalBytes(event[1], parent.RunID[:]) || !sameCanonical(event[11], []any{[]any{uint64(0), uint64(0), origin[1], origin[2]}}) {
			return errors.New("historical sent Event source differs from native root")
		}
		index := 6
		if kind == 19 {
			index = 9
		}
		if !sameCanonical(event[5], fields[index]) {
			return errors.New("historical communication instant differs from send")
		}
		if err := authenticateRoundEventOrigin(parent, []any{event[0], event[18]}, map[string]ports.CanonicalRecord{string(record.Identity[:]): record}, b); err != nil {
			return err
		}
		for _, journal := range archive["decision_ledger"] {
			value, err := codec.StrictDecodePayload(journal.Canonical)
			if err != nil {
				return err
			}
			f, ok := value.([]any)
			if !ok || len(f) != 24 || !equalBytes(f[0], parent.RunID[:]) || !sameCanonical(f[1], event[7]) {
				continue
			}
			result, valid := f[8].(uint64)
			cut, validCut := snapshot[3].(uint64)
			if !valid || !validCut || result > cut {
				return errors.New("historical send occurs after snapshot")
			}
			inputs, validInputs := f[11].([]any)
			events, validEvents := f[15].([]any)
			if !validInputs || !validEvents {
				return errors.New("historical sent journal rosters malformed")
			}
			inputFound, eventFound := false, false
			for _, id := range inputs {
				if sameCanonical(id, origin[1]) {
					inputFound = true
				}
			}
			for _, id := range events {
				if sameCanonical(id, event[0]) {
					eventFound = true
				}
			}
			if inputFound && eventFound {
				return nil
			}
		}
	}
	return errors.New("historical communication source has no committed send at cut")
}

package identity

import "fmt"

// RecordIdentityFields rebuilds a registered root identity from canonical fields.
// source supplies the external cycle witness required only by ConflictSet.
func RecordIdentityFields(root string, fields []any, source []any) ([]any, error) {
	if len(fields) < 3 {
		return nil, fmt.Errorf("record %s lacks identity fields", root)
	}
	from := func(indices ...int) ([]any, error) {
		out := make([]any, len(indices))
		for i, index := range indices {
			if index < 0 || index >= len(fields) {
				return nil, fmt.Errorf("record %s lacks identity field %d", root, index)
			}
			out[i] = fields[index]
		}
		return out, nil
	}
	switch root {
	case "event":
		return from(1, 7, 2)
	case "exogenous-input", "source-closure":
		return from(1, 2, 3)
	case "round-declaration":
		return from(6, 7, 8)
	case "slot-dispatch":
		return from(1, 2, 3, 4, 5)
	case "scheduled-occurrence":
		return from(9, 10, 11)
	case "commit-candidate":
		return from(1, 2, 3, 4, 5, 6, 7)
	case "admission-fence", "cycle-commit", "cycle-abort":
		return from(0, 1, 2)
	case "attempt-retry":
		return from(0, 1, 2, 3)
	case "perception-task":
		return from(1, 3, 6, 7, 8)
	case "observation":
		return from(1, 2, 3, 4)
	case "claim", "transmission":
		return from(1, 2, 3)
	case "snapshot":
		return from(0, 1, 3)
	case "genesis-manifest":
		return from(0)
	case "epistemic-checkpoint-ref":
		return from(0, 1, 2, 7)
	case "rng-draw":
		return from(1, 2, 3, 4, 5, 6, 7, 8)
	case "affordance-assessment":
		return from(0, 2)
	case "provisional-disposition":
		return from(1, 2, 3, 4)
	case "attempt-failure":
		return from(1, 2, 3, 5, 6, 4)
	case "action-proposal":
		return []any{fields[1], uint64(0)}, nil
	case "no-proposal":
		return []any{fields[1], uint64(1)}, nil
	case "trigger-activation":
		if len(fields) < 8 {
			return nil, fmt.Errorf("trigger activation lacks identity fields")
		}
		origin, ok := fields[5].([]any)
		if !ok || len(origin) < 2 {
			return nil, fmt.Errorf("invalid trigger activation origin")
		}
		tag, ok := origin[0].(uint64)
		if !ok {
			return nil, fmt.Errorf("invalid trigger origin discriminant")
		}
		revision := uint64(0)
		switch tag {
		case 0:
			if len(origin) != 4 {
				return nil, fmt.Errorf("invalid trigger cycle origin")
			}
			revision, ok = origin[2].(uint64)
			if !ok {
				return nil, fmt.Errorf("trigger result revision not unsigned")
			}
		case 1:
			if len(origin) != 2 {
				return nil, fmt.Errorf("invalid trigger genesis origin")
			}
		default:
			return nil, fmt.Errorf("unknown trigger origin")
		}
		return []any{fields[1], fields[3], fields[4], revision, fields[7]}, nil
	case "decision-record":
		if len(fields) < 5 {
			return nil, fmt.Errorf("decision lacks identity fields")
		}
		subject, ok := fields[4].([]any)
		if !ok || len(subject) < 3 {
			return nil, fmt.Errorf("invalid decision subject")
		}
		kind, ok := subject[0].(uint64)
		if !ok {
			return nil, fmt.Errorf("invalid decision kind")
		}
		index := 1
		if kind == 0 {
			index = 2
		}
		if index >= len(subject) {
			return nil, fmt.Errorf("invalid subject input ID")
		}
		return []any{fields[2], fields[3], subject[index]}, nil
	case "knowledge-input":
		if len(fields) < 8 {
			return nil, fmt.Errorf("knowledge input lacks identity fields")
		}
		ref, ok := fields[7].([]any)
		if !ok || len(ref) != 3 {
			return nil, fmt.Errorf("invalid observation ref")
		}
		return []any{ref[1], fields[3], fields[5], fields[1], fields[2]}, nil
	case "conflict-set":
		// The first ID-preimage component (cycle-id) is an external ledger
		// authority absent from the ConflictSet record itself. The binding
		// test can prove the remaining components and embedded ID, but not
		// the source of that cycle ID without an authoritative witness.
		if len(source) != 4 {
			return nil, fmt.Errorf("missing conflict cycle witness")
		}
		if cycle, ok := source[0].([]byte); !ok || len(cycle) != 32 {
			return nil, fmt.Errorf("invalid conflict cycle witness")
		}
		return []any{source[0], fields[1], fields[2], fields[3]}, nil
	default:
		return nil, fmt.Errorf("no record-derived identity projection for %s", root)
	}
}

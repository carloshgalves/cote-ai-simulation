package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/clock"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/inputledger"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/observatory"
)

var _ observatory.OperationalQueries = (*PrivilegedReader)(nil)

// ReadOperationalReport projects only verified durable authority. The
// outer data_version fence covers the privileged replay and all additional
// status queries, including status-only changes with an unchanged clock.
func (q *PrivilegedReader) ReadOperationalReport(ctx context.Context, run [32]byte) (observatory.OperationalReport, error) {
	var zero observatory.OperationalReport
	if q == nil || q.base == nil || q.base.bundle == nil {
		return zero, observatory.ErrInvalidPolicy
	}
	if err := q.base.bundle.VerifyInstalledBundle(); err != nil {
		return zero, fmt.Errorf("%w: %v", observatory.ErrInvalidPolicy, err)
	}
	version, err := q.base.dataVersion(ctx)
	if err != nil {
		return zero, err
	}
	verified, err := q.ReadPrivilegedReport(ctx, run)
	if err != nil {
		if ctx.Err() != nil {
			return zero, ctx.Err()
		}
		if errors.Is(err, observatory.ErrSourceChanged) || errors.Is(err, observatory.ErrInvalidPolicy) {
			return zero, err
		}
		return zero, fmt.Errorf("%w: %v", observatory.ErrInvalidReplay, err)
	}
	out := observatory.OperationalReport{
		State: verified.State, PendingPerceptionTasks: verified.PendingPerceptionTasks,
		RejectedIngress: verified.RejectedIngress,
	}
	pending, err := q.base.store.PendingClock(ctx, run)
	if err != nil {
		return zero, err
	}
	plan, exists, err := clock.Next(out.State.Clock, pending)
	if err != nil {
		return zero, fmt.Errorf("%w: %v", observatory.ErrInvalidReplay, err)
	}
	if exists {
		target := plan.Target
		out.NextPendingEligibility = &target
	}
	for _, item := range pending {
		p, err := q.operationalPending(ctx, run, item)
		if err != nil {
			return zero, err
		}
		out.Pending = append(out.Pending, p)
	}
	out.Sources, err = q.operationalSources(ctx, run, out.NextPendingEligibility)
	if err != nil {
		return zero, err
	}
	last, err := q.base.dataVersion(ctx)
	if err != nil {
		return zero, err
	}
	if version != last {
		return zero, observatory.ErrSourceChanged
	}
	return out, nil
}

// PendingClock is the existing normative pending membership verifier. Do
// not use lifecycle=0 SQL counts: previously settled immutable occurrences
// retain their historical lifecycle fields. Resolve only its actual roster.
func (q *PrivilegedReader) operationalPending(ctx context.Context, run [32]byte, item clock.Pending) (observatory.PendingDiagnostic, error) {
	var out observatory.PendingDiagnostic
	found := false
	id := [32]byte(item.ID)
	for _, source := range []struct {
		table, key, kind string
	}{
		{"input_ledger", "id", ""},
		{"scheduled_occurrences", "id", "OCCURRENCE"},
		{"trigger_activations", "id", "TRIGGER"},
		{"trigger_runtime", "definition_id", "TRIGGER_REPEAT"},
	} {
		var raw []byte
		query := "SELECT canonical FROM " + source.table + " WHERE " + source.key + "=?"
		args := []any{id[:]}
		if source.table != "input_ledger" {
			query += " AND run_id=?"
			args = append(args, run[:])
		}
		err := q.base.store.db.QueryRowContext(ctx, query, args...).Scan(&raw)
		if err == sql.ErrNoRows {
			continue
		}
		if err != nil {
			return out, err
		}
		if found {
			return observatory.PendingDiagnostic{}, errors.New("ambiguous pending authority")
		}
		value, err := codec.StrictDecodePayload(raw)
		if err != nil {
			return out, err
		}
		fields, ok := value.([]any)
		if !ok || len(fields) == 0 || !equalBytes(fields[0], id[:]) {
			return out, errors.New("invalid pending authority root")
		}
		kind := source.kind
		if source.table == "input_ledger" {
			if len(fields) == 11 && equalBytes(fields[1], run[:]) {
				kind = "ROUND"
			} else if len(fields) == 13 && equalBytes(fields[1], run[:]) {
				kind = "EXOGENOUS"
			} else {
				return out, errors.New("unknown pending input authority")
			}
		}
		found = true
		out = observatory.PendingDiagnostic{
			Authority: o2Ref(source.table, id, raw), Kind: kind, Eligibility: item.Eligibility,
		}
	}
	if !found {
		return out, errors.New("pending authority unavailable")
	}
	return out, nil
}

func (q *PrivilegedReader) operationalSources(ctx context.Context, run [32]byte,
	target *domain.EligibilityCoordinate) ([]observatory.SourceDiagnostic, error) {
	ledger, err := inputledger.Open(ctx, domain.ID(run), q.base.bundle, q.base.store)
	if err != nil {
		return nil, err
	}
	sources, _, closures, _, _, loadedRun := ledger.Snapshot()
	if loadedRun != domain.ID(run) {
		return nil, errors.New("operational source ledger run mismatch")
	}
	latest := map[string]inputledger.StoredClosure{}
	// Snapshot orders closures by authenticated ingress sequence. The last
	// record for each source is the latest closure, not latest rowid.
	for _, closure := range closures {
		latest[closure.Receipt.SourceID] = closure
	}
	out := make([]observatory.SourceDiagnostic, 0, len(sources))
	for _, name := range sources {
		d := observatory.SourceDiagnostic{Source: name, Coverage: "NO_TARGET"}
		if target != nil {
			d.Coverage = "GAP"
		}
		if c, ok := latest[name]; ok {
			f := c.Fields
			if len(f) != 7 || !equalBytes(f[0], c.ID[:]) || !equalBytes(f[1], run[:]) || f[2] != name {
				return nil, errors.New("operational closure scope mismatch")
			}
			coord, ok := f[4].([]any)
			final, finalOK := f[5].(bool)
			if !ok || len(coord) != 2 || !finalOK {
				return nil, errors.New("invalid closure coverage coordinate")
			}
			instant, err := o2Instant(coord[0])
			if err != nil {
				return nil, err
			}
			ordinal, err := o2Uint(coord[1])
			if err != nil {
				return nil, err
			}
			at := domain.EligibilityCoordinate{NotBeforeInstant: domain.SimulationInstant(instant), NotBeforeCycleOrdinal: domain.CycleOrdinal(ordinal)}
			raw, err := q.raw(ctx, "input_ledger", [32]byte(c.ID))
			if err != nil || !bytes.Equal(raw, c.Canonical) {
				return nil, errors.New("latest closure differs from verified input ledger")
			}
			ref := o2Ref("input_ledger", [32]byte(c.ID), raw)
			d.LatestClosure, d.ClosedThrough, d.Final = &ref, &at, final
			if final {
				d.Coverage = "FINAL"
			} else if target != nil && at.Compare(*target) >= 0 {
				d.Coverage = "COVERED"
			}
		}
		out = append(out, d)
	}
	return out, nil
}

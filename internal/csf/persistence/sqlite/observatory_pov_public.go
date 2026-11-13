package sqlite

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/contextboundary"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/observatory"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

// POVReader is the only actor-facing Observatory reader. Its identity and
// causal cutoff are fixed by a persisted SlotDispatch; they cannot be replaced
// by a caller at read time. Its SQL connection remains SQLite mode=ro.
type POVReader struct {
	base *ObservatoryReader
	inbox *ActorInbox
	run [32]byte
	actor [2]string
	through uint64
}

var _ observatory.POVQueries = (*POVReader)(nil)

// OpenPOVObservatory is a capability binding in the single-operator local
// host; a SlotDispatch ID is not a user authentication secret. Only the
// existing durable holder/cycle/round/authorization proof is accepted.
// This must NOT be exposed as an arbitrary-actor HTTP endpoint.
func OpenPOVObservatory(
	ctx context.Context, path string, bundle *policy.Bundle,
	run, dispatchID [32]byte, actor [2]string,
) (*POVReader, error) {
	if actor[0] == "" || actor[1] == "" {
		return nil, errors.New("POV holder namespace and actor ID required")
	}
	base, err := OpenObservatory(ctx, path, bundle)
	if err != nil {
		return nil, err
	}
	fail := func(err error) (*POVReader, error) {
		_ = base.Close()
		return nil, err
	}
	if _, err := base.ReadRun(ctx, run); err != nil {
		return fail(fmt.Errorf("POV run not authenticated: %w", err))
	}
	inbox, err := base.store.BindActorInbox(ctx, dispatchID, actor)
	if err != nil {
		return fail(fmt.Errorf("POV actor dispatch not authenticated: %w", err))
	}
	if inbox.run != run || inbox.actor != actor {
		return fail(errors.New("POV dispatch is bound to a different run/actor"))
	}
	return &POVReader{base: base, inbox: inbox, run: run, actor: actor, through: inbox.throughRevision}, nil
}

func (q *POVReader) Close() error {
	if q == nil || q.base == nil {
		return errors.New("POV reader is not initialized")
	}
	return q.base.Close()
}

// ReadPOV first authenticates the complete current run by normative replay,
// then delegates epistemic access to the same receiver-scoped ActorInbox and
// contextboundary.Build used by cognition. Context boundary rejects unpinned
// channels, missing receipts, future evidence and raw EventStore provenance.
// The returned bytes are clones of *delivered* Observation/KnowledgeInput
// only. A concurrent write causes fail-closed rather than a mixed result.
func (q *POVReader) ReadPOV(ctx context.Context) (observatory.POVView, error) {
	var zero observatory.POVView
	if q == nil || q.base == nil || q.inbox == nil {
		return zero, errors.New("POV reader is not initialized")
	}
	version, err := q.base.dataVersion(ctx)
	if err != nil {
		return zero, err
	}
	state, err := q.base.ReadRun(ctx, q.run)
	if err != nil {
		return zero, fmt.Errorf("POV source replay: %w", err)
	}
	if uint64(state.Clock.Revision) < q.through {
		return zero, errors.New("POV capability references a future revision")
	}
	view, err := contextboundary.Build(ctx, q.inbox, q.actor)
	if err != nil {
		return zero, fmt.Errorf("POV addressed evidence verification: %w", err)
	}
	if view.Actor != q.actor {
		return zero, errors.New("POV context boundary returned a different actor")
	}
	out := observatory.POVView{
		Run: q.run, Actor: q.actor, ThroughRevision: q.through,
		Entries: make([]observatory.AddressedEvidence, 0, len(view.Inbox)),
	}
	for _, entry := range view.Inbox {
		value, err := codec.StrictDecodePayload(entry.Input.Canonical)
		if err != nil {
			return zero, err
		}
		fields, ok := value.([]any)
		if !ok || len(fields) != 11 {
			return zero, errors.New("POV knowledge input is not a registered receipt")
		}
		channel, ok := fields[6].(string)
		if !ok || (channel != "school.round.lifecycle.v1" && channel != "school.message.v1") {
			return zero, errors.New("POV evidence has an unregistered channel")
		}
		out.Entries = append(out.Entries, observatory.AddressedEvidence{
			ObservationID: entry.Observation.Identity,
			KnowledgeInputID: entry.Input.Identity,
			ObservationCanonical: bytes.Clone(entry.Observation.Canonical),
			KnowledgeCanonical: bytes.Clone(entry.Input.Canonical),
			ReceivedAt: entry.ReceivedAt,
			Channel: channel,
		})
	}
	// O1 already establishes immutable replay proof for the first snapshot.
	// Version and state comparison detect concurrent writes by another handle.
	final, err := q.base.loadReadRun(ctx, q.run)
	if err != nil {
		return zero, err
	}
	now, err := q.base.dataVersion(ctx)
	if err != nil {
		return zero, err
	}
	if version != now || !reflect.DeepEqual(state, final) {
		return zero, errors.New("POV authorities changed during verified query")
	}
	return out, nil
}

// PublicReader is separate from POVReader and PrivilegedReader. The current
// V1 schema has an addressed evidence route, but no authenticated public
// publication channel or fanout. A public Event marker alone authorizes no
// knowledge: return no publications rather than silently leaking world truth.
type PublicReader struct {
	base *ObservatoryReader
}

var _ observatory.PublicQueries = (*PublicReader)(nil)

func OpenPublicObservatory(ctx context.Context, path string, bundle *policy.Bundle) (*PublicReader, error) {
	base, err := OpenObservatory(ctx, path, bundle)
	if err != nil {
		return nil, err
	}
	return &PublicReader{base: base}, nil
}

func (q *PublicReader) Close() error {
	if q == nil || q.base == nil {
		return errors.New("public Observatory reader is not initialized")
	}
	return q.base.Close()
}

func (q *PublicReader) ReadPublic(ctx context.Context, run [32]byte) (observatory.PublicView, error) {
	var zero observatory.PublicView
	if q == nil || q.base == nil {
		return zero, errors.New("public Observatory reader is not initialized")
	}
	version, err := q.base.dataVersion(ctx)
	if err != nil {
		return zero, err
	}
	state, err := q.base.ReadRun(ctx, run)
	if err != nil {
		return zero, fmt.Errorf("public run verification: %w", err)
	}
	// A future schema that installs a publisher must explicitly authorize
	// publication and extend the public projection before it can be used.
	// Never use EventStore, publicity bits, routes for private recipients,
	// actor context, decision journal or privileged Observatory as fallback.
	var candidate int
	if err := q.base.store.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name IN ('publications','public_routes','public_dispatches')").Scan(&candidate); err != nil {
		return zero, err
	}
	if candidate != 0 {
		return zero, errors.New("unsupported publication authority requires a registered public resolver")
	}
	final, err := q.base.loadReadRun(ctx, run)
	if err != nil {
		return zero, err
	}
	now, err := q.base.dataVersion(ctx)
	if err != nil {
		return zero, err
	}
	if version != now || !reflect.DeepEqual(state, final) {
		return zero, errors.New("public source changed during verified query")
	}
	return observatory.PublicView{
		Run: run,
		PublicationStatus: observatory.NoPublicRoute,
		Publications: make([]observatory.PublicPublication, 0),
	}, nil
}

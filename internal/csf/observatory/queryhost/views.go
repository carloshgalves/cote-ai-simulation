package queryhost

import (
	"bytes"
	"context"
	"reflect"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/observatory"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

// POVReader never holds the human audit authority. Its capability pins the
// full immutable dispatch and its holder/cutoff, rechecked on every read.
type POVReader struct {
	base      *ObservatoryReader
	addressed POVAuthority
	dispatch  [32]byte
	binding   Binding
}

var _ observatory.POVQueries = (*POVReader)(nil)

func restrictedViewError(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return observatory.ErrViewUnavailable
}

func NewPOV(ctx context.Context, host POVAuthority, bundle *policy.Bundle, run, dispatch [32]byte, actor [2]string) (*POVReader, error) {
	base := New(host, bundle)
	fail := func() (*POVReader, error) {
		_ = base.Close()
		return nil, restrictedViewError(ctx)
	}
	if actor[0] == "" || actor[1] == "" {
		return fail()
	}
	version, err := base.dataVersion(ctx)
	if err != nil {
		return fail()
	}
	if _, err := base.ReadRun(ctx, run); err != nil {
		return fail()
	}
	binding, err := host.Bind(ctx, dispatch, actor)
	if err != nil || binding.Run != run || binding.Actor != actor {
		return fail()
	}
	now, err := base.dataVersion(ctx)
	if err != nil || now != version {
		return fail()
	}
	return &POVReader{base: base, addressed: host, dispatch: dispatch, binding: binding}, nil
}

func (q *POVReader) Close() error {
	if q == nil || q.base == nil {
		return observatory.ErrViewUnavailable
	}
	if err := q.base.Close(); err != nil {
		return observatory.ErrViewUnavailable
	}
	return nil
}

func (q *POVReader) ReadPOV(ctx context.Context) (observatory.POVView, error) {
	var zero observatory.POVView
	if q == nil || q.base == nil || q.addressed == nil {
		return zero, restrictedViewError(ctx)
	}
	version, err := q.base.dataVersion(ctx)
	if err != nil {
		return zero, restrictedViewError(ctx)
	}
	state, err := q.base.ReadRun(ctx, q.binding.Run)
	if err != nil || uint64(state.Clock.Revision) < q.binding.ThroughRevision {
		return zero, restrictedViewError(ctx)
	}
	binding, entries, err := q.addressed.ReadAddressed(ctx, q.dispatch, q.binding.Actor)
	if err != nil || binding != q.binding {
		return zero, restrictedViewError(ctx)
	}
	out := observatory.POVView{Run: binding.Run, Actor: binding.Actor, ThroughRevision: binding.ThroughRevision,
		Entries: make([]observatory.AddressedEvidence, 0, len(entries))}
	for _, entry := range entries {
		if entry.Channel != "school.round.lifecycle.v1" && entry.Channel != "school.message.v1" {
			return zero, restrictedViewError(ctx)
		}
		entry.ObservationCanonical = bytes.Clone(entry.ObservationCanonical)
		entry.KnowledgeCanonical = bytes.Clone(entry.KnowledgeCanonical)
		out.Entries = append(out.Entries, entry)
	}
	final, err := q.base.host.LoadRun(ctx, binding.Run)
	if err != nil {
		return zero, restrictedViewError(ctx)
	}
	now, err := q.base.dataVersion(ctx)
	if err != nil || version != now || !reflect.DeepEqual(state, final) {
		return zero, restrictedViewError(ctx)
	}
	return out, nil
}

// PublicReader receives no raw audit or addressed-evidence capability.
type PublicReader struct {
	base   *ObservatoryReader
	public PublicAuthority
}

var _ observatory.PublicQueries = (*PublicReader)(nil)

func NewPublic(host PublicAuthority, bundle *policy.Bundle) *PublicReader {
	return &PublicReader{base: New(host, bundle), public: host}
}
func (q *PublicReader) Close() error {
	if q == nil || q.base == nil {
		return observatory.ErrViewUnavailable
	}
	if err := q.base.Close(); err != nil {
		return observatory.ErrViewUnavailable
	}
	return nil
}
func (q *PublicReader) ReadPublic(ctx context.Context, run [32]byte) (observatory.PublicView, error) {
	var zero observatory.PublicView
	if q == nil || q.base == nil || q.public == nil {
		return zero, restrictedViewError(ctx)
	}
	version, err := q.base.dataVersion(ctx)
	if err != nil {
		return zero, restrictedViewError(ctx)
	}
	state, err := q.base.ReadRun(ctx, run)
	if err != nil {
		return zero, restrictedViewError(ctx)
	}
	present, err := q.public.PublicationAuthorityPresent(ctx)
	if err != nil || present {
		return zero, restrictedViewError(ctx)
	}
	final, err := q.base.host.LoadRun(ctx, run)
	if err != nil {
		return zero, restrictedViewError(ctx)
	}
	now, err := q.base.dataVersion(ctx)
	if err != nil || version != now || !reflect.DeepEqual(state, final) {
		return zero, restrictedViewError(ctx)
	}
	return observatory.PublicView{
		Run: run, PublicationStatus: observatory.NoPublicRoute,
		Publications: make([]observatory.PublicPublication, 0),
	}, nil
}

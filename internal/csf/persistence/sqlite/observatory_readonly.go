package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/clock"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/contextboundary"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/inputledger"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/observatory"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/observatory/queryhost"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
	"github.com/carloshgalves/cote-ai-simulation/internal/localdbguard"
	"github.com/carloshgalves/cote-ai-simulation/internal/readonlysql"
	modernsqlite "modernc.org/sqlite"
)

// The engine composition root alone owns the legacy normative verifier.
// All queries receive a narrow capability over a separately opened,
// mode=ro, query_only, driver-guarded, inode-pinned connection.
type observatoryAuthority struct {
	store   *Store
	pathPin *localdbguard.Pin
}

var (
	_ queryhost.AuditAuthority  = (*observatoryAuthority)(nil)
	_ queryhost.CoreAuthority   = coreAuthority{}
	_ queryhost.POVAuthority    = actorAuthority{}
	_ queryhost.PublicAuthority = publicAuthority{}
)

// Nominal wrappers prevent dynamic type assertions from recovering the
// audit capability (including raw SQL) from a POV/public authority. Only
// the engine composition root has the unexported full host type.
type coreAuthority struct{ host *observatoryAuthority }

func (a coreAuthority) Close() error {
	return a.host.Close()
}
func (a coreAuthority) DataVersion(ctx context.Context) (int64, error) {
	return a.host.DataVersion(ctx)
}
func (a coreAuthority) LoadRun(ctx context.Context, run [32]byte) (observatory.RunState, error) {
	return a.host.LoadRun(ctx, run)
}
func (a coreAuthority) Replay(ctx context.Context, run [32]byte, bundle *policy.Bundle) (clock.State, error) {
	return a.host.Replay(ctx, run, bundle)
}

type actorAuthority struct{ coreAuthority }

func (a actorAuthority) Bind(ctx context.Context, dispatch [32]byte, actor [2]string) (queryhost.Binding, error) {
	return a.host.Bind(ctx, dispatch, actor)
}
func (a actorAuthority) ReadAddressed(ctx context.Context, dispatch [32]byte, actor [2]string) (queryhost.Binding, []observatory.AddressedEvidence, error) {
	return a.host.ReadAddressed(ctx, dispatch, actor)
}

type publicAuthority struct{ coreAuthority }

func (a publicAuthority) PublicationAuthorityPresent(ctx context.Context) (bool, error) {
	return a.host.PublicationAuthorityPresent(ctx)
}

type ObservatoryReader = queryhost.ObservatoryReader

func OpenObservatory(ctx context.Context, path string, bundle *policy.Bundle) (*ObservatoryReader, error) {
	host, err := openObservatoryAuthority(ctx, path, bundle)
	if err != nil {
		return nil, err
	}
	return queryhost.New(coreAuthority{host: host}, bundle), nil
}

func openObservatoryAuthority(ctx context.Context, path string, bundle *policy.Bundle) (*observatoryAuthority, error) {
	if bundle == nil {
		return nil, observatory.ErrInvalidPolicy
	}
	if err := bundle.VerifyInstalledBundle(); err != nil {
		return nil, fmt.Errorf("%w: %v", observatory.ErrInvalidPolicy, err)
	}
	pin, err := localdbguard.OpenPin(path)
	if err != nil {
		return nil, err
	}
	uri := &url.URL{Scheme: "file", Path: filepath.ToSlash(pin.Path())}
	q := url.Values{}
	// DO NOT use immutable=1: it ignores the live WAL and can silently
	// return a stale run. mode=ro is enforced by the SQLite pager.
	q.Set("mode", "ro")
	q.Add("_pragma", "query_only(ON)")
	q.Add("_pragma", "synchronous(FULL)")
	q.Add("_pragma", "foreign_keys(ON)")
	uri.RawQuery = q.Encode()
	connector, err := modernsqlite.NewConnector(uri.String())
	if err != nil {
		return nil, err
	}
	db := sql.OpenDB(readonlysql.Wrap(connector))
	// One persistent connection stabilizes PRAGMA data_version checks, which
	// are connection-relative and detect concurrent writes by other handles.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	s := &Store{db: db}
	failed := func(err error) (*observatoryAuthority, error) { _ = db.Close(); return nil, err }
	if err := db.PingContext(ctx); err != nil {
		return failed(fmt.Errorf("open existing CSF database in read-only mode: %w", err))
	}
	if err := s.verifyRuntime(ctx); err != nil {
		return failed(err)
	}
	var only int
	if err := db.QueryRowContext(ctx, "PRAGMA query_only").Scan(&only); err != nil {
		return failed(err)
	}
	if only != 1 {
		return failed(errors.New("observatory SQLite connection is not query_only"))
	}
	// Deliberately no Store.createSchema and no writable Store export.
	if err := pin.Check(); err != nil {
		return failed(err)
	}
	return &observatoryAuthority{store: s, pathPin: pin}, nil
}

func (r *observatoryAuthority) Close() error {
	if r == nil || r.store == nil {
		return errors.New("nil observatory reader")
	}
	return r.store.Close()
}

func (r *observatoryAuthority) DataVersion(ctx context.Context) (int64, error) {
	if err := r.pathPin.Check(); err != nil {
		return 0, fmt.Errorf("%w: %v", observatory.ErrSourceChanged, err)
	}
	var version int64
	if err := r.store.db.QueryRowContext(ctx, "PRAGMA data_version").Scan(&version); err != nil {
		return 0, err
	}
	if err := r.pathPin.Check(); err != nil {
		return 0, fmt.Errorf("%w: %v", observatory.ErrSourceChanged, err)
	}
	return version, nil
}

func (r *observatoryAuthority) LoadRun(ctx context.Context, run [32]byte) (observatory.RunState, error) {
	out := observatory.RunState{Run: run}
	tx, err := r.store.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return observatory.RunState{}, err
	}
	defer tx.Rollback()
	// The causal SQLite file holds one world. Never join a status from one
	// run with a world revision whose identity belongs to another genesis.
	var count int
	if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM genesis_records").Scan(&count); err != nil {
		return observatory.RunState{}, err
	}
	if count != 1 {
		return observatory.RunState{}, errors.New("observatory requires exactly one causal genesis")
	}
	var clockRaw []byte
	if err := tx.QueryRowContext(ctx, "SELECT canonical FROM clock_states WHERE run_id=?", run[:]).Scan(&clockRaw); err != nil {
		return observatory.RunState{}, fmt.Errorf("observatory clock authority absent: %w", err)
	}
	out.Clock, err = clock.Decode(clockRaw)
	if err != nil {
		return observatory.RunState{}, err
	}
	var worldHash []byte
	if err := tx.QueryRowContext(ctx, "SELECT state_hash FROM world_revisions WHERE revision=?", uint64(out.Clock.Revision)).Scan(&worldHash); err != nil {
		return observatory.RunState{}, fmt.Errorf("observatory world revision absent: %w", err)
	}
	if len(worldHash) != 32 {
		return observatory.RunState{}, errors.New("invalid authoritative world hash")
	}
	copy(out.WorldHash[:], worldHash)
	ctl, err := cycleControl(ctx, tx, run)
	if err != nil {
		return observatory.RunState{}, err
	}
	out.Control, err = observatoryControl(ctl)
	if err != nil {
		return observatory.RunState{}, err
	}
	if err := tx.Commit(); err != nil {
		return observatory.RunState{}, err
	}
	return out, nil
}

func observatoryControl(fields []any) (observatory.CycleControl, error) {
	out := observatory.CycleControl{Status: observatory.IDLE}
	if fields == nil {
		return out, nil
	}
	if len(fields) != 7 {
		return out, errors.New("invalid CycleControlState arity")
	}
	kind, ok := fields[0].(uint64)
	if !ok {
		return out, errors.New("invalid CycleControlState kind")
	}
	switch kind {
	case 0:
		out.Status = observatory.IDLE
	case 1:
		out.Status = observatory.ATTEMPT_IN_FLIGHT
	case 2:
		out.Status = observatory.RETRY_AUTHORIZED
	case 3:
		out.Status = observatory.HALTED_ON_ABORT
	default:
		return out, errors.New("unsupported CycleControlState kind")
	}
	next, ok := fields[6].(uint64)
	if !ok {
		return out, errors.New("invalid CycleControlState next attempt")
	}
	out.NextAttempt = next
	optional := func(v any) (any, bool, error) {
		arr, ok := v.([]any)
		if !ok {
			return nil, false, errors.New("invalid optional CycleControlState field")
		}
		if len(arr) == 1 && arr[0] == uint64(0) {
			return nil, false, nil
		}
		if len(arr) == 2 && arr[0] == uint64(1) {
			return arr[1], true, nil
		}
		return nil, false, errors.New("unknown optional CycleControlState variant")
	}
	cycle, hasCycle, err := optional(fields[1])
	if err != nil {
		return out, err
	}
	ordinal, hasOrdinal, err := optional(fields[2])
	if err != nil {
		return out, err
	}
	if hasCycle != hasOrdinal || (kind != 0 && !hasCycle) || (kind == 0 && hasCycle) {
		return out, errors.New("missing/forged CycleControlState cycle or ordinal")
	}
	if hasCycle {
		bytes, ok := cycle.([]byte)
		if !ok || len(bytes) != 32 {
			return out, errors.New("invalid CycleControlState cycle id")
		}
		var id [32]byte
		copy(id[:], bytes)
		out.CycleID = &id
		attempt, ok := ordinal.(uint64)
		if !ok {
			return out, errors.New("invalid CycleControlState attempt")
		}
		out.Attempt = &attempt
	}
	return out, nil
}

func (r *observatoryAuthority) Replay(ctx context.Context, run [32]byte, bundle *policy.Bundle) (clock.State, error) {
	proof, err := r.store.ReplayClock(ctx, run, bundle)
	return proof.ReconstructedClock, err
}

func (r *observatoryAuthority) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return r.store.db.QueryContext(ctx, query, args...)
}
func (r *observatoryAuthority) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return r.store.db.QueryRowContext(ctx, query, args...)
}
func (r *observatoryAuthority) Genesis(ctx context.Context, run [32]byte, bundle *policy.Bundle) (observatory.GenesisAuthority, error) {
	root, err := r.store.ValidatedGenesis(ctx, run, bundle)
	if err != nil {
		return observatory.GenesisAuthority{}, err
	}
	return observatory.GenesisAuthority{
		Run: run, GenesisID: [32]byte(root.ID), Digest: [32]byte(root.Digest),
		ContentHash: [32]byte(identity.OpaqueContentHash(root.CanonicalBytes)),
	}, nil
}
func (r *observatoryAuthority) PendingClock(ctx context.Context, run [32]byte) ([]clock.Pending, error) {
	return r.store.PendingClock(ctx, run)
}
func (r *observatoryAuthority) CheckPerceptionBarrier(ctx context.Context, run [32]byte, revision uint64) error {
	return r.store.CheckPerceptionBarrier(ctx, run, revision)
}
func (r *observatoryAuthority) LoadGenesis(ctx context.Context, run [32]byte) ([]byte, error) {
	return r.store.LoadGenesis(ctx, run)
}
func (r *observatoryAuthority) SourcePrefix(ctx context.Context, run [32]byte, bundle *policy.Bundle) (queryhost.SourcePrefix, error) {
	var out queryhost.SourcePrefix
	ledger, err := inputledger.RestoreReadPrefix(ctx, domain.ID(run), bundle, r.store)
	if err != nil {
		return out, err
	}
	out.Run, out.Sources = [32]byte(ledger.Run), ledger.Sources
	for _, c := range ledger.Closures {
		out.Closures = append(out.Closures, queryhost.StoredClosure{
			ID: [32]byte(c.ID), Source: c.Receipt.SourceID, Fields: c.Fields, Canonical: c.Canonical,
		})
	}
	return out, nil
}

func (r *observatoryAuthority) bind(ctx context.Context, dispatch [32]byte, actor [2]string) (*ActorInbox, queryhost.Binding, error) {
	var out queryhost.Binding
	inbox, err := r.store.BindActorInbox(ctx, dispatch, actor)
	if err != nil {
		return nil, out, err
	}
	var raw []byte
	if err := r.store.db.QueryRowContext(ctx, "SELECT canonical FROM input_ledger WHERE id=?", dispatch[:]).Scan(&raw); err != nil {
		return nil, out, err
	}
	return inbox, queryhost.Binding{Run: inbox.run, Actor: inbox.actor, ThroughRevision: inbox.throughRevision,
		DispatchHash: [32]byte(identity.OpaqueContentHash(raw))}, nil
}
func (r *observatoryAuthority) Bind(ctx context.Context, dispatch [32]byte, actor [2]string) (queryhost.Binding, error) {
	_, binding, err := r.bind(ctx, dispatch, actor)
	return binding, err
}
func (r *observatoryAuthority) ReadAddressed(ctx context.Context, dispatch [32]byte, actor [2]string) (queryhost.Binding, []observatory.AddressedEvidence, error) {
	inbox, binding, err := r.bind(ctx, dispatch, actor)
	if err != nil {
		return queryhost.Binding{}, nil, err
	}
	view, err := contextboundary.Build(ctx, inbox, actor)
	if err != nil {
		return queryhost.Binding{}, nil, err
	}
	if view.Actor != actor {
		return queryhost.Binding{}, nil, errors.New("context boundary returned a different actor")
	}
	entries := make([]observatory.AddressedEvidence, 0, len(view.Inbox))
	for _, entry := range view.Inbox {
		value, err := codec.StrictDecodePayload(entry.Input.Canonical)
		if err != nil {
			return queryhost.Binding{}, nil, err
		}
		fields, ok := value.([]any)
		if !ok || len(fields) != 11 {
			return queryhost.Binding{}, nil, errors.New("invalid knowledge input")
		}
		channel, ok := fields[6].(string)
		if !ok || (channel != "school.round.lifecycle.v1" && channel != "school.message.v1") {
			return queryhost.Binding{}, nil, errors.New("unregistered evidence channel")
		}
		entries = append(entries, observatory.AddressedEvidence{
			ObservationID:        entry.Observation.Identity,
			KnowledgeInputID:     entry.Input.Identity,
			ObservationCanonical: bytes.Clone(entry.Observation.Canonical),
			KnowledgeCanonical:   bytes.Clone(entry.Input.Canonical),
			ReceivedAt:           entry.ReceivedAt,
			Channel:              channel,
		})
	}
	return binding, entries, nil
}

func (r *observatoryAuthority) PublicationAuthorityPresent(ctx context.Context) (bool, error) {
	var count int
	err := r.store.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM sqlite_master WHERE type IN ('table','view') AND lower(name) IN ('publications','public_routes','public_dispatches')").Scan(&count)
	return count != 0, err
}

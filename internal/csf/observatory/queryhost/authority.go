// Package queryhost implements the real Observatory projections behind
// segregated, injected read capabilities. It must not import any causal
// write port, Store, owner, commit or sink package; the engine supplies
// normative replay proof through the composition root.
package queryhost

import (
	"bytes"
	"context"
	"database/sql"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/clock"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/observatory"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

type CoreAuthority interface {
	Close() error
	DataVersion(context.Context) (int64, error)
	LoadRun(context.Context, [32]byte) (observatory.RunState, error)
	Replay(context.Context, [32]byte, *policy.Bundle) (clock.State, error)
}

// The human-only capability owns the raw authority queries; actors and the
// public receive separate narrower interfaces, never an AuditAuthority.
type AuditAuthority interface {
	CoreAuthority
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
	Genesis(context.Context, [32]byte, *policy.Bundle) (observatory.GenesisAuthority, error)
	PendingClock(context.Context, [32]byte) ([]clock.Pending, error)
	CheckPerceptionBarrier(context.Context, [32]byte, uint64) error
	LoadGenesis(context.Context, [32]byte) ([]byte, error)
	SourcePrefix(context.Context, [32]byte, *policy.Bundle) (SourcePrefix, error)
}

type StoredClosure struct {
	ID        [32]byte
	Source    string
	Fields    []any
	Canonical []byte
}
type SourcePrefix struct {
	Run      [32]byte
	Sources  []string
	Closures []StoredClosure
}

type Binding struct {
	Run             [32]byte
	Actor           [2]string
	ThroughRevision uint64
	DispatchHash    [32]byte
}
type POVAuthority interface {
	CoreAuthority
	Bind(context.Context, [32]byte, [2]string) (Binding, error)
	ReadAddressed(context.Context, [32]byte, [2]string) (Binding, []observatory.AddressedEvidence, error)
}
type PublicAuthority interface {
	CoreAuthority
	PublicationAuthorityPresent(context.Context) (bool, error)
}

func registeredDigest(tag, schema string, value any) (domain.Digest, error) {
	encoded, err := codec.CanonicalBytes(tag, schema, 1, value)
	if err != nil {
		return domain.Digest{}, err
	}
	return identity.OpaqueContentHash(encoded), nil
}
func sameCanonical(a, b any) bool {
	ab, err := codec.CanonicalPayload(a)
	if err != nil {
		return false
	}
	bb, err := codec.CanonicalPayload(b)
	return err == nil && bytes.Equal(ab, bb)
}
func equalBytes(v any, other []byte) bool {
	b, ok := v.([]byte)
	return ok && bytes.Equal(b, other)
}
func asSigned64(v any) (int64, bool) {
	switch n := v.(type) {
	case int64:
		return n, true
	case uint64:
		if n <= uint64(^uint64(0)>>1) {
			return int64(n), true
		}
	}
	return 0, false
}

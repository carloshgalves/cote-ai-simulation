package sqlite

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/clock"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/genesis"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/inputledger"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/observatory"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/observatory/queryhost"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

// A test-only wrapper around the REAL SQLite and replay authority. Hooks
// run after durable replay or prefix restoration; no timing sleeps, no
// fake data_version, no synthetic successful authority in the query.
type f5FenceHost struct {
	*observatoryAuthority
	afterReplay func() error
	afterPrefix func() error
}

func (h *f5FenceHost) Replay(ctx context.Context, run [32]byte, bundle *policy.Bundle) (clock.State, error) {
	state, err := h.observatoryAuthority.Replay(ctx, run, bundle)
	if err == nil && h.afterReplay != nil {
		hook := h.afterReplay
		h.afterReplay = nil
		err = hook()
	}
	return state, err
}

func (h *f5FenceHost) SourcePrefix(ctx context.Context, run [32]byte, bundle *policy.Bundle) (queryhost.SourcePrefix, error) {
	prefix, err := h.observatoryAuthority.SourcePrefix(ctx, run, bundle)
	if err == nil && h.afterPrefix != nil {
		hook := h.afterPrefix
		h.afterPrefix = nil
		err = hook()
	}
	return prefix, err
}

func f5Genesis(t *testing.T, bundle *policy.Bundle) *genesis.Genesis {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(bundle.Directory, "fixtures.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixtures struct {
		Semantic []struct {
			CaseID             string `json:"case_id"`
			GenesisEnvelopeHex string `json:"genesis_envelope_hex"`
		} `json:"semantic"`
	}
	if err := json.Unmarshal(raw, &fixtures); err != nil {
		t.Fatal(err)
	}
	for _, item := range fixtures.Semantic {
		if item.CaseID != "genesis.pinning" {
			continue
		}
		encoded, err := hex.DecodeString(item.GenesisEnvelopeHex)
		if err != nil {
			t.Fatal(err)
		}
		env, err := codec.StrictDecodeEnvelope(encoded)
		if err != nil {
			t.Fatal(err)
		}
		fields := env.Value.([]any)
		fields[15] = []any{[]any{"cote.csf.schema.snapshot-pending-state", uint64(1)}}
		state := make([]any, 10)
		for i := range state {
			state[i] = []any{}
		}
		stateBytes, err := codec.CanonicalBytes("cote.csf.snapshot.pending-state", "cote.csf.schema.snapshot-pending-state", 1, state)
		if err != nil {
			t.Fatal(err)
		}
		digest := identity.OpaqueContentHash(stateBytes)
		fields[16] = []any{"cote.csf.schema.snapshot-pending-state", uint64(1), stateBytes, digest[:]}
		encoded, err = codec.CanonicalBytes("cote.csf.digest.genesis-manifest", env.SchemaID, env.SchemaVersion, fields)
		if err != nil {
			t.Fatal(err)
		}
		g, err := genesis.ValidateCanonical(encoded, bundle)
		if err != nil {
			t.Fatal(err)
		}
		return g
	}
	t.Fatal("missing normative Genesis fixture")
	return nil
}

// A legal writer advances the source-closure prefix at an exact point in
// an open query. The logical clock, world hash and cycle status do not
// change. Matching first/last RunState alone would falsely attest output.
func TestF5LiveWriterVersionFence(t *testing.T) {
	ctx := context.Background()
	bundle, err := policy.Load("../../../../docs/architecture/canonical-codec-v1-bundle")
	if err != nil {
		t.Fatal(err)
	}
	g := f5Genesis(t, bundle)
	path := filepath.Join(t.TempDir(), "live.db")
	writer, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	if err := writer.PersistGenesis(ctx, g, bundle); err != nil {
		t.Fatal(err)
	}
	if err := writer.BootstrapClock(ctx, g, bundle); err != nil {
		t.Fatal(err)
	}
	if err := writer.BootstrapGenesisTriggers(ctx, g, bundle, nil); err != nil {
		t.Fatal(err)
	}
	ledger, err := inputledger.New(ctx, g.RunID, bundle, writer)
	if err != nil {
		t.Fatal(err)
	}
	if err := ledger.DeclareSource(inputledger.Source{ID: "driver"}); err != nil {
		t.Fatal(err)
	}
	host, err := openObservatoryAuthority(ctx, path, bundle)
	if err != nil {
		t.Fatal(err)
	}
	defer host.Close()
	h := &f5FenceHost{observatoryAuthority: host}
	q := queryhost.New(h, bundle)
	audit := queryhost.NewPrivileged(h, bundle)
	run := [32]byte(g.RunID)
	initial, err := q.ReadRun(ctx, run)
	if err != nil {
		t.Fatal("positive real Genesis control failed", err)
	}
	original, err := audit.ReadOperationalReport(ctx, run)
	if err != nil {
		t.Fatal("positive real operational control failed", err)
	}
	seq, err := writer.NextIngressSequence(ctx, "driver")
	if err != nil {
		t.Fatal(err)
	}
	version, err := host.DataVersion(ctx)
	if err != nil {
		t.Fatal(err)
	}
	h.afterReplay = func() error {
		_, err := ledger.CloseSource(ctx, "driver", inputledger.Closure{ProducerKey: "f5-close-1",
			ClosedThrough: domain.EligibilityCoordinate{NotBeforeInstant: g.InitialInstant + 1}})
		return err
	}
	if got, err := q.ReadRun(ctx, run); !errors.Is(err, observatory.ErrSourceChanged) || !reflect.DeepEqual(got, observatory.RunState{}) {
		t.Fatal("query accepted interleaved writer with unchanged RunState", err)
	}
	after, err := q.ReadRun(ctx, run)
	if err != nil || !reflect.DeepEqual(initial, after) {
		t.Fatal("authorized closure unexpectedly changed temporal/world state", err)
	}
	newVersion, err := host.DataVersion(ctx)
	if err != nil || newVersion == version || h.afterReplay != nil {
		t.Fatal("writer interleaving did not change real connection-relative data_version", err)
	}
	first, err := audit.ReadOperationalReport(ctx, run)
	if err != nil || reflect.DeepEqual(first, original) || len(first.Sources) != 1 || first.Sources[0].LatestClosure == nil {
		t.Fatal("legitimate new closure not observed", err)
	}
	// Exercise the OUTER diagnostic fence after the verified privileged
	// report and after source-prefix restoration, not just ReadRun's fence.
	h.afterPrefix = func() error {
		_, err := ledger.CloseSource(ctx, "driver", inputledger.Closure{ProducerKey: "f5-close-2",
			ClosedThrough: domain.EligibilityCoordinate{NotBeforeInstant: g.InitialInstant + 2}, Final: true})
		return err
	}
	if got, err := audit.ReadOperationalReport(ctx, run); !errors.Is(err, observatory.ErrSourceChanged) || !reflect.DeepEqual(got, observatory.OperationalReport{}) {
		t.Fatal("outer operational fence accepted interleaved writer", err)
	}
	final, err := audit.ReadOperationalReport(ctx, run)
	if err != nil || len(final.Sources) != 1 || !final.Sources[0].Final ||
		final.Sources[0].Coverage != "FINAL" || !reflect.DeepEqual(final.State, initial) || h.afterPrefix != nil {
		t.Fatal("stable operational read did not observe final valid writer closure", err)
	}
	next, err := writer.NextIngressSequence(ctx, "driver")
	if err != nil || next != seq+2 {
		t.Fatal("unexpected mutations to authoritative cursor beyond the two explicit writer commits", err)
	}
}

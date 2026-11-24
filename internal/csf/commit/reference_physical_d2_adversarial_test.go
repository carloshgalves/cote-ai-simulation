//go:build linux

package commit

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/adapter/embodimentv1"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/admission"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/genesis"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/reducer"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/runbook"
	"github.com/carloshgalves/cote-ai-simulation/internal/physicalhost"
)

func referenceD2AdversarialPaths(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	owner := filepath.Join(root, "private")
	if err := os.Mkdir(owner, 0700); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(root, "causal.db"), filepath.Join(owner, "owner-b.db")
}

func referenceD2AssertPhysicalPrefix(t *testing.T, path, expectedRun string, epochCommits int) {
	t.Helper()
	ctx := context.Background()
	raw, err := referenceC4DecodeID(expectedRun)
	if err != nil {
		t.Fatal(err)
	}
	store, err := sqlite.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	state, err := store.LoadClock(ctx, raw)
	wantRevision := uint64(7 + epochCommits) // six C4, one explicit NoProposal, N physical
	if err != nil || uint64(state.Revision) != wantRevision || state.Instant != 35 ||
		state.LastOrdinal != int64(1+epochCommits) {
		t.Fatalf("failure changed causal clock: got=%+v want_revision=%d: %v", state, wantRevision, err)
	}
	_, _, err = store.LoadWorldState(ctx, wantRevision+1)
	if err == nil {
		t.Fatal("failed physical terminal published an extra WorldRevision")
	}
	world, _, err := store.LoadWorldState(ctx, wantRevision)
	if err != nil {
		t.Fatal(err)
	}
	stock, err := reducer.DecodeStock(world)
	if err != nil || stock.Available != 0 {
		t.Fatalf("failed physical terminal changed resource state: %v %v", stock, err)
	}
	db, err := openReferenceReadOnlyDB(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM committed_cycles").Scan(&count); err != nil || count != int(wantRevision) {
		t.Fatalf("failure created partial CycleCommit: %d expected=%d err=%v", count, wantRevision, err)
	}
	rows, err := db.QueryContext(ctx, "SELECT canonical FROM event_store")
	if err != nil {
		t.Fatal(err)
	}
	var physical int
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		event, err := codec.StrictDecodePayload(raw)
		if err != nil {
			rows.Close()
			t.Fatal(err)
		}
		fields, ok := event.([]any)
		if !ok || len(fields) != 19 {
			rows.Close()
			t.Fatal("invalid event canonical payload")
		}
		if fields[3] == "physical.advanced" {
			physical++
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		t.Fatal(err)
	}
	rows.Close()
	if physical != epochCommits {
		t.Fatalf("crash published partial physical event: got=%d expected=%d", physical, epochCommits)
	}
}

// Failure injection tests the *composed* causal run, not just the standalone
// physical transaction already covered exhaustively by TestPhysicalAtomic*.
// Ingress and the final source closure may be durable, but no failed batch
// may leak a partial WorldRevision, CycleCommit or physical.advanced Event.
func TestReferenceD2PhysicalAtomicFailuresLeaveEarlierNineCyclePrefixIntact(t *testing.T) {
	host, plans := referenceD2TestHost(t)
	for _, tc := range []struct {
		name  string
		epoch int
		step  uint8
		prior int
	}{
		{"epoch1-begin", 1, 1, 0}, {"epoch1-end", 1, 7, 0},
		{"epoch2-begin", 2, 1, 1}, {"epoch2-end", 2, 7, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, owner := referenceD2AdversarialPaths(t)
			proof, err := RunReferenceD2(context.Background(), db, owner, bundlePath,
				ReferenceD2Config{Host: host, EpochPlans: plans,
					FailPhysicalAtEpoch: tc.epoch, FailPhysicalAfterStep: tc.step},
				ReferenceOptions{})
			if err == nil {
				t.Fatal("injected physical failure accepted as success")
			}
			if proof.Status == "RUN_LOCAL_10D2" {
				t.Fatal("failure produced public terminal success")
			}
			// The Genesis carries one reproducible RunID, independently of
			// which phase first failed.
			g, err := buildReferenceGenesis(host.Bundle)
			if err != nil {
				t.Fatal(err)
			}
			referenceD2AssertPhysicalPrefix(t, db,
				func() string { return hexRun(g.RunID) }(), tc.prior)
		})
	}
}

func hexRun(run [32]byte) string {
	const hexchars = "0123456789abcdef"
	out := make([]byte, 64)
	for i, b := range run {
		out[2*i] = hexchars[b>>4]
		out[2*i+1] = hexchars[b&15]
	}
	return string(out)
}

func TestReferenceD2RejectsUnpinnedExecutableAndImportRoot(t *testing.T) {
	host, plans := referenceD2TestHost(t)
	for _, tc := range []struct {
		name  string
		alter func(*physicalhost.ProducerHost)
	}{
		{"tampered-executable", func(h *physicalhost.ProducerHost) {
			if err := os.Chmod(h.ExecutablePath, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(h.ExecutablePath, []byte("untrusted-python-image"), 0700); err != nil {
				t.Fatal(err)
			}
		}},
		{"foreign-import-root", func(h *physicalhost.ProducerHost) {
			h.ImportRoot = t.TempDir()
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Isolate per case; never corrupt another subtest's executable.
			local, localPlans := referenceD2TestHost(t)
			tc.alter(&local)
			db, owner := referenceD2AdversarialPaths(t)
			if _, err := RunReferenceD2(context.Background(), db, owner, bundlePath,
				ReferenceD2Config{Host: local, EpochPlans: localPlans}, ReferenceOptions{}); !errors.Is(err, runbook.ErrRejectedSource) || runbook.Error(err) != runbook.Rejected {
				t.Fatal("unregistered A1 executable or import root did not produce typed rejection")
			}
			g, err := buildReferenceGenesis(local.Bundle)
			if err != nil {
				t.Fatal(err)
			}
			referenceD2AssertPhysicalPrefix(t, db, hexRun(g.RunID), 0)
		})
	}
	_, _ = host, plans
}

func TestReferenceD2OpenPhysicalSourcePreventsC4Admission(t *testing.T) {
	host, plans := referenceD2TestHost(t)
	db, owner := referenceD2AdversarialPaths(t)
	if _, err := RunReferenceD2(context.Background(), db, owner, bundlePath,
		ReferenceD2Config{Host: host, EpochPlans: plans},
		ReferenceOptions{LeaveSourceOpen: true}); !errors.Is(err, admission.ErrWaitingForClosure) || runbook.Error(err) != runbook.WaitClosure {
		t.Fatal("C4 did not return typed source-closure wait")
	}
	store, err := sqlite.Open(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	g, err := buildReferenceGenesis(host.Bundle)
	if err != nil {
		t.Fatal(err)
	}
	state, err := store.LoadClock(context.Background(), [32]byte(g.RunID))
	if err != nil || state.Revision != 0 {
		t.Fatalf("open source silently committed a contested resource: %+v %v", state, err)
	}
}

func TestReferenceD2WrongEpochCannotReuseRealSignedA1Cut(t *testing.T) {
	host, plans := referenceD2TestHost(t)
	g, err := buildReferenceGenesis(host.Bundle)
	if err != nil {
		t.Fatal(err)
	}
	producer := host
	producer.GenesisCanonical = g.CanonicalBytes
	producer.Epoch = 1
	producer.Plan = plans[0]
	approved, err := physicalhost.RunSupervisedPhysical(context.Background(), producer)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := approved.CanonicalProof(); err != nil {
		t.Fatal(err)
	}
	duplicate := [2]embodimentv1.ApprovedCut{approved, approved}
	db, owner := referenceD2AdversarialPaths(t)
	if _, err := RunReferenceD2(context.Background(), db, owner, bundlePath,
		ReferenceD2Config{Host: host, EpochPlans: plans, frozenSupervisedCuts: &duplicate},
		ReferenceOptions{}); err == nil {
		t.Fatal("A1 epoch 1 signed cut replayed as second physical epoch")
	} else if errors.Is(err, context.Canceled) {
		t.Fatalf("wrong-epoch rejected by external cancellation, not signed source gate: %v", err)
	}
	referenceD2AssertPhysicalPrefix(t, db, hexRun(g.RunID), 1)
}

func TestReferenceD2RejectsAuthenticSignedCutFromForeignRunID(t *testing.T) {
	host, plans := referenceD2TestHost(t)
	g, err := buildReferenceGenesis(host.Bundle)
	if err != nil {
		t.Fatal(err)
	}
	env, err := codec.StrictDecodeEnvelope(g.CanonicalBytes)
	if err != nil {
		t.Fatal(err)
	}
	fields := append([]any(nil), env.Value.([]any)...)
	foreignRun := append([]byte(nil), fields[0].([]byte)...)
	foreignRun[0] ^= 0x4b
	fields[0] = foreignRun
	encoded, err := codec.CanonicalBytes(env.DomainTag, env.SchemaID, env.SchemaVersion, fields)
	if err != nil {
		t.Fatal(err)
	}
	foreignGenesis, err := genesis.ValidateCanonical(encoded, host.Bundle)
	if err != nil {
		t.Fatalf("could not derive genuine foreign registered Genesis: %v", err)
	}
	if foreignGenesis.RunID == g.RunID {
		t.Fatal("foreign RunID was not actually distinct")
	}
	producer := host
	producer.GenesisCanonical = foreignGenesis.CanonicalBytes
	producer.Epoch = 1
	producer.Plan = plans[0]
	alienCut, err := physicalhost.RunSupervisedPhysical(context.Background(), producer)
	if err != nil {
		t.Fatalf("foreign-run real A1 producer failed unexpectedly: %v", err)
	}
	if _, _, _, err := alienCut.CanonicalProof(); err != nil {
		t.Fatal("foreign A1 cut did not contain a real signed proof", err)
	}
	frozen := [2]embodimentv1.ApprovedCut{alienCut, alienCut}
	db, owner := referenceD2AdversarialPaths(t)
	if _, err := RunReferenceD2(context.Background(), db, owner, bundlePath,
		ReferenceD2Config{Host: host, EpochPlans: plans, frozenSupervisedCuts: &frozen},
		ReferenceOptions{}); err == nil {
		t.Fatal("authentically signed A1 physical cut from another RunID was admitted")
	}
	referenceD2AssertPhysicalPrefix(t, db, hexRun(g.RunID), 0)
}

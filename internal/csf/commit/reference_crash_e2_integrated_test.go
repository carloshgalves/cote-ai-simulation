//go:build linux

package commit

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/adapter/embodimentv1"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/admission"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/inputledger"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
	"github.com/carloshgalves/cote-ai-simulation/internal/physicalhost"
)

// Unlike an isolated physical fixture, E2's CROSS-acceptance uses the same
// B+C1+C2+C3+C4+NoProposal+two genuine A1 epochs as E1/D3. Both exogenous
// inputs were supervised and signed by Python exactly ONCE. Their signed
// bytes are frozen so independent continuous/crashed runs have identical
// authenticated sources even though Python's real wall_time varies per run.
func TestReferenceE2IntegratedC4D3CrashResumeByteExactAfterBothEpochs(t *testing.T) {
	ctx := context.Background()
	host, plans := referenceD2TestHost(t)
	g, err := buildReferenceGenesis(host.Bundle)
	if err != nil {
		t.Fatal(err)
	}
	host.GenesisCanonical = g.CanonicalBytes
	var cuts [2]embodimentv1.ApprovedCut
	for i := range cuts {
		producer := host
		producer.Epoch = uint64(i + 1)
		producer.Plan = plans[i]
		cuts[i], err = physicalhost.RunSupervisedPhysical(ctx, producer)
		if err != nil {
			t.Fatal("E2 original A1 supervision:", err)
		}
		if _, _, _, err := cuts[i].CanonicalProof(); err != nil {
			t.Fatal(err)
		}
	}
	host.GenesisCanonical = nil
	paths := func() (string, string) {
		root := t.TempDir()
		private := filepath.Join(root, "owner")
		if err := os.Mkdir(private, 0700); err != nil {
			t.Fatal(err)
		}
		return filepath.Join(root, "causal.db"), filepath.Join(private, "b.db")
	}
	capture := func(causal, owner string) referenceE1Snapshot {
		t.Helper()
		s, err := referenceE1Read(ctx, []referenceE1Location{
			{Name: "causal", Path: causal},
			{Name: "owner-a", Path: owner + ".actor-a"},
			{Name: "owner-b", Path: owner},
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := s.requireCausalAndOwners(); err != nil {
			t.Fatal(err)
		}
		return s
	}
	baselineDB, baselineOwner := paths()
	baseline, err := RunReferenceD3(ctx, baselineDB, baselineOwner, bundlePath,
		ReferenceD2Config{Host: host, EpochPlans: plans, frozenSupervisedCuts: &cuts}, ReferenceOptions{})
	if err != nil || baseline.Status != "RUN_LOCAL_10D3" {
		t.Fatalf("baseline D3: %v", err)
	}
	expected := capture(baselineDB, baselineOwner)
	manifest, err := host.Bundle.PhysicalManifestCanonical()
	if err != nil {
		t.Fatal(err)
	}
	// H2-F complete Cartesian fault grid: both original real signed
	// Python A1 epochs x every one of the seven Go atomic SQL steps.
	// Capture both epochs ONCE above; never resample Python wall_time here.
	cases := make([]struct {
		name  string
		epoch int
		step  uint8
	}, 0, 14)
	for epoch := 1; epoch <= 2; epoch++ {
		for step := uint8(1); step <= 7; step++ {
			cases = append(cases, struct {
				name  string
				epoch int
				step  uint8
			}{name: fmt.Sprintf("epoch%d-atomic-step%d", epoch, step), epoch: epoch, step: step})
		}
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dbPath, ownerPath := paths()
			bad := ReferenceD2Config{
				Host: host, EpochPlans: plans, frozenSupervisedCuts: &cuts,
				FailPhysicalAtEpoch: tc.epoch, FailPhysicalAfterStep: tc.step,
			}
			if _, err := RunReferenceD2(ctx, dbPath, ownerPath, bundlePath, bad, ReferenceOptions{}); err == nil {
				t.Fatal("E2 integrated SQL failpoint did not abort C4+D2 run")
			}
			run := domain.ID(g.RunID)
			// The C4 snapshot was recorded at revision six, before both
			// physical epochs. It is a valid immutable verification root.
			ro, err := openReferenceReadOnlyDB(dbPath)
			if err != nil {
				t.Fatal(err)
			}
			var snapRaw []byte
			if err := ro.QueryRowContext(ctx, "SELECT id FROM snapshots").Scan(&snapRaw); err != nil {
				ro.Close()
				t.Fatal(err)
			}
			if err := ro.Close(); err != nil {
				t.Fatal(err)
			}
			if len(snapRaw) != 32 {
				t.Fatal("E2 C4 snapshot was not durable")
			}
			var snapshot [32]byte
			copy(snapshot[:], snapRaw)
			partial, err := referenceE1Read(ctx, []referenceE1Location{{Name: "causal", Path: dbPath}})
			if err != nil {
				t.Fatal(err)
			}
			private := t.TempDir()
			path := filepath.Join(private, "public-physical-manifest.json")
			if err := os.WriteFile(path, manifest, 0600); err != nil {
				t.Fatal(err)
			}
			referenceE2CrashAfterTerminal(t, dbPath, path, [32]byte(run), snapshot)
			reopened, err := sqlite.Open(ctx, dbPath)
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close()
			loaded, err := inputledger.Open(ctx, run, host.Bundle, reopened)
			if err != nil {
				t.Fatal(err)
			}
			refs, err := policiesByRole(host.Bundle)
			if err != nil {
				t.Fatal(err)
			}
			policies := admission.Policies{
				AdmissionOrder: refs["admission-order"],
				CausalIdentity: refs["causal-identity"],
				Fence:          refs["admission-fence"], Rules: [][]any{},
			}
			coordinator := New(reopened, host.Bundle, policies)
			again, err := coordinator.ResumeRegisteredWorkV1(ctx, loaded, [32]byte(run), snapshot)
			if err != nil || again.Continued {
				t.Fatalf("E2 cross-run physical stage repeated after commit: %t %v", again.Continued, err)
			}
			state, err := reopened.LoadClock(ctx, [32]byte(run))
			if err != nil {
				t.Fatal(err)
			}
			expectedRevision := uint64(9)
			if tc.epoch == 1 {
				expectedRevision = 8
			}
			if uint64(state.Revision) != expectedRevision {
				t.Fatalf("E2 recovered wrong revision: got %d want %d", state.Revision, expectedRevision)
			}
			if _, err := reopened.ReplayClock(ctx, [32]byte(run), host.Bundle); err != nil {
				t.Fatal(err)
			}
			if tc.epoch == 1 {
				// Epoch 2 is a NEW, already-supervised signed input, not a
				// replay of epoch 1 or a newly executed Python producer.
				cycle, err := referenceC1Cycle(ctx, reopened, host.Bundle, run, state, 3)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := coordinator.StagePhysicalInput(ctx, loaded, cuts[1], cycle); err != nil {
					t.Fatal(err)
				}
				reg, err := host.Bundle.PhysicalRegistration()
				if err != nil {
					t.Fatal(err)
				}
				_, err = loaded.CloseSource(ctx, reg.SourceID, inputledger.Closure{
					ProducerKey: "physical.a1.final.epoch-2",
					ClosedThrough: domain.EligibilityCoordinate{
						NotBeforeInstant: cycle.Instant, NotBeforeCycleOrdinal: cycle.Ordinal,
					},
					Final: true,
				})
				if err != nil {
					t.Fatal(err)
				}
				fence, err := admission.NewCoordinator(admission.NewFenceLog(reopened)).
					AdmitThenEvaluate(ctx, host.Bundle, loaded, cycle, policies, nil)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := coordinator.CommitPhysical(ctx, loaded, cycle, fence, cuts[1]); err != nil {
					t.Fatal(err)
				}
			}
			final, err := reopened.LoadClock(ctx, [32]byte(run))
			if err != nil || uint64(final.Revision) != 9 || final.LastOrdinal != 3 {
				t.Fatalf("E2 recovery did not complete nine commits: %+v %v", final, err)
			}
			if _, err := reopened.ReplayClock(ctx, [32]byte(run), host.Bundle); err != nil {
				t.Fatal(err)
			}
			sealed, err := reopened.VerifySnapshotV1(ctx, [32]byte(run), snapshot, host.Bundle)
			if err != nil || sealed.TailCommits != 3 {
				t.Fatalf("E2 snapshot lost its sealed prefix: %v", err)
			}
			actual := capture(dbPath, ownerPath)
			if err := referenceE1Compare(expected, actual); err != nil {
				t.Fatalf("E2 C4+D3 resumed result differs from continuous input-identical baseline: %v", err)
			}
			if beforeCount := len(partial.Databases["causal"].Tables["committed_cycles"].Rows); beforeCount != 7 && beforeCount != 8 {
				t.Fatalf("E2 failed fault left invalid prefix of %d commits", beforeCount)
			}
		})
	}
	// Engine-privileged transcript bytes must never become CI artifacts.
	// Only counts, run shape and hashes are safe evidence for this oracle.
	if artifact := os.Getenv("CSF_E2_EVIDENCE_PATH"); artifact != "" {
		report := struct {
			Status                      string
			Reference                   string
			VerifiedBaselineCommits     int
			AuthenticatedA1Cuts         int
			IndependentCrashResumeCases int
			PhysicalSQLRollbackSteps    int
			SignerReexecution           bool
			CanonicalManifest           []referenceE1ManifestEntry
		}{
			Status:                      "E2_LOCAL_C4_D3_RECOVERY_BYTES_EQUAL",
			Reference:                   "CSFRV1-10/E2 #141",
			VerifiedBaselineCommits:     9,
			AuthenticatedA1Cuts:         2,
			IndependentCrashResumeCases: len(cases),
			PhysicalSQLRollbackSteps:    7,
			SignerReexecution:           false,
			CanonicalManifest:           expected.Manifest(),
		}
		raw, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		file, err := os.OpenFile(artifact, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write(append(raw, '\n')); err != nil {
			file.Close()
			t.Fatal(err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

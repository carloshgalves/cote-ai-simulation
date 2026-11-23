//go:build linux

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/commit"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
	"github.com/carloshgalves/cote-ai-simulation/internal/physicalhost"
)

const replayComparisonPathH2 = "replay/compare.json"

var replayRequiredPathsH2 = [...]string{
	"replay/causal.db",
	"replay/owner/actor-b.db.actor-a",
	"replay/owner/actor-b.db",
	replayComparisonPathH2,
}
var replaySidecarPathsH2 = [...]string{
	"replay/causal.db-wal", "replay/causal.db-shm",
	"replay/owner/actor-b.db.actor-a-wal", "replay/owner/actor-b.db.actor-a-shm",
	"replay/owner/actor-b.db-wal", "replay/owner/actor-b.db-shm",
}

// runExactSameA1H2 starts independent, fresh Go C4/D2/D3 databases and
// deliberately avoids spawning Python for the second run. Both signed source
// epochs are consumed EXACTLY as authored once by supervised Python A1; the
// normal RunReferenceD3 path remains unchanged.
func runExactSameA1H2(ctx context.Context, out, root, sha, run string,
	host physicalhost.ProducerHost, plans [2]physicalhost.ProducerPlan) error {
	if _, err := inspectRawA1H2(ctx, out, sha, run, host.Bundle); err != nil {
		return fmt.Errorf("H2 offline replay must start from verified genuine raw A1 source: %w", err)
	}
	epochs := [2]commit.OfflineOriginalA1Epoch{}
	for i := range epochs {
		base := filepath.Join(out, "physical-cuts", fmt.Sprintf("epoch-%d", i+1))
		for _, candidate := range []struct {
			path string
			slot *[]byte
		}{
			{"snapshot.yaml", &epochs[i].Snapshot},
			{"events.jsonl", &epochs[i].Events},
			{"evidence.json", &epochs[i].Evidence},
			{"cut.cbor", &epochs[i].Cut},
			{"signature.bin", &epochs[i].Signature},
		} {
			var err error
			*candidate.slot, err = readPrivateRegular(filepath.Join(base, candidate.path))
			if err != nil {
				return err
			}
		}
	}
	sub := filepath.Join(out, "replay")
	if err := os.Mkdir(sub, 0700); err != nil {
		return err
	}
	if err := os.Mkdir(filepath.Join(sub, "owner"), 0700); err != nil {
		return err
	}
	// Privileged signer's existing original key remains only in RAM; no raw
	// capture/export of the second run or fresh signature is allowed.
	host.PrivateCaptureRoot = ""
	secondary, err := commit.RunReferenceD3OfflineSameSignedA1(
		ctx, filepath.Join(sub, "causal.db"), filepath.Join(sub, "owner", "actor-b.db"),
		filepath.Join(root, bundleRelative),
		commit.ReferenceD2Config{Host: host, EpochPlans: plans},
		epochs, commit.ReferenceOptions{})
	if err != nil {
		return fmt.Errorf("H2 same-original-source independent D3: %w", err)
	}
	public, err := secondary.PublicReport()
	if err != nil {
		return err
	}
	originalReport, err := readPrivateRegular(filepath.Join(out, "report.json"))
	if err != nil {
		return err
	}
	newReport, err := json.MarshalIndent(public, "", "  ")
	if err != nil {
		return err
	}
	if !bytes.Equal(originalReport, append(newReport, '\n')) {
		return errors.New("H2 same original A1 replay produced different D3 public report")
	}
	compared, err := compareSameSignedSourcesH2(ctx, out, sha, run, host.Bundle)
	if err != nil {
		return fmt.Errorf("H2 same-source independent table and owner comparison: %w", err)
	}
	return exclusiveJSON(filepath.Join(sub, "compare.json"), compared)
}

// The cold verifier does NOT execute Python nor rerun the Go simulation. It
// independently reads both already-existing, authenticated SQLite roots and
// rebuilds their typed-cell multiset proofs. Private DBs stay private.
func coldVerifyReplayH2(ctx context.Context, out, sha, run string, bundle *policy.Bundle) error {
	if err := verifySameCutReplayH2(ctx, out, sha, run, bundle); err != nil {
		return fmt.Errorf("H2 independent SAME original A1 archive comparison: %w", err)
	}
	return nil
}

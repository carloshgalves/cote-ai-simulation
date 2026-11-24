//go:build linux

package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path/filepath"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/observatory"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

const causalReportPathH2 = "ledger-export/causal-report.json"

// A private derived view, never a canonical ledger or an input capability.
// Observatory's closed DTO contains only authenticated references and metadata.
type causalReportH2 struct {
	Schema      string                       `json:"schema"`
	SourceSHA   string                       `json:"source_sha"`
	RunID       string                       `json:"run_id"`
	Disposition string                       `json:"disposition"`
	Report      observatory.PrivilegedReport `json:"report"`
}

func captureCausalReportH2(ctx context.Context, db, sha, run string, p *policy.Bundle) (causalReportH2, error) {
	var zero causalReportH2
	raw, err := hex.DecodeString(run)
	if err != nil || len(raw) != 32 || hex.EncodeToString(raw) != run {
		return zero, errors.New("invalid causal report run identity")
	}
	var runID [32]byte
	copy(runID[:], raw)
	path, cleanup, err := detachedDBH2(db)
	if err != nil {
		return zero, err
	}
	defer cleanup()
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return zero, err
	}
	reader, err := sqlite.OpenPrivilegedObservatory(ctx, path, p)
	if err != nil {
		return zero, err
	}
	view, readErr := reader.ReadPrivilegedReport(ctx, runID)
	closeErr := reader.Close()
	if readErr != nil || closeErr != nil {
		return zero, errors.New("privileged causal report could not authenticate detached authority")
	}
	if view.State.Run != runID || view.Genesis.Run != runID ||
		view.State.Clock.Revision != 9 || len(view.Commits) != 9 ||
		view.State.Control.Status != observatory.IDLE ||
		view.PendingUnits != 0 || view.PendingPerceptionTasks != 0 ||
		view.RejectedIngress != "NOT_PERSISTED" {
		return zero, errors.New("causal report lacks completed reference authority")
	}
	return causalReportH2{
		Schema: "csfrv1-10h.causal-report.v1", SourceSHA: sha, RunID: run,
		Disposition: "PRIVATE_LOCAL_NOT_EXTERNALLY_ATTESTED", Report: view,
	}, nil
}

func exportCausalReportH2(ctx context.Context, out, sha, run string, p *policy.Bundle) error {
	report, err := captureCausalReportH2(ctx, filepath.Join(out, "causal.db"), sha, run, p)
	if err != nil {
		return err
	}
	return exclusiveJSON(filepath.Join(out, causalReportPathH2), report)
}

func verifyCausalReportH2(ctx context.Context, out, sha, run string, p *policy.Bundle) error {
	raw, err := readPrivateRegular(filepath.Join(out, causalReportPathH2))
	if err != nil {
		return errors.New("missing or unsafe private causal report")
	}
	var stored causalReportH2
	if err := json.Unmarshal(raw, &stored); err != nil || !exactLocalJSON(raw, stored) {
		return errors.New("noncanonical private causal report")
	}
	// Do not accept a stored declaration. Recompute both independently
	// executed authorities using the same authenticated query boundary.
	// The second DB's full equality is also checked by coldVerifyReplayH2.
	for _, db := range []string{"causal.db", "replay/causal.db"} {
		actual, err := captureCausalReportH2(ctx, filepath.Join(out, db), sha, run, p)
		if err != nil {
			return err
		}
		canonical, err := json.MarshalIndent(actual, "", "  ")
		if err != nil || !bytes.Equal(raw, append(canonical, '\n')) {
			return errors.New("causal report differs from authenticated replay")
		}
	}
	return nil
}

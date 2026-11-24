//go:build linux

package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime/debug"
	"syscall"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/commit"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/observatory"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/runbook"
)

const localUsage = "use exactly one of --out FRESH_DIR, --verify EXISTING_DIR, or --diagnose QUIESCENT_DIR --quiescent --run-id HEX --registration-sha256 HEX --source-sha SHA [--crash-confirmed]"

func localCommand(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("csf-reference-run-local", flag.ContinueOnError)
	// Flag errors can contain caller-controlled paths or arguments. Only
	// emit an allowlisted usage/status line, never raw error payloads.
	flags.SetOutput(io.Discard)
	out := flags.String("out", "", "")
	verify := flags.String("verify", "", "")
	diagnose := flags.String("diagnose", "", "")
	runID := flags.String("run-id", "", "")
	registration := flags.String("registration-sha256", "", "")
	source := flags.String("source-sha", "", "")
	quiescent := flags.Bool("quiescent", false, "")
	crash := flags.Bool("crash-confirmed", false, "")
	if err := flags.Parse(args); errors.Is(err, flag.ErrHelp) {
		fmt.Fprintln(stdout, localUsage)
		return 0
	} else if err != nil {
		fmt.Fprintln(stderr, localUsage)
		return emitOutcome(stderr, runbook.Usage)
	}
	operations := 0
	for _, path := range []string{*out, *verify, *diagnose} {
		if path != "" {
			operations++
		}
	}
	extras := *runID != "" || *registration != "" || *source != "" || *quiescent || *crash
	if flags.NArg() != 0 || operations != 1 || (*diagnose == "" && extras) ||
		(*diagnose != "" && (!*quiescent || !canonicalHex(*runID, 32) ||
			!canonicalHex(*registration, 32) || !canonicalHex(*source, 20))) {
		fmt.Fprintln(stderr, localUsage)
		return emitOutcome(stderr, runbook.Usage)
	}
	if *out != "" {
		syscall.Umask(0077)
		return emitOutcomeForError(stdout, stderr, runLocal(ctx, *out), false)
	}
	if *verify != "" {
		return emitOutcomeForError(stdout, stderr, verifyLocal(ctx, *verify), true)
	}
	var run [32]byte
	raw, _ := hex.DecodeString(*runID)
	copy(run[:], raw)
	outcome, err := diagnoseLocal(ctx, *diagnose, run, *registration, *source, *crash)
	if err != nil {
		return emitOutcomeForError(stdout, stderr, err, true)
	}
	return emitOutcome(stdout, outcome)
}

func canonicalHex(value string, size int) bool {
	raw, err := hex.DecodeString(value)
	return err == nil && len(raw) == size && hex.EncodeToString(raw) == value
}

func emitOutcome(w io.Writer, outcome runbook.Outcome) int {
	fmt.Fprintln(w, "LOCAL_REFERENCE_STATE="+outcome.State)
	fmt.Fprintf(w, "LOCAL_REFERENCE_EXIT_CODE=%d\n", outcome.Code)
	fmt.Fprintln(w, "PRODUCTION_APPROVAL=NO")
	return outcome.Code
}

func emitOutcomeForError(stdout, stderr io.Writer, err error, verification bool) int {
	outcome := runbook.Error(err)
	if err == nil {
		return emitOutcome(stdout, outcome)
	}
	if verification && outcome == runbook.Unknown {
		outcome = runbook.Unverified
	}
	fmt.Fprintln(stderr, "LOCAL_REFERENCE_NO_GO: "+outcome.State)
	return emitOutcome(stderr, outcome)
}

// captureDiagnosisFiles pins the source files for a quiescent operator
// diagnosis. Missing optional sidecars are represented by absent keys.
// Never use this copy protocol against a database with a live writer.
func captureDiagnosisFiles(dir string) (map[string]string, error) {
	out := make(map[string]string)
	for _, name := range []string{"registration.json", "causal.db", "causal.db-wal", "causal.db-shm"} {
		raw, err := readPrivateRegular(filepath.Join(dir, name))
		if errors.Is(err, os.ErrNotExist) && (name == "causal.db-wal" || name == "causal.db-shm") {
			continue
		}
		if err != nil {
			return nil, err
		}
		out[name] = hashBytes(raw)
	}
	return out, nil
}

// diagnoseLocal is a human-only read projection, not the complete archive
// verifier and not a resume command. The source SHA, public registration
// digest and RunID must come from a previously trusted operator record.
func diagnoseLocal(ctx context.Context, dir string, run [32]byte, regHash, sourceSHA string, crash bool) (runbook.Outcome, error) {
	// A clean checkout alone is not enough: a stale executable could read
	// newer source metadata. Require the direct VCS-stamped binary.
	if !exactBuildRevision(sourceSHA) {
		return runbook.Unknown, runbook.ErrIncompatible
	}
	root, err := gitOutput(".", "rev-parse", "--show-toplevel")
	if err != nil {
		return runbook.Unknown, err
	}
	sha, err := gitOutput(root, "rev-parse", "HEAD")
	if err != nil || sha != sourceSHA {
		return runbook.Unknown, runbook.ErrIncompatible
	}
	dirty, err := gitOutput(root, "status", "--porcelain", "--untracked-files=all")
	if err != nil || dirty != "" {
		return runbook.Unknown, runbook.ErrIncompatible
	}
	dir, err = filepath.Abs(dir)
	if err != nil {
		return runbook.Unknown, err
	}
	real, err := filepath.EvalSymlinks(dir)
	if err != nil || real != dir {
		return runbook.Unknown, runbook.ErrUnverified
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return runbook.Unknown, runbook.ErrUnverified
	}
	before, err := captureDiagnosisFiles(dir)
	if err != nil || before["registration.json"] != regHash {
		return runbook.Unknown, runbook.ErrUnverified
	}
	reg, err := readPrivateRegular(filepath.Join(dir, "registration.json"))
	if err != nil || hashBytes(reg) != regHash {
		return runbook.Unknown, runbook.ErrUnverified
	}
	var spec policy.PhysicalManifest
	if err := json.Unmarshal(reg, &spec); err != nil {
		return runbook.Unknown, runbook.ErrUnverified
	}
	canonical, err := json.Marshal(spec)
	if err != nil || !bytes.Equal(reg, canonical) {
		return runbook.Unknown, runbook.ErrUnverified
	}
	base, err := policy.Load(filepath.Join(root, bundleRelative))
	if err != nil {
		return runbook.Unknown, runbook.ErrIncompatible
	}
	bundle, err := policy.PhysicalExtensionV1(base, reg)
	if err != nil {
		return runbook.Unknown, runbook.ErrIncompatible
	}
	expected, err := commit.ReferenceRunID(bundle)
	if err != nil {
		return runbook.Unknown, runbook.ErrIncompatible
	}
	if run != expected {
		return runbook.Unknown, runbook.ErrUnverified
	}
	copyDB, cleanup, err := detachedDBH2(filepath.Join(dir, "causal.db"))
	if err != nil {
		return runbook.Unknown, err
	}
	defer cleanup()
	// Verify the detached bytes before opening SQLite, which may change
	// its scratch sidecars during recovery. The original is never opened.
	for _, suffix := range []string{"", "-wal", "-shm"} {
		data, err := readPrivateRegular(copyDB + suffix)
		expectedHash, present := before["causal.db"+suffix]
		if !present && errors.Is(err, os.ErrNotExist) {
			continue
		}
		if !present || err != nil || hashBytes(data) != expectedHash {
			return runbook.Unknown, observatory.ErrSourceChanged
		}
	}
	reader, err := sqlite.OpenPrivilegedObservatory(ctx, copyDB, bundle)
	if err != nil {
		return runbook.Unknown, err
	}
	report, readErr := reader.ReadOperationalReport(ctx, run)
	closeErr := reader.Close()
	after, err := captureDiagnosisFiles(dir)
	if err != nil || !reflect.DeepEqual(before, after) {
		return runbook.Unknown, observatory.ErrSourceChanged
	}
	if readErr != nil {
		return runbook.Unknown, readErr
	}
	if closeErr != nil {
		return runbook.Unknown, closeErr
	}
	return runbook.Report(report, crash), nil
}

// exactBuildRevision uses Go's executable metadata, not a caller-supplied
// environment string or a version read from the current working directory.
func exactBuildRevision(sourceSHA string) bool {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return false
	}
	revision, modified := "", ""
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			revision = setting.Value
		case "vcs.modified":
			modified = setting.Value
		}
	}
	return revision == sourceSHA && modified == "false"
}

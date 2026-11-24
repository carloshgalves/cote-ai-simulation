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
	"strings"
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
	var diagnostic *diagnosisError
	if errors.As(err, &diagnostic) {
		fmt.Fprintln(stderr, "LOCAL_REFERENCE_DIAGNOSTIC_STAGE="+diagnostic.stage)
		fmt.Fprintln(stderr, "LOCAL_REFERENCE_DIAGNOSTIC_REASON="+diagnosticReason(diagnostic.cause))
	}
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
			return nil, fmt.Errorf("diagnostic source family unreadable: %w", err)
		}
		out[name] = hashBytes(raw)
	}
	return out, nil
}

// diagnoseLocal is a human-only read projection, not the complete archive
// verifier and not a resume command. The source SHA, public registration
// digest and RunID must come from a previously trusted operator record.
func diagnoseLocal(ctx context.Context, dir string, run [32]byte, regHash, sourceSHA string, crash bool) (outcome runbook.Outcome, failure error) {
	stage := "BUILD_REVISION"
	defer func() {
		if failure != nil {
			failure = &diagnosisError{stage: stage, cause: failure}
		}
	}()
	// A clean checkout alone is not enough: a stale executable could read
	// newer source metadata. Require the direct VCS-stamped binary.
	if !exactBuildRevision(sourceSHA) {
		return runbook.Unknown, runbook.ErrIncompatible
	}
	stage = "CHECKOUT"
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
	stage = "SOURCE_DIRECTORY"
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
	stage = "SOURCE_FILES"
	before, err := captureDiagnosisFiles(dir)
	if err != nil {
		return runbook.Unknown, fmt.Errorf("%w: %w", runbook.ErrUnverified, err)
	}
	stage = "REGISTRATION_PIN"
	if before["registration.json"] != regHash {
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
	stage = "INSTALLED_POLICY"
	base, err := policy.Load(filepath.Join(root, bundleRelative))
	if err != nil {
		return runbook.Unknown, runbook.ErrIncompatible
	}
	bundle, err := policy.PhysicalExtensionV1(base, reg)
	if err != nil {
		return runbook.Unknown, runbook.ErrIncompatible
	}
	stage = "REFERENCE_RUN_ID"
	expected, err := commit.ReferenceRunID(bundle)
	if err != nil {
		return runbook.Unknown, runbook.ErrIncompatible
	}
	if run != expected {
		return runbook.Unknown, runbook.ErrUnverified
	}
	stage = "DETACHED_COPY_PREFLIGHT"
	copyDB, cleanup, err := detachedDBH2(filepath.Join(dir, "causal.db"))
	if err != nil {
		return runbook.Unknown, err
	}
	defer cleanup()
	copyDB, err = filepath.EvalSymlinks(copyDB)
	if err != nil {
		return runbook.Unknown, err
	}
	// Verify the detached bytes before opening SQLite, which may change
	// its scratch sidecars during recovery. The original is never opened.
	stage = "DETACHED_COPY_DIGESTS"
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
	stage = "OBSERVATORY_OPEN"
	reader, err := sqlite.OpenPrivilegedObservatory(ctx, copyDB, bundle)
	if err != nil {
		return runbook.Unknown, err
	}
	report, readErr := reader.ReadOperationalReport(ctx, run)
	closeErr := reader.Close()
	stage = "SOURCE_RECHECK"
	after, err := captureDiagnosisFiles(dir)
	if err != nil || !reflect.DeepEqual(before, after) {
		return runbook.Unknown, observatory.ErrSourceChanged
	}
	if readErr != nil {
		stage = "OBSERVATORY_REPLAY"
		return runbook.Unknown, readErr
	}
	if closeErr != nil {
		stage = "OBSERVATORY_CLOSE"
		return runbook.Unknown, closeErr
	}
	stage = "VERIFIED_REPORT"
	result := runbook.Report(report, crash)
	if result == runbook.Unverified {
		return runbook.Unknown, runbook.ErrUnverified
	}
	return result, nil
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

// diagnosisError carries only a closed code-level stage. Its underlying
// error is never printed; it may contain privileged paths or record IDs.
type diagnosisError struct {
	stage string
	cause error
}

func (e *diagnosisError) Error() string { return "local diagnosis failed at " + e.stage }
func (e *diagnosisError) Unwrap() error { return e.cause }

// diagnosticReason is an allowlist of fixed labels, not a second error
// classifier. Unrecognized error strings never appear in logs.
func diagnosticReason(err error) string {
	switch {
	case errors.Is(err, os.ErrNotExist):
		return "FILE_MISSING"
	case errors.Is(err, os.ErrPermission):
		return "ACCESS_DENIED"
	case errors.Is(err, observatory.ErrSourceChanged):
		return "SOURCE_CHANGED"
	}
	for _, item := range []struct {
		text   string
		reason string
	}{
		{"refuse nonregular, exposed or oversized evidence", "UNSAFE_EVIDENCE_FILE"},
		{"hardlinked or unverifiable private artifact", "UNSAFE_FILE_IDENTITY"},
		{"database ancestor is not a trusted directory", "UNTRUSTED_SCRATCH_ANCESTOR"},
		{"SQLite parent must be operator-owned", "UNTRUSTED_SCRATCH_PARENT"},
		{"SQLite workspace requires an operator-owned private ancestor", "NONPRIVATE_SCRATCH"},
		{"unsafe database/pager file", "UNSAFE_PAGER_FILE"},
		{"H2 WAL preflight:", "WAL_PREFLIGHT_REJECTED"},
		{"open existing CSF database in read-only mode", "READONLY_OPEN_REJECTED"},
		{"privileged causal replay:", "CAUSAL_REPLAY_REJECTED"},
		{"unsupported SQLite identity:", "SQLITE_RUNTIME_MISMATCH"},
		{"SQLite runtime manifest identity mismatch", "SQLITE_RUNTIME_MISMATCH"},
		{"journal_mode must be WAL", "SQLITE_JOURNAL_MODE"},
		{"synchronous must be FULL", "SQLITE_SYNCHRONOUS_MODE"},
		{"readonly SQL", "READONLY_DRIVER_REJECTED"},
	} {
		if strings.Contains(err.Error(), item.text) {
			return item.reason
		}
	}
	return "UNCLASSIFIED"
}

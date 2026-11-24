//go:build linux

package commit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/admission"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/runbook"
)

// Use the same authenticated raw CLI inputs as H2-C, no resampling Python.
// The executable is built from the checked-out HEAD by the private H job.
func TestH2ORunbookRealCuts(t *testing.T) {
	binary := os.Getenv("H2O_RUNBOOK_BINARY")
	if binary == "" {
		if os.Getenv("H2_ARCHIVED_SOURCE_REQUIRED") == "1" {
			t.Fatal("required H2-O executable missing")
		}
		t.Skip("requires the exact-HEAD executable and a verified H2 CLI archive")
	}
	root := os.Getenv("H2_ARCHIVED_SOURCE")
	host, cuts, _, read, _ := h2ArchivedInputs(t, root, os.Getenv("H2_ARCHIVED_INDEX_SHA256"))
	g, err := buildReferenceGenesis(host.Bundle)
	if err != nil {
		t.Fatal(err)
	}
	run := hex.EncodeToString(g.RunID[:])
	reg := read("registration.json")
	regHash := identityHashH2O(reg)
	shaRaw, err := exec.Command("git", "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal("missing exact source SHA")
	}
	sha := strings.TrimSpace(string(shaRaw))
	diagnose := func(t *testing.T, dir string, crash bool, source, digest string, want runbook.Outcome) {
		t.Helper()
		args := []string{"--diagnose", dir, "--quiescent", "--run-id", run, "--registration-sha256", digest, "--source-sha", source}
		if crash {
			args = append(args, "--crash-confirmed")
		}
		h2oAssertProcess(t, binary, args, want)
	}
	barrierStop := errors.New("H2-O deliberate stop after delivery before perception")
	completed := 0
	for _, tc := range []struct {
		name   string
		opts   ReferenceOptions
		stage  string
		epoch  int
		step   uint8
		expect runbook.Outcome
		crash  bool
	}{
		{"source-gap", ReferenceOptions{LeaveSourceOpen: true}, "initial admission", 0, 0, runbook.WaitClosure, false},
		{"in-flight", ReferenceOptions{FailAbortAt: 1}, "C3 atomic terminal abort", 0, 0, runbook.InFlight, false},
		{"halted", ReferenceOptions{FailRetryAt: 1}, "C3 operator-authorized canonical retry", 0, 0, runbook.Halted, false},
		{"retry", ReferenceOptions{stopAfterAuthorizedRetry: true}, "reference harness stopped at durable RETRY_AUTHORIZED", 0, 0, runbook.Retry, false},
		{"physical-crash", ReferenceOptions{}, "D2 atomic CommitPhysical epoch 1", 1, 1, runbook.Crash, true},
		{"barrier", ReferenceOptions{referenceBeforeDeliveryPerception: func(context.Context, string) error { return barrierStop }}, barrierStop.Error(), 0, 0, runbook.Barrier, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "private")
			if err := os.Mkdir(dir, 0700); err != nil {
				t.Fatal(err)
			}
			ownerDir := filepath.Join(dir, "owner")
			if err := os.Mkdir(ownerDir, 0700); err != nil {
				t.Fatal(err)
			}
			db := filepath.Join(dir, "causal.db")
			owner := filepath.Join(ownerDir, "actor-b.db")
			_, err := RunReferenceD2(context.Background(), db, owner, bundlePath,
				ReferenceD2Config{Host: host, frozenSupervisedCuts: &cuts, FailPhysicalAtEpoch: tc.epoch, FailPhysicalAfterStep: tc.step}, tc.opts)
			if err == nil || !strings.Contains(err.Error(), tc.stage) {
				t.Fatal("H2-O fixture did not reach its intended durable stop")
			}
			if tc.expect == runbook.WaitClosure && !errors.Is(err, admission.ErrWaitingForClosure) {
				t.Fatal("H2-O fixture did not produce a typed closure wait")
			}
			if tc.expect == runbook.Barrier && !errors.Is(err, barrierStop) {
				t.Fatal("H2-O fixture did not reach the committed delivery hook")
			}
			if tc.expect == runbook.Crash && !referenceE2ExpectedFault(err, 1) {
				t.Fatal("H2-O fixture did not reach the physical atomic rollback")
			}
			if err := os.WriteFile(filepath.Join(dir, "registration.json"), reg, 0600); err != nil {
				t.Fatal(err)
			}
			before := h2oFileDigests(t, dir)
			diagnose(t, dir, tc.crash, sha, regHash, tc.expect)
			if tc.crash {
				// The same durable fence does not itself prove process death.
				diagnose(t, dir, false, sha, regHash, runbook.InFlight)
			}
			if !reflect.DeepEqual(before, h2oFileDigests(t, dir)) {
				t.Fatal("H2-O diagnostic changed its source database or sidecars")
			}
			completed++
		})
	}
	if t.Failed() || completed != 6 {
		t.Fatalf("H2-O real stop grid incomplete: %d/6", completed)
	}
	// An idle, complete archive is NOT accepted as a D3 certificate by the
	// diagnostic command. Complete verification is its own operation.
	before := h2oFileDigests(t, root)
	diagnose(t, root, false, sha, regHash, runbook.NoStop)
	diagnose(t, root, false, strings.Repeat("0", 40), regHash, runbook.Incompatible)
	diagnose(t, root, false, sha, strings.Repeat("0", 64), runbook.Unverified)
	h2oAssertProcess(t, binary, []string{"--verify", root}, runbook.Success)
	h2oAssertProcess(t, binary, []string{"--diagnose", root}, runbook.Usage)
	if !reflect.DeepEqual(before, h2oFileDigests(t, root)) {
		t.Fatal("H2-O process verification changed the archived causal authority")
	}
	// Malformed detached DB: a valid public registration is not a proof of
	// an intact causal authority. Must never produce a positive diagnosis.
	damaged := filepath.Join(t.TempDir(), "private")
	if err := os.Mkdir(damaged, 0700); err != nil {
		t.Fatal(err)
	}
	for name, raw := range map[string][]byte{"registration.json": reg, "causal.db": []byte("PRIVATE_PAYLOAD")} {
		if err := os.WriteFile(filepath.Join(damaged, name), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	diagnose(t, damaged, false, sha, regHash, runbook.Unverified)

	// Exercise immediate process failures through the real --out command.
	// The intentionally invalid interpreter advertises a version and a
	// private dependency directory but never produces a valid physical cut.
	// Do not introduce fault flags into the operator-facing CLI.
	for _, tc := range []struct {
		version string
		want    runbook.Outcome
	}{
		{"Python 0.0.0", runbook.Incompatible},
		{"Python 3.13.18", runbook.Rejected},
	} {
		fake := t.TempDir()
		script := "#!/bin/sh\ncase \"$1\" in\n--version) printf '%s\\n' '" + tc.version + "';;\n-S) printf '%s\\n' '" + fake + "';;\n*) exit 42;;\nesac\n"
		if err := os.WriteFile(filepath.Join(fake, "python"), []byte(script), 0700); err != nil {
			t.Fatal(err)
		}
		out := filepath.Join(t.TempDir(), "failed")
		h2oAssertProcessEnv(t, binary, []string{"--out", out}, tc.want, []string{"PATH=" + fake + string(os.PathListSeparator) + os.Getenv("PATH")})
		if _, err := os.Lstat(filepath.Join(out, "checksums.json")); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("H2-O rejected producer minted a completed archive")
		}
	}
	t.Log("H2_O_REAL_CUTS=6_OF_6_AND_NEGATIVE_PROCESS_EXITS")
}

func h2oAssertProcess(t *testing.T, binary string, args []string, want runbook.Outcome) {
	t.Helper()
	h2oAssertProcessEnv(t, binary, args, want, nil)
}

func h2oAssertProcessEnv(t *testing.T, binary string, args []string, want runbook.Outcome, env []string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Env = append(os.Environ(), env...)
	output, err := cmd.CombinedOutput()
	code := 0
	if err != nil {
		var exited *exec.ExitError
		if !errors.As(err, &exited) {
			t.Fatal("H2-O executable did not return a process exit code")
		}
		code = exited.ExitCode()
	}
	if ctx.Err() != nil || code != want.Code || !strings.Contains(string(output), "LOCAL_REFERENCE_STATE="+want.State+"\n") ||
		strings.Contains(string(output), "PRIVATE_PAYLOAD") ||
		(want.Code != 0 && strings.Contains(string(output), "FIRST_REFERENCE_RUN_LOCAL=EXECUTED_AND_CHECKED")) {
		t.Fatalf("H2-O wrong executable outcome: got code %d want %d (%s)", code, want.Code, want.State)
	}
}

func identityHashH2O(raw []byte) string {
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

func h2oFileDigests(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := make(map[string]string)
	for _, name := range []string{"registration.json", "causal.db", "causal.db-wal", "causal.db-shm"} {
		path := filepath.Join(dir, name)
		raw, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) && strings.HasPrefix(name, "causal.db-") {
			continue
		}
		if err != nil {
			t.Fatal("H2-O source unavailable")
		}
		out[name] = identityHashH2O(raw)
	}
	return out
}

package commit

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/ownercheckpoint"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/ownercheckpoint/localdemo"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/ownercheckpoint/localowner"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
	_ "modernc.org/sqlite"
)

const b3CrashExit = 86
const b3Memory = "B3 synthetic private actor memory"

func b3Key() ed25519.PrivateKey {
	// Fixed synthetic key for isolated Go tests only; never a product credential.
	return ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0xb3}, ed25519.SeedSize))
}

func b3Checkpoint(scope ownercheckpoint.Scope, instant int64, revision uint64) sqlite.SnapshotCheckpoint {
	return sqlite.SnapshotCheckpoint{
		ActorNamespace: scope.Namespace, ActorID: scope.Actor,
		StoreID: "local-memory-b3", SchemaVersion: 1,
		Producer: "owner-b3", ProducerVersion: 1,
		CoversInstant: instant, CoversRevision: revision,
		Content: []byte(b3Memory),
	}
}

func b3Reference(t *testing.T, b *policy.Bundle, scope ownercheckpoint.Scope, cp sqlite.SnapshotCheckpoint) localdemo.Reference {
	t.Helper()
	seed, err := sqlite.CognitiveCheckpointSigningBytesV1(scope.Run, cp, "owner-seed")
	if err != nil {
		t.Fatal(err)
	}
	h := identity.OpaqueContentHash(seed)
	locator := "owner-" + hex.EncodeToString(h[:])
	raw, err := sqlite.CognitiveCheckpointSigningBytesV1(scope.Run, cp, locator)
	if err != nil {
		t.Fatal(err)
	}
	receipt := identity.OpaqueContentHash(raw)
	snapshot, _, err := identity.New(b).Sum("cote.csf.id.snapshot",
		"cote.csf.schema.snapshot-id-preimage", 1,
		[]any{scope.Run[:], uint64(1), cp.CoversRevision})
	if err != nil {
		t.Fatal(err)
	}
	return localdemo.Reference{Snapshot: [32]byte(snapshot), Receipt: receipt}
}

type b3CrashOwner struct {
	*localowner.Store
	phase string
}

func (o *b3CrashOwner) Restore(ctx context.Context, req ownercheckpoint.RestoreRequest) (ownercheckpoint.RestoredContent, error) {
	restored, err := o.Store.Restore(ctx, req)
	if err == nil && o.phase == "post-restore" {
		os.Exit(b3CrashExit)
	}
	return restored, err
}

func (o *b3CrashOwner) Acknowledge(ctx context.Context, ack ownercheckpoint.RestoreAck) error {
	err := o.Store.Acknowledge(ctx, ack)
	if err == nil && o.phase == "post-ack" {
		os.Exit(b3CrashExit)
	}
	return err
}

// TestCSFRV18R58B3Child is only entered as an independently executed test
// BINARY, never as a helper goroutine inside the parent process. os.Exit(86)
// simulates a process abruptly disappearing without closing SQLite handles.
func TestCSFRV18R58B3Child(t *testing.T) {
	if os.Getenv("COTE_B3_CHILD") != "isolated-test-process" {
		return
	}
	phase := os.Getenv("COTE_B3_PHASE")
	known := map[string]bool{
		"pre-owner": true, "post-owner": true, "post-escrow": true,
		"post-snapshot": true, "post-restore": true, "post-ack": true,
	}
	if !known[phase] {
		t.Fatalf("unsupported child phase %q", phase)
	}
	rawRun, err := hex.DecodeString(os.Getenv("COTE_B3_RUN"))
	if err != nil || len(rawRun) != 32 {
		t.Fatal("invalid synthetic run fixture")
	}
	var run [32]byte
	copy(run[:], rawRun)
	scope := ownercheckpoint.Scope{Run: run, Namespace: "cognition", Actor: "owner-b3"}
	ctx := context.Background()
	b, err := policy.Load(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	causal, err := sqlite.Open(ctx, os.Getenv("COTE_B3_CSF"))
	if err != nil {
		t.Fatal(err)
	}
	owner, err := localowner.Open(ctx, os.Getenv("COTE_B3_OWNER"), b3Key())
	if err != nil {
		t.Fatal(err)
	}
	now, err := causal.LoadClock(ctx, run)
	if err != nil {
		t.Fatal(err)
	}
	cp := b3Checkpoint(scope, int64(now.Instant), uint64(now.Revision))
	wrapped := &b3CrashOwner{Store: owner, phase: phase}
	// No defer Close: the injected process exit is the object of this test.
	switch phase {
	case "pre-owner":
		os.Exit(b3CrashExit)
	case "post-owner":
		if _, err := wrapped.PersistAndSign(ctx, ownercheckpoint.PersistRequest{Scope: scope, Checkpoint: cp}); err != nil {
			t.Fatal(err)
		}
		os.Exit(b3CrashExit)
	case "post-escrow":
		gate, err := ownercheckpoint.NewGate(wrapped, causal, b)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := gate.Stage(ctx, scope, cp); err != nil {
			t.Fatal(err)
		}
		os.Exit(b3CrashExit)
	case "post-snapshot":
		host, err := localdemo.New(wrapped, causal, b, scope, func(context.Context) error {
			t.Fatal("unexpected cognitive callback before restore")
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := host.StageSnapshot(ctx, cp); err != nil {
			t.Fatal(err)
		}
		os.Exit(b3CrashExit)
	case "post-restore", "post-ack":
		host, err := localdemo.New(wrapped, causal, b, scope, func(context.Context) error {
			t.Fatal("unexpected cognitive callback in recovery child")
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		ref := b3Reference(t, b, scope, cp)
		if err := host.Restore(ctx, ref); err != nil {
			t.Fatal(err)
		}
		t.Fatal("crash hook was not reached")
	}
	t.Fatal("child phase returned without an injected crash")
}

func b3SubprocessCrash(t *testing.T, phase, causalPath, ownerPath string, run [32]byte) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestCSFRV18R58B3Child$", "-test.v=false")
	cmd.Env = append(os.Environ(),
		"COTE_B3_CHILD=isolated-test-process", "COTE_B3_PHASE="+phase,
		"COTE_B3_CSF="+causalPath, "COTE_B3_OWNER="+ownerPath,
		"COTE_B3_RUN="+hex.EncodeToString(run[:]))
	output, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("%s child timed out: %v", phase, ctx.Err())
	}
	var ee *exec.ExitError
	if !errors.As(err, &ee) || ee.ExitCode() != b3CrashExit {
		t.Fatalf("%s did not stop at injected boundary, err=%v exit=%v output=%s", phase, err, cmd.ProcessState, string(output))
	}
	if len(output) != 0 {
		t.Fatalf("%s child unexpectedly emitted data: %q", phase, string(output))
	}
}

// b3LedgerDigest is a read-only, length-delimited fingerprint of the five
// canonical causal prefixes. It deliberately does not include private owner
// bytes, ephemeral checkpoints or the snapshot/receipt/ACK authority tables.
func b3LedgerDigest(t *testing.T, path string) [32]byte {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	fingerprint := sha256.New()
	for _, table := range [...]string{
		"input_ledger", "decision_ledger", "event_store",
		"evidence_ledger", "causal_outbox",
	} {
		// Table identifiers are a hard-coded allowlist, never user input.
		rows, err := db.Query("SELECT canonical FROM " + table + " ORDER BY id")
		if err != nil {
			t.Fatalf("canonical ledger %s query: %v", table, err)
		}
		appendFramed := func(data []byte) {
			var lenFrame [8]byte
			binary.BigEndian.PutUint64(lenFrame[:], uint64(len(data)))
			_, _ = fingerprint.Write(lenFrame[:])
			_, _ = fingerprint.Write(data)
		}
		appendFramed([]byte(table))
		for rows.Next() {
			var canonical []byte
			if err := rows.Scan(&canonical); err != nil {
				_ = rows.Close()
				t.Fatal(err)
			}
			appendFramed(canonical)
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			t.Fatal(err)
		}
		if err := rows.Close(); err != nil {
			t.Fatal(err)
		}
	}
	var result [32]byte
	copy(result[:], fingerprint.Sum(nil))
	return result
}

func b3Rows(t *testing.T, path, table string) int {
	t.Helper()
	// Allowlist only: table names cannot originate from untrusted input.
	allowed := map[string]bool{"checkpoints": true, "restore_acks": true, "cognitive_owner_receipts": true, "snapshot_seals": true, "fork_cognitive_owner_proofs": true}
	if !allowed[table] {
		t.Fatal("invalid table name")
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var count int
	if err := db.QueryRow("SELECT count(*) FROM " + table).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

// Proves abrupt PROCESS termination at six real boundaries using two actual
// SQLite files. Each case starts with a new run fixture and owner DB; no
// simulated in-memory owner and no LLM regeneration is involved.
func TestCSFRV18R58B3ProcessCrashMatrix(t *testing.T) {
	cases := []struct {
		phase                                  string
		ownerRows, escrowRows, snapshots, acks int
		needsPublishedSnapshot                 bool
	}{
		{"pre-owner", 0, 0, 0, 0, false},
		{"post-owner", 1, 0, 0, 0, false},
		{"post-escrow", 1, 1, 0, 0, false},
		{"post-snapshot", 1, 1, 1, 0, false},
		{"post-restore", 1, 1, 1, 0, true},
		{"post-ack", 1, 1, 1, 1, true},
	}
	for _, tc := range cases {
		t.Run(tc.phase, func(t *testing.T) {
			ctx := context.Background()
			_, causal, _, run, b, causalPath := testFixture(t)
			scope := ownercheckpoint.Scope{Run: [32]byte(run), Namespace: "cognition", Actor: "owner-b3"}
			private := b3Key()
			if err := causal.RegisterCognitiveActor(ctx, scope.Run, scope.Namespace, scope.Actor, b); err != nil {
				t.Fatal(err)
			}
			if err := causal.RegisterCognitiveOwnerKeyV1(ctx, scope.Run, scope.Namespace, scope.Actor, private.Public().(ed25519.PublicKey), b); err != nil {
				t.Fatal(err)
			}
			now, err := causal.LoadClock(ctx, scope.Run)
			if err != nil {
				t.Fatal(err)
			}
			cp := b3Checkpoint(scope, int64(now.Instant), uint64(now.Revision))
			ref := b3Reference(t, b, scope, cp)
			dir := t.TempDir()
			if err := os.Chmod(dir, 0700); err != nil {
				t.Fatal(err)
			}
			ownerPath := filepath.Join(dir, "owner.sqlite")
			if tc.needsPublishedSnapshot {
				owner, err := localowner.Open(ctx, ownerPath, private)
				if err != nil {
					t.Fatal(err)
				}
				host, err := localdemo.New(owner, causal, b, scope, func(context.Context) error {
					t.Fatal("WORK before crash")
					return nil
				})
				if err != nil {
					t.Fatal(err)
				}
				published, err := host.StageSnapshot(ctx, cp)
				if err != nil || published != ref {
					t.Fatalf("bad prepared snapshot: %v", err)
				}
				if err := owner.Close(); err != nil {
					t.Fatal(err)
				}
			}
			ledgerBefore := b3LedgerDigest(t, causalPath)
			if err := causal.Close(); err != nil {
				t.Fatal(err)
			}
			b3SubprocessCrash(t, tc.phase, causalPath, ownerPath, scope.Run)
			if got := b3LedgerDigest(t, causalPath); got != ledgerBefore {
				t.Fatal("crashed cognitive process changed authoritative five-ledger prefixes")
			}
			if got := b3Rows(t, ownerPath, "checkpoints"); got != tc.ownerRows {
				t.Fatalf("owner rows after crash: got %d want %d", got, tc.ownerRows)
			}
			if got := b3Rows(t, ownerPath, "restore_acks"); got != tc.acks {
				t.Fatalf("ACK rows after crash: got %d want %d", got, tc.acks)
			}
			if got := b3Rows(t, causalPath, "cognitive_owner_receipts"); got != tc.escrowRows {
				t.Fatalf("escrow rows after crash: got %d want %d", got, tc.escrowRows)
			}
			if got := b3Rows(t, causalPath, "snapshot_seals"); got != tc.snapshots {
				t.Fatalf("snapshot rows after crash: got %d want %d", got, tc.snapshots)
			}
			recovered, err := sqlite.Open(ctx, causalPath)
			if err != nil {
				t.Fatal(err)
			}
			defer recovered.Close()
			owner, err := localowner.Open(ctx, ownerPath, private)
			if err != nil {
				t.Fatal(err)
			}
			defer owner.Close()
			calls := 0
			host, err := localdemo.New(owner, recovered, b, scope, func(context.Context) error { calls++; return nil })
			if err != nil {
				t.Fatal(err)
			}
			if err := host.Run(ctx, ref); !errors.Is(err, ownercheckpoint.ErrNotAcknowledged) || calls != 0 {
				t.Fatalf("crashed process leaked WORK permit: calls=%d err=%v", calls, err)
			}
			// A missing snapshot must never authorize cognition, even when
			// an independent owner commit and/or CSF signed escrow survived.
			if tc.snapshots == 0 {
				if err := host.Restore(ctx, ref); err == nil {
					t.Fatal("orphan owner/escrow authorized WORK")
				}
			}
			published, err := host.StageSnapshot(ctx, cp)
			if err != nil || published != ref {
				t.Fatalf("idempotent crash recovery publication: %+v %v", published, err)
			}
			if got := b3Rows(t, ownerPath, "checkpoints"); got != 1 {
				t.Fatalf("recovery duplicated owner checkpoint: %d", got)
			}
			if got := b3Rows(t, causalPath, "cognitive_owner_receipts"); got != 1 {
				t.Fatalf("recovery duplicated escrow: %d", got)
			}
			if got := b3Rows(t, causalPath, "snapshot_seals"); got != 1 {
				t.Fatalf("recovery duplicated snapshot: %d", got)
			}
			if err := host.Run(ctx, ref); !errors.Is(err, ownercheckpoint.ErrNotAcknowledged) || calls != 0 {
				t.Fatal("Stage implicitly granted WORK after crash")
			}
			if err := host.Restore(ctx, ref); err != nil {
				t.Fatalf("post-crash exact restore/ACK failed: %v", err)
			}
			if err := host.Run(ctx, ref); err != nil || calls != 1 {
				t.Fatalf("post-crash gated work calls=%d err=%v", calls, err)
			}
			if got := b3Rows(t, ownerPath, "restore_acks"); got != 1 {
				t.Fatalf("recovery ACK not idempotent: %d", got)
			}
			if got := b3LedgerDigest(t, causalPath); got != ledgerBefore {
				t.Fatal("owner recovery or ACK altered the five causal ledgers")
			}
			verified, err := recovered.VerifySnapshotV1(ctx, scope.Run, ref.Snapshot, b)
			if err != nil || verified.Snapshot.ID != ref.Snapshot || verified.TailCommits != 0 {
				t.Fatalf("recovered snapshot/clock proof failed: %v", err)
			}
			escrow, _, err := recovered.LoadCognitiveCheckpointV1(ctx, scope.Run, ref.Receipt, scope.Namespace, scope.Actor, b)
			if err != nil || !bytes.Equal(escrow.Content, cp.Content) {
				t.Fatalf("recovered escrow mismatched synthetic memory: %v", err)
			}
			other := scope
			other.Actor = "foreign-actor"
			gate, err := ownercheckpoint.NewGate(owner, recovered, b)
			if err != nil {
				t.Fatal(err)
			}
			if err := gate.RestoreAndAcknowledge(ctx, other, ref.Snapshot, ref.Receipt); err == nil {
				t.Fatal("foreign actor restored memory")
			}
			other = scope
			other.Run[0] ^= 0x72
			if err := gate.RestoreAndAcknowledge(ctx, other, ref.Snapshot, ref.Receipt); err == nil {
				t.Fatal("foreign run restored memory")
			}
		})
	}
}

//go:build linux

package commit

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/adapter/embodimentv1"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/inputledger"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/ownercheckpoint"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/ownercheckpoint/localowner"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
	"github.com/carloshgalves/cote-ai-simulation/internal/physicalhost"
)

// Internal evidence only: no raw canonical rows, private owner bytes, A1
// signature, operator proof, or absolute path escapes the test process.
type referenceE5Row struct {
	Name        string                     `json:"name"`
	Seed        int64                      `json:"seed"`
	Status      string                     `json:"status"`
	Trace       map[string][]int           `json:"realized_order,omitempty"`
	FaultEpoch  int                        `json:"fault_epoch,omitempty"`
	FaultStep   uint8                      `json:"fault_step,omitempty"`
	Mutation    string                     `json:"mutation,omitempty"`
	ChangedRows int64                      `json:"changed_rows,omitempty"`
	Commits     int                        `json:"commits"`
	Before      []referenceE1ManifestEntry `json:"before,omitempty"`
	After       []referenceE1ManifestEntry `json:"after,omitempty"`
	Difference  string                     `json:"literal_difference,omitempty"`
	Guard       string                     `json:"guard,omitempty"`
}

type referenceE5Report struct {
	SHA          string           `json:"sha"`
	Status       string           `json:"status"`
	RunID        string           `json:"run_id"`
	GenesisHash  string           `json:"genesis_hash"`
	PolicyHash   string           `json:"policy_hash"`
	CutHashes    [2][2]string     `json:"signed_cut_and_signature_sha256"`
	Experiments  []referenceE5Row `json:"experiments"`
	Restrictions []string         `json:"restrictions"`
}

func referenceE5Hash(raw []byte) string {
	h := sha256.Sum256(raw)
	return hex.EncodeToString(h[:])
}

func referenceE5Evidence(t *testing.T, report *referenceE5Report) {
	t.Helper()
	report.Status = "MATRIX_PASS"
	if t.Failed() || len(report.Experiments) == 0 {
		report.Status = "MATRIX_FAIL"
	}
	for _, row := range report.Experiments {
		if row.Status != "BYTE_EQUAL" && row.Status != "FAIL_CLOSED" {
			report.Status = "MATRIX_FAIL"
			t.Errorf("experiment %s did not finish: %s", row.Name, row.Status)
		}
	}
	if report.Status != "MATRIX_PASS" {
		t.Error("E5 matrix did not fully pass")
	}
	path := os.Getenv("CSF_E5_EVIDENCE_PATH")
	if path == "" {
		return
	}
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Error(err)
		return
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Error(err)
		return
	}
	_, writeErr := f.Write(append(raw, '\n'))
	if err := f.Close(); err != nil {
		t.Error(err)
	}
	if writeErr != nil {
		t.Error(writeErr)
	}
}

func referenceE5Case(t *testing.T, report *referenceE5Report, row referenceE5Row, run func(*testing.T, *referenceE5Row)) {
	t.Helper()
	row.Status = "NOT_COMPLETED"
	ok := t.Run(row.Name, func(t *testing.T) { run(t, &row) })
	if !ok {
		row.Status = "FAIL"
	}
	report.Experiments = append(report.Experiments, row)
}

func referenceE5Run(t *testing.T, host physicalhost.ProducerHost, plans [2]physicalhost.ProducerPlan, cuts *[2]embodimentv1.ApprovedCut, order *referenceOrder, epoch int, step uint8, baseline *ReferenceD3Proof) (string, string, ReferenceD3Proof) {
	t.Helper()
	ctx := context.Background()
	path, owner := referenceE2Paths(t)
	cfg := ReferenceD2Config{Host: host, EpochPlans: plans, frozenSupervisedCuts: cuts, FailPhysicalAtEpoch: epoch, FailPhysicalAfterStep: step}
	opts := ReferenceOptions{order: order}
	if epoch != 0 {
		if _, err := RunReferenceD2(ctx, path, owner, bundlePath, cfg, opts); err == nil || !strings.Contains(err.Error(), "CommitPhysical") {
			t.Fatalf("missing exact injected physical rollback: %v", err)
		}
		if baseline == nil {
			t.Fatal("recovery missing baseline")
		}
		referenceD2AssertPhysicalPrefix(t, path, baseline.RunID, epoch-1)
		if err := referenceE2RecoverPhysical(ctx, path, host.Bundle, *cuts, epoch); err != nil {
			t.Fatal(err)
		}
		p, err := AuditReferenceD3(ctx, path, host.Bundle, baseline.d2)
		if err != nil {
			t.Fatal(err)
		}
		return path, owner, p
	}
	p, err := RunReferenceD3(ctx, path, owner, bundlePath, cfg, opts)
	if err != nil {
		t.Fatal(err)
	}
	if p.Status != "RUN_LOCAL_10D3" || p.Matrix.ReplayCommits != 9 {
		t.Fatal("not a completed authenticated D3 run")
	}
	if _, err := p.PublicReport(); err != nil {
		t.Fatal(err)
	}
	return path, owner, p
}

func referenceE5NoOp(t *testing.T, path string, proof ReferenceD3Proof, host physicalhost.ProducerHost, cuts [2]embodimentv1.ApprovedCut) {
	t.Helper()
	ctx := context.Background()
	if err := referenceE2RecoverPhysical(ctx, path, host.Bundle, cuts, 2); !errors.Is(err, errReferenceE2AlreadyCommitted) {
		t.Fatalf("duplicate terminal was not rejected: %v", err)
	}
	store, err := sqlite.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	task := referenceC2Decode32(t, proof.d2.C4.Communication.DeliveryTaskID)
	if _, err := store.DeliverCommunicationPerception(ctx, task); err != nil {
		t.Fatal(err)
	}
	run := referenceC2Decode32(t, proof.RunID)
	snap := referenceC2Decode32(t, proof.d2.C4.Epistemic.SnapshotID)
	check, err := store.VerifySnapshotV1(ctx, run, snap, host.Bundle)
	if err != nil || check.TailCommits != 3 || check.OwnerWorkReady {
		t.Fatalf("invalid historical snapshot or unauthorized new owner work: %v", err)
	}
}

// Every mutation starts from an independently executed authenticated D3
// with the SAME signed pair. Canonical-but-damaged bytes (rather than DB
// corruption or SQL syntax errors) must be rejected by actual guards.
func referenceE5Mutate(t *testing.T, path string, proof ReferenceD3Proof, kind string) int64 {
	t.Helper()
	ctx := context.Background()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var result sql.Result
	mustExec := func(query string, args ...any) sql.Result {
		t.Helper()
		res, err := db.ExecContext(ctx, query, args...)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	switch kind {
	case "input_ledger", "decision_ledger", "event_store", "evidence_ledger", "causal_outbox":
		result = mustExec("UPDATE "+kind+" SET canonical=x'80' WHERE id=(SELECT id FROM "+kind+" ORDER BY id LIMIT 1)")
	case "genesis-policy":
		var raw []byte
		run := referenceC2Decode32(t, proof.RunID)
		if err := db.QueryRowContext(ctx, "SELECT canonical FROM genesis_records WHERE run_id=?", run[:]).Scan(&raw); err != nil {
			t.Fatal(err)
		}
		env, err := codec.StrictDecodeEnvelope(raw)
		if err != nil {
			t.Fatal(err)
		}
		fields := env.Value.([]any)
		fields[10] = bytes.Repeat([]byte{0xab}, 32)
		mutant, err := codec.CanonicalBytes(env.DomainTag, env.SchemaID, env.SchemaVersion, fields)
		if err != nil {
			t.Fatal(err)
		}
		result = mustExec("UPDATE genesis_records SET canonical=? WHERE run_id=?", mutant, run[:])
	case "snapshot":
		id := referenceC2Decode32(t, proof.d2.C4.Epistemic.SnapshotID)
		result = mustExec("UPDATE snapshots SET canonical=x'80' WHERE id=?", id[:])
	case "owner-receipt":
		id := referenceC2Decode32(t, proof.d2.C4.Epistemic.OwnerReceiptID)
		result = mustExec("UPDATE cognitive_owner_receipts SET signature=zeroblob(64) WHERE receipt_id=?", id[:])
	case "source-cursor":
		result = mustExec("UPDATE input_source_cursors SET next_seq=next_seq+1 WHERE source_id='physical.a1'")
	case "source-closure":
		// Select the final closure by its actual canonical identity, never
		// by the last SQLite row or a string search over binary values.
		rows, err := db.QueryContext(ctx, "SELECT id,canonical FROM input_ledger")
		if err != nil {
			t.Fatal(err)
		}
		var id, mutant []byte
		for rows.Next() {
			var key, raw []byte
			if err := rows.Scan(&key, &raw); err != nil {
				rows.Close()
				t.Fatal(err)
			}
			v, err := codec.StrictDecodePayload(raw)
			if err != nil {
				rows.Close()
				t.Fatal(err)
			}
			f, ok := v.([]any)
			if !ok || len(f) != 7 || f[2] != "physical.a1" || f[5] != true {
				continue
			}
			if id != nil {
				rows.Close()
				t.Fatal("ambiguous final closure")
			}
			id = bytes.Clone(key)
			f[5] = false
			mutant, err = codec.CanonicalPayload(f)
			if err != nil {
				rows.Close()
				t.Fatal(err)
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		if err := rows.Close(); err != nil {
			t.Fatal(err)
		}
		if len(id) != 32 || len(mutant) == 0 {
			t.Fatal("missing actual final A1 closure")
		}
		result = mustExec("UPDATE input_ledger SET canonical=? WHERE id=?", mutant, id)
	case "physical-source", "physical-signature", "event-visibility":
		id := referenceC2Decode32(t, proof.d2.PhysicalInputIDs[0])
		table := "input_ledger"
		if kind == "event-visibility" {
			id = referenceC2Decode32(t, proof.d2.PhysicalEventIDs[0])
			table = "event_store"
		}
		var raw []byte
		if err := db.QueryRowContext(ctx, "SELECT canonical FROM "+table+" WHERE id=?", id[:]).Scan(&raw); err != nil {
			t.Fatal(err)
		}
		v, err := codec.StrictDecodePayload(raw)
		if err != nil {
			t.Fatal(err)
		}
		f := v.([]any)
		if kind == "event-visibility" {
			if len(f) != 19 || f[3] != "physical.advanced" {
				t.Fatal("wrong physical event")
			}
			f[16] = "public.unsafe"
		} else {
			if len(f) != 13 || f[2] != "physical.a1" {
				t.Fatal("wrong physical input")
			}
			p := f[7].([]any)
			index := 2
			if kind == "physical-signature" {
				index = 3
			}
			payload := bytes.Clone(p[index].([]byte))
			payload[len(payload)-1] ^= 1
			p[index] = payload
		}
		mutant, err := codec.CanonicalPayload(f)
		if err != nil || bytes.Equal(raw, mutant) {
			t.Fatal("mutation did not change canonical bytes", err)
		}
		result = mustExec("UPDATE "+table+" SET canonical=? WHERE id=?", mutant, id[:])
	default:
		t.Fatal("unregistered mutation")
	}
	if err != nil {
		t.Fatal(err)
	}
	count, err := result.RowsAffected()
	if err != nil || count != 1 {
		t.Fatalf("%s must change exactly one genuine row; rows=%d err=%v", kind, count, err)
	}
	return count
}

func TestReferenceE5CrossMatrix(t *testing.T) {
	ctx := context.Background()
	host, plans, cuts := referenceE2SignedPair(t)
	g, err := buildReferenceGenesis(host.Bundle)
	if err != nil {
		t.Fatal(err)
	}
	revision, err := exec.Command("git", "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal("matrix requires checkout identity", err)
	}
	report := referenceE5Report{
		SHA: strings.TrimSpace(string(revision)), RunID: hexRun(g.RunID),
		GenesisHash: referenceE5Hash(g.CanonicalBytes), PolicyHash: hex.EncodeToString(host.Bundle.CodecPolicyHash[:]),
		Restrictions: []string{"local experimental evidence; no external release gate", "A1 epoch dependency order preserved; signed source bytes including wall_time identical", "seed and realized scheduling order host-only; fresh C3 operator enrollment not persisted", "positive fork is C4; post-D3 physical fork remains unsupported"},
	}
	if len(report.SHA) != 40 {
		t.Fatal("invalid checkout SHA")
	}
	for i, cut := range cuts {
		raw, sig, _, err := cut.CanonicalProof()
		if err != nil {
			t.Fatal(err)
		}
		report.CutHashes[i] = [2]string{referenceE5Hash(raw), referenceE5Hash(sig)}
	}
	defer referenceE5Evidence(t, &report)
	path, owner, baselineProof := referenceE5Run(t, host, plans, &cuts, nil, 0, 0, nil)
	base := referenceE2Capture(t, path, owner)
	referenceE5Case(t, &report, referenceE5Row{Name: "normal"}, func(t *testing.T, row *referenceE5Row) {
		referenceE5NoOp(t, path, baselineProof, host, cuts)
		after := referenceE2Capture(t, path, owner)
		row.Before, row.After = base.Manifest(), after.Manifest()
		row.Commits = len(after.Databases["causal"].Tables["committed_cycles"].Rows)
		if err := referenceE1Compare(base, after); err != nil {
			row.Difference = err.Error()
			t.Fatal(err)
		}
		row.Status = "BYTE_EQUAL"
	})
	for _, spec := range []struct {
		name    string
		seed    int64
		reverse bool
		epoch   int
		step    uint8
	}{
		{"reverse", 0, true, 0, 0},
		{"seed-1", 1, false, 0, 0},
		{"seed-42", 42, false, 0, 0},
		{"seed-2026", 2026, false, 0, 0},
		{"seed-144-fault-1", 144, false, 1, 3},
		{"seed-999-fault-2", 999, false, 2, 7},
	} {
		referenceE5Case(t, &report, referenceE5Row{Name: spec.name, Seed: spec.seed, FaultEpoch: spec.epoch, FaultStep: spec.step}, func(t *testing.T, row *referenceE5Row) {
			order := &referenceOrder{seed: spec.seed, reverse: spec.reverse, trace: map[string][]int{}}
			db, owner, p := referenceE5Run(t, host, plans, &cuts, order, spec.epoch, spec.step, &baselineProof)
			row.Trace = order.trace
			for _, phase := range []string{"ingress-declarations", "worker-responses", "abort-validator-inputs", "contest-validator-inputs", "candidate-completions", "validator-assessments"} {
				indices := row.Trace[phase]
				if len(indices) != 2 || indices[0]+indices[1] != 1 || indices[0] == indices[1] {
					t.Fatalf("phase %s not genuinely permuted: %v", phase, indices)
				}
				if spec.reverse && !reflect.DeepEqual(indices, []int{1, 0}) {
					t.Fatal("reverse control did not reverse phase", phase)
				}
			}
			if p.Matrix.ReplayCommits != 9 {
				t.Fatal("perturbed run incomplete")
			}
			referenceE5NoOp(t, db, p, host, cuts)
			after := referenceE2Capture(t, db, owner)
			row.Commits = len(after.Databases["causal"].Tables["committed_cycles"].Rows)
			row.Before, row.After = base.Manifest(), after.Manifest()
			if err := referenceE1Compare(base, after); err != nil {
				row.Difference = err.Error()
				t.Fatal(err)
			}
			row.Status = "BYTE_EQUAL"
		})
	}
	// Authentic A1 signature verification is also exercised directly, so
	// rejection cannot be attributed only to a stale outer ledger digest.
	referenceE5Case(t, &report, referenceE5Row{Name: "signature-cryptographic", Mutation: "flip one signature bit"}, func(t *testing.T, row *referenceE5Row) {
		reg, err := host.Bundle.PhysicalRegistration()
		if err != nil {
			t.Fatal(err)
		}
		raw, sig, _, err := cuts[0].CanonicalProof()
		if err != nil {
			t.Fatal(err)
		}
		original, err := embodimentv1.DecodeSignedCut(raw, sig)
		if err != nil || embodimentv1.VerifySignedCut(original, reg, g.RunID, g.Digest) != nil {
			t.Fatal("positive cryptographic control missing")
		}
		sig[0] ^= 1
		altered, err := embodimentv1.DecodeSignedCut(raw, sig)
		if err != nil {
			t.Fatal("mutation must remain syntactically valid", err)
		}
		if err := embodimentv1.VerifySignedCut(altered, reg, g.RunID, g.Digest); err == nil {
			t.Fatal("invalid physical signature authenticated")
		}
		row.Guard = "VerifySignedCut"
		row.Status = "FAIL_CLOSED"
	})
	for _, kind := range []string{"input_ledger", "decision_ledger", "event_store", "evidence_ledger", "causal_outbox", "genesis-policy", "snapshot", "owner-receipt", "source-cursor", "source-closure", "physical-source", "physical-signature", "event-visibility"} {
		referenceE5Case(t, &report, referenceE5Row{Name: "corrupt-" + kind, Mutation: kind}, func(t *testing.T, row *referenceE5Row) {
			db, owner, p := referenceE5Run(t, host, plans, &cuts, nil, 0, 0, nil)
			clean := referenceE2Capture(t, db, owner)
			if err := referenceE1Compare(base, clean); err != nil {
				t.Fatal(err)
			}
			row.ChangedRows = referenceE5Mutate(t, db, p, kind)
			damaged := referenceE2Capture(t, db, owner)
			diff := referenceE1Compare(clean, damaged)
			if diff == nil {
				t.Fatal("mutation was invisible to the E1 oracle")
			}
			row.Difference = diff.Error()
			row.Before = damaged.Manifest()
			store, err := sqlite.Open(ctx, db)
			if err != nil {
				t.Fatal("expected schema-valid but corrupt authority, not a failed open", err)
			}
			run := referenceC2Decode32(t, p.RunID)
			snap := referenceC2Decode32(t, p.d2.C4.Epistemic.SnapshotID)
			_, err = store.VerifySnapshotV1(ctx, run, snap, host.Bundle)
			if err == nil {
				store.Close()
				t.Fatal("corrupted run acquired verified snapshot authority")
			}
			row.Guard = "VerifySnapshotV1"
			if kind != "snapshot" && kind != "owner-receipt" {
				if _, err := store.ReplayClock(ctx, run, host.Bundle); err == nil {
					store.Close()
					t.Fatal("corrupted causal input was accepted by ReplayClock")
				}
				row.Guard += "+ReplayClock"
			}
			// Exercise the actual recovery entrypoint, not solely a read-only
			// audit. Corrupt ingress may fail while opening the ledger; if
			// that succeeds, a registered resume must itself fail.
			ledger, ledgerErr := inputledger.Open(ctx, domain.ID(run), host.Bundle, store)
			if ledgerErr == nil {
				resumed, resumeErr := New(store, host.Bundle, referenceE4Policies(t, host.Bundle)).ResumeRegisteredWorkV1(ctx, ledger, run, snap)
				if resumeErr == nil || resumed.Continued {
					store.Close()
					t.Fatal("corrupt authority permitted registered resume")
				}
				row.Guard += "+ResumeRegisteredWorkV1"
			} else {
				row.Guard += "+InputLedger.Open"
			}
			if err := store.Close(); err != nil {
				t.Fatal(err)
			}
			if _, err := AuditReferenceD3(ctx, db, host.Bundle, p.d2); err == nil {
				t.Fatal("corrupted source obtained D3 disclosure certificate")
			}
			after := referenceE2Capture(t, db, owner)
			row.After = after.Manifest()
			row.Commits = len(after.Databases["causal"].Tables["committed_cycles"].Rows)
			if err := referenceE1Compare(damaged, after); err != nil {
				t.Fatalf("denied corruption mutated durable authority: %v", err)
			}
			row.Status = "FAIL_CLOSED"
		})
	}

	// Actual owner storage denial at a newly signed current R9 cut.
	referenceE5Case(t, &report, referenceE5Row{Name: "corrupt-private-owner", Mutation: "owner B opaque checkpoint"}, func(t *testing.T, row *referenceE5Row) {
		db, owner, p := referenceE5Run(t, host, plans, &cuts, nil, 0, 0, nil)
		run := referenceC2Decode32(t, p.RunID)
		store, err := sqlite.Open(ctx, db)
		if err != nil {
			t.Fatal(err)
		}
		defer store.Close()
		state, err := store.LoadClock(ctx, run)
		if err != nil || state.Revision != 9 {
			t.Fatal("missing current D3 clock", err)
		}
		count := uint64(len(base.Databases["causal"].Tables["evidence_ledger"].Rows))
		var checkpoints []sqlite.SnapshotCheckpoint
		var receipt [32]byte
		for _, actor := range []string{"a", "b"} {
			file := owner
			if actor == "a" {
				file += ".actor-a"
			}
			o, err := localowner.Open(ctx, file, referenceC2SyntheticSigner(run, actor))
			if err != nil {
				t.Fatal(err)
			}
			scope := ownercheckpoint.Scope{Run: run, Namespace: "school", Actor: actor}
			gate, err := ownercheckpoint.NewGate(o, store, host.Bundle)
			if err != nil {
				t.Fatal(err)
			}
			cp := sqlite.SnapshotCheckpoint{ActorNamespace: "school", ActorID: actor, StoreID: "reference-e5-" + actor, Producer: "reference-e5", ProducerVersion: 1, SchemaVersion: 1, Content: []byte("synthetic-e5-" + actor), CoversInstant: int64(state.Instant), CoversRevision: 9, CoversEvidenceCursor: count}
			receipt, err = gate.Stage(ctx, scope, cp)
			if err != nil {
				t.Fatal(err)
			}
			checkpoints = append(checkpoints, cp)
			if err := o.Close(); err != nil {
				t.Fatal(err)
			}
		}
		snap, err := store.CreateSnapshotV1AtRevision(ctx, run, host.Bundle, checkpoints, 9)
		if err != nil {
			t.Fatal(err)
		}
		o, err := localowner.Open(ctx, owner, referenceC2SyntheticSigner(run, "b"))
		if err != nil {
			t.Fatal(err)
		}
		scope := ownercheckpoint.Scope{Run: run, Namespace: "school", Actor: "b"}
		gate, err := ownercheckpoint.NewGate(o, store, host.Bundle)
		if err != nil {
			t.Fatal(err)
		}
		if err := gate.RestoreAndAcknowledge(ctx, scope, snap.ID, receipt); err != nil {
			t.Fatal("clean current owner does not restore", err)
		}
		if err := o.Close(); err != nil {
			t.Fatal(err)
		}
		clean := referenceE2Capture(t, db, owner)
		raw, err := sql.Open("sqlite", owner)
		if err != nil {
			t.Fatal(err)
		}
		result, err := raw.ExecContext(ctx, "UPDATE checkpoints SET content=x'80' WHERE covers_revision=9")
		if err != nil {
			t.Fatal(err)
		}
		row.ChangedRows, err = result.RowsAffected()
		if err != nil || row.ChangedRows != 1 {
			t.Fatal("mutation missed owner B at R9", err)
		}
		if err := raw.Close(); err != nil {
			t.Fatal(err)
		}
		damaged := referenceE2Capture(t, db, owner)
		diff := referenceE1Compare(clean, damaged)
		if diff == nil {
			t.Fatal("mutation did not change owner authority")
		}
		row.Difference = diff.Error()
		row.Before = damaged.Manifest()
		o, err = localowner.Open(ctx, owner, referenceC2SyntheticSigner(run, "b"))
		if err != nil {
			t.Fatal(err)
		}
		defer o.Close()
		gate, err = ownercheckpoint.NewGate(o, store, host.Bundle)
		if err != nil {
			t.Fatal(err)
		}
		calls := 0
		if err := gate.RestoreAndAcknowledge(ctx, scope, snap.ID, receipt); err == nil {
			t.Fatal("corrupt local owner ACK")
		}
		if err := gate.RunWork(ctx, scope, snap.ID, receipt, func(context.Context) error { calls++; return nil }); !errors.Is(err, ownercheckpoint.ErrNotAcknowledged) || calls != 0 {
			t.Fatal("corrupt owner acquired WORK")
		}
		if err := o.Close(); err != nil {
			t.Fatal(err)
		}
		after := referenceE2Capture(t, db, owner)
		if err := referenceE1Compare(damaged, after); err != nil {
			t.Fatal(err)
		}
		row.After = after.Manifest()
		row.Commits = 9
		row.Guard = "RestoreAndAcknowledge+RunWork"
		row.Status = "FAIL_CLOSED"
	})
	t.Logf("E5 completed %d experiments on %s", len(report.Experiments), report.SHA)
}

// RED guard: a skip or a pass without recorded completion cannot be labeled
// valid by the JSON consumer. CI verifies the exact matrix inventory too.
func TestReferenceE5ScheduleDeterministic(t *testing.T) {
	a := &referenceOrder{seed: 42, trace: map[string][]int{}}
	b := &referenceOrder{seed: 42, trace: map[string][]int{}}
	values := []int{0, 1, 2, 3, 4, 5, 6, 7}
	copyValues := append([]int(nil), values...)
	referencePermute(a, "worker", values)
	referencePermute(b, "unrelated", []int{0, 1})
	referencePermute(b, "worker", copyValues)
	if !reflect.DeepEqual(values, copyValues) || !reflect.DeepEqual(a.trace["worker"], b.trace["worker"]) {
		t.Fatal("phase order depends on prior unrelated host activity")
	}
}

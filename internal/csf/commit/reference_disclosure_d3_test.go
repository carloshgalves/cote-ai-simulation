//go:build linux

package commit

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/adapter/embodimentv1"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
	"github.com/carloshgalves/cote-ai-simulation/internal/physicalhost"
	_ "modernc.org/sqlite"
)

func referenceD3Paths(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	private := filepath.Join(dir, "private")
	if err := os.Mkdir(private, 0700); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(dir, "causal.db"), filepath.Join(private, "owner-b.db")
}

func referenceD3Run(t *testing.T, host physicalhost.ProducerHost,
	plans [2]physicalhost.ProducerPlan, options ReferenceOptions) (string, ReferenceD3Proof) {
	t.Helper()
	db, owner := referenceD3Paths(t)
	got, err := RunReferenceD3(context.Background(), db, owner, bundlePath,
		ReferenceD2Config{Host: host, EpochPlans: plans}, options)
	if err != nil {
		t.Fatalf("D3 real supervised run: %v", err)
	}
	return db, got
}

func TestReferenceD3RealPythonPrivateEventAndLiveObservatory(t *testing.T) {
	host, plans := referenceD2TestHost(t)
	db, got := referenceD3Run(t, host, plans, ReferenceOptions{})
	m := got.Matrix
	if m.PhysicalEvents != 2 || m.PhysicalPerceptionTasks != 0 ||
		m.ActorAAddressed == 0 || m.ActorBAddressed == 0 ||
		!m.RecipientBClaimOnly || !m.ActorPhysicalDenied || !m.PublicPhysicalDenied ||
		m.PublicPublications != 0 || m.ReplayCommits != 9 || m.SnapshotTailCommits != 3 ||
		!m.FiveLedgerPrefixStable || !m.ReadersDidNotWrite {
		t.Fatalf("D3 physical/cognitive/public disclosure matrix incomplete: %+v", m)
	}
	public, err := got.PublicReport()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(public)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "physical.advanced") ||
		strings.Contains(string(raw), "npc.0001") ||
		strings.Contains(string(raw), "world_hash") ||
		strings.Contains(string(raw), "body_hours") ||
		strings.Contains(string(raw), "private_key") ||
		strings.Contains(string(raw), "seed") ||
		strings.Contains(string(raw), "source_raw") {
		t.Fatalf("D3 public output leaked physical/state authority: %s", raw)
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatal(err)
	}
	if len(obj) != 3 || obj["status"] != "RUN_LOCAL_10D3" {
		t.Fatalf("D3 report lacks strict public allowlist: %s", raw)
	}
	if strings.Contains(string(raw), "actor_a_addressed") || strings.Contains(string(raw), "actor_b_addressed") {
		t.Fatalf("public certificate exposed recipient-specific evidence counts: %s",raw)
	}
	altered := got
	altered.Matrix.PublicPublications++
	if _, err := altered.PublicReport(); err == nil {
		t.Fatal("altered disclosure matrix was publicly attested")
	}
	ctx := context.Background()
	id, err := referenceC4DecodeID(got.RunID)
	if err != nil {
		t.Fatal(err)
	}
	pub, err := sqlite.OpenPublicObservatory(ctx, db, host.Bundle)
	if err != nil {
		t.Fatal(err)
	}
	foreign := id
	foreign[0] ^= 0x80
	if _, err := pub.ReadPublic(ctx, foreign); err == nil {
		pub.Close()
		t.Fatal("foreign run was available to public Observatory")
	}
	if err := pub.Close(); err != nil {
		t.Fatal(err)
	}
	ro, err := openReferenceReadOnlyDB(db)
	if err != nil {
		t.Fatal(err)
	}
	dispatches, err := referenceD3ActorDispatches(ctx, ro, id)
	_ = ro.Close()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sqlite.OpenPOVObservatory(ctx, db, host.Bundle, id, dispatches[1], [2]string{"school", "a"}); err == nil {
		t.Fatal("B dispatch was accepted as A actor capability")
	}
}

func TestReferenceD3AdversarialEventTaskAndHolderMutationsFailClosed(t *testing.T) {
	host, plans := referenceD2TestHost(t)
	db, got := referenceD3Run(t, host, plans, ReferenceOptions{})
	ctx := context.Background()
	database, err := sql.Open("sqlite", db)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	original, err := got.PublicReport()
	if err != nil {
		t.Fatal(err)
	}
	// A valid B-only KnowledgeInput must never be relabeled as actor A.
	claim, err := referenceC4DecodeID(got.d2.C4.Communication.KnowledgeInputID)
	if err != nil {
		t.Fatal(err)
	}
	res, err := database.ExecContext(ctx, "UPDATE evidence_routes SET holder_id='a' WHERE record_id=?", claim[:])
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		t.Fatalf("D3 absent B claim route, rows=%d", n)
	}
	if _, err := AuditReferenceD3(ctx, db, host.Bundle, got.d2); err == nil {
		t.Fatal("relabeled B route was accepted by D3 audit")
	}
	if _, err := database.ExecContext(ctx, "UPDATE evidence_routes SET holder_id='b' WHERE record_id=?", claim[:]); err != nil {
		t.Fatal(err)
	}
	if _, err := AuditReferenceD3(ctx, db, host.Bundle, got.d2); err != nil {
		t.Fatalf("restored immutable source failed: %v", err)
	}

	// Force an existing generic task to reference a real physical Event:
	// this tests persisted outbox, not a made-up ContextBoundary input.
	var taskID, taskRaw []byte
	if err := database.QueryRowContext(ctx, "SELECT id,canonical FROM causal_outbox ORDER BY id LIMIT 1").Scan(&taskID, &taskRaw); err != nil {
		t.Fatal(err)
	}
	v, err := codec.StrictDecodePayload(taskRaw)
	if err != nil {
		t.Fatal(err)
	}
	fields, ok := v.([]any)
	if !ok || len(fields) != 10 {
		t.Fatal("invalid original task fixture")
	}
	physical, err := referenceC4DecodeID(got.d2.PhysicalEventIDs[0])
	if err != nil {
		t.Fatal(err)
	}
	fields[3] = physical[:]
	forged, err := codec.CanonicalPayload(fields)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(forged, taskRaw) {
		t.Fatal("fixture did not corrupt task")
	}
	if _, err := database.ExecContext(ctx, "UPDATE causal_outbox SET canonical=? WHERE id=?", forged, taskID); err != nil {
		t.Fatal(err)
	}
	if _, err := AuditReferenceD3(ctx, db, host.Bundle, got.d2); err == nil {
		t.Fatal("physical generic PerceptionTask accepted")
	}
	if _, err := database.ExecContext(ctx, "UPDATE causal_outbox SET canonical=? WHERE id=?", taskRaw, taskID); err != nil {
		t.Fatal(err)
	}
	if _, err := AuditReferenceD3(ctx, db, host.Bundle, got.d2); err != nil {
		t.Fatalf("restored outbox failed: %v", err)
	}

	// Tamper the event's owner classification. There must be no fallback
	// from ENGINE_ONLY Event to public disclosure even with the same event ID.
	var eventRaw []byte
	if err := database.QueryRowContext(ctx, "SELECT canonical FROM event_store WHERE id=?", physical[:]).Scan(&eventRaw); err != nil {
		t.Fatal(err)
	}
	v, err = codec.StrictDecodePayload(eventRaw)
	if err != nil {
		t.Fatal(err)
	}
	event := v.([]any)
	event[16] = "public.unsafe"
	rewritten, err := codec.CanonicalPayload(event)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, "UPDATE event_store SET canonical=? WHERE id=?", rewritten, physical[:]); err != nil {
		t.Fatal(err)
	}
	if _, err := AuditReferenceD3(ctx, db, host.Bundle, got.d2); err == nil {
		t.Fatal("forged event type bypassed public audit")
	}
	if _, err := database.ExecContext(ctx, "UPDATE event_store SET canonical=? WHERE id=?", eventRaw, physical[:]); err != nil {
		t.Fatal(err)
	}
	if next, err := AuditReferenceD3(ctx, db, host.Bundle, got.d2); err != nil {
		t.Fatalf("D3 could not recover original clean audit: %v", err)
	} else if published, err := next.PublicReport(); err != nil || !reflect.DeepEqual(original.Disclosure, published.Disclosure) {
		t.Fatalf("D3 recovered disclosure differs from original public certificate: %v", err)
	}
}

func TestReferenceD3ImmutablePrefixesUnderSourceWorkerPermutation(t *testing.T) {
	host, plans := referenceD2TestHost(t)
	genesis, err := buildReferenceGenesis(host.Bundle)
	if err != nil {
		t.Fatal(err)
	}
	host.GenesisCanonical = genesis.CanonicalBytes
	var cuts [2]embodimentv1.ApprovedCut
	for i := range cuts {
		h := host
		h.Epoch = uint64(i + 1)
		h.Plan = plans[i]
		cuts[i], err = physicalhost.RunSupervisedPhysical(context.Background(), h)
		if err != nil {
			t.Fatalf("real A1 cut %d: %v", i+1, err)
		}
	}
	host.GenesisCanonical = nil
	run := func(options ReferenceOptions) ReferenceD3Proof {
		t.Helper()
		db, owner := referenceD3Paths(t)
		p, err := RunReferenceD3(context.Background(), db, owner, bundlePath,
			ReferenceD2Config{Host: host, EpochPlans: plans, frozenSupervisedCuts: &cuts}, options)
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	a := run(ReferenceOptions{})
	b := run(ReferenceOptions{ReverseSources: true, ReverseResponses: true})
	if a.RunID != b.RunID || a.d2.FinalWorldHash != b.d2.FinalWorldHash ||
		!reflect.DeepEqual(a.d2.FiveLedgerDigests, b.d2.FiveLedgerDigests) ||
		!reflect.DeepEqual(a.d2.PhysicalEventIDs, b.d2.PhysicalEventIDs) ||
		!reflect.DeepEqual(a.Matrix, b.Matrix) ||
		a.d2.C4.Epistemic.SnapshotID != b.d2.C4.Epistemic.SnapshotID {
		t.Fatal("same two real signed A1 cuts changed full physical/cognitive prefix or privacy audit under worker/source permutation")
	}
	one, err := a.PublicReport()
	if err != nil {
		t.Fatal(err)
	}
	two, err := b.PublicReport()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(one, two) {
		t.Fatal("D3 allowlisted public report is not permutation invariant")
	}
}

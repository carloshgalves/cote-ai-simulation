package commit

import (
	"bytes"
	"context"
	"database/sql"
	"reflect"
	"strings"
	"testing"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/inputledger"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/observatory"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
	_ "modernc.org/sqlite"
)

func o3FindDispatch(t *testing.T, ctx context.Context, path string, run [32]byte, actor [2]string, revision uint64) ([32]byte, bool) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, "SELECT id,canonical FROM input_ledger ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var found [32]byte
	okFound := false
	for rows.Next() {
		var id, raw []byte
		if err := rows.Scan(&id, &raw); err != nil {
			t.Fatal(err)
		}
		v, err := codec.StrictDecodePayload(raw)
		if err != nil {
			continue
		}
		f, ok := v.([]any)
		if !ok || len(f) != 11 || len(id) != 32 || f[8] != revision {
			continue
		}
		if !bytes.Equal(f[1].([]byte), run[:]) {
			continue
		}
		holder, ok := f[6].([]any)
		if !ok || len(holder) != 2 || holder[0] != actor[0] || holder[1] != actor[1] {
			continue
		}
		if okFound {
			t.Fatal("ambiguous dispatch for same actor and causal revision")
		}
		copy(found[:], id)
		okFound = true
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return found, okFound
}

func o3MintActorB(t *testing.T, ctx context.Context, store *sqlite.Store, ledger *inputledger.Ledger, bundle *policy.Bundle, actor [2]string) [32]byte {
	t.Helper()
	// This fixture creates an independently valid holder-bound SlotDispatch
	// for B in the same run/revision as A; B has no addressed evidence.
	_, _, _, _, _, run := ledger.Snapshot()
	state, err := store.LoadClock(ctx, [32]byte(run))
	if err != nil {
		t.Fatal(err)
	}
	root := validGenesis(t, bundle)
	manifest := root
	ordinal := uint64(state.LastOrdinal + 1)
	coordinate := domain.EligibilityCoordinate{NotBeforeInstant: state.Instant, NotBeforeCycleOrdinal: domain.CycleOrdinal(ordinal)}
	origin := []any{uint64(2), []any{manifest.ID[:], manifest.Digest[:]}}
	round, err := ledger.DeclareRound(ctx, "schedule", actor, []string{"act"}, coordinate, origin, 0, 1, 0, []any{uint64(0)})
	if err != nil {
		t.Fatal(err)
	}
	deriver := identity.New(bundle)
	cycle, _, err := deriver.Sum("cote.csf.id.cycle", "cote.csf.schema.cycle-id-preimage", 1, []any{run[:], []any{int64(state.Instant), ordinal}})
	if err != nil {
		t.Fatal(err)
	}
	dispatchID, _, err := deriver.Sum("cote.csf.id.dispatch", "cote.csf.schema.dispatch-id-preimage", 1, []any{run[:], cycle[:], round.ID[:], "act", uint64(0)})
	if err != nil {
		t.Fatal(err)
	}
	dispatch := inputledger.Dispatch{ID: domain.ID(dispatchID), RunID: run, CycleID: domain.ID(cycle), RoundID: round.ID,
		SlotID: "act", Actor: actor, Eligibility: coordinate, BaseRevision: state.Revision, EffectiveAt: state.Instant}
	if err := ledger.RegisterDispatch(ctx, dispatch, inputledger.CycleBinding{ID: domain.ID(cycle), BaseRevision: state.Revision, Instant: state.Instant}); err != nil {
		t.Fatal(err)
	}
	return [32]byte(dispatchID)
}

func TestCSFRV19O3POVStrictlyRecipientScopedAndReadonly(t *testing.T) {
	ctx := context.Background()
	c, store, ledger, cycle, fence, path := singleResourceFixture(t)
	if _, err := c.CommitSingleResource(ctx, ledger, cycle, fence); err != nil {
		t.Fatal(err)
	}
	_, _, _, _, _, runID := ledger.Snapshot()
	run := [32]byte(runID)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	task := findTerminalRoundTask(t, ctx, db)
	delivered, err := store.DeliverRoundPerception(ctx, task, c.bundle)
	if err != nil {
		t.Fatal(err)
	}
	// Construct the legitimate next-dispatch capability of A via the
	// existing test fixture; only then prepare a separate B capability.
	_ = resourceActorInbox(t, ctx, store, path, ledger, c.bundle)
	a := [2]string{"school", "a"}
	b := [2]string{"school", "b"}
	dispatchA, ok := o3FindDispatch(t, ctx, path, run, a, 1)
	if !ok {
		t.Fatal("no authenticated revision-one dispatch for A")
	}
	dispatchB := o3MintActorB(t, ctx, store, ledger, c.bundle, b)
	baseline := o1WholeDatabaseDigest(t, path)
	povA, err := sqlite.OpenPOVObservatory(ctx, path, c.bundle, run, dispatchA, a)
	if err != nil {
		t.Fatal(err)
	}
	defer povA.Close()
	povB, err := sqlite.OpenPOVObservatory(ctx, path, c.bundle, run, dispatchB, b)
	if err != nil {
		t.Fatal(err)
	}
	defer povB.Close()
	var _ observatory.POVQueries = povA
	// The publicly visible method set is deliberately not a general reader
	// capable of invoking ReadRun, ReadPrivilegedReport or write operations.
	methods := reflect.TypeOf(povA)
	if methods.NumMethod() != 2 || methods.Method(0).Name != "Close" || methods.Method(1).Name != "ReadPOV" {
		t.Fatalf("POV adapter unexpectedly exposes additional capability: %s", methods)
	}
	viewA, err := povA.ReadPOV(ctx)
	if err != nil {
		t.Fatal(err)
	}
	viewB, err := povB.ReadPOV(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if viewA.Actor != a || viewB.Actor != b || viewA.Run != run || viewB.Run != run ||
		viewA.ThroughRevision != 1 || viewB.ThroughRevision != 1 {
		t.Fatalf("incorrect run/holder/cutoff A=%+v B=%+v", viewA, viewB)
	}
	if len(viewA.Entries) != 1 || len(viewB.Entries) != 0 {
		t.Fatalf("private A evidence leaked to B: A=%d B=%d", len(viewA.Entries), len(viewB.Entries))
	}
	e := viewA.Entries[0]
	if e.ObservationID != delivered.Observation.Identity || e.KnowledgeInputID != delivered.Input.Identity ||
		!bytes.Equal(e.ObservationCanonical, delivered.Observation.Canonical) ||
		!bytes.Equal(e.KnowledgeCanonical, delivered.Input.Canonical) || e.Channel != "school.round.lifecycle.v1" {
		t.Fatalf("POV diverged from delivered evidence: %+v", e)
	}
	for i := 0; i < 20; i++ {
		nextA, err := povA.ReadPOV(ctx)
		if err != nil || !reflect.DeepEqual(viewA, nextA) {
			t.Fatalf("POV A repetition %d: %v", i, err)
		}
		nextB, err := povB.ReadPOV(ctx)
		if err != nil || !reflect.DeepEqual(viewB, nextB) {
			t.Fatalf("POV B repetition %d: %v", i, err)
		}
	}
	if got := o1WholeDatabaseDigest(t, path); got != baseline {
		t.Fatal("POV A/B observation query changed causal or epistemic authorities")
	}
	// Mutating caller-owned slices cannot mutate the verified next result.
	viewA.Entries[0].ObservationCanonical[0] ^= 0x7f
	again, err := povA.ReadPOV(ctx)
	if err != nil || !bytes.Equal(again.Entries[0].ObservationCanonical, delivered.Observation.Canonical) {
		t.Fatal("query reused mutable user-owned evidence slice", err)
	}
	if _, err := sqlite.OpenPOVObservatory(ctx, path, c.bundle, run, dispatchA, b); err == nil {
		t.Fatal("A's dispatch authorized B to view knowledge")
	}
	if _, err := sqlite.OpenPOVObservatory(ctx, path, c.bundle, [32]byte{0x87}, dispatchA, a); err == nil {
		t.Fatal("foreign run accepted A's dispatch")
	}
	foreignDispatch := dispatchA
	foreignDispatch[0] ^= 0x87
	if _, err := sqlite.OpenPOVObservatory(ctx, path, c.bundle, run, foreignDispatch, a); err == nil {
		t.Fatal("fabricated dispatch authorized A")
	}
	t.Logf("KNOWLEDGE-BOUNDARY-AUDIT O3 run=%x a_observations=1 b_observations=0 public=0", run)
}

func TestCSFRV19O3StaleDispatchCannotReadFutureEvidence(t *testing.T) {
	ctx := context.Background()
	c, store, ledger, cycle, fence, path := singleResourceFixture(t)
	if _, err := c.CommitSingleResource(ctx, ledger, cycle, fence); err != nil {
		t.Fatal(err)
	}
	_, _, _, _, _, runID := ledger.Snapshot()
	run := [32]byte(runID)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = store.DeliverRoundPerception(ctx, findTerminalRoundTask(t, ctx, db), c.bundle)
	if err != nil {
		t.Fatal(err)
	}
	_ = resourceActorInbox(t, ctx, store, path, ledger, c.bundle)
	old, ok := o3FindDispatch(t, ctx, path, run, [2]string{"school", "a"}, 0)
	if !ok {
		t.Fatal("fixture missing original dispatch")
	}
	q, err := sqlite.OpenPOVObservatory(ctx, path, c.bundle, run, old, [2]string{"school", "a"})
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close()
	report, err := q.ReadPOV(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if report.ThroughRevision != 0 || len(report.Entries) != 0 {
		t.Fatal("old dispatch learned future revision evidence")
	}
}

func TestCSFRV19O3PublicRequiresExplicitPublicationRoute(t *testing.T) {
	ctx := context.Background()
	c, _, ledger, run, bundle, path := testFixture(t)
	outcome, err := c.Advance(ctx, ledger, run)
	if err != nil {
		t.Fatal(err)
	}
	if len(outcome.Events) == 0 {
		t.Fatal("fixture lacks real EventStore rows")
	}
	reader, err := sqlite.OpenPublicObservatory(ctx, path, bundle)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	var _ observatory.PublicQueries = reader
	methods := reflect.TypeOf(reader)
	if methods.NumMethod() != 2 || methods.Method(0).Name != "Close" || methods.Method(1).Name != "ReadPublic" {
		t.Fatalf("public adapter unexpectedly exposes privileged method: %s", methods)
	}
	publicType := reflect.TypeOf(observatory.PublicPublication{})
	for i := 0; i < publicType.NumField(); i++ {
		name := publicType.Field(i).Name
		if strings.Contains(name, "Knowledge") || strings.Contains(name, "Observation") || strings.Contains(name, "Canonical") || strings.Contains(name, "Actor") {
			t.Fatalf("public contract leaks actor-private evidence field %q", name)
		}
	}
	before := o1WholeDatabaseDigest(t, path)
	first, err := reader.ReadPublic(ctx, [32]byte(run))
	if err != nil {
		t.Fatal(err)
	}
	if first.Run != [32]byte(run) || first.PublicationStatus != observatory.NoPublicRoute || len(first.Publications) != 0 {
		t.Fatalf("event existence was confused with publication: %+v", first)
	}
	for i := 0; i < 40; i++ {
		next, err := reader.ReadPublic(ctx, [32]byte(run))
		if err != nil || !reflect.DeepEqual(first, next) {
			t.Fatalf("public repeat %d changed: %+v %v", i, next, err)
		}
	}
	if o1WholeDatabaseDigest(t, path) != before {
		t.Fatal("public view mutated durable run")
	}
	other := [32]byte(run)
	other[0] ^= 0x87
	if _, err := reader.ReadPublic(ctx, other); err == nil {
		t.Fatal("public view returned another run")
	}
}

// The same globally verified evidence-route proof that authorizes A must
// reject a forged B route rather than leaking A's observation.
func TestCSFRV19O3RejectsForgedRecipientRoute(t *testing.T) {
	ctx := context.Background()
	c, store, ledger, cycle, fence, path := singleResourceFixture(t)
	if _, err := c.CommitSingleResource(ctx, ledger, cycle, fence); err != nil {
		t.Fatal(err)
	}
	_, _, _, _, _, runID := ledger.Snapshot()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	delivered, err := store.DeliverRoundPerception(ctx, findTerminalRoundTask(t, ctx, db), c.bundle)
	if err != nil {
		t.Fatal(err)
	}
	_ = resourceActorInbox(t, ctx, store, path, ledger, c.bundle)
	run := [32]byte(runID)
	d, ok := o3FindDispatch(t, ctx, path, run, [2]string{"school", "a"}, 1)
	if !ok {
		t.Fatal("missing A dispatch")
	}
	q, err := sqlite.OpenPOVObservatory(ctx, path, c.bundle, run, d, [2]string{"school", "a"})
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close()
	if _, err := q.ReadPOV(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE evidence_routes SET holder_id='b' WHERE record_id=?", delivered.Input.Identity[:]); err != nil {
		t.Fatal(err)
	}
	if got, err := q.ReadPOV(ctx); err == nil {
		t.Fatalf("forged holder route was exposed as a valid POV: %+v", got)
	}
	if _, err := db.ExecContext(ctx, "UPDATE evidence_routes SET holder_id='a' WHERE record_id=?", delivered.Input.Identity[:]); err != nil {
		t.Fatal(err)
	}
	if _, err := q.ReadPOV(ctx); err != nil {
		t.Fatal("restored route rejected:", err)
	}
}

func TestCSFRV19O3ActorDoesNotInheritPrivilegedEventTruth(t *testing.T) {
	ctx := context.Background()
	c, _, ledger, run, bundle, path := testFixture(t, activeTrigger())
	if _, err := c.Advance(ctx, ledger, run); err != nil {
		t.Fatal(err)
	}
	public, err := sqlite.OpenPublicObservatory(ctx, path, bundle)
	if err != nil {
		t.Fatal(err)
	}
	defer public.Close()
	p, err := public.ReadPublic(ctx, [32]byte(run))
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Publications) > 0 {
		t.Fatal("internal trigger event incorrectly became public world truth")
	}
	if strings.Contains(p.PublicationStatus, "PUBLIC") && p.PublicationStatus != observatory.NoPublicRoute {
		t.Fatal("unregistered public authority unexpectedly exposed")
	}
}

func TestCSFRV19O3UnknownPublicationSchemaFailsClosed(t *testing.T) {
	ctx := context.Background()
	_, _, _, run, bundle, path := testFixture(t)
	public, err := sqlite.OpenPublicObservatory(ctx, path, bundle)
	if err != nil {
		t.Fatal(err)
	}
	defer public.Close()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := public.ReadPublic(ctx, [32]byte(run)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "CREATE TABLE public_routes (id BLOB PRIMARY KEY)"); err != nil {
		t.Fatal(err)
	}
	if data, err := public.ReadPublic(ctx, [32]byte(run)); err == nil {
		t.Fatalf("unrecognized installed public authority silently fell back to an empty view: %+v", data)
	}
	if _, err := db.ExecContext(ctx, "DROP TABLE public_routes"); err != nil {
		t.Fatal(err)
	}
	if _, err := public.ReadPublic(ctx, [32]byte(run)); err != nil {
		t.Fatal("restored registered V1 schema rejected:", err)
	}
}

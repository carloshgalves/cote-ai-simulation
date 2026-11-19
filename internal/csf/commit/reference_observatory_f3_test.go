//go:build linux

package commit

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/observatory"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
)

// F3 is an internal privileged test oracle, not a combined actor-facing
// API. Each view is opened with its own existing read-only capability.
func TestReferenceF3CrossedViews(t *testing.T) {
	ctx := context.Background()
	host, plans, cuts := referenceE2SignedPair(t)
	path, owner := referenceE2Paths(t)
	p, err := RunReferenceD3(ctx, path, owner, bundlePath,
		ReferenceD2Config{Host: host, EpochPlans: plans, frozenSupervisedCuts: &cuts}, ReferenceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	run := referenceC2Decode32(t, p.RunID)
	before := referenceE2Capture(t, path, owner)
	db, err := openReferenceReadOnlyDB(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	dispatches, err := referenceD3ActorDispatches(ctx, db, run)
	if err != nil {
		t.Fatal(err)
	}
	audit, err := sqlite.OpenPrivilegedObservatory(ctx, path, host.Bundle)
	if err != nil {
		t.Fatal(err)
	}
	defer audit.Close()
	a, err := sqlite.OpenPOVObservatory(ctx, path, host.Bundle, run, dispatches[0], [2]string{"school", "a"})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, err := sqlite.OpenPOVObservatory(ctx, path, host.Bundle, run, dispatches[1], [2]string{"school", "b"})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	pub, err := sqlite.OpenPublicObservatory(ctx, path, host.Bundle)
	if err != nil {
		t.Fatal(err)
	}
	defer pub.Close()
	priv, err := audit.ReadPrivilegedReport(ctx, run)
	if err != nil || len(priv.Commits) != 9 || priv.State.Clock.Revision != 9 {
		t.Fatal("missing privileged terminal chain", err)
	}
	va, err := a.ReadPOV(ctx)
	if err != nil {
		t.Fatal(err)
	}
	vb, err := b.ReadPOV(ctx)
	if err != nil {
		t.Fatal(err)
	}
	vp, err := pub.ReadPublic(ctx, run)
	if err != nil || vp.Run != run || vp.PublicationStatus != observatory.NoPublicRoute || len(vp.Publications) != 0 {
		t.Fatal("public route inferred without publisher", err)
	}
	for i, reader := range []any{a, b, pub} {
		methods := reflect.TypeOf(reader)
		want := "ReadPOV"
		if i == 2 {
			want = "ReadPublic"
		}
		if methods.NumMethod() != 2 || methods.Method(0).Name != "Close" || methods.Method(1).Name != want {
			t.Fatal("restricted reader widened its public capability")
		}
	}

	// Derive the complete expected evidence roster independently from SQL,
	// not from POV output or its context builder. Join only the exact task
	// origin and route, and check all twelve authoritative evidence rows.
	type route struct {
		id, task, hash [32]byte
		holder         [2]string
		revision       uint64
		kind           int
		raw            []byte
	}
	rows, err := db.QueryContext(ctx, "SELECT r.record_id,r.task_id,r.canonical_hash,r.holder_namespace,r.holder_id,r.kind,e.canonical,t.canonical "+
		"FROM evidence_routes r JOIN evidence_ledger e ON e.id=r.record_id JOIN causal_outbox t ON t.id=r.task_id WHERE r.run_id=? ORDER BY r.record_id", run[:])
	if err != nil {
		t.Fatal(err)
	}
	roster := map[[32]byte]route{}
	for rows.Next() {
		var id, task, hash, taskRaw []byte
		var x route
		if err := rows.Scan(&id, &task, &hash, &x.holder[0], &x.holder[1], &x.kind, &x.raw, &taskRaw); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		if len(id) != 32 || len(task) != 32 || len(hash) != 32 {
			rows.Close()
			t.Fatal("malformed SQL authority identity")
		}
		copy(x.id[:], id)
		copy(x.task[:], task)
		copy(x.hash[:], hash)
		v, err := codec.StrictDecodePayload(taskRaw)
		if err != nil {
			rows.Close()
			t.Fatal(err)
		}
		f := v.([]any)
		x.revision = f[5].(uint64)
		if len(f) != 10 || !bytes.Equal(f[0].([]byte), task) || !bytes.Equal(f[1].([]byte), run[:]) ||
			x.hash != [32]byte(identity.OpaqueContentHash(x.raw)) || (x.kind != 0 && x.kind != 1) {
			rows.Close()
			t.Fatal("bad independent route proof")
		}
		if _, duplicate := roster[x.id]; duplicate {
			rows.Close()
			t.Fatal("duplicate evidence route")
		}
		roster[x.id] = x
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		t.Fatal(err)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	if len(roster) != 12 || len(roster) != len(before.Databases["causal"].Tables["evidence_ledger"].Rows) {
		t.Fatal("incomplete independent evidence roster")
	}

	// Verify that every privileged evidence edge originates in exactly
	// one real task/commit, and ENGINE_ONLY physical Events have no edge.
	physical := map[[32]byte]bool{}
	var forbidden [][]byte
	for _, token := range p.d2.PhysicalEventIDs {
		id := referenceC2Decode32(t, token)
		physical[id] = true
		forbidden = append(forbidden, bytes.Clone(id[:]))
	}
	for _, token := range p.d2.PhysicalInputIDs {
		id := referenceC2Decode32(t, token)
		forbidden = append(forbidden, bytes.Clone(id[:]))
	}
	var genesisRaw []byte
	if err := db.QueryRowContext(ctx, "SELECT canonical FROM genesis_records WHERE run_id=?", run[:]).Scan(&genesisRaw); err != nil {
		t.Fatal(err)
	}
	env, err := codec.StrictDecodeEnvelope(genesisRaw)
	if err != nil {
		t.Fatal(err)
	}
	seed := env.Value.([]any)[1].([]byte)
	if len(seed) != 32 {
		t.Fatal("missing genuine world seed")
	}
	forbidden = append(forbidden, seed, []byte("synthetic-a-private-only-memento"),
		[]byte("reference.c2.actor-b"), []byte("physical.advanced"), []byte("physical.a1"),
		[]byte("npc.0001"), []byte("npc.0002"), []byte("npc.0003"))
	// The private marker must really exist in the owner store, not just
	// in the test's blacklist. Never print private rows to CI logs.
	hasPrivate := false
	for _, table := range before.Databases["owner-a"].Tables {
		for _, raw := range table.Rows {
			hasPrivate = hasPrivate || bytes.Contains(raw, []byte("synthetic-a-private-only-memento"))
		}
	}
	if !hasPrivate {
		t.Fatal("missing actual private A owner fixture")
	}
	linked := map[[32]byte]bool{}
	physicalSeen, publicEvents := 0, 0
	for _, c := range priv.Commits {
		for _, e := range c.Events {
			var raw []byte
			if err := db.QueryRowContext(ctx, "SELECT canonical FROM event_store WHERE id=?", e.ID[:]).Scan(&raw); err != nil {
				t.Fatal(err)
			}
			v, err := codec.StrictDecodePayload(raw)
			if err != nil {
				t.Fatal(err)
			}
			f := v.([]any)
			if f[3] == "resource.stolen" {
				t.Fatal("false delivered Claim became objective Event")
			}
			if f[15] == uint64(0) {
				publicEvents++
			}
			if physical[e.ID] {
				physicalSeen++
				if f[3] != "physical.advanced" || f[15] != uint64(2) {
					t.Fatal("physical Event not ENGINE_ONLY")
				}
				forbidden = append(forbidden, raw)
				body, err := codec.CanonicalPayload(f[9])
				if err != nil {
					t.Fatal(err)
				}
				forbidden = append(forbidden, body)
			}
		}
		for _, l := range c.PerceptionLinks {
			if physical[l.Event.ID] {
				t.Fatal("ENGINE_ONLY event gained perception route")
			}
			for kind, refs := range [][]observatory.AuthorityRef{l.Observations, l.KnowledgeInputs} {
				for _, ref := range refs {
					x, ok := roster[ref.ID]
					if !ok || linked[ref.ID] || x.task != l.Task.ID || x.hash != ref.ContentHash || x.kind != kind || x.revision != c.ResultRevision {
						t.Fatal("privileged evidence link diverges from durable provenance")
					}
					linked[ref.ID] = true
				}
			}
		}
	}
	if physicalSeen != 2 || len(linked) != len(roster) {
		t.Fatal("missing physical or evidence authority")
	}

	claim := referenceC2Decode32(t, p.d2.C4.Communication.KnowledgeInputID)
	seen := map[[32]byte]bool{}
	for i, view := range []observatory.POVView{va, vb} {
		holder := [2]string{"school", []string{"a", "b"}[i]}
		if view.Run != run || view.Actor != holder || view.ThroughRevision != 6 || len(view.Entries) == 0 {
			t.Fatal("wrong actor/cutoff or vacuous positive POV")
		}
		messages := 0
		for _, e := range view.Entries {
			o, okO := roster[e.ObservationID]
			k, okK := roster[e.KnowledgeInputID]
			if !okO || !okK || o.kind != 0 || k.kind != 1 || o.holder != holder || k.holder != holder ||
				o.task != k.task || o.revision > view.ThroughRevision || k.revision > view.ThroughRevision ||
				seen[o.id] || seen[k.id] || !linked[o.id] || !linked[k.id] ||
				!bytes.Equal(o.raw, e.ObservationCanonical) || !bytes.Equal(k.raw, e.KnowledgeCanonical) {
				t.Fatal("POV is not exactly its authorized evidence subset")
			}
			seen[o.id], seen[k.id] = true, true
			for _, raw := range [][]byte{e.ObservationCanonical, e.KnowledgeCanonical} {
				for _, secret := range forbidden {
					if len(secret) == 0 || bytes.Contains(raw, secret) {
						t.Fatal("restricted POV contains engine/private material")
					}
				}
			}
			// Exact schema/actor/receipt/claim markers are structural
			// allowlists; no generic world-truth redaction is trusted.
			ov, err := codec.StrictDecodePayload(o.raw)
			if err != nil {
				t.Fatal(err)
			}
			kv, err := codec.StrictDecodePayload(k.raw)
			if err != nil {
				t.Fatal(err)
			}
			of, kf := ov.([]any), kv.([]any)
			if len(of) != 15 || len(kf) != 11 || !reflect.DeepEqual(of[2], []any{holder[0], holder[1]}) ||
				!reflect.DeepEqual(kf[3], []any{holder[0], holder[1]}) || kf[6] != e.Channel ||
				!bytes.Equal(kf[7].([]any)[1].([]byte), e.ObservationID[:]) {
				t.Fatal("invalid restricted evidence body")
			}
			switch e.Channel {
			case "school.round.lifecycle.v1":
				if len(of[10].([]any)) != 0 || !reflect.DeepEqual(of[11], []any{"source-inputs-redacted"}) {
					t.Fatal("lifecycle observation exposes sources")
				}
			case "school.message.v1":
				messages++
				if i != 1 || e.KnowledgeInputID != claim || e.ReceivedAt != 35 ||
					!reflect.DeepEqual(of[11], []any{"actual-sender-redacted", "claim-unverified"}) ||
					!bytes.Contains(o.raw, []byte("resource.stolen")) {
					t.Fatal("false Claim not isolated to B as unverified testimony")
				}
			default:
				t.Fatal("unexpected actor channel")
			}
		}
		if messages != i {
			t.Fatal("missing or extra communication disclosure")
		}
	}
	for id, x := range roster {
		expected := x.revision <= 6 && (x.holder == va.Actor || x.holder == vb.Actor)
		if seen[id] != expected {
			t.Fatal("POV omitted authorized evidence or exposed future/foreign record")
		}
	}
	t.Logf("F3 integrated views: 9 commits, 12 evidence records, 2 engine-only physical events, %d public-marked Events, zero publications", publicEvents)

	// An initial dispatch must see no post-genesis evidence even when the
	// current run contains both physical epochs and the delivered Claim.
	for _, actor := range [][2]string{va.Actor, vb.Actor} {
		id, ok := o3FindDispatch(t, ctx, path, run, actor, 0)
		if !ok {
			t.Fatal("missing genuine genesis dispatch")
		}
		stale, err := sqlite.OpenPOVObservatory(ctx, path, host.Bundle, run, id, actor)
		if err != nil {
			t.Fatal(err)
		}
		v, err := stale.ReadPOV(ctx)
		stale.Close()
		if err != nil || v.ThroughRevision != 0 || len(v.Entries) != 0 {
			t.Fatal("historic capability acquired future evidence", err)
		}
	}
	if err := referenceE1Compare(before, referenceE2Capture(t, path, owner)); err != nil {
		t.Fatal("four-view query changed causal/owner transcript", err)
	}
	// The remaining negative matrix uses the already-open views; a failed
	// query must not return partial results or a diagnostic side channel.
	admin, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	denied := func() {
		t.Helper()
		x, ea := a.ReadPOV(ctx)
		y, eb := b.ReadPOV(ctx)
		z, ep := pub.ReadPublic(ctx, run)
		if ea != observatory.ErrViewUnavailable || eb != observatory.ErrViewUnavailable || ep != observatory.ErrViewUnavailable ||
			!reflect.DeepEqual(x, observatory.POVView{}) || !reflect.DeepEqual(y, observatory.POVView{}) ||
			!reflect.DeepEqual(z, observatory.PublicView{}) {
			t.Fatal("tampered authority leaked partial restricted view or error details")
		}
		if report, err := audit.ReadPrivilegedReport(ctx, run); err == nil || !reflect.DeepEqual(report, observatory.PrivilegedReport{}) {
			t.Fatal("privileged replay accepted corrupt authority")
		}
	}
	restored := func() {
		t.Helper()
		x, ea := a.ReadPOV(ctx)
		y, eb := b.ReadPOV(ctx)
		z, ep := pub.ReadPublic(ctx, run)
		if ea != nil || eb != nil || ep != nil || !reflect.DeepEqual(x, va) || !reflect.DeepEqual(y, vb) || !reflect.DeepEqual(z, vp) {
			t.Fatal("restored authority diverged from original views")
		}
	}
	for _, tc := range []struct {
		name, table string
		id [32]byte
	}{
		{"physical-event", "event_store", referenceC2Decode32(t, p.d2.PhysicalEventIDs[0])},
		{"knowledge-input", "evidence_ledger", claim},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var raw []byte
			if err := admin.QueryRowContext(ctx, "SELECT canonical FROM "+tc.table+" WHERE id=?", tc.id[:]).Scan(&raw); err != nil {
				t.Fatal(err)
			}
			res, err := admin.ExecContext(ctx, "UPDATE "+tc.table+" SET canonical=x'80' WHERE id=?", tc.id[:])
			if err != nil {
				t.Fatal(err)
			}
			if n, err := res.RowsAffected(); err != nil || n != 1 {
				t.Fatal("mutation did not target real authority")
			}
			damaged := referenceE2Capture(t, path, owner)
			if referenceE1Compare(before, damaged) == nil {
				t.Fatal("vacuous mutation")
			}
			denied()
			if err := referenceE1Compare(damaged, referenceE2Capture(t, path, owner)); err != nil {
				t.Fatal("denied query mutated transcript", err)
			}
			if _, err := admin.ExecContext(ctx, "UPDATE "+tc.table+" SET canonical=? WHERE id=?", raw, tc.id[:]); err != nil {
				t.Fatal(err)
			}
			restored()
		})
	}
	t.Run("foreign-holder", func(t *testing.T) {
		res, err := admin.ExecContext(ctx, "UPDATE evidence_routes SET holder_id='a' WHERE record_id=?", claim[:])
		if err != nil {
			t.Fatal(err)
		}
		if n, err := res.RowsAffected(); err != nil || n != 1 {
			t.Fatal("missing B route")
		}
		damaged := referenceE2Capture(t, path, owner)
		denied()
		if err := referenceE1Compare(damaged, referenceE2Capture(t, path, owner)); err != nil {
			t.Fatal(err)
		}
		if _, err := admin.ExecContext(ctx, "UPDATE evidence_routes SET holder_id='b' WHERE record_id=?", claim[:]); err != nil {
			t.Fatal(err)
		}
		restored()
	})
	t.Run("swapped-dispatch", func(t *testing.T) {
		for i, actor := range [][2]string{va.Actor, vb.Actor} {
			reader, err := sqlite.OpenPOVObservatory(ctx, path, host.Bundle, run, dispatches[1-i], actor)
			if reader != nil || err != observatory.ErrViewUnavailable {
				t.Fatal("foreign actor dispatch granted capability")
			}
		}
	})
	t.Run("unsupported-public-view", func(t *testing.T) {
		if _, err := admin.ExecContext(ctx, "CREATE VIEW PUBLIC_ROUTES AS SELECT id FROM evidence_ledger"); err != nil {
			t.Fatal(err)
		}
		if v, err := pub.ReadPublic(ctx, run); err != observatory.ErrViewUnavailable || !reflect.DeepEqual(v, observatory.PublicView{}) {
			t.Fatal("unregistered case-variant publication view accepted")
		}
		if _, err := admin.ExecContext(ctx, "DROP VIEW PUBLIC_ROUTES"); err != nil {
			t.Fatal(err)
		}
		restored()
	})
	t.Run("cancellation", func(t *testing.T) {
		canceled, cancel := context.WithCancel(ctx)
		cancel()
		if _, err := pub.ReadPublic(canceled, run); !errors.Is(err, context.Canceled) {
			t.Fatal("restricted view lost cancellation")
		}
		if _, err := a.ReadPOV(canceled); !errors.Is(err, context.Canceled) {
			t.Fatal("restricted POV lost cancellation")
		}
	})
	if err := referenceE1Compare(before, referenceE2Capture(t, path, owner)); err != nil {
		t.Fatal("restored scenario differs", err)
	}
}

// Pin restricted DTO field names exactly: checking only known forbidden
// names misses a future leak via an innocuous newly added field.
func TestReferenceF3RestrictedAllowlists(t *testing.T) {
	for _, c := range []struct {
		value any
		names string
	}{
		{observatory.POVView{}, "Run Actor ThroughRevision Entries"},
		{observatory.AddressedEvidence{}, "ObservationID KnowledgeInputID ObservationCanonical KnowledgeCanonical ReceivedAt Channel"},
		{observatory.PublicView{}, "Run PublicationStatus Publications"},
		{observatory.PublicPublication{}, "PublicationID Channel PublishedAt ContentHash"},
	} {
		typ := reflect.TypeOf(c.value)
		got := make([]string, typ.NumField())
		for i := range got {
			got[i] = typ.Field(i).Name
		}
		if strings.Join(got, " ") != c.names {
			t.Fatal("restricted projection field allowlist changed")
		}
	}
}

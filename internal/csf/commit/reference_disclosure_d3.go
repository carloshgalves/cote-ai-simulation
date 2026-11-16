//go:build linux

package commit

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/observatory"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

// ReferenceD3Matrix is a disclosure audit, not an authorization to reveal
// physical data. All counters refer to the *same* committed causal database.
type ReferenceD3Matrix struct {
	PhysicalEvents          int  `json:"physical_events"`
	PhysicalPerceptionTasks int  `json:"physical_perception_tasks"`
	ActorAAddressed         int  `json:"actor_a_addressed"`
	ActorBAddressed         int  `json:"actor_b_addressed"`
	RecipientBClaimOnly     bool `json:"recipient_b_claim_only"`
	ActorPhysicalDenied     bool `json:"actor_physical_denied"`
	PublicPhysicalDenied    bool `json:"public_physical_denied"`
	PublicPublications      int  `json:"public_publications"`
	ReplayCommits           int  `json:"replay_commits"`
	SnapshotTailCommits     int  `json:"snapshot_tail_commits"`
	FiveLedgerPrefixStable  bool `json:"five_ledger_prefix_stable"`
	ReadersDidNotWrite      bool `json:"readers_did_not_write"`
}

type ReferenceD3Proof struct {
	Status string            `json:"status"`
	RunID  string            `json:"run_id"`
	Matrix ReferenceD3Matrix `json:"-"`
	// Never marshal the engine-only D2 oracle or the private physical state.
	d2       ReferenceD2Proof
	sealed   ReferenceD3Matrix
	verified bool
}

// The public audit certificate contains no actor-specific evidence counts,
// private event IDs, body, stock, seed or policy/producer internals.
type ReferenceD3PublicDisclosure struct {
	PhysicalEvents             int  `json:"physical_events"`
	PhysicalPerceptionTasks    int  `json:"physical_perception_tasks"`
	RecipientIsolationVerified bool `json:"recipient_isolation_verified"`
	ActorPhysicalDenied        bool `json:"actor_physical_denied"`
	PublicPhysicalDenied       bool `json:"public_physical_denied"`
	PublicPublications         int  `json:"public_publications"`
	ReplayCommits              int  `json:"replay_commits"`
	SnapshotTailCommits        int  `json:"snapshot_tail_commits"`
	FiveLedgerPrefixStable     bool `json:"five_ledger_prefix_stable"`
	ReadonlyVerified           bool `json:"readonly_verified"`
}
type ReferenceD3PublicReport struct {
	Status     string                      `json:"status"`
	RunID      string                      `json:"run_id"`
	Disclosure ReferenceD3PublicDisclosure `json:"disclosure"`
}

func (p ReferenceD3Proof) PublicReport() (ReferenceD3PublicReport, error) {
	var zero ReferenceD3PublicReport
	if !p.verified || p.Status != "RUN_LOCAL_10D3" || p.RunID != p.d2.RunID ||
		!reflect.DeepEqual(p.Matrix, p.sealed) || p.Matrix.PhysicalEvents != 2 ||
		p.Matrix.PhysicalPerceptionTasks != 0 ||
		!p.Matrix.RecipientBClaimOnly || !p.Matrix.ActorPhysicalDenied ||
		!p.Matrix.PublicPhysicalDenied || p.Matrix.PublicPublications != 0 ||
		p.Matrix.ReplayCommits != 9 || p.Matrix.SnapshotTailCommits != 3 ||
		!p.Matrix.FiveLedgerPrefixStable || !p.Matrix.ReadersDidNotWrite {
		return zero, errors.New("D3 refuses public report without complete disclosure audit")
	}
	if _, err := p.d2.PublicReport(); err != nil {
		return zero, err
	}
	return ReferenceD3PublicReport{Status: p.Status, RunID: p.RunID,
		Disclosure: ReferenceD3PublicDisclosure{
			PhysicalEvents:             p.Matrix.PhysicalEvents,
			PhysicalPerceptionTasks:    p.Matrix.PhysicalPerceptionTasks,
			RecipientIsolationVerified: p.Matrix.RecipientBClaimOnly,
			ActorPhysicalDenied:        p.Matrix.ActorPhysicalDenied,
			PublicPhysicalDenied:       p.Matrix.PublicPhysicalDenied,
			PublicPublications:         p.Matrix.PublicPublications,
			ReplayCommits:              p.Matrix.ReplayCommits,
			SnapshotTailCommits:        p.Matrix.SnapshotTailCommits,
			FiveLedgerPrefixStable:     p.Matrix.FiveLedgerPrefixStable,
			ReadonlyVerified:           p.Matrix.ReadersDidNotWrite,
		}}, nil
}

// RunReferenceD3 executes two *real* A1 Python epochs through D2, then
// inspects persisted causal records via independent read-only readers.
// This is local experimental acceptance; it does not attest production A1.
func RunReferenceD3(ctx context.Context, dbPath, ownerPath, bundlePath string,
	cfg ReferenceD2Config, options ReferenceOptions) (ReferenceD3Proof, error) {
	var zero ReferenceD3Proof
	d2, err := RunReferenceD2(ctx, dbPath, ownerPath, bundlePath, cfg, options)
	if err != nil {
		return zero, err
	}
	return AuditReferenceD3(ctx, dbPath, cfg.Host.Bundle, d2)
}

// AuditReferenceD3 is also callable after a run, so adversarial tests can
// corrupt independent persisted roots and prove that disclosure fails closed.
// A D2 boolean is never accepted as evidence in place of actual DB reads.
func AuditReferenceD3(ctx context.Context, path string, bundle *policy.Bundle,
	d2 ReferenceD2Proof) (ReferenceD3Proof, error) {
	var zero ReferenceD3Proof
	if ctx == nil || bundle == nil {
		return zero, errors.New("D3 requires installed physical policy and context")
	}
	if _, err := d2.PublicReport(); err != nil {
		return zero, fmt.Errorf("D3 missing real D2 baseline: %w", err)
	}
	if err := bundle.VerifyInstalledBundle(); err != nil {
		return zero, err
	}
	reg, err := bundle.PhysicalRegistration()
	if err != nil || reg.Disclosure != "ENGINE_ONLY" {
		return zero, errors.New("D3 refuses non-ENGINE_ONLY physical registration")
	}
	run, err := referenceC4DecodeID(d2.RunID)
	if err != nil {
		return zero, err
	}
	db, err := openReferenceReadOnlyDB(path)
	if err != nil {
		return zero, err
	}
	defer db.Close()

	// Read real EventStore records. Event IDs from the signed D2 Commit
	// must be exactly the two authenticated physical.advanced records.
	physicalIDs := map[[32]byte]bool{}
	for _, token := range d2.PhysicalEventIDs {
		id, err := referenceC4DecodeID(token)
		if err != nil || physicalIDs[id] {
			return zero, errors.New("D3 duplicate/invalid physical event identity")
		}
		physicalIDs[id] = true
	}
	physicalPayloads := make([][]byte, 0, 2)
	eventRows, err := db.QueryContext(ctx, "SELECT id, canonical FROM event_store ORDER BY id")
	if err != nil {
		return zero, err
	}
	physicalCount := 0
	for eventRows.Next() {
		var id, raw []byte
		if err := eventRows.Scan(&id, &raw); err != nil {
			eventRows.Close()
			return zero, err
		}
		value, err := codec.StrictDecodePayload(raw)
		if err != nil {
			eventRows.Close()
			return zero, err
		}
		fields, ok := value.([]any)
		if !ok || len(fields) != 19 || len(id) != 32 {
			eventRows.Close()
			return zero, errors.New("D3 malformed persisted Event")
		}
		var key [32]byte
		copy(key[:], id)
		if fields[3] != "physical.advanced" {
			if physicalIDs[key] {
				eventRows.Close()
				return zero, errors.New("D3 physical identity retyped")
			}
			continue
		}
		order, orderOK := fields[2].([]any)
		observers, observersOK := fields[12].([]any)
		eventID, eventIDOK := fields[0].([]byte)
		if !physicalIDs[key] || !eventIDOK || !bytes.Equal(id, eventID) ||
			!orderOK || len(order) != 6 || order[0] != uint64(1) ||
			order[2] != "physical.a1" || order[4] != uint64(0) ||
			fields[15] != uint64(2) || fields[16] != "physical.a1" ||
			!observersOK || len(observers) != 0 ||
			!referenceD3PrivateEventMetadata(fields) {
			eventRows.Close()
			return zero, errors.New("D3 physical event has public/actor route or foreign authority")
		}
		typed, ok := fields[9].([]any)
		if !ok || len(typed) != 4 {
			eventRows.Close()
			return zero, errors.New("D3 missing private typed physical payload")
		}
		canonical, err := codec.CanonicalPayload(typed)
		if err != nil {
			eventRows.Close()
			return zero, err
		}
		physicalPayloads = append(physicalPayloads, canonical)
		physicalCount++
	}
	if err := eventRows.Err(); err != nil {
		eventRows.Close()
		return zero, err
	}
	if err := eventRows.Close(); err != nil {
		return zero, err
	}
	if physicalCount != 2 {
		return zero, errors.New("D3 missing or extra physical EventStore authority")
	}

	// No generic task may be derived from EITHER physical Event, even when
	// its EventOrderKey local ordinal is zero. Every existing task is inspected.
	taskRows, err := db.QueryContext(ctx, "SELECT canonical FROM causal_outbox ORDER BY id")
	if err != nil {
		return zero, err
	}
	taskCount := 0
	for taskRows.Next() {
		var raw []byte
		if err := taskRows.Scan(&raw); err != nil {
			taskRows.Close()
			return zero, err
		}
		value, err := codec.StrictDecodePayload(raw)
		if err != nil {
			taskRows.Close()
			return zero, err
		}
		fields, ok := value.([]any)
		if !ok || len(fields) != 10 {
			taskRows.Close()
			return zero, errors.New("D3 outbox task malformed")
		}
		id, ok := fields[3].([]byte)
		if !ok || len(id) != 32 {
			taskRows.Close()
			return zero, errors.New("D3 task missing Event ID")
		}
		var source [32]byte
		copy(source[:], id)
		if physicalIDs[source] {
			taskCount++
		}
	}
	if err := taskRows.Err(); err != nil {
		taskRows.Close()
		return zero, err
	}
	if err := taskRows.Close(); err != nil {
		return zero, err
	}
	if taskCount != 0 {
		return zero, errors.New("D3 ENGINE_ONLY Event created generic PerceptionTask")
	}

	// EvidenceLedger is global and later revisions may carry routes not
	// visible through the revision-six C2 snapshot. Inspect it independently:
	// the D3 proof may not rely only on a pair of old actor POV cutoffs.
	if err := referenceD3CheckGlobalEvidence(ctx, db, physicalPayloads); err != nil {
		return zero, err
	}

	dispatches, err := referenceD3ActorDispatches(ctx, db, run)
	if err != nil {
		return zero, err
	}
	before, countsBefore, err := referenceC4FiveLedgers(ctx, path, bundle)
	if err != nil {
		return zero, err
	}
	for _, name := range referenceC4LedgerNames {
		if before[name] != d2.FiveLedgerDigests[name] || countsBefore[name] <= 0 {
			return zero, errors.New("D3 disclosure source differs from D2 sealed ledger tail")
		}
	}
	result := ReferenceD3Matrix{PhysicalEvents: physicalCount, PhysicalPerceptionTasks: taskCount}
	for i, actor := range [][2]string{{"school", "a"}, {"school", "b"}} {
		reader, err := sqlite.OpenPOVObservatory(ctx, path, bundle, run, dispatches[i], actor)
		if err != nil {
			return zero, fmt.Errorf("D3 actor %s capability: %w", actor[1], err)
		}
		view, readErr := reader.ReadPOV(ctx)
		closeErr := reader.Close()
		if readErr != nil {
			return zero, fmt.Errorf("D3 actor %s POV: %w", actor[1], readErr)
		}
		if closeErr != nil {
			return zero, closeErr
		}
		if view.Run != run || view.Actor != actor || view.ThroughRevision != 6 {
			return zero, errors.New("D3 POV escaped actor-scoped causal cutoff")
		}
		if i == 0 {
			result.ActorAAddressed = len(view.Entries)
		} else {
			result.ActorBAddressed = len(view.Entries)
		}
		for _, entry := range view.Entries {
			for _, raw := range [][]byte{entry.ObservationCanonical, entry.KnowledgeCanonical} {
				for _, forbidden := range []string{"physical.advanced", "physical.a1", "npc.0001", "npc.0002", "npc.0003"} {
					if bytes.Contains(raw, []byte(forbidden)) {
						return zero, errors.New("D3 actor context leaked physical authority")
					}
				}
				for _, private := range physicalPayloads {
					if bytes.Contains(raw, private) {
						return zero, errors.New("D3 actor context contains sealed BodyState")
					}
				}
			}
			claim, err := referenceC4DecodeID(d2.C4.Communication.KnowledgeInputID)
			if err != nil {
				return zero, err
			}
			if entry.KnowledgeInputID == claim {
				if i == 0 || entry.ReceivedAt != 35 || entry.Channel != "school.message.v1" {
					return zero, errors.New("D3 foreign or premature B-only Claim")
				}
			}
		}
	}
	claim, err := referenceC4DecodeID(d2.C4.Communication.KnowledgeInputID)
	if err != nil {
		return zero, err
	}
	// The B KnowledgeInput has exactly one durable holder route. A must never
	// inherit B's Claim simply because the engine sees it in EventStore.
	var bClaimRoutes, aClaimRoutes int
	if err := db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM evidence_routes WHERE record_id=? AND run_id=? AND holder_namespace='school' AND holder_id='b'",
		claim[:], run[:]).Scan(&bClaimRoutes); err != nil {
		return zero, err
	}
	if err := db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM evidence_routes WHERE record_id=? AND run_id=? AND holder_namespace='school' AND holder_id='a'",
		claim[:], run[:]).Scan(&aClaimRoutes); err != nil {
		return zero, err
	}
	if bClaimRoutes != 1 || aClaimRoutes != 0 {
		return zero, errors.New("D3 B-only Claim routing violated")
	}
	result.RecipientBClaimOnly = true
	result.ActorPhysicalDenied = true

	publicReader, err := sqlite.OpenPublicObservatory(ctx, path, bundle)
	if err != nil {
		return zero, err
	}
	public, readErr := publicReader.ReadPublic(ctx, run)
	closeErr := publicReader.Close()
	if readErr != nil {
		return zero, fmt.Errorf("D3 public Observatory: %w", readErr)
	}
	if closeErr != nil {
		return zero, closeErr
	}
	if public.Run != run || public.PublicationStatus != observatory.NoPublicRoute || len(public.Publications) != 0 {
		return zero, errors.New("D3 public Observatory exposed private engine/EventStore evidence")
	}
	result.PublicPhysicalDenied = true
	result.PublicPublications = len(public.Publications)

	store, err := sqlite.Open(ctx, path)
	if err != nil {
		return zero, err
	}
	replay, replayErr := store.ReplayClock(ctx, run, bundle)
	snapshotID, snapIDErr := referenceC4DecodeID(d2.C4.Epistemic.SnapshotID)
	if snapIDErr != nil {
		store.Close()
		return zero, snapIDErr
	}
	snapshot, snapErr := store.VerifySnapshotV1(ctx, run, snapshotID, bundle)
	closeErr = store.Close()
	if replayErr != nil || snapErr != nil || closeErr != nil {
		return zero, fmt.Errorf("D3 replay/snapshot failed: replay=%v snapshot=%v close=%v", replayErr, snapErr, closeErr)
	}
	if replay.ReplayedCommits != 9 || replay.ReconstructedClock.Revision != 9 ||
		snapshot.TailCommits != 3 {
		return zero, errors.New("D3 replay/snapshot lost shared physical+cognitive prefix")
	}
	result.ReplayCommits = replay.ReplayedCommits
	result.SnapshotTailCommits = int(snapshot.TailCommits)
	after, countsAfter, err := referenceC4FiveLedgers(ctx, path, bundle)
	if err != nil {
		return zero, err
	}
	if !reflect.DeepEqual(before, after) || !reflect.DeepEqual(countsBefore, countsAfter) {
		return zero, errors.New("D3 read-only Observatory mutated a causal ledger")
	}
	result.FiveLedgerPrefixStable = true
	result.ReadersDidNotWrite = true
	proof := ReferenceD3Proof{Status: "RUN_LOCAL_10D3", RunID: d2.RunID, Matrix: result,
		d2: d2, sealed: result, verified: true}
	if _, err := proof.PublicReport(); err != nil {
		return zero, err
	}
	return proof, nil
}

// Check all durable evidence, including records added after C2's owner
// checkpoint. ENGINE_ONLY data may exist in EventStore for the engine, but
// must not be materialized in recipient-facing EvidenceLedger.
func referenceD3CheckGlobalEvidence(ctx context.Context, db interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}, forbidden [][]byte) error {
	rows, err := db.QueryContext(ctx, "SELECT canonical FROM evidence_ledger ORDER BY id")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return err
		}
		if _, err := codec.StrictDecodePayload(raw); err != nil {
			return err
		}
		for _, s := range []string{"physical.advanced", "physical.a1", "npc.0001", "npc.0002", "npc.0003"} {
			if bytes.Contains(raw, []byte(s)) {
				return errors.New("D3 leaked ENGINE_ONLY source in evidence ledger")
			}
		}
		for _, secret := range forbidden {
			if len(secret) > 0 && bytes.Contains(raw, secret) {
				return errors.New("D3 leaked signed physical BodyState in evidence ledger")
			}
		}
	}
	return rows.Err()
}

// A registered ENGINE_ONLY physical Event must have no addressed actor,
// observer, publication or derived physical context channel.
func referenceD3PrivateEventMetadata(fields []any) bool {
	if len(fields) != 19 {
		return false
	}
	private, ok := fields[13].([]any)
	if !ok || len(private) != 0 {
		return false
	}
	confidentiality, ok := fields[14].([]any)
	return ok && len(confidentiality) == 1 && confidentiality[0] == uint64(0)
}

// The only valid audit capabilities are the two actual next-round
// SlotDispatches registered by C2 at revision six. Neither a free-form actor
// name nor an EventStore ID is usable as an actor capability.
func referenceD3ActorDispatches(ctx context.Context, db interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}, run [32]byte) ([2][32]byte, error) {
	var out [2][32]byte
	rows, err := db.QueryContext(ctx, "SELECT id,canonical FROM input_ledger ORDER BY id")
	if err != nil {
		return out, err
	}
	defer rows.Close()
	var counts [2]int
	for rows.Next() {
		var id, raw []byte
		if err := rows.Scan(&id, &raw); err != nil {
			return out, err
		}
		v, err := codec.StrictDecodePayload(raw)
		if err != nil {
			return out, err
		}
		f, ok := v.([]any)
		if !ok || len(f) != 11 || f[8] != uint64(6) || len(id) != 32 {
			continue
		}
		if runID, ok := f[1].([]byte); !ok || !bytes.Equal(runID, run[:]) {
			continue
		}
		actor, ok := f[6].([]any)
		if !ok || len(actor) != 2 || actor[0] != "school" {
			continue
		}
		idx := -1
		switch actor[1] {
		case "a":
			idx = 0
		case "b":
			idx = 1
		}
		if idx < 0 {
			continue
		}
		canonicalID, ok := f[0].([]byte)
		if !ok || !bytes.Equal(canonicalID, id) {
			return out, errors.New("D3 dispatch ID mismatch")
		}
		copy(out[idx][:], id)
		counts[idx]++
	}
	if err := rows.Err(); err != nil {
		return out, err
	}
	if counts != [2]int{1, 1} {
		return out, fmt.Errorf("D3 requires one authentic C2 capability per actor, got %v", counts)
	}
	return out, nil
}

package sqlite

import (
	"bytes"
	"context"
	"errors"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/clock"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
)

// verifyPublishedPhysical reconstructs the exact SQLite mode-9 terminal
// from canonical persisted authorities; it does not trust a cached output,
// stale runtime bundle, or ephemeral source path during crash/restart/fork.
// The same live terminal verifier rederives candidate, event, decision,
// signed producer and physical world reducer from their durable bytes.
func verifyPublishedPhysical(ctx context.Context, q queryOne, run, cycle, commit [32]byte,
	journal, fence []any, events []any) (domain.ID, error) {
	var zero domain.ID
	units, ok := fence[11].([]any)
	if !ok || len(units) != 1 {
		return zero, errors.New("replay physical source partition changed")
	}
	unit, ok := units[0].([]any)
	if !ok || len(unit) != 5 || unit[1] != uint64(1) {
		return zero, errors.New("replay source is not registered physical input")
	}
	sourceID, ok := unit[3].([]byte)
	if !ok || len(sourceID) != 32 {
		return zero, errors.New("replay physical source ID malformed")
	}
	if len(events) != 1 {
		return zero, errors.New("replay physical event count not one")
	}
	eventBytes, ok := events[0].([]byte)
	if !ok {
		return zero, errors.New("replay physical EventStore bytes missing")
	}
	rawEvent, err := codec.StrictDecodePayload(eventBytes)
	if err != nil {
		return zero, err
	}
	ev, ok := rawEvent.([]any)
	if !ok || len(ev) != 19 || ev[3] != "physical.advanced" ||
		!equalBytes(ev[7], cycle[:]) || !equalBytes(ev[1], run[:]) {
		return zero, errors.New("replay physical Event outside authenticated cycle")
	}
	key, ok := ev[2].([]any)
	if !ok || len(key) != 6 {
		return zero, errors.New("replay physical EventOrderKey malformed")
	}
	candidateID, ok := key[1].([]byte)
	if !ok || len(candidateID) != 32 {
		return zero, errors.New("replay physical candidate identity missing")
	}
	var candidateRaw []byte
	if err := q.QueryRowContext(ctx, "SELECT canonical FROM decision_ledger WHERE id=?", candidateID).Scan(&candidateRaw); err != nil {
		return zero, err
	}
	decisionIDs, ok := journal[16].([]any)
	if !ok || len(decisionIDs) != 1 {
		return zero, errors.New("replay physical decision partition invalid")
	}
	decisionID, ok := decisionIDs[0].([]byte)
	if !ok || len(decisionID) != 32 {
		return zero, errors.New("replay physical decision identity invalid")
	}
	var decisionRaw []byte
	if err := q.QueryRowContext(ctx, "SELECT canonical FROM decision_ledger WHERE id=?", decisionID).Scan(&decisionRaw); err != nil {
		return zero, err
	}
	previousRevision, ok := journal[6].(uint64)
	if !ok {
		return zero, errors.New("replay physical prior revision malformed")
	}
	resultRevision, ok := journal[8].(uint64)
	if !ok || resultRevision != previousRevision+1 {
		return zero, errors.New("replay physical revision not adjacent")
	}
	var previousRaw, revisionRaw, previousDigest, revisionDigest []byte
	if err := q.QueryRowContext(ctx, "SELECT canonical,state_hash FROM world_revisions WHERE revision=?", previousRevision).
		Scan(&previousRaw, &previousDigest); err != nil {
		return zero, err
	}
	if err := q.QueryRowContext(ctx, "SELECT canonical,state_hash FROM world_revisions WHERE revision=?", resultRevision).
		Scan(&revisionRaw, &revisionDigest); err != nil {
		return zero, err
	}
	if len(previousDigest)!=32 || len(revisionDigest)!=32 {
		return zero,errors.New("replay physical WorldRevision SHA256 length invalid")
	}
	prevParsed, err := codec.StrictDecodePayload(previousRaw)
	if err != nil {
		return zero, err
	}
	previous, ok := prevParsed.([]any)
	if !ok || len(previous) != 6 || !equalBytes(previous[5], previousDigest) ||
		previous[3] != previousRevision {
		return zero, errors.New("replay physical previous WorldRevision malformed")
	}
	revParsed, err := codec.StrictDecodePayload(revisionRaw)
	if err != nil {
		return zero, err
	}
	revision, ok := revParsed.([]any)
	if !ok || len(revision) != 6 || !equalBytes(revision[5], revisionDigest) ||
		revision[0] != previousRevision || revision[3] != resultRevision ||
		!sameCanonical(revision[1], previous[4]) ||
		!sameCanonical(revision[2], previousDigest) {
		return zero, errors.New("replay physical WorldRevision predecessor mismatch")
	}
	var genesisRaw []byte
	if err := q.QueryRowContext(ctx, "SELECT canonical FROM genesis_records WHERE run_id=?", run[:]).Scan(&genesisRaw); err != nil {
		return zero, err
	}
	env, err := codec.StrictDecodeEnvelope(genesisRaw)
	if err != nil {
		return zero, err
	}
	genesis, ok := env.Value.([]any)
	if !ok || len(genesis) != 18 {
		return zero, errors.New("replay physical Genesis invalid")
	}
	proof, err := verifyPhysicalInput(ctx, q, run, unit, genesis)
	if err != nil {
		return zero, err
	}
	instant, ok := asSigned64(journal[3])
	if !ok {
		return zero, errors.New("replay physical cycle instant invalid")
	}
	ordinal, ok := journal[4].(uint64)
	if !ok {
		return zero, errors.New("replay physical cycle ordinal invalid")
	}
	sequence, ok := ev[6].(uint64)
	if !ok {
		return zero, errors.New("replay physical event logical sequence invalid")
	}
	nextSequence, ok := journal[22].(uint64)
	if !ok {
		return zero, errors.New("replay physical next logical sequence invalid")
	}
	prior := clock.State{Instant: domain.SimulationInstant(instant),
		Revision:            domain.WorldRevision(previousRevision),
		NextLogicalSequence: domain.LogicalSequence(sequence)}
	next := clock.State{Instant: prior.Instant, Revision: domain.WorldRevision(resultRevision),
		LastOrdinal: int64(ordinal), NextLogicalSequence: domain.LogicalSequence(nextSequence)}
	var candidate, decision, event [32]byte
	copy(candidate[:], candidateID)
	copy(decision[:], decisionID)
	eventID, ok := ev[0].([]byte)
	if !ok || len(eventID) != 32 {
		return zero, errors.New("replay physical Event ID invalid")
	}
	copy(event[:], eventID)
	batch := ports.WorkBatch{
		Mode: 9, RunID: run, CycleID: cycle,
		Physical: &ports.PhysicalPublication{Manifest: proof.manifest,
			SignedCut: proof.cutRaw, Signature: proof.signature, CompletedTyped: proof.completed,
			Candidate: ports.CanonicalRecord{Identity: candidate, Canonical: candidateRaw}},
		Decisions:       []ports.CanonicalRecord{{Identity: decision, Canonical: decisionRaw}},
		Events:          []ports.CanonicalRecord{{Identity: event, Canonical: eventBytes}},
		BaseStateHash:   copyPhysicalDigest(previousDigest),
		ResultStateHash: copyPhysicalDigest(revisionDigest),
	}
	allowed := map[string]any{}
	refs, ok := genesis[14].([]any)
	if !ok {
		return zero, errors.New("replay physical Genesis policy refs invalid")
	}
	for _, ref := range refs {
		parts, ok := ref.([]any)
		if !ok || len(parts) != 3 {
			return zero, errors.New("replay physical policy binding malformed")
		}
		name, ok := parts[0].(string)
		if !ok || allowed[name] != nil {
			return zero, errors.New("replay physical duplicate policy ref")
		}
		allowed[name] = parts
	}
	if err := verifyPhysicalTerminal(ctx, q, batch, prior, next, fence, journal, previous, revision, genesis, allowed, map[domain.ID]map[string]bool{}); err != nil {
		return zero, err
	}
	if !bytes.Equal(commit[:], journalCommitPhysicalID(run, cycle, journal)) {
		return zero, errors.New("replay physical cycle commit ID mismatch")
	}
	copy(zero[:], sourceID)
	return zero, nil
}

func copyPhysicalDigest(raw []byte) [32]byte {
	var out [32]byte
	copy(out[:], raw)
	return out
}
func journalCommitPhysicalID(run, cycle [32]byte, journal []any) []byte {
	if len(journal) != 24 {
		return nil
	}
	expected, err := registeredDigest("cote.csf.id.cycle-commit",
		"cote.csf.schema.cycle-commit-id-preimage",
		[]any{run[:], cycle[:], journal[2]})
	if err != nil {
		return nil
	}
	return expected[:]
}

package commit

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/adapter/embodimentv1"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/admission"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/clock"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/decision"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/inputledger"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/reducer"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/settlement"
)

// StagePhysicalInput transfers an A1-authorized, sealed and completed
// physical cut into the existing durable InputLedger. It is recoverably
// idempotent, but does NOT publish any WorldRevision or Event by itself.
// A crashed stage without SourceClosure is not admitted at a fence.
func (c *Coordinator) StagePhysicalInput(ctx context.Context, ledger *inputledger.Ledger,
	approved embodimentv1.ApprovedCut, cycle admission.Cycle) (inputledger.StoredInput, error) {
	var zero inputledger.StoredInput
	if c == nil || c.bundle == nil || c.store == nil || ledger == nil || !ledger.SharesFenceAuthority(c.store) {
		return zero, errors.New("physical ingress requires one durable authority")
	}
	reg, err := c.bundle.PhysicalRegistration()
	if err != nil {
		return zero, err
	}
	cutRaw, signature, completed, err := approved.CanonicalProof()
	if err != nil {
		return zero, err
	}
	manifestRaw, err := c.bundle.PhysicalManifestCanonical()
	if err != nil {
		return zero, err
	}
	cut, err := embodimentv1.DecodeSignedCut(cutRaw, signature)
	if err != nil {
		return zero, err
	}
	// A foreign/forged cut must not leave a durable SourceDeclaration behind.
	// Snapshot freezes the source registry, so authenticate through the
	// non-freezing RunID accessor before any declaration or snapshot.
	run := ledger.RunID()
	g, err := c.store.ValidatedGenesis(ctx, [32]byte(run), c.bundle)
	if err != nil || g.RunID != run {
		return zero, errors.New("physical Genesis is not persisted and authenticated")
	}
	if err := embodimentv1.VerifySignedCut(cut, reg, run, g.Digest); err != nil {
		return zero, err
	}
	seen := false
	for _, source := range ledger.Sources() {
		if source == reg.SourceID {
			seen = true
			break
		}
	}
	if !seen {
		if err := ledger.DeclareSource(inputledger.Source{ID: reg.SourceID}); err != nil {
			return zero, err
		}
	}
	if cut.CompletedStateSHA256 != ([32]byte{}) && !bytes.Equal(cut.CompletedStateSHA256[:], completed[3].([]byte)) {
		return zero, errors.New("signed physical body does not match A1 projection")
	}
	if cycle.Plan != nil || cycle.Attempt != 0 || cycle.Instant != g.InitialInstant && cycle.BaseRevision == 0 {
		return zero, errors.New("invalid physical staging cycle")
	}
	eligibility := domain.EligibilityCoordinate{
		NotBeforeInstant: cycle.Instant, NotBeforeCycleOrdinal: cycle.Ordinal,
	}

	// Epoch is a producer unit identity, never a Python local sim_time or seq.
	epoch := strconv.FormatUint(cut.Epoch, 10)
	value := []any{"physical.source-cut.v1", manifestRaw, cutRaw, signature, completed}
	unit, err := ledger.AppendInput(ctx, reg.SourceID, inputledger.Input{
		ProducerUnitKey: "physical.a1.epoch." + epoch,
		Kind:            reg.ActionType, SchemaVersion: uint32(reg.ActionVersion),
		Actor: []any{uint64(0)}, Payload: value,
		DeclaredEffectiveAt: cycle.Instant, Eligibility: eligibility,
		Provenance: []any{uint64(0)}, IdempotencyKey: []any{uint64(0)},
	})
	if err != nil {
		return zero, err
	}
	_, err = ledger.CloseSource(ctx, reg.SourceID, inputledger.Closure{
		ProducerKey:   "physical.a1.closed." + epoch,
		ClosedThrough: eligibility, Final: false,
	})
	if err != nil {
		return zero, err
	}
	return unit, nil
}

// CommitPhysical is the only Go-owned physical WORK terminal builder. Its
// signed source must already be in the exact admitted InputLedger prefix;
// SQLite re-verifies the entire proof and reducer result inside CommitWork.
func (c *Coordinator) CommitPhysical(ctx context.Context, ledger *inputledger.Ledger,
	cycle admission.Cycle, fence admission.Fence, approved embodimentv1.ApprovedCut,
) (ports.CanonicalRecord, error) {
	var zero ports.CanonicalRecord
	if c == nil || c.bundle == nil || c.store == nil || ledger == nil || !ledger.SharesFenceAuthority(c.store) {
		return zero, errors.New("physical terminal requires shared durable authority")
	}
	reg, err := c.bundle.PhysicalRegistration()
	if err != nil {
		return zero, err
	}
	cutRaw, signature, completed, err := approved.CanonicalProof()
	if err != nil {
		return zero, err
	}
	manifestRaw, err := c.bundle.PhysicalManifestCanonical()
	if err != nil {
		return zero, err
	}
	cut, err := embodimentv1.DecodeSignedCut(cutRaw, signature)
	if err != nil {
		return zero, err
	}
	view, prefix, err := ledger.AdmissionView(ctx, c.bundle)
	if err != nil {
		return zero, err
	}
	if err := admission.VerifyFence(c.bundle, view, cycle, c.admissionPolicies, fence); err != nil {
		return zero, err
	}
	persisted, err := c.store.GetFence(ctx, [32]byte(fence.ID))
	if err != nil || !bytes.Equal(persisted, fence.Canonical) {
		return zero, errors.New("physical fence is not durably published")
	}
	_, inputs, _, _, _, run := view.Snapshot()
	if cycle.Plan != nil || uint64(cycle.Ordinal) > math.MaxInt64 {
		return zero, errors.New("physical terminal cannot skip a round or clock plan")
	}
	g, err := c.store.ValidatedGenesis(ctx, [32]byte(run), c.bundle)
	if err != nil {
		return zero, err
	}
	if err := embodimentv1.VerifySignedCut(cut, reg, run, g.Digest); err != nil {
		return zero, err
	}
	units, ok := fence.Fields[11].([]any)
	if !ok || len(units) != 1 {
		return zero, errors.New("physical WORK requires exactly one fenced source")
	}
	entry, ok := units[0].([]any)
	if !ok || len(entry) != 5 || entry[1] != uint64(1) || entry[2] != reg.SourceID {
		return zero, errors.New("physical source is not a registered exogenous input")
	}
	sourceID, ok := entry[3].([]byte)
	if !ok || len(sourceID) != 32 {
		return zero, errors.New("invalid physical source ID")
	}
	sourceDigest, ok := entry[4].([]byte)
	if !ok || len(sourceDigest) != 32 {
		return zero, errors.New("invalid physical source digest")
	}
	var input inputledger.StoredInput
	found := false
	for _, v := range inputs {
		if bytes.Equal(v.ID[:], sourceID) {
			if found {
				return zero, errors.New("duplicate source identity")
			}
			input = v
			found = true
		}
	}
	if !found || !bytes.Equal(input.Digest[:], sourceDigest) || len(input.Fields) != 13 ||
		input.Fields[2] != reg.SourceID || input.Fields[4] != reg.ActionType ||
		input.Fields[5] != uint64(reg.ActionVersion) {
		return zero, errors.New("fence does not bind authenticated physical ingress")
	}
	pv, ok := input.Fields[7].([]any)
	if !ok || len(pv) != 5 || pv[0] != "physical.source-cut.v1" {
		return zero, errors.New("physical InputLedger payload missing")
	}
	expected, err := codec.CanonicalPayload([]any{"physical.source-cut.v1", manifestRaw, cutRaw, signature, completed})
	if err != nil {
		return zero, err
	}
	stored, err := codec.CanonicalPayload(pv)
	if err != nil || !bytes.Equal(expected, stored) {
		return zero, errors.New("physical ingress differs from approved cut")
	}
	state, err := c.store.LoadClock(ctx, [32]byte(run))
	if err != nil {
		return zero, err
	}
	if state.Instant != cycle.Instant || state.Revision != cycle.BaseRevision ||
		state.LastOrdinal >= int64(cycle.Ordinal) || uint64(state.Revision) == math.MaxUint64 {
		return zero, errors.New("stale physical cycle coordinate")
	}
	if abort, hit, err := c.abortIfCascadeExceeded(ctx, run, cycle, fence); hit || err != nil {
		return abort, err
	}
	before, hash, err := c.store.LoadWorldState(ctx, uint64(state.Revision))
	if err != nil || hash != cycle.BaseStateHash {
		return zero, errors.New("physical predecessor not bound to fence")
	}
	updates, err := reducer.DecodePhysical(completed)
	if err != nil {
		return zero, err
	}
	after, afterHash, err := reducer.ApplyCompletedPhysical(before, updates)
	if err != nil {
		return zero, err
	}
	d := identity.New(c.bundle)
	policies, err := policiesByRole(c.bundle)
	if err != nil {
		return zero, err
	}
	var id domain.ID
	copy(id[:], sourceID)
	var digest domain.Digest
	copy(digest[:], sourceDigest)
	ref := []any{uint64(1), id[:], digest[:]}
	candidate, err := decision.MakeCandidate(d, run, cycle.ID, []domain.ID{id}, reg.ActionProducer,
		uint32(reg.ActionVersion), 0, 0, []any{}, []any{ref}, 0, policies["conflict-resolvers"])
	if err != nil {
		return zero, err
	}
	draft := []any{uint64(0), uint64(0), "physical.advanced", completed, []any{}}
	candidate.Fields[8] = []any{draft}
	candidate.Fields[12] = []any{"physical.monotonic-hours"}
	candidate.Digest, _, err = d.Sum("cote.csf.digest.commit-candidate",
		"cote.csf.schema.commit-candidate.body", 1, candidate.Fields)
	if err != nil {
		return zero, err
	}
	candidate.Canonical, err = codec.CanonicalPayload(append(cloneAny(candidate.Fields), candidate.Digest[:]))
	if err != nil {
		return zero, err
	}
	event, err := settlement.PhysicalAdvancedEvent(run, cycle.ID, candidate.ID, id, digest,
		state.Instant, state.Revision, state.NextLogicalSequence, completed)
	if err != nil {
		return zero, err
	}
	decisionID, _, err := d.Sum("cote.csf.id.decision",
		"cote.csf.schema.decision-id-preimage", 1,
		[]any{cycle.ID[:], uint64(cycle.Attempt), id[:]})
	if err != nil {
		return zero, err
	}
	decisionFields := []any{decisionID[:], run[:], cycle.ID[:], uint64(cycle.Attempt),
		ref, uint64(0), []any{uint64(1), candidate.ID[:]}, "physical.applied",
		[]any{}, policies["validators"], policies["conflict-resolvers"],
		[]any{}, []any{event.Identity[:]}, []any{uint64(0)}, []any{uint64(0)}}
	decisionDigest, _, err := d.Sum("cote.csf.digest.decision-record",
		"cote.csf.schema.decision-record.body", 1, decisionFields)
	if err != nil {
		return zero, err
	}
	decisionBytes, err := codec.CanonicalPayload(append(cloneAny(decisionFields), decisionDigest[:]))
	if err != nil {
		return zero, err
	}
	decisionRecord := ports.CanonicalRecord{Identity: [32]byte(decisionID), Canonical: decisionBytes}
	decisionsHash, _, err := d.Sum("cote.csf.digest.decision",
		"cote.csf.schema.decision-pair-list", 1, []any{[]any{id[:], decisionID[:], decisionDigest[:]}})
	if err != nil {
		return zero, err
	}
	tasksHash, _, err := d.Sum("cote.csf.digest.perception-task-list",
		"cote.csf.schema.perception-task-id-list", 1, []any{})
	if err != nil {
		return zero, err
	}
	next := state
	if uint64(next.NextLogicalSequence) == math.MaxUint64 {
		return zero, errors.New("physical logical sequence overflow")
	}
	next.Revision++
	next.LastOrdinal = int64(cycle.Ordinal)
	next.NextLogicalSequence++
	oldClock, err := clock.Encode(state)
	if err != nil {
		return zero, err
	}
	newClock, err := clock.Encode(next)
	if err != nil {
		return zero, err
	}
	revision, err := codec.CanonicalPayload([]any{uint64(state.Revision), before, hash[:],
		uint64(next.Revision), after, afterHash[:]})
	if err != nil {
		return zero, err
	}
	journalFields := []any{run[:], cycle.ID[:], uint64(cycle.Attempt), int64(cycle.Instant),
		uint64(cycle.Ordinal), []any{uint64(0)}, uint64(state.Revision), hash[:],
		uint64(next.Revision), afterHash[:], fence.Digest[:], []any{id[:]}, fence.InputDigest[:],
		policies["cycle-coordinate"], policies["event-order"], []any{event.Identity[:]},
		[]any{decisionID[:]}, decisionsHash[:], []any{}, tasksHash[:],
		policies["perception"], policies["perception-identity"], uint64(next.NextLogicalSequence)}
	batchHash, _, err := d.Sum("cote.csf.digest.batch", "cote.csf.schema.batch-preimage", 1,
		[]any{journalFields, []any{event.Canonical}})
	if err != nil {
		return zero, err
	}
	commitID, _, err := d.Sum("cote.csf.id.cycle-commit", "cote.csf.schema.cycle-commit-id-preimage",
		1, []any{run[:], cycle.ID[:], uint64(cycle.Attempt)})
	if err != nil {
		return zero, err
	}
	journalBytes, err := codec.CanonicalPayload(append(cloneAny(journalFields), batchHash[:]))
	if err != nil {
		return zero, err
	}
	batch := ports.WorkBatch{
		Mode: 9,
		Physical: &ports.PhysicalPublication{
			Bundle: c.bundle, Manifest: manifestRaw, SignedCut: cutRaw, Signature: signature, CompletedTyped: completed,
			Candidate: ports.CanonicalRecord{Identity: [32]byte(candidate.ID), Canonical: candidate.Canonical},
		},
		RunID: [32]byte(run), CycleID: [32]byte(cycle.ID), FenceID: [32]byte(fence.ID),
		ExpectedClock: oldClock, ResultClock: newClock, ExpectedInputPrefix: prefix,
		Journal:   ports.CanonicalRecord{Identity: [32]byte(commitID), Canonical: journalBytes},
		Decisions: []ports.CanonicalRecord{decisionRecord}, Events: []ports.CanonicalRecord{event},
		BaseStateHash: [32]byte(hash), ResultStateHash: [32]byte(afterHash),
		RevisionCanonical: revision, FailAfterStep: c.failAfterStep,
	}
	if err := c.commitPreparedWork(ctx, &batch); err != nil {
		return zero, fmt.Errorf("physical WORK rejected: %w", err)
	}
	return batch.Journal, nil
}

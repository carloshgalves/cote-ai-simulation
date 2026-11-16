//go:build linux

package commit

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/admission"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/inputledger"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
)

// referenceD2SettleScriptedCognitiveSlots is deliberately a real,
// deterministic Go NoProposal WORK, not a database patch or silent
// deletion of C2's live future actor RoundDeclarations.
//
// C2's two authenticated dispatches are both bound to t=35/ordinal=1.
// An A1 source cannot legally leap over unresolved cognitive slots: the
// AdmissionFence rejects missing responses. This synthetic *scripted*
// no-proposal boundary is the explicit causal event that closes those slots
// before epoch 1 can be admitted at t=35/ordinal=2.
func referenceD2SettleScriptedCognitiveSlots(ctx context.Context,
	store *sqlite.Store, ledger *inputledger.Ledger, b *policy.Bundle, run domain.ID,
	c *Coordinator, policies admission.Policies) (ports.CanonicalRecord, error) {
	var empty ports.CanonicalRecord
	state, err := store.LoadClock(ctx, [32]byte(run))
	if err != nil || state.Revision != 6 || state.Instant != 35 || state.LastOrdinal != 0 {
		return empty, errors.New("D2 NoProposal requires exact post-C4 clock")
	}
	if err := store.CheckPerceptionBarrier(ctx, [32]byte(run), 6); err != nil {
		return empty, fmt.Errorf("D2 epistemic barrier before NoProposal: %w", err)
	}
	cycle, err := referenceC1Cycle(ctx, store, b, run, state, 1)
	if err != nil {
		return empty, err
	}
	_, _, _, rounds, _, actualRun := ledger.Snapshot()
	if actualRun != run {
		return empty, errors.New("D2 cognitive slot prefix belongs to foreign run")
	}
	selected := 0
	d := identity.New(b)
	for _, round := range rounds {
		if round.Lifecycle != 0 ||
			round.Eligibility.NotBeforeInstant != state.Instant ||
			round.Eligibility.NotBeforeCycleOrdinal != 1 {
			continue
		}
		if round.SourceID != "schedule" || round.Actor[0] != "school" ||
			(round.Actor[1] != "a" && round.Actor[1] != "b") ||
			len(round.Slots) != 1 || round.Slots[0] != "act" {
			return empty, errors.New("D2 unexpected unclosed cognitive input in next actor cohort")
		}
		dispatchID, _, err := d.Sum("cote.csf.id.dispatch",
			"cote.csf.schema.dispatch-id-preimage", 1,
			[]any{run[:], cycle.ID[:], round.ID[:], "act", uint64(0)})
		if err != nil {
			return empty, err
		}
		responseID, _, err := d.Sum("cote.csf.id.no-proposal",
			"cote.csf.schema.slot-response-id-preimage", 1,
			[]any{dispatchID[:], uint64(1)})
		if err != nil {
			return empty, err
		}
		body := []any{
			responseID[:], dispatchID[:], run[:], cycle.ID[:], round.ID[:],
			"act", []any{round.Actor[0], round.Actor[1]},
			int64(state.Instant), uint64(state.Revision), uint64(0),
		}
		digest, _, err := d.Sum("cote.csf.digest.no-proposal-unit",
			"cote.csf.schema.no-proposal.body", 1, body)
		if err != nil {
			return empty, err
		}
		canonical, err := codec.CanonicalPayload(append(append([]any(nil), body...), digest[:]))
		if err != nil {
			return empty, err
		}
		response := inputledger.Response{
			Kind: 1, ID: domain.ID(responseID), Digest: digest,
			Canonical: canonical, Body: body,
			DispatchID: domain.ID(dispatchID), RunID: run,
			CycleID: cycle.ID, RoundID: round.ID, SlotID: "act",
			Actor: round.Actor, EffectiveAt: state.Instant,
			SubmittedAgainstRevision: state.Revision,
		}
		if err := ledger.Respond(ctx, response); err != nil {
			return empty, fmt.Errorf("D2 scripted NoProposal is not a registered C2 dispatch: %w", err)
		}
		selected++
	}
	if selected != 2 {
		return empty, errors.New("D2 requires exactly A/B authenticated C2 next actor dispatches")
	}
	fence, err := admission.NewCoordinator(admission.NewFenceLog(store)).
		AdmitThenEvaluate(ctx, b, ledger, cycle, policies, nil)
	if err != nil {
		return empty, fmt.Errorf("D2 native NoProposal AdmissionFence failed: %w", err)
	}
	accepted, ok := fence.Fields[11].([]any)
	if !ok || len(accepted) != 2 {
		return empty, errors.New("D2 NoProposal fence did not admit precisely two actor slots")
	}
	for _, item := range accepted {
		fields, ok := item.([]any)
		if !ok || len(fields) != 5 || fields[1] != uint64(0) {
			return empty, errors.New("D2 NoProposal fence admitted physical/event source prematurely")
		}
	}
	before, beforeHash, err := store.LoadWorldState(ctx, 6)
	if err != nil {
		return empty, err
	}
	journal, err := c.CommitNoProposal(ctx, ledger, cycle, fence)
	if err != nil {
		return empty, fmt.Errorf("D2 native scripted NoProposal WORK failed: %w", err)
	}
	after, afterHash, err := store.LoadWorldState(ctx, 7)
	if err != nil || afterHash != beforeHash {
		return empty, errors.New("D2 NoProposal changed WorldState hash")
	}
	left, err := codec.CanonicalPayload(before)
	if err != nil {
		return empty, err
	}
	right, err := codec.CanonicalPayload(after)
	if err != nil || !bytes.Equal(left, right) {
		return empty, errors.New("D2 NoProposal changed composed WorldState bytes")
	}
	clock, err := store.LoadClock(ctx, [32]byte(run))
	if err != nil || clock.Revision != 7 || clock.Instant != 35 || clock.LastOrdinal != 1 {
		return empty, errors.New("D2 NoProposal did not settle the exact cognitive cohort")
	}
	if err := store.CheckPerceptionBarrier(ctx, [32]byte(run), 7); err != nil {
		return empty, fmt.Errorf("D2 NoProposal left incomplete epistemic obligation: %w", err)
	}
	return journal, nil
}

//go:build linux

package commit

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"fmt"
	"math"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/adapter/embodimentv1"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/admission"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/inputledger"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/reducer"
	"github.com/carloshgalves/cote-ai-simulation/internal/physicalhost"
)

// ReferenceD2Config carries trusted local host configuration. It is not
// produced by a simulated actor, a JSON message or a model. The physical
// registration, interpreter and all Python imports must be authenticated
// by the existing A1 supervisor before any physical ingress.
type ReferenceD2Config struct {
	Host                  physicalhost.ProducerHost
	EpochPlans            [2]physicalhost.ProducerPlan
	FailPhysicalAtEpoch   int   // 0 disabled; local fault-injection negative tests only
	FailPhysicalAfterStep uint8 // 1..7 only when FailPhysicalAtEpoch is set
}

// ReferenceD2Proof is an INTERNAL privileged test oracle. Neither the raw
// C4 result, body hours, stock, nor world hashes may be published directly.
type ReferenceD2Proof struct {
	Status                             string
	RunID                              string
	C4                                 *ReferenceResult `json:"-"`
	FinalRevision                      uint64
	FinalInstant                       int64
	CommittedPhysicalEpochs            int
	ScriptedNoProposalJournalID        string
	ScriptedNoProposalVerified         bool
	PhysicalInputIDs                   [2]string
	PhysicalJournalIDs                 [2]string
	PhysicalEventIDs                   [2]string
	FiveLedgerDigests                  map[string]string
	GenesisSHA256                      string
	FinalWorldHash                     string             `json:"-"`
	PrivilegedBodyHours                map[string]float64 `json:"-"`
	PrivilegedFinalStock               uint64             `json:"-"`
	SnapshotVerifiedWithThreeTailCommits bool
	DigestChainVerified                bool
	ReplayVerified                     bool
	A1SignedByGenesisKey               bool
	SameCausalRun                      bool
	ResourceBranchUnchanged            bool
	BodyContinuityVerified             bool
	FinalSourceClosureVerified         bool
}

// ReferenceD2PublicReport has a fixed allowlist, separate from privileged
// ReferenceD2Proof and the engine-oriented ReferenceResult.
type ReferenceD2PublicReport struct {
	Status    string                     `json:"status"`
	RunID     string                     `json:"run_id"`
	C4        ReferenceC4PublicReport    `json:"c4"`
	Physical  ReferenceD2PublicPhysical  `json:"physical"`
	Integrity ReferenceD2PublicIntegrity `json:"integrity"`
}
type ReferenceD2PublicPhysical struct {
	EpochsCommitted int       `json:"epochs_committed"`
	InputIDs        [2]string `json:"input_ids"`
	JournalIDs      [2]string `json:"journal_ids"`
	EventIDs        [2]string `json:"event_ids"`
	FinalRevision   uint64    `json:"final_revision"`
	FinalInstant    int64     `json:"final_instant"`
	SourceFinalized bool      `json:"source_finalized"`
	ScriptedNoProposalJournalID string `json:"scripted_no_proposal_journal_id"`
}
type ReferenceD2PublicIntegrity struct {
	LedgerDigests               map[string]string `json:"ledger_digests"`
	SnapshotTailCommitsVerified bool              `json:"snapshot_tail_commits_verified"`
	AuthenticatedA1Cut          bool              `json:"authenticated_a1_cut"`
	SameCausalRun               bool              `json:"same_causal_run"`
	ResourcePreserved           bool              `json:"resource_preserved"`
	BodyContinuityVerified      bool              `json:"body_continuity_verified"`
	ReplayVerified              bool              `json:"replay_verified"`
}

func (p ReferenceD2Proof) PublicReport() (ReferenceD2PublicReport, error) {
	var zero ReferenceD2PublicReport
	if p.Status != "RUN_LOCAL_10D2" || p.C4 == nil || p.RunID != p.C4.RunID ||
		p.FinalRevision != 9 || p.FinalInstant != 35 || p.CommittedPhysicalEpochs != 2 ||
		!p.SnapshotVerifiedWithThreeTailCommits || !p.DigestChainVerified ||
		!p.ReplayVerified || !p.A1SignedByGenesisKey || !p.SameCausalRun ||
		!p.ResourceBranchUnchanged || !p.BodyContinuityVerified ||
		!p.FinalSourceClosureVerified || !p.ScriptedNoProposalVerified ||
		len(p.ScriptedNoProposalJournalID) != 64 || len(p.FiveLedgerDigests) != 5 {
		return zero, errors.New("D2 has incomplete or unverified causal/public proof")
	}
	c4, err := p.C4.C4PublicReport()
	if err != nil {
		return zero, err
	}
	for _, kind := range referenceC4LedgerNames {
		digest := p.FiveLedgerDigests[kind]
		if len(digest) != 64 {
			return zero, errors.New("D2 public ledger digest unavailable")
		}
	}
	for i := 0; i < 2; i++ {
		for _, token := range []string{p.PhysicalInputIDs[i], p.PhysicalJournalIDs[i], p.PhysicalEventIDs[i]} {
			if len(token) != 64 {
				return zero, errors.New("D2 missing opaque physical identity")
			}
		}
	}
	digests := make(map[string]string, 5)
	for _, kind := range referenceC4LedgerNames {
		digests[kind] = p.FiveLedgerDigests[kind]
	}
	return ReferenceD2PublicReport{
		Status: p.Status, RunID: p.RunID, C4: c4,
		Physical: ReferenceD2PublicPhysical{
			EpochsCommitted: 2, InputIDs: p.PhysicalInputIDs,
			JournalIDs: p.PhysicalJournalIDs, EventIDs: p.PhysicalEventIDs,
			FinalRevision: p.FinalRevision, FinalInstant: p.FinalInstant,
			SourceFinalized: p.FinalSourceClosureVerified,
			ScriptedNoProposalJournalID: p.ScriptedNoProposalJournalID,
		},
		Integrity: ReferenceD2PublicIntegrity{
			LedgerDigests: digests, SnapshotTailCommitsVerified: true,
			AuthenticatedA1Cut: true, SameCausalRun: true, ResourcePreserved: true,
			BodyContinuityVerified: true, ReplayVerified: true,
		},
	}, nil
}

func referenceD2StockBytes(state []any) ([]byte, error) {
	// Closed nested CBOR shape; DecodeComposite must have authenticated both
	// children and the outer content digest before this raw projection.
	if _, _, err := reducer.DecodeComposite(state); err != nil {
		return nil, err
	}
	outer := state[2].([]byte)
	env, err := codec.StrictDecodeEnvelope(outer)
	if err != nil {
		return nil, err
	}
	stock := env.Value.([]any)[2].([]any)[0].([]any)
	return bytes.Clone(stock[2].([]byte)), nil
}

func referenceD2Continuity(before, after []reducer.PhysicalBody) bool {
	if len(before) == 0 || len(before) != len(after) {
		return false
	}
	changed := false
	for i := range before {
		if before[i].Subject != after[i].Subject ||
			after[i].Hours < before[i].Hours ||
			after[i].Hours > math.MaxFloat64 {
			return false
		}
		if after[i].Hours > before[i].Hours {
			changed = true
		}
	}
	return changed
}

// RunReferenceD2 is a single, real C4+two-A1-epoch causal run. It FIRST
// executes the authentic C1+C2+C3+C4 script on a composite Genesis, then
// uses the SAME SQLite and RunID for the actual sealed Python producer,
// signed cut, Go InputLedger, AdmissionFence and CommitPhysical twice.
//
// No alternate physical database, precomputed A2 fixture, unsigned cut,
// manual WorldRevision mutation, callback to a cognitive actor or model-run
// physical projection is permitted on the D2 happy path.
func RunReferenceD2(ctx context.Context, dbPath, ownerPath, bundlePath string,
	cfg ReferenceD2Config, options ReferenceOptions) (ReferenceD2Proof, error) {
	var zero ReferenceD2Proof
	if ctx == nil || dbPath == "" || ownerPath == "" || cfg.Host.Bundle == nil ||
		len(cfg.Host.GenesisCanonical) != 0 ||
		len(cfg.Host.PrivateKey) != ed25519.PrivateKeySize ||
		(cfg.FailPhysicalAtEpoch != 0 && cfg.FailPhysicalAtEpoch != 1 && cfg.FailPhysicalAtEpoch != 2) ||
		(cfg.FailPhysicalAtEpoch == 0 && cfg.FailPhysicalAfterStep != 0) ||
		(cfg.FailPhysicalAtEpoch != 0 && (cfg.FailPhysicalAfterStep < 1 || cfg.FailPhysicalAfterStep > 7)) {
		return zero, errors.New("D2 invalid trusted physical host or fault configuration")
	}
	if err := cfg.Host.Bundle.VerifyInstalledBundle(); err != nil {
		return zero, err
	}
	reg, err := cfg.Host.Bundle.PhysicalRegistration()
	if err != nil {
		return zero, err
	}
	if reg.Disclosure != "ENGINE_ONLY" ||
		!bytes.Equal(cfg.Host.PrivateKey.Public().(ed25519.PublicKey), reg.PublicKeyEd25519[:]) {
		return zero, errors.New("D2 signer is not the Genesis-registration issuer")
	}
	if options.physicalBundle != nil {
		return zero, errors.New("D2 does not accept foreign bundle override")
	}
	options.physicalBundle = cfg.Host.Bundle
	options.IncludeAbortRetry = true
	options.IncludeCommunication = true
	c4, err := RunReferenceC4(ctx, dbPath, ownerPath, bundlePath, options)
	if err != nil {
		return zero, fmt.Errorf("D2 prerequisite C4 failed: %w", err)
	}
	if c4.Status != "RUN_LOCAL_10C4" || c4.FinalRevision != 6 || c4.FinalInstant != 35 {
		return zero, errors.New("D2 cannot begin without genuine C4 canonical history")
	}
	rawRun, err := referenceC4DecodeID(c4.RunID)
	if err != nil {
		return zero, err
	}
	run := domain.ID(rawRun)
	store, err := sqlite.Open(ctx, dbPath)
	if err != nil {
		return zero, err
	}
	defer store.Close()
	g, err := store.ValidatedGenesis(ctx, rawRun, cfg.Host.Bundle)
	if err != nil || g.RunID != run {
		return zero, errors.New("D2 Genesis was not authenticated after C4")
	}
	if _, _, err := reducer.DecodeComposite(g.Payload[16]); err != nil {
		return zero, errors.New("D2 Genesis lacks the physical+inventory compound world")
	}
	snapshotID, err := referenceC4DecodeID(c4.Epistemic.SnapshotID)
	if err != nil {
		return zero, err
	}
	beforeSnap, err := store.VerifySnapshotV1(ctx, rawRun, snapshotID, cfg.Host.Bundle)
	if err != nil || beforeSnap.TailCommits != 0 {
		return zero, errors.New("D2 C4 snapshot not sealed at six commits")
	}
	ledger, err := inputledger.Open(ctx, run, cfg.Host.Bundle, store)
	if err != nil {
		return zero, err
	}
	roles, err := policiesByRole(cfg.Host.Bundle)
	if err != nil {
		return zero, err
	}
	policies := admission.Policies{
		AdmissionOrder: roles["admission-order"], CausalIdentity: roles["causal-identity"],
		Fence: roles["admission-fence"], Rules: [][]any{},
	}
	coordinator := New(store, cfg.Host.Bundle, policies)
	cfg.Host.GenesisCanonical = bytes.Clone(g.CanonicalBytes)
	state, err := store.LoadClock(ctx, rawRun)
	if err != nil || uint64(state.Revision) != 6 || state.Instant != 35 || state.LastOrdinal != 0 {
		return zero, errors.New("D2 inherited unexpected C4 authoritative clock")
	}
	initial, _, err := store.LoadWorldState(ctx, 6)
	if err != nil {
		return zero, err
	}
	baseStock, err := referenceD2StockBytes(initial)
	if err != nil {
		return zero, err
	}
	bodyBefore, err := reducer.DecodePhysical(initial)
	if err != nil {
		return zero, err
	}
	// C2's two authentic future A/B dispatches are outstanding at
	// t=35/ordinal=1. An A1 cycle cannot skip them: the normative
	// AdmissionFence requires explicit terminal NoProposal responses.
	// This adds a world-neutral seventh CycleCommit, after which the
	// two physical epochs are eligible at ordinals 2 and 3.
	noProposalJournal, err := referenceD2SettleScriptedCognitiveSlots(
		ctx, store, ledger, cfg.Host.Bundle, run, coordinator, policies)
	if err != nil {
		return zero, fmt.Errorf("D2 could not settle C2 outstanding actor slots: %w", err)
	}
	var proof ReferenceD2Proof
	proof.C4 = &c4
	proof.RunID = c4.RunID
	proof.GenesisSHA256 = c4.GenesisSHA256
	proof.ScriptedNoProposalJournalID = hex.EncodeToString(noProposalJournal.Identity[:])
	proof.ScriptedNoProposalVerified = true
	proof.SameCausalRun = true
	proof.A1SignedByGenesisKey = true
	proof.ResourceBranchUnchanged = true
	proof.BodyContinuityVerified = true
	for i := 0; i < 2; i++ {
		// The producer's epoch is a source unit identity. It does not
		// create a SimulationInstant, CycleOrdinal or LogicalSequence.
		// The Go clock alone chooses the next admissible cycle.
		host := cfg.Host
		host.Epoch = uint64(i + 1)
		host.Plan = cfg.EpochPlans[i]
		approved, err := physicalhost.RunSupervisedPhysical(ctx, host)
		if err != nil {
			return zero, fmt.Errorf("D2 real Python A1 epoch %d refused: %w", i+1, err)
		}
		cutRaw, signature, _, err := approved.CanonicalProof()
		if err != nil {
			return zero, err
		}
		signed, err := embodimentv1.DecodeSignedCut(cutRaw, signature)
		if err != nil || signed.Epoch != uint64(i+1) {
			return zero, errors.New("D2 A1 cut identity/epoch was swapped")
		}
		if err := embodimentv1.VerifySignedCut(signed, reg, run, g.Digest); err != nil {
			return zero, errors.New("D2 A1 cut failed Genesis-bound signer authentication")
		}
		state, err = store.LoadClock(ctx, rawRun)
		if err != nil {
			return zero, err
		}
		if state.Instant != 35 || state.LastOrdinal < 0 || state.LastOrdinal >= math.MaxInt64 {
			return zero, errors.New("D2 Python altered or exhausted global causal coordinate")
		}
		before, baseHash, err := store.LoadWorldState(ctx, uint64(state.Revision))
		if err != nil {
			return zero, err
		}
		nextOrdinal := domain.CycleOrdinal(state.LastOrdinal + 1)
		id, _, err := identity.New(cfg.Host.Bundle).Sum("cote.csf.id.cycle",
			"cote.csf.schema.cycle-id-preimage", 1,
			[]any{run[:], []any{int64(state.Instant), uint64(nextOrdinal)}})
		if err != nil {
			return zero, err
		}
		cycle := admission.Cycle{
			ID: domain.ID(id), Instant: state.Instant, Ordinal: nextOrdinal,
			Attempt: 0, BaseRevision: state.Revision,
			BaseStateHash: domain.Digest(baseHash), RetryRef: []any{uint64(0)},
		}
		source, err := coordinator.StagePhysicalInput(ctx, ledger, approved, cycle)
		if err != nil {
			return zero, fmt.Errorf("D2 authentic source ingress %d: %w", i+1, err)
		}
		fence, err := admission.NewCoordinator(admission.NewFenceLog(store)).
			AdmitThenEvaluate(ctx, cfg.Host.Bundle, ledger, cycle, policies, nil)
		if err != nil {
			return zero, fmt.Errorf("D2 AdmissionFence epoch %d: %w", i+1, err)
		}
		if cfg.FailPhysicalAtEpoch == i+1 {
			coordinator.failAfterStep = cfg.FailPhysicalAfterStep
		}
		journal, err := coordinator.CommitPhysical(ctx, ledger, cycle, fence, approved)
		coordinator.failAfterStep = 0
		if err != nil {
			return zero, fmt.Errorf("D2 atomic CommitPhysical epoch %d: %w", i+1, err)
		}
		event, err := referenceD2PhysicalEventID(journal.Canonical)
		if err != nil {
			return zero, err
		}
		world, _, err := store.LoadWorldState(ctx, uint64(state.Revision+1))
		if err != nil {
			return zero, err
		}
		stock, err := referenceD2StockBytes(world)
		if err != nil || !bytes.Equal(stock, baseStock) {
			return zero, errors.New("D2 A1 physical evolution modified Go-owned stock bytes")
		}
		prevStock, err := referenceD2StockBytes(before)
		if err != nil || !bytes.Equal(stock, prevStock) {
			return zero, errors.New("D2 A1 physical transition modified immediate stock predecessor")
		}
		after, err := reducer.DecodePhysical(world)
		if err != nil || !referenceD2Continuity(bodyBefore, after) {
			return zero, errors.New("D2 A1 physical epoch erased/staled original body continuation")
		}
		bodyBefore = after
		proof.PhysicalInputIDs[i] = hex.EncodeToString(source.ID[:])
		proof.PhysicalJournalIDs[i] = hex.EncodeToString(journal.Identity[:])
		proof.PhysicalEventIDs[i] = hex.EncodeToString(event[:])
		proof.CommittedPhysicalEpochs++
	}
	last, err := store.LoadClock(ctx, rawRun)
	if err != nil || uint64(last.Revision) != 9 || last.Instant != 35 || last.LastOrdinal != 3 {
		return zero, errors.New("D2 did not append precisely two physical CycleCommits")
	}
	closure, err := ledger.CloseSource(ctx, reg.SourceID, inputledger.Closure{
		ProducerKey: "physical.a1.final.epoch-2",
		ClosedThrough: domain.EligibilityCoordinate{
			NotBeforeInstant: last.Instant, NotBeforeCycleOrdinal: domain.CycleOrdinal(last.LastOrdinal),
		}, Final: true,
	})
	if err != nil {
		return zero, fmt.Errorf("D2 final physical SourceClosure: %w", err)
	}
	if len(closure.Fields) != 7 || closure.Fields[5] != true {
		return zero, errors.New("D2 physical source still mutable")
	}
	proof.FinalSourceClosureVerified = true
	replayed, err := store.ReplayClock(ctx, rawRun, cfg.Host.Bundle)
	if err != nil || replayed.ReplayedCommits != 9 ||
		replayed.ReplayedEvents != int(c4.ReplayedEvents)+4 ||
		replayed.ReconstructedClock != last {
		return zero, fmt.Errorf("D2 replay failed after two physical epochs: %w", err)
	}
	proof.ReplayVerified = true
	snapshot, err := store.VerifySnapshotV1(ctx, rawRun, snapshotID, cfg.Host.Bundle)
	if err != nil || snapshot.TailCommits != 3 {
		return zero, fmt.Errorf("D2 original owner C4 snapshot did not verify 3-commit causal tail: %w", err)
	}
	proof.SnapshotVerifiedWithThreeTailCommits = true
	finalWorld, finalHash, err := store.LoadWorldState(ctx, 9)
	if err != nil {
		return zero, err
	}
	stock, bodies, err := reducer.DecodeComposite(finalWorld)
	if err != nil || stock.Available != 0 || len(bodies) != 3 {
		return zero, errors.New("D2 final WorldRevision lacks resource and physical state")
	}
	proof.PrivilegedFinalStock = stock.Available
	proof.PrivilegedBodyHours = make(map[string]float64, len(bodies))
	for _, body := range bodies {
		proof.PrivilegedBodyHours[body.Subject] = body.Hours
	}
	proof.FinalWorldHash = hex.EncodeToString(finalHash[:])
	proof.FinalRevision = uint64(last.Revision)
	proof.FinalInstant = int64(last.Instant)
	digests, counts, err := referenceC4FiveLedgers(ctx, dbPath, cfg.Host.Bundle)
	if err != nil || len(digests) != 5 || len(counts) != 5 {
		return zero, errors.New("D2 five ledger prefix recheck failed")
	}
	for _, name := range referenceC4LedgerNames {
		if counts[name] <= 0 || len(digests[name]) != 64 {
			return zero, errors.New("D2 incomplete ledger authority")
		}
	}
	proof.FiveLedgerDigests = digests
	proof.DigestChainVerified = true
	proof.Status = "RUN_LOCAL_10D2"
	if _, err := proof.PublicReport(); err != nil {
		return zero, err
	}
	return proof, nil
}

func referenceD2PhysicalEventID(raw []byte) ([32]byte, error) {
	var zero [32]byte
	val, err := codec.StrictDecodePayload(raw)
	if err != nil {
		return zero, err
	}
	items, ok := val.([]any)
	if !ok || len(items) != 24 {
		return zero, errors.New("D2 noncanonical physical journal")
	}
	events, ok := items[15].([]any)
	if !ok || len(events) != 1 {
		return zero, errors.New("D2 requires exactly one physical event per epoch")
	}
	id, ok := events[0].([]byte)
	if !ok || len(id) != 32 {
		return zero, errors.New("D2 physical event ID malformed")
	}
	copy(zero[:], id)
	return zero, nil
}

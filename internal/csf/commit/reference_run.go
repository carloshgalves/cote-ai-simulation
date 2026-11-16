package commit

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/admission"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/clock"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/extensions/testresourcev1"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/genesis"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/inputledger"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/reducer"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/trigger"
	_ "modernc.org/sqlite"
)

// ReferenceOptions controls only ordering and negative-test inputs, never rule
// outcomes. Two runs with the same set of canonical inputs must agree on
// fence/decision/event bytes even when the host presents sources or workers in
// a different order.
type ReferenceOptions struct {
	ReverseSources   bool
	ReverseResponses bool
	LeaveSourceOpen  bool  // negative control; must stop before the first fence
	FailContestAt    uint8 // internal negative control: 1..7, fail atomic WORK before publication
}

type ReferenceCycle struct {
	Kind          string `json:"kind"`
	Instant       int64  `json:"instant"`
	Ordinal       uint64 `json:"ordinal"`
	FenceID       string `json:"fence_id"`
	FenceDigest   string `json:"fence_digest"`
	JournalID     string `json:"journal_id"`
	JournalSHA256 string `json:"journal_raw_sha256"`
}

type ReferenceResult struct {
	Status              string            `json:"status"`
	RunID               string            `json:"run_id"`
	GenesisSHA256       string            `json:"genesis_raw_sha256"`
	CodecPolicyHash     string            `json:"codec_policy_hash"`
	SchemaBundleHash    string            `json:"schema_bundle_hash"`
	Database            string            `json:"database"`
	Ledgers             map[string]string `json:"ledgers"`
	Cycles              []ReferenceCycle  `json:"cycles"`
	ContestWinner       string            `json:"contest_winner"`
	ContestDecisions    map[string]string `json:"contest_decisions"`
	FinalRevision       uint64            `json:"final_revision"`
	FinalInstant        int64             `json:"final_instant"`
	GenesisWorldHash    string            `json:"genesis_world_hash"`
	ContestWorldHash    string            `json:"contest_world_hash"`
	AdvanceWorldHash    string            `json:"advance_world_hash"`
	FinalWorldHash      string            `json:"final_world_hash"`
	FinalStock          int64             `json:"final_stock"`
	TriggerActivations  int               `json:"trigger_activations"`
	TriggerTerminal     bool              `json:"trigger_terminal"`
	ConsumedSystemUnits int               `json:"consumed_system_units"`
	ReplayedCommits     uint64            `json:"replayed_commits"`
	ReplayedEvents      uint64            `json:"replayed_events"`
	EventStreamSHA256   string            `json:"event_stream_raw_sha256"`
	// Precise, length-delimited canonical bytes are kept for Go test oracles.
	// Never publish these in a public JSON report as they might contain world data.
	EventBytes         []byte `json:"-"`
	ContestFenceBytes  []byte `json:"-"`
	ContestCommitBytes []byte `json:"-"`
	CycleFenceBytes [][]byte `json:"-"`
	CycleCommitBytes [][]byte `json:"-"`
}

// buildReferenceGenesis converts the frozen, normative *fixture vector* into a
// separately validated production-schema Genesis with a real stock world. The
// actual scenario executes every transition against SQLite; it does NOT copy
// any expected decisions, winners or events from the test vector.
func buildReferenceGenesis(bundle *policy.Bundle) (*genesis.Genesis, error) {
	raw, err := os.ReadFile(filepath.Join(bundle.Directory, "fixtures.json"))
	if err != nil {
		return nil, err
	}
	var source struct {
		Semantic []struct {
			CaseID     string `json:"case_id"`
			GenesisHex string `json:"genesis_envelope_hex"`
		} `json:"semantic"`
	}
	if err := json.Unmarshal(raw, &source); err != nil {
		return nil, err
	}
	for _, item := range source.Semantic {
		if item.CaseID != "genesis.pinning" {
			continue
		}
		encoded, err := hex.DecodeString(item.GenesisHex)
		if err != nil {
			return nil, err
		}
		envelope, err := codec.StrictDecodeEnvelope(encoded)
		if err != nil {
			return nil, err
		}
		original, ok := envelope.Value.([]any)
		if !ok || len(original) != 18 {
			return nil, errors.New("normative Genesis fixture has wrong shape")
		}
		fields := append([]any(nil), original...)
		seed := make([]byte, 32)
		for i := range seed {
			seed[i] = byte(i)
		}
		fields[1] = seed
		// The physical.a1 declaration reserves the exact Go source identity, but
		// issues #118/#123 still own authorization of an actual signed A1 input.
		fields[13] = []any{"physical.a1", "school-events"}
		typed, _, err := reducer.EncodeStock(reducer.Stock{Available: 1})
		if err != nil {
			return nil, err
		}
		fields[15] = []any{[]any{reducer.StockSchemaID, uint64(1)}}
		fields[16] = typed
		encoded, err = codec.CanonicalBytes("cote.csf.digest.genesis-manifest", envelope.SchemaID, envelope.SchemaVersion, fields)
		if err != nil {
			return nil, err
		}
		return genesis.ValidateCanonical(encoded, bundle)
	}
	return nil, errors.New("normative Genesis source vector missing")
}

// RunReference performs three real, atomic causal publications in one newly
// allocated SQLite: two-round resource contest, clock advance to a scheduled
// instant, then same-cycle settlement of occurrence and trigger activation.
// It deliberately excludes A1 subprocesses, claims and replay/fork perturbation
// suites assigned to #117-#119. No LLM, network or wall clock selects facts.
func RunReference(ctx context.Context, dbPath, bundlePath string, options ReferenceOptions) (ReferenceResult, error) {
	var out ReferenceResult
	if ctx == nil {
		return out, errors.New("nil context")
	}
	if dbPath == "" {
		return out, errors.New("database path is required")
	}
	abs, err := filepath.Abs(dbPath)
	if err != nil {
		return out, err
	}
	if _, err := os.Lstat(abs); err == nil {
		return out, errors.New("reference run requires a fresh database path")
	} else if !errors.Is(err, os.ErrNotExist) {
		return out, err
	}
	bundle, err := policy.Load(bundlePath)
	if err != nil {
		return out, fmt.Errorf("load sealed policies: %w", err)
	}
	g, err := buildReferenceGenesis(bundle)
	if err != nil {
		return out, fmt.Errorf("build verified reference Genesis: %w", err)
	}
	store, err := sqlite.Open(ctx, abs)
	if err != nil {
		return out, err
	}
	defer store.Close()
	if err := store.PersistGenesis(ctx, g, bundle); err != nil {
		return out, err
	}
	if err := store.BootstrapClock(ctx, g, bundle); err != nil {
		return out, err
	}
	var triggerID domain.ID
	triggerID[0] = 0xb6
	def := trigger.Definition{ID: triggerID, Version: 1, Policy: trigger.RisingEdge, FireOnInitialTrue: true,
		Dependencies: []string{"clock"}, PredicateThreshold: g.InitialInstant + 10}
	if err := store.BootstrapGenesisTriggers(ctx, g, bundle, []trigger.Definition{def}); err != nil {
		return out, err
	}
	deriver := identity.New(bundle)
	due := g.InitialInstant + 10
	origin := []any{uint64(2), []any{g.ID[:], g.Digest[:]}}
	occurrenceID, _, err := deriver.Sum("cote.csf.id.occurrence", "cote.csf.schema.occurrence-id-preimage", 1,
		[]any{origin, uint64(0), uint64(0)})
	if err != nil {
		return out, err
	}
	occurrenceFields := []any{occurrenceID[:], g.RunID[:], "schedule", int64(due),
		[]any{int64(due), uint64(0)}, "test.occurrence", uint64(1), g.Payload[16], uint64(0), origin, uint64(0), uint64(0),
		[]any{uint64(0)}, []any{uint64(0)}}
	occurrenceDigest, _, err := deriver.Sum("cote.csf.digest.occurrence-unit", "cote.csf.schema.scheduled-occurrence.body", 1, occurrenceFields)
	if err != nil {
		return out, err
	}
	occurrenceBytes, err := codec.CanonicalPayload(append(cloneAny(occurrenceFields), occurrenceDigest[:]))
	if err != nil {
		return out, err
	}
	if err := store.SeedGenesisOccurrence(ctx, domain.ID(occurrenceID), g.RunID,
		domain.EligibilityCoordinate{NotBeforeInstant: due, NotBeforeCycleOrdinal: 0}, occurrenceBytes); err != nil {
		return out, err
	}
	ledger, err := inputledger.New(ctx, g.RunID, bundle, store)
	if err != nil {
		return out, err
	}
	sources := []string{"school-events", "physical.a1"}
	if options.ReverseSources {
		sources[0], sources[1] = sources[1], sources[0]
	}
	for _, name := range sources {
		if err := ledger.DeclareSource(inputledger.Source{ID: name}); err != nil {
			return out, err
		}
	}
	zero := domain.EligibilityCoordinate{NotBeforeInstant: g.InitialInstant, NotBeforeCycleOrdinal: 0}
	atDue := domain.EligibilityCoordinate{NotBeforeInstant: due, NotBeforeCycleOrdinal: 0}
	for _, name := range sources {
		if options.LeaveSourceOpen && name == "physical.a1" {
			continue
		}
		if _, err := ledger.CloseSource(ctx, name, inputledger.Closure{ProducerKey: name + ".partial", ClosedThrough: zero}); err != nil {
			return out, err
		}
		if _, err := ledger.CloseSource(ctx, name, inputledger.Closure{ProducerKey: name + ".final", ClosedThrough: atDue, Final: true}); err != nil {
			return out, err
		}
	}
	roles, err := policiesByRole(bundle)
	if err != nil {
		return out, err
	}
	policies := admission.Policies{AdmissionOrder: roles["admission-order"],
		CausalIdentity: roles["causal-identity"], Fence: roles["admission-fence"], Rules: [][]any{}}
	coordinator := New(store, bundle, policies)
	cycleID, _, err := deriver.Sum("cote.csf.id.cycle", "cote.csf.schema.cycle-id-preimage", 1,
		[]any{g.RunID[:], []any{int64(g.InitialInstant), uint64(0)}})
	if err != nil {
		return out, err
	}
	cycleIDValue := domain.ID(cycleID)
	responses := make([]inputledger.Response, 0, 2)
	actorForResponse := map[domain.ID]string{}
	for i, actor := range []string{"a", "b"} {
		round, err := ledger.DeclareRound(ctx, "schedule", [2]string{"school", actor}, []string{"act"}, zero,
			origin, 0, uint32(i), 0, []any{uint64(0)})
		if err != nil {
			return out, err
		}
		dispatchID, _, err := deriver.Sum("cote.csf.id.dispatch", "cote.csf.schema.dispatch-id-preimage", 1,
			[]any{g.RunID[:], cycleIDValue[:], round.ID[:], "act", uint64(0)})
		if err != nil {
			return out, err
		}
		dispatch := inputledger.Dispatch{ID: domain.ID(dispatchID), RunID: g.RunID, CycleID: cycleIDValue,
			RoundID: round.ID, SlotID: "act", Actor: round.Actor, Eligibility: round.Eligibility,
			BaseRevision: 0, EffectiveAt: g.InitialInstant}
		if err := ledger.RegisterDispatch(ctx, dispatch, inputledger.CycleBinding{
			ID: cycleIDValue, BaseRevision: 0, Instant: g.InitialInstant}); err != nil {
			return out, err
		}
		response, err := testresourcev1.ProposalForDispatch(bundle, dispatch)
		if err != nil {
			return out, err
		}
		responses = append(responses, response)
		actorForResponse[response.ID] = "actor." + actor
	}
	if options.ReverseResponses {
		responses[0], responses[1] = responses[1], responses[0]
	}
	for _, response := range responses {
		if err := ledger.Respond(ctx, response); err != nil {
			return out, err
		}
	}
	_, worldHash, err := store.LoadWorldState(ctx, 0)
	if err != nil {
		return out, err
	}
	cycle := admission.Cycle{ID: cycleIDValue, Attempt: 0, Instant: g.InitialInstant,
		Ordinal: 0, BaseRevision: 0, BaseStateHash: domain.Digest(worldHash), RetryRef: []any{uint64(0)}}
	fence, err := admission.NewCoordinator(admission.NewFenceLog(store)).AdmitThenEvaluate(
		ctx, bundle, ledger, cycle, policies, nil)
	if err != nil {
		return out, fmt.Errorf("initial admission: %w", err)
	}
	units, ok := fence.Fields[11].([]any)
	if !ok || len(units) != 2 {
		return out, errors.New("reference contest must admit exactly two slots")
	}
	coordinator.failAfterStep = options.FailContestAt
	result, err := coordinator.CommitTwoResourceContest(ctx, ledger, cycle, fence)
	coordinator.failAfterStep = 0
	if err != nil {
		return out, fmt.Errorf("contested WORK: %w", err)
	}
	firstCycle := referenceCycle("RESOURCE_CONTEST", cycle, fence, result)
	state, err := store.LoadClock(ctx, [32]byte(g.RunID))
	if err != nil {
		return out, err
	}
	if state.Revision != 1 || state.NextLogicalSequence != 3 {
		return out, errors.New("contest did not publish one atomic revision/three events")
	}
	world, contestHash, err := store.LoadWorldState(ctx, 1)
	if err != nil {
		return out, err
	}
	stock, err := reducer.DecodeStock(world)
	if err != nil || stock.Available != 0 {
		return out, errors.New("contest failed to consume exactly one resource")
	}
	decisions, winner, err := readContestDecisions(ctx, abs, result.Canonical, actorForResponse)
	if err != nil {
		return out, err
	}
	advance, err := coordinator.Advance(ctx, ledger, g.RunID)
	if err != nil {
		return out, fmt.Errorf("logical clock advance: %w", err)
	}
	if advance.Plan.Kind != clock.ClockAdvance || advance.Plan.To != due || len(advance.Activations) != 1 {
		return out, errors.New("logical advance did not materialize exactly one durable trigger")
	}
	afterAdvance := advance.ResultClock
	pending, err := store.PendingClock(ctx, [32]byte(g.RunID))
	if err != nil {
		return out, err
	}
	next, has, err := clock.Next(afterAdvance, pending)
	if err != nil || !has || next.Kind != clock.Work || next.OriginOrdinal != 0 || next.Target.NotBeforeInstant != due {
		return out, fmt.Errorf("expected same-coordinate trigger WORK: %+v / %v", next, err)
	}
	afterAdvanceWorld, afterAdvanceHash, err := store.LoadWorldState(ctx, uint64(afterAdvance.Revision))
	if err != nil {
		return out, err
	}
	if check, err := reducer.DecodeStock(afterAdvanceWorld); err != nil || check.Available != 0 {
		return out, errors.New("clock advance mutated inventory")
	}
	triggerCycleID, _, err := deriver.Sum("cote.csf.id.cycle", "cote.csf.schema.cycle-id-preimage", 1,
		[]any{g.RunID[:], []any{int64(due), uint64(0)}})
	if err != nil {
		return out, err
	}
	triggerCycle := admission.Cycle{ID: domain.ID(triggerCycleID), Attempt: 0, Instant: due, Ordinal: 0,
		BaseRevision: afterAdvance.Revision, BaseStateHash: domain.Digest(afterAdvanceHash), RetryRef: []any{uint64(0)}}
	triggerFence, err := admission.NewCoordinator(admission.NewFenceLog(store)).AdmitThenEvaluate(
		ctx, bundle, ledger, triggerCycle, policies, nil)
	if err != nil {
		return out, fmt.Errorf("trigger admission: %w", err)
	}
	systemUnits, ok := triggerFence.Fields[11].([]any)
	if !ok || len(systemUnits) != 2 {
		return out, errors.New("scheduled occurrence and trigger must be co-admitted")
	}
	var triggers, occurrences int
	for _, u := range systemUnits {
		v, ok := u.([]any)
		if !ok || len(v) != 5 {
			return out, errors.New("system unit malformed")
		}
		if v[1] == uint64(3) {
			triggers++
		} else if v[1] == uint64(2) {
			occurrences++
		} else {
			return out, errors.New("unknown system producer")
		}
	}
	if triggers != 1 || occurrences != 1 {
		return out, errors.New("not exactly one trigger and one occurrence")
	}
	systemJournal, err := coordinator.CommitSystemUnit(ctx, ledger, triggerCycle, triggerFence)
	if err != nil {
		return out, fmt.Errorf("trigger terminal WORK: %w", err)
	}
	final, err := store.LoadClock(ctx, [32]byte(g.RunID))
	if err != nil {
		return out, err
	}
	if final.Revision != 3 || final.Instant != due || final.LastOrdinal != 0 {
		return out, errors.New("system WORK did not publish expected successor coordinate")
	}
	finalWorld, finalHash, err := store.LoadWorldState(ctx, uint64(final.Revision))
	if err != nil {
		return out, err
	}
	last, err := reducer.DecodeStock(finalWorld)
	if err != nil || last.Available != 0 {
		return out, errors.New("system WORK modified contested inventory")
	}
	remaining, err := store.PendingClock(ctx, [32]byte(g.RunID))
	if err != nil || len(remaining) != 0 {
		return out, errors.New("durable source remained pending after system settlement")
	}
	replay, err := store.ReplayClock(ctx, [32]byte(g.RunID), bundle)
	if err != nil {
		return out, fmt.Errorf("SQLite causal replay: %w", err)
	}
	if replay.ReconstructedClock != final || replay.ReplayedCommits != 3 || replay.VerifiedActivations != 1 {
		return out, fmt.Errorf("replay mismatched committed run: %+v", replay)
	}
	triggerTerminal,err:=referenceTriggerTerminal(ctx,abs,g.RunID)
    if err!=nil{return out,err}
    events, err := referenceEventBytes(ctx, abs)
	if err != nil {
		return out, err
	}
	rawDigest := sha256.Sum256(events)
	digest := sha256.Sum256(g.CanonicalBytes)
	out = ReferenceResult{Status: "RUN_LOCAL_10B", RunID: hex.EncodeToString(g.RunID[:]),
		GenesisSHA256: hex.EncodeToString(digest[:]), CodecPolicyHash: hex.EncodeToString(bundle.CodecPolicyHash[:]),
		SchemaBundleHash: hex.EncodeToString(bundle.SchemaHash[:]), Database: abs,
		Ledgers:       map[string]string{"input": "input_ledger", "decision": "decision_ledger", "event": "event_store", "evidence": "evidence_ledger", "outbox": "causal_outbox", "snapshots": "snapshots"},
		Cycles:        []ReferenceCycle{firstCycle, referenceCycle("CLOCK_ADVANCE", admission.Cycle{Instant: advance.Plan.From, Ordinal: advance.Plan.OriginOrdinal}, advance.Fence, advance.Commit), referenceCycle("TRIGGER_AND_OCCURRENCE", triggerCycle, triggerFence, systemJournal)},
		ContestWinner: winner, ContestDecisions: decisions, FinalRevision: uint64(final.Revision), FinalInstant: int64(final.Instant),
		GenesisWorldHash: hex.EncodeToString(worldHash[:]), ContestWorldHash: hex.EncodeToString(contestHash[:]),
        AdvanceWorldHash: hex.EncodeToString(afterAdvanceHash[:]),
        FinalWorldHash: hex.EncodeToString(finalHash[:]), FinalStock: int64(last.Available), TriggerActivations: len(advance.Activations),TriggerTerminal:triggerTerminal,
		ConsumedSystemUnits: len(systemUnits), ReplayedCommits: uint64(replay.ReplayedCommits), ReplayedEvents: uint64(replay.ReplayedEvents),
		EventStreamSHA256: hex.EncodeToString(rawDigest[:]), EventBytes: events,
		ContestFenceBytes: bytes.Clone(fence.Canonical), ContestCommitBytes: bytes.Clone(result.Canonical),
        CycleFenceBytes: [][]byte{bytes.Clone(fence.Canonical),bytes.Clone(advance.Fence.Canonical),bytes.Clone(triggerFence.Canonical)},
        CycleCommitBytes: [][]byte{bytes.Clone(result.Canonical),bytes.Clone(advance.Commit.Canonical),bytes.Clone(systemJournal.Canonical)}}
	return out, nil
}

func referenceCycle(name string, cycle admission.Cycle, fence admission.Fence, journal ports.CanonicalRecord) ReferenceCycle {
	digest := sha256.Sum256(journal.Canonical)
	return ReferenceCycle{Kind: name, Instant: int64(cycle.Instant), Ordinal: uint64(cycle.Ordinal),
		FenceID: hex.EncodeToString(fence.ID[:]), FenceDigest: hex.EncodeToString(fence.Digest[:]),
		JournalID: hex.EncodeToString(journal.Identity[:]), JournalSHA256: hex.EncodeToString(digest[:])}
}

func readContestDecisions(ctx context.Context, dbPath string, journal []byte, actorForResponse map[domain.ID]string) (map[string]string, string, error) {
	decisionRecords, err := codec.StrictDecodePayload(journal)
	if err != nil {
		return nil, "", err
	}
	fields, ok := decisionRecords.([]any)
	if !ok || len(fields) < 17 {
		return nil, "", errors.New("invalid contested CycleCommit")
	}
	ids, ok := fields[16].([]any)
	if !ok || len(ids) != 2 {
		return nil, "", errors.New("contest missing exact two terminal decision IDs")
	}
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(dbPath)+"?mode=ro")
	if err != nil {
		return nil, "", err
	}
	defer db.Close()
	decisions := map[string]string{}
	winner := ""
	for _, entry := range ids {
		id, ok := entry.([]byte)
		if !ok || len(id) != 32 {
			return nil, "", errors.New("invalid durable decision ID")
		}
		var raw []byte
		if err := db.QueryRowContext(ctx, "SELECT canonical FROM decision_ledger WHERE id=?", id).Scan(&raw); err != nil {
			return nil, "", err
		}
		val, err := codec.StrictDecodePayload(raw)
		if err != nil {
			return nil, "", err
		}
		decision, ok := val.([]any)
		if !ok || len(decision) != 16 {
			return nil, "", errors.New("invalid persisted DecisionRecord")
		}
		ref, ok := decision[4].([]any)
		if !ok || len(ref) != 4 {
			return nil, "", errors.New("invalid decision source reference")
		}
		source, ok := ref[2].([]byte)
		if !ok || len(source) != 32 {
			return nil, "", errors.New("invalid decision source ID")
		}
		var sourceID domain.ID
		copy(sourceID[:], source)
		actor, known := actorForResponse[sourceID]
		if !known {
			return nil, "", errors.New("decision references unknown actor source")
		}
		if _, exists := decisions[actor]; exists {
			return nil, "", errors.New("duplicate actor DecisionRecord")
		}
		status, ok := decision[5].(uint64)
		if !ok {
			return nil, "", errors.New("decision status not canonical integer")
		}
		switch status {
		case 0:
			if winner != "" {
				return nil, "", errors.New("two winners for one resource")
			}
			winner = actor
			decisions[actor] = "COMMIT"
		case 1:
			decisions[actor] = "REJECT"
		default:
			return nil, "", errors.New("unexpected decision disposition")
		}
	}
	if len(decisions) != 2 || winner == "" {
		return nil, "", errors.New("contest lacks exact one winner and one loser")
	}
	return decisions, winner, nil
}

func referenceEventBytes(ctx context.Context, dbPath string) ([]byte, error) {
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(dbPath)+"?mode=ro")
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, "SELECT id,canonical FROM event_store ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var aggregate []byte
	for rows.Next() {
		var id, raw []byte
		if err := rows.Scan(&id, &raw); err != nil {
			return nil, err
		}
		if len(id) != 32 || len(raw) == 0 {
			return nil, errors.New("missing canonical event bytes")
		}
		aggregate = packRecord(aggregate, id)
		aggregate = packRecord(aggregate, raw)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(aggregate) == 0 {
		return nil, errors.New("event store produced no causal events")
	}
	return aggregate, nil
}

// packRecord adds an unambiguous binary length before every authoritative
// record. It is used only for byte-for-byte comparison, never for causal IDs.
func packRecord(dst []byte, record []byte) []byte {
	var n [8]byte
	binary.BigEndian.PutUint64(n[:], uint64(len(record)))
	return append(append(dst, n[:]...), record...)
}

// referenceTriggerTerminal checks the actual persisted lifecycle. A
// successful trigger activation is not proof that its pending unit was
// consumed: exactly one sealed activation must now be terminal.
func referenceTriggerTerminal(ctx context.Context, path string, run domain.ID) (bool,error) {
    db,err:=sql.Open("sqlite","file:"+filepath.ToSlash(path)+"?mode=ro")
    if err!=nil{return false,err}
    defer db.Close()
    var all,terminal int
    err=db.QueryRowContext(ctx,`SELECT COUNT(*), COALESCE(SUM(CASE WHEN lifecycle=2 THEN 1 ELSE 0 END),0)
            FROM trigger_activations WHERE run_id=?`,run[:]).Scan(&all,&terminal)
    if err!=nil{return false,err}
    if all!=1||terminal!=1{return false,fmt.Errorf("expected one terminal trigger activation, got %d total/%d terminal",all,terminal)}
    return true,nil
}

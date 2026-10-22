// Package ports defines the narrow persistence seams owned by the causal core.
package ports

import ("context"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy")

type CanonicalRecord struct {
	Identity  [32]byte
	Canonical []byte
}

type AppendOnlyStore interface {
	Append(context.Context, CanonicalRecord) error
	Get(context.Context, [32]byte) ([]byte, error)
}

type RecordListingStore interface {
	Records(context.Context) ([]CanonicalRecord, error)
}

// ConditionalAppendStore provides the slot's single-winner CAS. Existing bytes
// are returned so exact redelivery can be distinguished from a competing reply.
type ConditionalAppendStore interface {
	AppendIfAbsent(context.Context, [32]byte, CanonicalRecord) (inserted bool, existing []byte, err error)
}

type AdmissionFenceStore interface {
	AppendFence(context.Context, CanonicalRecord) error
	GetFence(context.Context, [32]byte) ([]byte, error)
}

// IngressAppendStore atomically appends a source record, its ingress receipt,
// and the source's gapless cursor.
type IngressAppendStore interface {
	AppendIngress(context.Context, string, uint64, CanonicalRecord, CanonicalRecord) error
	NextIngressSequence(context.Context, string) (uint64, error)
}

// AdmissionGuardedAppendStore atomically prevents source-set expansion and
// retroactively eligible rounds after a durable fence for the same run.
// Implementations must serialize these checks with AppendFenceIfPrefix.
type AdmissionGuardedAppendStore interface {
	AppendSourceDeclaration(context.Context, [32]byte, CanonicalRecord) error
	AppendRoundDeclaration(context.Context, [32]byte, int64, uint64, CanonicalRecord) error
}

// PrefixConditionalFenceStore publishes a fence only if the input-ledger prefix
// is exactly the one used for derivation. The comparison and insert must share
// one storage transaction, including across independently opened processes.
type PrefixConditionalFenceStore interface {
	AppendFenceIfPrefix(context.Context, CanonicalRecord, []CanonicalRecord) error
}

// Authorities deliberately exposes distinct owners instead of a global store.
type Authorities struct {
	InputLedger    AppendOnlyStore
	DecisionLedger AppendOnlyStore
	EventStore     AppendOnlyStore
	EvidenceLedger AppendOnlyStore
	CausalOutbox   AppendOnlyStore
	SnapshotStore  AppendOnlyStore
}

// ClockAdvanceBatch is the complete CLOCK_ADVANCE publication unit. The caller
// derives all records against one frozen revision. Only a CommitCoordinator may
// invoke ClockAdvanceStore; a storage adapter must compare and publish the whole
// unit in one transaction, never through individual append methods.
type TriggerStateChange struct {
 DefinitionID [32]byte
 DefinitionCanonical []byte
 Before []byte
 After []byte
}
type ClockAdvanceBatch struct {
 RunID [32]byte
 CycleID [32]byte
 FenceID [32]byte
 TargetInstant int64
 TargetOrdinal uint64
 ExpectedInputPrefix []CanonicalRecord
 ExpectedRevision uint64
 ResultRevision uint64
 ExpectedClock []byte
 ResultClock []byte
 BaseStateHash [32]byte
 ResultStateHash [32]byte
 Journal CanonicalRecord
 Events []CanonicalRecord
 PerceptionTasks []CanonicalRecord // PENDING task envelopes from Genesis-pinned event classification
 TriggerRuntime []TriggerStateChange
 Activations []CanonicalRecord
 RevisionCanonical []byte
 // Test-only crash injection inside the one transaction; any injected failure
 // must roll back all authorities and leave exactly the old prefix.
 FailAfterStep uint8
}
// TerminalAbortBatch and AttemptRetryBatch extend the existing SQLite
// transaction boundary; they do not establish a second ledger writer.
type TerminalAbortBatch struct {
 RunID [32]byte
 CycleID [32]byte
 FenceID [32]byte
 ExpectedClock []byte
 Envelope CanonicalRecord
 // Candidate subject roots, canonical and source-bound; committed together
 // with the abort journal if a per-candidate INDETERMINATE is emitted.
 CandidateEvidence []CanonicalRecord
 FailAfterStep uint8
}
type AttemptRetryBatch struct {
 RunID [32]byte
 CycleID [32]byte
 AbortID [32]byte
 ExpectedClock []byte
 Record CanonicalRecord
 FailAfterStep uint8
}
// WorkBatch is an indivisible WORK publication in the same authoritative
// SQLite transaction owner as ClockAdvanceBatch. The initial V1-5 slice only
// accepts a closed, zero-event NoProposal batch. The adapter must fail closed
// on resource-consuming or lifecycle-bearing variants until reducers exist.
// CommunicationPublication is a draft trio that may become durable ONLY
// inside the existing terminal CommitWork SQLite transaction. It does not
// grant an independent Claim/Transmission or scheduling append authority.
type CommunicationPublication struct {
 Claim CanonicalRecord
 Transmission CanonicalRecord
 Occurrence CanonicalRecord
}
type WorkBatch struct {
 // 0: NoProposal; 1: single resource; 2: two contenders;
 // 3: authenticated system-only cohort; 4: NoProposal slots and system
 // sources atomically combined in the same AdmissionFence/WorldRevision.
 Mode uint8
 // Mode 7: registered communication send, without world truth mutation.
 Communication *CommunicationPublication
 CommunicationBundle *policy.Bundle // verified against persisted Genesis before use
 CommunicationCandidate CanonicalRecord
 SystemCandidate CanonicalRecord
 // Remaining independent system-unit candidates in AdmissionFence order.
 SystemExtraCandidates []CanonicalRecord
 SystemSuccessors []CanonicalRecord
 // Mode 2 publishes its complete pure candidate/conflict/RNG assessment
 // evidence only after the same terminal SQLite CAS succeeds.
 ContestProofs []CanonicalRecord
 ResourceCandidate CanonicalRecord
 ResultStateHash [32]byte
 RunID [32]byte
 CycleID [32]byte
 FenceID [32]byte
 ExpectedClock []byte
 ResultClock []byte
 ExpectedInputPrefix []CanonicalRecord
 Journal CanonicalRecord
 Decisions []CanonicalRecord
 Events []CanonicalRecord // authenticated EventOrderKey order
 PerceptionTasks []CanonicalRecord // canonical PENDING entries, never observer outputs
 BaseStateHash [32]byte
 RevisionCanonical []byte
 FailAfterStep uint8
}
type ClockAdvanceStore interface {
 CommitWork(context.Context, WorkBatch) error
 CommitClockAdvance(context.Context, ClockAdvanceBatch) error
 CommitCycleAbort(context.Context, TerminalAbortBatch) error
 AuthorizeAttemptRetry(context.Context, AttemptRetryBatch) error
}

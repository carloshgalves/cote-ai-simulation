// Package ports defines the narrow persistence seams owned by the causal core.
package ports

import "context"

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
type ClockAdvanceBatch struct {
 RunID [32]byte
 CycleID [32]byte
 FenceID [32]byte
 ExpectedRevision uint64
 ResultRevision uint64
 ExpectedClock []byte
 ResultClock []byte
 BaseStateHash [32]byte
 ResultStateHash [32]byte
 Journal CanonicalRecord
 Events []CanonicalRecord
 TriggerRuntime []CanonicalRecord
 Activations []CanonicalRecord
 RevisionCanonical []byte
 // Test-only crash injection inside the one transaction; any injected failure
 // must roll back all authorities and leave exactly the old prefix.
 FailAfterStep uint8
}
type ClockAdvanceStore interface {
 CommitClockAdvance(context.Context, ClockAdvanceBatch) error
}

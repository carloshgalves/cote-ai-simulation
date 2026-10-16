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

// Authorities deliberately exposes distinct owners instead of a global store.
type Authorities struct {
	InputLedger    AppendOnlyStore
	DecisionLedger AppendOnlyStore
	EventStore     AppendOnlyStore
	EvidenceLedger AppendOnlyStore
	CausalOutbox   AppendOnlyStore
	SnapshotStore  AppendOnlyStore
}

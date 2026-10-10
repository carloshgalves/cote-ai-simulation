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

// Authorities deliberately exposes distinct owners instead of a global store.
type Authorities struct {
	InputLedger    AppendOnlyStore
	DecisionLedger AppendOnlyStore
	EventStore     AppendOnlyStore
	EvidenceLedger AppendOnlyStore
	CausalOutbox   AppendOnlyStore
	SnapshotStore  AppendOnlyStore
}

package observatory

import "context"

// AuthorityRef identifies a durable authority, not a capability to read it.
// ContentHash is SHA-256 of the stored canonical bytes (not the record's
// domain-separated normative digest). No canonical payload is exposed.
type AuthorityRef struct {
 Ledger string
 ID [32]byte
 ContentHash [32]byte
}

// CausalCommit is a privileged, verified, immutable summary of one terminal
// CycleCommit. It contains references and causal metadata, not actor context.
type CausalCommit struct {
 CycleID [32]byte
 Instant int64
 Ordinal uint64
 Attempt uint64
 Plan string
 BaseRevision uint64
 ResultRevision uint64
 BaseWorldHash [32]byte
 ResultWorldHash [32]byte
 InputDigest [32]byte
 DecisionDigest [32]byte
 Fence AuthorityRef
 Commit AuthorityRef
 Closures []AuthorityRef
 Admitted []AuthorityRef
 Decisions []AuthorityRef
 Events []AuthorityRef
 Tasks []AuthorityRef
}

// CausalAttempt preserves the durable fence/abort/retry chain even when the
// current coordinator state has moved on. A missing terminal is not an abort.
type CausalAttempt struct {
 CycleID [32]byte
 Instant int64
 Ordinal uint64
 Attempt uint64
 Fence AuthorityRef
 Closures []AuthorityRef
 Admitted []AuthorityRef
 Commit *AuthorityRef
 Abort *AuthorityRef
 Retry *AuthorityRef
 AbortReason *uint64
}

// PrivilegedReport is for the local human operator's diagnostics only.
// Never pass it as a KnowledgeInput, Observation or character prompt.
type PrivilegedReport struct {
 State RunState
 Commits []CausalCommit
 Attempts []CausalAttempt
 PendingUnits int
 PendingClosureSources []string
 PendingPerceptionTasks int
 // There is no persisted authoritative rejected-ingress ledger in CSF V1.
 // A rejection count must not be inferred from accepted input absence.
 RejectedIngress string
}

// PrivilegedQueries is deliberately distinct from the ordinary Queries
// interface. No human/actor authentication is added: the host is local and
// privileged distribution remains the responsibility of its single operator.
type PrivilegedQueries interface {
 ReadPrivilegedReport(context.Context, [32]byte) (PrivilegedReport,error)
}

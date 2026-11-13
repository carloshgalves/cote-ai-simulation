// Package observatory exposes immutable read projections. Actor POV is not
// world truth: it consists solely of delivered, recipient-scoped evidence.
package observatory

import "context"

// AddressedEvidence contains only a pair of canonical records that has
// passed the pre-existing context allow-list, including causal receipts and
// redaction checks. These bytes are delivered evidence, not EventStore bytes.
type AddressedEvidence struct {
	ObservationID        [32]byte
	KnowledgeInputID     [32]byte
	ObservationCanonical []byte
	KnowledgeCanonical   []byte
	ReceivedAt           int64
	Channel              string
}

// POVView is bound to a real actor/SlotDispatch capability at a sealed causal
// revision. It has no WorldState, decision journal, EventStore or third-party
// evidence; contents are not an unqualified statement of objective truth.
type POVView struct {
	Run             [32]byte
	Actor           [2]string
	ThroughRevision uint64
	Entries         []AddressedEvidence
}

// POVQueries never accepts a different actor at query time: the concrete
// reader is minted against a durable, actor-bound dispatch and its cutoff.
type POVQueries interface {
	ReadPOV(context.Context) (POVView, error)
}

// NoPublicRoute means the V1 CSF has not installed any authenticated public
// publication authority. Event.confidentiality=PUBLIC is not a publication.
const NoPublicRoute = "NO_REGISTERED_PUBLIC_ROUTE"

// PublicPublication deliberately cannot carry actor-private Observation or
// KnowledgeInput bodies. A future public source must prove a publication ID,
// authorized channel, publication time and public content digest separately.
type PublicPublication struct {
	PublicationID [32]byte
	Channel       string
	PublishedAt   int64
	ContentHash   [32]byte
}

// PublicView does not contain implicit public EventStore data. A later
// explicitly authorized public publication system must extend this contract
// together with its independent source and tests; until then it fails closed.
type PublicView struct {
	Run               [32]byte
	PublicationStatus string
	Publications      []PublicPublication
}

type PublicQueries interface {
	ReadPublic(context.Context, [32]byte) (PublicView, error)
}

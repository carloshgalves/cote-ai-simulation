package clock

// The base runtime's versioned same-instant cascade rule. This counts
// committed WORK cycles, not sparse CycleOrdinal values. A revision of
// these semantics must bump the runtime policy version and bundle pin.
const CascadePolicyVersionV1 uint64 = 1
const MaxSameInstantWorkV1 uint64 = 64

func CascadeExceeded(committedAtInstant uint64) bool {
 return committedAtInstant >= MaxSameInstantWorkV1
}

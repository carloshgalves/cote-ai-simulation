// Package reducer contains pure, versioned world-state transitions.
package reducer

import (
	"bytes"
	"errors"
	"math"
	"regexp"
	"sort"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
)

// PhysicalVariant is distinct from the synthetic testresource stock variant.
// The typed-value envelope is used only as a transport shape until the
// physical extension has been registered and pinned by Genesis. This reducer
// is pure: neither its inputs nor its outputs carry commit authority.
const (
	PhysicalVariant     = "physical.body-state.v1"
	PhysicalSchemaID    = "cote.csf.schema.typed-value"
	PhysicalDomainTag   = "cote.csf.hash.world-state"
	maxPhysicalSubjects = 4096
)

var ErrInvalidPhysicalState = errors.New("invalid physical WorldState")
var physicalSubjectID = regexp.MustCompile(`^[a-z][a-z0-9]*(?:[.-][a-z0-9]+)*$`)

type PhysicalSleepEpisode struct {
	Start, End float64
	Quality    string
}

type PhysicalSoreness struct {
	Latency   []float64
	Expressed float64
}

// PhysicalBody is the currently implemented PSV1-2 BodyState. Fields are
// deliberately closed: injury/illness episodes, pending soreness onset,
// repeated-bout adaptation and non-default interval loads require a new
// physical schema revision, not permissive map-based decoding.
type PhysicalBody struct {
	Subject                                             string
	Hours                                               float64
	WPrimeRemaining, WPrimeCapacity, WPrimeTau          float64
	Peripheral                                          [4]float64 // legs, arms, grip, core
	CentralFatigue                                      float64
	AcuteLoad, ChronicLoad                              float64
	SleepDebt, HoursAwake, Circadian                    float64
	LastSleep                                           *PhysicalSleepEpisode
	Substrate, EnergyBalance                            float64
	LastMeal                                            *float64
	HydrationDeficit                                    float64
	CoreOffset, WBGT, WorkRestRatio, ClothingInsulation float64
	Soreness                                            [4]PhysicalSoreness
	Pain                                                [4]float64 // legs, arms, grip, core
	GlobalPain, Analgesia                               float64
}

func physicalFinite(n float64) bool {
	return !math.IsNaN(n) && !math.IsInf(n, 0)
}
func physicalRange(n, low, high float64) bool {
	return physicalFinite(n) && n >= low && n <= high
}
func (b PhysicalBody) validate() error {
	if len(b.Subject) > 128 || !physicalSubjectID.MatchString(b.Subject) ||
		!physicalRange(b.Hours, 0, 99999) ||
		!physicalRange(b.WPrimeRemaining, 0, math.MaxFloat64) ||
		!physicalRange(b.WPrimeCapacity, math.SmallestNonzeroFloat64, math.MaxFloat64) ||
		!physicalRange(b.WPrimeTau, math.SmallestNonzeroFloat64, math.MaxFloat64) ||
		b.WPrimeRemaining > b.WPrimeCapacity ||
		!physicalRange(b.CentralFatigue, 0, 1) ||
		!physicalRange(b.AcuteLoad, 0, math.MaxFloat64) ||
		!physicalRange(b.ChronicLoad, 0, math.MaxFloat64) ||
		!physicalRange(b.SleepDebt, 0, math.MaxFloat64) ||
		!physicalRange(b.HoursAwake, 0, math.MaxFloat64) ||
		!physicalRange(b.Circadian, 0, 24) || b.Circadian == 24 ||
		!physicalRange(b.Substrate, 0, 1) ||
		!physicalFinite(b.EnergyBalance) ||
		!physicalRange(b.HydrationDeficit, 0, math.MaxFloat64) ||
		!physicalRange(b.CoreOffset, 0, math.MaxFloat64) ||
		!physicalRange(b.WBGT, -100, 100) ||
		!physicalRange(b.WorkRestRatio, math.SmallestNonzeroFloat64, math.MaxFloat64) ||
		!physicalRange(b.ClothingInsulation, 0, math.MaxFloat64) ||
		!physicalRange(b.GlobalPain, 0, 1) ||
		!physicalRange(b.Analgesia, 0, 1) {
		return ErrInvalidPhysicalState
	}
	if b.LastSleep != nil {
		s := b.LastSleep
		if !physicalRange(s.Start, 0, math.MaxFloat64) || !physicalRange(s.End, s.Start, math.MaxFloat64) ||
			(s.Quality != "good" && s.Quality != "fair" && s.Quality != "poor") {
			return ErrInvalidPhysicalState
		}
	}
	if b.LastMeal != nil && !physicalRange(*b.LastMeal, 0, math.MaxFloat64) {
		return ErrInvalidPhysicalState
	}
	for i := 0; i < 4; i++ {
		if !physicalRange(b.Peripheral[i], 0, 1) ||
			!physicalRange(b.Pain[i], 0, 1) ||
			!physicalRange(b.Soreness[i].Expressed, 0, 1) ||
			len(b.Soreness[i].Latency) == 0 || len(b.Soreness[i].Latency) > 100 {
			return ErrInvalidPhysicalState
		}
		for _, v := range b.Soreness[i].Latency {
			if !physicalRange(v, 0, math.MaxFloat64) {
				return ErrInvalidPhysicalState
			}
		}
	}
	return nil
}

func physicalFloats(v []float64) []any {
	out := make([]any, len(v))
	for i, n := range v {
		out[i] = n
	}
	return out
}
func physicalBodyPayload(b PhysicalBody) []any {
	var sleep, meal any
	if b.LastSleep != nil {
		sleep = []any{b.LastSleep.Start, b.LastSleep.End, b.LastSleep.Quality}
	}
	if b.LastMeal != nil {
		meal = *b.LastMeal
	}
	soreness := make([]any, 4)
	for i, s := range b.Soreness {
		soreness[i] = []any{physicalFloats(s.Latency), []any{}, s.Expressed}
	}
	return []any{
		b.Subject, b.Hours,
		[]any{b.WPrimeRemaining, b.WPrimeCapacity, b.WPrimeTau},
		physicalFloats(b.Peripheral[:]), b.CentralFatigue,
		[]any{b.AcuteLoad, b.ChronicLoad},
		[]any{b.SleepDebt, b.HoursAwake, b.Circadian, sleep},
		[]any{b.Substrate, b.EnergyBalance, meal},
		b.HydrationDeficit,
		[]any{b.CoreOffset, b.WBGT, b.WorkRestRatio, b.ClothingInsulation},
		[]any{soreness, []any{}},
		[]any{}, []any{},
		[]any{physicalFloats(b.Pain[:]), b.GlobalPain, b.Analgesia},
	}
}

func physicalArray(v any, size int) ([]any, bool) {
	a, ok := v.([]any)
	return a, ok && len(a) == size
}
func physicalNumber(v any) (float64, bool) {
	n, ok := v.(float64)
	return n, ok && physicalFinite(n)
}
func decodePhysicalBody(v any) (PhysicalBody, error) {
	var b PhysicalBody
	a, ok := physicalArray(v, 14)
	if !ok {
		return b, ErrInvalidPhysicalState
	}
	b.Subject, ok = a[0].(string)
	if !ok {
		return b, ErrInvalidPhysicalState
	}
	read := func(v any, dest *float64) bool {
		var valid bool
		*dest, valid = physicalNumber(v)
		return valid
	}
	if !read(a[1], &b.Hours) {
		return b, ErrInvalidPhysicalState
	}
	w, ok := physicalArray(a[2], 3)
	if !ok || !read(w[0], &b.WPrimeRemaining) || !read(w[1], &b.WPrimeCapacity) || !read(w[2], &b.WPrimeTau) {
		return b, ErrInvalidPhysicalState
	}
	pf, ok := physicalArray(a[3], 4)
	if !ok {
		return b, ErrInvalidPhysicalState
	}
	for i, n := range pf {
		if !read(n, &b.Peripheral[i]) {
			return b, ErrInvalidPhysicalState
		}
	}
	if !read(a[4], &b.CentralFatigue) {
		return b, ErrInvalidPhysicalState
	}
	load, ok := physicalArray(a[5], 2)
	if !ok || !read(load[0], &b.AcuteLoad) || !read(load[1], &b.ChronicLoad) {
		return b, ErrInvalidPhysicalState
	}
	sleep, ok := physicalArray(a[6], 4)
	if !ok || !read(sleep[0], &b.SleepDebt) || !read(sleep[1], &b.HoursAwake) || !read(sleep[2], &b.Circadian) {
		return b, ErrInvalidPhysicalState
	}
	if sleep[3] != nil {
		episode, ok := physicalArray(sleep[3], 3)
		if !ok {
			return b, ErrInvalidPhysicalState
		}
		s := &PhysicalSleepEpisode{}
		if !read(episode[0], &s.Start) || !read(episode[1], &s.End) {
			return b, ErrInvalidPhysicalState
		}
		s.Quality, ok = episode[2].(string)
		if !ok {
			return b, ErrInvalidPhysicalState
		}
		b.LastSleep = s
	}
	energy, ok := physicalArray(a[7], 3)
	if !ok || !read(energy[0], &b.Substrate) || !read(energy[1], &b.EnergyBalance) {
		return b, ErrInvalidPhysicalState
	}
	if energy[2] != nil {
		last, valid := physicalNumber(energy[2])
		if !valid {
			return b, ErrInvalidPhysicalState
		}
		b.LastMeal = &last
	}
	if !read(a[8], &b.HydrationDeficit) {
		return b, ErrInvalidPhysicalState
	}
	thermal, ok := physicalArray(a[9], 4)
	if !ok || !read(thermal[0], &b.CoreOffset) || !read(thermal[1], &b.WBGT) ||
		!read(thermal[2], &b.WorkRestRatio) || !read(thermal[3], &b.ClothingInsulation) {
		return b, ErrInvalidPhysicalState
	}
	sore, ok := physicalArray(a[10], 2)
	if !ok {
		return b, ErrInvalidPhysicalState
	}
	rows, ok := physicalArray(sore[0], 4)
	if !ok {
		return b, ErrInvalidPhysicalState
	}
	repeats, ok := physicalArray(sore[1], 0)
	if !ok || len(repeats) != 0 {
		return b, ErrInvalidPhysicalState
	}
	for i, row := range rows {
		item, valid := physicalArray(row, 3)
		if !valid {
			return b, ErrInvalidPhysicalState
		}
		latency, valid := item[0].([]any)
		if !valid || len(latency) == 0 || len(latency) > 100 {
			return b, ErrInvalidPhysicalState
		}
		b.Soreness[i].Latency = make([]float64, len(latency))
		for j, v := range latency {
			if !read(v, &b.Soreness[i].Latency[j]) {
				return b, ErrInvalidPhysicalState
			}
		}
		pending, valid := physicalArray(item[1], 0)
		if !valid || len(pending) != 0 || !read(item[2], &b.Soreness[i].Expressed) {
			return b, ErrInvalidPhysicalState
		}
	}
	for _, index := range []int{11, 12} {
		if _, ok := physicalArray(a[index], 0); !ok {
			return b, ErrInvalidPhysicalState
		}
	}
	pain, ok := physicalArray(a[13], 3)
	if !ok {
		return b, ErrInvalidPhysicalState
	}
	regions, ok := physicalArray(pain[0], 4)
	if !ok || !read(pain[1], &b.GlobalPain) || !read(pain[2], &b.Analgesia) {
		return b, ErrInvalidPhysicalState
	}
	for i, v := range regions {
		if !read(v, &b.Pain[i]) {
			return b, ErrInvalidPhysicalState
		}
	}
	return b, b.validate()
}

func validatePhysicalBodies(rows []any) ([]PhysicalBody, error) {
	if len(rows) == 0 || len(rows) > maxPhysicalSubjects {
		return nil, ErrInvalidPhysicalState
	}
	result := make([]PhysicalBody, len(rows))
	previous := ""
	for i, raw := range rows {
		b, err := decodePhysicalBody(raw)
		if err != nil || previous != "" && b.Subject <= previous {
			return nil, ErrInvalidPhysicalState
		}
		previous, result[i] = b.Subject, b
	}
	return result, nil
}

// ValidatePhysicalPayload is the closed, full-field validator for a
// versioned physical typed-value payload, including private numeric domains.
func ValidatePhysicalPayload(value any) error {
	fields, ok := physicalArray(value, 3)
	if !ok || fields[0] != PhysicalVariant || fields[1] != uint64(1) {
		return ErrInvalidPhysicalState
	}
	rows, ok := fields[2].([]any)
	if !ok {
		return ErrInvalidPhysicalState
	}
	_, err := validatePhysicalBodies(rows)
	return err
}

// EncodePhysical only computes a canonical typed-state value. It neither
// changes a revision nor verifies source authority or the policy bundle.
func EncodePhysical(bodies []PhysicalBody) ([]any, domain.Digest, error) {
	var zero domain.Digest
	if len(bodies) == 0 || len(bodies) > maxPhysicalSubjects {
		return nil, zero, ErrInvalidPhysicalState
	}
	copyOf := append([]PhysicalBody(nil), bodies...)
	sort.Slice(copyOf, func(i, j int) bool { return copyOf[i].Subject < copyOf[j].Subject })
	rows := make([]any, len(copyOf))
	for i, b := range copyOf {
		if err := b.validate(); err != nil {
			return nil, zero, err
		}
		if i > 0 && b.Subject == copyOf[i-1].Subject {
			return nil, zero, ErrInvalidPhysicalState
		}
		rows[i] = physicalBodyPayload(b)
	}
	raw, err := codec.CanonicalBytes(PhysicalDomainTag, PhysicalSchemaID, 1, []any{PhysicalVariant, uint64(1), rows})
	if err != nil {
		return nil, zero, err
	}
	digest := identity.OpaqueContentHash(raw)
	return []any{PhysicalSchemaID, uint64(1), raw, digest[:]}, digest, nil
}

func DecodePhysical(value any) ([]PhysicalBody, error) {
	fields, ok := physicalArray(value, 4)
	if !ok || fields[0] != PhysicalSchemaID || fields[1] != uint64(1) {
		return nil, ErrInvalidPhysicalState
	}
	raw, ok := fields[2].([]byte)
	if !ok {
		return nil, ErrInvalidPhysicalState
	}
	want, ok := fields[3].([]byte)
	if !ok || len(want) != 32 {
		return nil, ErrInvalidPhysicalState
	}
	digest := identity.OpaqueContentHash(raw)
	if !bytes.Equal(digest[:], want) {
		return nil, ErrInvalidPhysicalState
	}
	envelope, err := codec.StrictDecodeEnvelope(raw)
	if err != nil || envelope.DomainTag != PhysicalDomainTag ||
		envelope.SchemaID != PhysicalSchemaID || envelope.SchemaVersion != 1 {
		return nil, ErrInvalidPhysicalState
	}
	if payload, ok := envelope.Value.([]any); ok && len(payload) > 0 && payload[0] == CompositeVariant {
		_, bodies, err := DecodeComposite(value)
		return bodies, err
	}
	if ValidatePhysicalPayload(envelope.Value) != nil {
		return nil, ErrInvalidPhysicalState
	}
	rows := envelope.Value.([]any)[2].([]any)
	return validatePhysicalBodies(rows)
}

// ApplyCompletedPhysical is a pure full-state CAS reducer: each subject must
// already exist, must advance monotonically and is replaced in the canonical
// body vector exactly once. Callers MUST separately establish an authorized
// source, completion attestation, Genesis registration and AdmissionFence.
// This function is not a means to publish or admit state.
func ApplyCompletedPhysical(before []any, completed []PhysicalBody) ([]any, domain.Digest, error) {
	var zero domain.Digest
	old, err := DecodePhysical(before)
	if err != nil || len(completed) == 0 || len(completed) > len(old) {
		return nil, zero, ErrInvalidPhysicalState
	}
	updates := make(map[string]PhysicalBody, len(completed))
	for _, b := range completed {
		if err := b.validate(); err != nil {
			return nil, zero, err
		}
		if _, exists := updates[b.Subject]; exists {
			return nil, zero, ErrInvalidPhysicalState
		}
		updates[b.Subject] = b
	}
	next := make([]PhysicalBody, len(old))
	for i, prior := range old {
		replacement, exists := updates[prior.Subject]
		if exists {
			if replacement.Hours <= prior.Hours {
				return nil, zero, ErrInvalidPhysicalState
			}
			next[i] = replacement
			delete(updates, prior.Subject)
		} else {
			next[i] = prior
		}
	}
	if len(updates) != 0 {
		return nil, zero, ErrInvalidPhysicalState
	}
	if isCompositeWorldState(before) {
		stock, _, err := DecodeComposite(before)
		if err != nil {
			return nil, zero, err
		}
		return EncodeComposite(stock, next)
	}
	return EncodePhysical(next)
}

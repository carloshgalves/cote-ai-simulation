package reducer

import (
	"bytes"
	"errors"
	"math"
	"reflect"
	"testing"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
)

func physicalTestBody(id string, hours float64) PhysicalBody {
	b := PhysicalBody{
		Subject: id, Hours: hours, WPrimeRemaining: 1200,
		WPrimeCapacity: 2000, WPrimeTau: 300, Peripheral: [4]float64{0.1, 0.2, 0.1, 0.2},
		CentralFatigue: 0.3, AcuteLoad: 1, ChronicLoad: 1.2,
		SleepDebt: 2, HoursAwake: 11, Circadian: 18,
		Substrate: 0.8, EnergyBalance: -250, HydrationDeficit: 0.01,
		CoreOffset: 0.1, WBGT: 25, WorkRestRatio: 1, ClothingInsulation: 1,
		Pain: [4]float64{0, 0, 0, 0}, GlobalPain: 0, Analgesia: 0,
	}
	for i := range b.Soreness {
		b.Soreness[i] = PhysicalSoreness{Latency: []float64{0, 0, 0}, Expressed: 0.1}
	}
	return b
}

func TestPhysicalStateRoundTripAndDeterministicCAS(t *testing.T) {
	a := physicalTestBody("npc.0001", 24)
	b := physicalTestBody("npc.0002", 12)
	typed, hash, err := EncodePhysical([]PhysicalBody{b, a})
	if err != nil {
		t.Fatal(err)
	}
	reordered, other, err := EncodePhysical([]PhysicalBody{a, b})
	if err != nil {
		t.Fatal(err)
	}
	if hash != other || !reflect.DeepEqual(typed, reordered) {
		t.Fatal("subject input permutation changed canonical world")
	}
	decoded, err := DecodePhysical(typed)
	if err != nil || len(decoded) != 2 || !reflect.DeepEqual(decoded[0], a) {
		t.Fatalf("decode failed: %#v %v", decoded, err)
	}
	a.Hours = 48
	a.SleepDebt = 1
	next, nextHash, err := ApplyCompletedPhysical(typed, []PhysicalBody{a})
	if err != nil || nextHash == hash {
		t.Fatalf("completed physical reduction failed: %v", err)
	}
	after, err := DecodePhysical(next)
	if err != nil || len(after) != 2 || after[0].Hours != 48 || after[1].Hours != 12 {
		t.Fatalf("bad successor: %#v %v", after, err)
	}
	repeat, repeatHash, err := ApplyCompletedPhysical(typed, []PhysicalBody{a})
	if err != nil || nextHash != repeatHash || !reflect.DeepEqual(next, repeat) {
		t.Fatal("deterministic replay mismatch")
	}
	again, _, err := ApplyCompletedPhysical(next, []PhysicalBody{a})
	if !errors.Is(err, ErrInvalidPhysicalState) {
		t.Fatal("stale re-application advanced world")
	}
}

func TestPhysicalStateRejectsMalformedSemanticsAndAuthoredAliases(t *testing.T) {
	b := physicalTestBody("npc.0001", 12)
	cases := map[string]func(*PhysicalBody){
		"bad subject":      func(v *PhysicalBody) { v.Subject = "NPC/../../etc" },
		"nan":              func(v *PhysicalBody) { v.CentralFatigue = math.NaN() },
		"invalid capacity": func(v *PhysicalBody) { v.WPrimeRemaining = v.WPrimeCapacity + 1 },
		"negative time":    func(v *PhysicalBody) { v.Hours = -1 },
		"sleep phase":      func(v *PhysicalBody) { v.Circadian = 24 },
		"energy domain":    func(v *PhysicalBody) { v.Substrate = 1.01 },
		"absent latency":   func(v *PhysicalBody) { v.Soreness[2].Latency = nil },
		"pain domain":      func(v *PhysicalBody) { v.Pain[0] = -0.1 },
		"thermal":          func(v *PhysicalBody) { v.WBGT = math.Inf(1) },
		"injury schema":    func(v *PhysicalBody) { v.Soreness[0].Expressed = 3 },
	}
	for name, modify := range cases {
		t.Run(name, func(t *testing.T) {
			v := b
			modify(&v)
			if _, _, err := EncodePhysical([]PhysicalBody{v}); !errors.Is(err, ErrInvalidPhysicalState) {
				t.Fatalf("invalid domain accepted: %v", err)
			}
		})
	}
	if _, _, err := EncodePhysical([]PhysicalBody{b, b}); !errors.Is(err, ErrInvalidPhysicalState) {
		t.Fatal("duplicate subject admitted")
	}
	typed, _, err := EncodePhysical([]PhysicalBody{b})
	if err != nil {
		t.Fatal(err)
	}
	unknown := physicalTestBody("npc.0002", 30)
	if _, _, err := ApplyCompletedPhysical(typed, []PhysicalBody{unknown}); !errors.Is(err, ErrInvalidPhysicalState) {
		t.Fatal("unknown subject inserted")
	}
	older := physicalTestBody("npc.0001", 11)
	if _, _, err := ApplyCompletedPhysical(typed, []PhysicalBody{older}); !errors.Is(err, ErrInvalidPhysicalState) {
		t.Fatal("time rewound")
	}
	if _, _, err := ApplyCompletedPhysical(typed, []PhysicalBody{b, b}); !errors.Is(err, ErrInvalidPhysicalState) {
		t.Fatal("duplicate update accepted")
	}
}

func TestPhysicalStateRejectsTamperingWithoutDigestAuthority(t *testing.T) {
	typed, _, err := EncodePhysical([]PhysicalBody{physicalTestBody("npc.0001", 1)})
	if err != nil {
		t.Fatal(err)
	}
	raw := append([]byte(nil), typed[2].([]byte)...)
	raw[len(raw)-1] ^= 1
	damaged := append([]any(nil), typed...)
	damaged[2] = raw
	if _, err := DecodePhysical(damaged); !errors.Is(err, ErrInvalidPhysicalState) {
		t.Fatal("tampered canonical payload admitted")
	}
	damaged = append([]any(nil), typed...)
	damaged[3] = bytes.Repeat([]byte{0}, 32)
	if _, err := DecodePhysical(damaged); !errors.Is(err, ErrInvalidPhysicalState) {
		t.Fatal("forged digest accepted")
	}
	envelope, err := codec.StrictDecodeEnvelope(typed[2].([]byte))
	if err != nil {
		t.Fatal(err)
	}
	fields := envelope.Value.([]any)
	rows := fields[2].([]any)
	body := append([]any(nil), rows[0].([]any)...)
	body[11] = []any{[]any{"unregistered-injury"}}
	if err := ValidatePhysicalPayload([]any{PhysicalVariant, uint64(1), []any{body}}); !errors.Is(err, ErrInvalidPhysicalState) {
		t.Fatal("unregistered injury accepted")
	}
	body = append([]any(nil), rows[0].([]any)...)
	body[1] = uint64(1)
	if err := ValidatePhysicalPayload([]any{PhysicalVariant, uint64(1), []any{body}}); !errors.Is(err, ErrInvalidPhysicalState) {
		t.Fatal("numeric type substitution accepted")
	}
	if err := ValidatePhysicalPayload([]any{"testresource.stock.v1", uint64(1), rows}); !errors.Is(err, ErrInvalidPhysicalState) {
		t.Fatal("testresource alias accepted")
	}
}

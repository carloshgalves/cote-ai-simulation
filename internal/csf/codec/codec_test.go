package codec

import (
	"encoding/hex"
	"math"
	"testing"
)

func TestCanonicalEnvelopeAnchor(t *testing.T) {
	got, err := CanonicalBytes("cote.csf.test.empty", "cote.csf.test.empty", 1, []any{})
	if err != nil {
		t.Fatal(err)
	}
	want, _ := hex.DecodeString("8644435346000173636f74652e6373662e746573742e656d70747973636f74652e6373662e746573742e656d7074790180")
	if hex.EncodeToString(got) != hex.EncodeToString(want) {
		t.Fatalf("canonical envelope mismatch\nwant %x\n got %x", want, got)
	}
}

func TestCanonicalFloatRules(t *testing.T) {
	positive, err := CanonicalPayload(math.Copysign(0, -1))
	if err != nil {
		t.Fatal(err)
	}
	if hex.EncodeToString(positive) != "f90000" {
		t.Fatalf("negative zero was not normalized: %x", positive)
	}
	for _, value := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		if _, err := CanonicalPayload(value); err == nil {
			t.Fatalf("accepted non-finite value %v", value)
		}
	}
}

func TestStrictDecodeRejectsNonCanonicalBytes(t *testing.T) {
	if _, err := StrictDecodePayload([]byte{0x18, 0x00}); err == nil {
		t.Fatal("accepted non-preferred integer encoding")
	}
}

func TestUnicode151UnassignedCodePointIsRejected(t *testing.T) {
	if _, err := CanonicalPayload("\u0378"); err == nil {
		t.Fatal("accepted a Unicode 15.1 General_Category=Cn code point")
	}
}

package domain

import "testing"

func TestEligibilityCoordinateIsLexicographicallyOrdered(t *testing.T) {
	base := EligibilityCoordinate{NotBeforeInstant: 10, NotBeforeCycleOrdinal: 2}
	if base.Compare(EligibilityCoordinate{NotBeforeInstant: 10, NotBeforeCycleOrdinal: 3}) >= 0 {
		t.Fatal("cycle ordinal must break ties at the same instant")
	}
	if base.Compare(EligibilityCoordinate{NotBeforeInstant: 9, NotBeforeCycleOrdinal: 99}) <= 0 {
		t.Fatal("instant must be compared before cycle ordinal")
	}
}

func TestCausalTextGrammars(t *testing.T) {
	for _, valid := range []string{"source-1", "actor.school:1", "a/b"} {
		if err := ValidateMachineID(valid); err != nil {
			t.Fatalf("valid machine id %q: %v", valid, err)
		}
	}
	for _, invalid := range []string{"", "Upper", "a--b", "é"} {
		if ValidateMachineID(invalid) == nil {
			t.Fatalf("accepted invalid machine id %q", invalid)
		}
	}
	if err := ValidateDomainTag("cote.csf.id.event"); err != nil {
		t.Fatal(err)
	}
	if ValidateDomainTag("event") == nil {
		t.Fatal("domain tag requires at least two dot-separated segments")
	}
	if err := ValidateTimezone("Asia/Tokyo"); err != nil {
		t.Fatal(err)
	}
}

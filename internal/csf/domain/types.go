// Package domain defines causal value objects without depending on adapters.
package domain

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
)

type (
	SimulationInstant int64
	LogicalSequence   uint64
	WorldRevision     uint64
	CycleOrdinal      uint64
	SchemaVersion     uint32
	Digest            [32]byte
	ID                [32]byte
	MachineID         string
	DomainTag         string
	SchemaID          string
)

type EligibilityCoordinate struct {
	NotBeforeInstant      SimulationInstant
	NotBeforeCycleOrdinal CycleOrdinal
}

func (c EligibilityCoordinate) Compare(other EligibilityCoordinate) int {
	if c.NotBeforeInstant < other.NotBeforeInstant {
		return -1
	}
	if c.NotBeforeInstant > other.NotBeforeInstant {
		return 1
	}
	if c.NotBeforeCycleOrdinal < other.NotBeforeCycleOrdinal {
		return -1
	}
	if c.NotBeforeCycleOrdinal > other.NotBeforeCycleOrdinal {
		return 1
	}
	return 0
}

type PolicyRef struct {
	ID      MachineID
	Version uint32
	Hash    Digest
}

type SchemaRef struct {
	ID      SchemaID
	Version SchemaVersion
}

type CausalRef struct {
	Kind   uint16
	ID     ID
	Digest Digest
}

var (
	machineIDPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:[._:/-][a-z0-9]+)*$`)
	tagPattern       = regexp.MustCompile(`^[a-z][a-z0-9]*(?:-[a-z0-9]+)*(?:\.[a-z][a-z0-9]*(?:-[a-z0-9]+)*)+$`)
	timezonePattern  = regexp.MustCompile(`^[A-Za-z]+(?:[_-][A-Za-z]+)*(?:/[A-Za-z]+(?:[_-][A-Za-z]+)*)+$`)
)

func ValidateMachineID(value string) error {
	if len(value) < 1 || len(value) > 128 || !machineIDPattern.MatchString(value) {
		return fmt.Errorf("invalid machine-id %q", value)
	}
	return nil
}

func ValidateDomainTag(value string) error {
	if len(value) > 128 || !tagPattern.MatchString(value) {
		return fmt.Errorf("invalid domain/schema tag %q", value)
	}
	return nil
}

func ValidateTimezone(value string) error {
	if !timezonePattern.MatchString(value) {
		return fmt.Errorf("invalid IANA timezone %q", value)
	}
	return nil
}

func NewDigest(value []byte) (Digest, error) {
	var result Digest
	if len(value) != len(result) {
		return result, fmt.Errorf("digest must contain 32 bytes, got %d", len(value))
	}
	copy(result[:], value)
	return result, nil
}

func NewID(value []byte) (ID, error) {
	var result ID
	if len(value) != len(result) {
		return result, fmt.Errorf("id must contain 32 bytes, got %d", len(value))
	}
	copy(result[:], value)
	return result, nil
}

var ErrIdentityCollision = errors.New("causal identity collision")

// EqualPreimage fails closed when an identity is reused for different bytes.
func EqualPreimage(existing, candidate []byte) error {
	if !bytes.Equal(existing, candidate) {
		return ErrIdentityCollision
	}
	return nil
}

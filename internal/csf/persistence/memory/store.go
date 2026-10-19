package memory

import (
	"bytes"
	"context"
	"errors"
	"sync"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
)

var ErrNotFound = errors.New("canonical record not found")

type Store struct {
	mu      sync.RWMutex
	records map[[32]byte][]byte
	order   [][32]byte
}

func New() *Store { return &Store{records: make(map[[32]byte][]byte)} }

func (s *Store) Append(_ context.Context, record ports.CanonicalRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.records[record.Identity]; ok {
		return domain.EqualPreimage(existing, record.Canonical)
	}
	s.records[record.Identity] = bytes.Clone(record.Canonical)
	s.order = append(s.order, record.Identity)
	return nil
}

func (s *Store) Get(_ context.Context, identity [32]byte) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	record, ok := s.records[identity]
	if !ok {
		return nil, ErrNotFound
	}
	return bytes.Clone(record), nil
}

func (s *Store) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.order)
}

func NewAuthorities() ports.Authorities {
	return ports.Authorities{
		InputLedger: New(), DecisionLedger: New(), EventStore: New(),
		EvidenceLedger: New(), CausalOutbox: New(), SnapshotStore: New(),
	}
}

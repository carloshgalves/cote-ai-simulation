package memory

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"sync"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
)

var ErrNotFound = errors.New("canonical record not found")

type Store struct {
	mu            sync.RWMutex
	records       map[[32]byte][]byte
	conditional   map[[32]byte]ports.CanonicalRecord
	ingressCursor map[string]uint64
	order         [][32]byte
}

func New() *Store {
	return &Store{records: make(map[[32]byte][]byte), conditional: make(map[[32]byte]ports.CanonicalRecord), ingressCursor: make(map[string]uint64)}
}

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

func (s *Store) AppendIngress(_ context.Context, source string, sequence uint64, record, receipt ports.CanonicalRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sequence != s.ingressCursor[source] {
		return errors.New("source ingress sequence is not gapless")
	}
	receiptID := sha256.Sum256(receipt.Canonical)
	if existing, ok := s.records[record.Identity]; ok {
		if err := domain.EqualPreimage(existing, record.Canonical); err != nil {
			return err
		}
	}
	if existing, ok := s.records[receiptID]; ok {
		if err := domain.EqualPreimage(existing, receipt.Canonical); err != nil {
			return err
		}
	}
	if _, ok := s.records[record.Identity]; !ok {
		s.records[record.Identity] = bytes.Clone(record.Canonical)
		s.order = append(s.order, record.Identity)
	}
	if _, ok := s.records[receiptID]; !ok {
		s.records[receiptID] = bytes.Clone(receipt.Canonical)
		s.order = append(s.order, receiptID)
	}
	s.ingressCursor[source] = sequence + 1
	return nil
}

func (s *Store) NextIngressSequence(_ context.Context, source string) (uint64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ingressCursor[source], nil
}

func (s *Store) AppendFence(ctx context.Context, record ports.CanonicalRecord) error {
	return s.Append(ctx, record)
}
func (s *Store) GetFence(ctx context.Context, id [32]byte) ([]byte, error) { return s.Get(ctx, id) }

func (s *Store) AppendIfAbsent(_ context.Context, key [32]byte, record ports.CanonicalRecord) (bool, []byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.conditional[key]; ok {
		return false, bytes.Clone(existing.Canonical), nil
	}
	if existing, ok := s.records[record.Identity]; ok {
		if err := domain.EqualPreimage(existing, record.Canonical); err != nil {
			return false, nil, err
		}
	} else {
		s.records[record.Identity] = bytes.Clone(record.Canonical)
		s.order = append(s.order, record.Identity)
	}
	s.conditional[key] = ports.CanonicalRecord{Identity: record.Identity, Canonical: bytes.Clone(record.Canonical)}
	return true, nil, nil
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

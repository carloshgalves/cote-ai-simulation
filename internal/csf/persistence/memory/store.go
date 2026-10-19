package memory

import (
	"bytes"
	"context"
	"errors"
	"sync"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
)

var ErrNotFound = errors.New("canonical record not found")

type Store struct {
	mu            sync.RWMutex
	records       map[[32]byte][]byte
	conditional   map[[32]byte]ports.CanonicalRecord
	ingressCursor map[string]uint64
	fences        map[[32]byte][]byte
	seals         []admissionSeal
	order         [][32]byte
}

type admissionSeal struct { runID [32]byte; instant int64; ordinal uint64 }

func New() *Store {
	return &Store{records: make(map[[32]byte][]byte), conditional: make(map[[32]byte]ports.CanonicalRecord), ingressCursor: make(map[string]uint64), fences: make(map[[32]byte][]byte)}
}

func (s *Store) Append(ctx context.Context, record ports.CanonicalRecord) error {
	// Prevent direct generic writes from bypassing source/round guards.
	kind, run, instant, ordinal, err := guardedKind(record)
	if err != nil { return err }
	switch kind {
	case "source": return s.AppendSourceDeclaration(ctx,run,record)
	case "round": return s.AppendRoundDeclaration(ctx,run,instant,ordinal,record)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.appendInputLocked(record)
}

func (s *Store) appendInputLocked(record ports.CanonicalRecord) error {
	if existing,ok:=s.records[record.Identity];ok {return domain.EqualPreimage(existing,record.Canonical)}
	s.records[record.Identity]=bytes.Clone(record.Canonical)
	s.order=append(s.order,record.Identity)
	return nil
}

// CBOR positive integers decode as uint64 even when the encoder input was int64.
// Preserve negative instants and reject values overflowing int64.
func decodedInstant(value any) (int64,bool) {
	switch n:=value.(type) {
	case int64: return n,true
	case uint64: if n<=uint64(^uint64(0)>>1) {return int64(n),true}
	}
	return 0,false
}

func guardedKind(record ports.CanonicalRecord) (string,[32]byte,int64,uint64,error) {
	var run [32]byte
	payload,err:=codec.StrictDecodePayload(record.Canonical)
	if err!=nil {return "",run,0,0,nil}
	fields,ok:=payload.([]any)
	if !ok {return "",run,0,0,nil}
	source:=len(fields)==3 && fields[0]=="source-declaration-v1"
	isRound:=len(fields)==11 && isString(fields[2])
	if !source && !isRound {return "",run,0,0,nil}
	id,ok:=fields[1].([]byte)
	if !ok||len(id)!=32{return "",run,0,0,errors.New("guarded append invalid run")}
	copy(run[:],id)
	if source{return "source",run,0,0,nil}
	coord,ok:=fields[5].([]any)
	if !ok||len(coord)!=2{return "",run,0,0,errors.New("guarded round invalid coordinate")}
	instant,ok:=decodedInstant(coord[0])
	if !ok{return "",run,0,0,errors.New("guarded round invalid instant")}
	ordinal,ok:=coord[1].(uint64)
	if !ok{return "",run,0,0,errors.New("guarded round invalid ordinal")}
	return "round",run,instant,ordinal,nil
}
func isString(v any) bool {_,ok:=v.(string);return ok}

// The admission seal is selected from the record's canonical run/coordinate;
// caller routing arguments cannot redirect it to another run or cycle.
func validateGuardArguments(record ports.CanonicalRecord, wantedKind string, runID [32]byte, instant int64, ordinal uint64) error {
	kind, actualRun, actualInstant, actualOrdinal, err := guardedKind(record)
	if err != nil { return err }
	if kind != wantedKind || actualRun != runID {
		return errors.New("guarded record kind or run does not match canonical bytes")
	}
	if wantedKind == "round" && (actualInstant != instant || actualOrdinal != ordinal) {
		return errors.New("guarded round coordinate does not match canonical bytes")
	}
	return nil
}

func (s *Store) AppendSourceDeclaration(_ context.Context, runID [32]byte, record ports.CanonicalRecord) error {
	if err := validateGuardArguments(record, "source", runID, 0, 0); err != nil { return err }
	s.mu.Lock();defer s.mu.Unlock()
	for _,seal:=range s.seals {if seal.runID==runID {return errors.New("source declaration after admission fence")}}
	return s.appendInputLocked(record)
}

func (s *Store) AppendRoundDeclaration(_ context.Context,runID [32]byte,instant int64,ordinal uint64,record ports.CanonicalRecord) error {
	if err := validateGuardArguments(record, "round", runID, instant, ordinal); err != nil { return err }
	s.mu.Lock();defer s.mu.Unlock()
	for _,seal:=range s.seals {
		if seal.runID==runID && (instant<seal.instant || (instant==seal.instant && ordinal<=seal.ordinal)) {
			return errors.New("round eligible at or before admission fence")
		}
	}
	return s.appendInputLocked(record)
}

func (s *Store) AppendIngress(_ context.Context, source string, sequence uint64, record, receipt ports.CanonicalRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sequence != s.ingressCursor[source] {
		return errors.New("source ingress sequence is not gapless")
	}
	receiptID := [32]byte(identity.OpaqueContentHash(receipt.Canonical))
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

func (s *Store) AppendFence(_ context.Context, record ports.CanonicalRecord) error {
	// Unguarded publication cannot assert prefix consistency.
	return errors.New("admission fence requires prefix-conditional append")
}
func (s *Store) GetFence(_ context.Context,id [32]byte)([]byte,error){
	s.mu.RLock();defer s.mu.RUnlock()
	v,ok:=s.fences[id];if !ok{return nil,ErrNotFound}
	return bytes.Clone(v),nil
}
func (s *Store) AppendFenceIfPrefix(_ context.Context,fence ports.CanonicalRecord,expected []ports.CanonicalRecord)error{
	s.mu.Lock();defer s.mu.Unlock()
	if len(expected)!=len(s.order){return errors.New("admission input prefix changed")}
	for i,item:=range expected{
		if item.Identity!=s.order[i]||!bytes.Equal(item.Canonical,s.records[item.Identity]){
			return errors.New("admission input prefix changed")
		}
	}
	value,err:=codec.StrictDecodePayload(fence.Canonical)
	if err!=nil{return err}
	fields,ok:=value.([]any)
	if !ok||len(fields)<6{return errors.New("invalid admission fence payload")}
	id,ok:=fields[0].([]byte);if !ok||len(id)!=32{return errors.New("invalid admission fence run")}
	instant,ok:=decodedInstant(fields[4]);if !ok{return errors.New("invalid fence instant")}
	ordinal,ok:=fields[5].(uint64);if !ok{return errors.New("invalid fence ordinal")}
	if prior,ok:=s.fences[fence.Identity];ok{return domain.EqualPreimage(prior,fence.Canonical)}
	var runID [32]byte;copy(runID[:],id)
	s.fences[fence.Identity]=bytes.Clone(fence.Canonical)
	s.seals=append(s.seals,admissionSeal{runID:runID,instant:instant,ordinal:ordinal})
	return nil
}

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

func (s *Store) Records(_ context.Context) ([]ports.CanonicalRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]ports.CanonicalRecord, 0, len(s.order))
	for _, id := range s.order {
		result = append(result, ports.CanonicalRecord{Identity: id, Canonical: bytes.Clone(s.records[id])})
	}
	return result, nil
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

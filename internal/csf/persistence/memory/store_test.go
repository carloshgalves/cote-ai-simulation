package memory

import (
	"context"
	"errors"
	"testing"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
)

func TestAppendIsIdempotentButFailsOnIdentityCollision(t *testing.T) {
	store := New()
	record := ports.CanonicalRecord{Identity: [32]byte{1}, Canonical: []byte{1, 2, 3}}
	if err := store.Append(context.Background(), record); err != nil {
		t.Fatal(err)
	}
	if err := store.Append(context.Background(), record); err != nil {
		t.Fatal(err)
	}
	record.Canonical = []byte{1, 2, 4}
	if err := store.Append(context.Background(), record); !errors.Is(err, domain.ErrIdentityCollision) {
		t.Fatalf("want identity collision, got %v", err)
	}
	if store.Len() != 1 {
		t.Fatalf("idempotent append changed length to %d", store.Len())
	}
}

func TestAuthoritiesDoNotShareState(t *testing.T) {
	authorities := NewAuthorities()
	record := ports.CanonicalRecord{Identity: [32]byte{1}, Canonical: []byte{1}}
	if err := authorities.InputLedger.Append(context.Background(), record); err != nil {
		t.Fatal(err)
	}
	if _, err := authorities.EventStore.Get(context.Background(), record.Identity); !errors.Is(err, ErrNotFound) {
		t.Fatal("event store observed input ledger state")
	}
}

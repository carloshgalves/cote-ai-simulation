package ownercheckpoint

import (
	"context"
	"errors"
	"testing"
)

// The owner may take long enough to flush the ACK for a new fence or
// evidence completion to occur. Never convert that ACK to a live permit.
type e3AdvancingAckOwner struct {
	*testDurableOwner
	escrow *testEscrow
}

func (o *e3AdvancingAckOwner) Acknowledge(ctx context.Context, ack RestoreAck) error {
	if err := o.testDurableOwner.Acknowledge(ctx, ack); err != nil {
		return err
	}
	o.escrow.mu.Lock()
	o.escrow.ownerWorkBlocked = true
	o.escrow.mu.Unlock()
	return nil
}

func TestE3OwnerACKCannotGrantPermitAfterSameRevisionBarrierChanges(t *testing.T) {
	s := setupOwnerGate(t)
	receipt := testStage(t, s)
	s.publishSnapshot(t, s.cp)
	gate, err := NewGate(&e3AdvancingAckOwner{s.owner, s.escrow}, s.escrow, s.gate.bundle)
	if err != nil {
		t.Fatal(err)
	}
	s.gate = gate
	if err := s.gate.RestoreAndAcknowledge(context.Background(), s.scope, s.snapshot, receipt); !errors.Is(err, ErrNotAcknowledged) {
		t.Fatalf("ACK granted permit after the authoritative cut became blocked: %v", err)
	}
	if s.owner.acked != 1 {
		t.Fatal("test did not actually cross the ACK boundary")
	}
	// Even if the fake authority becomes available later, the failed
	// restore never installed a permit. A new explicit restore is required.
	s.escrow.mu.Lock()
	s.escrow.ownerWorkBlocked = false
	s.escrow.mu.Unlock()
	assertNotPermitted(t, s, receipt)
}

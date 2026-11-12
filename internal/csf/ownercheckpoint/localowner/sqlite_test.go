package localowner

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/ownercheckpoint"
	csfsqlite "github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
)

func fixture(t *testing.T) (string, ed25519.PrivateKey, ownercheckpoint.PersistRequest) {
	t.Helper()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	scope := ownercheckpoint.Scope{Run: [32]byte{0x11}, Namespace: "cognition", Actor: "agent-a"}
	cp := csfsqlite.SnapshotCheckpoint{
		ActorNamespace: scope.Namespace, ActorID: scope.Actor,
		StoreID: "private-memory", SchemaVersion: 1,
		Producer: "owner-a", ProducerVersion: 1,
		CoversInstant: 17, CoversRevision: 2, CoversEvidenceCursor: 9,
		Content: []byte("synthetic-private-memory"),
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(dir, "owner.db"), key, ownercheckpoint.PersistRequest{Scope: scope, Checkpoint: cp}
}
func openFixture(t *testing.T, path string, key ed25519.PrivateKey) *Store {
	t.Helper()
	o, err := Open(context.Background(), path, key)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = o.Close() })
	return o
}
func makeRestore(p ownercheckpoint.PersistRequest, s ownercheckpoint.SignedReceipt) ownercheckpoint.RestoreRequest {
	raw, _ := csfsqlite.CognitiveCheckpointSigningBytesV1(p.Scope.Run, p.Checkpoint, s.Locator)
	return ownercheckpoint.RestoreRequest{Scope: p.Scope, Locator: s.Locator,
		ReceiptID: identity.OpaqueContentHash(raw), ContentHash: identity.OpaqueContentHash(p.Checkpoint.Content)}
}
func makeACK(r ownercheckpoint.RestoreRequest) ownercheckpoint.RestoreAck {
	return ownercheckpoint.RestoreAck{Scope: r.Scope, Locator: r.Locator, ReceiptID: r.ReceiptID,
		ContentHash: r.ContentHash, SnapshotID: [32]byte{0x77}}
}

func TestB1CommitSignReopenAndDurableACK(t *testing.T) {
	ctx := context.Background()
	path, key, p := fixture(t)
	owner := openFixture(t, path, key)
	signed, err := owner.PersistAndSign(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := csfsqlite.CognitiveCheckpointSigningBytesV1(p.Scope.Run, p.Checkpoint, signed.Locator)
	if err != nil || !ed25519.Verify(key.Public().(ed25519.PublicKey), raw, signed.Signature) {
		t.Fatalf("bad Ed25519 receipt %v", err)
	}
	same, err := owner.PersistAndSign(ctx, p)
	if err != nil || same.Locator != signed.Locator || !bytes.Equal(same.Signature, signed.Signature) {
		t.Fatalf("non-idempotent receipt %v", err)
	}
	req := makeRestore(p, signed)
	ack := makeACK(req)
	if err := owner.Acknowledge(ctx, ack); err == nil {
		t.Fatal("ACK before restore")
	}
	if err := owner.Close(); err != nil {
		t.Fatal(err)
	}
	opened := openFixture(t, path, key)
	if err := opened.Acknowledge(ctx, ack); err == nil {
		t.Fatal("prior session authorized ACK")
	}
	restored, err := opened.Restore(ctx, req)
	if err != nil || restored.Locator != signed.Locator || !bytes.Equal(restored.Content, p.Checkpoint.Content) {
		t.Fatalf("bad restored bytes %v", err)
	}
	if err := opened.Acknowledge(ctx, ack); err != nil {
		t.Fatal(err)
	}
	if err := opened.Acknowledge(ctx, ack); err != nil {
		t.Fatalf("duplicate ACK: %v", err)
	}
	var count int
	if err := opened.Close(); err != nil {
		t.Fatal(err)
	}
	third := openFixture(t, path, key)
	if err := third.conn.QueryRowContext(ctx, "SELECT count(*) FROM restore_acks").Scan(&count); err != nil || count != 1 {
		t.Fatalf("ACK did not survive second reopen %d %v", count, err)
	}
	if err := third.Acknowledge(ctx, ack); err == nil {
		t.Fatal("persisted ACK falsely proved restoration in a new process")
	}
	if _, err := third.Restore(ctx, req); err != nil {
		t.Fatal(err)
	}
	if err := third.Acknowledge(ctx, ack); err != nil {
		t.Fatalf("idempotent ACK after second restore: %v", err)
	}
	stat, err := os.Stat(path)
	if err != nil || stat.Mode().Perm()&0077 != 0 {
		t.Fatalf("database permissions unsafe %v", err)
	}
}
func TestB1IsolationImmutabilityAndTamper(t *testing.T) {
	ctx := context.Background()
	path, key, p := fixture(t)
	o := openFixture(t, path, key)
	signed, err := o.PersistAndSign(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	changed := p
	changed.Checkpoint.Content = []byte("different")
	if _, err := o.PersistAndSign(ctx, changed); err == nil {
		t.Fatal("same-cut conflict accepted")
	}
	changed = p
	changed.Scope.Actor = "agent-b"
	if _, err := o.PersistAndSign(ctx, changed); err == nil {
		t.Fatal("wrong actor accepted")
	}
	r := makeRestore(p, signed)
	cases := []struct {
		name string
		edit func(*ownercheckpoint.RestoreRequest)
	}{
		{"run", func(x *ownercheckpoint.RestoreRequest) { x.Scope.Run[0]++ }},
		{"actor", func(x *ownercheckpoint.RestoreRequest) { x.Scope.Actor = "agent-b" }},
		{"locator", func(x *ownercheckpoint.RestoreRequest) { x.Locator = "foreign" }},
		{"receipt", func(x *ownercheckpoint.RestoreRequest) { x.ReceiptID[0]++ }},
		{"digest", func(x *ownercheckpoint.RestoreRequest) { x.ContentHash[0]++ }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bad := r
			tc.edit(&bad)
			if _, err := o.Restore(ctx, bad); err == nil {
				t.Fatal("foreign restore accepted")
			}
		})
	}
	if _, err := o.Restore(ctx, r); err != nil {
		t.Fatal(err)
	}
	ack := makeACK(r)
	bad := ack
	bad.ReceiptID[0]++
	if err := o.Acknowledge(ctx, bad); err == nil {
		t.Fatal("forged ACK")
	}
	bad = ack
	bad.ContentHash[0]++
	if err := o.Acknowledge(ctx, bad); err == nil {
		t.Fatal("wrong hash ACK")
	}
	bad = ack
	bad.SnapshotID = [32]byte{}
	if err := o.Acknowledge(ctx, bad); err == nil {
		t.Fatal("zero snapshot ACK")
	}
	if _, err := o.conn.ExecContext(ctx, "UPDATE checkpoints SET content=? WHERE locator=?", []byte("tampered"), signed.Locator); err != nil {
		t.Fatal(err)
	}
	if _, err := o.Restore(ctx, r); err == nil {
		t.Fatal("tampered payload restored")
	}
	if err := o.Close(); err != nil {
		t.Fatal(err)
	}
	_, wrong, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	alien := openFixture(t, path, wrong)
	if _, err := alien.Restore(ctx, r); err == nil {
		t.Fatal("wrong owner signing key restored")
	}
	if _, err := alien.PersistAndSign(ctx, p); err == nil {
		t.Fatal("wrong owner key signed old checkpoint")
	}
}
func TestB1CallerBufferAndReturnedRestoreAreNotOwnerMemory(t *testing.T) {
	ctx := context.Background()
	path, key, p := fixture(t)
	o := openFixture(t, path, key)
	original := bytes.Clone(p.Checkpoint.Content)
	signed, err := o.PersistAndSign(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	p.Checkpoint.Content[0] ^= 0x31
	p.Checkpoint.Content = original
	req := makeRestore(p, signed)
	restored, err := o.Restore(ctx, req)
	if err != nil || !bytes.Equal(restored.Content, original) {
		t.Fatalf("mutable caller buffer changed committed bytes: %v", err)
	}
	restored.Content[0] ^= 0x35
	ack := makeACK(req)
	if err := o.Acknowledge(ctx, ack); err != nil {
		t.Fatalf("caller-owned return mutated owner state: %v", err)
	}
	if !bytes.Equal(original, p.Checkpoint.Content) {
		t.Fatal("test input unexpectedly mutated")
	}
	if _, err := o.conn.ExecContext(ctx, "UPDATE checkpoints SET content=? WHERE locator=?", []byte("tampered"), signed.Locator); err != nil {
		t.Fatal(err)
	}
	if err := o.Acknowledge(ctx, ack); err == nil {
		t.Fatal("ACK ignored post-restore durable corruption")
	}
}

func TestB1PrivateRestoredBuffersWipedAtClose(t *testing.T) {
	ctx := context.Background()
	path, key, p := fixture(t)
	owner := openFixture(t, path, key)
	signed, err := owner.PersistAndSign(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	req := makeRestore(p, signed)
	received, err := owner.Restore(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	k := restoreKey{Scope: req.Scope, Locator: req.Locator, ReceiptID: req.ReceiptID, ContentHash: req.ContentHash}
	retained := owner.restored[k]
	if len(retained) == 0 || !bytes.Equal(received.Content, retained) {
		t.Fatal("restored checkpoint state not held by owner")
	}
	if err := owner.Close(); err != nil {
		t.Fatal(err)
	}
	for _, b := range retained {
		if b != 0 {
			t.Fatal("private owner buffer not erased on Close")
		}
	}
	if !bytes.Equal(received.Content, p.Checkpoint.Content) {
		t.Fatal("caller buffer unexpectedly wiped")
	}
}

func TestB1ConcurrentRetriesOneCheckpoint(t *testing.T) {
	ctx := context.Background()
	path, key, p := fixture(t)
	o := openFixture(t, path, key)
	var wg sync.WaitGroup
	const n = 16
	receipts := make(chan ownercheckpoint.SignedReceipt, n)
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s, e := o.PersistAndSign(ctx, p)
			if e != nil {
				errs <- e
			} else {
				receipts <- s
			}
		}()
	}
	wg.Wait()
	close(errs)
	close(receipts)
	for e := range errs {
		t.Error(e)
	}
	count := 0
	var first ownercheckpoint.SignedReceipt
	for s := range receipts {
		if count > 0 && (first.Locator != s.Locator || !bytes.Equal(first.Signature, s.Signature)) {
			t.Error("divergent receipt")
		}
		first = s
		count++
	}
	if count != n {
		t.Fatalf("got %d receipts", count)
	}
	var records int
	if err := o.conn.QueryRowContext(ctx, "SELECT count(*) FROM checkpoints").Scan(&records); err != nil || records != 1 {
		t.Fatalf("got %d checkpoints: %v", records, err)
	}
}
func TestB1RefuseUnsafeStoreAndScope(t *testing.T) {
	path, key, p := fixture(t)
	if _, err := Open(context.Background(), path, nil); err == nil {
		t.Fatal("nil signer accepted")
	}
	if _, err := Open(context.Background(), "", key); err == nil {
		t.Fatal("empty path accepted")
	}
	if _, err := Open(context.Background(), ":memory:", key); err == nil {
		t.Fatal("memory-only path accepted")
	}
	open := filepath.Join(t.TempDir(), "insecure.db")
	if err := os.WriteFile(open, nil, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(context.Background(), open, key); err == nil {
		t.Fatal("world-readable file accepted")
	}
	o := openFixture(t, path, key)
	p.Checkpoint.ActorID = "other"
	if _, err := o.PersistAndSign(context.Background(), p); err == nil {
		t.Fatal("scope mismatch accepted")
	}
}

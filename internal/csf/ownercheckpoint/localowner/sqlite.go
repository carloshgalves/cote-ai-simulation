// Package localowner provides an owner-owned, single-operator SQLite checkpoint store.
// It is deliberately separate from the CSF causal SQLite authority.
// This is a local crash-recovery demonstrator, not remote durability or tenant isolation.
package localowner

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/ownercheckpoint"
	csfsqlite "github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
	modernsqlite "modernc.org/sqlite"
)

// Store owns private checkpoint bytes. CSF remains the sole authority for
// world truth, canonical snapshots, signed-receipt escrow and their trust roster.
//
// One sql.Conn, guarded by mu, is deliberately used for this proof. Every
// physical connection is verified before any write; no pool replacement
// silently admits a connection with weaker PRAGMAs.
type Store struct {
	mu       sync.Mutex
	db       *sql.DB
	conn     *sql.Conn
	signer   ed25519.PrivateKey
	public   ed25519.PublicKey
	restored map[restoreKey][]byte
}

type restoreKey struct {
	Scope       ownercheckpoint.Scope
	Locator     string
	ReceiptID   [32]byte
	ContentHash [32]byte
}

var _ ownercheckpoint.DurableOwner = (*Store)(nil)

var errClosed = errors.New("cognitive owner store is closed")

const schema = `
CREATE TABLE IF NOT EXISTS checkpoints (
 locator TEXT PRIMARY KEY NOT NULL,
 run_id BLOB NOT NULL CHECK(length(run_id)=32),
 actor_namespace TEXT NOT NULL,
 actor_id TEXT NOT NULL,
 store_id TEXT NOT NULL,
 schema_version INTEGER NOT NULL CHECK(schema_version>0),
 producer TEXT NOT NULL,
 producer_version INTEGER NOT NULL CHECK(producer_version>0),
 covers_instant INTEGER NOT NULL,
 covers_revision INTEGER NOT NULL CHECK(covers_revision>=0),
 covers_evidence_cursor INTEGER NOT NULL CHECK(covers_evidence_cursor>=0),
 content BLOB NOT NULL,
 content_hash BLOB NOT NULL CHECK(length(content_hash)=32),
 signer_public BLOB NOT NULL CHECK(length(signer_public)=32),
 UNIQUE(run_id,actor_namespace,actor_id,covers_instant,covers_revision,covers_evidence_cursor)
) STRICT;
CREATE TABLE IF NOT EXISTS restore_acks (
 run_id BLOB NOT NULL CHECK(length(run_id)=32),
 actor_namespace TEXT NOT NULL,
 actor_id TEXT NOT NULL,
 receipt_id BLOB NOT NULL CHECK(length(receipt_id)=32),
 snapshot_id BLOB NOT NULL CHECK(length(snapshot_id)=32),
 locator TEXT NOT NULL,
 content_hash BLOB NOT NULL CHECK(length(content_hash)=32),
 PRIMARY KEY(run_id,actor_namespace,actor_id,receipt_id,snapshot_id),
 FOREIGN KEY(locator) REFERENCES checkpoints(locator)
) STRICT;`

// Open creates or reopens an owner database. Callers must supply a private,
// dedicated directory and the *same externally managed* signer on reopen.
// The key is cloned in memory, never written to SQLite or CSF.
func Open(ctx context.Context, path string, signer ed25519.PrivateKey) (*Store, error) {
	if ctx == nil || len(signer) != ed25519.PrivateKeySize {
		return nil, errors.New("owner requires context and Ed25519 private key")
	}
	public, ok := signer.Public().(ed25519.PublicKey)
	if !ok || len(public) != ed25519.PublicKeySize {
		return nil, errors.New("invalid cognitive owner signer")
	}
	probe := []byte("cote.owner.local.v1.key-check")
	if !ed25519.Verify(public, probe, ed25519.Sign(signer, probe)) {
		return nil, errors.New("cognitive owner keypair mismatch")
	}
	if path == "" || path == ":memory:" || strings.HasPrefix(path, "file:") {
		return nil, errors.New("owner requires a private on-disk SQLite path")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if err := preparePrivateFile(absolute); err != nil {
		return nil, err
	}
	query := url.Values{}
	query.Add("_pragma", "journal_mode(WAL)")
	query.Add("_pragma", "synchronous(FULL)")
	query.Add("_pragma", "foreign_keys(ON)")
	query.Add("_pragma", "busy_timeout(5000)")
	dsn := (&url.URL{Scheme: "file", Path: filepath.ToSlash(absolute), RawQuery: query.Encode()}).String()
	connector, err := modernsqlite.NewConnector(dsn)
	if err != nil {
		return nil, err
	}
	db := sql.OpenDB(connector)
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	conn, err := db.Conn(ctx)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	s := &Store{db: db, conn: conn, signer: bytes.Clone(signer), public: bytes.Clone(public),
		restored: make(map[restoreKey][]byte)}
	if err := s.verifyConnection(ctx); err != nil {
		_ = s.Close()
		return nil, err
	}
	if err := s.rejectCausalSchema(ctx); err != nil {
		_ = s.Close()
		return nil, err
	}
	if _, err := conn.ExecContext(ctx, schema); err != nil {
		_ = s.Close()
		return nil, err
	}
	return s, nil
}

// preparePrivateFile prevents accidental plaintext memory in shared directories.
// This is a single-operator file-path precondition, not protection from a
// malicious local process racing pathname replacement.
func preparePrivateFile(path string) error {
	parent, err := os.Stat(filepath.Dir(path))
	if err != nil {
		return err
	}
	if !parent.IsDir() || parent.Mode().Perm()&0077 != 0 {
		return errors.New("owner database parent directory must be private (0700)")
	}
	fi, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		f, e := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
		if e != nil {
			return e
		}
		return f.Close()
	}
	if err != nil {
		return err
	}
	if !fi.Mode().IsRegular() || fi.Mode().Perm()&0077 != 0 {
		return errors.New("owner database must be a private regular file (0600)")
	}
	return nil
}

func (s *Store) verifyConnection(ctx context.Context) error {
	var journal string
	var synchronous, foreignKeys int
	if err := s.conn.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&journal); err != nil {
		return err
	}
	if err := s.conn.QueryRowContext(ctx, "PRAGMA synchronous").Scan(&synchronous); err != nil {
		return err
	}
	if err := s.conn.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&foreignKeys); err != nil {
		return err
	}
	if !strings.EqualFold(journal, "wal") || synchronous != 2 || foreignKeys != 1 {
		return fmt.Errorf("owner SQLite requires WAL, FULL, foreign_keys ON: journal=%s synchronous=%d foreign_keys=%d", journal, synchronous, foreignKeys)
	}
	return nil
}

// rejectCausalSchema prevents accidental reuse of an existing CSF authority
// file as the owner-owned store. The B2 host must also enforce distinct paths
// if it opens the owner before its CSF file exists.
func (s *Store) rejectCausalSchema(ctx context.Context) error {
	var count int
	err := s.conn.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_master
        WHERE type='table' AND name IN (
          'genesis_records','input_ledger','world_revisions',
          'snapshot_seals','cognitive_owner_receipts')`).Scan(&count)
	if err != nil {
		return err
	}
	if count != 0 {
		return errors.New("refusing to reuse CSF causal SQLite file for cognitive owner")
	}
	return nil
}

// VerifyIndependent rechecks the live owner file, not only its state at Open.
// A causal Store opened on the same path *after* owner.Open adds CSF tables;
// that misconfiguration must fail closed before any B2 stage/restore/WORK.
// The local host invokes this check at each privileged entry point.
func (s *Store) VerifyIndependent(ctx context.Context) error {
	if s == nil || ctx == nil {
		return errors.New("owner independence check unconfigured")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.conn == nil {
		return errClosed
	}
	return s.rejectCausalSchema(ctx)
}

func (s *Store) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.conn == nil {
		return nil
	}
	conn := s.conn
	s.conn = nil
	for _, memory := range s.restored {
		clear(memory)
	}
	s.restored = nil
	for i := range s.signer {
		s.signer[i] = 0
	}
	err := conn.Close()
	if e := s.db.Close(); err == nil {
		err = e
	}
	return err
}

func validScope(scope ownercheckpoint.Scope) error {
	if err := domain.ValidateMachineID(scope.Namespace); err != nil {
		return err
	}
	return domain.ValidateMachineID(scope.Actor)
}
func validateCheckpoint(req ownercheckpoint.PersistRequest) error {
	if err := validScope(req.Scope); err != nil {
		return err
	}
	cp := req.Checkpoint
	if cp.ActorNamespace != req.Scope.Namespace || cp.ActorID != req.Scope.Actor {
		return errors.New("owner checkpoint scope differs from requested actor")
	}
	if cp.CoversRevision > math.MaxInt64 || cp.CoversEvidenceCursor > math.MaxInt64 {
		return errors.New("owner coverage exceeds SQLite integer range")
	}
	_, err := csfsqlite.CognitiveCheckpointSigningBytesV1(req.Scope.Run, cp, "owner-seed")
	return err
}

// ownerLocator is deterministic, opaque, portable machine-id. Its preimage
// uses the existing canonical R22 signing contract with a fixed seed locator.
func ownerLocator(req ownercheckpoint.PersistRequest) (string, error) {
	preimage, err := csfsqlite.CognitiveCheckpointSigningBytesV1(req.Scope.Run, req.Checkpoint, "owner-seed")
	if err != nil {
		return "", err
	}
	digest := identity.OpaqueContentHash(preimage)
	return "owner-" + hex.EncodeToString(digest[:]), nil
}

// PersistAndSign commits private bytes to the owner file BEFORE generating
// or returning the Ed25519 receipt. There is no transaction across owner/CSF.
func (s *Store) PersistAndSign(ctx context.Context, req ownercheckpoint.PersistRequest) (ownercheckpoint.SignedReceipt, error) {
	var zero ownercheckpoint.SignedReceipt
	if ctx == nil || s == nil {
		return zero, errors.New("owner persist unconfigured")
	}
	// Take a private immutable snapshot of the caller's buffer before computing
	// any digest, SQL value or signing bytes. Concurrent mutation *during*
	// this copy is a caller data race and remains explicitly unsupported.
	req.Checkpoint.Content = bytes.Clone(req.Checkpoint.Content)
	// Best-effort erasure of this method's private copy after commit/sign.
	defer clear(req.Checkpoint.Content)
	if err := validateCheckpoint(req); err != nil {
		return zero, err
	}
	locator, err := ownerLocator(req)
	if err != nil {
		return zero, err
	}
	digest := identity.OpaqueContentHash(req.Checkpoint.Content)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.conn == nil {
		return zero, errClosed
	}
	if err := s.rejectCausalSchema(ctx); err != nil {
		return zero, fmt.Errorf("owner/CSF database separation: %w", err)
	}
	cp := req.Checkpoint
	tx, err := s.conn.BeginTx(ctx, nil)
	if err != nil {
		return zero, err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO checkpoints
        (locator,run_id,actor_namespace,actor_id,store_id,schema_version,producer,producer_version,
         covers_instant,covers_revision,covers_evidence_cursor,content,content_hash,signer_public)
        VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		locator, req.Scope.Run[:], req.Scope.Namespace, req.Scope.Actor, cp.StoreID, cp.SchemaVersion,
		cp.Producer, cp.ProducerVersion, cp.CoversInstant, cp.CoversRevision, cp.CoversEvidenceCursor,
		bytes.Clone(cp.Content), digest[:], s.public)
	if err != nil {
		return zero, err
	}
	var storedLocator, storeID, producer string
	var version, producerVersion int64
	var data, storedHash, storedPub []byte
	err = tx.QueryRowContext(ctx, `SELECT locator,store_id,schema_version,producer,producer_version,
        content,content_hash,signer_public FROM checkpoints
        WHERE run_id=? AND actor_namespace=? AND actor_id=? AND covers_instant=?
          AND covers_revision=? AND covers_evidence_cursor=?`,
		req.Scope.Run[:], req.Scope.Namespace, req.Scope.Actor, cp.CoversInstant,
		cp.CoversRevision, cp.CoversEvidenceCursor).
		Scan(&storedLocator, &storeID, &version, &producer, &producerVersion, &data, &storedHash, &storedPub)
	if err != nil {
		return zero, fmt.Errorf("owner checkpoint cannot prove immutable cut: %w", err)
	}
	if storedLocator != locator || storeID != cp.StoreID || version != int64(cp.SchemaVersion) ||
		producer != cp.Producer || producerVersion != int64(cp.ProducerVersion) ||
		!bytes.Equal(data, cp.Content) || !bytes.Equal(storedHash, digest[:]) ||
		!bytes.Equal(storedPub, s.public) {
		return zero, errors.New("conflicting or corrupted owner checkpoint at same cut")
	}
	if err := tx.Commit(); err != nil {
		return zero, fmt.Errorf("owner durable commit failed: %w", err)
	}
	// Signing only happens after the SQLite FULL-synchronous commit returned.
	signedBytes, err := csfsqlite.CognitiveCheckpointSigningBytesV1(req.Scope.Run, cp, locator)
	if err != nil {
		return zero, err
	}
	for k, memory := range s.restored {
		if k.Scope == req.Scope {
			clear(memory)
			delete(s.restored, k)
		}
	}
	return ownercheckpoint.SignedReceipt{Locator: locator, Signature: ed25519.Sign(s.signer, signedBytes)}, nil
}

// loadVerified is called under mu and reads from the owner file, never from
// caller-provided content or a cached, model-generated memory representation.
func (s *Store) loadVerified(ctx context.Context, req ownercheckpoint.RestoreRequest) ([]byte, error) {
	if err := validScope(req.Scope); err != nil {
		return nil, err
	}
	if err := domain.ValidateMachineID(req.Locator); err != nil {
		return nil, err
	}
	if req.ReceiptID == ([32]byte{}) || req.ContentHash == ([32]byte{}) {
		return nil, errors.New("owner restore requires exact receipt and content hash")
	}
	cp := csfsqlite.SnapshotCheckpoint{ActorNamespace: req.Scope.Namespace, ActorID: req.Scope.Actor}
	var storedHash, storedPub []byte
	err := s.conn.QueryRowContext(ctx, `SELECT store_id,schema_version,producer,producer_version,
        covers_instant,covers_revision,covers_evidence_cursor,content,content_hash,signer_public
        FROM checkpoints WHERE locator=? AND run_id=? AND actor_namespace=? AND actor_id=?`,
		req.Locator, req.Scope.Run[:], req.Scope.Namespace, req.Scope.Actor).
		Scan(&cp.StoreID, &cp.SchemaVersion, &cp.Producer, &cp.ProducerVersion,
			&cp.CoversInstant, &cp.CoversRevision, &cp.CoversEvidenceCursor, &cp.Content, &storedHash, &storedPub)
	if err != nil {
		return nil, errors.New("owner checkpoint not found for requested scope and locator")
	}
	digest := identity.OpaqueContentHash(cp.Content)
	if !bytes.Equal(storedPub, s.public) || !bytes.Equal(storedHash, digest[:]) ||
		digest != req.ContentHash {
		return nil, errors.New("owner checkpoint integrity or key mismatch")
	}
	expectedLocator, err := ownerLocator(ownercheckpoint.PersistRequest{Scope: req.Scope, Checkpoint: cp})
	if err != nil || expectedLocator != req.Locator {
		return nil, errors.New("owner checkpoint locator integrity mismatch")
	}
	signingBytes, err := csfsqlite.CognitiveCheckpointSigningBytesV1(req.Scope.Run, cp, req.Locator)
	if err != nil {
		return nil, err
	}
	if identity.OpaqueContentHash(signingBytes) != req.ReceiptID {
		return nil, errors.New("owner receipt does not bind exact checkpoint")
	}
	owned := bytes.Clone(cp.Content)
	clear(cp.Content)
	return owned, nil
}

// Restore rehydrates the requested checkpoint from local durable storage.
// Only the exact receipt restored in this Store session may later be ACKed.
func (s *Store) Restore(ctx context.Context, req ownercheckpoint.RestoreRequest) (ownercheckpoint.RestoredContent, error) {
	var zero ownercheckpoint.RestoredContent
	if s == nil || ctx == nil {
		return zero, errors.New("owner restore unconfigured")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.conn == nil {
		return zero, errClosed
	}
	for k, memory := range s.restored {
		if k.Scope == req.Scope {
			clear(memory)
			delete(s.restored, k)
		}
	}
	if err := s.rejectCausalSchema(ctx); err != nil {
		return zero, fmt.Errorf("owner/CSF database separation: %w", err)
	}
	data, err := s.loadVerified(ctx, req)
	if err != nil {
		return zero, err
	}
	key := restoreKey{Scope: req.Scope, Locator: req.Locator, ReceiptID: req.ReceiptID, ContentHash: req.ContentHash}
	s.restored[key] = bytes.Clone(data)
	return ownercheckpoint.RestoredContent{Locator: req.Locator, Content: data}, nil
}

// Acknowledge records exactly which signed checkpoint was restored in this
// process, plus the snapshot identity supplied by the already-verifying Gate.
// An ACK on disk alone never grants a work permit on reopen.
func (s *Store) Acknowledge(ctx context.Context, ack ownercheckpoint.RestoreAck) error {
	if s == nil || ctx == nil {
		return errors.New("owner ACK unconfigured")
	}
	if ack.SnapshotID == ([32]byte{}) {
		return errors.New("owner ACK requires snapshot identity")
	}
	key := restoreKey{Scope: ack.Scope, Locator: ack.Locator, ReceiptID: ack.ReceiptID, ContentHash: ack.ContentHash}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.conn == nil {
		return errClosed
	}
	if err := s.rejectCausalSchema(ctx); err != nil {
		return fmt.Errorf("owner/CSF database separation: %w", err)
	}
	restored, ok := s.restored[key]
	if !ok {
		return errors.New("owner ACK requires restored checkpoint in current session")
	}
	req := ownercheckpoint.RestoreRequest{Scope: ack.Scope, Locator: ack.Locator,
		ReceiptID: ack.ReceiptID, ContentHash: ack.ContentHash}
	current, err := s.loadVerified(ctx, req)
	if err != nil {
		clear(restored)
		delete(s.restored, key)
		return err
	}
	defer clear(current)
	if !bytes.Equal(restored, current) {
		clear(restored)
		delete(s.restored, key)
		return errors.New("owner restored memory differs from durable checkpoint")
	}
	tx, err := s.conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO restore_acks
        (run_id,actor_namespace,actor_id,receipt_id,snapshot_id,locator,content_hash)
        VALUES(?,?,?,?,?,?,?)`,
		ack.Scope.Run[:], ack.Scope.Namespace, ack.Scope.Actor, ack.ReceiptID[:],
		ack.SnapshotID[:], ack.Locator, ack.ContentHash[:])
	if err != nil {
		return err
	}
	var locator string
	var hash []byte
	err = tx.QueryRowContext(ctx, `SELECT locator,content_hash FROM restore_acks
        WHERE run_id=? AND actor_namespace=? AND actor_id=? AND receipt_id=? AND snapshot_id=?`,
		ack.Scope.Run[:], ack.Scope.Namespace, ack.Scope.Actor, ack.ReceiptID[:], ack.SnapshotID[:]).
		Scan(&locator, &hash)
	if err != nil || locator != ack.Locator || !bytes.Equal(hash, ack.ContentHash[:]) {
		return errors.New("owner ACK identity conflict")
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("owner ACK durable commit failed: %w", err)
	}
	return nil
}

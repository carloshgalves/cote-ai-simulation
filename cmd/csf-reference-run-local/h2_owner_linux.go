//go:build linux

package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
	csfsqlite "github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
)

type ownerAuditH2 struct {
	Schema                 string `json:"schema"`
	SourceSHA              string `json:"source_sha"`
	RunID                  string `json:"run_id"`
	EscrowedReceipts       int    `json:"escrowed_receipts"`
	PrivateCheckpoints     int    `json:"private_checkpoints"`
	DurableRestoreAcks     int    `json:"durable_restore_acks"`
	SignaturesVerified     bool   `json:"signatures_verified"`
	DistinctOwnerDatabases bool   `json:"distinct_owner_databases"`
	Disposition            string `json:"disposition"`
}

type h2Receipt struct {
	ID, Run                   []byte
	Namespace, Actor, StoreID string
	SchemaVersion             int64
	Content, Hash             []byte
	Producer                  string
	ProducerVersion           int64
	Instant, Revision, Cursor int64
	Locator                   string
	Signature, Public         []byte
}

func sameOwnerH2(ctx context.Context, db *sql.DB, receipt h2Receipt) (bool, [][]byte, error) {
	var run, content, hash, signer []byte
	var ns, actor, store, producer string
	var schema, producerVersion, instant, revision, cursor int64
	err := db.QueryRowContext(ctx, `SELECT run_id, actor_namespace, actor_id, store_id,
 schema_version, content, content_hash, producer, producer_version, covers_instant,
 covers_revision, covers_evidence_cursor, signer_public FROM checkpoints WHERE locator=?`,
		receipt.Locator).Scan(&run, &ns, &actor, &store, &schema, &content, &hash, &producer,
		&producerVersion, &instant, &revision, &cursor, &signer)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil, nil
	}
	if err != nil {
		return false, nil, err
	}
	if !bytes.Equal(run, receipt.Run) || ns != receipt.Namespace || actor != receipt.Actor ||
		store != receipt.StoreID || schema != receipt.SchemaVersion ||
		!bytes.Equal(content, receipt.Content) || !bytes.Equal(hash, receipt.Hash) ||
		producer != receipt.Producer || producerVersion != receipt.ProducerVersion ||
		instant != receipt.Instant || revision != receipt.Revision || cursor != receipt.Cursor ||
		!bytes.Equal(signer, receipt.Public) {
		return false, nil, errors.New("private owner checkpoint differs from signed causal escrow")
	}
	rows, err := db.QueryContext(ctx, `SELECT snapshot_id, locator, content_hash FROM restore_acks
 WHERE run_id=? AND actor_namespace=? AND actor_id=? AND receipt_id=?`,
		receipt.Run, receipt.Namespace, receipt.Actor, receipt.ID)
	if err != nil {
		return false, nil, err
	}
	defer rows.Close()
	ackIDs := make([][]byte, 0)
	for rows.Next() {
		var snapshot, locatorHash []byte
		var locator string
		if err := rows.Scan(&snapshot, &locator, &locatorHash); err != nil {
			return false, nil, err
		}
		if len(snapshot) != 32 || locator != receipt.Locator || !bytes.Equal(locatorHash, receipt.Hash) {
			return false, nil, errors.New("private owner restore ACK has foreign receipt or content")
		}
		ackIDs = append(ackIDs, bytes.Clone(snapshot))
	}
	if err := rows.Err(); err != nil {
		return false, nil, err
	}
	if len(ackIDs) == 0 {
		return false, nil, errors.New("private owner checkpoint was never restored and ACKed")
	}
	return true, ackIDs, nil
}

func auditOwnersH2(ctx context.Context, out, sha, run string) (ownerAuditH2, error) {
	var zero ownerAuditH2
	id, err := hex.DecodeString(run)
	if err != nil || len(id) != 32 {
		return zero, errors.New("invalid H2 owner RunID")
	}
	causalPath, cleanup, err := detachedDBH2(filepath.Join(out, "causal.db"))
	if err != nil {
		return zero, err
	}
	defer cleanup()
	causal, err := readOnlyDBH2(ctx, causalPath)
	if err != nil {
		return zero, err
	}
	defer causal.Close()
	ownerFiles := []string{"owner/actor-b.db.actor-a", "owner/actor-b.db"}
	owners := make([]*sql.DB, 0, 2)
	for _, name := range ownerFiles {
		path, cleanupOwner, err := detachedDBH2(filepath.Join(out, filepath.FromSlash(name)))
		if err != nil {
			for _, d := range owners {
				_ = d.Close()
			}
			return zero, err
		}
		defer cleanupOwner()
		db, err := readOnlyDBH2(ctx, path)
		if err != nil {
			for _, d := range owners {
				_ = d.Close()
			}
			return zero, err
		}
		owners = append(owners, db)
	}
	defer func() {
		for _, db := range owners {
			_ = db.Close()
		}
	}()
	rows, err := causal.QueryContext(ctx, `SELECT r.receipt_id,r.run_id,r.actor_namespace,r.actor_id,
 r.store_id,r.schema_version,r.content,r.content_hash,r.producer,r.producer_version,
 r.covers_instant,r.covers_revision,r.covers_evidence_cursor,r.locator,r.signature,
 k.public_key FROM cognitive_owner_receipts r
 JOIN cognitive_owner_keys k ON k.run_id=r.run_id AND k.actor_namespace=r.actor_namespace
 AND k.actor_id=r.actor_id WHERE r.run_id=? ORDER BY r.actor_namespace,r.actor_id`, id)
	if err != nil {
		return zero, err
	}
	defer rows.Close()
	receipts, acked := 0, 0
	pendingSnapshotIDs := make([][]byte, 0)
	matched := make([]int, len(owners))
	var runID [32]byte
	copy(runID[:], id)
	for rows.Next() {
		var r h2Receipt
		err := rows.Scan(&r.ID, &r.Run, &r.Namespace, &r.Actor, &r.StoreID,
			&r.SchemaVersion, &r.Content, &r.Hash, &r.Producer, &r.ProducerVersion,
			&r.Instant, &r.Revision, &r.Cursor, &r.Locator, &r.Signature, &r.Public)
		if err != nil {
			return zero, err
		}
		if len(r.ID) != 32 || !bytes.Equal(r.Run, id) || len(r.Public) != 32 || len(r.Signature) != 64 ||
			len(r.Hash) != 32 || r.SchemaVersion < 1 || r.ProducerVersion < 1 ||
			r.Revision < 0 || r.Cursor < 0 ||
			r.SchemaVersion > int64(^uint32(0)) || r.ProducerVersion > int64(^uint32(0)) {
			return zero, errors.New("malformed owner escrow identity")
		}
		contentHash := identity.OpaqueContentHash(r.Content)
		if !bytes.Equal(contentHash[:], r.Hash) {
			return zero, errors.New("signed owner escrow content hash mismatch")
		}
		cp := csfsqlite.SnapshotCheckpoint{
			ActorNamespace: r.Namespace, ActorID: r.Actor, StoreID: r.StoreID,
			SchemaVersion: uint32(r.SchemaVersion), Content: r.Content,
			Producer: r.Producer, ProducerVersion: uint32(r.ProducerVersion),
			CoversInstant: r.Instant, CoversRevision: uint64(r.Revision),
			CoversEvidenceCursor: uint64(r.Cursor),
		}
		canonical, err := csfsqlite.CognitiveCheckpointSigningBytesV1(runID, cp, r.Locator)
		if err != nil {
			return zero, err
		}
		receiptID := identity.OpaqueContentHash(canonical)
		if !bytes.Equal(receiptID[:], r.ID) ||
			!ed25519.Verify(ed25519.PublicKey(r.Public), canonical, r.Signature) {
			return zero, errors.New("invalid signed owner receipt")
		}
		ownerMatches := 0
		for i, db := range owners {
			match, snapshots, err := sameOwnerH2(ctx, db, r)
			if err != nil {
				return zero, err
			}
			if match {
				// Do not issue a nested query on the same single-connection
				// causal reader while its outer receipt cursor is live.
				pendingSnapshotIDs = append(pendingSnapshotIDs, snapshots...)
				ownerMatches++
				matched[i]++
				acked += len(snapshots)
			}
		}
		if ownerMatches != 1 {
			return zero, errors.New("owner receipt not uniquely restored by matching private database")
		}
		receipts++
	}
	if err := rows.Err(); err != nil {
		return zero, err
	}
	if err := rows.Close(); err != nil {
		return zero, err
	}
	for _, snapshot := range pendingSnapshotIDs {
		var exists int
		if err := causal.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM snapshot_seals WHERE run_id=? AND snapshot_id=?",
			id, snapshot).Scan(&exists); err != nil || exists != 1 {
			return zero, errors.New("private owner ACK references a foreign or unsealed snapshot")
		}
	}
	if receipts != 2 || matched[0] != 1 || matched[1] != 1 || acked < 2 {
		return zero, fmt.Errorf("incomplete distinct owner receipt/ACK coverage: receipts=%d", receipts)
	}
	for i, owner := range owners {
		var n int
		if err := owner.QueryRowContext(ctx, "SELECT COUNT(*) FROM checkpoints").Scan(&n); err != nil {
			return zero, err
		}
		if n != matched[i] {
			return zero, errors.New("unregistered private owner checkpoint present")
		}
	}
	return ownerAuditH2{
		Schema: "csfrv1-10h.owner-audit.v1", SourceSHA: sha, RunID: run,
		EscrowedReceipts: receipts, PrivateCheckpoints: matched[0] + matched[1],
		DurableRestoreAcks: acked, SignaturesVerified: true, DistinctOwnerDatabases: true,
		Disposition: "PRIVATE_LOCAL_ONLY_NO_PRODUCTION_GO",
	}, nil
}

func exportOwnersH2(ctx context.Context, out, sha, run string) error {
	doc, err := auditOwnersH2(ctx, out, sha, run)
	if err != nil {
		return err
	}
	return exclusiveJSON(filepath.Join(out, "ledger-export", "owner-audit.json"), doc)
}

func verifyOwnersH2(ctx context.Context, out, sha, run string) error {
	path := filepath.Join(out, "ledger-export", "owner-audit.json")
	raw, err := readPrivateRegular(path)
	if err != nil {
		return err
	}
	var doc ownerAuditH2
	if err := json.Unmarshal(raw, &doc); err != nil {
		return err
	}
	if !exactLocalJSON(raw, doc) {
		return errors.New("forged or extended private owner attestation")
	}
	reconstructed, err := auditOwnersH2(ctx, out, sha, run)
	if err != nil {
		return err
	}
	if doc != reconstructed {
		return errors.New("signed private owner checkpoints differ from detached causal authority")
	}
	return nil
}


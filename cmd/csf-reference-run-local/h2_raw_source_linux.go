//go:build linux

package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/adapter/embodimentv1"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/genesis"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
	"golang.org/x/sys/unix"
)

const rawA1ProofPathH2 = "physical-cuts/raw-a1-proof.json"

var rawA1EpochNamesH2 = [...]string{
	"snapshot.yaml", "events.jsonl", "evidence.json", "cut.cbor", "signature.bin",
}

func rawA1PathsH2() []string {
	paths := make([]string, 0, 11)
	for epoch := 1; epoch <= 2; epoch++ {
		for _, name := range rawA1EpochNamesH2 {
			paths = append(paths, fmt.Sprintf("physical-cuts/epoch-%d/%s", epoch, name))
		}
	}
	return append(paths, rawA1ProofPathH2)
}

type rawA1EpochProofH2 struct {
	Epoch                int    `json:"epoch"`
	SnapshotBytes        int    `json:"snapshot_bytes"`
	EventsBytes          int    `json:"events_bytes"`
	EvidenceBytes        int    `json:"evidence_bytes"`
	SnapshotSHA256       string `json:"snapshot_sha256"`
	EventsSHA256         string `json:"events_sha256"`
	EvidenceSHA256       string `json:"evidence_sha256"`
	CutSHA256            string `json:"cut_sha256"`
	SignatureSHA256      string `json:"signature_sha256"`
	CompletedStateSHA256 string `json:"completed_state_sha256"`
}
type rawA1ProofH2 struct {
	Schema        string               `json:"schema"`
	SourceSHA     string               `json:"source_sha"`
	RunID         string               `json:"run_id"`
	GenesisDigest string               `json:"genesis_digest"`
	SourceID      string               `json:"source_id"`
	Epochs        [2]rawA1EpochProofH2 `json:"epochs"`
	Disposition   string               `json:"disposition"`
}

func sealPrivateRawA1H2(raw []byte) (*os.File, error) {
	if len(raw) == 0 || len(raw) > 64*1024*1024 {
		return nil, errors.New("raw A1 cut exceeds the sealed source upper bound")
	}
	fd, err := unix.MemfdCreate("csf-h2-cold-source", unix.MFD_CLOEXEC|unix.MFD_ALLOW_SEALING)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), "csf-h2-cold-source")
	if n, err := f.Write(raw); err != nil || n != len(raw) {
		_ = f.Close()
		return nil, errors.New("cannot seal complete archived A1 raw bytes")
	}
	_, err = unix.FcntlInt(f.Fd(), unix.F_ADD_SEALS,
		unix.F_SEAL_WRITE|unix.F_SEAL_SHRINK|unix.F_SEAL_GROW|unix.F_SEAL_SEAL)
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	return f, nil
}

// inspectRawA1H2 reconstructs the proof from the original PRIVATE Python
// bytes, not from names, claimed digests or previously minted certificates.
// It opens detached SQLite in query-only mode and creates two ephemeral
// kernel-sealed source descriptors. No new causal ingress is ever performed.
func inspectRawA1H2(ctx context.Context, out, sha, run string, installed *policy.Bundle) (rawA1ProofH2, error) {
	var zero rawA1ProofH2
	if ctx == nil || installed == nil {
		return zero, errors.New("H2-R requires installed policy and context")
	}
	signed, err := extractSignedIngressH2(ctx, out, sha, run, installed)
	if err != nil {
		return zero, fmt.Errorf("H2-R requires verified durable ingress: %w", err)
	}
	runBytes, err := hex.DecodeString(run)
	if err != nil || len(runBytes) != 32 {
		return zero, errors.New("H2-R wrong RunID")
	}
	detached, cleanup, err := detachedDBH2(filepath.Join(out, "causal.db"))
	if err != nil {
		return zero, err
	}
	defer cleanup()
	db, err := readOnlyDBH2(ctx, detached)
	if err != nil {
		return zero, err
	}
	defer db.Close()
	var canonical []byte
	if err := db.QueryRowContext(ctx, "SELECT canonical FROM genesis_records WHERE run_id=?", runBytes).
		Scan(&canonical); err != nil {
		return zero, err
	}
	g, err := genesis.ValidateCanonical(canonical, installed)
	if err != nil || !bytes.Equal(g.RunID[:], runBytes) {
		return zero, errors.New("H2-R raw source Genesis not authenticated")
	}
	reg, err := installed.PhysicalRegistration()
	if err != nil || reg.Disclosure != "ENGINE_ONLY" || reg.SourceID != signed.SourceID {
		return zero, errors.New("H2-R signed archive is not from registered private producer")
	}
	outProof := rawA1ProofH2{
		Schema: "csfrv1-10h.raw-a1-private.v1", SourceSHA: sha, RunID: run,
		GenesisDigest: hex.EncodeToString(g.Digest[:]), SourceID: reg.SourceID,
		Disposition: "PRIVATE_LOCAL_RAW_A1_SIGNED_SOURCE_NO_PRODUCTION_GO",
	}
	for epoch := 1; epoch <= 2; epoch++ {
		name := func(file string) string {
			return filepath.Join(out, "physical-cuts", fmt.Sprintf("epoch-%d", epoch), file)
		}
		snapshot, err := readPrivateRegular(name("snapshot.yaml"))
		if err != nil {
			return zero, err
		}
		events, err := readPrivateRegular(name("events.jsonl"))
		if err != nil {
			return zero, err
		}
		evidence, err := readPrivateRegular(name("evidence.json"))
		if err != nil {
			return zero, err
		}
		cutRaw, err := readPrivateRegular(name("cut.cbor"))
		if err != nil {
			return zero, err
		}
		signature, err := readPrivateRegular(name("signature.bin"))
		if err != nil {
			return zero, err
		}
		if len(snapshot) > 16*1024*1024 || len(events) > 64*1024*1024 ||
			len(evidence) > 4_194_304 || len(signature) != 64 ||
			len(cutRaw) == 0 || len(cutRaw) > 4096 {
			return zero, errors.New("H2-R archived raw A1 size/signature contract violated")
		}
		cut, err := embodimentv1.DecodeSignedCut(cutRaw, signature)
		if err != nil || cut.Epoch != uint64(epoch) || cut.RunID != g.RunID ||
			cut.GenesisDigest != g.Digest {
			return zero, errors.New("H2-R signed source refers to another epoch or Genesis")
		}
		if err := embodimentv1.VerifySignedCut(cut, reg, g.RunID, g.Digest); err != nil {
			return zero, errors.New("H2-R raw source declaration not signed by registered A1 key")
		}
		if cut.SnapshotSHA256 != identity.OpaqueContentHash(snapshot) ||
			cut.EventsSHA256 != identity.OpaqueContentHash(events) ||
			cut.EvidenceSHA256 != identity.OpaqueContentHash(evidence) {
			return zero, errors.New("H2-R archived original Python source hash differs from signed cut")
		}
		snapshotFD, err := sealPrivateRawA1H2(snapshot)
		if err != nil {
			return zero, err
		}
		eventsFD, err := sealPrivateRawA1H2(events)
		if err != nil {
			_ = snapshotFD.Close()
			return zero, err
		}
		verified, inspectErr := embodimentv1.InspectSignedSealedUnadmitted(
			snapshotFD, eventsFD, evidence, canonical, installed, cut)
		_ = eventsFD.Close()
		_ = snapshotFD.Close()
		if inspectErr != nil {
			return zero, fmt.Errorf("H2-R raw A1 sealed source/A2 completion check: %w", inspectErr)
		}
		typed, err := verified.PhysicalCompletedState()
		if err != nil {
			return zero, err
		}
		derivedCanonical, err := codec.CanonicalPayload(typed)
		if err != nil {
			return zero, err
		}
		expected := signed.Epochs[epoch-1]
		if expected.Epoch != uint64(epoch) ||
			expected.CanonicalCutSHA256 != hashBytes(cutRaw) ||
			expected.SignatureSHA256 != hashBytes(signature) ||
			expected.CompletedStateHash != hex.EncodeToString(cut.CompletedStateSHA256[:]) {
			return zero, errors.New("H2-R original A1 raw source is not the one durably ingressed")
		}
		id, err := hex.DecodeString(expected.InputID)
		if err != nil || len(id) != 32 {
			return zero, errors.New("H2-R missing signed source identity")
		}
		var ledgerCanonical []byte
		if err := db.QueryRowContext(ctx,
			"SELECT canonical FROM input_ledger WHERE id=?", id).Scan(&ledgerCanonical); err != nil {
			return zero, err
		}
		value, err := codec.StrictDecodePayload(ledgerCanonical)
		if err != nil {
			return zero, err
		}
		fields, ok := value.([]any)
		if !ok || len(fields) != 13 {
			return zero, errors.New("H2-R malformed durable source")
		}
		envelope, ok := fields[7].([]any)
		if !ok || len(envelope) != 5 || envelope[0] != "physical.source-cut.v1" ||
			!bytes.Equal(bytesH2(envelope[2]), cutRaw) ||
			!bytes.Equal(bytesH2(envelope[3]), signature) {
			return zero, errors.New("H2-R raw signed cut differs from persisted input ledger")
		}
		persistedCompleted, err := codec.CanonicalPayload(envelope[4])
		if err != nil || !bytes.Equal(derivedCanonical, persistedCompleted) {
			return zero, errors.New("H2-R raw A2 completed state differs from admitted Go payload")
		}
		outProof.Epochs[epoch-1] = rawA1EpochProofH2{
			Epoch: epoch, SnapshotBytes: len(snapshot), EventsBytes: len(events),
			EvidenceBytes: len(evidence), SnapshotSHA256: hashBytes(snapshot),
			EventsSHA256: hashBytes(events), EvidenceSHA256: hashBytes(evidence),
			CutSHA256: hashBytes(cutRaw), SignatureSHA256: hashBytes(signature),
			CompletedStateSHA256: hex.EncodeToString(cut.CompletedStateSHA256[:]),
		}
	}
	return outProof, nil
}

func exportRawA1H2(ctx context.Context, out, sha, run string, installed *policy.Bundle) error {
	proof, err := inspectRawA1H2(ctx, out, sha, run, installed)
	if err != nil {
		return err
	}
	return exclusiveJSON(filepath.Join(out, rawA1ProofPathH2), proof)
}

func verifyRawA1H2(ctx context.Context, out, sha, run string, installed *policy.Bundle) error {
	raw, err := readPrivateRegular(filepath.Join(out, rawA1ProofPathH2))
	if err != nil {
		return err
	}
	var captured rawA1ProofH2
	if err := json.Unmarshal(raw, &captured); err != nil {
		return err
	}
	if !exactLocalJSON(raw, captured) {
		return errors.New("H2-R private A1 proof contains unknown, duplicate or noncanonical JSON")
	}
	reconstructed, err := inspectRawA1H2(ctx, out, sha, run, installed)
	if err != nil {
		return err
	}
	if captured != reconstructed {
		return errors.New("H2-R stored original Python source does not match independently reconstructed signed proof")
	}
	return nil
}

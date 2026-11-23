//go:build linux

// H2-S: immutable signed physical ingress provenance, strictly reconstructed
// from the authoritative causal DB. This never grants a source admission port.
package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/adapter/embodimentv1"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/genesis"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

const signedIngressProofPathH2 = "ledger-export/signed-ingress-proof.json"

type signedIngressEpochH2 struct {
	Epoch              uint64 `json:"epoch"`
	InputID            string `json:"input_id"`
	IngressSequence    uint64 `json:"ingress_sequence"`
	CanonicalCutSHA256 string `json:"canonical_cut_sha256"`
	SignatureSHA256    string `json:"signature_sha256"`
	CompletedStateHash string `json:"completed_state_hash"`
}
type signedIngressProofH2 struct {
	Schema             string                  `json:"schema"`
	SourceSHA          string                  `json:"source_sha"`
	RunID              string                  `json:"run_id"`
	GenesisDigest      string                  `json:"genesis_digest"`
	RegistrationSHA256 string                  `json:"registration_sha256"`
	SourceID           string                  `json:"source_id"`
	IngressCount       int                     `json:"ingress_count"`
	ClosureCount       int                     `json:"closure_count"`
	FinalSourceClosure bool                    `json:"final_source_closure"`
	Epochs             [2]signedIngressEpochH2 `json:"epochs"`
	Disposition        string                  `json:"disposition"`
}

// extractSignedIngressH2 is a second, deliberately independent reader. The
// signed cut CBOR and signature are already inside the canonical InputLedger
// export; this digest-only index does NOT claim to contain raw Python YAML,
// JSONL or A2 evidence files, which were not preserved by the A1 supervisor.
func extractSignedIngressH2(ctx context.Context, archive, sourceSHA, run string, installed *policy.Bundle) (signedIngressProofH2, error) {
	var zero signedIngressProofH2
	if ctx == nil || installed == nil {
		return zero, errors.New("H2-S requires policy and context")
	}
	runBytes, err := hex.DecodeString(run)
	if err != nil || len(runBytes) != 32 {
		return zero, errors.New("H2-S invalid RunID")
	}
	detached, cleanup, err := detachedDBH2(filepath.Join(archive, "causal.db"))
	if err != nil {
		return zero, err
	}
	defer cleanup()
	db, err := readOnlyDBH2(ctx, detached)
	if err != nil {
		return zero, err
	}
	defer db.Close()

	var genesisCanonical, genesisDigest []byte
	if err := db.QueryRowContext(ctx,
		"SELECT canonical,digest FROM genesis_records WHERE run_id=?", runBytes).
		Scan(&genesisCanonical, &genesisDigest); err != nil {
		return zero, err
	}
	g, err := genesis.ValidateCanonical(genesisCanonical, installed)
	if err != nil || !bytes.Equal(g.RunID[:], runBytes) || !bytes.Equal(g.Digest[:], genesisDigest) {
		return zero, errors.New("H2-S Genesis does not match installed policy and persisted digest")
	}
	registration, err := installed.PhysicalRegistration()
	if err != nil || registration.Disclosure != "ENGINE_ONLY" {
		return zero, errors.New("H2-S no private registered source")
	}
	registrationBytes, err := installed.PhysicalManifestCanonical()
	if err != nil {
		return zero, err
	}

	proof := signedIngressProofH2{
		Schema: "csfrv1-10h.signed-ingress.v1", SourceSHA: sourceSHA, RunID: run,
		GenesisDigest:      hex.EncodeToString(g.Digest[:]),
		RegistrationSHA256: hashBytes(registrationBytes),
		SourceID:           registration.SourceID,
		Disposition:        "PRIVATE_LOCAL_ONLY_NO_REINGRESS_NO_PRODUCTION_GO",
	}
	rows, err := db.QueryContext(ctx,
		"SELECT ingress_seq,record_id,canonical FROM source_ingress_receipts WHERE source_id=? ORDER BY ingress_seq",
		registration.SourceID)
	if err != nil {
		return zero, err
	}
	defer rows.Close()
	// Fully finish the source receipt cursor before making another query on
	// the single read-only SQLite connection. Nested QueryRow would deadlock.
	type receiptRow struct {
		seq           int64
		id, canonical []byte
	}
	receiptRows := make([]receiptRow, 0, 5)
	for rows.Next() {
		var entry receiptRow
		if err := rows.Scan(&entry.seq, &entry.id, &entry.canonical); err != nil {
			return zero, err
		}
		if len(receiptRows) >= 8 || len(entry.canonical) == 0 || len(entry.canonical) > 1<<20 {
			return zero, errors.New("H2-S unbounded or empty signed ingress receipt")
		}
		receiptRows = append(receiptRows, entry)
	}
	if err := rows.Err(); err != nil {
		return zero, err
	}
	if err := rows.Close(); err != nil {
		return zero, err
	}
	// Frozen source-prefix contract from C4 plus D2:
	// C4 closes physical.a1 at Genesis, due time and next actor round,
	// then D2 admits exactly two signed epochs with local closures and
	// a single final closure. These are not six physical outputs.
	expectedKinds := [...]uint64{2, 2, 2, 1, 2, 1, 2, 2}
	expectedClosure := map[uint64]struct {
		producer string
		final    bool
	}{
		0: {"physical.a1.partial", false},
		1: {"physical.a1.final", false},
		2: {"physical.a1.c4-through-next-round", false},
		4: {"physical.a1.closed.1", false},
		6: {"physical.a1.closed.2", false},
		7: {"physical.a1.final.epoch-2", true},
	}
	seenEpochs := map[uint64]bool{}
	var lastClosed bool
	for _, entry := range receiptRows {
		sequence, recordID, receiptCanonical := entry.seq, entry.id, entry.canonical
		if sequence < 0 || uint64(sequence) != uint64(proof.IngressCount) || len(recordID) != 32 || lastClosed {
			return zero, errors.New("H2-S source receipts have a gap, duplicate, or post-final write")
		}
		receiptHash := identity.OpaqueContentHash(receiptCanonical)
		var mirroredReceipt []byte
		if err := db.QueryRowContext(ctx, "SELECT canonical FROM input_ledger WHERE id=?",
			receiptHash[:]).Scan(&mirroredReceipt); err != nil || !bytes.Equal(mirroredReceipt, receiptCanonical) {
			return zero, errors.New("H2-S source ingress receipt is not mirrored in canonical InputLedger")
		}
		envelope, err := codec.StrictDecodeEnvelope(receiptCanonical)
		if err != nil || envelope.DomainTag != "cote.csf.record.source-ingress-receipt" {
			return zero, errors.New("H2-S malformed source receipt")
		}
		receipt, ok := envelope.Value.([]any)
		if !ok || len(receipt) != 3 || receipt[0] != registration.SourceID ||
			receipt[1] != uint64(sequence) {
			return zero, errors.New("H2-S receipt metadata does not match durable cursor")
		}
		ref, ok := receipt[2].([]any)
		if !ok || len(ref) != 3 || !bytes.Equal(recordID, bytesH2(ref[1])) {
			return zero, errors.New("H2-S receipt identity does not reference persisted source record")
		}
		kind, ok := ref[0].(uint64)
		if !ok || (kind != 1 && kind != 2) || len(bytesH2(ref[2])) != 32 ||
			uint64(sequence) >= uint64(len(expectedKinds)) || kind != expectedKinds[sequence] {
			return zero, errors.New("H2-S unknown, reordered or mutated registered ingress/closure")
		}
		var recordCanonical []byte
		if err := db.QueryRowContext(ctx, "SELECT canonical FROM input_ledger WHERE id=?", recordID).Scan(&recordCanonical); err != nil {
			return zero, fmt.Errorf("H2-S source receipt record absent: %w", err)
		}
		value, err := codec.StrictDecodePayload(recordCanonical)
		if err != nil {
			return zero, err
		}
		fields, ok := value.([]any)
		if !ok || len(fields) < 7 || !bytes.Equal(bytesH2(fields[0]), recordID) ||
			!bytes.Equal(bytesH2(fields[1]), runBytes) || fields[2] != registration.SourceID {
			return zero, errors.New("H2-S source record identity/run/source mismatch")
		}
		if kind == 1 {
			if len(fields) != 13 || fields[4] != registration.ActionType ||
				fields[5] != uint64(registration.ActionVersion) {
				return zero, errors.New("H2-S physical input shape invalid")
			}
			wantID, _, err := identity.New(installed).Sum("cote.csf.id.exogenous-input",
				"cote.csf.schema.exogenous-input-id-preimage", 1, []any{fields[1], fields[2], fields[3]})
			if err != nil || !bytes.Equal(recordID, wantID[:]) {
				return zero, errors.New("H2-S exogenous input identity invalid")
			}
			digest, _, err := identity.New(installed).Sum("cote.csf.digest.exogenous-input-unit",
				"cote.csf.schema.exogenous-input.body", 1, fields[:12])
			if err != nil || !bytes.Equal(bytesH2(fields[12]), digest[:]) ||
				!bytes.Equal(bytesH2(ref[2]), digest[:]) {
				return zero, errors.New("H2-S exogenous input or receipt digest invalid")
			}
			source, ok := fields[7].([]any)
			if !ok || len(source) != 5 || source[0] != "physical.source-cut.v1" ||
				!bytes.Equal(bytesH2(source[1]), registrationBytes) {
				return zero, errors.New("H2-S physical cut manifest not installed or incomplete")
			}
			cutBytes, sig := bytesH2(source[2]), bytesH2(source[3])
			completed, ok := source[4].([]any)
			if !ok {
				return zero, errors.New("H2-S physical completed body absent")
			}
			signed, err := embodimentv1.DecodeSignedCut(cutBytes, sig)
			if err != nil || signed.Epoch < 1 || signed.Epoch > 2 || seenEpochs[signed.Epoch] ||
				fields[3] != fmt.Sprintf("physical.a1.epoch.%d", signed.Epoch) {
				return zero, errors.New("H2-S signed source epoch invalid or duplicated")
			}
			if err := embodimentv1.VerifySignedCut(signed, registration, g.RunID, g.Digest); err != nil {
				return zero, errors.New("H2-S Genesis-bound Ed25519 signature invalid")
			}
			recovered, err := embodimentv1.RecoverApprovedCutFromDurableIngress(
				cutBytes, sig, completed, registration, g.RunID, g.Digest)
			if err != nil || !recovered.IsDurablyRecovered() {
				return zero, errors.New("H2-S invalid completed source or illegal new ingress capability")
			}
			if proof.Epochs[signed.Epoch-1].Epoch != 0 {
				return zero, errors.New("H2-S duplicate epoch")
			}
			proof.Epochs[signed.Epoch-1] = signedIngressEpochH2{
				Epoch: signed.Epoch, InputID: hex.EncodeToString(recordID),
				IngressSequence: uint64(sequence), CanonicalCutSHA256: hashBytes(cutBytes),
				SignatureSHA256: hashBytes(sig), CompletedStateHash: hex.EncodeToString(signed.CompletedStateSHA256[:]),
			}
			seenEpochs[signed.Epoch] = true
		} else {
			if len(fields) != 7 || fields[5] == nil {
				return zero, errors.New("H2-S malformed source closure")
			}
			final, ok := fields[5].(bool)
			if !ok {
				return zero, errors.New("H2-S untyped final source closure")
			}
			expected, present := expectedClosure[uint64(sequence)]
			if !present || fields[3] != expected.producer || final != expected.final {
				return zero, errors.New("H2-S physical source closure producer/order/finalness differs from frozen C4/D2 contract")
			}
			wantID, _, err := identity.New(installed).Sum("cote.csf.id.source-closure",
				"cote.csf.schema.source-closure-id-preimage", 1, []any{fields[1], fields[2], fields[3]})
			if err != nil || !bytes.Equal(recordID, wantID[:]) {
				return zero, errors.New("H2-S invalid source closure identity")
			}
			digest, _, err := identity.New(installed).Sum("cote.csf.digest.source-closure",
				"cote.csf.schema.source-closure", 1, fields)
			if err != nil || !bytes.Equal(digest[:], bytesH2(ref[2])) {
				return zero, errors.New("H2-S invalid closure receipt digest")
			}
			if final {
				lastClosed = true
			}
			proof.ClosureCount++
		}
		proof.IngressCount++
	}
	if proof.IngressCount != 8 || proof.ClosureCount != 6 || !lastClosed ||
		!seenEpochs[1] || !seenEpochs[2] || proof.Epochs[0].Epoch != 1 || proof.Epochs[1].Epoch != 2 {
		return zero, fmt.Errorf("H2-S incomplete authenticated physical prefix: receipts=%d closures=%d final=%t epoch1=%t epoch2=%t", proof.IngressCount, proof.ClosureCount, lastClosed, seenEpochs[1], seenEpochs[2])
	}
	proof.FinalSourceClosure = true
	return proof, nil
}

// bytesH2 enforces explicit byte-array typing. No string/JSON fallback.
func bytesH2(value any) []byte { raw, _ := value.([]byte); return raw }

func exportSignedIngressH2(ctx context.Context, out, sha, run string, bundle *policy.Bundle) error {
	proof, err := extractSignedIngressH2(ctx, out, sha, run, bundle)
	if err != nil {
		return err
	}
	return exclusiveJSON(filepath.Join(out, signedIngressProofPathH2), proof)
}
func verifySignedIngressH2(ctx context.Context, out, sha, run string, bundle *policy.Bundle) error {
	raw, err := readPrivateRegular(filepath.Join(out, signedIngressProofPathH2))
	if err != nil {
		return err
	}
	var stored signedIngressProofH2
	if err := json.Unmarshal(raw, &stored); err != nil {
		return err
	}
	if !exactLocalJSON(raw, stored) {
		return errors.New("H2-S noncanonical signed-ingress proof")
	}
	verified, err := extractSignedIngressH2(ctx, out, sha, run, bundle)
	if err != nil {
		return err
	}
	if stored != verified {
		return errors.New("H2-S signed-ingress proof differs from cryptographically verified source receipts")
	}
	return nil
}

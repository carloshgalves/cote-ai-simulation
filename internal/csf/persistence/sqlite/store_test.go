package sqlite

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/genesis"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
)

func TestInputIngressAndSlotResponseAreAtomicAndConditional(t *testing.T) {
	store, err := Open(context.Background(), t.TempDir()+"/ledger.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var inputID, receiptID, slotKey, responseA, responseB [32]byte
	inputID[0] = 1
	receiptID[0] = 2
	slotKey[0] = 3
	responseA[0] = 4
	responseB[0] = 5
	input := ports.CanonicalRecord{Identity: inputID, Canonical: []byte{1, 2, 3}}
	receipt := ports.CanonicalRecord{Identity: receiptID, Canonical: []byte{4, 5, 6}}
	if err := store.AppendIngress(context.Background(), "source-a", 0, input, receipt); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendIngress(context.Background(), "source-a", 2, ports.CanonicalRecord{Identity: responseA, Canonical: []byte{7}}, receipt); err == nil {
		t.Fatal("accepted a gap in source ingress")
	}
	var next uint64
	if err := store.db.QueryRow(`SELECT next_seq FROM input_source_cursors WHERE source_id='source-a'`).Scan(&next); err != nil {
		t.Fatal(err)
	}
	if next != 1 {
		t.Fatalf("failed append advanced cursor to %d", next)
	}
	first := ports.CanonicalRecord{Identity: responseA, Canonical: []byte{7, 8}}
	inserted, existing, err := store.AppendIfAbsent(context.Background(), slotKey, first)
	if err != nil || !inserted || existing != nil {
		t.Fatalf("first slot CAS = %v %x %v", inserted, existing, err)
	}
	second := ports.CanonicalRecord{Identity: responseB, Canonical: []byte{9, 10}}
	inserted, existing, err = store.AppendIfAbsent(context.Background(), slotKey, second)
	if err != nil || inserted || !bytes.Equal(existing, first.Canonical) {
		t.Fatalf("competing slot CAS = %v %x %v", inserted, existing, err)
	}
	if _, err := store.Get(context.Background(), responseB); err == nil {
		t.Fatal("losing response was persisted")
	}
}

func TestOpenPinsSQLiteIdentityAndPragmas(t *testing.T) {
	store, err := Open(context.Background(), t.TempDir()+"/run.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var journal string
	var synchronous int
	if err := store.db.QueryRow("PRAGMA journal_mode").Scan(&journal); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow("PRAGMA synchronous").Scan(&synchronous); err != nil {
		t.Fatal(err)
	}
	if journal != "wal" || synchronous != 2 {
		t.Fatalf("unsafe SQLite configuration journal=%s synchronous=%d", journal, synchronous)
	}
}

func TestGenesisRoundTripsByteForByteAndCollisionFails(t *testing.T) {
	const bundlePath = "../../../../docs/architecture/canonical-codec-v1-bundle"
	bundle, err := policy.Load(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(bundlePath + "/fixtures.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures struct {
		Semantic []struct {
			CaseID             string `json:"case_id"`
			GenesisEnvelopeHex string `json:"genesis_envelope_hex"`
		} `json:"semantic"`
	}
	if err := json.Unmarshal(raw, &fixtures); err != nil {
		t.Fatal(err)
	}
	var encoded []byte
	for _, item := range fixtures.Semantic {
		if item.CaseID == "genesis.pinning" {
			encoded, err = hex.DecodeString(item.GenesisEnvelopeHex)
			if err != nil {
				t.Fatal(err)
			}
			envelope, err := codec.StrictDecodeEnvelope(encoded)
			if err != nil {
				t.Fatal(err)
			}
			payload := envelope.Value.([]any)
			envelope.DomainTag = "cote.csf.digest.genesis-manifest"
			payload[15] = []any{[]any{"cote.csf.schema.snapshot-pending-state", uint64(1)}}
			state := make([]any, 10)
			for i := range state {
				state[i] = []any{}
			}
			stateEnvelope, err := codec.CanonicalBytes("cote.csf.snapshot.pending-state", "cote.csf.schema.snapshot-pending-state", 1, state)
			if err != nil {
				t.Fatal(err)
			}
			digest := identity.OpaqueContentHash(stateEnvelope)
			payload[16] = []any{"cote.csf.schema.snapshot-pending-state", uint64(1), stateEnvelope, digest[:]}
			encoded, err = codec.CanonicalBytes(envelope.DomainTag, envelope.SchemaID, envelope.SchemaVersion, payload)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	if len(encoded) == 0 {
		t.Fatal("genesis fixture missing")
	}
	manifest, err := genesis.ValidateCanonical(encoded, bundle)
	if err != nil {
		t.Fatal(err)
	}
	store, err := Open(context.Background(), t.TempDir()+"/run.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.PersistGenesis(context.Background(), manifest, bundle); err != nil {
		t.Fatal(err)
	}
	if err := store.PersistGenesis(context.Background(), manifest, bundle); err != nil {
		t.Fatal(err)
	}
	stored, err := store.LoadGenesis(context.Background(), manifest.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if hex.EncodeToString(stored) != hex.EncodeToString(encoded) {
		t.Fatal("genesis did not round-trip byte for byte")
	}
	corrupt := *manifest
	corrupt.CanonicalBytes = append([]byte(nil), manifest.CanonicalBytes...)
	corrupt.CanonicalBytes[len(corrupt.CanonicalBytes)-1] ^= 1
	if err := store.PersistGenesis(context.Background(), &corrupt, bundle); err == nil {
		t.Fatal("accepted same run identity with divergent genesis bytes")
	}

	for _, scenario := range []struct {
		name   string
		mutate func(*genesis.Genesis)
	}{
		{"noncanonical-first-write", func(v *genesis.Genesis) { v.CanonicalBytes = []byte{0xff} }},
		{"run-id-mismatch", func(v *genesis.Genesis) { v.RunID[0] ^= 1 }},
		{"genesis-id-mismatch", func(v *genesis.Genesis) { v.ID[0] ^= 1 }},
		{"digest-mismatch", func(v *genesis.Genesis) { v.Digest[0] ^= 1 }},
		{"payload-mismatch", func(v *genesis.Genesis) { v.Payload = []any{} }},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fresh, err := Open(context.Background(), t.TempDir()+"/untrusted.db")
			if err != nil {
				t.Fatal(err)
			}
			defer fresh.Close()
			bad := *manifest
			scenario.mutate(&bad)
			if err := fresh.PersistGenesis(context.Background(), &bad, bundle); err == nil {
				t.Fatal("accepted invalid genesis on first insertion")
			}
			var count int
			if err := fresh.db.QueryRow("SELECT COUNT(*) FROM genesis_records").Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 0 {
				t.Fatal("invalid genesis was persisted")
			}
		})
	}
}

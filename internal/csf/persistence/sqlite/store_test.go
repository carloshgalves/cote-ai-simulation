package sqlite

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/genesis"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

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
	if err := store.PersistGenesis(context.Background(), manifest); err != nil {
		t.Fatal(err)
	}
	if err := store.PersistGenesis(context.Background(), manifest); err != nil {
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
	if err := store.PersistGenesis(context.Background(), &corrupt); err == nil {
		t.Fatal("accepted same run identity with divergent genesis bytes")
	}
}

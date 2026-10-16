package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
)

const testBundle = "../../docs/architecture/canonical-codec-v1-bundle"

func validEmptyGenesis(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(testBundle, "fixtures.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixtures struct {
		Semantic []struct {
			CaseID     string `json:"case_id"`
			GenesisHex string `json:"genesis_envelope_hex"`
		} `json:"semantic"`
	}
	if err := json.Unmarshal(raw, &fixtures); err != nil {
		t.Fatal(err)
	}
	for _, item := range fixtures.Semantic {
		if item.CaseID != "genesis.pinning" {
			continue
		}
		encoded, err := hex.DecodeString(item.GenesisHex)
		if err != nil {
			t.Fatal(err)
		}
		envelope, err := codec.StrictDecodeEnvelope(encoded)
		if err != nil {
			t.Fatal(err)
		}
		payload := envelope.Value.([]any)
		state := make([]any, 10)
		for index := range state {
			state[index] = []any{}
		}
		stateEnvelope, err := codec.CanonicalBytes(
			"cote.csf.snapshot.pending-state", "cote.csf.schema.snapshot-pending-state", 1, state,
		)
		if err != nil {
			t.Fatal(err)
		}
		digest := identity.OpaqueContentHash(stateEnvelope)
		payload[15] = []any{[]any{"cote.csf.schema.snapshot-pending-state", uint64(1)}}
		payload[16] = []any{"cote.csf.schema.snapshot-pending-state", uint64(1), stateEnvelope, digest[:]}
		result, err := codec.CanonicalBytes(
			"cote.csf.digest.genesis-manifest", "cote.csf.schema.genesis-manifest", 1, payload,
		)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	t.Fatal("genesis fixture not found")
	return nil
}

func TestCreateRunRejectsUnseededOccurrencesBeforeOpeningDatabase(t *testing.T) {
	for _, test := range []struct {
		name string
		kind uint64
	}{
		{name: "wrong-kind", kind: 17},
		{name: "right-kind-orphan", kind: 6},
	} {
		t.Run(test.name, func(t *testing.T) {
			envelope, err := codec.StrictDecodeEnvelope(validEmptyGenesis(t))
			if err != nil {
				t.Fatal(err)
			}
			payload := envelope.Value.([]any)
			typed := payload[16].([]any)
			inner, err := codec.StrictDecodeEnvelope(typed[2].([]byte))
			if err != nil {
				t.Fatal(err)
			}
			state := inner.Value.([]any)
			state[0] = []any{[]any{test.kind, bytes.Repeat([]byte{0x11}, 32), bytes.Repeat([]byte{0x22}, 32)}}
			innerBytes, err := codec.CanonicalBytes(inner.DomainTag, inner.SchemaID, inner.SchemaVersion, state)
			if err != nil {
				t.Fatal(err)
			}
			digest := identity.OpaqueContentHash(innerBytes)
			payload[16] = []any{typed[0], typed[1], innerBytes, digest[:]}
			corrupt, err := codec.CanonicalBytes(envelope.DomainTag, envelope.SchemaID, envelope.SchemaVersion, payload)
			if err != nil {
				t.Fatal(err)
			}
			dir := t.TempDir()
			genesisPath := filepath.Join(dir, "bad.cbor")
			dbPath := filepath.Join(dir, "run.db")
			if err := os.WriteFile(genesisPath, corrupt, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := createRun(context.Background(), testBundle, genesisPath, dbPath); err == nil {
				t.Fatal("created a run whose pending occurrence has no authoritative record")
			}
			if _, err := os.Stat(dbPath); !os.IsNotExist(err) {
				t.Fatalf("invalid genesis must not create a database, stat error = %v", err)
			}
		})
	}
}

func TestCreateRunRejectsUnverifiedExternalDependenciesBeforeOpeningDatabase(t *testing.T) {
	framedParent, err := hex.DecodeString("435346485245460001001a636f74652e6373662e636f6465632e63626f722d6465742e76310020000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f0016636f74652e6373662e686173682e736e617073686f740018636f74652e6373662e736368656d612e736e617073686f74000000000000000100077368612d3235360020000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f")
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name   string
		mutate func([]any)
	}{
		{
			name: "unloaded-extension-bundle",
			mutate: func(payload []any) {
				payload[12] = []any{[]any{"missing-extension", bytes.Repeat([]byte{0x5a}, 32)}}
			},
		},
		{
			name: "malformed-parent-checkpoint",
			mutate: func(payload []any) {
				payload[4] = []any{uint64(1), []byte{0x00}}
			},
		},
		{
			name: "well-framed-but-unresolved-parent-checkpoint",
			mutate: func(payload []any) {
				payload[4] = []any{uint64(1), framedParent}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			envelope, err := codec.StrictDecodeEnvelope(validEmptyGenesis(t))
			if err != nil {
				t.Fatal(err)
			}
			payload := envelope.Value.([]any)
			test.mutate(payload)
			encoded, err := codec.CanonicalBytes(envelope.DomainTag, envelope.SchemaID, envelope.SchemaVersion, payload)
			if err != nil {
				t.Fatal(err)
			}
			dir := t.TempDir()
			genesisPath := filepath.Join(dir, "bad.cbor")
			dbPath := filepath.Join(dir, "run.db")
			if err := os.WriteFile(genesisPath, encoded, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := createRun(context.Background(), testBundle, genesisPath, dbPath); err == nil {
				t.Fatal("created a run with unresolved external genesis dependencies")
			}
			if _, err := os.Stat(dbPath); !os.IsNotExist(err) {
				t.Fatalf("invalid genesis must not create a database, stat error = %v", err)
			}
		})
	}
}

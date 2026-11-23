//go:build linux

package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	_ "modernc.org/sqlite"
)

// This adversarial test is run with H2_REAL_PRIVATE_ARCHIVE after the real
// authenticated A1/Go D3 CI job, never against mock fixtures. All mutations
// occur in isolated private copies; the original archive remains untouched.
func TestH2RejectsRehashedSemanticCorruption(t *testing.T) {
	original := os.Getenv("H2_REAL_PRIVATE_ARCHIVE")
	if original == "" {
		t.Skip("requires actual private A1/D3 archive generated in this CI job")
	}
	sources, err := collectedPaths(original)
	if err != nil {
		t.Fatal(err)
	}
	previous := syscall.Umask(0077)
	defer syscall.Umask(previous)
	for _, variant := range []string{"owner-plaintext", "sealed-snapshot", "framed-ledger", "signed-ingress-proof", "missing-signed-ingress-proof"} {
		t.Run(variant, func(t *testing.T) {
			out := t.TempDir()
			if err := os.Chmod(out, 0700); err != nil {
				t.Fatal(err)
			}
			for _, subdir := range []string{"owner", "ledger-export"} {
				if err := os.Mkdir(filepath.Join(out, subdir), 0700); err != nil {
					t.Fatal(err)
				}
			}
			for _, name := range sources {
				data, err := readPrivateRegular(filepath.Join(original, filepath.FromSlash(name)))
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(out, filepath.FromSlash(name)), data, 0600); err != nil {
					t.Fatal(err)
				}
			}
			switch variant {
			case "owner-plaintext":
				path := filepath.Join(out, "owner", "actor-b.db")
				db, err := sql.Open("sqlite", path)
				if err != nil {
					t.Fatal(err)
				}
				res, e := db.Exec("UPDATE checkpoints SET content=x'00' WHERE locator=(SELECT locator FROM checkpoints ORDER BY locator LIMIT 1)")
				closeErr := db.Close()
				if e != nil || closeErr != nil {
					t.Fatalf("owner mutation failed: %v / %v", e, closeErr)
				}
				n, e := res.RowsAffected()
				if e != nil || n != 1 {
					t.Fatalf("owner mutation did not exercise real row: %d / %v", n, e)
				}
			case "sealed-snapshot":
				path := filepath.Join(out, "causal.db")
				db, err := sql.Open("sqlite", path)
				if err != nil {
					t.Fatal(err)
				}
				res, e := db.Exec("UPDATE snapshot_seals SET digest=zeroblob(32) WHERE revision=6")
				closeErr := db.Close()
				if e != nil || closeErr != nil {
					t.Fatalf("snapshot mutation failed: %v / %v", e, closeErr)
				}
				n, e := res.RowsAffected()
				if e != nil || n != 1 {
					t.Fatalf("snapshot mutation did not exercise real row: %d / %v", n, e)
				}
			case "framed-ledger":
				path := filepath.Join(out, "ledger-export", "input_ledger.bin")
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				data[len(data)-1] ^= 0xff
				if err := os.WriteFile(path, data, 0600); err != nil {
					t.Fatal(err)
				}
			case "signed-ingress-proof":
				path := filepath.Join(out, filepath.FromSlash(signedIngressProofPathH2))
				data, err := os.ReadFile(path)
				if err != nil { t.Fatal(err) }
				var proof signedIngressProofH2
				if err := json.Unmarshal(data, &proof); err != nil { t.Fatal(err) }
				if len(proof.Epochs[0].CanonicalCutSHA256) != 64 {
					t.Fatal("real signed ingress evidence absent")
				}
				proof.Epochs[0].CanonicalCutSHA256 = strings.Repeat("0",64)
				mutated, err := json.MarshalIndent(proof,"","  ")
				if err != nil { t.Fatal(err) }
				if err := os.WriteFile(path,append(mutated,'\n'),0600); err != nil { t.Fatal(err) }
			case "missing-signed-ingress-proof":
				if err := os.Remove(filepath.Join(out,filepath.FromSlash(signedIngressProofPathH2))); err != nil { t.Fatal(err) }
			}
			// Forge a new fully consistent *local SHA256 index* to prove that
			// private record signatures/snapshot seals/full byte replay are
			// checked beyond a self-attested archive hash.
			paths, err := collectedPaths(out)
			if err != nil {
				t.Fatal(err)
			}
			var index archiveIndex
			index.Schema = "csfrv1-10h.local-hashes.v1"
			for _, name := range paths {
				data, err := readPrivateRegular(filepath.Join(out, filepath.FromSlash(name)))
				if err != nil {
					t.Fatal(err)
				}
				index.Files = append(index.Files, fileRecord{
					Path: name, Size: int64(len(data)), SHA256: hashBytes(data),
				})
			}
			if err := exclusiveJSON(filepath.Join(out, "checksums.json"), index); err != nil {
				t.Fatal(err)
			}
			err = verifyLocal(context.Background(), out)
			if err == nil {
				t.Fatal("rehashed but cryptographically altered H2 archive accepted")
			}
			if strings.Contains(err.Error(), "evidence checksum mismatch") ||
				errors.Is(err, os.ErrNotExist) {
				t.Fatalf("mutation only tested local file hash/absence, not independent semantics: %v", err)
			}
			required := map[string]string{
				"owner-plaintext": "H2 independent signed owner verification",
				"sealed-snapshot": "H2 full canonical ledger verification",
				"framed-ledger":   "H2 full canonical ledger verification",
				"signed-ingress-proof": "H2-S source receipt and Genesis-bound signature audit",
				"missing-signed-ingress-proof": "H2-S signed ingress profile does not match evidence",
			}[variant]
			if !strings.Contains(err.Error(), required) {
				t.Fatalf("unexpected rejection path, not target cryptographic invariant %s: %v", required, err)
			}
			t.Logf("SEMANTIC_H2_MUTANT_REJECTED=%s", variant)
		})
	}
}

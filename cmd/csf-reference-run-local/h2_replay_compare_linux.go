//go:build linux

package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

type replayTableH2 struct {
	DDL                   string `json:"ddl_sha256"`
	Columns               string `json:"columns_sha256"`
	Rows                  int    `json:"rows"`
	OrderedMultisetSHA256 string `json:"ordered_multiset_sha256"`
}
type replayComparisonH2 struct {
	Schema               string                   `json:"schema"`
	SourceSHA            string                   `json:"source_sha"`
	RunID                string                   `json:"run_id"`
	GenesisSHA256        string                   `json:"genesis_sha256"`
	InputsIdentical      bool                     `json:"same_original_signed_a1_cuts"`
	IndependentCausalDBs bool                     `json:"fresh_independent_causal_databases"`
	OriginalTables       map[string]replayTableH2 `json:"original_tables"`
	ReplayTables         map[string]replayTableH2 `json:"replay_tables"`
	OwnerA               map[string]replayTableH2 `json:"owner_a"`
	ReplayOwnerA         map[string]replayTableH2 `json:"replay_owner_a"`
	OwnerB               map[string]replayTableH2 `json:"owner_b"`
	ReplayOwnerB         map[string]replayTableH2 `json:"replay_owner_b"`
	Disposition          string                   `json:"disposition"`
}

var replayTableNameH2 = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

// snapshotTableSetH2 compares the entire logical SQLite authority, not the
// binary SQLite container (freelist/WAL page order is not a causal contract).
// Every table, its exact DDL, column order and every SQLite-typed cell is
// included in a sorted row MULTISET digest, preserving duplicate rows.
func snapshotTableSetH2(ctx context.Context, db *sql.DB) (map[string]replayTableH2, error) {
	rows, err := db.QueryContext(ctx, "SELECT name,COALESCE(sql,'') FROM sqlite_master WHERE type='table' ORDER BY name")
	if err != nil {
		return nil, err
	}
	type table struct{ name, ddl string }
	var names []table
	for rows.Next() {
		var t table
		if err := rows.Scan(&t.name, &t.ddl); err != nil {
			rows.Close()
			return nil, err
		}
		if !replayTableNameH2.MatchString(t.name) || len(names) >= 160 {
			rows.Close()
			return nil, errors.New("H2 replay has illegal or excessive database tables")
		}
		names = append(names, t)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if len(names) == 0 {
		return nil, errors.New("H2 replay has no authoritative tables")
	}
	result := make(map[string]replayTableH2, len(names))
	// The inventory is operator-private but could still be compromised.
	// Budgets are GLOBAL, not only per table, to reject memory exhaustion.
	totalRows, totalSourceBytes := 0, 0
	for _, t := range names {
		// The table identifier was checked above, before interpolating SQL.
		r, err := db.QueryContext(ctx, "SELECT * FROM \""+t.name+"\"")
		if err != nil {
			return nil, fmt.Errorf("H2 table %s cannot be read: %w", t.name, err)
		}
		cols, err := r.Columns()
		if err != nil {
			r.Close()
			return nil, err
		}
		if len(cols) == 0 || len(cols) > 100 || len(stringsJoinH2(cols)) > 8192 {
			r.Close()
			return nil, errors.New("H2 invalid table column schema")
		}
		colBytes, _ := json.Marshal(cols)
		seen := map[string]bool{}
		for _, col := range cols {
			if !replayTableNameH2.MatchString(col) || seen[col] {
				r.Close()
				return nil, errors.New("H2 duplicate/unsafe SQLite column")
			}
			seen[col] = true
		}
		// Statically tagged values avoid JSON's float/int and bytes/text
		// ambiguities; the digest is NOT a normative CSF ledger digest.
		individual := make([]string, 0)
		for r.Next() {
			totalRows++
			if len(individual) >= 100000 || totalRows > 250000 {
				r.Close()
				return nil, errors.New("H2 table/global row ceiling")
			}
			raw := make([]any, len(cols))
			scans := make([]any, len(cols))
			for i := range raw {
				scans[i] = &raw[i]
			}
			if err := r.Scan(scans...); err != nil {
				r.Close()
				return nil, err
			}
			// Bound aggregate binary/text lengths BEFORE hex/JSON encoding.
			// A per-cell ceiling alone allows a large multi-column row to
			// amplify memory use. Never silently skip an oversized row.
			rowSourceBytes := 0
			for _, cell := range raw {
				switch v := cell.(type) {
				case []byte:
					rowSourceBytes += len(v)
				case string:
					rowSourceBytes += len(v)
				}
				if rowSourceBytes > 8<<20 {
					r.Close()
					return nil, errors.New("H2 oversized aggregate private row")
				}
			}
			totalSourceBytes += rowSourceBytes
			if totalSourceBytes > 128<<20 {
				r.Close()
				return nil, errors.New("H2 all-table private source byte ceiling")
			}
			cells := make([][2]string, len(raw))
			for i, cell := range raw {
				switch v := cell.(type) {
				case nil:
					cells[i] = [2]string{"null", ""}
				case []byte:
					if len(v) > 16<<20 {
						r.Close()
						return nil, errors.New("H2 oversized private cell")
					}
					cells[i] = [2]string{"blob", hex.EncodeToString(v)}
				case string:
					cells[i] = [2]string{"text", v}
				case int64:
					cells[i] = [2]string{"int", strconv.FormatInt(v, 10)}
				case float64:
					cells[i] = [2]string{"real", strconv.FormatUint(mathFloatH2(v), 16)}
				case bool:
					if v {
						cells[i] = [2]string{"bool", "1"}
					} else {
						cells[i] = [2]string{"bool", "0"}
					}
				default:
					r.Close()
					return nil, errors.New("H2 SQLite cell has unsupported type")
				}
			}
			rowBytes, err := json.Marshal(cells)
			if err != nil {
				r.Close()
				return nil, err
			}
			if len(rowBytes) > 32<<20 {
				r.Close()
				return nil, errors.New("H2 oversized private row")
			}
			digest := sha256.Sum256(rowBytes)
			individual = append(individual, hex.EncodeToString(digest[:]))
		}
		if err := r.Err(); err != nil {
			r.Close()
			return nil, err
		}
		if err := r.Close(); err != nil {
			return nil, err
		}
		sort.Strings(individual)
		canonical, err := json.Marshal([]any{"CSF-H2-SAME-CUT-TABLE-V1", t.name, t.ddl, cols, individual})
		if err != nil {
			return nil, err
		}
		result[t.name] = replayTableH2{
			DDL: hashBytes([]byte(t.ddl)), Columns: hashBytes(colBytes),
			Rows: len(individual), OrderedMultisetSHA256: hashBytes(canonical),
		}
	}
	// Also compare the complete SQLite schema registry: indexes, triggers
	// and views are authorities even if they own no persisted table rows.
	metadata, err := db.QueryContext(ctx,
		"SELECT type,name,tbl_name,COALESCE(sql,'') FROM sqlite_master ORDER BY type,name")
	if err != nil {
		return nil, err
	}
	objects := make([][4]string, 0)
	for metadata.Next() {
		if len(objects) >= 500 {
			metadata.Close()
			return nil, errors.New("H2 excessive SQLite schema objects")
		}
		var record [4]string
		if err := metadata.Scan(&record[0], &record[1], &record[2], &record[3]); err != nil {
			metadata.Close()
			return nil, err
		}
		objects = append(objects, record)
	}
	if err := metadata.Err(); err != nil {
		metadata.Close()
		return nil, err
	}
	if err := metadata.Close(); err != nil {
		return nil, err
	}
	rawObjects, err := json.Marshal([]any{"H2-SQLITE-FULL-SCHEMA-REGISTRY-V1", objects})
	if err != nil {
		return nil, err
	}
	if _, ok := result["__sqlite_schema_registry__"]; ok {
		return nil, errors.New("H2 collision with reserved schema registry evidence name")
	}
	result["__sqlite_schema_registry__"] = replayTableH2{
		DDL: hashBytes(rawObjects), Columns: hashBytes([]byte("type,name,tbl_name,sql")),
		Rows: len(objects), OrderedMultisetSHA256: hashBytes(rawObjects),
	}
	return result, nil
}

// local pure helpers avoid changing package-global SQLite behavior.
func stringsJoinH2(s []string) string {
	out := ""
	for _, v := range s {
		out += v + "\x00"
	}
	return out
}
func mathFloatH2(v float64) uint64 { return math.Float64bits(v) }

func replayDbTableSetH2(ctx context.Context, dbPath string) (map[string]replayTableH2, error) {
	copyPath, cleanup, err := detachedDBH2(dbPath)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	db, err := readOnlyDBH2(ctx, copyPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	return snapshotTableSetH2(ctx, db)
}

func compareSameSignedSourcesH2(ctx context.Context, out, sha, run string, installed *policy.Bundle) (replayComparisonH2, error) {
	var zero replayComparisonH2
	if installed == nil {
		return zero, errors.New("H2 same-cut replay needs installed policy")
	}
	raw, err := inspectRawA1H2(ctx, out, sha, run, installed)
	if err != nil {
		return zero, err
	}
	signed, err := extractSignedIngressH2(ctx, out, sha, run, installed)
	if err != nil {
		return zero, err
	}
	for i := range raw.Epochs {
		if raw.Epochs[i].CutSHA256 != signed.Epochs[i].CanonicalCutSHA256 ||
			raw.Epochs[i].SignatureSHA256 != signed.Epochs[i].SignatureSHA256 {
			return zero, errors.New("H2 replay raw original cuts diverge from first ingress")
		}
	}
	originalGenesis, err := replayAndGenesis(ctx, filepath.Join(out, "causal.db"), installed, run)
	if err != nil {
		return zero, err
	}
	replayGenesis, err := replayAndGenesis(ctx, filepath.Join(out, "replay", "causal.db"), installed, run)
	if err != nil || originalGenesis != replayGenesis {
		return zero, errors.New("H2 replay Genesis/reconstructed clocks differ")
	}
	for _, root := range []string{out, filepath.Join(out, "replay")} {
		if _, _, _, err := verifiedSnapshotH2(ctx, filepath.Join(root, "causal.db"), run, installed); err != nil {
			return zero, fmt.Errorf("H2 replay sealed C4 snapshot: %w", err)
		}
		if _, err := auditOwnersH2(ctx, root, sha, run); err != nil {
			return zero, fmt.Errorf("H2 replay signed private owner receipts: %w", err)
		}
	}
	left := filepath.Join(out, "causal.db")
	right := filepath.Join(out, "replay", "causal.db")
	original, err := replayDbTableSetH2(ctx, left)
	if err != nil {
		return zero, err
	}
	secondary, err := replayDbTableSetH2(ctx, right)
	if err != nil {
		return zero, err
	}
	if !reflect.DeepEqual(original, secondary) {
		return zero, errors.New("H2 same-cut replay DIFFERENT causal table count/schema/typed rows")
	}
	ownerPaths := []string{"owner/actor-b.db.actor-a", "owner/actor-b.db"}
	var owners [2]map[string]replayTableH2
	var replayOwners [2]map[string]replayTableH2
	for i, rel := range ownerPaths {
		owners[i], err = replayDbTableSetH2(ctx, filepath.Join(out, filepath.FromSlash(rel)))
		if err != nil {
			return zero, err
		}
		replayOwners[i], err = replayDbTableSetH2(ctx, filepath.Join(out, "replay", filepath.FromSlash(rel)))
		if err != nil {
			return zero, err
		}
		if !reflect.DeepEqual(owners[i], replayOwners[i]) {
			return zero, fmt.Errorf("H2 same-cut replay DIFFERENT private owner %d canonical table rows", i+1)
		}
	}
	if len(original) < 12 || len(owners[0]) < 1 || len(owners[1]) < 1 {
		return zero, errors.New("H2 replay does not contain a meaningful authoritative database")
	}
	return replayComparisonH2{
		Schema:    "csfrv1-10h.same-signed-source-independent-replay.v1",
		SourceSHA: sha, RunID: run, GenesisSHA256: originalGenesis,
		InputsIdentical: true, IndependentCausalDBs: true,
		OriginalTables: original, ReplayTables: secondary,
		OwnerA: owners[0], ReplayOwnerA: replayOwners[0],
		OwnerB: owners[1], ReplayOwnerB: replayOwners[1],
		Disposition: "PRIVATE_LOCAL_ONLY_NO_PRODUCTION_GO_NO_INDEPENDENT_AUDITOR",
	}, nil
}

func verifySameCutReplayH2(ctx context.Context, out, sha, run string, installed *policy.Bundle) error {
	raw, err := readPrivateRegular(filepath.Join(out, "replay", "compare.json"))
	if err != nil {
		return err
	}
	var claimed replayComparisonH2
	if err := json.Unmarshal(raw, &claimed); err != nil {
		return err
	}
	if !exactLocalJSON(raw, claimed) {
		return errors.New("H2 same-cut replay report has extra/duplicate or noncanonical fields")
	}
	real, err := compareSameSignedSourcesH2(ctx, out, sha, run, installed)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(claimed, real) {
		return errors.New("H2 same-cut replay comparison report differs from independently reconstructed DBs")
	}
	return nil
}

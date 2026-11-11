package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// forkTableClass distinguishes causal authorities from SQLite-owned metadata.
// Metadata is not a child-run authority; unknown application tables are NEVER
// implicitly classified as metadata.
type forkTableClass uint8

const (
	forkTableAuthority forkTableClass = iota + 1
	forkTableInternal
	forkTableMetadata
)

type forkDestinationTable struct {
	name  string
	class forkTableClass
}

// This is the ONE versioned inventory for the fork destination guard.
// Every application table installed by Store.createSchema must be declared here.
// Keep the installed-schema architecture test and fail-closed runtime inspection:
// an unclassified table, even when empty, makes fork bootstrap ineligible.
var forkDestinationTableInventory = []forkDestinationTable{
	{name: "causal_outbox", class: forkTableAuthority},
	{name: "clock_states", class: forkTableAuthority},
	{name: "cognitive_actor_registry", class: forkTableAuthority},
	{name: "cognitive_owner_keys", class: forkTableAuthority},
	{name: "cognitive_owner_receipts", class: forkTableAuthority},
	{name: "committed_cycles", class: forkTableAuthority},
	{name: "communication_claims", class: forkTableAuthority},
	{name: "communication_transmissions", class: forkTableAuthority},
	{name: "cycle_control_state", class: forkTableAuthority},
	{name: "decision_ledger", class: forkTableAuthority},
	{name: "event_store", class: forkTableAuthority},
	{name: "evidence_ledger", class: forkTableAuthority},
	{name: "evidence_routes", class: forkTableAuthority},
	{name: "fork_cognitive_owner_proofs", class: forkTableAuthority},
	{name: "fork_communication_imports", class: forkTableAuthority},
	{name: "fork_history_records", class: forkTableAuthority},
	{name: "fork_history_roots", class: forkTableAuthority},
	{name: "fork_lineage", class: forkTableAuthority},
	{name: "fork_occurrence_imports", class: forkTableAuthority},
	{name: "fork_owner_proof_seals", class: forkTableAuthority},
	{name: "fork_trigger_imports", class: forkTableAuthority},
	{name: "genesis_records", class: forkTableAuthority},
	{name: "genesis_schedule_seals", class: forkTableAuthority},
	{name: "input_ledger", class: forkTableAuthority},
	{name: "input_slot_responses", class: forkTableAuthority},
	{name: "input_source_cursors", class: forkTableAuthority},
	{name: "round_integrity_seals", class: forkTableAuthority},
	{name: "scheduled_occurrences", class: forkTableAuthority},
	{name: "snapshot_cognitive_actors", class: forkTableAuthority},
	{name: "snapshot_owner_key_seals", class: forkTableAuthority},
	{name: "snapshot_prefix_members", class: forkTableAuthority},
	{name: "snapshot_seals", class: forkTableAuthority},
	{name: "snapshots", class: forkTableAuthority},
	{name: "source_ingress_receipts", class: forkTableAuthority},
	{name: "trigger_activations", class: forkTableAuthority},
	{name: "trigger_definitions", class: forkTableAuthority},
	{name: "trigger_registry_seals", class: forkTableAuthority},
	{name: "trigger_runtime", class: forkTableAuthority},
	{name: "world_revisions", class: forkTableAuthority},
	{name: "sqlite_sequence", class: forkTableInternal},
	{name: "sqlite_stat1", class: forkTableMetadata},
	{name: "sqlite_stat4", class: forkTableMetadata},
}

func ensureForkDestinationEmpty(ctx context.Context, tx *sql.Tx) error {
	return ensureForkDestinationEmptyWithInventory(ctx, tx, forkDestinationTableInventory)
}

// ensureForkDestinationEmptyWithInventory runs entirely inside the same child
// transaction that will publish Genesis. The inventory parameter permits
// architecture tests to demonstrate RED-before-classification / GREEN-after
// without modifying process-global state.
func ensureForkDestinationEmptyWithInventory(ctx context.Context, tx *sql.Tx, inventory []forkDestinationTable) error {
	if tx == nil {
		return errors.New("fork destination transaction is missing")
	}
	classified := make(map[string]forkTableClass, len(inventory))
	for _, table := range inventory {
		if table.name == "" || strings.ContainsAny(table.name, "\x00\"") {
			return fmt.Errorf("fork destination inventory has invalid table name %q", table.name)
		}
		if table.class != forkTableAuthority && table.class != forkTableInternal && table.class != forkTableMetadata {
			return fmt.Errorf("fork destination inventory has invalid class for %q", table.name)
		}
		if _, duplicate := classified[table.name]; duplicate {
			return fmt.Errorf("fork destination inventory duplicates %q", table.name)
		}
		classified[table.name] = table.class
	}

	rows, err := tx.QueryContext(ctx, "SELECT name FROM main.sqlite_schema WHERE type='table' ORDER BY name")
	if err != nil {
		return fmt.Errorf("inspect fork destination schema: %w", err)
	}
	defer rows.Close()
	seen := make(map[string]bool, len(inventory))
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return fmt.Errorf("read fork destination schema: %w", err)
		}
		if _, known := classified[name]; !known {
			return fmt.Errorf("fork destination has unclassified SQLite table %q", name)
		}
		seen[name] = true
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate fork destination schema: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close fork destination schema: %w", err)
	}

	for _, table := range inventory {
		if table.class != forkTableAuthority {
			continue
		}
		if !seen[table.name] {
			return fmt.Errorf("fork destination missing declared authority table %q", table.name)
		}
		// Table names are code-owned inventory entries. Quote the identifier
		// and use EXISTS rather than COUNT(*) to stop on the first orphan row.
		var present int
		query := "SELECT EXISTS(SELECT 1 FROM \"" + strings.ReplaceAll(table.name, "\"", "\"\"") + "\")"
		if err := tx.QueryRowContext(ctx, query).Scan(&present); err != nil {
			return fmt.Errorf("inspect fork authority %s: %w", table.name, err)
		}
		if present != 0 {
			return fmt.Errorf("fork destination contains previous authority in %s", table.name)
		}
	}
	return nil
}

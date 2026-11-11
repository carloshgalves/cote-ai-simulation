package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

// The real installed schema, not a second hand-maintained table list, is the
// architectural oracle. Adding a CREATE TABLE to store.go requires an explicit
// classification in forkDestinationTableInventory.
func TestForkDestinationInventoryMatchesInstalledSchema(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "installed.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := runForkInventoryGuard(ctx, store.db, forkDestinationTableInventory); err != nil {
		t.Fatalf("fork inventory drifted from actual Store schema: %v", err)
	}
}

// An empty NEW authority must already be RED. Once it is classified, the
// schema check becomes GREEN, but inserting even one row must again be RED.
// Removing the row restores the clean destination's eligibility.
func TestForkDestinationRejectsUnclassifiedTableAndRecovers(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "new-schema.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	const future = "future_causal_authority"
	if _, err := store.db.ExecContext(ctx, "CREATE TABLE "+future+" (marker INTEGER)"); err != nil {
		t.Fatal(err)
	}
	if err := runForkInventoryGuard(ctx, store.db, forkDestinationTableInventory); err == nil || !strings.Contains(err.Error(), "unclassified SQLite table") {
		t.Fatalf("unclassified empty authority must fail closed, got %v", err)
	}
	classified := append(append([]forkDestinationTable(nil), forkDestinationTableInventory...),
		forkDestinationTable{name: future, class: forkTableAuthority})
	if err := runForkInventoryGuard(ctx, store.db, classified); err != nil {
		t.Fatalf("classifying an empty authority should succeed: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, "INSERT INTO "+future+" (marker) VALUES(1)"); err != nil {
		t.Fatal(err)
	}
	if err := runForkInventoryGuard(ctx, store.db, classified); err == nil || !strings.Contains(err.Error(), future) {
		t.Fatalf("classified orphan authority should reject, got %v", err)
	}
	if _, err := store.db.ExecContext(ctx, "DELETE FROM "+future); err != nil {
		t.Fatal(err)
	}
	if err := runForkInventoryGuard(ctx, store.db, classified); err != nil {
		t.Fatalf("clean destination cannot recover after orphan removal: %v", err)
	}
}

// Exercise EVERY declared authority using a minimalist SQL schema. The
// companion installed-schema test proves that this complete inventory also
// matches real DDL; the real-fork tests exercise native FK/STRICT constraints.
func TestForkDestinationRejectsOrphanInEveryDeclaredAuthority(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "authority-probe.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	for _, entry := range forkDestinationTableInventory {
		if entry.class == forkTableAuthority {
			if _, err := db.ExecContext(ctx, "CREATE TABLE "+entry.name+" (marker INTEGER)"); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := runForkInventoryGuard(ctx, db, forkDestinationTableInventory); err != nil {
		t.Fatal(err)
	}
	for _, entry := range forkDestinationTableInventory {
		if entry.class != forkTableAuthority {
			continue
		}
		t.Run(entry.name, func(t *testing.T) {
			if _, err := db.ExecContext(ctx, "INSERT INTO "+entry.name+" VALUES(1)"); err != nil {
				t.Fatal(err)
			}
			got := runForkInventoryGuard(ctx, db, forkDestinationTableInventory)
			if got == nil || !strings.Contains(got.Error(), entry.name) {
				t.Fatalf("orphan %s bypassed fork guard: %v", entry.name, got)
			}
			if _, err := db.ExecContext(ctx, "DELETE FROM "+entry.name); err != nil {
				t.Fatal(err)
			}
			if err := runForkInventoryGuard(ctx, db, forkDestinationTableInventory); err != nil {
				t.Fatalf("clean retry after %s failed: %v", entry.name, err)
			}
		})
	}
}

func TestForkDestinationRejectsMissingInventoryAuthority(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "missing.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	var omitted string
	for _, entry := range forkDestinationTableInventory {
		if entry.class != forkTableAuthority {
			continue
		}
		if omitted == "" {
			omitted = entry.name
			continue
		}
		if _, err := db.ExecContext(ctx, "CREATE TABLE "+entry.name+" (marker INTEGER)"); err != nil {
			t.Fatal(err)
		}
	}
	got := runForkInventoryGuard(ctx, db, forkDestinationTableInventory)
	if got == nil || !strings.Contains(got.Error(), fmt.Sprintf("%q", omitted)) {
		t.Fatalf("missing declared authority should fail closed, got %v", got)
	}
}

func runForkInventoryGuard(ctx context.Context, db *sql.DB, inventory []forkDestinationTable) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	return ensureForkDestinationEmptyWithInventory(ctx, tx, inventory)
}

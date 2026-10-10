package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/conformance"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/genesis"
	csfsqlite "github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

func main() {
	bundle := flag.String("bundle", "docs/architecture/canonical-codec-v1-bundle", "normative policy bundle directory")
	reportPath := flag.String("report", "", "write deterministic JSON report to this path")
	genesisPath := flag.String("genesis", "", "strict canonical genesis envelope to persist")
	databasePath := flag.String("db", "", "SQLite run database (required with --genesis)")
	flag.Parse()
	if *genesisPath != "" || *databasePath != "" {
		if *genesisPath == "" || *databasePath == "" {
			fmt.Fprintln(os.Stderr, "--genesis and --db must be provided together")
			os.Exit(2)
		}
		if err := createRun(context.Background(), *bundle, *genesisPath, *databasePath); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	report, err := conformance.Run(*bundle)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if *reportPath != "" {
		if err := report.Write(*reportPath); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	fmt.Printf("CSF V1 conformance: %d/%d passed\n", report.Passed, report.Total)
	if report.Failed != 0 {
		os.Exit(1)
	}
}

func createRun(ctx context.Context, bundlePath, genesisPath, databasePath string) error {
	bundle, err := policy.Load(bundlePath)
	if err != nil {
		return err
	}
	encoded, err := os.ReadFile(genesisPath)
	if err != nil {
		return err
	}
	manifest, err := genesis.ValidateCanonical(encoded, bundle)
	if err != nil {
		return err
	}
	store, err := csfsqlite.Open(ctx, databasePath)
	if err != nil {
		return err
	}
	defer store.Close()
	if err := store.PersistGenesis(ctx, manifest); err != nil {
		return err
	}
	stored, err := store.LoadGenesis(ctx, manifest.RunID)
	if err != nil {
		return err
	}
	if !bytes.Equal(stored, encoded) {
		return fmt.Errorf("persisted genesis failed byte-for-byte verification")
	}
	fmt.Printf("created causal run %x genesis=%x\n", manifest.RunID, manifest.ID)
	return nil
}

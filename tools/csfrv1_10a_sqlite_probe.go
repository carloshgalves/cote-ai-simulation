//go:build ignore

// Standalone Go program invoked with an explicit file path. Excluded from
// package-level go test ./... to avoid colliding with another tools/main.
// csfrv1_10a_sqlite_probe reports the Go-linked SQLite engine, not Python/system SQLite.
// Uses an isolated in-memory connection; no causal ledgers are opened or changed.
package main

import (
    "database/sql"
    "flag"
    "fmt"
    "os"

    _ "modernc.org/sqlite"
    csfgenesis "github.com/carloshgalves/cote-ai-simulation/internal/csf/genesis"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

func main() {
    genesisPath := flag.String("genesis", "", "canonical Genesis file to validate, read-only")
    bundlePath := flag.String("bundle", "docs/architecture/canonical-codec-v1-bundle", "pinned policy bundle")
    flag.Parse()
    db, err := sql.Open("sqlite", "file:csfrv110a_sqlite_probe?mode=memory&cache=private")
    if err != nil {
        fmt.Fprintln(os.Stderr, err)
        os.Exit(1)
    }
    defer db.Close()
    var engineVersion string
    if err := db.QueryRow("SELECT sqlite_version()").Scan(&engineVersion); err != nil {
        fmt.Fprintln(os.Stderr, err)
        os.Exit(1)
    }
    fmt.Println(engineVersion)
    if *genesisPath != "" {
        bundle, err := policy.Load(*bundlePath)
        if err != nil { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
        raw, err := os.ReadFile(*genesisPath)
        if err != nil { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
        g, err := csfgenesis.ValidateCanonical(raw, bundle)
        if err != nil { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
        fmt.Printf("GENESIS_VALIDATED:%x\n", g.RunID)
    }
}

//go:build !js

// csfrv1_10a_sqlite_probe reports the Go-linked SQLite engine, not Python/system SQLite.
// Uses an isolated in-memory connection; no causal ledgers are opened or changed.
package main

import (
    "database/sql"
    "fmt"
    "os"

    _ "modernc.org/sqlite"
)

func main() {
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
}

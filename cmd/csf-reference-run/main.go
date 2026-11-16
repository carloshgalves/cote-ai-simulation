// Command csf-reference-run executes CSFRV1-10/B as a real local SQLite run.
// The synthetic stock extension is not a production economy or A1 proof.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/commit"
)

func main() {
	db := flag.String("db", "", "fresh SQLite database file (required)")
	report := flag.String("report", "", "new JSON report path; never overwrite")
	bundle := flag.String("bundle", "docs/architecture/canonical-codec-v1-bundle", "verified V1 bundle")
	reverseSources := flag.Bool("reverse-sources", false, "permute source declaration order")
	reverseResponses := flag.Bool("reverse-responses", false, "permute slot response order")
	communication := flag.Bool("communication", false, "continue the same run with C1 false Claim and delayed delivery")
	flag.Parse()
	if *db == "" {
		fmt.Fprintln(os.Stderr, "--db required")
		os.Exit(2)
	}
	result, err := commit.RunReference(context.Background(), *db, *bundle,
		commit.ReferenceOptions{ReverseSources: *reverseSources, ReverseResponses: *reverseResponses, IncludeCommunication: *communication})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	encoded = append(encoded, '\n')
	if *report == "" {
		_, _ = os.Stdout.Write(encoded)
		return
	}
	f, err := os.OpenFile(*report, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	_, writeErr := f.Write(encoded)
	closeErr := f.Close()
	if writeErr != nil || closeErr != nil {
		fmt.Fprintln(os.Stderr, "cannot write report", writeErr, closeErr)
		os.Exit(1)
	}
	fmt.Println(*report)
}
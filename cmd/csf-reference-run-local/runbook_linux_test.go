//go:build linux

package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/runbook"
)

func TestH2ORunbookRejectsMalformedArgumentsWithoutEcho(t *testing.T) {
	for _, args := range [][]string{
		nil,
		{"--out", "PRIVATE_PAYLOAD", "--verify", "PRIVATE_PAYLOAD"},
		{"--diagnose", "PRIVATE_PAYLOAD"},
		{"--verify", "PRIVATE_PAYLOAD", "--crash-confirmed"},
		{"--unknown-PRIVATE_PAYLOAD"},
	} {
		var out, err bytes.Buffer
		code := localCommand(context.Background(), args, &out, &err)
		if code != runbook.Usage.Code || !strings.Contains(err.String(), "LOCAL_REFERENCE_STATE=INVALID_ARGUMENTS") ||
			strings.Contains(out.String()+err.String(), "PRIVATE_PAYLOAD") {
			t.Fatal("invalid arguments leaked or were accepted")
		}
	}
}

func TestH2ORunbookErrorOutputIsRedacted(t *testing.T) {
	var out, err bytes.Buffer
	code := emitOutcomeForError(&out, &err, errors.New("PRIVATE_PAYLOAD"), true)
	if code != runbook.Unverified.Code || strings.Contains(out.String()+err.String(), "PRIVATE_PAYLOAD") ||
		!strings.Contains(err.String(), "LOCAL_REFERENCE_STATE=CORRUPT_OR_UNVERIFIED") {
		t.Fatal("untrusted verification error leaked or was misclassified")
	}
}

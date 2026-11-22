//go:build linux

package commit

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"testing"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/adapter/embodimentv1"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
)

// G4 consumes two real, supervised, Genesis-signed A1 source cuts. Only the
// already authenticated cuts are reused: their raw Python wall_time differs
// between independent captures, so recapturing them is not a permutation.
// The comparator covers *all* authoritative rows, both cognitive owner DBs,
// and each event/input identity, not a generated public success flag.
func TestReferenceG4AuthenticatedTwoEpochsAndFullLedgerPermutation(t *testing.T) {
	host, plans, cuts := referenceE2SignedPair(t)
	genesis, err := buildReferenceGenesis(host.Bundle)
	if err != nil {
		t.Fatal(err)
	}
	registration, err := host.Bundle.PhysicalRegistration()
	if err != nil || registration.Disclosure != "ENGINE_ONLY" {
		t.Fatalf("invalid installed physical disclosure registration: %v", err)
	}
	for epoch, approved := range cuts {
		raw, signature, completed, err := approved.CanonicalProof()
		if err != nil || len(completed) != 4 {
			t.Fatalf("supervised epoch %d lacks complete A2 proof: %v", epoch+1, err)
		}
		parsed, err := embodimentv1.DecodeSignedCut(raw, signature)
		if err != nil || parsed.Epoch != uint64(epoch+1) ||
			parsed.RunID != genesis.RunID || parsed.GenesisDigest != genesis.Digest {
			t.Fatalf("signed epoch %d does not bind immutable Genesis: %v", epoch+1, err)
		}
		if err := embodimentv1.VerifySignedCut(parsed, registration, genesis.RunID, genesis.Digest); err != nil {
			t.Fatalf("epoch %d signature not verified independently of D3: %v", epoch+1, err)
		}
	}
	run := func(opts ReferenceOptions) (ReferenceD3Proof, referenceE1Snapshot) {
		t.Helper()
		db, owner := referenceE2Paths(t)
		proof, err := RunReferenceD3(context.Background(), db, owner, bundlePath,
			ReferenceD2Config{Host: host, EpochPlans: plans, frozenSupervisedCuts: &cuts}, opts)
		if err != nil {
			t.Fatalf("signed two-epoch Go D1-D3 integration: %v", err)
		}
		if proof.d2.CommittedPhysicalEpochs != 2 || proof.d2.FinalRevision != 9 ||
			proof.d2.FinalInstant != 35 || !proof.d2.FinalSourceClosureVerified ||
			!proof.d2.A1SignedByGenesisKey || !proof.d2.SameCausalRun ||
			!proof.d2.ResourceBranchUnchanged || !proof.d2.BodyContinuityVerified ||
			!proof.d2.ReplayVerified || !proof.d2.SnapshotVerifiedWithThreeTailCommits ||
			proof.Matrix.PhysicalEvents != 2 || proof.Matrix.PhysicalPerceptionTasks != 0 ||
			proof.Matrix.ReplayCommits != 9 || !proof.Matrix.ReadersDidNotWrite {
			t.Fatal("physical source was not committed twice by Go or crossed an ENGINE_ONLY boundary")
		}
		if proof.d2.GenesisSHA256 == "" || proof.d2.RunID == "" {
			t.Fatal("physical event is not bound to a persisted Go Genesis")
		}
		ro, err := openReferenceReadOnlyDB(db)
		if err != nil {
			t.Fatal(err)
		}
		defer ro.Close()
		for i := 0; i != 2; i++ {
			source, err := referenceC4DecodeID(proof.d2.PhysicalInputIDs[i])
			if err != nil {
				t.Fatal(err)
			}
			var input []byte
			if err := ro.QueryRow("SELECT canonical FROM input_ledger WHERE id=?", source[:]).Scan(&input); err != nil || len(input) == 0 {
				t.Fatalf("physical ingress %d absent from real InputLedger: %v", i+1, err)
			}
			event, err := referenceC4DecodeID(proof.d2.PhysicalEventIDs[i])
			if err != nil {
				t.Fatal(err)
			}
			var canonical []byte
			if err := ro.QueryRow("SELECT canonical FROM event_store WHERE id=?", event[:]).Scan(&canonical); err != nil {
				t.Fatalf("physical event %d absent from authoritative EventStore: %v", i+1, err)
			}
			record, err := codec.StrictDecodePayload(canonical)
			if err != nil {
				t.Fatal(err)
			}
			fields, ok := record.([]any)
			if !ok || len(fields) != 19 || fields[3] != "physical.advanced" {
				t.Fatal("physical ingress did not produce an explicitly typed engine event")
			}
		}
		var events int
		rows, err := ro.Query("SELECT canonical FROM event_store")
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var raw []byte
			if err := rows.Scan(&raw); err != nil {
				rows.Close()
				t.Fatal(err)
			}
			parsed, err := codec.StrictDecodePayload(raw)
			if err != nil {
				rows.Close()
				t.Fatal(err)
			}
			fields, ok := parsed.([]any)
			if !ok || len(fields) != 19 {
				rows.Close()
				t.Fatal("noncanonical persisted event")
			}
			if fields[3] == "physical.advanced" {
				events++
			}
		}
		err = errors.Join(rows.Err(), rows.Close())
		if err != nil || events != 2 {
			t.Fatalf("expected exactly two real physical transitions, got %d: %v", events, err)
		}
		public, err := proof.PublicReport()
		if err != nil {
			t.Fatal(err)
		}
		out, err := json.Marshal(public)
		if err != nil || len(out) == 0 {
			t.Fatalf("cross-disclosure certificate failed: %v", err)
		}
		return proof, referenceE2Capture(t, db, owner)
	}
	baseline, left := run(ReferenceOptions{})
	flipped, right := run(ReferenceOptions{ReverseSources: true, ReverseResponses: true})
	if baseline.RunID != flipped.RunID ||
		!reflect.DeepEqual(baseline.d2.PhysicalInputIDs, flipped.d2.PhysicalInputIDs) ||
		!reflect.DeepEqual(baseline.d2.PhysicalEventIDs, flipped.d2.PhysicalEventIDs) ||
		!reflect.DeepEqual(baseline.d2.PhysicalJournalIDs, flipped.d2.PhysicalJournalIDs) ||
		!reflect.DeepEqual(baseline.Matrix, flipped.Matrix) {
		t.Fatal("source/worker order changed signed A1 identities, Go commits or disclosure matrix")
	}
	if err := referenceE1Compare(left, right); err != nil {
		t.Fatalf("physical source order or Python-local time changed Go authoritative/owner bytes: %v", err)
	}
}

// The positive control authenticates the genuine source. Each mutation is
// canonical and shape-valid, but must fail the Genesis-pinned Ed25519 check.
// This is an ingress gate test; the cross-policy test below also verifies the
// absence of a physical CycleCommit in the real SQLite transaction authority.
func TestReferenceG4SignedCutMutationMatrix(t *testing.T) {
	host, _, cuts := referenceE2SignedPair(t)
	genesis, err := buildReferenceGenesis(host.Bundle)
	if err != nil {
		t.Fatal(err)
	}
	reg, err := host.Bundle.PhysicalRegistration()
	if err != nil {
		t.Fatal(err)
	}
	raw, signature, _, err := cuts[0].CanonicalProof()
	if err != nil {
		t.Fatal(err)
	}
	signed, err := embodimentv1.DecodeSignedCut(raw, signature)
	if err != nil {
		t.Fatal(err)
	}
	if err := embodimentv1.VerifySignedCut(signed, reg, genesis.RunID, genesis.Digest); err != nil {
		t.Fatalf("positive signed A1 cut rejected: %v", err)
	}
	for _, tc := range []struct {
		name  string
		index int
	}{
		{"run-id", 1}, {"genesis-digest", 2}, {"source-id", 3},
		{"executable-hash", 4}, {"snapshot-hash", 5}, {"events-hash", 6},
		{"epoch", 7}, {"a2-evidence-hash", 8}, {"completed-body-hash", 9},
	} {
		t.Run(tc.name, func(t *testing.T) {
			value, err := codec.StrictDecodePayload(raw)
			if err != nil {
				t.Fatal(err)
			}
			fields := value.([]any)
			switch tc.index {
			case 3:
				fields[tc.index] = "physical.forged"
			case 7:
				fields[tc.index] = uint64(2)
			default:
				changed := append([]byte(nil), fields[tc.index].([]byte)...)
				changed[0] ^= 0x80
				fields[tc.index] = changed
			}
			mutant, err := codec.CanonicalPayload(fields)
			if err != nil {
				t.Fatal(err)
			}
			probe, err := embodimentv1.DecodeSignedCut(mutant, signature)
			if err != nil {
				t.Fatalf("mutation did not preserve canonical signed-cut shape: %v", err)
			}
			if err := embodimentv1.VerifySignedCut(probe, reg, genesis.RunID, genesis.Digest); err == nil {
				t.Fatal("forged field retained signed-source authority")
			}
		})
	}
	t.Run("signature", func(t *testing.T) {
		mutant := append([]byte(nil), signature...)
		mutant[0] ^= 0x80
		probe, err := embodimentv1.DecodeSignedCut(raw, mutant)
		if err != nil {
			t.Fatal(err)
		}
		if err := embodimentv1.VerifySignedCut(probe, reg, genesis.RunID, genesis.Digest); err == nil {
			t.Fatal("corrupt signature retained signed-source authority")
		}
	})
}

// The foreign policy is itself installed and valid; only the A1 cuts belong
// to a different key/Genesis. C4 and the NoProposal prerequisite may commit,
// but no physical WorldRevision, CycleCommit or event can be published.
func TestReferenceG4ForeignRegisteredPolicyRejectsSignedPhysicalEpochBeforeCommit(t *testing.T) {
	_, _, oldCuts := referenceE2SignedPair(t)
	foreignHost, plans := referenceD2TestHost(t)
	genesis, err := buildReferenceGenesis(foreignHost.Bundle)
	if err != nil {
		t.Fatal(err)
	}
	db, owner := referenceE2Paths(t)
	proof, err := RunReferenceD2(context.Background(), db, owner, bundlePath,
		ReferenceD2Config{Host: foreignHost, EpochPlans: plans, frozenSupervisedCuts: &oldCuts}, ReferenceOptions{})
	if err == nil || proof.Status == "RUN_LOCAL_10D2" {
		t.Fatal("cut from a different installed physical signer/policy was committed")
	}
	if _, statErr := os.Stat(db); statErr == nil {
		referenceD2AssertPhysicalPrefix(t, db, hexRun(genesis.RunID), 0)
	} else if !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("cannot inspect rejected physical run: %v", statErr)
	}
}


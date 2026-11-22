//go:build linux

package commit

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"path/filepath"
	"testing"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/contextboundary"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/observatory"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
)

// This adapter exists only in the G5 test: it cannot query a global ledger,
// change an actor at read time, or grant additional capabilities. The real
// SQLite POV reader has already authenticated the durable SlotDispatch and
// its sealed revision before issuing the records supplied to Build.
type g5ScopedPOV struct {
	view observatory.POVView
}

func (r g5ScopedPOV) ReadAddressedEvidence(_ context.Context, actor [2]string) ([]ports.CanonicalRecord, []ports.CanonicalRecord, error) {
	if actor != r.view.Actor {
		return nil, nil, errors.New("G5 cannot reuse a foreign actor capability")
	}
	observations := make([]ports.CanonicalRecord, 0, len(r.view.Entries))
	inputs := make([]ports.CanonicalRecord, 0, len(r.view.Entries))
	for _, entry := range r.view.Entries {
		observations = append(observations, ports.CanonicalRecord{
			Identity: entry.ObservationID, Canonical: bytes.Clone(entry.ObservationCanonical),
		})
		inputs = append(inputs, ports.CanonicalRecord{
			Identity: entry.KnowledgeInputID, Canonical: bytes.Clone(entry.KnowledgeCanonical),
		})
	}
	return observations, inputs, nil
}

// G5 checks the actual D3/F5 four-view split and passes the addressed
// durable evidence through the genuine context builder. This is not a
// synthetic input fixture or a privileged world-state redaction.
func TestReferenceG5RealFourViewActorContextsAndFalseClaim(t *testing.T) {
	ctx := context.Background()
	host, plans, cuts := referenceE2SignedPair(t)
	path, owner := referenceE2Paths(t)
	proof, err := RunReferenceD3(ctx, path, owner, bundlePath,
		ReferenceD2Config{Host: host, EpochPlans: plans, frozenSupervisedCuts: &cuts}, ReferenceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if proof.Matrix.PhysicalEvents != 2 || proof.Matrix.PhysicalPerceptionTasks != 0 ||
		proof.Matrix.ReplayCommits != 9 || !proof.Matrix.RecipientBClaimOnly ||
		!proof.Matrix.ActorPhysicalDenied || !proof.Matrix.PublicPhysicalDenied ||
		!proof.Matrix.ReadersDidNotWrite {
		t.Fatal("G5 missing actual C4-D3 source, receipt and ENGINE_ONLY boundaries")
	}
	initial := referenceE2Capture(t, path, owner)
	run := referenceC2Decode32(t, proof.RunID)
	readers := f5OpenReaders(t, path, run, host.Bundle)
	views := readers.read(t)
	if views.Public.PublicationStatus != observatory.NoPublicRoute ||
		len(views.Public.Publications) != 0 || len(views.A.Entries) == 0 ||
		len(views.B.Entries) == 0 || views.A.ThroughRevision != 6 ||
		views.B.ThroughRevision != 6 || len(views.Audit.Commits) != 9 {
		t.Fatal("G5 four-view authentic control is vacuous or not cut-scoped")
	}
	var claimID [32]byte
	claimID = referenceC2Decode32(t, proof.d2.C4.Communication.KnowledgeInputID)
	for i, pov := range []observatory.POVView{views.A, views.B} {
		actor := [2]string{"school", []string{"a", "b"}[i]}
		scope := g5ScopedPOV{view: pov}
		context, err := contextboundary.Build(ctx, scope, actor)
		if err != nil || len(context.Inbox) != len(pov.Entries) || context.Actor != actor {
			t.Fatalf("G5 real actor %d evidence did not form exact addressed context: %v", i, err)
		}
		claims := 0
		for _, entry := range context.Inbox {
			if entry.ReceivedAt > 35 || entry.ReceivedAt < 0 {
				t.Fatal("agent received future or invalid causal knowledge")
			}
			if entry.Input.Identity == claimID {
				claims++
				if actor[1] != "b" || entry.ReceivedAt != 35 {
					t.Fatal("A learned private B allegation or clock shifted on receipt")
				}
				value, err := codec.StrictDecodePayload(entry.Observation.Canonical)
				if err != nil {
					t.Fatal(err)
				}
				fields := value.([]any)
				percept := fields[9].([]any)[0].([]any)
				envelope, err := codec.StrictDecodeEnvelope(percept[2].([]byte))
				if err != nil || envelope.DomainTag != "cote.csf.communication.delivered.v1" {
					t.Fatal("false testimony was not delivered as a claim")
				}
				notice := envelope.Value.([]any)
				if len(notice) != 6 || notice[1] != "C" ||
					!reflect.DeepEqual(fields[11], []any{"actual-sender-redacted", "claim-unverified"}) {
					t.Fatal("true sender was disclosed or false claim promoted to fact")
				}
			}
		}
		if claims != i {
			t.Fatalf("actor %d received wrong number of B-only claims: %d", i, claims)
		}
		if _, err := contextboundary.Build(ctx, scope, [2]string{"school", []string{"b", "a"}[i]}); err == nil {
			t.Fatal("valid POV capability was reusable by a foreign actor")
		}
	}
	// Contrast with privileged truth, without exposing privileged data through
	// either POV or emitting owner/seed data into the CI certificate.
	if !proof.d2.C4.CrossAcceptance.ClaimNotWorldTruth ||
		!proof.d2.C4.CrossAcceptance.OwnerAndPOVVerified {
		t.Fatal("the real C4 engine did not distinguish false Claim from world truth")
	}
	if err := referenceE1Compare(initial, referenceE2Capture(t, path, owner)); err != nil {
		t.Fatal("G5 actor/public/privileged queries modified any causal or private owner byte", err)
	}
}

// A real C1 delivery fault at its first transactional boundary leaves a
// durably sent, IN_TRANSIT transmission but no delivered event or recipient
// KnowledgeInput. This probes the intermediate causal state, not only a
// completed six-commit happy path.
func TestReferenceG5InTransitClaimHasNoRecipientKnowledge(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "in-transit.db")
	_, err := RunReference(ctx, path, bundlePath, ReferenceOptions{
		IncludeCommunication: true, FailCommunicationDeliveryAt: 1,
	})
	if err == nil {
		t.Fatal("injected due-delivery fault did not interrupt real transaction")
	}
	db, err := openReferenceReadOnlyDB(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var total, inTransit int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*), COALESCE(SUM(CASE WHEN status=0 THEN 1 ELSE 0 END),0) FROM communication_transmissions").Scan(&total, &inTransit); err != nil {
		t.Fatal(err)
	}
	if total != 1 || inTransit != 1 {
		t.Fatalf("missing durable unsent-to-recipient transmission: %d/%d", total, inTransit)
	}
	rows, err := db.QueryContext(ctx, "SELECT canonical FROM evidence_ledger")
	if err != nil {
		t.Fatal(err)
	}
	receivedMessages := 0
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		value, err := codec.StrictDecodePayload(raw)
		if err != nil {
			rows.Close()
			t.Fatal(err)
		}
		fields, ok := value.([]any)
		if ok && len(fields) == 11 && fields[5] == uint64(1) && fields[6] == "school.message.v1" {
			receivedMessages++
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		t.Fatal(err)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	if receivedMessages != 0 {
		t.Fatalf("IN_TRANSIT false claim became %d recipient KnowledgeInputs", receivedMessages)
	}
	var delivered int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM event_store WHERE canonical IS NULL").Scan(&delivered); err != nil || delivered != 0 {
		t.Fatalf("malformed event authority in pre-delivery prefix: %d %v", delivered, err)
	}
	store, err := sqlite.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	// The rejected delivery attempt must not create a sixth commit.
	var commits int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM committed_cycles").Scan(&commits); err != nil || commits != 5 {
		t.Fatalf("faulted delivery wrote partial causal commit: %d %v", commits, err)
	}
}

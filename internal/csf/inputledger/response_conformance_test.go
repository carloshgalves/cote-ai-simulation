package inputledger_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/extensions/testresourcev1"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/inputledger"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/memory"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
)

// The codec's record-to-reference vectors specify one array of body fields.
// A runtime round-trip alone cannot detect an extra array in both writer and reader.
func TestCSFRV18R19Finding2SlotResponseUsesNormativeBody(t *testing.T) {
	for _, variant := range [][2]uint8{{1, 0}, {1, 1}, {2, 0}, {2, 1}} {
		kind := variant[1]
		t.Run(string(rune('0'+variant[0]))+"/"+string(rune('0'+kind)), func(t *testing.T) {
			ctx := context.Background()
			b, err := policy.Load("../../../docs/architecture/canonical-codec-v1-bundle")
			if variant[0] == 2 {
				b, err = policy.LoadV2("../../../docs/architecture/canonical-codec-v2-bundle")
			}
			if err != nil {
				t.Fatal(err)
			}
			store := memory.New()
			run, cycle := domain.ID{0xa1}, domain.ID{0xb2}
			l, err := inputledger.Open(ctx, run, b, store)
			if err != nil {
				t.Fatal(err)
			}
			round, err := l.DeclareRound(ctx, "schedule", [2]string{"school", "a"}, []string{"act"}, domain.EligibilityCoordinate{},
				[]any{uint64(2), []any{make([]byte, 32), make([]byte, 32)}}, 0, 0, 0, []any{uint64(0)})
			if err != nil {
				t.Fatal(err)
			}
			did, _, err := identity.New(b).Sum("cote.csf.id.dispatch", "cote.csf.schema.dispatch-id-preimage", 1, []any{run[:], cycle[:], round.ID[:], "act", uint64(0)})
			if err != nil {
				t.Fatal(err)
			}
			d := inputledger.Dispatch{ID: domain.ID(did), RunID: run, CycleID: cycle, RoundID: round.ID, SlotID: "act", Actor: round.Actor}
			if err := l.RegisterDispatch(ctx, d, inputledger.CycleBinding{ID: cycle}); err != nil {
				t.Fatal(err)
			}
			r, err := testresourcev1.ProposalForDispatch(b, d)
			if err != nil {
				t.Fatal(err)
			}
			tag, schema := "cote.csf.digest.action-proposal-unit", "cote.csf.schema.action-proposal.body"
			if kind == 1 {
				tag, schema = "cote.csf.digest.no-proposal-unit", "cote.csf.schema.no-proposal.body"
				id, _, err := identity.New(b).Sum("cote.csf.id.no-proposal", "cote.csf.schema.slot-response-id-preimage", 1, []any{did[:], uint64(1)})
				if err != nil {
					t.Fatal(err)
				}
				r = inputledger.Response{Kind: 1, ID: domain.ID(id), DispatchID: d.ID, RunID: run, CycleID: cycle, RoundID: round.ID, SlotID: "act", Actor: round.Actor,
					Body: []any{id[:], did[:], run[:], cycle[:], round.ID[:], "act", []any{"school", "a"}, int64(0), uint64(0), uint64(0)}}
			}
			if err := l.Respond(ctx, r); err != nil {
				t.Fatal(err)
			}
			_, _, _, _, slots, _ := l.Snapshot()
			stored := slots[round.ID]["act"]
			want, _, err := identity.New(b).Sum(tag, schema, 1, stored.Body)
			if err != nil {
				t.Fatal(err)
			}
			if stored.Digest != want {
				t.Fatal("runtime unit digest does not match normative flat body")
			}
			reopened, err := inputledger.Open(ctx, run, b, store)
			if err != nil {
				t.Fatal(err)
			}
			_, _, _, _, again, _ := reopened.Snapshot()
			if !bytes.Equal(again[round.ID]["act"].Canonical, stored.Canonical) {
				t.Fatal("normative response changed after reopen")
			}
			// Bytes from the old erroneous wrapper are incompatible, never silently
			// rehashed in place or accepted as a second encoding of this authority.
			wrong, _, err := identity.New(b).Sum(tag, schema, 1, []any{stored.Body})
			if err != nil {
				t.Fatal(err)
			}
			bad, err := codec.CanonicalPayload(append(append([]any{}, stored.Body...), wrong[:]))
			if err != nil {
				t.Fatal(err)
			}
			records, err := store.Records(ctx)
			if err != nil {
				t.Fatal(err)
			}
			corrupt := memory.New()
			for _, record := range records {
				if record.Identity == [32]byte(stored.ID) {
					record = ports.CanonicalRecord{Identity: record.Identity, Canonical: bad}
				}
				if err := corrupt.Append(ctx, record); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := inputledger.Open(ctx, run, b, corrupt); err == nil {
				t.Fatal("non-normative historical body was accepted")
			}
		})
	}
}

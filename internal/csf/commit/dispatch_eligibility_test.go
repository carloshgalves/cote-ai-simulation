package commit

import (
	"context"
	"testing"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/inputledger"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
)

func TestCSFRV18R19Finding4DispatchRequiresReachedEligibility(t *testing.T) {
	for _, mode := range []string{"register", "append"} {
		for _, when := range []string{"future-instant", "future-ordinal", "eligible"} {
			t.Run(mode+"/"+when, func(t *testing.T) {
				ctx := context.Background()
				_, parent, ledger, run, b, path := testFixture(t)
				root, err := parent.ValidatedGenesis(ctx, [32]byte(run), b)
				if err != nil {
					t.Fatal(err)
				}
				at := domain.EligibilityCoordinate{NotBeforeInstant: root.InitialInstant}
				if when == "future-instant" {
					at.NotBeforeInstant += 5
				}
				if when == "future-ordinal" {
					at.NotBeforeCycleOrdinal = 1
				}
				round, err := ledger.DeclareRound(ctx, "schedule", [2]string{"school", "a"}, []string{"act"}, at,
					[]any{uint64(2), []any{root.ID[:], root.Digest[:]}}, 1, 0, 0, []any{uint64(0)})
				if err != nil {
					t.Fatal(err)
				}
				cid, _, err := identity.New(b).Sum("cote.csf.id.cycle", "cote.csf.schema.cycle-id-preimage", 1, []any{run[:], []any{int64(root.InitialInstant), uint64(0)}})
				if err != nil {
					t.Fatal(err)
				}
				d := r19Dispatch(t, b, run, domain.ID(cid), round, "act", 0, 0, root.InitialInstant)
				if mode == "register" {
					err = ledger.RegisterDispatch(ctx, d, inputledger.CycleBinding{ID: d.CycleID, Instant: d.EffectiveAt})
				} else {
					raw, e := codec.CanonicalPayload([]any{d.ID[:], run[:], cid[:], round.ID[:], d.SlotID, uint64(0), []any{d.Actor[0], d.Actor[1]}, []any{int64(at.NotBeforeInstant), uint64(at.NotBeforeCycleOrdinal)}, uint64(0), int64(d.EffectiveAt), d.RequestDigest[:]})
					if e != nil {
						t.Fatal(e)
					}
					err = parent.Append(ctx, ports.CanonicalRecord{Identity: [32]byte(d.ID), Canonical: raw})
				}
				if when != "eligible" {
					if err == nil {
						t.Fatal("future round received current dispatch authority")
					}
					records, e := parent.Records(ctx)
					if e != nil {
						t.Fatal(e)
					}
					for _, r := range records {
						if r.Identity == [32]byte(d.ID) {
							t.Fatal("rejected future dispatch was persisted")
						}
					}
					return
				}
				if err != nil {
					t.Fatal("eligible current round rejected:", err)
				}
				if err := parent.Close(); err != nil {
					t.Fatal(err)
				}
				reopened, err := sqlite.Open(ctx, path)
				if err != nil {
					t.Fatal(err)
				}
				defer reopened.Close()
				ledger, err = inputledger.Open(ctx, run, b, reopened)
				if err != nil {
					t.Fatal(err)
				}
				if err := ledger.Respond(ctx, r19NoProposal(t, b, d, 0)); err != nil {
					t.Fatal("eligible capability lost on reopen:", err)
				}
				if _, err := reopened.ReplayClock(ctx, [32]byte(run), b); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

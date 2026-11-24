package queryhost

import (
	"errors"
	"testing"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/observatory"
)

func TestOperationalStateBindsAuthenticatedGenesisIdentity(t *testing.T) {
	for _, run := range [][32]byte{{}, {1}} {
		base := observatory.PrivilegedReport{
			State: observatory.RunState{Run: run, Control: observatory.CycleControl{Status: observatory.ATTEMPT_IN_FLIGHT}},
			Genesis: observatory.GenesisAuthority{
				Run: run, GenesisID: [32]byte{2}, Digest: [32]byte{3}, ContentHash: [32]byte{4},
			},
		}
		got, err := bindOperationalState(run, base)
		if err != nil || got != base.State {
			t.Fatal("authenticated Genesis failed to bind the projected identity")
		}
		for _, tc := range []struct {
			name string
			edit func(*observatory.PrivilegedReport)
		}{
			{"foreign-state", func(r *observatory.PrivilegedReport) { r.State.Run = [32]byte{9} }},
			{"foreign-genesis", func(r *observatory.PrivilegedReport) { r.Genesis.Run = [32]byte{9} }},
		} {
			t.Run(tc.name, func(t *testing.T) {
				bad := base
				tc.edit(&bad)
				got, err := bindOperationalState(run, bad)
				if !errors.Is(err, observatory.ErrInvalidReplay) || got != (observatory.RunState{}) {
					t.Fatal("conflicting operational identity was accepted")
				}
			})
		}
	}
}

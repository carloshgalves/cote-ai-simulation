package sqlite

import (
	"bytes"
	"testing"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/genesis"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
)

func fork57ID(b byte) [32]byte { var id [32]byte; id[0] = b; return id }
func fork57Archived(t *testing.T, fields any) ports.CanonicalRecord {
	t.Helper()
	raw, err := codec.CanonicalPayload(fields)
	if err != nil {
		t.Fatal(err)
	}
	return ports.CanonicalRecord{Identity: [32]byte(identity.OpaqueContentHash(raw)), Canonical: raw}
}
func fork57Slot(id, round [32]byte, slot string, ordinal uint64) *forkSlotHistory {
	source, cycle := fork57ID(0x90), fork57ID(0x91)
	f := []any{id[:], source[:], cycle[:], round[:], slot, ordinal,
		[]any{"school", "agent-a"}, []any{int64(40), uint64(0)}, uint64(0), int64(40), []any{uint64(0)}}
	return &forkSlotHistory{oldID: id, fields: f}
}

func TestFork57ArchivedRevocationAndResponseFailClosed(t *testing.T) {
	dispatch := fork57ID(0x11)
	revoked := fork57Archived(t, []any{dispatch[:], uint64(0), []any{uint64(0)}})
	mk := func() map[string]*forkSlotHistory {
		return map[string]*forkSlotHistory{string(dispatch[:]): fork57Slot(dispatch, fork57ID(0x21), "slot-a", 0)}
	}
	cases := []struct {
		name      string
		items     []ports.CanonicalRecord
		wantError bool
	}{
		{"valid", []ports.CanonicalRecord{revoked}, false},
		{"duplicate", []ports.CanonicalRecord{revoked, revoked}, true},
		{"bad-hash", []ports.CanonicalRecord{{Identity: fork57ID(0xfe), Canonical: revoked.Canonical}}, true},
		{"non-array", []ports.CanonicalRecord{fork57Archived(t, uint64(11))}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			histories := mk()
			err := attachForkSlotOutcomesV2(tc.items, histories)
			if tc.wantError && err == nil {
				t.Fatal("invalid archived receipt accepted")
			}
			if !tc.wantError && (err != nil || histories[string(dispatch[:])].revoked == nil) {
				t.Fatal("valid revocation was not linked:", err)
			}
		})
	}
	responseID := fork57ID(0x40)
	response := make([]any, 11)
	for i := range response {
		response[i] = uint64(0)
	}
	response[0], response[1] = responseID[:], dispatch[:]
	responseRecord := fork57Archived(t, response)
	responseRecord.Identity = responseID
	histories := mk()
	if err := attachForkSlotOutcomesV2([]ports.CanonicalRecord{revoked, responseRecord}, histories); err != nil {
		t.Fatal(err)
	}
	if _, err := validateAndOrderForkSlotHistoriesV2(nil, histories, make([]any, 19),
		&genesis.Genesis{InitialInstant: 40}, 0, nil); err == nil {
		t.Fatal("dispatch both revoked and answered escaped exclusive settlement")
	}
	responseRecord.Identity = fork57ID(0x41)
	if err := attachForkSlotOutcomesV2([]ports.CanonicalRecord{responseRecord}, mk()); err == nil {
		t.Fatal("response record ID mismatch accepted")
	}
}

func TestFork57SlotOrderingAndRevocationOrdinalInvariant(t *testing.T) {
	first, second, round := fork57ID(1), fork57ID(2), fork57ID(3)
	a, b := fork57Slot(first, round, "a", 0), fork57Slot(second, round, "a", 1)
	a.revoked, b.revoked = []any{first[:], uint64(0), []any{uint64(0)}}, []any{second[:], uint64(0), []any{uint64(0)}}
	history := map[string]*forkSlotHistory{string(second[:]): b, string(first[:]): a}
	ordered, err := validateAndOrderForkSlotHistoriesV2(nil, history, make([]any, 19), &genesis.Genesis{InitialInstant: 40}, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(ordered) != 2 || ordered[0].oldID != first || ordered[1].oldID != second {
		t.Fatal("dispatch ordering depended on Go map iteration")
	}
	b.fields[5] = uint64(2)
	if _, err := validateAndOrderForkSlotHistoriesV2(nil, history, make([]any, 19),
		&genesis.Genesis{InitialInstant: 40}, 0, nil); err == nil {
		t.Fatal("noncontiguous revoked dispatch ordinal accepted")
	}
	b.fields[5] = uint64(1)
	b.revoked = nil
	b.fields[7] = []any{int64(41), uint64(0)}
	if _, err := validateAndOrderForkSlotHistoriesV2(nil, history, make([]any, 19),
		&genesis.Genesis{InitialInstant: 40}, 0, nil); err == nil {
		t.Fatal("unrevoked dispatch at another instant accepted")
	}
}

func TestFork57RemintDispatchPreservesSourceAndRekeysChild(t *testing.T) {
	v1, err := policy.Load("../../../../docs/architecture/canonical-codec-v1-bundle")
	if err != nil {
		t.Fatal(err)
	}
	v2, err := policy.LoadV2("../../../../docs/architecture/canonical-codec-v2-bundle")
	if err != nil {
		t.Fatal(err)
	}
	oldRun, newRun, oldRound, newRound, oldID := fork57ID(0x20), fork57ID(0x21), fork57ID(0x30), fork57ID(0x31), fork57ID(0x32)
	parent := &genesis.Genesis{RunID: domain.ID(oldRun), InitialInstant: 40}
	child := &genesis.Genesis{RunID: domain.ID(newRun), InitialInstant: 40}
	history := fork57Slot(oldID, oldRound, "slot-a", 0)
	round := []any{newRound[:], newRun[:], "source", []any{"school", "agent-a"}, []any{"slot-a"}, []any{int64(40), uint64(0)}}
	result, err := remintForkSlotHistoriesV2(parent, child, nil, nil,
		map[string][]any{string(oldRound[:]): round}, []*forkSlotHistory{history}, v1, v2)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 || result[0].Identity == oldID {
		t.Fatal("child inherited parent dispatch ID")
	}
	decoded, err := codec.StrictDecodePayload(result[0].Canonical)
	if err != nil {
		t.Fatal(err)
	}
	fields, ok := decoded.([]any)
	if !ok || len(fields) != 11 || !bytes.Equal(fields[0].([]byte), result[0].Identity[:]) ||
		!bytes.Equal(fields[1].([]byte), newRun[:]) || !bytes.Equal(fields[3].([]byte), newRound[:]) ||
		fields[4] != "slot-a" || fields[5] != uint64(0) || fields[8] != uint64(0) || !sameCanonical(fields[9], int64(40)) {
		t.Fatal("remint altered frozen round/slot semantics")
	}
	source := fork57ID(0x90)
	if !bytes.Equal(history.fields[1].([]byte), source[:]) {
		t.Fatal("source dispatch mutated")
	}
}

func TestFork57ForeignRunDispatchCannotPassAuthenticatedHistory(t *testing.T) {
	b, err := policy.Load("../../../../docs/architecture/canonical-codec-v1-bundle")
	if err != nil {
		t.Fatal(err)
	}
	own, foreign, roundID, cycleID := fork57ID(0x41), fork57ID(0x42), fork57ID(0x43), fork57ID(0x44)
	id, _, err := identity.New(b).Sum("cote.csf.id.dispatch", "cote.csf.schema.dispatch-id-preimage", 1,
		[]any{foreign[:], cycleID[:], roundID[:], "slot-a", uint64(0)})
	if err != nil {
		t.Fatal(err)
	}
	fields := []any{id[:], foreign[:], cycleID[:], roundID[:], "slot-a", uint64(0),
		[]any{"school", "agent-a"}, []any{int64(40), uint64(0)}, uint64(0), int64(40), []any{uint64(0)}}
	raw, err := codec.CanonicalPayload(fields)
	if err != nil {
		t.Fatal(err)
	}
	round := []any{roundID[:], own[:], "source", []any{"school", "agent-a"}, []any{"slot-a"}, []any{int64(40), uint64(0)}}
	_, err = authenticateForkSlotDispatchesV2(&genesis.Genesis{RunID: domain.ID(own)},
		[]ports.CanonicalRecord{{Identity: [32]byte(id), Canonical: raw}},
		map[string][]any{string(roundID[:]): round}, map[string]any{string(roundID[:]): round[5]},
		map[uint64]snapshotClock{0: {Instant: 40, LastOrdinal: -1}}, b)
	if err == nil {
		t.Fatal("cross-run dispatch was authenticated under local source run")
	}
}

// Pure helpers never drop unparseable canonical bytes. Valid ingress envelopes
// are six-element CBOR arrays and stay classified as non-slot records.
func TestFork57RejectsCorruptArchivedCBORButPermitsIngressEnvelope(t *testing.T){
 broken:=ports.CanonicalRecord{Identity:fork57ID(0x55),Canonical:[]byte{0xff}}
 if _,err:=authenticateForkSlotDispatchesV2(nil,[]ports.CanonicalRecord{broken},nil,nil,nil,nil);err==nil{
  t.Fatal("invalid CBOR was silently skipped in source dispatch authentication")
 }
 if err:=attachForkSlotOutcomesV2([]ports.CanonicalRecord{broken},nil);err==nil{
  t.Fatal("invalid CBOR was silently skipped during receipt linkage")
 }
 env,err:=codec.CanonicalBytes("cote.csf.record.source-ingress-receipt",
  "cote.csf.schema.source-ingress-receipt",1,[]any{})
 if err!=nil{t.Fatal(err)}
 valid:=ports.CanonicalRecord{Canonical:env}
 if histories,err:=authenticateForkSlotDispatchesV2(nil,[]ports.CanonicalRecord{valid},nil,nil,nil,nil);err!=nil||len(histories)!=0{
  t.Fatal("legitimate ingress envelope was mistaken for an invalid dispatch:",err)
 }
 if err:=attachForkSlotOutcomesV2([]ports.CanonicalRecord{valid},nil);err!=nil{
  t.Fatal("ingress envelope was not ignored by the slot receipt collector:",err)
 }
}

func TestFork57DuplicateAuthenticDispatchMustNotOverwriteHistory(t *testing.T){
 v1,err:=policy.Load("../../../../docs/architecture/canonical-codec-v1-bundle")
 if err!=nil{t.Fatal(err)}
 run,round:=fork57ID(0x61),fork57ID(0x62)
 cycle,_,err:=identity.New(v1).Sum("cote.csf.id.cycle","cote.csf.schema.cycle-id-preimage",1,
  []any{run[:],[]any{int64(40),uint64(0)}})
 if err!=nil{t.Fatal(err)}
 id,_,err:=identity.New(v1).Sum("cote.csf.id.dispatch","cote.csf.schema.dispatch-id-preimage",1,
  []any{run[:],cycle[:],round[:],"slot-a",uint64(0)})
 if err!=nil{t.Fatal(err)}
 actor:=[]any{"school","agent-a"}
 coordinate:=[]any{int64(40),uint64(0)}
 f:=[]any{id[:],run[:],cycle[:],round[:],"slot-a",uint64(0),actor,coordinate,uint64(0),int64(40),[]any{uint64(0)}}
 raw,err:=codec.CanonicalPayload(f);if err!=nil{t.Fatal(err)}
 record:=ports.CanonicalRecord{Identity:[32]byte(id),Canonical:raw}
 projectedRound:=[]any{round[:],run[:],"source",actor,[]any{"slot-a"},coordinate}
 args:=[]ports.CanonicalRecord{record}
 parent:=&genesis.Genesis{RunID:domain.ID(run),InitialInstant:40}
 roster:=map[string][]any{string(round[:]):projectedRound}
 coords:=map[string]any{string(round[:]):coordinate}
 clocks:=map[uint64]snapshotClock{0:{Instant:40,LastOrdinal:-1}}
 one,err:=authenticateForkSlotDispatchesV2(parent,args,roster,coords,clocks,v1)
 if err!=nil||len(one)!=1{t.Fatal("valid historical dispatch rejected:",err)}
 _,err=authenticateForkSlotDispatchesV2(parent,append(args,record),roster,coords,clocks,v1)
 if err==nil{t.Fatal("duplicate historical dispatch overwritten without rejection")}
}

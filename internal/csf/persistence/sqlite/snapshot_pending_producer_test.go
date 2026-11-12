package sqlite

import (
	"bytes"
	"strings"
	"testing"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
)

func snapshot57SetPartition(p *snapshotPendingPartitions, index int, roster []any) {
	switch index {
	case 0:
		p.occurrences = roster
	case 1:
		p.triggerRuntime = roster
	case 2:
		p.triggerActivations = roster
	case 3:
		p.sourceClosures = roster
	case 4:
		p.rounds = roster
	case 5:
		p.dispatches = roster
	case 6:
		p.responses = roster
	case 7:
		p.pendingInputs = roster
	case 8:
		p.perceptionTasks = roster
	case 9:
		p.perceptionCompletions = roster
	default:
		panic("invalid test partition index")
	}
}

// The producer's output must be byte-for-byte equivalent to the old ordered
// [10][]any, including the position of input-ref kind 0 and response kind 23.
func TestSnapshot57ProducerAllTenPartitionsPreserveCanonicalBytes(t *testing.T) {
	fixture := pending57Parts()
	p := newSnapshotPendingPartitions()
	for i, raw := range fixture {
		snapshot57SetPartition(&p, i, raw.([]any))
	}
	ordered, err := p.canonicalOrder()
	if err != nil {
		t.Fatal(err)
	}
	if len(ordered) != 10 {
		t.Fatalf("got %d normative partitions", len(ordered))
	}
	got, err := codec.CanonicalPayload(ordered)
	if err != nil {
		t.Fatal(err)
	}
	want, err := codec.CanonicalPayload(fixture)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("producer changed partition order or canonical bytes")
	}
	// The P1 decoder independently validates the normative wire shape.
	decoded, err := decodeSnapshotPendingRosters(pending57Snapshot(t, ordered))
	if err != nil {
		t.Fatal("P1 consumer rejected P3 producer:", err)
	}
	if len(decoded.Rounds) != 1 || len(decoded.Responses) != 2 ||
		len(decoded.PendingInputs) != 5 || len(decoded.PerceptionCompletions) != 1 {
		t.Fatal("producer/consumer partition membership differs")
	}
}

func TestSnapshot57ProducerRequiresAllTenPresentEmptyArrays(t *testing.T) {
	p := newSnapshotPendingPartitions()
	ordered, err := p.canonicalOrder()
	if err != nil {
		t.Fatal(err)
	}
	if len(ordered) != 10 {
		t.Fatal("empty producer omitted a normative partition")
	}
	for i, raw := range ordered {
		r, ok := raw.([]any)
		if !ok || r == nil || len(r) != 0 {
			t.Fatalf("empty roster %d encoded as null or non-array", i)
		}
	}
	if _, err := decodeSnapshotPendingRosters(pending57Snapshot(t, ordered)); err != nil {
		t.Fatal("P1 decoder rejected present empty rosters:", err)
	}
	for index := 0; index < 10; index++ {
		fault := newSnapshotPendingPartitions()
		snapshot57SetPartition(&fault, index, nil)
		if _, err := fault.canonicalOrder(); err == nil || !strings.Contains(err.Error(), pendingSnapshotPartSpecs[index].name) {
			t.Fatalf("missing roster %d not rejected by name: %v", index, err)
		}
	}
}

func TestSnapshot57ProducerRejectsEveryOversizedRoster(t *testing.T) {
	// Fail before encoding any oversized data; max differs for closures.
	for index, spec := range pendingSnapshotPartSpecs {
		t.Run(spec.name, func(t *testing.T) {
			p := newSnapshotPendingPartitions()
			snapshot57SetPartition(&p, index, make([]any, spec.max+1))
			if _, err := p.canonicalOrder(); err == nil {
				t.Fatal("oversized producer roster accepted")
			}
		})
	}
}

func snapshot57Ref(kind uint64, id byte) []any {
	return []any{kind, pending57ID(id), pending57ID(id + 64)}
}

func TestSnapshot57LiveSourceProjectionPreservesMembershipAndOrder(t *testing.T) {
	live := map[[32]byte]bool{}
	for _, byteID := range []byte{2, 5} {
		var id [32]byte
		id[0] = byteID
		live[id] = true
	}
	occurrences := []any{
		snapshot57Ref(6, 1), snapshot57Ref(6, 2), snapshot57Ref(6, 3),
		snapshot57Ref(6, 5),
	}
	active, inputs, err := retainLiveSnapshotSources(occurrences, 2, live)
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 2 || len(inputs) != 2 {
		t.Fatal("source membership was not filtered against live clock")
	}
	for index, id := range []byte{2, 5} {
		ref := active[index].([]any)
		unit := inputs[index].([]any)
		if ref[0] != uint64(6) || unit[0] != uint64(2) ||
			ref[1].([]byte)[0] != id || unit[1].([]byte)[0] != id ||
			!bytes.Equal(ref[2].([]byte), unit[2].([]byte)) {
			t.Fatal("pending source filter changed ID, digest or order")
		}
	}
	activations := []any{snapshot57Ref(7, 5), snapshot57Ref(7, 1)}
	active, inputs, err = retainLiveSnapshotSources(activations, 3, live)
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 1 || len(inputs) != 1 ||
		inputs[0].([]any)[0] != uint64(3) || inputs[0].([]any)[1].([]byte)[0] != 5 {
		t.Fatal("trigger activation membership/kind changed")
	}
}

func TestSnapshot57LiveSourcesRejectWrongKindIdentityDigestAndShape(t *testing.T) {
	for _, tc := range []struct {
		name string
		refs []any
		kind uint64
	}{
		{"not-array", []any{uint64(1)}, 2},
		{"short-reference", []any{[]any{uint64(6), pending57ID(1)}}, 2},
		{"wrong-causal-kind", []any{snapshot57Ref(7, 2)}, 2},
		{"short-identity", []any{[]any{uint64(6), []byte{1}, pending57ID(60)}}, 2},
		{"short-digest", []any{[]any{uint64(6), pending57ID(2), []byte{1}}}, 2},
		{"wrong-activation-kind", []any{snapshot57Ref(6, 2)}, 3},
		{"unsupported-input-kind", []any{snapshot57Ref(6, 2)}, 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, err := retainLiveSnapshotSources(tc.refs, tc.kind, nil); err == nil {
				t.Fatal("invalid source reference accepted")
			}
		})
	}
	active, inputs, err := retainLiveSnapshotSources([]any{}, 2, nil)
	if err != nil || active == nil || inputs == nil || len(active) != 0 || len(inputs) != 0 {
		t.Fatal("empty live source roster must be present and empty:", err)
	}
}

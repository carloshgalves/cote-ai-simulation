package perception

import (
    "bytes"
    "testing"

    "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
)

func TestEmptyCompletionUsesRegisteredDomainIdentityAndIsIdempotent(t *testing.T) {
    bundle, err := policy.Load("../../../docs/architecture/canonical-codec-v1-bundle")
    if err != nil { t.Fatal(err) }
    var run, cycle, event, task [32]byte
    run[0], cycle[0], event[0], task[0] = 1, 2, 3, 4
    fields := []any{task[:], run[:], cycle[:], event[:], uint64(0), uint64(1),
        []any{"perception-v1",uint64(1),bytes.Repeat([]byte{1},32)},
        []any{"validators-v1",uint64(1),bytes.Repeat([]byte{2},32)},
        []any{"perception-identity-v1",uint64(1),bytes.Repeat([]byte{3},32)}, uint64(0)}
    encoded, err := codec.CanonicalPayload(fields)
    if err != nil { t.Fatal(err) }
    record := ports.CanonicalRecord{Identity:task,Canonical:encoded}
    a, err := EmptyCompletion(bundle,record)
    if err != nil { t.Fatal(err) }
    b, err := EmptyCompletion(bundle,record)
    if err != nil || a.Identity != b.Identity || !bytes.Equal(a.Canonical,b.Canonical) {
        t.Fatal("same task must have identical completion bytes",err)
    }
    decoded, err := codec.StrictDecodePayload(a.Canonical)
    if err != nil { t.Fatal(err) }
    value := decoded.([]any)
    if len(value)!=5 || !bytes.Equal(value[0].([]byte),a.Identity[:]) ||
        !bytes.Equal(value[1].([]byte),task[:]) ||
        len(value[2].([]any))!=0 || len(value[3].([]any))!=0 ||
        len(value[4].([]byte))!=32 {
        t.Fatal("noncanonical or nonempty completion")
    }
    tampered := record
    tampered.Identity[0]^=1
    if _,err:=EmptyCompletion(bundle,tampered);err==nil {
        t.Fatal("accepted task identity mismatch")
    }
    nonPending := append([]any(nil),fields...)
    nonPending[9]=uint64(1)
    raw,err:=codec.CanonicalPayload(nonPending)
    if err!=nil{t.Fatal(err)}
    if _,err:=EmptyCompletion(bundle,ports.CanonicalRecord{Identity:task,Canonical:raw});err==nil{
        t.Fatal("completed task accepted as pending")
    }
    if _,err:=EmptyCompletion(nil,record);err==nil {t.Fatal("missing bundle accepted")}
}

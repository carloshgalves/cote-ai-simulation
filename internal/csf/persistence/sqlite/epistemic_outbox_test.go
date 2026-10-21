package sqlite

import (
    "bytes"
    "context"
    "testing"

    "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/perception"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
)

func TestEmptyEpistemicBarrierFailsClosedOnPendingAndCorruptCompletion(t *testing.T) {
    ctx := context.Background()
    bundle,err:=policy.Load("../../../../docs/architecture/canonical-codec-v1-bundle")
    if err!=nil{t.Fatal(err)}
    store,err:=Open(ctx,t.TempDir()+"/epistemic.db")
    if err!=nil{t.Fatal(err)}
    defer store.Close()
    var run,cycle,event,task [32]byte
    run[0],cycle[0],event[0],task[0]=1,2,3,4
    raw,err:=codec.CanonicalPayload([]any{task[:],run[:],cycle[:],event[:],
        uint64(0),uint64(1),
        []any{"perception-v1",uint64(1),bytes.Repeat([]byte{1},32)},
        []any{"validators-v1",uint64(1),bytes.Repeat([]byte{2},32)},
        []any{"perception-identity-v1",uint64(1),bytes.Repeat([]byte{3},32)},uint64(0)})
    if err!=nil{t.Fatal(err)}
    _,err=store.db.ExecContext(ctx,"INSERT INTO causal_outbox(id,canonical) VALUES(?,?)",task[:],raw)
    if err!=nil{t.Fatal(err)}
    if err:=store.CheckEmptyPerceptionBarrier(ctx,run,0,bundle);err!=nil{
        t.Fatal("future task incorrectly blocked earlier revision",err)
    }
    if err:=store.CheckEmptyPerceptionBarrier(ctx,run,1,bundle);err==nil{
        t.Fatal("pending task failed to block barrier")
    }
    if _,err:=store.CompleteInternalPerception(ctx,task,bundle);err==nil{
        t.Fatal("uncommitted task accepted as completion")
    }
    completion,err:=perception.EmptyCompletion(bundle,ports.CanonicalRecord{Identity:task,Canonical:raw})
    if err!=nil{t.Fatal(err)}
    _,err=store.db.ExecContext(ctx,"INSERT INTO causal_outbox(id,canonical) VALUES(?,?)",
        completion.Identity[:],completion.Canonical)
    if err!=nil{t.Fatal(err)}
    if err:=store.CheckEmptyPerceptionBarrier(ctx,run,1,bundle);err!=nil{
        t.Fatal("durable completion should release empty barrier",err)
    }
    wrong:=bytes.Clone(completion.Canonical)
    wrong[len(wrong)-1]^=1
    _,err=store.db.ExecContext(ctx,"UPDATE causal_outbox SET canonical=? WHERE id=?",wrong,completion.Identity[:])
    if err!=nil{t.Fatal(err)}
    if err:=store.CheckEmptyPerceptionBarrier(ctx,run,1,bundle);err==nil{
        t.Fatal("corrupted completion released barrier")
    }
}

func TestSecretWorkerCannotCompleteUnknownTaskOrForgeObserver(t *testing.T){
    ctx:=context.Background()
    store,err:=Open(ctx,t.TempDir()+"/missing.db")
    if err!=nil{t.Fatal(err)}
    defer store.Close()
    bundle,err:=policy.Load("../../../../docs/architecture/canonical-codec-v1-bundle")
    if err!=nil{t.Fatal(err)}
    var id [32]byte
    id[0]=77
    if _,err:=store.CompleteInternalPerception(ctx,id,bundle);err==nil {
        t.Fatal("unknown task accepted")
    }
}

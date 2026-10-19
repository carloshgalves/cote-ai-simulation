package memory

import (
    "context"
    "testing"

    "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
)

func TestMemoryAdmissionGuardsVerifyCanonicalAuthority(t *testing.T) {
    ctx:=context.Background()
    store:=New()
    var run,other,cycle,sourceID,roundID,fenceID [32]byte
    run[0],other[0],cycle[0],sourceID[0],roundID[0],fenceID[0]=1,2,3,4,5,6
    fenceBytes,err:=codec.CanonicalPayload([]any{run[:],cycle[:],uint64(0),nil,int64(100),uint64(1)})
    if err!=nil{t.Fatal(err)}
    if err:=store.AppendFenceIfPrefix(ctx,ports.CanonicalRecord{Identity:fenceID,Canonical:fenceBytes},nil);err!=nil{t.Fatal(err)}
    sourceBytes,err:=codec.CanonicalPayload([]any{"source-declaration-v1",run[:],"late-source"})
    if err!=nil{t.Fatal(err)}
    source:=ports.CanonicalRecord{Identity:sourceID,Canonical:sourceBytes}
    if err:=store.AppendSourceDeclaration(ctx,other,source);err==nil{t.Fatal("cross-run source guard bypass")}
    if err:=store.AppendSourceDeclaration(ctx,run,source);err==nil{t.Fatal("source admitted after seal")}
    roundBytes,err:=codec.CanonicalPayload([]any{roundID[:],run[:],"schedule",[]any{"school","actor"},[]any{"slot"},[]any{int64(100),uint64(1)},[]any{uint64(2),[]any{cycle[:],fenceID[:]}},uint64(0),uint64(3),uint64(0),nil})
    if err!=nil{t.Fatal(err)}
    round:=ports.CanonicalRecord{Identity:roundID,Canonical:roundBytes}
    if err:=store.AppendRoundDeclaration(ctx,run,101,0,round);err==nil{t.Fatal("future coordinate argument bypass")}
    if err:=store.AppendRoundDeclaration(ctx,other,100,1,round);err==nil{t.Fatal("cross-run round guard bypass")}
    if err:=store.AppendRoundDeclaration(ctx,run,100,1,round);err==nil{t.Fatal("sealed round admitted")}
    if store.Len()!=0{t.Fatal("bypassed guard mutated input prefix")}
    if _,err:=store.GetFence(ctx,fenceID);err!=nil{t.Fatalf("durable fence missing: %v",err)}
}

func TestMemoryDirectFenceWriteDenied(t *testing.T) {
    store:=New()
    var id [32]byte;id[0]=9
    if err:=store.AppendFence(context.Background(),ports.CanonicalRecord{Identity:id,Canonical:[]byte{1,2,3}});err==nil {t.Fatal("direct fence write succeeded")}
    if _,err:=store.GetFence(context.Background(),id);err==nil {t.Fatal("direct fence write persisted")}
}

package genesis

import (
    "bytes"
    "testing"

    "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/reducer"
)

func registeredCompositeGenesis(t *testing.T,b *policy.Bundle)[]byte{
    t.Helper()
    raw:=registeredPhysicalGenesis(t,b)
    env,err:=codec.StrictDecodeEnvelope(raw)
    if err!=nil{t.Fatal(err)}
    fields:=append([]any(nil),env.Value.([]any)...)
    bodies,err:=reducer.DecodePhysical(fields[16])
    if err!=nil{t.Fatal(err)}
    composite,_,err:=reducer.EncodeComposite(reducer.Stock{Available:1},bodies)
    if err!=nil{t.Fatal(err)}
    fields[16]=composite
    encoded,err:=codec.CanonicalBytes(env.DomainTag,env.SchemaID,env.SchemaVersion,fields)
    if err!=nil{t.Fatal(err)}
    return encoded
}

func TestGenesisCompositeRequiresPinnedPhysicalManifest(t *testing.T){
    installed:=registeredPhysicalBundle(t)
    raw:=registeredCompositeGenesis(t,installed)
    g,err:=ValidateCanonical(raw,installed)
    if err!=nil||g==nil{t.Fatalf("Genesis refused sealed stock+physical typed WorldState: %v",err)}
    base,err:=policy.Load(bundlePath)
    if err!=nil{t.Fatal(err)}
    if _,err:=ValidateCanonical(raw,base);err==nil{
        t.Fatal("unregistered bundle admitted stock+private physical state")
    }
    env,err:=codec.StrictDecodeEnvelope(raw);if err!=nil{t.Fatal(err)}
    mutate:=func(change func([]any))[]byte{
        p:=append([]any(nil),env.Value.([]any)...)
        change(p)
        out,err:=codec.CanonicalBytes(env.DomainTag,env.SchemaID,env.SchemaVersion,p)
        if err!=nil{t.Fatal(err)}
        return out
    }
    cases:=map[string]func([]any){
        "no physical manifest":func(p []any){p[12]=[]any{}},
        "wrong manifest hash":func(p []any){p[12]=[]any{[]any{"cote.embodiment.physical.v1",bytes.Repeat([]byte{0x17},32)}}},
        "physical source missing":func(p []any){p[13]=[]any{"school-events"}},
        "unknown schema version":func(p []any){p[15]=[]any{[]any{reducer.StockSchemaID,uint64(2)}}},
        "missing stock resource":func(p []any){
            bodies,err:=reducer.DecodePhysical(p[16]);if err!=nil{t.Fatal(err)}
            x,_,err:=reducer.EncodePhysical(bodies);if err!=nil{t.Fatal(err)}
            p[16]=x
        },
    }
    // Standalone physical is legal when registered; composite is compulsory
    // only for this B resource+physical scenario, not all physical Geneses.
    delete(cases,"missing stock resource")
    for name,f:=range cases{t.Run(name,func(t *testing.T){
        if _,err:=ValidateCanonical(mutate(f),installed);err==nil{
            t.Fatal("Genesis accepted missing/mismatched signed physical authority")
        }
    })}
    if _,err:=ValidateCanonical(raw,installed);err!=nil{
        t.Fatal("signed physical+resource Genesis failed deterministically",err)
    }
}

func TestGenesisCompositeRefusesForgedInnerStateEvenWithValidOuterDigest(t *testing.T){
    b:=registeredPhysicalBundle(t)
    raw:=registeredCompositeGenesis(t,b)
    env,err:=codec.StrictDecodeEnvelope(raw);if err!=nil{t.Fatal(err)}
    p:=append([]any(nil),env.Value.([]any)...)
    typed:=append([]any(nil),p[16].([]any)...)
    typed[3]=bytes.Repeat([]byte{0x22},32)
    p[16]=typed
    forged,err:=codec.CanonicalBytes(env.DomainTag,env.SchemaID,env.SchemaVersion,p)
    if err!=nil{t.Fatal(err)}
    if _,err:=ValidateCanonical(forged,b);err==nil{
        t.Fatal("Genesis accepted malicious composite digest while policy registered")
    }
}

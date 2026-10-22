package genesis

import (
 "bytes"
 "context"
 "testing"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/fork"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

func TestCSFRV18V2GenesisPinsNormativeExecutablePolicy(t *testing.T){
 v1,err:=policy.Load(bundlePath);if err!=nil{t.Fatal(err)}
 v2,err:=policy.LoadV2("../../../docs/architecture/canonical-codec-v2-bundle")
 if err!=nil{t.Fatal(err)}
 original:=genesisFixture(t)
 env,err:=codec.StrictDecodeEnvelope(original);if err!=nil{t.Fatal(err)}
 f:=env.Value.([]any)
 v2ID,v2Version,v2Hash,v2Schema,err:=v2.CodecIdentity();if err!=nil{t.Fatal(err)}
 f[5]=v2ID;f[6]=uint64(v2Version);f[10]=bytes.Clone(v2Hash[:]);f[11]=bytes.Clone(v2Schema[:])
 raw,err:=codec.CanonicalBytes(env.DomainTag,env.SchemaID,env.SchemaVersion,f)
 if err!=nil{t.Fatal(err)}
 child,err:=ValidateCanonical(raw,v2)
 if err!=nil{t.Fatal("V2 Genesis should execute under verified V2 policy:",err)}
 if child.RunID==([32]byte{})||child.Digest==([32]byte{}){t.Fatal("V2 Genesis causal digest not derived")}
 if _,err:=ValidateCanonical(raw,v1);err==nil{t.Fatal("V1 must reject V2 Genesis policy pin")}
 f[10]=bytes.Clone(v1.CodecPolicyHash[:])
 tampered,err:=codec.CanonicalBytes(env.DomainTag,env.SchemaID,env.SchemaVersion,f)
 if err!=nil{t.Fatal(err)}
 if _,err:=ValidateCanonical(tampered,v2);err==nil{
  t.Fatal("V2 accepted V1 policy hash as an authority")
 }
 // CSFHREF continues to authenticate V1 by V1, never by V2.
 historical,err:=fork.NewSnapshotReference(v1,original)
 if err!=nil{t.Fatal(err)}
 refRaw,err:=fork.Encode(historical);if err!=nil{t.Fatal(err)}
 registry,err:=fork.NewInstalledPolicyRegistry(v1,v2);if err!=nil{t.Fatal(err)}
 _,origin,err:=fork.VerifyOriginWithResolver(context.Background(),refRaw,original,registry)
 // A Genesis record is not a valid snapshot: the resolver is required to
 // reject it after resolving the exact policy identity.
 if err==nil||origin!=nil{t.Fatal("forged snapshot reference accepted with Genesis bytes")}
 resolved,err:=registry.ResolveOriginPolicy(context.Background(),historical.OriginPolicyID,historical.OriginPolicyHash)
 if err!=nil||resolved!=v1{t.Fatalf("registry failed V1 historical origin lookup: %v",err)}
}

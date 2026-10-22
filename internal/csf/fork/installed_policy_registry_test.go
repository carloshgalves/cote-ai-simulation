package fork

import (
 "context"
 "testing"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

func TestCSFRV18InstalledOriginPolicyRegistryExactIdentity(t *testing.T) {
 b,err:=policy.Load("../../../docs/architecture/canonical-codec-v1-bundle")
 if err!=nil{t.Fatal(err)}
 registry,err:=NewInstalledPolicyRegistry(b)
 if err!=nil{t.Fatal(err)}
 id:=policy.CodecPolicyID
 got,err:=registry.ResolveOriginPolicy(context.Background(),id,b.CodecPolicyHash)
 if err!=nil||got!=b{t.Fatal("registered policy was not resolved by its pinned hash",err)}
 if _,err:=registry.ResolveOriginPolicy(context.Background(),"cote.csf.codec.cbor-det.v2",b.CodecPolicyHash);err==nil{
  t.Fatal("future policy identity silently reused V1 bundle")
 }
 unknown:=b.CodecPolicyHash;unknown[0]^=0x01
 if _,err:=registry.ResolveOriginPolicy(context.Background(),id,unknown);err==nil{
  t.Fatal("registry accepted a different codec manifest under V1 identity")
 }
 if _,err:=NewInstalledPolicyRegistry(b,b);err==nil{
  t.Fatal("registry accepted duplicate policy installation")
 }
 if _,err:=NewInstalledPolicyRegistry(nil);err==nil{
  t.Fatal("registry accepted nil origin policy")
 }
 if _,err:=(&InstalledPolicyRegistry{}).ResolveOriginPolicy(context.Background(),id,b.CodecPolicyHash);err==nil{
  t.Fatal("registry resolved an absent historical policy")
 }
}

func TestCSFRV18InstalledPolicyRegistryDetectsIdentityMutation(t *testing.T) {
 b,err:=policy.Load("../../../docs/architecture/canonical-codec-v1-bundle")
 if err!=nil{t.Fatal(err)}
 registry,err:=NewInstalledPolicyRegistry(b)
 if err!=nil{t.Fatal(err)}
 originalHash:=b.CodecPolicyHash
 b.CodecPolicyHash[0]^=0x01
 if _,err:=registry.ResolveOriginPolicy(context.Background(),policy.CodecPolicyID,originalHash);err==nil{
  t.Fatal("mutated installed policy was silently accepted")
 }
}

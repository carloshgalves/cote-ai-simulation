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


func TestCSFRV18InstalledPolicyRegistryRejectsPostInstallContractMutation(t *testing.T) {
 mutations:=[]struct{
  name string
  change func(*policy.Bundle)
 }{
  {"profile",func(b *policy.Bundle){b.Profile["codec_version"]=float64(999)}},
  {"operations",func(b *policy.Bundle){b.Registries["domain_operations"]=[]any{}}},
  {"schemas",func(b *policy.Bundle){b.Registries["schemas"]=[]any{}}},
  {"transitions",func(b *policy.Bundle){b.Transitions["required_genesis_policy_roles"]=[]any{}}},
  {"schema hash",func(b *policy.Bundle){b.SchemaHash[0]^=1}},
  {"conformance hash",func(b *policy.Bundle){b.SuiteHash[0]^=1}},
  {"directory",func(b *policy.Bundle){b.Directory="../different-bundle"}},
 }
 for _,tc:=range mutations {
  t.Run(tc.name,func(t *testing.T){
   b,err:=policy.Load("../../../docs/architecture/canonical-codec-v1-bundle")
   if err!=nil{t.Fatal(err)}
   registry,err:=NewInstalledPolicyRegistry(b)
   if err!=nil{t.Fatal(err)}
   original:=b.CodecPolicyHash
   tc.change(b)
   if _,err:=registry.ResolveOriginPolicy(context.Background(),policy.CodecPolicyID,original);err==nil{
    t.Fatal("mutated installed policy still resolved as immutable authority")
   }
  })
 }
}

func TestCSFRV18InstalledPolicyRegistrySealsNestedRegistryContent(t *testing.T){
 b,err:=policy.Load("../../../docs/architecture/canonical-codec-v1-bundle")
 if err!=nil{t.Fatal(err)}
 registry,err:=NewInstalledPolicyRegistry(b)
 if err!=nil{t.Fatal(err)}
 // A nested policy registry entry can change without changing either lookup
 // field; such a mutation must still block historical checkpoint resolution.
 entries,ok:=b.Registries["domain_operations"].([]any)
 if !ok||len(entries)==0{t.Fatal("missing operation registry")}
 entry,ok:=entries[0].([]any)
 if !ok||len(entry)!=3{t.Fatal("unexpected operation shape")}
 entry[0]="cote.csf.invalid.replacement"
 if _,err:=registry.ResolveOriginPolicy(context.Background(),policy.CodecPolicyID,b.CodecPolicyHash);err==nil{
  t.Fatal("nested mutated registry was accepted as installed policy")
 }
}

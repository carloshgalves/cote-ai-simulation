package policy

import (
 "testing"
)

func TestCSFRV18CodecIdentityUsesOnlyInstalledPinnedAuthority(t *testing.T){
 b,err:=Load(bundlePath)
 if err!=nil{t.Fatal(err)}
 id,version,codecHash,schemaHash,err:=b.CodecIdentity()
 if err!=nil{t.Fatal(err)}
 if id!=CodecPolicyID||version!=CodecVersion||
  codecHash!=b.CodecPolicyHash||schemaHash!=b.SchemaHash{
  t.Fatal("version-aware codec identity differs from pinned V1 authority")
 }
 // Exported fields are not loader provenance. A bundle assembled or copied
 // without a private installation seal must not act as a V2 policy.
 forged:=&Bundle{Profile:map[string]any{"codec_policy_id":"cote.csf.codec.cbor-det.v2","codec_version":float64(2)}}
 if _,_,_,_,err:=forged.CodecIdentity();err==nil{
  t.Fatal("unverified fabricated V2 was accepted as executable policy")
 }
 b.Profile["codec_version"]=float64(2)
 if _,_,_,_,err:=b.CodecIdentity();err==nil{
  t.Fatal("mutated installed policy returned a versioned identity")
 }
}

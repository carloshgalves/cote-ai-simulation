package policy

import (
 "os"
 "path/filepath"
 "testing"
)

const v2BundlePath="../../../docs/architecture/canonical-codec-v2-bundle"

func TestCSFRV18V2NormativeBundleLoadsAndIsIndependent(t *testing.T){
 v1,err:=Load(bundlePath);if err!=nil{t.Fatal(err)}
 v2,err:=LoadV2(v2BundlePath);if err!=nil{t.Fatal("V2 normative loader:",err)}
 id,version,codecHash,schemaHash,err:=v2.CodecIdentity()
 if err!=nil{t.Fatal(err)}
 if id!=CodecPolicyV2ID||version!=2||codecHash==v1.CodecPolicyHash||
  schemaHash==v1.SchemaHash||v2.SuiteHash==v1.SuiteHash {
  t.Fatal("V2 not a distinct, independently signed policy")
 }
 if _,err:=LoadV2(bundlePath);err==nil{t.Fatal("V1 bundle accepted as V2")}
 if _,err:=Load(v2BundlePath);err==nil{t.Fatal("V2 bundle accepted as V1")}
 if _,err:=v2.Operation("cote.csf.digest.claim","cote.csf.schema.claim.body",1,false);err!=nil{
  t.Fatal("V2 must execute compatible causal operations:",err)
 }
 if err:=v2.VerifyInstalledBundle();err!=nil{t.Fatal(err)}
 v2.Transitions["fork_migration_v2"]=map[string]any{}
 if err:=v2.VerifyInstalledBundle();err==nil{
  t.Fatal("mutated installed V2 transition accepted")
 }
}

func TestCSFRV18V2RejectsTamperedTransitionAndContractVectors(t *testing.T){
 for _,name:=range []string{"profile.json","causal-transition-contracts.json","fork-v2-vectors.json","BUNDLE.sha256"} {
  t.Run(name,func(t *testing.T){
   dir:=t.TempDir()
   entries,err:=os.ReadDir(v2BundlePath);if err!=nil{t.Fatal(err)}
   for _,entry:=range entries {
    if entry.IsDir(){continue}
    raw,err:=os.ReadFile(filepath.Join(v2BundlePath,entry.Name()))
    if err!=nil{t.Fatal(err)}
    if entry.Name()==name{raw=append(raw,' ')}
    if err:=os.WriteFile(filepath.Join(dir,entry.Name()),raw,0600);err!=nil{t.Fatal(err)}
   }
   if _,err:=LoadV2(dir);err==nil{
    t.Fatal("V2 accepted modified normative artifact")
   }
  })
 }
}

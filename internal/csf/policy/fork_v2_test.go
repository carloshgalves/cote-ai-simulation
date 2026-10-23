package policy

import (
 "bytes"
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

func copyCSFRV18PolicyBundle(t *testing.T,src,destination string){
 t.Helper()
 if err:=os.MkdirAll(destination,0700);err!=nil{t.Fatal(err)}
 entries,err:=os.ReadDir(src);if err!=nil{t.Fatal(err)}
 for _,entry:=range entries{
  if entry.IsDir(){continue}
  original,err:=os.ReadFile(filepath.Join(src,entry.Name()))
  if err!=nil{t.Fatal(err)}
  if err:=os.WriteFile(filepath.Join(destination,entry.Name()),original,0600);err!=nil{t.Fatal(err)}
 }
}

func TestCSFRV18V2RejectsTamperedTransitionAndContractVectors(t *testing.T){
 for _,name:=range []string{"profile.json","causal-transition-contracts.json","fork-v2-vectors.json","BUNDLE.sha256","registries.json"}{
  t.Run(name,func(t *testing.T){
   root:=t.TempDir()
   copyCSFRV18PolicyBundle(t,bundlePath,filepath.Join(root,"canonical-codec-v1-bundle"))
   dir:=filepath.Join(root,"canonical-codec-v2-bundle")
   copyCSFRV18PolicyBundle(t,v2BundlePath,dir)
   if _,err:=LoadV2(dir);err!=nil{
    t.Fatalf("clean copied V2 and historical V1 fail before mutation: %v",err)
   }
   raw,err:=os.ReadFile(filepath.Join(dir,name))
   if err!=nil{t.Fatal(err)}
   if name=="BUNDLE.sha256"{
    needle:=[]byte("codec_policy_hash ")
    index:=bytes.Index(raw,needle)
    if index<0{t.Fatal("V2 receipt missing codec policy hash")}
    pos:=index+len(needle)
    if raw[pos]=='0'{raw[pos]='f'}else{raw[pos]='0'}
   }else{raw=append(raw,' ')}
   if err:=os.WriteFile(filepath.Join(dir,name),raw,0600);err!=nil{t.Fatal(err)}
   if _,err:=LoadV2(dir);err==nil{
    t.Fatal("V2 accepted a changed signed artifact with valid historical V1 installed")
   }
  })
 }
}

func TestCSFRV18SignedRuntimeProfileCaptureCannotSwapAfterManifestCheck(t *testing.T){
 tests:=[]struct{name,dir,domain string;load func(string)(*Bundle,error)}{
  {"V1",bundlePath,"cote.csf.bundle.codec-policy.v1",Load},
  {"V2",v2BundlePath,"cote.csf.bundle.codec-policy.v2",LoadV2},
 }
 for _,tc:=range tests{
  t.Run(tc.name,func(t *testing.T){
   installed,err:=tc.load(tc.dir);if err!=nil{t.Fatal(err)}
   temp:=t.TempDir()
   for _,name:=range []string{"policy-manifest.json","profile.json"}{
    raw,err:=os.ReadFile(filepath.Join(tc.dir,name));if err!=nil{t.Fatal(err)}
    if err:=os.WriteFile(filepath.Join(temp,name),raw,0600);err!=nil{t.Fatal(err)}
   }
   var parsed map[string]any
   if err:=decodePinnedJSON(temp,"policy-manifest.json",tc.domain,
    installed.CodecPolicyHash,"profile.json",&parsed);err!=nil{
    t.Fatal("authentic captured runtime policy rejected:",err)
   }
   raw,err:=os.ReadFile(filepath.Join(temp,"profile.json"));if err!=nil{t.Fatal(err)}
   if err:=os.WriteFile(filepath.Join(temp,"profile.json"),append(raw,' '),0600);err!=nil{t.Fatal(err)}
   if err:=decodePinnedJSON(temp,"policy-manifest.json",tc.domain,
    installed.CodecPolicyHash,"profile.json",&parsed);err==nil{
    t.Fatal("runtime profile swapped after signed manifest accepted")
   }
   if err:=os.WriteFile(filepath.Join(temp,"profile.json"),raw,0600);err!=nil{t.Fatal(err)}
   manifest,err:=os.ReadFile(filepath.Join(temp,"policy-manifest.json"));if err!=nil{t.Fatal(err)}
   if err:=os.WriteFile(filepath.Join(temp,"policy-manifest.json"),append(manifest,' '),0600);err!=nil{t.Fatal(err)}
   if err:=decodePinnedJSON(temp,"policy-manifest.json",tc.domain,
    installed.CodecPolicyHash,"profile.json",&parsed);err==nil{
    t.Fatal("runtime policy manifest swapped after verification accepted")
   }
  })
 }
}

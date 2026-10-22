package policy

import (
 "bytes"
 "os"
 "path/filepath"
 "reflect"
 "testing"
)

// Genesis policy binding comes from the verified, cached artifact; reading a
// changed fixtures.json after installation would silently change historical
// authorities without changing the installed policy manifest identity.
func TestCSFRV18VerifiedPolicyFixturesAreFrozenAfterLoad(t *testing.T) {
 destination:=t.TempDir()
 entries,err:=os.ReadDir(bundlePath)
 if err!=nil{t.Fatal(err)}
 for _,entry:=range entries {
  if entry.IsDir(){continue}
  raw,err:=os.ReadFile(filepath.Join(bundlePath,entry.Name()))
  if err!=nil{t.Fatal(err)}
  if err:=os.WriteFile(filepath.Join(destination,entry.Name()),raw,0600);err!=nil{t.Fatal(err)}
 }
 loaded,err:=Load(destination)
 if err!=nil{t.Fatal(err)}
 before,err:=loaded.GenesisPolicyRefs()
 if err!=nil{t.Fatal(err)}
 if len(before)==0{t.Fatal("fixture policy roster missing")}
 saved,err:=os.ReadFile(filepath.Join(destination,"fixtures.json"))
 if err!=nil{t.Fatal(err)}
 if err:=os.WriteFile(filepath.Join(destination,"fixtures.json"),[]byte("{}"),0600);err!=nil{t.Fatal(err)}
 after,err:=loaded.GenesisPolicyRefs()
 if err!=nil{t.Fatalf("runtime reopened tampered fixture file: %v",err)}
 if !reflect.DeepEqual(before,after){t.Fatal("Genesis policy refs changed after verified installation")}
 if err:=loaded.VerifyInstalledBundle();err!=nil{t.Fatalf("immutable loaded policy lost seal after external file edit: %v",err)}
 if _,err:=Load(destination);err==nil{t.Fatal("fresh loader accepted tampered fixture")}
 // External writes do not affect the frozen runtime, and a fresh verified
 // load is available again only when the original artifact is restored.
 if err:=os.WriteFile(filepath.Join(destination,"fixtures.json"),saved,0600);err!=nil{t.Fatal(err)}
 reloaded,err:=Load(destination)
 if err!=nil{t.Fatal(err)}
 if !bytes.Equal(loaded.fixtureBytes,reloaded.fixtureBytes){t.Fatal("restored fixture differs from original verified bytes")}
 // The registered reference roster must not share mutable []byte storage
 // with a caller, even after its backing fixture has been cached.
 first,err:=reloaded.GenesisPolicyRefs()
 if err!=nil{t.Fatal(err)}
 original,err:=reloaded.GenesisPolicyRefs()
 if err!=nil{t.Fatal(err)}
 first[0][0]="forged-policy-id"
 first[0][2].([]byte)[0]^=1
 stable,err:=reloaded.GenesisPolicyRefs()
 if err!=nil{t.Fatal(err)}
 if !reflect.DeepEqual(stable,original){
  t.Fatal("caller mutated cached Genesis policy references")
 }
}

func TestCSFRV18RejectsCapturedFixtureSwapEvenIfManifestRemainsValid(t *testing.T){
 folder:=t.TempDir()
 entries,err:=os.ReadDir(bundlePath);if err!=nil{t.Fatal(err)}
 for _,entry:=range entries{
  if entry.IsDir(){continue}
  raw,err:=os.ReadFile(filepath.Join(bundlePath,entry.Name()))
  if err!=nil{t.Fatal(err)}
  if err:=os.WriteFile(filepath.Join(folder,entry.Name()),raw,0600);err!=nil{t.Fatal(err)}
 }
 verified,err:=Load(folder);if err!=nil{t.Fatal(err)}
 if err:=verifyCapturedConformanceFixture(folder,verified.fixtureBytes,verified.SuiteHash);err!=nil{
  t.Fatalf("captured signed bytes rejected: %v",err)
 }
 forged:=bytes.Clone(verified.fixtureBytes)
 forged[0]^=1
 // The artifact ON DISK and manifest remain pristine, so the old
 // check (re-reading disk only) passed. The newly pinned captured-byte
 // assertion must reject this exact swap-and-restore race.
 if err:=verifyCapturedConformanceFixture(folder,forged,verified.SuiteHash);err==nil{
  t.Fatal("unsigned in-memory fixture bytes accepted because on-disk manifest was valid")
 }
}

package fork

import (
 "context"
 "crypto/sha256"
 "encoding/json"
 "errors"
 "fmt"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

// InstalledPolicyRegistry is the explicit immutable-version lookup for archived
// checkpoints. It resolves by both policy identity and manifest hash. It does
// NOT silently fall back to the child's bundle, and it never upgrades an
// existing run in place. New policy versions must first pass their own signed
// manifest/conformance loader and register a distinct bundle here.
type InstalledPolicyRegistry struct {
 entries map[policyVersion]installedPolicy
}

type installedPolicy struct {
 bundle *policy.Bundle
 fingerprint [32]byte
}

type policyVersion struct {
 id string
 hash [32]byte
}

var _ OriginPolicyResolver = (*InstalledPolicyRegistry)(nil)

// NewInstalledPolicyRegistry accepts only previously loaded policy bundles.
// The caller owns their lifetime; do not mutate their registries after load.
func NewInstalledPolicyRegistry(bundles ...*policy.Bundle) (*InstalledPolicyRegistry,error) {
 out:=&InstalledPolicyRegistry{entries:make(map[policyVersion]installedPolicy,len(bundles))}
 for _,bundle:=range bundles {
  if bundle==nil{return nil,errors.New("nil installed policy bundle")}
  id,ok:=bundle.Profile["codec_policy_id"].(string)
  if !ok||id==""||bundle.CodecPolicyHash==([32]byte{})||bundle.SchemaHash==([32]byte{}) {
   return nil,errors.New("unverified installed policy identity")
  }
  key:=policyVersion{id:id,hash:bundle.CodecPolicyHash}
  if _,exists:=out.entries[key];exists{return nil,fmt.Errorf("duplicate installed policy %q",id)}
  fp,err:=installedPolicyFingerprint(bundle)
  if err!=nil{return nil,fmt.Errorf("cannot seal installed policy %q: %w",id,err)}
  out.entries[key]=installedPolicy{bundle:bundle,fingerprint:fp}
 }
 return out,nil
}

func (r *InstalledPolicyRegistry) ResolveOriginPolicy(_ context.Context,id string,hash [32]byte)(*policy.Bundle,error) {
 if r==nil||r.entries==nil{return nil,errors.New("origin policy registry unavailable")}
 entry,exists:=r.entries[policyVersion{id:id,hash:hash}]
 if !exists{return nil,fmt.Errorf("origin policy not installed: %q",id)}
 // Detect accidental mutation/cross-registration before any decoding.
 b:=entry.bundle
 if b==nil||b.CodecPolicyHash!=hash||b.Profile["codec_policy_id"]!=id {
  return nil,errors.New("registered origin policy identity mutated")
 }
 fp,err:=installedPolicyFingerprint(b)
 if err!=nil||fp!=entry.fingerprint {
  return nil,errors.New("registered origin policy contents mutated after installation")
 }
 return b,nil
}

// installedPolicyFingerprint seals the complete parsed policy authority, not
// merely the lookup key. JSON encoding orders map keys deterministically; all
// loaded profile/registry/transition values are JSON-derived. Replacing a
// domain operation, reducer policy, schema or transition in memory must never
// change how an already installed historical bundle interprets a checkpoint.
// The bundle's manifest hashes remain independently verified by policy.Load.
func installedPolicyFingerprint(b *policy.Bundle)([32]byte,error){
 if b==nil{return [32]byte{},errors.New("nil installed bundle")}
 canonical,err:=json.Marshal(struct{
  Directory string
  Profile map[string]any
  Registries map[string]any
  Transitions map[string]any
  CodecHash [32]byte
  SchemaHash [32]byte
  SuiteHash [32]byte
 }{b.Directory,b.Profile,b.Registries,b.Transitions,b.CodecPolicyHash,b.SchemaHash,b.SuiteHash})
 if err!=nil{return [32]byte{},err}
 return sha256.Sum256(canonical),nil
}

package fork

import (
 "context"
 "errors"
 "fmt"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

// OriginPolicyResolver is deliberately keyed by the complete immutable origin
// policy identity, never by the child policy or a filesystem locator. A V2
// child must resolve V1 from its versioned installed policy registry and
// validate the parent under that exact bundle before any migration runs.
type OriginPolicyResolver interface {
 ResolveOriginPolicy(context.Context,string,[32]byte)(*policy.Bundle,error)
}

type ResolveOriginFunc func(context.Context,string,[32]byte)(*policy.Bundle,error)
func (f ResolveOriginFunc) ResolveOriginPolicy(ctx context.Context,id string,hash [32]byte)(*policy.Bundle,error){
 if f==nil{return nil,errors.New("origin policy resolver missing")}
 return f(ctx,id,hash)
}

// VerifyOriginWithResolver first rejects bad CSFHREF framing, then resolves
// the historical policy by its own id+hash, then verifies the exact snapshot
// bytes under that policy. No child bundle is accepted as a substitute.
// This handles cross-version parent authentication independently of whether
// an explicit child-schema migration has been registered.
func VerifyOriginWithResolver(ctx context.Context,framing,canonical []byte,
 resolver OriginPolicyResolver)(HistoryReference,*policy.Bundle,error){
 var empty HistoryReference
 ref,err:=Decode(framing);if err!=nil{return empty,nil,err}
 if resolver==nil{return empty,nil,errors.New("origin policy bundle not registered")}
 bundle,err:=resolver.ResolveOriginPolicy(ctx,ref.OriginPolicyID,ref.OriginPolicyHash)
 if err!=nil{return empty,nil,fmt.Errorf("resolving origin policy: %w",err)}
 if bundle==nil||bundle.CodecPolicyHash!=ref.OriginPolicyHash{
  return empty,nil,errors.New("origin resolver supplied wrong or absent immutable policy")
 }
 if err:=VerifyOriginSnapshot(ref,canonical,bundle);err!=nil{return empty,nil,err}
 return ref,bundle,nil
}

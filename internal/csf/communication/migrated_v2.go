package communication

import (
 "errors"
 "math"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
)

// ValidateMigratedOccurrenceV2 validates a newly rekeyed due record and its
// Claim/Transmission against the CHILD Genesis origin. It does not by itself
// authorize publication: an independently verified historical migration-birth
// receipt must also be durably present and linked to the parent's sent commit.
func ValidateMigratedOccurrenceV2(b *policy.Bundle,p SendPlan,rootID domain.ID,
 rootDigest domain.Digest)(domain.SimulationInstant,error){
 if b==nil{return 0,errors.New("missing V2 policy")}
 id,version,_,_,err:=b.CodecIdentity()
 if err!=nil||id!=policy.CodecPolicyV2ID||version!=2{
  return 0,errors.New("only installed policy V2 authorizes imported due schema")
 }
 claim,err:=AuthenticateClaim(b,p.Claim)
 if err!=nil{return 0,err}
 transmission,err:=AuthenticateTransmission(b,p.Transmission)
 if err!=nil{return 0,err}
 if claim.Origin.Kind!=21||transmission.Origin.Kind!=21||
  claim.Origin.ID!=rootID||transmission.Origin.ID!=rootID||
  claim.Origin.Digest!=rootDigest||transmission.Origin.Digest!=rootDigest||
  transmission.Occurrence!=domain.ID(p.Occurrence.Identity)||
  transmission.Status!=0||len(transmission.ClaimRefs)!=1||
  transmission.ClaimRefs[0].ID!=claim.ID||
  transmission.ClaimRefs[0].Digest!=claim.Digest {
  return 0,errors.New("imported Claim/Transmission not child Genesis born")
 }
 value,err:=codec.StrictDecodePayload(p.Occurrence.Canonical)
 if err!=nil{return 0,err}
 f,ok:=value.([]any)
 if !ok||len(f)!=15||!bytesEqualID(f[0],domain.ID(p.Occurrence.Identity))||
  !bytesEqualID(f[1],p.Run)||f[2]!="communication"||
  f[5]!="communication.delivery"||f[6]!=uint64(1)||f[8]!=uint64(0)||
  f[10]!=uint64(65535)||f[12]==nil||
  !sameCBOR(f[12],[]any{uint64(0)})||
  !sameCBOR(f[13],[]any{uint64(0)}) {
  return 0,errors.New("V2 imported due occurrence not canonical")
 }
 ordinal,ok:=f[11].(uint64)
 if !ok||ordinal>math.MaxUint32{return 0,errors.New("V2 imported occurrence ordinal invalid")}
 genesisOrigin:=[]any{uint64(2),[]any{rootID[:],rootDigest[:]}}
 if !sameCBOR(f[9],genesisOrigin){return 0,errors.New("V2 due origin is not child Genesis")}
 want,_,err:=identity.New(b).Sum("cote.csf.id.occurrence",
  "cote.csf.schema.occurrence-id-preimage",1,[]any{genesisOrigin,uint64(65535),ordinal})
 if err!=nil||domain.ID(want)!=domain.ID(p.Occurrence.Identity){
  return 0,errors.New("V2 due occurrence causal ID forged")
 }
 due,ok:=causalTime(f[3])
 if !ok||due<=transmission.SentAt{return 0,errors.New("V2 imported due time not after sent instant")}
 elig,ok:=f[4].([]any)
 if !ok||len(elig)!=2||elig[1]!=uint64(0)||!sameCBOR(elig[0],int64(due)){
  return 0,errors.New("V2 imported due coordinate not deterministic")
 }
 typed,ok:=f[7].([]any)
 if !ok{return 0,errors.New("V2 imported due body absent")}
 if err:=checkedTyped(typed);err!=nil{return 0,err}
 env,err:=codec.StrictDecodeEnvelope(rawBytes(typed[2]))
 if err!=nil||env.DomainTag!="cote.csf.communication.delivery.v1"||
  !sameCBOR(env.Value,[]any{transmission.ID[:],transmission.Digest[:],claim.ID[:],claim.Digest[:]}){
  return 0,errors.New("V2 imported due typed references forged")
 }
 digest,_,err:=identity.New(b).Sum("cote.csf.digest.occurrence-unit",
  "cote.csf.schema.scheduled-occurrence.body",1,f[:14])
 if err!=nil||!bytesEqualDigest(f[14],digest){
  return 0,errors.New("V2 imported due digest invalid")
 }
 return due,nil
}

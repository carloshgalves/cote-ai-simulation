package contextboundary

import (
 "bytes"
 "errors"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
)

// checkCommunicationContext admits precisely the registered delivered-claim
// projection. It never queries the global Claim ledger (which knows the
// actual sender), EventStore, world facts or producer intentions. A received
// claim is reported content, not a true fact or belief update.
func checkCommunicationContext(observation []any, actor [2]string,notice any)(string,error){
 parts,ok:=notice.([]any)
 if !ok||len(parts)!=6{return "",errors.New("communicated percept has unregistered fields")}
 if !canonicalActor(parts[0],actor){
  return "",errors.New("communicated percept addressed to a different actor")
 }
 presented,ok:=parts[1].(string)
 if !ok||presented==""||len(presented)>512{return "",errors.New("communicated presented sender invalid")}
 claimID:=rawBytes(parts[2])
 if len(claimID)!=32{return "",errors.New("communicated claim identity invalid")}
 proposition,ok:=parts[3].([]any)
 if !ok||len(proposition)!=4{return "",errors.New("communicated proposition malformed")}
 claimType,ok:=proposition[0].(string)
 if !ok||claimType==""{return "",errors.New("communicated proposition type missing")}
 polarity,ok:=proposition[1].(uint64)
 if !ok||polarity>2{return "",errors.New("communicated proposition polarity invalid")}
 for idx,tag:=range []string{"cote.csf.communication.scope.v1","cote.csf.communication.allegation.v1"}{
  typed,ok:=proposition[idx+2].([]any)
  if !ok||len(typed)!=4||typed[0]!="cote.csf.schema.typed-value"||typed[1]!=uint64(1){
   return "",errors.New("communicated proposition typed field invalid")
  }
  encoded:=rawBytes(typed[2])
  env,err:=codec.StrictDecodeEnvelope(encoded)
  if err!=nil||env.DomainTag!=tag||env.SchemaID!="cote.csf.schema.typed-value"||env.SchemaVersion!=1{
   return "",errors.New("communicated proposition typed envelope unregistered")
  }
  digest:=identity.OpaqueContentHash(encoded)
  if !bytes.Equal(digest[:],rawBytes(typed[3])){
   return "",errors.New("communicated typed proposition digest mismatched")
  }
 }
 sent,ok:=signedTime(parts[4])
 if !ok{return "",errors.New("communicated timestamp invalid")}
 received,ok:=signedTime(observation[5])
 if !ok||sent!=received{return "",errors.New("communicated timestamp differs from source Event")}
 channel,ok:=parts[5].(string)
 if !ok||channel!="school.message.v1"{return "",errors.New("unregistered successful communication channel")}
 claims,ok:=observation[10].([]any)
 if !ok||len(claims)!=1{return "",errors.New("communicated claim reference missing or extra")}
 claimRef,ok:=claims[0].([]any)
 if !ok||len(claimRef)!=3||claimRef[0]!=uint64(18)||
  !bytes.Equal(rawBytes(claimRef[1]),claimID)||len(rawBytes(claimRef[2]))!=32{
  return "",errors.New("communicated notice and Claim reference differ")
 }
 if !sameTextList(observation[11],[]string{"actual-sender-redacted","claim-unverified"}){
  return "",errors.New("communicated claim provenance not explicitly unverified/redacted")
 }
 return channel,nil
}

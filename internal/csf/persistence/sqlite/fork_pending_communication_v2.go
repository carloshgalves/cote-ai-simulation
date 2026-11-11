package sqlite

import (
 "bytes"
 "context"
 "errors"
 "fmt"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/communication"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/fork"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/genesis"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
)

// V2CommunicationBirth is a NON-PUBLISHED child-authority proposal. Its
// parent snapshot, sent commit and exact immutable parent records have all
// been authenticated at the origin checkpoint, while each candidate child
// record has a new child-born identity. A storage migrator must first publish
// and replay-verify a signed migration-birth proof before these can become
// active and deliverable in a child run.
type V2CommunicationBirth struct{
 Parent InTransitDependencyV1
 ChildClaim,ChildTransmission,ChildOccurrence ports.CanonicalRecord
}

func (s *Store) PrepareV2CommunicationBirths(ctx context.Context, sourceRun,snapshotID [32]byte,
 child *genesis.Genesis,originPolicy,childPolicy *policy.Bundle)([]V2CommunicationBirth,error){
 if s==nil||child==nil||originPolicy==nil||childPolicy==nil{return nil,errors.New("V2 migration requires verified origin and child policies")}
 id,version,_,_,err:=childPolicy.CodecIdentity()
 if err!=nil||id!=policy.CodecPolicyV2ID||version!=2{return nil,errors.New("migration child policy must be installed V2")}
 originID,originVersion,_,_,err:=originPolicy.CodecIdentity()
 if err!=nil||originID!=policy.CodecPolicyID||originVersion!=1{return nil,errors.New("migration source policy must be installed V1")}
 verifiedChild,err:=genesis.ValidateForkCanonical(child.CanonicalBytes,childPolicy)
 if err!=nil{return nil,fmt.Errorf("V2 child Genesis not authenticated: %w",err)}
 if verifiedChild.ID!=child.ID||verifiedChild.Digest!=child.Digest||verifiedChild.RunID!=child.RunID{
  return nil,errors.New("child Genesis object fields do not match canonical authority")
 }
 if [32]byte(child.RunID)==sourceRun||child.RunID==([32]byte{}){
  return nil,errors.New("V2 child RunID is not distinct")
 }
 verified,err:=s.VerifySnapshotV1(ctx,sourceRun,snapshotID,originPolicy)
 if err!=nil{return nil,err}
 historyRef,err:=fork.NewSnapshotReference(originPolicy,verified.Snapshot.Canonical)
 if err!=nil{return nil,err}
 ref,err:=fork.Encode(historyRef)
 if err!=nil{return nil,err}
 parentRef,ok:=child.Payload[4].([]any)
 refBytes,refOK:=func()([]byte,bool){if !ok||len(parentRef)!=2{return nil,false};v,valid:=parentRef[1].([]byte);return v,valid}()
 if !ok||!refOK||len(parentRef)!=2||parentRef[0]!=uint64(1)||!bytes.Equal(refBytes,ref){
  return nil,errors.New("child Genesis does not pin exact verified V1 checkpoint")
 }
 sources,err:=s.PrepareInTransitDependenciesV1(ctx,sourceRun,snapshotID,originPolicy)
 if err!=nil{return nil,err}
 if uint64(len(sources))>uint64(^uint32(0)){return nil,errors.New("too many pending communication imports")}
 out:=make([]V2CommunicationBirth,0,len(sources))
 childOrigin:=communication.Origin{Kind:21,ID:child.ID,Digest:child.Digest}
 genesisOrigin:=[]any{uint64(2),[]any{child.ID[:],child.Digest[:]}}
 for i,dep:=range sources{
  var birth V2CommunicationBirth
  birth.Parent=dep
  claimValue,err:=codec.StrictDecodePayload(dep.Claim);if err!=nil{return nil,err}
  f,ok:=claimValue.([]any)
  if !ok||len(f)!=10{return nil,errors.New("origin Claim body malformed")}
  prop,ok:=f[4].([]any)
  if !ok||len(prop)!=4{return nil,errors.New("origin Claim proposition malformed")}
  kind,ok:=prop[0].(string)
  polarity,pok:=prop[1].(uint64)
  assertion,aok:=f[5].([]any)
  sentAt,tok:=asSigned64(f[6])
  if !ok||!pok||polarity>2||!aok||len(assertion)!=2||assertion[0]!=uint64(1)||!tok{
   return nil,errors.New("origin Claim assertion/proposition invalid")
  }
  actorFields,ok:=assertion[1].([]any)
  if !ok||len(actorFields)!=2{return nil,errors.New("origin Claim author malformed")}
  namespace,one:=actorFields[0].(string);name,two:=actorFields[1].(string)
  if !one||!two{return nil,errors.New("origin Claim actor invalid")}
  scope,ok:=prop[2].([]any);content,okContent:=prop[3].([]any)
  if !ok||!okContent{return nil,errors.New("origin Claim typed content absent")}
  claim,err:=communication.NewClaim(childPolicy,communication.ClaimSpec{
   Origin:childOrigin,Role:0,Ordinal:uint32(i),
   Proposition:communication.Proposition{Type:kind,Polarity:uint8(polarity),TemporalScope:scope,Content:content},
   AssertedBy:communication.Actor{namespace,name},AssertedAt:domain.SimulationInstant(sentAt),
  })
  if err!=nil{return nil,err}
  authenticated,err:=communication.AuthenticateClaim(childPolicy,claim)
  if err!=nil{return nil,err}
  txValue,err:=codec.StrictDecodePayload(dep.Transmission);if err!=nil{return nil,err}
  tx,ok:=txValue.([]any)
  if !ok||len(tx)!=14{return nil,errors.New("origin Transmission body malformed")}
  recipientRaw,ok:=tx[7].([]any)
  if !ok||len(recipientRaw)!=1{return nil,errors.New("origin recipient roster malformed")}
  recipient,ok:=recipientRaw[0].([]any)
  if !ok||len(recipient)!=2{return nil,errors.New("origin recipient malformed")}
  toNS,validNS:=recipient[0].(string);toName,validName:=recipient[1].(string)
  presented,validPresented:=tx[6].(string);channel,validChannel:=tx[8].(string)
  sourceAt,validSourceAt:=asSigned64(tx[9])
  if !validNS||!validName||!validPresented||!validChannel||!validSourceAt||sourceAt!=sentAt{
   return nil,errors.New("origin Transmission assertion differs from Claim")
  }
  occValue,err:=codec.StrictDecodePayload(dep.Occurrence);if err!=nil{return nil,err}
  occ,ok:=occValue.([]any)
  if !ok||len(occ)!=15{return nil,errors.New("origin due occurrence malformed")}
  dueAt,validDue:=asSigned64(occ[3])
  if !validDue||dueAt<int64(child.InitialInstant)||dueAt<=sourceAt{
   return nil,errors.New("origin delivery due before child Genesis or sent instant")
  }
  // Reserved V2 Genesis birth role 65535 segregates forked communication
  // occurrences from ordinary Genesis schedules. Ordinals follow the
  // lexicographically ordered and authenticated parent Transmission IDs.
  occurrenceID,_,err:=identity.New(childPolicy).Sum("cote.csf.id.occurrence",
   "cote.csf.schema.occurrence-id-preimage",1,
   []any{genesisOrigin,uint64(65535),uint64(i)})
  if err!=nil{return nil,err}
  childOccurrenceID:=domain.ID(occurrenceID)
  transmission,err:=communication.NewTransmission(childPolicy,communication.TransmissionSpec{
   Origin:childOrigin,Role:0,Ordinal:uint32(i),
   ClaimRefs:[]domain.CausalRef{{Kind:18,ID:domain.ID(claim.Identity),Digest:authenticated.Digest}},
   ActualSender:communication.Actor{namespace,name},PresentedSender:presented,
   Recipients:[]communication.Actor{{toNS,toName}},Channel:channel,
   SentAt:domain.SimulationInstant(sourceAt),DeliveryOccurrence:&childOccurrenceID,Status:0,
  })
  if err!=nil{return nil,err}
  verifiedTX,err:=communication.AuthenticateTransmission(childPolicy,transmission)
  if err!=nil{return nil,err}
  typed,err:=communication.MakeTyped("cote.csf.communication.delivery.v1",
   "cote.csf.schema.typed-value",[]any{
    transmission.Identity[:],verifiedTX.Digest[:],claim.Identity[:],authenticated.Digest[:],
   })
  if err!=nil{return nil,err}
  body:=[]any{occurrenceID[:],child.RunID[:],"communication",dueAt,
   []any{dueAt,uint64(0)},"communication.delivery",uint64(1),typed,
   uint64(0),genesisOrigin,uint64(65535),uint64(i),[]any{uint64(0)},[]any{uint64(0)}}
  digest,_,err:=identity.New(childPolicy).Sum("cote.csf.digest.occurrence-unit",
   "cote.csf.schema.scheduled-occurrence.body",1,body)
  if err!=nil{return nil,err}
  occurrenceBytes,err:=codec.CanonicalPayload(append(body,digest[:]))
  if err!=nil{return nil,err}
  occurrence:=ports.CanonicalRecord{Identity:[32]byte(occurrenceID),Canonical:occurrenceBytes}
  if _,life,err:=validateScheduledOccurrence([32]byte(child.RunID),[32]byte(occurrenceID),occurrenceBytes);err!=nil||life!=0{
   return nil,errors.New("V2 rekeyed occurrence failed canonical pending verification")
  }
  if claim.Identity==dep.ClaimID||transmission.Identity==dep.TransmissionID||
   occurrence.Identity==dep.OccurrenceID||claim.Identity==transmission.Identity{
   return nil,errors.New("V2 rekey silently reused parent identity")
  }
  birth.ChildClaim,birth.ChildTransmission,birth.ChildOccurrence=claim,transmission,occurrence
  out=append(out,birth)
 }
 return out,nil
}

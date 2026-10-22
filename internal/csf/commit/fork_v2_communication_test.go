package commit

import (
 "bytes"
 "context"
 "testing"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/communication"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/fork"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/genesis"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

func csfrv18V2PreparedGenesis(t *testing.T,source *sqlite.Store,
 sourceRun,snapshotID [32]byte,v1,v2 *policy.Bundle)*genesis.Genesis{
 t.Helper()
 ctx:=context.Background()
 snap,err:=source.VerifySnapshotV1(ctx,sourceRun,snapshotID,v1)
 if err!=nil{t.Fatal(err)}
 parent,err:=source.ValidatedGenesis(ctx,sourceRun,v1)
 if err!=nil{t.Fatal(err)}
 original,err:=codec.StrictDecodePayload(snap.Snapshot.Canonical)
 if err!=nil{t.Fatal(err)}
 f:=original.([]any)
 p:=append([]any(nil),parent.Payload...)
 nextRun:=sourceRun
 nextRun[0]^=0x9b
 if nextRun==([32]byte{}){nextRun[0]=1}
 ref,err:=fork.NewSnapshotReference(v1,snap.Snapshot.Canonical);if err!=nil{t.Fatal(err)}
 encoded,err:=fork.Encode(ref);if err!=nil{t.Fatal(err)}
 id,version,codecHash,schemaHash,err:=v2.CodecIdentity();if err!=nil{t.Fatal(err)}
 p[0]=nextRun[:]
 p[3]=f[2]
 p[4]=[]any{uint64(1),encoded}
 p[5]=id;p[6]=uint64(version);p[10]=codecHash[:];p[11]=schemaHash[:]
 p[16]=f[6];p[17]=f[14]
 raw,err:=codec.CanonicalBytes("cote.csf.digest.genesis-manifest",
  "cote.csf.schema.genesis-manifest",1,p)
 if err!=nil{t.Fatal(err)}
 g,err:=genesis.ValidateForkCanonical(raw,v2)
 if err!=nil{t.Fatalf("V2 canonical fork Genesis: %v",err)}
 return g
}

func TestCSFRV18V2RekeysTransitWithoutImportingParentAuthorities(t *testing.T){
 ctx:=context.Background()
 c,store,ledger,cycle,fence,_:=communicationFixture(t)
 _,_,_,_,_,run:=ledger.Snapshot()
 if _,err:=c.CommitCommunicationSend(ctx,ledger,cycle,fence);err!=nil{t.Fatal(err)}
 snap,err:=store.CreateSnapshotV1(ctx,[32]byte(run),c.bundle,nil);if err!=nil{t.Fatal(err)}
 v2,err:=policy.LoadV2("../../../docs/architecture/canonical-codec-v2-bundle")
 if err!=nil{t.Fatal(err)}
 child:=csfrv18V2PreparedGenesis(t,store,[32]byte(run),snap.ID,c.bundle,v2)
 births,err:=store.PrepareV2CommunicationBirths(ctx,[32]byte(run),snap.ID,child,c.bundle,v2)
 if err!=nil{t.Fatal(err)}
 if len(births)!=1{t.Fatalf("expected one V2 birth, got %d",len(births))}
 birth:=births[0]
 if birth.ChildClaim.Identity==birth.Parent.ClaimID||
  birth.ChildTransmission.Identity==birth.Parent.TransmissionID||
  birth.ChildOccurrence.Identity==birth.Parent.OccurrenceID{
  t.Fatal("cross-policy migration retained parental causal identities")
 }
 originalClaim,err:=communication.AuthenticateClaim(v2,birth.ChildClaim)
 if err!=nil{t.Fatal("child Claim invalid:",err)}
 originalTransmission,err:=communication.AuthenticateTransmission(v2,birth.ChildTransmission)
 if err!=nil{t.Fatal("child Transmission invalid:",err)}
 if originalClaim.ID!=domain.ID(birth.ChildClaim.Identity)||
  originalTransmission.Occurrence!=domain.ID(birth.ChildOccurrence.Identity)||
  originalTransmission.Origin.Kind!=21||originalClaim.Origin.Kind!=21||
  originalTransmission.Origin.ID!=child.ID||originalClaim.Origin.ID!=child.ID {
  t.Fatal("V2 Claim/Transmission not based on child Genesis authority")
 }
 if _,err:=communication.ValidateMigratedOccurrenceV2(v2,communication.SendPlan{
  Run:child.RunID,Claim:birth.ChildClaim,Transmission:birth.ChildTransmission,
  Occurrence:birth.ChildOccurrence,
 },child.ID,child.Digest);err!=nil{
  t.Fatal("V2 imported occurrence not child-authenticated:",err)
 }
 parts,err:=codec.StrictDecodePayload(birth.ChildOccurrence.Canonical);if err!=nil{t.Fatal(err)}
 occ:=parts.([]any)
 if !bytes.Equal(occ[1].([]byte),child.RunID[:])||
  occ[5]!="communication.delivery"||occ[10]!=uint64(65535){
  t.Fatal("rekeyed due occurrence does not target child run and reserved migration role")
 }
 repeat,err:=store.PrepareV2CommunicationBirths(ctx,[32]byte(run),snap.ID,child,c.bundle,v2)
 if err!=nil||len(repeat)!=1||
  !bytes.Equal(repeat[0].ChildClaim.Canonical,birth.ChildClaim.Canonical)||
  !bytes.Equal(repeat[0].ChildTransmission.Canonical,birth.ChildTransmission.Canonical)||
  !bytes.Equal(repeat[0].ChildOccurrence.Canonical,birth.ChildOccurrence.Canonical){
  t.Fatal("rekey under identical signed parent/child policies was not deterministic:",err)
 }
 if _,err:=store.VerifySnapshotV1(ctx,[32]byte(run),snap.ID,c.bundle);err!=nil{
  t.Fatal("read-only V2 rekey modified parental snapshot:",err)
 }
}

func TestCSFRV18V2RekeyRejectsUnverifiedChildGenesis(t *testing.T){
 ctx:=context.Background()
 c,store,ledger,cycle,fence,_:=communicationFixture(t)
 _,_,_,_,_,run:=ledger.Snapshot()
 if _,err:=c.CommitCommunicationSend(ctx,ledger,cycle,fence);err!=nil{t.Fatal(err)}
 snap,err:=store.CreateSnapshotV1(ctx,[32]byte(run),c.bundle,nil);if err!=nil{t.Fatal(err)}
 v2,err:=policy.LoadV2("../../../docs/architecture/canonical-codec-v2-bundle")
 if err!=nil{t.Fatal(err)}
 child:=csfrv18V2PreparedGenesis(t,store,[32]byte(run),snap.ID,c.bundle,v2)
 child.ID[0]^=1
 if _,err:=store.PrepareV2CommunicationBirths(ctx,[32]byte(run),snap.ID,child,c.bundle,v2);err==nil{
  t.Fatal("rekey accepted tampered in-memory V2 Genesis identity")
 }
}

package sqlite

import (
 "bytes"
 "context"
 "database/sql"
 "errors"
 "fmt"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/communication"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/genesis"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
)

// The V2 import proof is an immutable twenty-element tuple. Its exact bytes
// bind an authenticated V1 causal send chain to three newly minted V2 child
// roots without importing any parent's live run-bound ID as child authority.
func v2CommunicationBirthBytes(parentRun,parentSnapshotID [32]byte,
 parentPolicy *policy.Bundle,child *genesis.Genesis,birth V2CommunicationBirth)([]byte,error){
 if parentPolicy==nil||child==nil{return nil,errors.New("missing V2 migration provenance")}
 d:=birth.Parent
 f:=[]any{uint64(2),child.RunID[:],child.ID[:],child.Digest[:],
  parentRun[:],parentSnapshotID[:],parentPolicy.CodecPolicyHash[:],
  d.SentCommitID[:],d.ClaimID[:],d.TransmissionID[:],d.OccurrenceID[:],
  birth.ChildClaim.Identity[:],birth.ChildTransmission.Identity[:],
  birth.ChildOccurrence.Identity[:],
  bytes.Clone(d.Claim),bytes.Clone(d.Transmission),bytes.Clone(d.Occurrence),
  bytes.Clone(birth.ChildClaim.Canonical),bytes.Clone(birth.ChildTransmission.Canonical),
  bytes.Clone(birth.ChildOccurrence.Canonical)}
 return codec.CanonicalPayload(f)
}

func publishV2CommunicationBirths(ctx context.Context,tx *sql.Tx,run [32]byte,
 births []V2CommunicationBirth,sourceRun,snapshotID [32]byte,sourcePolicy *policy.Bundle,
 child *genesis.Genesis)error{
 for _,birth:=range births{
  proof,err:=v2CommunicationBirthBytes(sourceRun,snapshotID,sourcePolicy,child,birth)
  if err!=nil{return err}
  hash:=identity.OpaqueContentHash(proof)
  if _,err:=tx.ExecContext(ctx,
   "INSERT INTO communication_claims(id,run_id,canonical) VALUES(?,?,?)",
   birth.ChildClaim.Identity[:],run[:],birth.ChildClaim.Canonical);err!=nil{return err}
  if _,err:=tx.ExecContext(ctx,
   "INSERT INTO communication_transmissions(id,run_id,claim_id,occurrence_id,sent_commit_id,status,canonical) "+
   "VALUES(?,?,?,?,?,0,?)",
   birth.ChildTransmission.Identity[:],run[:],birth.ChildClaim.Identity[:],
   birth.ChildOccurrence.Identity[:],birth.Parent.SentCommitID[:],
   birth.ChildTransmission.Canonical);err!=nil{return err}
  if _,err:=tx.ExecContext(ctx,
   "INSERT INTO fork_communication_imports(child_transmission_id,child_run_id,parent_sent_commit_id,canonical,canonical_hash) "+
   "VALUES(?,?,?,?,?)",
   birth.ChildTransmission.Identity[:],run[:],birth.Parent.SentCommitID[:],proof,hash[:]);err!=nil{return err}
 }
 return nil
}

// validateV2ImportedBirthTx verifies the source origin AGAIN from the signed
// archived V1 ledger members. It must run at child bootstrap and on later
// child replay, so restart cannot turn an unverified external SQL row into a
// legitimate imported message.
func validateV2ImportedBirthTx(ctx context.Context,tx *sql.Tx,childRun,childTxID [32]byte,
 childPolicy *policy.Bundle)error{
 if childPolicy==nil{return errors.New("missing V2 import policy")}
 id,v,_,_,err:=childPolicy.CodecIdentity()
 if err!=nil||id!=policy.CodecPolicyV2ID||v!=2{
  return errors.New("V2 migration requires installed V2 runtime")
 }
 var raw,digest,parentSent []byte
 if err:=tx.QueryRowContext(ctx,
  "SELECT canonical,canonical_hash,parent_sent_commit_id FROM fork_communication_imports "+
   "WHERE child_run_id=? AND child_transmission_id=?",
  childRun[:],childTxID[:]).Scan(&raw,&digest,&parentSent);err!=nil{return err}
 hash:=identity.OpaqueContentHash(raw)
 if !bytes.Equal(digest,hash[:])||len(parentSent)!=32{return errors.New("V2 import proof seal invalid")}
 value,err:=codec.StrictDecodePayload(raw);if err!=nil{return err}
 f,ok:=value.([]any)
 if !ok||len(f)!=20||f[0]!=uint64(2)||!equalBytes(f[1],childRun[:])||
  !equalBytes(f[12],childTxID[:])||!equalBytes(f[7],parentSent){
  return errors.New("V2 birth proof schema/identity invalid")
 }
 var parentRun,originHash,checkpoint,rootID,rootDigest [32]byte
 for _,entry:=range []struct{src any;dst *[32]byte}{
  {f[4],&parentRun},{f[6],&originHash},{f[2],&rootID},{f[3],&rootDigest},
 }{
  raw,ok:=entry.src.([]byte)
  if !ok||len(raw)!=32{return errors.New("V2 proof root ID malformed")}
  copy(entry.dst[:],raw)
 }
 checkpointRaw,ok:=f[5].([]byte)
 if !ok||len(checkpointRaw)!=32{return errors.New("V2 proof checkpoint invalid")}
 copy(checkpoint[:],checkpointRaw)
 var lineageParent,lineageSnap,lineagePolicy []byte
 if err:=tx.QueryRowContext(ctx,
  "SELECT parent_run,parent_snapshot_id,origin_policy_hash FROM fork_lineage WHERE child_run=?",
  childRun[:]).Scan(&lineageParent,&lineageSnap,&lineagePolicy);err!=nil{return err}
 if !bytes.Equal(lineageParent,parentRun[:])||!bytes.Equal(lineageSnap,checkpoint[:])||
  !bytes.Equal(lineagePolicy,originHash[:]){
  return errors.New("V2 import proof not anchored in fork lineage")
 }
 historic,err:=childPolicy.ResolveHistoricalOrigin(policy.CodecPolicyID,originHash)
 if err!=nil{return err}
 var genesisRaw []byte
 if err:=tx.QueryRowContext(ctx,"SELECT canonical FROM genesis_records WHERE run_id=?",
  childRun[:]).Scan(&genesisRaw);err!=nil{return err}
 child,err:=genesis.ValidateForkCanonical(genesisRaw,childPolicy)
 if err!=nil||child.ID!=domain.ID(rootID)||child.Digest!=domain.Digest(rootDigest){
  return errors.New("V2 imported authority differs from child Genesis")
 }
 var originalClaim,originalTransmission,originalDue []byte
 var originalClaimID,originalTXID,originalDueID,childClaimID,childDueID [32]byte
 for _,entry:=range []struct{index int;dst *[32]byte}{
  {8,&originalClaimID},{9,&originalTXID},{10,&originalDueID},
  {11,&childClaimID},{13,&childDueID},
 }{
  val,ok:=f[entry.index].([]byte)
  if !ok||len(val)!=32{return errors.New("import source or destination ID malformed")}
  copy(entry.dst[:],val)
 }
 for _,entry:=range []struct{index int;out *[]byte}{
  {14,&originalClaim},{15,&originalTransmission},{16,&originalDue},
 }{
  v,ok:=f[entry.index].([]byte)
  if !ok||len(v)==0{return errors.New("import missing exact parent canonical record")}
  *entry.out=bytes.Clone(v)
 }
 oldClaim:=ports.CanonicalRecord{Identity:originalClaimID,Canonical:originalClaim}
 oldTX:=ports.CanonicalRecord{Identity:originalTXID,Canonical:originalTransmission}
 oldDue:=ports.CanonicalRecord{Identity:originalDueID,Canonical:originalDue}
 parentDue,err:=communication.ValidateOccurrence(historic,communication.SendPlan{
  Run:domain.ID(parentRun),Claim:oldClaim,Transmission:oldTX,Occurrence:oldDue,
 })
 if err!=nil{return fmt.Errorf("imported parent due not historically authentic: %w",err)}
 var originalCommit []byte
 if err:=tx.QueryRowContext(ctx,"SELECT canonical FROM fork_history_records "+
  "WHERE child_run=? AND ledger_name='decision_ledger' AND record_id=?",
  childRun[:],parentSent).Scan(&originalCommit);err!=nil{return errors.New("import parent sent commit missing from immutable prefix")}
 parsed,err:=codec.StrictDecodePayload(originalCommit)
 if err!=nil{return err}
 journal,ok:=parsed.([]any)
 if !ok||len(journal)!=24||!equalBytes(journal[0],parentRun[:]){
  return errors.New("import parent sent commit does not belong to archived run")
 }
 oldVerified,err:=communication.AuthenticateClaim(historic,oldClaim)
 if err!=nil{return err}
 oldVerifiedTX,err:=communication.AuthenticateTransmission(historic,oldTX)
 if err!=nil||oldVerifiedTX.Origin!=oldVerified.Origin||oldVerified.Origin.Kind!=5||
  !containsRoot(journal[11],[32]byte(oldVerified.Origin.ID)){
  return errors.New("archived sent commit lacks original Claim/Transmission source")
 }
 var originalSource []byte
 if err:=tx.QueryRowContext(ctx,"SELECT canonical FROM fork_history_records "+
  "WHERE child_run=? AND ledger_name='input_ledger' AND record_id=?",
  childRun[:],oldVerified.Origin.ID[:]).Scan(&originalSource);err!=nil{
  return errors.New("V1 authored ActionProposal missing from immutable source prefix")
 }
 authored,err:=communication.AuthoredAction(historic,ports.CanonicalRecord{
  Identity:[32]byte(oldVerified.Origin.ID),Canonical:originalSource,
 })
 if err!=nil||authored.Source.Digest!=oldVerified.Origin.Digest||
  authored.Spec.SentAt!=oldVerified.AssertedAt{
  return errors.New("V1 source ActionProposal did not author imported allegation")
 }
 plan,err:=communication.PrepareSend(historic,authored.Spec)
 if err!=nil||!bytes.Equal(plan.Claim.Canonical,originalClaim)||
  !bytes.Equal(plan.Transmission.Canonical,originalTransmission)||
  !bytes.Equal(plan.Occurrence.Canonical,originalDue){
  return errors.New("archived parent send is not deterministically reconstructible")
 }
 // The future child authority must be rederived, not merely a renamed V1
 // record. Its Genesis origin and typed Claim/Transmission/due chain are
 // independently validated using the installed V2 schema.
 var claimDB,txDB []byte
 if err:=tx.QueryRowContext(ctx,"SELECT canonical FROM communication_claims WHERE run_id=? AND id=?",
  childRun[:],childClaimID[:]).Scan(&claimDB);err!=nil{return err}
 if err:=tx.QueryRowContext(ctx,"SELECT canonical FROM communication_transmissions WHERE run_id=? AND id=? AND claim_id=? AND occurrence_id=?",
  childRun[:],childTxID[:],childClaimID[:],childDueID[:]).Scan(&txDB);err!=nil{return err}
 if !bytes.Equal(claimDB,rawBytesImport(f[17]))||!bytes.Equal(txDB,rawBytesImport(f[18])){
  return errors.New("child imported Claim/Transmission differ from immutable birth proof")
 }
 planChild:=communication.SendPlan{
  Run:domain.ID(childRun),
  Claim:ports.CanonicalRecord{Identity:childClaimID,Canonical:claimDB},
  Transmission:ports.CanonicalRecord{Identity:childTxID,Canonical:txDB},
  Occurrence:ports.CanonicalRecord{Identity:childDueID,Canonical:rawBytesImport(f[19])},
 }
 childDue,err:=communication.ValidateMigratedOccurrenceV2(childPolicy,planChild,child.ID,child.Digest)
 if err!=nil{return fmt.Errorf("invalid V2 child imported due: %w",err)}
 // An attacker must not be able to rewrite a child's allegation and
 // recompute the unkeyed SQL integrity hash. Cross-policy migration
 // preserves all communication semantics, not only the causal ID links.
 newClaim,err:=communication.AuthenticateClaim(childPolicy,planChild.Claim)
 if err!=nil{return err}
 newTX,err:=communication.AuthenticateTransmission(childPolicy,planChild.Transmission)
 if err!=nil{return err}
 if childDue!=parentDue||
  newClaim.AssertedBy!=oldVerified.AssertedBy||
  newClaim.AssertedAt!=oldVerified.AssertedAt||
  !sameCanonical(newClaim.Proposition,oldVerified.Proposition)||
  newTX.ActualSender!=oldVerifiedTX.ActualSender||
  newTX.PresentedSender!=oldVerifiedTX.PresentedSender||
  newTX.Recipient!=oldVerifiedTX.Recipient||
  newTX.Channel!=oldVerifiedTX.Channel||
  newTX.SentAt!=oldVerifiedTX.SentAt||
  newTX.Status!=oldVerifiedTX.Status||
  parentRun==childRun||
  childClaimID==originalClaimID||childTxID==originalTXID||
  childDueID==originalDueID {
  return errors.New("V2 import changed parental communication semantics or reused identity")
 }
 oldClaimFields,err:=codec.StrictDecodePayload(originalClaim)
 if err!=nil{return err}
 newClaimFields,err:=codec.StrictDecodePayload(claimDB)
 if err!=nil{return err}
 oldTransmissionFields,err:=codec.StrictDecodePayload(originalTransmission)
 if err!=nil{return err}
 newTransmissionFields,err:=codec.StrictDecodePayload(txDB)
 if err!=nil{return err}
 oldCF,okOldC:=oldClaimFields.([]any)
 childCF,okChildC:=newClaimFields.([]any)
 oldTF,okOldT:=oldTransmissionFields.([]any)
 childTF,okChildT:=newTransmissionFields.([]any)
 if !okOldC||!okChildC||!okOldT||!okChildT||
  len(oldCF)!=10||len(childCF)!=10||len(oldTF)!=14||len(childTF)!=14||
  childCF[2]!=uint64(0)||childTF[2]!=uint64(0)||
  childCF[3]!=childTF[3]||childCF[3]!=func()any{
   dueFields,err:=codec.StrictDecodePayload(planChild.Occurrence.Canonical)
   if err!=nil{return nil}
   parsed,ok:=dueFields.([]any)
   if !ok||len(parsed)!=15{return nil}
   return parsed[11]
  }()||!sameCanonical(oldCF[4],childCF[4])||
  !sameCanonical(oldCF[5],childCF[5])||
  !sameCanonical(oldTF[5],childTF[5])||
  !sameCanonical(oldTF[6],childTF[6])||
  !sameCanonical(oldTF[7],childTF[7])||
  !sameCanonical(oldTF[8],childTF[8])||
  !sameCanonical(oldTF[9],childTF[9])||
  !sameCanonical(oldTF[11],childTF[11]){
  return errors.New("V2 import changed body fields or cross-record ordinal")
 }
 return nil
}
func rawBytesImport(v any)[]byte{b,_:=v.([]byte);return b}

// All imported child authorities must be admitted via a matching proof.
// Missing or unexpected rows are treated as dangling live authority.
func verifyAllV2ImportsTx(ctx context.Context,tx *sql.Tx,run [32]byte,b *policy.Bundle)error{
 id,_,_,_,err:=b.CodecIdentity()
 if err!=nil{return err}
 if id!=policy.CodecPolicyV2ID{return nil}
 rows,err:=tx.QueryContext(ctx,"SELECT child_transmission_id FROM fork_communication_imports WHERE child_run_id=? ORDER BY child_transmission_id",run[:])
 if err!=nil{return err}
 var ids [][32]byte
 for rows.Next(){var raw []byte;if err:=rows.Scan(&raw);err!=nil{rows.Close();return err};if len(raw)!=32{rows.Close();return errors.New("bad V2 migration ID")};var id [32]byte;copy(id[:],raw);ids=append(ids,id)}
 if err:=rows.Err();err!=nil{rows.Close();return err}
 if err:=rows.Close();err!=nil{return err}
 proofIDs:=map[[32]byte]bool{}
 for _,key:=range ids{
  if proofIDs[key]{return errors.New("duplicate V2 imported birth identity")}
  proofIDs[key]=true
  if err:=validateV2ImportedBirthTx(ctx,tx,run,key,b);err!=nil{return err}
 }
 // Cross-check both directions. If an attacker drops the import receipt but
 // leaves its child Transmission/Claim/due live, merely iterating the
 // surviving proofs would incorrectly declare replay healthy.
 rows,err=tx.QueryContext(ctx,
  "SELECT id,sent_commit_id FROM communication_transmissions WHERE run_id=? ORDER BY id",
  run[:])
 if err!=nil{return err}
 type activeTransmission struct{id,sent [32]byte}
 var live []activeTransmission
 for rows.Next(){
  var idRaw,sentRaw []byte
  if err:=rows.Scan(&idRaw,&sentRaw);err!=nil{rows.Close();return err}
  if len(idRaw)!=32||len(sentRaw)!=32{rows.Close();return errors.New("V2 transmission SQL identity malformed")}
  var item activeTransmission
  copy(item.id[:],idRaw);copy(item.sent[:],sentRaw)
  live=append(live,item)
 }
 if err:=rows.Err();err!=nil{rows.Close();return err}
 if err:=rows.Close();err!=nil{return err}
 for _,item:=range live{
  var localCommit int
  if err:=tx.QueryRowContext(ctx,
   "SELECT count(*) FROM committed_cycles WHERE run_id=? AND commit_id=?",
   run[:],item.sent[:]).Scan(&localCommit);err!=nil{return err}
  imported:=proofIDs[item.id]
  if localCommit==0&&!imported{
   return errors.New("V2 transmission lacks both local send and imported historical birth")
  }
  if localCommit!=0&&imported{
   return errors.New("V2 transmission has conflicting local and imported send births")
  }
 }
 return nil
}

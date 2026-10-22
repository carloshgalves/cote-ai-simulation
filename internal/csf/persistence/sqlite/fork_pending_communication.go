package sqlite

import (
 "bytes"
 "context"
 "errors"
 "fmt"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

// InTransitDependencyV1 is a verified READ-ONLY migration prerequisite.
// A pending communication is causally born in the parent's ActionProposal,
// Claim, Transmission, CycleCommit and delivery ScheduledOccurrence. Merely
// copying these bytes to a new run would create foreign live authority.
// Only a versioned V2 migrator may consume this graph to rekey and publish
// equivalent child-owned work with an explicit provenance bridge.
type InTransitDependencyV1 struct {
 ParentRun, TransmissionID, ClaimID, OccurrenceID, SentCommitID [32]byte
 Transmission, Claim, Occurrence []byte
}

// PrepareInTransitDependenciesV1 checks the entire parent snapshot and live
// send history, then extracts the exact dependent canonical bytes in a stable
// order. No child store is accepted or mutated by this phase.
func (s *Store) PrepareInTransitDependenciesV1(ctx context.Context,
 parentRun, checkpoint [32]byte, b *policy.Bundle) ([]InTransitDependencyV1,error) {
 if s==nil||b==nil{return nil,errors.New("communication migration preparation requires parent store and policy")}
 verified,err:=s.VerifySnapshotV1(ctx,parentRun,checkpoint,b)
 if err!=nil{return nil,fmt.Errorf("communication migration source snapshot: %w",err)}
 // Replay authenticates sent CycleCommit/Fence, Claim, Transmission,
 // delivery occurrence and historical state. The graph below requires
 // a matching pending occurrence in this exact signed snapshot.
 if _,err:=s.ReplayClock(ctx,parentRun,b);err!=nil{
  return nil,fmt.Errorf("communication migration parent replay: %w",err)
 }
 root,err:=codec.StrictDecodePayload(verified.Snapshot.Canonical)
 if err!=nil{return nil,err}
 snap,ok:=root.([]any)
 if !ok||len(snap)!=19||!equalBytes(snap[0],parentRun[:]){
  return nil,errors.New("communication migration snapshot payload invalid")
 }
 typed,ok:=snap[11].([]any)
 if !ok||len(typed)!=4{return nil,errors.New("communication migration missing pending state")}
 env,err:=codec.StrictDecodeEnvelope(communicationBytes(typed[2]))
 if err!=nil{return nil,err}
 parts,ok:=env.Value.([]any)
 if !ok||len(parts)!=10{return nil,errors.New("communication migration pending partition invalid")}
 sourceRefs,ok:=parts[0].([]any)
 if !ok{return nil,errors.New("communication migration occurrence roster invalid")}
 expected:=map[[32]byte][]byte{}
 for _,raw:=range sourceRefs {
  ref,ok:=raw.([]any)
  if !ok||len(ref)!=3||ref[0]!=uint64(6){return nil,errors.New("communication migration schedule ref malformed")}
  id,ok:=idField(ref[1])
  digest,okDigest:=ref[2].([]byte)
  if !ok||!okDigest||len(digest)!=32{return nil,errors.New("communication migration schedule ID/hash malformed")}
  if _,duplicate:=expected[id];duplicate{return nil,errors.New("duplicate pending communication schedule ID")}
  expected[id]=bytes.Clone(digest)
 }
 rows,err:=s.db.QueryContext(ctx,
  "SELECT id,claim_id,occurrence_id,sent_commit_id,status,canonical "+
   "FROM communication_transmissions WHERE run_id=? ORDER BY id",parentRun[:])
 if err!=nil{return nil,err}
 type transmissionRow struct{
  tid,cid,oid,sid,raw []byte
  status int
 }
 collected:=make([]transmissionRow,0)
 for rows.Next(){
  var item transmissionRow
  if err:=rows.Scan(&item.tid,&item.cid,&item.oid,&item.sid,&item.status,&item.raw);err!=nil{
   rows.Close();return nil,err
  }
  collected=append(collected,item)
 }
 if err:=rows.Err();err!=nil{rows.Close();return nil,err}
 if err:=rows.Close();err!=nil{return nil,err}
 // Store limits SQLite to one connection. Close the streaming cursor
 // before resolving Claim/Occurrence dependencies via QueryRowContext.
 result:=make([]InTransitDependencyV1,0)
 for _,item:=range collected{
  tid,cid,oid,sid,txRaw,status:=item.tid,item.cid,item.oid,item.sid,item.raw,item.status
  if status!=0{continue}
  if len(tid)!=32||len(cid)!=32||len(oid)!=32||len(sid)!=32{
   return nil,errors.New("communication migration SQL identities malformed")
  }
  var dependency InTransitDependencyV1
  dependency.ParentRun=parentRun
  copy(dependency.TransmissionID[:],tid)
  copy(dependency.ClaimID[:],cid)
  copy(dependency.OccurrenceID[:],oid)
  copy(dependency.SentCommitID[:],sid)
  if _,ok:=expected[dependency.OccurrenceID];!ok{
   return nil,errors.New("in-transit transmission absent from signed pending occurrence roster")
  }
  txValue,err:=codec.StrictDecodePayload(txRaw);if err!=nil{return nil,err}
  tx,ok:=txValue.([]any)
  if !ok||len(tx)!=14||!equalBytes(tx[0],tid)||tx[12]!=uint64(0)||
   !equalBytes(tx[13],func()[]byte{
    d,_,e:=identity.New(b).Sum("cote.csf.digest.transmission","cote.csf.schema.transmission.body",1,tx[:13])
    if e!=nil{return nil};return d[:]
   }()){
   return nil,errors.New("in-transit Transmission canonical identity/status/digest invalid")
  }
  if !sameCanonical(tx[10],[]any{uint64(1),oid}){
   return nil,errors.New("in-transit Transmission due occurrence mismatched")
  }
  wantID,_,err:=identity.New(b).Sum("cote.csf.id.transmission",
   "cote.csf.schema.transmission-id-preimage",1,[]any{tx[1],tx[2],tx[3]})
  if err!=nil||!bytes.Equal(wantID[:],tid){
   return nil,errors.New("in-transit Transmission ID is not derived from origin")
  }
  if err:=s.db.QueryRowContext(ctx,
   "SELECT canonical FROM communication_claims WHERE run_id=? AND id=?",parentRun[:],cid).
   Scan(&dependency.Claim);err!=nil{return nil,err}
  claimValue,err:=codec.StrictDecodePayload(dependency.Claim);if err!=nil{return nil,err}
  claim,ok:=claimValue.([]any)
  if !ok||len(claim)!=10||!equalBytes(claim[0],cid)||
   !sameCanonical(tx[1],claim[1])||!sameCanonical(tx[4],[]any{uint64(0),[]any{[]any{uint64(18),cid,claim[9]}}}){
   return nil,errors.New("in-transit Claim is not the Transmission causal source")
  }
  claimID,_,err:=identity.New(b).Sum("cote.csf.id.claim",
   "cote.csf.schema.claim-id-preimage",1,[]any{claim[1],claim[2],claim[3]})
  if err!=nil||!bytes.Equal(claimID[:],cid){
   return nil,errors.New("in-transit Claim causal identity forged")
  }
  claimDigest,_,err:=identity.New(b).Sum("cote.csf.digest.claim","cote.csf.schema.claim.body",1,claim[:9])
  if err!=nil||!equalBytes(claim[9],claimDigest[:]){
   return nil,errors.New("in-transit Claim canonical digest forged")
  }
  if err:=s.db.QueryRowContext(ctx,
   "SELECT canonical FROM scheduled_occurrences WHERE run_id=? AND id=?",parentRun[:],oid).
   Scan(&dependency.Occurrence);err!=nil{return nil,err}
  coordinate,life,err:=validateScheduledOccurrence(parentRun,dependency.OccurrenceID,dependency.Occurrence)
  if err!=nil||life!=0{return nil,errors.New("in-transit occurrence no longer pending")}
  _=coordinate
  occurrence,err:=codec.StrictDecodePayload(dependency.Occurrence)
  if err!=nil{return nil,err}
  occ,ok:=occurrence.([]any)
  if !ok||len(occ)!=15||occ[5]!="communication.delivery"||
   !bytes.Equal(expected[dependency.OccurrenceID],communicationBytes(occ[14])){
   return nil,errors.New("in-transit due source does not match snapshot")
  }
  typed,ok:=occ[7].([]any)
  if !ok||len(typed)!=4{return nil,errors.New("in-transit due typed delivery absent")}
  envelope,err:=codec.StrictDecodeEnvelope(communicationBytes(typed[2]))
  if err!=nil||envelope.DomainTag!="cote.csf.communication.delivery.v1"||
   !sameCanonical(envelope.Value,[]any{tid,tx[13],cid,claim[9]})||
   !communicationTypedDigestMatches(typed){
   return nil,errors.New("in-transit due Claim/Transmission digest chain invalid")
  }
  dependency.Transmission=bytes.Clone(txRaw)
  result=append(result,dependency)
  delete(expected,dependency.OccurrenceID)
 }
 // It is fine for another pending Genesis schedule to remain; the real
 // consumer uses the typed delivery envelope to select the due subset.
 return result,nil
}

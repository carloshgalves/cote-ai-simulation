package sqlite

import (
 "context"
 "errors"
 "math"
 "bytes"
 "encoding/hex"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/clock"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
)

// The V1 cascade limit is a semantic part of the Genesis-bound
// cycle-coordinate-v1@1 policy. Its exact 32-byte pin is in the canonical
// Genesis fixture and verified by genesis.ValidateCanonical. A future
// runtime must allocate a NEW policy ID/hash before changing this limit.
func verifyCascadePolicy(ctx context.Context,q queryOne,run [32]byte)error{
 var genesisRaw []byte
 if err:=q.QueryRowContext(ctx,"SELECT canonical FROM genesis_records WHERE run_id=?",run[:]).Scan(&genesisRaw);err!=nil{return err}
 envelope,err:=codec.StrictDecodeEnvelope(genesisRaw);if err!=nil{return err}
 fields,ok:=envelope.Value.([]any)
 if !ok||len(fields)!=18{return errors.New("CASCADE_LIMIT genesis envelope malformed")}
 refs,ok:=fields[14].([]any)
 if !ok{return errors.New("CASCADE_LIMIT policy refs missing")}
 return verifyCascadePolicyRefs(refs)
}

func verifyCascadePolicyRefs(refs []any)error{
 if clock.CascadePolicyVersionV1!=1||clock.MaxSameInstantWorkV1!=64{
  return errors.New("CASCADE_LIMIT V1 semantics changed without a new policy version")
 }
 // Hash from the canonical genesis.pinning fixture. The abstract
 // transition-contract role declaration is not the canonical hash.
 expectedHash,err:=hex.DecodeString("0bac988281bc2b0c4af79adfc9dbd238395ffee5b8ab8758eedc89a75bcd6a6c")
 if err!=nil{return err}
 matched:=0
 for _,v:=range refs{
  ref,ok:=v.([]any)
  if !ok||len(ref)!=3{return errors.New("CASCADE_LIMIT malformed genesis policy ref")}
  if ref[0]!="cycle-coordinate-v1"{continue}
  ver,valid:=ref[1].(uint64)
  hash,hashOK:=ref[2].([]byte)
  if !valid||ver!=clock.CascadePolicyVersionV1||!hashOK||!bytes.Equal(hash,expectedHash){
   return errors.New("CASCADE_LIMIT genesis cycle-coordinate policy pin incompatible with V1")
  }
  matched++
 }
 if matched!=1{return errors.New("CASCADE_LIMIT requires exactly one pinned cycle-coordinate-v1 policy")}
 return nil
}

func countWorkAt(ctx context.Context,q queryOne,run [32]byte,instant int64)(uint64,error){
 if err:=verifyCascadePolicy(ctx,q,run);err!=nil{return 0,err}
 rows,err:=q.QueryContext(ctx,"SELECT cycle_id,commit_id FROM committed_cycles WHERE run_id=?",run[:])
 if err!=nil{return 0,err}
 type root struct{cycle,commit [32]byte}
 var roots []root
 for rows.Next(){
  var cycle,commit []byte
  if err:=rows.Scan(&cycle,&commit);err!=nil{rows.Close();return 0,err}
  if len(cycle)!=32||len(commit)!=32{rows.Close();return 0,errors.New("cascade history contains malformed commit key")}
  var r root;copy(r.cycle[:],cycle);copy(r.commit[:],commit);roots=append(roots,r)
 }
 if err:=rows.Err();err!=nil{rows.Close();return 0,err}
 if err:=rows.Close();err!=nil{return 0,err}
 var count uint64
 for _,r:=range roots{
  var raw []byte
  if err:=q.QueryRowContext(ctx,"SELECT canonical FROM decision_ledger WHERE id=?",r.commit[:]).Scan(&raw);err!=nil{return 0,err}
  decoded,err:=codec.StrictDecodePayload(raw);if err!=nil{return 0,err}
  journal,ok:=decoded.([]any)
  if !ok||len(journal)!=24||!equalBytes(journal[0],run[:])||
   !equalBytes(journal[1],r.cycle[:]){return 0,errors.New("cascade history journal root corrupt")}
  attempt,ok:=journal[2].(uint64);if !ok||attempt>math.MaxUint32{return 0,errors.New("cascade attempt invalid")}
  id,err:=registeredDigest("cote.csf.id.cycle-commit","cote.csf.schema.cycle-commit-id-preimage",
   []any{run[:],r.cycle[:],attempt});if err!=nil||id!=domain.Digest(r.commit){return 0,errors.New("cascade committed cycle ID unauthenticated")}
  events,err:=loadJournalEvents(ctx,q,journal);if err!=nil{return 0,err}
  digest,err:=registeredDigest("cote.csf.digest.batch","cote.csf.schema.batch-preimage",
   []any{journal[:23],events});if err!=nil||!equalBytes(journal[23],digest[:]){
   return 0,errors.New("cascade committed journal digest invalid")
  }
  at,ok:=asSigned64(journal[3]);if !ok{return 0,errors.New("cascade journal instant invalid")}
  plan,ok:=journal[5].([]any);if !ok||len(plan)==0{return 0,errors.New("cascade journal plan invalid")}
  if at==instant&&plan[0]==uint64(0){
   if len(plan)!=1{return 0,errors.New("cascade WORK plan malformed")}
   if count==math.MaxUint64{return 0,errors.New("cascade count overflow")}
   count++
  }
 }
 return count,nil
}

func (s *Store) CountSameInstantWork(ctx context.Context,run [32]byte,instant int64)(uint64,error){
 return countWorkAt(ctx,s.db,run,instant)
}

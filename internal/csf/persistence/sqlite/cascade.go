package sqlite

import (
 "context"
 "errors"
 "math"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
)

// CascadePolicyVersionV1 is the immutable CSFRV1-5 same-instant execution
// bound for the first base-policy runtime. Each subsequent policy change must
// issue a different bundle/version. This is a count of terminal WORK cycles,
// NOT the (possibly sparse) logical cycle ordinal.
const CascadePolicyVersionV1 uint64=1
const MaxSameInstantWorkV1 uint64=64

func countWorkAt(ctx context.Context,q queryOne,run [32]byte,instant int64)(uint64,error){
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

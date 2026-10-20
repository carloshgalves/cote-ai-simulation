package sqlite

import (
 "context"
 "database/sql"
 "errors"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/clock"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
)

// admitTerminalFence is called in the SAME SQLite transaction that inserts
// the immutable AdmissionFence. A halted run cannot skip explicit retry by
// publishing a second fence. Retry preserves the entire semantic cut.
func admitTerminalFence(ctx context.Context,tx *sql.Tx,record ports.CanonicalRecord)([]any,error){
 decoded,err:=codec.StrictDecodePayload(record.Canonical);if err!=nil{
  // Legacy ingress-guard fixtures exercise conditional writes before
  // bootstrapping a causal run. A bootstrapped run never bypasses this check.
  var count int
  if e:=tx.QueryRowContext(ctx,"SELECT count(*) FROM clock_states").Scan(&count);e!=nil{return nil,e}
  if count==0{return nil,nil}
  return nil,err
 }
 fence,ok:=decoded.([]any)
 if !ok||len(fence)!=18{return nil,errors.New("active run requires canonical AdmissionFence")}
 runBytes,ok:=fence[0].([]byte);if !ok||len(runBytes)!=32{return nil,errors.New("fence run missing")}
 cycleBytes,ok:=fence[1].([]byte);if !ok||len(cycleBytes)!=32{return nil,errors.New("fence cycle missing")}
 var run,cycle [32]byte;copy(run[:],runBytes);copy(cycle[:],cycleBytes)
 var clockBytes []byte
 err=tx.QueryRowContext(ctx,"SELECT canonical FROM clock_states WHERE run_id=?",run[:]).Scan(&clockBytes)
 if err==sql.ErrNoRows{return nil,nil} // no causal run: legacy standalone source guard
 if err!=nil{return nil,err}
 state,err:=clock.Decode(clockBytes);if err!=nil{return nil,err}
 at,ok:=asSigned64(fence[4])
 if !ok||at!=int64(state.Instant)||fence[7]!=uint64(state.Revision){
  return nil,errors.New("AdmissionFence clock/revision is stale")
 }
 ordinal,ok:=fence[5].(uint64)
 if !ok||state.LastOrdinal< -1||ordinal<=uint64(state.LastOrdinal){
  // When LastOrdinal=-1 no cycle has committed at this instant, so
  // ordinal=0 is valid. Handle that sentinel below.
  if state.LastOrdinal!= -1||!ok{return nil,errors.New("AdmissionFence ordinal not after last terminal")}
 }
 var persistedHash []byte
 if err:=tx.QueryRowContext(ctx,"SELECT state_hash FROM world_revisions WHERE revision=?",uint64(state.Revision)).Scan(&persistedHash);err!=nil{return nil,err}
 if !equalBytes(fence[8],persistedHash){return nil,errors.New("AdmissionFence world hash not authoritative")}
 attempt,ok:=fence[2].(uint64);if !ok||attempt>uint64(^uint32(0)){return nil,errors.New("fence attempt invalid")}
 id,err:=registeredDigest("cote.csf.id.admission-fence","cote.csf.schema.admission-fence-id-preimage",[]any{run[:],cycle[:],attempt})
 if err!=nil{return nil,err}
 hash,err:=registeredDigest("cote.csf.digest.fence","cote.csf.schema.admission-fence.body",fence[:17])
 if err!=nil{return nil,err}
 if id!=record.Identity||!equalBytes(fence[17],hash[:]){return nil,errors.New("fence root or digest unauthenticated")}
 previous,err:=cycleControl(ctx,tx,run);if err!=nil{return nil,err}
 var terminal any=noRef()
 switch{
 case previous==nil||previous[0]==uint64(0):
  if attempt!=0||!sameCanonical(fence[3],noRef()){return nil,errors.New("first fence cannot claim retry")}
 case previous[0]==uint64(1):
  if present(previous[1],cycle[:])&&present(previous[2],attempt)&&
   present(previous[3],fullRef(11,id[:],hash[:])){
   return previous,nil // byte-identical AdmissionFence re-delivery
  }
  return nil,errors.New("another AdmissionFence is already in flight")
 case previous[0]==uint64(3):
  return nil,errors.New("run HALTED_ON_ABORT until durable AttemptRetryRecord")
 case previous[0]==uint64(2):
  if attempt!=previous[6]||!present(previous[1],cycle[:])||
   !sameCanonical(fence[3],previous[4]){
   return nil,errors.New("retry fence does not follow authorized attempt/ref")
  }
  oldRef,ok:=previous[3].([]any)
  if !ok||len(oldRef)!=2{return nil,errors.New("retry predecessor fence ref missing")}
  causal,ok:=oldRef[1].([]any)
  if !ok||len(causal)!=3{return nil,errors.New("retry predecessor fence invalid")}
  oldID,ok:=causal[1].([]byte)
  if !ok||len(oldID)!=32{return nil,errors.New("retry predecessor ID invalid")}
  var oldCanonical []byte
  if err:=tx.QueryRowContext(ctx,"SELECT canonical FROM decision_ledger WHERE id=?",oldID).Scan(&oldCanonical);err!=nil{return nil,err}
  oldValue,err:=codec.StrictDecodePayload(oldCanonical);if err!=nil{return nil,err}
  old,ok:=oldValue.([]any)
  if !ok||len(old)!=18||!sameCanonical(causal[2],old[17]){return nil,errors.New("invalid predecessor fence")}
  for _,index:=range []int{0,1,4,5,6,7,8,9,10,11,12,13,14,15,16}{
   if !sameCanonical(fence[index],old[index]){return nil,errors.New("retry changed sealed semantic cut")}
  }
  terminal=previous[5]
 default:return nil,errors.New("unknown cycle control status")
 }
 control:=[]any{uint64(1),someRef(cycle[:]),someRef(attempt),
  someRef(fullRef(11,id[:],hash[:])),fence[3],terminal,attempt}
 return control,nil
}

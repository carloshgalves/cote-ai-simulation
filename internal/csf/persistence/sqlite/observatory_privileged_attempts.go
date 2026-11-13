package sqlite

import (
 "bytes"
 "context"
 "errors"
 "fmt"
 "sort"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/observatory"
)

type o2AttemptKey struct{
 cycle [32]byte
 attempt uint64
}

// Authenticate every persisted fence/abort/retry root, including historical
// aborts that are no longer projected by today's CycleControlState. These
// records must never be counted merely by consulting the current status.
func o2ReadAttempts(run [32]byte,records map[[32]byte]o2Record)(
 map[o2AttemptKey]*observatory.CausalAttempt,map[o2AttemptKey]o2Record,error){
 attempts:=map[o2AttemptKey]*observatory.CausalAttempt{}
 fences:=map[o2AttemptKey]o2Record{}
 aborts:=map[o2AttemptKey]o2Record{}
 retries:=map[o2AttemptKey]o2Record{}
 ids:=make([][32]byte,0,len(records))
 for id:=range records{ids=append(ids,id)}
 sort.Slice(ids,func(i,j int)bool{return bytes.Compare(ids[i][:],ids[j][:])<0})
 for _,id:=range ids{
  rec:=records[id]
  f:=rec.fields
  if len(f)!=18&&len(f)!=15&&len(f)!=7{continue}
  // DecisionLedger includes non-fence records. Classify only exact causal
  // run/cycle/attempt shapes; nonmatching types are not falsely reported.
  if len(f)<3||!equalBytes(f[0],run[:]){continue}
  cycle,err:=o2Bytes(f[1]);if err!=nil{return nil,nil,fmt.Errorf("decision_ledger %x cycle: %w",id,err)}
  attempt,err:=o2Uint(f[2]);if err!=nil{return nil,nil,fmt.Errorf("decision_ledger %x attempt: %w",id,err)}
  if attempt>uint64(^uint32(0)){return nil,nil,errors.New("attempt ordinal exceeds registered range")}
  k:=o2AttemptKey{cycle:cycle,attempt:attempt}
  switch len(f){
  case 18:
   expected,err:=registeredDigest("cote.csf.id.admission-fence","cote.csf.schema.admission-fence-id-preimage",
    []any{run[:],cycle[:],attempt})
   if err!=nil||[32]byte(expected)!=id{return nil,nil,fmt.Errorf("fence %x identity invalid",id)}
   digest,err:=registeredDigest("cote.csf.digest.fence","cote.csf.schema.admission-fence.body",f[:17])
   if err!=nil||!equalBytes(f[17],digest[:]){return nil,nil,fmt.Errorf("fence %x digest invalid",id)}
   instant,err:=o2Instant(f[4]);if err!=nil{return nil,nil,err}
   ordinal,err:=o2Uint(f[5]);if err!=nil{return nil,nil,err}
   if _,exists:=fences[k];exists{return nil,nil,fmt.Errorf("duplicate fence attempt for cycle %x",cycle)}
   fences[k]=rec
   attempts[k]=&observatory.CausalAttempt{
    CycleID:cycle,Instant:instant,Ordinal:ordinal,Attempt:attempt,
    Fence:o2Ref("decision_ledger",id,rec.bytes),
   }
  case 15:
   expected,err:=registeredDigest("cote.csf.id.cycle-abort","cote.csf.schema.cycle-abort-id-preimage",
    []any{run[:],cycle[:],attempt})
   if err!=nil||[32]byte(expected)!=id{return nil,nil,fmt.Errorf("abort %x identity invalid",id)}
   digest,err:=registeredDigest("cote.csf.digest.abort","cote.csf.schema.cycle-abort.body",f[:14])
   if err!=nil||!equalBytes(f[14],digest[:]){return nil,nil,fmt.Errorf("abort %x digest invalid",id)}
   reason,err:=o2Uint(f[10]);if err!=nil||reason>6{return nil,nil,fmt.Errorf("abort %x reason invalid",id)}
   if _,exists:=aborts[k];exists{return nil,nil,fmt.Errorf("duplicate abort %x",id)}
   aborts[k]=rec
  case 7:
   from,err:=o2Uint(f[2]);if err!=nil||from>=uint64(^uint32(0)){return nil,nil,fmt.Errorf("retry %x attempt invalid",id)}
   to,err:=o2Uint(f[3]);if err!=nil||to!=from+1{return nil,nil,fmt.Errorf("retry %x successor invalid",id)}
   expected,err:=registeredDigest("cote.csf.id.attempt-retry","cote.csf.schema.attempt-retry-id-preimage",
    []any{run[:],cycle[:],from,to})
   if err!=nil||[32]byte(expected)!=id{return nil,nil,fmt.Errorf("retry %x identity invalid",id)}
   reason,ok:=f[5].(string);if !ok||reason==""{return nil,nil,fmt.Errorf("retry %x reason missing",id)}
   if _,exists:=retries[k];exists{return nil,nil,fmt.Errorf("duplicate retry %x",id)}
   retries[k]=rec
  }
 }
 for k,record:=range aborts{
  attempt,ok:=attempts[k]
  if !ok{return nil,nil,fmt.Errorf("orphan abort %x missing fence",record.id)}
  fence:=fences[k].fields
  body:=record.fields
  for _,pair:=range [][2]int{{4,3},{5,4},{6,5},{7,6},{8,7},{12,9},{16,13}}{
   if !sameCanonical(fence[pair[0]],body[pair[1]]){return nil,nil,fmt.Errorf("abort %x frozen fence coordinate/policy mismatch",record.id)}
  }
  if !sameCanonical(fence[17],body[8]){return nil,nil,fmt.Errorf("abort %x fence digest mismatch",record.id)}
  if _,ok:=body[11].([]any);!ok{return nil,nil,fmt.Errorf("abort %x subjects malformed",record.id)}
  evidence,ok:=body[12].([]any)
  if !ok||len(evidence)!=0{return nil,nil,fmt.Errorf("abort %x has unapproved evidence refs",record.id)}
  ref:=o2Ref("decision_ledger",record.id,record.bytes)
  reason:=body[10].(uint64)
  attempt.Abort=&ref
  attempt.AbortReason=&reason
 }
 for k,rec:=range retries{
  attempt,ok:=attempts[k]
  if !ok||attempt.Abort==nil{return nil,nil,fmt.Errorf("orphan retry %x missing authenticated abort",rec.id)}
  from:=rec.fields[2].(uint64)
  if from!=k.attempt{return nil,nil,fmt.Errorf("retry %x from foreign attempt",rec.id)}
  abort:=aborts[k]
  dg,err:=registeredDigest("cote.csf.digest.abort","cote.csf.schema.cycle-abort.body",abort.fields[:14])
  if err!=nil{return nil,nil,err}
  if !sameCanonical(rec.fields[4],[]any{uint64(13),abort.id[:],dg[:]})||
   !sameCanonical(rec.fields[6],abort.fields[13]){
   return nil,nil,fmt.Errorf("retry %x does not cite exact aborted policy cut",rec.id)
  }
  ref:=o2Ref("decision_ledger",rec.id,rec.bytes)
  attempt.Retry=&ref
 }
 return attempts,fences,nil
}

func o2FinalizeAttempts(state observatory.RunState,attempts map[o2AttemptKey]*observatory.CausalAttempt)([]observatory.CausalAttempt,error){
 out:=make([]observatory.CausalAttempt,0,len(attempts))
 var unresolved []*observatory.CausalAttempt
 for _,attempt:=range attempts{
  if attempt.Commit!=nil&&attempt.Abort!=nil{return nil,errors.New("one attempt has both committed and aborted terminals")}
  if attempt.Commit==nil&&attempt.Abort==nil{unresolved=append(unresolved,attempt)}
  if attempt.Retry!=nil&&attempt.Abort==nil{return nil,errors.New("retry has no authenticated abort")}
  out=append(out,*attempt)
 }
 sort.Slice(out,func(i,j int)bool{
  a,b:=out[i],out[j]
  if a.Instant!=b.Instant{return a.Instant<b.Instant}
  if a.Ordinal!=b.Ordinal{return a.Ordinal<b.Ordinal}
  if c:=bytes.Compare(a.CycleID[:],b.CycleID[:]);c!=0{return c<0}
  return a.Attempt<b.Attempt
 })
 ctl:=state.Control
 switch ctl.Status{
 case observatory.IDLE:
  if len(unresolved)>0{return nil,errors.New("IDLE run has unfinished AdmissionFence")}
 case observatory.ATTEMPT_IN_FLIGHT:
  if len(unresolved)!=1||ctl.CycleID==nil||ctl.Attempt==nil||
   unresolved[0].CycleID!=*ctl.CycleID||unresolved[0].Attempt!=*ctl.Attempt{
   return nil,errors.New("in-flight projection does not match sole unfinished fence")
  }
 case observatory.HALTED_ON_ABORT,observatory.RETRY_AUTHORIZED:
  if len(unresolved)!=0||ctl.CycleID==nil||ctl.Attempt==nil{
   return nil,errors.New("terminal control retained an unfinished fence")
  }
  found:=false
  for i:=range out{
   a:=&out[i]
   if a.CycleID==*ctl.CycleID&&a.Attempt==*ctl.Attempt{
    if a.Abort==nil||(ctl.Status==observatory.HALTED_ON_ABORT&&a.Retry!=nil)||
     (ctl.Status==observatory.RETRY_AUTHORIZED&&a.Retry==nil){
     return nil,errors.New("control abort/retry projection diverged from persisted terminal")
    }
    found=true;break
   }
  }
  if !found{return nil,errors.New("active terminal control lacks authenticated abort")}
 default:return nil,errors.New("unknown privileged control status")
 }
 // A successful committed retry may leave a historical abort/retry pair.
 // Check each successor, never invent a second attempt automatically.
 for _,a:=range out{
  if a.Attempt==0{continue}
  prior,ok:=attempts[o2AttemptKey{cycle:a.CycleID,attempt:a.Attempt-1}]
  if !ok||prior.Abort==nil||prior.Retry==nil{
   return nil,fmt.Errorf("attempt %d lacks authenticated predecessor retry",a.Attempt)
  }
 }
 return out,nil
}

// Closure proofs use the same canonical input ledger as the admission
// coordinator, not a separate operational list.
func(q *PrivilegedReader) o2Closures(ctx context.Context,run [32]byte,fence []any)([]observatory.AuthorityRef,error){
 roster,ok:=fence[9].([]any);if !ok{return nil,errors.New("fence closure proofs malformed")}
 out:=make([]observatory.AuthorityRef,0,len(roster))
 for _,item:=range roster{
  p,ok:=item.([]any);if !ok||len(p)!=5{return nil,errors.New("closure proof tuple malformed")}
  source,ok:=p[0].(string);if !ok||source==""{return nil,errors.New("closure source missing")}
  ref,ok:=p[1].([]any);if !ok||len(ref)!=3||ref[0]!=uint64(2){return nil,errors.New("closure ref is not input-ledger closure")}
  id,err:=o2Bytes(ref[1]);if err!=nil{return nil,err}
  digest,err:=o2Bytes(ref[2]);if err!=nil{return nil,err}
  auth,body,err:=q.readRef(ctx,"input_ledger",id);if err!=nil{return nil,err}
  if len(body)!=7||!equalBytes(body[0],id[:])||!equalBytes(body[1],run[:])||body[2]!=source||body[6]!=p[2]||
   !sameCanonical(body[4],p[3])||!sameCanonical(body[5],p[4]){
   return nil,fmt.Errorf("closure %x source/sequence/coordinate unbound",id)
  }
  derived,err:=registeredDigest("cote.csf.id.source-closure","cote.csf.schema.source-closure-id-preimage",[]any{run[:],source,body[3]})
  if err!=nil||[32]byte(derived)!=id{return nil,fmt.Errorf("closure %x ID invalid",id)}
  want,err:=registeredDigest("cote.csf.digest.source-closure","cote.csf.schema.source-closure",body)
  if err!=nil||[32]byte(want)!=digest{return nil,fmt.Errorf("closure %x digest invalid",id)}
  out=append(out,auth)
 }
 return out,nil
}
func(q *PrivilegedReader) o2Admitted(ctx context.Context,run [32]byte,fence []any)([]observatory.AuthorityRef,error){
 roster,ok:=fence[11].([]any);if !ok{return nil,errors.New("fence admitted unit roster malformed")}
 out:=make([]observatory.AuthorityRef,0,len(roster))
 for _,item:=range roster{
  p,ok:=item.([]any);if !ok||len(p)!=5{return nil,errors.New("fence admitted unit malformed")}
  kind,err:=o2Uint(p[1]);if err!=nil{return nil,err}
  id,err:=o2Bytes(p[3]);if err!=nil{return nil,err}
  digest,err:=o2Bytes(p[4]);if err!=nil{return nil,err}
  table:="input_ledger"
  switch kind{case 0,1:case 2:table="scheduled_occurrences";case 3:table="trigger_activations"
  default:return nil,fmt.Errorf("unregistered admitted source kind %d",kind)}
  ref,body,err:=q.readRef(ctx,table,id);if err!=nil{return nil,err}
  if len(body)==0||!equalBytes(body[0],id[:]){return nil,fmt.Errorf("admitted source %x root mismatch",id)}
  if kind==2||kind==3{
   var expected []byte
   if kind==2{if len(body)!=15{return nil,errors.New("scheduled source schema invalid")};expected,_=body[14].([]byte)
   }else{if len(body)!=12{return nil,errors.New("trigger source schema invalid")};expected,_=body[11].([]byte)}
   if !bytes.Equal(expected,digest[:])||!equalBytes(body[1],run[:]){
    return nil,fmt.Errorf("admitted system unit %x digest/run invalid",id)
   }
  }else if kind==1{
   if len(body)!=13||!bytes.Equal(digest[:],func()[]byte{raw,_:=body[12].([]byte);return raw}()){
    return nil,fmt.Errorf("admitted exogenous input %x digest invalid",id)
   }
  }
  out=append(out,ref)
 }
 return out,nil
}

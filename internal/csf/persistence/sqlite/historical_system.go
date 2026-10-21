package sqlite

import (
 "context"
 "errors"
 "fmt"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/clock"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
)

// historicalSystemUnits reconstructs whether each immutable schedule/trigger
// source existed and was still pending at the particular REPLAY revision.
// The live pending projection deliberately excludes all terminal units, so
// reusing its current membership for previous cycle proofs would be wrong.
func (s *Store) historicalSystemUnits(ctx context.Context,run [32]byte,revision uint64)([]clock.Pending,error){
 type historical struct{id domain.ID;kind uint8;canonical []byte;eligibility domain.EligibilityCoordinate}
 all:=map[domain.ID]historical{}
 for _,entry:=range []struct{kind uint8;table string}{{2,"scheduled_occurrences"},{3,"trigger_activations"}}{
  rows,err:=s.db.QueryContext(ctx,"SELECT id,canonical FROM "+entry.table+" WHERE run_id=?",run[:])
  if err!=nil{return nil,err}
  for rows.Next(){
   var id,raw []byte
   if err:=rows.Scan(&id,&raw);err!=nil{rows.Close();return nil,err}
   if len(id)!=32{rows.Close();return nil,errors.New("historical system source ID invalid")}
   var key [32]byte;copy(key[:],id)
   var coord domain.EligibilityCoordinate
   var life uint64
   if entry.kind==2{coord,life,err=validateScheduledOccurrence(run,key,raw)
   }else{coord,life,err=validateTriggerActivation(run,key,raw)}
   if err!=nil||life!=0{rows.Close();return nil,errors.New("historical system source was not immutable pending identity")}
   if _,duplicate:=all[domain.ID(key)];duplicate{rows.Close();return nil,errors.New("duplicate schedule/trigger source identity")}
   all[domain.ID(key)]=historical{id:domain.ID(key),kind:entry.kind,canonical:raw,eligibility:coord}
  }
  if err:=rows.Err();err!=nil{rows.Close();return nil,err}
  if err:=rows.Close();err!=nil{return nil,err}
 }
 // The entire immutable journal supplies the event publication revision
 // and each system source's unique terminal revision.
 eventAt:=map[string]uint64{}
 settledAt:=map[domain.ID]uint64{}
 successorBornAt:=map[domain.ID]uint64{}
 rows,err:=s.db.QueryContext(ctx,"SELECT commit_id FROM committed_cycles WHERE run_id=?",run[:])
 if err!=nil{return nil,err}
 var commits [][]byte
 for rows.Next(){
  var id []byte
  if err:=rows.Scan(&id);err!=nil{rows.Close();return nil,err}
  if len(id)!=32{rows.Close();return nil,errors.New("historical commit ID malformed")}
  commits=append(commits,id)
 }
 if err:=rows.Err();err!=nil{rows.Close();return nil,err}
 if err:=rows.Close();err!=nil{return nil,err}
 for _,commitID:=range commits{
  var raw []byte
  if err:=s.db.QueryRowContext(ctx,"SELECT canonical FROM decision_ledger WHERE id=?",commitID).Scan(&raw);err!=nil{return nil,err}
  decoded,err:=codec.StrictDecodePayload(raw);if err!=nil{return nil,err}
  fields,ok:=decoded.([]any)
  if !ok||len(fields)!=24||!equalBytes(fields[0],run[:]){return nil,errors.New("historical CycleCommit invalid")}
  at,ok:=fields[8].(uint64);if !ok{return nil,errors.New("historical result revision malformed")}
  events,ok:=fields[15].([]any);if !ok{return nil,errors.New("historical event IDs absent")}
  for _,v:=range events{
   id,ok:=v.([]byte);if !ok||len(id)!=32{return nil,errors.New("historical event ID invalid")}
   if _,dup:=eventAt[string(id)];dup{return nil,errors.New("event committed in multiple revisions")}
   eventAt[string(id)]=at
  }
  decisionIDs,ok:=fields[16].([]any)
  if !ok{return nil,errors.New("historical terminal decisions missing")}
  for _,decisionID:=range decisionIDs{
   id,ok:=decisionID.([]byte)
   if !ok||len(id)!=32{return nil,errors.New("historical decision ID invalid")}
   var decisionRaw []byte
   if err:=s.db.QueryRowContext(ctx,"SELECT canonical FROM decision_ledger WHERE id=?",id).Scan(&decisionRaw);err!=nil{return nil,err}
   parsed,err:=codec.StrictDecodePayload(decisionRaw);if err!=nil{return nil,err}
   decision,ok:=parsed.([]any)
   if !ok||len(decision)!=16{return nil,errors.New("historical terminal DecisionRecord malformed")}
   sourceRef,ok:=decision[4].([]any)
   if !ok||len(sourceRef)<3||sourceRef[0]!=uint64(2){continue}
   if decision[5]!=uint64(2){continue}
   opt,ok:=decision[13].([]any)
   if !ok||len(opt)!=2||opt[0]!=uint64(1){return nil,errors.New("deferred source lost successor reference")}
   child,ok:=opt[1].([]byte)
   if !ok||len(child)!=32{return nil,errors.New("deferred successor identity malformed")}
   var key domain.ID;copy(key[:],child)
   if _,duplicate:=successorBornAt[key];duplicate{return nil,errors.New("successor created more than once")}
   successorBornAt[key]=at
  }
  admitted,ok:=fields[11].([]any);if !ok{return nil,errors.New("historical unit set missing")}
  for _,v:=range admitted{
   id,ok:=v.([]byte);if !ok||len(id)!=32{return nil,errors.New("historical unit ID invalid")}
   var key domain.ID;copy(key[:],id)
   if _,system:=all[key];!system{continue}
   if _,dup:=settledAt[key];dup{return nil,errors.New("system source settled twice")}
   settledAt[key]=at
  }
 }
 pending:=make([]clock.Pending,0,len(all))
 for id,source:=range all{
  decoded,err:=codec.StrictDecodePayload(source.canonical);if err!=nil{return nil,err}
  fields:=decoded.([]any)
  var created uint64
  if source.kind==2{
   origin,ok:=fields[9].([]any);if !ok||len(origin)!=2{return nil,errors.New("occurrence lacks historical origin")}
   v,ok:=origin[0].(uint64);if !ok{return nil,errors.New("occurrence origin kind malformed")}
   if v==2{
    created=0
   }else if v==0{
    ref,ok:=origin[1].([]any);if !ok||len(ref)!=2{return nil,errors.New("occurrence event ref malformed")}
    eid,ok:=ref[0].([]byte);if !ok||len(eid)!=32{return nil,errors.New("occurrence parent event ID invalid")}
    at,exists:=eventAt[string(eid)]
    if !exists{return nil,errors.New("occurrence successor event absent from terminal journal")}
    declared,authorized:=successorBornAt[id]
    if !authorized||declared!=at{
     return nil,errors.New("occurrence successor not authorized by committed DEFER decision")
    }
    var eventRaw []byte
    if err:=s.db.QueryRowContext(ctx,"SELECT canonical FROM event_store WHERE id=?",eid).Scan(&eventRaw);err!=nil{return nil,err}
    eventValue,err:=codec.StrictDecodePayload(eventRaw);if err!=nil{return nil,err}
    parentDigest,valid:=ref[1].([]byte)
    if !valid||len(parentDigest)!=32{return nil,errors.New("occurrence parent digest malformed")}
    ev,ok:=eventValue.([]any);if !ok||len(ev)!=19||!equalBytes(ev[18],parentDigest){
     return nil,errors.New("occurrence origin event-digest provenance mismatch")
    }
    created=at
   }else{return nil,fmt.Errorf("unsupported occurrence origin %d",v)}
  }else{
   origin,ok:=fields[5].([]any);if !ok||len(origin)<2{return nil,errors.New("activation historical origin invalid")}
   v,ok:=origin[0].(uint64);if !ok{return nil,errors.New("activation origin tag malformed")}
   if v==1{created=0}else if v==0{
    created,ok=origin[2].(uint64);if !ok{return nil,errors.New("activation revision malformed")}
   }else{return nil,errors.New("unknown activation origin")}
  }
  if created>revision{continue}
  if terminal,ok:=settledAt[id];ok&&terminal<=revision{continue}
  pending=append(pending,clock.Pending{ID:id,Eligibility:source.eligibility})
 }
 return pending,nil
}

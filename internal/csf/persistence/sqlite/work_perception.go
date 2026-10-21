package sqlite

import (
 "bytes"
 "context"
 "errors"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/perception"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
)

func verifyWorkPerception(b ports.WorkBatch,journal []any,genesis []any)([]ports.CanonicalRecord,error){
 if len(journal)!=24||len(genesis)!=18{return nil,errors.New("invalid perception authority shapes")}
 source,ok:=genesis[14].([]any)
 if !ok{return nil,errors.New("perception Genesis policy roster missing")}
 byID:=map[string][]any{}
 canonical:=make([][]any,len(source))
 for i,v:=range source{
  fields,ok:=v.([]any)
  if !ok||len(fields)!=3{return nil,errors.New("invalid perception genesis policy ref")}
  name,ok:=fields[0].(string)
  if !ok||byID[name]!=nil{return nil,errors.New("duplicate or invalid perception policy name")}
  byID[name]=fields
  canonical[i]=fields
 }
 before,ok:=journal[6].(uint64)
 if !ok{return nil,errors.New("perception base revision malformed")}
 after,ok:=journal[8].(uint64)
 if !ok{return nil,errors.New("perception result revision malformed")}
 policyRef,ok:=journal[20].([]any)
 if !ok{return nil,errors.New("perception classifier ref malformed")}
 identityRef,ok:=journal[21].([]any)
 if !ok{return nil,errors.New("perception identity ref malformed")}
 expected,err:=perception.PlanPinned(canonical,domain.ID(b.RunID),domain.ID(b.CycleID),
  domain.WorldRevision(before),domain.WorldRevision(after),b.Events,
  perception.Pins{Classification:policyRef,Resolver:byID["validators-v1"],Identity:identityRef})
 if err!=nil{return nil,err}
 ids:=make([]any,len(expected))
 for i,item:=range expected{ids[i]=item.Identity[:]}
 actualIDs,ok:=journal[18].([]any)
 if !ok||!sameCanonical(actualIDs,ids)||len(b.PerceptionTasks)!=len(expected){
  return nil,errors.New("WORK task references are not exactly the registered potential-event projection")
 }
 digest,err:=registeredDigest("cote.csf.digest.perception-task-list",
  "cote.csf.schema.perception-task-id-list",ids)
 if err!=nil||!sameCanonical(journal[19],digest[:]){
  return nil,errors.New("WORK perception task-list digest mismatch")
 }
 for i,task:=range expected{
  actual:=b.PerceptionTasks[i]
  if actual.Identity!=task.Identity||!bytes.Equal(actual.Canonical,task.Canonical){
   return nil,errors.New("WORK task contents/identity differ from replayable genesis-pinned policy")
  }
 }
 return expected,nil
}

// verifyPublishedWorkTasks re-derives the actual causal outbox tasks from the
// committed EventStore and the verified Genesis pin. It never trusts a list
// of task IDs from the journal without verifying the stored task bodies.
func verifyPublishedWorkTasks(ctx context.Context,q queryOne,run,cycle [32]byte,journal,genesis []any,
 events [][]byte)error{
 ids,ok:=journal[15].([]any)
 if !ok||len(ids)!=len(events){return errors.New("perception replay EventStore projection incomplete")}
 evs:=make([]ports.CanonicalRecord,len(events))
 for i,canonical:=range events{
  id,ok:=ids[i].([]byte)
  if !ok||len(id)!=32{return errors.New("perception replay event identity invalid")}
  var key [32]byte;copy(key[:],id)
  evs[i]=ports.CanonicalRecord{Identity:key,Canonical:canonical}
 }
 b:=ports.WorkBatch{RunID:run,CycleID:cycle,Events:evs}
 listed,ok:=journal[18].([]any)
 if !ok{return errors.New("perception replay task list invalid")}
 b.PerceptionTasks=make([]ports.CanonicalRecord,len(listed))
 for i,v:=range listed{
  id,ok:=v.([]byte)
  if !ok||len(id)!=32{return errors.New("perception replay task identity malformed")}
  var raw []byte
  if err:=q.QueryRowContext(ctx,"SELECT canonical FROM causal_outbox WHERE id=?",id).Scan(&raw);err!=nil{return err}
  var key [32]byte;copy(key[:],id)
  b.PerceptionTasks[i]=ports.CanonicalRecord{Identity:key,Canonical:raw}
 }
 _,err:=verifyWorkPerception(b,journal,genesis)
 return err
}

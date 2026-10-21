package sqlite

import (
 "context"
 "errors"
 "math"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/clock"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
)

// verifyPublishedHybrid rehydrates every source from the immutable SQLite
// ledger. Replay is not permitted to trust caller-selected partial outputs.
func verifyPublishedHybrid(ctx context.Context,q queryOne,run,cycle,commit [32]byte,
 journal,fence []any,eventRaw []any)([]domain.ID,error){
 units,ok:=fence[11].([]any)
 if !ok||len(units)<2{return nil,errors.New("hybrid admission malformed")}
 names,ok:=journal[16].([]any)
 if !ok||len(names)!=len(units){return nil,errors.New("hybrid persisted decision list incomplete")}
 decisions:=make([]ports.CanonicalRecord,len(names))
 candidates:=make([]ports.CanonicalRecord,0)
 children:=make([]ports.CanonicalRecord,0)
 sources:=make([]domain.ID,len(units))
 for i,item:=range units{
  fields,ok:=item.([]any)
  if !ok||len(fields)!=5{return nil,errors.New("hybrid source ref malformed")}
  id,ok:=fields[3].([]byte)
  if !ok||len(id)!=32{return nil,errors.New("hybrid source ID invalid")}
  copy(sources[i][:],id)
  decisionID,ok:=names[i].([]byte)
  if !ok||len(decisionID)!=32{return nil,errors.New("hybrid DecisionRecord ID invalid")}
  var raw []byte
  if err:=q.QueryRowContext(ctx,"SELECT canonical FROM decision_ledger WHERE id=?",decisionID).Scan(&raw);err!=nil{return nil,err}
  decoded,err:=codec.StrictDecodePayload(raw);if err!=nil{return nil,err}
  decision,ok:=decoded.([]any)
  if !ok||len(decision)!=16{return nil,errors.New("hybrid decision malformed")}
  var key [32]byte;copy(key[:],decisionID)
  decisions[i]=ports.CanonicalRecord{Identity:key,Canonical:raw}
  if fields[1]==uint64(0){continue}
  choice,ok:=decision[6].([]any)
  if !ok||len(choice)!=2||choice[0]!=uint64(1){return nil,errors.New("hybrid system candidate reference missing")}
  candidateID,ok:=choice[1].([]byte)
  if !ok||len(candidateID)!=32{return nil,errors.New("hybrid candidate ID invalid")}
  var candidateRaw []byte
  if err:=q.QueryRowContext(ctx,"SELECT canonical FROM decision_ledger WHERE id=?",candidateID).Scan(&candidateRaw);err!=nil{return nil,err}
  var cid [32]byte;copy(cid[:],candidateID)
  candidates=append(candidates,ports.CanonicalRecord{Identity:cid,Canonical:candidateRaw})
  option,ok:=decision[13].([]any)
  if !ok{return nil,errors.New("hybrid successor option invalid")}
  if len(option)==2&&option[0]==uint64(1){
   childID,ok:=option[1].([]byte)
   if !ok||len(childID)!=32{return nil,errors.New("hybrid child ID invalid")}
   var next []byte
   if err:=q.QueryRowContext(ctx,"SELECT canonical FROM scheduled_occurrences WHERE run_id=? AND id=?",
    run[:],childID).Scan(&next);err!=nil{return nil,err}
   var sid [32]byte;copy(sid[:],childID)
   children=append(children,ports.CanonicalRecord{Identity:sid,Canonical:next})
  }else if len(option)!=1||option[0]!=uint64(0){return nil,errors.New("hybrid unregistered successor")}
 }
 if len(candidates)==0{return nil,errors.New("hybrid has no system candidate")}
 eventIDs,ok:=journal[15].([]any)
 if !ok||len(eventIDs)!=len(eventRaw){return nil,errors.New("hybrid event list incomplete")}
 events:=make([]ports.CanonicalRecord,len(eventIDs))
 for i,v:=range eventIDs{
  id,ok:=v.([]byte)
  raw,valid:=eventRaw[i].([]byte)
  if !ok||!valid||len(id)!=32{return nil,errors.New("hybrid event identity/canonical malformed")}
  var key [32]byte;copy(key[:],id)
  events[i]=ports.CanonicalRecord{Identity:key,Canonical:raw}
 }
 revision,ok:=journal[6].(uint64)
 if !ok||revision==math.MaxUint64{return nil,errors.New("hybrid predecessor revision malformed")}
 var baseRaw,resultRaw []byte
 if err:=q.QueryRowContext(ctx,"SELECT canonical FROM world_revisions WHERE revision=?",revision).Scan(&baseRaw);err!=nil{return nil,err}
 if err:=q.QueryRowContext(ctx,"SELECT canonical FROM world_revisions WHERE revision=?",revision+1).Scan(&resultRaw);err!=nil{return nil,err}
 baseValue,err:=codec.StrictDecodePayload(baseRaw);if err!=nil{return nil,err}
 resultValue,err:=codec.StrictDecodePayload(resultRaw);if err!=nil{return nil,err}
 before,ok:=baseValue.([]any)
 if !ok||len(before)!=6{return nil,errors.New("hybrid predecessor WorldRevision malformed")}
 after,ok:=resultValue.([]any)
 if !ok||len(after)!=6{return nil,errors.New("hybrid successor WorldRevision malformed")}
 previousHash,ok:=before[5].([]byte)
 if !ok||len(previousHash)!=32{return nil,errors.New("hybrid world hash malformed")}
 var graw []byte
 if err:=q.QueryRowContext(ctx,"SELECT canonical FROM genesis_records WHERE run_id=?",run[:]).Scan(&graw);err!=nil{return nil,err}
 decoded,err:=codec.StrictDecodeEnvelope(graw);if err!=nil{return nil,err}
 genesis,ok:=decoded.Value.([]any)
 if !ok||len(genesis)!=18{return nil,errors.New("hybrid Genesis malformed")}
 refs,ok:=genesis[14].([]any)
 if !ok{return nil,errors.New("hybrid pinned policy list missing")}
 allowed:=map[string]any{}
 for _,entry:=range refs{
  fields,ok:=entry.([]any)
  if !ok||len(fields)!=3{return nil,errors.New("hybrid policy ref malformed")}
  name,ok:=fields[0].(string)
  if !ok{return nil,errors.New("hybrid policy ID invalid")}
  allowed[name]=fields
 }
 instant,ok:=asSigned64(journal[3]);if !ok{return nil,errors.New("hybrid logical instant invalid")}
 ordinal,ok:=journal[4].(uint64)
 if !ok||ordinal>math.MaxInt64{return nil,errors.New("hybrid ordinal invalid")}
 end,ok:=journal[22].(uint64)
 if !ok||end<uint64(len(events)){return nil,errors.New("hybrid logical sequence underflow")}
 prior:=clock.State{Instant:domain.SimulationInstant(instant),Revision:domain.WorldRevision(revision),
  NextLogicalSequence:domain.LogicalSequence(end-uint64(len(events))),LastOrdinal:int64(ordinal)-1}
 next:=prior;next.Revision++;next.NextLogicalSequence=domain.LogicalSequence(end);next.LastOrdinal=int64(ordinal)
 var hash [32]byte;copy(hash[:],previousHash)
 batch:=ports.WorkBatch{Mode:4,RunID:run,CycleID:cycle,
  Decisions:decisions,Events:events,SystemCandidate:candidates[0],SystemExtraCandidates:candidates[1:],
  SystemSuccessors:children,BaseStateHash:hash,ResultStateHash:hash,
  RevisionCanonical:resultRaw,Journal:ports.CanonicalRecord{Identity:commit}}
 // The source ledger also stores the declared RoundDeclaration slots. Rebuild
 // expectedSlots from the admitted slot responses and each authenticated
 // RoundDeclaration; partial slot cohorts are rejected by verifyHybridTerminal.
 expectedSlots:=map[domain.ID]map[string]bool{}
 for _,unit:=range units{
  fields:=unit.([]any)
  if fields[1]!=uint64(0){continue}
  source:=fields[3].([]byte)
  var raw []byte
  if err:=q.QueryRowContext(ctx,"SELECT canonical FROM input_ledger WHERE id=?",source).Scan(&raw);err!=nil{return nil,err}
  decoded,err:=codec.StrictDecodePayload(raw);if err!=nil{return nil,err}
  response,ok:=decoded.([]any)
  if !ok||len(response)!=11{return nil,errors.New("hybrid slot response malformed")}
  roundID,ok:=response[4].([]byte)
  if !ok||len(roundID)!=32{return nil,errors.New("hybrid round ID malformed")}
  var key domain.ID;copy(key[:],roundID)
  if expectedSlots[key]!=nil{continue}
  var roundRaw []byte
  if err:=q.QueryRowContext(ctx,"SELECT canonical FROM input_ledger WHERE id=?",roundID).Scan(&roundRaw);err!=nil{return nil,err}
  decodedRound,err:=codec.StrictDecodePayload(roundRaw);if err!=nil{return nil,err}
  round,ok:=decodedRound.([]any)
  if !ok||len(round)!=11{return nil,errors.New("hybrid RoundDeclaration malformed")}
  members,ok:=round[4].([]any)
  if !ok||len(members)==0{return nil,errors.New("hybrid round declared no slots")}
  all:=map[string]bool{}
  for _,v:=range members{
   slot,ok:=v.(string)
   if !ok||slot==""||all[slot]{return nil,errors.New("hybrid RoundDeclaration slot malformed")}
   all[slot]=true
  }
  expectedSlots[key]=all
 }
 if err:=verifyHybridTerminal(ctx,q,batch,prior,next,fence,journal,before,after,allowed,expectedSlots);err!=nil{return nil,err}
 return sources,nil
}

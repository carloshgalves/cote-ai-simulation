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

func verifyPublishedSystemBatch(ctx context.Context,q queryOne,run,cycle,commit [32]byte,
 journal,fence []any,eventRaw []any)([]domain.ID,error){
 units,ok:=fence[11].([]any)
 if !ok||len(units)<2{return nil,errors.New("multi-system replay needs two or more unit refs")}
 n:=len(units)
 ids,ok:=journal[16].([]any)
 if !ok||len(ids)!=n||len(eventRaw)!=n{return nil,errors.New("multi-system replay not bijective")}
 events:=make([]ports.CanonicalRecord,n)
 eventIDs,ok:=journal[15].([]any)
 if !ok||len(eventIDs)!=n{return nil,errors.New("multi-system event references missing")}
 for i,item:=range eventRaw{
  id,ok:=eventIDs[i].([]byte)
  raw,valid:=item.([]byte)
  if !ok||!valid||len(id)!=32{return nil,errors.New("multi-system Event bytes or ID malformed")}
  var key [32]byte;copy(key[:],id)
  events[i]=ports.CanonicalRecord{Identity:key,Canonical:raw}
 }
 decisions:=make([]ports.CanonicalRecord,n)
 candidates:=make([]ports.CanonicalRecord,n)
 successors:=make([]ports.CanonicalRecord,0)
 sources:=make([]domain.ID,n)
 for i,v:=range units{
  unit,ok:=v.([]any)
  if !ok||len(unit)!=5{return nil,errors.New("multi-system admission malformed")}
  sourceRaw,ok:=unit[3].([]byte)
  if !ok||len(sourceRaw)!=32{return nil,errors.New("multi-system admitted ID invalid")}
  copy(sources[i][:],sourceRaw)
  decisionID,ok:=ids[i].([]byte)
  if !ok||len(decisionID)!=32{return nil,errors.New("multi-system DecisionRecord ID malformed")}
  var decisionBytes []byte
  if err:=q.QueryRowContext(ctx,"SELECT canonical FROM decision_ledger WHERE id=?",decisionID).Scan(&decisionBytes);err!=nil{return nil,err}
  decoded,err:=codec.StrictDecodePayload(decisionBytes);if err!=nil{return nil,err}
  fields,ok:=decoded.([]any)
  if !ok||len(fields)!=16{return nil,errors.New("multi-system DecisionRecord malformed")}
  var did [32]byte;copy(did[:],decisionID)
  decisions[i]=ports.CanonicalRecord{Identity:did,Canonical:decisionBytes}
  candidateRef,ok:=fields[6].([]any)
  if !ok||len(candidateRef)!=2||candidateRef[0]!=uint64(1){return nil,errors.New("multi-system candidate reference missing")}
  cid,ok:=candidateRef[1].([]byte)
  if !ok||len(cid)!=32{return nil,errors.New("multi-system candidate ID malformed")}
  var candidateRaw []byte
  if err:=q.QueryRowContext(ctx,"SELECT canonical FROM decision_ledger WHERE id=?",cid).Scan(&candidateRaw);err!=nil{return nil,err}
  var candidateID [32]byte;copy(candidateID[:],cid)
  candidates[i]=ports.CanonicalRecord{Identity:candidateID,Canonical:candidateRaw}
  opt,ok:=fields[13].([]any)
  if !ok{return nil,errors.New("multi-system successor ref malformed")}
  if len(opt)==2&&opt[0]==uint64(1){
   childID,ok:=opt[1].([]byte)
   if !ok||len(childID)!=32{return nil,errors.New("multi-system child ID malformed")}
   var raw []byte
   if err:=q.QueryRowContext(ctx,"SELECT canonical FROM scheduled_occurrences WHERE run_id=? AND id=?",run[:],childID).Scan(&raw);err!=nil{return nil,err}
   var id [32]byte;copy(id[:],childID)
   successors=append(successors,ports.CanonicalRecord{Identity:id,Canonical:raw})
  }else if len(opt)!=1||opt[0]!=uint64(0){return nil,errors.New("multi-system unregistered successor option")}
 }
 base,ok:=journal[6].(uint64)
 if !ok||base==math.MaxUint64{return nil,errors.New("multi-system predecessor revision invalid")}
 var beforeRaw,afterRaw []byte
 if err:=q.QueryRowContext(ctx,"SELECT canonical FROM world_revisions WHERE revision=?",base).Scan(&beforeRaw);err!=nil{return nil,err}
 if err:=q.QueryRowContext(ctx,"SELECT canonical FROM world_revisions WHERE revision=?",base+1).Scan(&afterRaw);err!=nil{return nil,err}
 beforeValue,err:=codec.StrictDecodePayload(beforeRaw);if err!=nil{return nil,err}
 afterValue,err:=codec.StrictDecodePayload(afterRaw);if err!=nil{return nil,err}
 predecessor,ok:=beforeValue.([]any)
 if !ok||len(predecessor)!=6{return nil,errors.New("multi-system base WorldRevision invalid")}
 after,ok:=afterValue.([]any)
 if !ok||len(after)!=6{return nil,errors.New("multi-system result WorldRevision invalid")}
 h,ok:=predecessor[5].([]byte)
 if !ok||len(h)!=32{return nil,errors.New("multi-system base hash invalid")}
 var genesisRaw []byte
 if err:=q.QueryRowContext(ctx,"SELECT canonical FROM genesis_records WHERE run_id=?",run[:]).Scan(&genesisRaw);err!=nil{return nil,err}
 envelope,err:=codec.StrictDecodeEnvelope(genesisRaw);if err!=nil{return nil,err}
 genesis,ok:=envelope.Value.([]any)
 if !ok||len(genesis)!=18{return nil,errors.New("multi-system Genesis malformed")}
 refs,ok:=genesis[14].([]any)
 if !ok{return nil,errors.New("multi-system policy refs missing")}
 allowed:=map[string]any{}
 for _,v:=range refs{
  ref,ok:=v.([]any)
  if !ok||len(ref)!=3{return nil,errors.New("multi-system policy ref malformed")}
  id,ok:=ref[0].(string)
  if !ok{return nil,errors.New("multi-system policy ID malformed")}
  allowed[id]=ref
 }
 now,ok:=asSigned64(journal[3])
 if !ok{return nil,errors.New("multi-system instant malformed")}
 ordinal,ok:=journal[4].(uint64)
 if !ok||ordinal>math.MaxInt64{return nil,errors.New("multi-system ordinal invalid")}
 end,ok:=journal[22].(uint64)
 if !ok||end<uint64(n){return nil,errors.New("multi-system event sequence underflow")}
 prior:=clock.State{Instant:domain.SimulationInstant(now),Revision:domain.WorldRevision(base),
  NextLogicalSequence:domain.LogicalSequence(end-uint64(n)),LastOrdinal:int64(ordinal)-1}
 next:=prior;next.Revision++;next.LastOrdinal=int64(ordinal);next.NextLogicalSequence=domain.LogicalSequence(end)
 var hash [32]byte;copy(hash[:],h)
 batch:=ports.WorkBatch{Mode:3,RunID:run,CycleID:cycle,
  SystemCandidate:candidates[0],SystemExtraCandidates:candidates[1:],SystemSuccessors:successors,
  Decisions:decisions,Events:events,BaseStateHash:hash,ResultStateHash:hash,
  RevisionCanonical:afterRaw,Journal:ports.CanonicalRecord{Identity:commit}}
 if err:=verifySystemBatch(ctx,q,batch,prior,next,fence,journal,predecessor,after,allowed,
  map[domain.ID]map[string]bool{});err!=nil{return nil,err}
 return sources,nil
}

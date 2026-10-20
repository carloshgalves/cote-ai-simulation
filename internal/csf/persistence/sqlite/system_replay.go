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

// verifyPublishedSystem derives one system-source settlement solely from
// durable journal, genesis pins, previous/successor WorldRevisions, event,
// candidate, DecisionRecord and any newly scheduled successor.
func verifyPublishedSystem(ctx context.Context,q queryOne,run,cycle,commit [32]byte,
 journal,fence []any,eventRaw []any)(domain.ID,error){
 var zero domain.ID
 if len(eventRaw)!=1{return zero,errors.New("system settlement has wrong Event cardinality")}
 units,ok:=fence[11].([]any)
 if !ok||len(units)!=1{return zero,errors.New("system source set malformed")}
 fields,ok:=units[0].([]any)
 if !ok||len(fields)!=5{return zero,errors.New("system source envelope missing")}
 key,ok:=fields[3].([]byte)
 if !ok||len(key)!=32{return zero,errors.New("system source ID invalid")}
 var source domain.ID;copy(source[:],key)
 ids,ok:=journal[16].([]any)
 if !ok||len(ids)!=1{return zero,errors.New("system decision list malformed")}
 did,ok:=ids[0].([]byte)
 if !ok||len(did)!=32{return zero,errors.New("system DecisionRecord ID invalid")}
 var recordBytes []byte
 if err:=q.QueryRowContext(ctx,"SELECT canonical FROM decision_ledger WHERE id=?",did).Scan(&recordBytes);err!=nil{return zero,err}
 recordRaw,err:=codec.StrictDecodePayload(recordBytes);if err!=nil{return zero,err}
 record,ok:=recordRaw.([]any)
 if !ok||len(record)!=16{return zero,errors.New("system DecisionRecord invalid")}
 chosen,ok:=record[6].([]any)
 if !ok||len(chosen)!=2||chosen[0]!=uint64(1){return zero,errors.New("system terminal candidate absent")}
 candidateID,ok:=chosen[1].([]byte)
 if !ok||len(candidateID)!=32{return zero,errors.New("system candidate ID invalid")}
 var candidateBytes []byte
 if err:=q.QueryRowContext(ctx,"SELECT canonical FROM decision_ledger WHERE id=?",candidateID).Scan(&candidateBytes);err!=nil{return zero,err}
 optional,ok:=record[13].([]any)
 if !ok{return zero,errors.New("system successor option malformed")}
 successors:=[]ports.CanonicalRecord{}
 if len(optional)==2&&optional[0]==uint64(1){
  nextID,ok:=optional[1].([]byte)
  if !ok||len(nextID)!=32{return zero,errors.New("system successor ID invalid")}
  var nextRaw []byte
  if err:=q.QueryRowContext(ctx,"SELECT canonical FROM scheduled_occurrences WHERE run_id=? AND id=?",
   run[:],nextID).Scan(&nextRaw);err!=nil{return zero,err}
  var k [32]byte;copy(k[:],nextID)
  successors=append(successors,ports.CanonicalRecord{Identity:k,Canonical:nextRaw})
 }else if len(optional)!=1||optional[0]!=uint64(0){return zero,errors.New("system unsupported successor")}
 base,ok:=journal[6].(uint64)
 if !ok||base==math.MaxUint64{return zero,errors.New("system revision invalid")}
 var beforeRaw,afterRaw []byte
 if err:=q.QueryRowContext(ctx,"SELECT canonical FROM world_revisions WHERE revision=?",base).Scan(&beforeRaw);err!=nil{return zero,err}
 if err:=q.QueryRowContext(ctx,"SELECT canonical FROM world_revisions WHERE revision=?",base+1).Scan(&afterRaw);err!=nil{return zero,err}
 pre,err:=codec.StrictDecodePayload(beforeRaw);if err!=nil{return zero,err}
 post,err:=codec.StrictDecodePayload(afterRaw);if err!=nil{return zero,err}
 predecessor,ok:=pre.([]any)
 if !ok||len(predecessor)!=6{return zero,errors.New("system prior WorldRevision malformed")}
 revision,ok:=post.([]any)
 if !ok||len(revision)!=6{return zero,errors.New("system resulting WorldRevision malformed")}
 beforeHash,ok:=predecessor[5].([]byte)
 if !ok||len(beforeHash)!=32{return zero,errors.New("system base WorldState hash missing")}
 instant,ok:=asSigned64(journal[3])
 if !ok{return zero,errors.New("system logical instant missing")}
 ordinal,ok:=journal[4].(uint64)
 if !ok||ordinal>math.MaxInt64{return zero,errors.New("system ordinal invalid")}
 end,ok:=journal[22].(uint64)
 if !ok||end<1{return zero,errors.New("system sequence invalid")}
 before:=clock.State{Instant:domain.SimulationInstant(instant),Revision:domain.WorldRevision(base),
  NextLogicalSequence:domain.LogicalSequence(end-1),LastOrdinal:int64(ordinal)-1}
 after:=before;after.Revision++;after.LastOrdinal=int64(ordinal);after.NextLogicalSequence++
 var genesisRaw []byte
 if err:=q.QueryRowContext(ctx,"SELECT canonical FROM genesis_records WHERE run_id=?",run[:]).Scan(&genesisRaw);err!=nil{return zero,err}
 decodedGenesis,err:=codec.StrictDecodeEnvelope(genesisRaw);if err!=nil{return zero,err}
 genesis,ok:=decodedGenesis.Value.([]any)
 if !ok||len(genesis)!=18{return zero,errors.New("system genesis malformed")}
 refs,ok:=genesis[14].([]any)
 if !ok{return zero,errors.New("system genesis policies missing")}
 allowed:=map[string]any{}
 for _,v:=range refs{
  fields,ok:=v.([]any)
  if !ok||len(fields)!=3{return zero,errors.New("system policy ref invalid")}
  name,ok:=fields[0].(string)
  if !ok{return zero,errors.New("system policy ID invalid")}
  allowed[name]=fields
 }
 events,ok:=journal[15].([]any)
 if !ok||len(events)!=1{return zero,errors.New("system journal event list invalid")}
 eventID,ok:=events[0].([]byte)
 if !ok||len(eventID)!=32{return zero,errors.New("system lifecycle Event ID invalid")}
 var eventKey,recordKey,candidateKey [32]byte
 copy(eventKey[:],eventID);copy(recordKey[:],did);copy(candidateKey[:],candidateID)
 rawEvent,ok:=eventRaw[0].([]byte)
 if !ok{return zero,errors.New("system lifecycle Event missing bytes")}
 var oldHash [32]byte;copy(oldHash[:],beforeHash)
 batch:=ports.WorkBatch{Mode:3,RunID:run,CycleID:cycle,
  SystemCandidate:ports.CanonicalRecord{Identity:candidateKey,Canonical:candidateBytes},
  SystemSuccessors:successors,
  Decisions:[]ports.CanonicalRecord{{Identity:recordKey,Canonical:recordBytes}},
  Events:[]ports.CanonicalRecord{{Identity:eventKey,Canonical:rawEvent}},
  BaseStateHash:oldHash,RevisionCanonical:afterRaw,
  Journal:ports.CanonicalRecord{Identity:commit},
 }
 if err:=verifySystemTerminal(ctx,q,batch,before,after,fence,journal,predecessor,revision,allowed,
  map[domain.ID]map[string]bool{});err!=nil{return zero,err}
 return source,nil
}

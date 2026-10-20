package sqlite

import (
 "bytes"
 "context"
 "errors"
 "math"
 "sort"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/clock"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
)

// verifyPublishedTwoResource recovers the exact six provisional proofs,
// two terminal decisions, three events and both world revisions from
// durable tables; it uses the same pure verifier as the live SQLite CAS.
func verifyPublishedTwoResource(ctx context.Context,q queryOne,runID,cycleID,commitID [32]byte,
 journal,fence []any,eventRaw []any)([]domain.ID,error){
 if len(eventRaw)!=3{return nil,errors.New("contested terminal must have 3 events")}
 admitted,ok:=fence[11].([]any)
 if !ok||len(admitted)!=2{return nil,errors.New("contested terminal source count malformed")}
 sources:=map[string]bool{}
 candidateIDs:=map[string]bool{}
 rounds:=map[domain.ID]map[string]bool{}
 settled:=make([]domain.ID,0,2)
 for _,v:=range admitted{
  entry,ok:=v.([]any);if !ok||len(entry)!=5{return nil,errors.New("contest source envelope malformed")}
  id,ok:=entry[3].([]byte);if !ok||len(id)!=32{return nil,errors.New("contest source ID malformed")}
  if sources[string(id)]{return nil,errors.New("duplicate contest source")}
  sources[string(id)]=true
  var unitID domain.ID;copy(unitID[:],id);settled=append(settled,unitID)
  candidateID,err:=registeredDigest("cote.csf.id.commit-candidate",
   "cote.csf.schema.candidate-id-preimage",
   []any{runID[:],cycleID[:],[]any{id},"resource.consume",uint64(1),uint64(0),uint64(0)})
  if err!=nil{return nil,err}
  candidateIDs[string(candidateID[:])]=true
  var raw []byte
  if err:=q.QueryRowContext(ctx,"SELECT canonical FROM input_ledger WHERE id=?",id).Scan(&raw);err!=nil{return nil,err}
  value,err:=codec.StrictDecodePayload(raw);if err!=nil{return nil,err}
  proposal,ok:=value.([]any);if !ok||len(proposal)!=19{return nil,errors.New("contest input is not an action proposal")}
  rb,ok:=proposal[13].([]byte);if !ok||len(rb)!=32{return nil,errors.New("contest round ref invalid")}
  slot,ok:=proposal[14].(string);if !ok{return nil,errors.New("contest slot ref invalid")}
  var roundID domain.ID;copy(roundID[:],rb)
  var roundRaw []byte
  if err:=q.QueryRowContext(ctx,"SELECT canonical FROM input_ledger WHERE id=?",rb).Scan(&roundRaw);err!=nil{return nil,err}
  decoded,err:=codec.StrictDecodePayload(roundRaw);if err!=nil{return nil,err}
  round,ok:=decoded.([]any);if !ok||len(round)!=11{return nil,errors.New("contest RoundDeclaration invalid")}
  values,ok:=round[4].([]any);if !ok||len(values)!=1||values[0]!=slot{
   return nil,errors.New("contested round has unaccounted slot")
  }
  if rounds[roundID]!=nil{return nil,errors.New("two contestants share round")}
  rounds[roundID]=map[string]bool{slot:true}
 }
 ids,ok:=journal[16].([]any);if !ok||len(ids)!=2{return nil,errors.New("contested decision references missing")}
 decisions:=make([]ports.CanonicalRecord,len(ids))
 for i,v:=range ids{
  rawID,ok:=v.([]byte);if !ok||len(rawID)!=32{return nil,errors.New("contested DecisionRecord ID invalid")}
  copy(decisions[i].Identity[:],rawID)
  if err:=q.QueryRowContext(ctx,"SELECT canonical FROM decision_ledger WHERE id=?",rawID).Scan(&decisions[i].Canonical);err!=nil{return nil,err}
 }
 eventIDs,ok:=journal[15].([]any);if !ok||len(eventIDs)!=len(eventRaw){return nil,errors.New("contested event references incomplete")}
 events:=make([]ports.CanonicalRecord,len(eventRaw))
 for i,v:=range eventRaw{
  b,ok:=v.([]byte);if !ok{return nil,errors.New("contested event bytes invalid")}
  key,ok:=eventIDs[i].([]byte);if !ok||len(key)!=32{return nil,errors.New("contested Event ID invalid")}
  copy(events[i].Identity[:],key);events[i].Canonical=b
 }
 var baseTyped,afterTyped []byte
 revision,ok:=journal[6].(uint64);if !ok||revision>=math.MaxUint64{return nil,errors.New("contested revision malformed")}
 if err:=q.QueryRowContext(ctx,"SELECT canonical FROM world_revisions WHERE revision=?",revision).Scan(&baseTyped);err!=nil{return nil,err}
 if err:=q.QueryRowContext(ctx,"SELECT canonical FROM world_revisions WHERE revision=?",revision+1).Scan(&afterTyped);err!=nil{return nil,err}
 beforeValue,err:=codec.StrictDecodePayload(baseTyped);if err!=nil{return nil,err}
 afterValue,err:=codec.StrictDecodePayload(afterTyped);if err!=nil{return nil,err}
 before,ok:=beforeValue.([]any);if !ok||len(before)!=6{return nil,errors.New("contested predecessor WorldRevision invalid")}
 after,ok:=afterValue.([]any);if !ok||len(after)!=6{return nil,errors.New("contested successor WorldRevision invalid")}
 beforeHash,ok:=before[5].([]byte);if !ok||len(beforeHash)!=32{return nil,errors.New("contested base state hash invalid")}
 afterHash,ok:=after[5].([]byte);if !ok||len(afterHash)!=32{return nil,errors.New("contested result state hash invalid")}
 var genesisRaw []byte
 if err:=q.QueryRowContext(ctx,"SELECT canonical FROM genesis_records WHERE run_id=?",runID[:]).Scan(&genesisRaw);err!=nil{return nil,err}
 genesisEnvelope,err:=codec.StrictDecodeEnvelope(genesisRaw);if err!=nil{return nil,err}
 genesis,ok:=genesisEnvelope.Value.([]any);if !ok||len(genesis)!=18{return nil,errors.New("contested Genesis invalid")}
 genesisRefs,ok:=genesis[14].([]any);if !ok{return nil,errors.New("contested Genesis policies missing")}
 allowed:=map[string]any{}
 for _,v:=range genesisRefs{
  ref,ok:=v.([]any);if !ok||len(ref)!=3{return nil,errors.New("contested policy ref invalid")}
  name,ok:=ref[0].(string);if !ok{return nil,errors.New("contested policy ID invalid")}
  allowed[name]=ref
 }
 // Provisional material belongs to this cycle, never to a caller's supplied
 // proof list. Assessment roots are content addressed, so identify them by
 // their exact authenticated slot-response subject and proof arity.
 rows,err:=q.QueryContext(ctx,"SELECT id,canonical FROM decision_ledger ORDER BY id")
 if err!=nil{return nil,err}
 proofs:=make([]ports.CanonicalRecord,0,6)
 for rows.Next(){
  var id,raw []byte
  if err:=rows.Scan(&id,&raw);err!=nil{rows.Close();return nil,err}
  if len(id)!=32{rows.Close();return nil,errors.New("malformed ledger root")}
  v,err:=codec.StrictDecodePayload(raw);if err!=nil{rows.Close();return nil,err}
  fields,ok:=v.([]any);if !ok{continue}
  include:=false
  switch len(fields){
  case 17:
   include=candidateIDs[string(id)]&&equalBytes(fields[1],runID[:])&&equalBytes(fields[2],cycleID[:])
  case 3:
   subj,ok:=fields[0].([]any)
   include=ok&&len(subj)==3&&subj[0]==uint64(5)&&func()bool{v,ok:=subj[1].([]byte);return ok&&sources[string(v)]}()
  case 7:
   participants,ok:=fields[1].([]any)
   include=ok&&len(participants)==2
   if include{
    for _,p:=range participants{
     b,ok:=p.([]byte)
     if !ok||len(b)!=32||!candidateIDs[string(b)]{include=false;break}
    }
   }
  case 11:
   include=equalBytes(fields[1],runID[:])&&equalBytes(fields[2],cycleID[:])
  }
  if include{
   var key [32]byte;copy(key[:],id)
   proofs=append(proofs,ports.CanonicalRecord{Identity:key,Canonical:raw})
  }
 }
 if err:=rows.Err();err!=nil{rows.Close();return nil,err}
 if err:=rows.Close();err!=nil{return nil,err}
 sort.Slice(proofs,func(i,j int)bool{return bytes.Compare(proofs[i].Identity[:],proofs[j].Identity[:])<0})
 instant,ok:=asSigned64(journal[3]);if !ok{return nil,errors.New("contest instant malformed")}
 ordinal,ok:=journal[4].(uint64);if !ok||ordinal>math.MaxInt64{return nil,errors.New("contest ordinal malformed")}
 end,ok:=journal[22].(uint64);if !ok||end<uint64(len(events)){return nil,errors.New("contest sequence invalid")}
 prior:=clock.State{Instant:domain.SimulationInstant(instant),Revision:domain.WorldRevision(revision),
  NextLogicalSequence:domain.LogicalSequence(end-uint64(len(events))),LastOrdinal:int64(ordinal)-1}
 next:=prior;next.Revision++;next.NextLogicalSequence=domain.LogicalSequence(end);next.LastOrdinal=int64(ordinal)
 var sourceHash,resultHash [32]byte
 copy(sourceHash[:],beforeHash);copy(resultHash[:],afterHash)
 batch:=ports.WorkBatch{
  Mode:2,RunID:runID,CycleID:cycleID,ContestProofs:proofs,Events:events,Decisions:decisions,
  BaseStateHash:sourceHash,ResultStateHash:resultHash,RevisionCanonical:afterTyped,
  Journal:ports.CanonicalRecord{Identity:commitID},
 }
 if err:=verifyTwoResourceTerminal(ctx,q,batch,prior,next,fence,journal,before,after,genesis,allowed,rounds);err!=nil{
  return nil,err
 }
 return settled,nil
}

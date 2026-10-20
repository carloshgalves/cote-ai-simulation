package sqlite

import (
 "bytes"
 "context"
 "database/sql"
 "errors"
 "sort"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/settlement"
)

// SettledNoProposalUnits projects only authoritative CycleCommit and
// DecisionRecord roots. A provisional disposition, a naked decision, or a
// dangling committed_cycles index cannot consume a source. This projection is
// read-only and participates in later Clock.Next and AdmissionView selection.
func (s *Store) SettledNoProposalUnits(ctx context.Context,runID [32]byte)(map[domain.ID]bool,error){
 tx,err:=s.db.BeginTx(ctx,&sql.TxOptions{ReadOnly:true})
 if err!=nil{return nil,err}
 defer tx.Rollback()
 result,err:=settledNoProposalUnits(ctx,tx,runID)
 if err!=nil{return nil,err}
 if err:=tx.Commit();err!=nil{return nil,err}
 return result,nil
}
func settledNoProposalUnits(ctx context.Context,q queryOne,runID [32]byte)(map[domain.ID]bool,error){
 result:=map[domain.ID]bool{}
 rows,err:=q.QueryContext(ctx,`SELECT cycle_id,commit_id FROM committed_cycles WHERE run_id=? ORDER BY cycle_id`,runID[:])
 if err!=nil{return nil,err}
 type root struct{cycle,commit [32]byte}
 var roots []root
 for rows.Next(){
  var cid,id []byte
  if err:=rows.Scan(&cid,&id);err!=nil{rows.Close();return nil,err}
  if len(cid)!=32||len(id)!=32{rows.Close();return nil,errors.New("corrupt committed cycle index")}
  var item root;copy(item.cycle[:],cid);copy(item.commit[:],id)
  roots=append(roots,item)
 }
 if err:=rows.Err();err!=nil{rows.Close();return nil,err}
 if err:=rows.Close();err!=nil{return nil,err}
 for _,root:=range roots{
  var raw []byte
  if err:=q.QueryRowContext(ctx,"SELECT canonical FROM decision_ledger WHERE id=?",root.commit[:]).Scan(&raw);err!=nil{return nil,err}
  v,err:=codec.StrictDecodePayload(raw);if err!=nil{return nil,err}
  journal,ok:=v.([]any)
  if !ok||len(journal)!=24||!equalBytes(journal[0],runID[:])||
   !equalBytes(journal[1],root.cycle[:]){return nil,errors.New("committed cycle root or journal corrupt")}
  plan,ok:=journal[5].([]any)
  if !ok||len(plan)==0{return nil,errors.New("invalid committed CyclePlan")}
  if plan[0]==uint64(1){continue} // CLOCK_ADVANCE has no settled decisions
  if len(plan)!=1||plan[0]!=uint64(0){return nil,errors.New("unknown committed WORK plan")}
  attempt,ok:=journal[2].(uint64)
  if !ok{return nil,errors.New("malformed WORK attempt")}
  expected,err:=registeredDigest("cote.csf.id.cycle-commit","cote.csf.schema.cycle-commit-id-preimage",
   []any{runID[:],root.cycle[:],attempt});if err!=nil{return nil,err}
  eventRaw,err:=loadJournalEvents(ctx,q,journal)
  if err!=nil{return nil,err}
  digest,err:=registeredDigest("cote.csf.digest.batch","cote.csf.schema.batch-preimage",
   []any{journal[:23],eventRaw});if err!=nil{return nil,err}
  if expected!=root.commit||!equalBytes(journal[23],digest[:]){
   return nil,errors.New("WORK terminal journal identity/digest invalid")
  }
  eventIDs,ok:=journal[15].([]any)
  if !ok||len(eventIDs)==0{return nil,errors.New("WORK omitted required lifecycle events")}
  unitIDs,ok:=journal[11].([]any)
  if !ok{return nil,errors.New("WORK source membership absent")}
  decisions,ok:=journal[16].([]any)
  if !ok||len(unitIDs)!=len(decisions){return nil,errors.New("WORK decision bijection changed")}
  fenceID,err:=registeredDigest("cote.csf.id.admission-fence","cote.csf.schema.admission-fence-id-preimage",
   []any{runID[:],root.cycle[:],attempt});if err!=nil{return nil,err}
  var fenceRaw []byte
  if err:=q.QueryRowContext(ctx,"SELECT canonical FROM decision_ledger WHERE id=?",fenceID[:]).Scan(&fenceRaw);err!=nil{return nil,err}
  f,err:=codec.StrictDecodePayload(fenceRaw);if err!=nil{return nil,err}
  fence,ok:=f.([]any)
  if !ok||len(fence)!=18{return nil,errors.New("committed WORK fence invalid")}
  fd,err:=registeredDigest("cote.csf.digest.fence","cote.csf.schema.admission-fence.body",fence[:17])
  if err!=nil{return nil,err}
  if !equalBytes(journal[10],fd[:])||!equalBytes(fence[17],fd[:])||
   !equalBytes(fence[0],runID[:])||!equalBytes(fence[1],root.cycle[:])||
   !sameCanonical(journal[11],noProposalIDs(fence)){
   return nil,errors.New("committed WORK fence source identity differs")
  }
  units,ok:=fence[11].([]any);if !ok||len(units)!=len(unitIDs){return nil,errors.New("invalid fenced units")}
  pairs:=make([]any,0,len(units))
  type slotProof struct{slot string;id,digest []byte}
  eventRefs:=map[domain.ID][]slotProof{}
  for i,rawUnit:=range units{
   parts,ok:=rawUnit.([]any);if !ok||len(parts)!=5||parts[1]!=uint64(0){
    return nil,errors.New("non-slot unit cannot be settled by NoProposal")
   }
   idBytes,ok:=parts[3].([]byte);if !ok||len(idBytes)!=32{return nil,errors.New("settled input ID invalid")}
   digestBytes,ok:=parts[4].([]byte);if !ok||len(digestBytes)!=32{return nil,errors.New("settled unit digest invalid")}
   decisionBytes,ok:=decisions[i].([]byte);if !ok||len(decisionBytes)!=32{return nil,errors.New("settled decision ID invalid")}
   sourceID,ok:=unitIDs[i].([]byte)
   if !ok||!bytes.Equal(sourceID,idBytes){return nil,errors.New("settled source order differs")}
   var unitRaw []byte
   if err:=q.QueryRowContext(ctx,"SELECT canonical FROM input_ledger WHERE id=?",idBytes).Scan(&unitRaw);err!=nil{return nil,err}
   parsed,err:=codec.StrictDecodePayload(unitRaw);if err!=nil{return nil,err}
   unitBody,ok:=parsed.([]any);if !ok||len(unitBody)!=11{return nil,errors.New("committed NoProposal source body malformed")}
   roundBytes,ok:=unitBody[4].([]byte);if !ok||len(roundBytes)!=32{return nil,errors.New("committed source round missing")}
   slot,ok:=unitBody[5].(string);if !ok{return nil,errors.New("committed source slot missing")}
   var roundID domain.ID;copy(roundID[:],roundBytes)
   eventRefs[roundID]=append(eventRefs[roundID],slotProof{slot:slot,id:idBytes,digest:digestBytes})
   var record []byte
   if err:=q.QueryRowContext(ctx,"SELECT canonical FROM decision_ledger WHERE id=?",decisionBytes).Scan(&record);err!=nil{return nil,err}
   rawDecision,err:=codec.StrictDecodePayload(record);if err!=nil{return nil,err}
   fields,ok:=rawDecision.([]any)
   if !ok||len(fields)!=16{return nil,errors.New("terminal decision root invalid")}
   did,err:=registeredDigest("cote.csf.id.decision","cote.csf.schema.decision-id-preimage",
    []any{root.cycle[:],attempt,idBytes});if err!=nil{return nil,err}
   dh,err:=registeredDigest("cote.csf.digest.decision-record","cote.csf.schema.decision-record.body",fields[:15])
   if err!=nil{return nil,err}
   if !bytes.Equal(did[:],decisionBytes)||!equalBytes(fields[0],decisionBytes)||
    !equalBytes(fields[15],dh[:])||fields[5]!=uint64(3)||
    !sameCanonical(fields[4],[]any{uint64(0),uint64(1),idBytes,digestBytes}){
    return nil,errors.New("settlement DecisionRecord unauthenticated")
   }
   var id domain.ID;copy(id[:],idBytes)
   if result[id]{return nil,errors.New("source unit settled twice")}
   result[id]=true
   pairs=append(pairs,[]any{idBytes,decisionBytes,dh[:]})
  }
  if len(eventRefs)!=len(eventIDs){return nil,errors.New("WORK lifecycle/round count differs")}
  roundIDs:=make([]domain.ID,0,len(eventRefs))
  for id:=range eventRefs{roundIDs=append(roundIDs,id)}
  sort.Slice(roundIDs,func(i,j int)bool{return bytes.Compare(roundIDs[i][:],roundIDs[j][:])<0})
  at,ok:=asSigned64(journal[3]);if !ok{return nil,errors.New("WORK instant malformed")}
  base,ok:=journal[6].(uint64);if !ok{return nil,errors.New("WORK base revision missing")}
  end,ok:=journal[22].(uint64);if !ok||end<uint64(len(eventIDs)){return nil,errors.New("WORK logical sequence underflow")}
  for i,roundID:=range roundIDs{
   items:=eventRefs[roundID]
   sort.Slice(items,func(i,j int)bool{return items[i].slot<items[j].slot})
   refs:=make([]any,len(items))
   for j,v:=range items{refs[j]=[]any{uint64(0),uint64(1),v.id,v.digest}}
   want,err:=settlement.ConsumedRoundEvent(domain.ID(runID),domain.ID(root.cycle),roundID,
    domain.SimulationInstant(at),domain.WorldRevision(base),domain.LogicalSequence(end-uint64(len(eventIDs))+uint64(i)),refs)
   if err!=nil{return nil,err}
   if !sameCanonical(eventIDs[i],want.Identity[:])||!bytes.Equal(eventRaw[i].([]byte),want.Canonical){
    return nil,errors.New("WORK lifecycle Event differs from canonical terminal round")
   }
  }
  want,err:=registeredDigest("cote.csf.digest.decision","cote.csf.schema.decision-pair-list",pairs)
  if err!=nil{return nil,err}
  if !equalBytes(journal[17],want[:]){return nil,errors.New("settlement digest differs from DecisionRecord list")}
 }
 return result,nil
}

func loadJournalEvents(ctx context.Context,q queryOne,journal []any)([]any,error){
 if len(journal)!=24{return nil,errors.New("CycleCommit body malformed")}
 list,ok:=journal[15].([]any);if !ok{return nil,errors.New("CycleCommit event IDs malformed")}
 out:=make([]any,0,len(list))
 for _,v:=range list{
  id,ok:=v.([]byte);if !ok||len(id)!=32{return nil,errors.New("CycleCommit event ID not a digest")}
  var raw []byte
  if err:=q.QueryRowContext(ctx,"SELECT canonical FROM event_store WHERE id=?",id).Scan(&raw);err!=nil{return nil,err}
  fields,err:=codec.StrictDecodePayload(raw);if err!=nil{return nil,err}
  f,ok:=fields.([]any)
  if !ok||len(f)!=19||!equalBytes(f[0],id){return nil,errors.New("referenced event body invalid")}
  dg,err:=registeredDigest("cote.csf.digest.event","cote.csf.schema.event.body",f[:18])
  if err!=nil{return nil,err}
  if !equalBytes(f[18],dg[:]){return nil,errors.New("referenced event digest invalid")}
  out=append(out,raw)
 }
 return out,nil
}

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

// verifyPublishedSingleResource reconstitutes a terminal WorkBatch entirely
// from the five durable causal tables. No caller-supplied provisional result,
// worker callback or mutable hidden in-memory proof is accepted on replay.
func verifyPublishedSingleResource(ctx context.Context,q queryOne,runID,cycleID,commitID [32]byte,
 journal,fence []any,events []any)(domain.ID,error){
 var zero domain.ID
 if len(events)!=2{return zero,errors.New("resource terminal event cardinality invalid")}
 sourceEntries,ok:=fence[11].([]any)
 if !ok||len(sourceEntries)!=1{return zero,errors.New("resource terminal source cardinality invalid")}
 source,ok:=sourceEntries[0].([]any)
 if !ok||len(source)!=5{return zero,errors.New("resource terminal source ref malformed")}
 sourceRaw,ok:=source[3].([]byte)
 if !ok||len(sourceRaw)!=32{return zero,errors.New("resource source identity missing")}
 var id domain.ID;copy(id[:],sourceRaw)
 list,ok:=journal[16].([]any)
 if !ok||len(list)!=1{return zero,errors.New("resource DecisionRecord list not singleton")}
 did,ok:=list[0].([]byte)
 if !ok||len(did)!=32{return zero,errors.New("resource DecisionRecord ID malformed")}
 var decisionBytes []byte
 if err:=q.QueryRowContext(ctx,"SELECT canonical FROM decision_ledger WHERE id=?",did).Scan(&decisionBytes);err!=nil{return zero,err}
 rawDecision,err:=codec.StrictDecodePayload(decisionBytes);if err!=nil{return zero,err}
 decision,ok:=rawDecision.([]any)
 if !ok||len(decision)!=16{return zero,errors.New("resource DecisionRecord malformed")}
 choice,ok:=decision[6].([]any)
 if !ok||len(choice)!=2||choice[0]!=uint64(1){return zero,errors.New("committed resource decision has no candidate")}
 candidateID,ok:=choice[1].([]byte)
 if !ok||len(candidateID)!=32{return zero,errors.New("resource candidate ID malformed")}
 var candidateBytes []byte
 if err:=q.QueryRowContext(ctx,"SELECT canonical FROM decision_ledger WHERE id=?",candidateID).Scan(&candidateBytes);err!=nil{return zero,err}
 encodedCandidate,err:=codec.StrictDecodePayload(candidateBytes);if err!=nil{return zero,err}
 candidateFields,ok:=encodedCandidate.([]any)
 if !ok||len(candidateFields)!=17{return zero,errors.New("resource candidate missing")}
 var sourceBytes []byte
 if err:=q.QueryRowContext(ctx,"SELECT canonical FROM input_ledger WHERE id=?",sourceRaw).Scan(&sourceBytes);err!=nil{return zero,err}
 rawSource,err:=codec.StrictDecodePayload(sourceBytes);if err!=nil{return zero,err}
 proposal,ok:=rawSource.([]any)
 if !ok||len(proposal)!=19{return zero,errors.New("source is not an ActionProposal")}
 roundBytes,ok:=proposal[13].([]byte)
 if !ok||len(roundBytes)!=32{return zero,errors.New("resource source round reference missing")}
 var roundID domain.ID;copy(roundID[:],roundBytes)
 var roundCanonical []byte
 if err:=q.QueryRowContext(ctx,"SELECT canonical FROM input_ledger WHERE id=?",roundBytes).Scan(&roundCanonical);err!=nil{return zero,err}
 decodedRound,err:=codec.StrictDecodePayload(roundCanonical);if err!=nil{return zero,err}
 roundFields,ok:=decodedRound.([]any)
 if !ok||len(roundFields)!=11{return zero,errors.New("resource RoundDeclaration invalid")}
 slots,ok:=roundFields[4].([]any)
 if !ok{return zero,errors.New("resource round slots invalid")}
 slotSet:=map[string]bool{}
 for _,v:=range slots{
  name,ok:=v.(string);if !ok||slotSet[name]{return zero,errors.New("replayed source round slots malformed")}
  slotSet[name]=true
 }
 var baseRaw,resultRaw []byte
 baseRev,ok:=journal[6].(uint64);if !ok||baseRev>=math.MaxUint64{return zero,errors.New("resource base revision invalid")}
 if err:=q.QueryRowContext(ctx,"SELECT canonical FROM world_revisions WHERE revision=?",baseRev).Scan(&baseRaw);err!=nil{return zero,err}
 if err:=q.QueryRowContext(ctx,"SELECT canonical FROM world_revisions WHERE revision=?",baseRev+1).Scan(&resultRaw);err!=nil{return zero,err}
 baseDecoded,err:=codec.StrictDecodePayload(baseRaw);if err!=nil{return zero,err}
 resultDecoded,err:=codec.StrictDecodePayload(resultRaw);if err!=nil{return zero,err}
 before,ok:=baseDecoded.([]any);if !ok||len(before)!=6{return zero,errors.New("resource predecessor WorldRevision malformed")}
 after,ok:=resultDecoded.([]any);if !ok||len(after)!=6{return zero,errors.New("resource successor WorldRevision malformed")}
 stateHash,ok:=before[5].([]byte);if !ok||len(stateHash)!=32{return zero,errors.New("resource predecessor hash malformed")}
 resultHash,ok:=after[5].([]byte);if !ok||len(resultHash)!=32{return zero,errors.New("resource successor hash malformed")}
 instant,ok:=asSigned64(journal[3]);if !ok{return zero,errors.New("resource instant malformed")}
 ordinal,ok:=journal[4].(uint64);if !ok||ordinal>math.MaxInt64{return zero,errors.New("resource cycle ordinal malformed")}
 endSeq,ok:=journal[22].(uint64)
 if !ok||endSeq<uint64(len(events)){return zero,errors.New("resource logical sequence missing")}
 state:=clock.State{Instant:domain.SimulationInstant(instant),Revision:domain.WorldRevision(baseRev),
  NextLogicalSequence:domain.LogicalSequence(endSeq-uint64(len(events))),LastOrdinal:int64(ordinal)-1}
 next:=state;next.Revision++;next.NextLogicalSequence=domain.LogicalSequence(endSeq);next.LastOrdinal=int64(ordinal)
 var runGenesis []byte
 if err:=q.QueryRowContext(ctx,"SELECT canonical FROM genesis_records WHERE run_id=?",runID[:]).Scan(&runGenesis);err!=nil{return zero,err}
 decodedGenesis,err:=codec.StrictDecodeEnvelope(runGenesis);if err!=nil{return zero,err}
 manifest,ok:=decodedGenesis.Value.([]any)
 if !ok||len(manifest)!=18{return zero,errors.New("resource Genesis manifest malformed")}
 policyRefs,ok:=manifest[14].([]any);if !ok{return zero,errors.New("resource Genesis policies missing")}
 allowed:=map[string]any{}
 for _,value:=range policyRefs{
  ref,ok:=value.([]any);if !ok||len(ref)!=3{return zero,errors.New("resource Genesis policy ref malformed")}
  name,ok:=ref[0].(string);if !ok{return zero,errors.New("resource policy id missing")}
  allowed[name]=ref
 }
 var digestID,choiceID [32]byte;copy(digestID[:],did);copy(choiceID[:],candidateID)
 canonicalEvents:=make([]ports.CanonicalRecord,len(events))
 eventIDs,ok:=journal[15].([]any);if !ok||len(eventIDs)!=len(events){return zero,errors.New("resource event id list incomplete")}
 for i,raw:=range events{
  encoded,ok:=raw.([]byte);if !ok{return zero,errors.New("resource canonical event missing")}
  eid,ok:=eventIDs[i].([]byte);if !ok||len(eid)!=32{return zero,errors.New("resource Event ID malformed")}
  copy(canonicalEvents[i].Identity[:],eid);canonicalEvents[i].Canonical=encoded
 }
 batch:=ports.WorkBatch{Mode:1,RunID:runID,CycleID:cycleID,
  ResourceCandidate:ports.CanonicalRecord{Identity:choiceID,Canonical:candidateBytes},
  Decisions:[]ports.CanonicalRecord{{Identity:digestID,Canonical:decisionBytes}},
  Events:canonicalEvents,BaseStateHash:[32]byte{},ResultStateHash:[32]byte{},
 }
 copy(batch.BaseStateHash[:],stateHash);copy(batch.ResultStateHash[:],resultHash)
 batch.Journal=ports.CanonicalRecord{Identity:commitID}
 batch.RevisionCanonical=resultRaw
 expected:=map[domain.ID]map[string]bool{roundID:slotSet}
 if err:=verifySingleResourceTerminal(ctx,q,batch,state,next,fence,journal,before,after,allowed,expected);err!=nil{return zero,err}
 return id,nil
}

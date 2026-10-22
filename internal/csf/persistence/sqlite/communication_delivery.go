package sqlite

import (
 "bytes"
 "context"
 "errors"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/perception"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
)

// loadVerifiedCommunicationProjection derives B's complete evidence only
// from the durable task/Genesis/Claim/Transmission/due CycleCommit. The SQL
// source authority is independently checked from the sent-to-due journal.
// No caller or worker can supply the purported author, claim, or recipient.
func loadVerifiedCommunicationProjection(ctx context.Context,q queryOne,taskID [32]byte)(
 perception.RoundProjection,error){
 var zero perception.RoundProjection
 var taskRaw []byte
 if err:=q.QueryRowContext(ctx,"SELECT canonical FROM causal_outbox WHERE id=?",taskID[:]).Scan(&taskRaw);err!=nil{return zero,err}
 v,err:=codec.StrictDecodePayload(taskRaw);if err!=nil{return zero,err}
 task,ok:=v.([]any)
 if !ok||len(task)!=10||!equalBytes(task[0],taskID[:])||task[9]!=uint64(0){
  return zero,errors.New("communication task invalid/not pending")
 }
 run,rok:=idField(task[1]);cycle,cok:=idField(task[2]);eventID,eok:=idField(task[3])
 base,bok:=task[4].(uint64);revision,vok:=task[5].(uint64)
 if !rok||!cok||!eok||!bok||!vok{return zero,errors.New("communication task causal coordinate invalid")}
 if err:=verifyTaskStateTransition(ctx,q,base,revision);err!=nil{return zero,err}
 want,err:=registeredDigest("cote.csf.id.perception-task","cote.csf.schema.perception-task-id-preimage",
  []any{run[:],eventID[:],task[6],task[7],task[8]})
 if err!=nil||want!=taskID{return zero,errors.New("communication task identity not pinned")}
 var genesisRaw []byte
 if err:=q.QueryRowContext(ctx,"SELECT canonical FROM genesis_records WHERE run_id=?",run[:]).Scan(&genesisRaw);err!=nil{return zero,err}
 envelope,err:=codec.StrictDecodeEnvelope(genesisRaw);if err!=nil{return zero,err}
 g,ok:=envelope.Value.([]any);if !ok||len(g)!=18||!equalBytes(g[0],run[:]){return zero,errors.New("communication Genesis invalid")}
 policyRefs,ok:=g[14].([]any);if !ok{return zero,errors.New("communication Genesis policy roster absent")}
 for _,idx:=range []int{6,7,8}{
  matched:=false
  for _,v:=range policyRefs{if sameCanonical(v,task[idx]){matched=true;break}}
  if !matched{return zero,errors.New("communication task policy not pinned by Genesis")}
 }
 if !sameCanonical(task[6],func()any{for _,v:=range policyRefs{ref,_:=v.([]any);if len(ref)==3&&ref[0]=="perception-v1"{return ref}};return nil}())||
  !sameCanonical(task[8],func()any{for _,v:=range policyRefs{ref,_:=v.([]any);if len(ref)==3&&ref[0]=="perception-identity-v1"{return ref}};return nil}()){
  return zero,errors.New("communication classification/identity policy incorrect")
 }
 var eventRaw,journalRaw []byte
 if err:=q.QueryRowContext(ctx,"SELECT canonical FROM event_store WHERE id=?",eventID[:]).Scan(&eventRaw);err!=nil{return zero,err}
 if err:=q.QueryRowContext(ctx,
  "SELECT j.canonical FROM committed_cycles c JOIN decision_ledger j ON j.id=c.commit_id WHERE c.run_id=? AND c.cycle_id=?",
  run[:],cycle[:]).Scan(&journalRaw);err!=nil{return zero,err}
 parsed,err:=codec.StrictDecodePayload(journalRaw);if err!=nil{return zero,err}
 journal,ok:=parsed.([]any)
 if !ok||len(journal)!=24||!equalBytes(journal[0],run[:])||!equalBytes(journal[1],cycle[:])||
  journal[6]!=base||journal[8]!=revision||!containsRoot(journal[18],taskID)||
  !containsRoot(journal[15],eventID){return zero,errors.New("delivered task/Event absent from due CycleCommit")}
 var commitID [32]byte
 sourceCommit,err:=registeredDigest("cote.csf.id.cycle-commit","cote.csf.schema.cycle-commit-id-preimage",
  []any{run[:],cycle[:],journal[2]})
 if err!=nil{return zero,err}
 copy(commitID[:],sourceCommit[:])
 fid,err:=registeredDigest("cote.csf.id.admission-fence","cote.csf.schema.admission-fence-id-preimage",
  []any{run[:],cycle[:],journal[2]})
 if err!=nil{return zero,err}
 var fenceRaw []byte
 if err:=q.QueryRowContext(ctx,"SELECT canonical FROM decision_ledger WHERE id=?",fid[:]).Scan(&fenceRaw);err!=nil{return zero,err}
 rawFence,err:=codec.StrictDecodePayload(fenceRaw);if err!=nil{return zero,err}
 fence,ok:=rawFence.([]any);if !ok||len(fence)!=18{return zero,errors.New("communication due admission fence corrupt")}
 if _,err:=verifyPublishedCommunication(ctx,q,run,cycle,commitID,journal,fence,[]any{eventRaw});err!=nil{return zero,err}
 parsed,err=codec.StrictDecodePayload(eventRaw);if err!=nil{return zero,err}
 event,ok:=parsed.([]any);if !ok||len(event)!=19||event[3]!="communication.delivered"{
  return zero,errors.New("only a successful delivery may address a claim")
 }
 key,keyOK:=event[2].([]any)
 if !keyOK||len(key)!=6||key[0]!=uint64(1)||key[2]!="communication.delivery"||
  key[3]!=uint64(1)||key[4]!=uint64(0)||key[5]!=uint64(0)||
  !sameCanonical(event[5],journal[3])||!sameCanonical(event[8],task[4]){
  return zero,errors.New("delivery Event coordinate differs from committed occurrence")
 }
 actors,ok:=event[12].([]any);if !ok||len(actors)!=1{return zero,errors.New("communication observer roster must contain B only")}
 actor,ok:=actors[0].([]any)
 if !ok||len(actor)!=2{return zero,errors.New("communication actor malformed")}
 ns,ok:=actor[0].(string);if !ok{return zero,errors.New("communication observer namespace missing")}
 name,ok:=actor[1].(string);if !ok{return zero,errors.New("communication observer identity missing")}
 sourceRefs,ok:=event[11].([]any)
 if !ok||len(sourceRefs)!=1{return zero,errors.New("due Event lacks occurrence input")}
 source,ok:=sourceRefs[0].([]any)
 if !ok||len(source)!=3||source[0]!=uint64(2){return zero,errors.New("due source ref invalid")}
 occID,ok:=idField(source[1]);if !ok{return zero,errors.New("due occurrence reference invalid")}
 var txRaw,claimRaw,claimID []byte
 if err:=q.QueryRowContext(ctx,
  "SELECT canonical,claim_id FROM communication_transmissions WHERE run_id=? AND occurrence_id=? AND status=1",
  run[:],occID[:]).Scan(&txRaw,&claimID);err!=nil{return zero,err}
 tv,err:=codec.StrictDecodePayload(txRaw);if err!=nil{return zero,err}
 transmission,ok:=tv.([]any)
 if !ok||len(transmission)!=14||!sameCanonical(transmission[7],[]any{actor}){
  return zero,errors.New("delivered transmission not addressed to B")
 }
 if err:=q.QueryRowContext(ctx,"SELECT canonical FROM communication_claims WHERE run_id=? AND id=?",
  run[:],claimID).Scan(&claimRaw);err!=nil{return zero,err}
 cv,err:=codec.StrictDecodePayload(claimRaw);if err!=nil{return zero,err}
 claim,ok:=cv.([]any);if !ok||len(claim)!=10{return zero,errors.New("claim missing")}
 claimDigest,err:=registeredDigest("cote.csf.digest.claim","cote.csf.schema.claim.body",claim[:9])
 if err!=nil||!sameCanonical(claim[9],claimDigest[:]){return zero,errors.New("communicated Claim not verifiable")}
 eventDigest,err:=registeredDigest("cote.csf.digest.event","cote.csf.schema.event.body",event[:18])
 if err!=nil||!sameCanonical(event[18],eventDigest[:]){return zero,errors.New("communication Event digest invalid")}
 resolver,ok:=task[7].([]any)
 if !ok||len(resolver)!=3||resolver[0]!="validators-v1"||resolver[1]!=uint64(1){
  return zero,errors.New("communication perceptual resolver not registered")
 }
 evRef:=[]any{eventID[:],eventDigest[:]}
 evidence:=[]any{[]any{uint64(0),eventID[:],eventDigest[:]}}
 claims:=[]any{[]any{uint64(18),claimID,claimDigest[:]}}
 oid,err:=registeredDigest("cote.csf.id.observation","cote.csf.schema.observation-id-preimage",
  []any{taskID[:],actor,uint64(0),uint64(0)})
 if err!=nil{return zero,err}
 observationBody:=[]any{oid[:],taskID[:],actor,uint64(0),uint64(0),
  event[5],[]any{evRef},uint64(5),[]any{uint64(0)},[]any{event[9]},
  claims,[]any{"actual-sender-redacted","claim-unverified"},evidence,resolver}
 obsDigest,err:=registeredDigest("cote.csf.digest.observation","cote.csf.schema.observation.body",observationBody)
 if err!=nil{return zero,err}
 observationRaw,err:=codec.CanonicalPayload(append(observationBody,obsDigest[:]))
 if err!=nil{return zero,err}
 kid,err:=registeredDigest("cote.csf.id.knowledge-input","cote.csf.schema.knowledge-input-id-preimage",
  []any{oid[:],actor,uint64(1),uint64(0),uint64(0)})
 if err!=nil{return zero,err}
 inputBody:=[]any{kid[:],uint64(0),uint64(0),actor,event[5],uint64(1),
  transmission[8],[]any{uint64(16),oid[:],obsDigest[:]},claims,evidence}
 kd,err:=registeredDigest("cote.csf.digest.knowledge-input","cote.csf.schema.knowledge-input.body",inputBody)
 if err!=nil{return zero,err}
 inputRaw,err:=codec.CanonicalPayload(append(inputBody,kd[:]))
 if err!=nil{return zero,err}
 completionID,err:=registeredDigest("cote.csf.id.perception-completion",
  "cote.csf.schema.perception-completion-id-preimage",[]any{taskID[:]})
 if err!=nil{return zero,err}
 completionBody:=[]any{completionID[:],taskID[:],[]any{oid[:]},[]any{kid[:]}}
 completionDigest,err:=registeredDigest("cote.csf.digest.perception-completion",
  "cote.csf.schema.perception-task-completion.body",completionBody)
 if err!=nil{return zero,err}
 completionRaw,err:=codec.CanonicalPayload(append(completionBody,completionDigest[:]))
 if err!=nil{return zero,err}
 return perception.RoundProjection{Actor:[2]string{ns,name},
  Observation:ports.CanonicalRecord{Identity:[32]byte(oid),Canonical:observationRaw},
  Input:ports.CanonicalRecord{Identity:[32]byte(kid),Canonical:inputRaw},
  Completion:ports.CanonicalRecord{Identity:[32]byte(completionID),Canonical:completionRaw}},nil
}

// DeliverCommunicationPerception commits the addressed Observation,
// KnowledgeInput and completion in the same SQLite CAS. Its proof is
// recomputed from immutable records; a competing worker cannot choose B.
func (s *Store) DeliverCommunicationPerception(ctx context.Context,taskID [32]byte)(
 perception.RoundProjection,error){
 return s.deliverCommunicationPerception(ctx,taskID,0)
}
func (s *Store) deliverCommunicationPerception(ctx context.Context,taskID [32]byte,failAt uint8)(
 perception.RoundProjection,error){
 var zero perception.RoundProjection
 if s==nil{return zero,errors.New("communication writer requires store")}
 tx,err:=s.db.BeginTx(ctx,nil);if err!=nil{return zero,err}
 defer tx.Rollback()
 p,err:=loadVerifiedCommunicationProjection(ctx,tx,taskID);if err!=nil{return zero,err}
 parsed,err:=codec.StrictDecodePayload(p.Observation.Canonical);if err!=nil{return zero,err}
 fields:=parsed.([]any)
 _=fields
 var run [32]byte
 var raw []byte
 if err:=tx.QueryRowContext(ctx,"SELECT canonical FROM causal_outbox WHERE id=?",taskID[:]).Scan(&raw);err!=nil{return zero,err}
 v,err:=codec.StrictDecodePayload(raw);if err!=nil{return zero,err}
 t:=v.([]any);copy(run[:],t[1].([]byte))
 if err:=insertExactEvidence(ctx,tx,p.Observation);err!=nil{return zero,err}
 if err:=insertExactRoute(ctx,tx,p.Observation,run,taskID,p.Actor,0);err!=nil{return zero,err}
 if failAt==1{return zero,errors.New("injected failure after communication Observation")}
 if err:=insertExactEvidence(ctx,tx,p.Input);err!=nil{return zero,err}
 if err:=insertExactRoute(ctx,tx,p.Input,run,taskID,p.Actor,1);err!=nil{return zero,err}
 if failAt==2{return zero,errors.New("injected failure after communication KnowledgeInput")}
 if err:=checkProjectionEvidence(ctx,tx,run,taskID,p);err!=nil{return zero,err}
 res,err:=tx.ExecContext(ctx,"INSERT OR IGNORE INTO causal_outbox(id,canonical) VALUES(?,?)",
  p.Completion.Identity[:],p.Completion.Canonical)
 if err!=nil{return zero,err}
 affected,err:=res.RowsAffected();if err!=nil{return zero,err}
 if affected==0{
  var old []byte
  if err:=tx.QueryRowContext(ctx,"SELECT canonical FROM causal_outbox WHERE id=?",
   p.Completion.Identity[:]).Scan(&old);err!=nil{return zero,err}
  if !bytes.Equal(old,p.Completion.Canonical){return zero,errors.New("communication completion identity collision")}
 }
 if failAt==3{return zero,errors.New("injected failure after communication completion")}
 if err:=tx.Commit();err!=nil{return zero,err}
 return p,nil
}

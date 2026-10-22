package sqlite

import (
 "bytes"
 "context"
 "errors"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
)

// verifyPublishedCommunication is the shared historical gate used before
// PendingClock/ReplayClock is allowed to treat a source as terminal.
// It cross-checks the immutable communication roots and the persisted
// source/decision/event path without a worker-chosen observer or replay flag.
func verifyPublishedCommunication(ctx context.Context,q queryOne,run,cycle,commit [32]byte,
 journal,fence []any,eventRaw []any)(domain.ID,error){
 var zero domain.ID
 units,ok:=fence[11].([]any);if !ok||len(units)!=1{return zero,errors.New("communication historical source cardinality changed")}
 u,ok:=units[0].([]any);if !ok||len(u)!=5{return zero,errors.New("communication historical unit malformed")}
 kind,ok:=u[1].(uint64);if !ok||kind!=0&&kind!=2{return zero,errors.New("communication historical source kind not registered")}
 id,ok:=idField(u[3]);if !ok{return zero,errors.New("communication source identity invalid")}
 sourceDigest,ok:=idField(u[4]);if !ok{return zero,errors.New("communication source digest malformed")}
 if !sameCanonical(journal[11],[]any{id[:]}){return zero,errors.New("communication historical journal omitted source")}
 events,ok:=journal[15].([]any);if !ok||len(events)!=len(eventRaw){return zero,errors.New("communication journal/event bijection wrong")}
 if (kind==0&&len(events)!=2)||(kind==2&&len(events)!=1){return zero,errors.New("communication Event cardinality invalid")}
 var f []any
 for i,raw:=range eventRaw{
  v,ok:=raw.([]byte);if !ok{return zero,errors.New("historical communication event missing")}
  decoded,err:=codec.StrictDecodePayload(v);if err!=nil{return zero,err}
  e,ok:=decoded.([]any);if !ok||len(e)!=19{return zero,errors.New("historical communication Event malformed")}
  expectedID,err:=registeredDigest("cote.csf.id.event","cote.csf.schema.event-id-preimage",
   []any{run[:],cycle[:],e[2]})
  if err!=nil||!equalBytes(e[0],expectedID[:])||!equalBytes(events[i],expectedID[:])||
   !equalBytes(e[1],run[:])||!equalBytes(e[7],cycle[:]){return zero,errors.New("communication Event causal ID invalid")}
  hash,err:=registeredDigest("cote.csf.digest.event","cote.csf.schema.event.body",e[:18])
  if err!=nil||!equalBytes(e[18],hash[:]){return zero,errors.New("communication Event body hash changed")}
  if i==0{f=e}
 }
 var transmissionID,claimID,occurrenceID,sentCommit []byte
 var transmissionRaw,claimRaw,occurrenceRaw []byte
 var sqlStatus int
 if kind==0{
  if f[3]!="communication.sent"||f[15]!=uint64(2)||!sameCanonical(f[12],[]any{})||
   f[16]!="communication.send"||!sameCanonical(f[11],[]any{[]any{uint64(0),uint64(0),id[:],sourceDigest[:]}}){
   return zero,errors.New("historical send Event exposes a recipient or misses its admitted origin")
  }
  if err:=q.QueryRowContext(ctx,"SELECT id,claim_id,occurrence_id,canonical,status FROM communication_transmissions WHERE run_id=? AND sent_commit_id=?",
   run[:],commit[:]).Scan(&transmissionID,&claimID,&occurrenceID,&transmissionRaw,&sqlStatus);err!=nil{return zero,err}
 }else{
  if f[3]!="communication.delivered"&&f[3]!="communication.delivery_failed"{
   return zero,errors.New("unregistered historical communication due Event")
  }
  occurrenceID=id[:]
  if err:=q.QueryRowContext(ctx,"SELECT id,claim_id,sent_commit_id,canonical,status FROM communication_transmissions WHERE run_id=? AND occurrence_id=?",
   run[:],id[:]).Scan(&transmissionID,&claimID,&sentCommit,&transmissionRaw,&sqlStatus);err!=nil{return zero,err}
  if f[3]=="communication.delivered"&&sqlStatus!=1||
    f[3]=="communication.delivery_failed"&&sqlStatus!=2{return zero,errors.New("communication status not terminalized by due Event")}
 }
 if len(transmissionID)!=32||len(claimID)!=32||len(occurrenceID)!=32{return zero,errors.New("communication registry ID malformed")}
 if err:=q.QueryRowContext(ctx,"SELECT canonical FROM communication_claims WHERE run_id=? AND id=?",run[:],claimID).Scan(&claimRaw);err!=nil{return zero,err}
 if err:=q.QueryRowContext(ctx,"SELECT canonical FROM scheduled_occurrences WHERE run_id=? AND id=?",run[:],occurrenceID).Scan(&occurrenceRaw);err!=nil{return zero,err}
 claimValue,err:=codec.StrictDecodePayload(claimRaw);if err!=nil{return zero,err}
 claim,ok:=claimValue.([]any);if !ok||len(claim)!=10||!equalBytes(claim[0],claimID){return zero,errors.New("historical Claim malformed")}
 claimIDWant,err:=registeredDigest("cote.csf.id.claim","cote.csf.schema.claim-id-preimage",[]any{claim[1],claim[2],claim[3]})
 if err!=nil||!bytes.Equal(claimIDWant[:],claimID){return zero,errors.New("Claim origin-role identity forged")}
 claimDigest,err:=registeredDigest("cote.csf.digest.claim","cote.csf.schema.claim.body",claim[:9])
 if err!=nil||!equalBytes(claim[9],claimDigest[:]){return zero,errors.New("historical Claim digest forged")}
 txValue,err:=codec.StrictDecodePayload(transmissionRaw);if err!=nil{return zero,err}
 transmission,ok:=txValue.([]any)
 if !ok||len(transmission)!=14||!equalBytes(transmissionID,communicationBytes(transmission[0]))||
  transmission[12]!=uint64(0)||!sameCanonical(transmission[1],claim[1])||
  !sameCanonical(transmission[10],[]any{uint64(1),occurrenceID})||
  !sameCanonical(transmission[4],[]any{uint64(0),[]any{[]any{uint64(18),claimID,claimDigest[:]}}}){
  return zero,errors.New("historical Transmission not bound to canonical Claim")
 }
 txID,err:=registeredDigest("cote.csf.id.transmission","cote.csf.schema.transmission-id-preimage",
  []any{transmission[1],transmission[2],transmission[3]})
 if err!=nil||!bytes.Equal(txID[:],transmissionID){return zero,errors.New("historical Transmission ID forged")}
 txDigest,err:=registeredDigest("cote.csf.digest.transmission","cote.csf.schema.transmission.body",transmission[:13])
 if err!=nil||!equalBytes(txDigest[:],communicationBytes(transmission[13])){return zero,errors.New("historical Transmission digest forged")}
 occ,err:=codec.StrictDecodePayload(occurrenceRaw);if err!=nil{return zero,err}
 occurrence,ok:=occ.([]any)
 if !ok||len(occurrence)!=15||!equalBytes(occurrence[0],occurrenceID)||
  !equalBytes(occurrence[1],run[:])||occurrence[5]!="communication.delivery"||
  occurrence[8]!=uint64(0){return zero,errors.New("historical communication occurrence malformed")}
 var oid [32]byte;copy(oid[:],occurrenceID)
 if _,life,err:=validateScheduledOccurrence(run,oid,occurrenceRaw);err!=nil||life!=0{
  return zero,errors.New("historical occurrence signature invalid")
 }
 typed,ok:=occurrence[7].([]any);if !ok||len(typed)!=4{return zero,errors.New("occurrence typed payload missing")}
 env,err:=codec.StrictDecodeEnvelope(communicationBytes(typed[2]));if err!=nil{return zero,err}
 if env.DomainTag!="cote.csf.communication.delivery.v1"||!sameCanonical(env.Value,
  []any{transmissionID,txDigest[:],claimID,claimDigest[:]})||
  !communicationTypedDigestMatches(typed){
  return zero,errors.New("communication occurrence does not authorize delivery of claimed message")
 }
 // Reconcile the source's provenance with the terminal admitted roster.
 if kind==0{
  if !sameCanonical(claim[1],[]any{uint64(5),id[:],sourceDigest[:]})||
   !sameCanonical(transmission[1],claim[1]){
   return zero,errors.New("source Claim was not authored by admitted proposal")
  }
  if !sameCanonical(f[9],communicationSentTyped(transmissionID,txDigest,claimID,claimDigest,
   occurrenceID,occurrence[3])){return zero,errors.New("send Event projection does not bind immutable transmission")}
 }else{
  if !bytes.Equal(id[:],occurrenceID)||!equalBytes(occurrence[14],sourceDigest[:])||
   !sameCanonical(f[11],[]any{[]any{uint64(2),id[:],sourceDigest[:]}}){
   return zero,errors.New("due Event did not consume the exact admitted occurrence")
  }
  if err:=checkDueHistoricalProjection(f,transmission,claim,transmissionID,claimID,occurrenceID);err!=nil{return zero,err}
 }
 // Terminal DecisionRecord must be closed over exactly this admitted source.
 decisions,ok:=journal[16].([]any);if !ok||len(decisions)!=1{return zero,errors.New("communication terminal DecisionRecord roster wrong")}
 did,ok:=idField(decisions[0]);if !ok{return zero,errors.New("terminal DecisionRecord ID missing")}
 var decisionRaw []byte
 if err:=q.QueryRowContext(ctx,"SELECT canonical FROM decision_ledger WHERE id=?",did[:]).Scan(&decisionRaw);err!=nil{return zero,err}
 dv,err:=codec.StrictDecodePayload(decisionRaw);if err!=nil{return zero,err}
 d,ok:=dv.([]any);if !ok||len(d)!=16{return zero,errors.New("communication historical DecisionRecord malformed")}
 wantD,err:=registeredDigest("cote.csf.id.decision","cote.csf.schema.decision-id-preimage",[]any{cycle[:],journal[2],id[:]})
 if err!=nil||wantD!=domain.Digest(did){return zero,errors.New("communication historical DecisionRecord identity differs from source")}
 dh,err:=registeredDigest("cote.csf.digest.decision-record","cote.csf.schema.decision-record.body",d[:15])
 if err!=nil||!sameCanonical(d[15],dh[:])||d[5]!=uint64(0){return zero,errors.New("communication historical DecisionRecord altered")}
 subj:=[]any{uint64(2),id[:],sourceDigest[:]}
 if kind==0{subj=[]any{uint64(0),uint64(0),id[:],sourceDigest[:]}}
 if !sameCanonical(d[4],subj)||!sameCanonical(d[12],[]any{f[0]})||
  !sameCanonical(journal[17],func()[]byte{
   dg,e:=registeredDigest("cote.csf.digest.decision","cote.csf.schema.decision-pair-list",
    []any{[]any{id[:],did[:],dh[:]}})
   if e!=nil{return nil};return dg[:]
  }()){return zero,errors.New("communication historical decision/fence closure differs")}
 return domain.ID(id),nil
}
func communicationSentTyped(txID []byte,txDigest domain.Digest,claimID []byte,claimDigest domain.Digest,
 occ []byte,due any)any{
 at,ok:=asSigned64(due);if !ok{return nil}
 raw,err:=codec.CanonicalBytes("cote.csf.communication.sent.v1","cote.csf.schema.typed-value",1,
  []any{txID,txDigest[:],claimID,claimDigest[:],occ,at})
 if err!=nil{return nil}
 dg:=identity.OpaqueContentHash(raw)
 return []any{"cote.csf.schema.typed-value",uint64(1),raw,dg[:]}
}
func checkDueHistoricalProjection(event,transmission,claim []any,txID,claimID,occurrenceID []byte)error{
 if !sameCanonical(event[12],func()[]any{
  if event[3]=="communication.delivery_failed"{return []any{}}
  return []any{transmission[7].([]any)[0]}
 }()){return errors.New("due Event not addressed to intended recipient")}
 at,ok:=asSigned64(event[5]);if !ok{return errors.New("due Event occurred-at malformed")}
 var tag string
 var content any
 if event[3]=="communication.delivered"{
  tag="cote.csf.communication.delivered.v1"
  content=[]any{transmission[7].([]any)[0],transmission[6],claimID,claim[4],at,transmission[8]}
  if event[15]!=uint64(1){return errors.New("delivered claim event visibility wrong")}
 }else{
  tag="cote.csf.communication.failure.v1"
  content=[]any{txID,occurrenceID,at}
  if event[15]!=uint64(2){return errors.New("failed delivery cannot create recipient observation")}
 }
 raw,err:=codec.CanonicalBytes(tag,"cote.csf.schema.typed-value",1,content)
 if err!=nil{return err}
 digest:=identity.OpaqueContentHash(raw)
 if !sameCanonical(event[9],[]any{"cote.csf.schema.typed-value",uint64(1),raw,digest[:]})||
  !sameCanonical(event[13],[]any{})||event[16]!="communication.delivery"{
  return errors.New("historical communicated content altered or leaked actual author")
 }
 return nil
}

func communicationTypedDigestMatches(typed []any)bool{
 bytes,ok:=typed[2].([]byte);if !ok{return false}
 h:=identity.OpaqueContentHash(bytes)
 return sameCanonical(typed[3],h[:])
}

func communicationBytes(v any)[]byte{x,_:=v.([]byte);return x}

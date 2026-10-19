package sqlite

import (
 "bytes"
 "context"
 "database/sql"
 "errors"
 "fmt"
 "encoding/hex"
 "sort"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/clock"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/genesis"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
)

func (s *Store) BootstrapClock(ctx context.Context, manifest *genesis.Genesis, bundle *policy.Bundle) error {
 if manifest==nil||bundle==nil{return errors.New("verified genesis and bundle required")}
 verified,err:=genesis.ValidateCanonical(manifest.CanonicalBytes,bundle)
 if err!=nil{return err}
 if verified.RunID!=manifest.RunID||verified.ID!=manifest.ID{return errors.New("genesis identity mismatch")}
 state:=clock.State{Instant:verified.InitialInstant,LastOrdinal:-1}
 encoded,err:=clock.Encode(state);if err!=nil{return err}
 typed,ok:=verified.Payload[16].([]any)
 if !ok||len(typed)!=4{return errors.New("genesis missing initial world state")}
 hash,ok:=typed[3].([]byte)
 if !ok||len(hash)!=32{return errors.New("invalid initial state hash")}
 tx,err:=s.db.BeginTx(ctx,nil);if err!=nil{return err}
 defer tx.Rollback()
 var genesisBytes []byte
 if err:=tx.QueryRowContext(ctx,"SELECT canonical FROM genesis_records WHERE run_id=?",verified.RunID[:]).Scan(&genesisBytes);err!=nil{return err}
 if !bytes.Equal(genesisBytes,manifest.CanonicalBytes){return errors.New("genesis persistence mismatch")}
 var previous []byte
 err=tx.QueryRowContext(ctx,"SELECT canonical FROM clock_states WHERE run_id=?",verified.RunID[:]).Scan(&previous)
 if err==nil {
  if !bytes.Equal(previous,encoded){return errors.New("bootstrap clock conflicts with existing run")}
  return tx.Commit()
 }
 if err!=sql.ErrNoRows{return err}
 if _,err=tx.ExecContext(ctx,"INSERT INTO clock_states(run_id,canonical) VALUES(?,?)",verified.RunID[:],encoded);err!=nil{return err}
 // The revision zero is anchored by the genesis typed state (not an event).
 if _,err=tx.ExecContext(ctx,"INSERT INTO world_revisions(revision,state_hash,canonical) VALUES(?,?,?)",0,hash,typed[2]);err!=nil{return err}
 return tx.Commit()
}

func (s *Store) LoadClock(ctx context.Context,runID [32]byte)(clock.State,error) {
 var canonical []byte
 err:=s.db.QueryRowContext(ctx,"SELECT canonical FROM clock_states WHERE run_id=?",runID[:]).Scan(&canonical)
 if err!=nil{return clock.State{},err}
 return clock.Decode(canonical)
}

// SeedGenesisOccurrence is intentionally available only at genesis revision,
// before a cycle is committed. Later schedule changes belong to the future
// CycleCommit publisher and cannot call this bootstrap seam.
func (s *Store) SeedGenesisOccurrence(ctx context.Context,occ domain.ID,runID domain.ID,elig domain.EligibilityCoordinate,canonical []byte) error {
 decoded,err:=codec.StrictDecodePayload(canonical);if err!=nil{return err}
 fields,ok:=decoded.([]any);if !ok||len(fields)!=15{return errors.New("invalid scheduled occurrence schema")}
 if !equalBytes(fields[0],occ[:])||!equalBytes(fields[1],runID[:]){return errors.New("occurrence identity mismatch")}
 storedCoord,ok:=fields[4].([]any)
 if !ok||len(storedCoord)!=2{return errors.New("occurrence eligibility missing")}
 storedInstant,ok:=asSigned64(storedCoord[0]);if !ok{return errors.New("invalid occurrence instant")}
 storedOrdinal,ok:=storedCoord[1].(uint64)
 if !ok||storedInstant!=int64(elig.NotBeforeInstant)||storedOrdinal!=uint64(elig.NotBeforeCycleOrdinal){return errors.New("occurrence eligibility bytes mismatch")}
 if state,err:=s.LoadClock(ctx,[32]byte(runID));err!=nil||state.Revision!=0{return errors.New("genesis occurrence requires initial clock")}
 tx,err:=s.db.BeginTx(ctx,nil);if err!=nil{return err}
 defer tx.Rollback()
 var count int
 if err:=tx.QueryRowContext(ctx,"SELECT count(*) FROM committed_cycles WHERE run_id=?",runID[:]).Scan(&count);err!=nil||count!=0{return errors.New("cannot seed after a cycle commit")}
 if err:=tx.QueryRowContext(ctx,"SELECT count(*) FROM decision_ledger").Scan(&count);err!=nil||count!=0{return errors.New("cannot seed occurrences after fence publication")}
 if _,err=tx.ExecContext(ctx,"INSERT INTO scheduled_occurrences(id,run_id,eligibility_instant,eligibility_ordinal,lifecycle,canonical) VALUES(?,?,?,?,0,?)",occ[:],runID[:],int64(elig.NotBeforeInstant),int64(elig.NotBeforeCycleOrdinal),canonical);err!=nil{return err}
 return tx.Commit()
}

func equalBytes(v any,other []byte) bool {b,ok:=v.([]byte);return ok&&bytes.Equal(b,other)}

func (s *Store) PendingClock(ctx context.Context, runID [32]byte) ([]clock.Pending,error) {
 return pendingClock(ctx,s.db,runID)
}
type clockQuery interface { QueryContext(context.Context,string,...any)(*sql.Rows,error) }
func pendingClock(ctx context.Context,q clockQuery,runID [32]byte)([]clock.Pending,error) {
 rows,err:=q.QueryContext(ctx,`
 SELECT id,eligibility_instant,eligibility_ordinal FROM scheduled_occurrences WHERE run_id=? AND lifecycle=0
 UNION ALL
 SELECT id,eligibility_instant,eligibility_ordinal FROM trigger_activations WHERE run_id=? AND lifecycle=0
 ORDER BY 2,3,1`,runID[:],runID[:])
 if err!=nil{return nil,err}
 defer rows.Close()
 var out []clock.Pending
 for rows.Next(){
  var id []byte;var instant,ordinal int64
  if err:=rows.Scan(&id,&instant,&ordinal);err!=nil{return nil,err}
  if len(id)!=32||ordinal<0{return nil,errors.New("corrupted pending authority")}
  var key domain.ID;copy(key[:],id)
  out=append(out,clock.Pending{ID:key,Eligibility:domain.EligibilityCoordinate{NotBeforeInstant:domain.SimulationInstant(instant),NotBeforeCycleOrdinal:domain.CycleOrdinal(ordinal)}})
 }
 if err:=rows.Err();err!=nil{return nil,err}
 // RoundDeclarations and exogenous inputs share the input authority; they
 // participate in the exact same global minimum as schedule/activations.
 inputs,err:=q.QueryContext(ctx,"SELECT id,canonical FROM input_ledger ORDER BY rowid")
 if err!=nil{return nil,err}
 defer inputs.Close()
 for inputs.Next() {
  var key,raw []byte
  if err:=inputs.Scan(&key,&raw);err!=nil{return nil,err}
  decoded,err:=codec.StrictDecodePayload(raw)
  if err!=nil{continue} // canonical enveloped receipt is not pending work
  fields,ok:=decoded.([]any);if !ok||len(key)!=32{continue}
  var index int
  switch len(fields) {
  case 13:
   // ExogenousInput: the canonical producer and kind are machine IDs.
   if _,ok:=fields[3].(string);!ok{continue}
   if _,ok:=fields[4].(string);!ok{continue}
   index=9
  case 11:
   // RoundDeclaration: actor, slots and lifecycle are schema-bound.
   if _,ok:=fields[3].([]any);!ok{continue}
   if _,ok:=fields[4].([]any);!ok{continue}
   status,ok:=fields[9].(uint64);if !ok||status!=0{continue}
   index=5
  default:continue
  }
  if !equalBytes(fields[1],runID[:]) {continue}
  if !equalBytes(fields[0],key) {return nil,errors.New("pending record ID differs from canonical bytes")}
  coord,ok:=fields[index].([]any)
  if !ok||len(coord)!=2{return nil,errors.New("pending coordinate missing")}
  instant,ok:=asSigned64(coord[0]);if !ok{return nil,errors.New("invalid pending instant")}
  ordinal,ok:=coord[1].(uint64);if !ok{return nil,errors.New("invalid pending ordinal")}
  var id domain.ID;copy(id[:],key)
  out=append(out,clock.Pending{ID:id,Eligibility:domain.EligibilityCoordinate{NotBeforeInstant:domain.SimulationInstant(instant),NotBeforeCycleOrdinal:domain.CycleOrdinal(ordinal)}})
 }
 if err:=inputs.Err();err!=nil{return nil,err}
 // REPEAT_WHILE_TRUE registers its next due instant durably in the
 // trigger runtime. A pending cadence participates in the minimum even
 // without an intervening world-domain event.
 runtimes,err:=q.QueryContext(ctx,"SELECT definition_id,canonical FROM trigger_runtime WHERE run_id=?",runID[:])
 if err!=nil{return nil,err}
 for runtimes.Next(){
  var id,raw []byte
  if err:=runtimes.Scan(&id,&raw);err!=nil{runtimes.Close();return nil,err}
  v,err:=codec.StrictDecodePayload(raw)
  if err!=nil{runtimes.Close();return nil,err}
  fields,ok:=v.([]any)
  if !ok||len(fields)!=8||len(id)!=32||!equalBytes(fields[0],id) {runtimes.Close();return nil,errors.New("corrupted repeat runtime authority")}
  last,ok:=fields[2].([]any);if !ok{runtimes.Close();return nil,errors.New("invalid repeat last-value option")}
  // Only runtime with a true predicate and a next cadence may schedule
  // another repeat. Other policies have ABSENT next-repeat by contract.
  next,ok:=fields[6].([]any);if !ok{runtimes.Close();return nil,errors.New("invalid repeat next option")}
  if len(next)==2 && next[0]==uint64(1) && len(last)==2 && last[0]==uint64(1) && last[1]==true {
   instant,ok:=asSigned64(next[1]);if !ok{runtimes.Close();return nil,errors.New("invalid next repeat instant")}
   var def domain.ID;copy(def[:],id)
   out=append(out,clock.Pending{ID:def,Eligibility:domain.EligibilityCoordinate{NotBeforeInstant:domain.SimulationInstant(instant),NotBeforeCycleOrdinal:0}})
  } else if len(next)!=1 && len(next)!=2 {runtimes.Close();return nil,errors.New("invalid repeat option shape")}
 }
 if err:=runtimes.Err();err!=nil{runtimes.Close();return nil,err}
 if err:=runtimes.Close();err!=nil{return nil,err}
 // The clock's minimum is order-insensitive, but the stored projection has
 // stable byte ordering for deterministic tests and audit queries.
 sort.Slice(out,func(i,j int)bool {
  if d:=out[i].Eligibility.Compare(out[j].Eligibility);d!=0{return d<0}
  return bytes.Compare(out[i].ID[:],out[j].ID[:])<0
 })
 return out,nil
}

func strictFull(record ports.CanonicalRecord,minFields int) ([]any,error){
 decoded,err:=codec.StrictDecodePayload(record.Canonical)
 if err!=nil{return nil,err}
 fields,ok:=decoded.([]any)
 if !ok||len(fields)!=minFields||!equalBytes(fields[0],record.Identity[:]) {
  return nil,errors.New("malformed authoritative record identity")
 }
 return fields,nil
}

// CommitClockAdvance is the SINGLE authority for CSFRV1-3 CLOCK_ADVANCE.
// It checks the admitted fence and input prefix, reselects pending work,
// and atomically publishes journal, events, trigger updates/activations,
// revision and clock. All failures (including fault injection) roll back.
// The SQLite adapter is the last authority boundary. Verify the registered
// ID/digest preimages against the exact canonical bytes, not only record counts.
func registeredDigest(domainTag,schemaID string,value any)(domain.Digest,error){
 encoded,err:=codec.CanonicalBytes(domainTag,schemaID,1,value)
 if err!=nil{return domain.Digest{},err}
 return identity.OpaqueContentHash(encoded),nil
}
func verifyAdvanceEnvelope(b ports.ClockAdvanceBatch,journal []any,state,next clock.State)(error){
 if len(b.Events)!=1{return errors.New("clock advance requires exactly one temporal event")}
 if len(journal)!=24{return errors.New("malformed CycleCommit body")}
 if !equalBytes(journal[0],b.RunID[:])||!equalBytes(journal[1],b.CycleID[:]){return errors.New("CycleCommit run/cycle mismatch")}
 instant,ok:=asSigned64(journal[3])
 if !ok||instant!=int64(state.Instant)||journal[2]!=uint64(0)||journal[4]!=uint64(state.LastOrdinal+1) {
  return errors.New("CycleCommit attempt/origin mismatch")
 }
 if journal[6]!=b.ExpectedRevision||journal[8]!=b.ResultRevision||journal[22]!=uint64(next.NextLogicalSequence){
  return errors.New("CycleCommit revision/logical sequence mismatch")
 }
 if !equalBytes(journal[7],b.BaseStateHash[:])||!equalBytes(journal[9],b.ResultStateHash[:]) {
  return errors.New("CycleCommit world hash mismatch")
 }
 for _,index:=range []int{11,16,18}{
  group,ok:=journal[index].([]any)
  if !ok||len(group)!=0{return errors.New("CLOCK_ADVANCE cannot settle work or perception tasks")}
 }
 ids,ok:=journal[15].([]any)
 if !ok||len(ids)!=1||!equalBytes(ids[0],b.Events[0].Identity[:]) {
  return errors.New("CycleCommit references an event outside its batch")
 }
 actualEvent,err:=strictFull(b.Events[0],19)
 if err!=nil{return err}
 if !equalBytes(actualEvent[1],b.RunID[:])||!equalBytes(actualEvent[7],b.CycleID[:]) ||
  actualEvent[3]!="clock.advanced"||actualEvent[6]!=uint64(state.NextLogicalSequence)||
  actualEvent[8]!=b.ExpectedRevision||actualEvent[15]!=uint64(2) {
  return errors.New("invalid clock event origin, ordering or confidentiality")
 }
 sourceAt,ok:=asSigned64(actualEvent[5])
 if !ok||sourceAt!=int64(state.Instant){return errors.New("clock event at wrong source instant")}
 id,err:=registeredDigest("cote.csf.id.event","cote.csf.schema.event-id-preimage",[]any{b.RunID[:],b.CycleID[:],actualEvent[2]})
 if err!=nil{return err}
 if id!=b.Events[0].Identity{return errors.New("clock event identity preimage mismatch")}
 digest,err:=registeredDigest("cote.csf.digest.event","cote.csf.schema.event.body",actualEvent[:18])
 if err!=nil{return err}
 if !equalBytes(actualEvent[18],digest[:]){return errors.New("clock event digest mismatch")}
 commitID,err:=registeredDigest("cote.csf.id.cycle-commit","cote.csf.schema.cycle-commit-id-preimage",[]any{b.RunID[:],b.CycleID[:],uint64(0)})
 if err!=nil{return err}
 if commitID!=b.Journal.Identity {return errors.New("CycleCommit identity mismatch")}
 digest,err=registeredDigest("cote.csf.digest.batch","cote.csf.schema.batch-preimage",[]any{journal[:23],[]any{b.Events[0].Canonical}})
 if err!=nil{return err}
 if !equalBytes(journal[23],digest[:]){return errors.New("CycleCommit batch digest mismatch")}
 revision,err:=codec.StrictDecodePayload(b.RevisionCanonical)
 if err!=nil{return err}
 fields,ok:=revision.([]any)
 if !ok||len(fields)!=6||fields[0]!=b.ExpectedRevision||fields[3]!=b.ResultRevision ||
 !equalBytes(fields[2],b.BaseStateHash[:])||!equalBytes(fields[5],b.ResultStateHash[:]){
  return errors.New("world revision transition does not match journal")
 }
 return nil
}

func (s *Store) CommitClockAdvance(ctx context.Context,b ports.ClockAdvanceBatch) error {
 if b.ResultRevision!=b.ExpectedRevision+1 || len(b.Events)==0||len(b.Journal.Canonical)==0{return errors.New("incomplete clock advance batch")}
 state,err:=clock.Decode(b.ExpectedClock);if err!=nil{return err}
 next,err:=clock.Decode(b.ResultClock);if err!=nil{return err}
 if uint64(state.Revision)!=b.ExpectedRevision||uint64(next.Revision)!=b.ResultRevision||next.Instant<=state.Instant||next.LastOrdinal!=-1 {return errors.New("invalid clock revision or destination")}
 journalRaw,err:=codec.StrictDecodePayload(b.Journal.Canonical)
 if err!=nil{return err}
 journal,ok:=journalRaw.([]any);if !ok||len(journal)!=24{return errors.New("invalid canonical CycleCommit")}
 if err:=verifyAdvanceEnvelope(b,journal,state,next);err!=nil{return err}
 tx,err:=s.db.BeginTx(ctx,nil);if err!=nil{return err}
 defer tx.Rollback()
 var current []byte
 if err:=tx.QueryRowContext(ctx,"SELECT canonical FROM clock_states WHERE run_id=?",b.RunID[:]).Scan(&current);err!=nil{return err}
 if !bytes.Equal(current,b.ExpectedClock){return errors.New("clock revision changed before publication")}
 var fence []byte
 if err:=tx.QueryRowContext(ctx,"SELECT canonical FROM decision_ledger WHERE id=?",b.FenceID[:]).Scan(&fence);err!=nil{return fmt.Errorf("clock advance has no persisted admission fence: %w",err)}
 f,err:=codec.StrictDecodePayload(fence);if err!=nil{return err}
 ff,ok:=f.([]any)
 if !ok||len(ff)<17||!equalBytes(ff[0],b.RunID[:])||!equalBytes(ff[1],b.CycleID[:]) {return errors.New("invalid clock admission fence")}
 plan,ok:=ff[6].([]any)
 if !ok||len(plan)!=4||plan[0]!=uint64(1){return errors.New("CLOCK_ADVANCE requires persisted clock-advance fence")}
 units,ok:=ff[11].([]any);if !ok||len(units)!=0{return errors.New("CLOCK_ADVANCE cannot admit work units")}
 if !(plan[1]==int64(state.Instant)||plan[1]==uint64(state.Instant)) || !(plan[2]==int64(next.Instant)||plan[2]==uint64(next.Instant)) {
  return errors.New("fence plan does not match clock transition")
 }
 rows,err:=tx.QueryContext(ctx,"SELECT id,canonical FROM input_ledger ORDER BY rowid");if err!=nil{return err}
 cursor:=0
 for rows.Next(){
  var id,canonical []byte
  if err:=rows.Scan(&id,&canonical);err!=nil{rows.Close();return err}
  if cursor>=len(b.ExpectedInputPrefix)||!bytes.Equal(id,b.ExpectedInputPrefix[cursor].Identity[:])||!bytes.Equal(canonical,b.ExpectedInputPrefix[cursor].Canonical){
   rows.Close();return errors.New("clock input prefix changed")
  }
  cursor++
 }
 if err:=rows.Err();err!=nil{rows.Close();return err}
 if err:=rows.Close();err!=nil{return err}
 if cursor!=len(b.ExpectedInputPrefix){return errors.New("clock input prefix changed")}
 pending,err:=pendingClock(ctx,tx,b.RunID);if err!=nil{return err}
 p,ok,err:=clock.Next(state,pending)
 if err!=nil{return err}
 if !ok||p.Kind!=clock.ClockAdvance||int64(p.To)!=b.TargetInstant||uint64(p.Target.NotBeforeCycleOrdinal)!=b.TargetOrdinal{
  return errors.New("provisional clock plan invalidated by pending work")
 }
 // Fence closure proof must have covered the target, not merely origin.
 actualTarget,ok:=plan[3].([]any);if !ok||len(actualTarget)!=2{return errors.New("invalid fence target")}
 if actualTarget[0]!=b.TargetInstant&&actualTarget[0]!=uint64(b.TargetInstant) {return errors.New("fence target mismatch")}
 if actualTarget[1]!=b.TargetOrdinal {return errors.New("fence ordinal mismatch")}
 // CycleCommit is terminal decision authority, NEVER an input record.
 if _,err:=tx.ExecContext(ctx,"INSERT INTO decision_ledger(id,canonical) VALUES(?,?)",b.Journal.Identity[:],b.Journal.Canonical);err!=nil{return err}
 if err:=failClock(b.FailAfterStep,1);err!=nil{return err}
 for _,item:=range b.Events {
  if _,err:=strictFull(item,19);err!=nil{return err}
  if err:=appendToTx(ctx,tx,"event_store",item);err!=nil{return err}
 }
 if err:=failClock(b.FailAfterStep,2);err!=nil{return err}
 for _,change:=range b.TriggerRuntime {
  raw,err:=codec.StrictDecodePayload(change.After);if err!=nil{return err}
  runtime,ok:=raw.([]any)
  if !ok||len(runtime)!=8||!equalBytes(runtime[0],change.DefinitionID[:])||runtime[7]!=b.ResultRevision {
   return errors.New("trigger runtime authority/revision mismatch")
  }
  var prior []byte
  err:=tx.QueryRowContext(ctx,"SELECT canonical FROM trigger_runtime WHERE definition_id=? AND run_id=?",change.DefinitionID[:],b.RunID[:]).Scan(&prior)
  if err!=nil&&err!=sql.ErrNoRows{return err}
  if err==sql.ErrNoRows {
   if len(change.Before)!=0{return errors.New("trigger runtime missing expected prior state")}
   if _,err:=tx.ExecContext(ctx,"INSERT INTO trigger_runtime(definition_id,run_id,canonical) VALUES(?,?,?)",change.DefinitionID[:],b.RunID[:],change.After);err!=nil{return err}
  } else {
   if !bytes.Equal(prior,change.Before){return errors.New("trigger runtime changed")}
   if _,err:=tx.ExecContext(ctx,"UPDATE trigger_runtime SET canonical=? WHERE definition_id=? AND run_id=? AND canonical=?",change.After,change.DefinitionID[:],b.RunID[:],change.Before);err!=nil{return err}
  }
 }
 if err:=failClock(b.FailAfterStep,3);err!=nil{return err}
 for _,item:=range b.Activations {
  fields,err:=strictFull(item,12);if err!=nil{return err}
  if !equalBytes(fields[1],b.RunID[:])||fields[9]!=uint64(0) {
   return errors.New("invalid trigger run/lifecycle")
  }
  definitionID,ok:=fields[3].([]byte);if !ok||len(definitionID)!=32 {
   return errors.New("invalid trigger definition ID")
  }
  if fields[2]!="trigger:"+hex.EncodeToString(definitionID) {
   return errors.New("trigger source is not bound to definition")
  }
  origin,ok:=fields[5].([]any)
  if !ok||len(origin)!=4||origin[0]!=uint64(0)||!equalBytes(origin[1],b.CycleID[:])||origin[2]!=b.ResultRevision {
   return errors.New("trigger activation origin not bound to committed revision")
  }
  causeIDs,ok:=origin[3].([]any);if !ok||len(causeIDs)>1{return errors.New("invalid trigger causing event set")}
  if len(causeIDs)==1&&!equalBytes(causeIDs[0],b.Events[0].Identity[:]) {
   return errors.New("trigger causal origin is outside committed events")
  }
  wantedID,err:=registeredDigest("cote.csf.id.trigger-activation","cote.csf.schema.trigger-activation-id-preimage",[]any{b.RunID[:],definitionID,fields[4],b.ResultRevision,fields[7]})
  if err!=nil{return err}
  if wantedID!=item.Identity{return errors.New("trigger activation identity mismatch")}
  unitDigest,err:=registeredDigest("cote.csf.digest.trigger-activation-unit","cote.csf.schema.trigger-activation.body",fields[:11])
  if err!=nil{return err}
  if !equalBytes(fields[11],unitDigest[:]){return errors.New("trigger activation digest mismatch")}
  eligibility,ok:=fields[6].([]any);if !ok||len(eligibility)!=2{return errors.New("invalid trigger activation eligibility")}
  instant,ok:=asSigned64(eligibility[0]);if !ok{return errors.New("invalid trigger activation instant")}
  ordinal,ok:=eligibility[1].(uint64);if !ok||instant<int64(next.Instant){return errors.New("trigger activation precedes successor floor")}
  if _,err:=tx.ExecContext(ctx,"INSERT INTO trigger_activations(id,run_id,eligibility_instant,eligibility_ordinal,lifecycle,canonical) VALUES(?,?,?,?,0,?)",item.Identity[:],b.RunID[:],instant,ordinal,item.Canonical);err!=nil{return err}
 }
 if err:=failClock(b.FailAfterStep,4);err!=nil{return err}
 if _,err:=tx.ExecContext(ctx,"INSERT INTO world_revisions(revision,state_hash,canonical) VALUES(?,?,?)",b.ResultRevision,b.ResultStateHash[:],b.RevisionCanonical);err!=nil{return err}
 if err:=failClock(b.FailAfterStep,5);err!=nil{return err}
 result,err:=tx.ExecContext(ctx,"UPDATE clock_states SET canonical=? WHERE run_id=? AND canonical=?",b.ResultClock,b.RunID[:],b.ExpectedClock)
 if err!=nil{return err}
 changed,err:=result.RowsAffected();if err!=nil||changed!=1{return errors.New("clock CAS did not update exactly one state")}
 if err:=failClock(b.FailAfterStep,6);err!=nil{return err}
 if _,err:=tx.ExecContext(ctx,"INSERT INTO committed_cycles(run_id,cycle_id,commit_id) VALUES(?,?,?)",b.RunID[:],b.CycleID[:],b.Journal.Identity[:]);err!=nil{return err}
 return tx.Commit()
}

func failClock(inject,stage uint8)error {if inject==stage{return errors.New("injected clock commit failure")};return nil}
func asSigned64(v any)(int64,bool){
 switch n:=v.(type){case int64:return n,true;case uint64:if n<=uint64(^uint64(0)>>1){return int64(n),true}}
 return 0,false
}
func strictFullRelaxed(item ports.CanonicalRecord,length int)([]any,error){
 data,err:=codec.StrictDecodePayload(item.Canonical);if err!=nil{return nil,err}
 fields,ok:=data.([]any);if !ok||len(fields)!=length{return nil,errors.New("invalid canonical authoritative state")}
 return fields,nil
}
func appendToTx(ctx context.Context,tx *sql.Tx,table string,record ports.CanonicalRecord)error{
 if table!="event_store"{return errors.New("not an event authority table")}
 _,err:=tx.ExecContext(ctx,"INSERT INTO event_store(id,canonical) VALUES(?,?)",record.Identity[:],record.Canonical)
 return err
}

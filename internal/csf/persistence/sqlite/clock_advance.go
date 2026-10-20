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
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/trigger"
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
 revisionZero,err:=codec.CanonicalPayload([]any{uint64(0),typed,hash,uint64(0),typed,hash})
 if err!=nil{return err}
 if _,err=tx.ExecContext(ctx,"INSERT INTO world_revisions(revision,state_hash,canonical) VALUES(?,?,?)",0,hash,revisionZero);err!=nil{return err}
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
// validateScheduledOccurrence authenticates the authoritative record rather than
// trusting its query index. The lifecycle/coordinate columns are projections.
func validateScheduledOccurrence(runID [32]byte,id [32]byte,raw []byte)(domain.EligibilityCoordinate,uint64,error){
 var coord domain.EligibilityCoordinate
 decoded,err:=codec.StrictDecodePayload(raw);if err!=nil{return coord,0,err}
 fields,ok:=decoded.([]any);if !ok||len(fields)!=15||!equalBytes(fields[0],id[:])||!equalBytes(fields[1],runID[:]){return coord,0,errors.New("invalid scheduled occurrence identity/schema")}
 if _,ok:=fields[2].(string);!ok{return coord,0,errors.New("occurrence source missing")}
 if _,ok:=fields[5].(string);!ok{return coord,0,errors.New("occurrence kind missing")}
 due,ok:=asSigned64(fields[3]);if !ok{return coord,0,errors.New("invalid occurrence due instant")}
 pair,ok:=fields[4].([]any);if !ok||len(pair)!=2{return coord,0,errors.New("occurrence eligibility missing")}
 instant,ok:=asSigned64(pair[0]);if !ok||due>instant{return coord,0,errors.New("occurrence due after eligibility")}
 ordinal,ok:=pair[1].(uint64);if !ok||ordinal>uint64(^uint64(0)>>1){return coord,0,errors.New("invalid occurrence ordinal")}
 life,ok:=fields[8].(uint64);if !ok||life>2{return coord,0,errors.New("invalid occurrence lifecycle")}
 origin,ok:=fields[9].([]any);if !ok||len(origin)!=2{return coord,0,errors.New("invalid occurrence origin")}
 role,ok:=fields[10].(uint64);if !ok||role>65535{return coord,0,errors.New("invalid occurrence role")}
 local,ok:=fields[11].(uint64);if !ok||local>uint64(^uint32(0)){return coord,0,errors.New("invalid occurrence local ordinal")}
 derived,err:=registeredDigest("cote.csf.id.occurrence","cote.csf.schema.occurrence-id-preimage",[]any{fields[9],fields[10],fields[11]});if err!=nil{return coord,0,err}
 if derived!=domain.Digest(id){return coord,0,errors.New("scheduled occurrence ID preimage mismatch")}
 digest,err:=registeredDigest("cote.csf.digest.occurrence-unit","cote.csf.schema.scheduled-occurrence.body",fields[:14]);if err!=nil{return coord,0,err}
 if !equalBytes(fields[14],digest[:]){return coord,0,errors.New("scheduled occurrence digest mismatch")}
 return domain.EligibilityCoordinate{NotBeforeInstant:domain.SimulationInstant(instant),NotBeforeCycleOrdinal:domain.CycleOrdinal(ordinal)},life,nil
}

func validateTriggerActivation(runID [32]byte,id [32]byte,raw []byte)(domain.EligibilityCoordinate,uint64,error){
 var coord domain.EligibilityCoordinate
 decoded,err:=codec.StrictDecodePayload(raw);if err!=nil{return coord,0,err}
 fields,ok:=decoded.([]any);if !ok||len(fields)!=12||!equalBytes(fields[0],id[:])||!equalBytes(fields[1],runID[:]){return coord,0,errors.New("invalid trigger activation identity/schema")}
 pair,ok:=fields[6].([]any);if !ok||len(pair)!=2{return coord,0,errors.New("trigger activation eligibility missing")}
 instant,ok:=asSigned64(pair[0]);if !ok{return coord,0,errors.New("invalid trigger activation instant")}
 ordinal,ok:=pair[1].(uint64);if !ok||ordinal>uint64(^uint64(0)>>1){return coord,0,errors.New("invalid trigger activation ordinal")}
 life,ok:=fields[9].(uint64);if !ok||(life!=0&&life!=2){return coord,0,errors.New("invalid trigger activation lifecycle")}
 digest,err:=registeredDigest("cote.csf.digest.trigger-activation-unit","cote.csf.schema.trigger-activation.body",fields[:11]);if err!=nil{return coord,0,err}
 if !equalBytes(fields[11],digest[:]){return coord,0,errors.New("trigger activation digest mismatch")}
 return domain.EligibilityCoordinate{NotBeforeInstant:domain.SimulationInstant(instant),NotBeforeCycleOrdinal:domain.CycleOrdinal(ordinal)},life,nil
}

// Genesis seeding is a one-time authorized mutation; the persisted source
// must be fully derivable from the genesis identity and canonical bytes.
func (s *Store) SeedGenesisOccurrence(ctx context.Context,occ domain.ID,runID domain.ID,elig domain.EligibilityCoordinate,canonical []byte) error {
 computed,life,err:=validateScheduledOccurrence([32]byte(runID),[32]byte(occ),canonical)
 if err!=nil{return err}
 if life!=0||computed!=elig{return errors.New("initial occurrence must be pending at its declared eligibility")}
 decoded,_:=codec.StrictDecodePayload(canonical)
 fields:=decoded.([]any)
 origin:=fields[9].([]any)
 if origin[0]!=uint64(2){return errors.New("bootstrap occurrence must originate at genesis")}
 genesisRef,ok:=origin[1].([]any);if !ok||len(genesisRef)!=2{return errors.New("invalid genesis occurrence origin")}
 tx,err:=s.db.BeginTx(ctx,nil);if err!=nil{return err}
 defer tx.Rollback()
 var genesisID,genesisDigest []byte
 if err:=tx.QueryRowContext(ctx,"SELECT genesis_id,digest FROM genesis_records WHERE run_id=?",runID[:]).Scan(&genesisID,&genesisDigest);err!=nil{return err}
 if !equalBytes(genesisRef[0],genesisID)||!equalBytes(genesisRef[1],genesisDigest){return errors.New("occurrence origin does not match verified genesis")}
 var current []byte
 if err:=tx.QueryRowContext(ctx,"SELECT canonical FROM clock_states WHERE run_id=?",runID[:]).Scan(&current);err!=nil{return err}
 state,err:=clock.Decode(current);if err!=nil||state.Revision!=0{return errors.New("genesis occurrence requires initial clock")}
 if elig.NotBeforeInstant<state.Instant{return errors.New("genesis occurrence cannot be eligible before the initial clock")}
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
 // Read every row, including terminal rows: filtering lifecycle before
 // comparing its indexed projection with the canonical record hides corruption.
 selections:=[]struct{query string;activation bool}{
  {"SELECT id,eligibility_instant,eligibility_ordinal,lifecycle,canonical FROM scheduled_occurrences WHERE run_id=?",false},
  {"SELECT id,eligibility_instant,eligibility_ordinal,lifecycle,canonical FROM trigger_activations WHERE run_id=?",true},
 }
 var out []clock.Pending
 for _,selection:=range selections{
  rows,err:=q.QueryContext(ctx,selection.query,runID[:]);if err!=nil{return nil,err}
  for rows.Next(){
   var id,raw []byte
   var instant,ordinal,life int64
   if err:=rows.Scan(&id,&instant,&ordinal,&life,&raw);err!=nil{rows.Close();return nil,err}
   if len(id)!=32||ordinal<0||life<0{return nil,errors.New("corrupted pending authority columns")}
   var key [32]byte;copy(key[:],id)
   var derived domain.EligibilityCoordinate
   var derivedLife uint64
   if selection.activation {derived,derivedLife,err=validateTriggerActivation(runID,key,raw)
   }else{derived,derivedLife,err=validateScheduledOccurrence(runID,key,raw)}
   if err!=nil{rows.Close();return nil,err}
   if int64(derived.NotBeforeInstant)!=instant||uint64(derived.NotBeforeCycleOrdinal)!=uint64(ordinal)||derivedLife!=uint64(life){
    rows.Close();return nil,errors.New("pending index columns disagree with canonical authority")
   }
   if life==0{out=append(out,clock.Pending{ID:domain.ID(key),Eligibility:derived})}
  }
  if err:=rows.Err();err!=nil{rows.Close();return nil,err}
  if err:=rows.Close();err!=nil{return nil,err}
 }
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
func verifyAdvanceEnvelope(b ports.ClockAdvanceBatch,journal []any,state,next clock.State,initialState []any) error {
 if len(b.Events)<2{return errors.New("clock advance requires clock and synthetic body events")}
 if len(journal)!=24{return errors.New("malformed CycleCommit body")}
 if !equalBytes(journal[0],b.RunID[:])||!equalBytes(journal[1],b.CycleID[:]){return errors.New("CycleCommit run/cycle mismatch")}
 instant,ok:=asSigned64(journal[3])
 if !ok||instant!=int64(state.Instant)||journal[2]!=uint64(0)||journal[4]!=uint64(state.LastOrdinal+1){
  return errors.New("CycleCommit attempt/origin mismatch")
 }
 if uint64(next.NextLogicalSequence)!=uint64(state.NextLogicalSequence)+uint64(len(b.Events))||journal[6]!=b.ExpectedRevision||
  journal[8]!=b.ResultRevision||journal[22]!=uint64(next.NextLogicalSequence){
  return errors.New("CycleCommit revision/logical sequence mismatch")
 }
 if !equalBytes(journal[7],b.BaseStateHash[:])||!equalBytes(journal[9],b.ResultStateHash[:]){
  return errors.New("CycleCommit world hash mismatch")
 }
 for _,index:=range []int{11,16,18}{
  group,ok:=journal[index].([]any)
  if !ok||len(group)!=0{return errors.New("CLOCK_ADVANCE cannot settle work or perception tasks")}
 }
 ids,ok:=journal[15].([]any)
 if !ok||len(ids)!=len(b.Events){return errors.New("CycleCommit event count mismatch")}
 canonicalEvents:=make([]any,0,2)
 for i,item:=range b.Events{
  if !equalBytes(ids[i],item.Identity[:]){return errors.New("CycleCommit event order mismatch")}
  fields,err:=strictFull(item,19);if err!=nil{return err}
  if i>=2{
   var kind,tag string
   key,ok:=fields[2].([]any)
   if !ok||len(key)!=6{return errors.New("trigger lifecycle event order key malformed")}
   phase,ok:=key[0].(uint64);if !ok{return errors.New("trigger lifecycle event phase malformed")}
   switch phase{
   case 3:kind="trigger.runtime.transition";tag="cote.csf.trigger.runtime-transition.v1"
   case 4:kind="trigger.activation.created";tag="cote.csf.trigger.activation-created.v1"
   default:return errors.New("unexpected event phase in temporal CycleCommit")
   }
   if !equalBytes(fields[1],b.RunID[:])||!equalBytes(fields[7],b.CycleID[:])||
    fields[3]!=kind||fields[4]!=uint64(1)||fields[6]!=uint64(state.NextLogicalSequence)+uint64(i)||
    fields[8]!=b.ExpectedRevision||fields[15]!=uint64(2)||fields[16]!="trigger"||fields[17]!=uint64(1)||
    key[2]!="trigger"||key[3]!=uint64(1)||key[4]!=uint64(0)||key[5]!=uint64(0){
    return errors.New("trigger lifecycle event origin/sequence invalid")
   }
   from,ok:=asSigned64(fields[5]);if !ok||from!=int64(state.Instant){return errors.New("trigger lifecycle event instant invalid")}
   typed,ok:=fields[9].([]any);if !ok||len(typed)!=4||typed[0]!="cote.csf.schema.typed-value"||typed[1]!=uint64(1){return errors.New("trigger lifecycle typed-value invalid")}
   payload,ok:=typed[2].([]byte);if !ok{return errors.New("trigger lifecycle payload invalid")}
   hash:=identity.OpaqueContentHash(payload)
   if !equalBytes(typed[3],hash[:]){return errors.New("trigger lifecycle payload hash invalid")}
   env,err:=codec.StrictDecodeEnvelope(payload);if err!=nil||env.DomainTag!=tag||env.SchemaID!="cote.csf.schema.typed-value"||env.SchemaVersion!=1{return errors.New("trigger lifecycle schema invalid")}
   val,ok:=env.Value.([]any);if !ok||len(val)!=2||!sameCanonical(val[0],key[1]){return errors.New("trigger lifecycle origin payload mismatch")}
   for _,slot:=range []int{10,11,12,13}{part,ok:=fields[slot].([]any);if !ok||len(part)!=0{return errors.New("trigger lifecycle leaks source or observer refs")}}
   if !sameCanonical(fields[14],[]any{uint64(0)}){return errors.New("trigger lifecycle location unexpected")}
   derived,err:=registeredDigest("cote.csf.id.event","cote.csf.schema.event-id-preimage",[]any{b.RunID[:],b.CycleID[:],fields[2]});if err!=nil{return err}
   if derived!=domain.Digest(item.Identity){return errors.New("trigger lifecycle event ID invalid")}
   digest,err:=registeredDigest("cote.csf.digest.event","cote.csf.schema.event.body",fields[:18]);if err!=nil{return err}
   if !equalBytes(fields[18],digest[:]){return errors.New("trigger lifecycle event digest invalid")}
   // EventOrderKey must increase strictly, including across phase 3 and 4.
   preceding,err:=codec.StrictDecodePayload(b.Events[i-1].Canonical);if err!=nil{return err}
   pre:=preceding.([]any)
   lastKey,err:=codec.CanonicalPayload(pre[2]);if err!=nil{return err}
   thisKey,err:=codec.CanonicalPayload(key);if err!=nil{return err}
   if bytes.Compare(lastKey,thisKey)>=0{return errors.New("trigger lifecycle events out of causal order")}
   canonicalEvents=append(canonicalEvents,item.Canonical)
   continue
  }
  if !sameCanonical(fields[14],[]any{uint64(0)}){return errors.New("synthetic event location mismatch")}
  for _,slot:=range []int{10,11,12,13}{
   part,ok:=fields[slot].([]any)
   if !ok||len(part)!=0{return errors.New("synthetic event carries unauthorized causal provenance")}
  }
  kind:="clock.advanced"
  tag:="cote.csf.event.clock-advanced"
  schema:="cote.csf.schema.clock-advanced"
  if i==1{kind="body.advanced";tag="cote.csf.body.synthetic-advance.v1";schema="cote.csf.schema.typed-value"}
  if !equalBytes(fields[1],b.RunID[:])||!equalBytes(fields[7],b.CycleID[:])||
   fields[3]!=kind||fields[4]!=uint64(1)||fields[6]!=uint64(state.NextLogicalSequence)+uint64(i)||
   fields[8]!=b.ExpectedRevision||fields[15]!=uint64(2)||fields[16]!="clock"||fields[17]!=uint64(1){
   return errors.New("invalid synthetic event origin, sequence or confidentiality")
  }
  instant,ok:=asSigned64(fields[5]);if !ok||instant!=int64(state.Instant){return errors.New("synthetic event at wrong source instant")}
  order:=[]any{uint64(0),b.CycleID[:],"clock",uint64(1),uint64(i),uint64(0)}
  expectedOrder,err:=codec.CanonicalPayload(order);if err!=nil{return err}
  actualOrder,err:=codec.CanonicalPayload(fields[2]);if err!=nil||!bytes.Equal(actualOrder,expectedOrder){return errors.New("synthetic event order changed")}
  typed,ok:=fields[9].([]any)
  if !ok||len(typed)!=4||typed[0]!=schema||typed[1]!=uint64(1){return errors.New("synthetic event payload typed-value mismatch")}
  payloadBytes,ok:=typed[2].([]byte);if !ok{return errors.New("synthetic event payload missing")}
  givenHash,ok:=typed[3].([]byte);if !ok{return errors.New("synthetic event value hash missing")}
  payloadHash:=identity.OpaqueContentHash(payloadBytes)
  if !bytes.Equal(payloadHash[:],givenHash){return errors.New("synthetic event value digest mismatch")}
  env,err:=codec.StrictDecodeEnvelope(payloadBytes)
  if err!=nil||env.DomainTag!=tag||env.SchemaID!=schema||env.SchemaVersion!=1{return errors.New("unregistered synthetic event contract")}
  data,ok:=env.Value.([]any)
  if !ok||len(data)!=2{return errors.New("synthetic event temporal payload malformed")}
  from,ok:=asSigned64(data[0]);if !ok||from!=int64(state.Instant){return errors.New("synthetic event temporal origin mismatch")}
  to,ok:=asSigned64(data[1]);if !ok||to!=int64(next.Instant){return errors.New("synthetic event temporal target mismatch")}
  id,err:=registeredDigest("cote.csf.id.event","cote.csf.schema.event-id-preimage",[]any{b.RunID[:],b.CycleID[:],fields[2]})
  if err!=nil{return err}
  if id!=item.Identity{return errors.New("synthetic event identity preimage mismatch")}
  digest,err:=registeredDigest("cote.csf.digest.event","cote.csf.schema.event.body",fields[:18])
  if err!=nil{return err}
  if !equalBytes(fields[18],digest[:]){return errors.New("synthetic event digest mismatch")}
  canonicalEvents=append(canonicalEvents,item.Canonical)
 }
 commitID,err:=registeredDigest("cote.csf.id.cycle-commit","cote.csf.schema.cycle-commit-id-preimage",[]any{b.RunID[:],b.CycleID[:],uint64(0)})
 if err!=nil{return err}
 if commitID!=b.Journal.Identity{return errors.New("CycleCommit identity mismatch")}
 digest,err:=registeredDigest("cote.csf.digest.batch","cote.csf.schema.batch-preimage",[]any{journal[:23],canonicalEvents})
 if err!=nil{return err}
 if !equalBytes(journal[23],digest[:]){return errors.New("CycleCommit batch digest mismatch")}
 revision,err:=codec.StrictDecodePayload(b.RevisionCanonical);if err!=nil{return err}
 fields,ok:=revision.([]any)
 if !ok||len(fields)!=6||fields[0]!=b.ExpectedRevision||fields[3]!=b.ResultRevision||
  !equalBytes(fields[2],b.BaseStateHash[:])||!equalBytes(fields[5],b.ResultStateHash[:]){
  return errors.New("world revision transition does not match journal")
 }
 if len(initialState)!=4||!equalBytes(initialState[3],b.BaseStateHash[:])||
  !sameCanonical(fields[1],initialState)||!sameCanonical(fields[4],initialState){
  return errors.New("temporal world revision state differs from genesis")
 }
 return nil
}

func sameCanonical(a,b any) bool{
 ab,err:=codec.CanonicalPayload(a);if err!=nil{return false}
 bb,err:=codec.CanonicalPayload(b);return err==nil&&bytes.Equal(ab,bb)
}
func (s *Store) initialTypedState(ctx context.Context,runID [32]byte)([]any,error){
 raw,err:=s.LoadGenesis(ctx,runID);if err!=nil{return nil,err}
 env,err:=codec.StrictDecodeEnvelope(raw);if err!=nil{return nil,err}
 payload,ok:=env.Value.([]any);if !ok||len(payload)<17{return nil,errors.New("invalid stored genesis payload")}
 typed,ok:=payload[16].([]any);if !ok||len(typed)!=4{return nil,errors.New("invalid genesis state typed value")}
 stateRaw,ok:=typed[2].([]byte);if !ok{return nil,errors.New("genesis state envelope missing")}
 h:=identity.OpaqueContentHash(stateRaw)
 if !equalBytes(typed[3],h[:]){return nil,errors.New("genesis state typed-value hash differs")}
 return typed,nil
}

type queryOne interface{
 QueryRowContext(context.Context,string,...any)*sql.Row
 QueryContext(context.Context,string,...any)(*sql.Rows,error)
}
// Confirm the full admitted coordinate and the authentic fence whose stable
// (run,cycle,attempt) identity must exist before a terminal CycleCommit.
func verifyCommittedFence(ctx context.Context,q queryOne,runID,cycleID [32]byte,journal []any)([32]byte,error){
 var zero [32]byte
 fenceID,err:=registeredDigest("cote.csf.id.admission-fence","cote.csf.schema.admission-fence-id-preimage",[]any{runID[:],cycleID[:],uint64(0)})
 if err!=nil{return zero,err}
 var canonical []byte
 if err:=q.QueryRowContext(ctx,"SELECT canonical FROM decision_ledger WHERE id=?",fenceID[:]).Scan(&canonical);err!=nil{return zero,fmt.Errorf("CycleCommit without AdmissionFence: %w",err)}
 decoded,err:=codec.StrictDecodePayload(canonical);if err!=nil{return zero,err}
 fence,ok:=decoded.([]any)
 if !ok||len(fence)!=18||!equalBytes(fence[0],runID[:])||!equalBytes(fence[1],cycleID[:])||fence[2]!=uint64(0){return zero,errors.New("AdmissionFence identity invalid")}
 fd,err:=registeredDigest("cote.csf.digest.fence","cote.csf.schema.admission-fence.body",fence[:17]);if err!=nil{return zero,err}
 if !equalBytes(fence[17],fd[:])||!equalBytes(journal[10],fd[:]){return zero,errors.New("CycleCommit has wrong persisted AdmissionFence digest")}
 // This slice only publishes the initial CLOCK_ADVANCE attempt. No
 // AttemptRetryRecord or alternate rule version can be introduced here.
 if !sameCanonical(fence[3],[]any{uint64(0)}) {return zero,errors.New("initial clock advance cannot claim retry authority")}
 rules,ok:=fence[16].([]any)
 if !ok||len(rules)!=0{return zero,errors.New("clock advance fence has unapproved rule versions")}

 for _,pair:=range [][2]int{{4,3},{5,4},{6,5},{7,6},{8,7},{11,11},{12,12}}{
  if !sameCanonical(fence[pair[0]],journal[pair[1]]){return zero,errors.New("CycleCommit is not the admitted fence plan/inputs")}
 }
 admitted,ok:=fence[11].([]any);if !ok||len(admitted)!=0{return zero,errors.New("CLOCK_ADVANCE admitted WORK units")}
  // The terminal empty cohorts are versioned claims, not arbitrary 32-byte
  // strings. Hash the actual empty lists, including the cohort identity,
  // before trusting even an internally re-hashed fence and journal.
  emptyInput,err:=registeredDigest("cote.csf.digest.input","cote.csf.schema.admitted-unit-list",[]any{})
  if err!=nil{return zero,err}
  if !equalBytes(fence[12],emptyInput[:])||!equalBytes(journal[12],emptyInput[:]){
   return zero,errors.New("CLOCK_ADVANCE input digest is not the canonical empty digest")
  }
  cohort,ok:=fence[10].([]any)
  if !ok||len(cohort)!=2{return zero,errors.New("CLOCK_ADVANCE cohort malformed")}
  rounds,ok:=cohort[1].([]any)
  if !ok||len(rounds)!=0{return zero,errors.New("CLOCK_ADVANCE cannot contain a round cohort")}
  cohortID,err:=registeredDigest("cote.csf.id.cohort","cote.csf.schema.cohort-id-preimage",[]any{cycleID[:],journal[6],[]any{}})
  if err!=nil{return zero,err}
  if !equalBytes(cohort[0],cohortID[:]){return zero,errors.New("CLOCK_ADVANCE empty cohort identity mismatch")}
  emptyDecision,err:=registeredDigest("cote.csf.digest.decision","cote.csf.schema.decision-pair-list",[]any{})
  if err!=nil{return zero,err}
  if !equalBytes(journal[17],emptyDecision[:]){return zero,errors.New("CLOCK_ADVANCE decision digest differs from empty decision list")}
  emptyTasks,err:=registeredDigest("cote.csf.digest.perception-task-list","cote.csf.schema.perception-task-id-list",[]any{})
  if err!=nil{return zero,err}
  if !equalBytes(journal[19],emptyTasks[:]){return zero,errors.New("CLOCK_ADVANCE perception digest differs from empty task list")}

 proofs,ok:=fence[9].([]any);if !ok{return zero,errors.New("AdmissionFence proof shape invalid")}
 var previous string
 for i,item:=range proofs{
  proof,ok:=item.([]any);if !ok||len(proof)!=5{return zero,errors.New("AdmissionFence closure proof malformed")}
  source,ok:=proof[0].(string);if !ok||source==""||(i>0&&source<=previous){return zero,errors.New("AdmissionFence closure sources unordered")}
  previous=source
  ref,ok:=proof[1].([]any);if !ok||len(ref)!=3||ref[0]!=uint64(2){return zero,errors.New("closure proof ref malformed")}
  id,ok:=ref[1].([]byte);if !ok||len(id)!=32{return zero,errors.New("closure proof source ID malformed")}
  var stored []byte
  if err:=q.QueryRowContext(ctx,"SELECT canonical FROM input_ledger WHERE id=?",id).Scan(&stored);err!=nil{return zero,err}
  root,err:=codec.StrictDecodePayload(stored);if err!=nil{return zero,err}
  fields,ok:=root.([]any)
  if !ok||len(fields)!=7||!equalBytes(fields[0],id)||fields[2]!=source||
   !sameCanonical(fields[4],proof[3])||!sameCanonical(fields[5],proof[4])||!sameCanonical(fields[6],proof[2]){
   return zero,errors.New("closure proof fields not supported by SourceClosure")
  }
  expectedID,err:=registeredDigest("cote.csf.id.source-closure","cote.csf.schema.source-closure-id-preimage",[]any{fields[1],fields[2],fields[3]});if err!=nil{return zero,err}
  expectedDigest,err:=registeredDigest("cote.csf.digest.source-closure","cote.csf.schema.source-closure",fields);if err!=nil{return zero,err}
  if !bytes.Equal(expectedID[:],id)||!equalBytes(ref[2],expectedDigest[:]){return zero,errors.New("closure proof source hash mismatch")}
  var receiptID []byte
  if err:=q.QueryRowContext(ctx,"SELECT record_id FROM source_ingress_receipts WHERE source_id=? AND ingress_seq=?",source,proof[2]).Scan(&receiptID);err!=nil{return zero,err}
  if !bytes.Equal(receiptID,id){return zero,errors.New("closure proof receipt differs from source")}
  coverage,ok:=proof[3].([]any);if !ok||len(coverage)!=2{return zero,errors.New("closure proof coordinate malformed")}
  from,ok:=asSigned64(coverage[0]);if !ok{return zero,errors.New("closure proof instant malformed")}
  ordinal,ok:=coverage[1].(uint64);if !ok{return zero,errors.New("closure proof ordinal malformed")}
  plan,ok:=journal[5].([]any);if !ok||len(plan)!=4{return zero,errors.New("clock plan invalid")}
  target,ok:=plan[3].([]any);if !ok||len(target)!=2{return zero,errors.New("clock target invalid")}
  to,ok:=asSigned64(target[0]);if !ok{return zero,errors.New("clock target instant invalid")}
  targetOrdinal,ok:=target[1].(uint64);if !ok{return zero,errors.New("clock target ordinal invalid")}
  if proof[4]!=true&&(from<to||(from==to&&ordinal<targetOrdinal)){return zero,errors.New("closure proof does not cover target")}
 }
 if err:=verifyFenceClosureRoster(ctx,q,runID,fence[9],fence[6]);err!=nil{return zero,err}
 if err:=verifyJournalPolicyPins(ctx,q,runID,fence,journal);err!=nil{return zero,err}
 return [32]byte(fenceID),nil
}

// The closure proof is a function of every declared source and the FIRST
// covering closure by ingress sequence, not a user-chosen witness subset.
func verifyFenceClosureRoster(ctx context.Context,q queryOne,runID [32]byte,proofsRaw,planRaw any) error {
 proofs,ok:=proofsRaw.([]any);if !ok{return errors.New("invalid fence closure proofs")}
 plan,ok:=planRaw.([]any);if !ok||len(plan)!=4||plan[0]!=uint64(1){return errors.New("invalid CLOCK_ADVANCE closure target")}
 target,ok:=plan[3].([]any);if !ok||len(target)!=2{return errors.New("missing closure target")}
 targetInstant,ok:=asSigned64(target[0]);if !ok{return errors.New("invalid closure target instant")}
 targetOrdinal,ok:=target[1].(uint64);if !ok{return errors.New("invalid closure target ordinal")}
 type witness struct{seq uint64;data []any}
 declared:=map[string]bool{}
 first:=map[string]witness{}
 rows,err:=q.QueryContext(ctx,"SELECT id,canonical FROM input_ledger ORDER BY rowid")
 if err!=nil{return err}
 for rows.Next(){
  var id,raw []byte
  if err:=rows.Scan(&id,&raw);err!=nil{rows.Close();return err}
  fieldsAny,err:=codec.StrictDecodePayload(raw)
  if err!=nil{continue} // SourceIngressReceipt is an enveloped record.
  fields,ok:=fieldsAny.([]any);if !ok{rows.Close();return errors.New("input ledger has invalid canonical record")}
  if len(fields)==3 && fields[0]=="source-declaration-v1" && equalBytes(fields[1],runID[:]){
   source,ok:=fields[2].(string)
   h:=identity.OpaqueContentHash(raw)
   if !ok||source==""||!bytes.Equal(h[:],id)||declared[source]{rows.Close();return errors.New("source roster not canonical")}
   declared[source]=true
  }
  if len(fields)!=7||!equalBytes(fields[1],runID[:]){continue}
  source,ok:=fields[2].(string);if !ok||source==""{rows.Close();return errors.New("closure source invalid")}
  seq,ok:=fields[6].(uint64);if !ok{return errors.New("closure ingress sequence invalid")}
  coverage,ok:=fields[4].([]any);if !ok||len(coverage)!=2{rows.Close();return errors.New("closure eligibility invalid")}
  at,ok:=asSigned64(coverage[0]);if !ok{rows.Close();return errors.New("closure coverage instant invalid")}
  ord,ok:=coverage[1].(uint64);if !ok{rows.Close();return errors.New("closure coverage ordinal invalid")}
  final,ok:=fields[5].(bool);if !ok{rows.Close();return errors.New("closure final flag invalid")}
  if !final&&(at<targetInstant||(at==targetInstant&&ord<targetOrdinal)){continue}
  expectedID,err:=registeredDigest("cote.csf.id.source-closure","cote.csf.schema.source-closure-id-preimage",[]any{fields[1],fields[2],fields[3]})
  if err!=nil{rows.Close();return err}
  if !equalBytes(fields[0],id)||!bytes.Equal(expectedID[:],id){rows.Close();return errors.New("covering closure identity invalid")}
  digest,err:=registeredDigest("cote.csf.digest.source-closure","cote.csf.schema.source-closure",fields)
  if err!=nil{rows.Close();return err}
  candidate:=witness{seq:seq,data:[]any{source,[]any{uint64(2),id,digest[:]},seq,fields[4],final}}
  previous,ok:=first[source];if !ok||seq<previous.seq{first[source]=candidate}
 }
 if err:=rows.Err();err!=nil{rows.Close();return err}
 if err:=rows.Close();err!=nil{return err}
 if len(declared)!=len(proofs){return errors.New("admission fence omitted a declared source closure")}
 previousSource:=""
 for _,raw:=range proofs{
  proof,ok:=raw.([]any);if !ok||len(proof)!=5{return errors.New("malformed canonical closure witness")}
  source,ok:=proof[0].(string)
  if !ok||!declared[source]||source<=previousSource{return errors.New("fence closure roster/order differs from declared sources")}
  previousSource=source
  expected,ok:=first[source]
  if !ok||!sameCanonical(proof,expected.data){return errors.New("fence closure witness is not the first covering closure")}
 }
 return nil
}

// Only the role-specific policy refs persisted by the verified genesis are
// permitted in either the fence or its CycleCommit journal.
func verifyJournalPolicyPins(ctx context.Context,q queryOne,runID [32]byte,fence,journal []any) error {
 var canonical []byte
 if err:=q.QueryRowContext(ctx,"SELECT canonical FROM genesis_records WHERE run_id=?",runID[:]).Scan(&canonical);err!=nil{return err}
 envelope,err:=codec.StrictDecodeEnvelope(canonical);if err!=nil{return err}
 payload,ok:=envelope.Value.([]any);if !ok||len(payload)!=18{return errors.New("genesis policy contract not found")}
 refs,ok:=payload[14].([]any);if !ok{return errors.New("genesis policy registry not found")}
 bindings:=[]struct{observed any;id string}{
  {fence[13],"admission-order-v1"},
  {fence[14],"causal-identity-v1"},
  {fence[15],"admission-fence-v1"},
  {journal[13],"cycle-coordinate-v1"},
  {journal[14],"event-order-v1"},
  {journal[20],"perception-v1"},
  {journal[21],"perception-identity-v1"},
 }
 for _,binding:=range bindings{
  matched:=false
  for _,raw:=range refs{
   ref,ok:=raw.([]any)
   if ok&&len(ref)==3&&ref[0]==binding.id{
    if !sameCanonical(binding.observed,ref){return fmt.Errorf("cycle policy ref %s not bound to genesis",binding.id)}
    matched=true;break
   }
  }
  if !matched{return errors.New("missing required genesis policy role")}
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
 initialState,err:=s.initialTypedState(ctx,b.RunID);if err!=nil{return err}
 if err:=verifyAdvanceEnvelope(b,journal,state,next,initialState);err!=nil{return err}
 tx,err:=s.db.BeginTx(ctx,nil);if err!=nil{return err}
 defer tx.Rollback()
 var current []byte
 if err:=tx.QueryRowContext(ctx,"SELECT canonical FROM clock_states WHERE run_id=?",b.RunID[:]).Scan(&current);err!=nil{return err}
 if !bytes.Equal(current,b.ExpectedClock){return errors.New("clock revision changed before publication")}
  // A clock CAS alone does not prove the predecessor WorldRevision still
  // exists: publication cannot mint a revision with an orphaned parent.
  var predecessorHash []byte
  if err:=tx.QueryRowContext(ctx,"SELECT state_hash FROM world_revisions WHERE revision=?",b.ExpectedRevision).Scan(&predecessorHash);err!=nil{return fmt.Errorf("missing predecessor world revision: %w",err)}
  if !bytes.Equal(predecessorHash,b.BaseStateHash[:]){return errors.New("predecessor world revision hash mismatch")}

 derivedFence,err:=verifyCommittedFence(ctx,tx,b.RunID,b.CycleID,journal)
 if err!=nil{return err}
 if derivedFence!=b.FenceID{return errors.New("clock batch references a different admission fence")}
 var fence []byte
 if err:=tx.QueryRowContext(ctx,"SELECT canonical FROM decision_ledger WHERE id=?",b.FenceID[:]).Scan(&fence);err!=nil{return err}
 f,err:=codec.StrictDecodePayload(fence);if err!=nil{return err}
 ff:=f.([]any)
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
 // An omitted trigger is just as unsafe as a forged runtime. Compare the
 // complete registered definition set and re-evaluate each predicate from
 // the database's immutable definition and previous runtime.
 registered,err:=tx.QueryContext(ctx,"SELECT definition_id,canonical FROM trigger_definitions WHERE run_id=? ORDER BY definition_id",b.RunID[:])
 if err!=nil{return err}
 type defRow struct{id [32]byte;raw []byte}
 var definitions []defRow
 for registered.Next(){
  var id,raw []byte
  if err:=registered.Scan(&id,&raw);err!=nil{registered.Close();return err}
  if len(id)!=32{registered.Close();return errors.New("invalid registered trigger ID")}
  var entry defRow;copy(entry.id[:],id);entry.raw=raw
  definitions=append(definitions,entry)
 }
 if err:=registered.Err();err!=nil{registered.Close();return err}
 if err:=registered.Close();err!=nil{return err}
 if len(definitions)!=len(b.TriggerRuntime){return errors.New("CLOCK_ADVANCE omitted or invented registered trigger")}
 preEvents:=make([]trigger.Event,0,2)
 for _,event:=range b.Events[:2]{
  raw,err:=codec.StrictDecodePayload(event.Canonical);if err!=nil{return err}
  fields,ok:=raw.([]any);if !ok||len(fields)!=19{return errors.New("invalid pre-trigger event")}
  kind,ok:=fields[3].(string);if !ok{return errors.New("missing event kind")}
  deps,err:=trigger.EventFootprint(kind);if err!=nil{return err}
  key,err:=codec.CanonicalPayload(fields[2]);if err!=nil{return err}
  preEvents=append(preEvents,trigger.Event{ID:domain.ID(event.Identity),OrderKey:key,Footprint:deps})
 }
 expectedActivations:=make(map[[32]byte][]domain.ID)
 for i,change:=range b.TriggerRuntime {
  if change.DefinitionID!=definitions[i].id||!bytes.Equal(change.DefinitionCanonical,definitions[i].raw){
   return errors.New("trigger state change targets a different definition registry")
  }
  raw,err:=codec.StrictDecodePayload(change.DefinitionCanonical);if err!=nil{return err}
  fields,ok:=raw.([]any);if !ok||len(fields)!=8{return errors.New("invalid persisted definition shape")}
  handler,ok:=fields[7].([]any);if !ok{return errors.New("trigger definition handler missing")}
  definition,_,err:=trigger.DecodeDefinition(change.DefinitionCanonical,[][]any{handler},nil)
  if err!=nil{return err}
  before,err:=trigger.DecodeRuntime(change.Before);if err!=nil{return err}
  after,err:=trigger.Evaluate(definition,before,next.Instant,next.Revision,trigger.PredicateValue(definition,next.Instant),preEvents)
  if err!=nil{return err}
  expectedAfter,err:=trigger.RuntimeBytes(after.Runtime);if err!=nil{return err}
  if !bytes.Equal(expectedAfter,change.After){return errors.New("trigger runtime changed outside deterministic predicate")}
  if after.Fired{
   id,err:=registeredDigest("cote.csf.id.trigger-activation","cote.csf.schema.trigger-activation-id-preimage",
    []any{b.RunID[:],definition.ID[:],uint64(definition.Version),b.ResultRevision,after.Runtime.ActivationCount})
   if err!=nil{return err}
   expectedActivations[[32]byte(id)]=after.CausingEventIDs
  }
 }
 if len(expectedActivations)!=len(b.Activations){return errors.New("missing or invented trigger activation")}
 for _,change:=range b.TriggerRuntime {
  raw,err:=codec.StrictDecodePayload(change.After);if err!=nil{return err}
  runtime,ok:=raw.([]any)
  if !ok||len(runtime)!=8||!equalBytes(runtime[0],change.DefinitionID[:])||runtime[7]!=b.ResultRevision {
   return errors.New("trigger runtime authority/revision mismatch")
  }
  var prior []byte
  err=tx.QueryRowContext(ctx,"SELECT canonical FROM trigger_runtime WHERE definition_id=? AND run_id=?",change.DefinitionID[:],b.RunID[:]).Scan(&prior)
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
  causeIDs,ok:=origin[3].([]any)
  if !ok||len(causeIDs)>len(b.Events){return errors.New("invalid trigger causing event set")}
  previousIndex:=-1
  for _,cause:=range causeIDs {
   found:=-1
   for i,item:=range b.Events {if equalBytes(cause,item.Identity[:]){found=i;break}}
   if found<=previousIndex{return errors.New("trigger cause order/authority is invalid")}
   previousIndex=found
  }
  wantedCauses,ok:=expectedActivations[item.Identity]
  if !ok{return errors.New("activation not emitted by persisted trigger predicate")}
  if len(causeIDs)!=len(wantedCauses){return errors.New("activation omitted declared dependency causes")}
  for i,expected:=range wantedCauses{
   if !equalBytes(causeIDs[i],expected[:]){return errors.New("activation cause differs from registered footprint")}
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
 if len(b.Events)!=2+len(b.TriggerRuntime)+len(b.Activations){return errors.New("temporal batch omitted trigger lifecycle events")}
 for i,change:=range b.TriggerRuntime{
  delta,err:=codec.CanonicalPayload([]any{change.DefinitionID[:],change.Before,change.After});if err!=nil{return err}
  if err:=verifyLifecyclePayload(b.Events[2+i],3,change.DefinitionID,delta);err!=nil{return err}
 }
 orderedActivations:=append([]ports.CanonicalRecord(nil),b.Activations...)
 sort.Slice(orderedActivations,func(i,j int)bool{return bytes.Compare(orderedActivations[i].Identity[:],orderedActivations[j].Identity[:])<0})
 for i,activation:=range orderedActivations{
  if err:=verifyLifecyclePayload(b.Events[2+len(b.TriggerRuntime)+i],4,activation.Identity,activation.Canonical);err!=nil{return err}
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

// A lifecycle event must assert exactly the runtime delta or activation
// record that this one transaction is materializing; a valid self-hash alone
// does not prove the correct causal state transition.
func verifyLifecyclePayload(event ports.CanonicalRecord,phase uint64,origin [32]byte,payload []byte)error{
 decoded,err:=codec.StrictDecodePayload(event.Canonical);if err!=nil{return err}
 fields,ok:=decoded.([]any);if !ok||len(fields)!=19{return errors.New("lifecycle event malformed")}
 key,ok:=fields[2].([]any);if !ok||len(key)!=6||key[0]!=phase||!equalBytes(key[1],origin[:]){return errors.New("lifecycle event order key origin mismatch")}
 typed,ok:=fields[9].([]any);if !ok||len(typed)!=4{return errors.New("lifecycle event typed value malformed")}
 envBytes,ok:=typed[2].([]byte);if !ok{return errors.New("lifecycle event payload absent")}
 env,err:=codec.StrictDecodeEnvelope(envBytes);if err!=nil{return err}
 data,ok:=env.Value.([]any);if !ok||len(data)!=2||!equalBytes(data[0],origin[:])||!equalBytes(data[1],payload){return errors.New("lifecycle event payload diverges from committed trigger mutation")}
 return nil
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

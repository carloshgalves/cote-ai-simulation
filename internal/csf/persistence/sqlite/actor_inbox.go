package sqlite

import (
 "bytes"
 "context"
 "database/sql"
 "errors"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/clock"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
)

// ActorInbox is a recipient-scoped read capability. It does not expose the
// Store, EventStore, global EvidenceLedger, or a method to change the holder.
type ActorInbox struct{
 store *Store
 run [32]byte
 throughRevision uint64
 actor [2]string
}

// BindActorInbox mints a holder-only reader from an authenticated, durable
// SlotDispatch. The actor argument is verified against both the dispatch and
// its atomically sealed RoundDeclaration. A caller cannot freely select A
// when holding a different actor's dispatch.
func (s *Store) BindActorInbox(ctx context.Context,dispatchID [32]byte,actor [2]string)(
 *ActorInbox,error){
 if s==nil||actor[0]==""||actor[1]==""{return nil,errors.New("inbox requires valid actor/store")}
 var raw []byte
 if err:=s.db.QueryRowContext(ctx,"SELECT canonical FROM input_ledger WHERE id=?",
  dispatchID[:]).Scan(&raw);err!=nil{return nil,errors.New("inbox requires durable SlotDispatch")}
 value,err:=codec.StrictDecodePayload(raw);if err!=nil{return nil,err}
 f,ok:=value.([]any)
 if !ok||len(f)!=11||!equalBytes(f[0],dispatchID[:])||
  !sameCanonical(f[6],[]any{actor[0],actor[1]}){
  return nil,errors.New("inbox capability actor/dispatch mismatch")
 }
 name,ok:=f[4].(string)
 if !ok||name==""{return nil,errors.New("inbox dispatch lacks slot")}
 want,err:=registeredDigest("cote.csf.id.dispatch",
  "cote.csf.schema.dispatch-id-preimage",[]any{f[1],f[2],f[3],f[4],f[5]})
 if err!=nil||want!=dispatchID{return nil,errors.New("inbox dispatch is not an authenticated causal ID")}
 run,runOK:=idField(f[1]);round,roundOK:=idField(f[3])
 through,revisionOK:=f[8].(uint64)
 if !revisionOK{return nil,errors.New("inbox dispatch base revision malformed")}
 if !runOK||!roundOK{return nil,errors.New("inbox dispatch run/round invalid")}
 var roundRaw,seal []byte
 if err:=s.db.QueryRowContext(ctx,"SELECT canonical FROM input_ledger WHERE id=?",round[:]).Scan(&roundRaw);err!=nil{
  return nil,errors.New("inbox owner round absent")
 }
 if err:=s.db.QueryRowContext(ctx,
 "SELECT canonical_hash FROM round_integrity_seals WHERE id=? AND run_id=?",
 round[:],run[:]).Scan(&seal);err!=nil{return nil,errors.New("inbox round not sealed")}
 h:=identity.OpaqueContentHash(roundRaw)
 if !bytes.Equal(seal,h[:]){return nil,errors.New("inbox round owner seal changed")}
 rv,err:=codec.StrictDecodePayload(roundRaw);if err!=nil{return nil,err}
 roundFields,ok:=rv.([]any)
 if !ok||len(roundFields)!=11||!sameCanonical(roundFields[3],[]any{actor[0],actor[1]})||
  !sameCanonical(roundFields[5],f[7]){
  return nil,errors.New("inbox round owner/eligibility differs from dispatch")
 }
 slot,ok:=f[4].(string)
 if !ok{return nil,errors.New("inbox dispatch slot malformed")}
 slots,ok:=roundFields[4].([]any)
 allowed:=false
 if ok{for _,v:=range slots{if v==slot{allowed=true;break}}}
 if !allowed{return nil,errors.New("inbox slot is not declared by sealed round")}
 // Dispatch IDs deliberately exclude base revision and effective timestamp.
 // Do not let an altered record escalate its causal cutoff: authenticate
 // historic bindings with their committed cycle, and pending bindings with
 // the current clock/cycle coordinate. No global actor authority is granted.
 var journalRaw []byte
 err=s.db.QueryRowContext(ctx,
  "SELECT d.canonical FROM committed_cycles c JOIN decision_ledger d ON d.id=c.commit_id "+
  "WHERE c.run_id=? AND c.cycle_id=?",run[:],f[2]).Scan(&journalRaw)
 switch err{
 case nil:
  parsed,err:=codec.StrictDecodePayload(journalRaw)
  if err!=nil{return nil,err}
  journal,ok:=parsed.([]any)
  if !ok||len(journal)!=24||!equalBytes(journal[0],run[:])||
   !sameCanonical(journal[1],f[2])||
   !sameCanonical(journal[3],f[9])||journal[6]!=through{
   return nil,errors.New("historic inbox dispatch disagrees with committed cycle")
  }
 case sql.ErrNoRows:
  var clockRaw []byte
  if err:=s.db.QueryRowContext(ctx,
   "SELECT canonical FROM clock_states WHERE run_id=?",run[:]).Scan(&clockRaw);err!=nil{return nil,err}
  state,err:=clock.Decode(clockRaw)
  if err!=nil{return nil,err}
  effective,ok:=decodedInstant(f[9])
  if !ok||uint64(state.Revision)!=through||int64(state.Instant)!=effective||
   state.LastOrdinal< -1{
   return nil,errors.New("pending inbox dispatch disagrees with current clock")
  }
  next:=uint64(state.LastOrdinal+1)
  cycle,err:=registeredDigest("cote.csf.id.cycle",
   "cote.csf.schema.cycle-id-preimage",
   []any{run[:],[]any{int64(state.Instant),next}})
  if err!=nil||!equalBytes(f[2],cycle[:]){
   return nil,errors.New("pending inbox dispatch has unauthorized cycle")
  }
 default:return nil,err
 }
 return &ActorInbox{store:s,run:run,throughRevision:through,actor:actor},nil
}

// The actor parameter remains for the context-builder interface, but cannot
// authorize a holder different from the bound SlotDispatch capability.
func (i *ActorInbox) ReadAddressedEvidence(ctx context.Context,actor [2]string)(
 []ports.CanonicalRecord,[]ports.CanonicalRecord,error){
 if i==nil||i.store==nil||actor!=i.actor{
  return nil,nil,errors.New("recipient capability denies cross-actor evidence")
 }
 return i.store.readAddressedEvidence(ctx,i.run,i.actor,i.throughRevision)
}

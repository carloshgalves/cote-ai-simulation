package sqlite

import (
 "bytes"
 "context"
 "database/sql"
 "errors"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
)

// genesisScheduleDigest binds all initial occurrences, including future-due
// ones, to the first admission transaction. Generated successors do not
// enter this root: they are individually proven by a terminal DEFER decision.
func genesisScheduleDigest(ctx context.Context,q queryOne,run [32]byte)([32]byte,error){
 var empty [32]byte
 var genesisID,genesisDigest []byte
 if err:=q.QueryRowContext(ctx,"SELECT genesis_id,digest FROM genesis_records WHERE run_id=?",run[:]).Scan(&genesisID,&genesisDigest);err!=nil{return empty,err}
 rows,err:=q.QueryContext(ctx,"SELECT id,canonical FROM scheduled_occurrences WHERE run_id=? ORDER BY id",run[:])
 if err!=nil{return empty,err}
 roster:=[]any{}
 for rows.Next(){
  var id,raw []byte
  if err:=rows.Scan(&id,&raw);err!=nil{rows.Close();return empty,err}
  if len(id)!=32{rows.Close();return empty,errors.New("initial occurrence ID malformed")}
  var key [32]byte;copy(key[:],id)
  if _,_,err:=validateScheduledOccurrence(run,key,raw);err!=nil{rows.Close();return empty,err}
  v,err:=codec.StrictDecodePayload(raw);if err!=nil{rows.Close();return empty,err}
  fields:=v.([]any)
  origin,ok:=fields[9].([]any)
  if !ok||len(origin)!=2{rows.Close();return empty,errors.New("occurrence origin malformed")}
  tag,ok:=origin[0].(uint64);if !ok{rows.Close();return empty,errors.New("occurrence origin kind malformed")}
  switch tag{
  case 0: // terminal Event-owned successor, verified separately
  case 2:
   ref,ok:=origin[1].([]any)
   if !ok||len(ref)!=2||!equalBytes(ref[0],genesisID)||!equalBytes(ref[1],genesisDigest){
    rows.Close();return empty,errors.New("genesis schedule source references a different manifest")
   }
   roster=append(roster,[]any{id,fields[14]})
  default:
   rows.Close();return empty,errors.New("unregistered genesis schedule origin")
  }
 }
 if err:=rows.Err();err!=nil{rows.Close();return empty,err}
 if err:=rows.Close();err!=nil{return empty,err}
 payload,err:=codec.CanonicalPayload([]any{genesisID,genesisDigest,roster})
 if err!=nil{return empty,err}
 return [32]byte(identity.OpaqueContentHash(payload)),nil
}

func verifyGenesisScheduleSeal(ctx context.Context,q queryOne,run [32]byte)error{
 digest,err:=genesisScheduleDigest(ctx,q,run);if err!=nil{return err}
 var stored []byte
 err=q.QueryRowContext(ctx,"SELECT roster_digest FROM genesis_schedule_seals WHERE run_id=?",run[:]).Scan(&stored)
 if err==sql.ErrNoRows{
  // Before the first fence, genesis seeding is still an authorized bootstrap.
  var count int
  if err:=q.QueryRowContext(ctx,"SELECT count(*) FROM decision_ledger").Scan(&count);err!=nil{return err}
  if count!=0{return errors.New("genesis schedule authority missing first-admission seal")}
  return nil
 }
 if err!=nil{return err}
 if !bytes.Equal(stored,digest[:]){return errors.New("genesis scheduled roster differs from first-admission seal")}
 return nil
}

func ensureGenesisScheduleSeal(ctx context.Context,tx *sql.Tx,run [32]byte)error{
 digest,err:=genesisScheduleDigest(ctx,tx,run);if err!=nil{return err}
 var old []byte
 err=tx.QueryRowContext(ctx,"SELECT roster_digest FROM genesis_schedule_seals WHERE run_id=?",run[:]).Scan(&old)
 if err==nil{
  if !bytes.Equal(old,digest[:]){return errors.New("initial occurrence roster changed after seal")}
  return nil
 }
 if err!=sql.ErrNoRows{return err}
 var prior int
 if err:=tx.QueryRowContext(ctx,"SELECT count(*) FROM decision_ledger").Scan(&prior);err!=nil{return err}
 if prior!=0{return errors.New("cannot backfill genesis schedule seal after an admission")}
 if _,err:=tx.ExecContext(ctx,"INSERT INTO genesis_schedule_seals(run_id,roster_digest) VALUES(?,?)",run[:],digest[:]);err!=nil{return err}
 return nil
}


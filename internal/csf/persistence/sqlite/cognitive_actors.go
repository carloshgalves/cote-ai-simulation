package sqlite

import (
 "context"
 "database/sql"
 "errors"
 "fmt"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/clock"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

// RegisterCognitiveActor enrolls an actor whose memory cannot be regenerated
// by causal replay. A snapshot must then carry that actor's exact-byte,
// versioned epistemic checkpoint even if the actor has not yet received any
// ObservationRecord or KnowledgeInput. Registrations are append-only.
func (s *Store) RegisterCognitiveActor(ctx context.Context, run [32]byte, namespace,id string,b *policy.Bundle)error{
 if s==nil||b==nil{return errors.New("cognitive registration requires verified store and policy")}
 for _,v:=range []string{namespace,id}{
  if err:=domain.ValidateMachineID(v);err!=nil{return fmt.Errorf("cognitive actor identifier: %w",err)}
 }
 if _,err:=s.ValidatedGenesis(ctx,run,b);err!=nil{return err}
 tx,err:=s.db.BeginTx(ctx,nil);if err!=nil{return err}
 defer tx.Rollback()
 // Acquire the SQLite writer lock before checking snapshot publication.
 var raw []byte
 if err:=tx.QueryRowContext(ctx,"SELECT canonical FROM clock_states WHERE run_id=?",run[:]).Scan(&raw);err!=nil{return err}
 state,err:=clock.Decode(raw);if err!=nil{return err}
 if _,err:=tx.ExecContext(ctx,"UPDATE clock_states SET canonical=canonical WHERE run_id=?",run[:]);err!=nil{return err}
 var existing int
 if err:=tx.QueryRowContext(ctx,
  "SELECT count(*) FROM cognitive_actor_registry WHERE run_id=? AND actor_namespace=? AND actor_id=?",
  run[:],namespace,id).Scan(&existing);err!=nil{return err}
 if existing==1{return tx.Commit()}
 var published int
 if err:=tx.QueryRowContext(ctx,"SELECT count(*) FROM snapshot_seals WHERE run_id=? AND revision=?",
  run[:],uint64(state.Revision)).Scan(&published);err!=nil{return err}
 if published!=0{return errors.New("actor enrollment cannot modify a published same-revision snapshot")}
 if _,err:=tx.ExecContext(ctx,
  "INSERT INTO cognitive_actor_registry(run_id,actor_namespace,actor_id,registered_revision) VALUES(?,?,?,?)",
  run[:],namespace,id,uint64(state.Revision));err!=nil{return err}
 return tx.Commit()
}

// snapshotCognitiveActorRoster is derived exclusively from durable registry
// entries, never from a caller-supplied list or an agent checkpoint payload.
func snapshotCognitiveActorRoster(ctx context.Context,tx *sql.Tx,run [32]byte,id *[32]byte)(map[string]bool,error){
 query:="SELECT actor_namespace,actor_id FROM cognitive_actor_registry WHERE run_id=? ORDER BY actor_namespace,actor_id"
 args:=[]any{run[:]}
 if id!=nil{
  query="SELECT actor_namespace,actor_id FROM snapshot_cognitive_actors WHERE snapshot_id=? ORDER BY actor_namespace,actor_id"
  args=[]any{id[:]}
 }
 rows,err:=tx.QueryContext(ctx,query,args...);if err!=nil{return nil,err}
 defer rows.Close()
 members:=map[string]bool{}
 for rows.Next(){
  var ns,actor string
  if err:=rows.Scan(&ns,&actor);err!=nil{return nil,err}
  for _,v:=range []string{ns,actor}{if err:=domain.ValidateMachineID(v);err!=nil{return nil,err}}
  key:=ns+"\x00"+actor
  if members[key]{return nil,errors.New("duplicate cognitive actor registration")}
  members[key]=true
 }
 if err:=rows.Err();err!=nil{return nil,err}
 return members,nil
}

func sealSnapshotCognitiveActors(ctx context.Context,tx *sql.Tx,run,id [32]byte)error{
 rows,err:=tx.QueryContext(ctx,
 "SELECT actor_namespace,actor_id FROM cognitive_actor_registry WHERE run_id=? ORDER BY actor_namespace,actor_id",run[:])
 if err!=nil{return err}
 type actor struct{ns,id string}
 list:=[]actor{}
 for rows.Next(){
  var ns,who string
  if err:=rows.Scan(&ns,&who);err!=nil{rows.Close();return err}
  list=append(list,actor{ns,who})
 }
 if err:=rows.Err();err!=nil{rows.Close();return err}
 if err:=rows.Close();err!=nil{return err}
 for _,actor:=range list{
  if _,err:=tx.ExecContext(ctx,"INSERT INTO snapshot_cognitive_actors(snapshot_id,actor_namespace,actor_id) VALUES(?,?,?)",
   id[:],actor.ns,actor.id);err!=nil{return err}
 }
 return nil
}

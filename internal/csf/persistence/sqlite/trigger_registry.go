package sqlite

import (
 "bytes"
 "context"
 "database/sql"
 "errors"
 "fmt"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/clock"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/genesis"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/trigger"
)

type definitionEntry struct{definition trigger.Definition;raw []byte;digest domain.Digest}

// BootstrapGenesisTriggers registers exactly one immutable, ordered trigger
// registry per verified genesis run, in the same SQLite authority as clock.
// No per-cycle caller may add, amend or replace definitions after this seal.
func (s *Store) BootstrapGenesisTriggers(ctx context.Context,g *genesis.Genesis,bundle *policy.Bundle,defs []trigger.Definition)error{
 if g==nil||bundle==nil{return errors.New("trigger bootstrap requires genesis and verified bundle")}
 verified,err:=genesis.ValidateCanonical(g.CanonicalBytes,bundle)
 if err!=nil{return err}
 if verified.ID!=g.ID||verified.Digest!=g.Digest||verified.RunID!=g.RunID{return errors.New("trigger registry genesis mismatch")}
 refs,err:=bundle.GenesisPolicyRefs();if err!=nil{return err}
 roles,err:=bundle.RequiredPolicyRoles();if err!=nil{return err}
 if len(roles)!=len(refs){return errors.New("trigger role registry mismatch")}
 var handler []any
 for i,role:=range roles {
  if role=="dependency-footprints"{handler=refs[i];break}
 }
 if handler==nil{return errors.New("genesis lacks dependency-footprint policy")}
 canonical,err:=trigger.CanonicalDefinitions(defs);if err!=nil{return err}
 if len(canonical)>1024{return errors.New("too many initial triggers")}
 entries:=make([]definitionEntry,0,len(canonical))
 digestRefs:=make([]any,0,len(canonical))
 d:=identity.New(bundle)
 for _,def:=range canonical{
  if def.Version!=1{return errors.New("clock trigger predicate version unsupported")}
  if len(def.Dependencies)==0{return errors.New("clock trigger dependencies cannot be empty")}
  for _,dep:=range def.Dependencies{
   if dep!="clock"&&dep!="body"{return fmt.Errorf("unregistered trigger dependency %q",dep)}
  }
  raw,err:=trigger.DefinitionBytes(def,handler);if err!=nil{return err}
  _,_,err=trigger.DecodeDefinition(raw,refs,bundle);if err!=nil{return err}
  fields,err:=codec.StrictDecodePayload(raw);if err!=nil{return err}
  hash,_,err:=d.Sum("cote.csf.digest.trigger-definition","cote.csf.schema.trigger-definition",1,fields);if err!=nil{return err}
  entries=append(entries,definitionEntry{definition:def,raw:raw,digest:hash})
  digestRefs=append(digestRefs,[]any{def.ID[:],hash[:]})
 }
 sealCanonical,err:=codec.CanonicalPayload([]any{g.ID[:],g.Digest[:],digestRefs});if err!=nil{return err}
 sealDigest:=identity.OpaqueContentHash(sealCanonical)

 tx,err:=s.db.BeginTx(ctx,nil);if err!=nil{return err}
 defer tx.Rollback()
 var persisted []byte
 if err:=tx.QueryRowContext(ctx,"SELECT canonical FROM genesis_records WHERE run_id=?",g.RunID[:]).Scan(&persisted);err!=nil{return err}
 if !bytes.Equal(persisted,g.CanonicalBytes){return errors.New("genesis record differs from trigger authority")}
 var current []byte
 if err:=tx.QueryRowContext(ctx,"SELECT canonical FROM clock_states WHERE run_id=?",g.RunID[:]).Scan(&current);err!=nil{return err}
 initial,err:=clock.Decode(current);if err!=nil{return err}
 if initial.Revision!=0||initial.Instant!=g.InitialInstant{return errors.New("trigger bootstrap requires genesis revision")}
 var count int
 if err:=tx.QueryRowContext(ctx,"SELECT count(*) FROM decision_ledger").Scan(&count);err!=nil||count!=0{return errors.New("trigger registry cannot change after admission fence")}
 if err:=tx.QueryRowContext(ctx,"SELECT count(*) FROM committed_cycles WHERE run_id=?",g.RunID[:]).Scan(&count);err!=nil||count!=0{return errors.New("trigger registry cannot change after cycle")}
 var existing []byte
 err=tx.QueryRowContext(ctx,"SELECT registry_hash FROM trigger_registry_seals WHERE run_id=?",g.RunID[:]).Scan(&existing)
 if err==nil{
  if !bytes.Equal(existing,sealDigest[:]){return errors.New("initial trigger registry is immutable")}
  return tx.Commit()
 }
 if err!=sql.ErrNoRows{return err}
 for _,item:=range entries{
  def:=item.definition
  if _,err:=tx.ExecContext(ctx,"INSERT INTO trigger_definitions(definition_id,run_id,genesis_id,genesis_digest,content_digest,canonical) VALUES(?,?,?,?,?,?)",
   def.ID[:],g.RunID[:],g.ID[:],g.Digest[:],item.digest[:],item.raw);err!=nil{return err}
  start:=trigger.Runtime{DefinitionID:def.ID,Version:def.Version,Armed:true}
  evaluated,err:=trigger.Evaluate(def,start,g.InitialInstant,0,trigger.PredicateValue(def,g.InitialInstant),nil)
  if err!=nil{return err}
  encoded,err:=trigger.RuntimeBytes(evaluated.Runtime);if err!=nil{return err}
  if _,err:=tx.ExecContext(ctx,"INSERT INTO trigger_runtime(definition_id,run_id,canonical) VALUES(?,?,?)",def.ID[:],g.RunID[:],encoded);err!=nil{return err}
  if evaluated.Fired {
   activation,err:=genesisActivation(bundle,g,def,evaluated.Runtime)
   if err!=nil{return err}
   if _,err:=tx.ExecContext(ctx,"INSERT INTO trigger_activations(id,run_id,eligibility_instant,eligibility_ordinal,lifecycle,canonical) VALUES(?,?,?,?,0,?)",activation.id[:],g.RunID[:],int64(g.InitialInstant),0,activation.canonical);err!=nil{return err}
  }
 }
 if _,err:=tx.ExecContext(ctx,"INSERT INTO trigger_registry_seals(run_id,genesis_id,genesis_digest,registry_hash) VALUES(?,?,?,?)",g.RunID[:],g.ID[:],g.Digest[:],sealDigest[:]);err!=nil{return err}
 return tx.Commit()
}

type genesisUnit struct{id domain.ID;canonical []byte}
func genesisActivation(bundle *policy.Bundle,g *genesis.Genesis,def trigger.Definition,r trigger.Runtime)(genesisUnit,error){
 var unit genesisUnit
 d:=identity.New(bundle)
 id,_,err:=d.Sum("cote.csf.id.trigger-activation","cote.csf.schema.trigger-activation-id-preimage",1,[]any{g.RunID[:],def.ID[:],uint64(def.Version),uint64(0),r.ActivationCount})
 if err!=nil{return unit,err}
 pv:=trigger.PredicateValue(def,g.InitialInstant)
 payload,err:=codec.CanonicalBytes("cote.csf.trigger.predicate","cote.csf.schema.typed-value",1,pv);if err!=nil{return unit,err}
 hash:=identity.OpaqueContentHash(payload)
 pd,_,err:=d.Sum("cote.csf.digest.trigger-predicate-result","cote.csf.schema.typed-value",1,[]any{"cote.csf.schema.typed-value",uint64(1),payload,hash[:]})
 if err!=nil{return unit,err}
 sourceID:=fmt.Sprintf("trigger:%x",def.ID)
 body:=[]any{id[:],g.RunID[:],sourceID,def.ID[:],uint64(def.Version),[]any{uint64(1),[]any{g.ID[:],g.Digest[:]}},
  []any{int64(g.InitialInstant),uint64(0)},r.ActivationCount,pd[:],uint64(0),[]any{uint64(0)}}
 ud,_,err:=d.Sum("cote.csf.digest.trigger-activation-unit","cote.csf.schema.trigger-activation.body",1,body)
 if err!=nil{return unit,err}
 canonical,err:=codec.CanonicalPayload(append(body,ud[:]));if err!=nil{return unit,err}
 return genesisUnit{id:domain.ID(id),canonical:canonical},nil
}

// Reads are checked against the immutable registry seal, genesis reference,
// definition digest, runtime version and the fixed predicate/handler contract.
func (s *Store) LoadRegisteredTriggers(ctx context.Context,runID [32]byte,bundle *policy.Bundle)([]trigger.Registered,error){
 if bundle==nil{return nil,errors.New("bundle required")}
 genesisBytes,err:=s.LoadGenesis(ctx,runID);if err!=nil{return nil,err}
 g,err:=genesis.ValidateCanonical(genesisBytes,bundle);if err!=nil{return nil,err}
 if g.RunID!=domain.ID(runID){return nil,errors.New("registry genesis run mismatch")}
 refs,err:=bundle.GenesisPolicyRefs();if err!=nil{return nil,err}
 var boundID,boundDigest,root []byte
 if err:=s.db.QueryRowContext(ctx,"SELECT genesis_id,genesis_digest,registry_hash FROM trigger_registry_seals WHERE run_id=?",runID[:]).Scan(&boundID,&boundDigest,&root);err!=nil{return nil,err}
 if !bytes.Equal(boundID,g.ID[:])||!bytes.Equal(boundDigest,g.Digest[:]){return nil,errors.New("trigger registry bound to different genesis")}
 rows,err:=s.db.QueryContext(ctx,"SELECT definition_id,genesis_id,genesis_digest,content_digest,canonical FROM trigger_definitions WHERE run_id=? ORDER BY definition_id",runID[:])
 if err!=nil{return nil,err}
 var out []trigger.Registered
 digestRefs:=[]any{}
 d:=identity.New(bundle)
 for rows.Next(){
  var id,genesisID,genesisDigest,contentDigest,raw []byte
  if err:=rows.Scan(&id,&genesisID,&genesisDigest,&contentDigest,&raw);err!=nil{rows.Close();return nil,err}
  def,_,err:=trigger.DecodeDefinition(raw,refs,bundle);if err!=nil{rows.Close();return nil,err}
  if !bytes.Equal(def.ID[:],id)||!bytes.Equal(genesisID,g.ID[:])||!bytes.Equal(genesisDigest,g.Digest[:]){rows.Close();return nil,errors.New("trigger registry row unbound")}
  fields,err:=codec.StrictDecodePayload(raw);if err!=nil{rows.Close();return nil,err}
  hash,_,err:=d.Sum("cote.csf.digest.trigger-definition","cote.csf.schema.trigger-definition",1,fields)
  if err!=nil{rows.Close();return nil,err}
  if !bytes.Equal(hash[:],contentDigest){rows.Close();return nil,errors.New("trigger definition digest mismatch")}
  digestRefs=append(digestRefs,[]any{def.ID[:],hash[:]})
  out=append(out,trigger.Registered{Definition:def,DefinitionCanonical:raw,DefinitionDigest:hash})
 }
 if err:=rows.Err();err!=nil{rows.Close();return nil,err}
 if err:=rows.Close();err!=nil{return nil,err}
 canonical,err:=codec.CanonicalPayload([]any{g.ID[:],g.Digest[:],digestRefs});if err!=nil{return nil,err}
 wanted:=identity.OpaqueContentHash(canonical)
 if !bytes.Equal(root,wanted[:]){return nil,errors.New("immutable trigger registry root mismatch")}
 for i:=range out{
  var runtime []byte
  if err:=s.db.QueryRowContext(ctx,"SELECT canonical FROM trigger_runtime WHERE definition_id=? AND run_id=?",out[i].Definition.ID[:],runID[:]).Scan(&runtime);err!=nil{return nil,err}
  state,err:=trigger.DecodeRuntime(runtime);if err!=nil{return nil,err}
  if state.DefinitionID!=out[i].Definition.ID||state.Version!=out[i].Definition.Version{return nil,errors.New("trigger runtime diverged from immutable definition")}
  out[i].Runtime=state
  out[i].RuntimeCanonical=runtime
 }
 return out,nil
}

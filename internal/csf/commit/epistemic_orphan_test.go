package commit

import (
 "context"
 "database/sql"
 "testing"

 _ "modernc.org/sqlite"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/perception"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
)

// The journal-only replay is not enough: attackers can attach evidence
// to no committed task at all, so no per-task verification would visit it.
func TestCSFRV16ReplayAuditsEntireEvidenceRegistry(t *testing.T){
 for _,tc:=range []struct{
  name string
  mutate func(*testing.T,*sql.DB,perception.RoundProjection,[32]byte)
 }{
  {"orphan-canonical-observation",func(t *testing.T,db *sql.DB,p perception.RoundProjection,run [32]byte){
   v,err:=codec.StrictDecodePayload(p.Observation.Canonical);if err!=nil{t.Fatal(err)}
   fields:=append([]any(nil),v.([]any)...)
   var unknownTask [32]byte
   unknownTask[0]=0xed
   fields[1]=unknownTask[:]
   d:=func(domainTag,schema string,body any)[32]byte{
    t.Helper()
    encoded,err:=codec.CanonicalBytes(domainTag,schema,1,body)
    if err!=nil{t.Fatal(err)}
    return [32]byte(identity.OpaqueContentHash(encoded))
   }
   newID:=d("cote.csf.id.observation","cote.csf.schema.observation-id-preimage",
    []any{fields[1],fields[2],fields[3],fields[4]})
   fields[0]=newID[:]
   digest:=d("cote.csf.digest.observation","cote.csf.schema.observation.body",fields[:14])
   fields[14]=digest[:]
   raw,err:=codec.CanonicalPayload(fields);if err!=nil{t.Fatal(err)}
   if _,err:=db.Exec("INSERT INTO evidence_ledger(id,canonical) VALUES(?,?)",newID[:],raw);err!=nil{t.Fatal(err)}
  }},
  {"orphan-address-route",func(t *testing.T,db *sql.DB,p perception.RoundProjection,run [32]byte){
   var inventedRecord,inventedTask [32]byte
   inventedRecord[0],inventedTask[0]=0xfb,0xfc
   if _,err:=db.Exec("INSERT INTO evidence_routes(record_id,run_id,task_id,holder_namespace,holder_id,kind,canonical_hash) VALUES(?,?,?,?,?,?,?)",
    inventedRecord[:],run[:],inventedTask[:],"school","a",0,make([]byte,32));err!=nil{t.Fatal(err)}
  }},
  {"missing-address-route",func(t *testing.T,db *sql.DB,p perception.RoundProjection,run [32]byte){
   if _,err:=db.Exec("DELETE FROM evidence_routes WHERE record_id=?",p.Observation.Identity[:]);err!=nil{t.Fatal(err)}
  }},
 }{
  t.Run(tc.name,func(t *testing.T){
   ctx:=context.Background()
   c,store,ledger,cycle,fence,path:=singleResourceFixture(t)
   if _,err:=c.CommitSingleResource(ctx,ledger,cycle,fence);err!=nil{t.Fatal(err)}
   db,err:=sql.Open("sqlite","file:"+path);if err!=nil{t.Fatal(err)}
   defer db.Close()
   task:=findTerminalRoundTask(t,ctx,db)
   projection,err:=store.DeliverRoundPerception(ctx,task,c.bundle);if err!=nil{t.Fatal(err)}
   _,_,_,_,_,run:=ledger.Snapshot()
   tc.mutate(t,db,projection,[32]byte(run))
   reopened,err:=sqlite.Open(ctx,path);if err!=nil{t.Fatal(err)}
   defer reopened.Close()
   if _,err:=reopened.ReplayClock(ctx,[32]byte(run),c.bundle);err==nil{
    t.Fatal("replay accepted globally unattached or incomplete evidence")
   }
  })
 }
}

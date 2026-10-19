package commit

import (
 "bytes"
 "context"
 "database/sql"
 "testing"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/inputledger"
)

func review3DB(t *testing.T,path string)*sql.DB{
 t.Helper()
 db,err:=sql.Open("sqlite",path);if err!=nil{t.Fatal(err)}
 t.Cleanup(func(){db.Close()})
 return db
}

func TestReview3ReplayRejectsRehashedEmptyClaims(t *testing.T){
 cases:=[]struct{name string;mutate func([]any,[]any)}{
  {"empty-decision-digest",func(_ []any,j []any){j[17]=bytes.Repeat([]byte{0x41},32)}},
  {"empty-perception-task-digest",func(_ []any,j []any){j[19]=bytes.Repeat([]byte{0x42},32)}},
  {"empty-input-digest",func(f,j []any){f[12]=bytes.Repeat([]byte{0x43},32);j[12]=f[12]}},
  {"empty-cohort-identity",func(f,_ []any){f[10]=[]any{bytes.Repeat([]byte{0x44},32),[]any{}}}},
  {"empty-cohort-membership",func(f,_ []any){f[10]=[]any{f[10].([]any)[0],[]any{[]any{bytes.Repeat([]byte{0x55},32),[]any{}}}}}},
 }
 for _,tc:=range cases {
  t.Run(tc.name,func(t *testing.T){
   ctx:=context.Background()
   coordinator,store,ledger,run,bundle,path:=testFixture(t)
   out,err:=coordinator.Advance(ctx,ledger,run);if err!=nil{t.Fatal(err)}
   fAny,err:=codec.StrictDecodePayload(out.Fence.Canonical);if err!=nil{t.Fatal(err)}
   fence:=fAny.([]any)
   jAny,err:=codec.StrictDecodePayload(out.Commit.Canonical);if err!=nil{t.Fatal(err)}
   journal:=jAny.([]any)
   tc.mutate(fence,journal)
   d:=identity.New(bundle)
   fenceHash,_,err:=d.Sum("cote.csf.digest.fence","cote.csf.schema.admission-fence.body",1,fence[:17]);if err!=nil{t.Fatal(err)}
   fence[17]=fenceHash[:]
   journal[10]=fenceHash[:]
   eventBytes:=make([]any,len(out.Events))
   for i,event:=range out.Events{eventBytes[i]=event.Canonical}
   batchHash,_,err:=d.Sum("cote.csf.digest.batch","cote.csf.schema.batch-preimage",1,[]any{journal[:23],eventBytes});if err!=nil{t.Fatal(err)}
   journal[23]=batchHash[:]
   fRaw,err:=codec.CanonicalPayload(fence);if err!=nil{t.Fatal(err)}
   jRaw,err:=codec.CanonicalPayload(journal);if err!=nil{t.Fatal(err)}
   db:=review3DB(t,path)
   if _,err:=db.ExecContext(ctx,"UPDATE decision_ledger SET canonical=? WHERE id=?",fRaw,out.Fence.ID[:]);err!=nil{t.Fatal(err)}
   if _,err:=db.ExecContext(ctx,"UPDATE decision_ledger SET canonical=? WHERE id=?",jRaw,out.Commit.Identity[:]);err!=nil{t.Fatal(err)}
   if _,err:=store.ReplayClock(ctx,[32]byte(run),bundle);err==nil{t.Fatal("replay accepted a false empty-set commitment with consistent outer digests")}
  })
 }
}

func TestReview3ReplayRejectsMissingOrRetrodatedTarget(t *testing.T){
 for _,kind:=range []string{"deleted-genesis-target","rehashed-earlier-target"}{
  t.Run(kind,func(t *testing.T){
   ctx:=context.Background()
   coordinator,store,ledger,run,bundle,path:=testFixture(t)
   out,err:=coordinator.Advance(ctx,ledger,run);if err!=nil{t.Fatal(err)}
   db:=review3DB(t,path)
   var id,raw []byte
   if err:=db.QueryRowContext(ctx,"SELECT id,canonical FROM scheduled_occurrences WHERE run_id=?",run[:]).Scan(&id,&raw);err!=nil{t.Fatal(err)}
   if kind=="deleted-genesis-target"{
    if _,err:=db.ExecContext(ctx,"DELETE FROM scheduled_occurrences WHERE id=?",id);err!=nil{t.Fatal(err)}
   }else{
    value,err:=codec.StrictDecodePayload(raw);if err!=nil{t.Fatal(err)}
    fields:=value.([]any)
    q:=int64(out.Plan.To)-1
    fields[3]=q
    fields[4]=[]any{q,uint64(0)}
    digest,_,err:=identity.New(bundle).Sum("cote.csf.digest.occurrence-unit","cote.csf.schema.scheduled-occurrence.body",1,fields[:14]);if err!=nil{t.Fatal(err)}
    fields[14]=digest[:]
    rewritten,err:=codec.CanonicalPayload(fields);if err!=nil{t.Fatal(err)}
    if _,err:=db.ExecContext(ctx,"UPDATE scheduled_occurrences SET canonical=?,eligibility_instant=?,eligibility_ordinal=? WHERE id=?",rewritten,q,0,id);err!=nil{t.Fatal(err)}
   }
   if _,err:=store.ReplayClock(ctx,[32]byte(run),bundle);err==nil{t.Fatalf("replay accepted %s",kind)}
  })
 }
}

func TestReview3RejectsPreGenesisIngressAndRounds(t *testing.T){
 ctx:=context.Background()
 _,store,ledger,run,bundle,_:=testFixture(t)
 g:=validGenesis(t,bundle)
 before:=domain.EligibilityCoordinate{NotBeforeInstant:g.InitialInstant-1,NotBeforeCycleOrdinal:0}
 floor:=domain.EligibilityCoordinate{NotBeforeInstant:g.InitialInstant,NotBeforeCycleOrdinal:0}
 origin:=[]any{uint64(2),[]any{g.ID[:],g.Digest[:]}}
 if _,err:=ledger.DeclareRound(ctx,"schedule",[2]string{"school","actor-x"},[]string{"slot"},before,origin,0,987,0,[]any{uint64(0)});err==nil{t.Fatal("pre-genesis round inserted")}
 if _,err:=ledger.DeclareRound(ctx,"schedule",[2]string{"school","actor-x"},[]string{"slot"},floor,origin,0,987,0,[]any{uint64(0)});err!=nil{t.Fatalf("at-genesis round rejected: %v",err)}
 if err:=ledger.DeclareSource(inputledger.Source{ID:"retro-test"});err!=nil{t.Fatal(err)}
 sample:=inputledger.Input{ProducerUnitKey:"retro-input",Kind:"test.kind",SchemaVersion:1,
  Actor:[]any{uint64(0)},Payload:g.Payload[16],DeclaredEffectiveAt:g.InitialInstant-1,
  Eligibility:before,Provenance:[]any{uint64(0)},IdempotencyKey:[]any{uint64(0)}}
 if _,err:=ledger.AppendInput(ctx,"retro-test",sample);err==nil{t.Fatal("pre-genesis input inserted")}
 sample.Eligibility=floor
 if _,err:=ledger.AppendInput(ctx,"retro-test",sample);err!=nil{t.Fatalf("at-genesis input rejected: %v",err)}
 pending,err:=store.PendingClock(ctx,[32]byte(run));if err!=nil{t.Fatal(err)}
 if len(pending)<2{t.Fatalf("expected initial eligible units, found %d",len(pending))}
}

func TestReview3ReplayRejectsOrphanWorldRevision(t *testing.T){
 ctx:=context.Background()
 coordinator,store,ledger,run,bundle,path:=testFixture(t)
 if _,err:=coordinator.Advance(ctx,ledger,run);err!=nil{t.Fatal(err)}
 db:=review3DB(t,path)
 var hash []byte
 if err:=db.QueryRowContext(ctx,"SELECT state_hash FROM world_revisions WHERE revision=1").Scan(&hash);err!=nil{t.Fatal(err)}
 if _,err:=db.ExecContext(ctx,"INSERT INTO world_revisions(revision,state_hash,canonical) VALUES(?,?,?)",99,hash,[]byte{0x80});err!=nil{t.Fatal(err)}
 if _,err:=store.ReplayClock(ctx,[32]byte(run),bundle);err==nil{t.Fatal("replay accepted world revision without a causal commit")}
}

func TestReview3CommitRejectsMissingPredecessorRevision(t *testing.T){
 ctx:=context.Background()
 coordinator,store,ledger,run,_,path:=testFixture(t)
 db:=review3DB(t,path)
 if _,err:=db.ExecContext(ctx,"DELETE FROM world_revisions WHERE revision=0");err!=nil{t.Fatal(err)}
 if _,err:=coordinator.Advance(ctx,ledger,run);err==nil{t.Fatal("commit published revision without predecessor")}
 audit,err:=store.AuditClock(ctx,[32]byte(run));if err!=nil{t.Fatal(err)}
 if audit.CommitCount!=0||audit.RevisionCount!=0{t.Fatalf("partial commit after missing predecessor: %+v",audit)}
}

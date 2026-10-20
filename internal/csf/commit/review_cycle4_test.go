package commit

import (
 "bytes"
 "context"
 "testing"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/inputledger"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
)

func review4LateInput(t *testing.T,ctx context.Context,ledger *inputledger.Ledger,now domain.SimulationInstant) inputledger.StoredInput {
 t.Helper()
 got,err:=ledger.AppendInput(ctx,"driver",inputledger.Input{
  ProducerUnitKey:"late-review-four",Kind:"review.four",SchemaVersion:1,
  Actor:[]any{uint64(0)},Payload:[]any{},DeclaredEffectiveAt:now+50,
  Eligibility:domain.EligibilityCoordinate{NotBeforeInstant:now+50,NotBeforeCycleOrdinal:0},
  Provenance:[]any{uint64(0)},IdempotencyKey:[]any{uint64(0)},
 })
 if err!=nil{t.Fatal(err)}
 return got
}

func TestReview4GenericAppendCannotBypassSourceIngress(t *testing.T){
 ctx:=context.Background()
 coordinator,store,ledger,run,bundle,_:=testFixture(t)
 out,err:=coordinator.Advance(ctx,ledger,run);if err!=nil{t.Fatal(err)}
 valid:=review4LateInput(t,ctx,ledger,out.ResultClock.Instant)
 if err:=store.Append(ctx,ports.CanonicalRecord{Identity:valid.ID,Canonical:valid.Canonical});err==nil{
  t.Fatal("generic Append was allowed to handle ExogenousInput outside source ingress receipt authority")
 }
 // A distinct authenticated input at an already sealed coordinate must also
 // fail, not merely be rejected because of an identity collision.
 altered,err:=codec.StrictDecodePayload(valid.Canonical);if err!=nil{t.Fatal(err)}
 fields:=altered.([]any)
 fields[3]="retro-bypass"
 fields[9]=[]any{int64(out.Plan.To)-1,uint64(0)}
 d:=identity.New(bundle)
 newID,_,err:=d.Sum("cote.csf.id.exogenous-input","cote.csf.schema.exogenous-input-id-preimage",1,[]any{run[:],fields[2],fields[3]});if err!=nil{t.Fatal(err)}
 fields[0]=newID[:]
 sum,_,err:=d.Sum("cote.csf.digest.exogenous-input-unit","cote.csf.schema.exogenous-input.body",1,[]any{fields[:12]});if err!=nil{t.Fatal(err)}
 fields[12]=sum[:]
 raw,err:=codec.CanonicalPayload(fields);if err!=nil{t.Fatal(err)}
 if err:=store.Append(ctx,ports.CanonicalRecord{Identity:newID,Canonical:raw});err==nil{
  t.Fatal("generic Append persisted a retroactive input without a receipt or guard")
 }
 pending,err:=store.PendingClock(ctx,[32]byte(run));if err!=nil{t.Fatal(err)}
 for _,p:=range pending{if p.ID==domain.ID(newID){t.Fatal("rejected retroactive input was persisted")}}
 // Closure must not be creatable through the generic append API either.
 closing,err:=ledger.CloseSource(ctx,"driver",inputledger.Closure{ProducerKey:"late-close-review-four",ClosedThrough:domain.EligibilityCoordinate{NotBeforeInstant:out.ResultClock.Instant+100}})
 if err!=nil{t.Fatal(err)}
 if err:=store.Append(ctx,ports.CanonicalRecord{Identity:closing.ID,Canonical:closing.Canonical});err==nil{
  t.Fatal("generic Append was allowed to handle SourceClosure without receipt/cursor")
 }
}

func TestReview4ReplayRejectsUntouchedMinimumWithTamperedFutureInput(t *testing.T){
 ctx:=context.Background()
 coordinator,store,ledger,run,bundle,path:=testFixture(t)
 out,err:=coordinator.Advance(ctx,ledger,run);if err!=nil{t.Fatal(err)}
 future:=review4LateInput(t,ctx,ledger,out.ResultClock.Instant)
 db:=review3DB(t,path)
 v,err:=codec.StrictDecodePayload(future.Canonical);if err!=nil{t.Fatal(err)}
 fields:=v.([]any)
 fields[7]=[]any{"tampered.future.payload"}
 raw,err:=codec.CanonicalPayload(fields);if err!=nil{t.Fatal(err)}
 if _,err:=db.ExecContext(ctx,"UPDATE input_ledger SET canonical=? WHERE id=?",raw,future.ID[:]);err!=nil{t.Fatal(err)}
 if _,err:=store.ReplayClock(ctx,[32]byte(run),bundle);err==nil{
  t.Fatal("replay accepted future input with mismatching payload/digest while P remained unchanged")
 }
}

func TestReview4ReplayRejectsTamperedFutureRoundContent(t *testing.T){
 ctx:=context.Background()
 coordinator,store,ledger,run,bundle,path:=testFixture(t)
 g:=validGenesis(t,bundle)
 due:=g.InitialInstant+170
 origin:=[]any{uint64(2),[]any{g.ID[:],g.Digest[:]}}
 round,err:=ledger.DeclareRound(ctx,"driver",[2]string{"school","actor-one"},[]string{"first-slot"},
  domain.EligibilityCoordinate{NotBeforeInstant:due},origin,0,785,0,[]any{uint64(0)})
 if err!=nil{t.Fatal(err)}
 if _,err:=coordinator.Advance(ctx,ledger,run);err!=nil{t.Fatal(err)}
 db:=review3DB(t,path)
 decoded,err:=codec.StrictDecodePayload(round.Canonical);if err!=nil{t.Fatal(err)}
 fields:=decoded.([]any)
 fields[3]=[]any{"school","actor-forged"}
 fields[4]=[]any{"forged-slot"}
 raw,err:=codec.CanonicalPayload(fields);if err!=nil{t.Fatal(err)}
 if _,err:=db.ExecContext(ctx,"UPDATE input_ledger SET canonical=? WHERE id=?",raw,round.ID[:]);err!=nil{t.Fatal(err)}
 if _,err:=store.ReplayClock(ctx,[32]byte(run),bundle);err==nil{
  t.Fatal("replay accepted changed future round actor/slots without independent digest authority")
 }
}

func TestReview4ReplayRejectsForgedFirstAttemptRetryOrRules(t *testing.T){
 for _,tc:=range []struct{name string;mutate func([]any)}{
  {"retry-ref",func(f []any){f[3]=[]any{uint64(1),bytes.Repeat([]byte{0xa1},32)} }},
  {"rule-versions",func(f []any){f[16]=[]any{[]any{"forged.rule",uint64(999)}} }},
 }{
  t.Run(tc.name,func(t *testing.T){
   ctx:=context.Background()
   coordinator,store,ledger,run,bundle,path:=testFixture(t)
   out,err:=coordinator.Advance(ctx,ledger,run);if err!=nil{t.Fatal(err)}
   fv,err:=codec.StrictDecodePayload(out.Fence.Canonical);if err!=nil{t.Fatal(err)}
   f:=fv.([]any)
   jv,err:=codec.StrictDecodePayload(out.Commit.Canonical);if err!=nil{t.Fatal(err)}
   j:=jv.([]any)
   tc.mutate(f)
   d:=identity.New(bundle)
   fd,_,err:=d.Sum("cote.csf.digest.fence","cote.csf.schema.admission-fence.body",1,f[:17]);if err!=nil{t.Fatal(err)}
   f[17]=fd[:];j[10]=fd[:]
   eventBytes:=make([]any,len(out.Events));for i,e:=range out.Events{eventBytes[i]=e.Canonical}
   jd,_,err:=d.Sum("cote.csf.digest.batch","cote.csf.schema.batch-preimage",1,[]any{j[:23],eventBytes});if err!=nil{t.Fatal(err)}
   j[23]=jd[:]
   fenceBytes,err:=codec.CanonicalPayload(f);if err!=nil{t.Fatal(err)}
   journalBytes,err:=codec.CanonicalPayload(j);if err!=nil{t.Fatal(err)}
   db:=review3DB(t,path)
   if _,err:=db.ExecContext(ctx,"UPDATE decision_ledger SET canonical=? WHERE id=?",fenceBytes,out.Fence.ID[:]);err!=nil{t.Fatal(err)}
   if _,err:=db.ExecContext(ctx,"UPDATE decision_ledger SET canonical=? WHERE id=?",journalBytes,out.Commit.Identity[:]);err!=nil{t.Fatal(err)}
   if _,err:=store.ReplayClock(ctx,[32]byte(run),bundle);err==nil{
    t.Fatal("replay accepted a forged first-attempt fence with fully rehashed digests")
   }
  })
 }
}

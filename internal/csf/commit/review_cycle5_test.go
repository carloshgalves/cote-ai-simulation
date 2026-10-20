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
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

func review5Receipt(t *testing.T,source string,seq,kind uint64,id,digest []byte)ports.CanonicalRecord{
 t.Helper()
 raw,err:=codec.CanonicalBytes("cote.csf.record.source-ingress-receipt","cote.csf.schema.source-ingress-receipt",1,
  []any{source,seq,[]any{kind,id,digest}})
 if err!=nil{t.Fatal(err)}
 return ports.CanonicalRecord{Identity:[32]byte(identity.OpaqueContentHash(raw)),Canonical:raw}
}

func review5RekeyInput(t *testing.T,bundle *policy.Bundle,original inputledger.StoredInput,source,producer string,instant domain.SimulationInstant)(ports.CanonicalRecord,[]byte){
 t.Helper()
 v,err:=codec.StrictDecodePayload(original.Canonical);if err!=nil{t.Fatal(err)}
 f:=v.([]any)
 f[2]=source;f[3]=producer
 f[8]=int64(instant)
 f[9]=[]any{int64(instant),uint64(0)}
 d:=identity.New(bundle)
 id,_,err:=d.Sum("cote.csf.id.exogenous-input","cote.csf.schema.exogenous-input-id-preimage",1,[]any{f[1],f[2],f[3]})
 if err!=nil{t.Fatal(err)}
 f[0]=id[:]
 dig,_,err:=d.Sum("cote.csf.digest.exogenous-input-unit","cote.csf.schema.exogenous-input.body",1,[]any{f[:12]})
 if err!=nil{t.Fatal(err)}
 f[12]=dig[:]
 raw,err:=codec.CanonicalPayload(f);if err!=nil{t.Fatal(err)}
 return ports.CanonicalRecord{Identity:id,Canonical:raw},dig[:]
}

func review5Closure(t *testing.T,bundle *policy.Bundle,run domain.ID,source,producer string,coord domain.EligibilityCoordinate,final bool,seq uint64)(ports.CanonicalRecord,[]byte){
 t.Helper()
 d:=identity.New(bundle)
 id,_,err:=d.Sum("cote.csf.id.source-closure","cote.csf.schema.source-closure-id-preimage",1,[]any{run[:],source,producer})
 if err!=nil{t.Fatal(err)}
 fields:=[]any{id[:],run[:],source,producer,[]any{int64(coord.NotBeforeInstant),uint64(coord.NotBeforeCycleOrdinal)},final,seq}
 digest,_,err:=d.Sum("cote.csf.digest.source-closure","cote.csf.schema.source-closure",1,fields)
 if err!=nil{t.Fatal(err)}
 raw,err:=codec.CanonicalPayload(fields);if err!=nil{t.Fatal(err)}
 return ports.CanonicalRecord{Identity:id,Canonical:raw},digest[:]
}

func TestReview5RejectUndeclaredIngressAndWrongReceipt(t *testing.T){
 ctx:=context.Background()
 coordinator,store,ledger,run,bundle,_:=testFixture(t)
 outcome,err:=coordinator.Advance(ctx,ledger,run);if err!=nil{t.Fatal(err)}
 existing:=review4LateInput(t,ctx,ledger,outcome.ResultClock.Instant)
 ghost,digest:=review5RekeyInput(t,bundle,existing,"ghost","ghost-unit",outcome.ResultClock.Instant+77)
 ghostReceipt:=review5Receipt(t,"ghost",0,1,ghost.Identity[:],digest)
 if err:=store.AppendIngress(ctx,"ghost",0,ghost,ghostReceipt);err==nil{t.Fatal("accepted input from undeclared source")}
 if _,err:=store.NextIngressSequence(ctx,"ghost");err!=nil{t.Fatal(err)}
 good,digest:=review5RekeyInput(t,bundle,existing,"driver","receipt-forgery",outcome.ResultClock.Instant+79)
 seq,err:=store.NextIngressSequence(ctx,"driver");if err!=nil{t.Fatal(err)}
 wrongReceipt:=review5Receipt(t,"driver",seq,1,bytes.Repeat([]byte{0x9b},32),digest)
 if err:=store.AppendIngress(ctx,"driver",seq,good,wrongReceipt);err==nil{t.Fatal("accepted mismatched SourceIngressReceipt reference")}
 malformed:=ports.CanonicalRecord{Canonical:[]byte{0x80}}
 if err:=store.AppendIngress(ctx,"driver",seq,good,malformed);err==nil{t.Fatal("accepted malformed SourceIngressReceipt")}
 if seq2,err:=store.NextIngressSequence(ctx,"driver");err!=nil||seq2!=seq{t.Fatalf("failed ingress advanced cursor to %d: %v",seq2,err)}
 if _,err:=inputledger.Open(ctx,run,bundle,store);err!=nil{t.Fatal("failed ingress attempts polluted persisted ledger:",err)}
}

func TestReview5RejectAfterFinalAndRegressiveClosure(t *testing.T){
 ctx:=context.Background()
 coordinator,store,ledger,run,bundle,_:=testFixture(t)
 outcome,err:=coordinator.Advance(ctx,ledger,run);if err!=nil{t.Fatal(err)}
 previous:=review4LateInput(t,ctx,ledger,outcome.ResultClock.Instant)
 // First perform a genuine terminal closure.
 closedAt:=domain.EligibilityCoordinate{NotBeforeInstant:outcome.ResultClock.Instant+200}
 if _,err:=ledger.CloseSource(ctx,"driver",inputledger.Closure{ProducerKey:"review5-final",ClosedThrough:closedAt,Final:true});err!=nil{t.Fatal(err)}
 seq,err:=store.NextIngressSequence(ctx,"driver");if err!=nil{t.Fatal(err)}
 input,digest:=review5RekeyInput(t,bundle,previous,"driver","after-final",outcome.ResultClock.Instant+250)
 rec:=review5Receipt(t,"driver",seq,1,input.Identity[:],digest)
 if err:=store.AppendIngress(ctx,"driver",seq,input,rec);err==nil{t.Fatal("accepted input after final closure")}
 closure,closureDigest:=review5Closure(t,bundle,run,"driver","after-final-closure",
  domain.EligibilityCoordinate{NotBeforeInstant:outcome.ResultClock.Instant+300},false,seq)
 rec=review5Receipt(t,"driver",seq,2,closure.Identity[:],closureDigest)
 if err:=store.AppendIngress(ctx,"driver",seq,closure,rec);err==nil{t.Fatal("accepted closure after final closure")}
 if _,err:=store.ReplayClock(ctx,[32]byte(run),bundle);err!=nil{t.Fatalf("replay rejected valid final closure: %v",err)}
}

func TestReview5RejectRegressingClosureAndUnnormalizedInput(t *testing.T){
 ctx:=context.Background()
 coordinator,store,ledger,run,bundle,_:=testFixture(t)
 outcome,err:=coordinator.Advance(ctx,ledger,run);if err!=nil{t.Fatal(err)}
 prev:=review4LateInput(t,ctx,ledger,outcome.ResultClock.Instant)
 at:=domain.EligibilityCoordinate{NotBeforeInstant:outcome.ResultClock.Instant+200}
 if _,err:=ledger.CloseSource(ctx,"driver",inputledger.Closure{ProducerKey:"review5-progress",ClosedThrough:at});err!=nil{t.Fatal(err)}
 seq,err:=store.NextIngressSequence(ctx,"driver");if err!=nil{t.Fatal(err)}
 regressive,digest:=review5Closure(t,bundle,run,"driver","regressive-closure",domain.EligibilityCoordinate{NotBeforeInstant:outcome.ResultClock.Instant+150},false,seq)
 rec:=review5Receipt(t,"driver",seq,2,regressive.Identity[:],digest)
 if err:=store.AppendIngress(ctx,"driver",seq,regressive,rec);err==nil{t.Fatal("accepted regressive source closure")}
 input,digest:=review5RekeyInput(t,bundle,prev,"driver","not-normalized",outcome.ResultClock.Instant+175)
 rec=review5Receipt(t,"driver",seq,1,input.Identity[:],digest)
 if err:=store.AppendIngress(ctx,"driver",seq,input,rec);err==nil{t.Fatal("accepted input before already closed source coordinate")}
}

func TestReview5ReplayRejectsTamperedIngressCursorOrIndex(t *testing.T){
 for _,tc:=range []struct{name,query string}{
  {"cursor","UPDATE input_source_cursors SET next_seq=next_seq+1 WHERE source_id='driver'"},
  {"record-ref","UPDATE source_ingress_receipts SET record_id=zeroblob(32) WHERE source_id='driver' AND ingress_seq=0"},
  {"receipt-canonical","UPDATE source_ingress_receipts SET canonical=x'80' WHERE source_id='driver' AND ingress_seq=0"},
 }{
  t.Run(tc.name,func(t *testing.T){
   ctx:=context.Background()
   coordinator,store,ledger,run,bundle,path:=testFixture(t)
   if _,err:=coordinator.Advance(ctx,ledger,run);err!=nil{t.Fatal(err)}
   db:=review3DB(t,path)
   if _,err:=db.ExecContext(ctx,tc.query);err!=nil{t.Fatal(err)}
   if _,err:=store.ReplayClock(ctx,[32]byte(run),bundle);err==nil{t.Fatal("replay accepted a divergent ingress projection")}
   if _,err:=store.NextIngressSequence(ctx,"driver");err==nil{t.Fatal("writer accepted a divergent ingress projection")}
  })
 }
}

func review5ForceIngress(t *testing.T,ctx context.Context,path,source string,seq uint64,record,receipt ports.CanonicalRecord){
 t.Helper()
 db:=review3DB(t,path)
 tx,err:=db.BeginTx(ctx,nil);if err!=nil{t.Fatal(err)}
 defer tx.Rollback()
 for _,item:=range []ports.CanonicalRecord{record,receipt}{
  if _,err:=tx.ExecContext(ctx,"INSERT INTO input_ledger(id,canonical) VALUES(?,?)",item.Identity[:],item.Canonical);err!=nil{t.Fatal(err)}
 }
 if _,err:=tx.ExecContext(ctx,"INSERT INTO source_ingress_receipts(source_id,ingress_seq,record_id,canonical) VALUES(?,?,?,?)",source,seq,record.Identity[:],receipt.Canonical);err!=nil{t.Fatal(err)}
 if _,err:=tx.ExecContext(ctx,"UPDATE input_source_cursors SET next_seq=? WHERE source_id=?",seq+1,source);err!=nil{t.Fatal(err)}
 if err:=tx.Commit();err!=nil{t.Fatal(err)}
}

func TestReview5ReplayRejectsForgedPostFinalOrRegressiveHistory(t *testing.T){
 for _,kind:=range []string{"post-final","regressive-closure"}{
  t.Run(kind,func(t *testing.T){
   ctx:=context.Background()
   coordinator,store,ledger,run,bundle,path:=testFixture(t)
   outcome,err:=coordinator.Advance(ctx,ledger,run);if err!=nil{t.Fatal(err)}
   existing:=review4LateInput(t,ctx,ledger,outcome.ResultClock.Instant)
   terminal:=kind=="post-final"
   prior:=domain.EligibilityCoordinate{NotBeforeInstant:outcome.ResultClock.Instant+200}
   if _,err:=ledger.CloseSource(ctx,"driver",inputledger.Closure{ProducerKey:"review5-prefix",ClosedThrough:prior,Final:terminal});err!=nil{t.Fatal(err)}
   seq,err:=store.NextIngressSequence(ctx,"driver");if err!=nil{t.Fatal(err)}
   var record ports.CanonicalRecord
   var digest []byte
   var receiptKind uint64
   if terminal{
    record,digest=review5RekeyInput(t,bundle,existing,"driver","forged-post-final",outcome.ResultClock.Instant+250)
    receiptKind=1
   }else{
    record,digest=review5Closure(t,bundle,run,"driver","forged-regressive",
     domain.EligibilityCoordinate{NotBeforeInstant:outcome.ResultClock.Instant+150},false,seq)
    receiptKind=2
   }
   receipt:=review5Receipt(t,"driver",seq,receiptKind,record.Identity[:],digest)
   review5ForceIngress(t,ctx,path,"driver",seq,record,receipt)
   if _,err:=store.ReplayClock(ctx,[32]byte(run),bundle);err==nil{
    t.Fatalf("replay accepted %s source history with internally consistent receipts and SQL projections",kind)
   }
  })
 }
}

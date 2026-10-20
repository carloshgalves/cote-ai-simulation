package commit

import (
 "bytes"
 "context"
 "strings"
 "testing"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/inputledger"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
)

func review6Input(now domain.SimulationInstant) inputledger.Input {
 return inputledger.Input{
  ProducerUnitKey:"review-six-retry",Kind:"review.six",SchemaVersion:1,
  Actor:[]any{uint64(0)},Payload:[]any{},DeclaredEffectiveAt:now+70,
  Eligibility:domain.EligibilityCoordinate{NotBeforeInstant:now+70,NotBeforeCycleOrdinal:0},
  Provenance:[]any{uint64(0)},IdempotencyKey:[]any{uint64(0)},
 }
}

// Two correctly constructed ledgers can start before a producer publishes its
// input. The stale ledger must never manufacture a second ingress receipt for
// the same causal identity, regardless of whether its retry returns an error.
func TestReview6StaleLedgerRedeliveryDoesNotDuplicateReceipt(t *testing.T){
 ctx:=context.Background()
 coordinator,store,ledgerA,run,bundle,_:=testFixture(t)
 outcome,err:=coordinator.Advance(ctx,ledgerA,run);if err!=nil{t.Fatal(err)}
 ledgerB,err:=inputledger.Open(ctx,run,bundle,store);if err!=nil{t.Fatal(err)}
 input:=review6Input(outcome.ResultClock.Instant)
 first,err:=ledgerA.AppendInput(ctx,"driver",input);if err!=nil{t.Fatal(err)}
 before,err:=store.Records(ctx);if err!=nil{t.Fatal(err)}
 repeated,retryErr:=ledgerB.AppendInput(ctx,"driver",input)
 if retryErr==nil && (repeated.ID!=first.ID||repeated.Receipt.Sequence!=first.Receipt.Sequence||!bytes.Equal(repeated.Receipt.Canonical,first.Receipt.Canonical)){
  t.Fatal("successful redelivery reported a different ingress receipt")
 }
 after,err:=store.Records(ctx);if err!=nil{t.Fatal(err)}
 if len(after)!=len(before){t.Fatalf("redelivery changed canonical prefix from %d records to %d: error=%v",len(before),len(after),retryErr)}
 for i:=range before{
  if before[i].Identity!=after[i].Identity||!bytes.Equal(before[i].Canonical,after[i].Canonical){t.Fatalf("redelivery changed record %d",i)}
 }
 next,err:=store.NextIngressSequence(ctx,"driver");if err!=nil{t.Fatal(err)}
 if next!=first.Receipt.Sequence+1{t.Fatalf("redelivery consumed ingress sequence: got %d want %d",next,first.Receipt.Sequence+1)}
 restored,err:=inputledger.Open(ctx,run,bundle,store);if err!=nil{t.Fatalf("redelivery poisoned restore: %v",err)}
 _,inputs,_,_,_,_:=restored.Snapshot()
 occurrences:=0
 for _,item:=range inputs{if item.ID==first.ID{occurrences++}}
 if occurrences!=1{t.Fatalf("same causal input restored %d times",occurrences)}
 if _,err:=store.ReplayClock(ctx,[32]byte(run),bundle);err!=nil{t.Fatalf("redelivery poisoned replay: %v",err)}
}

func TestReview6SameIdentityDifferentBytesFailsWithoutNewReceipt(t *testing.T){
 ctx:=context.Background()
 coordinator,store,ledger,run,bundle,_:=testFixture(t)
 outcome,err:=coordinator.Advance(ctx,ledger,run);if err!=nil{t.Fatal(err)}
 first,err:=ledger.AppendInput(ctx,"driver",review6Input(outcome.ResultClock.Instant));if err!=nil{t.Fatal(err)}
 decoded,err:=codec.StrictDecodePayload(first.Canonical);if err!=nil{t.Fatal(err)}
 fields:=decoded.([]any)
 fields[7]=[]any{"mutated-payload"}
 dig,_,err:=identity.New(bundle).Sum("cote.csf.digest.exogenous-input-unit","cote.csf.schema.exogenous-input.body",1,[]any{fields[:12]})
 if err!=nil{t.Fatal(err)}
 fields[12]=dig[:]
 raw,err:=codec.CanonicalPayload(fields);if err!=nil{t.Fatal(err)}
 record:=ports.CanonicalRecord{Identity:[32]byte(first.ID),Canonical:raw}
 seq,err:=store.NextIngressSequence(ctx,"driver");if err!=nil{t.Fatal(err)}
 receipt:=review5Receipt(t,"driver",seq,1,first.ID[:],dig[:])
 before,err:=store.Records(ctx);if err!=nil{t.Fatal(err)}
 if err:=store.AppendIngress(ctx,"driver",seq,record,receipt);err==nil{t.Fatal("identity collision accepted")}
 after,err:=store.Records(ctx);if err!=nil{t.Fatal(err)}
 if len(before)!=len(after){t.Fatal("identity collision created an ingress receipt")}
 if _,err:=inputledger.Open(ctx,run,bundle,store);err!=nil{t.Fatal(err)}
}

func TestReview6DuplicateLegacyReceiptMustFailClosed(t *testing.T){
 ctx:=context.Background()
 coordinator,store,ledger,run,bundle,path:=testFixture(t)
 outcome,err:=coordinator.Advance(ctx,ledger,run);if err!=nil{t.Fatal(err)}
 first,err:=ledger.AppendInput(ctx,"driver",review6Input(outcome.ResultClock.Instant));if err!=nil{t.Fatal(err)}
 next,err:=store.NextIngressSequence(ctx,"driver");if err!=nil{t.Fatal(err)}
 duplicate:=review5Receipt(t,"driver",next,1,first.ID[:],first.Digest[:])
 db:=review3DB(t,path)
 tx,err:=db.BeginTx(ctx,nil);if err!=nil{t.Fatal(err)}
 defer tx.Rollback()
 if _,err:=tx.ExecContext(ctx,"INSERT INTO input_ledger(id,canonical) VALUES(?,?)",duplicate.Identity[:],duplicate.Canonical);err!=nil{t.Fatal(err)}
 if _,err:=tx.ExecContext(ctx,"INSERT INTO source_ingress_receipts(source_id,ingress_seq,record_id,canonical) VALUES(?,?,?,?)","driver",next,first.ID[:],duplicate.Canonical);err!=nil{t.Fatal(err)}
 if _,err:=tx.ExecContext(ctx,"UPDATE input_source_cursors SET next_seq=? WHERE source_id=?",next+1,"driver");err!=nil{t.Fatal(err)}
 if err:=tx.Commit();err!=nil{t.Fatal(err)}
 if _,err:=store.NextIngressSequence(ctx,"driver");err==nil{t.Fatal("projection accepted duplicate receipt for same record ID")}
 if _,err:=inputledger.Open(ctx,run,bundle,store);err==nil||!strings.Contains(err.Error(),"duplicate"){t.Fatalf("restore did not explicitly detect duplicate record receipt: %v",err)}
 if _,err:=store.ReplayClock(ctx,[32]byte(run),bundle);err==nil{t.Fatal("replay accepted duplicate receipt")}
}

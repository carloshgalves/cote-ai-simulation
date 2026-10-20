package reducer

import (
 "bytes"
 "errors"
 "testing"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
)

func TestStockReducerDeterminismAndConservation(t *testing.T){
 original,hash,err:=EncodeStock(Stock{Available:1});if err!=nil{t.Fatal(err)}
 parsed,err:=DecodeStock(original);if err!=nil||parsed.Available!=1{t.Fatal(parsed,err)}
 after,afterHash,err:=Consume(original,1);if err!=nil{t.Fatal(err)}
 got,err:=DecodeStock(after);if err!=nil||got.Available!=0{t.Fatal(got,err)}
 if hash==afterHash{t.Fatal("resource consumption did not change WorldState hash")}
 if original[3]==nil{t.Fatal("input state modified")}
 first,err:=codec.CanonicalPayload(after);if err!=nil{t.Fatal(err)}
 for i:=0;i<12;i++{
  replay,replayHash,err:=Consume(original,1);if err!=nil{t.Fatal(err)}
  same,err:=codec.CanonicalPayload(replay);if err!=nil{t.Fatal(err)}
  if replayHash!=afterHash||!bytes.Equal(first,same){t.Fatal("identical input produced nondeterministic resource state")}
 }
 if _,_,err:=Consume(after,1);!errors.Is(err,ErrInsufficientStock){t.Fatalf("reducer allowed negative resource stock: %v",err)}
 if _,_,err:=Consume(original,0);!errors.Is(err,ErrInvalidStock){t.Fatalf("zero consumption accepted: %v",err)}
}
func TestStockReducerRejectsForgedAndUnknownWorldState(t *testing.T){
 typed,_,err:=EncodeStock(Stock{Available:2});if err!=nil{t.Fatal(err)}
 corrupt:=append([]any(nil),typed...)
 corrupt[3]=make([]byte,32)
 if _,err:=DecodeStock(corrupt);!errors.Is(err,ErrInvalidStock){t.Fatalf("forged digest accepted: %v",err)}
 corrupt=append([]any(nil),typed...)
 corrupt[0]="cote.unregistered.schema"
 if _,err:=DecodeStock(corrupt);!errors.Is(err,ErrInvalidStock){t.Fatal(err)}
 for _,item:=range []any{
  []any{StockVariant,uint64(2),[]any{[]any{"inventory","last.resource",uint64(1)}}},
  []any{StockVariant,uint64(1),[]any{[]any{"inventory","other.resource",uint64(1)}}},
  []any{StockVariant,uint64(1),[]any{[]any{"inventory","last.resource",int64(-1)}}},
  []any{StockVariant,uint64(1),[]any{}},
  []any{StockVariant,uint64(1),[]any{[]any{"inventory","last.resource",uint64(1)},[]any{"inventory","last.resource",uint64(2)}}},
 }{
  if err:=ValidateStockPayload(item);!errors.Is(err,ErrInvalidStock){t.Fatalf("unregistered stock shape accepted: %#v",item)}
 }
}

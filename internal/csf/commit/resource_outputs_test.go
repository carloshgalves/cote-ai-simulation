package commit

import (
 "bytes"
 "testing"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/clock"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/decision"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/extensions/testresourcev1"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/reducer"
)

func TestResourceOutputDerivesOneWinnerThreeEventsAndExactStock(t *testing.T){
 bundle,err:=policy.Load(bundlePath);if err!=nil{t.Fatal(err)}
 req,err:=testresourcev1.LastUnit(bundle);if err!=nil{t.Fatal(err)}
 tentative,err:=decision.Run(req);if err!=nil{t.Fatal(err)}
 before,_,err:=reducer.EncodeStock(reducer.Stock{Available:1});if err!=nil{t.Fatal(err)}
 initial:=clock.State{Instant:100,Revision:7,NextLogicalSequence:11}
 out,err:=MaterializeResource(req,tentative,before,initial);if err!=nil{t.Fatal(err)}
 if len(out.Events)!=3{t.Fatalf("expected 1 resource EventDraft plus 2 lifecycle events, got %d",len(out.Events))}
 stock,err:=reducer.DecodeStock(out.ResultWorld);if err!=nil||stock.Available!=0{t.Fatal(stock,err)}
 for i,item:=range out.Events{
  v,err:=codec.StrictDecodePayload(item.Canonical);if err!=nil{t.Fatal(err)}
  fields:=v.([]any)
  key:=fields[2].([]any)
  expectedPhase:=uint64(2);if i==0{expectedPhase=1}
  if key[0]!=expectedPhase||fields[6]!=uint64(11+i)||fields[15]!=uint64(2){
   t.Fatalf("incorrect canonical event order or secret-only visibility: %d %v",i,fields)
  }
  if i==0&&fields[3]!="resource.consumed"{t.Fatal("winning event type is not resource.consumed")}
  if i>0&&fields[3]!="round.consumed"{t.Fatal("lifecycle event missing")}
 }
 swapped:=req
 swapped.Units=append([]decision.Unit(nil),req.Units...)
 swapped.Units[0],swapped.Units[1]=swapped.Units[1],swapped.Units[0]
 again,err:=decision.Run(swapped);if err!=nil{t.Fatal(err)}
 other,err:=MaterializeResource(swapped,again,before,initial);if err!=nil{t.Fatal(err)}
 if other.ResultHash!=out.ResultHash{t.Fatal("worker permutation altered resource state hash")}
 for i:=range out.Events{
  if out.Events[i].Identity!=other.Events[i].Identity||!bytes.Equal(out.Events[i].Canonical,other.Events[i].Canonical){
   t.Fatalf("worker permutation changed resource/round Event at %d",i)
  }
 }
}
func TestResourceOutputFailsClosedOnUnprovenSnapshot(t *testing.T){
 bundle,err:=policy.Load(bundlePath);if err!=nil{t.Fatal(err)}
 req,err:=testresourcev1.LastUnit(bundle);if err!=nil{t.Fatal(err)}
 tentative,err:=decision.Run(req);if err!=nil{t.Fatal(err)}
 wrong,_,err:=reducer.EncodeStock(reducer.Stock{Available:2});if err!=nil{t.Fatal(err)}
 initial:=clock.State{Instant:100,Revision:7,NextLogicalSequence:0}
 if _,err:=MaterializeResource(req,tentative,wrong,initial);err==nil{
  t.Fatal("resource resolver accepted a caller snapshot inconsistent with typed WorldState")
 }
 before,_,err:=reducer.EncodeStock(reducer.Stock{Available:1});if err!=nil{t.Fatal(err)}
 forged:=tentative
 forged.Canonical=[]byte{0x80}
 if _,err:=MaterializeResource(req,forged,before,initial);err==nil{
  t.Fatal("resource materialization accepted forged provisional result")
 }
}

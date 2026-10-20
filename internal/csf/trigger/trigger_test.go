package trigger
import (
 "bytes"
 "testing"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
)
func definition(p Policy) Definition {
 var id domain.ID;id[0]=2
 d:=Definition{ID:id,Version:1,Policy:p,Dependencies:[]string{"body","clock"}}
 if p==RepeatWhileTrue{d.RepeatEvery=10}
 return d
}
func runtime(d Definition) Runtime {return Runtime{DefinitionID:d.ID,Version:d.Version,Armed:true}}
func TestEdgeOnceAndRepeat(t *testing.T){
 now:=domain.SimulationInstant(100)
 d:=definition(RisingEdge);d.FireOnInitialTrue=true
 initial,err:=Evaluate(d,runtime(d),now,1,true,nil)
 if err!=nil||!initial.Fired||initial.Runtime.ActivationCount!=1{t.Fatal(initial,err)}
 second,err:=Evaluate(d,initial.Runtime,now,2,true,nil)
 if err!=nil||second.Fired{t.Fatal(second,err)}
 down,err:=Evaluate(d,second.Runtime,now,3,false,nil)
 if err!=nil||down.Fired{t.Fatal(down,err)}
 up,err:=Evaluate(d,down.Runtime,now,4,true,nil)
 if err!=nil||!up.Fired||up.Runtime.ActivationCount!=2{t.Fatal(up,err)}
 d=definition(FallingEdge)
 a,_:=Evaluate(d,runtime(d),now,1,true,nil)
 b,_:=Evaluate(d,a.Runtime,now,2,false,nil)
 if !b.Fired{t.Fatal("falling edge failed")}
 d=definition(OnceWhenTrue)
 a,_=Evaluate(d,runtime(d),now,1,true,nil)
 b,_=Evaluate(d,a.Runtime,now,2,true,nil)
 if !a.Fired||b.Fired||!b.Runtime.Exhausted{t.Fatal("once repeated")}
 d=definition(RepeatWhileTrue)
 a,_=Evaluate(d,runtime(d),now,1,true,nil)
 b,_=Evaluate(d,a.Runtime,now+9,2,true,nil)
 c,_:=Evaluate(d,b.Runtime,now+10,3,true,nil)
 if !a.Fired||b.Fired||!c.Fired||c.Runtime.NextRepeat!=120 {t.Fatal(a,b,c)}
}
func TestFootprintOrderDoesNotFollowWorkers(t *testing.T){
 d:=definition(RisingEdge);d.FireOnInitialTrue=true
 var a,b,c domain.ID;a[0],b[0],c[0]=1,2,3
 e:=[]Event{
  {ID:c,OrderKey:[]byte{3},Footprint:[]string{"body"}},
  {ID:a,OrderKey:[]byte{1},Footprint:[]string{"clock","other"}},
  {ID:b,OrderKey:[]byte{2},Footprint:[]string{"other"}},
 }
 got,err:=Evaluate(d,runtime(d),100,1,true,e)
 if err!=nil||!got.Fired||len(got.CausingEventIDs)!=2||got.CausingEventIDs[0]!=a||got.CausingEventIDs[1]!=c{t.Fatal(got,err)}
 for i:=0;i<6;i++{
  e[0],e[1]=e[1],e[0]
  check,err:=Evaluate(d,runtime(d),100,1,true,e)
  if err!=nil||!bytes.Equal(check.CausingEventIDs[0][:],got.CausingEventIDs[0][:])||check.CausingEventIDs[1]!=c {t.Fatal(check,err)}
 }
}
func TestRepeatCadenceRequiredAndRevisionMonotonic(t *testing.T){
 d:=definition(RepeatWhileTrue);d.RepeatEvery=0
 if Validate(d)==nil{t.Fatal("accepted missing cadence")}
 d=definition(RisingEdge);d.RepeatEvery=5
 if Validate(d)==nil{t.Fatal("accepted forbidden cadence")}
 d.RepeatEvery=0
 v,err:=Evaluate(d,runtime(d),1,1,true,nil)
 if err!=nil{t.Fatal(err)}
 if _,err:=Evaluate(d,v.Runtime,1,1,true,nil);err==nil{t.Fatal("double evaluated same revision")}
}

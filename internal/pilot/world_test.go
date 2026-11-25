package pilot

import (
 "context"
 "path/filepath"
 "reflect"
 "strings"
 "testing"
)
func worldJournal(t *testing.T)Journal{t.Helper();j:=scheduledJournal(t);j.Version=WorldVersion;j.WorldSeed=strings.Repeat("a",64);return j}
func TestWorldEconomyAndMonthlyPayout(t *testing.T){
 j:=worldJournal(t);s,err:=initial(j);if err!=nil{t.Fatal(err)}
 if len(s.World.Ledger)!=6||s.Minds["hirata"].Wallet!=100000{t.Fatal("initial issuance")}
 if !pay(&s,"hirata","kushida",1000,"transfer","test"){t.Fatal("transfer rejected")}
 if s.Minds["hirata"].Wallet!=99000||s.Minds["kushida"].Wallet!=101000{t.Fatal("transfer not zero sum")}
 if pay(&s,"hirata","school",100000,"ordinary","invalid"){t.Fatal("negative balance")}
 s.World.ClassPoints["D"]=950;s.Minute=30*1440+480;advanceWorld(j,&s)
 if s.Minds["hirata"].Wallet!=194000||s.Minds["kushida"].Wallet!=196000{t.Fatal("monthly income reset or wrong CP")}
 v:=actorContext(j,s,"hirata");if v.School.Wallet!=194000{t.Fatal("private balance unavailable")}
}
func TestWorldVIPFourOutcomesAndSecrecy(t *testing.T){
 for _,outcome:=range []int{1,2,3,4}{
  j:=worldJournal(t);s,_:=initial(j);s.Minute=1440+600;advanceWorld(j,&s)
  e:=s.World.Exam;vip:=e.VIP;other:=""
  for _,id:=range e.Group{if classOf(j,id)!=classOf(j,vip){other=id;break}}
  v:=actorContext(j,s,other)
  if v.School.Exam.IsVIP||v.School.Exam.Outcome!=0{t.Fatal("wrong private projection")}
  if outcome<=2{e.Phase="final"}
  guess:=vip;if outcome==2||outcome==4{for _,id:=range e.Group{if id!=vip&&id!=other{guess=id;break}}}
  settleGuesses(j,s,&s,[]Chosen{{Actor:other,Decision:Decision{Action:"guess",Target:guess}}})
  if e.Outcome!=outcome{t.Fatalf("outcome %d expected %d",e.Outcome,outcome)}
  if actorContext(j,s,other).School.Exam.Outcome!=0{t.Fatal("premature result")}
  n:=len(s.World.Ledger);settleExam(j,&s,outcome,other);if len(s.World.Ledger)!=n{t.Fatal("duplicate reward")}
  sum:=int64(0);for _,p:=range s.World.ClassPoints{sum+=p};if sum!=4000{t.Fatal("CP not zero sum")}
  switch outcome{case 1:if s.Minds[vip].Wallet!=1100000||s.Minds[other].Wallet!=600000{t.Fatal("wrong shared rewards")};case 2,4:if s.Minds[vip].Wallet!=600000{t.Fatal("wrong VIP reward")};case 3:if s.Minds[other].Wallet!=600000{t.Fatal("wrong early reward")}}
 }
}
func TestWorldDaysAdvanceAndReplay(t *testing.T){
 model:=modelFunc(func(_ context.Context,_ Context)(Decision,error){return wait(),nil})
 path:=filepath.Join(t.TempDir(),"v5.json")
 s,err:=OpenScheduled(path,"test",model);if err!=nil{t.Fatal(err)}
 for i:=0;i<20;i++{if _,err=s.Step(context.Background(),i);err!=nil{t.Fatal(err)}}
 before:=s.Snapshot();if before.State.Minute<=1440||before.State.World==nil||before.Done{t.Fatal("did not advance beyond first morning")}
 s.Close();s,err=OpenScheduled(path,"test",model);if err!=nil{t.Fatal(err)};defer s.Close()
 if !reflect.DeepEqual(before,s.Snapshot()){t.Fatal("world replay diverged")}
}
func TestWorldActionsAndDiscovery(t *testing.T){
 j:=worldJournal(t);s,_:=initial(j)
 m:=s.Minds["hirata"];m.Location="shop";s.Minds["hirata"]=m
 v:=actorContext(j,s,"hirata")
 d,err:=decodeLocalChoice(v,[]byte(`{"choice":"buy:snack","reason":"Comprar lanche"}`));if err!=nil{t.Fatal(err)}
 next,err:=scheduledAdvance(j,s,chooseFor(j,s,map[string]Decision{"hirata":d}));if err!=nil{t.Fatal(err)}
 if next.Minds["hirata"].Wallet!=99800||next.Minds["hirata"].Inventory["snack"]!=1{t.Fatal("purchase not executed")}
 if has(v.Locations,"dormitory-kushida"){t.Fatal("foreign room available")}
 e:=v.Memory[0];good:=Discovery{Text:"Há instalações disponíveis",Evidence:e.ID,Grounding:"a biblioteca"}
 if !validDiscovery(v,good){t.Fatal("grounded discovery rejected")}
 good.Grounding="invented quote";if validDiscovery(v,good){t.Fatal("invented evidence accepted")}
}

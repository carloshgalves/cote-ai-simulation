package pilot
import("testing")
func TestAnnualGateRejectsIncompleteRun(t *testing.T){
 j:=autonomyJournal(t);j.Version=FidelityVersion;initializeIdentities(&j);s,err:=initial(j);if err!=nil{t.Fatal(err)}
 a,err:=auditYear(j,s);if err!=nil{t.Fatal(err)}
 if !a.ReplayEqual||a.YearComplete||a.StructuralGate||a.TransitionAllowed{t.Fatal("premature annual acceptance")}
}

func TestV29AnnualAuditRequiresQualifiedGroups(t *testing.T){
 group:=[]string{"a","b","c","d","e"}
 slot:=ExamSlot{ID:"exam-01",Kind:"kokkuri",Groups:[][]string{group},Boundaries:[]int64{480,540,600,660,720,780,840}}
 good:=GroupResult{ID:"exam-01-g01",Group:clone(group),Outcome:1,Submitted:clone(group),Rounds:[]SchoolExamRound{{Number:1,At:540,Answer:"yes",Submitted:clone(group)}}}
 if !qualifiedV29Group(slot,0,good){t.Fatal("valid complete success rejected")}
 failed:=clone(good);failed.Outcome=2;failed.Rounds=nil
 for i:=1;i<=5;i++{failed.Rounds=append(failed.Rounds,SchoolExamRound{Number:i,At:slot.Boundaries[i],Answer:"no",Submitted:clone(group)})}
 if !qualifiedV29Group(slot,0,failed){t.Fatal("legitimate complete loss rejected")}
 for _,tc:=range []struct{name string;change func(*GroupResult)}{
  {"invalid round",func(g *GroupResult){g.Rounds[0].Answer="invalid"}},
  {"omitted ballot",func(g *GroupResult){g.Rounds[0].Submitted=g.Rounds[0].Submitted[:4]}},
  {"duplicate ballot",func(g *GroupResult){g.Rounds[0].Submitted[4]="a"}},
  {"wrong boundary",func(g *GroupResult){g.Rounds[0].At++}},
  {"wrong group",func(g *GroupResult){g.Group[4]="foreign"}},
  {"wrong group id",func(g *GroupResult){g.ID="exam-01-g02"}},
  {"missing result",func(g *GroupResult){g.Outcome=0}},
  {"premature loss",func(g *GroupResult){g.Outcome=2;g.Rounds[0].Answer="no"}},
 }{
  t.Run(tc.name,func(t *testing.T){g:=clone(good);tc.change(&g);if qualifiedV29Group(slot,0,g){t.Fatal("invalid result qualified")}})
 }
 slot.Kind="poll";poll:=GroupResult{ID:good.ID,Group:clone(group),Outcome:1,Submitted:clone(group),Poll:&PollState{Status:"resolved",Winner:"a",Loser:"b",Votes:map[string][]string{}}}
 for _,id:=range group{poll.Poll.Votes[id]=make([]string,7)}
 if !qualifiedV29Group(slot,0,poll){t.Fatal("resolved poll rejected")}
 poll.Poll.Status="incomplete";if qualifiedV29Group(slot,0,poll){t.Fatal("incomplete poll qualified")}
 slot.Kind="vip";vip:=GroupResult{ID:good.ID,Group:clone(group),Outcome:2}
 if !qualifiedV29Group(slot,0,vip){t.Fatal("legal Zodiac omission rejected")}
}

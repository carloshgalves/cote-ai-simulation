package pilot
import("reflect";"testing")
func TestInstitutionalReplayAndPerspective(t *testing.T){
 a,b:=newInstitutionalState(),newInstitutionalState();for day:=int64(1);day<=360;day++{advanceInstitutionalDay("seed",a,day);advanceInstitutionalDay("seed",b,day)}
 if !reflect.DeepEqual(a,b)||len(a.Requests)==0||len(a.Events)==0{t.Fatal("non-deterministic or inactive")}
 for _,actor:=range a.Actors{if actor.Budget<0||actor.Influence>20{t.Fatal("invalid budget")}}
 for id:=range a.Actors{_,qs,es,ok:=institutionalPerspective(a,id);if !ok{t.Fatal("missing actor")};for _,q:=range qs{if q.From!=id&&q.To!=id{t.Fatal("foreign request")}};for _,e:=range es{if !has(e.Audience,id){t.Fatal("foreign event")}}}
 before:=clone(a);advanceInstitutionalDay("seed",a,360);if !reflect.DeepEqual(a,before){t.Fatal("duplicate day changed state")}
}
func TestAnnualCalendarWholeGrades(t *testing.T){
 j:=autonomyJournal(t);j.Version=InstitutionalVersion;initializeIdentities(&j)
 slots:=generateExamCalendar(j);if len(slots)!=18||!reflect.DeepEqual(slots,generateExamCalendar(j)){t.Fatal("calendar")}
 years:=map[int]int{};last:=int64(0);for _,slot:=range slots{if slot.Start<=last{t.Fatal("overlap")};last=slot.Boundaries[len(slot.Boundaries)-1];years[slot.Years[0]]++;if len(slot.Group)!=160{t.Fatal("incomplete grade")};seen:=map[string]bool{};for _,g:=range slot.Groups{for _,id:=range g{if seen[id]||rosterYear(j,id)!=slot.Years[0]{t.Fatal("invalid participant")};seen[id]=true}};if len(seen)!=160{t.Fatal("bad partition")}}
 for y:=1;y<=3;y++{if years[y]!=6{t.Fatal("unbalanced grade")}}
}

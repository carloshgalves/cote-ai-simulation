package pilot
import "testing"
func TestSchoolExamCalendarV27(t *testing.T){
 j:=autonomyJournal(t);j.Version=SchoolExamVersion;initializeIdentities(&j)
 slots:=generateExamCalendar(j);if len(slots)!=2{t.Fatal("unqualified game scheduled")}
 for i,slot:=range slots{seen:=map[string]bool{};for _,g:=range slot.Groups{for _,id:=range g{if seen[id]{t.Fatal("duplicate member")};seen[id]=true}};if len(seen)!=160{t.Fatal("incomplete grade")};want:=32;if i==1{want=12};if len(slot.Groups)!=want{t.Fatal("incorrect group count")}}
}
func TestKokkuriMinorityAndDebt(t *testing.T){
 j:=Journal{Version:SchoolExamVersion};e:=&VIPExam{ID:"g",Kind:"kokkuri",ExamRound:1,Group:[]string{"a","b","c","d","e"},VirtualDebt:map[string]int64{"a":4000000,"b":4000000,"c":4000000,"d":4000000,"e":4000000},Submitted:map[string]bool{"a":true,"b":true,"c":true,"d":true,"e":true},Ballots:map[string]string{"a":"yes","b":"yes","c":"yes","d":"no","e":"no"}}
 s:=State{Minute:540};resolveSchoolKokkuri(j,&s,e)
 if e.ExamRound!=2||e.VirtualDebt["a"]!=2000000||e.RoundHistory[0].Answer!="no"{t.Fatal("minority/debt rule")}
 e.Submitted=map[string]bool{"a":true,"b":true,"c":true,"d":true,"e":true};e.Ballots=map[string]string{"a":"yes","b":"no","c":"no","d":"no","e":"no"};resolveSchoolKokkuri(j,&s,e)
 if e.Outcome!=1||e.VirtualDebt["a"]!=1600000||e.VirtualDebt["b"]!=3600000{t.Fatal("clear rule")}
}

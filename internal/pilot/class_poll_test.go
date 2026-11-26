package pilot
import("testing";"reflect")
func TestClassPollOptionsAndExtrema(t *testing.T){
 j:=autonomyJournal(t);j.Version=ClassPollVersion;initializeIdentities(&j);slot:=generateExamCalendar(j)[15];if slot.Kind!="poll"||len(slot.Groups)!=4{t.Fatal("missing class poll")}
 e:=&VIPExam{Group:clone(slot.Groups[0]),Phase:"early",Poll:&PollState{Votes:map[string][]string{}}};id:=e.Group[0]
 for i:=0;i<7;i++{opts:=pollOptions(j,State{},id,e);if len(opts)==0{t.Fatal("missing option",i)};e.Poll.Votes[id]=append(e.Poll.Votes[id],opts[0])}
 if len(pollOptions(j,State{},id,e))!=0{t.Fatal("accepted eighth vote")}
 top,bottom,zero:=pollExtremes([]string{"a","b","c"},map[string]int{"a":3,"b":-2,"c":0});if zero||!reflect.DeepEqual(top,[]string{"a"})||!reflect.DeepEqual(bottom,[]string{"b"}){t.Fatal("extrema")}
}
func TestPollTieRequiresCompleteVotes(t *testing.T){
 j:=Journal{WorldSeed:"seed"};e:=&VIPExam{ID:"poll",Group:[]string{"a","b","c"},Poll:&PollState{Status:"top",Deciding:map[string]string{"a":"a"}}}
 if _,ok:=pollChoose(j,e,[]string{"a","b"});ok{t.Fatal("accepted incomplete decisive vote")}
 e.Poll.Deciding=map[string]string{"a":"a","b":"b","c":"b"};if v,ok:=pollChoose(j,e,[]string{"a","b"});!ok||v!="b"{t.Fatal("wrong decisive result")}
}

package pilot
import (
 "reflect"
 "testing"
)
func TestGeneratedCalendarAndPrivateBallot(t *testing.T){
 j:=autonomyJournal(t);j.Version=CalendarVersion;if err:=admitExternalStudents(&j);err!=nil{t.Fatal(err)};initializeIdentities(&j)
 s,err:=initial(j);if err!=nil{t.Fatal(err)}
 if len(s.World.Calendar)!=18||!reflect.DeepEqual(s.World.Calendar,generateExamCalendar(j)){t.Fatal("invalid calendar")}
 seen:=map[string]bool{};for i,slot:=range s.World.Calendar{if i>0&&slot.Day<=s.World.Calendar[i-1].Day{t.Fatal("calendar overlap")};if slot.Kind=="vip"&&slot.Start<schoolDisclosure{t.Fatal("premature disclosure")};for _,id:=range slot.Group{seen[id]=true;if !isStudent(j,id)||j.Population[id].Year!=1{t.Fatal("invalid participant")}}}
 for _,id:=range ids(j.Profiles){if j.Population[id].Year==1&&!seen[id]{t.Fatal("never examined",id)}}
 slot:=s.World.Calendar[0];s.Minute=slot.Start;advanceWorld(j,&s);e:=s.World.Exam
 if e==nil||e.Kind!="minority"{t.Fatal("missing exam")}
 id:=e.Group[0];other:=e.Group[1];before:=s.Minds[id].Wallet
 recordGeneratedVotes(&s,[]Chosen{{Actor:id,Decision:Decision{Action:"vote",Target:"a"}},{Actor:other,Decision:Decision{Action:"vote",Target:"b"}},{Actor:e.Group[2],Decision:Decision{Action:"vote",Target:"b"}}})
 if len(e.Ballots)!=3||schoolView(j,s,other).Exam.Outcome!=0{t.Fatal("ballot integrity")}
 s.Minute=slot.Start+300;advanceWorld(j,&s);if s.Minds[id].Wallet!=before+25000||e.Outcome!=1{t.Fatal("minority settlement")}
 s.Minute=slot.Start+360;advanceWorld(j,&s);if len(s.World.ExamHistory)!=1{t.Fatal("missing result history")}
}

package pilot
import("testing";"reflect")
func TestWholeGradePartition(t *testing.T){
 j:=autonomyJournal(t);j.Version=WholeGradeVersion;if err:=admitExternalStudents(&j);err!=nil{t.Fatal(err)};initializeIdentities(&j)
 slots:=generateExamCalendar(j);if len(slots)!=18||!reflect.DeepEqual(slots,generateExamCalendar(j)){t.Fatal("calendar")}
 for _,slot:=range slots{seen:=map[string]bool{};for _,g:=range slot.Groups{for _,id:=range g{if seen[id]||rosterYear(j,id)!=slot.Years[0]{t.Fatal("invalid member",id)};seen[id]=true}};if len(seen)!=160||len(slot.Group)!=160{t.Fatal("incomplete grade",slot.ID,len(seen))}}
 s,err:=initial(j);if err!=nil{t.Fatal(err)};if len(s.World.GradePoints)!=12{t.Fatal("CP scope")}
 slot:=slots[0];s.Minute=slot.Start;advanceWorld(j,&s);if len(s.World.ExamGroups)!=len(slot.Groups){t.Fatal("missing groups")}
 for _,id:=range slot.Group{if examFor(s,id)==nil{t.Fatal("missing private group")}}
 s.Minute=slot.Start+300;advanceWorld(j,&s);s.Minute=slot.Start+360;advanceWorld(j,&s);if len(s.World.ExamHistory)!=1||len(s.World.ExamHistory[0].GroupResults)!=len(slot.Groups){t.Fatal("missing results")}
}

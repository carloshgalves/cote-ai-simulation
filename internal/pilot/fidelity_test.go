package pilot
import("testing";"strings")
func TestFidelityGuidanceScope(t *testing.T){
 j:=autonomyJournal(t);j.Version=FidelityVersion;if err:=admitExternalStudents(&j);err!=nil{t.Fatal(err)};initializeIdentities(&j);if err:=attachCorpus(&j);err!=nil{t.Fatal(err)}
 if len(j.PackInventory)<150{t.Fatal("incomplete corpus inventory")}
 s,err:=initial(j);if err!=nil{t.Fatal(err)}
 y:=actorContext(j,s,"tg-yuuichi-katagiri");if len(y.BehaviorGuidance)==0{t.Fatal("missing abstract guidance")}
 for _,g:=range actorContext(j,s,"horikita").BehaviorGuidance{if strings.Contains(g,"dissenter"){t.Fatal("foreign guidance")}}
 for _,g:=range y.BehaviorGuidance{if strings.Contains(g,"tg.case."){t.Fatal("case identity leaked")}}
}
func TestPracticeCountsElapsedNotGrades(t *testing.T){
 m:=Mind{Activity:&Activity{Kind:"task",Target:"study:"}}
 accountPractice(&m,540,570);accountPractice(&m,570,600)
 if m.PracticeMinutes["study/matemática"]!=60{t.Fatal("elapsed time not conserved")}
}

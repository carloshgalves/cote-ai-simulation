package pilot
import "testing"
func TestStaffCareNeedsAndShift(t *testing.T){
 j:=autonomyJournal(t);j.Version=StaffCareVersion;s,err:=initial(j);if err!=nil{t.Fatal(err)}
 for _,id:=range ids(j.Profiles){
  if isStudent(j,id){continue}
  m:=s.Minds[id];m.Body=&PilotBody{Energy:0,Hydration:0,Fatigue:1000};s.Minds[id]=m;s.Minute=600
  d:=routineDecision(j,s,id);if d.Action!="move"&&d.Action!="eat"{t.Fatal(id,d)};if err:=validateDecision(actorContext(j,s,id),d);err!=nil{t.Fatal(id,err)}
  m.Location="cafeteria";s.Minds[id]=m;d=routineDecision(j,s,id);if d.Action!="eat"{t.Fatal(id,d)}
  m.Body=&PilotBody{Energy:900,Hydration:900,Fatigue:300};m.Location="dormitory-"+id;s.Minds[id]=m;s.Minute=1320
  d=routineDecision(j,s,id);if d.Action!="sleep"{t.Fatal(id,d)};if err:=validateDecision(actorContext(j,s,id),d);err!=nil{t.Fatal(id,err)}
  s.Minute=600;m.Location=staffStation(j,id);s.Minds[id]=m;if routineDecision(j,s,id).Action!="work"{t.Fatal(id)}
 }
}

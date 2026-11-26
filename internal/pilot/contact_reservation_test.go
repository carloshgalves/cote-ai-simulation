package pilot
import "testing"
func TestContactReservationDefersOnlyRoutine(t *testing.T){
 j:=autonomyJournal(t);j.Version=ContactVersion;s,err:=initial(j);if err!=nil{t.Fatal(err)}
 s.Minute=970;for _,id:=range []string{"horikita","kushida"}{m:=s.Minds[id];m.Location="courtyard";m.Activity=nil;s.Minds[id]=m}
 a:=Chosen{Actor:"kushida",Decision:Decision{Action:"talk",Target:"horikita",Text:"Olá"}}
 b:=Chosen{Actor:"horikita",Decision:Decision{Action:"move",Target:"library",Source:routineSource}}
 out:=clone(s);got:=reserveRoutineContacts(j,s,&out,[]Chosen{a,b});if len(got)!=1||out.Minds["horikita"].Activity.Kind!="contact_hold"{t.Fatal("routine not deferred")}
 b.Decision.Source="model";out=clone(s);if len(reserveRoutineContacts(j,s,&out,[]Chosen{a,b}))!=2{t.Fatal("model move overridden")}
 b.Decision.Source=routineSource;b.Decision.Target="cafeteria";out=clone(s);if len(reserveRoutineContacts(j,s,&out,[]Chosen{a,b}))!=2{t.Fatal("meal travel overridden")}
}

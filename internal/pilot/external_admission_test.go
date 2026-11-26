package pilot
import (
 "reflect"
 "strings"
 "testing"
)
func TestExternalAdmissionDeterministicAndIsolated(t *testing.T){
 j:=autonomyJournal(t);j.Version=CrossoverVersion;k:=clone(j)
 if err:=admitExternalStudents(&j);err!=nil{t.Fatal(err)};if err:=admitExternalStudents(&k);err!=nil{t.Fatal(err)}
 if !reflect.DeepEqual(j.Profiles,k.Profiles)||len(j.Profiles)!=len(k.Profiles){t.Fatal("non-deterministic placement")}
 initializeIdentities(&j);s,err:=initial(j);if err!=nil{t.Fatal(err)}
 for _,e:=range externalAdmissions{id:="tg-"+e.Slug;if !known(j.Profiles,id)||j.Population[id].Role!="student"{t.Fatal(id)}}
 c:=actorContext(j,s,"ayanokoji");for _,f:=range c.Identity.Background{if strings.Contains(f.ID,"tg-"){t.Fatal("foreign biography")}}
 c=actorContext(j,s,"tg-yuuichi-katagiri");if len(c.Identity.Background)!=4{t.Fatal("pregame circle not preserved")}
}

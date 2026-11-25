package pilot
import (
 "strings"
 "testing"
)
func TestVeteranAdmissionHorizon(t *testing.T){
 j:=autonomyJournal(t);j.Version=VeteranVersion;initializeIdentities(&j)
 s,err:=initial(j);if err!=nil{t.Fatal(err)}
 for _,id:=range ids(j.Profiles){
  c:=actorContext(j,s,id)
  if j.Population[id].Year>1{
   found:=false;for _,f:=range c.Identity.Background{if strings.HasPrefix(f.ID,"bio-veteran-tenure-"){found=true}}
   if !found||!strings.Contains(strings.Join(c.School.Rules," "),classGuarantee){t.Fatal("veteran without prior experience",id)}
  }else{
   for _,f:=range c.Identity.Background{if strings.HasPrefix(f.ID,"bio-veteran-tenure-"){t.Fatal("foreign veteran biography",id)}}
  }
 }
}

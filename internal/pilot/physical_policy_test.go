package pilot
import "testing"
func TestPhysicalPolicyConstrainsOffers(t *testing.T){
 j:=autonomyJournal(t);j.Version=PhysicalVersion;initializeIdentities(&j);s,err:=initial(j);if err!=nil{t.Fatal(err)}
 for _,id:=range []string{"arisu-sakayanagi","ken-sudo"}{
  m:=s.Minds[id];m.Location="gym";m.Body=&PilotBody{Energy:900,Hydration:900,Fatigue:100};s.Minds[id]=m
  c:=actorContext(j,s,id);err:=validateDecision(c,Decision{Action:"exercise"})
  if (err==nil)!=(id=="ken-sudo"){t.Fatal("physical constraint not enforced",id,err)}
 }
}

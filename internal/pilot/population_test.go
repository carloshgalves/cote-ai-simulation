package pilot
import (
 "context"
 "encoding/json"
 "path/filepath"
 "strings"
 "testing"
)
func populationJournal(t *testing.T)Journal{
 t.Helper();j:=worldJournal(t);j.Version=PopulationVersion
 if err:=expandPopulation(&j);err!=nil{t.Fatal(err)}
 return j
}
func TestPopulationCountsAndRoles(t *testing.T){
 j:=populationJournal(t);s,err:=initial(j);if err!=nil{t.Fatal(err)}
 if len(j.Profiles)!=33{t.Fatalf("got %d profiles",len(j.Profiles))}
 n:=0;full:=0;teachers:=0
 for _,p:=range j.Profiles{a:=j.Population[p.ID];if a.Role=="student"&&a.Year==1{n++};if s.Minds[p.ID].Tier==FullTier{full++};if a.Role=="teacher"{teachers++;if s.Minds[p.ID].Wallet!=0{t.Fatal("teacher got student PP")}}}
 for _,c:=range s.Cohorts{n+=c.Count}
 if n!=160||full!=6||teachers!=4{t.Fatalf("counts %d %d %d",n,full,teachers)}
 v:=actorContext(j,s,"horikita")
 if has(v.Locations,"staffroom")||v.Population["sae-chiyabashira"].Role!="teacher"||sameYearPeer(v,"manabu-horikita"){t.Fatal("role or access boundary")}
 raw,_:=json.Marshal(v);if strings.Contains(string(raw),j.WorldSeed)||v.Population["kei-karuizawa"].Tier!=""{t.Fatal("operator metadata leaked")}
}
func TestPopulationRoutineAndPromotion(t *testing.T){
 j:=populationJournal(t);s,_:=initial(j)
 id:="kanji-ike"
 if needsModel(s,id)||s.Minds[id].Tier!=LatentTier{t.Fatal("latent invoked")}
 engage(j,&s,"hirata",id)
 if s.Minds[id].Tier!=LightTier{t.Fatal("contact did not promote")}
 engage(j,&s,"hirata",id);engage(j,&s,"hirata",id)
 if s.Minds[id].Tier!=FullTier||!needsModel(s,id){t.Fatal("repeated contact did not promote")}
 s.Minute+=2880;updatePopulation(j,&s)
 if s.Minds[id].Tier==FullTier||s.Minds["hirata"].Tier!=FullTier{t.Fatal("demotion failure")}
}
func TestPopulationSessionSixInitialModelCallsAndReplay(t *testing.T){
 calls:=0
 model:=modelFunc(func(_ context.Context,v Context)(Decision,error){calls++;return wait(),nil})
 path:=filepath.Join(t.TempDir(),"population.json")
 s,err:=OpenPopulation(path,"test",model);if err!=nil{t.Fatal(err)}
 out,err:=s.Step(context.Background(),0);if err!=nil{t.Fatal(err)}
 if calls!=6||out.Version!=PopulationVersion{t.Fatalf("unexpected calls/version %d/%d",calls,out.Version)}
 n:=0;for _,c:=range out.LastChoices{if c.Decision.Source==routineSource{n++}}
 if n!=27{t.Fatalf("expected 27 local choices, got %d",n)}
 s.Close();s,err=OpenPopulation(path,"test",model);if err!=nil{t.Fatal(err)};defer s.Close()
 if s.Snapshot().Digest!=out.Digest||calls!=6{t.Fatal("replay changed journal or invoked model")}
}
func TestPopulationExamExcludesStaffAndNonparticipants(t *testing.T){
 j:=populationJournal(t);s,_:=initial(j);s.Minute=1440+600;advanceWorld(j,&s)
 if len(s.World.Exam.Group)!=6||actorContext(j,s,"kei-karuizawa").School.Exam!=nil||actorContext(j,s,"sae-chiyabashira").School.Exam!=nil{t.Fatal("exam membership leaked")}
}

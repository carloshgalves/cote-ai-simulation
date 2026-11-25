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
 if n!=160||full!=33||teachers!=4{t.Fatalf("counts %d %d %d",n,full,teachers)}
 v:=actorContext(j,s,"horikita")
 if has(v.Locations,"staffroom")||v.Population["sae-chiyabashira"].Role!="teacher"||sameYearPeer(v,"manabu-horikita"){t.Fatal("role or access boundary")}
 raw,_:=json.Marshal(v);if strings.Contains(string(raw),j.WorldSeed)||v.Population["kei-karuizawa"].Tier!=""{t.Fatal("operator metadata leaked")}
}
func TestPopulationRoutineAndPromotion(t *testing.T){
 j:=populationJournal(t);s,_:=initial(j)
 id:="kanji-ike"
 // Exercise the optional lower-resolution policy without altering the default.
 member:=j.Population[id];member.InitialTier=LatentTier;j.Population[id]=member
 m:=s.Minds[id];m.Tier=LatentTier;s.Minds[id]=m
 if needsModel(s,id)||s.Minds[id].Tier!=LatentTier{t.Fatal("latent invoked")}
 engage(j,&s,"hirata",id)
 if s.Minds[id].Tier!=LightTier{t.Fatal("contact did not promote")}
 engage(j,&s,"hirata",id);engage(j,&s,"hirata",id)
 if s.Minds[id].Tier!=FullTier||!needsModel(s,id){t.Fatal("repeated contact did not promote")}
 s.Minute+=2880;updatePopulation(j,&s)
 if s.Minds[id].Tier==FullTier||s.Minds["hirata"].Tier!=FullTier{t.Fatal("demotion failure")}
}
func TestPopulationSessionAll33InitialModelCallsAndReplay(t *testing.T){
 calls:=0
 model:=modelFunc(func(_ context.Context,v Context)(Decision,error){calls++;return wait(),nil})
 path:=filepath.Join(t.TempDir(),"population.json")
 s,err:=OpenPopulation(path,"test",model);if err!=nil{t.Fatal(err)}
 out,err:=s.Step(context.Background(),0);if err!=nil{t.Fatal(err)}
 if calls!=33||out.Version!=PopulationVersion{t.Fatalf("unexpected calls/version %d/%d",calls,out.Version)}
 n:=0;for _,c:=range out.LastChoices{if c.Decision.Source==routineSource{n++}}
 if n!=0||len(out.LastChoices)!=33{t.Fatalf("expected 33 individual decisions, got %d decisions and %d local choices",len(out.LastChoices),n)}
 s.Close();s,err=OpenPopulation(path,"test",model);if err!=nil{t.Fatal(err)};defer s.Close()
 if s.Snapshot().Digest!=out.Digest||calls!=33{t.Fatal("replay changed journal or invoked model")}
}
func TestPopulationExamExcludesStaffAndNonparticipants(t *testing.T){
 j:=populationJournal(t);s,_:=initial(j);s.Minute=1440+600;advanceWorld(j,&s)
 if len(s.World.Exam.Group)!=6||actorContext(j,s,"kei-karuizawa").School.Exam!=nil||actorContext(j,s,"sae-chiyabashira").School.Exam!=nil{t.Fatal("exam membership leaked")}
}

func TestPopulationRoutinePreservesMindAndStaffContact(t *testing.T){
 j:=populationJournal(t);s,_:=initial(j)
 id:="sae-chiyabashira";m:=s.Minds[id];m.Activity=&Activity{Kind:"task",Target:"work:",Until:600};s.Minds[id]=m
 h:=s.Minds["horikita"];h.Location=m.Location;s.Minds["horikita"]=h
 if !has(actorContext(j,s,"horikita").Contactable,id){t.Fatal("staff cannot be contacted during service")}
 id="kanji-ike";m=s.Minds[id];m.Tier=LatentTier;m.Thought="Memória anterior";s.Minds[id]=m
 member:=j.Population[id];member.InitialTier=LatentTier;j.Population[id]=member
 choices:=[]Chosen{}
 for _,actor:=range eligibleActors(j,s){d:=wait();if !needsModel(s,actor){d=routineDecision(j,s,actor)};choices=append(choices,Chosen{Actor:actor,Decision:d})}
 next,err:=scheduledAdvance(j,s,choices);if err!=nil{t.Fatal(err)}
 if next.Minds[id].Thought!="Memória anterior"{t.Fatal("routine overwrote private cognition")}
}

func TestPopulationAllIndividualActorsRemainFullAndBusyActorsAreNotConsulted(t *testing.T){
 j:=populationJournal(t);s,err:=initial(j);if err!=nil{t.Fatal(err)}
 s.Minute+=3*1440;updatePopulation(j,&s)
 if len(eligibleActors(j,s))!=33{t.Fatal("not all actors initially eligible")}
 for _,id:=range ids(j.Profiles){
  m:=s.Minds[id]
  if m.Tier!=FullTier||!needsModel(s,id){t.Fatalf("%s lost FULL resolution",id)}
  m.Activity=&Activity{Kind:"wait",Until:s.Minute+60};s.Minds[id]=m
 }
 if len(eligibleActors(j,s))!=0{t.Fatal("busy FULL actors must not be consulted")}
 m:=s.Minds["kei-karuizawa"];m.Activity=nil;s.Minds["kei-karuizawa"]=m
 due:=eligibleActors(j,s)
 if len(due)!=1||due[0]!="kei-karuizawa"{t.Fatalf("unexpected eligible actors: %v",due)}
}

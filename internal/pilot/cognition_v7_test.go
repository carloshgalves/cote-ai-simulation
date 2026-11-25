package pilot

import (
 "context"
 "encoding/json"
 "path/filepath"
 "reflect"
 "strings"
 "testing"
)
func cognitionJournal(t *testing.T)Journal{
 t.Helper()
 j:=populationJournal(t);j.Version=CognitionVersion;initializeIdentities(&j);return j
}
func TestV7AdmissionKnowledgeAndChronology(t *testing.T){
 j:=cognitionJournal(t);s,err:=initial(j);if err!=nil{t.Fatal(err)}
 k:=actorContext(j,s,"kushida")
 if !has(k.Recognized,"horikita"){t.Fatal("Kushida did not recognize Horikita at admission")}
 if e,ok:=evidence(k,"bio-kushida-past");!ok||e.Kind!="biography"{t.Fatal("missing own history")}
 for _,id:=range []string{"horikita","ayanokoji","ryuen","manabu-horikita"}{
  v:=actorContext(j,s,id)
  if _,ok:=evidence(v,"bio-kushida-past");ok{t.Fatalf("private history leaked to %s",id)}
 }
 h:=actorContext(j,s,"horikita")
 if has(h.Goals,"Alcançar a Classe A")||strings.Contains(strings.Join(h.School.Rules," "),classGuarantee){t.Fatal("premature class competition")}
 for _,id:=range []string{"ayanokoji","kushida","horikita","arisu-sakayanagi"}{
  v:=actorContext(j,s,id)
  if strings.Contains(strings.Join(v.School.Rules," "),"CP × 100"){t.Fatal("premature CP formula")}
 }
 for _,id:=range []string{"manabu-horikita","sae-chiyabashira"}{v:=actorContext(j,s,id);if !strings.Contains(strings.Join(v.School.Rules," "),classGuarantee){t.Fatal("institutional knowledge absent")}}
 s.Minute=1440+600;advanceWorld(j,&s)
 if s.World.Exam!=nil{t.Fatal("premature exam")}
 s.Minute=schoolDisclosure;advanceWorld(j,&s)
 v:=actorContext(j,s,"horikita")
 if !strings.Contains(strings.Join(v.School.Rules," "),classGuarantee){t.Fatal("missing institutional revelation")}
 found:=false;for _,e:=range v.Memory{if e.Kind=="revelation"{found=true}};if !found{t.Fatal("disclosure not retained")}
 s.Minute=examBase(j.Version)+600;advanceWorld(j,&s)
 if s.World.Exam==nil{t.Fatal("post-disclosure exam missing")}
}
func TestV7BiographyAndMemoryIndividualization(t *testing.T){
 j:=cognitionJournal(t);s,_:=initial(j)
 for i:=0;i<45;i++{appendEvent(&s,Event{Kind:"claim",Actor:"hirata",Target:"ayanokoji",Text:"Experiência de teste",Audience:[]string{"ayanokoji","hirata"}})}
 a,h:=actorContext(j,s,"ayanokoji"),actorContext(j,s,"hirata")
 if a.Identity.Memory.WorkingMemory<=h.Identity.Memory.WorkingMemory||len(a.Memory)<=len(h.Memory){t.Fatal("memory profiles ineffective")}
 ar:=actorContext(j,s,"arisu-sakayanagi")
 if _,ok:=evidence(ar,"bio-arisu-kiyotaka");!ok{t.Fatal("Arisu prehistory lost")}
 if _,ok:=evidence(a,"bio-arisu-kiyotaka");ok{t.Fatal("asymmetric recognition leaked")}
 raw,_:=json.Marshal(a);if strings.Contains(string(raw),"bio-kushida-past"){t.Fatal("foreign memory visible")}
 for _,e:=range s.Events{if e.Kind=="biography"{t.Fatal("prehistory inserted into runtime event log")}}
}
func TestV7PlansCoexistAndExpire(t *testing.T){
 j:=cognitionJournal(t);s,_:=initial(j);id:="horikita"
 m:=s.Minds[id]
 applyHorizon(&m,PlanChange{Horizon:"long",Steps:[]string{"Conquistar reconhecimento"},Evidence:"bio-horikita-manabu"})
 applyHorizon(&m,PlanChange{Horizon:"immediate",Steps:[]string{"Estudar"},Evidence:"bio-horikita-manabu",Deadline:500})
 s.Minds[id]=m;s.Minute=500;expirePlans(j,&s)
 m=s.Minds[id]
 if len(m.Plans)!=1||m.Plans["long"].Horizon!="long"||m.Plan==nil||m.Plan.Horizon!="long"{t.Fatal("expiring immediate plan destroyed long plan")}
 s.Minute=examBase(j.Version)+600;advanceWorld(j,&s)
 m=s.Minds[id];applyHorizon(&m,PlanChange{Horizon:"short",Scope:"vip",Steps:[]string{"Proteger papel"}});s.Minds[id]=m
 settleExam(j,&s,2,"");expirePlans(j,&s)
 if _,ok:=s.Minds[id].Plans["short"];ok{t.Fatal("obsolete exam plan remained")}
}
func TestV7BodyChangesOnlyWithElapsedTime(t *testing.T){
 j:=cognitionJournal(t);s,_:=initial(j)
 id:="horikita";before:=clone(s.Minds[id].Body)
 advanceBodies(j,&s,s.Minute);if !reflect.DeepEqual(before,s.Minds[id].Body){t.Fatal("zero duration altered body")}
 m:=s.Minds[id];m.Activity=&Activity{Kind:"task",Target:"exercise:",Until:s.Minute+30};s.Minds[id]=m
 start:=s.Minute;s.Minute+=30;advanceBodies(j,&s,start)
 if s.Minds[id].Body.Fatigue<=before.Fatigue||s.Minds[id].Body.Energy>=before.Energy{t.Fatal("exercise did not consume energy")}
 m=s.Minds[id];m.Activity=&Activity{Kind:"task",Target:"eat:free",Until:s.Minute+20}
 old:=m.Body.Energy;completeTask(j,&s,id,&m,m.Activity);if m.Body.Energy<=old{t.Fatal("meal did not restore energy")}
}
func TestV7MeetingOverlapAndTrust(t *testing.T){
 j:=cognitionJournal(t);s,_:=initial(j);s.Minute=600
 s.World.Meetings=[]Meeting{{ID:"meeting-1",Host:"hirata",Guest:"kushida",Location:"hall",At:595,Status:"accepted"}}
 for _,id:=range []string{"hirata","kushida"}{
  m:=s.Minds[id];m.Location="hall";m.Activity=&Activity{Kind:"task",Target:"attend:meeting-1",Until:600};s.Minds[id]=m
  completeTask(j,&s,id,&m,m.Activity);s.Minds[id]=m
 }
 if s.World.Meetings[0].Status!="met"||s.Minds["hirata"].Social["kushida"].Fulfilled!=1{t.Fatal("overlap not recognized")}
 m:=s.Minds["hirata"];m.Location="library";s.Minds["hirata"]=m;s.Minute=610;advanceWorld(j,&s)
 if s.World.Meetings[0].Status!="met"{t.Fatal("later departure invalidated meeting")}
}
func TestV7AdvanceNeverInvokesModelAndReplay(t *testing.T){
 calls:=0
 model:=modelFunc(func(_ context.Context,_ Context)(Decision,error){calls++;return Decision{Action:"wait"},nil})
 path:=filepath.Join(t.TempDir(),"v7.json");s,err:=OpenCognition(path,"test",model);if err!=nil{t.Fatal(err)}
 before,err:=s.AdvanceUntilEvent(context.Background(),0);if err!=nil||before.Cycles!=0||calls!=0{t.Fatal("skip invoked model",err)}
 first,err:=s.Step(context.Background(),0);if err!=nil{t.Fatal(err)}
 if calls!=14{t.Fatalf("initial calls %d",calls)}
 n:=calls
 next,err:=s.AdvanceUntilEvent(context.Background(),first.State.Round);if err!=nil{t.Fatal(err)}
 if calls!=n{t.Fatal("advance invoked model")}
 if next.Cycles<1||next.Snapshot.State.Round<=first.State.Round{t.Fatal("no deterministic cycle advanced")}
 digest:=next.Snapshot.Digest;s.Close()
 s,err=OpenCognition(path,"test",model);if err!=nil{t.Fatal(err)};defer s.Close()
 if s.Snapshot().Digest!=digest||calls!=n{t.Fatal("replay changed state or invoked model")}
}

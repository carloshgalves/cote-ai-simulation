package pilot

import (
 "context"
 "path/filepath"
 "reflect"
 "testing"
)

func autonomyJournal(t *testing.T) Journal {
 t.Helper()
 j:=selectiveJournal(t);j.Version=AutonomyVersion
 return j
}
func TestAutonomyPriorIsIndividualNotTier(t *testing.T){
 j:=autonomyJournal(t);s,err:=initial(j);if err!=nil{t.Fatal(err)}
 if personalPrior(j,s,"horikita").leisure!="read"||personalPrior(j,s,"kushida").leisure!="socialize"||personalPrior(j,s,"ken-sudo").leisure!="exercise"{t.Fatal("dispositions ignored")}
 before:=personalPrior(j,s,"horikita")
 m:=s.Minds["horikita"];m.Tier=LatentTier;s.Minds["horikita"]=m
 if personalPrior(j,s,"horikita")!=before{t.Fatal("tier altered personal routine")}
}
func TestAutonomyNeedsAndNoRepeatedMeal(t *testing.T){
 j:=autonomyJournal(t);s,err:=initial(j);if err!=nil{t.Fatal(err)}
 id:="horikita";s.Minute=740
 m:=s.Minds[id];m.Location="cafeteria";m.Body.Energy=900;m.Body.Hydration=900;m.Body.Fatigue=100;s.Minds[id]=m
 if d:=routineDecision(j,s,id);d.Action=="eat"{t.Fatal("satiated actor eats repeatedly")}
 m.Body.Hydration=100;m.Inventory["water"]=1;s.Minds[id]=m
 if d:=routineDecision(j,s,id);d.Action!="use"||d.Target!="water"{t.Fatal("need did not use available water",d)}
 delete(m.Inventory,"water");s.Minds[id]=m
 if d:=routineDecision(j,s,id);d.Action!="eat"{t.Fatal("hydration need ignored",d)}
}
func TestAutonomyInitiativeAndCommitment(t *testing.T){
 j:=autonomyJournal(t);s,err:=initial(j);if err!=nil{t.Fatal(err)}
 id:="kushida";s.Minute=970
 m:=s.Minds[id];m.PendingStimulus="";m.Activity=nil;m.Location="hall";s.Minds[id]=m
 if !decisionNeedsModel(j,s,id){t.Fatal("goal-driven initiative absent")}
 m.LastLightInitiativeDay=1;s.Minds[id]=m
 if decisionNeedsModel(j,s,id){t.Fatal("daily initiative repeated")}
 s.World.Meetings=[]Meeting{{ID:"meeting-1",Host:id,Guest:"hirata",Status:"accepted",Location:"library",At:974}}
 d:=routineDecision(j,s,id)
 if d.Action!="move"||d.Target!="library"{t.Fatal("accepted appointment ignored",d)}
}
func TestAutonomySharedLeisurePrivacyAndCompletion(t *testing.T){
 j:=autonomyJournal(t);s,err:=initial(j);if err!=nil{t.Fatal(err)}
 s.Minute=980
 for _,id:=range []string{"hirata","kushida","ryuen"}{
  m:=s.Minds[id];m.Location="courtyard";m.Activity=&Activity{Kind:"task",Target:"socialize:",Until:980};if id=="ryuen"{m.Activity.Until=990};s.Minds[id]=m
 }
 completeSharedLeisure(j,&s)
 a:=s.Minds["hirata"].Social["kushida"];b:=s.Minds["kushida"].Social["hirata"]
 if a.SharedLeisureDays!=1||b.SharedLeisureDays!=1||a.Interactions!=0||b.Interactions!=0||a.Trust!=0||b.Trust!=0{t.Fatal("invalid social effects")}
 if s.Minds["ryuen"].Social["hirata"].SharedLeisureDays!=0{t.Fatal("uncompleted task counted")}
 count:=len(s.Events)
 completeSharedLeisure(j,&s)
 if len(s.Events)!=count||s.Minds["hirata"].Social["kushida"].SharedLeisureDays!=1{t.Fatal("duplicated shared leisure")}
 for _,e:=range visible(s,"ryuen"){if e.Kind=="social"{t.Fatal("private encounter leaked")}}
}
func TestAutonomyNewAndLegacyJournalsReplay(t *testing.T){
 calls:=0
 model:=modelFunc(func(_ context.Context,_ Context)(Decision,error){calls++;return Decision{Action:"wait"},nil})
 path:=filepath.Join(t.TempDir(),"v9.json")
 session,err:=OpenAutonomy(path,"test",model);if err!=nil{t.Fatal(err)}
 for i:=0;i<8;i++{if _,err:=session.Step(context.Background(),i);err!=nil{session.Close();t.Fatal(err)}}
 before:=session.Snapshot();n:=calls;session.Close()
 session,err=OpenAutonomy(path,"test",model);if err!=nil{t.Fatal(err)}
 defer session.Close()
 if before.Version!=AutonomyVersion||!reflect.DeepEqual(before,session.Snapshot())||calls!=n{t.Fatal("replay diverged or invoked model")}
 legacyPath:=filepath.Join(t.TempDir(),"v8.json")
 old,err:=OpenSelective(legacyPath,"test",model);if err!=nil{t.Fatal(err)}
 prior:=old.Snapshot();old.Close()
 old,err=OpenAutonomy(legacyPath,"test",model);if err!=nil{t.Fatal(err)};defer old.Close()
 if old.Snapshot().Version!=SelectiveVersion||!reflect.DeepEqual(prior,old.Snapshot()){t.Fatal("legacy save migrated")}
}


func TestAutonomyLegacyStaffAccounting(t *testing.T){
 j:=populationJournal(t)
 s,err:=initial(j);if err!=nil{t.Fatal(err)}
 s.Minute=900
 id:="sae-chiyabashira"
 m:=s.Minds[id];m.Tier=LightTier;m.Activity=nil;s.Minds[id]=m
 if !decisionNeedsModel(j,s,id){t.Fatal("legacy staff opportunity missing")}
 choices:=[]Chosen{}
 for _,actor:=range eligibleActors(j,s){
  d:=routineDecision(j,s,actor)
  if decisionNeedsModel(j,s,actor){d=Decision{Action:"wait"}}
  choices=append(choices,Chosen{Actor:actor,Decision:d})
 }
 next,err:=scheduledAdvance(j,s,choices);if err!=nil{t.Fatal(err)}
 if next.Minds[id].LastLightInitiativeDay!=1{t.Fatal("legacy staff initiative accounting changed")}
}

func TestAutonomyEarlyCommitmentDeparture(t *testing.T){
 j:=autonomyJournal(t);s,err:=initial(j);if err!=nil{t.Fatal(err)}
 s.Minute=960
 id:="kushida"
 m:=s.Minds[id];m.Location="hall";m.PendingStimulus="";s.Minds[id]=m
 s.World.Meetings=[]Meeting{{ID:"meeting-1",Host:id,Guest:"hirata",Status:"accepted",Location:"library",At:976}}
 d:=routineDecision(j,s,id)
 if d.Action!="move"||d.Target!="library"{t.Fatal("missed early travel opportunity",d)}
 m.Location="library";s.Minds[id]=m;s.Minute=964
 d=routineDecision(j,s,id)
 if d.Action!="wait"||d.WaitMinutes!=12{t.Fatal("did not wait at accepted appointment",d)}
}

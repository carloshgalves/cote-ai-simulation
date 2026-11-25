package pilot

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"
)
func scheduledJournal(t *testing.T) Journal {
	t.Helper();p,err:=decodeStrict[[]Profile](defaultProfiles);if err!=nil{t.Fatal(err)}
	return Journal{Version:ScheduledVersion,Model:"test",Profiles:p}
}
func chooseFor(j Journal,s State,overrides map[string]Decision)[]Chosen{
	out:=[]Chosen{}
	for _,id:=range eligibleActors(j,s){d:=Decision{Action:"wait",Reason:"Aguardar um estímulo"};if custom,ok:=overrides[id];ok{d=custom};out=append(out,Chosen{Actor:id,Decision:d})}
	return out
}
func TestScheduledWaitSkipsUnrelatedCognition(t *testing.T){
	j:=scheduledJournal(t);s,_:=initial(j)
	// Horikita is alone at the library. Two hall students converse.
	m:=s.Minds["horikita"];m.Location="library";s.Minds["horikita"]=m
	choices:=chooseFor(j,s,map[string]Decision{"hirata":{Action:"talk",Target:"kushida",Text:"Olá, vamos conversar."}})
	next,err:=scheduledAdvance(j,s,choices);if err!=nil{t.Fatal(err)}
	if next.Minute!=482||next.Minds["horikita"].Activity==nil||next.Minds["horikita"].Activity.Kind!="wait"{t.Fatalf("unexpected state: %+v",next)}
	if !reflect.DeepEqual(eligibleActors(j,next),[]string{"hirata","kushida"}){t.Fatal("unrelated actor awakened",eligibleActors(j,next))}
	next,err=scheduledAdvance(j,next,chooseFor(j,next,nil));if err!=nil{t.Fatal(err)}
	if next.Minute!=490||len(eligibleActors(j,next))!=6{t.Fatal("announcement did not activate all waiting actors")}
}
func TestScheduledMovementAndArrivalStimulus(t *testing.T){
	j:=scheduledJournal(t);s,_:=initial(j)
	m:=s.Minds["horikita"];m.Location="library";s.Minds["horikita"]=m
	next,err:=scheduledAdvance(j,s,chooseFor(j,s,map[string]Decision{"hirata":{Action:"move",Target:"library"},"ryuen":{Action:"observe"}}));if err!=nil{t.Fatal(err)}
	// Departure can cause a same-time cycle. Advance using waits.
	for next.Minute<484{
		if next.Minds["hirata"].Activity==nil||next.Minds["hirata"].Location!="hall"{t.Fatal("premature arrival")}
		for _,p:=range actorContext(j,next,"kushida").Present{if p.ID=="hirata"{t.Fatal("traveler still contactable")}}
		if next.Minds["horikita"].Activity==nil{t.Fatal("isolated actor awakened before arrival")}
		next,err=scheduledAdvance(j,next,chooseFor(j,next,nil));if err!=nil{t.Fatal(err)}
	}
	if next.Minute!=484||next.Minds["hirata"].Location!="library"||next.Minds["horikita"].Activity!=nil{t.Fatal("arrival timing or wake failed")}
	for _,e:=range visible(next,"kushida"){if e.Kind=="arrival"||e.Text=="Chegou a library"{t.Fatal("arrival leaked to origin")}}
}
func TestScheduledConversationCannotReachDepartingActor(t *testing.T){
	j:=scheduledJournal(t);s,_:=initial(j)
	next,err:=scheduledAdvance(j,s,chooseFor(j,s,map[string]Decision{"hirata":{Action:"move",Target:"library"},"kushida":{Action:"talk",Target:"hirata",Text:"Vamos conversar."}}));if err!=nil{t.Fatal(err)}
	found:=false
	for _,e:=range next.Events{if e.Kind=="claim"{t.Fatal("impossible conversation published")};if e.Kind=="obstacle"&&e.Actor=="kushida"{found=true}}
	if !found{t.Fatal("missing obstacle")}
}
func TestScheduledSessionPersistsSparseCyclesAndReplays(t *testing.T){
	calls:=0
	model:=modelFunc(func(_ context.Context,c Context)(Decision,error){calls++;return Decision{Action:"wait",WaitMinutes:1,Reason:"Aguardar"},nil})
	path:=filepath.Join(t.TempDir(),"v3.json")
	s,err:=OpenScheduled(path,"test",model);if err!=nil{t.Fatal(err)}
	if _,err=s.Step(context.Background(),0);err!=nil{t.Fatal(err)}
	if calls!=6||s.Snapshot().State.Minute!=481{t.Fatal("explicit wait duration ignored")}
	before:=s.Snapshot();s.Close()
	s,err=OpenScheduled(path,"test",model);if err!=nil{t.Fatal(err)};defer s.Close()
	if !reflect.DeepEqual(before,s.Snapshot()){t.Fatal("replay diverged")}
}
func TestScheduledZeroCallCycle(t *testing.T){
	j:=scheduledJournal(t);s,_:=initial(j)
	for id,m:=range s.Minds{m.Activity=&Activity{Kind:"wait",Until:490};s.Minds[id]=m}
	next,err:=scheduledAdvance(j,s,nil);if err!=nil||next.Minute!=490||len(eligibleActors(j,next))!=6{t.Fatal("world-only cycle failed",err)}
}

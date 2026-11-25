package pilot

import (
 "context"
 "path/filepath"
 "reflect"
 "testing"
)

func selectiveJournal(t *testing.T) Journal {
 t.Helper()
 j := cognitionJournal(t)
 j.Version = SelectiveVersion
 return j
}

func TestSelectiveRoutineAndAddressedStimulus(t *testing.T) {
 j := selectiveJournal(t)
 s, err := initial(j)
 if err != nil { t.Fatal(err) }
 s.Minute = 600
 id := "ayanokoji"
 m := s.Minds[id]
 m.PendingStimulus = ""
 m.Activity = nil
 m.AwaitEvent = false
 s.Minds[id] = m
 if m.Tier != FullTier || decisionNeedsModel(j,s,id) { t.Fatal("routine FULL actor polled") }
 stimulate(&s,id,"Recebeu uma fala")
 if !decisionNeedsModel(j,s,id) { t.Fatal("addressed stimulus suppressed") }
 if decisionNeedsModel(j,s,"ryuen") { t.Fatal("private stimulus leaked to another actor") }
 m = s.Minds[id]
 m.Activity = &Activity{Kind:"move",Until:610}
 s.Minds[id] = m
 if decisionNeedsModel(j,s,id) { t.Fatal("busy actor polled") }
}

func TestSelectiveInitiativeTimerAndLegacy(t *testing.T) {
 j := selectiveJournal(t)
 s, err := initial(j)
 if err != nil { t.Fatal(err) }
 id := "ayanokoji"
 m := s.Minds[id]
 m.PendingStimulus = ""
 m.Activity = nil
 s.Minute = 900
 s.Minds[id] = m
 if !decisionNeedsModel(j,s,id) { t.Fatal("missing free-period initiative") }
 m.LastLightInitiativeDay = 1
 s.Minds[id] = m
 if decisionNeedsModel(j,s,id) { t.Fatal("unbounded free-period polling") }
 m.NextDeliberation = 910
 s.Minds[id] = m
 if nextDue(j,s) != 910 || decisionNeedsModel(j,s,id) { t.Fatal("explicit recheck deadline ignored") }
 s.Minute = 910
 if !decisionNeedsModel(j,s,id) { t.Fatal("explicit recheck suppressed") }
 j.Version = CognitionVersion
 s.Minute = 600
 if !decisionNeedsModel(j,s,id) { t.Fatal("legacy semantics changed") }
}

func TestSelectiveExamDoesNotWakeOutsiders(t *testing.T) {
 j := selectiveJournal(t)
 s, err := initial(j)
 if err != nil { t.Fatal(err) }
 s.Minute = examBase(j.Version)+600
 advanceWorld(j,&s)
 if !decisionNeedsModel(j,s,"ayanokoji") { t.Fatal("participant not activated") }
 if decisionNeedsModel(j,s,"manabu-horikita") { t.Fatal("outsider activated by private exam") }
}

func TestSelectiveSessionRoutesAndReplays(t *testing.T) {
 calls := 0
 model := modelFunc(func(_ context.Context,c Context)(Decision,error){
  calls++
  return Decision{Action:"wait"},nil
 })
 path := filepath.Join(t.TempDir(),"v8.json")
 session,err := OpenSelective(path,"test",model)
 if err != nil { t.Fatal(err) }
 before := session.Snapshot()
 after,err := session.Step(context.Background(),0)
 if err != nil { session.Close();t.Fatal(err) }
 if after.Version != SelectiveVersion || calls >= 14 || after.State.Round != before.State.Round+1 {
  session.Close();t.Fatalf("unexpected calls/version %d/%d",calls,after.Version)
 }
 n := calls
 session.Close()
 session,err = OpenSelective(path,"test",model)
 if err != nil { t.Fatal(err) }
 defer session.Close()
 if !reflect.DeepEqual(after,session.Snapshot()) || calls != n { t.Fatal("replay diverged or called provider") }
}


// A deliberate trip to inspect a location must resume cognition on arrival;
// an engine-owned trip must not acquire that privilege.
func TestSelectiveDeliberateMovementContinuation(t *testing.T) {
 j:=selectiveJournal(t)
 s,err:=initial(j)
 if err!=nil{t.Fatal(err)}
 id:="ayanokoji"
 m:=s.Minds[id]
 m.PendingStimulus="Avaliar biblioteca"
 s.Minds[id]=m
 choices:=[]Chosen{}
 for _,actor:=range eligibleActors(j,s){
  d:=routineDecision(j,s,actor)
  if decisionNeedsModel(j,s,actor){d=Decision{Action:"wait"}}
  if actor==id{d=Decision{Action:"move",Target:"library"}}
  choices=append(choices,Chosen{Actor:actor,Decision:d})
 }
 next,err:=scheduledAdvance(j,s,choices)
 if err!=nil{t.Fatal(err)}
 due:=s.Minute+walkMinutes(m.Location,"library")
 if next.Minds[id].NextDeliberation!=due{t.Fatal("deliberate move lost follow-up")}
 for actor,other:=range next.Minds{
  if actor!=id&&other.LastAction=="move"&&other.NextDeliberation!=0{t.Fatal("routine move gained follow-up",actor)}
 }
}

func TestSelectiveRecheckSurvivesEarlierScheduleBoundary(t *testing.T) {
 j:=selectiveJournal(t)
 s,err:=initial(j)
 if err!=nil{t.Fatal(err)}
 id:="ayanokoji"
 m:=s.Minds[id]
 m.PendingStimulus=""
 m.Activity=nil
 m.NextDeliberation=550
 s.Minds[id]=m
 s.Minute=540
 if decisionNeedsModel(j,s,id){t.Fatal("premature deliberation")}
 d:=routineDecision(j,s,id)
 if d.Action!="wait"||d.WaitMinutes!=10||d.Source!=routineSource{t.Fatalf("explicit recheck replaced by routine: %+v",d)}
 s.Minute=550
 if !decisionNeedsModel(j,s,id){t.Fatal("recheck did not activate at its deadline")}
}

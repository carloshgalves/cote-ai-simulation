package pilot

import (
 "encoding/json"
 "os"
 "reflect"
 "strings"
 "testing"
)

// Exercise the current policy separately from the intentionally retained
// historical V9 failing witness. The model substitute never supplies
// dialogue or future knowledge; it is not a behavioral fidelity score.
func TestV16StackLongitudinal(t *testing.T){
 if os.Getenv("COTE_V16_STACK_EVAL")!="1"{t.Skip("set COTE_V16_STACK_EVAL=1")}
 for _,seed:=range []string{"a","b"}{
  t.Run(seed,func(t *testing.T){
   j:=autonomyJournal(t);j.Version=CompactContextVersion;j.WorldSeed=strings.Repeat(seed,64);initializeIdentities(&j)
   s,err:=initial(j);if err!=nil{t.Fatal(err)}
   goal:=s.Minute+1440;opportunities:=map[string]int{};staffTasks:=map[string]map[string]int{}
   for _,id:=range ids(j.Profiles){if !isStudent(j,id){staffTasks[id]=map[string]int{}}}
   for s.Minute<goal&&s.Round<1000{
    choices:=[]Chosen{}
    for _,id:=range eligibleActors(j,s){
     d:=routineDecision(j,s,id)
     if decisionNeedsModel(j,s,id){
      c:=actorContext(j,s,id);if c.Body!=nil||c.Interoception==nil{t.Fatal("body projection")}
      if c.Opportunity!=nil{opportunities[id]++}
      d=Decision{Action:"wait",Source:"eval/wait-only"}
     }
     choices=append(choices,Chosen{Actor:id,Decision:d})
    }
    next,err:=scheduledAdvance(j,s,choices);if err!=nil{t.Fatalf("cycle %d minute %d: %v",s.Round,s.Minute,err)}
    if err:=autonomyTransitionError(s,next);err!=nil{t.Fatalf("cycle %d: %v",s.Round,err)}
    for _,e:=range next.Events[len(s.Events):]{if e.Kind=="task"&&staffTasks[e.Actor]!=nil&&strings.HasPrefix(e.Text,"Concluiu "){staffTasks[e.Actor][strings.TrimPrefix(e.Text,"Concluiu ")]++}}
    j.Rounds=append(j.Rounds,Round{Number:next.Round,Minute:next.Minute,Choices:choices});s=next
   }
   raw,err:=json.Marshal(struct{Opportunities map[string]int `json:"opportunities"`;Staff map[string]map[string]int `json:"staff_tasks"`}{opportunities,staffTasks});if err!=nil{t.Fatal(err)}
   t.Logf("V16_STACK_METRICS cycles=%d minute=%d %s",s.Round,s.Minute,raw)
   if s.Minute<goal{t.Fatal("incomplete trajectory")}
   for _,id:=range []string{"ayanokoji","horikita","kushida","hirata","ichinose","ryuen"}{if opportunities[id]==0{t.Fatal("no opportunity considered",id)}}
   for id,tasks:=range staffTasks{
    if tasks["eat:free"]==0||tasks["sleep:"]==0{t.Fatal("staff recovery incomplete",id)}
    m:=s.Minds[id];if m.Body==nil||m.Body.Energy==0||m.Body.Hydration==0||m.Body.Fatigue==1000{t.Fatal("staff collapse",id)}
   }
   replay,err:=Replay(j);if err!=nil{t.Fatal(err)};if !reflect.DeepEqual(s,replay){t.Fatal("v16 replay diverged")}
  })
 }
}

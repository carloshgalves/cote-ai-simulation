package pilot

import (
 "encoding/json"
 "fmt"
 "os"
 "reflect"
 "strings"
 "testing"
)

// These are committed-decision counts, not provider request counts or token
// usage. The fixed wait-only model isolates the motor, not character fidelity.
type autonomyActorMetrics struct {
 Routine int `json:"routine_decisions"`
 Deliberative int `json:"deliberative_decisions"`
 Initiatives int `json:"endogenous_only_decisions"`
 Tasks map[string]int `json:"completed_tasks"`
 SharedDays int `json:"shared_leisure_pair_days"`
 DialogueTurns int `json:"dialogue_turns"`
}
type autonomyMetrics struct {
 Version int `json:"version"`
 Seed string `json:"seed"`
 Start int64 `json:"start_minute"`
 End int64 `json:"end_minute"`
 Cycles int `json:"cycles"`
 Routine int `json:"routine_decisions"`
 Deliberative int `json:"deliberative_decisions"`
 WorldOnly int `json:"world_only_cycles"`
 Actors map[string]*autonomyActorMetrics `json:"actors"`
}
func newAutonomyMetrics(j Journal,s State)*autonomyMetrics{
 m:=&autonomyMetrics{Version:j.Version,Seed:j.WorldSeed,Start:s.Minute,End:s.Minute,Actors:map[string]*autonomyActorMetrics{}}
 for _,id:=range ids(j.Profiles){m.Actors[id]=&autonomyActorMetrics{Tasks:map[string]int{}}}
 return m
}
func endogenousOnly(j Journal,s State,id string)bool{
 a:=s.Minds[id]
 if !initiativeDue(j,s,id)||a.PendingStimulus!=""||a.Conversation!=nil||a.NextDeliberation!=0{return false}
 if s.World.Exam!=nil&&has(s.World.Exam.Group,id)&&(s.World.Exam.Phase=="early"||s.World.Exam.Phase=="final"){return false}
 return true
}
func (m *autonomyMetrics) observe(j Journal,before,after State,choices []Chosen){
 m.Cycles++;m.End=after.Minute
 if len(choices)==0{m.WorldOnly++}
 for _,c:=range choices{
  a:=m.Actors[c.Actor]
  if c.Decision.Source==routineSource{a.Routine++;m.Routine++}else{
   a.Deliberative++;m.Deliberative++
   if endogenousOnly(j,before,c.Actor){a.Initiatives++}
  }
 }
 for _,e:=range after.Events[len(before.Events):]{
  a:=m.Actors[e.Actor]
  if a==nil{continue}
  if e.Kind=="task"{
   if strings.HasPrefix(e.Text,"Concluiu "){a.Tasks[strings.TrimPrefix(e.Text,"Concluiu ")]++}
   if e.Text=="Permaneceu em área de convivência"{a.Tasks["socialize:"]++}
  }
  if e.Kind=="claim"{a.DialogueTurns++}
 }
 for id,a:=range m.Actors{
  a.SharedDays=0
  for _,bond:=range after.Minds[id].Social{a.SharedDays+=bond.SharedLeisureDays}
 }
}

// Check independent observable invariants across transitions. The reducer
// validates decisions, but this additionally checks its resulting effects.
func autonomyTransitionError(before,after State)error{
 if after.Round!=before.Round+1||after.Minute<before.Minute{return fmt.Errorf("non-monotonic logical coordinates")}
 if before.World==nil||after.World==nil{return fmt.Errorf("world absent")}
 if len(after.Events)<len(before.Events)||len(before.Events)>0&&!reflect.DeepEqual(before.Events,after.Events[:len(before.Events)]){return fmt.Errorf("event history rewritten")}
 if len(after.World.Ledger)<len(before.World.Ledger)||len(before.World.Ledger)>0&&!reflect.DeepEqual(before.World.Ledger,after.World.Ledger[:len(before.World.Ledger)]){return fmt.Errorf("ledger history rewritten")}
 if len(before.Minds)!=len(after.Minds){return fmt.Errorf("population changed unexpectedly")}
 lastMinute:=before.Minute
 for i,e:=range after.Events[len(before.Events):]{
  if e.Minute<lastMinute||e.Minute>after.Minute||e.Round!=after.Round{return fmt.Errorf("event outside transition")}
  if e.ID!=fmt.Sprintf("e%06d",len(before.Events)+i+1){return fmt.Errorf("noncanonical event ID")}
  lastMinute=e.Minute
  for _,id:=range e.Audience{if _,ok:=after.Minds[id];!ok{return fmt.Errorf("unknown event audience")}}
  if e.Kind=="claim"&&(!has(e.Audience,e.Actor)||!has(e.Audience,e.Target)||len(e.Audience)!=2){return fmt.Errorf("dialogue audience")}
  if e.Kind=="body_alert"&&(len(e.Audience)!=1||e.Audience[0]!=e.Actor){return fmt.Errorf("body alert leaked")}
 }
 delta:=map[string]int64{}
 for _,entry:=range after.World.Ledger[len(before.World.Ledger):]{
  if entry.Amount<=0||entry.From==entry.To||entry.Minute<before.Minute||entry.Minute>after.Minute{return fmt.Errorf("invalid ledger entry")}
  if _,ok:=after.Minds[entry.From];!ok&&entry.From!="school"{return fmt.Errorf("unknown ledger payer")}
  if _,ok:=after.Minds[entry.To];!ok&&entry.To!="school"{return fmt.Errorf("unknown ledger recipient")}
  delta[entry.From]-=entry.Amount;delta[entry.To]+=entry.Amount
 }
 for id,a:=range after.Minds{
  prior,ok:=before.Minds[id];if !ok{return fmt.Errorf("unexpected actor")}
  if a.Wallet<0||a.Wallet!=prior.Wallet+delta[id]{return fmt.Errorf("wallet/ledger mismatch")}
  if a.Body!=nil{for _,v:=range []int{a.Body.Energy,a.Body.Hydration,a.Body.Fatigue}{if v<0||v>1000{return fmt.Errorf("body outside bounds")}}}
  if a.Activity!=nil&&a.Activity.Until<=after.Minute{return fmt.Errorf("expired activity retained")}
  for peer,bond:=range a.Social{
   other,ok:=after.Minds[peer];if !ok{return fmt.Errorf("unknown social peer")}
   if bond.SharedLeisureDays!=other.Social[id].SharedLeisureDays{return fmt.Errorf("asymmetric shared leisure")}
  }
 }
 var cp int64
 for _,n:=range after.World.ClassPoints{if n<0{return fmt.Errorf("negative class points")};cp+=n}
 if cp!=4000{return fmt.Errorf("class points not conserved")}
 return nil
}

func checkAutonomyTransition(t *testing.T,before,after State){
 t.Helper()
 if err:=autonomyTransitionError(before,after);err!=nil{t.Fatalf("cycle %d minute %d: %v",after.Round,after.Minute,err)}
}

func checkAutonomyView(t *testing.T,j Journal,s State,id string){
 t.Helper()
 c:=actorContext(j,s,id)
 for _,e:=range c.Memory{if !has(e.Audience,id)||e.Minute>s.Minute{t.Fatal("foreign/future memory",id,e.ID)}}
 if s.Minute<schoolDisclosure&&j.Population[id].Role=="student"&&j.Population[id].Year==1&&strings.Contains(strings.Join(c.School.Rules," "),classGuarantee){t.Fatal("premature class disclosure",id)}
 if s.World.Exam!=nil&&!has(s.World.Exam.Group,id)&&c.School.Exam!=nil{t.Fatal("private exam projected to outsider",id)}
 if _,ok:=evidence(c,"bio-kushida-past");ok&&id!="kushida"{t.Fatal("foreign biography",id)}
}

// A bounded full-population trajectory through 24 simulated hours; no
// provider, network, credentials or private corpus are used. Opt-in because
// replay of a growing event history is deliberately exercised.
func TestAutonomyLongitudinalEvaluation(t *testing.T){
 if os.Getenv("COTE_AUTONOMY_EVAL")!="1"{t.Skip("run with COTE_AUTONOMY_EVAL=1")}
 for _,seed:=range []string{strings.Repeat("a",64),strings.Repeat("b",64)}{
  t.Run(seed[:1],func(t *testing.T){
   j:=autonomyJournal(t);j.WorldSeed=seed
   s,err:=initial(j);if err!=nil{t.Fatal(err)}
   m:=newAutonomyMetrics(j,s)
   goal:=s.Minute+1440
   for s.Minute<goal&&s.Round<600{
    choices:=[]Chosen{}
    for _,id:=range eligibleActors(j,s){
     var d Decision
     if decisionNeedsModel(j,s,id){
      checkAutonomyView(t,j,s,id)
      d=Decision{Action:"wait",Source:"eval/wait-only"}
     }else{d=routineDecision(j,s,id)}
     choices=append(choices,Chosen{Actor:id,Decision:d})
    }
    next,err:=scheduledAdvance(j,s,choices);if err!=nil{t.Fatalf("cycle %d minute %d: %v",s.Round,s.Minute,err)}
    checkAutonomyTransition(t,s,next)
    m.observe(j,s,next,choices)
    j.Rounds=append(j.Rounds,Round{Number:next.Round,Minute:next.Minute,Choices:choices})
    s=next
   }
   raw,err:=json.Marshal(m);if err!=nil{t.Fatal(err)}
   t.Logf("AUTONOMY_METRICS %s",raw)
   if s.Minute<goal{t.Fatal("trajectory failed to reach horizon")}
   if m.Routine==0||m.Deliberative==0||m.Routine<m.Deliberative{t.Fatal("autonomy collapsed or cognition suppressed")}
   for _,id:=range []string{"horikita","kushida","hirata","ken-sudo"}{
    a:=m.Actors[id]
    if a.Routine==0||len(a.Tasks)==0{t.Fatal("focal actor had no ordinary life",id)}
   }
   // Always check replay, including the historically failing seed.
   replay,err:=Replay(j);if err!=nil{t.Fatal(err)}
   if !reflect.DeepEqual(s,replay){t.Fatal("state replay divergence")}
   if m.Actors["horikita"].Initiatives==0{t.Fatal("Horikita endogenous initiative absent")}
   if seed==strings.Repeat("b",64)&&m.Actors["kushida"].Initiatives==0&&m.Actors["kushida"].Deliberative>0{
    // V9 consumes a day's initiative even when a simultaneous addressed
    // stimulus caused the model dispatch. This is a known legacy gap, not
    // a successful autonomy outcome. Do not change old journal semantics.
    t.Log("KNOWN_LEGACY_GAP V9 seed b: Kushida had deliberative decisions but no endogenous-only dispatch")
    if os.Getenv("COTE_STRICT_LEGACY_AUTONOMY")=="1"{t.Error("focal endogenous initiative absent (known V9 witness)")}
   }else if m.Actors["kushida"].Initiatives==0{t.Fatal("Kushida endogenous initiative absent")}
  })
 }
 // A legacy fixture cannot qualify the shipping policy. The V29 gate uses
 // the same admission/corpus construction as OpenClassPoll and both seeds.
 for _,label:=range []string{"a","b"}{
  t.Run("v29-"+label,func(t *testing.T){evaluateV29Autonomy(t,label)})
 }
}

func evaluateV29Autonomy(t *testing.T,label string){
 t.Helper()
 j:=autonomyJournal(t)
 j.Version=ClassPollVersion
 j.WorldSeed=strings.Repeat(label,64)
 if err:=admitExternalStudents(&j);err!=nil{t.Fatal(err)}
 initializeIdentities(&j)
 if err:=attachCorpus(&j);err!=nil{t.Fatal(err)}
 s,err:=initial(j);if err!=nil{t.Fatal(err)}
 if len(s.World.Calendar)!=18||len(s.World.GradePoints)!=12||!known(j.Profiles,"tg-yuuichi-katagiri"){t.Fatal("not a full V29 admission")}
 goal:=s.Minute+1440
 m:=newAutonomyMetrics(j,s)
 strategic:=0
 for s.Minute<goal&&s.Round<1000{
  choices:=[]Chosen{}
  for _,id:=range eligibleActors(j,s){
   var d Decision
   if decisionNeedsModel(j,s,id){
    checkAutonomyView(t,j,s,id)
    c:=actorContext(j,s,id)
    if c.Body!=nil||c.Interoception==nil{t.Fatal("body projection",id)}
    if c.Opportunity!=nil&&!essentialCognition(j,s,id)&&s.Minds[id].NextDeliberation==0{strategic++}
    d=Decision{Action:"wait",Source:"eval/wait-only"}
   }else{d=routineDecision(j,s,id)}
   choices=append(choices,Chosen{Actor:id,Decision:d})
  }
  next,err:=scheduledAdvance(j,s,choices);if err!=nil{t.Fatalf("cycle %d minute %d: %v",s.Round,s.Minute,err)}
  checkAutonomyTransition(t,s,next)
  m.observe(j,s,next,choices)
  j.Rounds=append(j.Rounds,Round{Number:next.Round,Minute:next.Minute,Choices:choices})
  s=next
 }
 t.Logf("V29_AUTONOMY seed=%s minute=%d cycles=%d routine=%d deliberative=%d strategic_only=%d",label,s.Minute,s.Round,m.Routine,m.Deliberative,strategic)
 if s.Minute<goal||m.Routine==0||m.Deliberative==0||strategic==0{t.Fatal("incomplete or degenerate V29 trajectory")}
 if s.World.Institutions==nil||s.World.Institutions.LastDay!=s.Minute/1440+1{t.Fatal("institutional clock not advanced")}
 for _,id:=range []string{"horikita","kushida","hirata","ken-sudo","tg-yuuichi-katagiri"}{
  a:=m.Actors[id]
  if a==nil||a.Routine==0||len(a.Tasks)==0{t.Fatal("no ordinary life",id)}
 }
 for _,id:=range ids(j.Profiles){
  if isStudent(j,id){continue}
  a:=m.Actors[id]
  if a.Tasks["eat:free"]==0||a.Tasks["sleep:"]==0{t.Fatal("staff recovery incomplete",id)}
  b:=s.Minds[id].Body
  if b==nil||b.Energy==0||b.Hydration==0||b.Fatigue==1000{t.Fatal("staff collapse",id)}
 }
 replay,err:=Replay(j);if err!=nil{t.Fatal(err)}
 if !reflect.DeepEqual(s,replay){t.Fatal("V29 replay divergence")}
}

// Positive, contrast and anti-cases based on admission dispositions.
// These do not claim fidelity of an LLM or reproduce a later canon scene.
func TestAutonomyBehavioralContrast(t *testing.T){
 j:=autonomyJournal(t);s,err:=initial(j);if err!=nil{t.Fatal(err)}
 s.Minute=1040 // paid-leisure branch is closed; enough time for a hobby
 for _,tc:=range []struct{id,location,action string}{
  {"horikita","library","read"},
  {"kushida","courtyard","socialize"},
  {"ken-sudo","gym","exercise"},
  {"ayanokoji","park","relax"},
 }{
  m:=s.Minds[tc.id];m.Location=tc.location;m.PendingStimulus="";m.Activity=nil
  m.Body=&PilotBody{Energy:900,Hydration:900,Fatigue:100};s.Minds[tc.id]=m
  d:=routineDecision(j,s,tc.id)
  if d.Action!=tc.action{t.Fatalf("%s: got %s want %s",tc.id,d.Action,tc.action)}
  if err:=validateDecision(actorContext(j,s,tc.id),d);err!=nil{t.Fatal(err)}
 }
 // A disposition is not an immutable script. Acute needs override hobbies.
 m:=s.Minds["ken-sudo"];m.Body.Hydration=100;s.Minds["ken-sudo"]=m
 if d:=routineDecision(j,s,"ken-sudo");d.Action!="move"||d.Target!="cafeteria"{t.Fatal("sport overrides acute need",d)}
 // Private plans and identity of someone else must not influence a routine.
 before:=routineDecision(j,s,"horikita")
 foreign:=j.Identities["kushida"];foreign.Values=[]string{"test private marker"};j.Identities["kushida"]=foreign
 if !equalDecision(before,routineDecision(j,s,"horikita")){t.Fatal("foreign identity influences routine")}
 checkAutonomyView(t,j,s,"horikita")
}

func TestAutonomyNegativeControlForgedRoutine(t *testing.T){
 j:=autonomyJournal(t);s,err:=initial(j);if err!=nil{t.Fatal(err)}
 choices:=[]Chosen{}
 mutated:=false
 for _,id:=range eligibleActors(j,s){
  d:=Decision{Action:"wait",Source:"eval/wait-only"}
  if !decisionNeedsModel(j,s,id){
   d=routineDecision(j,s,id)
   if !mutated{d.Source="eval/forged-model";mutated=true}
  }
  choices=append(choices,Chosen{Actor:id,Decision:d})
 }
 if !mutated{t.Fatal("negative control has no routine actor")}
 if _,err:=scheduledAdvance(j,s,choices);err==nil||!strings.Contains(err.Error(),"invalid decision provenance"){t.Fatal("forged source accepted",err)}
}

// Optional local-only assessment of a real V9 journal, with no provider
// calls. Print aggregate counts only; never print thoughts, dialogue, IDs,
// seeds or raw journal data. Canonical quality requires separate assessment.
func TestAutonomyEvaluateLocalJournal(t *testing.T){
 path:=os.Getenv("COTE_AUTONOMY_JOURNAL")
 if path==""{t.Skip("set COTE_AUTONOMY_JOURNAL to a local V9 journal")}
 info,err:=os.Stat(path);if err!=nil||!info.Mode().IsRegular()||info.Size()>32<<20{t.Fatal("journal unavailable or exceeds pilot size limit")}
 raw,err:=os.ReadFile(path);if err!=nil{t.Fatal("cannot read journal")}
 j,err:=decodeStrict[Journal](raw);if err!=nil||j.Version!=AutonomyVersion{t.Fatal("expected valid V9 journal")}
 if len(j.Rounds)==0||len(j.Rounds)>roundLimit(j.Version){t.Fatal("invalid evaluation horizon")}
 s,err:=initial(j);if err!=nil{t.Fatal("invalid initial state")}
 m:=newAutonomyMetrics(j,s)
 for _,r:=range j.Rounds{
  next,err:=apply(j,s,r);if err!=nil{t.Fatalf("invalid replay at cycle %d",r.Number)}
  // Unlike CI synthetic fixtures, avoid leaking actor identifiers or event
  // contents from an operator's real journal into test output.
  if err:=autonomyTransitionError(s,next);err!=nil{t.Fatalf("invalid effects at cycle %d: %v",r.Number,err)}
  m.observe(j,s,next,r.Choices);s=next
 }
 if m.End<=m.Start{t.Fatal("journal contains no elapsed simulated time")}
 t.Log(fmt.Sprintf("V9_JOURNAL cycles=%d minutes=%d routine=%d deliberative=%d world_only=%d",m.Cycles,m.End-m.Start,m.Routine,m.Deliberative,m.WorldOnly))
}

func TestAutonomyAuditDetectsRewrittenHistory(t *testing.T){
 j:=autonomyJournal(t);before,err:=initial(j);if err!=nil{t.Fatal(err)}
 choices:=[]Chosen{}
 for _,id:=range eligibleActors(j,before){
  d:=routineDecision(j,before,id)
  if decisionNeedsModel(j,before,id){d=Decision{Action:"wait",Source:"eval/wait-only"}}
  choices=append(choices,Chosen{Actor:id,Decision:d})
 }
 good,err:=scheduledAdvance(j,before,choices);if err!=nil{t.Fatal(err)}
 if err:=autonomyTransitionError(before,good);err!=nil{t.Fatal("unmodified transition rejected",err)}
 for _,tc:=range []struct{name,want string;mutate func(*State)}{
  {"event-rewrite","event history rewritten",func(s *State){s.Events[0].Text="rewritten"}},
  {"event-truncate","event history rewritten",func(s *State){s.Events=nil}},
  {"ledger-rewrite","ledger history rewritten",func(s *State){s.World.Ledger[0].Amount++}},
  {"wallet","wallet/ledger mismatch",func(s *State){m:=s.Minds["hirata"];m.Wallet++;s.Minds["hirata"]=m}},
  {"event-id","noncanonical event ID",func(s *State){appendEvent(s,Event{Kind:"task",Actor:"hirata",Audience:[]string{"hirata"}});s.Events[len(s.Events)-1].ID="forged"}},
 }{
  t.Run(tc.name,func(t *testing.T){
   bad:=clone(good);tc.mutate(&bad)
   if err:=autonomyTransitionError(before,bad);err==nil||err.Error()!=tc.want{t.Fatal("mutation survived or wrong rejection",err)}
  })
 }
}

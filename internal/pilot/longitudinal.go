package pilot

import (
 "context"
 "errors"
 "fmt"
 "reflect"
 "time"
)
type LongitudinalRun struct{
 Target string `json:"target"`
 StartMinute int64 `json:"start_minute"`
 EndMinute int64 `json:"end_minute"`
 StartRound int `json:"start_round"`
 EndRound int `json:"end_round"`
 WallMillis int64 `json:"wall_millis"`
 ModelAttempts uint64 `json:"model_attempts"`
 Reached bool `json:"reached"`
}
type YearAudit struct{
 Version int `json:"version"`
 Minute int64 `json:"minute"`
 Rounds int `json:"rounds"`
 Digest string `json:"digest"`
 ReplayEqual bool `json:"replay_equal"`
 PlannedExams int `json:"planned_exams"`
 PublishedExams int `json:"published_exams"`
 CompleteGradeExams int `json:"complete_grade_exams"`
 NamedSubmissions int `json:"named_submissions"`
 GenericSubmissions int `json:"generic_submissions"`
 NamedParticipantSlots int `json:"named_participant_slots"`
 DialogueEvents int `json:"dialogue_events"`
 SocialEvents int `json:"social_events"`
 CognitiveDecisions int `json:"cognitive_decisions"`
 RoutineDecisions int `json:"routine_decisions"`
 CollapsedBodies []string `json:"collapsed_bodies"`
 GradePoints map[string]int64 `json:"grade_points"`
 YearComplete bool `json:"year_complete"`
 StructuralGate bool `json:"structural_gate"`
 FidelityGate string `json:"fidelity_gate"`
 TransitionAllowed bool `json:"transition_allowed"`
}
func(s *Session) RunUntil(ctx context.Context,target string)(run LongitudinalRun,retErr error){
 snap:=s.Snapshot();run=LongitudinalRun{Target:target,StartMinute:snap.State.Minute,EndMinute:snap.State.Minute,StartRound:snap.State.Round,EndRound:snap.State.Round}
 if snap.Version<FidelityVersion{return run,errors.New("longitudinal command requires v26+")}
 start:=time.Now();calls:=s.Progress().ModelCalls
 defer func(){end:=s.Snapshot();run.EndMinute=end.State.Minute;run.EndRound=end.State.Round;run.WallMillis=time.Since(start).Milliseconds();run.ModelAttempts=s.Progress().ModelCalls-calls}()
 if snap.State.World==nil||len(snap.State.World.Calendar)==0{return run,errors.New("calendar missing")}
 var at int64
 switch target{case "first-exam":at=snap.State.World.Calendar[0].Start;case "first-result":at=snap.State.World.Calendar[0].Start+360;case "year-end":at=endMinute(snap.Version);default:return run,errors.New("unknown longitudinal target")}
 for snap.State.Minute<at{
  if err:=ctx.Err();err!=nil{return run,err}
  if snap.Done{return run,errors.New("simulation limit before target")}
  old:=snap.State.Round
  if snap.NeedsInference{next,err:=s.Step(ctx,old);if err!=nil{return run,err};snap=next}else{batch,err:=s.advanceRoutineMacro(ctx,old,at);if err!=nil{return run,err};snap=batch.Snapshot;if batch.Cycles==0&&!snap.NeedsInference{return run,errors.New("routine macro made no progress")}}
  if snap.State.Round==old&&!snap.NeedsInference{return run,errors.New("no committed progress")}
 }
 run.Reached=true;return run,nil
}
func(s *Session) AuditYear()(YearAudit,error){
 s.mu.Lock();j,state:=clone(s.journal),clone(s.state);s.mu.Unlock()
 return auditYear(j,state)
}
func auditYear(j Journal,state State)(YearAudit,error){
 a:=YearAudit{Version:j.Version,Minute:state.Minute,Rounds:state.Round,Digest:Digest(j),FidelityGate:"NOT_EVALUATED",CollapsedBodies:[]string{}}
 replay,err:=Replay(j);if err!=nil{return a,err};a.ReplayEqual=reflect.DeepEqual(replay,state)
 for _,r:=range j.Rounds{for _,c:=range r.Choices{if c.Decision.Source==routineSource{a.RoutineDecisions++}else{a.CognitiveDecisions++}}}
 for _,e:=range state.Events{if e.Kind=="claim"{a.DialogueEvents++};if e.Kind=="social"||e.Kind=="relation"{a.SocialEvents++}}
 for _,id:=range ids(j.Profiles){m:=state.Minds[id];if m.Body!=nil&&(m.Body.Energy==0||m.Body.Hydration==0||m.Body.Fatigue==1000){a.CollapsedBodies=append(a.CollapsedBodies,id)}}
 if state.World==nil{return a,nil}
 a.PlannedExams=len(state.World.Calendar);a.PublishedExams=len(state.World.ExamHistory);a.GradePoints=clone(state.World.GradePoints)
 slots:=map[string]ExamSlot{};for _,slot:=range state.World.Calendar{slots[slot.ID]=slot}
 seenExams:=map[string]bool{}
 for _,res:=range state.World.ExamHistory{
  slot,ok:=slots[res.ID];if !ok||seenExams[res.ID]{return a,fmt.Errorf("unexpected exam result %s",res.ID)};seenExams[res.ID]=true
  expected:=map[string]bool{};for _,id:=range slot.Group{expected[id]=true;if known(j.Profiles,id){a.NamedParticipantSlots++}}
  actual:=map[string]bool{};submitted:=map[string]bool{};valid:=len(expected)==160&&len(res.GroupResults)==len(slot.Groups)
  for _,g:=range res.GroupResults{for _,id:=range g.Group{if actual[id]||!expected[id]{valid=false};actual[id]=true};for _,id:=range g.Submitted{if submitted[id]||!has(g.Group,id){valid=false;continue};submitted[id]=true;if known(j.Profiles,id){a.NamedSubmissions++}else{a.GenericSubmissions++}}}
  if valid&&len(actual)==len(expected){a.CompleteGradeExams++}
 }
 a.YearComplete=j.Version>=WholeGradeVersion&&state.Minute>=endMinute(j.Version)&&a.PlannedExams==18&&a.PublishedExams==18
 a.StructuralGate=a.YearComplete&&a.ReplayEqual&&a.CompleteGradeExams==18&&len(a.CollapsedBodies)==0
 // Do not promote on structural evidence alone. Actual model fidelity,
 // academic performance and independent review are separate gates.
 a.TransitionAllowed=false
 return a,nil
}

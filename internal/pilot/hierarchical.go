package pilot

import (
 "fmt"
 "strings"
)

// A versioned approximation. Own goals and plans determine whether a
// discretionary opportunity needs language-level reasoning. Explicit
// stimuli, exam participation and deliberate deadlines are checked first
// by calibratedNeedsModel. Ordinary social/academic interests run locally.
func strategicOpportunity(j Journal,s State,id string)bool{
 op:=personalOpportunity(j,s,id);if op==nil{return false}
 m:=s.Minds[id]
 if m.Plan!=nil||len(m.Plans)>0||op.Kind=="plan"{return true}
 if m.OpportunityDay==s.Minute/1440+1&&m.OpportunityCount>=1{return false}
 for _,goal:=range m.Goals{if strategicGoal(goal){return true}}
 // Unrecognized/ambiguous goals are never assigned a fabricated motive.
 // Routine interests may generate co-presence, not dialogue or trust.
 return false
}

func strategicGoal(goal string)bool{
 g:=strings.ToLower(goal)
 for _,term:=range []string{"influência","estratég","vantagen","manipul","investig","segredo","suspeita","ameaça","rival","proteger","ocultar","reconhecimento","talento","desafios de raciocínio","cooperação"}{if strings.Contains(g,term){return true}}
 return false
}

func macroCycleLimit(version int)int{if version>=HierarchicalVersion{return 96};return 32}

// Coarsen only repeatable time-integrated actions. Keep public period,
// all exam boundaries, plan deadlines, physiological needs and scheduled
// meetings hard limits. No meal, purchase, exercise or move aggregation.
func hierarchicalDuration(j Journal,s State,id,action string,current int64)int64{
 if !has([]string{"lesson","study","work","read","relax","socialize","rest"},action){return current}
 m:=s.Minds[id]
 if m.Body==nil||m.PendingStimulus!=""||m.Conversation!=nil||m.Plan!=nil||len(m.Plans)>0||m.NextDeliberation>0{return current}
 if e:=examFor(s,id);e!=nil&&e.Outcome==0&&has(e.Group,id){return current}
 for _,mt:=range s.World.Meetings{if (mt.Status=="pending"||mt.Status=="accepted")&&(mt.Host==id||mt.Guest==id){return current}}
 // Study and work may run until the end of a school period; free-time
 // activities return at least hourly so new social opportunities persist.
 n:=nextDue(j,s)-s.Minute
 if has([]string{"read","relax","socialize"},action)&&n>60{n=60}
 if action=="rest"&&n>120{n=120}
 for _,cap:=range []int64{int64(m.Body.Energy)-350,int64(m.Body.Hydration)-350}{if cap<n{n=cap}}
 if action!="rest"&&action!="relax"{if cap:=int64(850-m.Body.Fatigue);cap<n{n=cap}}
 if n>current{return n}
 return current
}

// Interrupt only V22 explicit coarse activities; retain elapsed bodily
// effects and an auditable partial activity, not a completed task.
func interruptCoarse(s *State,id,reason string){
 m:=s.Minds[id];a:=m.Activity
 if a==nil||!a.Coarse{return}
 elapsed:=s.Minute-a.Started;if elapsed<0{elapsed=0}
 appendEvent(s,Event{Kind:"activity",Actor:id,Text:fmt.Sprintf("Rotina %s interrompida após %d min: %s",a.Target,elapsed,reason),Audience:[]string{id}})
 m.Activity=nil;m.WakeReason=reason;s.Minds[id]=m
}

// Actual co-presence for at least 10 minutes of concurrent leisure.
// Both actors have selected socialize; no inferred consent, speech,
// belief, trust or affiliation is manufactured from shared presence.
func recordOverlappingLeisure(j Journal,s *State){
 group:=[]string{}
 for _,id:=range ids(j.Profiles){m:=s.Minds[id];if m.Activity!=nil&&m.Activity.Kind=="task"&&m.Activity.Target=="socialize:"&&m.Activity.Started>0&&m.Location=="courtyard"{group=append(group,id)}}
 for i,a:=range group{for _,b:=range group[i+1:]{
  ma,mb:=s.Minds[a],s.Minds[b];start:=ma.Activity.Started;if mb.Activity.Started>start{start=mb.Activity.Started}
  end:=s.Minute;if ma.Activity.Until<end{end=ma.Activity.Until};if mb.Activity.Until<end{end=mb.Activity.Until}
  if end-start<10{continue}
  day:=s.Minute/1440+1;if ma.Social[b].LastSharedLeisureDay==day{continue}
  for _,pair:=range [][2]string{{a,b},{b,a}}{
   m:=s.Minds[pair[0]];if m.Social==nil{m.Social=map[string]SocialBond{}}
   bond:=m.Social[pair[1]];bond.SharedLeisureDays++;bond.LastSharedLeisureDay=day;m.Social[pair[1]]=bond;s.Minds[pair[0]]=m
  }
  appendEvent(s,Event{Kind:"social",Actor:a,Target:b,Text:fmt.Sprintf("Convivência observada no pátio: pelo menos %d min de presença simultânea; sem inferir diálogo ou confiança.",end-start),Audience:[]string{a,b}})
 }}
}

// Deterministic routine decisions cannot contain language-level cognition.
// Validate the same legal offer against an intentionally minimal context.
// Full actorContext remains mandatory for all model-generated decisions.
func routineValidationContext(j Journal,s State,id string)Context{
 c:=Context{Version:j.Version,Location:s.Minds[id].Location,Locations:worldLocations(id),School:schoolView(j,s,id)}
 projectPopulation(j,s,id,&c)
 return c
}
func validateScheduledChoice(j Journal,s State,x Chosen)error{
 if j.Version>=HierarchicalVersion&&x.Decision.Source==routineSource{
  return validateDecision(routineValidationContext(j,s,x.Actor),x.Decision)
 }
 return validateDecision(actorContext(j,s,x.Actor),x.Decision)
}
func(r *Router) SimulationAttempts()uint64{r.mu.Lock();defer r.mu.Unlock();return r.simulationAttempts}

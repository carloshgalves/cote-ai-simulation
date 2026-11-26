package pilot

import "fmt"

// Provisional accounting adapter for the local pilot. These dimensionless
// meters are not the calibrated CapacityProfile/BodyState of src/embodiment.
type PilotBody struct{
 Energy int `json:"energy"`
 Hydration int `json:"hydration"`
 Fatigue int `json:"fatigue"`
}
type SocialBond struct{
 Interactions int `json:"interactions"`
 Trust int `json:"trust"`
 Fulfilled int `json:"fulfilled"`
 Missed int `json:"missed"`
 LastMinute int64 `json:"last_minute"`
 SharedLeisureDays int `json:"shared_leisure_days,omitempty"`
 LastSharedLeisureDay int64 `json:"last_shared_leisure_day,omitempty"`
}
func clampMeter(x int)int{if x<0{return 0};if x>1000{return 1000};return x}
func recognizes(j Journal,id,other string)bool{for _,f:=range j.Identities[id].Background{if f.Person==other{return true}};return false}
func stimulate(s *State,id,reason string){
 m,ok:=s.Minds[id];if !ok{return}
 if m.Activity!=nil&&m.Activity.Coarse{interruptCoarse(s,id,reason);m=s.Minds[id]}
 m.PendingStimulus=reason
 if m.Activity!=nil&&m.Activity.Kind=="wait"{m.Activity=nil;m.WakeReason=reason}
 s.Minds[id]=m
}
func hasCognitiveStimulus(s State,id string)bool{
 m:=s.Minds[id]
 if m.PendingStimulus!=""||m.Conversation!=nil&&m.Conversation.Turn==id{return true}
 if s.World!=nil{for _,mt:=range s.World.Meetings{if mt.Guest==id&&mt.Status=="pending"&&s.Minute<mt.At{return true}}}
 return false
}
func stimulateExam(s *State,reason string){if s.World.Exam!=nil{for _,id:=range s.World.Exam.Group{stimulate(s,id,reason)}}}
func advanceBodies(j Journal,s *State,previous int64){
 dt:=int(s.Minute-previous);if dt<=0{return}
 for _,id:=range ids(j.Profiles){
  m:=s.Minds[id];if m.Body==nil{continue};old:=*m.Body
  kind,target:="","";if m.Activity!=nil{kind=m.Activity.Kind;target=m.Activity.Target}
  // Integer rates make clock subdivision deterministic; food is applied
  // only on completed meal, sleep/rest only over elapsed time.
  e,h,f:=1,1,1
  if kind=="move"||kind=="task"&&target=="exercise:"{e=3;h=3;f=4}
  if kind=="task"&&target=="rest:"{f=-3}
  if j.Version>=AutonomyVersion&&kind=="task"&&target=="relax:"{f=-2}
  if kind=="task"&&target=="sleep:"{e=0;h=0;f=-3}
  m.Body.Energy=clampMeter(old.Energy-e*dt)
  m.Body.Hydration=clampMeter(old.Hydration-h*dt)
  m.Body.Fatigue=clampMeter(old.Fatigue+f*dt)
  alert:=(isStudent(j,id)||j.Version>=StaffCareVersion)&&(old.Energy>=250&&m.Body.Energy<250||old.Hydration>=250&&m.Body.Hydration<250||old.Fatigue<=850&&m.Body.Fatigue>850)
  s.Minds[id]=m
  if alert{appendEvent(s,Event{Kind:"body_alert",Actor:id,Text:"Percebe necessidade de alimentação, hidratação ou repouso.",Audience:[]string{id}});if j.Version<SelectiveVersion{stimulate(s,id,"Necessidade corporal relevante")}}
 }
}
func recordExchange(s *State,a,b string){
 for _,pair:=range [][2]string{{a,b},{b,a}}{
  m:=s.Minds[pair[0]];if m.Social==nil{m.Social=map[string]SocialBond{}}
  v:=m.Social[pair[1]];v.Interactions++;v.LastMinute=s.Minute;m.Social[pair[1]]=v;s.Minds[pair[0]]=m
 }
}
func recordMeeting(s *State,mt Meeting){
 for _,pair:=range [][2]string{{mt.Host,mt.Guest},{mt.Guest,mt.Host}}{
  m:=s.Minds[pair[0]];if m.Social==nil{m.Social=map[string]SocialBond{}}
  v:=m.Social[pair[1]];v.LastMinute=s.Minute
  if mt.Status=="met"{v.Fulfilled++;if v.Trust<10{v.Trust++}}else{
   // An actor who attended receives a negative signal only when the peer
   // did not. Two missed appointments do not imply deliberate betrayal.
   selfAttended,peerAttended:=mt.HostAttended,mt.GuestAttended
   if pair[0]==mt.Guest{selfAttended,peerAttended=peerAttended,selfAttended}
   if selfAttended&&!peerAttended{v.Missed++;if v.Trust> -10{v.Trust--}}
  }
  m.Social[pair[1]]=v;s.Minds[pair[0]]=m
  appendEvent(s,Event{Kind:"social",Actor:pair[0],Target:pair[1],Text:fmt.Sprintf("Compromisso %s: %s; confiança percebida %+d.",mt.ID,mt.Status,v.Trust),Audience:[]string{pair[0]}})
  stimulate(s,pair[0],"Resultado de compromisso")
 }
}
func planHorizon(p PlanChange)string{if p.Horizon==""{return "short"};return p.Horizon}
func validateHorizon(c Context,p PlanChange)error{
 if !has([]string{"immediate","short","medium","long"},planHorizon(p)){return fmt.Errorf("invalid plan horizon")}
 if p.Deadline!=0&&(p.Deadline<=c.Minute||p.Deadline>WorldEnd){return fmt.Errorf("invalid plan deadline")}
 if p.Scope!=""&&p.Scope!="general"&&p.Scope!="vip"{return fmt.Errorf("invalid plan scope")}
 if p.Scope=="vip"&&(c.School==nil||c.School.Exam==nil||c.School.Exam.Phase=="settled"){return fmt.Errorf("VIP plan requires an active exam")}
 return nil
}
func applyHorizon(m *Mind,p PlanChange){
 if m.Plans==nil{m.Plans=map[string]PlanChange{}}
 h:=planHorizon(p);p.Horizon=h
 if len(p.Steps)==0{delete(m.Plans,h)}else{m.Plans[h]=clone(p)}
 refreshPlan(m)
}
func refreshPlan(m *Mind){
 m.Plan=nil
 for _,h:=range []string{"immediate","short","medium","long"}{if p,ok:=m.Plans[h];ok{m.Plan=&p;return}}
}
func expirePlans(j Journal,s *State){
 for _,id:=range ids(j.Profiles){
  m:=s.Minds[id];changed:=false
  for _,h:=range []string{"immediate","short","medium","long"}{
   p,ok:=m.Plans[h];if !ok{continue}
   if p.Deadline>0&&p.Deadline<=s.Minute||p.Scope=="vip"&&examFor(*s,id)!=nil&&examFor(*s,id).Phase=="settled"{
    delete(m.Plans,h);changed=true
    appendEvent(s,Event{Kind:"plan_end",Actor:id,Text:"Plano "+h+" encerrado por prazo ou fim do exame.",Audience:[]string{id},Evidence:p.Evidence})
   }
  }
  if changed{refreshPlan(&m);s.Minds[id]=m;stimulate(s,id,"Plano expirado; reavaliar objetivos")}
 }
}

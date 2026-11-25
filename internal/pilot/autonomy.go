package pilot

import (
 "crypto/sha256"
 "fmt"
 "strings"
)

// The V9 local-pilot policy uses only the actor's own initial dispositions,
// current goals, own plans, needs and legitimately available offers. Its
// weights are provisional simulator priors, not inferred canonical scores.
type routinePrior struct {
 leisure string
 cadence int64
}
func personalPrior(j Journal,s State,id string) routinePrior {
 m:=s.Minds[id]
 ident:=j.Identities[id]
 words:=strings.ToLower(strings.Join(append(append(clone(ident.Values),ident.Capacities...),m.Goals...)," "))
 includes:=func(terms ...string)bool{
  for _,term:=range terms{if strings.Contains(words,term){return true}}
  return false
 }
 // Stable seeded fallback for sparse profiles; never use representation tier
 // or another person's secret identity to select an actor's routine.
 h:=sha256.Sum256([]byte(j.WorldSeed+"/pilot-v9/prior/"+id))
 p:=routinePrior{leisure:[]string{"read","relax","socialize","exercise","relax"}[int(h[0])%5],cadence:2+int64(h[1]%2)}
 switch {
 case includes("esportiv","esporte","atlet"):
  p.leisure="exercise"
 case includes("desempenho acadêmico","competência e desempenho","estudar","leitura","livros"):
  p.leisure="read"
 case includes("sociável","rede de relações","integrar os colegas","confiança com colegas","relações sociais"):
  p.leisure="socialize";p.cadence=1
 case includes("reservado","discrição","autossuficiente"):
  p.leisure="relax"
 }
 if includes("influência","estratég","vantagens pessoais","reconhecimento","desempenho acadêmico"){p.cadence=1}
 if m.Plan!=nil&&len(m.Plan.Steps)>0{p.cadence=1}
 return p
}

func initiativeDue(j Journal,s State,id string) bool {
 if j.Version<AutonomyVersion{return isStudent(j,id)&&lightInitiativeDue(s,id)}
 if !isStudent(j,id){return false}
 m:=s.Minds[id]
 day:=s.Minute/1440+1
 if m.LastLightInitiativeDay==day{return false}
 p:=personalPrior(j,s,id)
 h:=sha256.Sum256([]byte(j.WorldSeed+"/pilot-v9/initiative/"+id))
 // Active plans receive a daily opportunity. Other personal goals use
 // staggered windows and heterogeneous cadences, even in the same tier.
 if (day-1+int64(h[0])%p.cadence)%p.cadence!=0{return false}
 minute:=s.Minute%1440
 // A first-day opportunity permits purposeful introductions before class.
 if day==1&&p.cadence==1&&minute>=480+int64(h[1]%25)&&minute<540{return true}
 return minute>=900+int64(h[1]%60)&&minute<1080
}

// The historical scheduler counted staff opportunities too. Keep accounting
// distinct from the V8/V9 student-only endogenous routing predicate.
func initiativeAccountingDue(j Journal,s State,id string)bool{
 if j.Version<AutonomyVersion{return lightInitiativeDue(s,id)}
 return initiativeDue(j,s,id)
}

func nextCommitmentDeparture(j Journal,s State,id string) int64 {
 if s.World==nil{return 0}
 at:=int64(0)
 m:=s.Minds[id]
 for _,mt:=range s.World.Meetings{
  if mt.Status!="accepted"||(mt.Host!=id&&mt.Guest!=id)||mt.At<=s.Minute{continue}
  if mt.Host==id&&mt.HostAttended||mt.Guest==id&&mt.GuestAttended{continue}
  travel:=walkMinutes(m.Location,mt.Location)
  if m.Location==mt.Location{travel=0}
  depart:=mt.At-travel
  if depart>s.Minute&&(at==0||depart<at){at=depart}
 }
 return at
}

func autonomousRoutine(j Journal,s State,id string) Decision {
 a,m:=j.Population[id],s.Minds[id]
 d:=Decision{Action:"wait",WaitMinutes:60,Source:routineSource}
 choose:=func(action,target string)Decision{
  return Decision{Action:action,Target:target,Source:routineSource}
 }
 waitFor:=func(minutes int64)Decision{
  if minutes<1{minutes=1};if minutes>60{minutes=60}
  d.WaitMinutes=int(minutes);return d
 }
 if j.Version>=StaffCareVersion&&a.Role!="student"{return staffCareDecision(j,s,id)}
 if m.NextDeliberation>s.Minute{return waitFor(m.NextDeliberation-s.Minute)}
 if at:=nextCommitmentDeparture(j,s,id);at>s.Minute&&at-s.Minute<60{d.WaitMinutes=int(at-s.Minute)}
 v:=Context{Location:m.Location,Locations:worldLocations(id),School:schoolView(j,s,id)}
 projectPopulation(j,s,id,&v)
 offered:=func(action,target string)bool{
  for _,o:=range v.School.Options{if o.Action==action&&o.Target==target{return true}}
  return false
 }
 goOr:=func(dest,action,target string)Decision{
  if dest!=m.Location&&has(v.Locations,dest){return choose("move",dest)}
  if dest==m.Location&&offered(action,target){return choose(action,target)}
  return d
 }
 for _,o:=range v.School.Options{if o.Action=="attend"{return choose(o.Action,o.Target)}}
 // Accepted appointments are persistent commitments, not a prompt poll.
 // Pick the earliest reachable one; the ledger owns acceptance/status.
 var meeting *Meeting
 for _,mt:=range v.School.Meetings{
  if mt.Status!="accepted"||mt.At+10<s.Minute{continue}
  if mt.Host==id&&mt.HostAttended||mt.Guest==id&&mt.GuestAttended{continue}
  if meeting==nil||mt.At<meeting.At{x:=mt;meeting=&x}
 }
 if meeting!=nil{
  travel:=walkMinutes(m.Location,meeting.Location)
  if m.Location==meeting.Location{travel=0}
  if s.Minute+travel>=meeting.At-15&&s.Minute+travel<=meeting.At+10{
   if travel>0&&has(v.Locations,meeting.Location){return choose("move",meeting.Location)}
  }
  if travel==0&&meeting.At>s.Minute&&meeting.At-s.Minute<=15{return waitFor(meeting.At-s.Minute)}
 }
 period:=periodAt(s.Minute)
 if a.Role!="student"{
  if period=="repouso"{return goOr("staffroom","wait","")}
  return goOr(a.Station,"work","")
 }
 if m.Body!=nil{
  // Use existing consumables before walking; food/fluids are applied
  // only by the world action resolver. Severe needs override lessons.
  if m.Body.Hydration<350&&offered("use","water"){return choose("use","water")}
  if m.Body.Energy<350&&offered("use","snack"){return choose("use","snack")}
  if m.Body.Energy<350||m.Body.Hydration<350{return goOr("cafeteria","eat","free")}
  if m.Body.Fatigue>850{return goOr("dormitory-"+id,func()string{if period=="repouso"{return "sleep"};return "rest"}(),"")}
 }
 classroom:="class-"+strings.ToLower(classOf(j,id));if a.Year>1{classroom=seniorClassroom(classOf(j,id))}
 switch period{
 case "aulas da manhã","aulas da tarde":
  action:="lesson";if a.Year>1{action="study"}
  return goOr(classroom,action,"")
 case "almoço":
  if m.Body==nil||m.Body.Energy<700||m.Body.Hydration<650{return goOr("cafeteria","eat","free")}
  // Once satiated, the actor can spend the remainder of lunch resting
  // rather than repeatedly purchasing/consuming meals.
  return goOr("courtyard","relax","")
 case "repouso":
  return goOr("dormitory-"+id,"sleep","")
 case "jantar e repouso":
  if m.Body==nil||m.Body.Energy<550||m.Body.Hydration<550{return goOr("cafeteria","eat","free")}
  return goOr("dormitory-"+id,"rest","")
 case "atividades livres":
  // A modest, infrequent discretionary purchase is a versioned fallback
  // prior, not a canonical spending habit. Preserve an ample balance.
  day:=s.Minute/1440+1
  h:=sha256.Sum256([]byte(fmt.Sprintf("%s/pilot-v9/leisure/%s/%d",j.WorldSeed,id,day)))
  if m.LastRoutinePurchaseDay!=day&&m.Wallet>=10000&&h[0]%7==0&&s.Minute%1440<1020&&(meeting==nil||meeting.At-s.Minute>60){
   return goOr("mall","recreation","")
  }
  switch personalPrior(j,s,id).leisure{
  case "read":return goOr("library","read","")
  case "exercise":
   if m.Body!=nil&&(m.Body.Energy<600||m.Body.Hydration<600||m.Body.Fatigue>500){return goOr("park","relax","")}
   return goOr("gym","exercise","")
  case "socialize":return goOr("courtyard","socialize","")
  default:return goOr("park","relax","")
  }
 default:
  return goOr(classroom,"study","")
 }
}

// Only simultaneous, completed and co-located leisure sessions create a
// shared encounter. No scripted speech, inferred trust or forced friendship.
func completeSharedLeisure(j Journal,s *State){
 groups:=[]string{}
 for _,id:=range ids(j.Profiles){
  m:=s.Minds[id]
  if m.Activity!=nil&&m.Activity.Kind=="task"&&m.Activity.Target=="socialize:"&&m.Activity.Until==s.Minute&&m.Location=="courtyard"{groups=append(groups,id)}
 }
 for i,a:=range groups{
  for _,b:=range groups[i+1:]{
   day:=s.Minute/1440+1
   if s.Minds[a].Social[b].LastSharedLeisureDay==day{continue}
   for _,pair:=range [][2]string{{a,b},{b,a}}{
    m:=s.Minds[pair[0]]
    if m.Social==nil{m.Social=map[string]SocialBond{}}
    bond:=m.Social[pair[1]]
    bond.SharedLeisureDays++
    bond.LastSharedLeisureDay=day
    m.Social[pair[1]]=bond
    s.Minds[pair[0]]=m
   }
   appendEvent(s,Event{Kind:"social",Actor:a,Target:b,Text:"Estiveram no pátio durante uma atividade de convivência no mesmo período.",Audience:[]string{a,b}})
  }
 }
}

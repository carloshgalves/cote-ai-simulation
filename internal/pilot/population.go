package pilot
import (
 _ "embed"
 "encoding/json"
 "errors"
 "fmt"
 "reflect"
 "strings"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
)
const FullTier="FULL"
const LightTier="LIGHTWEIGHT"
const LatentTier="LATENT"
const CohortTier="COHORT"
const maxFull=18
const routineSource="motor/rotina"
//go:embed population.json
var populationData []byte
type PopulationMember struct{
 Role string `json:"role"`
 Year int `json:"year"`
 InitialTier string `json:"initial_tier"`
 Core bool `json:"core,omitempty"`
 Source string `json:"source"`
 Station string `json:"station,omitempty"`
}
type PublicMember struct{
 Role string `json:"role"`
 Year int `json:"year"`
 Tier string `json:"tier,omitempty"`
}
type PopulationEntry struct{Profile Profile `json:"profile"`; Member PopulationMember `json:"member"`}
type Cohort struct{
 ID string `json:"id"`
 Class string `json:"class"`
 Count int `json:"count"`
 Location string `json:"location"`
 Tier string `json:"tier"`
}
func expandPopulation(j *Journal)error{
 var entries []PopulationEntry
 if err:=json.Unmarshal(populationData,&entries);err!=nil{return err}
 j.Population=map[string]PopulationMember{}
 for _,p:=range j.Profiles{j.Population[p.ID]=PopulationMember{Role:"student",Year:1,InitialTier:FullTier,Core:true,Source:canonFor(p.ID).Source}}
 for _,e:=range entries{j.Profiles=append(j.Profiles,e.Profile);j.Population[e.Profile.ID]=e.Member}
 return validateJournalProfiles(*j)
}
func validateJournalProfiles(j Journal)error{
 if j.Version<PopulationVersion{return validateProfiles(j.Profiles)}
 if len(j.Profiles)<6||len(j.Profiles)>160||len(j.Population)!=len(j.Profiles){return errors.New("invalid population size")}
 seen:=map[string]bool{};core:=0;full:=0
 for _,p:=range j.Profiles{
  a,ok:=j.Population[p.ID]
  if !ok||seen[p.ID]||domain.ValidateMachineID(p.ID)!=nil||!validText(p.Name,100)||!validText(a.Source,300)||!has([]string{FullTier,LightTier,LatentTier},a.InitialTier)||!has([]string{"student","teacher","staff"},a.Role)||len(p.Goals)<1||len(p.Goals)>5||len(p.Traits)<1||len(p.Traits)>8||len(p.Background)>8{return errors.New("invalid population member")}
  if a.Role=="student"{if a.Year<1||a.Year>3||(!has([]string{"A","B","C","D"},p.Class)&&!(a.Year>1&&p.Class=="")){return errors.New("invalid student")}}else if a.Year!=0||a.Station==""{return errors.New("invalid staff")}
  if a.Core{core++;if a.Role!="student"||a.Year!=1||a.InitialTier!=FullTier{return errors.New("invalid core")}}
  if a.InitialTier==FullTier{full++}
  for _,list:=range [][]string{p.Goals,p.Traits,p.Background}{for _,x:=range list{if !validText(x,500){return errors.New("invalid profile text")}}}
  seen[p.ID]=true
 }
 if core!=6||full>maxFull{return errors.New("invalid full population")}
 return nil
}
func isStudent(j Journal,id string)bool{return j.Population[id].Role=="student"}
func staffStation(j Journal,id string)string{return j.Population[id].Station}
func initializePopulation(j Journal,s *State){
 for _,id:=range ids(j.Profiles){
  a:=j.Population[id];m:=s.Minds[id];m.Tier=a.InitialTier
  if !a.Core{
   if a.Role=="student"{m.Location="class-"+strings.ToLower(classOf(j,id));if a.Year>1{m.Location=seniorClassroom(classOf(j,id))}}else{m.Location=a.Station}
  }
  s.Minds[id]=m
 }
 updateCohorts(j,s)
}
func publicPopulation(j Journal,s State)map[string]PublicMember{
 if j.Version<PopulationVersion{return nil}
 out:=map[string]PublicMember{}
 for id,a:=range j.Population{out[id]=PublicMember{Role:a.Role,Year:a.Year,Tier:s.Minds[id].Tier}}
 return out
}
func projectPopulation(j Journal,s State,id string,c *Context){
 a:=j.Population[id];c.SchoolYear=a.Year
 c.PeerRelationship="Ano e função constam do cadastro público. Classe não indica senioridade."
 c.Population=publicPopulation(j,s)
 for k,v:=range c.Population{v.Tier="";c.Population[k]=v}
 if !a.Core{c.OwnCanon=CanonProjection{Source:a.Source,SchoolYear:a.Year,Status:"Identidade provisória; sem episódios posteriores à divergência"}}
 c.Locations=append(c.Locations,"class-a","staffroom","senior-class-a","senior-class-b","senior-class-c","senior-class-d","senior-common")
 if a.Role!="student"{
  filtered:=[]string{};for _,loc:=range c.Locations{if !strings.HasPrefix(loc,"dormitory-"){filtered=append(filtered,loc)}};c.Locations=filtered
 }else{
  filtered:=[]string{};for _,loc:=range c.Locations{if loc!="staffroom"{filtered=append(filtered,loc)}};c.Locations=filtered
 }
 for _,co:=range s.Cohorts{if co.Location==c.Location{co.Tier="";c.NearbyCohorts=append(c.NearbyCohorts,co)}}
}
func sameYearPeer(c Context,target string)bool{
 if c.Version<PopulationVersion{return true}
 a,ok:=c.Population[target];self:=c.Population[c.Self.ID];return ok&&a.Role=="student"&&self.Role=="student"&&a.Year==self.Year
}
func needsModel(s State,id string)bool{
 m:=s.Minds[id]
 if m.Tier==FullTier{return m.Body==nil||!m.AwaitEvent||hasCognitiveStimulus(s,id)}
 if m.Tier!=LightTier{return false}
 if m.Body!=nil&&hasCognitiveStimulus(s,id){return true}
 if m.Conversation!=nil&&m.Conversation.Turn==id{return true}
 if s.World!=nil{for _,mt:=range s.World.Meetings{if mt.Guest==id&&mt.Status=="pending"&&s.Minute<mt.At{return true}}}
 return m.WakeReason=="Recebeu convite"||m.WakeReason=="Atualização de compromisso"||m.WakeReason=="Compromisso agendado"||lightInitiativeDue(s,id)
}
func equalDecision(a,b Decision)bool{return reflect.DeepEqual(a,b)}
func routineDecision(j Journal,s State,id string)Decision{
 if j.Version>=AutonomyVersion{return autonomousRoutine(j,s,id)}
 a,m:=j.Population[id],s.Minds[id]
 d:=Decision{Action:"wait",WaitMinutes:60,Source:routineSource}
 // A public schedule boundary may shorten a previously chosen deliberate
 // wait. Do not replace its remaining interval with an unrelated task.
 if j.Version>=SelectiveVersion&&m.NextDeliberation>s.Minute{
  remaining:=m.NextDeliberation-s.Minute
  if remaining<60{d.WaitMinutes=int(remaining)}
  return d
 }
 v:=Context{Location:m.Location,Locations:worldLocations(id),School:schoolView(j,s,id)}
 projectPopulation(j,s,id,&v)
 choose:=func(action,target string)Decision{d.Action=action;d.Target=target;d.WaitMinutes=0;return d}
 if v.School!=nil{for _,o:=range v.School.Options{if o.Action=="attend"{return choose(o.Action,o.Target)}}}
 if j.Version>=CognitionVersion&&a.Role=="student"&&m.Body!=nil{
  if m.Body.Energy<350||m.Body.Hydration<350{
   if m.Location!="cafeteria"{return choose("move","cafeteria")}
   return choose("eat","free")
  }
  if m.Body.Fatigue>850{
   if m.Location!="dormitory-"+id{return choose("move","dormitory-"+id)}
   if periodAt(s.Minute)=="repouso"{return choose("sleep","")}
   return choose("rest","")
  }
 }
 period:=periodAt(s.Minute)
 dest:="";action:="";target:=""
 if a.Role!="student"{dest=a.Station;if period=="aulas da manhã"||period=="aulas da tarde"||a.Role=="staff"&&period!="repouso"{action="work"}else if period=="repouso"{dest="staffroom"}}else{
  classroom:="class-"+strings.ToLower(classOf(j,id));if a.Year>1{classroom=seniorClassroom(classOf(j,id))}
  switch period{
  case "aulas da manhã","aulas da tarde":dest=classroom;action="lesson";if a.Year>1{action="study"}
  case "almoço":dest="cafeteria";action="eat";target="free"
  case "jantar e repouso":dest="dormitory-"+id;action="rest"
  case "repouso":dest="dormitory-"+id;action="sleep"
  case "atividades livres":if a.InitialTier==LightTier{dest="library";action="study"}else{dest=classroom}
  default:dest=classroom
  }
 }
 if dest!=""&&dest!=m.Location&&has(v.Locations,dest){return choose("move",dest)}
 if v.School!=nil{for _,o:=range v.School.Options{if o.Action==action&&o.Target==target{return choose(action,target)}}}
 return d
}
func wakeOnMovement(j Journal,s *State,id,actor,reason string){
 if j.Version>=CognitionVersion{
  if strings.Contains(reason,"chegou")&&recognizes(j,id,actor)&&s.Minds[id].Social[actor].Interactions==0{stimulate(s,id,"Presença de pessoa conhecida antes da matrícula");return}
  if j.Version>=SelectiveVersion||s.Minds[id].AwaitEvent{return}
 }
 if j.Version>=PopulationVersion&&(s.Minds[actor].Tier!=FullTier||s.Minds[id].Tier!=FullTier){return}
 wakeWaiting(s,id,reason)
}
func engage(j Journal,s *State,from,to string){
 for _,id:=range []string{from,to}{
  m:=s.Minds[id];m.LastEngagement=s.Minute;m.Engagements++
  old:=m.Tier
  if old==LatentTier{m.Tier=LightTier}else if old==LightTier&&strategicPromotion(s,id,m){
   full:=0;for _,x:=range s.Minds{if x.Tier==FullTier{full++}}
   if full<maxFull{m.Tier=FullTier}
  }
  s.Minds[id]=m
  if old!=m.Tier{appendEvent(s,Event{Kind:"resolution",Actor:id,Text:fmt.Sprintf("%s → %s: interação concluída",old,m.Tier),Audience:[]string{id}})}
 }
}
func updatePopulation(j Journal,s *State){
 for _,id:=range ids(j.Profiles){
  a,m:=j.Population[id],s.Minds[id]
  if a.Core||a.InitialTier==FullTier||m.Conversation!=nil{continue}
  old:=m.Tier
  if m.Tier==FullTier&&s.Minute-m.LastEngagement>=2880{m.Tier=LightTier;m.Engagements=0}
  if m.Tier==LightTier&&a.InitialTier==LatentTier&&s.Minute-m.LastEngagement>=1440{m.Tier=LatentTier;m.Engagements=0}
  s.Minds[id]=m
  if old!=m.Tier{appendEvent(s,Event{Kind:"resolution",Actor:id,Text:old+" → "+m.Tier+": inatividade",Audience:[]string{id}})}
 }
 updateCohorts(j,s)
}
func updateCohorts(j Journal,s *State){
 s.Cohorts=[]Cohort{}
 for _,class:=range []string{"A","B","C","D"}{
  n:=40
  for _,p:=range j.Profiles{a:=j.Population[p.ID];if a.Role=="student"&&a.Year==1&&p.Class==class{n--}}
  if n<=0{continue}
  loc:="class-"+strings.ToLower(class)
  switch periodAt(s.Minute){case "almoço":loc="cafeteria";case "repouso","jantar e repouso":loc="residential-area";case "atividades livres":loc="courtyard"}
  s.Cohorts=append(s.Cohorts,Cohort{ID:"cohort-y1-"+strings.ToLower(class),Class:class,Count:n,Location:loc,Tier:CohortTier})
 }
}

func activateInvite(s *State,id string){
 m:=s.Minds[id]
 if m.Tier==LatentTier{m.Tier=LightTier;m.LastEngagement=s.Minute;s.Minds[id]=m
 appendEvent(s,Event{Kind:"resolution",Actor:id,Text:"LATENT → LIGHTWEIGHT: convite recebido",Audience:[]string{id}})}
}

func equalMind(a,b Mind)bool{return reflect.DeepEqual(a,b)}

func seniorClassroom(class string)string{if class==""{return "senior-common"};return "senior-class-"+strings.ToLower(class)}
// At most one autonomous free-period consultation per day, in addition to
// addressed conversations and commitments. No repeated polling during routine.
func lightInitiativeDue(s State,id string)bool{
 m:=s.Minds[id];return periodAt(s.Minute)=="atividades livres"&&m.LastLightInitiativeDay!=s.Minute/1440+1
}
func strategicPromotion(s *State,id string,m Mind)bool{
 if m.Engagements<6||m.Plan==nil||len(m.Plan.Steps)==0{return false}
 peers:=map[string]bool{}
 for i:=len(s.Events)-1;i>=0;i--{e:=s.Events[i];if s.Minute-e.Minute>2880{break};if e.Kind!="claim"{continue};if e.Actor==id&&e.Target!=id{peers[e.Target]=true};if e.Target==id&&e.Actor!=id{peers[e.Actor]=true}}
 return len(peers)>=3
}

package pilot

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/clock"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
)

const Version = 2
const ScheduledVersion = 3
const ConversationVersion = 4
const WorldVersion = 5
const PopulationVersion = 6
const CognitionVersion = 7
const SelectiveVersion = 8
const AutonomyVersion = 9
const StaffCareVersion = 10
const ContactVersion = 11
const MotivationVersion = 12
const ObservationVersion = 13
const VeteranVersion = 14
const PhysicalVersion = 15
const CompactContextVersion = 16
const CalibratedVersion = 17
const CrossoverVersion = 18
const CalendarVersion = 19
const PortfolioVersion = 20
const MultiresolutionVersion = 21
const LegacyVersion = 1
const EndMinute = 540
const MaxRounds = 200
const RoundMinutes = 5

type Profile struct {
	ID string `json:"id"`
	Name string `json:"name"`
	Class string `json:"class"`
	Traits []string `json:"traits"`
	Background []string `json:"background"`
	Goals []string `json:"goals"`
}

type Person struct {
	ID string `json:"id"`
	Name string `json:"name"`
	Class string `json:"class"`
}
type Presence struct {
	Person
	Location string `json:"location"`
}
type Belief struct {
	Text string `json:"text"`
	Evidence string `json:"evidence"`
	Confidence int `json:"confidence"`
	Subject string `json:"subject,omitempty"`
	Grounding string `json:"grounding,omitempty"`
}
type GoalChange struct {
	Replace string `json:"replace"`
	With string `json:"with"`
	Evidence string `json:"evidence"`
}
type RelationChange struct {
	Person string `json:"person"`
	Delta int `json:"delta"`
	Evidence string `json:"evidence"`
}
type PlanChange struct {
	Steps []string `json:"steps"`
	Evidence string `json:"evidence"`
	ProgressNote string `json:"progress_note,omitempty"`
 Horizon string `json:"horizon,omitempty"`
 Deadline int64 `json:"deadline,omitempty"`
 Scope string `json:"scope,omitempty"`
}
type Decision struct {
	Action string `json:"action"`
	Target string `json:"target"`
	Text string `json:"text"`
	Reason string `json:"reason"`
	Beliefs []Belief `json:"beliefs"`
	Goal *GoalChange `json:"goal"`
	Relation *RelationChange `json:"relation"`
	Plan *PlanChange `json:"plan"`
	Ignored []string `json:"ignored,omitempty"`
	Note string `json:"note,omitempty"`
	Source string `json:"source,omitempty"`
	WaitMinutes int `json:"wait_minutes,omitempty"`
	Discovery *Discovery `json:"discovery,omitempty"`

}
type Event struct {
	ID string `json:"id"`
	Round int `json:"round"`
	Minute int64 `json:"minute"`
	Kind string `json:"kind"`
	Actor string `json:"actor"`
	Target string `json:"target,omitempty"`
	Text string `json:"text"`
	Audience []string `json:"audience"`
	Evidence string `json:"evidence,omitempty"`
}
type Mind struct {
	Location string `json:"location"`
	Goals []string `json:"goals"`
	Beliefs []Belief `json:"beliefs"`
	Relations map[string]int `json:"relations"`
	Plan *PlanChange `json:"plan"`
	Thought string `json:"thought,omitempty"`
	LastAction string `json:"last_action,omitempty"`
	Activity *Activity `json:"activity,omitempty"`
	WakeReason string `json:"wake_reason,omitempty"`
	Conversation *Conversation `json:"conversation,omitempty"`
	Wallet int64 `json:"wallet,omitempty"`
	Inventory map[string]int `json:"inventory,omitempty"`
	Discoveries []Discovery `json:"discoveries,omitempty"`
 Tier string `json:"tier,omitempty"`
 Engagements int `json:"engagements,omitempty"`
 LastEngagement int64 `json:"last_engagement,omitempty"`
 LastLightInitiativeDay int64 `json:"last_light_initiative_day,omitempty"`
 Body *PilotBody `json:"body,omitempty"`
 Social map[string]SocialBond `json:"social,omitempty"`
 Plans map[string]PlanChange `json:"plans,omitempty"`
 AwaitEvent bool `json:"await_event,omitempty"`
 PendingStimulus string `json:"pending_stimulus,omitempty"`
 NextDeliberation int64 `json:"next_deliberation,omitempty"`
 LastRoutinePurchaseDay int64 `json:"last_routine_purchase_day,omitempty"`
 LastOpportunityMinute int64 `json:"last_opportunity_minute,omitempty"`
 LastDeliberationMinute int64 `json:"last_deliberation_minute,omitempty"`
 OpportunityDay int64 `json:"opportunity_day,omitempty"`
 OpportunityCount int `json:"opportunity_count,omitempty"`
}
type Chosen struct {
	Actor string `json:"actor"`
	Decision Decision `json:"decision"`
	Failure string `json:"failure,omitempty"`
}
type Round struct {
	Number int `json:"number"`
	Minute int64 `json:"minute"`
	Choices []Chosen `json:"choices"`
}
type Journal struct {
	Version int `json:"version"`
	Profiles []Profile `json:"profiles"`
	Model string `json:"model"`
	Rounds []Round `json:"rounds"`
 WorldSeed string `json:"world_seed,omitempty"`
 Population map[string]PopulationMember `json:"population,omitempty"`
 Identities map[string]CharacterIdentity `json:"identities,omitempty"`
}
type State struct {
	Round int `json:"round"`
	Minute int64 `json:"minute"`
	Minds map[string]Mind `json:"minds"`
	Events []Event `json:"events"`
	World *SchoolWorld `json:"world,omitempty"`
 Cohorts []Cohort `json:"cohorts,omitempty"`
}
type Context struct {
	Round int `json:"round"`
	Minute int64 `json:"minute"`
	Self Profile `json:"self"`
	Location string `json:"location"`
	People []Person `json:"people"`
	Present []Presence `json:"present"`
	SchoolYear int `json:"school_year"`
	PeerRelationship string `json:"peer_relationship"`
	OwnCanon CanonProjection `json:"own_canon"`
	Locations []string `json:"locations"`
	Goals []string `json:"goals"`
	Beliefs []Belief `json:"beliefs"`
	Relations map[string]int `json:"relations"`
	Memory []Event `json:"memory"`
	Plan *PlanChange `json:"plan"`
	Thought string `json:"thought,omitempty"`
	LastAction string `json:"last_action,omitempty"`
	CanObserve bool `json:"can_observe"`
	WakeReason string `json:"wake_reason,omitempty"`
	Activity *Activity `json:"activity,omitempty"`
	Conversation *Conversation `json:"conversation,omitempty"`
	Contactable []string `json:"contactable,omitempty"`
	Version int `json:"version"`
	School *SchoolView `json:"school,omitempty"`
 Discoveries []Discovery `json:"discoveries,omitempty"`
 Population map[string]PublicMember `json:"population,omitempty"`
 NearbyCohorts []Cohort `json:"nearby_cohorts,omitempty"`
 Identity *CharacterIdentity `json:"identity,omitempty"`
 Recognized []string `json:"recognized,omitempty"`
 Opportunity *PersonalOpportunity `json:"opportunity,omitempty"`
 Interoception *PilotInteroception `json:"interoception,omitempty"`
 Body *PilotBody `json:"body,omitempty"`
 Social map[string]SocialBond `json:"social,omitempty"`
 Plans map[string]PlanChange `json:"plans,omitempty"`
}
type Model interface {
	Decide(context.Context, Context) (Decision,error)
}
var locations = []string{"hall","class-b","class-c","class-d","library","courtyard"}

func validText(s string,n int) bool {
	return utf8.ValidString(s) && strings.TrimSpace(s)!="" && utf8.RuneCountInString(s)<=n
}
func has(xs []string,x string) bool {
	for _, v := range xs { if x==v {return true} }
	return false
}
func clone[T any](x T) T {
	raw,err:=json.Marshal(x)
	if err!=nil { panic(err) }
	var result T
	if err=json.Unmarshal(raw,&result);err!=nil {panic(err)}
	return result
}
func known(p []Profile,id string) bool {
	for _, a:=range p {if a.ID==id {return true}}
	return false
}
func validateProfiles(p []Profile) error {
	if len(p)!=6 {return errors.New("pilot requires six characters")}
	seen:=map[string]bool{}
	for _, a:=range p {
		if domain.ValidateMachineID(a.ID)!=nil || seen[a.ID] || !validText(a.Name,100) ||
			!has([]string{"B","C","D"},a.Class) || len(a.Goals)<1 || len(a.Goals)>5 ||
			len(a.Traits)<1 || len(a.Traits)>8 || len(a.Background)>8 {return errors.New("invalid character profile")}
		seen[a.ID]=true
		for _, list:=range [][]string{a.Goals,a.Traits,a.Background} {for _, x:=range list {if !validText(x,500){return errors.New("invalid profile text")}}}
	}
	return nil
}
func initial(j Journal) (State,error) {
	if (j.Version!=Version&&j.Version!=LegacyVersion&&j.Version!=ScheduledVersion&&j.Version!=ConversationVersion&&j.Version!=WorldVersion&&j.Version!=PopulationVersion&&j.Version!=CognitionVersion&&j.Version!=SelectiveVersion&&j.Version!=AutonomyVersion&&j.Version!=StaffCareVersion&&j.Version!=ContactVersion&&j.Version!=MotivationVersion&&j.Version!=ObservationVersion&&j.Version!=VeteranVersion&&j.Version!=PhysicalVersion&&j.Version!=CompactContextVersion&&j.Version!=CalibratedVersion&&j.Version!=CrossoverVersion&&j.Version!=CalendarVersion&&j.Version!=PortfolioVersion&&j.Version!=MultiresolutionVersion) || !validText(j.Model,200) || validateJournalProfiles(j)!=nil {return State{},errors.New("unsupported journal")}
	if j.Version>=WorldVersion&&len(j.WorldSeed)!=64{return State{},errors.New("invalid world seed")}
 if j.Version>=CognitionVersion{if err:=validateIdentities(j);err!=nil{return State{},err}}
 s:=State{Minute:480,Minds:map[string]Mind{},Events:[]Event{}}
	for _, p:=range j.Profiles { s.Minds[p.ID]=Mind{Location:"hall",Goals:clone(p.Goals),Beliefs:[]Belief{},Relations:map[string]int{}};if j.Version>=ScheduledVersion{m:=s.Minds[p.ID];m.WakeReason="Início do período livre";s.Minds[p.ID]=m} }
 if j.Version>=PopulationVersion{
 initializePopulation(j,&s)
 appendEvent(&s,Event{Kind:"announcement",Actor:"school",Text:"Primeiro dia. Estudantes, professores e funcionários têm rotinas próprias. Cadastro de nomes e funções disponível.",Audience:ids(j.Profiles)})
 }else{	appendEvent(&s,Event{Kind:"announcement",Actor:"school",Text:"Primeiro dia. A escola abriu o hall, as salas, a biblioteca e o pátio. Os estudantes têm um período livre para conhecer o ambiente e os colegas. A lista de nomes e classes dos seis participantes está disponível a todos. As regras detalhadas serão anunciadas depois.",Audience:ids(j.Profiles)})}
	if j.Version>=WorldVersion{initializeWorld(j,&s)}
 if j.Version>=CognitionVersion{initializeCognition(j,&s)}
 if j.Version>=VeteranVersion{initializeVeteranState(j,&s)}
 if j.Version>=SelectiveVersion{initializeSelectiveStimuli(j,&s)}
	return s,nil
}
func ids(p []Profile) []string {
	out:=[]string{};for _, a:=range p {out=append(out,a.ID)};sort.Strings(out);return out
}
func appendEvent(s *State,e Event) {
	e.Round=s.Round;e.Minute=s.Minute
	e.ID=fmt.Sprintf("e%06d",len(s.Events)+1)
	sort.Strings(e.Audience)
	s.Events=append(s.Events,e)
}
func visible(s State,id string) []Event {
	out:=[]Event{}
	for _, e:=range s.Events {if has(e.Audience,id) {out=append(out,e)}}
	return out
}
func actorContext(j Journal,s State,id string) Context {
	m:=s.Minds[id]
	c:=Context{OwnCanon:canonFor(id),SchoolYear:1,PeerRelationship:"Todos os participantes são alunos do primeiro ano; nenhuma senioridade decorre da classe.",Version:j.Version,CanObserve:true,Thought:m.Thought,LastAction:m.LastAction,Round:s.Round+1,Minute:s.Minute,Location:m.Location,Locations:clone(locations),Goals:clone(m.Goals),Beliefs:clone(m.Beliefs),Relations:clone(m.Relations),Plan:clone(m.Plan),Memory:[]Event{},People:[]Person{},Present:[]Presence{}}
	for _, p:=range j.Profiles {
		if p.ID==id {c.Self=clone(p);continue}
		person:=Person{p.ID,p.Name,p.Class}
		c.People=append(c.People,person)
		if s.Minds[p.ID].Location==m.Location{
			other:=s.Minds[p.ID]
			if j.Version>=ConversationVersion {
				if other.Activity==nil||other.Activity.Kind!="move"{c.Present=append(c.Present,Presence{person,m.Location})}
				if canContact(m,p.ID,other,id){c.Contactable=append(c.Contactable,p.ID)}
			}else if j.Version!=ScheduledVersion||availableForContact(other){c.Present=append(c.Present,Presence{person,m.Location})}
		}
	}
	if j.Version>=Version {c.CanObserve=observationChanged(s,id,roster(s,id))}
	if j.Version>=ScheduledVersion{c.WakeReason=m.WakeReason;c.Activity=clone(m.Activity);if j.Version>=ConversationVersion{c.Conversation=clone(m.Conversation)}}
	if j.Version>=WorldVersion{c.School=schoolView(j,s,id);c.Locations=worldLocations(id);c.Discoveries=clone(m.Discoveries)}
	if j.Version>=PopulationVersion{projectPopulation(j,s,id,&c)}
 if j.Version>=CognitionVersion{projectIdentity(j,s,id,&c)}
 if j.Version>=MotivationVersion{c.Opportunity=personalOpportunity(j,s,id)}
 // Recent lived memories plus older evidence currently referenced by cognition.
	// Do not retrieve any event outside the actor's audience.
	all:=visible(s,id)
	keep:=map[string]bool{}
	for _, b:=range m.Beliefs {keep[b.Evidence]=true}
	if m.Plan!=nil {keep[m.Plan.Evidence]=true}
 if j.Version>=CognitionVersion{for _,p:=range m.Plans{keep[p.Evidence]=true}}
	// Keep only the most recent goal-change evidence for each active goal.
	pinnedGoals:=map[string]bool{}
	for i:=len(all)-1;i>=0;i-- {
		e:=all[i]
		if e.Kind=="goal"&&e.Evidence!="" {
			for _, goal:=range m.Goals {
				if !pinnedGoals[goal]&&strings.HasSuffix(e.Text," → "+goal) {
					keep[e.Evidence]=true
					pinnedGoals[goal]=true
					break
				}
			}
		}
	}
	recent:=[]Event{}
	if j.Version>=WorldVersion{for _,d:=range m.Discoveries{keep[d.Evidence]=true}}
	for _, e:=range all {
		if (j.Version>=WorldVersion&&has([]string{"resource","exam","meeting","task","conversation_end","revelation","body_alert","plan_end","social"},e.Kind))||j.Version==LegacyVersion||e.Kind=="announcement"||e.Kind=="claim"||e.Kind=="move"||e.Kind=="departure"||e.Kind=="arrival"||e.Kind=="obstacle"||e.Kind=="observation"||keep[e.ID]{recent=append(recent,e)}
	}
	if j.Version>=CognitionVersion{c.Memory=retrieveMemory(j,s,id,recent,keep);if j.Version>=ObservationVersion{for i:=range c.Memory{if c.Memory[i].Kind=="observation"{c.Memory[i].Audience=[]string{id}}}}}else{for i,e:=range recent {if i>=len(recent)-20 || keep[e.ID] || (j.Version<WorldVersion&&e.Kind=="announcement") {c.Memory=append(c.Memory,clone(e))}}}
	return c
}
func evidence(c Context,id string) (Event,bool) {
	for _, e:=range c.Memory {if e.ID==id{return e,true}}
 if c.Version>=CognitionVersion&&c.Identity!=nil{for _,f:=range c.Identity.Background{if f.ID==id{return Event{ID:f.ID,Kind:"biography",Actor:c.Self.ID,Text:f.Text,Audience:[]string{c.Self.ID}},true}}}
	return Event{},false
}
func validateDecision(c Context,d Decision) error {
	if (d.Source!=""&&!validText(d.Source,300)) || (d.Note!=""&&!validText(d.Note,500)) || (d.Reason!=""&&!validText(d.Reason,300)) || len(d.Beliefs)>3 {return errors.New("invalid intention or belief count")}
	if len(d.Ignored)>8 {return errors.New("invalid ignored field count")}
	for _, key:=range d.Ignored{if !has([]string{"reason","text","beliefs","goal","relation","plan","discovery","campo extra"},key){return errors.New("invalid ignored field")}}
	if d.WaitMinutes!=0&&(c.Version<ScheduledVersion||d.Action!="wait"||d.WaitMinutes<1||d.WaitMinutes>60){return errors.New("invalid wait duration")}
	worldAction:=c.Version>=WorldVersion&&isWorldAction(d.Action)
	if d.Action!="wait" && d.Action!="observe" && d.Action!="move" && d.Action!="talk"&&!worldAction {return errors.New("unsupported action")}
	if worldAction {if err:=validateWorldAction(c,d);err!=nil{return err}
	} else if d.Action=="talk" {
		found:=false;for _, p:=range c.Present {if p.ID==d.Target {found=true}}
		if c.Version>=ConversationVersion&&!has(c.Contactable,d.Target){found=false}
		if !found || !validText(d.Text,800) {return errors.New("message requires a present recipient and text")}
	} else if d.Action=="move" {
		if !has(c.Locations,d.Target)||d.Target==c.Location||d.Text!="" {return errors.New("invalid movement")}
	} else if d.Target!="" || d.Text!="" {return errors.New("wait/observe cannot assert world events")}
	for _, b:=range d.Beliefs {
		e,ok:=evidence(c,b.Evidence)
		if !ok||!validText(b.Text,300)||b.Confidence<0||b.Confidence>100 {return errors.New("belief requires accessible evidence")}
		if c.Version>=ConversationVersion {
			if b.Subject!=""{
				found:=false;for _,p:=range c.People{if p.ID==b.Subject{found=true}}
				if !found{return errors.New("colega da hipótese desconhecido")}
			}
			if e.Kind=="announcement"&&b.Subject!=""{return errors.New("anúncio geral não fundamenta hipótese sobre colega")}
			if b.Subject!=""&&(e.Actor!=b.Subject&&e.Target!=b.Subject){return errors.New("evidência não envolve o colega indicado")}
			if !validText(b.Grounding,240)||len(strings.Fields(b.Grounding))<2||!strings.Contains(e.Text,b.Grounding){return errors.New("hipótese requer trecho literal da evidência")}
			if e.Kind=="claim"&&b.Confidence>70{return errors.New("alegação não confirmada não sustenta confiança acima de 70")}
		}
	}
	if d.Discovery!=nil{
		if c.Version<WorldVersion||!validDiscovery(c,*d.Discovery){return errors.New("descoberta requer acontecimento e trecho literal")}
	}
	if d.Goal!=nil {
		g:=d.Goal
		if _,ok:=evidence(c,g.Evidence);!ok||!has(c.Goals,g.Replace)||has(c.Goals,g.With)||!validText(g.With,180) {return errors.New("goal change requires accessible evidence and existing goal")}
	}
	if d.Plan!=nil {
		if _,ok:=evidence(c,d.Plan.Evidence);!ok||len(d.Plan.Steps)>5{return errors.New("plan requires accessible evidence")}
		previous:=c.Plan
  if c.Version>=CognitionVersion{
   if err:=validateHorizon(c,*d.Plan);err!=nil{return err}
   previous=nil;if p,ok:=c.Plans[planHorizon(*d.Plan)];ok{previous=&p}
  }else if d.Plan.Horizon!=""||d.Plan.Deadline!=0||d.Plan.Scope!=""{return errors.New("plan extension requires v7")}
  if c.Version>=ConversationVersion&&previous!=nil&&strings.Join(previous.Steps,"\x00")!=strings.Join(d.Plan.Steps,"\x00")&&!validText(d.Plan.ProgressNote,240){return errors.New("explique a mudança ou conclusão do plano")}
		for _, step:=range d.Plan.Steps {if !validText(step,180){return errors.New("invalid plan step")}}
	}
	if d.Relation!=nil {
		r:=d.Relation;e,ok:=evidence(c,r.Evidence)
		if !ok||r.Person==c.Self.ID||r.Delta < -1||r.Delta>1||r.Delta==0||
			(e.Actor!=r.Person&&e.Target!=r.Person) {return errors.New("relationship change requires relevant experienced event")}
		found:=false;for _, p:=range c.People {if p.ID==r.Person {found=true}}
		if !found{return errors.New("unknown relationship target")}
	}
	return nil
}
// A bounded, authored school morning; no claim these events are canon.
var schoolAgenda = []struct{ Minute int64; Text string }{
	{490,"Anúncio de orientação: os estudantes podem visitar as instalações e trocar informações entre turmas. A escola convida cada aluno a conhecer pelo menos um colega de outra classe antes da próxima orientação. A participação é voluntária."},
	{505,"A escola anuncia que, às 09:00, haverá uma reunião de orientação nas salas de cada turma. Até lá, os estudantes podem conversar e organizar seus planos. Nenhuma pontuação é atribuída nesta atividade."},
	{540,"O período livre terminou. A reunião de orientação começa; este piloto da manhã se encerra."},
}
func roster(s State,id string) string {
	m:=s.Minds[id]
	names:=[]string{}
	for other, mind:=range s.Minds {if other!=id&&mind.Location==m.Location&&(mind.Activity==nil||mind.Activity.Kind!="move"){names=append(names,other)}}
	sort.Strings(names)
	return "Presentes em "+m.Location+": "+strings.Join(names,", ")
}
func observationChanged(s State,id,text string) bool {
	for i:=len(s.Events)-1;i>=0;i--{
		e:=s.Events[i]
		if e.Actor==id&&e.Kind=="observation"&&strings.HasPrefix(e.Text,"Presentes em "){return e.Text!=text}
	}
	return true
}
func nextWorld(s State) int64 {
	for _, e:=range schoolAgenda{if e.Minute>s.Minute{return e.Minute}}
	return EndMinute
}
func nextEventMinute(j Journal,s State,choices []Chosen)(int64,error){
	if j.Version>=ScheduledVersion {next,err:=scheduledAdvance(j,s,choices);return next.Minute,err}
	if j.Version==LegacyVersion{return nextMinute(s)}
	if s.Minute>=EndMinute{return 0,errors.New("manhã concluída")}
	target:=nextWorld(s)
	duration:=int64(0)
	for _, x:=range choices{
		if x.Failure!=""{continue}
		d:=x.Decision
		switch d.Action {
		case "talk":if duration<1{duration=1}
		case "move":if duration<2{duration=2}
		case "observe":if observationChanged(s,x.Actor,roster(s,x.Actor))&&duration<1{duration=1}
		}
	}
	if duration>0&&s.Minute+duration<target{target=s.Minute+duration}
	state:=clock.State{Instant:domain.SimulationInstant(s.Minute),Revision:domain.WorldRevision(s.Round),LastOrdinal:0}
	plan,ok,err:=clock.Next(state,[]clock.Pending{{Eligibility:domain.EligibilityCoordinate{NotBeforeInstant:domain.SimulationInstant(target)}}})
	if err!=nil||!ok||plan.Kind!=clock.ClockAdvance{return 0,errors.New("invalid event advance")}
	next,err:=clock.ApplyAdvance(state,plan)
	return int64(next.Instant),err
}
func nextMinute(s State) (int64,error) {
	// Reuse the existing CSF logical clock planner, not wall time.
	state:=clock.State{Instant:domain.SimulationInstant(s.Minute),Revision:domain.WorldRevision(s.Round),LastOrdinal:0}
	pending:=[]clock.Pending{{Eligibility:domain.EligibilityCoordinate{NotBeforeInstant:state.Instant+RoundMinutes}}}
	plan,ok,err:=clock.Next(state,pending)
	if err!=nil||!ok||plan.Kind!=clock.ClockAdvance{return 0,errors.New("invalid logical advance")}
	next,err:=clock.ApplyAdvance(state,plan)
	return int64(next.Instant),err
}
func apply(j Journal,s State,r Round) (State,error) {
	if j.Version>=ScheduledVersion {
		if r.Number!=s.Round+1||r.Number>roundLimit(j.Version){return State{},errors.New("invalid scheduled cycle")}
		next,err:=scheduledAdvance(j,s,r.Choices);if err!=nil{return State{},err}
		if next.Minute!=r.Minute{return State{},errors.New("invalid scheduled time")}
		return next,nil
	}
	minute,err:=nextEventMinute(j,s,r.Choices)
	if err!=nil||r.Number!=s.Round+1||r.Minute!=minute||r.Number>MaxRounds {return State{},errors.New("invalid round coordinate")}
	order:=ids(j.Profiles)
	if len(r.Choices)!=len(order){return State{},errors.New("incomplete round")}
	for i, x:=range r.Choices {
		if x.Actor!=order[i] {return State{},errors.New("noncanonical actor order")}
		if x.Failure!="" {
			d:=x.Decision
			if d.Action!="wait"||d.Target!=""||d.Text!=""||d.Reason!="Sem decisão válida do modelo"||len(d.Beliefs)!=0||d.Goal!=nil||d.Plan!=nil||d.Relation!=nil||len(d.Ignored)!=0||d.Note!=""||!validText(x.Failure,500){return State{},errors.New("invalid model failure marker")}
		}
		if err:=validateDecision(actorContext(j,s,x.Actor),x.Decision);err!=nil {return State{},fmt.Errorf("%s: %w",x.Actor,err)}
	}
	valid:=0
	for _,x:=range r.Choices{if x.Failure==""{valid++}}
	if valid==0{return State{},errors.New("nenhum agente produziu decisão válida")}
	// Validate the entire cohort on the immutable start-of-round snapshot.
	// Only then apply deterministic effects; no response sees another response.
	out:=clone(s);out.Round=r.Number;out.Minute=r.Minute
	for _, x:=range r.Choices {
		d:=x.Decision;m:=out.Minds[x.Actor]
		if x.Failure!="" {
			appendEvent(&out,Event{Kind:"model_failure",Actor:x.Actor,Text:x.Failure,Audience:[]string{}})
			continue
		}
		m=applyCognition(j,&out,x.Actor,m,d)
		switch d.Action {
		case "talk":
			appendEvent(&out,Event{Kind:"claim",Actor:x.Actor,Target:d.Target,Text:d.Text,Audience:[]string{x.Actor,d.Target}})
		case "move":
			// Movement is visible to people present at the origin or destination
			// at the beginning of the round. No same-round arrival advantage.
			appendEvent(&out,Event{Kind:"move",Actor:x.Actor,Target:d.Target,Text:"Deslocou-se de "+m.Location+" para "+d.Target,Audience:[]string{x.Actor}})
			for _, id:=range order {
				if id==x.Actor {continue}
				if s.Minds[id].Location==m.Location {
					appendEvent(&out,Event{Kind:"observation",Actor:x.Actor,Text:"Saiu de "+m.Location,Audience:[]string{id}})
				} else if s.Minds[id].Location==d.Target {
					appendEvent(&out,Event{Kind:"observation",Actor:x.Actor,Text:"Chegou a "+d.Target,Audience:[]string{id}})
				}
			}
			m.Location=d.Target
		case "observe":
			text:=roster(s,x.Actor)
			if j.Version==LegacyVersion||observationChanged(s,x.Actor,text){appendEvent(&out,Event{Kind:"observation",Actor:x.Actor,Text:text,Audience:[]string{x.Actor}})}
		case "wait":
			if j.Version==LegacyVersion{appendEvent(&out,Event{Kind:"wait",Actor:x.Actor,Text:"Aguardou.",Audience:[]string{x.Actor}})}
		}
		out.Minds[x.Actor]=m
	}
	if j.Version==Version {
		for _, e:=range schoolAgenda{if e.Minute==out.Minute{appendEvent(&out,Event{Kind:"announcement",Actor:"school",Text:e.Text,Audience:ids(j.Profiles)})}}
	}
	return out,nil
}
func Replay(j Journal) (State,error) {
	s,err:=initial(j);if err!=nil{return State{},err}
	if len(j.Rounds)>roundLimit(j.Version){return State{},errors.New("round limit exceeded")}
	for _, r:=range j.Rounds {s,err=apply(j,s,r);if err!=nil{return State{},err}}
	return s,nil
}
func Digest(j Journal) string {
	raw,_:=json.Marshal(j);sum:=sha256.Sum256(raw);return hex.EncodeToString(sum[:])
}

func applyCognition(j Journal,out *State,actor string,m Mind,d Decision)Mind {
		if len(d.Ignored)>0{appendEvent(out,Event{Kind:"ignored_cognition",Actor:actor,Text:strings.Join(d.Ignored,", "),Audience:[]string{}})}
		oldMind:=clone(m)
		if j.Version>=WorldVersion&&d.Discovery!=nil{
			found:=false;for _,old:=range m.Discoveries{if old.Evidence==d.Discovery.Evidence&&old.Text==d.Discovery.Text{found=true}}
			if !found{m.Discoveries=append(m.Discoveries,*d.Discovery);if len(m.Discoveries)>24{m.Discoveries=m.Discoveries[len(m.Discoveries)-24:]};appendEvent(out,Event{Kind:"discovery",Actor:actor,Text:d.Discovery.Text,Evidence:d.Discovery.Evidence,Audience:[]string{actor}})}
		}
		for _, b:=range d.Beliefs {
			found:=false;for i, old:=range m.Beliefs {if old.Text==b.Text {m.Beliefs[i]=b;found=true}}
			if !found {m.Beliefs=append(m.Beliefs,b)}
		}
		if len(m.Beliefs)>12 {m.Beliefs=m.Beliefs[len(m.Beliefs)-12:]}
		if d.Goal!=nil {for i,g:=range m.Goals {if g==d.Goal.Replace {m.Goals[i]=d.Goal.With}}}
		if d.Relation!=nil {
			v:=m.Relations[d.Relation.Person]+d.Relation.Delta
			if v>5 {v=5};if v< -5 {v= -5};m.Relations[d.Relation.Person]=v
		}
		if j.Version>=Version {
			thought:=d.Reason
			if d.Note!="" {if thought!=""{thought+=" · "};thought+=d.Note}
			if thought!=""&&thought!=m.Thought{appendEvent(out,Event{Kind:"intention",Actor:actor,Text:thought,Audience:[]string{actor}})}
			m.Thought=thought;m.LastAction=d.Action
			if d.Target!=""{m.LastAction+=":"+d.Target}
		} else if d.Reason!=""{appendEvent(out,Event{Kind:"intention",Actor:actor,Text:d.Reason,Audience:[]string{actor}})}
		if d.Goal!=nil {appendEvent(out,Event{Kind:"goal",Actor:actor,Text:d.Goal.Replace+" → "+d.Goal.With,Audience:[]string{actor},Evidence:d.Goal.Evidence})}
		if d.Plan!=nil {
			m.Plan=clone(d.Plan)
   if j.Version>=CognitionVersion{applyHorizon(&m,*d.Plan)}
			if j.Version==LegacyVersion||oldMind.Plan==nil||strings.Join(oldMind.Plan.Steps,"\x00")!=strings.Join(d.Plan.Steps,"\x00")||oldMind.Plan.Evidence!=d.Plan.Evidence||(j.Version>=WorldVersion&&oldMind.Plan.ProgressNote!=d.Plan.ProgressNote){appendEvent(out,Event{Kind:"plan",Actor:actor,Text:strings.Join(d.Plan.Steps," → "),Audience:[]string{actor},Evidence:d.Plan.Evidence})}
		}
		for _, b:=range d.Beliefs {
			same:=false;for _, old:=range oldMind.Beliefs{if old==b{same=true;break}}
			if j.Version==LegacyVersion||!same{appendEvent(out,Event{Kind:"belief",Actor:actor,Text:b.Text,Audience:[]string{actor},Evidence:b.Evidence})}
		}
		if d.Relation!=nil&&(j.Version==LegacyVersion||oldMind.Relations[d.Relation.Person]!=m.Relations[d.Relation.Person]) {appendEvent(out,Event{Kind:"relation",Actor:actor,Target:d.Relation.Person,Text:fmt.Sprintf("Relação: %+d",d.Relation.Delta),Audience:[]string{actor},Evidence:d.Relation.Evidence})}
	return m
}

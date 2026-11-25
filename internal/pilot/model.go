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

const Version = 1
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
}
type State struct {
	Round int `json:"round"`
	Minute int64 `json:"minute"`
	Minds map[string]Mind `json:"minds"`
	Events []Event `json:"events"`
}
type Context struct {
	Round int `json:"round"`
	Minute int64 `json:"minute"`
	Self Profile `json:"self"`
	Location string `json:"location"`
	People []Person `json:"people"`
	Present []Presence `json:"present"`
	Locations []string `json:"locations"`
	Goals []string `json:"goals"`
	Beliefs []Belief `json:"beliefs"`
	Relations map[string]int `json:"relations"`
	Memory []Event `json:"memory"`
	Plan *PlanChange `json:"plan"`
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
	if j.Version!=Version || !validText(j.Model,200) || validateProfiles(j.Profiles)!=nil {return State{},errors.New("unsupported journal")}
	s:=State{Minute:480,Minds:map[string]Mind{},Events:[]Event{}}
	for _, p:=range j.Profiles { s.Minds[p.ID]=Mind{Location:"hall",Goals:clone(p.Goals),Beliefs:[]Belief{},Relations:map[string]int{}} }
	appendEvent(&s,Event{Kind:"announcement",Actor:"school",Text:"Primeiro dia. A escola abriu o hall, as salas, a biblioteca e o pátio. Os estudantes têm um período livre para conhecer o ambiente e os colegas. A lista de nomes e classes dos seis participantes está disponível a todos. As regras detalhadas serão anunciadas depois.",Audience:ids(j.Profiles)})
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
	c:=Context{Round:s.Round+1,Minute:s.Minute,Location:m.Location,Locations:clone(locations),Goals:clone(m.Goals),Beliefs:clone(m.Beliefs),Relations:clone(m.Relations),Plan:clone(m.Plan),Memory:[]Event{},People:[]Person{},Present:[]Presence{}}
	for _, p:=range j.Profiles {
		if p.ID==id {c.Self=clone(p);continue}
		person:=Person{p.ID,p.Name,p.Class}
		c.People=append(c.People,person)
		if s.Minds[p.ID].Location==m.Location {c.Present=append(c.Present,Presence{person,m.Location})}
	}
	// Recent lived memories plus older evidence currently referenced by cognition.
	// Do not retrieve any event outside the actor's audience.
	all:=visible(s,id)
	keep:=map[string]bool{}
	for _, b:=range m.Beliefs {keep[b.Evidence]=true}
	if m.Plan!=nil {keep[m.Plan.Evidence]=true}
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
	for i,e:=range all {if i>=len(all)-20 || keep[e.ID] || e.Kind=="announcement" {c.Memory=append(c.Memory,clone(e))}}
	return c
}
func evidence(c Context,id string) (Event,bool) {
	for _, e:=range c.Memory {if e.ID==id{return e,true}}
	return Event{},false
}
func validateDecision(c Context,d Decision) error {
	if (d.Reason!=""&&!validText(d.Reason,300)) || len(d.Beliefs)>3 {return errors.New("invalid intention or belief count")}
	if len(d.Ignored)>7 {return errors.New("invalid ignored field count")}
	for _, key:=range d.Ignored{if !has([]string{"reason","text","beliefs","goal","relation","plan","campo extra"},key){return errors.New("invalid ignored field")}}
	if d.Action!="wait" && d.Action!="observe" && d.Action!="move" && d.Action!="talk" {return errors.New("unsupported action")}
	if d.Action=="talk" {
		found:=false;for _, p:=range c.Present {if p.ID==d.Target {found=true}}
		if !found || !validText(d.Text,800) {return errors.New("message requires a present recipient and text")}
	} else if d.Action=="move" {
		if !has(c.Locations,d.Target)||d.Target==c.Location||d.Text!="" {return errors.New("invalid movement")}
	} else if d.Target!="" || d.Text!="" {return errors.New("wait/observe cannot assert world events")}
	for _, b:=range d.Beliefs {
		if _,ok:=evidence(c,b.Evidence);!ok||!validText(b.Text,300)||b.Confidence<0||b.Confidence>100 {return errors.New("belief requires accessible evidence")}
	}
	if d.Goal!=nil {
		g:=d.Goal
		if _,ok:=evidence(c,g.Evidence);!ok||!has(c.Goals,g.Replace)||has(c.Goals,g.With)||!validText(g.With,180) {return errors.New("goal change requires accessible evidence and existing goal")}
	}
	if d.Plan!=nil {
		if _,ok:=evidence(c,d.Plan.Evidence);!ok||len(d.Plan.Steps)>5{return errors.New("plan requires accessible evidence")}
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
	minute,err:=nextMinute(s)
	if err!=nil||r.Number!=s.Round+1||r.Minute!=minute||r.Number>MaxRounds {return State{},errors.New("invalid round coordinate")}
	order:=ids(j.Profiles)
	if len(r.Choices)!=len(order){return State{},errors.New("incomplete round")}
	for i, x:=range r.Choices {
		if x.Actor!=order[i] {return State{},errors.New("noncanonical actor order")}
		if x.Failure!="" {
			d:=x.Decision
			if d.Action!="wait"||d.Target!=""||d.Text!=""||d.Reason!="Sem decisão válida do modelo"||len(d.Beliefs)!=0||d.Goal!=nil||d.Plan!=nil||d.Relation!=nil||len(d.Ignored)!=0||!validText(x.Failure,500){return State{},errors.New("invalid model failure marker")}
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
		if len(d.Ignored)>0{appendEvent(&out,Event{Kind:"ignored_cognition",Actor:x.Actor,Text:strings.Join(d.Ignored,", "),Audience:[]string{}})}
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
		if d.Reason!=""{appendEvent(&out,Event{Kind:"intention",Actor:x.Actor,Text:d.Reason,Audience:[]string{x.Actor}})}
		if d.Goal!=nil {appendEvent(&out,Event{Kind:"goal",Actor:x.Actor,Text:d.Goal.Replace+" → "+d.Goal.With,Audience:[]string{x.Actor},Evidence:d.Goal.Evidence})}
		if d.Plan!=nil {
			m.Plan=clone(d.Plan)
			appendEvent(&out,Event{Kind:"plan",Actor:x.Actor,Text:strings.Join(d.Plan.Steps," → "),Audience:[]string{x.Actor},Evidence:d.Plan.Evidence})
		}
		for _, b:=range d.Beliefs {appendEvent(&out,Event{Kind:"belief",Actor:x.Actor,Text:b.Text,Audience:[]string{x.Actor},Evidence:b.Evidence})}
		if d.Relation!=nil {appendEvent(&out,Event{Kind:"relation",Actor:x.Actor,Target:d.Relation.Person,Text:fmt.Sprintf("Relação: %+d",d.Relation.Delta),Audience:[]string{x.Actor},Evidence:d.Relation.Evidence})}
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
			names:=[]string{};for _, id:=range order {if id!=x.Actor&&s.Minds[id].Location==m.Location {names=append(names,id)}}
			appendEvent(&out,Event{Kind:"observation",Actor:x.Actor,Text:"Presentes em "+m.Location+": "+strings.Join(names,", "),Audience:[]string{x.Actor}})
		case "wait":
			appendEvent(&out,Event{Kind:"wait",Actor:x.Actor,Text:"Aguardou.",Audience:[]string{x.Actor}})
		}
		out.Minds[x.Actor]=m
	}
	return out,nil
}
func Replay(j Journal) (State,error) {
	s,err:=initial(j);if err!=nil{return State{},err}
	if len(j.Rounds)>MaxRounds{return State{},errors.New("round limit exceeded")}
	for _, r:=range j.Rounds {s,err=apply(j,s,r);if err!=nil{return State{},err}}
	return s,nil
}
func Digest(j Journal) string {
	raw,_:=json.Marshal(j);sum:=sha256.Sum256(raw);return hex.EncodeToString(sum[:])
}

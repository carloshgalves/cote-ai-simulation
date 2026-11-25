package pilot

import (
	"errors"
	"fmt"
	"unicode/utf8"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/clock"
	"github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
)

// Activity is engine-owned. A plan or intention alone never creates effects.
type Activity struct {
	Kind string `json:"kind"`
	Target string `json:"target,omitempty"`
	Text string `json:"text,omitempty"`
	Until int64 `json:"until"`
}
func eligibleActors(j Journal,s State)[]string{
	order:=ids(j.Profiles)
	if j.Version!=ScheduledVersion{return order}
	out:=[]string{}
	for _,id:=range order{if s.Minds[id].Activity==nil{out=append(out,id)}}
	return out
}
func availableForContact(m Mind)bool{return m.Activity==nil||m.Activity.Kind=="wait"||m.Activity.Kind=="observe"}
func walkMinutes(from,to string)int64{
	isClass:=func(s string)bool{return s=="class-b"||s=="class-c"||s=="class-d"}
	if from==to{return 0}
	if isClass(from)&&isClass(to){return 2}
	if from=="hall"&&isClass(to)||to=="hall"&&isClass(from){return 2}
	if from=="hall"&&to=="library"||to=="hall"&&from=="library"{return 4}
	if from=="hall"&&to=="courtyard"||to=="hall"&&from=="courtyard"{return 3}
	if isClass(from)&&to=="library"||isClass(to)&&from=="library"{return 5}
	if isClass(from)&&to=="courtyard"||isClass(to)&&from=="courtyard"{return 4}
	return 6
}
func talkMinutes(text string)int64{
	n:=utf8.RuneCountInString(text)
	if n>400{return 4};if n>160{return 3};return 2
}
func scheduledAdvance(j Journal,s State,choices []Chosen)(State,error){
	if s.Minute>=EndMinute{return State{},errors.New("manhã concluída")}
	order:=eligibleActors(j,s)
	if len(choices)!=len(order){return State{},errors.New("incomplete eligible cohort")}
	for i,x:=range choices{
		if x.Actor!=order[i]{return State{},errors.New("invalid eligible actor order")}
		if x.Failure!=""{return State{},errors.New("falha técnica não constitui decisão; repita o ciclo")}
		if err:=validateDecision(actorContext(j,s,x.Actor),x.Decision);err!=nil{return State{},fmt.Errorf("%s: %w",x.Actor,err)}
	}
	out:=clone(s);out.Round++
	// All cognition uses the frozen pre-cycle context. Resolve competing
	// physical actions separately. Movement takes precedence over invitations.
	byID:=map[string]Decision{}
	for _,x:=range choices{
		byID[x.Actor]=x.Decision
		m:=applyCognition(j,&out,x.Actor,out.Minds[x.Actor],x.Decision)
		m.WakeReason="";out.Minds[x.Actor]=m
	}
	// A movement removes its actor from the contactable scene immediately.
	for _,x:=range choices{if x.Decision.Action=="move"{
		m:=out.Minds[x.Actor];d:=x.Decision
		m.Activity=&Activity{Kind:"move",Target:d.Target,Until:s.Minute+walkMinutes(m.Location,d.Target)}
		out.Minds[x.Actor]=m
		appendEvent(&out,Event{Kind:"departure",Actor:x.Actor,Target:d.Target,Text:"Partiu de "+m.Location+" para "+d.Target,Audience:[]string{x.Actor}})
		for _,id:=range ids(j.Profiles){if id!=x.Actor&&s.Minds[id].Location==m.Location&&(s.Minds[id].Activity==nil||s.Minds[id].Activity.Kind!="move"){appendEvent(&out,Event{Kind:"observation",Actor:x.Actor,Text:"Saiu de "+m.Location,Audience:[]string{id}});wakeWaiting(&out,id,"Um colega saiu do ambiente")}}
	}}
	// Rotate priority to avoid systematic alphabetical advantage.
	for i:=0;i<len(order);i++{
		id:=order[(i+s.Round)%len(order)];d:=byID[id];m:=out.Minds[id]
		if d.Action=="move"{continue}
		if m.Activity!=nil&&m.Activity.Kind=="listen"{
			if d.Action=="talk"&&d.Target!=m.Activity.Target{appendEvent(&out,Event{Kind:"obstacle",Actor:id,Text:"A conversa pretendida foi adiada por outra interação.",Audience:[]string{id}})}
			continue
		}
		switch d.Action{
		case "talk":
			target:=out.Minds[d.Target]
			if target.Location!=m.Location||!availableForContact(target){
				m.Activity=&Activity{Kind:"retry",Until:s.Minute+1}
				appendEvent(&out,Event{Kind:"obstacle",Actor:id,Target:d.Target,Text:"Interlocutor indisponível; reavalie a abordagem.",Audience:[]string{id}})
				break
			}
			until:=s.Minute+talkMinutes(d.Text)
			m.Activity=&Activity{Kind:"talk",Target:d.Target,Text:d.Text,Until:until}
			target.Activity=&Activity{Kind:"listen",Target:id,Until:until};target.WakeReason=""
			out.Minds[d.Target]=target
		case "observe":
			m.Activity=&Activity{Kind:"observe",Until:s.Minute+1}
		case "wait":
			until:=nextWorld(s)
			if d.WaitMinutes>0&&s.Minute+int64(d.WaitMinutes)<until{until=s.Minute+int64(d.WaitMinutes)}
			m.Activity=&Activity{Kind:"wait",Until:until}
		}
		out.Minds[id]=m
	}
	// Next actual due time, not the longest action in the cohort.
	minute:=nextWorld(s)
	for _,m:=range out.Minds{if m.Activity!=nil&&m.Activity.Until<minute{minute=m.Activity.Until}}
	if minute<=s.Minute{return State{},errors.New("invalid scheduled advance")}
	cs:=clock.State{Instant:domain.SimulationInstant(s.Minute),Revision:domain.WorldRevision(s.Round),LastOrdinal:0}
	plan,ok,err:=clock.Next(cs,[]clock.Pending{{Eligibility:domain.EligibilityCoordinate{NotBeforeInstant:domain.SimulationInstant(minute)}}})
	if err!=nil||!ok||plan.Kind!=clock.ClockAdvance{return State{},errors.New("invalid scheduled clock")}
	advanced,err:=clock.ApplyAdvance(cs,plan);if err!=nil{return State{},err}
	out.Minute=int64(advanced.Instant)
	// Complete arrivals first, then publish messages and observations.
	for _,id:=range ids(j.Profiles){
		m:=out.Minds[id];a:=m.Activity
		if a==nil||a.Until>minute||a.Kind!="move"{continue}
		m.Location=a.Target;m.Activity=nil;m.WakeReason="Deslocamento concluído";out.Minds[id]=m
		appendEvent(&out,Event{Kind:"arrival",Actor:id,Target:m.Location,Text:"Chegou a "+m.Location,Audience:[]string{id}})
		for _,other:=range ids(j.Profiles){if other==id{continue};p:=out.Minds[other];if p.Location==m.Location&&(p.Activity==nil||p.Activity.Kind!="move"){appendEvent(&out,Event{Kind:"observation",Actor:id,Text:"Chegou a "+m.Location,Audience:[]string{other}});wakeWaiting(&out,other,"Um colega chegou ao ambiente")}}
	}
	for _,id:=range ids(j.Profiles){
		m:=out.Minds[id];a:=m.Activity
		if a==nil||a.Until>minute{continue}
		switch a.Kind{
		case "talk":appendEvent(&out,Event{Kind:"claim",Actor:id,Target:a.Target,Text:a.Text,Audience:[]string{id,a.Target}});m.WakeReason="Conversa concluída"
		case "listen":m.WakeReason="Recebeu uma fala"
		case "observe":text:=roster(out,id);if observationChanged(out,id,text){appendEvent(&out,Event{Kind:"observation",Actor:id,Text:text,Audience:[]string{id}})};m.WakeReason="Observação concluída"
		case "wait":m.WakeReason="Prazo de reavaliação atingido"
		case "retry":m.WakeReason="Interlocutor indisponível"
		}
		m.Activity=nil;out.Minds[id]=m
	}
	for _,e:=range schoolAgenda{if e.Minute==minute{
		appendEvent(&out,Event{Kind:"announcement",Actor:"school",Text:e.Text,Audience:ids(j.Profiles)})
		for _,id:=range ids(j.Profiles){wakeWaiting(&out,id,"Novo anúncio escolar")}
	}}
	return out,nil
}
func wakeWaiting(s *State,id,reason string){
	m:=s.Minds[id]
	if m.Activity!=nil&&m.Activity.Kind=="wait"{m.Activity=nil;m.WakeReason=reason;s.Minds[id]=m}
}

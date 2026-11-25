package pilot

import (
	"errors"
	"fmt"
	"unicode/utf8"
 "strings"

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
	if j.Version<ScheduledVersion{return order}
	out:=[]string{}
	for _,id:=range order{if s.Minds[id].Activity==nil{out=append(out,id)}}
	return out
}
type Conversation struct{
 Peer string `json:"peer"`
 Turn string `json:"turn"`
 Started int64 `json:"started"`
 Turns int `json:"turns"`
 Stale int `json:"stale,omitempty"`
}
func canContact(m Mind,peer string,other Mind,self string)bool{
 if !availableForContact(other){return false}
 if m.Conversation!=nil&&(m.Conversation.Peer!=peer||m.Conversation.Turn!=self){return false}
 if other.Conversation!=nil&&(other.Conversation.Peer!=self||other.Conversation.Turn!=self){return false}
 return true
}
func closeConversation(s *State,id string){
 m:=s.Minds[id];if m.Conversation==nil{return}
 peer:=m.Conversation.Peer;m.Conversation=nil;s.Minds[id]=m
 p:=s.Minds[peer];if p.Conversation!=nil&&p.Conversation.Peer==id{
  p.Conversation=nil
  if p.Activity!=nil&&p.Activity.Kind=="conversation_hold"{p.Activity=nil;p.WakeReason="Interlocutor encerrou a conversa"}
  s.Minds[peer]=p
 }
}
func availableForContact(m Mind)bool{return m.Activity==nil||m.Activity.Kind=="wait"||m.Activity.Kind=="observe"||m.Activity.Kind=="conversation_hold"||(m.Tier!=""&&m.Activity.Kind=="task"&&m.Activity.Target=="work:")}
func walkMinutes(from,to string)int64{
	if strings.HasPrefix(from,"dormitory-")||strings.HasPrefix(to,"dormitory-"){return 12}
	if has([]string{"shop","mall","seafront","park","gym","cafeteria"},from)||has([]string{"shop","mall","seafront","park","gym","cafeteria"},to){return 8}
	isClass:=func(s string)bool{return s=="class-a"||s=="class-b"||s=="class-c"||s=="class-d"}
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
	if s.Minute>=endMinute(j.Version){return State{},errors.New("período concluído")}
	order:=eligibleActors(j,s)
	if len(choices)!=len(order){return State{},errors.New("incomplete eligible cohort")}
	for i,x:=range choices{
		if x.Actor!=order[i]{return State{},errors.New("invalid eligible actor order")}
		if x.Failure!=""{return State{},errors.New("falha técnica não constitui decisão; repita o ciclo")}
		if j.Version>=PopulationVersion{
   local:=!decisionNeedsModel(j,s,x.Actor)
   if local&&!equalDecision(x.Decision,routineDecision(j,s,x.Actor))||!local&&x.Decision.Source==routineSource{return State{},errors.New("invalid decision provenance")}
  }
  if err:=validateDecision(actorContext(j,s,x.Actor),x.Decision);err!=nil{return State{},fmt.Errorf("%s: %w",x.Actor,err)}
	}
	out:=clone(s);out.Round++
	// All cognition uses the frozen pre-cycle context. Resolve competing
	// physical actions separately. Movement takes precedence over invitations.
	byID:=map[string]Decision{}
	for _,x:=range choices{
		byID[x.Actor]=x.Decision
		m:=out.Minds[x.Actor]
  if j.Version<PopulationVersion||x.Decision.Source!=routineSource{
   if j.Version>=PopulationVersion&&(m.Tier==LightTier||j.Version>=SelectiveVersion&&m.Tier==FullTier)&&initiativeAccountingDue(j,s,x.Actor){m.LastLightInitiativeDay=s.Minute/1440+1}
   if j.Version>=CalibratedVersion{m.LastDeliberationMinute=s.Minute;if personalOpportunity(j,s,x.Actor)!=nil&&!essentialCognition(j,s,x.Actor){day:=s.Minute/1440+1;if m.OpportunityDay!=day{m.OpportunityDay=day;m.OpportunityCount=0};m.OpportunityCount++;m.LastOpportunityMinute=s.Minute}}else if j.Version>=MotivationVersion&&personalOpportunity(j,s,x.Actor)!=nil{m.LastOpportunityMinute=s.Minute}
   if j.Version>=SelectiveVersion{
    m.NextDeliberation=0
    switch x.Decision.Action{
    case "wait":if x.Decision.WaitMinutes>0{m.NextDeliberation=s.Minute+int64(x.Decision.WaitMinutes)}
    case "move":if j.Version<CalibratedVersion||x.Decision.Plan!=nil||m.Plan!=nil{m.NextDeliberation=s.Minute+walkMinutes(m.Location,x.Decision.Target)}
    case "observe":m.NextDeliberation=s.Minute+1
    }
   }
   m=applyCognition(j,&out,x.Actor,m,x.Decision)
   if j.Version>=CognitionVersion{m.PendingStimulus="";m.AwaitEvent=x.Decision.Action=="wait"&&x.Decision.WaitMinutes==0}
  }else{m.LastAction=x.Decision.Action;if j.Version>=AutonomyVersion&&(x.Decision.Action=="buy"||x.Decision.Action=="recreation"){m.LastRoutinePurchaseDay=s.Minute/1440+1}}
		m.WakeReason="";out.Minds[x.Actor]=m
	}
	if j.Version>=ConversationVersion{
		for _,x:=range choices{
			m:=out.Minds[x.Actor]
			if m.Conversation!=nil&&(x.Decision.Action!="talk"||x.Decision.Target!=m.Conversation.Peer){closeConversation(&out,x.Actor)}
		}
	}
	executed:=choices
 if j.Version>=ContactVersion{executed=reserveRoutineContacts(j,s,&out,choices)}
 if j.Version>=WorldVersion{applyImmediateWorld(j,s,&out,executed)}
	// A movement removes its actor from the contactable scene immediately.
	for _,x:=range executed{if x.Decision.Action=="move"{
		m:=out.Minds[x.Actor];d:=x.Decision
		m.Activity=&Activity{Kind:"move",Target:d.Target,Until:s.Minute+walkMinutes(m.Location,d.Target)}
		out.Minds[x.Actor]=m
		appendEvent(&out,Event{Kind:"departure",Actor:x.Actor,Target:d.Target,Text:"Partiu de "+m.Location+" para "+d.Target,Audience:[]string{x.Actor}})
		if j.Version>=ObservationVersion{publishMovementObservation(j,&out,s,x.Actor,m.Location,false)}else{for _,id:=range ids(j.Profiles){if id!=x.Actor&&s.Minds[id].Location==m.Location&&(s.Minds[id].Activity==nil||s.Minds[id].Activity.Kind!="move"){appendEvent(&out,Event{Kind:"observation",Actor:x.Actor,Text:"Saiu de "+m.Location,Audience:[]string{id}});wakeOnMovement(j,&out,id,x.Actor,"Um colega saiu do ambiente")}}}
	}}
	// Rotate priority to avoid systematic alphabetical advantage.
	for i:=0;i<len(order);i++{
		id:=order[(i+s.Round)%len(order)];d:=byID[id];m:=out.Minds[id]
  if j.Version>=ContactVersion&&m.Activity!=nil&&m.Activity.Kind=="contact_hold"{continue}
		if d.Action=="move"||(j.Version>=WorldVersion&&isWorldAction(d.Action)){continue}
		if m.Activity!=nil&&m.Activity.Kind=="listen"{
			if d.Action=="talk"&&d.Target!=m.Activity.Target{appendEvent(&out,Event{Kind:"obstacle",Actor:id,Text:"A conversa pretendida foi adiada por outra interação.",Audience:[]string{id}})}
			continue
		}
		switch d.Action{
		case "talk":
			target:=out.Minds[d.Target]
   if j.Version>=ContactVersion&&target.Activity!=nil&&target.Activity.Kind=="contact_hold"&&target.Activity.Target==id{target.Activity=nil;out.Minds[d.Target]=target}
			if target.Location!=m.Location||!availableForContact(target)||(j.Version>=ConversationVersion&&!canContact(m,d.Target,target,id)){
				m.Activity=&Activity{Kind:"retry",Until:s.Minute+1}
    if j.Version>=ContactVersion&&j.Version<CalibratedVersion{m.PendingStimulus="Abordagem não concluída; avalie outra ocasião ou interlocutor."}
				appendEvent(&out,Event{Kind:"obstacle",Actor:id,Target:d.Target,Text:"Interlocutor indisponível; reavalie a abordagem.",Audience:[]string{id}})
				break
			}
			until:=s.Minute+talkMinutes(d.Text)
			if j.Version>=ConversationVersion{
				started,turns:=s.Minute,0
				if m.Conversation!=nil{started=m.Conversation.Started;turns=m.Conversation.Turns}
				stale:=0
				if j.Version>=WorldVersion&&m.Conversation!=nil{stale=m.Conversation.Stale}
				m.Conversation=&Conversation{Peer:d.Target,Turn:id,Started:started,Turns:turns,Stale:stale}
				target.Conversation=&Conversation{Peer:id,Turn:id,Started:started,Turns:turns,Stale:stale}
			}
			m.Activity=&Activity{Kind:"talk",Target:d.Target,Text:d.Text,Until:until}
			target.Activity=&Activity{Kind:"listen",Target:id,Until:until};target.WakeReason=""
			out.Minds[d.Target]=target
		case "observe":
			m.Activity=&Activity{Kind:"observe",Until:s.Minute+1}
		case "wait":
			until:=nextDue(j,s)
			if d.WaitMinutes>0&&s.Minute+int64(d.WaitMinutes)<until{until=s.Minute+int64(d.WaitMinutes)}
			m.Activity=&Activity{Kind:"wait",Until:until}
		}
		out.Minds[id]=m
	}
	// Departures were published at the current instant. A newly selected
	// wait also reacts to those events, rather than sleeping through them.
	for _,e:=range out.Events[len(s.Events):]{
		if e.Kind=="observation"&&e.Text!=""{for _,id:=range e.Audience{wakeOnMovement(j,&out,id,e.Actor,"Um colega saiu do ambiente")}}
		if j.Version>=WorldVersion&&e.Kind=="meeting"{for _,id:=range e.Audience{if id!=e.Actor{wakeWaiting(&out,id,"Atualização de compromisso")}}}
	}
	// An immediate stimulus can make another decision due at the same
	// simulated minute. Publish this cycle before advancing any clock.
	if len(eligibleActors(j,out))>0{return out,nil}
	// Next actual due time, not the longest action in the cohort.
	minute:=nextDue(j,s)
	for _,m:=range out.Minds{if m.Activity!=nil&&m.Activity.Until<minute{minute=m.Activity.Until}}
	if minute<=s.Minute{return State{},errors.New("invalid scheduled advance")}
	cs:=clock.State{Instant:domain.SimulationInstant(s.Minute),Revision:domain.WorldRevision(s.Round),LastOrdinal:0}
	plan,ok,err:=clock.Next(cs,[]clock.Pending{{Eligibility:domain.EligibilityCoordinate{NotBeforeInstant:domain.SimulationInstant(minute)}}})
	if err!=nil||!ok||plan.Kind!=clock.ClockAdvance{return State{},errors.New("invalid scheduled clock")}
	advanced,err:=clock.ApplyAdvance(cs,plan);if err!=nil{return State{},err}
	out.Minute=int64(advanced.Instant)
 if j.Version>=CognitionVersion{advanceBodies(j,&out,s.Minute)}
 if j.Version>=AutonomyVersion{completeSharedLeisure(j,&out)}
	// Complete arrivals first, then publish messages and observations.
	arrivals:=[]string{}
	for _,id:=range ids(j.Profiles){
		m:=out.Minds[id];a:=m.Activity
		if a==nil||a.Until>minute||a.Kind!="move"{continue}
		m.Location=a.Target;m.Activity=nil;m.WakeReason="Deslocamento concluído";out.Minds[id]=m
		arrivals=append(arrivals,id)
	}
	for _,id:=range arrivals{
		m:=out.Minds[id]
		appendEvent(&out,Event{Kind:"arrival",Actor:id,Target:m.Location,Text:"Chegou a "+m.Location,Audience:[]string{id}})
		if j.Version>=ObservationVersion{publishMovementObservation(j,&out,out,id,m.Location,true)}else{for _,other:=range ids(j.Profiles){if other==id{continue};p:=out.Minds[other];if p.Location==m.Location&&(p.Activity==nil||p.Activity.Kind!="move"){appendEvent(&out,Event{Kind:"observation",Actor:id,Text:"Chegou a "+m.Location,Audience:[]string{other}});wakeOnMovement(j,&out,other,id,"Um colega chegou ao ambiente")}}}
	}
	for _,id:=range ids(j.Profiles){
		m:=out.Minds[id];a:=m.Activity
		if a==nil||a.Until>minute{continue}
		switch a.Kind{
		case "talk":appendEvent(&out,Event{Kind:"claim",Actor:id,Target:a.Target,Text:a.Text,Audience:[]string{id,a.Target}});m.WakeReason="Conversa concluída"
   if j.Version>=PopulationVersion{out.Minds[id]=m;engage(j,&out,id,a.Target)
    if j.Version>=CognitionVersion{recordExchange(&out,id,a.Target);stimulate(&out,a.Target,"Recebeu uma fala")}
    m=out.Minds[id]}
			if j.Version>=ConversationVersion&&m.Conversation!=nil{
				m.Conversation.Turn=a.Target;m.Conversation.Turns++
				if j.Version>=WorldVersion{
					m.Conversation.Stale=dialogueStale(out.Events,id,a.Target,a.Text,m.Conversation.Stale)
					if m.Conversation.Turns>=12||m.Conversation.Stale>=2||out.Minute-m.Conversation.Started>=25{
						out.Minds[id]=m;closeConversation(&out,id);m=out.Minds[id]
						appendEvent(&out,Event{Kind:"conversation_end",Actor:id,Target:a.Target,Text:"Conversa encerrada por duração ou repetição; reavalie a abordagem.",Audience:[]string{id,a.Target}})
						m.WakeReason="Conversa encerrada";break
					}
				}
				peer:=out.Minds[a.Target];if peer.Conversation!=nil{peer.Conversation.Turn=a.Target;peer.Conversation.Turns=m.Conversation.Turns;peer.Conversation.Stale=m.Conversation.Stale;out.Minds[a.Target]=peer}
				m.Activity=&Activity{Kind:"conversation_hold",Target:a.Target,Until:nextDue(j,out)}
				m.WakeReason="";out.Minds[id]=m;continue
			}
		case "listen":m.WakeReason="Recebeu uma fala"
		case "task":completeTask(j,&out,id,&m,a)
		case "world_action":m.WakeReason="Ação concluída"
		case "observe":text:=roster(out,id);if observationChanged(out,id,text){appendEvent(&out,Event{Kind:"observation",Actor:id,Text:text,Audience:[]string{id}})};m.WakeReason="Observação concluída"
		case "wait":m.WakeReason="Prazo de reavaliação atingido";if minute==nextDue(j,s){m.WakeReason="Novo anúncio escolar"}
		case "retry":m.WakeReason="Interlocutor indisponível"
		case "conversation_hold":
			closeConversation(&out,id);m=out.Minds[id];m.WakeReason="Conversa expirou"
		}
		m.Activity=nil;out.Minds[id]=m
	}
	if j.Version>=WorldVersion{advanceWorld(j,&out);if j.Version>=PopulationVersion{updatePopulation(j,&out)};if j.Version>=CognitionVersion{expirePlans(j,&out)};return out,nil}
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

package pilot

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type ActorCycle struct {
	Cycle int `json:"cycle"`
	StartMinute int64 `json:"start_minute"`
	EndMinute int64 `json:"end_minute"`
	Before Mind `json:"before"`
	Choice Chosen `json:"choice"`
	Events []Event `json:"events"`
	After Mind `json:"after"`
	AvailableContext Context `json:"reconstructed_context"`
}
type ActorReport struct {
	Profile Profile `json:"profile"`
	Cycles []ActorCycle `json:"cycles"`
	Events []Event `json:"events"`
	Final Mind `json:"final"`
	Pending *Chosen `json:"pending,omitempty"`
}
type OperatorReport struct {
	GeneratedAt time.Time `json:"generated_at"`
	Journal Journal `json:"journal"`
	State State `json:"state"`
	Digest string `json:"digest"`
	Progress Progress `json:"pending_progress"`
	Routes []RouteView `json:"routes"`
	Calls []CallTelemetry `json:"calls_current_process"`
	DroppedCalls int `json:"dropped_calls"`
}
func (s *Session) ReportZIP()([]byte,error){
	s.mu.Lock()
	j,state,p:=clone(s.journal),clone(s.state),clone(s.progress)
	s.mu.Unlock()
	if p.Round<=state.Round{p=Progress{}}
	o:=OperatorReport{GeneratedAt:time.Now(),Journal:j,State:state,Digest:Digest(j),Progress:p}
	if r,ok:=s.model.(*Router);ok{o.Routes=r.Routes();o.Calls,o.DroppedCalls=r.Telemetry()}
	return buildReport(o)
}
func buildReport(o OperatorReport)([]byte,error){
	j:=o.Journal
	s,err:=initial(j);if err!=nil{return nil,err}
	actors:=map[string]*ActorReport{}
	for _,profile:=range j.Profiles{actors[profile.ID]=&ActorReport{Profile:profile,Cycles:[]ActorCycle{}}}
	for _,round:=range j.Rounds{
		next,err:=apply(j,s,round);if err!=nil{return nil,err}
		for _,choice:=range round.Choices{
			id:=choice.Actor
			events:=[]Event{}
			for _,e:=range next.Events[len(s.Events):]{if has(e.Audience,id){events=append(events,e)}}
			// Failure markers are operator diagnostics, not character knowledge.
			c:=clone(choice)
			c.Decision.Source="";c.Decision.Ignored=nil
			if c.Failure!=""{c=Chosen{Actor:id}}
			actors[id].Cycles=append(actors[id].Cycles,ActorCycle{Cycle:round.Number,StartMinute:s.Minute,EndMinute:next.Minute,Before:clone(s.Minds[id]),Choice:c,Events:events,After:clone(next.Minds[id]),AvailableContext:actorContext(j,s,id)})
		}
		s=next
	}
	for id,a:=range actors{
		a.Events=visible(s,id);a.Final=clone(s.Minds[id])
		if o.Progress.Round>s.Round{for _,c:=range o.Progress.Choices{if c.Actor==id&&c.Failure==""{copy:=clone(c);copy.Decision.Source="";copy.Decision.Ignored=nil;a.Pending=&copy}}}
	}
	var buf bytes.Buffer
	z:=zip.NewWriter(&buf)
	write:=func(name string,b []byte)error{w,err:=z.Create(name);if err!=nil{return err};_,err=w.Write(b);return err}
	writeJSON:=func(name string,v any)error{b,err:=json.MarshalIndent(v,"","  ");if err!=nil{return err};return write(name,b)}
	if err:=write("LEIA-ME.md",[]byte("# Relatório COTE\n\nExportação local sem chamadas ao LLM. Operador: diário, todos os eventos e estados, propostas pendentes e diagnósticos. Personagens: somente eventos endereçados, decisões próprias e evolução cognitiva. Contextos anteriores são reconstruídos por replay com o código/packs atuais; não são uma gravação literal de prompts históricos. Intenções são declarações ficcionais, não cadeia de pensamento interna. Propostas rejeitadas e respostas brutas não armazenadas anteriormente não podem ser recuperadas. Telemetria registra chamadas concluídas no processo atual (incluindo testes), não toda a história nem consumo de outros aplicativos. Cotas são observações de headers, não saldo garantido. Dados privados ficcionais: compartilhe apenas a perspectiva desejada.\n"));err!=nil{return nil,err}
	if err:=writeJSON("operador.json",o);err!=nil{return nil,err}
	var b strings.Builder
	fmt.Fprintf(&b,"# Operador — COTE\n\nCiclos confirmados: %d | Horário: %s | Registro: %s\n\n",o.State.Round,reportTime(o.State.Minute),o.Digest)
	fmt.Fprintln(&b,"## Eventos iniciais");for _,e:=range o.State.Events{if e.Round==0{writeEvent(&b,e)}}
	for _,r:=range j.Rounds{
		fmt.Fprintf(&b,"\n## Ciclo %d — %s\n\n",r.Number,reportTime(r.Minute))
		for _,c:=range r.Choices{writeChoice(&b,c)}
		for _,e:=range o.State.Events{if e.Round==r.Number{writeEvent(&b,e)}}
	}

	if o.Progress.Round>o.State.Round{fmt.Fprintln(&b,"\n## Propostas pendentes (não executadas)");for _,c:=range o.Progress.Choices{writeChoice(&b,c)}}
	fmt.Fprintln(&b,"\n## Chamadas do processo atual")
	for _,c:=range o.Calls{fmt.Fprintf(&b,"- %s | ciclo %d | %s | %s / %s | HTTP %d | %d ms | aceita: %t | %s",c.StartedAt.Format(time.RFC3339),c.Cycle,c.Actor,c.Provider,c.Model,c.HTTPStatus,c.LatencyMS,c.Accepted,c.Error);if c.Usage!=nil{fmt.Fprintf(&b," | tokens entrada/saída: %d/%d",c.Usage.Input,c.Usage.Output)};fmt.Fprintln(&b)}
	if err:=write("operador.md",[]byte(b.String()));err!=nil{return nil,err}
	for _,id:=range ids(j.Profiles){
		a:=actors[id]
		if err:=writeJSON("personagens/"+id+".json",a);err!=nil{return nil,err}
		b.Reset();fmt.Fprintf(&b,"# %s — perspectiva individual\n\n",a.Profile.Name)
		fmt.Fprintln(&b,"## Perfil inicial");writeJSONText(&b,a.Profile)
		fmt.Fprintln(&b,"\n## Acontecimentos iniciais");for _,e:=range a.Events{if e.Round==0{writeEvent(&b,e)}}
		for _,c:=range a.Cycles{
			fmt.Fprintf(&b,"\n## Ciclo %d — %s a %s\n\n",c.Cycle,reportTime(c.StartMinute),reportTime(c.EndMinute))
			fmt.Fprintln(&b,"### Antes da decisão");writeMind(&b,c.Before)
			fmt.Fprintln(&b,"\n### Decisão própria");writeChoice(&b,c.Choice)
			fmt.Fprintln(&b,"\n### Acontecimentos percebidos");for _,e:=range c.Events{writeEvent(&b,e)}
			fmt.Fprintln(&b,"\n### Depois da decisão");writeMind(&b,c.After)
		}
		if a.Pending!=nil{fmt.Fprintln(&b,"\n## Proposta pendente (não executada)");writeChoice(&b,*a.Pending)}
		if err:=write("personagens/"+id+".md",[]byte(b.String()));err!=nil{return nil,err}
	}
	if err:=z.Close();err!=nil{return nil,err};return buf.Bytes(),nil
}
func reportTime(m int64)string{return fmt.Sprintf("%02d:%02d",m/60,m%60)}
func writeChoice(b *strings.Builder,c Chosen){
	d:=c.Decision
	if c.Failure!=""{fmt.Fprintf(b,"- %s — falha: %s\n",c.Actor,c.Failure);return}
	if d.Action==""{fmt.Fprintln(b,"- Sem decisão executada registrada.");return}
	fmt.Fprintf(b,"- %s — %s %s\n",c.Actor,d.Action,d.Target)
	if d.Source!=""{fmt.Fprintf(b,"  - Modelo: %s\n",d.Source)}
	if d.Reason!=""{fmt.Fprintf(b,"  - Intenção: %s\n",d.Reason)}
	if d.Note!=""{fmt.Fprintf(b,"  - Nota privada: %s\n",d.Note)}
	if d.Text!=""{fmt.Fprintf(b,"  - Fala: %s\n",d.Text)}
	for _,belief:=range d.Beliefs{fmt.Fprintf(b,"  - Hipótese: %s (%d%%; %s)\n",belief.Text,belief.Confidence,belief.Evidence)}
	if d.Goal!=nil{fmt.Fprintf(b,"  - Meta: %s → %s (%s)\n",d.Goal.Replace,d.Goal.With,d.Goal.Evidence)}
	if d.Plan!=nil{fmt.Fprintf(b,"  - Plano: %s (%s)\n",strings.Join(d.Plan.Steps," → "),d.Plan.Evidence)}
	if d.Relation!=nil{fmt.Fprintf(b,"  - Relação: %s %+d (%s)\n",d.Relation.Person,d.Relation.Delta,d.Relation.Evidence)}
	if len(d.Ignored)>0{fmt.Fprintf(b,"  - Descartes: %s\n",strings.Join(d.Ignored,", "))}
}
func writeEvent(b *strings.Builder,e Event){fmt.Fprintf(b,"- %s | %s | %s | %s → %s | %s",reportTime(e.Minute),e.ID,e.Kind,e.Actor,e.Target,e.Text);if e.Evidence!=""{fmt.Fprintf(b," | evidência: %s",e.Evidence)};fmt.Fprintln(b)}
func writeMind(b *strings.Builder,m Mind){
	fmt.Fprintf(b,"- Local: %s\n- Objetivos: %s\n- Intenção: %s\n",m.Location,strings.Join(m.Goals,"; "),m.Thought)
	if m.Plan!=nil{fmt.Fprintf(b,"- Plano: %s (%s)\n",strings.Join(m.Plan.Steps," → "),m.Plan.Evidence)}
	for _,v:=range m.Beliefs{fmt.Fprintf(b,"- Hipótese: %s (%d%%; %s)\n",v.Text,v.Confidence,v.Evidence)}
	raw,_:=json.Marshal(m.Relations);fmt.Fprintf(b,"- Relações: %s\n",raw)
}

func writeJSONText(b *strings.Builder,v any){
	raw,_:=json.MarshalIndent(v,"","  ")
	for _,line:=range strings.Split(string(raw),"\n"){fmt.Fprintln(b,"    "+line)}
}

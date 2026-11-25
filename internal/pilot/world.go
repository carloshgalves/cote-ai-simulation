package pilot

import (
 "crypto/sha256"
 "errors"
 "fmt"
 "sort"
 "strings"
 "unicode"
)

const WorldEnd int64 = 3*1440+480 // three full days from the initial 08:00
func endMinute(version int)int64{if version>=WorldVersion{return WorldEnd};return EndMinute}
func roundLimit(version int)int{if version>=WorldVersion{return 1200};return MaxRounds}

type Discovery struct{
 Text string `json:"text"`
 Evidence string `json:"evidence"`
 Grounding string `json:"grounding"`
}
func validDiscovery(c Context,d Discovery)bool{
 e,ok:=evidence(c,d.Evidence)
 return ok&&validText(d.Text,300)&&validText(d.Grounding,240)&&len(strings.Fields(d.Grounding))>=2&&strings.Contains(e.Text,d.Grounding)
}
type LedgerEntry struct{
 Minute int64 `json:"minute"`
 From string `json:"from"`
 To string `json:"to"`
 Amount int64 `json:"amount"`
 Category string `json:"category"`
 Cause string `json:"cause"`
}
type Meeting struct{
 ID string `json:"id"`
 Host string `json:"host"`
 Guest string `json:"guest"`
 Location string `json:"location"`
 At int64 `json:"at"`
 Status string `json:"status"`
}
type VIPExam struct{
 Phase string `json:"phase"`
 Group []string `json:"group"`
 VIP string `json:"vip"`
 Submitted map[string]bool `json:"submitted"`
 Outcome int `json:"outcome,omitempty"`
}
type SchoolWorld struct{
 ClassPoints map[string]int64 `json:"class_points"`
 Ledger []LedgerEntry `json:"ledger"`
 Meetings []Meeting `json:"meetings"`
 Exam *VIPExam `json:"exam,omitempty"`
}
type ExamView struct{
 Phase string `json:"phase"`
 Group []string `json:"group"`
 IsVIP bool `json:"is_vip"`
 Submitted bool `json:"submitted"`
 Outcome int `json:"outcome,omitempty"`
}
type SchoolView struct{
 Wallet int64 `json:"wallet"`
 Inventory map[string]int `json:"inventory"`
 Meetings []Meeting `json:"meetings"`
 Rules []string `json:"rules"`
 Options []offeredAction `json:"options"`
 Catalog map[string]int64 `json:"catalog,omitempty"`
 Exam *ExamView `json:"exam,omitempty"`
}
func worldLocations(id string)[]string{return append(clone(locations),"shop","cafeteria","dormitory")}
func initializeWorld(j Journal,s *State){
 s.World=&SchoolWorld{ClassPoints:map[string]int64{"A":1000,"B":1000,"C":1000,"D":1000},Ledger:[]LedgerEntry{},Meetings:[]Meeting{}}
 for _,id:=range ids(j.Profiles){
  m:=s.Minds[id];m.Inventory=map[string]int{"student-id":1,"phone":1};s.Minds[id]=m
  pay(s,"school",id,100000,"initial","Depósito inicial")
 }
 appendEvent(s,Event{Kind:"announcement",Actor:"school",Text:"Cada aluno recebeu 100.000 PP no cartão. Há loja, refeitório, biblioteca e dormitório. Regras de uso serão divulgadas na orientação às 09:00.",Audience:ids(j.Profiles)})
}
func pay(s *State,from,to string,amount int64,category,cause string)bool{
 if amount<=0||from==to{return false}
 a,src:=s.Minds[from];b,dst:=s.Minds[to]
 if !src&&from!="school"||!dst&&to!="school"||src&&a.Wallet<amount{return false}
 if src{a.Wallet-=amount;s.Minds[from]=a}
 if dst{b.Wallet+=amount;s.Minds[to]=b}
 s.World.Ledger=append(s.World.Ledger,LedgerEntry{s.Minute,from,to,amount,category,cause})
 audience:=[]string{};if src{audience=append(audience,from)};if dst{audience=append(audience,to)}
 appendEvent(s,Event{Kind:"resource",Actor:from,Target:to,Text:fmt.Sprintf("%s: %d PP (%s)",cause,amount,category),Audience:audience})
 return true
}
var shopPrices=map[string]int64{"notebook":300,"pen":100,"snack":200,"water":100}
func isWorldAction(a string)bool{return has([]string{"buy","transfer","study","eat","rest","invite","accept","decline","attend","guess"},a)}
func schoolView(j Journal,s State,id string)*SchoolView{
 m:=s.Minds[id]
 v:=&SchoolView{Wallet:m.Wallet,Inventory:clone(m.Inventory),Meetings:[]Meeting{},Rules:[]string{"PP próprios persistem; o motor é autoridade sobre transações. Não há despesa obrigatória.","Regras completas da escola ainda não foram divulgadas."},Options:[]offeredAction{}}
 add:=func(a,target string){k:=a;if target!=""{k+=":"+target};v.Options=append(v.Options,offeredAction{k,a,target})}
 if s.Minute>=540{v.Rules[1]="Transferências voluntárias são permitidas; preço explícito, sem saldo negativo. Subsistência gratuita disponível. A fórmula mensal de pontos ainda não foi revelada."}
 if m.Location=="shop"{
  v.Catalog=clone(shopPrices)
  for _,item:=range []string{"notebook","pen","snack","water"}{if m.Wallet>=shopPrices[item]{add("buy",item)}}
 }
 if m.Location=="library"||strings.HasPrefix(m.Location,"class-"){add("study","")}
 if m.Location=="cafeteria"{add("eat","free");if m.Wallet>=500{add("eat","paid")}}
 if m.Location=="dormitory"{add("rest","")}
 for _,other:=range ids(j.Profiles){
  if other==id{continue}
  p:=s.Minds[other]
  if p.Location==m.Location&&canContact(m,other,p,id)&& (p.Activity==nil||p.Activity.Kind!="move"){
   if s.Minute>=540&&m.Wallet>=1000{add("transfer",other)}
   pending:=false;for _,meeting:=range s.World.Meetings{if (meeting.Status=="pending"||meeting.Status=="accepted")&&((meeting.Host==id&&meeting.Guest==other)||(meeting.Host==other&&meeting.Guest==id)){pending=true}}
   if !pending&&s.Minute+60<WorldEnd{add("invite",other)}
  }
 }
 for _,mt:=range s.World.Meetings{
  if mt.Host!=id&&mt.Guest!=id{continue}
  v.Meetings=append(v.Meetings,mt)
  if mt.Status=="pending"&&mt.Guest==id&&s.Minute<mt.At{add("accept",mt.ID);add("decline",mt.ID)}
  if mt.Status=="accepted"&&s.Minute>=mt.At&&s.Minute<mt.At+15&&m.Location==mt.Location{add("attend",mt.ID)}
 }
 if e:=s.World.Exam;e!=nil{
  v.Exam=&ExamView{e.Phase,clone(e.Group),e.VIP==id,e.Submitted[id],e.Outcome}
  v.Rules=append(v.Rules,"Zodiac/VIP adaptado: um VIP secreto no grupo; uma submissão por não VIP. Fase antecipada: acerto recompensa 500.000 PP ao autor e transfere 50 CP da classe VIP à dele; erro recompensa VIP e inverte CP. Janela final: acerto 1.000.000 PP ao VIP e 500.000 aos demais; erro ou omissão 500.000 ao VIP. Submissão da classe do VIP não produz resultado. O resultado não revela identidades.")
  if (e.Phase=="early"||e.Phase=="final")&&!e.Submitted[id]&&e.VIP!=id&&e.Outcome==0{for _,other:=range e.Group{if other!=id{add("guess",other)}}}
 }
 return v
}
func validateWorldAction(c Context,d Decision)error{
 if c.School==nil||d.Text!=""{return errors.New("ação institucional inválida")}
 for _,o:=range c.School.Options{if o.Action==d.Action&&o.Target==d.Target{return nil}}
 return errors.New("ação institucional não disponível")
}
func nextDue(j Journal,s State)int64{
 if j.Version<WorldVersion{return nextWorld(s)}
 next:=WorldEnd
 // Fixed authored schedule, disclosure at time of event only.
 for day:=int64(0);day<4;day++{for _,at:=range []int64{480,540,720,780,900,1080,1320,1440}{n:=day*1440+at;if n>s.Minute&&n<next{next=n}}}
 for _,n:=range []int64{1440+600,1440+840,1440+870,1440+900,1440+960}{if n>s.Minute&&n<next{next=n}}
 for _,mt:=range s.World.Meetings{if mt.Status=="pending"||mt.Status=="accepted"{for _,n:=range []int64{mt.At,mt.At+15}{if n>s.Minute&&n<next{next=n}}}}
 return next
}
func applyImmediateWorld(j Journal,before State,s *State,choices []Chosen){
 // Economic and institutional choices resolve from authorized offers;
 // no action can create points, a result or an item from generated prose.
 for _,c:=range choices{
  d,id:=c.Decision,c.Actor
  if !isWorldAction(d.Action){continue}
  m:=s.Minds[id]
  duration:=int64(1);kind:="world_action"
  switch d.Action{
  case "buy":
   if pay(s,id,"school",shopPrices[d.Target],"durable","Compra: "+d.Target){
    m=s.Minds[id];m.Inventory[d.Target]++;s.Minds[id]=m
   }
  case "transfer":pay(s,id,d.Target,1000,"transfer","Transferência voluntária");m=s.Minds[id]
  case "study":duration=25;kind="task"
  case "eat":duration=20;kind="task";if d.Target=="paid"{pay(s,id,"school",500,"ordinary","Refeição paga");m=s.Minds[id]}
  case "rest":duration=60;kind="task"
  case "invite":
   mt:=Meeting{ID:fmt.Sprintf("meeting-%d",len(s.World.Meetings)+1),Host:id,Guest:d.Target,Location:m.Location,At:s.Minute+30,Status:"pending"}
   s.World.Meetings=append(s.World.Meetings,mt)
   appendEvent(s,Event{Kind:"meeting",Actor:id,Target:d.Target,Text:fmt.Sprintf("Convite %s em %s às %s; aguarda aceite.",mt.ID,mt.Location,reportTime(mt.At)),Audience:[]string{id,d.Target}})
   wakeWaiting(s,d.Target,"Recebeu convite")
  case "accept","decline","attend":
   for i,mt:=range s.World.Meetings{if mt.ID==d.Target{
    switch d.Action{case "accept":mt.Status="accepted";case "decline":mt.Status="declined";case "attend":duration=10;kind="task"}
    s.World.Meetings[i]=mt
    appendEvent(s,Event{Kind:"meeting",Actor:id,Target:d.Target,Text:"Compromisso "+mt.ID+": "+d.Action,Audience:[]string{mt.Host,mt.Guest}})
   }}
  case "guess": // resolved as one cohort below
  }
  m.Activity=&Activity{Kind:kind,Target:d.Action+":"+d.Target,Until:s.Minute+duration};s.Minds[id]=m
 }
 settleGuesses(j,before,s,choices)
}
func completeTask(j Journal,s *State,id string,m *Mind,a *Activity){
 m.WakeReason="Atividade concluída"
 appendEvent(s,Event{Kind:"task",Actor:id,Text:"Concluiu "+a.Target,Audience:[]string{id}})
}
func classOf(j Journal,id string)string{for _,p:=range j.Profiles{if p.ID==id{return p.Class}};return ""}
func settleGuesses(j Journal,before State,s *State,choices []Chosen){
 e:=s.World.Exam;if e==nil||e.Outcome!=0{return}
 type candidate struct{id,target,key string}
 cs:=[]candidate{}
 for _,c:=range choices{
  if c.Decision.Action!="guess"{continue}
  id:=c.Actor
  if e.Submitted[id]{continue};e.Submitted[id]=true
  appendEvent(s,Event{Kind:"exam",Actor:id,Text:"Submissão recebida pela escola.",Audience:[]string{id}})
  if id==e.VIP||classOf(j,id)==classOf(j,e.VIP){continue}
  h:=sha256.Sum256([]byte(fmt.Sprintf("pilot-v5/%d/%d/%s",before.Minute,before.Round,id)))
  cs=append(cs,candidate{id,c.Decision.Target,fmt.Sprintf("%x",h)})
 }
 sort.Slice(cs,func(i,k int)bool{return cs[i].key<cs[k].key})
 if len(cs)==0{return}
 winner:=cs[0]
 outcome:=2
 if e.Phase=="early"{outcome=4;if winner.target==e.VIP{outcome=3}}else if winner.target==e.VIP{outcome=1}
 settleExam(j,s,outcome,winner.id)
}
func settleExam(j Journal,s *State,outcome int,submitter string){
 e:=s.World.Exam;if e==nil||e.Outcome!=0{return}
 e.Outcome=outcome;e.Phase="settled"
 switch outcome{
 case 1:for _,id:=range e.Group{n:=int64(500000);if id==e.VIP{n=1000000};pay(s,"school",id,n,"exam","Prêmio Zodiac/VIP")}
 case 2:pay(s,"school",e.VIP,500000,"exam","Prêmio Zodiac/VIP")
 case 3,4:
  a,b:=classOf(j,submitter),classOf(j,e.VIP)
  winner,loser:=a,b;recipient:=submitter
  if outcome==4{winner,loser=b,a;recipient=e.VIP}
  delta:=int64(50);if s.World.ClassPoints[loser]<delta{delta=s.World.ClassPoints[loser]}
  s.World.ClassPoints[loser]-=delta;s.World.ClassPoints[winner]+=delta
  pay(s,"school",recipient,500000,"exam","Prêmio Zodiac/VIP")
 }
 // Public result is delayed; reward receipts are addressed only to payees.
}
func advanceWorld(j Journal,s *State){
 now:=s.Minute
 for i,mt:=range s.World.Meetings{
  if mt.Status!="pending"&&mt.Status!="accepted"{continue}
  if now==mt.At{
   if mt.Status=="pending"{mt.Status="expired"}else{
    appendEvent(s,Event{Kind:"meeting",Actor:"school",Target:mt.ID,Text:"Horário do compromisso "+mt.ID+" em "+mt.Location,Audience:[]string{mt.Host,mt.Guest}})
    wakeWaiting(s,mt.Host,"Compromisso agendado");wakeWaiting(s,mt.Guest,"Compromisso agendado")
   }
  }
  if now==mt.At+15&&mt.Status=="accepted"{
   a,b:=s.Minds[mt.Host],s.Minds[mt.Guest]
   if a.Location==mt.Location&&b.Location==mt.Location&&(a.Activity==nil||a.Activity.Kind!="move")&&(b.Activity==nil||b.Activity.Kind!="move"){mt.Status="met"}else{mt.Status="missed"}
   appendEvent(s,Event{Kind:"meeting",Actor:"school",Target:mt.ID,Text:"Compromisso "+mt.ID+": "+mt.Status,Audience:[]string{mt.Host,mt.Guest}})
  }
  s.World.Meetings[i]=mt
 }
 announce:=func(t string){appendEvent(s,Event{Kind:"announcement",Actor:"school",Text:t,Audience:ids(j.Profiles)});for _,id:=range ids(j.Profiles){wakeWaiting(s,id,"Novo anúncio escolar")}}
 switch now{
 case 1440+600:
  group:=ids(j.Profiles)
  // Assignment is deterministic for replay, not exposed in actor context.
  h:=sha256.Sum256([]byte(Digest(Journal{Version:j.Version,Profiles:j.Profiles,Model:j.Model})))
  vip:=group[int(h[0])%len(group)]
  s.World.Exam=&VIPExam{Phase:"early",Group:group,VIP:vip,Submitted:map[string]bool{}}
  announce("Início do Zodiac/VIP adaptado: grupo único. Janela antecipada até 14:30; janela final 14:30–15:00; publicação às 16:00. Consulte as regras privadas.")
  for _,id:=range group{appendEvent(s,Event{Kind:"exam",Actor:"school",Text:fmt.Sprintf("Sua função VIP: %t",id==vip),Audience:[]string{id}})}
 case 1440+840:announce("Fim do período ativo do exame; submissões antecipadas continuam até 14:30.")
 case 1440+870:if e:=s.World.Exam;e!=nil&&e.Outcome==0{e.Phase="final"};announce("Janela final de submissões do exame aberta.")
 case 1440+900:if e:=s.World.Exam;e!=nil&&e.Outcome==0{settleExam(j,s,2,"")};announce("Submissões encerradas.")
 case 1440+960:if e:=s.World.Exam;e!=nil{announce(fmt.Sprintf("Resultado do grupo Zodiac/VIP: Outcome %d. CP: A=%d B=%d C=%d D=%d.",e.Outcome,s.World.ClassPoints["A"],s.World.ClassPoints["B"],s.World.ClassPoints["C"],s.World.ClassPoints["D"]))}
 default:
  switch now%1440{
  case 480:announce("Novo dia escolar. Confira seus compromissos e recursos.")
  case 540:announce("Orientação e aulas da manhã; as transferências voluntárias de PP e compras são permitidas.")
  case 720:announce("Intervalo de almoço. Refeição essencial gratuita disponível no refeitório.")
  case 780:announce("Período de estudos da tarde.")
  case 900:announce("Fim das aulas; atividades livres.")
  case 1080:announce("Horário de jantar e retorno ao dormitório.")
  case 1320:announce("Período de repouso. Considere descansar.")
  }
 }
}
func dialogueStale(events []Event,id,peer,text string,old int)int{
 words:=func(t string)map[string]bool{m:=map[string]bool{};for _,w:=range strings.FieldsFunc(strings.ToLower(t),func(r rune)bool{return !unicode.IsLetter(r)&&!unicode.IsNumber(r)}){if len(w)>3{m[w]=true}};return m}
 a:=words(text);seen:=0
 for i:=len(events)-2;i>=0;i--{e:=events[i];if e.Kind!="claim"||e.Actor!=id||e.Target!=peer{continue};seen++;b:=words(e.Text);common:=0;for w:=range a{if b[w]{common++}};union:=len(a)+len(b)-common
  if union>0&&common*100>=70*union{return old+1};if seen==3{break}
 }
 return 0
}

package pilot

import (
 "crypto/sha256"
 "fmt"
 "sort"
 "strings"
)

// School-safe, bounded adaptations. Canon references are provenance, not
// a grant of future knowledge or a claim of exact reproduction.
type ExamTemplate struct {
 Kind,Title,Source string
 MinGroup int
 Grades []int
 AfterDisclosure bool
}
var portfolioTemplates=[]ExamTemplate{
 {"minority","Voto de minoria","Tomodachi Game — coordenação por minoria (modelo experimental)",3,[]int{1,2,3},false},
 {"vip","Zodiac/VIP","COTE: y1-zodiac-cruise.json",4,[]int{1},true},
 {"kokkuri","Kokkuri — unanimidade","Tomodachi Game: kokkuri-san.json",3,[]int{1,2,3},false},
 {"janken","Friendship Janken — cartas","Tomodachi Game: friendship-janken.json",3,[]int{1,2,3},false},
 {"poll","Class Poll — consulta anônima","COTE: y1-class-poll.json",4,[]int{1,2,3},false},
 {"island","Ilha — orçamento de provisões","COTE: y1-deserted-island.json",4,[]int{1,2,3},false},
 {"trial","Friend-Sin Trial — júri","Tomodachi Game: friend-sin-trial.json",3,[]int{1,2,3},false},
 {"hide","Friendship Hide-and-Seek — setores","Tomodachi Game: hide-and-seek.json",4,[]int{1,2,3},false},
}
func portfolioTemplate(kind string)(ExamTemplate,bool){for _,t:=range portfolioTemplates{if t.Kind==kind{return t,true}};return ExamTemplate{},false}
func examKey(j Journal,scope string)string{return fmt.Sprintf("%x",sha256.Sum256([]byte(j.WorldSeed+"/portfolio/v1/"+scope)))}

// Each eligible template is used once before repeating when feasible.
// The seed decides order and tie-breaks. Select grades and members by
// least prior participation. VIP is year-one-only because CP is year-one.
func generatePortfolioCalendar(j Journal)[]ExamSlot{
 byYear:=map[int][]string{}
 for _,id:=range ids(j.Profiles){if isStudent(j,id){y:=j.Population[id].Year;byYear[y]=append(byYear[y],id)}}
 use:=map[string]int{};yearUse:=map[int]int{};actorUse:=map[string]int{};out:=[]ExamSlot{}
 for i:=0;i<18;i++{
  day:=int64(3+i*20)
  key:=fmt.Sprintf("%02d",i)
  candidates:=append([]ExamTemplate{},portfolioTemplates...)
  sort.Slice(candidates,func(a,b int)bool{x,y:=candidates[a],candidates[b];if use[x.Kind]!=use[y.Kind]{return use[x.Kind]<use[y.Kind]};return examKey(j,key+x.Kind)<examKey(j,key+y.Kind)})
  for _,t:=range candidates{
   if t.AfterDisclosure&&(day-1)*1440+600<schoolDisclosure{continue}
   ys:=append([]int{},t.Grades...)
   sort.Slice(ys,func(a,b int)bool{if yearUse[ys[a]]!=yearUse[ys[b]]{return yearUse[ys[a]]<yearUse[ys[b]]};return examKey(j,fmt.Sprintf("%s/year/%d",key,ys[a]))<examKey(j,fmt.Sprintf("%s/year/%d",key,ys[b]))})
   selected:=false
   for _,y:=range ys{
    pool:=append([]string{},byYear[y]...)
    // Poll is a consultation within one class; the other templates
    // may use students from multiple classes of the same grade.
    if t.Kind=="poll"{
     classes:=[]string{"A","B","C","D"};sort.Slice(classes,func(a,b int)bool{return examKey(j,key+classes[a])<examKey(j,key+classes[b])})
     chosen:=[]string{}
     for _,cl:=range classes{g:=[]string{};for _,id:=range pool{if classOf(j,id)==cl{g=append(g,id)}};if len(g)>=t.MinGroup{chosen=g;break}}
     pool=chosen
    }
    if len(pool)<t.MinGroup{continue}
    sort.Slice(pool,func(a,b int)bool{if actorUse[pool[a]]!=actorUse[pool[b]]{return actorUse[pool[a]]<actorUse[pool[b]]};return examKey(j,key+pool[a])<examKey(j,key+pool[b])})
    n:=8+i%3;if t.Kind=="kokkuri"||t.Kind=="janken"{n=5};if n>len(pool){n=len(pool)}
    group:=append([]string{},pool[:n]...)
    if t.Kind=="vip"{diverse:=false;for _,id:=range group{if classOf(j,id)!=classOf(j,group[0]){diverse=true}};if !diverse{for _,id:=range pool[n:]{if classOf(j,id)!=classOf(j,group[0]){group[n-1]=id;diverse=true;break}}};if !diverse{continue}}
    sort.Strings(group)
    out=append(out,ExamSlot{ID:fmt.Sprintf("exam-%02d",i+1),Kind:t.Kind,Day:day,Start:(day-1)*1440+600,Group:group,Title:t.Title,Source:t.Source,Years:[]int{y}})
    use[t.Kind]++;yearUse[y]++;for _,id:=range group{actorUse[id]++};selected=true;break
   }
   if selected{break}
  }
 }
 return out
}
func portfolioOffers(j Journal,s State,id string,e *VIPExam)([]string,[]string){
 if !has(e.Group,id){return nil,nil}
 var rules,opts []string
 switch e.Kind{
 case "kokkuri":
  rules=[]string{"Kokkuri adaptado: a questão pública é '2 + 3 = 5?'. Cada participante vota sim ou não privadamente uma vez. Só unanimidade correta de todo o grupo rende 25.000 PP a cada membro; qualquer omissão ou divergência não rende prêmio."};opts=[]string{"yes","no"}
 case "janken":
  rules=[]string{"Janken adaptado: cada participante escolhe pedra, papel ou tesoura privadamente. Se exatamente dois símbolos aparecem, todos que escolheram o símbolo vencedor ganham 25.000 PP. Um ou três símbolos, ou ausência de voto, não gera vitória para o abstente."};opts=[]string{"rock","paper","scissors"}
 case "poll":
  rules=[]string{"Class Poll adaptado, sem expulsão: escolha uma única aprovação ou censura anônima de outro integrante. A pontuação é aprovações menos censuras. Só o maior saldo positivo isolado ganha 25.000 PP; empates não rendem prêmio. Nenhuma atribuição de voto é divulgada."}
  for _,peer:=range e.Group{if peer!=id{opts=append(opts,"praise-"+peer,"censure-"+peer)}}
 case "island":
  rules=[]string{"Ilha adaptada: o grupo dispõe de 300 pontos de orçamento virtual. Cada membro escolhe pacote de provisões de 20 ou 40 pontos. Para concluir, todos devem escolher, pelo menos metade (arredondada para cima) deve escolher 40 e o gasto não pode exceder 300. Cada integrante recebe 100 PP por ponto remanescente. Sem deslocamento real, lesões ou CP."};opts=[]string{"basic","enhanced"}
 case "trial":
  rules=[]string{"Júri adaptado: cada membro recebe um voto de apoio sem custo privado. Somente apoio unânime de todos os jurados libera prêmio de 25.000 PP a cada um. Votos privados; não há julgamento de crimes reais, acusações ou expulsões."};opts=[]string{"support","withhold"}
 case "hide":
  side:="busca";for i,peer:=range e.Group{if id==peer&&i%2==0{side="ocultação"}}
  rules=[]string{"Hide-and-Seek adaptado: cada membro escolhe setor A, B ou C privadamente. Os integrantes de ocultação ganham 25.000 PP se nenhum integrante de busca escolher seu setor; integrantes de busca ganham 25.000 PP se encontrarem pelo menos um setor escolhido pela ocultação. Equipes públicas pela ordem alternada da lista: índices 1,3,5... ocultação; 2,4,6... busca. Não altera localização física.","Sua equipe: "+side};opts=[]string{"a","b","c"}
 }
 if e.Outcome!=0||e.Submitted[id]||(e.Phase!="early"&&e.Phase!="final"){opts=nil}
 return rules,opts
}
func recordPortfolioVotes(j Journal,before State,s *State,choices []Chosen){
 e:=s.World.Exam;if e==nil||e.Outcome!=0{return}
 for _,c:=range choices{
  if c.Decision.Action!="vote"||e.Submitted[c.Actor]{continue}
  _,opts:=portfolioOffers(j,before,c.Actor,before.World.Exam)
  if !has(opts,c.Decision.Target){continue}
  e.Submitted[c.Actor]=true;e.Ballots[c.Actor]=c.Decision.Target
  appendEvent(s,Event{Kind:"exam",Actor:c.Actor,Text:"Escolha privada registrada.",Audience:[]string{c.Actor}})
 }
}
func portfolioWinners(e *VIPExam)(map[string]int64){
 result:=map[string]int64{};reward:=func(id string){result[id]=25000}
 switch e.Kind{
 case "kokkuri","trial":
  correct:="yes";if e.Kind=="trial"{correct="support"}
  for _,id:=range e.Group{if e.Ballots[id]!=correct{return result}}
  for _,id:=range e.Group{reward(id)}
 case "janken":
  seen:=map[string]bool{};for _,id:=range e.Group{if b:=e.Ballots[id];b!=""{seen[b]=true}}
  if len(seen)!=2{return result}
  winner:="";if seen["rock"]&&seen["scissors"]{winner="rock"}else if seen["paper"]&&seen["rock"]{winner="paper"}else if seen["scissors"]&&seen["paper"]{winner="scissors"}
  for _,id:=range e.Group{if e.Ballots[id]==winner&&winner!=""{reward(id)}}
 case "poll":
  scores:=map[string]int{};for _,id:=range e.Group{v:=e.Ballots[id];if strings.HasPrefix(v,"praise-"){scores[strings.TrimPrefix(v,"praise-")]++};if strings.HasPrefix(v,"censure-"){scores[strings.TrimPrefix(v,"censure-")]--}}
  best:=0;winner:="";ties:=0;for _,id:=range e.Group{n:=scores[id];if n>best{best=n;winner=id;ties=1}else if n==best{ties++}}
  if best>0&&ties==1{reward(winner)}
 case "island":
  cost,enhanced:=0,0;for _,id:=range e.Group{switch e.Ballots[id]{case "basic":cost+=20;case "enhanced":cost+=40;enhanced++;default:return result}}
  if cost<=300&&enhanced*2>=len(e.Group)&&cost<300{for _,id:=range e.Group{result[id]=int64(300-cost)*100}}
 case "hide":
  hiding,searching:=map[string]bool{},map[string]bool{}
  for i,id:=range e.Group{v:=e.Ballots[id];if v==""{continue};if i%2==0{hiding[v]=true}else{searching[v]=true}}
  for i,id:=range e.Group{v:=e.Ballots[id];if v==""{continue};if i%2==0&&!searching[v]||i%2==1&&hiding[v]{reward(id)}}
 }
 return result
}
func settlePortfolioExam(j Journal,s *State){
 e:=s.World.Exam;if e==nil||e.Outcome!=0{return}
 winners:=portfolioWinners(e);e.Outcome=2;if len(winners)>0{e.Outcome=1};e.Phase="settled"
 for _,id:=range e.Group{if n:=winners[id];n>0{pay(s,"school",id,n,"exam","Prêmio "+e.ID)}}
}

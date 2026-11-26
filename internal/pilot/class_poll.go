package pilot

import (
 "fmt"
 "sort"
 "strconv"
 "strings"
)
const pollRevocationCost int64=20000000
type PollState struct{
 Votes map[string][]string `json:"votes,omitempty"`
 Net map[string]int `json:"net,omitempty"`
 TopCandidates []string `json:"top_candidates,omitempty"`
 BottomCandidates []string `json:"bottom_candidates,omitempty"`
 Deciding map[string]string `json:"deciding,omitempty"`
 Winner string `json:"winner,omitempty"`
 Loser string `json:"loser,omitempty"`
 Escrow map[string]int64 `json:"escrow,omitempty"`
 Status string `json:"status"`
 Revoked bool `json:"revoked,omitempty"`
}
func expelled(s State,id string)bool{return s.World!=nil&&s.World.Expelled[id]}
func pollMembers(j Journal,e *VIPExam)[]string{
 if len(e.Group)==0{return nil};year:=rosterYear(j,e.Group[0]);roster:=gradeRoster(j,year);out:=[]string{};for _,c:=range []string{"A","B","C","D"}{out=append(out,roster[c]...)};sort.Strings(out);return out
}
func pollTotal(p *PollState)int64{n:=int64(0);for _,v:=range p.Escrow{n+=v};return n}
func pollOptions(j Journal,s State,id string,e *VIPExam)[]string{
 p:=e.Poll;if p==nil||e.Outcome!=0||!has(e.Group,id){return nil}
 if e.Phase=="early"{
  old:=p.Votes[id];n:=len(old);if n>=7{return nil};prefix:="praise-";candidates:=e.Group;if n>=3{prefix="censure-"};if n==6{prefix="cross-";candidates=pollMembers(j,e)}
  opts:=[]string{};for _,peer:=range candidates{if peer==id||n==6&&has(e.Group,peer)||n<6&&!has(e.Group,peer){continue};used:=false;for _,v:=range old{if strings.TrimPrefix(strings.TrimPrefix(v,"praise-"),"censure-")==peer{used=true}};if !used{opts=append(opts,prefix+peer)}};return opts
 }
 if e.Phase=="final"&&p.Deciding[id]==""{if p.Status=="top"{return clone(p.TopCandidates)};if p.Status=="bottom"{out:=[]string{};for _,v:=range p.BottomCandidates{if v!=p.Winner{out=append(out,v)}};return out}}
 if e.Phase=="appeal"&&p.Loser!=""&&!p.Revoked{
  remaining:=pollRevocationCost-pollTotal(p);wallet:=s.Minds[id].Wallet;opts:=[]string{}
  for _,n:=range []int64{100000,1000000,10000000,20000000}{if n<=remaining&&n<=wallet{opts=append(opts,"fund-"+strconv.FormatInt(n,10))}}
  if remaining>0&&remaining<=wallet&&!has(opts,"fund-"+strconv.FormatInt(remaining,10)){opts=append(opts,"fund-"+strconv.FormatInt(remaining,10))}
  return opts
 }
 return nil
}
func pollOffers(j Journal,s State,id string,e *VIPExam)([]string,[]string){
 p:=e.Poll;if p==nil{return nil,nil}
 rules:=[]string{"Class Poll: quatro dias de preparação; cada aluno deve registrar três aprovações e três censuras de colegas distintos da própria classe e uma aprovação de outra classe, anonimamente. Maior saldo recebe proteção; menor saldo pode ser expulso. A escola abre uma segunda janela para votos faltantes ou revotação se uma classe ficar toda em zero. Extremos empatados exigem voto decisivo; empate persistente usa sorteio escolar determinístico declarado nesta adaptação. Sem alterações de CP. Revogação custa 20 milhões PP, via contribuições voluntárias da própria classe; se insuficientes, são devolvidas. Sem votação válida não há expulsão automática."}
 switch e.Phase{case "early":rules=append(rules,fmt.Sprintf("Você registrou %d/7 escolhas privadas nesta votação. Não conhece votos dos outros.",len(p.Votes[id])))
 case "final":rules=append(rules,"Voto decisivo: "+p.Status+"; consulte apenas os candidatos disponíveis.")
 case "appeal":rules=append(rules,fmt.Sprintf("Resultado preliminar: proteção %s; expulsão %s. Fundo confirmado %d de 20.000.000 PP.",p.Winner,p.Loser,pollTotal(p)))
 }
 return rules,pollOptions(j,s,id,e)
}
func recordClassPoll(j Journal,before State,s *State,choices []Chosen){
 e:=s.World.Exam;if e==nil||e.Poll==nil||before.World==nil||before.World.Exam==nil{return};p:=e.Poll
 for _,c:=range choices{if c.Decision.Action!="vote"||!has(pollOptions(j,before,c.Actor,before.World.Exam),c.Decision.Target){continue}
  id,v:=c.Actor,c.Decision.Target
  switch e.Phase{
  case "early":if p.Votes==nil{p.Votes=map[string][]string{}};p.Votes[id]=append(p.Votes[id],v);if len(p.Votes[id])==7{if e.Submitted==nil{e.Submitted=map[string]bool{}};e.Submitted[id]=true}
  case "final":if p.Deciding==nil{p.Deciding=map[string]string{}};p.Deciding[id]=v;if e.Submitted==nil{e.Submitted=map[string]bool{}};e.Submitted[id]=true
  case "appeal":n,err:=strconv.ParseInt(strings.TrimPrefix(v,"fund-"),10,64);if err!=nil||n<=0||n>pollRevocationCost-pollTotal(p){continue};if pay(s,id,"school",n,"poll-revocation","Depósito condicional de revogação "+e.ID){if p.Escrow==nil{p.Escrow=map[string]int64{}};p.Escrow[id]+=n}
  }
  appendEvent(s,Event{Kind:"exam",Actor:id,Target:e.ID,Text:"Ação privada do Class Poll confirmada.",Audience:[]string{id}})
 }
}
func fillGenericPoll(j Journal,s *State,e *VIPExam){
 for _,id:=range e.Group{if known(j.Profiles,id){continue};p:=e.Poll
  for step:=0;step<7;step++{opts:=pollOptions(j,*s,id,e);if len(opts)==0{break}
   sort.Slice(opts,func(a,b int)bool{return examKey(j,e.ID+"/"+id+"/"+fmt.Sprint(step)+"/"+opts[a])<examKey(j,e.ID+"/"+id+"/"+fmt.Sprint(step)+"/"+opts[b])})
   if e.Phase=="early"{if p.Votes==nil{p.Votes=map[string][]string{}};p.Votes[id]=append(p.Votes[id],opts[0]);if len(p.Votes[id])==7{if e.Submitted==nil{e.Submitted=map[string]bool{}};e.Submitted[id]=true}}else if e.Phase=="final"{if p.Deciding==nil{p.Deciding=map[string]string{}};p.Deciding[id]=opts[0];if e.Submitted==nil{e.Submitted=map[string]bool{}};e.Submitted[id]=true;break}else{break}
  }
 }
}
func pollNet(groups []*VIPExam)(map[string]int,bool){
 net:=map[string]int{};for _,e:=range groups{for _,id:=range e.Group{net[id]=0}}
 for _,e:=range groups{for _,id:=range e.Group{votes:=e.Poll.Votes[id];if len(votes)!=7{return nil,false};for _,v:=range votes{switch{case strings.HasPrefix(v,"praise-"):net[strings.TrimPrefix(v,"praise-")]++;case strings.HasPrefix(v,"censure-"):net[strings.TrimPrefix(v,"censure-")]--;case strings.HasPrefix(v,"cross-"):net[strings.TrimPrefix(v,"cross-")]++;default:return nil,false}}}}
 return net,true
}
func pollExtremes(group []string,net map[string]int)(top,bottom []string,zero bool){
 if len(group)==0{return nil,nil,true};max,min:=net[group[0]],net[group[0]];zero=true;for _,id:=range group{v:=net[id];if v!=0{zero=false};if v>max{max=v};if v<min{min=v}}
 for _,id:=range group{if net[id]==max{top=append(top,id)};if net[id]==min{bottom=append(bottom,id)}};return
}
func pollStimulate(j Journal,s *State,e *VIPExam,msg string){
 for _,id:=range e.Group{if known(j.Profiles,id)&&!expelled(*s,id){activateInvite(s,id);stimulate(s,id,msg)}}
}
func pollChoose(j Journal,e *VIPExam,candidates []string)(string,bool){
 if len(candidates)==1{return candidates[0],true};if len(candidates)==0{return "",false}
 counts:=map[string]int{};for _,id:=range e.Group{v:=e.Poll.Deciding[id];if !has(candidates,v){return "",false};counts[v]++}
 ranked:=clone(candidates);sort.Slice(ranked,func(a,b int)bool{if counts[ranked[a]]!=counts[ranked[b]]{return counts[ranked[a]]>counts[ranked[b]]};return examKey(j,e.ID+"/"+e.Poll.Status+"/"+ranked[a])<examKey(j,e.ID+"/"+e.Poll.Status+"/"+ranked[b])});return ranked[0],true
}
func pollIncomplete(e *VIPExam){e.Phase="settled";e.Outcome=2;e.Poll.Status="incomplete"}
func removeExpelled(s *State,id string){
 if s.World.Expelled==nil{s.World.Expelled=map[string]bool{}};s.World.Expelled[id]=true
 if m,ok:=s.Minds[id];ok{closeConversation(s,id);m=s.Minds[id];m.Location="off-campus";m.Activity=nil;m.PendingStimulus="";m.NextDeliberation=0;m.Conversation=nil;s.Minds[id]=m}
 for i,mt:=range s.World.Meetings{if (mt.Host==id||mt.Guest==id)&&(mt.Status=="pending"||mt.Status=="accepted"){mt.Status="expired";s.World.Meetings[i]=mt}}
}
func advanceClassPoll(j Journal,s *State,slot ExamSlot,idx int,announce func(string))bool{
 groups:=s.World.ExamGroups
 switch idx{
 case 0:
  s.World.Exam=nil;s.World.ExamGroups=nil
  for i,g:=range slot.Groups{e:=&VIPExam{ID:fmt.Sprintf("%s-g%02d",slot.ID,i+1),Kind:"poll",Group:clone(g),Phase:"preparation",PublishedAt:slot.Boundaries[len(slot.Boundaries)-1],Submitted:map[string]bool{},Poll:&PollState{Status:"preparation"}};s.World.ExamGroups=append(s.World.ExamGroups,e);pollStimulate(j,s,e,"Quatro dias para preparar o Class Poll")}
  announce("Class Poll anunciado: "+slot.ID+". Consulte as regras da prova.");return true
 case 1:for _,e:=range groups{e.Phase="early";e.Poll.Status="voting";pollStimulate(j,s,e,"Votação privada do Class Poll aberta")};return true
 case 2:
  for _,e:=range groups{fillGenericPoll(j,s,e)}
  net,ok:=pollNet(groups);revote:=false;if ok{for _,e:=range groups{_,_,zero:=pollExtremes(e.Group,net);if zero{revote=true}}}
  if revote{for _,e:=range groups{e.Poll.Votes=nil;e.Submitted=map[string]bool{};pollStimulate(j,s,e,"Revotação integral obrigatória")}}else if !ok{for _,e:=range groups{pollStimulate(j,s,e,"Última janela para completar os sete votos obrigatórios")}}else{for _,e:=range groups{e.Phase="review"}}
  return true
 case 3:
  for _,e:=range groups{if e.Phase=="early"{fillGenericPoll(j,s,e)}}
  net,ok:=pollNet(groups);if !ok{for _,e:=range groups{pollIncomplete(e)};return true}
  for _,e:=range groups{p:=e.Poll;top,bottom,zero:=pollExtremes(e.Group,net);if zero{pollIncomplete(e);continue};p.Net=map[string]int{};for _,id:=range e.Group{p.Net[id]=net[id]};p.TopCandidates=top;p.BottomCandidates=bottom;p.Deciding=nil;p.Status="top";e.Submitted=map[string]bool{};e.Phase="final";if len(top)==1{p.Winner=top[0];e.Phase="review"}else{pollStimulate(j,s,e,"Voto decisivo para proteção")}}
  return true
 case 4:
  for _,e:=range groups{if e.Outcome!=0{continue};p:=e.Poll;if p.Winner==""{fillGenericPoll(j,s,e);winner,ok:=pollChoose(j,e,p.TopCandidates);if !ok{pollIncomplete(e);continue};p.Winner=winner}
   candidates:=[]string{};for _,id:=range p.BottomCandidates{if id!=p.Winner{candidates=append(candidates,id)}};p.BottomCandidates=candidates;p.Status="bottom";p.Deciding=nil;e.Submitted=map[string]bool{};e.Phase="final";if len(candidates)==1{p.Loser=candidates[0];e.Phase="review"}else{pollStimulate(j,s,e,"Voto decisivo para expulsão")}
  };return true
 case 5:
  for _,e:=range groups{if e.Outcome!=0{continue};p:=e.Poll;if p.Loser==""{fillGenericPoll(j,s,e);loser,ok:=pollChoose(j,e,p.BottomCandidates);if !ok{pollIncomplete(e);continue};p.Loser=loser}
   if p.Winner==p.Loser{pollIncomplete(e);continue};p.Status="appeal";e.Phase="appeal";if s.World.Protection==nil{s.World.Protection=map[string]int{}};s.World.Protection[p.Winner]++
   for _,id:=range e.Group{if known(j.Profiles,id){appendEvent(s,Event{Kind:"exam",Actor:"school",Target:e.ID,Text:fmt.Sprintf("Resultado preliminar: %s recebe proteção; %s indicado para expulsão. Revogação possível por 20 milhões PP.",p.Winner,p.Loser),Audience:[]string{id}})}}
   pollStimulate(j,s,e,"Janela de revogação da expulsão")
  };return true
 case 6:
  for _,e:=range groups{if e.Outcome!=0{continue};p:=e.Poll;if p.Status!="appeal"||p.Loser==""{pollIncomplete(e);continue}
   if pollTotal(p)==pollRevocationCost{p.Revoked=true}else{donors:=[]string{};for id:=range p.Escrow{donors=append(donors,id)};sort.Strings(donors);for _,id:=range donors{pay(s,"school",id,p.Escrow[id],"poll-revocation","Devolução de depósito "+e.ID)};removeExpelled(s,p.Loser)}
   p.Status="resolved";e.Phase="settled";e.Outcome=1
  };return true
 case 7:
  result:=ExamResult{ID:slot.ID,Kind:"poll",Day:slot.Day,Group:clone(slot.Group),PublishedAt:s.Minute};for _,e:=range groups{gr:=GroupResult{ID:e.ID,Group:clone(e.Group),Outcome:e.Outcome,Poll:clone(e.Poll)};for _,id:=range e.Group{if len(e.Poll.Votes[id])==7{gr.Submitted=append(gr.Submitted,id);result.Submitted=append(result.Submitted,id)}};result.GroupResults=append(result.GroupResults,gr)}
  s.World.ExamHistory=append(s.World.ExamHistory,result);announce("Resultados publicados: "+slot.ID);return true
 };return true
}

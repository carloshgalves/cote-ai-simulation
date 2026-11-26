package pilot

import (
 "fmt"
 "sort"
)

// V27 is intentionally fail-closed: only the two operational adaptations
// below enter the school calendar. Reference-only exams remain in the library.
type SchoolExamRound struct{
 Number int `json:"number"`
 At int64 `json:"at"`
 Answer string `json:"answer"`
 Submitted []string `json:"submitted"`
}
func schoolExamCalendar(j Journal)[]ExamSlot{
 if j.Version>=InstitutionalVersion{return annualSchoolCalendar(j)}
 out:=[]ExamSlot{}
 for i,kind:=range []string{"kokkuri","vip"}{
  day:=int64(3);year:=1;size:=5;title:="Kokkuri — incentivos e confiança";source:="Tomodachi Game: modelo de referência adaptado à escola"
  if kind=="vip"{day=32;size=0;title="Zodiac/VIP — três dias";source="COTE: y1-zodiac-cruise.json; resolução simultânea e cronologia adotada"}
  id:=fmt.Sprintf("exam-%02d",i+1);roster:=gradeRoster(j,year)
  mixed:=[]string{}
  for _,c:=range []string{"A","B","C","D"}{g:=roster[c];sort.Slice(g,func(a,b int)bool{return examKey(j,id+g[a])<examKey(j,id+g[b])});roster[c]=g}
  for k:=0;k<40;k++{for _,c:=range []string{"A","B","C","D"}{mixed=append(mixed,roster[c][k])}}
  slot:=ExamSlot{ID:id,Kind:kind,Day:day,Start:(day-1)*1440+480,Group:clone(mixed),Title:title,Source:source,Years:[]int{year}}
  if size>0{
   for k:=0;k<len(mixed);k+=size{slot.Groups=append(slot.Groups,clone(mixed[k:k+size]))}
   // Five one-hour voting windows; publication an hour after the last.
   for k:=int64(0);k<=6;k++{slot.Boundaries=append(slot.Boundaries,slot.Start+k*60)}
  }else{
   // Twelve mixed-class groups, differing by at most one seat.
   at:=0;for k:=0;k<12;k++{n:=len(mixed)/12;if k<len(mixed)%12{n++};slot.Groups=append(slot.Groups,clone(mixed[at:at+n]));at+=n}
   slot.Boundaries=append(slot.Boundaries,slot.Start)
   for d:=int64(0);d<3;d++{base:=(day-1+d)*1440;slot.Boundaries=append(slot.Boundaries,base+780,base+840,base+1200,base+1260)}
   last:=(day+1)*1440;slot.Boundaries=append(slot.Boundaries,last+1290,last+1320,last+1380)
  }
  out=append(out,slot)
 }
 return out
}
func schoolKokkuriOffers(s State,id string,e *VIPExam)([]string,[]string){
 if !has(e.Group,id)||e.ExamRound<1||e.ExamRound>5{return nil,nil}
 q:=e.Group[e.ExamRound-1]
 rules:=[]string{fmt.Sprintf("Kokkuri escolar: rodada %d/5; questionador %s; saldo de dívida virtual próprio %d. Votos simultâneos e privados. Unanimidade decide; em divisão, a minoria decide. Nas quatro primeiras rodadas, SIM conclui; NÃO reduz à metade a dívida do questionador. Na quinta, SIM conclui (critério anunciado pela escola nesta adaptação). Ao concluir, 2.000.000 créditos virtuais divididos reduzem dívidas; se falhar cinco vezes, os maiores devedores absorvem a dívida total. Omissão invalida a rodada, não equivale a voto. Créditos não são PP nem dívida real. Fala fora do turno durante votação não está modelada.",e.ExamRound,q,e.VirtualDebt[id])}
 if id==q&&e.ExamRound<5{rules=append(rules,"Incentivo privado do questionador: um NÃO reduz sua dívida própria.")} // incentive already publicly described in adaptation; no hidden information fabricated
 if e.Outcome!=0||e.Submitted[id]||s.Minute>=e.Deadline{return rules,nil}
 return rules,[]string{"yes","no"}
}
func recordSchoolKokkuri(j Journal,before State,s *State,choices []Chosen){
 e:=s.World.Exam;if e==nil||e.Outcome!=0{return}
 for _,c:=range choices{
  if c.Decision.Action!="vote"||!has(e.Group,c.Actor)||e.Submitted[c.Actor]{continue}
  _,opts:=schoolKokkuriOffers(before,c.Actor,before.World.Exam);if !has(opts,c.Decision.Target){continue}
  if e.Ballots==nil{e.Ballots=map[string]string{}};if e.Submitted==nil{e.Submitted=map[string]bool{}}
  e.Ballots[c.Actor]=c.Decision.Target;e.Submitted[c.Actor]=true
  appendEvent(s,Event{Kind:"exam",Actor:c.Actor,Target:e.ID,Text:"Voto privado confirmado.",Audience:[]string{c.Actor}})
 }
}
func resolveSchoolKokkuri(j Journal,s *State,e *VIPExam){
 if e.Outcome!=0{return}
 // Generic baseline votes before the deadline are modeled as a sealed
 // deterministic decision, not access to named players' ballots.
 for _,id:=range e.Group{if known(j.Profiles,id)||e.Submitted[id]{continue};if e.Ballots==nil{e.Ballots=map[string]string{}};if e.Submitted==nil{e.Submitted=map[string]bool{}}
  v:="yes";if id==e.Group[e.ExamRound-1]{v="no"};e.Ballots[id]=v;e.Submitted[id]=true
 }
 yes:=0;submitted:=[]string{};for _,id:=range e.Group{if e.Submitted[id]{submitted=append(submitted,id)};if e.Ballots[id]=="yes"{yes++}}
 answer:="invalid";if len(submitted)==len(e.Group){answer="no";if yes==len(e.Group)||yes>0&&yes<len(e.Group)-yes{answer="yes"}}
 e.RoundHistory=append(e.RoundHistory,SchoolExamRound{e.ExamRound,s.Minute,answer,submitted})
 audience:=[]string{};for _,id:=range e.Group{if known(j.Profiles,id){audience=append(audience,id)}}
 appendEvent(s,Event{Kind:"exam",Actor:"school",Target:e.ID,Text:fmt.Sprintf("Rodada %d: resultado coletivo %s. Votos individuais permanecem privados.",e.ExamRound,answer),Audience:audience})
 if answer=="no"&&e.ExamRound<5{q:=e.Group[e.ExamRound-1];e.VirtualDebt[q]/=2}
 if answer=="yes"{e.Outcome=1;e.Phase="settled";for _,id:=range e.Group{n:=e.VirtualDebt[id]-400000;if n<0{n=0};e.VirtualDebt[id]=n};return}
 if e.ExamRound==5{
  e.Outcome=2;e.Phase="settled";total,max:=int64(0),int64(-1);worst:=[]string{}
  for _,id:=range e.Group{n:=e.VirtualDebt[id];total+=n;if n>max{max=n;worst=[]string{id}}else if n==max{worst=append(worst,id)};e.VirtualDebt[id]=0}
  sort.Strings(worst);for i,id:=range worst{e.VirtualDebt[id]=total/int64(len(worst));if int64(i)<total%int64(len(worst)){e.VirtualDebt[id]++}};return
 }
 e.ExamRound++;e.Ballots=map[string]string{};e.Submitted=map[string]bool{};e.Deadline=s.Minute+60
 for _,id:=range audience{activateInvite(s,id);stimulate(s,id,"Nova rodada de Kokkuri")}
}
func advanceSchoolExam(j Journal,s *State,announce func(string))bool{
 for _,slot:=range s.World.Calendar{
  bs:=examBoundaries(slot);if len(bs)==0||s.Minute<bs[0]||s.Minute>bs[len(bs)-1]{continue}
  idx:=-1;for i,n:=range bs{if n==s.Minute{idx=i;break}};if idx<0{return false}
  if j.Version>=ClassPollVersion&&slot.Kind=="poll"{return advanceClassPoll(j,s,slot,idx,announce)}
  if idx==0{
   s.World.Exam=nil;s.World.ExamGroups=nil
   for i,group:=range slot.Groups{
    e:=&VIPExam{ID:fmt.Sprintf("%s-g%02d",slot.ID,i+1),Kind:slot.Kind,Group:clone(group),Phase:"early",PublishedAt:bs[len(bs)-1],Submitted:map[string]bool{},Ballots:map[string]string{}}
    if slot.Kind=="vip"{pool:=clone(group);sort.Slice(pool,func(a,b int)bool{return examKey(j,e.ID+pool[a])<examKey(j,e.ID+pool[b])});e.VIP=pool[0]}else{e.ExamRound=1;e.Deadline=bs[1];e.VirtualDebt=map[string]int64{};for _,id:=range group{e.VirtualDebt[id]=4000000}}
    s.World.ExamGroups=append(s.World.ExamGroups,e)
    for _,id:=range group{if !known(j.Profiles,id){continue};activateInvite(s,id);msg:="Consulte regras privadas.";if e.Kind=="vip"{msg=fmt.Sprintf("Sua função VIP: %t",id==e.VIP)};appendEvent(s,Event{Kind:"exam",Actor:"school",Target:e.ID,Text:msg,Audience:[]string{id}});stimulate(s,id,"Início do exame "+slot.ID)}
   };announce(fmt.Sprintf("Início de %s: %d alunos, %d grupos",slot.Title,len(slot.Group),len(slot.Groups)));return true
  }
  if idx==len(bs)-1{
   res:=ExamResult{ID:slot.ID,Kind:slot.Kind,Day:slot.Day,Group:clone(slot.Group),PublishedAt:s.Minute}
   for _,e:=range s.World.ExamGroups{gr:=GroupResult{ID:e.ID,Group:clone(e.Group),Outcome:e.Outcome,Rounds:clone(e.RoundHistory),VirtualDebt:clone(e.VirtualDebt)};seen:=map[string]bool{};for _,rd:=range e.RoundHistory{for _,id:=range rd.Submitted{seen[id]=true}};for _,id:=range e.Group{if e.Submitted[id]||seen[id]{gr.Submitted=append(gr.Submitted,id);res.Submitted=append(res.Submitted,id)}};res.GroupResults=append(res.GroupResults,gr)}
   s.World.ExamHistory=append(s.World.ExamHistory,res);announce("Resultados publicados: "+slot.ID);return true
  }
  if slot.Kind=="kokkuri"{for _,e:=range s.World.ExamGroups{resolveSchoolKokkuri(j,s,e)};return true}
  if idx==len(bs)-3{for _,e:=range s.World.ExamGroups{if e.Outcome==0{e.Phase="final";for _,id:=range e.Group{if known(j.Profiles,id){stimulate(s,id,"Janela final Zodiac/VIP")}}}};announce("Janela final Zodiac/VIP");return true}
  if idx==len(bs)-2{for _,e:=range s.World.ExamGroups{if e.Outcome==0{s.World.Exam=e;if j.Version>=InstitutionalVersion{applyGenericVIPFinal(j,s,e)};if e.Outcome==0{settleExam(j,s,2,"")}}};s.World.Exam=nil;announce("Submissões encerradas: "+slot.ID);return true}
  // Discussions are scheduled reminders. Physical attendance/channel
  // enforcement needs a separate adapter; never fabricate a conversation.
  if s.Minute%1440==780||s.Minute%1440==1200{for _,e:=range s.World.ExamGroups{if e.Outcome==0{for _,id:=range e.Group{if known(j.Profiles,id){stimulate(s,id,"Janela de discussão Zodiac/VIP")}}}};announce("Janela de discussão Zodiac/VIP")}
  return true
 }
 return false
}

package pilot

import (
 "crypto/sha256"
 "fmt"
 "sort"
)

type ExamSlot struct {
 ID string `json:"id"`
 Kind string `json:"kind"`
 Day int64 `json:"day"`
 Start int64 `json:"start"`
 Group []string `json:"group"`
 Title string `json:"title,omitempty"`
 Source string `json:"source,omitempty"`
 Years []int `json:"years,omitempty"`
 Groups [][]string `json:"groups,omitempty"`
}
type ExamResult struct {
 ID string `json:"id"`
 Kind string `json:"kind"`
 Day int64 `json:"day"`
 Group []string `json:"group"`
 Outcome int `json:"outcome"`
 Submitted []string `json:"submitted"`
 PublishedAt int64 `json:"published_at"`
 GroupResults []GroupResult `json:"group_results,omitempty"`
}

// A deterministic synthetic academic-year calendar. Early events cannot
// disclose the school's class-point mechanics. Participants rotate across
// all first-year individualized students rather than only the six core.
func generateExamCalendar(j Journal)[]ExamSlot{
 if j.Version>=WholeGradeVersion{return generateWholeGradeCalendar(j)}
 if j.Version>=PortfolioVersion{return generatePortfolioCalendar(j)}
 students:=[]string{}
 for _,id:=range ids(j.Profiles){if isStudent(j,id)&&j.Population[id].Year==1{students=append(students,id)}}
 key:=func(id string)string{return fmt.Sprintf("%x",sha256.Sum256([]byte(j.WorldSeed+"/exam-calendar/"+id)))}
 sort.Slice(students,func(a,b int)bool{return key(students[a])<key(students[b])})
 result:=[]ExamSlot{};if len(students)==0{return result}
 cursor:=0
 for i:=0;i<18;i++{
  day:=int64(3+20*i)
  kind:="minority";if i>=2&&i%2==0{kind="vip"}
  n:=8+i%3;if n>len(students){n=len(students)}
  group:=[]string{};for k:=0;k<n;k++{group=append(group,students[(cursor+k)%len(students)])};cursor=(cursor+n)%len(students);sort.Strings(group)
  result=append(result,ExamSlot{ID:fmt.Sprintf("exam-%02d",i+1),Kind:kind,Day:day,Start:(day-1)*1440+600,Group:group})
 }
 return result
}
func examBoundaries(s ExamSlot)[]int64{return []int64{s.Start,s.Start+240,s.Start+270,s.Start+300,s.Start+360}}
func recordGeneratedVotes(s *State,choices []Chosen){
 e:=s.World.Exam;if e==nil||e.Kind!="minority"||e.Outcome!=0||e.Phase=="settled"{return}
 for _,c:=range choices{
  if c.Decision.Action!="vote"||!has(e.Group,c.Actor)||e.Submitted[c.Actor]||!has([]string{"a","b"},c.Decision.Target){continue}
  e.Submitted[c.Actor]=true;e.Ballots[c.Actor]=c.Decision.Target
  appendEvent(s,Event{Kind:"exam",Actor:c.Actor,Text:"Voto privado recebido pela escola.",Audience:[]string{c.Actor}})
 }
}
func settleMinority(s *State){
 e:=s.World.Exam;if e==nil||e.Kind!="minority"||e.Outcome!=0{return}
 a,b:=0,0;for _,v:=range e.Ballots{if v=="a"{a++}else if v=="b"{b++}}
 winning:="";e.Outcome=2
 if a>0&&b>0&&a!=b{if a<b{winning="a";e.Outcome=1}else{winning="b";e.Outcome=3}}
 e.Phase="settled"
 for _,id:=range e.Group{if winning!=""&&e.Ballots[id]==winning{pay(s,"school",id,25000,"exam","Prêmio minoria "+e.ID)}}
}
func advanceGeneratedExam(j Journal,s *State,announce func(string))bool{
 now:=s.Minute
 for _,slot:=range s.World.Calendar{
  switch now{
  case slot.Start:
   group:=clone(slot.Group);vip:=""
   if slot.Kind=="vip"{h:=sha256.Sum256([]byte(j.WorldSeed+"/"+slot.ID+"/vip"));vip=group[int(h[0])%len(group)]}
   s.World.Exam=&VIPExam{ID:slot.ID,Kind:slot.Kind,PublishedAt:slot.Start+360,Phase:"early",Group:group,VIP:vip,Submitted:map[string]bool{},Ballots:map[string]string{}}
   label:="Exame de minoria";if slot.Kind=="vip"{label="Zodiac/VIP adaptado"};if slot.Title!=""{label=slot.Title}
   announce("Início de "+label+" ("+slot.ID+"). Participantes designados; consulte regras privadas. Resultado às 16:00.")
   for _,id:=range group{
    activateInvite(s,id)
    if slot.Kind=="vip"{appendEvent(s,Event{Kind:"exam",Actor:"school",Text:fmt.Sprintf("Sua função VIP: %t",id==vip),Audience:[]string{id}})}else{message:="Você participa do exame de minoria. Voto individual privado.";if j.Version>=PortfolioVersion&&slot.Kind!="minority"{message="Você foi designado para "+label+". Consulte suas regras e ações privadas."};appendEvent(s,Event{Kind:"exam",Actor:"school",Text:message,Audience:[]string{id}})}
    stimulate(s,id,"Início do exame "+slot.ID)
   }
   return true
  case slot.Start+240:
   announce("Período ativo encerrando: "+slot.ID);return true
  case slot.Start+270:
   if e:=s.World.Exam;e!=nil&&e.ID==slot.ID&&e.Outcome==0{e.Phase="final";stimulateExam(s,"Última janela do exame "+slot.ID)}
   announce("Janela final: "+slot.ID);return true
  case slot.Start+300:
   if e:=s.World.Exam;e!=nil&&e.ID==slot.ID&&e.Outcome==0{if slot.Kind=="minority"{settleMinority(s)}else if slot.Kind=="vip"{settleExam(j,s,2,"")}else if j.Version>=PortfolioVersion{settlePortfolioExam(j,s)}}
   announce("Submissões encerradas: "+slot.ID);return true
  case slot.Start+360:
   e:=s.World.Exam;if e!=nil&&e.ID==slot.ID{
    submitted:=[]string{};for _,id:=range e.Group{if e.Submitted[id]{submitted=append(submitted,id)}}
    s.World.ExamHistory=append(s.World.ExamHistory,ExamResult{ID:slot.ID,Kind:slot.Kind,Day:slot.Day,Group:clone(e.Group),Outcome:e.Outcome,Submitted:submitted,PublishedAt:now})
    announce(fmt.Sprintf("Resultado %s (%s): código %d. Consulte seus recibos privados.",slot.ID,slot.Kind,e.Outcome))
   }
   return true
  }
 }
 return false
}

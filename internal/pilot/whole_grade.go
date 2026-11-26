package pilot

import (
 "fmt"
 "sort"
 "strings"
)
type GroupResult struct{
 ID string `json:"id"`
 Group []string `json:"group"`
 Outcome int `json:"outcome"`
 Submitted []string `json:"submitted"`
 Rounds []SchoolExamRound `json:"rounds,omitempty"`
 VirtualDebt map[string]int64 `json:"virtual_debt,omitempty"`
}
func gradeKey(y int,c string)string{return fmt.Sprintf("%d-%s",y,c)}
// Unclassified named veterans get an explicit stable balancing prior;
// their class in the source Profile is not rewritten.
func rosterClass(j Journal,id string)string{
 if strings.HasPrefix(id,"npc-y")&&len(id)>=9{return strings.ToUpper(id[7:8])}
 counts:=map[string]int{};unknown:=[]string{};year:=j.Population[id].Year
 for _,p:=range j.Profiles{if p.ID==id&&p.Class!=""{return p.Class};if j.Population[p.ID].Year==year&&isStudent(j,p.ID){if p.Class==""{unknown=append(unknown,p.ID)}else{counts[p.Class]++}}}
 sort.Strings(unknown)
 for _,u:=range unknown{best:="A";for _,c:=range []string{"B","C","D"}{if counts[c]<counts[best]{best=c}};counts[best]++;if u==id{return best}}
 return ""
}
func gradeRoster(j Journal,y int)map[string][]string{
 out:=map[string][]string{"A":{},"B":{},"C":{},"D":{}}
 for _,id:=range ids(j.Profiles){if isStudent(j,id)&&j.Population[id].Year==y{c:=rosterClass(j,id);out[c]=append(out[c],id)}}
 for _,c:=range []string{"A","B","C","D"}{for i:=len(out[c]);i<40;i++{out[c]=append(out[c],fmt.Sprintf("npc-y%d-%s-%02d",y,strings.ToLower(c),i+1))}}
 return out
}
func rosterYear(j Journal,id string)int{if a,ok:=j.Population[id];ok{return a.Year};if strings.HasPrefix(id,"npc-y")&&len(id)>5{return int(id[5]-'0')};return 0}
func initializeGradeEconomy(j Journal,s *State){
 s.World.GradePoints=map[string]int64{};s.World.GenericWallets=map[string]int64{}
 for y:=1;y<=3;y++{roster:=gradeRoster(j,y);for _,c:=range []string{"A","B","C","D"}{s.World.GradePoints[gradeKey(y,c)]=1000;for _,id:=range roster[c]{if !known(j.Profiles,id){s.World.GenericWallets[id]=0;pay(s,"school",id,100000,"initial","Depósito inicial")}}}}
}
func payGradeIncome(j Journal,s *State){for y:=1;y<=3;y++{roster:=gradeRoster(j,y);for _,c:=range []string{"A","B","C","D"}{for _,id:=range roster[c]{pay(s,"school",id,s.World.GradePoints[gradeKey(y,c)]*100,"monthly","Renda mensal")}}}}
func transferGradePoints(j Journal,s *State,id,loser,winner string){
 y:=rosterYear(j,id);l,w:=gradeKey(y,loser),gradeKey(y,winner);delta:=int64(50);if s.World.GradePoints[l]<delta{delta=s.World.GradePoints[l]};s.World.GradePoints[l]-=delta;s.World.GradePoints[w]+=delta
 if y==1{s.World.ClassPoints[loser]=s.World.GradePoints[l];s.World.ClassPoints[winner]=s.World.GradePoints[w]}
}
func updateGradeCohorts(j Journal,s *State){
 s.Cohorts=[]Cohort{}
 for y:=1;y<=3;y++{roster:=gradeRoster(j,y);for _,c:=range []string{"A","B","C","D"}{n:=0;for _,id:=range roster[c]{if !known(j.Profiles,id){n++}};if n==0{continue};loc:="class-"+strings.ToLower(c);if y>1{loc=seniorClassroom(c)};switch periodAt(s.Minute){case "almoço":loc="cafeteria";case "repouso","jantar e repouso":loc="residential-area";case "atividades livres":loc="courtyard"};s.Cohorts=append(s.Cohorts,Cohort{ID:fmt.Sprintf("cohort-y%d-%s",y,strings.ToLower(c)),Class:c,Count:n,Location:loc,Tier:CohortTier,Year:y})}}
}
func generateWholeGradeCalendar(j Journal)[]ExamSlot{
 if j.Version>=SchoolExamVersion{return schoolExamCalendar(j)}
 // Choose template order from the seeded portfolio, but replace sampling
 // with the full grade roster. All 160 seats participate, independently of tier.
 old:=j;old.Version=PortfolioVersion;slots:=generatePortfolioCalendar(old)
 for i:=range slots{
  slot:=&slots[i];y:=slot.Years[0];roster:=gradeRoster(j,y);group:=[]string{}
  for _,c:=range []string{"A","B","C","D"}{group=append(group,roster[c]...)}
  slot.Group=clone(group)
  if slot.Kind=="poll"{for _,c:=range []string{"A","B","C","D"}{slot.Groups=append(slot.Groups,clone(roster[c]))};continue}
  // Interleave classes in seed-shuffled order so VIP groups are diverse.
  for _,c:=range []string{"A","B","C","D"}{g:=roster[c];sort.Slice(g,func(a,b int)bool{return examKey(j,slot.ID+g[a])<examKey(j,slot.ID+g[b])});roster[c]=g}
  group=nil;for k:=0;k<40;k++{for _,c:=range []string{"A","B","C","D"}{group=append(group,roster[c][k])}}
  size:=8;if slot.Kind=="kokkuri"||slot.Kind=="janken"{size=5}
  for k:=0;k<len(group);k+=size{end:=k+size;if end>len(group){end=len(group)};slot.Groups=append(slot.Groups,clone(group[k:end]))}
 }
 return slots
}
func examFor(s State,id string)*VIPExam{
 if s.World==nil{return nil}
 if len(s.World.ExamGroups)>0{for _,e:=range s.World.ExamGroups{if has(e.Group,id){return e}};return nil}
 return s.World.Exam
}
func resolveGradeChoices(j Journal,before State,s *State,choices []Chosen){
 saved:=s.World.Exam;defer func(){s.World.Exam=saved}()
 for _,e:=range s.World.ExamGroups{
  if e.Outcome!=0{continue};scoped:=[]Chosen{};for _,c:=range choices{if has(e.Group,c.Actor){scoped=append(scoped,c)}}
  s.World.Exam=e;b:=before;b.World=&SchoolWorld{Exam:examFor(before,e.Group[0])}
  if j.Version>=SchoolExamVersion&&e.Kind=="kokkuri"{recordSchoolKokkuri(j,b,s,scoped)}else if e.Kind=="vip"{settleGuesses(j,b,s,scoped)}else if e.Kind=="minority"{recordGeneratedVotes(s,scoped)}else{recordPortfolioVotes(j,b,s,scoped)}
 }
}
// Generic students have stable IDs and balances. Their baseline policy
// uses only own group options and a seeded preference, never hidden roles.
func genericGradeChoices(j Journal,s *State,e *VIPExam)[]Chosen{
 out:=[]Chosen{}
 for _,id:=range e.Group{if known(j.Profiles,id)||e.Submitted[id]{continue}
  d:=Decision{Action:"vote",Source:routineSource};opts:=[]string{}
  switch e.Kind{case "vip":if id==e.VIP{continue};d.Action="guess";for _,p:=range e.Group{if p!=id{opts=append(opts,p)}}
  case "minority":opts=[]string{"a","b"}
  default:_,opts=portfolioOffers(j,*s,id,e)}
  if len(opts)==0{continue};sort.Slice(opts,func(a,b int)bool{return examKey(j,e.ID+id+opts[a])<examKey(j,e.ID+id+opts[b])});d.Target=opts[0];out=append(out,Chosen{Actor:id,Decision:d})
 }
 return out
}
func advanceWholeGradeExam(j Journal,s *State,announce func(string))bool{
 if j.Version>=SchoolExamVersion{return advanceSchoolExam(j,s,announce)}
 for _,slot:=range s.World.Calendar{
  switch s.Minute{
  case slot.Start:
   s.World.ExamGroups=nil;s.World.Exam=nil
   for i,group:=range slot.Groups{
    e:=&VIPExam{ID:fmt.Sprintf("%s-g%02d",slot.ID,i+1),Kind:slot.Kind,Group:clone(group),Phase:"early",PublishedAt:slot.Start+360,Submitted:map[string]bool{},Ballots:map[string]string{}}
    if e.Kind=="vip"{pool:=clone(group);sort.Slice(pool,func(a,b int)bool{return examKey(j,e.ID+pool[a])<examKey(j,e.ID+pool[b])});e.VIP=pool[0]}
    s.World.ExamGroups=append(s.World.ExamGroups,e)
    for _,id:=range group{if !known(j.Profiles,id){continue};activateInvite(s,id);msg:="Consulte as regras privadas de seu grupo.";if e.Kind=="vip"{msg=fmt.Sprintf("Sua função VIP: %t",e.VIP==id)};appendEvent(s,Event{Kind:"exam",Actor:"school",Text:msg,Audience:[]string{id}});stimulate(s,id,"Início do exame "+slot.ID)}
   };announce(fmt.Sprintf("Início de %s: %d estudantes em %d grupos.",slot.Title,len(slot.Group),len(slot.Groups)));return true
  case slot.Start+240:announce("Período ativo encerrando: "+slot.ID);return true
  case slot.Start+270:
   for _,e:=range s.World.ExamGroups{if e.Outcome!=0{continue};e.Phase="final";for _,id:=range e.Group{if known(j.Profiles,id){stimulate(s,id,"Última janela do exame "+slot.ID)}}};announce("Janela final: "+slot.ID);return true
  case slot.Start+300:
   before:=clone(*s);all:=[]Chosen{};for _,e:=range s.World.ExamGroups{if e.Outcome==0{all=append(all,genericGradeChoices(j,s,e)...)}}
   resolveGradeChoices(j,before,s,all)
   for _,e:=range s.World.ExamGroups{if e.Outcome!=0{continue};s.World.Exam=e;if e.Kind=="vip"{settleExam(j,s,2,"")}else if e.Kind=="minority"{settleMinority(s)}else{settlePortfolioExam(j,s)}};s.World.Exam=nil;announce("Submissões encerradas: "+slot.ID);return true
  case slot.Start+360:
   result:=ExamResult{ID:slot.ID,Kind:slot.Kind,Day:slot.Day,Group:clone(slot.Group),PublishedAt:s.Minute}
   for _,e:=range s.World.ExamGroups{g:=GroupResult{ID:e.ID,Group:clone(e.Group),Outcome:e.Outcome};for _,id:=range e.Group{if e.Submitted[id]{g.Submitted=append(g.Submitted,id);result.Submitted=append(result.Submitted,id)}};result.GroupResults=append(result.GroupResults,g)}
   s.World.ExamHistory=append(s.World.ExamHistory,result);announce(fmt.Sprintf("Resultados publicados: %s, %d grupos.",slot.ID,len(result.GroupResults)));return true
  }
 }
 return false
}

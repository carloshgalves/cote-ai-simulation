package pilot

import (
 "bytes"
 "context"
 "path/filepath"
 "reflect"
 "strings"
 "testing"
)

func TestZodiacDay2Calendar(t *testing.T){
 for _,label:=range []string{"zodiac-day2","comparison-02","comparison-03"}{
  t.Run(label,func(t *testing.T){
   j:=autonomyJournal(t)
   j.Version=ClassPollVersion
   j.WorldSeed=experimentSeed(label)
   normal:=annualSchoolCalendar(j)
   j.Scenario=ScenarioZodiacDay2
   slots:=annualSchoolCalendar(j)
   if len(slots)!=18||!reflect.DeepEqual(slots,annualSchoolCalendar(j)){t.Fatal("unstable annual calendar")}
   first:=slots[0]
   if first.Kind!="vip"||first.Day!=2||first.Start!=1920||!reflect.DeepEqual(first.Years,[]int{1})||len(first.Groups)!=12||len(first.Group)!=160{t.Fatal("invalid first exam",first.ID,first.Kind,first.Day,first.Years,len(first.Groups),len(first.Group))}
   if first.Boundaries[len(first.Boundaries)-1]!=3*1440+1380{t.Fatal("unexpected result boundary")}
   seen:=map[string]bool{};classCounts:=map[string]int{};years:=map[int]int{}
   for _,group:=range first.Groups{
    if len(group)<13||len(group)>14{t.Fatal("unbalanced group size")}
    classes:=map[string]bool{}
    for _,id:=range group{
     if seen[id]||rosterYear(j,id)!=1{t.Fatal("duplicate or non-first-year participant",id)}
     seen[id]=true;c:=rosterClass(j,id);classCounts[c]++;classes[c]=true
    }
    if len(classes)!=4{t.Fatal("group lacks a class")}
   }
   if len(seen)!=160{t.Fatal("incomplete first-year roster")}
   for _,c:=range []string{"A","B","C","D"}{if classCounts[c]!=40{t.Fatal("class roster",c,classCounts[c])}}
   for _,id:=range ids(j.Profiles){if isStudent(j,id)&&rosterYear(j,id)==1&&!seen[id]{t.Fatal("named student excluded",id)}}
   for i,slot:=range slots{
    years[slot.Years[0]]++
    if i>0&&slots[i-1].Boundaries[len(slots[i-1].Boundaries)-1]>=slot.Start{t.Fatal("overlapping exams")}
    if i>0&&normal[i].Years[0]!=1&&!reflect.DeepEqual(slot,normal[i]){t.Fatal("unrelated slot changed",slot.ID)}
   }
   for y:=1;y<=3;y++{if years[y]!=6{t.Fatal("grade exam count",y,years[y])}}
   j.Scenario=""
   if !reflect.DeepEqual(normal,annualSchoolCalendar(j)){t.Fatal("normal calendar changed")}
  })
 }
}

func TestZodiacDay2SurpriseAndPrivateRoles(t *testing.T){
 model:=modelFunc(func(_ context.Context,_ Context)(Decision,error){return Decision{Action:"wait"},nil})
 s,err:=OpenClassPollScenario(filepath.Join(t.TempDir(),"experiment.json"),"test",model,ScenarioZodiacDay2,"comparison-01")
 if err!=nil{t.Fatal(err)}
 defer s.Close()
 j:=s.journal
 st,err:=initial(j);if err!=nil{t.Fatal(err)}
 baseline:=j;baseline.Scenario=""
 normal,err:=initial(baseline);if err!=nil{t.Fatal(err)}
 first:=st.World.Calendar[0]
 for _,minute:=range []int64{480,first.Start-1}{
  st.Minute=minute;normal.Minute=minute
  for _,id:=range ids(j.Profiles){
   if !isStudent(j,id)||rosterYear(j,id)!=1{continue}
   c:=actorContext(j,st,id)
   if !c.ordinaryLifeBaseline||c.School.Exam!=nil{t.Fatal("experimental context or preannouncement exam",id)}
   if strings.Contains(strings.Join(c.School.Rules," "),classGuarantee){t.Fatal("premature disclosure",id)}
   for _,o:=range c.School.Options{if o.Action=="guess"{t.Fatal("premature exam action",id)}}
   payload,err:=modelContextPayload(c);if err!=nil{t.Fatal(err)}
   other,err:=modelContextPayload(actorContext(baseline,normal,id));if err!=nil{t.Fatal(err)}
   if !bytes.Equal(payload,other){t.Fatal("future calendar leaked into actor payload",id)}
  }
 }
 if nextDue(j,st)!=first.Start{t.Fatal("scheduler would skip surprise boundary")}
 st.Minute=first.Start
 advanceWorld(j,&st)
 if len(st.World.ExamGroups)!=12{t.Fatal("exam not opened")}
 for _,id:=range ids(j.Profiles){
  c:=actorContext(j,st,id)
  if !isStudent(j,id)||rosterYear(j,id)!=1{
   if c.School.Exam!=nil{t.Fatal("outsider received private exam",id)}
   continue
  }
  e:=examFor(st,id)
  if e==nil||c.School.Exam==nil||!reflect.DeepEqual(c.School.Exam.Group,e.Group)||c.School.Exam.IsVIP!=(e.VIP==id){t.Fatal("private role or group mismatch",id)}
  n:=0
  for _,ev:=range visible(st,id){
   if strings.HasPrefix(ev.Text,"Sua função VIP:"){
    n++
    if len(ev.Audience)!=1||ev.Audience[0]!=id{t.Fatal("role leaked",id)}
   }
  }
  if n!=1{t.Fatal("role not delivered exactly once",id,n)}
  if strings.Contains(strings.Join(c.School.Rules," "),classGuarantee){t.Fatal("exam revealed graduation rule",id)}
  if st.Minds[id].PendingStimulus==""||st.Minds[id].Tier==LatentTier{t.Fatal("named participant not activated",id)}
 }
}

func TestZodiacDay2SeedAndSaveIsolation(t *testing.T){
 model:=modelFunc(func(_ context.Context,_ Context)(Decision,error){return Decision{Action:"wait"},nil})
 dir:=t.TempDir()
 a:=filepath.Join(dir,"weak.json");b:=filepath.Join(dir,"strong.json")
 first,err:=OpenClassPollScenario(a,"weak",model,ScenarioZodiacDay2,"comparison")
 if err!=nil{t.Fatal(err)}
 j:=first.journal;before:=first.Snapshot()
 if err:=first.Close();err!=nil{t.Fatal(err)}
 second,err:=OpenClassPollScenario(b,"strong",model,ScenarioZodiacDay2,"comparison")
 if err!=nil{t.Fatal(err)}
 if second.journal.WorldSeed!=j.WorldSeed||!reflect.DeepEqual(second.Snapshot().State.World.Calendar,before.State.World.Calendar){t.Fatal("models start with different world setup")}
 if err:=second.Close();err!=nil{t.Fatal(err)}
 if bad,err:=OpenClassPollScenario(a,"weak",model,ScenarioZodiacDay2,"other");err==nil{bad.Close();t.Fatal("changed seed on resume")}
 resumed,err:=OpenClassPoll(a,"weak",model)
 if err!=nil{t.Fatal(err)}
 if resumed.journal.Scenario!=ScenarioZodiacDay2||resumed.journal.WorldSeed!=j.WorldSeed||resumed.Snapshot().Digest!=before.Digest{t.Fatal("scenario not persisted")}
 resumed.Close()
 normalPath:=filepath.Join(dir,"normal.json")
 normal,err:=OpenClassPoll(normalPath,"weak",model)
 if err!=nil{t.Fatal(err)}
 if normal.journal.Scenario!=""||normal.Snapshot().State.World.Calendar[0].Day!=3{t.Fatal("default changed")}
 normal.Close()
 if bad,err:=OpenClassPollScenario(normalPath,"weak",model,ScenarioZodiacDay2,"comparison");err==nil{bad.Close();t.Fatal("normal save silently migrated")}
 j.Version=InstitutionalVersion
 if _,err:=initial(j);err==nil{t.Fatal("old journal accepted experimental policy")}
}

func TestZodiacDay2OrdinaryLifePrompt(t *testing.T){
 original:="o que deseja obter, testar, evitar ou descobrir. Considere agir mesmo sem ameaça ou novidade estratégica; se decidir esperar, fundamente em obstáculos concretos. procure resultados concretos. Não aguarde evidência perfeita quando uma experiência reversível puder produzir informação."
 got:=ordinaryLifePrompt(original)
 for _,old:=range []string{"o que deseja obter, testar, evitar ou descobrir","fundamente em obstáculos concretos","procure resultados concretos","Não aguarde evidência perfeita"}{if strings.Contains(got,old){t.Fatal("investigative framing retained",old)}}
 for _,want:=range []string{"prazer","esperar sem justificar um obstáculo","suspeitas específicas exigem indícios acessíveis","planejamento, negociação e investigação continuam permitidos"}{if !strings.Contains(got,want){t.Fatal("missing policy",want)}}
 for _,secret:=range []string{"Zodiac","zodiac-day2","VIP","segundo dia"}{if strings.Contains(got,secret){t.Fatal("prompt leaks surprise",secret)}}
}

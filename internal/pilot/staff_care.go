package pilot

// Provisional work/rest schedule; staff do not receive student PP. The
// private recovery room is a pilot abstraction, not a canon residence.
func staffOnDuty(j Journal,s State,id string)bool{
 p:=periodAt(s.Minute)
 return p=="aulas da manhã"||p=="aulas da tarde"||j.Population[id].Role=="staff"&&p=="atividades livres"
}
func staffUrgentCare(j Journal,s State,id string)bool{
 m:=s.Minds[id]
 return m.Body!=nil&&(m.Body.Energy<350||m.Body.Hydration<350||m.Body.Fatigue>850)||periodAt(s.Minute)=="repouso"
}
func staffCareDecision(j Journal,s State,id string)Decision{
 m:=s.Minds[id];v:=schoolView(j,s,id)
 d:=Decision{Action:"wait",WaitMinutes:30,Source:routineSource}
 goOr:=func(loc,action,target string)Decision{
  if m.Location!=loc{return Decision{Action:"move",Target:loc,Source:routineSource}}
  for _,o:=range v.Options{if o.Action==action&&o.Target==target{return Decision{Action:action,Target:target,Source:routineSource}}}
  return d
 }
 period:=periodAt(s.Minute)
 if m.Body!=nil&&(m.Body.Energy<350||m.Body.Hydration<350){return goOr("cafeteria","eat","free")}
 if period=="repouso"{return goOr("dormitory-"+id,"sleep","")}
 if m.Body!=nil&&m.Body.Fatigue>850{return goOr("dormitory-"+id,"rest","")}
 if period=="almoço"||period=="jantar e repouso"{
  if m.Body==nil||m.Body.Energy<700||m.Body.Hydration<650{return goOr("cafeteria","eat","free")}
  return goOr("dormitory-"+id,"rest","")
 }
 if staffOnDuty(j,s,id){return goOr(staffStation(j,id),"work","")}
 return goOr("courtyard","relax","")
}

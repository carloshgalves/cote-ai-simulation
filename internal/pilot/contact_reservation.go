package pilot

// The cohort is frozen. A contactable peer selecting a local, deferrable
// routine may hear an approach before starting that routine. Never cancel a
// model-selected move, sleep, meal, meeting, exam action or financial effect.
func reserveRoutineContacts(j Journal,s State,out *State,choices []Chosen)[]Chosen{
 byID:=map[string]Chosen{};for _,c:=range choices{byID[c.Actor]=c}
 reserved:=map[string]bool{}
 order:=eligibleActors(j,s)
 for i:=0;i<len(order);i++{
  id:=order[(i+s.Round)%len(order)];c:=byID[id];target,ok:=byID[c.Decision.Target]
  if c.Decision.Action!="talk"||!ok||reserved[id]||reserved[target.Actor]||target.Decision.Source!=routineSource{continue}
  if !has([]string{"move","wait","observe","study","read","relax","socialize"},target.Decision.Action){continue}
  m,p:=s.Minds[id],s.Minds[target.Actor]
  if m.Location!=p.Location||!canContact(m,target.Actor,p,id){continue}
  if p.Body!=nil&&(p.Body.Energy<350||p.Body.Hydration<350||p.Body.Fatigue>850){continue}
  if target.Decision.Action=="move"&&(target.Decision.Target=="cafeteria"||target.Decision.Target=="dormitory-"+target.Actor||periodAt(s.Minute)=="aulas da manhã"||periodAt(s.Minute)=="aulas da tarde"){continue}
  if nextCommitmentDeparture(j,s,target.Actor)>0&&nextCommitmentDeparture(j,s,target.Actor)<=s.Minute+5{continue}
  reserved[target.Actor]=true
  peer:=out.Minds[target.Actor];peer.Activity=&Activity{Kind:"contact_hold",Target:id,Until:s.Minute+1};out.Minds[target.Actor]=peer
  appendEvent(out,Event{Kind:"activity",Actor:target.Actor,Text:"Rotina adiada por abordagem presencial.",Audience:[]string{target.Actor}})
 }
 result:=[]Chosen{};for _,c:=range choices{if !reserved[c.Actor]{result=append(result,c)}}
 return result
}

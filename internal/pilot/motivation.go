package pilot

import "strings"

type PersonalOpportunity struct{
 Goal string `json:"goal"`
 Kind string `json:"kind"`
 Peer string `json:"peer,omitempty"`
 Location string `json:"location"`
}
func personalOpportunity(j Journal,s State,id string)*PersonalOpportunity{
 if !isStudent(j,id){return nil}
 m:=s.Minds[id]
 if m.Body!=nil&&(m.Body.Energy<350||m.Body.Hydration<350||m.Body.Fatigue>850){return nil}
 if m.LastOpportunityMinute>0&&s.Minute-m.LastOpportunityMinute<120{return nil}
 if j.Version>=CalibratedVersion&&m.OpportunityDay==s.Minute/1440+1&&m.OpportunityCount>=dailyOpportunityBudget(m){return nil}
 period:=periodAt(s.Minute)
 if period!="período livre"&&period!="atividades livres"{return nil}
 if m.Conversation!=nil{return nil}
 for _,goal:=range m.Goals{
  g:=strings.ToLower(goal)
  if strings.Contains(g,"vida escolar comum")||strings.Contains(g,"relaç")||strings.Contains(g,"coleg")||strings.Contains(g,"influência")||strings.Contains(g,"confiança")||strings.Contains(g,"vantagen"){
   peers:=ids(j.Profiles)
   if j.Version>=CalibratedVersion{peers=rotatedOpportunityPeers(j,s,id,peers)}
   for _,peer:=range peers{
    if peer==id||!isStudent(j,peer){continue}
    p:=s.Minds[peer]
    if p.Location!=m.Location||!canContact(m,peer,p,id){continue}
    b:=m.Social[peer]
    if b.LastMinute>0&&s.Minute-b.LastMinute<180{continue}
    return &PersonalOpportunity{Goal:goal,Kind:"social",Peer:peer,Location:m.Location}
   }
   // The actor may choose to seek ordinary contact; no hidden roster or
   // promise of any specific peer's presence is provided.
   return &PersonalOpportunity{Goal:goal,Kind:"explore",Location:m.Location}
  }
  if strings.Contains(g,"acadêm")||strings.Contains(g,"reconhecimento")||strings.Contains(g,"talento")||strings.Contains(g,"autonomia"){
   return &PersonalOpportunity{Goal:goal,Kind:"personal",Location:m.Location}
  }
 }
 if m.Plan!=nil&&len(m.Plan.Steps)>0&&len(m.Goals)>0{return &PersonalOpportunity{Goal:m.Goals[0],Kind:"plan",Location:m.Location}}
 return nil
}

package pilot

import (
 "fmt"
 "sort"
)

// Owner-side external political world. Policies are deterministic provisional
// dispositions, not fixed canon outcomes. No external actor is a school mind.
type InstitutionalActor struct {
 ID string `json:"id"`
 Name string `json:"name"`
 Organization string `json:"organization"`
 Goal string `json:"goal"`
 Budget int `json:"budget"`
 Influence int `json:"influence"`
 LastActionDay int64 `json:"last_action_day,omitempty"`
}
type InstitutionalRequest struct {
 ID string `json:"id"`
 From string `json:"from"`
 To string `json:"to"`
 Kind string `json:"kind"`
 Day int64 `json:"day"`
 Status string `json:"status"`
}
type InstitutionalEvent struct {
 Day int64 `json:"day"`
 Actor string `json:"actor"`
 Target string `json:"target,omitempty"`
 Action string `json:"action"`
 Request string `json:"request,omitempty"`
 Audience []string `json:"audience"`
}
type InstitutionalState struct {
 Actors map[string]InstitutionalActor `json:"actors"`
 Requests []InstitutionalRequest `json:"requests"`
 Events []InstitutionalEvent `json:"events"`
 LastDay int64 `json:"last_day,omitempty"`
}
func newInstitutionalState()*InstitutionalState{
 actors:=[]InstitutionalActor{
  {ID:"atsuomi-ayanokoji",Name:"Atsuomi Ayanokōji",Organization:"org.white_room",Goal:"funding",Budget:12,Influence:5},
  {ID:"tsukishiro",Name:"Tokinari Tsukishiro",Organization:"external",Goal:"information",Budget:10,Influence:5},
  {ID:"jinnosuke-naoe",Name:"Jinnosuke Naoe",Organization:"org.naoe_faction",Goal:"coalition",Budget:12,Influence:7},
  {ID:"kijima",Name:"Kijima",Organization:"org.kijima_faction",Goal:"oversight",Budget:12,Influence:6},
  {ID:"isomaru",Name:"Yōkō Isomaru",Organization:"org.isomaru_faction",Goal:"coalition",Budget:10,Influence:6},
  {ID:"miyako",Name:"Miyako",Organization:"org.miyako_faction",Goal:"coalition",Budget:10,Influence:6},
  {ID:"sakayanagi-father",Name:"Sakayanagi",Organization:"org.anhs",Goal:"oversight",Budget:10,Influence:6},
  {ID:"tomohiro-kanzaki",Name:"Tomohiro Kanzaki",Organization:"org.kanzaki_engineering",Goal:"funding",Budget:10,Influence:4},
  {ID:"goro-ishigami",Name:"Gorō Ishigami",Organization:"org.ishigami_group",Goal:"funding",Budget:10,Influence:4},
 }
 out:=&InstitutionalState{Actors:map[string]InstitutionalActor{},Requests:[]InstitutionalRequest{},Events:[]InstitutionalEvent{}}
 for _,a:=range actors{out.Actors[a.ID]=a};return out
}
func institutionTargets(a InstitutionalActor)[]string{
 switch a.Goal{
 case "funding":return []string{"jinnosuke-naoe","tomohiro-kanzaki","goro-ishigami"}
 case "information":return []string{"atsuomi-ayanokoji","sakayanagi-father"}
 case "coalition":return []string{"jinnosuke-naoe","isomaru","miyako","kijima"}
 case "oversight":return []string{"sakayanagi-father","kijima"}
 };return nil
}
func institutionCompatible(a InstitutionalActor,kind string)bool{
 switch kind{
 case "funding":return a.Goal=="funding"||a.Goal=="coalition"
 case "information":return a.Goal=="information"||a.Goal=="funding" // public/own institutional summary only
 case "coalition":return a.Goal=="coalition"
 case "oversight":return a.Goal=="oversight"
 };return false
}
func institutionalEvent(x *InstitutionalState,day int64,actor,target,action,request string){
 audience:=[]string{actor};if target!=""&&target!=actor{audience=append(audience,target)};sort.Strings(audience)
 x.Events=append(x.Events,InstitutionalEvent{day,actor,target,action,request,audience})
}
// An actor reads only addressed requests, its own budget and own policy.
// No input includes ANHS students, VIP roles, private school information
// or another external actor's goals/budget. Offers cannot confer authority.
func advanceInstitutionalDay(seed string,x *InstitutionalState,day int64){
 if x==nil||day<=x.LastDay{return}
 x.LastDay=day
 ids:=[]string{};for id:=range x.Actors{ids=append(ids,id)};sort.Strings(ids)
 for _,id:=range ids{
  a:=x.Actors[id]
  if day%30==1&&a.Budget<12{a.Budget++}
  for i:=range x.Requests{
   q:=&x.Requests[i];if q.Status!="pending"||q.To!=id||q.Day>=day{continue}
   if day-q.Day>7{q.Status="expired";institutionalEvent(x,day,id,q.From,"expired",q.ID);continue}
   if a.LastActionDay==day{break}
   q.Status="declined";if a.Budget>0&&institutionCompatible(a,q.Kind){q.Status="accepted";a.Budget--;if a.Influence<20{a.Influence++}}
   a.LastActionDay=day;institutionalEvent(x,day,id,q.From,q.Status,q.ID);break
  }
  if a.LastActionDay!=day&&a.Budget>0&&day%7==int64(len(id)%7){
   targets:=[]string{};for _,peer:=range institutionTargets(a){if peer!=id{if _,ok:=x.Actors[peer];ok{targets=append(targets,peer)}}}
   sort.Slice(targets,func(i,k int)bool{return examKey(Journal{WorldSeed:seed},fmt.Sprintf("institution/%d/%s/%s",day,id,targets[i]))<examKey(Journal{WorldSeed:seed},fmt.Sprintf("institution/%d/%s/%s",day,id,targets[k]))})
   for _,peer:=range targets{
    pending:=false;for _,q:=range x.Requests{if q.From==id&&q.To==peer&&q.Status=="pending"{pending=true;break}};if pending{continue}
    q:=InstitutionalRequest{fmt.Sprintf("inst-%d",len(x.Requests)+1),id,peer,a.Goal,day,"pending"}
    x.Requests=append(x.Requests,q);a.Budget--;a.LastActionDay=day;institutionalEvent(x,day,id,peer,"proposed",q.ID);break
   }
  }
  x.Actors[id]=a
 }
}
func advanceInstitutions(j Journal,s *State){
 if s.World==nil||s.World.Institutions==nil{return}
 advanceInstitutionalDay(j.WorldSeed,s.World.Institutions,s.Minute/1440+1)
}
func institutionalPerspective(x *InstitutionalState,id string)(InstitutionalActor,[]InstitutionalRequest,[]InstitutionalEvent,bool){
 if x==nil{return InstitutionalActor{},nil,nil,false};actor,ok:=x.Actors[id];if !ok{return actor,nil,nil,false}
 requests:=[]InstitutionalRequest{};events:=[]InstitutionalEvent{}
 for _,q:=range x.Requests{if q.From==id||q.To==id{requests=append(requests,q)}}
 for _,e:=range x.Events{if has(e.Audience,id){events=append(events,clone(e))}}
 return actor,requests,events,true
}

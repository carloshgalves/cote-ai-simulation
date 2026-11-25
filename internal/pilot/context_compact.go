package pilot

import "encoding/json"

// Qualitative self-perception only, not an exact capacity or world meter.
type PilotInteroception struct{
 Energy string `json:"energy"`
 Hydration string `json:"hydration"`
 Fatigue string `json:"fatigue"`
}
func qualitativeBody(b *PilotBody)*PilotInteroception{
 if b==nil{return nil}
 need:=func(v int)string{if v<250{return "necessidade intensa"};if v<550{return "necessidade perceptível"};return "suficiente"}
 fatigue:="descansado";if b.Fatigue>850{fatigue="exausto"}else if b.Fatigue>500{fatigue="cansado"}else if b.Fatigue>250{fatigue="fadiga leve"}
 return &PilotInteroception{need(b.Energy),need(b.Hydration),fatigue}
}

// Keep the validated Context unchanged. Only the provider-facing encoding
// is deduplicated; all legal option IDs and evidence texts remain available.
func modelContextPayload(view Context)([]byte,error){
 if view.Version<CompactContextVersion{return json.Marshal(struct{Context;Options []offeredAction `json:"options"`}{view,offeredActions(view)})}
 raw,err:=json.Marshal(view);if err!=nil{return nil,err}
 var payload map[string]any;if err=json.Unmarshal(raw,&payload);err!=nil{return nil,err}
 if self,ok:=payload["self"].(map[string]any);ok&&view.Identity!=nil{delete(self,"traits");delete(self,"goals")}
 if school,ok:=payload["school"].(map[string]any);ok{delete(school,"options")}
 delete(payload,"population")
 delete(payload,"body")
 type directoryEntry struct{Person;Role string `json:"role,omitempty"`;Year int `json:"year,omitempty"`}
 people:=[]directoryEntry{}
 for _,p:=range append([]Person{{view.Self.ID,view.Self.Name,view.Self.Class}},view.People...){
  m:=view.Population[p.ID];people=append(people,directoryEntry{p,m.Role,m.Year})
 }
 payload["people"]=people
 options:=[]string{};for _,o:=range offeredActions(view){options=append(options,o.ID)}
 payload["options"]=options
 return json.Marshal(payload)
}

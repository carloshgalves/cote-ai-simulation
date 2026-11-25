package pilot

// A single physical observation with an audience ACL. Each recipient
// receives the same fact as before, without exposing the other observers
// through its projected memory (see actorContext).
func publishMovementObservation(j Journal,out *State,scene State,actor,loc string,arrival bool){
 audience:=[]string{}
 for _,id:=range ids(j.Profiles){
  if id==actor{continue}
  m:=scene.Minds[id]
  if m.Location==loc&&(m.Activity==nil||m.Activity.Kind!="move"){audience=append(audience,id)}
 }
 if len(audience)==0{return}
 text,reason:="Saiu de "+loc,"Um colega saiu do ambiente"
 if arrival{text,reason="Chegou a "+loc,"Um colega chegou ao ambiente"}
 appendEvent(out,Event{Kind:"observation",Actor:actor,Text:text,Audience:audience})
 for _,id:=range audience{wakeOnMovement(j,out,id,actor,reason)}
}

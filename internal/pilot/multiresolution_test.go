package pilot
import("context";"reflect";"testing")
func TestRoutineMacroMatchesDetailedReduction(t *testing.T){
 j:=autonomyJournal(t);j.Version=MultiresolutionVersion;initializeIdentities(&j);s,err:=initial(j);if err!=nil{t.Fatal(err)}
 // An isolated deterministic fixture; exact same frozen starting state
 // goes into macro and sequential execution.
 for _,id:=range ids(j.Profiles){m:=s.Minds[id];m.Tier=LatentTier;m.PendingStimulus="";m.Conversation=nil;s.Minds[id]=m}
 next,rs,_,err:=routineMacro(context.Background(),j,s,s.Minute+1440,8);if err!=nil{t.Fatal(err)}
 if len(rs)==0{t.Fatal("no deterministic rounds")}
 fine:=s
 for _,r:=range rs{fine,err=apply(j,fine,r);if err!=nil{t.Fatal(err)}}
 if !reflect.DeepEqual(next,fine){t.Fatal("macro changed detailed history")}
}
func TestRoutineMacroStopsForCognition(t *testing.T){
 j:=autonomyJournal(t);j.Version=MultiresolutionVersion;initializeIdentities(&j);s,err:=initial(j);if err!=nil{t.Fatal(err)}
 m:=s.Minds["horikita"];m.Activity=nil;m.Tier=FullTier;m.PendingStimulus="Recebeu uma fala";s.Minds["horikita"]=m
 next,rs,_,err:=routineMacro(context.Background(),j,s,s.Minute+1440,8);if err!=nil||len(rs)!=0||!reflect.DeepEqual(next,s){t.Fatal("addressed cognition skipped",err)}
}
func TestQuietRestBoundaries(t *testing.T){
 j:=autonomyJournal(t);j.Version=MultiresolutionVersion;initializeIdentities(&j);s,err:=initial(j);if err!=nil{t.Fatal(err)}
 s.Minute=1080;id:="horikita";m:=s.Minds[id];m.Location="dormitory-"+id;m.Body=&PilotBody{Energy:900,Hydration:900,Fatigue:300};m.PendingStimulus="";m.NextDeliberation=0;m.Plan=nil;m.Plans=nil;m.Conversation=nil;s.Minds[id]=m
 if n:=quietRestDuration(j,s,id);n!=120{t.Fatal("quiet rest not consolidated",n)}
 m.PendingStimulus="Recebeu convite";s.Minds[id]=m;if quietRestDuration(j,s,id)!=60{t.Fatal("stimulus ignored")}
 m.PendingStimulus="";m.Body.Hydration=400;s.Minds[id]=m;if quietRestDuration(j,s,id)!=60{t.Fatal("needs boundary ignored")}
}

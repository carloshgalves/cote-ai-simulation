package pilot
import(
 "context"
 "reflect"
 "testing"
)
func TestHierarchicalPeriodAndStimulus(t *testing.T){
 j:=autonomyJournal(t);j.Version=HierarchicalVersion;initializeIdentities(&j);s,err:=initial(j);if err!=nil{t.Fatal(err)}
 s.Minute=540;id:="horikita";m:=s.Minds[id];m.Activity=nil;m.Location="class-d";m.Body=&PilotBody{Energy:950,Hydration:950,Fatigue:100};m.PendingStimulus="";m.Conversation=nil;m.Plan=nil;m.Plans=nil;m.NextDeliberation=0;s.Minds[id]=m
 if n:=hierarchicalDuration(j,s,id,"lesson",50);n!=180{t.Fatal("period not coarsened",n)}
 m.Activity=&Activity{Kind:"task",Target:"lesson:",Started:540,Until:720,Coarse:true,SpanMinutes:180};s.Minds[id]=m;s.Minute=575
 before:=len(s.Events);stimulate(&s,id,"Recebeu convite")
 if s.Minds[id].Activity!=nil||!decisionNeedsModel(j,s,id)||len(s.Events)!=before+1{t.Fatal("stimulus failed to interrupt")}
 m=s.Minds[id];m.PendingStimulus="";m.Body.Hydration=390;s.Minds[id]=m
 if n:=hierarchicalDuration(j,s,id,"lesson",50);n!=50{t.Fatal("needs bound ignored",n)}
}
func TestHierarchicalOrdinaryVersusStrategic(t *testing.T){
 j:=autonomyJournal(t);j.Version=HierarchicalVersion;initializeIdentities(&j);s,err:=initial(j);if err!=nil{t.Fatal(err)}
 s.Minute=900;id:="horikita";m:=s.Minds[id];m.Activity=nil;m.PendingStimulus="";m.Conversation=nil;m.Plan=nil;m.Plans=nil;m.NextDeliberation=0;m.OpportunityDay=0;m.OpportunityCount=0;m.LastOpportunityMinute=0;m.Body=&PilotBody{Energy:900,Hydration:900,Fatigue:100}
 m.Goals=[]string{"Construir relações sociais"};s.Minds[id]=m
 if decisionNeedsModel(j,s,id){t.Fatal("ordinary initiative polled")}
 m.Goals=[]string{"Estabelecer influência entre colegas"};s.Minds[id]=m
 if !decisionNeedsModel(j,s,id){t.Fatal("strategic initiative suppressed")}
 m.OpportunityDay=1;m.OpportunityCount=1;s.Minds[id]=m
 if decisionNeedsModel(j,s,id){t.Fatal("discretionary budget ignored")}
 stimulate(&s,id,"Recebeu uma fala");if !decisionNeedsModel(j,s,id){t.Fatal("essential stimulus suppressed")}
}
func TestHierarchicalObservedLeisure(t *testing.T){
 j:=autonomyJournal(t);j.Version=HierarchicalVersion;initializeIdentities(&j);s,err:=initial(j);if err!=nil{t.Fatal(err)}
 s.Minute=930
 for i,id:=range []string{"kushida","horikita"}{m:=s.Minds[id];m.Location="courtyard";m.Activity=&Activity{Kind:"task",Target:"socialize:",Started:900+int64(i)*15,Until:960,Coarse:true};s.Minds[id]=m}
 before:=len(s.Events);recordOverlappingLeisure(j,&s)
 a,b:=s.Minds["kushida"].Social["horikita"],s.Minds["horikita"].Social["kushida"]
 if len(s.Events)!=before+1||a.SharedLeisureDays!=1||b.SharedLeisureDays!=1||a.Trust!=0||b.Trust!=0||a.Interactions!=0{t.Fatal("invented or missing social fact")}
 recordOverlappingLeisure(j,&s);if len(s.Events)!=before+1{t.Fatal("duplicate encounter")}
}
func TestHierarchicalMacroEquivalent(t *testing.T){
 j:=autonomyJournal(t);j.Version=HierarchicalVersion;initializeIdentities(&j);s,err:=initial(j);if err!=nil{t.Fatal(err)}
 for _,id:=range ids(j.Profiles){m:=s.Minds[id];m.Tier=LatentTier;m.PendingStimulus="";m.Conversation=nil;s.Minds[id]=m}
 next,rs,_,err:=routineMacro(context.Background(),j,s,540,16);if err!=nil||len(rs)==0{t.Fatal("macro failed",err)}
 fine:=s;for _,r:=range rs{fine,err=apply(j,fine,r);if err!=nil{t.Fatal(err)}}
 if !reflect.DeepEqual(next,fine){t.Fatal("macro changed replay")}
}

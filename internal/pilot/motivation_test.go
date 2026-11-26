package pilot
import "testing"
func TestPersistentMotivationOpportunity(t *testing.T){
 j:=autonomyJournal(t);j.Version=MotivationVersion;s,err:=initial(j);if err!=nil{t.Fatal(err)}
 id:="ayanokoji";m:=s.Minds[id];m.Activity=nil;m.PendingStimulus="";m.AwaitEvent=true;s.Minds[id]=m;s.Minute=970
 op:=personalOpportunity(j,s,id);if op==nil||!decisionNeedsModel(j,s,id){t.Fatal("ordinary goal did not produce opportunity")}
 m.LastOpportunityMinute=s.Minute;s.Minds[id]=m
 if personalOpportunity(j,s,id)!=nil{t.Fatal("cooldown ignored")}
 s.Minute=1090;if personalOpportunity(j,s,id)!=nil{t.Fatal("opportunity outside free period")}
 s.Minute=2500;m.LastOpportunityMinute=0;m.Body.Hydration=0;s.Minds[id]=m
 if personalOpportunity(j,s,id)!=nil{t.Fatal("urgent need ignored")}
}

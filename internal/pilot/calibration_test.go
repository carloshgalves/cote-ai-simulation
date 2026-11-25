package pilot
import "testing"
func TestCalibrationBudgetNeverSuppressesAddressedStimulus(t *testing.T){
 j:=autonomyJournal(t);j.Version=CalibratedVersion;initializeIdentities(&j);s,err:=initial(j);if err!=nil{t.Fatal(err)}
 s.Minute=970;id:="ayanokoji";m:=s.Minds[id];m.Activity=nil;m.PendingStimulus="";m.NextDeliberation=0;m.Conversation=nil;m.OpportunityDay=1;m.OpportunityCount=3;s.Minds[id]=m
 if decisionNeedsModel(j,s,id){t.Fatal("discretionary budget ignored")}
 m.PendingStimulus="Recebeu convite";s.Minds[id]=m
 if !decisionNeedsModel(j,s,id){t.Fatal("addressed stimulus suppressed")}
 m.PendingStimulus="";m.OpportunityCount=0;s.Minds[id]=m
 if !decisionNeedsModel(j,s,id){t.Fatal("ordinary motivation lost")}
}

package pilot
import("testing")
func TestAnnualGateRejectsIncompleteRun(t *testing.T){
 j:=autonomyJournal(t);j.Version=FidelityVersion;initializeIdentities(&j);s,err:=initial(j);if err!=nil{t.Fatal(err)}
 a,err:=auditYear(j,s);if err!=nil{t.Fatal(err)}
 if !a.ReplayEqual||a.YearComplete||a.StructuralGate||a.TransitionAllowed{t.Fatal("premature annual acceptance")}
}

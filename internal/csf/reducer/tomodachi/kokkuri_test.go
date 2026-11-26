package tomodachi
import("testing";"reflect";"strings";"encoding/json")
func votes(s State,choice string)[]Submission{out:=[]Submission{};for _,a:=range s.Actors{out=append(out,Submission{a.ID,Decision{Choice:choice}})};return out}
func apply(t *testing.T,s State,a []Submission)State{t.Helper();n,e:=Apply(s,s.Revision,a);if e!=nil{t.Fatal(e)};return n}
func TestRoundOrderAndIsolation(t *testing.T){
 s:=New();a:=votes(s,"pass");a[2].Decision=Decision{"talk","I claim I will cooperate."}
 n:=apply(t,s,a);if n.Actors[2].Debt!=2*InitialDebt||s.Actors[2].Debt!=InitialDebt{t.Fatal("penalty or mutation")}
 for i,j:=0,len(a)-1;i<j;i,j=i+1,j-1{a[i],a[j]=a[j],a[i]}
 if !reflect.DeepEqual(n,apply(t,s,a)){t.Fatal("submission order affected state")}
 v,e:=Project(n,n.Actors[1].ID);if e!=nil||v.PrivateCard!=""||v.OwnDebt!=InitialDebt{t.Fatal("private card/debt leak")}
 b,_:=json.Marshal(v);if strings.Contains(string(b),"8000000"){t.Fatal("other debt leak")}
 q,_:=Project(n,n.Actors[0].ID);if q.PrivateCard==""{t.Fatal("missing own card")}
 ballot:=votes(n,"yes");ballot[0].Decision.Choice="no";r:=apply(t,n,ballot)
 if r.Round!=2||r.Actors[0].Debt!=2000000||r.Results[0]!="no"{t.Fatal("minority or debt incentive")}
 r=apply(t,r,votes(r,"pass"));r=apply(t,r,votes(r,"yes"));if r.Phase!=Terminal||len(r.Winners)!=5||r.Actors[0].Debt!=1600000{t.Fatal("incorrect clear")}
 if _,e:=Apply(r,r.Revision,nil);e==nil{t.Fatal("terminal accepted")}
}
func TestFailureAndInvalidCohorts(t *testing.T){
 s:=New();if _,e:=Apply(s,1,votes(s,"pass"));e==nil{t.Fatal("stale accepted")}
 a:=votes(s,"pass");a[1].Actor=a[0].Actor;if _,e:=Apply(s,0,a);e==nil{t.Fatal("duplicate accepted")}
 a=votes(s,"pass");a[0].Decision=Decision{"yes",""};if _,e:=Apply(s,0,a);e==nil{t.Fatal("phase violation")}
 for i:=0;i<5;i++{s=apply(t,s,votes(s,"pass"));s=apply(t,s,votes(s,"no"))}
 if s.Phase!=Terminal||len(s.Winners)!=0||s.Actors[4].Debt!=12000000{t.Fatalf("failed-game debt: %+v",s.Actors)}
}
func TestCanonicalCandidate(t *testing.T){s:=New();a,h,e:=CandidateBytes(s);if e!=nil||len(a)==0||h==([32]byte{}){t.Fatal(e)};_,h2,e:=CandidateBytes(s);if e!=nil||h!=h2{t.Fatal("noncanonical")}}

package pilot

import "testing"

// Empty maps tagged omitempty disappear in the JSON clone used by the
// scheduled reducer. Exercise that exact boundary before the first vote.
func TestFirstBallotAfterStateClone(t *testing.T) {
 for _,kind:=range []string{"minority","janken","kokkuri","poll","island","trial","hide"}{
  t.Run(kind,func(t *testing.T){
   j:=Journal{Version:HierarchicalVersion}
   group:=[]string{"a","b","c","d"}
   before:=State{World:&SchoolWorld{Exam:&VIPExam{Kind:kind,Phase:"early",Group:group,Submitted:map[string]bool{},Ballots:map[string]string{}}}}
   out:=clone(before)
   if out.World.Exam.Ballots!=nil{t.Fatal("fixture must exercise omitted empty ballot map")}
   target:="a"
   if kind!="minority"{_,opts:=portfolioOffers(j,before,"a",before.World.Exam);if len(opts)==0{t.Fatal("no legal option")};target=opts[0]}
   choice:=Chosen{Actor:"a",Decision:Decision{Action:"vote",Target:target}}
   if kind=="minority"{recordGeneratedVotes(&out,[]Chosen{choice})}else{recordPortfolioVotes(j,before,&out,[]Chosen{choice})}
   if out.World.Exam.Ballots["a"]!=target||!out.World.Exam.Submitted["a"]||len(out.Events)!=1{t.Fatal("valid ballot not recorded")}
   if len(before.World.Exam.Ballots)!=0||before.World.Exam.Submitted["a"]{t.Fatal("frozen input mutated")}
   next:=clone(out)
   if kind=="minority"{recordGeneratedVotes(&next,[]Chosen{choice})}else{recordPortfolioVotes(j,out,&next,[]Chosen{choice})}
   if len(next.World.Exam.Ballots)!=1||len(next.Events)!=1{t.Fatal("duplicate ballot accepted")}
  })
 }
}

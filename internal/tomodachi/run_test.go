package tomodachi
import("context";"strings";"testing";game "github.com/carloshgalves/cote-ai-simulation/internal/csf/reducer/tomodachi")
func rag(_ context.Context,a,g,q string)(Guidance,error){return Guidance{[]string{"Own lesson"},[]string{"ev.tg.guidance."+strings.TrimPrefix(a,"actor.")+".test"},strings.Repeat("a",64)},nil}
func TestAutonomousCohortReplay(t *testing.T){
 count:=0
 r,e:=Execute(context.Background(),"fake",rag,func(_ context.Context,in Input)(game.Decision,error){
  count++;if len(in.BehavioralGuidance)!=1{t.Fatal("missing guidance")}
  if in.Observation.Actor!="actor.tenji-mikasa"&&strings.Contains(in.Identity,"father's death"){t.Fatal("foreign biography")}
  if in.Observation.Phase==game.Discussion{return game.Decision{Choice:"pass"},nil}
  if in.Observation.Round==1&&in.Observation.Actor=="actor.makoto-shibe"{return game.Decision{Choice:"no"},nil}
  return game.Decision{Choice:"yes"},nil
 })
 if e!=nil||count!=20||len(r.Frames)!=4{t.Fatalf("run: %v %d",e,count)}
 if e=Replay(r);e!=nil{t.Fatal(e)}
 r.Frames[0].After="forged";if Replay(r)==nil{t.Fatal("tamper accepted")}
}
func TestForeignGuidance(t *testing.T){
 _,e:=Execute(context.Background(),"fake",func(_ context.Context,a,g,q string)(Guidance,error){return Guidance{[]string{"foreign"},[]string{"ev.tg.guidance.someone-else.x"},strings.Repeat("a",64)},nil},func(_ context.Context,in Input)(game.Decision,error){t.Fatal("model called");return game.Decision{},nil});if e==nil{t.Fatal("foreign guidance accepted")}
}

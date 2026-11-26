package pilot
import("testing";"path/filepath";"reflect";"context";"time";"fmt")
func TestSegmentedRoundtrip(t *testing.T){
 path:=filepath.Join(t.TempDir(),"test.json")
 model:=modelFunc(func(_ context.Context,_ Context)(Decision,error){return Decision{Action:"wait",WaitMinutes:60},nil})
 s,err:=OpenSegmented(path,"test",model);if err!=nil{t.Fatal(err)}
 for i:=0;i<3;i++{if _,err=s.Step(context.Background(),s.state.Round);err!=nil{t.Fatal(err)}}
 want:=s.Snapshot();s.Close();s,err=OpenSegmented(path,"test",model);if err!=nil{t.Fatal(err)};defer s.Close()
 if !reflect.DeepEqual(want.State,s.Snapshot().State){t.Fatal("recovery changed state")}
}
func TestFirstExamElapsedBenchmark(t *testing.T){
 if testing.Short(){t.Skip("longitudinal benchmark")}
 if !benchmarkEnabled(){t.Skip("set COTE_FIRST_EXAM_BENCH=1")}
 model:=modelFunc(func(_ context.Context,_ Context)(Decision,error){return Decision{Action:"wait",WaitMinutes:60},nil})
 s,err:=OpenSegmented(filepath.Join(t.TempDir(),"bench.json"),"scripted",model);if err!=nil{t.Fatal(err)};defer s.Close()
 start:=time.Now();target:=s.state.World.Calendar[0].Start
 for s.state.Minute<target{if _,err=s.Step(context.Background(),s.state.Round);err!=nil{t.Fatal(err)}}
 t.Log(fmt.Sprintf("scripted elapsed=%s rounds=%d minute=%d",time.Since(start),s.state.Round,s.state.Minute))
}

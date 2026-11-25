package pilot

import (
 "encoding/json"
 "fmt"
 "os"
 "strings"
 "testing"
 "time"
)

type benchmarkRow struct {
 Version int `json:"version"`
 SeedLabel string `json:"seed_label"`
 ElapsedMinutes int64 `json:"elapsed_simulated_minutes"`
 Cycles int `json:"cycles"`
 Routine int `json:"routine_decisions"`
 Deliberative int `json:"deliberative_decisions"`
 Initiative int `json:"endogenous_only_decisions_v8_v9"`
 CompletedTasks int `json:"recorded_tasks"`
 AcademicTasks int `json:"completed_academic_tasks"`
 ActiveActors int `json:"actors_with_recorded_tasks"`
 SharedPairDays int `json:"shared_pair_days"`
 ContextBytes int64 `json:"serialized_context_bytes_not_tokens"`
 RuntimeMS int64 `json:"elapsed_wall_ms"`
}

// Same admission profiles, seed, duration target, and wait-only provider
// substitute. Different logical end instants are reported, not silently
// normalized as if decisions per cycle were comparable.
func TestPilotCostQualityBenchmark(t *testing.T){
 if os.Getenv("COTE_COST_BENCHMARK")!="1"{t.Skip("set COTE_COST_BENCHMARK=1")}
 for _,seedLabel:=range []string{"a","b"}{
  for _,version:=range []int{CognitionVersion,SelectiveVersion,AutonomyVersion}{
   t.Run(fmt.Sprintf("%s-v%d",seedLabel,version),func(t *testing.T){
    j:=autonomyJournal(t);j.Version=version;j.WorldSeed=strings.Repeat(seedLabel,64)
    s,err:=initial(j);if err!=nil{t.Fatal(err)}
    start:=s.Minute;target:=start+600
    m:=newAutonomyMetrics(j,s);row:=benchmarkRow{Version:version,SeedLabel:seedLabel}
    began:=time.Now()
    for s.Minute<target&&s.Round<600{
     choices:=[]Chosen{}
     for _,id:=range eligibleActors(j,s){
      var d Decision
      if decisionNeedsModel(j,s,id){
       c:=actorContext(j,s,id);raw,err:=json.Marshal(c);if err!=nil{t.Fatal(err)}
       row.ContextBytes+=int64(len(raw))
       d=Decision{Action:"wait",Source:"eval/wait-only"}
      }else{d=routineDecision(j,s,id)}
      choices=append(choices,Chosen{Actor:id,Decision:d})
     }
     next,err:=scheduledAdvance(j,s,choices);if err!=nil{t.Fatalf("cycle %d: %v",s.Round,err)}
     if err:=autonomyTransitionError(s,next);err!=nil{t.Fatalf("cycle %d: %v",next.Round,err)}
     m.observe(j,s,next,choices);s=next
    }
    row.ElapsedMinutes=s.Minute-start;row.Cycles=m.Cycles;row.Routine=m.Routine;row.Deliberative=m.Deliberative
    for _,a:=range m.Actors{
     if version>=SelectiveVersion{row.Initiative+=a.Initiatives}
     if len(a.Tasks)>0{row.ActiveActors++}
     for _,n:=range a.Tasks{row.CompletedTasks+=n}
     row.AcademicTasks+=a.Tasks["study:"]+a.Tasks["lesson:"]
     row.SharedPairDays+=a.SharedDays
    }
    row.SharedPairDays/=2;row.RuntimeMS=time.Since(began).Milliseconds()
    raw,err:=json.Marshal(row);if err!=nil{t.Fatal(err)}
    t.Logf("COST_QUALITY_BENCHMARK %s",raw)
    if s.Minute<target{t.Fatal("benchmark horizon incomplete")}
    if row.Deliberative==0||row.Routine==0||row.ActiveActors==0{t.Fatal("degenerate trajectory")}
   })
  }
 }
}

type telemetryAggregate struct {
 Provider string `json:"provider"`
 Model string `json:"model"`
 RecordedAttempts int `json:"recorded_attempts"`
 Accepted int `json:"accepted_attempts"`
 Repairs int `json:"repair_attempts"`
 WithUsage int `json:"attempts_with_usage"`
 Input int64 `json:"reported_input_tokens"`
 Output int64 `json:"reported_output_tokens"`
 LatencyMS int64 `json:"sum_latency_ms"`
}
func aggregateBenchmarkCalls(calls []CallTelemetry)[]telemetryAggregate{
 out:=[]telemetryAggregate{}
 index:=map[[2]string]int{}
 for _,c:=range calls{
  key:=[2]string{c.Provider,c.Model};i,ok:=index[key]
  if !ok{i=len(out);index[key]=i;out=append(out,telemetryAggregate{Provider:c.Provider,Model:c.Model})}
  a:=&out[i];a.RecordedAttempts++;if c.Accepted{a.Accepted++};if c.Repair{a.Repairs++};a.LatencyMS+=c.LatencyMS
  if c.Usage!=nil{a.WithUsage++;a.Input+=c.Usage.Input;a.Output+=c.Usage.Output}
 }
 return out
}
func TestPilotBenchmarkTelemetryMissingUsage(t *testing.T){
 calls:=[]CallTelemetry{{Provider:"x",Model:"a",Accepted:true,Usage:&TokenUsage{Input:10,Output:2}},{Provider:"x",Model:"a",Repair:true},{Provider:"x",Model:"b",Usage:&TokenUsage{Input:3,Output:4}}}
 got:=aggregateBenchmarkCalls(calls)
 if len(got)!=2||got[0].RecordedAttempts!=2||got[0].WithUsage!=1||got[0].Input!=10||got[0].Output!=2||got[0].Accepted!=1||got[0].Repairs!=1||got[1].Input!=3{t.Fatal("telemetry aggregation incorrect")}
}

// Run locally on operador.json extracted from the report ZIP. Avoid logging
// actor IDs, private context, route IDs, credentials, or error messages.
func TestPilotBenchmarkLocalTelemetry(t *testing.T){
 path:=os.Getenv("COTE_BENCHMARK_REPORT");if path==""{t.Skip("set COTE_BENCHMARK_REPORT to local operador.json")}
 info,err:=os.Stat(path);if err!=nil||!info.Mode().IsRegular()||info.Size()>64<<20{t.Fatal("report unavailable or too large")}
 raw,err:=os.ReadFile(path);if err!=nil{t.Fatal("cannot read report")}
 var report struct{Calls []CallTelemetry `json:"calls_current_process"`;Dropped *int `json:"dropped_calls"`}
 if err:=json.Unmarshal(raw,&report);err!=nil||report.Dropped==nil||*report.Dropped<0{t.Fatal("invalid telemetry report")}
 // Model/provider names are not printed: they can be user-defined strings.
 for i,a:=range aggregateBenchmarkCalls(report.Calls){
  a.Provider="";a.Model=""
  out,err:=json.Marshal(a);if err!=nil{t.Fatal(err)}
  t.Logf("TELEMETRY group=%d %s",i+1,out)
 }
 t.Logf("TELEMETRY_SCOPE recorded=%d dropped=%d current_process_only=true includes_route_tests=true",len(report.Calls),*report.Dropped)
}

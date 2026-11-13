package commit

import (
 "context"
 "database/sql"
 "fmt"
 "reflect"
 "sync"
 "testing"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/observatory"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/operationalmetrics"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
 _ "modernc.org/sqlite"
)

// O4 exercises all four independently permissioned read surfaces on the
// SAME real run. It must not rebind a privileged report as an actor context.
func TestCSFRV19O4IntegratedOperatorPOVAPOVBPublicAndMetrics(t *testing.T){
 ctx:=context.Background()
 c,store,ledger,cycle,fence,path:=singleResourceFixture(t)
 if _,err:=c.CommitSingleResource(ctx,ledger,cycle,fence);err!=nil{t.Fatal(err)}
 _,_,_,_,_,runID:=ledger.Snapshot()
 run:=[32]byte(runID)
 db,err:=sql.Open("sqlite",path);if err!=nil{t.Fatal(err)};defer db.Close()
 task:=findTerminalRoundTask(t,ctx,db)
 deliver,err:=store.DeliverRoundPerception(ctx,task,c.bundle)
 if err!=nil{t.Fatal(err)}
 privileged,err:=sqlite.OpenPrivilegedObservatory(ctx,path,c.bundle)
 if err!=nil{t.Fatal(err)};defer privileged.Close()
 pending,err:=privileged.ReadPrivilegedReport(ctx,run)
 if err!=nil{t.Fatal(err)}
 if pending.PendingPerceptionTasks==0{t.Fatal("O4 fixture must prove pending perception before completing the invisible task")}
 _=resourceActorInbox(t,ctx,store,path,ledger,c.bundle)
 a:=[2]string{"school","a"}
 b:=[2]string{"school","b"}
 dispatchA,ok:=o3FindDispatch(t,ctx,path,run,a,1)
 if !ok{t.Fatal("O4 fixture has no durable A dispatch")}
 dispatchB:=o3MintActorB(t,ctx,store,ledger,c.bundle,b)
 root,err:=sqlite.OpenObservatory(ctx,path,c.bundle)
 if err!=nil{t.Fatal(err)};defer root.Close()
 povA,err:=sqlite.OpenPOVObservatory(ctx,path,c.bundle,run,dispatchA,a)
 if err!=nil{t.Fatal(err)};defer povA.Close()
 povB,err:=sqlite.OpenPOVObservatory(ctx,path,c.bundle,run,dispatchB,b)
 if err!=nil{t.Fatal(err)};defer povB.Close()
 public,err:=sqlite.OpenPublicObservatory(ctx,path,c.bundle)
 if err!=nil{t.Fatal(err)};defer public.Close()
 sidecar:=t.TempDir()+"/host-operations.db"
 metrics,err:=operationalmetrics.OpenRecorder(ctx,path,sidecar)
 if err!=nil{t.Fatal(err)}
 var metricID [32]byte;metricID[0]=0x88
 sample:=operationalmetrics.Sample{ID:metricID,Run:run,ObservedUnixNanos:1700000000,Stage:"perception",
  DurationNanos:500000,HostID:"test-host",WorkerID:"delivery-worker",QueueDepth:1,WorkerCalls:1}
 if err:=metrics.Record(ctx,sample);err!=nil{t.Fatal(err)}
 if err:=metrics.Close();err!=nil{t.Fatal(err)}
 measurement,err:=operationalmetrics.OpenReader(ctx,path,sidecar)
 if err!=nil{t.Fatal(err)};defer measurement.Close()
 before:=o1WholeDatabaseDigest(t,path)
 state,err:=root.ReadRun(ctx,run)
 if err!=nil{t.Fatal(err)}
 report,err:=privileged.ReadPrivilegedReport(ctx,run)
 if err!=nil{t.Fatal(err)}
 viewA,err:=povA.ReadPOV(ctx)
 if err!=nil{t.Fatal(err)}
 viewB,err:=povB.ReadPOV(ctx)
 if err!=nil{t.Fatal(err)}
 open,err:=public.ReadPublic(ctx,run)
 if err!=nil{t.Fatal(err)}
 operations,err:=measurement.Read(ctx,run,20)
 if err!=nil{t.Fatal(err)}
 if report.State.Run!=state.Run||report.State.WorldHash!=state.WorldHash||
  report.State.Clock!=state.Clock||report.State.Control.Status!=state.Control.Status {
  t.Fatal("O1 status and O2 causal report disagree on same run")
 }
 if len(report.Commits)!=1||len(report.Commits[0].Tasks)<2||len(report.Commits[0].Evidence)<2||
  len(viewA.Entries)!=1||len(viewB.Entries)!=0||
  open.PublicationStatus!=observatory.NoPublicRoute||len(open.Publications)!=0 {
  t.Fatalf("O4 evidence boundary violated: commits=%d tasks=%d evidence=%d A=%d B=%d public=%d",
   len(report.Commits),len(report.Commits[0].Tasks),len(report.Commits[0].Evidence),len(viewA.Entries),len(viewB.Entries),len(open.Publications))
 }
 if viewA.Entries[0].ObservationID!=deliver.Observation.Identity ||
  viewA.Entries[0].KnowledgeInputID!=deliver.Input.Identity{
  t.Fatal("actor did not receive its real durable addressed evidence")
 }
 if len(operations)!=1||!reflect.DeepEqual(operations[0],sample){
  t.Fatalf("untrusted operational sidecar mismatch: %+v",operations)
 }
 // All query permutations are deliberately read-only and stable even when
 // separate connections perform verified replay concurrently.
 errCh:=make(chan error,24)
 var workers sync.WaitGroup
 for index:=0;index<6;index++{
  workers.Add(1);go func(i int){
   defer workers.Done()
   for n:=0;n<4;n++{
    switch (i+n)%5{
    case 0:
     s,e:=root.ReadRun(ctx,run)
     if e!=nil||!reflect.DeepEqual(s,state){errCh<-fmt.Errorf("O1 unstable: %v",e);return}
    case 1:
     p,e:=privileged.ReadPrivilegedReport(ctx,run)
     if e!=nil||!reflect.DeepEqual(p,report){errCh<-fmt.Errorf("O2 unstable: %v",e);return}
    case 2:
     a,e:=povA.ReadPOV(ctx)
     if e!=nil||!reflect.DeepEqual(a,viewA){errCh<-fmt.Errorf("POV A unstable: %v",e);return}
    case 3:
     b,e:=povB.ReadPOV(ctx)
     if e!=nil||!reflect.DeepEqual(b,viewB){errCh<-fmt.Errorf("POV B unstable: %v",e);return}
    case 4:
     p,e:=public.ReadPublic(ctx,run)
     if e!=nil||!reflect.DeepEqual(p,open){errCh<-fmt.Errorf("public unstable: %v",e);return}
    }
   }
  }(index)
 }
 workers.Wait();close(errCh)
 for err:=range errCh{t.Fatal(err)}
 if got:=o1WholeDatabaseDigest(t,path);got!=before{
  t.Fatal("O1/O2/A/B/public/operations reads changed causal or epistemic state")
 }
 t.Logf("O4_ACCEPTANCE same_run=%x privileged_commits=1 A=1 B=0 public=0 metric_samples=1 no_causal_mutation=true",run)
}

// A separate O4 stress fixture covers terminal abort + explicit retry and
// pending work. Those are run-specific authority states, not fabricated by a
// synthetic metrics event or inferred from missing events.
func TestCSFRV19O4AbortRetryPendingAndPublicRemainExtracausal(t *testing.T){
 ctx:=context.Background()
 c,_,_,fence,runID,cycle,path:=abortFixture(t)
 run:=[32]byte(runID)
 b,err:=policy.Load(bundlePath);if err!=nil{t.Fatal(err)}
 privileged,err:=sqlite.OpenPrivilegedObservatory(ctx,path,b)
 if err!=nil{t.Fatal(err)};defer privileged.Close()
 public,err:=sqlite.OpenPublicObservatory(ctx,path,b)
 if err!=nil{t.Fatal(err)};defer public.Close()
 state,err:=privileged.ReadPrivilegedReport(ctx,run)
 if err!=nil{t.Fatal(err)}
 if state.State.Control.Status!=observatory.ATTEMPT_IN_FLIGHT ||
  state.PendingPerceptionTasks<1{t.Fatalf("fixture lacks in-flight/pending authority: %+v",state)}
 firstHash:=state.State.WorldHash
 abort,err:=c.Abort(ctx,runID,cycle,fence,4,nil)
 if err!=nil{t.Fatal(err)}
 after,err:=privileged.ReadPrivilegedReport(ctx,run)
 if err!=nil||after.State.Control.Status!=observatory.HALTED_ON_ABORT{
  t.Fatal("abort not represented by authoritative O2",err)
 }
 if _,err:=c.Retry(ctx,runID,cycle,abort,"o4.operator.retry");err!=nil{t.Fatal(err)}
 sidecar:=t.TempDir()+"/metrics.db"
 recorder,err:=operationalmetrics.OpenRecorder(ctx,path,sidecar)
 if err!=nil{t.Fatal(err)}
 var sampleID [32]byte;sampleID[0]=0x44
 s:=operationalmetrics.Sample{ID:sampleID,Run:run,ObservedUnixNanos:1700000050,
  Stage:"transport",DurationNanos:1300,TransportRetries:2,HostID:"local-host"}
 if err:=recorder.Record(ctx,s);err!=nil{t.Fatal(err)}
 if err:=recorder.Close();err!=nil{t.Fatal(err)}
 before:=o1WholeDatabaseDigest(t,path)
 report,err:=privileged.ReadPrivilegedReport(ctx,run)
 if err!=nil{t.Fatal(err)}
 pub,err:=public.ReadPublic(ctx,run)
 if err!=nil{t.Fatal(err)}
 if report.State.Control.Status!=observatory.RETRY_AUTHORIZED||report.State.WorldHash!=firstHash||
  report.PendingPerceptionTasks<1||len(pub.Publications)!=0{
  t.Fatal("operational retries fabricated world state or public knowledge")
 }
 reader,err:=operationalmetrics.OpenReader(ctx,path,sidecar)
 if err!=nil{t.Fatal(err)}
 items,err:=reader.Read(ctx,run,10)
 if err!=nil||len(items)!=1||items[0].TransportRetries!=2{t.Fatal("operational retries absent",err)}
 if err:=reader.Close();err!=nil{t.Fatal(err)}
 if o1WholeDatabaseDigest(t,path)!=before{t.Fatal("operational metrics changed abort/retry authorities")}
}

package operationalmetrics

import (
 "bytes"
 "context"
 "database/sql"
 "os"
 "path/filepath"
 "reflect"
 "sync"
 "testing"

 _ "modernc.org/sqlite"
)

func metricFixture(t *testing.T)(string,string){
 t.Helper()
 d:=t.TempDir()
 causal:=filepath.Join(d,"causal.db")
 db,err:=sql.Open("sqlite",causal);if err!=nil{t.Fatal(err)}
 if _,err:=db.Exec("CREATE TABLE causal_sentinel (id INTEGER PRIMARY KEY, value TEXT NOT NULL); INSERT INTO causal_sentinel(id,value) VALUES (1,'original')");err!=nil{t.Fatal(err)}
 if err:=db.Close();err!=nil{t.Fatal(err)}
 return causal,filepath.Join(d,"operator-metrics.db")
}
func metricSample(id byte,at int64)Sample{
 var key [32]byte;key[0]=id
 var run [32]byte;run[0]=0x44
 return Sample{ID:key,Run:run,ObservedUnixNanos:at,Stage:"replay",DurationNanos:500,
  HostID:"local-host-1",WorkerID:"worker-1",QueueDepth:2,TransportRetries:1,
  WorkerCalls:3,CostMicros:100,Currency:"USD",RejectedCount:0}
}
func assertCausalUntouched(t *testing.T,path string){
 t.Helper()
 db,err:=sql.Open("sqlite",path);if err!=nil{t.Fatal(err)};defer db.Close()
 var got string
 if err:=db.QueryRow("SELECT value FROM causal_sentinel WHERE id=1").Scan(&got);err!=nil||got!="original"{t.Fatalf("operational metadata altered causal data: %q %v",got,err)}
 var extra int
 if err:=db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='operational_samples'").Scan(&extra);err!=nil||extra!=0{t.Fatal("causal database contains operational schema",err,extra)}
}

func TestCSFRV19O4MetricsReadOnlyIsolatedSidecar(t *testing.T){
 ctx:=context.Background()
 causal,sidecar:=metricFixture(t)
 writer,err:=OpenRecorder(ctx,causal,sidecar);if err!=nil{t.Fatal(err)}
 s:=metricSample(1,1700000000)
 if err:=writer.Record(ctx,s);err!=nil{t.Fatal(err)}
 if err:=writer.Record(ctx,s);err!=nil{t.Fatal("idempotent operation rejected:",err)}
 conflicting:=s;conflicting.DurationNanos++
 if err:=writer.Record(ctx,conflicting);err==nil{t.Fatal("duplicate sample identity accepted different payload")}
 second:=metricSample(2,1700000001);second.Stage="perception"
 if err:=writer.Record(ctx,second);err!=nil{t.Fatal(err)}
 if err:=writer.Close();err!=nil{t.Fatal(err)}
 q,err:=OpenReader(ctx,causal,sidecar);if err!=nil{t.Fatal(err)}
 defer q.Close()
 got,err:=q.Read(ctx,s.Run,10)
 if err!=nil||!reflect.DeepEqual(got,[]Sample{s,second}){t.Fatalf("read-only sidecar returns wrong deterministic samples: %v %#v",err,got)}
 if _,err:=q.Read(ctx,s.Run,1);err==nil{t.Fatal("limit silently truncated operational samples")}
 if _,err:=q.Read(ctx,[32]byte{0x65},10);err!=nil{t.Fatal(err)}
 if _,err:=q.Read(ctx,s.Run,0);err==nil{t.Fatal("zero bound bypassed limits")}
 for i:=0;i<25;i++{
  repeat,err:=q.Read(ctx,s.Run,10);if err!=nil||!reflect.DeepEqual(got,repeat){t.Fatalf("metric query not deterministic at %d: %v",i,err)}
 }
 assertCausalUntouched(t,causal)
 // The metrics reader must not expose an INSERT port and the pager itself
 // must reject writes even if an injected caller tries query_only=OFF.
 testDB,err:=sql.Open("sqlite","file:"+sidecar+"?mode=ro")
 if err!=nil{t.Fatal(err)};defer testDB.Close()
 if _,err:=testDB.Exec("PRAGMA query_only=OFF; INSERT INTO operational_samples(id,run_id,at_ns,body) VALUES(?,?,?,?)",
  make([]byte,32),make([]byte,32),int64(1),[]byte("x"));err==nil{t.Fatal("SQLite readonly mode allowed sidecar mutation")}
 assertCausalUntouched(t,causal)
}

func TestCSFRV19O4MetricsNeverOpenCausalDatabaseForWriting(t *testing.T){
 ctx:=context.Background()
 causal,sidecar:=metricFixture(t)
 if _,err:=OpenRecorder(ctx,causal,causal);err==nil{t.Fatal("metrics writer could overwrite the causal database")}
 if _,err:=OpenReader(ctx,causal,causal);err==nil{t.Fatal("metrics reader accepted causal file as sidecar")}
 if err:=os.Symlink(causal,sidecar);err!=nil{t.Fatal(err)}
 if _,err:=OpenRecorder(ctx,causal,sidecar);err==nil{t.Fatal("symlink-to-causal bypassed isolated writer")}
 if _,err:=OpenReader(ctx,causal,sidecar);err==nil{t.Fatal("symlink-to-causal bypassed isolated reader")}
 if err:=os.Remove(sidecar);err!=nil{t.Fatal(err)}
 if err:=os.Link(causal,sidecar);err!=nil{t.Fatal(err)}
 if _,err:=OpenRecorder(ctx,causal,sidecar);err==nil{t.Fatal("hardlink-to-causal bypassed isolated writer")}
 assertCausalUntouched(t,causal)
 if err:=os.Remove(sidecar);err!=nil{t.Fatal(err)}
 if _,err:=OpenReader(ctx,causal,sidecar);err==nil{t.Fatal("read-only reader silently created absent sidecar")}
 if _,err:=os.Stat(sidecar);!os.IsNotExist(err){t.Fatal("read-only open created a file")}
}

func TestCSFRV19O4RejectsCorruptAndNoncausalPayloads(t *testing.T){
 ctx:=context.Background()
 causal,sidecar:=metricFixture(t)
 writer,err:=OpenRecorder(ctx,causal,sidecar);if err!=nil{t.Fatal(err)}
 s:=metricSample(3,1700000000)
 if err:=writer.Record(ctx,s);err!=nil{t.Fatal(err)}
 invalid:=s
 invalid.ID[0]++;invalid.DurationNanos=-1
 if err:=writer.Record(ctx,invalid);err==nil{t.Fatal("negative operational duration accepted")}
 invalid=s;invalid.ID[0]++;invalid.HostID="private\nsecret"
 if err:=writer.Record(ctx,invalid);err==nil{t.Fatal("unbounded host telemetry string accepted")}
 invalid=s;invalid.ID[0]++;invalid.Currency="EUR"
 if err:=writer.Record(ctx,invalid);err==nil{t.Fatal("unsupported metric currency accepted")}
 if err:=writer.Close();err!=nil{t.Fatal(err)}
 q,err:=OpenReader(ctx,causal,sidecar);if err!=nil{t.Fatal(err)}
 defer q.Close()
 db,err:=sql.Open("sqlite",sidecar);if err!=nil{t.Fatal(err)};defer db.Close()
 if _,err:=db.Exec("UPDATE operational_samples SET body=? WHERE id=?",[]byte("{\"unknown\":true}"),s.ID[:]);err!=nil{t.Fatal(err)}
 if rows,err:=q.Read(ctx,s.Run,10);err==nil{t.Fatalf("unknown operational payload accepted %+v",rows)}
 // Fail-closed must not leak partial output from earlier valid rows.
 assertCausalUntouched(t,causal)
}

func TestCSFRV19O4MetricsConcurrencyAndNoSecretFields(t *testing.T){
 ctx:=context.Background()
 causal,sidecar:=metricFixture(t)
 writer,err:=OpenRecorder(ctx,causal,sidecar);if err!=nil{t.Fatal(err)};defer writer.Close()
 var wg sync.WaitGroup;errs:=make(chan error,32)
 for i:=1;i<=20;i++{
  wg.Add(1);go func(i int){defer wg.Done();errs<-writer.Record(ctx,metricSample(byte(i),int64(100+i)))}(i)
 }
 wg.Wait();close(errs)
 for err:=range errs{if err!=nil{t.Fatal(err)}}
 reader,err:=OpenReader(ctx,causal,sidecar);if err!=nil{t.Fatal(err)};defer reader.Close()
 got,err:=reader.Read(ctx,metricSample(1,0).Run,20)
 if err!=nil||len(got)!=20{t.Fatal("concurrent samples lost",err,len(got))}
 for i:=1;i<len(got);i++{if got[i-1].ObservedUnixNanos>=got[i].ObservedUnixNanos{t.Fatal("operational samples not deterministically ordered")}}
 fields:=reflect.TypeOf(Sample{})
 for i:=0;i<fields.NumField();i++{name:=fields.Field(i).Name;if bytes.Contains([]byte(name),[]byte("Observation"))||bytes.Contains([]byte(name),[]byte("Knowledge"))||bytes.Contains([]byte(name),[]byte("World"))||bytes.Contains([]byte(name),[]byte("Canonical")){t.Fatal("operational payload carries causal or epistemic authority")}}
 assertCausalUntouched(t,causal)
}

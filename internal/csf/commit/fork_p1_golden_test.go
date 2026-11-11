package commit

import (
 "bytes"
 "compress/gzip"
 "context"
 "database/sql"
 "encoding/base64"
 "encoding/hex"
 "encoding/json"
 "errors"
 "fmt"
 "os"
 "path/filepath"
 "runtime"
 "strings"
 "testing"
 "time"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/inputledger"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence/sqlite"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
 _ "modernc.org/sqlite"
)

// This is a pre-refactor byte anchor. Rows are never decoded/re-encoded.
// Hex is stored inside gzip+base64 to make the checked-in goldens compact.
// Each table name is present even when the result set is empty (no silent
// conflation of "not captured" and "zero rows").
type forkP1Golden map[string]map[string][]string

var forkP1GoldenQueries = []struct{name,query string}{
 {"genesis_records", "SELECT canonical FROM genesis_records ORDER BY run_id"},
 {"input_ledger", "SELECT canonical FROM input_ledger ORDER BY id"},
 {"decision_ledger", "SELECT canonical FROM decision_ledger ORDER BY id"},
 {"event_store", "SELECT canonical FROM event_store ORDER BY id"},
 {"evidence_ledger", "SELECT canonical FROM evidence_ledger ORDER BY id"},
 {"causal_outbox", "SELECT canonical FROM causal_outbox ORDER BY id"},
 {"snapshots", "SELECT canonical FROM snapshots ORDER BY id"},
 {"fork_lineage.reference_canonical", "SELECT reference_canonical FROM fork_lineage ORDER BY child_run"},
 {"fork_lineage.parent_genesis", "SELECT parent_genesis_canonical FROM fork_lineage ORDER BY child_run"},
 {"fork_lineage.parent_snapshot", "SELECT parent_snapshot_canonical FROM fork_lineage ORDER BY child_run"},
 {"fork_history_records", "SELECT canonical FROM fork_history_records ORDER BY ledger_name,ordinal"},
 {"fork_history_roots", "SELECT canonical FROM fork_history_roots ORDER BY reference_kind,record_id"},
 {"fork_occurrence_imports", "SELECT canonical FROM fork_occurrence_imports ORDER BY parent_occurrence_id"},
 {"input_slot_responses", "SELECT canonical FROM input_slot_responses ORDER BY slot_key"},
 {"communication_claims", "SELECT canonical FROM communication_claims ORDER BY id"},
 {"communication_transmissions", "SELECT canonical FROM communication_transmissions ORDER BY id"},
 {"fork_communication_imports", "SELECT canonical FROM fork_communication_imports ORDER BY child_transmission_id"},
 {"fork_trigger_imports", "SELECT canonical FROM fork_trigger_imports ORDER BY child_run"},
 {"trigger_definitions", "SELECT canonical FROM trigger_definitions ORDER BY definition_id"},
 {"trigger_runtime", "SELECT canonical FROM trigger_runtime ORDER BY definition_id"},
 {"trigger_activations", "SELECT canonical FROM trigger_activations ORDER BY id"},
 {"scheduled_occurrences", "SELECT canonical FROM scheduled_occurrences ORDER BY id"},
 {"cognitive_owner_receipts.content", "SELECT content FROM cognitive_owner_receipts ORDER BY receipt_id"},
 {"fork_cognitive_owner_proofs.signature", "SELECT signature FROM fork_cognitive_owner_proofs ORDER BY actor_namespace,actor_id"},
}

func forkP1CaptureStore(t *testing.T,path string) map[string][]string {
 t.Helper()
 ctx:=context.Background()
 db,err:=sql.Open("sqlite",path);if err!=nil{t.Fatal(err)}
 defer db.Close()
 data:=make(map[string][]string,len(forkP1GoldenQueries))
 for _,table:=range forkP1GoldenQueries {
  rows,err:=db.QueryContext(ctx,table.query);if err!=nil{t.Fatalf("%s: %v",table.name,err)}
  records:=[]string{}
  for rows.Next(){
   var raw []byte
   if err:=rows.Scan(&raw);err!=nil{rows.Close();t.Fatal(err)}
   records=append(records,hex.EncodeToString(raw))
  }
  if err:=rows.Err();err!=nil{rows.Close();t.Fatal(err)}
  if err:=rows.Close();err!=nil{t.Fatal(err)}
  data[table.name]=records
 }
 return data
}

func forkP1Archive(t *testing.T,sourceRun domain.ID,source *sqlite.Store,snap sqlite.SnapshotV1,
 v1 *policy.Bundle,childID [32]byte,childPath string,crossPolicy bool)(map[string][]string,*sqlite.Store){
 t.Helper()
 ctx:=context.Background()
 child,err:=sqlite.Open(ctx,childPath);if err!=nil{t.Fatal(err)}
 t.Cleanup(func(){child.Close()})
 if crossPolicy{
  v2,err:=policy.LoadV2("../../../docs/architecture/canonical-codec-v2-bundle");if err!=nil{t.Fatal(err)}
  if _,err:=source.ForkIdleSnapshotV1ToV2(ctx,[32]byte(sourceRun),snap.ID,childID,child,v1,v2);err!=nil{t.Fatal("P1 baseline V2 fork:",err)}
  if _,err:=child.ReplayClock(ctx,childID,v2);err!=nil{t.Fatal("P1 baseline replay:",err)}
 }else{
  if _,err:=source.ForkIdleSnapshotV1(ctx,[32]byte(sourceRun),snap.ID,childID,child,v1);err!=nil{t.Fatal("P1 baseline V1 fork:",err)}
  if _,err:=child.ReplayClock(ctx,childID,v1);err!=nil{t.Fatal("P1 baseline same-policy replay:",err)}
 }
 return forkP1CaptureStore(t,childPath),child
}

func forkP1GoldenCompressed(t *testing.T,g forkP1Golden) string{
 t.Helper()
 raw,err:=json.Marshal(g);if err!=nil{t.Fatal(err)}
 var buf bytes.Buffer
 writer:=gzip.NewWriter(&buf)
 if _,err:=writer.Write(raw);err!=nil{t.Fatal(err)}
 if err:=writer.Close();err!=nil{t.Fatal(err)}
 return base64.StdEncoding.EncodeToString(buf.Bytes())
}

func TestCSFRV18ForkP1GoldenBytes(t *testing.T){
 ctx:=context.Background()
 gold:=forkP1Golden{}

 t.Run("v1-same-policy",func(t *testing.T){
  b,err:=policy.Load(bundlePath);if err!=nil{t.Fatal(err)}
  g:=validGenesis(t,b)
  sourcePath:=filepath.Join(t.TempDir(),"parent.db")
  src,err:=sqlite.Open(ctx,sourcePath);if err!=nil{t.Fatal(err)};defer src.Close()
  if err:=src.PersistGenesis(ctx,g,b);err!=nil{t.Fatal(err)}
  if err:=src.BootstrapClock(ctx,g,b);err!=nil{t.Fatal(err)}
  if err:=src.BootstrapGenesisTriggers(ctx,g,b,nil);err!=nil{t.Fatal(err)}
  snap,err:=src.CreateSnapshotV1(ctx,[32]byte(g.RunID),b,nil);if err!=nil{t.Fatal(err)}
  id:=[32]byte(g.RunID);id[0]^=0x40
  childPath:=filepath.Join(t.TempDir(),"child.db")
  childBytes,_:=forkP1Archive(t,g.RunID,src,snap,b,id,childPath,false)
  gold["v1-same-policy:parent"]=forkP1CaptureStore(t,sourcePath)
  gold["v1-same-policy:child"]=childBytes
 })

 t.Run("v2-round-slots",func(t *testing.T){
  src,v1,run,snap:=r19Source(t)
  defer src.Close()
  id:=[32]byte(run);id[0]^=0x69
  childPath:=filepath.Join(t.TempDir(),"child.db")
  childBytes,_:=forkP1Archive(t,run,src,snap,v1,id,childPath,true)
  gold["v2-round-slots:source"]=map[string][]string{
   "parent_genesis":{hex.EncodeToString(func()[]byte{b,e:=src.LoadGenesis(ctx,[32]byte(run));if e!=nil{t.Fatal(e)};return b}())},
   "parent_snapshot":{hex.EncodeToString(snap.Canonical)},
  }
  gold["v2-round-slots:child"]=childBytes
 })

 t.Run("v2-evolved-triggers",func(t *testing.T){
  _,src,_,run,v1,snap,sourcePath:=r21EvolvedTriggerSource(t)
  id:=[32]byte(run);id[0]^=0x73
  childPath:=filepath.Join(t.TempDir(),"child.db")
  // Three samples permit subsequent before/after time and allocation checks.
  for sample:=0;sample<3;sample++{
   p:=childPath
   if sample!=0{p=filepath.Join(t.TempDir(),fmt.Sprintf("child-%d.db",sample))}
   runtime.GC()
   var m1,m2 runtime.MemStats
   runtime.ReadMemStats(&m1)
   start:=time.Now()
   childBytes,_:=forkP1Archive(t,run,src,snap,v1,id,p,true)
   duration:=time.Since(start)
   runtime.ReadMemStats(&m2)
   if sample==0{gold["v2-evolved-triggers:child"]=childBytes
   }else{
    compare:=gold["v2-evolved-triggers:child"]
    for k,values:=range compare{
     if !equalGoldenRecords(values,childBytes[k]){t.Fatalf("sample %d diverged table %s",sample,k)}
    }
   }
   t.Logf("FORK_P1_METRIC sample=%d elapsed_ms=%d allocated_bytes=%d go=%s gomaxprocs=%d",
    sample,duration.Milliseconds(),m2.TotalAlloc-m1.TotalAlloc,runtime.Version(),runtime.GOMAXPROCS(0))
  }
  gold["v2-evolved-triggers:parent"]=forkP1CaptureStore(t,sourcePath)
 })

 t.Run("v2-in-transit-claims",func(t *testing.T){
  c,src,ledger,cycle,fence,sourcePath:=communicationFixture(t)
  _,_,_,_,_,run:=ledger.Snapshot()
  if _,err:=c.CommitCommunicationSend(ctx,ledger,cycle,fence);err!=nil{t.Fatal(err)}
  csfrv18SettleSendOutbox(t,src,c.bundle,sourcePath)
  state,err:=src.LoadClock(ctx,[32]byte(run));if err!=nil{t.Fatal(err)}
  if _,err:=ledger.CloseSource(ctx,"driver",inputledger.Closure{
   ProducerKey:"v2-fork-final",
   ClosedThrough:domain.EligibilityCoordinate{NotBeforeInstant:state.Instant+1000},
   Final:true,
  });err!=nil{t.Fatal(err)}
  cp:=csfrv18SyntheticActorCheckpoints(t,sourcePath,state.Instant,int64(state.Revision))
  snap,err:=src.CreateSnapshotV1(ctx,[32]byte(run),c.bundle,cp);if err!=nil{t.Fatal(err)}
  id:=[32]byte(run);id[0]^=0x94
  childPath:=filepath.Join(t.TempDir(),"child.db")
  childBytes,_:=forkP1Archive(t,run,src,snap,c.bundle,id,childPath,true)
  gold["v2-in-transit-claims:parent"]=forkP1CaptureStore(t,sourcePath)
  gold["v2-in-transit-claims:child"]=childBytes
 })
 got:=forkP1GoldenCompressed(t,gold)
 if os.Getenv("CSFRV18_FORK_GOLDEN_CAPTURE")=="1"{
  t.Log("FORK_P1_GOLDEN_B64="+got)
  return
 }
 raw,err:=os.ReadFile("testdata/csfrv18-fork-p1-golden.gz.b64")
 if errors.Is(err,os.ErrNotExist){t.Skip("P1 baseline capture not pinned yet");return}
 if err!=nil{t.Fatal(err)}
 if strings.TrimSpace(string(raw))!=got{t.Fatal("fork P1 stored canonical bytes differ from pre-refactor golden")}
}

func equalGoldenRecords(a,b []string)bool{
 if len(a)!=len(b){return false}
 for i:=range a{if a[i]!=b[i]{return false}}
 return true
}

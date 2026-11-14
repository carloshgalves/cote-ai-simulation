package embodimentv1

import (
 "bytes"
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "errors"
 "os"
 "path/filepath"
 "strings"
 "testing"
)

func realSource(t *testing.T) Source {
 t.Helper()
 folder:=os.Getenv("PHYSICAL_GO_FIXTURE_DIR")
 if folder==""{t.Skip("cross-language fixture must be supplied by CI")}
 read:=func(name string)[]byte{
  b,err:=os.ReadFile(filepath.Join(folder,name))
  if err!=nil{t.Fatal(err)}
  return b
 }
 s:=Source{Snapshot:read("snapshot.yaml"),Events:read("events.jsonl"),Evidence:read("evidence.json")}
 var pins map[string]string
 if err:=json.Unmarshal(read("pins.json"),&pins);err!=nil{t.Fatal(err)}
 decode:=func(name string)[32]byte{
  var d [32]byte
  b,err:=hex.DecodeString(pins[name]);if err!=nil||len(b)!=32{t.Fatalf("invalid fixture %s",name)}
  copy(d[:],b);return d
 }
 s.PinnedSnapshotSHA256=decode("snapshot_raw_sha256")
 s.PinnedEventsSHA256=decode("events_raw_sha256")
 return s
}

func changedEvidence(t *testing.T,s Source,change func(map[string]any)) Source{
 t.Helper()
 value,err:=parseStrict(s.Evidence)
 if err!=nil{t.Fatal(err)}
 m:=value.(map[string]any)
 change(m)
 s.Evidence,err=pyJSON(m,false)
 if err!=nil{t.Fatal(err)}
 return s
}

func TestActualPythonSourceIsGoVerifiedButNeverCommitted(t *testing.T){
 s:=realSource(t)
 v,err:=InspectUnadmitted(s)
 if err!=nil{t.Fatalf("real PSV1-2/A2 fixture rejected by Go: %v",err)}
 if v.commandCount!=2||v.transitionCount!=6{t.Fatalf("unexpected counts: %+v",v)}
 if v.source[0]!=sha256.Sum256(s.Snapshot)||v.source[1]!=sha256.Sum256(s.Events){
  t.Fatal("Go source fingerprints unbound")
 }
 if !errors.Is(v.AdmitToWorld(),ErrUnadmitted){
  t.Fatal("unregistered physical reducer cannot permit a CSF commit")
 }
}

func TestForgedEvidenceNeverPassesGo(t *testing.T){
 orig:=realSource(t)
 cases:=map[string]func(map[string]any){
  "unattested":func(m map[string]any){m["completion"]="UNATTESTED_PREFIX"},
  "pretend admitted":func(m map[string]any){m["admission"]="ADMITTED"},
  "forged source":func(m map[string]any){m["source"].(map[string]any)["events_raw_sha256"]=strings.Repeat("f",64)},
  "wrong seq":func(m map[string]any){m["transitions"].([]any)[0].(map[string]any)["source_local_seq"]=json.Number("900")},
  "wrong delta":func(m map[string]any){m["transitions"].([]any)[0].(map[string]any)["source_local_delta_hours"]=json.Number("3.0")},
  "swapped actor":func(m map[string]any){m["transitions"].([]any)[0].(map[string]any)["subject_id"]="npc.0002"},
  "wrong diff hash":func(m map[string]any){m["transitions"].([]any)[0].(map[string]any)["channel_diff_sha256"]=strings.Repeat("f",64)},
  "no receipt":func(m map[string]any){m["command_receipts"]=[]any{}},
  "wrong footer":func(m map[string]any){m["command_receipts"].([]any)[0].(map[string]any)["complete_seq"]=json.Number("99")},
  "changed body":func(m map[string]any){m["subject_tips"].([]any)[0].(map[string]any)["verified_body"].(map[string]any)["central_fatigue"]=json.Number("0.999")},
  "unknown body":func(m map[string]any){m["subject_tips"].([]any)[0].(map[string]any)["verified_body"].(map[string]any)["rng_state"]="secret"},
  "agent context":func(m map[string]any){m["KnowledgeInput"]="secret"},
  "wrong phase count":func(m map[string]any){m["source"].(map[string]any)["completed_phase_count"]=json.Number("3")},
  "foreign body":func(m map[string]any){m["subject_tips"].([]any)[0].(map[string]any)["subject_id"]="npc.9999"},
 }
 for label,change:=range cases{
  t.Run(label,func(t *testing.T){
   s:=changedEvidence(t,orig,change)
   if _,err:=InspectUnadmitted(s);err==nil{t.Fatal("tampered source passed Go")}
  })
 }
}
func TestPinsAreExternallyRequiredAndNeverInferred(t *testing.T){
 s:=realSource(t)
 s.PinnedSnapshotSHA256=[32]byte{}
 if _,err:=InspectUnadmitted(s);!errors.Is(err,ErrInvalidPhysical){t.Fatalf("missing A1 pin accepted: %v",err)}
 s=realSource(t);s.Snapshot=append([]byte(nil),s.Snapshot...)
 s.Snapshot[0]^=1
 if _,err:=InspectUnadmitted(s);!errors.Is(err,ErrInvalidPhysical){t.Fatalf("mutated snapshot accepted: %v",err)}
 s=realSource(t);s.Events=bytes.TrimSuffix(s.Events,[]byte{'\n'})
 s.PinnedEventsSHA256=sha256.Sum256(s.Events)
 if _,err:=InspectUnadmitted(s);err==nil{t.Fatal("truncated source accepted")}
}
func TestGoRejectsAnUnfinishedPhysicalCommandEvenWithRepinnedBytes(t *testing.T){
 s:=realSource(t)
 rows:=bytes.Split(s.Events,[]byte{'\n'})
 cut:=-1
 for i,line:=range rows{
  if bytes.Contains(line,[]byte("body.advance.complete.v1")){cut=i;break}
 }
 if cut<0{t.Fatal("fixture missing final receipt")}
 s.Events=append(bytes.Join(rows[:cut],[]byte{'\n'}),'\n')
 s.PinnedEventsSHA256=sha256.Sum256(s.Events)
 if _,err:=InspectUnadmitted(s);err==nil{t.Fatal("source ending before terminal proof passed")}
}
func TestGoRejectsDuplicateFieldsAndMalformedNumbers(t *testing.T){
 s:=realSource(t)
 s.Evidence=bytes.Replace(s.Evidence,[]byte(`"kind":`),[]byte(`"kind":"forged","kind":`),1)
 if _,err:=InspectUnadmitted(s);err==nil{t.Fatal("duplicate evidence key passed")}
 for _,raw:=range [][]byte{[]byte(`{"a":1,"a":2}`),[]byte(`{"x":NaN}`),
  []byte(`{"x":1e400}`),[]byte(`{"x":0} trailing`)}{
  if _,err:=parseStrict(raw);err==nil{t.Errorf("invalid JSON passed: %s",raw)}
 }
}

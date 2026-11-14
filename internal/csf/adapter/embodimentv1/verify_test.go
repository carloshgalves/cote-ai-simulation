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
  "float seq":func(m map[string]any){m["transitions"].([]any)[0].(map[string]any)["source_local_seq"]=json.Number("7.0")},
  "exponent footer seq":func(m map[string]any){m["command_receipts"].([]any)[0].(map[string]any)["complete_seq"]=json.Number("11e0")},
  "float tip count":func(m map[string]any){m["subject_tips"].([]any)[0].(map[string]any)["verified_advance_count"]=json.Number("4.0")},

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


func TestGoPhysicalDomainRejectsSchemaShapedNumericViolations(t *testing.T){
 source:=realSource(t)
 decoded,err:=parseStrict(source.Evidence)
 if err!=nil{t.Fatal(err)}
 obj:=decoded.(map[string]any)
 tip:=obj["subject_tips"].([]any)[0].(map[string]any)
 original:=tip["verified_body"].(map[string]any)
 id:=tip["subject_id"].(string)
 attacks:=[]struct{name string;alter func(map[string]any)}{
  {"central fatigue >1",func(b map[string]any){b["central_fatigue"]=json.Number("1.1")}},
  {"reserve above capacity",func(b map[string]any){
   w:=b["w_prime_balance"].(map[string]any)
   w["remaining_j"]=json.Number("9999999999")
  }},
  {"negative sleep debt",func(b map[string]any){b["sleep"].(map[string]any)["debt_hours"]=json.Number("-1")}},
  {"invalid circadian phase",func(b map[string]any){b["sleep"].(map[string]any)["circadian_phase"]=json.Number("24")}},
  {"negative hydration",func(b map[string]any){
   b["hydration"].(map[string]any)["deficit_pct_body_mass"]=json.Number("-1")
  }},
  {"negative energy substrate",func(b map[string]any){
   b["energy"].(map[string]any)["substrate_availability"]=json.Number("-0.1")
  }},
  {"negative soreness",func(b map[string]any){
   b["soreness"].(map[string]any)["by_region"].(map[string]any)["legs"].(map[string]any)["expressed"]=json.Number("-1")
  }},
  {"negative core thermal offset",func(b map[string]any){
   b["thermal"].(map[string]any)["core_offset_c"]=json.Number("-0.1")
  }},
  {"unbounded body time",func(b map[string]any){b["t_hours"]=json.Number("999999")}},
 }
 for _,attack:=range attacks{
  t.Run(attack.name,func(t *testing.T){
   raw,err:=pyJSON(original,false);if err!=nil{t.Fatal(err)}
   duplicate,err:=parseStrict(raw);if err!=nil{t.Fatal(err)}
   target:=duplicate.(map[string]any)
   attack.alter(target)
   if err:=checkBody(target,id);err==nil{
    t.Fatal("forged domain-valid-looking numeric body passed Go P3 gate")
   }
  })
 }
}

func TestGoRejectsUnknownPhysicalRequiredComponents(t *testing.T){
 source:=realSource(t)
 rows,err:=physicalRecords(source.Events);if err!=nil{t.Fatal(err)}
 mutated:=false
 for _,row:=range rows{
  if row["event"]=="run.extended"{
   row["payload"].(map[string]any)["required_components"]=[]any{"prior","unregistered"}
   mutated=true
   break
  }
 }
 if !mutated{t.Fatal("actual fixture missing run.extended")}
 if _,_,err:=scanCompletedSource(rows);err==nil{
  t.Fatal("unregistered physical component accepted by Go")
 }
}


func TestGoRequiresClosedPSV1TwoEnvironmentAndNoLoad(t *testing.T){
 s:=realSource(t)
 rows,err:=physicalRecords(s.Events)
 if err!=nil{t.Fatal(err)}
 for _,tt:=range []struct{name string; mutate func(map[string]any)}{
  {"unknown env field",func(p map[string]any){
   p["environment"].(map[string]any)["WorldRevision"]=json.Number("9")
  }},
  {"different fluid",func(p map[string]any){
   p["environment"].(map[string]any)["fluid_access"]=json.Number("0.2")
  }},
  {"different carbohydrate",func(p map[string]any){
   p["environment"].(map[string]any)["carbohydrate_intake"]=json.Number("0")
  }},
  {"hidden work",func(p map[string]any){
   p["load"].(map[string]any)["intensity"]=json.Number("1")
  }},
  {"extra load field",func(p map[string]any){
   p["load"].(map[string]any)["event_id"]="forged"
  }},
 }{
  t.Run(tt.name,func(t *testing.T){
   copied:=make([]map[string]any,len(rows))
   for i,rec:=range rows{
    encoded,err:=pyJSON(rec,false);if err!=nil{t.Fatal(err)}
    value,err:=parseStrict(encoded);if err!=nil{t.Fatal(err)}
    copied[i]=value.(map[string]any)
   }
   changed:=false
   for _,rec:=range copied{
    if rec["event"]=="body.advanced"{
     tt.mutate(rec["payload"].(map[string]any))
     changed=true;break
    }
   }
   if !changed{t.Fatal("missing physical interval")}
   if _,_,err:=scanCompletedSource(copied);err==nil{
    t.Fatal("unregistered physical environment/load passed Go preflight")
   }
  })
 }
}

func TestGoRejectsInvalidSubjectIDGrammar(t *testing.T){
 src:=realSource(t)
 rows,err:=physicalRecords(src.Events);if err!=nil{t.Fatal(err)}
 for _,subject:=range []string{"NPC.0001","../escape","npc/0001","npc.0001\\u0000","npc.0001 ", ""}{
  if validSubjectID(subject){t.Fatalf("unexpected subject accepted: %q",subject)}
 }
 target:=-1
 for i,rec:=range rows{
  if rec["event"]=="body.advance.intent.v1"{target=i;break}
 }
 if target<0{t.Fatal("no physical intent")}
 rows[target]["payload"].(map[string]any)["character_id"]="npc/invalid"
 if _,_,err:=scanCompletedSource(rows);err==nil{t.Fatal("invalid physical actor ID passed")}
}

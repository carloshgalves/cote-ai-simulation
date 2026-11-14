// Package embodimentv1 provides the Go-owned, read-only physical source gate.
//
// It DOES NOT provide a WorldRevision, EventDraft, CycleCommit, or producer
// authorization. A caller must establish a coherent OS-enforced A1 source cut
// and a Genesis-pinned producer policy before physical admission is possible.
// This package intentionally has no CommitCoordinator or storage interface.
package embodimentv1

import (
 "bytes"
 "encoding/hex"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "math"
 "sort"
 "regexp"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
 "strconv"
 "strings"
)

var ErrUnadmitted = errors.New("physical source not admitted to CSF")
var ErrInvalidPhysical = errors.New("physical source contract invalid")
const (
 maxEnvelope = 4_194_304
 maxSnapshot = 16 * 1024 * 1024
 maxLog = 64 * 1024 * 1024
 maxLine = 1024 * 1024
 maxRecords = 100000
)

type Source struct {
 Snapshot, Events, Evidence []byte
 // These must come from an independently authorized, immutable host-owned
 // cut. A caller-produced checksum cannot authorize itself.
 PinnedSnapshotSHA256, PinnedEventsSHA256 [32]byte
}

// Verified is intentionally opaque. This is NOT an admission permit or
// world-state handle. No method exposes BodyState to actor contexts.
type Verified struct {
 source [2][32]byte
 commandCount, transitionCount int
}

// InspectUnadmitted independently checks typed envelope, physical event order,
// completion receipts, source hashes and final numeric-body linkage in Go.
// It cannot claim Python's dynamics equations are correct: PSV1-2 replays them
// on a source whose immutable provenance must be established by A1.
func InspectUnadmitted(s Source) (Verified, error) {
 var zero Verified
 if len(s.Snapshot)==0||len(s.Snapshot)>maxSnapshot||len(s.Events)==0||
  len(s.Events)>maxLog||len(s.Evidence)==0||len(s.Evidence)>maxEnvelope{
  return zero,ErrInvalidPhysical
 }
 snapshotHash:=identity.OpaqueContentHash(s.Snapshot)
 eventsHash:=identity.OpaqueContentHash(s.Events)
 if snapshotHash!=s.PinnedSnapshotSHA256||eventsHash!=s.PinnedEventsSHA256{
  return zero,ErrInvalidPhysical
 }
 v,err:=parseStrict(s.Evidence)
 if err!=nil{return zero,ErrInvalidPhysical}
 m,ok:=object(v,"kind","schema_version","authority","admission","completion","source","transitions","subject_tips","command_receipts")
 if !ok||m["kind"]!="physical.verified-transition-evidence.v1"||
  !integerEq(m["schema_version"],1)||m["authority"]!="ENGINE_ONLY"||
  m["admission"]!="PENDING_GO_ADMISSION"||m["completion"]!="ATTESTED_COMPLETE"{
  return zero,ErrUnadmitted
 }
 src,ok:=object(m["source"],"snapshot_raw_sha256","events_raw_sha256","snapshot_document_sha256",
  "normalised_log_sha256","snapshot_version","local_origin","prior_version","dynamics_version",
  "log_record_count","verified_transition_count","completed_phase_count","completed_transition_count")
 if !ok||!integerEq(src["snapshot_version"],1)||src["local_origin"]!="Y1_START"||
  src["snapshot_raw_sha256"]!=hex.EncodeToString(snapshotHash[:])||
  src["events_raw_sha256"]!=hex.EncodeToString(eventsHash[:]) ||
  !hexDigest(src["snapshot_document_sha256"])||!hexDigest(src["normalised_log_sha256"]){
  return zero,ErrInvalidPhysical
 }
 for _,field:=range []string{"prior_version","dynamics_version"}{
  label,ok:=src[field].(string)
  if !ok||len(label)==0||len(label)>96{return zero,ErrInvalidPhysical}
 }
 recs,err:=physicalRecords(s.Events)
 if err!=nil{return zero,ErrInvalidPhysical}
 n,ok:=intValue(src["log_record_count"]);if !ok||n!=len(recs){return zero,ErrInvalidPhysical}
 norm:=bytes.NewBuffer(nil)
 for _,rec:=range recs{
  row:=cloneMap(rec)
  row["wall_time"]="1970-01-01T00:00:00+00:00"
  encoded,err:=pyJSON(row,true)
  if err!=nil{return zero,ErrInvalidPhysical}
  norm.Write(encoded);norm.WriteByte('\n')
 }
 normHash:=identity.OpaqueContentHash(norm.Bytes())
 if src["normalised_log_sha256"]!=hex.EncodeToString(normHash[:]){
  return zero,ErrInvalidPhysical
 }
 transitions,ok:=m["transitions"].([]any)
 if !ok||len(transitions)==0||len(transitions)>maxRecords{
  return zero,ErrInvalidPhysical
 }
 sourceTransitions,sourceCommands,err:=scanCompletedSource(recs)
 if err!=nil||len(sourceTransitions)!=len(transitions){return zero,ErrInvalidPhysical}
 for i,raw:=range transitions{
  t,ok:=object(raw,"subject_id","source_local_seq","source_local_sim_time",
   "source_local_after_hours","source_local_delta_hours","source_segment",
   "semantic_record_sha256","channel_diff_sha256")
  if !ok||!transitionMatches(t,sourceTransitions[i]){return zero,ErrInvalidPhysical}
 }
 receipts,ok:=m["command_receipts"].([]any)
 if !ok||len(receipts)==0||len(receipts)!=len(sourceCommands){return zero,ErrInvalidPhysical}
 for i,raw:=range receipts{
  got,ok:=object(raw,"subject_id","intent_seq","complete_seq","first_advance_seq",
   "last_advance_seq","planned_days","emitted_event_count","t_hours_after",
   "final_body_sha256","semantic_completion_sha256")
  if !ok||!commandMatches(got,sourceCommands[i]){return zero,ErrInvalidPhysical}
 }
 tips,ok:=m["subject_tips"].([]any)
 if !ok||len(tips)!=len(receipts){return zero,ErrInvalidPhysical}
 tipsBySubject:=map[string]map[string]any{}
 for _,raw:=range tips{
  tip,ok:=object(raw,"subject_id","last_local_seq","verified_body_sha256",
   "verified_body","verified_advance_count","visibility")
  if !ok||tip["visibility"]!="ENGINE_ONLY"{return zero,ErrInvalidPhysical}
  subject,ok:=tip["subject_id"].(string)
  if !ok||len(subject)==0||len(subject)>128||tipsBySubject[subject]!=nil{return zero,ErrInvalidPhysical}
  if err:=checkBody(tip["verified_body"],subject);err!=nil{return zero,ErrInvalidPhysical}
  canonical,err:=pyJSON(tip["verified_body"],false)
  if err!=nil{return zero,ErrInvalidPhysical}
  dg:=identity.OpaqueContentHash(canonical)
  if tip["verified_body_sha256"]!=hex.EncodeToString(dg[:]){return zero,ErrInvalidPhysical}
  tipsBySubject[subject]=tip
 }
 for _,command:=range sourceCommands{
  subject:=command["subject_id"].(string)
  tip:=tipsBySubject[subject]
  if tip==nil||tip["verified_body_sha256"]!=command["final_body_sha256"] ||
   !numberEq(tip["verified_body"].(map[string]any)["t_hours"],command["t_hours_after"]) ||
   !numberEq(tip["last_local_seq"],command["last_advance_seq"]) ||
   !numberEq(tip["verified_advance_count"],command["emitted_event_count"]){
   return zero,ErrInvalidPhysical
  }
 }
 count,ok:=intValue(src["verified_transition_count"])
 if !ok||count!=len(transitions){return zero,ErrInvalidPhysical}
 phases,ok:=intValue(src["completed_phase_count"])
 if !ok||phases!=len(sourceCommands){return zero,ErrInvalidPhysical}
 completed,ok:=intValue(src["completed_transition_count"])
 if !ok||completed!=len(transitions){return zero,ErrInvalidPhysical}
 return Verified{source:[2][32]byte{snapshotHash,eventsHash},
  commandCount:len(sourceCommands),transitionCount:len(transitions)},nil
}

// There is no public Commit method: only the existing CSF CommitCoordinator
// may publish a physical world revision after bundle registration/A3/A4.
func (Verified) AdmitToWorld() error { return ErrUnadmitted }

func hexDigest(v any)bool{
 s,ok:=v.(string);if !ok||len(s)!=64{return false}
 for _,c:=range s{if !strings.ContainsRune("0123456789abcdef",c){return false}}
 return true
}
func intValue(v any)(int,bool){
 s,ok:=v.(json.Number);if !ok||strings.ContainsAny(s.String(),".eE"){return 0,false}
 n,err:=strconv.Atoi(s.String());return n,err==nil&&n>=0
}
func integerEq(v any,want int)bool{n,ok:=intValue(v);return ok&&n==want}
func number(v any)(float64,bool){
 n,ok:=v.(json.Number);if !ok{return 0,false}
 f,err:=strconv.ParseFloat(n.String(),64)
 return f,err==nil&&!math.IsNaN(f)&&!math.IsInf(f,0)
}
func numberEq(a,b any)bool{
 x,ok:=number(a);if !ok{return false}
 y,ok:=number(b);return ok&&x==y
}
func object(value any,fields ...string)(map[string]any,bool){
 m,ok:=value.(map[string]any)
 if !ok||len(m)!=len(fields){return nil,false}
 for _,k:=range fields{if _,exists:=m[k];!exists{return nil,false}}
 return m,true
}
func cloneMap(m map[string]any)map[string]any{
 out:=make(map[string]any,len(m))
 for k,v:=range m{out[k]=v}
 return out
}

// strict lexical decoder prevents duplicate keys and rejects excessive
// nesting, unsupported numeric magnitudes, trailing JSON and NaN/Infinity.
func parseStrict(raw []byte)(any,error){
 dec:=json.NewDecoder(bytes.NewReader(raw));dec.UseNumber()
 value,err:=decodeValue(dec,0);if err!=nil{return nil,err}
 if _,err=dec.Token();err!=io.EOF{return nil,ErrInvalidPhysical}
 return value,nil
}
func decodeValue(dec *json.Decoder,depth int)(any,error){
 if depth>64{return nil,ErrInvalidPhysical}
 tok,err:=dec.Token();if err!=nil{return nil,err}
 switch v:=tok.(type){
 case json.Delim:
  if v=='{'{
   m:=map[string]any{}
   for dec.More(){
    k,err:=dec.Token();if err!=nil{return nil,err}
    key,ok:=k.(string);if !ok{return nil,ErrInvalidPhysical}
    if _,exists:=m[key];exists{return nil,ErrInvalidPhysical}
    value,err:=decodeValue(dec,depth+1);if err!=nil{return nil,err}
    m[key]=value
   }
   end,err:=dec.Token();if err!=nil||end!=json.Delim('}'){return nil,ErrInvalidPhysical}
   return m,nil
  }
  if v=='['{
   values:=[]any{}
   for dec.More(){
    value,err:=decodeValue(dec,depth+1);if err!=nil{return nil,err}
    values=append(values,value)
    if len(values)>maxRecords{return nil,ErrInvalidPhysical}
   }
   end,err:=dec.Token();if err!=nil||end!=json.Delim(']'){return nil,ErrInvalidPhysical}
   return values,nil
  }
  return nil,ErrInvalidPhysical
 case json.Number:
  if _,ok:=number(v);!ok{return nil,ErrInvalidPhysical}
  return v,nil
 case string,bool,nil:
  return v,nil
 default:return nil,ErrInvalidPhysical
 }
}
func pyJSON(v any,spaced bool)([]byte,error){
 b:=bytes.NewBuffer(nil)
 if err:=appendJSON(b,v,spaced,0);err!=nil{return nil,err}
 return b.Bytes(),nil
}
func appendJSON(b *bytes.Buffer,v any,spaced bool,depth int)error{
 if depth>64{return ErrInvalidPhysical}
 sep,colon:=",",":"
 if spaced{sep=", ";colon=": "}
 switch x:=v.(type){
 case map[string]any:
  keys:=make([]string,0,len(x))
  for k:=range x{keys=append(keys,k)}
  sort.Strings(keys);b.WriteByte('{')
  for i,k:=range keys{
   if i>0{b.WriteString(sep)}
   if err:=appendJSON(b,k,spaced,depth+1);err!=nil{return err}
   b.WriteString(colon)
   if err:=appendJSON(b,x[k],spaced,depth+1);err!=nil{return err}
  }
  b.WriteByte('}')
 case []any:
  b.WriteByte('[')
  for i,child:=range x{
   if i>0{b.WriteString(sep)}
   if err:=appendJSON(b,child,spaced,depth+1);err!=nil{return err}
  }
  b.WriteByte(']')
 case string:
  e:=json.NewEncoder(b);e.SetEscapeHTML(false)
  if err:=e.Encode(x);err!=nil{return err}
  out:=b.Bytes();b.Truncate(len(out)-1)
 case json.Number:
  if _,ok:=number(x);!ok{return ErrInvalidPhysical}
  b.WriteString(x.String())
 case bool:
  if x{b.WriteString("true")}else{b.WriteString("false")}
 case nil:b.WriteString("null")
 default:return ErrInvalidPhysical
 }
 return nil
}
var wallClockPattern = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(?:\.[0-9]{1,9})?\+00:00// Package embodimentv1 provides the Go-owned, read-only physical source gate.
//
// It DOES NOT provide a WorldRevision, EventDraft, CycleCommit, or producer
// authorization. A caller must establish a coherent OS-enforced A1 source cut
// and a Genesis-pinned producer policy before physical admission is possible.
// This package intentionally has no CommitCoordinator or storage interface.
package embodimentv1

import (
 "bytes"
 "encoding/hex"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "math"
 "sort"
 "regexp"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
 "strconv"
 "strings"
)

var ErrUnadmitted = errors.New("physical source not admitted to CSF")
var ErrInvalidPhysical = errors.New("physical source contract invalid")
const (
 maxEnvelope = 4_194_304
 maxSnapshot = 16 * 1024 * 1024
 maxLog = 64 * 1024 * 1024
 maxLine = 1024 * 1024
 maxRecords = 100000
)

type Source struct {
 Snapshot, Events, Evidence []byte
 // These must come from an independently authorized, immutable host-owned
 // cut. A caller-produced checksum cannot authorize itself.
 PinnedSnapshotSHA256, PinnedEventsSHA256 [32]byte
}

// Verified is intentionally opaque. This is NOT an admission permit or
// world-state handle. No method exposes BodyState to actor contexts.
type Verified struct {
 source [2][32]byte
 commandCount, transitionCount int
}

// InspectUnadmitted independently checks typed envelope, physical event order,
// completion receipts, source hashes and final numeric-body linkage in Go.
// It cannot claim Python's dynamics equations are correct: PSV1-2 replays them
// on a source whose immutable provenance must be established by A1.
func InspectUnadmitted(s Source) (Verified, error) {
 var zero Verified
 if len(s.Snapshot)==0||len(s.Snapshot)>maxSnapshot||len(s.Events)==0||
  len(s.Events)>maxLog||len(s.Evidence)==0||len(s.Evidence)>maxEnvelope{
  return zero,ErrInvalidPhysical
 }
 snapshotHash:=identity.OpaqueContentHash(s.Snapshot)
 eventsHash:=identity.OpaqueContentHash(s.Events)
 if snapshotHash!=s.PinnedSnapshotSHA256||eventsHash!=s.PinnedEventsSHA256{
  return zero,ErrInvalidPhysical
 }
 v,err:=parseStrict(s.Evidence)
 if err!=nil{return zero,ErrInvalidPhysical}
 m,ok:=object(v,"kind","schema_version","authority","admission","completion","source","transitions","subject_tips","command_receipts")
 if !ok||m["kind"]!="physical.verified-transition-evidence.v1"||
  !integerEq(m["schema_version"],1)||m["authority"]!="ENGINE_ONLY"||
  m["admission"]!="PENDING_GO_ADMISSION"||m["completion"]!="ATTESTED_COMPLETE"{
  return zero,ErrUnadmitted
 }
 src,ok:=object(m["source"],"snapshot_raw_sha256","events_raw_sha256","snapshot_document_sha256",
  "normalised_log_sha256","snapshot_version","local_origin","prior_version","dynamics_version",
  "log_record_count","verified_transition_count","completed_phase_count","completed_transition_count")
 if !ok||!integerEq(src["snapshot_version"],1)||src["local_origin"]!="Y1_START"||
  src["snapshot_raw_sha256"]!=hex.EncodeToString(snapshotHash[:])||
  src["events_raw_sha256"]!=hex.EncodeToString(eventsHash[:]) ||
  !hexDigest(src["snapshot_document_sha256"])||!hexDigest(src["normalised_log_sha256"]){
  return zero,ErrInvalidPhysical
 }
 for _,field:=range []string{"prior_version","dynamics_version"}{
  label,ok:=src[field].(string)
  if !ok||len(label)==0||len(label)>96{return zero,ErrInvalidPhysical}
 }
 recs,err:=physicalRecords(s.Events)
 if err!=nil{return zero,ErrInvalidPhysical}
 n,ok:=intValue(src["log_record_count"]);if !ok||n!=len(recs){return zero,ErrInvalidPhysical}
 norm:=bytes.NewBuffer(nil)
 for _,rec:=range recs{
  row:=cloneMap(rec)
  row["wall_time"]="1970-01-01T00:00:00+00:00"
  encoded,err:=pyJSON(row,true)
  if err!=nil{return zero,ErrInvalidPhysical}
  norm.Write(encoded);norm.WriteByte('\n')
 }
 normHash:=identity.OpaqueContentHash(norm.Bytes())
 if src["normalised_log_sha256"]!=hex.EncodeToString(normHash[:]){
  return zero,ErrInvalidPhysical
 }
 transitions,ok:=m["transitions"].([]any)
 if !ok||len(transitions)==0||len(transitions)>maxRecords{
  return zero,ErrInvalidPhysical
 }
 sourceTransitions,sourceCommands,err:=scanCompletedSource(recs)
 if err!=nil||len(sourceTransitions)!=len(transitions){return zero,ErrInvalidPhysical}
 for i,raw:=range transitions{
  t,ok:=object(raw,"subject_id","source_local_seq","source_local_sim_time",
   "source_local_after_hours","source_local_delta_hours","source_segment",
   "semantic_record_sha256","channel_diff_sha256")
  if !ok||!transitionMatches(t,sourceTransitions[i]){return zero,ErrInvalidPhysical}
 }
 receipts,ok:=m["command_receipts"].([]any)
 if !ok||len(receipts)==0||len(receipts)!=len(sourceCommands){return zero,ErrInvalidPhysical}
 for i,raw:=range receipts{
  got,ok:=object(raw,"subject_id","intent_seq","complete_seq","first_advance_seq",
   "last_advance_seq","planned_days","emitted_event_count","t_hours_after",
   "final_body_sha256","semantic_completion_sha256")
  if !ok||!commandMatches(got,sourceCommands[i]){return zero,ErrInvalidPhysical}
 }
 tips,ok:=m["subject_tips"].([]any)
 if !ok||len(tips)!=len(receipts){return zero,ErrInvalidPhysical}
 tipsBySubject:=map[string]map[string]any{}
 for _,raw:=range tips{
  tip,ok:=object(raw,"subject_id","last_local_seq","verified_body_sha256",
   "verified_body","verified_advance_count","visibility")
  if !ok||tip["visibility"]!="ENGINE_ONLY"{return zero,ErrInvalidPhysical}
  subject,ok:=tip["subject_id"].(string)
  if !ok||len(subject)==0||len(subject)>128||tipsBySubject[subject]!=nil{return zero,ErrInvalidPhysical}
  if err:=checkBody(tip["verified_body"],subject);err!=nil{return zero,ErrInvalidPhysical}
  canonical,err:=pyJSON(tip["verified_body"],false)
  if err!=nil{return zero,ErrInvalidPhysical}
  dg:=identity.OpaqueContentHash(canonical)
  if tip["verified_body_sha256"]!=hex.EncodeToString(dg[:]){return zero,ErrInvalidPhysical}
  tipsBySubject[subject]=tip
 }
 for _,command:=range sourceCommands{
  subject:=command["subject_id"].(string)
  tip:=tipsBySubject[subject]
  if tip==nil||tip["verified_body_sha256"]!=command["final_body_sha256"] ||
   !numberEq(tip["verified_body"].(map[string]any)["t_hours"],command["t_hours_after"]) ||
   !numberEq(tip["last_local_seq"],command["last_advance_seq"]) ||
   !numberEq(tip["verified_advance_count"],command["emitted_event_count"]){
   return zero,ErrInvalidPhysical
  }
 }
 count,ok:=intValue(src["verified_transition_count"])
 if !ok||count!=len(transitions){return zero,ErrInvalidPhysical}
 phases,ok:=intValue(src["completed_phase_count"])
 if !ok||phases!=len(sourceCommands){return zero,ErrInvalidPhysical}
 completed,ok:=intValue(src["completed_transition_count"])
 if !ok||completed!=len(transitions){return zero,ErrInvalidPhysical}
 return Verified{source:[2][32]byte{snapshotHash,eventsHash},
  commandCount:len(sourceCommands),transitionCount:len(transitions)},nil
}

// There is no public Commit method: only the existing CSF CommitCoordinator
// may publish a physical world revision after bundle registration/A3/A4.
func (Verified) AdmitToWorld() error { return ErrUnadmitted }

func hexDigest(v any)bool{
 s,ok:=v.(string);if !ok||len(s)!=64{return false}
 for _,c:=range s{if !strings.ContainsRune("0123456789abcdef",c){return false}}
 return true
}
func intValue(v any)(int,bool){
 s,ok:=v.(json.Number);if !ok||strings.ContainsAny(s.String(),".eE"){return 0,false}
 n,err:=strconv.Atoi(s.String());return n,err==nil&&n>=0
}
func integerEq(v any,want int)bool{n,ok:=intValue(v);return ok&&n==want}
func number(v any)(float64,bool){
 n,ok:=v.(json.Number);if !ok{return 0,false}
 f,err:=strconv.ParseFloat(n.String(),64)
 return f,err==nil&&!math.IsNaN(f)&&!math.IsInf(f,0)
}
func numberEq(a,b any)bool{
 x,ok:=number(a);if !ok{return false}
 y,ok:=number(b);return ok&&x==y
}
func object(value any,fields ...string)(map[string]any,bool){
 m,ok:=value.(map[string]any)
 if !ok||len(m)!=len(fields){return nil,false}
 for _,k:=range fields{if _,exists:=m[k];!exists{return nil,false}}
 return m,true
}
func cloneMap(m map[string]any)map[string]any{
 out:=make(map[string]any,len(m))
 for k,v:=range m{out[k]=v}
 return out
}

// strict lexical decoder prevents duplicate keys and rejects excessive
// nesting, unsupported numeric magnitudes, trailing JSON and NaN/Infinity.
func parseStrict(raw []byte)(any,error){
 dec:=json.NewDecoder(bytes.NewReader(raw));dec.UseNumber()
 value,err:=decodeValue(dec,0);if err!=nil{return nil,err}
 if _,err=dec.Token();err!=io.EOF{return nil,ErrInvalidPhysical}
 return value,nil
}
func decodeValue(dec *json.Decoder,depth int)(any,error){
 if depth>64{return nil,ErrInvalidPhysical}
 tok,err:=dec.Token();if err!=nil{return nil,err}
 switch v:=tok.(type){
 case json.Delim:
  if v=='{'{
   m:=map[string]any{}
   for dec.More(){
    k,err:=dec.Token();if err!=nil{return nil,err}
    key,ok:=k.(string);if !ok{return nil,ErrInvalidPhysical}
    if _,exists:=m[key];exists{return nil,ErrInvalidPhysical}
    value,err:=decodeValue(dec,depth+1);if err!=nil{return nil,err}
    m[key]=value
   }
   end,err:=dec.Token();if err!=nil||end!=json.Delim('}'){return nil,ErrInvalidPhysical}
   return m,nil
  }
  if v=='['{
   values:=[]any{}
   for dec.More(){
    value,err:=decodeValue(dec,depth+1);if err!=nil{return nil,err}
    values=append(values,value)
    if len(values)>maxRecords{return nil,ErrInvalidPhysical}
   }
   end,err:=dec.Token();if err!=nil||end!=json.Delim(']'){return nil,ErrInvalidPhysical}
   return values,nil
  }
  return nil,ErrInvalidPhysical
 case json.Number:
  if _,ok:=number(v);!ok{return nil,ErrInvalidPhysical}
  return v,nil
 case string,bool,nil:
  return v,nil
 default:return nil,ErrInvalidPhysical
 }
}
func pyJSON(v any,spaced bool)([]byte,error){
 b:=bytes.NewBuffer(nil)
 if err:=appendJSON(b,v,spaced,0);err!=nil{return nil,err}
 return b.Bytes(),nil
}
func appendJSON(b *bytes.Buffer,v any,spaced bool,depth int)error{
 if depth>64{return ErrInvalidPhysical}
 sep,colon:=",",":"
 if spaced{sep=", ";colon=": "}
 switch x:=v.(type){
 case map[string]any:
  keys:=make([]string,0,len(x))
  for k:=range x{keys=append(keys,k)}
  sort.Strings(keys);b.WriteByte('{')
  for i,k:=range keys{
   if i>0{b.WriteString(sep)}
   if err:=appendJSON(b,k,spaced,depth+1);err!=nil{return err}
   b.WriteString(colon)
   if err:=appendJSON(b,x[k],spaced,depth+1);err!=nil{return err}
  }
  b.WriteByte('}')
 case []any:
  b.WriteByte('[')
  for i,child:=range x{
   if i>0{b.WriteString(sep)}
   if err:=appendJSON(b,child,spaced,depth+1);err!=nil{return err}
  }
  b.WriteByte(']')
 case string:
  e:=json.NewEncoder(b);e.SetEscapeHTML(false)
  if err:=e.Encode(x);err!=nil{return err}
  out:=b.Bytes();b.Truncate(len(out)-1)
 case json.Number:
  if _,ok:=number(x);!ok{return ErrInvalidPhysical}
  b.WriteString(x.String())
 case bool:
  if x{b.WriteString("true")}else{b.WriteString("false")}
 case nil:b.WriteString("null")
 default:return ErrInvalidPhysical
 }
 return nil
}
)

func physicalRecords(raw []byte)([]map[string]any,error){
 if len(raw)==0||raw[len(raw)-1]!='\n'{return nil,ErrInvalidPhysical}
 lines:=bytes.Split(raw[:len(raw)-1],[]byte{'\n'})
 if len(lines)<4||len(lines)>maxRecords{return nil,ErrInvalidPhysical}
 recs:=make([]map[string]any,0,len(lines))
 for i,line:=range lines{
  if len(line)==0||len(line)>maxLine{return nil,ErrInvalidPhysical}
  v,err:=parseStrict(line);if err!=nil{return nil,err}
  rec,ok:=object(v,"seq","event","sim_time","wall_time","payload")
  if !ok||!integerEq(rec["seq"],i){return nil,ErrInvalidPhysical}
  if _,ok:=rec["payload"].(map[string]any);!ok{return nil,ErrInvalidPhysical}
  clock,ok:=rec["wall_time"].(string)
  if !ok||len(clock)>64{return nil,ErrInvalidPhysical}
  if !wallClockPattern.MatchString(clock) {
   return nil,ErrInvalidPhysical
  }
  recs=append(recs,rec)
 }
 return recs,nil
}


func hashJSON(value any, spaced bool) (string,error) {
 b,err:=pyJSON(value,spaced);if err!=nil{return "",err}
 dig:=identity.OpaqueContentHash(b);return hex.EncodeToString(dig[:]),nil
}

func onlyPhysicalComponents(value any)bool{
 parts,ok:=value.([]any)
 return ok&&len(parts)==2&&parts[0]=="prior"&&parts[1]=="dynamics"
}

func strictRecordField(r map[string]any,k string)string {
 s,_:=r[k].(string);return s
}

func localInstant(hours float64)(string,error) {
 if hours<0||hours>=100000||math.IsNaN(hours)||math.IsInf(hours,0){return "",ErrInvalidPhysical}
 if hours==0{return "Y1_START",nil}
 return fmt.Sprintf("Y1_START+%09.3fh",hours),nil
}

func scanCompletedSource(rows []map[string]any)([]map[string]any,[]map[string]any,error){
 failed:=func()([]map[string]any,[]map[string]any,error){return nil,nil,ErrInvalidPhysical}
 if strictRecordField(rows[0],"event")!="run.started"||
  strictRecordField(rows[0],"sim_time")!="Y1_START"{return failed()}
 opening,ok:=rows[0]["payload"].(map[string]any);if !ok{return failed()}
 originalMeta,ok:=opening["run_metadata"].(map[string]any);if !ok{return failed()}
 if _,ok=object(opening,"run_metadata","required_components");!ok||
  !onlyPhysicalComponents(opening["required_components"]){return failed()}
 originalMetaJSON,err:=pyJSON(originalMeta,false);if err!=nil{return failed()}
 completedSeeds:=false
 phase:="seed"
 used:=map[string]bool{}
 var intent map[string]any
 var intentSeq,advanceCount,firstSeq,lastSeq int
 var lastEnd float64
 var segmentHours float64
 var transitions,commands []map[string]any
 sampled:=map[string]bool{}
 for i:=1;i<len(rows);i++{
  row:=rows[i];kind:=strictRecordField(row,"event")
  payload:=row["payload"].(map[string]any)
  switch kind {
  case "capacity.sampled":
   if phase!="seed"||completedSeeds||row["sim_time"]!="Y1_START"{return failed()}
   subject,ok:=payload["character_id"].(string)
   if !ok||subject==""||sampled[subject]{return failed()}
   sampled[subject]=true
  case "cohort.correlation_report":
   if phase!="seed"||completedSeeds||len(sampled)==0||row["sim_time"]!="Y1_START"{return failed()}
   completedSeeds=true
  case "run.extended":
   if !completedSeeds||(phase!="seed"&&phase!="completed")||row["sim_time"]!="Y1_START"{return failed()}
   ext,ok:=object(payload,"run_metadata","required_components");if !ok{return failed()}
   meta,err:=pyJSON(ext["run_metadata"],false)
   if err!=nil||!bytes.Equal(meta,originalMetaJSON)||
    !onlyPhysicalComponents(ext["required_components"]){return failed()}
   phase="extended"
   intent=nil
   intentSeq=0;advanceCount=0;firstSeq=0;lastSeq=0;lastEnd=0
  case "body.advance.intent.v1":
   if phase!="extended"{return failed()}
   intent,ok=object(payload,"protocol","character_id","planned_days",
    "expected_event_count","sleep_hours","quality","wbgt_c",
    "t_hours_before","dynamics_version")
   if !ok||intent["protocol"]!="psv1-2.advance.v1"{return failed()}
   subject,ok:=intent["character_id"].(string)
   if !ok||subject==""||used[subject]||len(subject)>128{return failed()}
   if !numberEq(intent["t_hours_before"],json.Number("0"))||
    row["sim_time"]!="Y1_START"{return failed()}
   days,valid:=intValue(intent["planned_days"])
   if !valid||days<1||days>50000{return failed()}
   n,valid:=intValue(intent["expected_event_count"])
   if !valid||n<1||n>100000{return failed()}
   sleep,valid:=number(intent["sleep_hours"])
   if !valid||sleep<0||sleep>24{return failed()}
   wbgt,valid:=number(intent["wbgt_c"])
   if !valid||wbgt < -100||wbgt>100{return failed()}
   if intent["quality"]!="poor"&&intent["quality"]!="fair"&&intent["quality"]!="good"{return failed()}
   expected:=days*2
   if sleep==0||sleep==24{expected=days}
   if expected!=n{return failed()}
   if intent["dynamics_version"]!=originalMeta["dynamics_version"]{return failed()}
   segmentHours=sleep
   intentSeq=i
   phase="intent"
   used[subject]=true
  case "body.advanced":
   if phase!="intent"&&phase!="advancing"{return failed()}
   subject,ok:=payload["character_id"].(string)
   if !ok||subject!=intent["character_id"]{return failed()}
   expectedN,_:=intValue(intent["expected_event_count"])
   if advanceCount>=expectedN{return failed()}
   if _,ok=object(payload,"character_id","dt_hours","segment",
    "t_hours_before","t_hours_after","environment","load",
    "dynamics_version","channels_changed");!ok{return failed()}
   dt,valid:=number(payload["dt_hours"]);if !valid||dt<=0||dt>24{return failed()}
   start,valid:=number(payload["t_hours_before"]);if !valid||start!=lastEnd{return failed()}
   end,valid:=number(payload["t_hours_after"])
   if !valid||math.Abs((start+dt)-end)>1e-8{return failed()}
   clock,err:=localInstant(end)
   if err!=nil||row["sim_time"]!=clock{return failed()}
   env,ok:=payload["environment"].(map[string]any)
   if !ok||!numberEq(env["wbgt_c"],intent["wbgt_c"]){return failed()}
   wantLabel,wantHours:="awake",24.0-segmentHours
   if segmentHours==24{wantLabel="asleep";wantHours=24}
   if segmentHours>0&&segmentHours<24 && advanceCount%2==1{
    wantLabel="asleep";wantHours=segmentHours
   }
   if segmentHours==0{wantLabel="awake";wantHours=24}
   if payload["segment"]!=wantLabel||math.Abs(dt-wantHours)>1e-9{return failed()}
   asleep,ok:=env["asleep"].(bool)
   if !ok||asleep!=(wantLabel=="asleep"){return failed()}
   if wantLabel=="asleep"&&env["sleep_quality"]!=intent["quality"]{return failed()}
   if payload["dynamics_version"]!=intent["dynamics_version"]{return failed()}
   normalized:=cloneMap(row);normalized["wall_time"]="1970-01-01T00:00:00+00:00"
   semHash,err:=hashJSON(normalized,false);if err!=nil{return failed()}
   diffHash,err:=hashJSON(payload["channels_changed"],false);if err!=nil{return failed()}
   transitions=append(transitions,map[string]any{
    "subject_id":subject,"source_local_seq":row["seq"],
    "source_local_sim_time":row["sim_time"],
    "source_local_after_hours":payload["t_hours_after"],
    "source_local_delta_hours":payload["dt_hours"],
    "source_segment":payload["segment"],
    "semantic_record_sha256":semHash,"channel_diff_sha256":diffHash,
   })
   advanceCount++
   if advanceCount==1{firstSeq=i}
   lastSeq=i;lastEnd=end
   phase="advancing"
  case "body.advance.complete.v1":
   if phase!="advancing"{return failed()}
   complete,ok:=object(payload,"protocol","character_id","intent_seq","first_advance_seq",
    "last_advance_seq","planned_days","emitted_event_count","t_hours_after",
    "final_body_sha256","dynamics_version")
   if !ok||complete["protocol"]!="psv1-2.advance.v1"||
    complete["character_id"]!=intent["character_id"]||
    complete["dynamics_version"]!=intent["dynamics_version"]||
    !numberEq(complete["planned_days"],intent["planned_days"])||
    !integerEq(complete["intent_seq"],intentSeq)||
    !integerEq(complete["first_advance_seq"],firstSeq)||
    !integerEq(complete["last_advance_seq"],lastSeq)||
    !integerEq(complete["emitted_event_count"],advanceCount)||
    !numberEq(complete["t_hours_after"],json.Number(strconv.FormatFloat(lastEnd,'f',-1,64)))||
    !hexDigest(complete["final_body_sha256"]){return failed()}
   expectedN,_:=intValue(intent["expected_event_count"])
   if advanceCount!=expectedN||i!=lastSeq+1{return failed()}
   clock,err:=localInstant(lastEnd)
   if err!=nil||row["sim_time"]!=clock{return failed()}
   normalized:=cloneMap(row);normalized["wall_time"]="1970-01-01T00:00:00+00:00"
   semHash,err:=hashJSON(normalized,false);if err!=nil{return failed()}
   commands=append(commands,map[string]any{
    "subject_id":intent["character_id"],"intent_seq":json.Number(strconv.Itoa(intentSeq)),
    "complete_seq":row["seq"],"first_advance_seq":json.Number(strconv.Itoa(firstSeq)),
    "last_advance_seq":json.Number(strconv.Itoa(lastSeq)),
    "planned_days":intent["planned_days"],
    "emitted_event_count":json.Number(strconv.Itoa(advanceCount)),
    "t_hours_after":complete["t_hours_after"],
    "final_body_sha256":complete["final_body_sha256"],
    "semantic_completion_sha256":semHash,
   })
   phase="completed"
  default:return failed()
  }
 }
 if phase!="completed"||len(commands)==0{return failed()}
 return transitions,commands,nil
}

func transitionMatches(received,actual map[string]any)bool{
 for key,want:=range actual{
  got,ok:=received[key];if !ok{return false}
  switch key {
  case "source_local_seq","source_local_after_hours","source_local_delta_hours":
   if !numberEq(got,want){return false}
  default:if got!=want{return false}
  }
 }
 return true
}
func commandMatches(received,actual map[string]any)bool{
 for key,want:=range actual{
  got,ok:=received[key];if !ok{return false}
  switch key {
  case "intent_seq","complete_seq","first_advance_seq","last_advance_seq",
   "planned_days","emitted_event_count","t_hours_after":
   if !numberEq(got,want){return false}
  default:if got!=want{return false}
  }
 }
 return true
}
func checkBody(value any,subject string)error{
 fail:=ErrInvalidPhysical
 b,ok:=object(value,
  "character_id","t_hours","w_prime_balance","peripheral_fatigue",
  "central_fatigue","cumulative_load","sleep","energy","hydration",
  "thermal","soreness","injuries","illnesses","pain")
 if !ok||b["character_id"]!=subject{return fail}
 for _,name:=range []string{"t_hours","central_fatigue"}{
  if _,ok:=number(b[name]);!ok{return fail}
 }
 for _,entry:=range []struct{name string;fields []string}{
  {"w_prime_balance",[]string{"remaining_j","capacity_j","tau_s"}},
  {"cumulative_load",[]string{"acute_7d","chronic_28d"}},
  {"sleep",[]string{"debt_hours","hours_since_wake","circadian_phase","last_sleep"}},
  {"energy",[]string{"substrate_availability","balance_kcal_24h","last_meal_at_h"}},
  {"hydration",[]string{"deficit_pct_body_mass"}},
  {"thermal",[]string{"core_offset_c","wbgt","work_rest_ratio","clothing_insulation"}},
  {"soreness",[]string{"by_region","repeated_bout_adaptation"}},
  {"pain",[]string{"by_region","global_intensity","analgesia"}},
 }{
  m,ok:=object(b[entry.name],entry.fields...)
  if !ok{return fail}
  for k,v:=range m{
   if k=="last_sleep"||k=="by_region"||k=="repeated_bout_adaptation"{continue}
   if _,ok:=number(v);!ok{return fail}
  }
 }
 sleep:=b["sleep"].(map[string]any)
 if sleep["last_sleep"]!=nil{
  last,ok:=object(sleep["last_sleep"],"start_h","end_h","quality")
  if !ok{return fail}
  for _,name:=range []string{"start_h","end_h"}{if _,ok:=number(last[name]);!ok{return fail}}
  if last["quality"]!="good"&&last["quality"]!="fair"&&last["quality"]!="poor"{return fail}
 }
 for _,name:=range []string{"injuries","illnesses"}{
  a,ok:=b[name].([]any)
  if !ok||len(a)!=0{return fail}
 }
 for _,name:=range []string{"peripheral_fatigue","pain"}{
  mapping:=b[name]
  if name=="pain"{mapping=b["pain"].(map[string]any)["by_region"]}
  if !checkRegions(mapping,false){return fail}
 }
 sore:=b["soreness"].(map[string]any)
 regions,ok:=sore["by_region"].(map[string]any)
 if !ok||len(regions)!=4{return fail}
 for _,name:=range []string{"legs","arms","grip","core"}{
  r,ok:=object(regions[name],"latency","pending_onset","expressed")
  if !ok{return fail}
  if _,ok:=number(r["expressed"]);!ok{return fail}
  lag,ok:=r["latency"].([]any);if !ok||len(lag)==0||len(lag)>100{return fail}
  for _,v:=range lag{if _,ok:=number(v);!ok{return fail}}
  pending,ok:=r["pending_onset"].([]any)
  if !ok||len(pending)!=0{return fail}
 }
 repeats,ok:=sore["repeated_bout_adaptation"].(map[string]any)
 if !ok||len(repeats)!=0{return fail}
 // Go-side independent P3 numeric domain gate. The Python replay also
 // validates this, but a forged JSON envelope must not bypass domain checks.
 bounded:=func(v any,min,max float64)bool{
  n,ok:=number(v);return ok&&n>=min&&n<=max
 }
 nonnegative:=func(v any)bool{n,ok:=number(v);return ok&&n>=0}
 positive:=func(v any)bool{n,ok:=number(v);return ok&&n>0}
 if !bounded(b["t_hours"],0,99999)||
  !bounded(b["central_fatigue"],0,1){return fail}
 w:=b["w_prime_balance"].(map[string]any)
 if !nonnegative(w["remaining_j"])||!positive(w["capacity_j"])||
  !positive(w["tau_s"]){return fail}
 rest,_:=number(w["remaining_j"])
 cap,_:=number(w["capacity_j"])
 if rest>cap{return fail}
 load:=b["cumulative_load"].(map[string]any)
 if !nonnegative(load["acute_7d"])||!nonnegative(load["chronic_28d"]){return fail}
 if !nonnegative(sleep["debt_hours"])||!nonnegative(sleep["hours_since_wake"])||
  !bounded(sleep["circadian_phase"],0,24){return fail}
 circ,_:=number(sleep["circadian_phase"]);if circ==24{return fail}
 if sleep["last_sleep"]!=nil{
  episode:=sleep["last_sleep"].(map[string]any)
  begin,validBegin:=number(episode["start_h"])
  end,validEnd:=number(episode["end_h"])
  if !validBegin||!validEnd||begin<0||end<begin{return fail}
 }
 energy:=b["energy"].(map[string]any)
 if !bounded(energy["substrate_availability"],0,1){return fail}
 if energy["last_meal_at_h"]!=nil&&!nonnegative(energy["last_meal_at_h"]){return fail}
 if !nonnegative(b["hydration"].(map[string]any)["deficit_pct_body_mass"]){return fail}
 thermal:=b["thermal"].(map[string]any)
 if !nonnegative(thermal["core_offset_c"])||
  !positive(thermal["work_rest_ratio"])||
  !nonnegative(thermal["clothing_insulation"])||
  !bounded(thermal["wbgt"],-100,100){return fail}
 pain:=b["pain"].(map[string]any)
 if !bounded(pain["global_intensity"],0,1)||
  !bounded(pain["analgesia"],0,1){return fail}
 for _,field:=range []string{"legs","arms","grip","core"}{
  r:=regions[field].(map[string]any)
  if !bounded(r["expressed"],0,1){return fail}
 }
 return nil
}
func checkRegions(value any,_ bool)bool{
 m,ok:=object(value,"legs","arms","grip","core")
 if !ok{return false}
 for _,v:=range m{
  n,ok:=number(v);if !ok||n<0||n>1{return false}
 }
 return true
}

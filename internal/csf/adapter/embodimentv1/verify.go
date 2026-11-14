// Package embodimentv1 provides the Go-owned, read-only physical source gate.
//
// It DOES NOT provide a WorldRevision, EventDraft, CycleCommit, or producer
// authorization. A caller must establish a coherent OS-enforced A1 source cut
// and a Genesis-pinned producer policy before physical admission is possible.
// This package intentionally has no CommitCoordinator or storage interface.
package embodimentv1

import (
 "bytes"
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "math"
 "sort"
 "strconv"
 "strings"
 "time"
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
 snapshotHash:=sha256.Sum256(s.Snapshot)
 eventsHash:=sha256.Sum256(s.Events)
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
 normHash:=sha256.Sum256(norm.Bytes())
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
  dg:=sha256.Sum256(canonical)
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
  wall,err:=time.Parse(time.RFC3339Nano,clock)
  if err!=nil||wall.Location().String()!="Local" && wall.UTC()!=wall{ /* no-op; UTC by offset tested below */ }
  if err!=nil||!strings.HasSuffix(clock,"+00:00"){return nil,ErrInvalidPhysical}
  recs=append(recs,rec)
 }
 return recs,nil
}

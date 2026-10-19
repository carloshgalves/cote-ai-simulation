package trigger

import (
 "errors"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
)

func RuntimeBytes(r Runtime)([]byte,error){
 last:=[]any{uint64(0)}
 if r.HasLast{last=[]any{uint64(1),r.Last}}
 next:=[]any{uint64(0)}
 if r.HasNextRepeat{next=[]any{uint64(1),int64(r.NextRepeat)}}
 return codec.CanonicalPayload([]any{r.DefinitionID[:],uint64(r.Version),last,r.Armed,r.Exhausted,r.ActivationCount,next,uint64(r.EvaluatedThrough)})
}
func DecodeRuntime(raw []byte)(Runtime,error){
 var r Runtime
 v,err:=codec.StrictDecodePayload(raw);if err!=nil{return r,err}
 fields,ok:=v.([]any);if !ok||len(fields)!=8{return r,errors.New("invalid trigger runtime shape")}
 id,ok:=fields[0].([]byte);if !ok||len(id)!=32{return r,errors.New("invalid trigger runtime id")}
 copy(r.DefinitionID[:],id)
 version,ok:=fields[1].(uint64);if !ok||version==0||version>uint64(^uint32(0)){return r,errors.New("invalid trigger runtime version")}
 r.Version=uint32(version)
 opt,ok:=fields[2].([]any);if !ok{return r,errors.New("invalid last-value option")}
 if len(opt)==2&&opt[0]==uint64(1){r.Last,ok=opt[1].(bool);if !ok{return r,errors.New("invalid last predicate")};r.HasLast=true
 }else if len(opt)!=1||opt[0]!=uint64(0){return r,errors.New("invalid last-value option")}
 r.Armed,ok=fields[3].(bool);if !ok{return r,errors.New("invalid armed")}
 r.Exhausted,ok=fields[4].(bool);if !ok{return r,errors.New("invalid exhausted")}
 count,ok:=fields[5].(uint64);if !ok{return r,errors.New("invalid activation count")}
 r.ActivationCount=count
 opt,ok=fields[6].([]any);if !ok{return r,errors.New("invalid repeat option")}
 if len(opt)==2&&opt[0]==uint64(1){at,ok:=asSigned(opt[1]);if !ok{return r,errors.New("invalid next repeat")};r.NextRepeat=domain.SimulationInstant(at);r.HasNextRepeat=true
 }else if len(opt)!=1||opt[0]!=uint64(0){return r,errors.New("invalid repeat option")}
 revision,ok:=fields[7].(uint64);if !ok{return r,errors.New("invalid evaluated revision")}
 r.EvaluatedThrough=domain.WorldRevision(revision)
 canonical,err:=RuntimeBytes(r);if err!=nil{return r,err}
 if string(canonical)!=string(raw){return r,errors.New("noncanonical runtime")}
 return r,nil
}

// RearmPure never writes mutable state. Its causal lifecycle event must be
// materialized through a future WORK CycleCommit, never direct SQLite update.
func RearmPure(d Definition,old Runtime,causingEvent domain.ID)(Runtime,error){
 if old.DefinitionID!=d.ID||old.Version!=d.Version||causingEvent==(domain.ID{}) {
  return Runtime{},errors.New("rearm requires matching definition and explicit causal event")
 }
 if d.Policy!=OnceWhenTrue && d.Policy!=RisingEdge && d.Policy!=FallingEdge {
  return Runtime{},errors.New("rearm is unsupported for this policy")
 }
 next:=old
 next.Armed=true
 next.Exhausted=false
 next.HasLast=false
 next.HasNextRepeat=false
 return next,nil
}

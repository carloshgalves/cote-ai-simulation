package sqlite

import (
 "bytes"
 "context"
 "errors"
 "sort"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/inputledger"
)

// systemUnits is the authoritative scheduling/trigger roster reader. It
// validates terminal rows as well; an altered SQL lifecycle projection must
// never silently remove a unit from the next fence.
func systemUnits(ctx context.Context,q queryOne,run [32]byte,upTo domain.EligibilityCoordinate)([]inputledger.SystemUnit,error){
 selections:=[]struct{kind uint8;table string}{
  {2,"scheduled_occurrences"},{3,"trigger_activations"},
 }
 out:=make([]inputledger.SystemUnit,0)
 for _,selection:=range selections{
  // Table names are fixed literals, never caller-provided SQL identifiers.
  rows,err:=q.QueryContext(ctx,"SELECT id,canonical,lifecycle,eligibility_instant,eligibility_ordinal FROM "+selection.table+" WHERE run_id=?",run[:])
  if err!=nil{return nil,err}
  for rows.Next(){
   var id,raw []byte
   var lifecycle,instant,ordinal int64
   if err:=rows.Scan(&id,&raw,&lifecycle,&instant,&ordinal);err!=nil{rows.Close();return nil,err}
   if len(id)!=32||lifecycle<0||ordinal<0{rows.Close();return nil,errors.New("invalid indexed system-unit identity/lifecycle")}
   var key [32]byte;copy(key[:],id)
   var coord domain.EligibilityCoordinate
   var life uint64
   if selection.kind==2{coord,life,err=validateScheduledOccurrence(run,key,raw)
   }else{coord,life,err=validateTriggerActivation(run,key,raw)}
   if err!=nil{rows.Close();return nil,err}
   if life!=uint64(lifecycle)||int64(coord.NotBeforeInstant)!=instant||
    uint64(coord.NotBeforeCycleOrdinal)!=uint64(ordinal){
    rows.Close();return nil,errors.New("system-unit index differs from signed canonical authority")
   }
   if lifecycle!=0||coord.Compare(upTo)>0{continue}
   decoded,err:=codec.StrictDecodePayload(raw);if err!=nil{rows.Close();return nil,err}
   fields:=decoded.([]any)
   var source string
   var dg []byte
   if selection.kind==2{
    source,err=func()(string,error){v,ok:=fields[2].(string);if !ok||v==""{return "",errors.New("scheduled source missing")};return v,nil}()
    dg,_=fields[14].([]byte)
   }else{
    source,_=fields[2].(string)
    dg,_=fields[11].([]byte)
   }
   if err!=nil||len(dg)!=32{rows.Close();return nil,errors.New("canonical system-unit digest or source missing")}
   var digest domain.Digest;copy(digest[:],dg)
   out=append(out,inputledger.SystemUnit{Kind:selection.kind,Source:source,ID:domain.ID(key),Digest:digest,Eligibility:coord})
  }
  if err:=rows.Err();err!=nil{rows.Close();return nil,err}
  if err:=rows.Close();err!=nil{return nil,err}
 }
 sort.Slice(out,func(i,j int)bool{
  a,b:=out[i],out[j]
  if c:=a.Eligibility.Compare(b.Eligibility);c!=0{return c<0}
  if a.Kind!=b.Kind{return a.Kind<b.Kind}
  if a.Source!=b.Source{return a.Source<b.Source}
  if c:=bytes.Compare(a.Digest[:],b.Digest[:]);c!=0{return c<0}
  return bytes.Compare(a.ID[:],b.ID[:])<0
 })
 for i:=1;i<len(out);i++{if out[i].ID==out[i-1].ID{return nil,errors.New("duplicate system-unit identity")}}
 return out,nil
}

func (s *Store) SystemUnits(ctx context.Context,run [32]byte,upTo domain.EligibilityCoordinate)([]inputledger.SystemUnit,error){
 return systemUnits(ctx,s.db,run,upTo)
}

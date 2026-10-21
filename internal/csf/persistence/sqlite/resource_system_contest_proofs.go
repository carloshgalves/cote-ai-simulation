package sqlite

import (
 "bytes"
 "context"
 "errors"
 "sort"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/ports"
)

// replayContestProofSet selects exactly the six content-addressed
// CSFRV1-4 proofs from one run and one contested cycle. Extraneous ledger
// records are not authority and exact cardinality/rederivation is enforced
// later by verifyTwoResourceTerminal.
func replayContestProofSet(ctx context.Context,q queryOne,run,cycle [32]byte,
 sources []domain.ID)([]ports.CanonicalRecord,error){
 if len(sources)!=2{return nil,errors.New("contest proof source count not two")}
 sourceIDs,candidateIDs:=map[string]bool{},map[string]bool{}
 for _,source:=range sources{
  sourceIDs[string(source[:])]=true
  id,err:=registeredDigest("cote.csf.id.commit-candidate","cote.csf.schema.candidate-id-preimage",
   []any{run[:],cycle[:],[]any{source[:]},"resource.consume",uint64(1),uint64(0),uint64(0)})
  if err!=nil{return nil,err}
  candidateIDs[string(id[:])]=true
 }
 rows,err:=q.QueryContext(ctx,"SELECT id,canonical FROM decision_ledger ORDER BY id")
 if err!=nil{return nil,err}
 out:=make([]ports.CanonicalRecord,0,6)
 for rows.Next(){
  var id,raw []byte
  if err:=rows.Scan(&id,&raw);err!=nil{rows.Close();return nil,err}
  if len(id)!=32{rows.Close();return nil,errors.New("malformed contest replay ledger proof key")}
  v,err:=codec.StrictDecodePayload(raw);if err!=nil{rows.Close();return nil,err}
  fields,ok:=v.([]any);if !ok{continue}
  take:=false
  switch len(fields){
  case 17:
   take=candidateIDs[string(id)]&&equalBytes(fields[1],run[:])&&equalBytes(fields[2],cycle[:])
  case 3:
   subject,ok:=fields[0].([]any)
   if ok&&len(subject)==3&&subject[0]==uint64(5){
    rawID,ok:=subject[1].([]byte)
    take=ok&&sourceIDs[string(rawID)]
   }
  case 7:
   contestants,ok:=fields[1].([]any)
   take=ok&&len(contestants)==2
   if take{
    for _,part:=range contestants{
     key,ok:=part.([]byte)
     if !ok||len(key)!=32||!candidateIDs[string(key)]{take=false;break}
    }
   }
  case 11:
   take=equalBytes(fields[1],run[:])&&equalBytes(fields[2],cycle[:])
  }
  if take{
   var key [32]byte;copy(key[:],id)
   out=append(out,ports.CanonicalRecord{Identity:key,Canonical:raw})
  }
 }
 if err:=rows.Err();err!=nil{rows.Close();return nil,err}
 if err:=rows.Close();err!=nil{return nil,err}
 sort.Slice(out,func(i,j int)bool{return bytes.Compare(out[i].Identity[:],out[j].Identity[:])<0})
 return out,nil
}

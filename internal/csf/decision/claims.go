package decision

import (
 "bytes"
 "encoding/hex"
 "fmt"
 "sort"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
)

// DetectClaims conservatively identifies overlapping resource, read/write and
// invariant claims. This function is pure: absence of a resource capacity
// fails closed, and conflict membership never depends on worker order.
// Resolving non-resource conflicts belongs to a separately pinned policy.
func DetectClaims(d identity.Deriver,cycle domain.ID,candidates []Candidate,
 resources map[string]int64,detector,resolver []any)([]Conflict,error){
 type group struct{kind uint64;key string;participants map[domain.ID]bool}
 groups:=map[string]*group{}
 add:=func(kind uint64,key string,id domain.ID){
  gkey:=fmt.Sprintf("%d/%s",kind,key)
  g:=groups[gkey]
  if g==nil{g=&group{kind:kind,key:key,participants:map[domain.ID]bool{}};groups[gkey]=g}
  g.participants[id]=true
 }
 resourcesSeen:=map[string]int64{}
 resourceHolders:=map[string]map[domain.ID]bool{}
 readHolders:=map[string]map[domain.ID]bool{}
 writeHolders:=map[string]map[domain.ID]bool{}
 candidatesSeen:=map[domain.ID]bool{}
 for _,c:=range candidates{
  if candidatesSeen[c.ID]||len(c.Fields)!=16{return nil,ErrProvenance}
  candidatesSeen[c.ID]=true
  if !sameBytes(c.Fields[0],c.ID[:]){return nil,ErrProvenance}
  expected,e:=digest(d,"cote.csf.digest.commit-candidate","cote.csf.schema.commit-candidate.body",c.Fields)
  if e!=nil||expected!=c.Digest{return nil,ErrProvenance}
  raw,e:=encode(c.Fields,c.Digest);if e!=nil||!bytes.Equal(raw,c.Canonical){return nil,ErrProvenance}
  claims,ok:=c.Fields[9].([]any);if !ok{return nil,ErrProvenance}
  for _,value:=range claims{
   claim,ok:=value.([]any);if !ok||len(claim)!=4{return nil,ErrProvenance}
   resource,ok:=claim[1].(string);if !ok||domain.ValidateMachineID(resource)!=nil{return nil,ErrProvenance}
   mode,ok:=claim[2].(uint64);if !ok||mode!=1{return nil,ErrIndeterminate}
   amount,ok:=asPositiveAmount(claim[3]);if !ok{return nil,ErrProvenance}
   if _,known:=resources[resource];!known{return nil,ErrIndeterminate}
   if resourceHolders[resource]==nil{resourceHolders[resource]=map[domain.ID]bool{}}
   if !resourceHolders[resource][c.ID]{
    resourceHolders[resource][c.ID]=true
   }
   if resourcesSeen[resource]>int64(^uint64(0)>>1)-amount{return nil,ErrIndeterminate}
   resourcesSeen[resource]+=amount
  }
  read,ok:=c.Fields[10].([]any);if !ok{return nil,ErrProvenance}
  write,ok:=c.Fields[11].([]any);if !ok{return nil,ErrProvenance}
  for _,pair:=range []struct{items []any;dst map[string]map[domain.ID]bool}{{read,readHolders},{write,writeHolders}}{
   for _,ref:=range pair.items{
    raw,e:=codec.CanonicalPayload(ref);if e!=nil{return nil,e}
    // Opaque ref bytes are only grouped, not used to derive causal IDs.
    opaque:=identity.OpaqueContentHash(raw)
    label:="claim."+hex.EncodeToString(opaque[:])
    if pair.dst[label]==nil{pair.dst[label]=map[domain.ID]bool{}}
    pair.dst[label][c.ID]=true
   }
  }
  invariants,ok:=c.Fields[12].([]any);if !ok{return nil,ErrProvenance}
  for _,value:=range invariants{
   name,ok:=value.(string);if !ok||domain.ValidateMachineID(name)!=nil{return nil,ErrProvenance}
   add(2,name,c.ID)
  }
 }
 for resource,holders:=range resourceHolders{
  if resources[resource]<0{return nil,ErrIndeterminate}
  if resourcesSeen[resource]>resources[resource]{
   for id:=range holders{add(0,resource,id)}
  }
 }
 for ref,writers:=range writeHolders{
  readers:=readHolders[ref]
  union:=map[domain.ID]bool{}
  for id:=range readers{union[id]=true}
  for id:=range writers{union[id]=true}
  if len(union)<2{continue}
  for id:=range union{add(1,ref,id)}
 }
 result:=make([]Conflict,0)
 for _,g:=range groups{
  if len(g.participants)<2{continue}
  ids:=make([]domain.ID,0,len(g.participants))
  for id:=range g.participants{ids=append(ids,id)}
  sort.Slice(ids,func(i,j int)bool{return canonicalIDLess(ids[i],ids[j])})
  id,e:=derive(d,"cote.csf.id.conflict-set","cote.csf.schema.conflict-set-id-preimage",
   []any{cycle[:],bytesIDs(ids),g.kind,g.key})
  if e!=nil{return nil,e}
  fields:=[]any{id[:],bytesIDs(ids),g.kind,g.key,detector,resolver}
  dg,e:=digest(d,"cote.csf.digest.conflict-set","cote.csf.schema.conflict-set.body",fields)
  if e!=nil{return nil,e}
  raw,e:=encode(fields,dg);if e!=nil{return nil,e}
  result=append(result,Conflict{ID:id,CandidateIDs:ids,Fields:fields,Digest:dg,Canonical:raw})
 }
 sort.Slice(result,func(i,j int)bool{return canonicalIDLess(result[i].ID,result[j].ID)})
 return result,nil
}

func asPositiveAmount(v any)(int64,bool){
 switch x:=v.(type) {
 case int64:return x,x>0
 case uint64:
  if x==0||x>uint64(^uint64(0)>>1){return 0,false}
  return int64(x),true
 }
 return 0,false
}


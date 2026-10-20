package decision

import (
 "errors"
 "fmt"
 "sort"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
)

func Run(input Input)(Result,error){
 result:=Result{PostAvailable:input.Snapshot.Available}
 if input.Bundle==nil{return result,errors.New("canonical bundle missing")}
 if input.RunID==(domain.ID{})||input.CycleID==(domain.ID{})||input.Snapshot.Available<0 {
  return result,fmt.Errorf("%w: invalid run/cycle/resource state",ErrProvenance)
 }
 if domain.ValidateMachineID(input.Snapshot.ResourceKind)!=nil||domain.ValidateMachineID(input.Snapshot.ResourceID)!=nil{
  return result,fmt.Errorf("%w: malformed resource identity",ErrProvenance)
 }
 refs,e:=policyRefs(input.Bundle);if e!=nil{return result,e}
 d:=identity.New(input.Bundle)
 byID:=map[domain.ID]prepared{}
 for _,unit:=range input.Units{
  p,err:=validateResponse(d,input,unit);if err!=nil{return result,err}
  if _,duplicate:=byID[p.id];duplicate{return result,fmt.Errorf("%w: duplicate delivered unit",ErrProvenance)}
  byID[p.id]=p
 }
 ordered:=make([]prepared,0,len(input.Admitted))
 seen:=map[domain.ID]bool{}
 for _,id:=range input.Admitted{
  if seen[id]{return result,fmt.Errorf("%w: duplicated fence unit",ErrProvenance)}
  seen[id]=true
  p,exists:=byID[id]
  if !exists{return result,fmt.Errorf("%w: fenced unit has no response",ErrProvenance)}
  ordered=append(ordered,p)
 }
 if len(ordered)!=len(byID){return result,fmt.Errorf("%w: unfenced response",ErrProvenance)}
 ordered,e=resolveAliases(d,ordered,input.History,refs["idempotency"])
 if e!=nil {
  if errors.Is(e,ErrIdempotency){result.Failure="IDEMPOTENCY_CONFLICT";seal(&result,input);return result,e}
  return result,e
 }
 noProposals,aliases:=map[domain.ID]bool{},map[domain.ID]bool{}
 blocked:=map[domain.ID]bool{}
 current:=map[domain.ID]int64{}
 for _,p:=range ordered{
  if p.noProposal{noProposals[p.id]=true;continue}
  if p.aliasFound{aliases[p.id]=true;continue}
  if p.indeterminate||!input.Snapshot.Known {
   result.Indeterminate=append(result.Indeterminate,Indeterminate{
    Subject:p.id,Facet:2,Validator:refs["validators"],Reason:"resource.validator.missing",Evidence:[]any{},
   })
   continue
  }
  p.blocked=p.amount>input.Snapshot.Available
  actor:=p.unit.Response.Actor[0]+":"+p.unit.Response.Actor[1]
  assessment,err:=assess(d,p,true,p.blocked,input.Snapshot.ForbiddenActors[actor],refs["validators"])
  if err!=nil{return result,err}
  result.Assessments=append(result.Assessments,assessment)
  blocked[p.id]=p.blocked
  // Candidate provenance resolves the authentic ActionProposal unit root.
  source:=[]any{uint64(5),p.id[:],p.unit.Response.Digest[:]}
  claims:=p.unit.Response.Body[9].([]any)
  candidate,err:=MakeCandidate(d,input.RunID,input.CycleID,[]domain.ID{p.id},
   "resource.consume",1,0,0,claims,[]any{source},0,refs["conflict-resolvers"])
  if err!=nil{return result,err}
  result.Candidates=append(result.Candidates,candidate)
  current[candidate.ID]=p.amount
 }
 if len(result.Indeterminate)>0 {
  result.Failure="INDETERMINATE_FACET"
  seal(&result,input)
  return result,ErrIndeterminate
 }
 if err:=CheckPartition(input.Admitted,noProposals,aliases,result.Candidates);err!=nil{
  result.Failure="PROVENANCE_FAILURE";seal(&result,input);return result,err
 }
 // Ignore losers rejected individually by the affordability facet; they
 // cannot participate in a resource conflict and receive REJECT later.
 viable:=make([]Candidate,0,len(result.Candidates))
 for _,c:=range result.Candidates{if !blocked[c.SourceUnitIDs[0]]{viable=append(viable,c)}}
 sort.Slice(viable,func(i,j int)bool{return canonicalIDLess(viable[i].ID,viable[j].ID)})
 winners:=map[domain.ID]bool{}
 var total int64
 for _,c:=range viable{
  qty:=current[c.ID]
  if qty<=0||qty>input.Snapshot.Available{return result,ErrProvenance}
  if total>input.Snapshot.Available-qty{total=input.Snapshot.Available+1;break}
  total+=qty
 }
 if total<=input.Snapshot.Available {
  for _,c:=range viable {winners[c.ID]=true}
 } else {
  if len(viable)!=2 {
   for _,c:=range viable{result.Indeterminate=append(result.Indeterminate,Indeterminate{
    Subject:c.SourceUnitIDs[0],Facet:2,Validator:refs["conflict-resolvers"],Reason:"resource.resolver.unavailable",Evidence:[]any{},
   })}
   result.Failure="INDETERMINATE_FACET";seal(&result,input);return result,ErrIndeterminate
  }
  // This V0 resolver supports exactly two claims for one last unit.
  if current[viable[0].ID]!=input.Snapshot.Available||current[viable[1].ID]!=input.Snapshot.Available{
   return result,fmt.Errorf("%w: unsupported resource allocation policy",ErrIndeterminate)
  }
  ids:=[]domain.ID{viable[0].ID,viable[1].ID}
  id,e:=derive(d,"cote.csf.id.conflict-set","cote.csf.schema.conflict-set-id-preimage",
   []any{input.CycleID[:],bytesIDs(ids),uint64(0),input.Snapshot.ResourceID})
  if e!=nil{return result,e}
  fields:=[]any{id[:],bytesIDs(ids),uint64(0),input.Snapshot.ResourceID,refs["validators"],refs["conflict-resolvers"]}
  dg,e:=digest(d,"cote.csf.digest.conflict-set","cote.csf.schema.conflict-set.body",fields)
  if e!=nil{return result,e}
  raw,e:=encode(fields,dg);if e!=nil{return result,e}
  result.Conflicts=append(result.Conflicts,Conflict{ID:id,CandidateIDs:ids,Fields:fields,Digest:dg,Canonical:raw})
  draw,index,e:=namedDraw(d,input,ids,refs["rng"]);if e!=nil{return result,e}
  result.Draws=append(result.Draws,draw)
  winners[ids[index]]=true
 }
 // All resulting dispositions are still provisional. No lifecycle output,
 // DecisionRecord, Event or WorldRevision may be inferred from them here.
 bySource:=map[domain.ID]Candidate{}
 for _,c:=range result.Candidates{for _,id:=range c.SourceUnitIDs{bySource[id]=c}}
 for _,p:=range ordered{
  disposition:=Disposition{Subject:p.id}
  switch {
  case p.noProposal:disposition.Status="NO_PROPOSAL";disposition.Reason="slot.no-proposal"
  case p.aliasFound:disposition.Status="DEDUPLICATED";disposition.Reason="idempotency.alias";disposition.CanonicalUnit=p.alias
  default:
   c,ok:=bySource[p.id];if !ok{return result,ErrProvenance}
   disposition.CandidateID=c.ID
   switch{
   case blocked[p.id]:disposition.Status="REJECT";disposition.Reason="resource.insufficient"
   case winners[c.ID]:
    disposition.Status="COMMIT";disposition.Reason="resource.allocated"
    if current[c.ID]>result.PostAvailable{return result,ErrProvenance}
    result.PostAvailable-=current[c.ID]
   default:disposition.Status="REJECT";disposition.Reason="resource.conflict"
   }
  }
  result.Dispositions=append(result.Dispositions,disposition)
 }
 if result.PostAvailable<0{return result,ErrProvenance}
 if err:=seal(&result,input);err!=nil{return result,err}
 return result,nil
}

// Explicitly non-authoritative audit data. It has no registered causal
// identity: CSFRV1-5 must revalidate and derive its own terminal records.
func seal(r *Result,input Input)error{
 a:=make([]any,len(r.Assessments))
 for i,v:=range r.Assessments{a[i]=v.Canonical}
 c:=make([]any,len(r.Candidates))
 for i,v:=range r.Candidates{c[i]=v.Canonical}
 conflicts:=make([]any,len(r.Conflicts))
 for i,v:=range r.Conflicts{conflicts[i]=v.Canonical}
 draws:=make([]any,len(r.Draws))
 for i,v:=range r.Draws{draws[i]=v.Canonical}
 ind:=make([]any,len(r.Indeterminate))
 for i,v:=range r.Indeterminate{ind[i]=[]any{v.Subject[:],uint64(v.Facet),v.Validator,v.Reason,v.Evidence}}
 disp:=make([]any,len(r.Dispositions))
 for i,v:=range r.Dispositions{disp[i]=[]any{v.Subject[:],v.CandidateID[:],v.Status,v.Reason,v.CanonicalUnit[:]}}
 raw,e:=codec.CanonicalPayload([]any{"csfrv1-4.provisional",input.RunID[:],input.CycleID[:],
  uint64(input.Attempt),uint64(input.Snapshot.BaseRevision),input.Snapshot.ResourceKind,
  input.Snapshot.ResourceID,input.Snapshot.Available,a,c,conflicts,draws,ind,disp,r.PostAvailable,r.Failure})
 if e==nil{r.Canonical=raw}
 return e
}

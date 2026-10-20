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
 seenSchemas:=map[string]bool{}
 for _,schema:=range input.Schemas{
  for _,s:=range []string{schema.Type,schema.Operation,schema.Producer}{
   if domain.ValidateMachineID(s)!=nil{return result,fmt.Errorf("%w: malformed action registry entry",ErrProvenance)}
  }
  if schema.Version==0{return result,fmt.Errorf("%w: zero schema version",ErrProvenance)}
  key:=fmt.Sprintf("%s/%d",schema.Type,schema.Version)
  if seenSchemas[key]{return result,fmt.Errorf("%w: duplicate action schema registration",ErrProvenance)}
  seenSchemas[key]=true
 }
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
  if errors.Is(e,ErrIdempotency){result.Failure="IDEMPOTENCY_CONFLICT";if se:=seal(&result,input);se!=nil{return result,se};return result,e}
  return result,e
 }
 noProposals,aliases:=map[domain.ID]bool{},map[domain.ID]bool{}
 blocked:=map[domain.ID]bool{}
 accessBlocked:=map[domain.ID]bool{}
 current:=map[domain.ID]int64{}
 for _,p:=range ordered{
  if p.noProposal{noProposals[p.id]=true;continue}
  if p.aliasFound{aliases[p.id]=true;continue}
  if p.indeterminate||!input.Snapshot.Known {
   result.Indeterminate=append(result.Indeterminate,Indeterminate{
    Subject:p.id,SubjectDigest:p.unit.Response.Digest,SubjectKind:0,Facet:2,Validator:refs["validators"],Reason:"resource.validator.missing",Evidence:[]any{},
   })
   continue
  }
  actor:=p.unit.Response.Actor[0]+":"+p.unit.Response.Actor[1]
  canAccess,accessKnown:=input.Snapshot.AccessibleActors[actor]
  forbidden,institutionKnown:=input.Snapshot.ForbiddenActors[actor]
  if !accessKnown||!institutionKnown{
   facet,reason:=uint8(1),"access.validator.missing"
   if accessKnown{facet=4;reason="institution.validator.missing"}
   result.Indeterminate=append(result.Indeterminate,Indeterminate{
    Subject:p.id,SubjectDigest:p.unit.Response.Digest,SubjectKind:0,
    Facet:facet,Validator:refs["validators"],Reason:reason,Evidence:[]any{},
   })
   continue
  }
  resourceBlocked:=p.amount>input.Snapshot.Available
  assessment,err:=assess(d,p,resourceBlocked,canAccess,forbidden,refs["validators"])
  if err!=nil{return result,err}
  result.Assessments=append(result.Assessments,assessment)
  blocked[p.id]=resourceBlocked||!canAccess
  accessBlocked[p.id]=!canAccess
  // Candidate provenance resolves the authentic ActionProposal unit root.
  source:=[]any{uint64(5),p.id[:],p.unit.Response.Digest[:]}
  claims:=p.unit.Response.Body[9].([]any)
  candidate,err:=MakeCandidate(d,input.RunID,input.CycleID,[]domain.ID{p.id},
   p.producer,1,0,0,claims,[]any{source},0,refs["conflict-resolvers"])
  if err!=nil{return result,err}
  result.Candidates=append(result.Candidates,candidate)
  current[candidate.ID]=p.amount
 }
 if len(result.Indeterminate)>0 {
  result.Failure="INDETERMINATE_FACET"
  if err:=seal(&result,input);err!=nil{return result,err}
  return result,ErrIndeterminate
 }
 if err:=CheckPartition(input.Admitted,noProposals,aliases,result.Candidates);err!=nil{
  result.Failure="PROVENANCE_FAILURE";if se:=seal(&result,input);se!=nil{return result,se};return result,err
 }
 // Ignore losers rejected individually by the affordability facet; they
 // cannot participate in a resource conflict and receive REJECT later.
 viable:=make([]Candidate,0,len(result.Candidates))
 for _,c:=range result.Candidates{if !blocked[c.SourceUnitIDs[0]]{viable=append(viable,c)}}
 sort.Slice(viable,func(i,j int)bool{return canonicalIDLess(viable[i].ID,viable[j].ID)})
 winners:=map[domain.ID]bool{}
 var total int64
 exhausted:=false
 for _,c:=range viable{
  qty:=current[c.ID]
  if qty<=0||qty>input.Snapshot.Available{return result,ErrProvenance}
  if total>input.Snapshot.Available-qty{exhausted=true;break}
  total+=qty
 }
 // The resolver must consume the conservative detector's authenticated
 // candidate claims rather than reconstructing a separate conflict model.
 detected,e:=DetectClaims(d,input.CycleID,viable,
  map[string]int64{input.Snapshot.ResourceID:input.Snapshot.Available},
  refs["validators"],refs["conflict-resolvers"])
 if e!=nil{
  if errors.Is(e,ErrIndeterminate){
   for _,c:=range viable{
    result.Indeterminate=append(result.Indeterminate,Indeterminate{
     Subject:c.ID,SubjectDigest:c.Digest,SubjectKind:1,Facet:2,
     Validator:refs["validators"],Reason:"resource.detector.indeterminate",Evidence:[]any{},
    })
   }
   result.Failure="INDETERMINATE_FACET"
   if err:=seal(&result,input);err!=nil{return result,err}
  }
  return result,e
 }
 var resourceConflict *Conflict
 unsupported:=map[domain.ID]bool{}
 for _,c:=range detected{
  kind,key:=c.Fields[2],c.Fields[3]
  if kind==uint64(2)&&key=="resource.nonnegative"{
   // The resource capacity/quantity check already discharges this exact
   // invariant. It must not create a second lottery for the same stock.
   continue
  }
  if kind==uint64(0)&&key==input.Snapshot.ResourceID{
   if resourceConflict!=nil{return result,ErrProvenance}
   copied:=c
   resourceConflict=&copied
   continue
  }
  result.Conflicts=append(result.Conflicts,c)
  for _,id:=range c.CandidateIDs{unsupported[id]=true}
 }
 if len(unsupported)>0{
  if resourceConflict!=nil{result.Conflicts=append(result.Conflicts,*resourceConflict)}
  sort.Slice(result.Conflicts,func(i,j int)bool{
   return canonicalIDLess(result.Conflicts[i].ID,result.Conflicts[j].ID)
  })
  for _,c:=range viable{
   if unsupported[c.ID]{
    result.Indeterminate=append(result.Indeterminate,Indeterminate{
     Subject:c.ID,SubjectDigest:c.Digest,SubjectKind:1,Facet:2,
     Validator:refs["conflict-resolvers"],
     Reason:"resource.resolver.unsupported-claims",Evidence:[]any{},
    })
   }
  }
  result.Failure="INDETERMINATE_FACET"
  if err:=seal(&result,input);err!=nil{return result,err}
  return result,ErrIndeterminate
 }
 if (resourceConflict==nil)!=(!exhausted){
  return result,fmt.Errorf("%w: detector and resource capacity disagree",ErrProvenance)
 }
 if !exhausted{
  for _,c:=range viable{winners[c.ID]=true}
 }else{
  // V0 resolves only two full-capacity consumers, never a generic allocation.
  if len(viable)!=2||len(resourceConflict.CandidateIDs)!=2{
   for _,c:=range viable{result.Indeterminate=append(result.Indeterminate,Indeterminate{
    Subject:c.ID,SubjectDigest:c.Digest,SubjectKind:1,Facet:2,
    Validator:refs["conflict-resolvers"],Reason:"resource.resolver.unavailable",Evidence:[]any{},
   })}
   result.Failure="INDETERMINATE_FACET"
   if err:=seal(&result,input);err!=nil{return result,err}
   return result,ErrIndeterminate
  }
  if current[viable[0].ID]!=input.Snapshot.Available||current[viable[1].ID]!=input.Snapshot.Available{
   for _,c:=range viable{result.Indeterminate=append(result.Indeterminate,Indeterminate{
    Subject:c.ID,SubjectDigest:c.Digest,SubjectKind:1,Facet:2,
    Validator:refs["conflict-resolvers"],Reason:"resource.resolver.unsupported",Evidence:[]any{},
   })}
   result.Failure="INDETERMINATE_FACET"
   if err:=seal(&result,input);err!=nil{return result,err}
   return result,ErrIndeterminate
  }
  result.Conflicts=append(result.Conflicts,*resourceConflict)
  draw,index,e:=namedDraw(d,input,resourceConflict.CandidateIDs,refs["rng"])
  if e!=nil{return result,e}
  result.Draws=append(result.Draws,draw)
  winners[resourceConflict.CandidateIDs[index]]=true
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
   case accessBlocked[p.id]:disposition.Status="REJECT";disposition.Reason="access.denied"
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
 for i,v:=range r.Indeterminate{
  if len(v.Validator)!=3{return fmt.Errorf("%w: incomplete validator policy ref",ErrProvenance)}
  ind[i]=[]any{
   []any{uint64(v.SubjectKind),v.Subject[:],v.SubjectDigest[:]},
   uint64(v.Facet),v.Validator[0],v.Validator[1],v.Validator[2],
   v.Reason,v.Evidence,v.Validator,
  }
 }
 disp:=make([]any,len(r.Dispositions))
 for i,v:=range r.Dispositions{disp[i]=[]any{v.Subject[:],v.CandidateID[:],v.Status,v.Reason,v.CanonicalUnit[:]}}
 raw,e:=codec.CanonicalPayload([]any{"csfrv1-4.provisional",input.RunID[:],input.CycleID[:],
  uint64(input.Attempt),uint64(input.Snapshot.BaseRevision),input.Snapshot.ResourceKind,
  input.Snapshot.ResourceID,input.Snapshot.Available,a,c,conflicts,draws,ind,disp,r.PostAvailable,r.Failure})
 if e==nil{r.Canonical=raw}
 return e
}

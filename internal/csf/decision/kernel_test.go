package decision

import (
 "bytes"
 "errors"
 "os"
 "path/filepath"
 "testing"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/inputledger"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

func fixture(t *testing.T,known bool)(Input,inputledger.Response,inputledger.Response){
 t.Helper()
 bundle,e:=policy.Load("../../../docs/architecture/canonical-codec-v1-bundle")
 if e!=nil{t.Fatal(e)}
 var run,cycle,seed domain.ID
 run[0]=17;cycle[0]=18;seed[0]=19
 makeResp:=func(actor,slot string)inputledger.Response{
  var dispatch,round domain.ID
  dispatch[0]=actor[0];round[0]=actor[0]+1
  d:=identity.New(bundle)
  id,_,e:=d.Sum("cote.csf.id.action-proposal","cote.csf.schema.slot-response-id-preimage",1,[]any{dispatch[:],uint64(0)})
  if e!=nil{t.Fatal(e)}
  v,e:=codec.CanonicalBytes("cote.csf.testresource.params","cote.csf.schema.typed-value",1,[]any{})
  if e!=nil{t.Fatal(e)}
  h:=identity.OpaqueContentHash(v)
  fields:=[]any{id[:],dispatch[:],run[:],cycle[:],"resource.consume",uint64(1),[]any{"school",actor},[]any{},
   []any{"cote.csf.schema.typed-value",uint64(1),v,h[:]},
   []any{[]any{"inventory","last.resource",uint64(1),int64(1)}},[]any{uint64(0)},int64(100),[]any{uint64(0)},
   round[:],slot,uint64(7),[]any{uint64(0)},[]any{uint64(0)}}
  dg,_,e:=d.Sum("cote.csf.digest.action-proposal-unit","cote.csf.schema.action-proposal.body",1,[]any{fields})
  if e!=nil{t.Fatal(e)}
  raw,e:=codec.CanonicalPayload(append(append([]any{},fields...),dg[:]))
  if e!=nil{t.Fatal(e)}
  return inputledger.Response{Kind:0,ID:domain.ID(id),DispatchID:dispatch,RunID:run,CycleID:cycle,
   RoundID:round,SlotID:slot,Actor:[2]string{"school",actor},EffectiveAt:100,SubmittedAgainstRevision:7,
   Body:fields,Digest:dg,Canonical:raw}
 }
 a,b:=makeResp("a","one"),makeResp("b","two")
 return Input{RunID:run,CycleID:cycle,Attempt:0,Bundle:bundle,
  Seed:domain.Digest(seed),Schemas:[]ActionSchema{{Type:"resource.consume",Version:1,Operation:"resource.consume",Producer:"resource.consume"}},Snapshot:Snapshot{BaseRevision:7,ResourceKind:"inventory",ResourceID:"last.resource",Available:1,Known:known,
  AccessibleActors:map[string]bool{"school:a":true,"school:b":true},
  ForbiddenActors:map[string]bool{"school:a":true,"school:b":false}},
  Admitted:[]domain.ID{a.ID,b.ID},Units:[]Unit{{Response:a,ProducerScope:"exam"},{Response:b,ProducerScope:"exam"}}},a,b
}

func TestLastResourceStableAcrossWorkerPermutation(t *testing.T){
 in,a,b:=fixture(t,true)
 baseline,e:=Run(in);if e!=nil{t.Fatal(e)}
 if len(baseline.Candidates)!=2||len(baseline.Conflicts)!=1||len(baseline.Draws)!=1||len(baseline.Assessments)!=2{
  t.Fatalf("incomplete candidate/conflict/draw evidence: %+v",baseline)
 }
 commits:=0
 for _,o:=range baseline.Dispositions{if o.Status=="COMMIT"{commits++}}
 if commits!=1||baseline.PostAvailable!=0||in.Snapshot.Available!=1{t.Fatal("one last unit was not conserved")}
 // Institutional prohibition records a violation without simulating a
 // physical force or ruling out actor A.
 fac,ok:=baseline.Assessments[0].Fields[1].([]any)
 if !ok||len(fac)!=6{t.Fatal("missing typed prohibition facet")}
 prohibited:=fac[4].([]any)
 if prohibited[0]!=uint64(4)||prohibited[1]!=uint64(1)||prohibited[2]!=uint64(1){t.Fatal("institutional prohibition incorrectly blocks physical possibility")}
 in.Units[0],in.Units[1]=in.Units[1],in.Units[0]
 permuted,e:=Run(in);if e!=nil{t.Fatal(e)}
 if !bytes.Equal(baseline.Canonical,permuted.Canonical){t.Fatal("worker completion order changed candidate/conflict/RNG/dispositions bytes")}
 // The same draw key is replayable with a different unrelated named draw.
 refs,e:=policyRefs(in.Bundle);if e!=nil{t.Fatal(e)}
 d:=identity.New(in.Bundle)
 ids:=baseline.Conflicts[0].CandidateIDs
 rawA,_,e:=namedDraw(d,in,ids,refs["rng"]);if e!=nil{t.Fatal(e)}
 if !bytes.Equal(rawA.Canonical,baseline.Draws[0].Canonical){t.Fatal("replay changed named RNG result")}
 _,_,_=a,b,refs
}

func TestIndeterminateRetainsEachSubjectAndNoDisposition(t *testing.T){
 in,a,b:=fixture(t,false)
 out,e:=Run(in)
 if !errors.Is(e,ErrIndeterminate)||out.Failure!="INDETERMINATE_FACET"||len(out.Indeterminate)!=2||len(out.Dispositions)!=0{
  t.Fatalf("missing fail-closed per-subject audit: %v %+v",e,out)
 }
 if out.Indeterminate[0].Subject!=a.ID||out.Indeterminate[1].Subject!=b.ID||out.PostAvailable!=1||len(out.Canonical)==0{
  t.Fatal("indeterminate conflated subjects or mutated world")
 }
}

func TestPartitionRejectsGapOverlapAndExternalUnit(t *testing.T){
 in,a,b:=fixture(t,true)
 d:=identity.New(in.Bundle)
 refs,e:=policyRefs(in.Bundle);if e!=nil{t.Fatal(e)}
 mk:=func(ids ...domain.ID) Candidate{
  c,e:=MakeCandidate(d,in.RunID,in.CycleID,ids,"resource.consume",1,0,0,
   a.Body[9].([]any),[]any{[]any{uint64(5),a.ID[:],a.Digest[:]}},0,refs["conflict-resolvers"])
  if e!=nil{t.Fatal(e)}
  return c
 }
 for _,items:=range [][]Candidate{
  {mk(a.ID)}, // gap
  {mk(a.ID),mk(a.ID),mk(b.ID)}, // overlap
  {mk(a.ID),mk(b.ID),mk(domain.ID{0xfa})}, // external
 }{
  if !errors.Is(CheckPartition(in.Admitted,map[domain.ID]bool{},map[domain.ID]bool{},items),ErrProvenance){
   t.Fatal("invalid partition was accepted")
  }
 }
 if err:=CheckPartition(in.Admitted,map[domain.ID]bool{},map[domain.ID]bool{},[]Candidate{mk(a.ID),mk(b.ID)});err!=nil{t.Fatal(err)}
}

func TestIdempotentAliasWithoutRemovingDelivery(t *testing.T){
 in,a,b:=fixture(t,true)
 b.Body[6]=[]any{"school","a"}
 b.Actor=[2]string{"school","a"}
 a.Body[17]=[]any{uint64(1),"same-logical-operation"}
 b.Body[17]=[]any{uint64(1),"same-logical-operation"}
 d:=identity.New(in.Bundle)
 rebuild:=func(x *inputledger.Response){
  dg,_,e:=d.Sum("cote.csf.digest.action-proposal-unit","cote.csf.schema.action-proposal.body",1,[]any{x.Body});if e!=nil{t.Fatal(e)}
  x.Digest=dg
  x.Canonical,e=codec.CanonicalPayload(append(append([]any{},x.Body...),dg[:]));if e!=nil{t.Fatal(e)}
 }
 rebuild(&a);rebuild(&b)
 in.Units=[]Unit{{Response:b,ProducerScope:"exam"},{Response:a,ProducerScope:"exam"}}
 baseline,e:=Run(in);if e!=nil{t.Fatal(e)}
 if len(baseline.Candidates)!=1||len(baseline.Dispositions)!=2||baseline.Dispositions[1].Status!="DEDUPLICATED"||
 baseline.Dispositions[1].CanonicalUnit!=a.ID {t.Fatalf("alias was not retained and resolved: %+v",baseline.Dispositions)}
 in.Units[0],in.Units[1]=in.Units[1],in.Units[0]
 again,e:=Run(in);if e!=nil||!bytes.Equal(baseline.Canonical,again.Canonical){t.Fatalf("alias depends on worker order: %v",e)}
 b.Body[9]=[]any{[]any{"inventory","last.resource",uint64(1),int64(2)}};rebuild(&b)
 in.Units=[]Unit{{Response:a,ProducerScope:"exam"},{Response:b,ProducerScope:"exam"}}
 out,e:=Run(in)
 if !errors.Is(e,ErrIdempotency)||out.Failure!="IDEMPOTENCY_CONFLICT"||len(out.Dispositions)!=0{t.Fatal("logical idempotency collision was not fail-closed")}
}

func TestAuditBytesPersistAndReopenWithoutWorldMutation(t *testing.T){
 in,_,_:=fixture(t,true)
 out,e:=Run(in);if e!=nil{t.Fatal(e)}
 path:=filepath.Join(t.TempDir(),"provisional.cbor")
 if e:=os.WriteFile(path,out.Canonical,0600);e!=nil{t.Fatal(e)}
 again,e:=os.ReadFile(path);if e!=nil{t.Fatal(e)}
 v,e:=codec.StrictDecodePayload(again);if e!=nil{t.Fatal(e)}
 fields:=v.([]any)
 if fields[0]!="csfrv1-4.provisional"||!bytes.Equal(again,out.Canonical)||
 fields[len(fields)-2]!=uint64(0){t.Fatal("durable provisional audit corrupted")}
}

func TestNoProposalSettlesOnlyProvisionallyWithoutCandidate(t *testing.T){
 in,a,b:=fixture(t,true)
 d:=identity.New(in.Bundle)
 id,_,e:=d.Sum("cote.csf.id.no-proposal","cote.csf.schema.slot-response-id-preimage",1,[]any{a.DispatchID[:],uint64(1)})
 if e!=nil{t.Fatal(e)}
 body:=[]any{id[:],a.DispatchID[:],in.RunID[:],in.CycleID[:],a.RoundID[:],a.SlotID,
  []any{"school","a"},int64(a.EffectiveAt),uint64(a.SubmittedAgainstRevision),uint64(0)}
 dg,_,e:=d.Sum("cote.csf.digest.no-proposal-unit","cote.csf.schema.no-proposal.body",1,[]any{body})
 if e!=nil{t.Fatal(e)}
 raw,e:=codec.CanonicalPayload(append(append([]any{},body...),dg[:]))
 if e!=nil{t.Fatal(e)}
 no:=inputledger.Response{Kind:1,ID:domain.ID(id),DispatchID:a.DispatchID,RunID:a.RunID,CycleID:a.CycleID,RoundID:a.RoundID,
  SlotID:a.SlotID,Actor:a.Actor,EffectiveAt:a.EffectiveAt,SubmittedAgainstRevision:a.SubmittedAgainstRevision,
  Body:body,Digest:dg,Canonical:raw}
 in.Admitted=[]domain.ID{no.ID,b.ID}
 in.Units=[]Unit{{Response:b,ProducerScope:"exam"},{Response:no,ProducerScope:"exam"}}
 out,e:=Run(in);if e!=nil{t.Fatal(e)}
 if len(out.Candidates)!=1||len(out.Conflicts)!=0||len(out.Draws)!=0||len(out.Dispositions)!=2||
  out.Dispositions[0].Status!="NO_PROPOSAL"||out.Dispositions[1].Status!="COMMIT"||out.PostAvailable!=0{
  t.Fatalf("NoProposal got an illegitimate candidate or consumed two units: %+v",out)
 }
 for _,bad:=range []any{uint64(3),"not-a-reason"}{
  corrupted:=no
  corrupted.Body=append([]any(nil),no.Body...)
  corrupted.Body[9]=bad
  badDigest,_,err:=d.Sum("cote.csf.digest.no-proposal-unit",
   "cote.csf.schema.no-proposal.body",1,[]any{corrupted.Body})
  if err!=nil{t.Fatal(err)}
  corrupted.Digest=badDigest
  corrupted.Canonical,err=codec.CanonicalPayload(
   append(append([]any(nil),corrupted.Body...),badDigest[:]))
  if err!=nil{t.Fatal(err)}
  in.Units=[]Unit{{Response:b,ProducerScope:"exam"},{Response:corrupted,ProducerScope:"exam"}}
  if _,err:=Run(in);!errors.Is(err,ErrProvenance){
   t.Fatalf("unknown/retyped no-proposal reason %v accepted: %v",bad,err)
  }
 }
}

func TestHistoricalAliasKeepsIndependentSourceUnit(t *testing.T){
 in,a,b:=fixture(t,true)
 b.Body[6]=[]any{"school","a"}
 b.Actor=a.Actor
 a.Body[17]=[]any{uint64(1),"same-logical-operation"}
 b.Body[17]=[]any{uint64(1),"same-logical-operation"}
 d:=identity.New(in.Bundle)
 for _,resp:=range []*inputledger.Response{&a,&b}{
  h,_,e:=d.Sum("cote.csf.digest.action-proposal-unit","cote.csf.schema.action-proposal.body",1,[]any{resp.Body})
  if e!=nil{t.Fatal(e)}
  resp.Digest=h
  resp.Canonical,e=codec.CanonicalPayload(append(append([]any{},resp.Body...),h[:]))
  if e!=nil{t.Fatal(e)}
 }
 tmp:=in
 tmp.Units=[]Unit{{Response:a,ProducerScope:"exam"}}
 tmp.Admitted=[]domain.ID{a.ID}
 unitPrep,e:=validateResponse(d,tmp,tmp.Units[0]);if e!=nil{t.Fatal(e)}
 refs,e:=policyRefs(in.Bundle);if e!=nil{t.Fatal(e)}
 candidates,e:=resolveAliases(d,[]prepared{unitPrep},nil,refs["idempotency"])
 if e!=nil{t.Fatal(e)}
 binding:=HistoryBinding{Namespace:candidates[0].ns,LogicalDigest:candidates[0].logical,CanonicalUnit:a.ID}
 in.Admitted=[]domain.ID{b.ID}
 in.Units=[]Unit{{Response:b,ProducerScope:"exam"}}
 in.History=[]HistoryBinding{binding}
 out,e:=Run(in);if e!=nil{t.Fatal(e)}
 if len(out.Candidates)!=0||len(out.Dispositions)!=1||out.Dispositions[0].Status!="DEDUPLICATED"||
 out.Dispositions[0].Subject!=b.ID||out.Dispositions[0].CanonicalUnit!=a.ID||out.PostAvailable!=1{
  t.Fatalf("historical alias was pre-consumed or settled as new action: %+v",out)
 }
}

func TestUnregisteredActionCannotSilentlyBecomeSatisfied(t *testing.T){
 in,a,_:=fixture(t,true)
 a.Body[4]="unknown.action"
 d:=identity.New(in.Bundle)
 h,_,e:=d.Sum("cote.csf.digest.action-proposal-unit","cote.csf.schema.action-proposal.body",1,[]any{a.Body})
 if e!=nil{t.Fatal(e)}
 a.Digest=h
 a.Canonical,e=codec.CanonicalPayload(append(append([]any{},a.Body...),h[:]))
 if e!=nil{t.Fatal(e)}
 in.Admitted=[]domain.ID{a.ID};in.Units=[]Unit{{Response:a,ProducerScope:"exam"}}
 out,e:=Run(in)
 if !errors.Is(e,ErrIndeterminate)||len(out.Indeterminate)!=1||out.Indeterminate[0].Subject!=a.ID||
 len(out.Dispositions)!=0||out.PostAvailable!=1{t.Fatal("unregistered handler granted action by default")}
}

func TestRegistryPermutationAndIrrelevantHandlerDoNotChangeDraws(t *testing.T){
 in,_,_:=fixture(t,true)
 before,e:=Run(in);if e!=nil{t.Fatal(e)}
 in.Schemas=append(in.Schemas,ActionSchema{Type:"irrelevant.action",Version:1,Operation:"not.available",Producer:"other.handler"})
 after,e:=Run(in);if e!=nil||!bytes.Equal(before.Canonical,after.Canonical){
  t.Fatalf("irrelevant registered handler changed causal decision: %v",e)
 }
 in.Schemas[0],in.Schemas[1]=in.Schemas[1],in.Schemas[0]
 again,e:=Run(in);if e!=nil||!bytes.Equal(before.Canonical,again.Canonical){t.Fatalf("registration order changed policy: %v",e)}
 in.Schemas=append(in.Schemas,in.Schemas[1])
 if _,e:=Run(in);!errors.Is(e,ErrProvenance){t.Fatal("duplicate action schema silently replaced the registered handler")}
}

func TestResolverUsesConservativeResourceConflictIdentity(t *testing.T){
 in,_,_:=fixture(t,true)
 result,err:=Run(in);if err!=nil{t.Fatal(err)}
 if len(result.Conflicts)!=1||len(result.Draws)!=1{t.Fatal("expected one resource conflict and one draw")}
 refs,err:=policyRefs(in.Bundle);if err!=nil{t.Fatal(err)}
 d:=identity.New(in.Bundle)
 detected,err:=DetectClaims(d,in.CycleID,result.Candidates,
  map[string]int64{in.Snapshot.ResourceID:in.Snapshot.Available},
  refs["validators"],refs["conflict-resolvers"])
 if err!=nil{t.Fatal(err)}
 resource:=0
 for _,conflict:=range detected{
  if conflict.Fields[2]==uint64(0)&&conflict.Fields[3]==in.Snapshot.ResourceID{
   resource++
   if conflict.ID!=result.Conflicts[0].ID||
    !bytes.Equal(conflict.Canonical,result.Conflicts[0].Canonical){
    t.Fatal("resolver substituted a conflict not emitted by the claim detector")
   }
  }
 }
 if resource!=1{t.Fatalf("detector must expose exactly one resource conflict; got %d",resource)}
}

func TestSharedNonnegativeInvariantDoesNotWasteAvailableResource(t *testing.T){
 in,_,_:=fixture(t,true)
 in.Snapshot.Available=2
 result,err:=Run(in);if err!=nil{t.Fatal(err)}
 if result.PostAvailable!=0||len(result.Candidates)!=2||len(result.Conflicts)!=0||len(result.Draws)!=0{
  t.Fatalf("two affordable consumes must not require a lottery: %+v",result)
 }
 for _,disposition:=range result.Dispositions{
  if disposition.Status!="COMMIT"{t.Fatalf("affordable claimant rejected: %+v",disposition)}
 }
 if in.Snapshot.Available!=2{t.Fatal("pure kernel mutated caller snapshot")}
 in.Units[0],in.Units[1]=in.Units[1],in.Units[0]
 permuted,err:=Run(in)
 if err!=nil||!bytes.Equal(result.Canonical,permuted.Canonical){
  t.Fatalf("no-conflict decision depends on worker order: %v",err)
 }
}

func TestCandidateSourcesAreCanonicalSortedSet(t *testing.T){
 in,a,b:=fixture(t,true)
 ids:=[]domain.ID{a.ID,b.ID}
 if canonicalIDLess(ids[1],ids[0]){ids[0],ids[1]=ids[1],ids[0]}
 d:=identity.New(in.Bundle)
 refs,err:=policyRefs(in.Bundle);if err!=nil{t.Fatal(err)}
 makeCandidate:=func(source []domain.ID)error{
  _,err:=MakeCandidate(d,in.RunID,in.CycleID,source,"resource.consume",1,0,0,
   a.Body[9].([]any),[]any{[]any{uint64(5),a.ID[:],a.Digest[:]}},
   0,refs["conflict-resolvers"])
  return err
 }
 if err:=makeCandidate(ids);err!=nil{t.Fatalf("canonical order rejected: %v",err)}
 if err:=makeCandidate([]domain.ID{ids[1],ids[0]});!errors.Is(err,ErrProvenance){
  t.Fatal("permutation created a second identity for the same source set")
 }
 if err:=makeCandidate([]domain.ID{ids[0],ids[1],ids[0]});!errors.Is(err,ErrProvenance){
  t.Fatal("nonadjacent duplicate source could create a candidate")
 }
}

func TestResourceAccumulatorOverflowProducesAttributedIndeterminate(t *testing.T){
 in,_,_:=fixture(t,true)
 in.Snapshot.Available=int64(^uint64(0)>>1)
 d:=identity.New(in.Bundle)
 for i:=range in.Units{
  r:=&in.Units[i].Response
  r.Body=append([]any(nil),r.Body...)
  r.Body[9]=[]any{[]any{"inventory","last.resource",uint64(1),in.Snapshot.Available}}
  hash,_,err:=d.Sum("cote.csf.digest.action-proposal-unit",
   "cote.csf.schema.action-proposal.body",1,[]any{r.Body})
  if err!=nil{t.Fatal(err)}
  r.Digest=hash
  r.Canonical,err=codec.CanonicalPayload(
   append(append([]any(nil),r.Body...),hash[:]))
  if err!=nil{t.Fatal(err)}
 }
 result,err:=Run(in)
 if !errors.Is(err,ErrIndeterminate)||result.Failure!="INDETERMINATE_FACET"||
  len(result.Indeterminate)!=2||len(result.Dispositions)!=0||
  result.PostAvailable!=in.Snapshot.Available||len(result.Canonical)==0{
  t.Fatalf("overflow may not discard subjects or grant resource: %v %+v",err,result)
 }
 subjects:=map[domain.ID]bool{}
 for _,c:=range result.Candidates{subjects[c.ID]=true}
 for _,rec:=range result.Indeterminate{
  if rec.SubjectKind!=1||!subjects[rec.Subject]{
   t.Fatal("detector failure missing authentic candidate subject")
  }
  delete(subjects,rec.Subject)
 }
 if len(subjects)!=0{t.Fatal("detector failure conflated candidate subjects")}
}

func TestAccessRuleIsExplicitAndDenialBlocksWithoutRewritingPossibility(t *testing.T){
 input,a,b:=fixture(t,true)
 delete(input.Snapshot.AccessibleActors,"school:a")
 unknown,err:=Run(input)
 if !errors.Is(err,ErrIndeterminate)||unknown.Failure!="INDETERMINATE_FACET"||
  len(unknown.Indeterminate)!=1||unknown.Indeterminate[0].Subject!=a.ID||
  unknown.Indeterminate[0].Facet!=1||len(unknown.Dispositions)!=0||
  unknown.PostAvailable!=1{
  t.Fatalf("absent access rule must fail closed: %v %+v",err,unknown)
 }
 input.Snapshot.AccessibleActors["school:a"]=false
 denied,err:=Run(input)
 if err!=nil{t.Fatal(err)}
 if len(denied.Conflicts)!=0||len(denied.Draws)!=0||
  len(denied.Dispositions)!=2||denied.PostAvailable!=0{
  t.Fatalf("denied actor must not enter the conflict lottery: %+v",denied)
 }
 if denied.Dispositions[0].Subject!=a.ID||
  denied.Dispositions[0].Status!="REJECT"||
  denied.Dispositions[0].Reason!="access.denied"||
  denied.Dispositions[1].Subject!=b.ID||
  denied.Dispositions[1].Status!="COMMIT"{
  t.Fatalf("access-denied action was admitted: %+v",denied.Dispositions)
 }
 facets,ok:=denied.Assessments[0].Fields[1].([]any)
 if !ok||facets[0].([]any)[1]!=uint64(0)||
  facets[1].([]any)[1]!=uint64(1)||
  facets[1].([]any)[2]!=uint64(0){
  t.Fatal("denial incorrectly changed physical possibility or failed to block")
 }
 input.Units[0],input.Units[1]=input.Units[1],input.Units[0]
 reversed,err:=Run(input)
 if err!=nil||!bytes.Equal(denied.Canonical,reversed.Canonical){
  t.Fatalf("access decision depends on completion order: %v",err)
 }
 input.Snapshot.AccessibleActors["school:a"]=true
 delete(input.Snapshot.ForbiddenActors,"school:b")
 missingInstitution,err:=Run(input)
 if !errors.Is(err,ErrIndeterminate)||len(missingInstitution.Indeterminate)!=1||
  missingInstitution.Indeterminate[0].Facet!=4||
  missingInstitution.Indeterminate[0].Subject!=b.ID||
  len(missingInstitution.Dispositions)!=0{
  t.Fatalf("missing institutional policy treated as permission: %v %+v",err,missingInstitution)
 }
}

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
  Seed:domain.Digest(seed),Snapshot:Snapshot{BaseRevision:7,ResourceKind:"inventory",ResourceID:"last.resource",Available:1,Known:known,
  ForbiddenActors:map[string]bool{"school:a":true}},
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
 if !ok||len(fac)!=4{t.Fatal("missing typed prohibition facet")}
 prohibited:=fac[3].([]any)
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

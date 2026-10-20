package decision

import (
 "bytes"
 "errors"
 "testing"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
)

func TestConservativeDetectorCoversResourceReadsWritesAndInvariants(t *testing.T){
 req,a,b:=fixture(t,true)
 d:=identity.New(req.Bundle)
 refs,e:=policyRefs(req.Bundle);if e!=nil{t.Fatal(e)}
 build:=func(id domain.ID,origin domain.ID,dig domain.Digest)Candidate{
  c,e:=MakeCandidate(d,req.RunID,req.CycleID,[]domain.ID{id},"resource.consume",1,0,0,
   a.Body[9].([]any),[]any{[]any{uint64(5),origin[:],dig[:]}},0,refs["conflict-resolvers"])
  if e!=nil{t.Fatal(e)}
  return c
 }
 x,y:=build(a.ID,a.ID,a.Digest),build(b.ID,b.ID,b.Digest)
 recalc:=func(c *Candidate){
  dg,e:=digest(d,"cote.csf.digest.commit-candidate","cote.csf.schema.commit-candidate.body",c.Fields)
  if e!=nil{t.Fatal(e)}
  c.Digest=dg
  c.Canonical,e=encode(c.Fields,dg);if e!=nil{t.Fatal(e)}
 }
 // The shared stock alone causes one resource conflict.
 x.Fields[12]=[]any{};y.Fields[12]=[]any{}
 recalc(&x);recalc(&y)
 pair:=[]Candidate{x,y}
 actual,e:=DetectClaims(d,req.CycleID,pair,map[string]int64{"last.resource":1},refs["validators"],refs["conflict-resolvers"])
 if e!=nil||len(actual)!=1||actual[0].Fields[2]!=uint64(0){t.Fatalf("resource conflict detection: %v %+v",e,actual)}
 // Read/write overlap must conflict even with two units in stock.
 var subject,hash domain.ID;subject[0]=0x29;hash[0]=0x3a
 causal:=[]any{uint64(5),subject[:],hash[:]}
 x.Fields[10]=[]any{causal}
 y.Fields[11]=[]any{causal}
 recalc(&x);recalc(&y)
 pair=[]Candidate{x,y}
 actual,e=DetectClaims(d,req.CycleID,pair,map[string]int64{"last.resource":2},refs["validators"],refs["conflict-resolvers"])
 if e!=nil||len(actual)!=1||actual[0].Fields[2]!=uint64(1){t.Fatalf("read-write conflict detection: %v %+v",e,actual)}
 pair[0],pair[1]=pair[1],pair[0]
 reversed,e:=DetectClaims(d,req.CycleID,pair,map[string]int64{"last.resource":2},refs["validators"],refs["conflict-resolvers"])
 if e!=nil||!bytes.Equal(actual[0].Canonical,reversed[0].Canonical){t.Fatal("claim detector depends on registration or completion order")}
 // Shared invariant must be conservative even when stock would suffice.
 x.Fields[10]=[]any{};y.Fields[11]=[]any{}
 x.Fields[12]=[]any{"resource.nonnegative"}
 y.Fields[12]=[]any{"resource.nonnegative"}
 recalc(&x);recalc(&y)
 actual,e=DetectClaims(d,req.CycleID,[]Candidate{x,y},map[string]int64{"last.resource":2},refs["validators"],refs["conflict-resolvers"])
 if e!=nil||len(actual)!=1||actual[0].Fields[2]!=uint64(2){t.Fatalf("invariant conflict detection: %v %+v",e,actual)}
 if _,e:=DetectClaims(d,req.CycleID,[]Candidate{x,y},nil,refs["validators"],refs["conflict-resolvers"]);!errors.Is(e,ErrIndeterminate){
  t.Fatal("missing resource capacity silently grants conflict")
 }
 // Tampering resource claims without changing the registered digest fails.
 broken:=x;broken.Fields=append([]any(nil),x.Fields...)
 broken.Fields[9]=[]any{}
 if _,e:=DetectClaims(d,req.CycleID,[]Candidate{broken,y},map[string]int64{"last.resource":2},refs["validators"],refs["conflict-resolvers"]);!errors.Is(e,ErrProvenance){
  t.Fatal("altered candidate claims accepted without digest")
 }
}

func TestClaimDetectorRefusesForgedCandidateSourceOrCycle(t *testing.T){
 in,a,b:=fixture(t,true)
 d:=identity.New(in.Bundle)
 refs,err:=policyRefs(in.Bundle);if err!=nil{t.Fatal(err)}
 makeCandidate:=func(id domain.ID,origin domain.Digest)Candidate{
  candidate,err:=MakeCandidate(d,in.RunID,in.CycleID,[]domain.ID{id},
   "resource.consume",1,0,0,a.Body[9].([]any),
   []any{[]any{uint64(5),id[:],origin[:]}},0,refs["conflict-resolvers"])
  if err!=nil{t.Fatal(err)}
  return candidate
 }
 one:=makeCandidate(a.ID,a.Digest)
 two:=makeCandidate(b.ID,b.Digest)
 check:=func(c Candidate){
  t.Helper()
  if _,err:=DetectClaims(d,in.CycleID,[]Candidate{c,two},
   map[string]int64{"last.resource":1},refs["validators"],refs["conflict-resolvers"]);
   !errors.Is(err,ErrProvenance){
   t.Fatalf("forged candidate passed conflict grouping: %v",err)
  }
 }
 replacedSource:=one
 replacedSource.SourceUnitIDs=[]domain.ID{b.ID}
 check(replacedSource)
 rehash:=func(c *Candidate){
  t.Helper()
  digest,err:=digest(d,"cote.csf.digest.commit-candidate",
   "cote.csf.schema.commit-candidate.body",c.Fields)
  if err!=nil{t.Fatal(err)}
  c.Digest=digest
  c.Canonical,err=encode(c.Fields,digest)
  if err!=nil{t.Fatal(err)}
 }
 forgedID:=one
 forgedID.Fields=append([]any(nil),one.Fields...)
 forgedID.ID[0]^=0xff
 forgedID.Fields[0]=forgedID.ID[:]
 rehash(&forgedID)
 check(forgedID)
 foreignCycle:=one
 foreignCycle.Fields=append([]any(nil),one.Fields...)
 foreign:=in.CycleID
 foreign[0]^=0xf0
 foreignCycle.Fields[2]=foreign[:]
 rehash(&foreignCycle)
 check(foreignCycle)
 correct,err:=DetectClaims(d,in.CycleID,[]Candidate{one,two},
  map[string]int64{"last.resource":1},refs["validators"],refs["conflict-resolvers"])
 if err!=nil||len(correct)<1{t.Fatalf("valid candidates rejected: %v %+v",err,correct)}
 resourceConflicts:=0
 for _,conflict:=range correct{if conflict.Fields[2]==uint64(0){resourceConflicts++}}
 if resourceConflicts!=1{t.Fatalf("expected one resource conflict, got %d",resourceConflicts)}
}

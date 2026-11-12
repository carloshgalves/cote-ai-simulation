package sqlite

import (
 "bytes"
 "strings"
 "testing"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
)

func test57ProvenanceFixture(t *testing.T)(snapshotPendingPartitions,*snapshotPendingProvenance){
 t.Helper()
 run:=[32]byte{0x91}
 parts:=newSnapshotPendingPartitions()
 proof:=newSnapshotPendingProvenance(run)
 ref:=func(kind uint64,id byte)[]any{
  return []any{kind,pending57ID(id),pending57ID(id+64)}
 }
 parts.occurrences=[]any{ref(6,1)}
 parts.triggerRuntime=[]any{pending57Parts()[1].([]any)[0]}
 parts.triggerActivations=[]any{ref(7,2)}
 parts.sourceClosures=[]any{ref(2,3)}
 parts.rounds=[]any{ref(3,4)}
 parts.dispatches=[]any{ref(4,5)}
 parts.responses=[]any{ref(5,6),ref(23,7)}
 parts.pendingInputs=[]any{
  ref(1,8),ref(4,4),ref(2,1),ref(3,2),
  []any{uint64(0),uint64(1),pending57ID(9),pending57ID(73)},
 }
 parts.perceptionTasks=[]any{ref(15,10)}
 parts.perceptionCompletions=[]any{
  []any{pending57ID(11),pending57ID(10),[]any{},[]any{},pending57ID(79)},
 }
 for part,entries:=range []struct{owner string;kind uint64;items []any}{
  {snapshotOwnerOccurrence,6,parts.occurrences},
  {snapshotOwnerRuntime,0,parts.triggerRuntime},
  {snapshotOwnerActivation,7,parts.triggerActivations},
  {snapshotOwnerClosure,2,parts.sourceClosures},
  {snapshotOwnerRound,3,parts.rounds},
  {snapshotOwnerDispatch,4,parts.dispatches},
  {snapshotOwnerResponse,0,parts.responses},
  {snapshotOwnerExogenous,1,parts.pendingInputs[:1]},
  {snapshotOwnerSlotResponse,0,parts.pendingInputs[4:]},
  {snapshotOwnerTask,15,parts.perceptionTasks},
  {snapshotOwnerCompletion,0,parts.perceptionCompletions},
 } {
  _=part
  for _,raw:=range entries.items{
   f:=raw.([]any)
   var id,digest,canonical []byte
   kind:=entries.kind
   switch entries.owner {
   case snapshotOwnerRuntime,snapshotOwnerCompletion:
    id=f[0].([]byte)
    var err error
    canonical,err=codec.CanonicalPayload(f)
    if err!=nil{t.Fatal(err)}
   case snapshotOwnerSlotResponse:
    id,digest=f[2].([]byte),f[3].([]byte)
   default:
    kind=f[0].(uint64)
    id,digest=f[1].([]byte),f[2].([]byte)
   }
   if err:=proof.bind(entries.owner,run,id,kind,digest,canonical);err!=nil{t.Fatal(err)}
  }
 }
 return parts,proof
}

func TestSnapshot57P4AllTenRostersHaveSourceWitnessAndLinks(t *testing.T){
 p,proof:=test57ProvenanceFixture(t)
 if err:=proof.verify(p);err!=nil{t.Fatal(err)}
 ordered,err:=p.canonicalOrder();if err!=nil{t.Fatal(err)}
 // P1's independent decoder must accept these exact emitted references.
 if _,err:=decodeSnapshotPendingRosters(pending57Snapshot(t,ordered));err!=nil{
  t.Fatal("valid source-bound snapshot not decodable:",err)
 }
}

func TestSnapshot57P4RejectForeignRunAndMissingSourceInEveryRoster(t *testing.T){
 for part:=0;part<10;part++{
  t.Run(pendingSnapshotPartSpecs[part].name,func(t *testing.T){
   p,proof:=test57ProvenanceFixture(t)
   rosters,err:=p.canonicalOrder();if err!=nil{t.Fatal(err)}
   first:=rosters[part].([]any)[0].([]any)
   var id [32]byte
   var kind uint64
   if part==1||part==9{
    id,err=pendingSnapshotID(first[0]);if err!=nil{t.Fatal(err)}
   }else{
    r,e:=pendingSnapshotRef(first,part);if e!=nil{t.Fatal(e)}
    id,kind=r.ID,r.Kind
   }
   owner,err:=snapshotOwnerForPart(part,kind);if err!=nil{t.Fatal(err)}
   key:=snapshotPendingWitnessKey{owner,id}
   original,ok:=proof.witnesses[key];if !ok{t.Fatal("fixture missing witness")}
   delete(proof.witnesses,key)
   if err:=proof.verify(p);err==nil{
    t.Fatal("snapshot accepted source root absent")
   }
   proof.witnesses[key]=original
   foreign:=original
   foreign.run[0]^=0xff
   proof.witnesses[key]=foreign
   if err:=proof.verify(p);err==nil||!strings.Contains(err.Error(),"foreign"){
    t.Fatal("snapshot accepted foreign-run witness:",err)
   }
   proof.witnesses[key]=original
   if err:=proof.verify(p);err!=nil{t.Fatal("restored witness rejected:",err)}
  })
 }
}

func TestSnapshot57P4RejectDigestIdentityAndCanonicalSwap(t *testing.T){
 for part:=0;part<10;part++{
  t.Run(pendingSnapshotPartSpecs[part].name,func(t *testing.T){
   p,proof:=test57ProvenanceFixture(t)
   rosters,err:=p.canonicalOrder();if err!=nil{t.Fatal(err)}
   f:=append([]any(nil),rosters[part].([]any)[0].([]any)...)
   switch part{
   case 1,9:
    f[len(f)-1]=pending57ID(0xfe)
   default:
    ix:=len(f)-1
    tampered:=bytes.Clone(f[ix].([]byte))
    tampered[0]^=0x8f
    f[ix]=tampered
   }
   snapshot57SetPartition(&p,part,append([]any{f},rosters[part].([]any)[1:]...))
   if err:=proof.verify(p);err==nil{
    t.Fatal("snapshot accepted mutated digest/canonical or swapped identity")
   }
  })
 }
}

func TestSnapshot57P4PendingInputMustLinkExactSourceRoster(t *testing.T){
 for _,tc:=range []struct{
  name string
  missingSource bool
  missingInput bool
  corruptInput bool
 }{
  {"missing-round-roster",true,false,false},
  {"missing-round-input",false,true,false},
  {"corrupt-round-digest",false,false,true},
 }{
  t.Run(tc.name,func(t *testing.T){
   p,proof:=test57ProvenanceFixture(t)
   switch{
   case tc.missingSource:p.rounds=[]any{}
   case tc.missingInput:p.pendingInputs=append(p.pendingInputs[:1],p.pendingInputs[2:]...)
   case tc.corruptInput:
    f:=append([]any(nil),p.pendingInputs[1].([]any)...)
    f[2]=pending57ID(0xff)
    p.pendingInputs[1]=f
   }
   if err:=proof.verify(p);err==nil{
    t.Fatal("snapshot accepted orphan, missing or mismatched round/input link")
   }
  })
 }
}

func TestSnapshot57P4BindingRejectsForeignRunDuplicatesAndMalformedIDs(t *testing.T){
 _,proof:=test57ProvenanceFixture(t)
 foreign:=proof.run;foreign[0]^=1
 id:=pending57ID(0xb1)
 digest:=pending57ID(0xf1)
 if err:=proof.bind(snapshotOwnerOccurrence,foreign,id,6,digest,nil);err==nil{
  t.Fatal("foreign run inserted as witness")
 }
 if err:=proof.bind(snapshotOwnerOccurrence,proof.run,[]byte{0x01},6,digest,nil);err==nil{
  t.Fatal("short source identity accepted")
 }
 if err:=proof.bind(snapshotOwnerOccurrence,proof.run,id,6,[]byte{1},nil);err==nil{
  t.Fatal("short source digest accepted")
 }
 if err:=proof.bind(snapshotOwnerOccurrence,proof.run,id,6,digest,nil);err!=nil{t.Fatal(err)}
 if err:=proof.bind(snapshotOwnerOccurrence,proof.run,id,6,digest,nil);err==nil{
  t.Fatal("duplicate owner+identity overwrote first proof")
 }
}

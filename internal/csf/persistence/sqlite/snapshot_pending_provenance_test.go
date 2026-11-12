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
  []any{pending57ID(11),pending57ID(14),[]any{},[]any{},pending57ID(79)},
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
 // IDs 12/13 are settled dispatches, while 5 is still outstanding.
 // Task 14 has completed; task 10 remains outstanding.
 for _,settled:=range []byte{12,13}{
  if err:=proof.bind(snapshotOwnerDispatch,run,pending57ID(settled),4,pending57ID(settled+64),nil);err!=nil{t.Fatal(err)}
 }
 if err:=proof.bind(snapshotOwnerTask,run,pending57ID(14),15,pending57ID(14+64),nil);err!=nil{t.Fatal(err)}
 if err:=proof.bindLink(snapshotOwnerResponse,pending57ID(6),pending57ID(12));err!=nil{t.Fatal(err)}
 if err:=proof.bindLink(snapshotOwnerResponse,pending57ID(7),pending57ID(13));err!=nil{t.Fatal(err)}
 if err:=proof.bindLink(snapshotOwnerCompletion,pending57ID(11),pending57ID(14));err!=nil{t.Fatal(err)}
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

func TestSnapshot57P4ResponseAndCompletionLinksFailClosed(t *testing.T){
 for _,tc:=range []struct{
  name string
  owner string
  source byte
  target byte
  targetOwner string
  mutateCompletion bool
 }{
  {"response-link-missing",snapshotOwnerResponse,6,12,snapshotOwnerDispatch,false},
  {"completion-link-missing",snapshotOwnerCompletion,11,14,snapshotOwnerTask,false},
  {"response-parent-missing",snapshotOwnerResponse,6,12,snapshotOwnerDispatch,false},
  {"completion-parent-missing",snapshotOwnerCompletion,11,14,snapshotOwnerTask,false},
  {"completion-target-changed",snapshotOwnerCompletion,11,14,snapshotOwnerTask,true},
 }{
  t.Run(tc.name,func(t *testing.T){
   p,proof:=test57ProvenanceFixture(t)
   childID,parID:=pending57ID(tc.source),pending57ID(tc.target)
   child,_:=pendingSnapshotID(childID)
   parent,_:=pendingSnapshotID(parID)
   key:=snapshotPendingWitnessKey{tc.owner,child}
   parentKey:=snapshotPendingWitnessKey{tc.targetOwner,parent}
   switch tc.name{
   case "response-link-missing","completion-link-missing":delete(proof.links,key)
   case "response-parent-missing","completion-parent-missing":delete(proof.witnesses,parentKey)
   case "completion-target-changed":
    bad:=append([]any(nil),p.perceptionCompletions[0].([]any)...)
    bad[1]=pending57ID(0xc2)
    p.perceptionCompletions=[]any{bad}
    proof.witnesses[key]=snapshotPendingWitness{
     run:proof.run, kind:0, canonical:func()[]byte{
      b,err:=codec.CanonicalPayload(bad);if err!=nil{t.Fatal(err)};return b
     }(),
    }
   }
   if err:=proof.verify(p);err==nil{
    t.Fatal("orphan or mislinked historical response/completion accepted")
   }
  })
 }
}

func TestSnapshot57P4WitnessKindAndLinkRegistrationAreStrict(t *testing.T){
 _,proof:=test57ProvenanceFixture(t)
 id,digest:=pending57ID(0xb2),pending57ID(0xc2)
 for _,tc:=range []struct{owner string;kind uint64;digest,canonical []byte}{
  {"unknown-owner",6,digest,nil},
  {snapshotOwnerOccurrence,7,digest,nil},
  {snapshotOwnerOccurrence,6,nil,nil},
  {snapshotOwnerResponse,5,nil,nil},
  {snapshotOwnerRuntime,0,digest,nil},
  {snapshotOwnerCompletion,0,nil,nil},
 }{
  if err:=proof.bind(tc.owner,proof.run,id,tc.kind,tc.digest,tc.canonical);err==nil{
   t.Fatalf("incomplete or unknown witness accepted: owner=%s kind=%d",tc.owner,tc.kind)
  }
 }
 if err:=proof.bindLink(snapshotOwnerRound,id,digest);err==nil{
  t.Fatal("unregistered link owner accepted")
 }
 if err:=proof.bindLink(snapshotOwnerResponse,id,digest);err==nil{
  t.Fatal("source link registered without an authenticated owner witness")
 }
 if err:=proof.bindLink(snapshotOwnerResponse,pending57ID(6),pending57ID(12));err==nil{
  t.Fatal("duplicate response link overwrote first source target")
 }
}

func TestSnapshot57P4PendingAndSettledAuthoritiesAreExclusive(t *testing.T){
 for _,tc:=range []string{
  "answered-dispatch-still-pending",
  "two-responses-one-dispatch",
  "completed-task-still-pending",
  "two-completions-one-task",
 }{
  t.Run(tc,func(t *testing.T){
   parts,proof:=test57ProvenanceFixture(t)
   switch tc{
   case "answered-dispatch-still-pending":
    // Source 12 has a response; it must not be emitted in pending dispatches.
    parts.dispatches=[]any{snapshot57Ref(4,12)}
   case "two-responses-one-dispatch":
    id,_:=pendingSnapshotID(pending57ID(7))
    proof.links[snapshotPendingWitnessKey{owner:snapshotOwnerResponse,id:id}]=func()[32]byte{
     target,_:=pendingSnapshotID(pending57ID(12));return target
    }()
   case "completed-task-still-pending":
    parts.perceptionTasks=[]any{snapshot57Ref(15,14)}
   case "two-completions-one-task":
    original:=parts.perceptionCompletions[0].([]any)
    second:=append([]any(nil),original...)
    second[0]=pending57ID(15)
    parts.perceptionCompletions=append(parts.perceptionCompletions,second)
    raw,err:=codec.CanonicalPayload(second);if err!=nil{t.Fatal(err)}
    if err:=proof.bind(snapshotOwnerCompletion,proof.run,pending57ID(15),0,nil,raw);err!=nil{t.Fatal(err)}
    if err:=proof.bindLink(snapshotOwnerCompletion,pending57ID(15),pending57ID(14));err!=nil{t.Fatal(err)}
   }
   if err:=proof.verify(parts);err==nil{
    t.Fatal("snapshot accepted contradictory pending/settled membership")
   }
  })
 }
}

func TestSnapshot57P4ZeroValueProvenanceReturnsErrors(t *testing.T){
 var zero snapshotPendingProvenance
 id,digest:=pending57ID(0xa1),pending57ID(0xa2)
 if err:=zero.bind(snapshotOwnerOccurrence,zero.run,id,6,digest,nil);err==nil{
  t.Fatal("zero-value witness index caused silent acceptance")
 }
 if err:=zero.bindLink(snapshotOwnerResponse,id,id);err==nil{
  t.Fatal("zero-value link index caused silent acceptance")
 }
 zero.witnesses=make(map[snapshotPendingWitnessKey]snapshotPendingWitness)
 if err:=zero.bindLink(snapshotOwnerResponse,id,id);err==nil{
  t.Fatal("missing links map caused silent acceptance")
 }
}

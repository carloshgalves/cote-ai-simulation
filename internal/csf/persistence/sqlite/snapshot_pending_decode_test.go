package sqlite

import (
 "bytes"
 "strings"
 "testing"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/identity"
)

func pending57ID(value byte)[]byte{
 raw:=make([]byte,32)
 raw[0]=value
 return raw
}

func pending57Parts()[]any{
 parts:=make([]any,10)
 for i:=range parts{parts[i]=[]any{}}
 ref:=func(kind uint64,value byte)[]any{return []any{kind,pending57ID(value),pending57ID(value+64)}}
 parts[0]=[]any{ref(6,1)}
 parts[1]=[]any{[]any{pending57ID(2),uint64(1),[]any{uint64(0)},false,false,uint64(0),[]any{uint64(0)},uint64(0)}}
 parts[2]=[]any{ref(7,3)}
 parts[3]=[]any{ref(2,4)}
 parts[4]=[]any{ref(3,5)}
 parts[5]=[]any{ref(4,6)}
 parts[6]=[]any{ref(5,7),ref(23,8)}
 parts[7]=[]any{ref(1,9),ref(2,10),ref(3,11),ref(4,12),
  []any{uint64(0),uint64(1),pending57ID(13),pending57ID(77)}}
 parts[8]=[]any{ref(15,14)}
 parts[9]=[]any{[]any{pending57ID(15),pending57ID(16),[]any{},[]any{},pending57ID(79)}}
 return parts
}

func pending57Snapshot(t *testing.T,parts []any)[]any{
 t.Helper()
 raw,err:=codec.CanonicalBytes("cote.csf.snapshot.pending-state","cote.csf.schema.snapshot-pending-state",1,parts)
 if err!=nil{t.Fatal(err)}
 digest:=identity.OpaqueContentHash(raw)
 snapshot:=make([]any,19)
 snapshot[11]=[]any{"cote.csf.schema.snapshot-pending-state",uint64(1),raw,digest[:]}
 return snapshot
}

func TestPending57AllTenPartitionsAreTypedAndOrderPreserved(t *testing.T){
 input:=pending57Parts()
 out,err:=decodeSnapshotPendingRosters(pending57Snapshot(t,input))
 if err!=nil{t.Fatal(err)}
 counts:=[]int{len(out.Occurrences),len(out.TriggerRuntimes),len(out.TriggerActivations),
  len(out.SourceClosures),len(out.Rounds),len(out.Dispatches),len(out.Responses),
  len(out.PendingInputs),len(out.PerceptionTasks),len(out.PerceptionCompletions)}
 want:=[]int{1,1,1,1,1,1,2,5,1,1}
 for i,n:=range counts{if n!=want[i]{t.Fatalf("partition %d contains %d, want %d",i,n,want[i])}}
 if out.Responses[0].Kind!=5||out.Responses[1].Kind!=23||
  out.PendingInputs[0].Kind!=1||out.PendingInputs[4].Kind!=0||
  !out.PendingInputs[4].HasSlotResponseKind||out.PendingInputs[4].SlotResponseKind!=1{
  t.Fatal("typed snapshot decoder reordered refs or dropped slot-response discriminator")
 }
 if out.Rounds[0].ID[0]!=5{
  t.Fatal("round ID was reminted")
 }
 before,err:=codec.CanonicalPayload(input);if err!=nil{t.Fatal(err)}
 legacy,err:=codec.CanonicalPayload(out.legacyPartitions());if err!=nil{t.Fatal(err)}
 if !bytes.Equal(before,legacy){t.Fatal("type projection rewrote canonical snapshot pending bytes")}
 originalID:=input[4].([]any)[0].([]any)[1].([]byte)
 originalID[0]^=0xff
 if out.Rounds[0].ID[0]!=5{t.Fatal("typed IDs alias untrusted input buffers")}
}

func TestPending57AcceptsEmptyRostersButNotMissingPartitions(t *testing.T){
 parts:=make([]any,10)
 for i:=range parts{parts[i]=[]any{}}
 if _,err:=decodeSnapshotPendingRosters(pending57Snapshot(t,parts));err!=nil{
  t.Fatal("ten present, empty rosters are valid:",err)
 }
 for _,n:=range []int{0,1,9,11}{
  faulty:=make([]any,n)
  for i:=range faulty{faulty[i]=[]any{}}
  if _,err:=decodeSnapshotPendingRosters(pending57Snapshot(t,faulty));err==nil{
   t.Fatalf("accepted %d instead of ten partitions",n)
  }
 }
}

func TestPending57RejectsMalformedDuplicateAndWrongKindInEveryPartition(t *testing.T){
 for part:=0;part<10;part++{
  t.Run(pendingSnapshotPartSpecs[part].name,func(t *testing.T){
   valid:=pending57Parts()
   items:=valid[part].([]any)
   for label,mutator:=range map[string]func([]any)[]any{
    "absent":func(p []any)[]any{p[part]=nil;return p},
    "duplicate":func(p []any)[]any{p[part]=append(append([]any(nil),items...),items[0]);return p},
    "not-array":func(p []any)[]any{p[part]=[]any{"not-a-valid-ref"};return p},
    "oversized":func(p []any)[]any{p[part]=make([]any,pendingSnapshotPartSpecs[part].max+1);return p},
   }{
    p:=append([]any(nil),valid...)
    p=mutator(p)
    if _,err:=decodeSnapshotPendingRosters(pending57Snapshot(t,p));err==nil{
     t.Fatalf("partition %s accepted %s",pendingSnapshotPartSpecs[part].name,label)
    }
   }
   if part!=1&&part!=9 {
    p:=append([]any(nil),valid...)
    wrong:=[]any{uint64(65534),pending57ID(22),pending57ID(23)}
    p[part]=[]any{wrong}
    if _,err:=decodeSnapshotPendingRosters(pending57Snapshot(t,p));err==nil{
     t.Fatal("accepted foreign kind for roster")
    }
    p=append([]any(nil),valid...)
    p[part]=[]any{[]any{pendingSnapshotPartSpecs[part].kinds[0],pending57ID(22),[]byte{1}}}
    if _,err:=decodeSnapshotPendingRosters(pending57Snapshot(t,p));err==nil{
     t.Fatal("accepted short identity/digest")
    }
   }
  })
 }
}

func TestPending57RejectsTamperedEnvelopeAndInvalidVersion(t *testing.T){
 valid:=pending57Snapshot(t,pending57Parts())
 typed:=valid[11].([]any)
 corrupt:=append([]byte(nil),typed[2].([]byte)...)
 corrupt[len(corrupt)-1]^=1
 bad:=append([]any(nil),valid...)
 bad[11]=[]any{typed[0],typed[1],corrupt,typed[3]}
 if _,err:=decodeSnapshotPendingRosters(bad);err==nil||!strings.Contains(err.Error(),"digest"){
  t.Fatal("one-byte changed pending envelope did not fail closed:",err)
 }
 bad=append([]any(nil),valid...)
 bad[11]=[]any{typed[0],uint64(2),typed[2],typed[3]}
 if _,err:=decodeSnapshotPendingRosters(bad);err==nil{t.Fatal("unknown schema version accepted")}
 bad=append([]any(nil),valid...)
 bad[11]=[]any{typed[0],typed[1],typed[2],[]byte{0x11}}
 if _,err:=decodeSnapshotPendingRosters(bad);err==nil{t.Fatal("short pending digest accepted")}
}

func TestPending57RejectsMalformedTriggerRuntimeAndCompletionDetails(t *testing.T){
 for _,tc:=range []struct{
  name string
  part int
  field int
  value any
 }{
  {"trigger-last-value",1,2,[]any{uint64(1),"not-a-bool"}},
  {"trigger-armed",1,3,uint64(0)},
  {"trigger-version-overflow",1,1,uint64(1)<<32},
  {"completion-observation-list",9,2,"not-an-array"},
  {"completion-observation-id",9,2,[]any{[]byte{0x11}}},
  {"completion-knowledge-id",9,3,[]any{[]byte{0x22}}},
 }{
  t.Run(tc.name,func(t *testing.T){
   parts:=pending57Parts()
   raw:=append([]any(nil),parts[tc.part].([]any)[0].([]any)...)
   raw[tc.field]=tc.value
   parts[tc.part]=[]any{raw}
   if _,err:=decodeSnapshotPendingRosters(pending57Snapshot(t,parts));err==nil{
    t.Fatal("malformed normative runtime/completion field accepted")
   }
  })
 }
}

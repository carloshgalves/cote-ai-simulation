package sqlite

import (
 "bytes"
 "testing"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
)

func forkP2Ref(id [32]byte) []any {
 return []any{uint64(6),bytes.Clone(id[:]),bytes.Repeat([]byte{0x23},32)}
}

func forkP2Import(id [32]byte) V2CommunicationBirth {
 var birth V2CommunicationBirth
 birth.Parent.OccurrenceID=id
 return birth
}

func TestForkP2PartitionV2PendingOccurrences(t *testing.T){
 var a,b,c [32]byte
 a[0],b[0],c[0]=0x11,0x22,0x33
 source:=[]any{forkP2Ref(a),forkP2Ref(b),forkP2Ref(c)}
 original,err:=codec.CanonicalPayload(source)
 if err!=nil{t.Fatal(err)}
 remaining,err:=partitionV2PendingOccurrenceRefs(source,[]V2CommunicationBirth{forkP2Import(b)})
 if err!=nil{t.Fatal(err)}
 if len(remaining)!=2||!equalBytes(remaining[0].([]any)[1],a[:])||!equalBytes(remaining[1].([]any)[1],c[:]){
  t.Fatal("authenticated source roster order or imported exclusion changed")
 }
 after,err:=codec.CanonicalPayload(source)
 if err!=nil||!bytes.Equal(original,after){t.Fatal("pure partition modified caller-owned source refs",err)}
 if _,err:=partitionV2PendingOccurrenceRefs(source,[]V2CommunicationBirth{forkP2Import(b),forkP2Import(b)});err==nil{
  t.Fatal("duplicate imported occurrence not rejected")
 }
 var absent [32]byte;absent[0]=0x44
 if _,err:=partitionV2PendingOccurrenceRefs(source,[]V2CommunicationBirth{forkP2Import(absent)});err==nil{
  t.Fatal("in-transit occurrence missing from signed source roster accepted")
 }
 for _,bad:=range []any{
  []any{uint64(7),a[:],bytes.Repeat([]byte{0x23},32)},
  []any{uint64(6),a[:]},
  []any{uint64(6),[]byte{0x11},bytes.Repeat([]byte{0x23},32)},
 }{
  if _,err:=partitionV2PendingOccurrenceRefs([]any{bad},nil);err==nil{
   t.Fatal("malformed pending occurrence escaped strict partition guard")
  }
 }
}

func TestForkP2ProjectV2CommunicationBirths(t *testing.T){
 fields:=make([]any,15)
 for i:=range fields{fields[i]=uint64(0)}
 fields[4]=[]any{int64(30),uint64(9)}
 raw,err:=codec.CanonicalPayload(fields)
 if err!=nil{t.Fatal(err)}
 var id [32]byte;id[0]=0x77
 birth:=forkP2Import(id)
 birth.ChildOccurrence.Identity=id
 birth.ChildOccurrence.Canonical=raw
 out,err:=remintV2CommunicationOccurrences([]V2CommunicationBirth{birth})
 if err!=nil{t.Fatal(err)}
 if len(out)!=1||out[0].ID!=id||out[0].EligibilityInstant!=30||out[0].EligibilityOrdinal!=9||
  !bytes.Equal(out[0].Canonical,raw){t.Fatal("pure projected occurrence changed authenticated bytes")}
 out[0].Canonical[0]^=1
 if !bytes.Equal(birth.ChildOccurrence.Canonical,raw){t.Fatal("test invariant broken")}
 // Immutable input contract: the projector must not let mutation of returned
 // planning records alias a previously authenticated birth.
 if bytes.Equal(out[0].Canonical,birth.ChildOccurrence.Canonical){
  t.Fatal("projection returned mutable alias to authenticated child birth")
 }
 for _,broken:=range [][]any{
  fields[:14],
  func()[]any{copyFields:=append([]any(nil),fields...);copyFields[4]=uint64(0);return copyFields}(),
  func()[]any{copyFields:=append([]any(nil),fields...);copyFields[4]=[]any{int64(30),"nine"};return copyFields}(),
 }{
  bad,err:=codec.CanonicalPayload(broken)
  if err!=nil{t.Fatal(err)}
  invalid:=birth
  invalid.ChildOccurrence.Canonical=bad
  if _,err:=remintV2CommunicationOccurrences([]V2CommunicationBirth{invalid});err==nil{
   t.Fatal("invalid pending occurrence coordinate/body was accepted")
  }
 }
}

func TestForkP2PureCognitiveRoster(t *testing.T){
 actor:=[]any{"school","agent-a"}
 ref:=[]any{actor,uint64(0),uint64(0),uint64(0),uint64(0),uint64(0),uint64(0),uint64(0)}
 out,err:=deriveForkCognitiveActors([]any{ref},true)
 if err!=nil||len(out)!=1||out[0].namespace!="school"||out[0].id!="agent-a"{
  t.Fatal("typed V2 cognitive roster failed",err)
 }
 if none,err:=deriveForkCognitiveActors([]any{ref},false);err!=nil||len(none)!=0{
  t.Fatal("same-policy cognitive roster incorrectly imported",err)
 }
 invalid:=[]any{
  []any{[]any{"school","bad space"},uint64(0),uint64(0),uint64(0),uint64(0),uint64(0),uint64(0),uint64(0)},
  []any{[]any{"school"},uint64(0),uint64(0),uint64(0),uint64(0),uint64(0),uint64(0),uint64(0)},
  []any{actor,uint64(0)},
 }
 for _,bad:=range invalid{
  if _,err:=deriveForkCognitiveActors([]any{bad},true);err==nil{
   t.Fatal("malformed cognitive actor accepted")
  }
 }
 if _,err:=deriveForkCognitiveActors([]any{ref,ref},true);err==nil{
  t.Fatal("duplicate cognitive actor accepted")
 }
}

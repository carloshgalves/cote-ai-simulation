package sqlite

import (
 "bytes"
 "encoding/hex"
 "testing"
)

func TestCascadeLimitRequiresExactGenesisPolicyIDVersionAndDigest(t *testing.T){
 pinned,err:=hex.DecodeString("0bac988281bc2b0c4af79adfc9dbd238395ffee5b8ab8758eedc89a75bcd6a6c")
 if err!=nil{t.Fatal(err)}
 valid:=[]any{[]any{"cycle-coordinate-v1",uint64(1),pinned}}
 cases:=[]struct{name string;refs []any;accepted bool}{
  {"pinned-v1",valid,true},
  {"missing-policy",[]any{},false},
  {"wrong-version",[]any{[]any{"cycle-coordinate-v1",uint64(2),pinned}},false},
  {"wrong-hash",[]any{[]any{"cycle-coordinate-v1",uint64(1),bytes.Repeat([]byte{0x05},32)}},false},
  {"wrong-id",[]any{[]any{"unrelated-v1",uint64(1),pinned}},false},
  {"duplicate-policy",append(append([]any{},valid...),valid...),false},
  {"malformed-policy",[]any{[]any{"cycle-coordinate-v1",uint64(1),[]byte{4}}},false},
 }
 for _,tc:=range cases{t.Run(tc.name,func(t *testing.T){
  err:=verifyCascadePolicyRefs(tc.refs)
  if (err==nil)!=tc.accepted{t.Fatalf("cascade pin accepted=%v expected=%v error=%v",err==nil,tc.accepted,err)}
 })}
}

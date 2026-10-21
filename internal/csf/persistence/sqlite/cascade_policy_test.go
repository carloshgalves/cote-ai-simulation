package sqlite

import (
 "bytes"
 "testing"
)

func TestCascadeLimitRequiresExactGenesisPolicyIDVersionAndDigest(t *testing.T){
 valid:=[]any{[]any{"cycle-coordinate-v1",uint64(1),bytes.Repeat([]byte{0x04},32)}}
 cases:=[]struct{name string;refs []any;accepted bool}{
  {"pinned-v1",valid,true},
  {"missing-policy",[]any{},false},
  {"wrong-version",[]any{[]any{"cycle-coordinate-v1",uint64(2),bytes.Repeat([]byte{0x04},32)}},false},
  {"wrong-hash",[]any{[]any{"cycle-coordinate-v1",uint64(1),bytes.Repeat([]byte{0x05},32)}},false},
  {"wrong-id",[]any{[]any{"unrelated-v1",uint64(1),bytes.Repeat([]byte{0x04},32)}},false},
  {"duplicate-policy",append(append([]any{},valid...),valid...),false},
  {"malformed-policy",[]any{[]any{"cycle-coordinate-v1",uint64(1),[]byte{4}}},false},
 }
 for _,tc:=range cases{t.Run(tc.name,func(t *testing.T){
  err:=verifyCascadePolicyRefs(tc.refs)
  if (err==nil)!=tc.accepted{t.Fatalf("cascade pin accepted=%v expected=%v error=%v",err==nil,tc.accepted,err)}
 })}
}

package commit

import "testing"

func TestCSFRV18VerifiedRetryOrdinalStrictSuccessorAndOverflow(t *testing.T){
 cases:=[]struct{
  name string
  previous,proposed uint64
  accepted bool
 }{
  {"first authorized retry",0,1,true},
  {"largest representable retry",uint64(^uint32(0))-1,uint64(^uint32(0)),true},
  {"zero does not advance",0,0,false},
  {"skipped attempt",1,3,false},
  {"reused attempt",9,9,false},
  {"regression",9,8,false},
  {"V1 overflow",uint64(^uint32(0)),uint64(^uint32(0))+1,false},
  {"V1 reused max",uint64(^uint32(0)),uint64(^uint32(0)),false},
  {"uint64 wraparound",^uint64(0),0,false},
  {"huge forged ordinal",0,^uint64(0),false},
 }
 for _,tc:=range cases{
  t.Run(tc.name,func(t *testing.T){
   got,err:=verifiedRetrySuccessor(tc.previous,tc.proposed)
   if (err==nil)!=tc.accepted{t.Fatalf("previous=%d proposed=%d accepted=%t error=%v",tc.previous,tc.proposed,tc.accepted,err)}
   if tc.accepted&&uint64(got)!=tc.proposed{t.Fatalf("ordinal truncated: got=%d proposed=%d",got,tc.proposed)}
  })
 }
}

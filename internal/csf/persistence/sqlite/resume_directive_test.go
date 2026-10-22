package sqlite

import (
 "bytes"
 "errors"
 "testing"
)

func TestCSFRV18ResumeControlFourStatesAndPerceptionBarrier(t *testing.T){
 var cycle,fenceID,retryID,abortID [32]byte
 cycle[0],fenceID[0],retryID[0],abortID[0]=1,2,3,4
 optID:=func(id [32]byte)[]any{return []any{uint64(1),id[:]}}
 ref:=func(kind uint64,id [32]byte)[]any{
  digest:=bytes.Repeat([]byte{byte(kind)},32)
  return []any{uint64(1),[]any{kind,id[:],digest}}
 }
 absent:=func()[]any{return []any{uint64(0)}}
 fence:=make([]any,18)
 for i:=range fence{fence[i]=[]any{}}
 fence[0]=bytes.Repeat([]byte{0},32)
 fence[1]=cycle[:]
 fence[2]=uint64(0)
 fence[6]=[]any{uint64(1),int64(100),int64(200),uint64(0)}
 loader:=func(id [32]byte)([]any,error){
  if id!=fenceID{return nil,errors.New("unexpected fence ID")}
  return fence,nil
 }
 idle:=[]any{uint64(0),absent(),absent(),absent(),absent(),absent(),uint64(0)}
 inFlight:=[]any{uint64(1),optID(cycle),[]any{uint64(1),uint64(0)},ref(11,fenceID),absent(),absent(),uint64(0)}
 retry:=[]any{uint64(2),optID(cycle),[]any{uint64(1),uint64(0)},ref(11,fenceID),ref(14,retryID),ref(13,abortID),uint64(1)}
 halt:=[]any{uint64(3),optID(cycle),[]any{uint64(1),uint64(0)},ref(11,fenceID),absent(),ref(13,abortID),uint64(1)}
 for _,test:=range []struct{name string;fields []any;pending int;want ResumeActionKind}{
  {"idle",idle,0,ResumeIdle},
  {"idle-drain",idle,2,ResumeDrainPerception},
  {"in-flight-frozen",inFlight,0,ResumeReexecuteFencedAttempt},
  {"in-flight-drain",inFlight,1,ResumeDrainPerception},
  {"retry-authorized",retry,0,ResumeFenceAuthorizedRetry},
  {"retry-drain",retry,1,ResumeDrainPerception},
  {"halt-with-pending",halt,4,ResumeHalted},
 }{
  t.Run(test.name,func(t *testing.T){
   got,err:=resumeActionForControl(test.fields,test.pending,loader)
   if err!=nil{t.Fatal(err)}
   if got.Kind!=test.want{t.Fatalf("resume %v, expected %v",got.Kind,test.want)}
   if got.Kind==ResumeReexecuteFencedAttempt{
    if got.FenceID!=fenceID||len(got.FrozenAdmissionCut)==0||len(got.FrozenCyclePlan)==0{
     t.Fatal("in-flight recovery lost immutable fence plan/closure cut")
    }
   }
   if test.want==ResumeFenceAuthorizedRetry&&got.AttemptOrdinal!=1{
    t.Fatal("retry resumed wrong authorized attempt")
   }
  })
 }
 tests:=[]struct{name string;control []any;loader func([32]byte)([]any,error)}{
  {"reselected-attempt",[]any{uint64(1),optID(cycle),[]any{uint64(1),uint64(1)},ref(11,fenceID),absent(),absent(),uint64(0)},loader},
  {"unknown-status",[]any{uint64(4),absent(),absent(),absent(),absent(),absent(),uint64(0)},loader},
  {"retry-lacks-retry-ref",[]any{uint64(2),optID(cycle),[]any{uint64(1),uint64(0)},ref(11,fenceID),absent(),ref(13,abortID),uint64(1)},loader},
  {"missing-fence-record",inFlight,func([32]byte)([]any,error){return nil,errors.New("fence missing")}},
 }
 for _,test:=range tests{
  t.Run(test.name,func(t *testing.T){
   if _,err:=resumeActionForControl(test.control,0,test.loader);err==nil{
    t.Fatal("unsafe resume direction accepted")
   }
  })
 }
}

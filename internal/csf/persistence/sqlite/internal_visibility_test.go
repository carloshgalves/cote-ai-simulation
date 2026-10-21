package sqlite

import "testing"

func TestRegisteredInvisibleEventContractsAreNotPrivacyFallbacks(t *testing.T){
 valid:=[]struct{kind,producer string;phase uint64}{
  {"clock.advanced","clock",0},
  {"resource.consumed","resource.consume",1},
  {"occurrence.rejected","schedule",2},
  {"occurrence.deferred","schedule",2},
  {"trigger.activation.consumed","trigger",2},
  {"trigger.runtime.transition","trigger",3},
  {"trigger.activation.created","trigger",4},
 }
 for _,item:=range valid{
  t.Run(item.kind,func(t *testing.T){
   event:=make([]any,19)
   for i:=range event{event[i]=[]any{}}
   event[2]=[]any{item.phase,make([]byte,32),item.producer,uint64(1),uint64(0),uint64(0)}
   event[3]=item.kind
   event[4],event[15],event[17]=uint64(1),uint64(2),uint64(1)
   event[12]=[]any{}
   event[16]=item.producer
   if err:=registeredInvisibleEngineEvent(event);err!=nil{t.Fatal("valid internal producer rejected:",err)}
   tests:=[]struct{name string;index int;value any}{
    {"PUBLIC_without_channel",15,uint64(0)},
    {"add_unsolicited_observer",12,[]any{[]any{"school","a"}}},
    {"invented_secret_Event",3,"canon.future.secret"},
    {"wrong_producer",16,"school"},
    {"wrong_version",17,uint64(2)},
    {"wrong_phase",2,[]any{uint64(7),make([]byte,32),item.producer,uint64(1),uint64(0),uint64(0)}},
    {"nonzero_local_ordinal",2,[]any{item.phase,make([]byte,32),item.producer,uint64(1),uint64(1),uint64(0)}},
   }
   for _,tc:=range tests{
    t.Run(tc.name,func(t *testing.T){
     forged:=append([]any(nil),event...)
     forged[tc.index]=tc.value
     if err:=registeredInvisibleEngineEvent(forged);err==nil{
      t.Fatal("unregistered or public event was treated as silently invisible")
     }
    })
   }
  })
 }
}

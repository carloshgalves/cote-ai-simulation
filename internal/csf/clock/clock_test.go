package clock

import (
 "errors"
 "math/rand"
 "testing"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
)
func TestOrdinalGapAndAdvance(t *testing.T) {
 start:=State{Instant:100,LastOrdinal:1}
 p,ok,err:=Next(start,[]Pending{{Eligibility:domain.EligibilityCoordinate{NotBeforeInstant:100,NotBeforeCycleOrdinal:3}}})
 if err!=nil||!ok||p.Kind!=Work||p.OriginOrdinal!=3{t.Fatalf("%+v %v %v",p,ok,err)}
 future:=[]Pending{{Eligibility:domain.EligibilityCoordinate{NotBeforeInstant:200,NotBeforeCycleOrdinal:3}}}
 p,ok,err=Next(start,future)
 if err!=nil||!ok||p.Kind!=ClockAdvance||p.OriginOrdinal!=2||p.To!=200{t.Fatalf("%+v %v",p,err)}
 floor,err:=SuccessorFloor(p)
 if err!=nil||floor.NotBeforeInstant!=200||floor.NotBeforeCycleOrdinal!=0{t.Fatal(floor,err)}
 end,err:=ApplyAdvance(start,p)
 if err!=nil||end.Instant!=200||end.LastOrdinal!=-1{t.Fatal(end,err)}
 p,ok,err=Next(end,future)
 if err!=nil||!ok||p.Kind!=Work||p.OriginOrdinal!=3{t.Fatal(p,err)}
}
func TestNewEarlierPendingInvalidatesAdvance(t *testing.T) {
 st:=State{Instant:100,LastOrdinal:1}
 old:=[]Pending{{Eligibility:domain.EligibilityCoordinate{NotBeforeInstant:200}}}
 plan,_,_:=Next(st,old)
 if err:=Confirm(st,append(old,Pending{Eligibility:domain.EligibilityCoordinate{NotBeforeInstant:150}}),plan);!errors.Is(err,ErrInvalidAdvance){t.Fatal(err)}
}
func TestMinimumOrderIndependentAndNoDecorativeCycles(t *testing.T) {
 rng:=rand.New(rand.NewSource(73))
 st:=State{Instant:100,LastOrdinal:1}
 original:=[]Pending{
 {Eligibility:domain.EligibilityCoordinate{NotBeforeInstant:240,NotBeforeCycleOrdinal:8}},
 {Eligibility:domain.EligibilityCoordinate{NotBeforeInstant:240,NotBeforeCycleOrdinal:2}},
 {Eligibility:domain.EligibilityCoordinate{NotBeforeInstant:140,NotBeforeCycleOrdinal:5}},
 }
 want,_,err:=Next(st,original);if err!=nil{t.Fatal(err)}
 for n:=0;n<300;n++{
  v:=append([]Pending(nil),original...)
  rng.Shuffle(len(v),func(i,j int){v[i],v[j]=v[j],v[i]})
  got,ok,err:=Next(st,v);if err!=nil||!ok||got!=want{t.Fatalf("%+v != %+v: %v",got,want,err)}
 }
 if _,ok,err:=Next(st,nil);ok||err!=nil{t.Fatal(ok,err)}
 if _,_,err:=Next(st,[]Pending{{Eligibility:domain.EligibilityCoordinate{NotBeforeInstant:100,NotBeforeCycleOrdinal:1}}});!errors.Is(err,ErrStalePending){t.Fatal(err)}
}

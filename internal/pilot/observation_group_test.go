package pilot
import "testing"
func TestGroupedObservationAudience(t *testing.T){
 j:=autonomyJournal(t);j.Version=ObservationVersion;s,err:=initial(j);if err!=nil{t.Fatal(err)}
 for _,id:=range ids(j.Profiles){m:=s.Minds[id];m.Location="park";m.Activity=nil;s.Minds[id]=m}
 for _,id:=range []string{"ayanokoji","horikita","kushida"}{m:=s.Minds[id];m.Location="hall";s.Minds[id]=m}
 out:=clone(s);n:=len(out.Events);publishMovementObservation(j,&out,s,"ayanokoji","hall",false)
 if len(out.Events)!=n+1{t.Fatal("observation not grouped")}
 e:=out.Events[n];if len(e.Audience)!=2||!has(e.Audience,"horikita")||!has(e.Audience,"kushida"){t.Fatal("wrong ACL",e.Audience)}
 for _,event:=range actorContext(j,out,"ryuen").Memory{if event.ID==e.ID{t.Fatal("outsider received observation")}}
 for _,event:=range actorContext(j,out,"horikita").Memory{if event.ID==e.ID&&len(event.Audience)!=1{t.Fatal("other observer exposed")}}
}

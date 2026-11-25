package pilot

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)
func addTestRoute(t *testing.T,r *Router,provider,model,key string,handler func(*http.Request)(*http.Response,error)) RouteView {
	t.Helper()
	v,err:=r.Add(RouteInput{Provider:provider,Model:model,Key:key,Format:"auto"});if err!=nil{t.Fatal(err)}
	r.routes[len(r.routes)-1].model.Client.Transport=roundTripperFunc(handler)
	return v
}
func response(status int,body string,header http.Header)*http.Response{
	if header==nil{header=make(http.Header)}
	return &http.Response{StatusCode:status,Body:io.NopCloser(strings.NewReader(body)),Header:header}
}
const validChoiceResponse = `{"choices":[{"message":{"content":"{\"choice\":\"talk:hirata\",\"text\":\"Bom dia, Hirata-kun.\",\"reason\":\"Quero conhecê-lo.\"}"},"finish_reason":"stop"}]}`
func kushidaView(t *testing.T)Context{
	t.Helper();j:=Journal{Version:Version,Model:"test"};j.Profiles,_=decodeStrict[[]Profile](defaultProfiles)
	s,err:=initial(j);if err!=nil{t.Fatal(err)}
	return actorContext(j,s,"kushida")
}
func TestRouterQuotaFallbackSharedModelAndProvenance(t *testing.T){
	r:=NewRouter();calls:=0;second:=0;fallback:=0
	first:=addTestRoute(t,r,"groq","same-model","key-one",func(req *http.Request)(*http.Response,error){calls++;return response(429,"{}",http.Header{"Retry-After":[]string{"90"}}),nil})
	addTestRoute(t,r,"groq","same-model","key-two",func(req *http.Request)(*http.Response,error){second++;return nil,errors.New("must not probe same-model quota")})
	addTestRoute(t,r,"gemini","backup","gemini-key",func(req *http.Request)(*http.Response,error){fallback++;if req.Header.Get("Authorization")!="Bearer gemini-key"{t.Error("credential mismatch")};return response(200,validChoiceResponse,nil),nil})
	view:=kushidaView(t)
	for i:=0;i<2;i++{d,err:=r.Decide(context.Background(),view);if err!=nil||d.Source!="gemini / backup"||d.Target!="hirata"{t.Fatalf("fallback failed: %+v %v",d,err)}}
	if calls!=1||second!=0||fallback!=2{t.Fatal("quota cooldown not respected")}
	views:=r.Routes();if views[0].ID!=first.ID||views[0].RetryAt.Before(time.Now().Add(80*time.Second))||views[1].RetryAt.IsZero(){t.Fatal("retry-after not shared")}
	raw,_:=json.Marshal(views);if strings.Contains(string(raw),"key-one")||strings.Contains(string(raw),"gemini-key"){t.Fatal("key leaked")}
}
func TestRouterRepairThenFallbackAndCancellation(t *testing.T){
	r:=NewRouter();calls:=0;fallback:=0
	addTestRoute(t,r,"groq","invalid","test",func(req *http.Request)(*http.Response,error){calls++;return response(200,`{"choices":[{"message":{"content":"{\"choice\":\"talk:hirata\",\"text\":\"Hirata-senpai, bom dia\"}"},"finish_reason":"stop"}]}`,nil),nil})
	addTestRoute(t,r,"anthropic","claude-test","secret",func(req *http.Request)(*http.Response,error){
		fallback++;if req.URL.Path!="/v1/messages"||req.Header.Get("x-api-key")!="secret"||req.Header.Get("Authorization")!=""{t.Error("wrong Anthropic protocol")}
		return response(200,`{"stop_reason":"end_turn","content":[{"type":"text","text":"{\"choice\":\"talk:hirata\",\"text\":\"Oi, Hirata!\",\"reason\":\"Quero conversar.\"}"}]}`,nil),nil
	})
	d,err:=r.Decide(context.Background(),kushidaView(t));if err!=nil||d.Source!="anthropic / claude-test"||calls!=2||fallback!=1{t.Fatalf("repair/fallback: %+v %v %d %d",d,err,calls,fallback)}
	ctx,cancel:=context.WithCancel(context.Background());cancel()
	if _,err:=r.Decide(ctx,kushidaView(t));!errors.Is(err,context.Canceled)||fallback!=1{t.Fatal("cancellation not respected")}
}
func TestRouterSaveReloadPrivateAndOrder(t *testing.T){
	r:=NewRouter()
	a,err:=r.Add(RouteInput{Provider:"groq",Model:"one",Key:"private-one"});if err!=nil{t.Fatal(err)}
	b,err:=r.Add(RouteInput{Provider:"gemini",Model:"two",Key:"private-two"});if err!=nil{t.Fatal(err)}
	if err=r.Change(b.ID,"up");err!=nil{t.Fatal(err)}
	if err=r.Change(a.ID,"toggle");err!=nil{t.Fatal(err)}
	path:=filepath.Join(t.TempDir(),"settings","routes.json")
	if err=r.Save(path);err!=nil{t.Fatal(err)}
	info,err:=os.Stat(path);if err!=nil||info.Mode().Perm()!=0600{t.Fatal("insecure config")}
	reloaded,err:=LoadRouter(path);if err!=nil{t.Fatal(err)}
	v:=reloaded.Routes();if len(v)!=2||v[0].Model!="two"||v[1].Enabled||reloaded.routes[0].model.Key!="private-two"{t.Fatal("config not preserved")}
	if err=os.Chmod(path,0644);err!=nil{t.Fatal(err)}
	if _,err=LoadRouter(path);err==nil{t.Fatal("exposed config accepted")}
}
func TestCanonProjectionPrivateAndPeerHonorific(t *testing.T){
	v:=kushidaView(t)
	if v.SchoolYear!=1||v.OwnCanon.SchoolYear!=1||v.OwnCanon.ClassRef!="entity.class.y1-d"||!strings.Contains(v.OwnCanon.Source,"kikyo-kushida"){t.Fatal("missing pack context")}
	raw,_:=json.Marshal(v.OwnCanon);if strings.Contains(string(raw),"White Room")||strings.Contains(string(raw),"Hirata privately"){t.Fatal("foreign secrets leaked")}
	if len(v.OwnCanon.PrivateKnowledge)==0||len(v.OwnCanon.Dispositions)==0{t.Fatal("missing own evidence")}
	if _,err:=decodeLocalChoice(v,[]byte(`{"choice":"talk:hirata","text":"Hirata-senpai, bom dia!"}`));err==nil{t.Fatal("incorrect honorific admitted")}
	if _,err:=decodeLocalChoice(v,[]byte(`{"choice":"talk:hirata","text":"Hirata-kun, você conhece algum senpai?"}`));err!=nil{t.Fatal("legitimate mention rejected",err)}
	if !reflect.DeepEqual(canonFor("ryuen").PrivateKnowledge,[]string{})&&len(canonFor("ryuen").PrivateKnowledge)!=0{t.Fatal("future knowledge leaked")}
}

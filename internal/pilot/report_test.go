package pilot

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReportContainsAllCyclesAndOnlyAddressedActorEvents(t *testing.T){
	m:=modelFunc(func(_ context.Context,v Context)(Decision,error){
		if v.Self.ID=="kushida"{return Decision{Action:"talk",Target:"hirata",Text:"Mensagem privada de teste",Reason:"Intenção privada de Kushida",Source:"test / model"},nil}
		return wait(),nil
	})
	s,err:=Open(filepath.Join(t.TempDir(),"report.json"),"test",m);if err!=nil{t.Fatal(err)};defer s.Close()
	for i:=0;i<2;i++{if _,err:=s.Step(context.Background(),i);err!=nil{t.Fatal(err)}}
	before:=s.Snapshot()
	raw,err:=s.ReportZIP();if err!=nil{t.Fatal(err)}
	if s.Snapshot().Digest!=before.Digest{t.Fatal("export mutated journal")}
	z,err:=zip.NewReader(bytes.NewReader(raw),int64(len(raw)));if err!=nil{t.Fatal(err)}
	files:=map[string][]byte{}
	for _,entry:=range z.File{f,err:=entry.Open();if err!=nil{t.Fatal(err)};b,err:=io.ReadAll(f);f.Close();if err!=nil{t.Fatal(err)};files[entry.Name]=b}
	if len(files)!=15{t.Fatalf("expected 15 files, got %d",len(files))}
	var op OperatorReport
	if err:=json.Unmarshal(files["operador.json"],&op);err!=nil{t.Fatal(err)}
	if op.Digest!=before.Digest||len(op.Journal.Rounds)!=2||len(op.State.Events)!=len(before.State.Events){t.Fatal("operator report incomplete")}
	for _,id:=range []string{"kushida","hirata","ayanokoji"}{
		var a ActorReport
		if err:=json.Unmarshal(files["personagens/"+id+".json"],&a);err!=nil{t.Fatal(err)}
		if len(a.Cycles)!=2{t.Fatal("missing cycle")}
		for _,event:=range a.Events{if !has(event.Audience,id){t.Fatal("foreign event exposed")}}
		for _,cycle:=range a.Cycles{if cycle.Choice.Actor!=id{t.Fatal("foreign choice exposed")};for _,event:=range cycle.AvailableContext.Memory{if !has(event.Audience,id){t.Fatal("foreign context exposed")}}}
		content:=string(files["personagens/"+id+".json"])
		if id=="ayanokoji"&&strings.Contains(content,"Mensagem privada de teste"){t.Fatal("private dialogue leaked")}
		if id=="hirata"&&strings.Contains(content,"Intenção privada de Kushida"){t.Fatal("private intention leaked")}
		if id!="ayanokoji"&&!strings.Contains(content,"Mensagem privada de teste"){t.Fatal("addressed dialogue missing")}
	}
}
func TestQuotaTelemetryAndPreventiveFallback(t *testing.T){
	r:=NewRouter();first:=0;backup:=0
	addTestRoute(t,r,"groq","limited","secret",func(req *http.Request)(*http.Response,error){
		first++
		h:=make(http.Header);h.Set("x-ratelimit-remaining-tokens","0");h.Set("x-ratelimit-reset-tokens","45s");h.Set("x-ratelimit-remaining-requests","88")
		body:=strings.TrimSuffix(validChoiceResponse,"}")+",\"usage\":{\"prompt_tokens\":100,\"completion_tokens\":20,\"total_tokens\":120}}"
		return response(200,body,h),nil
	})
	addTestRoute(t,r,"groq","backup","other",func(req *http.Request)(*http.Response,error){backup++;return response(200,validChoiceResponse,nil),nil})
	for i:=0;i<2;i++{if _,err:=r.Decide(context.Background(),kushidaView(t));err!=nil{t.Fatal(err)}}
	v:=r.Routes()[0];calls,dropped:=r.Telemetry()
	if first!=1||backup!=1||len(calls)!=2||dropped!=0||v.PromptTokens!=100||v.CompletionTokens!=20||v.UsageReports!=1||v.RetryAt.Before(time.Now().Add(35*time.Second)){t.Fatalf("bad quota handling: %+v",v)}
	if calls[0].HTTPStatus!=200||!calls[0].Accepted||calls[0].Usage==nil||calls[0].Quota.TokensRemaining==nil||*calls[0].Quota.TokensRemaining!=0{t.Fatal("missing observation")}
	raw,_:=json.Marshal(calls);if strings.Contains(string(raw),"secret"){t.Fatal("credential leaked")}
}
func TestUnknownQuotaDoesNotBecomeZero(t *testing.T){
	q:=readQuota(make(http.Header),time.Now(),true)
	if q.Observed||q.TokensRemaining!=nil||!q.ExhaustedUntil(time.Now()).IsZero(){t.Fatal("absent quota treated as exhausted")}
	if readUsage([]byte(`{"usage":{}}`))!=nil{t.Fatal("unknown usage treated as zero")}
}

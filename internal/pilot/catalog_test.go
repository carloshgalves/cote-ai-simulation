package pilot

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestGeminiCatalogPaginationAndFilter(t *testing.T){
	calls:=0
	client:=&http.Client{Transport:roundTripperFunc(func(r *http.Request)(*http.Response,error){
		calls++
		if r.Method!="GET"||r.URL.Host!="generativelanguage.googleapis.com"||r.URL.Path!="/v1beta/models"||r.Header.Get("x-goog-api-key")!="secret"||strings.Contains(r.URL.String(),"secret"){t.Fatal("wrong Gemini request")}
		if calls==1{return response(200,`{"models":[{"name":"models/gemini-flash","displayName":"Flash","supportedGenerationMethods":["generateContent"]},{"name":"models/embedding-test","supportedGenerationMethods":["embedContent"]}],"nextPageToken":"next"}`,nil),nil}
		if r.URL.Query().Get("pageToken")!="next"{t.Fatal("missing pagination")}
		return response(200,`{"models":[{"name":"models/gemini-pro","supportedGenerationMethods":["generateContent"]},{"name":"models/gemini-flash","displayName":"Flash","supportedGenerationMethods":["generateContent"]},{"name":"models/imagen-test","supportedGenerationMethods":["generateContent"]}]}`,nil),nil
	})}
	out,err:=discoverModels(context.Background(),RouteInput{Provider:"gemini",Key:"secret"},client)
	if err!=nil||calls!=2||out.Partial||len(out.Models)!=2||out.Models[0].ID!="gemini-flash"||out.Models[1].ID!="gemini-pro"{t.Fatalf("bad catalog: %+v %v",out,err)}
	raw,_:=json.Marshal(out);if strings.Contains(string(raw),"secret"){t.Fatal("secret returned")}
}
func TestCatalogCompatibleAndAnthropic(t *testing.T){
	for _,provider:=range []string{"groq","ollama","anthropic","openrouter"}{
		t.Run(provider,func(t *testing.T){
			in,_,err:=normalizeRoute(RouteInput{Provider:provider,Key:map[bool]string{true:"",false:"secret"}[provider=="ollama"]},false);if err!=nil{t.Fatal(err)}
			client:=&http.Client{Transport:roundTripperFunc(func(r *http.Request)(*http.Response,error){
				if !strings.HasSuffix(r.URL.Path,"/models"){t.Fatal("wrong endpoint")}
				if provider=="anthropic"{
					if r.Header.Get("x-api-key")!="secret"||r.Header.Get("anthropic-version")==""{t.Fatal("wrong Anthropic auth")}
				}else if provider!="ollama"&&r.Header.Get("Authorization")!="Bearer secret"{t.Fatal("wrong bearer auth")}
				return response(200,`{"data":[{"id":"chat","display_name":"Chat model"},{"id":"whisper-test"},{"id":"inactive","active":false},{"id":"free:free","pricing":{"prompt":"0","completion":"0"}}]}`,nil),nil
			})}
			out,err:=discoverModels(context.Background(),in,client)
			if err!=nil||len(out.Models)!=2{t.Fatalf("bad catalog %+v %v",out,err)}
			if provider=="openrouter"&&(out.Models[0].ID!="free:free"||out.Models[0].PriceHint==""){t.Fatal("missing provider price hint")}
		})
	}
}
func TestCatalogErrorsDoNotEchoCredentials(t *testing.T){
	for _,status:=range []int{401,429,404,500}{
		client:=&http.Client{Transport:roundTripperFunc(func(r *http.Request)(*http.Response,error){return response(status,"secret",nil),nil})}
		_,err:=discoverModels(context.Background(),RouteInput{Provider:"groq",Endpoint:providerEndpoints["groq"],Key:"secret"},client)
		if err==nil||strings.Contains(err.Error(),"secret"){t.Fatal("unsafe error")}
	}
	if _,_,err:=normalizeRoute(RouteInput{Provider:"openai-compatible",Endpoint:"http://example.com",Key:"secret"},false);err==nil{t.Fatal("unsafe endpoint accepted")}
	if _,_,err:=normalizeRoute(RouteInput{Provider:"gemini"},false);err==nil{t.Fatal("missing credential accepted")}
}

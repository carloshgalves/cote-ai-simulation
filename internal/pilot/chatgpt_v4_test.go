package pilot

import (
 "context"
 "crypto"
 "crypto/rand"
 "crypto/rsa"
 "crypto/sha256"
 "encoding/base64"
 "encoding/json"
 "net/http"
 "net/url"
 "path/filepath"
 "reflect"
 "strings"
 "testing"
 "time"
)
func TestChatGPTAuthorizationStateAndPKCE(t *testing.T){
 a,err:=OpenChatGPT(filepath.Join(t.TempDir(),"chatgpt.json"));if err!=nil{t.Fatal(err)};defer a.Close()
 raw,err:=a.Start("http://127.0.0.1:8765/auth/callback");if err!=nil{t.Fatal(err)}
 u,err:=url.Parse(raw);if err!=nil{t.Fatal(err)};q:=u.Query()
 if u.Host!="auth.openai.com"||q.Get("client_id")!="dynamic_agent_client"||q.Get("agent_name_hint")!="COTE AI Simulation"||q.Get("resource")!=chatResource||q.Get("state")==""||q.Get("nonce")==""{t.Fatal("incorrect registration")}
 sum:=sha256.Sum256([]byte(a.pending.Verifier))
 if q.Get("code_challenge")!=base64.RawURLEncoding.EncodeToString(sum[:])||q.Get("code_challenge_method")!="S256"{t.Fatal("incorrect PKCE")}
 if err:=a.Complete(context.Background(),url.Values{"state":{"wrong"},"code":{"secret"}});err==nil{t.Fatal("wrong state accepted")}
 if err:=a.Complete(context.Background(),url.Values{"state":{q.Get("state")},"error":{"access_denied"}});err==nil||a.pending!=nil{t.Fatal("denial not consumed")}
}
func TestChatGPTIDVerification(t *testing.T){
 key,err:=rsa.GenerateKey(rand.Reader,2048);if err!=nil{t.Fatal(err)}
 enc:=base64.RawURLEncoding.EncodeToString
 jwks,_:=json.Marshal(map[string]any{"keys":[]any{map[string]string{"kty":"RSA","kid":"test","alg":"RS256","n":enc(key.N.Bytes()),"e":enc([]byte{1,0,1})}}})
 now:=time.Now()
 header:=enc([]byte(`{"alg":"RS256","kid":"test"}`))
 claims,_:=json.Marshal(map[string]any{"iss":chatIssuer,"aud":"oaiapp_test","sub":"test-user","nonce":"nonce","exp":now.Add(time.Hour).Unix()})
 message:=header+"."+enc(claims);sum:=sha256.Sum256([]byte(message));sig,err:=rsa.SignPKCS1v15(rand.Reader,key,crypto.SHA256,sum[:]);if err!=nil{t.Fatal(err)}
 jwt:=message+"."+enc(sig)
 if _,err=verifyChatID(jwt,jwks,"oaiapp_test","nonce",now);err!=nil{t.Fatal(err)}
 for _,test:=range []struct{client,nonce,token string}{{"oaiapp_other","nonce",jwt},{"oaiapp_test","wrong",jwt},{"oaiapp_test","nonce",message+"."+enc([]byte("bad"))}}{
  if _,err=verifyChatID(test.token,jwks,test.client,test.nonce,now);err==nil{t.Fatal("invalid token accepted")}
 }
}
func TestChatGPTStreamRequiresCompletedAndRecognizesQuota(t *testing.T){
 v:=kushidaView(t)
 delta,_:=json.Marshal(map[string]string{"type":"response.output_text.delta","delta":`{"choice":"talk:hirata","text":"Oi!","reason":"Conhecê-lo."}`})
 stream:="data: "+string(delta)+"\n\n"
 var tlog CallTelemetry
 if _,err:=readChatStream(v,strings.NewReader(stream),&tlog);err==nil{t.Fatal("unfinished stream accepted")}
 completed:=stream+"data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":20,\"output_tokens\":10}}}\n\n"
 d,err:=readChatStream(v,strings.NewReader(completed),&tlog)
 if err!=nil||d.Target!="hirata"||tlog.Usage==nil||tlog.Usage.Input!=20{t.Fatal("completed response not accepted",err)}
 if _,err=readChatStream(v,strings.NewReader(stream+"data: {\"type\":\"response.failed\",\"response\":{\"error\":{\"code\":\"subscription_sharing_usage_limit_exceeded\"}}}\n\n"),nil);err==nil{t.Fatal("quota stream accepted")}
}
func TestBudgetAvoidsPositiveButInsufficientQuota(t *testing.T){
 r:=NewRouter();calls:=0;backup:=0
 addTestRoute(t,r,"groq","limited","secret",func(req *http.Request)(*http.Response,error){
  calls++;h:=make(http.Header);h.Set("x-ratelimit-remaining-tokens","10");h.Set("x-ratelimit-reset-tokens","50s")
  return response(200,validChoiceResponse,h),nil
 })
 addTestRoute(t,r,"gemini","backup","secret",func(req *http.Request)(*http.Response,error){backup++;return response(200,validChoiceResponse,nil),nil})
 for i:=0;i<2;i++{if _,err:=r.Decide(context.Background(),kushidaView(t));err!=nil{t.Fatal(err)}}
 if calls!=1||backup!=1||r.Routes()[0].Prevented!=1{t.Fatal("preventive budget failed")}
 if generationBudget("ernie",true,false)<generationBudget("ernie",false,false)||generationBudget("ernie",true,true)<=generationBudget("ernie",false,false){t.Fatal("truncation recovery shrank budget")}
}
func TestV4ConversationTurnAndPhysicalPresence(t *testing.T){
 j:=scheduledJournal(t);j.Version=ConversationVersion;s,_:=initial(j)
 next,err:=scheduledAdvance(j,s,chooseFor(j,s,map[string]Decision{"hirata":{Action:"talk",Target:"kushida",Text:"Bom dia."}}));if err!=nil{t.Fatal(err)}
 if !reflect.DeepEqual(eligibleActors(j,next),[]string{"kushida"}){t.Fatal("both speakers awakened",eligibleActors(j,next))}
 v:=actorContext(j,next,"kushida")
 if v.Conversation==nil||v.Conversation.Peer!="hirata"||!has(v.Contactable,"hirata"){t.Fatal("reply not available")}
 outsider:=actorContext(j,next,"ryuen");present:=false;for _,p:=range outsider.Present{if p.ID=="hirata"{present=true}}
 if !present||has(outsider.Contactable,"hirata"){t.Fatal("physical presence conflated with availability")}
 next,err=scheduledAdvance(j,next,chooseFor(j,next,map[string]Decision{"kushida":{Action:"talk",Target:"hirata",Text:"Bom dia, Hirata-kun."}}));if err!=nil{t.Fatal(err)}
 if next.Minds["hirata"].Conversation==nil||next.Minds["hirata"].Conversation.Turns!=2||!reflect.DeepEqual(eligibleActors(j,next),[]string{"hirata"}){t.Fatal("conversation not continued")}
 next,err=scheduledAdvance(j,next,chooseFor(j,next,nil));if err!=nil{t.Fatal(err)}
 if next.Minds["kushida"].Conversation!=nil||next.Minds["hirata"].Conversation!=nil{t.Fatal("conversation not closed")}
}
func TestV4BeliefGroundingAndPlanContinuity(t *testing.T){
 j:=scheduledJournal(t);j.Version=ConversationVersion;s,_:=initial(j)
 v:=actorContext(j,s,"kushida")
 e:=v.Memory[0]
 d:=Decision{Action:"wait",Beliefs:[]Belief{{Text:"Hirata lidera",Subject:"hirata",Evidence:e.ID,Grounding:"Primeiro dia",Confidence:60}}}
 if validateDecision(v,d)==nil{t.Fatal("announcement used as evidence about colleague")}
 d.Beliefs[0].Subject="";d.Beliefs[0].Grounding="texto inventado"
 if validateDecision(v,d)==nil{t.Fatal("fabricated excerpt accepted")}
 d.Beliefs[0].Grounding="Primeiro dia";if err:=validateDecision(v,d);err!=nil{t.Fatal(err)}
 v.Plan=&PlanChange{Steps:[]string{"A"},Evidence:e.ID}
 d.Plan=&PlanChange{Steps:[]string{"B"},Evidence:e.ID}
 if validateDecision(v,d)==nil{t.Fatal("unexplained plan replacement accepted")}
 d.Plan.ProgressNote="Reorganizar após a orientação";if err:=validateDecision(v,d);err!=nil{t.Fatal(err)}
}

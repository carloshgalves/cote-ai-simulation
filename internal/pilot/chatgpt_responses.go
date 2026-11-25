package pilot

import (
 "bufio"
 "bytes"
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "net/http"
 "strings"
 "time"
)

func(m *HTTPModel) chatDecision(ctx context.Context,view Context,prompt,input string)(Decision,error){
 if m.chat==nil{return Decision{},errors.New("conecte a conta ChatGPT")}
 token,err:=m.chat.Token(ctx);if err!=nil{return Decision{},err}
 body,err:=json.Marshal(map[string]any{"model":m.Name,"instructions":prompt+" Responda somente um objeto JSON curto.","input":[]map[string]string{{"role":"user","content":input}},"store":false,"stream":true});if err!=nil{return Decision{},err}
 req,err:=http.NewRequestWithContext(ctx,"POST",chatResource+"/responses",bytes.NewReader(body));if err!=nil{return Decision{},err}
 req.Header.Set("Authorization","Bearer "+token);req.Header.Set("Content-Type","application/json");req.Header.Set("Accept","text/event-stream")
 res,err:=m.Client.Do(req);if err!=nil{return Decision{},errors.New("ChatGPT indisponível ou timeout")};defer res.Body.Close()
 if m.telemetry!=nil{m.telemetry.HTTPStatus=res.StatusCode;m.telemetry.Quota=readQuota(res.Header,time.Now(),false)}
 if res.StatusCode!=200{return Decision{},providerError{Status:res.StatusCode,RetryAt:retryAt(res.Header.Get("Retry-After"),time.Now()),Message:fmt.Sprintf("ChatGPT retornou HTTP %d",res.StatusCode)}}
 return readChatStream(view,res.Body,m.telemetry)
}
func readChatStream(view Context,body io.Reader,t *CallTelemetry)(Decision,error){
 scanner:=bufio.NewScanner(io.LimitReader(body,4<<20));scanner.Buffer(make([]byte,4096),2<<20)
 var text strings.Builder
 for scanner.Scan(){
  line:=scanner.Text();if !strings.HasPrefix(line,"data:"){continue}
  raw:=strings.TrimSpace(strings.TrimPrefix(line,"data:"));if raw=="[DONE]"{break}
  var event struct{Type,Delta,Code string;Response json.RawMessage}
  if json.Unmarshal([]byte(raw),&event)!=nil{return Decision{},responseError{"evento Responses inválido"}}
  switch event.Type{
  case "response.output_text.delta":text.WriteString(event.Delta);if text.Len()>1<<20{return Decision{},responseError{"resposta longa demais"}}
  case "response.completed":
   if t!=nil{t.Usage=readUsage(event.Response)}
   return decodeLocalChoice(view,unwrapJSON(text.String()))
  case "response.failed","error":
   if t!=nil{t.Usage=readUsage(event.Response)}
   var failure struct{Error struct{Code string}}
   _=json.Unmarshal(event.Response,&failure);code:=failure.Error.Code;if code==""{code=event.Code}
   if code=="subscription_sharing_usage_limit_exceeded"||code=="subscription_sharing_usage_unavailable"{return Decision{},providerError{Status:429,RetryAt:time.Now().Add(5*time.Minute),Message:"limite ou permissão de uso do plano ChatGPT indisponível; confira Configurações → Uso"}}
   return Decision{},errors.New("Responses API encerrou com falha")
  case "response.incomplete":if t!=nil{t.Usage=readUsage(event.Response)};return Decision{},responseError{"resposta ChatGPT incompleta"}
  }
 }
 return Decision{},errors.New("stream ChatGPT terminou sem response.completed")
}

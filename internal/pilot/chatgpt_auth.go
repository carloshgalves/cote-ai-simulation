package pilot

import (
 "context"
 "crypto"
 "crypto/rand"
 "crypto/rsa"
 "crypto/sha256"
 "encoding/base64"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "math/big"
 "net/http"
 "net/url"
 "os"
 "path/filepath"
 "strings"
 "sync"
 "syscall"
 "time"
)
const chatIssuer="https://auth.openai.com"
const chatResource="https://api.openai.com/v1"
const chatScope="openid profile email offline_access resource.invoke chatgpt.tokens.use.direct"
type chatTokens struct {
 Access string `json:"access_token"`
 Refresh string `json:"refresh_token"`
 ID string `json:"id_token"`
 Scope string `json:"scope"`
 Expires int64 `json:"expires_in"`
 Type string `json:"token_type"`
}
type chatRecord struct {
 Host string
 Client string
 Subject string
 Email string
 Tokens chatTokens
 Expiry time.Time
}
type chatPending struct {State,Nonce,Verifier,Redirect,Client,Subject string; Expires time.Time}
type ChatGPTAccount struct {
 mu sync.Mutex
 path string
 lock *os.File
 record chatRecord
 pending *chatPending
 client *http.Client
}
func randomSecret()(string,error){b:=make([]byte,32);if _,err:=rand.Read(b);err!=nil{return "",err};return base64.RawURLEncoding.EncodeToString(b),nil}
func OpenChatGPT(path string)(*ChatGPTAccount,error){
 if err:=os.MkdirAll(filepath.Dir(path),0700);err!=nil{return nil,err}
 lock,err:=os.OpenFile(path+".lock",os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW,0600);if err!=nil{return nil,err}
 if err=syscall.Flock(int(lock.Fd()),syscall.LOCK_EX|syscall.LOCK_NB);err!=nil{lock.Close();return nil,errors.New("conta ChatGPT já aberta em outro processo")}
 a:=&ChatGPTAccount{path:path,lock:lock,client:&http.Client{Timeout:25*time.Second,CheckRedirect:func(*http.Request,[]*http.Request)error{return http.ErrUseLastResponse}}}
 info,err:=os.Lstat(path)
 if err==nil{
  if !info.Mode().IsRegular()||info.Mode().Perm()&0077!=0||info.Size()>256<<10{a.Close();return nil,errors.New("arquivo de credenciais inválido")}
  raw,err:=os.ReadFile(path);if err!=nil||json.Unmarshal(raw,&a.record)!=nil||a.record.Host==""{a.Close();return nil,errors.New("configuração ChatGPT inválida")}
 }else if errors.Is(err,os.ErrNotExist){
  b:=make([]byte,16);if _,err=rand.Read(b);err!=nil{a.Close();return nil,err}
  b[6]=(b[6]&15)|64;b[8]=(b[8]&63)|128
  a.record.Host=fmt.Sprintf("urn:uuid:%x-%x-%x-%x-%x",b[:4],b[4:6],b[6:8],b[8:10],b[10:])
  if err=a.save(a.record);err!=nil{a.Close();return nil,err}
 }else{a.Close();return nil,err}
 return a,nil
}
func(a *ChatGPTAccount) Close()error{return a.lock.Close()}
func(a *ChatGPTAccount) save(r chatRecord)error{
 raw,err:=json.Marshal(r);if err!=nil{return err}
 f,err:=os.CreateTemp(filepath.Dir(a.path),".chatgpt-");if err!=nil{return err};defer os.Remove(f.Name())
 _,err=f.Write(raw);if err==nil{err=f.Sync()};ce:=f.Close();if err!=nil{return err};if ce!=nil{return ce}
 if err=os.Rename(f.Name(),a.path);err!=nil{return err};a.record=r;return nil
}
func(a *ChatGPTAccount) Status()map[string]any{
 a.mu.Lock();defer a.mu.Unlock()
 connected:=a.record.Tokens.Refresh!=""&&has(strings.Fields(a.record.Tokens.Scope),"chatgpt.tokens.use.direct")
 return map[string]any{"connected":connected,"email":a.record.Email,"message":"Autorize o uso do plano ChatGPT no navegador."}
}
func(a *ChatGPTAccount) Start(redirect string)(string,error){
 a.mu.Lock();defer a.mu.Unlock()
 state,err:=randomSecret();if err!=nil{return "",err};nonce,err:=randomSecret();if err!=nil{return "",err};verifier,err:=randomSecret();if err!=nil{return "",err}
 p:=&chatPending{state,nonce,verifier,redirect,a.record.Client,a.record.Subject,time.Now().Add(10*time.Minute)}
 sum:=sha256.Sum256([]byte(verifier))
 q:=url.Values{"client_id":{p.Client},"response_type":{"code"},"redirect_uri":{redirect},"scope":{chatScope},"resource":{chatResource},"state":{state},"nonce":{nonce},"code_challenge_method":{"S256"},"code_challenge":{base64.RawURLEncoding.EncodeToString(sum[:])},"ext_agent_host_id":{a.record.Host}}
 if p.Client==""{q.Set("client_id","dynamic_agent_client");q.Set("agent_name_hint","COTE AI Simulation")}else if a.record.Tokens.ID!=""{q.Set("id_token_hint",a.record.Tokens.ID)}
 a.pending=p
 return chatIssuer+"/api/accounts/authorize?"+q.Encode(),nil
}
func(a *ChatGPTAccount) request(ctx context.Context,method,endpoint,token string,form url.Values)([]byte,error){
 var body io.Reader
 if form!=nil{body=strings.NewReader(form.Encode())}
 req,err:=http.NewRequestWithContext(ctx,method,endpoint,body);if err!=nil{return nil,errors.New("requisição OAuth inválida")}
 if form!=nil{req.Header.Set("Content-Type","application/x-www-form-urlencoded")}
 if token!=""{req.Header.Set("Authorization","Bearer "+token)}
 res,err:=a.client.Do(req);if err!=nil{return nil,errors.New("serviço de autenticação indisponível")};defer res.Body.Close()
 raw,err:=io.ReadAll(io.LimitReader(res.Body,4<<20+1));if err!=nil||len(raw)>4<<20{return nil,errors.New("resposta inválida")}
 if res.StatusCode!=200{return nil,fmt.Errorf("OpenAI retornou HTTP %d",res.StatusCode)}
 return raw,nil
}
func(a *ChatGPTAccount) exchange(ctx context.Context,form url.Values)(chatTokens,error){
 raw,err:=a.request(ctx,"POST",chatIssuer+"/api/accounts/oauth/token","",form);if err!=nil{return chatTokens{},err}
 var t chatTokens
 if json.Unmarshal(raw,&t)!=nil||t.Access==""||t.Refresh==""||!strings.EqualFold(t.Type,"bearer")||t.Expires<=0||t.Expires>86400{return t,errors.New("credenciais OAuth incompletas")}
 return t,nil
}
func(a *ChatGPTAccount) Complete(ctx context.Context,q url.Values)error{
 a.mu.Lock();defer a.mu.Unlock()
 p:=a.pending
 if p==nil||p.State!=q.Get("state")||time.Now().After(p.Expires){return errors.New("autorização expirada ou estado inválido")}
 a.pending=nil
 if q.Get("error")!=""{return errors.New("autorização não concedida")}
 client:=q.Get("client_id");if p.Client!=""{if client!=""&&client!=p.Client{return errors.New("cliente OAuth divergente")};client=p.Client}
 if !strings.HasPrefix(client,"oaiapp_")||q.Get("code")==""{return errors.New("registro OAuth incompleto")}
 t,err:=a.exchange(ctx,url.Values{"grant_type":{"authorization_code"},"client_id":{client},"code":{q.Get("code")},"code_verifier":{p.Verifier},"redirect_uri":{p.Redirect},"resource":{chatResource}});if err!=nil{return err}
 raw,err:=a.request(ctx,"GET",chatIssuer+"/.well-known/jwks.json","",nil);if err!=nil{return err}
 id,err:=verifyChatID(t.ID,raw,client,p.Nonce,time.Now());if err!=nil{return err}
 if p.Subject!=""&&p.Subject!=id.Sub{return errors.New("conta diferente da registrada")}
 if !has(strings.Fields(t.Scope),"chatgpt.tokens.use.direct")||!has(strings.Fields(t.Scope),"resource.invoke"){return errors.New("uso do plano ChatGPT não autorizado")}
 return a.save(chatRecord{Host:a.record.Host,Client:client,Subject:id.Sub,Email:id.Email,Tokens:t,Expiry:time.Now().Add(time.Duration(t.Expires)*time.Second)})
}
type chatIdentity struct{Sub string `json:"sub"`; Email string `json:"email"`; Iss string `json:"iss"`; Aud json.RawMessage `json:"aud"`; Nonce string `json:"nonce"`; Exp int64 `json:"exp"`; Azp string `json:"azp"`}
func verifyChatID(token string,keys []byte,client,nonce string,now time.Time)(chatIdentity,error){
 bad:=errors.New("identidade OpenAI não verificada")
 var id chatIdentity
 parts:=strings.Split(token,".");if len(parts)!=3{return id,bad}
 dec:=base64.RawURLEncoding.DecodeString
 h,err:=dec(parts[0]);if err!=nil{return id,bad};p,err:=dec(parts[1]);if err!=nil{return id,bad};sig,err:=dec(parts[2]);if err!=nil{return id,bad}
 var header struct{Alg,Kid string};var jwks struct{Keys []struct{Kty,Kid,N,E,Alg,Use string}}
 if json.Unmarshal(h,&header)!=nil||header.Alg!="RS256"||header.Kid==""||json.Unmarshal(keys,&jwks)!=nil{return id,bad}
 verified:=false;sum:=sha256.Sum256([]byte(parts[0]+"."+parts[1]))
 for _,k:=range jwks.Keys{
  if k.Kid!=header.Kid||k.Kty!="RSA"||(k.Alg!=""&&k.Alg!="RS256")||(k.Use!=""&&k.Use!="sig"){continue}
  n,ne:=dec(k.N);e,ee:=dec(k.E);ex:=new(big.Int).SetBytes(e)
  if ne!=nil||ee!=nil||len(n)<256||!ex.IsInt64()||ex.Int64()<3||ex.Int64()>2147483647{continue}
  if rsa.VerifyPKCS1v15(&rsa.PublicKey{N:new(big.Int).SetBytes(n),E:int(ex.Int64())},crypto.SHA256,sum[:],sig)==nil{verified=true;break}
 }
 if !verified||json.Unmarshal(p,&id)!=nil||id.Iss!=chatIssuer||id.Sub==""||id.Nonce!=nonce||id.Exp<=now.Unix(){return id,bad}
 var aud string;var list []string
 if json.Unmarshal(id.Aud,&aud)==nil{if aud!=client{return id,bad}}else if json.Unmarshal(id.Aud,&list)!=nil||!has(list,client)||(len(list)>1&&id.Azp!=client){return id,bad}
 if id.Azp!=""&&id.Azp!=client{return id,bad}
 return id,nil
}
func(a *ChatGPTAccount) Token(ctx context.Context)(string,error){
 a.mu.Lock();defer a.mu.Unlock()
 r:=a.record
 if r.Tokens.Refresh==""||!has(strings.Fields(r.Tokens.Scope),"chatgpt.tokens.use.direct"){return "",errors.New("conecte a conta ChatGPT")}
 if r.Expiry.After(time.Now().Add(time.Minute)){return r.Tokens.Access,nil}
 t,err:=a.exchange(ctx,url.Values{"grant_type":{"refresh_token"},"client_id":{r.Client},"refresh_token":{r.Tokens.Refresh},"resource":{chatResource}});if err!=nil{return "",err}
 if t.Scope==""{t.Scope=r.Tokens.Scope}
 if !has(strings.Fields(t.Scope),"chatgpt.tokens.use.direct")||!has(strings.Fields(t.Scope),"resource.invoke"){return "",errors.New("permissão de uso do plano revogada")}
 t.ID=r.Tokens.ID // retain the previously validated identity hint
 r.Tokens=t;r.Expiry=time.Now().Add(time.Duration(t.Expires)*time.Second)
 if err=a.save(r);err!=nil{return "",err}
 return t.Access,nil
}
func(a *ChatGPTAccount) Models(ctx context.Context)(ModelCatalog,error){
 token,err:=a.Token(ctx);if err!=nil{return ModelCatalog{},err}
 raw,err:=a.request(ctx,"GET",chatResource+"/models",token,nil);if err!=nil{return ModelCatalog{},err}
 var env struct{Models []struct{Slug string; DisplayName string `json:"display_name"`;Visibility string}}
 if json.Unmarshal(raw,&env)!=nil{return ModelCatalog{},errors.New("catálogo ChatGPT inválido")}
 out:=ModelCatalog{Models:[]CatalogModel{}}
 for _,m:=range env.Models{if m.Visibility=="list"&&validText(m.Slug,200){out.Models=append(out.Models,CatalogModel{ID:m.Slug,Name:m.DisplayName})}}
 return out,nil
}
func(a *ChatGPTAccount) Logout(ctx context.Context)error{
 a.mu.Lock();defer a.mu.Unlock();r:=a.record;a.pending=nil
 var revokeErr error
 if r.Tokens.Refresh!=""{
  raw,err:=a.request(ctx,"GET",chatIssuer+"/.well-known/openid-configuration","",nil)
  var d struct{Revocation string `json:"revocation_endpoint"`}
  if err!=nil||json.Unmarshal(raw,&d)!=nil||!strings.HasPrefix(d.Revocation,chatIssuer+"/"){revokeErr=errors.New("revogação remota não confirmada")}else{
   _,revokeErr=a.request(ctx,"POST",d.Revocation,"",url.Values{"token":{r.Tokens.Refresh},"token_type_hint":{"refresh_token"},"client_id":{r.Client}})
  }
 }
 r.Tokens=chatTokens{};r.Expiry=time.Time{}
 if err:=a.save(r);err!=nil{return err}
 if revokeErr!=nil{return errors.New("sessão apagada localmente; revogação remota não confirmada. Desconecte em Configurações do ChatGPT.")}
 return nil
}

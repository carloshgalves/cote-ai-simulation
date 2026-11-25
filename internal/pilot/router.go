package pilot

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"
)

type RouteInput struct {
	Provider string `json:"provider"`
	Endpoint string `json:"endpoint"`
	Model string `json:"model"`
	Key string `json:"key"`
	Format string `json:"format"`
}
type RouteView struct {
	ID string `json:"id"`
	Provider string `json:"provider"`
	Endpoint string `json:"endpoint"`
	Model string `json:"model"`
	Format string `json:"format"`
	HasKey bool `json:"has_key"`
	Enabled bool `json:"enabled"`
	Status string `json:"status"`
	RetryAt time.Time `json:"retry_at"`
	Attempts int `json:"attempts"`
	Accepted int `json:"accepted"`
	LastError string `json:"last_error,omitempty"`
	LastLatencyMS int64 `json:"last_latency_ms,omitempty"`
	EffectiveFormat string `json:"effective_format,omitempty"`
	Quota QuotaReading `json:"quota"`
	PromptTokens int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`
	UsageReports int `json:"usage_reports"`
}
type route struct {
	view RouteView
	model *HTTPModel
	quotaScope string
	authScope string
}
type cooldown struct {until time.Time; message string}
type Router struct {
	mu sync.Mutex
	callMu sync.Mutex
	next int
	routes []*route
	cooldowns map[string]cooldown
	telemetry []CallTelemetry
	telemetryDropped int
	chat *ChatGPTAccount
}
func NewRouter()*Router{return &Router{cooldowns:map[string]cooldown{}}}
var providerEndpoints=map[string]string{
	"ollama":"http://127.0.0.1:11434/v1",
	"groq":"https://api.groq.com/openai/v1",
	"gemini":"https://generativelanguage.googleapis.com/v1beta/openai",
	"openrouter":"https://openrouter.ai/api/v1",
	"openai":"https://api.openai.com/v1",
	"chatgpt":"https://api.openai.com/v1",
	"cerebras":"https://api.cerebras.ai/v1",
	"nvidia":"https://integrate.api.nvidia.com/v1",
	"anthropic":"https://api.anthropic.com/v1",
	"mistral":"https://api.mistral.ai/v1",
	"deepseek":"https://api.deepseek.com/v1",
}
func normalizeRoute(in RouteInput,requireModel bool)(RouteInput,*HTTPModel,error){
	in.Provider=strings.ToLower(strings.TrimSpace(in.Provider))
	in.Model=strings.TrimSpace(in.Model)
	in.Endpoint=strings.TrimSpace(in.Endpoint)
	in.Key=strings.TrimSpace(in.Key)
	if in.Format==""{in.Format="auto"}
	if !has([]string{"auto","json","schema","plain"},in.Format){return in,nil,errors.New("formato de resposta inválido")}
	if in.Provider!="openai-compatible"{
		endpoint,ok:=providerEndpoints[in.Provider];if !ok{return in,nil,errors.New("provedor não suportado")}
		if in.Provider!="ollama"||in.Endpoint==""{in.Endpoint=endpoint}
	}
	if len(in.Key)>4096||strings.ContainsAny(in.Key,"\r\n")||len(in.Endpoint)>500||(requireModel&&!validText(in.Model,200)){return in,nil,errors.New("configuração de modelo inválida")}
	if in.Provider=="chatgpt"&&in.Key!=""{return in,nil,errors.New("ChatGPT usa autorização OAuth, não chave")}
	if in.Provider!="ollama"&&in.Provider!="chatgpt"&&in.Provider!="openai-compatible"&&in.Key==""{return in,nil,errors.New("informe uma chave")}
	name:=in.Model;if !requireModel{name="catalog"}
	m,err:=NewHTTPModel(in.Endpoint,name,in.Key);if err!=nil{return in,nil,err}
	if in.Format!="auto"{m.Format=in.Format}
	if in.Provider!="ollama" {
		// Remote routes should not monopolize a cycle for four minutes.
		m.Client.Timeout=45*time.Second
	}
	if in.Provider=="chatgpt"{m.Protocol="chatgpt";m.Endpoint=chatResource+"/responses";m.Client.Timeout=90*time.Second}
	if in.Provider=="anthropic"{m.Protocol="anthropic";m.Endpoint=strings.TrimRight(in.Endpoint,"/")+"/messages"}
	u,_:=url.Parse(m.Endpoint)
	if u.Scheme=="http"&&in.Key!=""{return in,nil,errors.New("chave de API requer HTTPS")}
	return in,m,nil
}
func (r *Router) Add(in RouteInput)(RouteView,error){
	in,m,err:=normalizeRoute(in,true);if err!=nil{return RouteView{},err}
	u,_:=url.Parse(m.Endpoint)
	r.mu.Lock();defer r.mu.Unlock()
	if len(r.routes)>=24{return RouteView{},errors.New("limite de 24 rotas")}
	for _,old:=range r.routes{if old.model.Endpoint==m.Endpoint&&old.model.Name==m.Name&&old.model.Key==m.Key&&old.model.Format==m.Format{return RouteView{},errors.New("rota já cadastrada")}}
	r.next++
	view:=RouteView{ID:fmt.Sprint(r.next),Provider:in.Provider,Endpoint:in.Endpoint,Model:in.Model,Format:in.Format,HasKey:in.Key!="",Enabled:true,Status:"não testado"}
	// Same provider/model can share account-level quota across keys. Respect
	// its cooldown rather than repeatedly probing other keys for that model.
	quota:=u.Hostname()+"|"+in.Model
	if in.Provider=="chatgpt"{quota="chatgpt-plan"}
	auth:=fmt.Sprintf("%s|%x",u.Hostname(),sha256.Sum256([]byte(in.Key)))
	if in.Provider=="chatgpt"{auth="chatgpt-plan"}
	r.routes=append(r.routes,&route{view:view,model:m,quotaScope:quota,authScope:auth})
	return view,nil
}
func (r *Router) blocked(rt *route,now time.Time) cooldown {
	var max cooldown
	for _,k:=range []string{"quota:"+rt.quotaScope,"auth:"+rt.authScope,"route:"+rt.view.ID}{
		if c:=r.cooldowns[k];c.until.After(now)&&c.until.After(max.until){max=c}
	}
	return max
}
func (r *Router) Routes() []RouteView{
	r.mu.Lock();defer r.mu.Unlock()
	out:=[]RouteView{}
	for _,rt:=range r.routes{
		v:=rt.view
		if c:=r.blocked(rt,time.Now());!c.until.IsZero(){v.RetryAt=c.until;v.Status=c.message}else if v.Status!="disponível"&&v.Status!="consultando"&&v.Status!="não testado"{v.Status="pronto para nova tentativa"}
		out=append(out,v)
	}
	return out
}
func (r *Router) Change(id,action string)error{
	r.mu.Lock();defer r.mu.Unlock()
	for i,rt:=range r.routes{if rt.view.ID==id{
		switch action{
		case "remove":r.routes=append(r.routes[:i],r.routes[i+1:]...)
		case "up":if i>0{r.routes[i-1],r.routes[i]=r.routes[i],r.routes[i-1]}
		case "down":if i+1<len(r.routes){r.routes[i+1],r.routes[i]=r.routes[i],r.routes[i+1]}
		case "toggle":rt.view.Enabled=!rt.view.Enabled
		default:return errors.New("ação de rota inválida")
		};return nil
	}}
	return errors.New("rota não encontrada")
}
func (r *Router) Decide(ctx context.Context,view Context)(Decision,error){return r.decide(ctx,view,"")}
func (r *Router) Repair(ctx context.Context,view Context,problem string)(Decision,error){return r.decide(ctx,view,problem)}
func (r *Router) decide(ctx context.Context,view Context,problem string)(Decision,error){
	r.callMu.Lock();defer r.callMu.Unlock()
	r.mu.Lock();routes:=append([]*route{},r.routes...);r.mu.Unlock()
	for _,rt:=range routes{
		if err:=ctx.Err();err!=nil{return Decision{},err}
		r.mu.Lock()
		present:=false;for _,current:=range r.routes{if current==rt{present=true;break}}
		available:=present&&rt.view.Enabled&&r.blocked(rt,time.Now()).until.IsZero()
		if available{rt.view.Status="consultando"}
		r.mu.Unlock()
		if !available{continue}
		d,err:=r.callRoute(ctx,rt,view,problem)
		if ctx.Err()!=nil{
			r.mu.Lock();rt.view.Status="cancelado";r.mu.Unlock()
			return Decision{},ctx.Err()
		}
		r.mu.Lock()
		if err==nil{
			rt.view.Status="disponível";delete(r.cooldowns,"route:"+rt.view.ID)
			d.Source=rt.view.Provider+" / "+rt.view.Model
			r.mu.Unlock()
			return d,nil
		}
		now:=time.Now()
		scope:="route:"+rt.view.ID
		until:=now.Add(30*time.Second)
		var pe providerError
		if errors.As(err,&pe){
			switch pe.Status{
			case 429:scope="quota:"+rt.quotaScope;until=pe.RetryAt;if !until.After(now){until=now.Add(time.Second)}
			case 401,403:scope="auth:"+rt.authScope;until=now.Add(10*time.Minute)
			case 400,402,404:until=now.Add(5*time.Minute)
			}
		}
		rt.view.Status=err.Error()
		if old:=r.cooldowns[scope];old.until.After(until){until=old.until}
		r.cooldowns[scope]=cooldown{until,err.Error()}
		r.mu.Unlock()
	}
	return Decision{},errors.New("nenhuma rota conseguiu gerar uma decisão válida; consulte o diagnóstico de cada modelo em Modelos e APIs. Decisões anteriores preservadas.")
}

type storedRoute struct {
	Input RouteInput `json:"input"`
	Enabled bool `json:"enabled"`
}
func (r *Router) Save(path string)error{
	r.mu.Lock()
	routes:=[]storedRoute{}
	for _,rt:=range r.routes{routes=append(routes,storedRoute{RouteInput{Provider:rt.view.Provider,Endpoint:rt.view.Endpoint,Model:rt.view.Model,Key:rt.model.Key,Format:rt.view.Format},rt.view.Enabled})}
	r.mu.Unlock()
	if err:=os.MkdirAll(filepath.Dir(path),0700);err!=nil{return errors.New("não foi possível criar diretório local")}
	raw,err:=json.MarshalIndent(routes,"","  ");if err!=nil{return err}
	tmp,err:=os.CreateTemp(filepath.Dir(path),".cote-routes-");if err!=nil{return errors.New("não foi possível criar arquivo de modelos")}
	defer os.Remove(tmp.Name())
	_,err=tmp.Write(raw);if err==nil{err=tmp.Sync()};closeErr:=tmp.Close()
	if err!=nil||closeErr!=nil{return errors.New("não foi possível gravar modelos")}
	if err:=os.Rename(tmp.Name(),path);err!=nil{return errors.New("não foi possível salvar configuração")}
	return nil
}
func LoadRouter(path string)(*Router,error){
	r:=NewRouter()
	info,err:=os.Lstat(path)
	if errors.Is(err,os.ErrNotExist){return r,nil}
	if err!=nil||!info.Mode().IsRegular()||info.Mode().Perm()&0077!=0||info.Size()>256<<10{return nil,errors.New("arquivo de modelos inválido ou permissões expostas")}
	f,err:=os.Open(path);if err!=nil{return nil,err};defer f.Close()
	raw,err:=io.ReadAll(io.LimitReader(f,256<<10+1));if err!=nil||len(raw)>256<<10{return nil,errors.New("não foi possível ler modelos")}
	list,err:=decodeStrict[[]storedRoute](raw);if err!=nil{return nil,errors.New("configuração de modelos inválida")}
	for _,item:=range list{
		v,err:=r.Add(item.Input);if err!=nil{return nil,err}
		if !item.Enabled{if err:=r.Change(v.ID,"toggle");err!=nil{return nil,err}}
	}
	return r,nil
}

func (r *Router) callRoute(ctx context.Context,rt *route,view Context,problem string)(Decision,error){
	// Work on a copy of the client configuration; the saved format is not
	// changed by negotiation. Only the effective format is cached in memory.
	r.mu.Lock()
	m:=*rt.model
	m.chat=r.chat
	if rt.view.EffectiveFormat!=""&&rt.view.EffectiveFormat!="auto"{m.Format=rt.view.EffectiveFormat}
	r.mu.Unlock()
	call:=func(repair string)(Decision,error){
		start:=time.Now()
		t:=CallTelemetry{StartedAt:start,Actor:view.Self.ID,Cycle:view.Round,Provider:rt.view.Provider,Model:rt.view.Model,RouteID:rt.view.ID,Repair:repair!="",Format:m.Format}
		m.telemetry=&t
		r.mu.Lock();rt.view.Attempts++;r.mu.Unlock()
		d,err:=m.decide(ctx,view,repair)
		if err==nil{if validation:=validateDecision(view,d);validation!=nil{err=responseError{validation.Error()}}}
		r.mu.Lock()
		t.LatencyMS=time.Since(start).Milliseconds()
		t.Accepted=err==nil&&ctx.Err()==nil
		rt.view.LastLatencyMS=t.LatencyMS
		if err!=nil{rt.view.LastError=err.Error();t.Error=err.Error()}
		if t.Quota.Observed {rt.view.Quota=t.Quota}
		if t.Usage!=nil{rt.view.PromptTokens+=t.Usage.Input;rt.view.CompletionTokens+=t.Usage.Output;rt.view.UsageReports++}
		// The provider's own explicit exhaustion signal is authoritative;
		// do not guess at remaining quota from local call counts.
		if t.Quota.Observed {
			now:=time.Now()
			until:=t.Quota.ExhaustedUntil(now)
			if until.After(now){
				scope:="quota:"+rt.quotaScope
				if r.cooldowns[scope].until.Before(until){r.cooldowns[scope]=cooldown{until,"cota indicada pelo provedor esgotada; aguardando renovação"}}
			}
		}
		r.telemetry=append(r.telemetry,t)
		if len(r.telemetry)>10000{r.telemetry=r.telemetry[1:];r.telemetryDropped++}
		r.mu.Unlock()
		return d,err
	}
	d,err:=call(problem)
	var pe providerError
	r.mu.Lock();blocked:=!r.blocked(rt,time.Now()).until.IsZero();r.mu.Unlock()
	if ctx.Err()==nil&&!blocked&&rt.view.Format=="auto"&&m.Format!="plain"&&errors.As(err,&pe)&&pe.FormatRejected{
		// One compatibility downgrade, not a retry storm on quota/auth errors.
		m.Format="plain"
		d,err=call(problem)
	}
	var malformed responseError
	r.mu.Lock();blocked=!r.blocked(rt,time.Now()).until.IsZero();r.mu.Unlock()
	if ctx.Err()==nil&&!blocked&&errors.As(err,&malformed){
		d,err=call(malformed.Error())
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err==nil&&ctx.Err()==nil{
		rt.view.Accepted++
		rt.view.LastError=""
		rt.view.EffectiveFormat=m.Format
		if rt.view.EffectiveFormat==""{rt.view.EffectiveFormat="auto"}
	}
	return d,err
}

// Test a single configured route using a tiny synthetic context. This invokes
// generation (and may consume quota), but never reads or changes the journal.
func (r *Router) Test(ctx context.Context,id string)(RouteView,error){
	if !r.callMu.TryLock(){return RouteView{},errors.New("há uma consulta em andamento; pause a simulação antes de testar")}
	defer r.callMu.Unlock()
	r.mu.Lock()
	var rt *route
	for _,candidate:=range r.routes{if candidate.view.ID==id{rt=candidate;break}}
	if rt==nil{r.mu.Unlock();return RouteView{},errors.New("rota não encontrada")}
	if c:=r.blocked(rt,time.Now());c.until.After(time.Now()){r.mu.Unlock();return RouteView{},errors.New("rota em cooldown: "+c.message)}
	rt.view.Status="consultando"
	r.mu.Unlock()
	peer:=Person{ID:"peer",Name:"Colega fictício",Class:"D"}
	view:=Context{Version:Version,CanObserve:true,Self:Profile{ID:"probe",Name:"Aluno de teste",Class:"D",Traits:[]string{"Reservado"},Goals:[]string{"Aguardar orientação"}},Goals:[]string{"Aguardar orientação"},Location:"hall",Locations:[]string{"hall","library"},People:[]Person{peer},Present:[]Presence{{Person:peer,Location:"hall"}},SchoolYear:1,PeerRelationship:"Teste sintético com um colega fictício da mesma série."}
	_,err:=r.callRoute(ctx,rt,view,"")
	r.mu.Lock();defer r.mu.Unlock()
	if err!=nil{
		rt.view.Status=err.Error()
		var pe providerError
		if errors.As(err,&pe){
			if pe.Status==429{scope:="quota:"+rt.quotaScope;if r.cooldowns[scope].until.Before(pe.RetryAt){r.cooldowns[scope]=cooldown{pe.RetryAt,err.Error()}}}
			if pe.Status==401||pe.Status==403{r.cooldowns["auth:"+rt.authScope]=cooldown{time.Now().Add(10*time.Minute),err.Error()}}
		}
	}else{rt.view.Status="disponível"}
	return rt.view,nil
}

func(r *Router) AttachChatGPT(a *ChatGPTAccount){
	r.mu.Lock();defer r.mu.Unlock();r.chat=a
	delete(r.cooldowns,"auth:chatgpt-plan")
	delete(r.cooldowns,"quota:chatgpt-plan")
	for _,rt:=range r.routes{if rt.view.Provider=="chatgpt"{delete(r.cooldowns,"route:"+rt.view.ID);rt.view.Status="não testado"}}
}

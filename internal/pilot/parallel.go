package pilot

import (
 "context"
 "errors"
 "fmt"
 "net/url"
 "sync"
 "time"
)
func(r *Router) parallelEligible()bool{
 r.mu.Lock();defer r.mu.Unlock()
 hosts:=map[string]bool{};for _,rt:=range r.routes{if rt.view.Enabled&&rt.view.Provider!="chatgpt"{u,_:=url.Parse(rt.view.Endpoint);if u!=nil{hosts[u.Hostname()]=true}}}
 return len(hosts)>=2
}
// Only independent endpoint hosts can overlap. The ChatGPT account and
// each host are serialized; quota negotiation remains inside callRoute.
func(r *Router) decideConcurrent(ctx context.Context,view Context,problem string)(Decision,error){
 if !r.parallelEligible(){r.callMu.Lock();defer r.callMu.Unlock()}else{r.callMu.RLock();defer r.callMu.RUnlock()}
 tried:=map[string]bool{}
 for {
  if err:=ctx.Err();err!=nil{return Decision{},err}
  r.mu.Lock();if r.busyScopes==nil{r.busyScopes=map[string]bool{}}
  var selected *route;scope:="";waiting:=false
  for _,rt:=range r.routes{
   if tried[rt.view.ID]||!rt.view.Enabled||!r.blocked(rt,time.Now()).until.IsZero(){continue}
   u,_:=url.Parse(rt.view.Endpoint);key:=rt.view.Provider;if u!=nil{key=u.Hostname()};if rt.view.Provider=="chatgpt"{key="chatgpt-account"}
   if r.busyScopes[key]{waiting=true;continue};selected=rt;scope=key;break
  }
  if selected!=nil{r.busyScopes[scope]=true;selected.view.Status="consultando"};r.mu.Unlock()
  if selected==nil{if !waiting{return Decision{},errors.New("nenhuma rota disponível")};select{case <-ctx.Done():return Decision{},ctx.Err();case <-time.After(50*time.Millisecond):continue}}
  d,err:=r.callRoute(ctx,selected,view,problem)
  r.mu.Lock();delete(r.busyScopes,scope);tried[selected.view.ID]=true
  if err==nil&&ctx.Err()==nil{selected.view.Status="disponível";d.Source=selected.view.Provider+" / "+selected.view.Model;r.mu.Unlock();return d,nil}
  selected.view.Status="consulta rejeitada";until:=time.Now().Add(30*time.Second);key:="route:"+selected.view.ID
  var pe providerError;if errors.As(err,&pe){if pe.Status==429{key="quota:"+selected.quotaScope;if pe.RetryAt.After(until){until=pe.RetryAt}}else if pe.Status==401||pe.Status==403{key="auth:"+selected.authScope;until=time.Now().Add(10*time.Minute)}}
  if r.cooldowns[key].until.Before(until){r.cooldowns[key]=cooldown{until,"consulta rejeitada; aguarde"}}
  r.mu.Unlock()
 }
}
func(s *Session) parallelChoices(ctx context.Context,j Journal,state State,order []string)([]Chosen,error){
 type result struct{choice Chosen;err error}
 out:=make([]result,len(order));var wg sync.WaitGroup;slots:=make(chan struct{},3)
 for i,id:=range order{
  if !decisionNeedsModel(j,state,id){d:=routineDecision(j,state,id);out[i]=result{Chosen{Actor:id,Decision:d},validateScheduledChoice(j,state,Chosen{Actor:id,Decision:d})};continue}
  wg.Add(1);go func(i int,id string){defer wg.Done();select{case slots<-struct{}{}:case <-ctx.Done():out[i].err=ctx.Err();return};defer func(){<-slots}()
   view:=actorContext(j,state,id);d,err:=s.model.Decide(ctx,view);if err==nil{err=validateDecision(view,d);if d.Source==routineSource{err=errors.New("invalid model provenance")}}
   out[i]=result{Chosen{Actor:id,Decision:d},err}
  }(i,id)
 }
 wg.Wait();choices:=[]Chosen{};for i,x:=range out{if x.err!=nil{return choices,fmt.Errorf("%s: %w",order[i],x.err)};choices=append(choices,x.choice)};return choices,nil
}

package pilot

import (
 "encoding/json"
 "time"
)
type minuteBudget struct{
 remaining int64
 reset time.Time
 maxInput int64
 known bool
}
func(r *Router) admit(rt *route,v Context,now time.Time,repair,expanded bool)time.Time{
 b:=r.budgets[rt.quotaScope]
 if !b.known||!b.reset.After(now){return time.Time{}}
 // Estimate conservatively, using measured input when available. This is
 // admission control, not a provider tokenizer or guarantee of remaining quota.
 raw,_:=json.Marshal(v)
 estimate:=int64(len(raw)/3+500)
 if b.maxInput>estimate{estimate=b.maxInput}
 estimate+=int64(generationBudget(rt.model.Name,repair,expanded))
 if b.remaining<estimate{return b.reset}
 b.remaining-=estimate;r.budgets[rt.quotaScope]=b
 return time.Time{}
}
func(r *Router) observeBudget(rt *route,t CallTelemetry){
 b:=r.budgets[rt.quotaScope]
 if t.Usage!=nil&&t.Usage.Input>b.maxInput{b.maxInput=t.Usage.Input}
 q:=t.Quota
 // Only use explicitly identified minute-token windows (Groq).
 if q.TokensWindow=="TPM"&&q.TokensRemaining!=nil&&q.TokensReset.After(t.StartedAt){
  b.remaining=*q.TokensRemaining;b.reset=q.TokensReset;b.known=true
 }
 r.budgets[rt.quotaScope]=b
}

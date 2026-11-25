package pilot

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type TokenUsage struct {
	Input int64 `json:"input"`
	Output int64 `json:"output"`
	Total int64 `json:"total"`
}
type QuotaReading struct {
	Observed bool `json:"observed"`
	At time.Time `json:"at"`
	RequestsRemaining *int64 `json:"requests_remaining,omitempty"`
	TokensRemaining *int64 `json:"tokens_remaining,omitempty"`
	RequestsLimit *int64 `json:"requests_limit,omitempty"`
	TokensLimit *int64 `json:"tokens_limit,omitempty"`
	RequestsReset time.Time `json:"requests_reset"`
	TokensReset time.Time `json:"tokens_reset"`
	RequestsWindow string `json:"requests_window,omitempty"`
	TokensWindow string `json:"tokens_window,omitempty"`
}
func headerInt(h http.Header,name string)*int64{
	n,err:=strconv.ParseInt(strings.TrimSpace(h.Get(name)),10,64)
	if err!=nil||n<0{return nil};return &n
}
func quotaReset(raw string,now time.Time)time.Time{
	if d,err:=time.ParseDuration(raw);err==nil&&d>=0&&d<=365*24*time.Hour{return now.Add(d)}
	return time.Time{}
}
func readQuota(h http.Header,now time.Time,groq bool)QuotaReading{
	q:=QuotaReading{At:now,RequestsRemaining:headerInt(h,"x-ratelimit-remaining-requests"),TokensRemaining:headerInt(h,"x-ratelimit-remaining-tokens"),RequestsLimit:headerInt(h,"x-ratelimit-limit-requests"),TokensLimit:headerInt(h,"x-ratelimit-limit-tokens"),RequestsReset:quotaReset(h.Get("x-ratelimit-reset-requests"),now),TokensReset:quotaReset(h.Get("x-ratelimit-reset-tokens"),now)}
	q.Observed=q.RequestsRemaining!=nil||q.TokensRemaining!=nil||q.RequestsLimit!=nil||q.TokensLimit!=nil
	if groq {q.RequestsWindow="RPD";q.TokensWindow="TPM"}
	return q
}
func(q QuotaReading) ExhaustedUntil(now time.Time)time.Time{
	var until time.Time
	if q.RequestsRemaining!=nil&&*q.RequestsRemaining==0&&q.RequestsReset.After(now){until=q.RequestsReset}
	if q.TokensRemaining!=nil&&*q.TokensRemaining==0&&q.TokensReset.After(now)&&q.TokensReset.After(until){until=q.TokensReset}
	return until
}
func readUsage(raw []byte)*TokenUsage{
	var env struct{Usage *struct{
		Prompt *int64 `json:"prompt_tokens"`
		Completion *int64 `json:"completion_tokens"`
		Input *int64 `json:"input_tokens"`
		Output *int64 `json:"output_tokens"`
		Total *int64 `json:"total_tokens"`
	} `json:"usage"`}
	if json.Unmarshal(raw,&env)!=nil||env.Usage==nil{return nil}
	u:=env.Usage;input,output:=u.Prompt,u.Completion
	if input==nil{input=u.Input};if output==nil{output=u.Output}
	if input==nil||output==nil||*input<0||*output<0||*input>1000000000||*output>1000000000{return nil}
	total:=*input+*output;if u.Total!=nil&&*u.Total>=0{total=*u.Total}
	return &TokenUsage{Input:*input,Output:*output,Total:total}
}
type CallTelemetry struct {
	StartedAt time.Time `json:"started_at"`
	Actor string `json:"actor"`
	Cycle int `json:"cycle"`
	Provider string `json:"provider"`
	Model string `json:"model"`
	RouteID string `json:"route_id"`
	Repair bool `json:"repair"`
	Format string `json:"format"`
	HTTPStatus int `json:"http_status"`
	LatencyMS int64 `json:"latency_ms"`
	Accepted bool `json:"accepted"`
	Error string `json:"error,omitempty"`
	Quota QuotaReading `json:"quota"`
	Usage *TokenUsage `json:"usage,omitempty"`
}
func(r *Router) Telemetry()([]CallTelemetry,int){
	r.mu.Lock();defer r.mu.Unlock()
	return clone(r.telemetry),r.telemetryDropped
}

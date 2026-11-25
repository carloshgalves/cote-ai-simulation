package pilot

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type modelFunc func(context.Context, Context) (Decision,error)
func (f modelFunc) Decide(ctx context.Context,c Context)(Decision,error){return f(ctx,c)}
func wait() Decision {return Decision{Action:"wait",Reason:"avaliar a situação",Beliefs:[]Belief{}}}

func TestRealRoundsPrivacyAndReplay(t *testing.T) {
	path:=filepath.Join(t.TempDir(),"run.json")
	secret:="SINAL_PRIVADO_741"
	model:=modelFunc(func(_ context.Context,c Context)(Decision,error){
		d:=wait()
		if c.Round==1 {
			for _,e:=range c.Memory {if strings.Contains(e.Text,secret){t.Fatal("same-round future leak")}}
			if c.Self.ID=="ayanokoji"{d.Action="talk";d.Target="horikita";d.Text=secret}
			if c.Self.ID=="ryuen"{d.Action="move";d.Target="library"}
		}
		if c.Round==2 {
			var eID string
			for _,e:=range c.Memory {if e.Text==secret{eID=e.ID}}
			if (eID!="")!=(c.Self.ID=="ayanokoji"||c.Self.ID=="horikita"){t.Fatal("wrong knowledge recipient")}
			if c.Self.ID=="horikita"{
				d.Beliefs=[]Belief{{Text:"Hipótese não confirmada",Evidence:eID,Confidence:40}}
				d.Goal=&GoalChange{Replace:c.Goals[0],With:"Investigar a alegação",Evidence:eID}
				d.Plan=&PlanChange{Steps:[]string{"Buscar confirmação"},Evidence:eID}
				d.Relation=&RelationChange{Person:"ayanokoji",Delta:1,Evidence:eID}
			}
			if c.Self.ID=="ryuen"&&len(c.Present)!=0{t.Fatal("movement did not persist")}
			if c.Self.ID!="ryuen" {
				for _, e:=range c.Memory {if e.Actor=="ryuen"&&strings.Contains(e.Text,"library"){t.Fatal("departure revealed unseen destination")}}
			}
		}
		return d,nil
	})
	s,err:=Open(path,"test",model);if err!=nil{t.Fatal(err)}
	if _,err:=Open(path,"test",model);err==nil{t.Fatal("concurrent owner admitted")}
	if _,err:=s.Step(context.Background(),0);err!=nil{t.Fatal(err)}
	if _,err:=s.Step(context.Background(),1);err!=nil{t.Fatal(err)}
	snap:=s.Snapshot()
	if snap.State.Minute!=490||snap.State.Round!=2||len(snap.State.Minds["horikita"].Beliefs)!=1||snap.State.Minds["horikita"].Plan==nil||
		snap.State.Minds["horikita"].Goals[0]!="Investigar a alegação"||snap.State.Minds["horikita"].Relations["ayanokoji"]!=1{t.Fatal("cognition not applied")}
	if _,err:=s.Step(context.Background(),1);err==nil{t.Fatal("stale request committed")}
	if err:=s.Close();err!=nil{t.Fatal(err)}
	reopened,err:=Open(path,"test",model);if err!=nil{t.Fatal(err)};defer reopened.Close()
	if !reflect.DeepEqual(snap,reopened.Snapshot()){t.Fatal("resume diverges from recorded rounds")}
	for _, id:=range []string{"ryuen","kushida","ichinose","hirata"}{
		view,err:=reopened.POV(id);if err!=nil{t.Fatal(err)}
		raw,_:=json.Marshal(view);if strings.Contains(string(raw),secret){t.Fatal("private content leaked")}
	}
}

func TestInvalidOrFailedDecisionNeverCommitsPartialRound(t *testing.T) {
	for _,failure:=range []string{"foreign-evidence","remote-talk","provider"}{
		t.Run(failure,func(t *testing.T){
			path:=filepath.Join(t.TempDir(),"run.json")
			model:=modelFunc(func(_ context.Context,c Context)(Decision,error){
				d:=wait();if c.Self.ID!="ryuen"{return d,nil}
				switch failure{
				case "foreign-evidence":d.Beliefs=[]Belief{{Text:"invented",Evidence:"e999999",Confidence:100}}
				case "remote-talk":d.Action="talk";d.Target="not-an-actor";d.Text="hello"
				default:return d,errors.New("model unavailable")
				};return d,nil
			})
			s,err:=Open(path,"test",model);if err!=nil{t.Fatal(err)};defer s.Close()
			before,_:=os.ReadFile(path)
			if _,err:=s.Step(context.Background(),0);err==nil{t.Fatal("invalid round accepted")}
			after,_:=os.ReadFile(path)
			if string(before)!=string(after)||s.Snapshot().State.Round!=0{t.Fatal("partial publication")}
		})
	}
}

func TestHTTPModelSendsOnlyActorContextAndParsesRealWire(t *testing.T) {
	var calls atomic.Int32
	server:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
		calls.Add(1)
		if r.Method!="POST"||r.URL.Path!="/v1/chat/completions"{t.Error("wrong model endpoint")}
		var req struct{Model string;Messages []struct{Role,Content string}}
		if err:=json.NewDecoder(r.Body).Decode(&req);err!=nil||len(req.Messages)!=2{t.Error("wrong request");w.WriteHeader(400);return}
		var view Context
		if err:=json.Unmarshal([]byte(req.Messages[1].Content),&view);err!=nil||view.Self.ID==""{t.Error("missing scoped context")}
		if view.Self.ID!="ayanokoji"&&strings.Contains(req.Messages[1].Content,"treinamento isolado"){t.Error("foreign background leaked")}
		d,_:=json.Marshal(localChoice{Choice:"wait",Reason:"avaliar a situação"})
		_ = json.NewEncoder(w).Encode(map[string]any{"choices":[]any{map[string]any{"message":map[string]string{"content":string(d)},"finish_reason":"stop"}}})
	}));defer server.Close()
	model,err:=NewHTTPModel(server.URL+"/v1","test","");if err!=nil{t.Fatal(err)}
	s,err:=Open(filepath.Join(t.TempDir(),"run.json"),"test",model);if err!=nil{t.Fatal(err)};defer s.Close()
	snap,err:=s.Step(context.Background(),0);if err!=nil{t.Fatal(err)}
	if calls.Load()!=6||snap.State.Round!=1{t.Fatal("not six actual HTTP decisions")}
}

func TestHTTPRepairKeepsCompletedActorsAndProgress(t *testing.T) {
	var calls atomic.Int32
	var bad atomic.Int32
	seen:=make(chan struct{})
	release:=make(chan struct{})
	server:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
		calls.Add(1)
		var req struct{Messages []struct{Role,Content string};ResponseFormat map[string]any `json:"response_format"`}
		if err:=json.NewDecoder(r.Body).Decode(&req);err!=nil{t.Error(err);return}
		if req.ResponseFormat["type"]!="json_schema"{t.Error("local schema missing")}
		var view Context
		if err:=json.Unmarshal([]byte(req.Messages[1].Content),&view);err!=nil{t.Error(err);return}
		content,_:=json.Marshal(localChoice{Choice:"wait",Reason:"avaliar a situação"})
		if view.Self.ID=="horikita"&&bad.Add(1)==1{
			close(seen);<-release
			content=[]byte("{\"world-assets\":true}")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"choices":[]any{map[string]any{"message":map[string]string{"content":string(content)},"finish_reason":"stop"}}})
	}));defer server.Close()
	model,err:=NewHTTPModel(server.URL+"/v1","test","");if err!=nil{t.Fatal(err)}
	s,err:=Open(filepath.Join(t.TempDir(),"run.json"),"test",model);if err!=nil{t.Fatal(err)};defer s.Close()
	done:=make(chan error,1)
	go func(){_,err:=s.Step(context.Background(),0);done<-err}()
	select{case <-seen:case <-time.After(5*time.Second):close(release);t.Fatal("model never reached Horikita")}
	p:=s.Progress()
	if !p.Active||p.Actor!="horikita"||p.Completed!=2||len(p.Choices)!=2||p.Choices[0].Decision.Reason!="avaliar a situação"||s.Snapshot().State.Round!=0{t.Error("incorrect live progress")}
	close(release)
	if err:=<-done;err!=nil{t.Fatal(err)}
	if calls.Load()!=7||s.Snapshot().State.Round!=1{t.Fatal("retry repeated completed agents or failed to commit")}
}

func TestCanceledRoundRetainsOnlyValidatedChoices(t *testing.T){
	var calls atomic.Int32
	started:=make(chan struct{})
	m:=modelFunc(func(ctx context.Context,c Context)(Decision,error){
		n:=calls.Add(1)
		if n==2{close(started);<-ctx.Done();return Decision{},ctx.Err()}
		return wait(),nil
	})
	s,err:=Open(filepath.Join(t.TempDir(),"run.json"),"test",m);if err!=nil{t.Fatal(err)};defer s.Close()
	done:=make(chan error,1)
	go func(){_,err:=s.Step(context.Background(),0);done<-err}()
	select{case <-started:case <-time.After(5*time.Second):t.Fatal("model never started second actor")}
	if !s.Cancel(1){t.Error("cancel rejected")}
	if err:=<-done;err==nil{t.Fatal("canceled step committed")}
	if s.Snapshot().State.Round!=0||s.Progress().Completed!=1{t.Fatal("partial state or lost valid decision")}
	if _,err:=s.Step(context.Background(),0);err!=nil{t.Fatal(err)}
	if calls.Load()!=7||s.Snapshot().State.Round!=1{t.Fatal("failed to resume cached decisions")}
}

func TestLocalSchemaLimitsIdentifiersToCurrentContext(t *testing.T){
	j:=Journal{Version:Version,Model:"test"}
	j.Profiles,_=decodeStrict[[]Profile](defaultProfiles)
	s,err:=initial(j);if err!=nil{t.Fatal(err)}
	m:=s.Minds["horikita"];m.Location="library";s.Minds["horikita"]=m
	view:=actorContext(j,s,"horikita")
	raw,err:=localDecisionSchema(view);if err!=nil{t.Fatal(err)}
	var schema map[string]any
	if err:=json.Unmarshal(raw,&schema);err!=nil{t.Fatal(err)}
	props:=schema["properties"].(map[string]any)
	targets:=props["choice"].(map[string]any)["enum"].([]any)
	for _,v:=range targets {if strings.HasPrefix(v.(string),"talk:")||v=="move:library"{t.Fatal("schema permitted inaccessible action")}}
}

func TestLocalChoiceKeepsActionDespiteInvalidOptionalCognition(t *testing.T){
	j:=Journal{Version:Version,Model:"test"};j.Profiles,_=decodeStrict[[]Profile](defaultProfiles)
	s,err:=initial(j);if err!=nil{t.Fatal(err)}
	view:=actorContext(j,s,"ayanokoji")
	d,err:=decodeLocalChoice(view,[]byte(`{"choice":"talk:horikita","text":"Bom dia.","reason":"Conhecer colega","beliefs":[{"text":"inventado","evidence":"e999999","confidence":100}],"goal":{"replace":"inexistente","with":"nova","evidence":"e000001"},"plan":{"steps":["Observar"],"evidence":"e000001"},"world-assets":true}`))
	if err!=nil||d.Action!="talk"||d.Target!="horikita"||d.Text!="Bom dia."||d.Goal!=nil||len(d.Beliefs)!=0||d.Plan==nil||len(d.Ignored)!=3{t.Fatalf("unexpected decision: %+v %v",d,err)}
	if _,err:=decodeLocalChoice(view,[]byte(`{"choice":"talk:unknown","text":"Oi","reason":"testar"}`));err==nil{t.Fatal("invented choice accepted")}
}
func TestInvalidAgentDoesNotBlockOtherAgents(t *testing.T){
	server:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
		var req struct{Messages []struct{Content string}}
		if err:=json.NewDecoder(r.Body).Decode(&req);err!=nil{t.Error(err);return}
		var view Context
		if err:=json.Unmarshal([]byte(req.Messages[1].Content),&view);err!=nil{t.Error(err);return}
		content,_:=json.Marshal(localChoice{Choice:"observe",Reason:"avaliar"})
		if view.Self.ID=="horikita"{content=[]byte(`{"choice":"invented","text":"","reason":"errado"}`)}
		_ = json.NewEncoder(w).Encode(map[string]any{"choices":[]any{map[string]any{"message":map[string]string{"content":string(content)},"finish_reason":"stop"}}})
	}));defer server.Close()
	model,err:=NewHTTPModel(server.URL+"/v1","test","");if err!=nil{t.Fatal(err)}
	path:=filepath.Join(t.TempDir(),"run.json")
	s,err:=Open(path,"test",model);if err!=nil{t.Fatal(err)}
	snap,err:=s.Step(context.Background(),0);if err!=nil{t.Fatal(err)}
	if snap.State.Round!=1{t.Fatal("round failed to advance")}
	failures:=0
	for _,e:=range snap.State.Events{
		if e.Actor=="horikita"{if e.Kind!="model_failure"||len(e.Audience)!=0{t.Fatal("failure misrepresented as character action")};failures++}
	}
	if failures!=1{t.Fatal("missing failure provenance")}
	view,_:=s.POV("horikita");for _,e:=range view.Memory{if e.Kind=="model_failure"{t.Fatal("technical failure leaked into character memory")}}
	s.Close()
	reopened,err:=Open(path,"test",model);if err!=nil{t.Fatal(err)};defer reopened.Close()
	if !reflect.DeepEqual(snap,reopened.Snapshot()){t.Fatal("failure replay diverged")}
}

func TestMinimalActionDoesNotRequireIntention(t *testing.T){
	j:=Journal{Version:Version,Model:"test"};j.Profiles,_=decodeStrict[[]Profile](defaultProfiles)
	s,err:=initial(j);if err!=nil{t.Fatal(err)}
	view:=actorContext(j,s,"ayanokoji")
	d,err:=decodeLocalChoice(view,[]byte(`{"choice":"move:library"}`))
	if err!=nil||d.Action!="move"||d.Target!="library"||d.Reason!=""{t.Fatalf("minimal choice failed: %+v %v",d,err)}
	if _,err:=decodeLocalChoice(view,[]byte(`{"choice":"talk:horikita"}`));err==nil{t.Fatal("empty speech accepted")}
}
func TestAllInvalidAgentsDoNotAdvanceAndRetryFresh(t *testing.T){
	var calls atomic.Int32
	server:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
		n:=calls.Add(1)
		content:=`{"choice":"invented"}`
		if n>18{content=`{"choice":"observe"}`}
		_ = json.NewEncoder(w).Encode(map[string]any{"choices":[]any{map[string]any{"message":map[string]string{"content":content},"finish_reason":"stop"}}})
	}));defer server.Close()
	model,err:=NewHTTPModel(server.URL+"/v1","test","");if err!=nil{t.Fatal(err)}
	s,err:=Open(filepath.Join(t.TempDir(),"run.json"),"test",model);if err!=nil{t.Fatal(err)};defer s.Close()
	if _,err:=s.Step(context.Background(),0);err==nil{t.Fatal("all invalid round advanced")}
	if s.Snapshot().State.Round!=0||calls.Load()!=18||len(s.Progress().Choices)!=6{t.Fatal("missing failures or bad round state")}
	if _,err:=s.Step(context.Background(),0);err!=nil{t.Fatal(err)}
	if calls.Load()!=24||s.Snapshot().State.Round!=1{t.Fatal("all-failed retry did not start fresh")}
}

func TestEventClockIdleJumpAndNoRepeatedObservation(t *testing.T) {
	j:=Journal{Version:Version,Model:"test"};j.Profiles,_=decodeStrict[[]Profile](defaultProfiles)
	s,err:=initial(j);if err!=nil{t.Fatal(err)}
	choices:=[]Chosen{}
	for _,id:=range ids(j.Profiles){choices=append(choices,Chosen{Actor:id,Decision:Decision{Action:"observe",Reason:"Conhecer o ambiente"}})}
	minute,err:=nextEventMinute(j,s,choices);if err!=nil||minute!=481{t.Fatal("observation duration",minute,err)}
	r:=Round{Number:1,Minute:minute,Choices:choices}
	s,err=apply(j,s,r);if err!=nil{t.Fatal(err)}
	view:=actorContext(j,s,"ayanokoji")
	for _,o:=range offeredActions(view){if o.ID=="observe"{t.Fatal("repetitive observation offered")}}
	count:=len(s.Events)
	minute,err=nextEventMinute(j,s,choices);if err!=nil||minute!=490{t.Fatal("idle must jump to next school event",minute,err)}
	// An old or injected observer can still replay without adding repeated memories.
	s,err=apply(j,s,Round{Number:2,Minute:minute,Choices:choices});if err!=nil{t.Fatal(err)}
	if len(s.Events)!=count+1||s.Events[len(s.Events)-1].Kind!="announcement"{t.Fatal("repeated memories or missing school event")}
	if s.Minds["ayanokoji"].Thought!="Conhecer o ambiente"{t.Fatal("missing intention")}
}
func TestLegacyClockStillReplays(t *testing.T) {
	j:=Journal{Version:LegacyVersion,Model:"test"};j.Profiles,_=decodeStrict[[]Profile](defaultProfiles)
	choices:=[]Chosen{};for _,id:=range ids(j.Profiles){choices=append(choices,Chosen{Actor:id,Decision:wait()})}
	j.Rounds=[]Round{{Number:1,Minute:485,Choices:choices}}
	s,err:=Replay(j);if err!=nil||s.Minute!=485||len(s.Events)!=13{t.Fatal("legacy replay changed",err)}
}
func TestNonSpeechTextPreservedAsUnverifiedPrivateNote(t *testing.T){
	j:=Journal{Version:Version,Model:"test"};j.Profiles,_=decodeStrict[[]Profile](defaultProfiles)
	s,_:=initial(j);view:=actorContext(j,s,"ayanokoji")
	d,err:=decodeLocalChoice(view,[]byte(`{"choice":"observe","text":"Quero identificar os líderes."}`))
	if err!=nil||d.Note==""||d.Text!=""||len(d.Ignored)!=0{t.Fatal("private note discarded",d,err)}
}

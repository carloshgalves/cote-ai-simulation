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
	"testing"
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
	calls:=0
	server:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
		calls++
		if r.Method!="POST"||r.URL.Path!="/v1/chat/completions"{t.Error("wrong model endpoint")}
		var req struct{Model string;Messages []struct{Role,Content string}}
		if err:=json.NewDecoder(r.Body).Decode(&req);err!=nil||len(req.Messages)!=2{t.Error("wrong request");w.WriteHeader(400);return}
		var view Context
		if err:=json.Unmarshal([]byte(req.Messages[1].Content),&view);err!=nil||view.Self.ID==""{t.Error("missing scoped context")}
		if view.Self.ID!="ayanokoji"&&strings.Contains(req.Messages[1].Content,"treinamento isolado"){t.Error("foreign background leaked")}
		d,_:=json.Marshal(wait())
		_ = json.NewEncoder(w).Encode(map[string]any{"choices":[]any{map[string]any{"message":map[string]string{"content":string(d)},"finish_reason":"stop"}}})
	}));defer server.Close()
	model,err:=NewHTTPModel(server.URL+"/v1","test","");if err!=nil{t.Fatal(err)}
	s,err:=Open(filepath.Join(t.TempDir(),"run.json"),"test",model);if err!=nil{t.Fatal(err)};defer s.Close()
	snap,err:=s.Step(context.Background(),0);if err!=nil{t.Fatal(err)}
	if calls!=6||snap.State.Round!=1{t.Fatal("not six actual HTTP decisions")}
}

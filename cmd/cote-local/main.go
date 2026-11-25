package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/carloshgalves/cote-ai-simulation/internal/pilot"
)

//go:embed index.html
var page []byte

func main() {
	provider:=flag.String("provider","ollama","ollama, groq, custom or ui")
	configDir,configErr:=os.UserConfigDir();if configErr!=nil{log.Fatal(configErr)}
	routesPath:=flag.String("routes",filepath.Join(configDir,"cote-ai-simulation","routes.json"),"private optional model configuration")
	modelName:=flag.String("model",os.Getenv("COTE_MODEL"),"LLM model ID")
	endpoint:=flag.String("endpoint","http://127.0.0.1:11434/v1","OpenAI-compatible base URL")
	save:=flag.String("save","cote-local.json","local simulation journal")
	port:=flag.Int("port",8765,"loopback HTTP port")
	steps:=flag.Int("steps",0,"run N rounds in terminal instead of browser")
	flag.Parse()
	key:=os.Getenv("COTE_API_KEY")
	switch *provider {
	case "ollama":
		key=""
		if *modelName==""{*modelName="qwen2.5:3b"}
		if v:=os.Getenv("COTE_LLM_URL");v!=""{*endpoint=v}
	case "groq":
		*endpoint="https://api.groq.com/openai/v1"
		if *modelName==""{*modelName="qwen/qwen3.8-27b"}
		if v:=os.Getenv("GROQ_API_KEY");v!=""{key=v}
		if key==""{*modelName=""}
	case "custom":
		if v:=os.Getenv("COTE_LLM_URL");v!=""{*endpoint=v}
	case "ui":
		*modelName=""
	default:log.Fatal("unknown provider")
	}
	if *steps<0||*steps>pilot.MaxRounds||*port<1||*port>65535 {log.Fatal("invalid rounds or port")}
	router,err:=pilot.LoadRouter(*routesPath);if err!=nil{log.Fatal(err)}
	chat,err:=pilot.OpenChatGPT(filepath.Join(configDir,"cote-ai-simulation","chatgpt.json"));if err!=nil{log.Fatal(err)}
	defer chat.Close()
	router.AttachChatGPT(chat)
	if len(router.Routes())==0&&*modelName!="" {
		kind:=*provider;if kind=="custom"{kind="openai-compatible"}
		if _,err:=router.Add(pilot.RouteInput{Provider:kind,Endpoint:*endpoint,Model:*modelName,Key:key,Format:"auto"});err!=nil{log.Fatal(err)}
	}
	session,err:=pilot.OpenCognition(*save,"router",router);if err!=nil{log.Fatal(err)}
	defer session.Close()
	ctx,cancel:=signal.NotifyContext(context.Background(),os.Interrupt,syscall.SIGTERM);defer cancel()
	if *steps>0 {
		for i:=0;i<*steps;i++ {
			s,err:=session.Step(ctx,session.Snapshot().State.Round);if err!=nil{log.Fatal(err)}
			fmt.Printf("Rodada %d | %02d:%02d | %d eventos\n",s.State.Round,s.State.Minute/60,s.State.Minute%60,len(s.State.Events))
		}
		return
	}
	addr:=net.JoinHostPort("127.0.0.1",strconv.Itoa(*port))
	mux:=http.NewServeMux()
	reply:=func(w http.ResponseWriter,status int,value any){
		w.Header().Set("Content-Type","application/json; charset=utf-8")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(value)
	}
	fail:=func(w http.ResponseWriter,status int,err error){reply(w,status,map[string]string{"error":err.Error()})}
	mux.HandleFunc("GET /{$}",func(w http.ResponseWriter,r *http.Request){
		w.Header().Set("Content-Type","text/html; charset=utf-8");_,_=w.Write(page)
	})
	mux.HandleFunc("POST /api/models",func(w http.ResponseWriter,r *http.Request){
		if r.Header.Get("Origin")!="http://"+addr||!strings.HasPrefix(r.Header.Get("Content-Type"),"application/json"){fail(w,403,errors.New("local browser request required"));return}
		var input pilot.RouteInput
		d:=json.NewDecoder(http.MaxBytesReader(w,r.Body,16384));d.DisallowUnknownFields()
		if err:=d.Decode(&input);err!=nil{fail(w,400,errors.New("configuração de catálogo inválida"));return}
		if err:=d.Decode(new(any));err!=io.EOF{fail(w,400,errors.New("trailing request data"));return}
		ctx,cancel:=context.WithTimeout(r.Context(),30*time.Second);defer cancel()
		var catalog pilot.ModelCatalog;var err error
		if input.Provider=="chatgpt"{catalog,err=chat.Models(ctx)}else{catalog,err=pilot.DiscoverModels(ctx,input)}
		if err!=nil{fail(w,400,err);return}
		reply(w,200,catalog)
	})
	mux.HandleFunc("GET /api/chatgpt",func(w http.ResponseWriter,r *http.Request){reply(w,200,chat.Status())})
	mux.HandleFunc("POST /api/chatgpt/start",func(w http.ResponseWriter,r *http.Request){
		if r.Header.Get("Origin")!="http://"+addr{fail(w,403,errors.New("local browser request required"));return}
		u,err:=chat.Start("http://"+addr+"/auth/callback");if err!=nil{fail(w,400,err);return}
		reply(w,200,map[string]string{"url":u})
	})
	mux.HandleFunc("GET /auth/callback",func(w http.ResponseWriter,r *http.Request){
		ctx,cancel:=context.WithTimeout(r.Context(),40*time.Second);defer cancel()
		err:=chat.Complete(ctx,r.URL.Query())
		if err==nil{router.AttachChatGPT(chat)}
		w.Header().Set("Content-Type","text/plain; charset=utf-8")
		if err!=nil{w.WriteHeader(400);fmt.Fprintln(w,err.Error());return}
		fmt.Fprintln(w,"Conta autorizada. Volte ao observatório e abra Modelos e APIs para selecionar um modelo.")
	})
	mux.HandleFunc("POST /api/chatgpt/logout",func(w http.ResponseWriter,r *http.Request){
		if r.Header.Get("Origin")!="http://"+addr{fail(w,403,errors.New("local browser request required"));return}
		if err:=chat.Logout(r.Context());err!=nil{fail(w,400,err);return};reply(w,200,chat.Status())
	})
	mux.HandleFunc("GET /api/routes",func(w http.ResponseWriter,r *http.Request){reply(w,200,router.Routes())})
	mux.HandleFunc("POST /api/routes",func(w http.ResponseWriter,r *http.Request){
		if r.Header.Get("Origin")!="http://"+addr||!strings.HasPrefix(r.Header.Get("Content-Type"),"application/json"){fail(w,403,errors.New("local browser request required"));return}
		var input struct{Action string `json:"action"`; ID string `json:"id"`; Route pilot.RouteInput `json:"route"`}
		d:=json.NewDecoder(http.MaxBytesReader(w,r.Body,16384));d.DisallowUnknownFields()
		if err:=d.Decode(&input);err!=nil{fail(w,400,errors.New("invalid route request"));return}
		if err:=d.Decode(new(any));err!=io.EOF{fail(w,400,errors.New("trailing route data"));return}
		var err error
		switch input.Action{
		case "add":_,err=router.Add(input.Route)
		case "remove","up","down","toggle":err=router.Change(input.ID,input.Action)
		case "save":err=router.Save(*routesPath)
		case "test":_,err=router.Test(r.Context(),input.ID)
		default:err=errors.New("unknown route action")
		}
		if err!=nil{fail(w,400,err);return};reply(w,200,router.Routes())
	})
	mux.HandleFunc("GET /api/report",func(w http.ResponseWriter,r *http.Request){
		data,err:=session.ReportZIP();if err!=nil{fail(w,500,errors.New("não foi possível gerar relatório"));return}
		w.Header().Set("Content-Type","application/zip")
		w.Header().Set("Content-Disposition","attachment; filename=cote-relatorio.zip")
		_,_=w.Write(data)
	})
	mux.HandleFunc("GET /api/state",func(w http.ResponseWriter,r *http.Request){reply(w,200,session.Snapshot())})
	mux.HandleFunc("GET /api/progress",func(w http.ResponseWriter,r *http.Request){reply(w,200,session.Progress())})
	mux.HandleFunc("POST /api/cancel",func(w http.ResponseWriter,r *http.Request){
		if r.Header.Get("Origin")!="http://"+addr||!strings.HasPrefix(r.Header.Get("Content-Type"),"application/json"){fail(w,403,errors.New("local browser request required"));return}
		var input struct{Round int `json:"round"`}
		d:=json.NewDecoder(http.MaxBytesReader(w,r.Body,1024));d.DisallowUnknownFields()
		if err:=d.Decode(&input);err!=nil||input.Round<1{fail(w,400,errors.New("expected round required"));return}
		if err:=d.Decode(new(any));err!=io.EOF{fail(w,400,errors.New("trailing request data"));return}
		reply(w,200,map[string]bool{"canceled":session.Cancel(input.Round)})
	})
	mux.HandleFunc("GET /api/pov",func(w http.ResponseWriter,r *http.Request){
		c,err:=session.POV(r.URL.Query().Get("actor"));if err!=nil{fail(w,400,err);return};reply(w,200,c)
	})
	mux.HandleFunc("POST /api/advance",func(w http.ResponseWriter,r *http.Request){
  if r.Header.Get("Origin")!="http://"+addr||!strings.HasPrefix(r.Header.Get("Content-Type"),"application/json"){fail(w,403,errors.New("local browser request required"));return}
  var input struct{Round *int `json:"round"`}
  d:=json.NewDecoder(http.MaxBytesReader(w,r.Body,1024));d.DisallowUnknownFields()
  if err:=d.Decode(&input);err!=nil||input.Round==nil{fail(w,400,errors.New("expected round required"));return}
  if err:=d.Decode(new(any));err!=io.EOF{fail(w,400,errors.New("trailing request data"));return}
  result,err:=session.AdvanceUntilEvent(r.Context(),*input.Round);if err!=nil{fail(w,409,err);return};reply(w,200,result)
 })
 mux.HandleFunc("POST /api/step",func(w http.ResponseWriter,r *http.Request){
		if r.Header.Get("Origin")!="http://"+addr||!strings.HasPrefix(r.Header.Get("Content-Type"),"application/json"){fail(w,403,errors.New("local browser request required"));return}
		var input struct{Round *int `json:"round"`}
		d:=json.NewDecoder(http.MaxBytesReader(w,r.Body,1024));d.DisallowUnknownFields()
		if err:=d.Decode(&input);err!=nil||input.Round==nil{fail(w,400,errors.New("expected round required"));return}
		if err:=d.Decode(new(any));err!=io.EOF{fail(w,400,errors.New("trailing request data"));return}
		s,err:=session.Step(r.Context(),*input.Round);if err!=nil{fail(w,409,err);return};reply(w,200,s)
	})
	handler:=http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
		if r.Host!=addr {http.Error(w,"local host required",403);return}
		w.Header().Set("Cache-Control","no-store")
		w.Header().Set("X-Content-Type-Options","nosniff")
		w.Header().Set("Content-Security-Policy","default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; connect-src 'self'; frame-ancestors 'none'")
		mux.ServeHTTP(w,r)
	})
	server:=&http.Server{Addr:addr,Handler:handler,ReadHeaderTimeout:5*time.Second,IdleTimeout:30*time.Second}
	listener,err:=net.Listen("tcp",addr);if err!=nil{log.Fatal(err)}
	go func(){<-ctx.Done();_ = server.Close()}()
	fmt.Println("Abra http://"+addr+" no navegador. Ctrl+C encerra; rodadas concluídas ficam salvas.")
	if err:=server.Serve(listener);err!=nil&&!errors.Is(err,http.ErrServerClosed){log.Fatal(err)}
}

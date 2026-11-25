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
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/carloshgalves/cote-ai-simulation/internal/pilot"
)

//go:embed index.html
var page []byte

func main() {
	modelName:=flag.String("model",os.Getenv("COTE_MODEL"),"LLM model ID")
	endpoint:=flag.String("endpoint","http://127.0.0.1:11434/v1","OpenAI-compatible base URL")
	save:=flag.String("save","cote-local.json","local simulation journal")
	port:=flag.Int("port",8765,"loopback HTTP port")
	steps:=flag.Int("steps",0,"run N rounds in terminal instead of browser")
	flag.Parse()
	if v:=os.Getenv("COTE_LLM_URL");v!="" { *endpoint=v }
	if *steps<0||*steps>pilot.MaxRounds||*port<1||*port>65535 {log.Fatal("invalid rounds or port")}
	model,err:=pilot.NewHTTPModel(*endpoint,*modelName,os.Getenv("COTE_API_KEY"));if err!=nil{log.Fatal(err)}
	session,err:=pilot.Open(*save,*modelName,model);if err!=nil{log.Fatal(err)}
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
	mux.HandleFunc("GET /api/state",func(w http.ResponseWriter,r *http.Request){reply(w,200,session.Snapshot())})
	mux.HandleFunc("GET /api/pov",func(w http.ResponseWriter,r *http.Request){
		c,err:=session.POV(r.URL.Query().Get("actor"));if err!=nil{fail(w,400,err);return};reply(w,200,c)
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

package main

import (
	"encoding/json"
	"fmt"
	"net/http"

	tgdata "github.com/carloshgalves/cote-ai-simulation/data/external-worlds/tomodachi-game"
)

type tgCatalogEntry struct {
	ID string `json:"id"`
	Name string `json:"display_name"`
	Tier string `json:"tier"`
	Pack string `json:"pack"`
}
type tgCatalog struct {
	World string `json:"world"`
	SourceRevision string `json:"source_revision"`
	SourceStatus string `json:"source_status"`
	Characters []tgCatalogEntry `json:"characters"`
	Execution json.RawMessage `json:"execution"`
}

func loadTomodachiCatalog() (tgCatalog,map[string]tgdata.Pack,error) {
	var catalog tgCatalog
	if err:=json.Unmarshal(tomodachiCatalog,&catalog);err!=nil{return catalog,nil,err}
	packs,err:=tgdata.Load();if err!=nil{return catalog,nil,err}
	if len(catalog.Characters)!=len(packs){return catalog,nil,fmt.Errorf("Tomodachi catalog coverage mismatch")}
	seen:=map[string]bool{}
	for i,e:=range catalog.Characters{
		p,ok:=packs[e.ID];if !ok||seen[e.ID]||p.Tier!=e.Tier{return catalog,nil,fmt.Errorf("Tomodachi catalog mismatch: %s",e.ID)}
		seen[e.ID]=true
		catalog.Characters[i].Name=p.Name
	}
	return catalog,packs,nil
}

func registerTomodachiEndpoints(mux *http.ServeMux,catalog tgCatalog,packs map[string]tgdata.Pack){
	write:=func(w http.ResponseWriter,status int,v any){
		w.Header().Set("Content-Type","application/json; charset=utf-8")
		w.Header().Set("Cache-Control","no-store")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(v)
	}
	mux.HandleFunc("GET /api/tomodachi/catalog",func(w http.ResponseWriter,r *http.Request){write(w,200,catalog)})
	mux.HandleFunc("GET /api/tomodachi/character",func(w http.ResponseWriter,r *http.Request){
		ids:=r.URL.Query()["id"]
		if len(ids)!=1{write(w,404,map[string]string{"error":"Personagem não encontrado"});return}
		p,ok:=packs[ids[0]]
		if !ok{write(w,404,map[string]string{"error":"Personagem não encontrado"});return}
		write(w,200,p)
	})
}

package pilot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

type CatalogModel struct {
	ID string `json:"id"`
	Name string `json:"name"`
	PriceHint string `json:"price_hint,omitempty"`
}
type ModelCatalog struct {
	Models []CatalogModel `json:"models"`
	Partial bool `json:"partial"`
}

// Discovery only lists models. It does not invoke generation or prove that
// the account has inference quota, access to a particular model, or free pricing.
func DiscoverModels(ctx context.Context,in RouteInput)(ModelCatalog,error){
	in,m,err:=normalizeRoute(in,false);if err!=nil{return ModelCatalog{},err}
	m.Client.Timeout=20*time.Second
	return discoverModels(ctx,in,m.Client)
}
func discoverModels(ctx context.Context,in RouteInput,client *http.Client)(ModelCatalog,error){
	base:=strings.TrimRight(in.Endpoint,"/")+"/models"
	if in.Provider=="gemini"{base="https://generativelanguage.googleapis.com/v1beta/models"}
	models:=map[string]CatalogModel{}
	cursor:=""
	seen:=map[string]bool{}
	partial:=false
	for page:=0;page<10;page++{
		u,err:=url.Parse(base);if err!=nil{return ModelCatalog{},errors.New("endpoint de catálogo inválido")}
		q:=u.Query()
		if in.Provider=="gemini"{q.Set("pageSize","100");if cursor!=""{q.Set("pageToken",cursor)}}
		if in.Provider=="anthropic"{q.Set("limit","100");if cursor!=""{q.Set("after_id",cursor)}}
		u.RawQuery=q.Encode()
		req,err:=http.NewRequestWithContext(ctx,http.MethodGet,u.String(),nil);if err!=nil{return ModelCatalog{},errors.New("consulta de catálogo inválida")}
		req.Header.Set("Accept","application/json")
		switch in.Provider{
		case "gemini":req.Header.Set("x-goog-api-key",in.Key)
		case "anthropic":req.Header.Set("x-api-key",in.Key);req.Header.Set("anthropic-version","2023-06-01")
		default:if in.Key!=""{req.Header.Set("Authorization","Bearer "+in.Key)}
		}
		res,err:=client.Do(req)
		if err!=nil{return ModelCatalog{},errors.New("não foi possível consultar o catálogo; verifique a conexão e o endpoint")}
		raw,err:=io.ReadAll(io.LimitReader(res.Body,8<<20+1));res.Body.Close()
		switch res.StatusCode{
		case 401,403:return ModelCatalog{},errors.New("chave inválida ou sem permissão para consultar modelos")
		case 429:return ModelCatalog{},errors.New("limite de consultas do provedor atingido (429); tente novamente mais tarde")
		case 404,405,501:return ModelCatalog{},errors.New("provedor não oferece catálogo neste endpoint; use a opção manual")
		}
		if res.StatusCode!=200{return ModelCatalog{},fmt.Errorf("consulta ao catálogo retornou HTTP %d",res.StatusCode)}
		if err!=nil||len(raw)>8<<20{return ModelCatalog{},errors.New("resposta do catálogo inválida ou muito grande")}
		var envelope struct{
			Data []struct{
				ID string `json:"id"`
				Name string `json:"name"`
				DisplayName string `json:"display_name"`
				Active *bool `json:"active"`
				Pricing struct{Prompt string `json:"prompt"`; Completion string `json:"completion"`} `json:"pricing"`
				Architecture struct{Output []string `json:"output_modalities"`} `json:"architecture"`
			} `json:"data"`
			Models []struct{
				Name string `json:"name"`
				DisplayName string `json:"displayName"`
				Methods []string `json:"supportedGenerationMethods"`
			} `json:"models"`
			NextPageToken string `json:"nextPageToken"`
			HasMore bool `json:"has_more"`
			LastID string `json:"last_id"`
		}
		if json.Unmarshal(raw,&envelope)!=nil{return ModelCatalog{},errors.New("formato de catálogo não reconhecido; use a opção manual")}
		add:=func(id,name,hint string){
			if !validText(id,200){return}
			if !validText(name,250){name=id}
			models[id]=CatalogModel{ID:id,Name:name,PriceHint:hint}
		}
		next:=""
		if in.Provider=="gemini"{
			for _,m:=range envelope.Models{
				if !has(m.Methods,"generateContent"){continue}
				id:=strings.TrimPrefix(m.Name,"models/")
				// generateContent also lists specialized media models. This
				// observatory uses only text generation.
				if nonChatModel(id){continue}
				add(id,m.DisplayName,"")
			}
			next=envelope.NextPageToken
		}else{
			for _,m:=range envelope.Data{
				if m.Active!=nil&&!*m.Active||nonChatModel(m.ID){continue}
				if len(m.Architecture.Output)>0&&!has(m.Architecture.Output,"text"){continue}
				name:=m.DisplayName;if name==""{name=m.Name}
				hint:=""
				if in.Provider=="openrouter"&&(strings.HasSuffix(m.ID,":free")||(m.Pricing.Prompt=="0"&&m.Pricing.Completion=="0")){hint="Preço de tokens indicado como zero"}
				add(m.ID,name,hint)
			}
			if envelope.HasMore{
				if in.Provider=="anthropic"{next=envelope.LastID}else{partial=true}
				if next==""{partial=true}
			}
		}
		if next==""{break}
		if seen[next]||page==9{partial=true;break}
		seen[next]=true;cursor=next
	}
	out:=ModelCatalog{Models:[]CatalogModel{},Partial:partial}
	for _,m:=range models{out.Models=append(out.Models,m)}
	sort.Slice(out.Models,func(i,j int)bool{
		a,b:=out.Models[i],out.Models[j]
		if (a.PriceHint!="")!=(b.PriceHint!=""){return a.PriceHint!=""}
		return strings.ToLower(a.ID)<strings.ToLower(b.ID)
	})
	return out,nil
}
func nonChatModel(id string)bool{
	id=strings.ToLower(id)
	for _,term:=range []string{"embedding","embed-","whisper","tts","speech-to-text","text-to-speech","rerank","moderation","imagen","veo-","image-generation","-image","-audio","-live","transcribe","dall-e","sora-","guard-"}{
		if strings.Contains(id,term){return true}
	}
	return false
}

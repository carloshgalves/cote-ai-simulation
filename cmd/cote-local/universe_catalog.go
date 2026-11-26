package main

import(
 "encoding/json"
 "net/http"
 canondata "github.com/carloshgalves/cote-ai-simulation/data/canon"
 tgdata "github.com/carloshgalves/cote-ai-simulation/data/external-worlds/tomodachi-game"
)
// Operator-only local UI registry, deliberately not part of agent Context.
// Canon associations below are historical/source-qualified, not live authority.
type institutionalLink struct {
 Actor string `json:"actor"`
 Relation string `json:"relation"`
}
var institutionalLinks=map[string][]institutionalLink{
 "org.anhs":{{"actor.sakayanagi-father","Vínculo administrativo documentado; poderes atuais dependem da cronologia"},{"actor.tsukishiro","Possível atuação posterior; não admitido na escola inicialmente"}},
 "org.white_room":{{"actor.atsuomi-ayanokoji","Fundador/diretor segundo o pack"},{"actor.kiyotaka-ayanokoji","Histórico de treinamento"},{"actor.shiro","Histórico de treinamento; desligado"},{"actor.yuki","Histórico de treinamento"}},
 "org.naoe_faction":{{"actor.jinnosuke-naoe","Liderança no período documentado"}},
 "org.citizens_party":{{"actor.jinnosuke-naoe","Operador político veterano"},{"actor.isomaru","Liderança de facção rival"}},
 "org.kijima_faction":{{"actor.kijima","Figura central no período documentado"}},
 "org.isomaru_faction":{{"actor.isomaru","Liderança no período documentado"}},
 "org.miyako_faction":{{"actor.miyako","Liderança no período de lançamento do projeto"}},
 "org.ooba_network":{{"actor.ooba","Associação descrita no pack"}},
 "org.kanzaki_engineering":{{"actor.tomohiro-kanzaki","Presidência descrita no pack"}},
 "org.ishigami_group":{{"actor.goro-ishigami","Presidência descrita no pack"}},
 "org.student-council":{{"actor.manabu-horikita","Presidente no início do cenário"},{"actor.nagumo","Membro veterano"},{"actor.tachibana","Membro veterana"}},
}
func registerUniverseCatalog(mux *http.ServeMux)error{
 chars,err:=canondata.Catalog();if err!=nil{return err}
 orgs,err:=canondata.Institutions();if err!=nil{return err}
 exams,err:=canondata.ExamReferences();if err!=nil{return err}
 games,err:=tgdata.ExamReferences();if err!=nil{return err}
 type catalog struct {
  Characters []canondata.Metadata `json:"characters"`
  Institutions []canondata.Institution `json:"institutions"`
  Links map[string][]institutionalLink `json:"links"`
  Exams []canondata.ExamReference `json:"exams"`
  Games []tgdata.ExamReference `json:"games"`
 }
 raw,err:=json.Marshal(catalog{chars,orgs,institutionalLinks,exams,games});if err!=nil{return err}
 mux.HandleFunc("GET /api/universe/catalog",func(w http.ResponseWriter,r *http.Request){w.Header().Set("Content-Type","application/json; charset=utf-8");_,_=w.Write(raw)})
 return nil
}

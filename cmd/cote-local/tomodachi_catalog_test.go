package main

import (
 "encoding/json"
 "net/http"
 "net/http/httptest"
 "testing"
)

func TestTomodachiCharacterCatalog(t *testing.T) {
 catalog,packs,err:=loadTomodachiCatalog()
 if err!=nil {t.Fatal(err)}
 if len(catalog.Characters)!=60||len(packs)!=60 {t.Fatalf("catalog=%d packs=%d",len(catalog.Characters),len(packs))}
 for _,entry:=range catalog.Characters {
  p,ok:=packs[entry.ID];if !ok||entry.Name==""||entry.Name!=p.Name||entry.Tier!=p.Tier {t.Fatalf("inconsistent entry: %+v",entry)}
 }
 mux:=http.NewServeMux()
 registerTomodachiEndpoints(mux,catalog,packs)
 for _,entry:=range catalog.Characters {
  w:=httptest.NewRecorder()
  mux.ServeHTTP(w,httptest.NewRequest("GET","/api/tomodachi/character?id="+entry.ID,nil))
  if w.Code!=200 {t.Fatalf("%s status=%d",entry.ID,w.Code)}
  var p struct {ID string `json:"character_id"`;Name string `json:"display_name"`}
  if err=json.Unmarshal(w.Body.Bytes(),&p);err!=nil||p.ID!=entry.ID||p.Name!=entry.Name{t.Fatalf("invalid detail %s: %v",entry.ID,err)}
 }
 for _,id:=range []string{"","actor.unknown","../characters/yuuichi-katagiri.json","ayanokoji"} {
  w:=httptest.NewRecorder();mux.ServeHTTP(w,httptest.NewRequest("GET","/api/tomodachi/character?id="+id,nil))
  if w.Code!=404 {t.Errorf("unexpected result for %q: %d",id,w.Code)}
 }
}

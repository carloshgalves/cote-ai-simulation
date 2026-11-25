package main

import (
 "encoding/json"
 "testing"
)
func TestTomodachiCatalogEmbedded(t *testing.T){
 var v struct{
  World string `json:"world"`
  SourceRevision string `json:"source_revision"`
  Characters []struct{ID string `json:"id"`;Tier string `json:"tier"`;Pack string `json:"pack"`} `json:"characters"`
 }
 if err:=json.Unmarshal(tomodachiCatalog,&v);err!=nil{t.Fatal(err)}
 if v.World!="tomodachi-game"||len(v.Characters)!=60||len(v.SourceRevision)!=40{t.Fatal("catalog scope or coverage")}
 seen:=map[string]bool{};for _,p:=range v.Characters{if p.ID==""||p.Tier==""||p.Pack==""||seen[p.ID]{t.Fatal("invalid catalog metadata")};seen[p.ID]=true}
}

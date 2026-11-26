package canondata
import("embed";"encoding/json";"io/fs";"fmt";"sort")
//go:embed consolidated/characters/*.json
var packs embed.FS
type Metadata struct{
 ID string `json:"character_id"`
 Name string `json:"display_name"`
 Tier string `json:"modeling_tier"`
 SourceStatus string `json:"source_status"`
}
func Catalog()([]Metadata,error){
 names,err:=fs.Glob(packs,"consolidated/characters/*.json");if err!=nil{return nil,err};sort.Strings(names)
 out:=[]Metadata{};seen:=map[string]bool{}
 for _,name:=range names{raw,err:=packs.ReadFile(name);if err!=nil{return nil,err};var p Metadata;if err:=json.Unmarshal(raw,&p);err!=nil{return nil,err};if p.ID==""||seen[p.ID]{return nil,fmt.Errorf("invalid corpus metadata: %s",name)};seen[p.ID]=true;out=append(out,p)}
 return out,nil
}

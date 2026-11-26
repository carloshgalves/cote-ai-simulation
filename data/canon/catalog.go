package canondata
import("embed";"encoding/json";"io/fs";"fmt";"sort")
//go:embed consolidated/characters/*.json consolidated/institutions/*.json consolidated/exams/*.json
var packs embed.FS
type Metadata struct{
 ID string `json:"character_id"`
 Name string `json:"display_name"`
 Tier string `json:"modeling_tier"`
 SourceStatus string `json:"source_status"`
 Identity struct{RoleSummary string `json:"role_summary"`} `json:"identity"`
}
func Catalog()([]Metadata,error){
 names,err:=fs.Glob(packs,"consolidated/characters/*.json");if err!=nil{return nil,err};sort.Strings(names)
 out:=[]Metadata{};seen:=map[string]bool{}
 for _,name:=range names{raw,err:=packs.ReadFile(name);if err!=nil{return nil,err};var p Metadata;if err:=json.Unmarshal(raw,&p);err!=nil{return nil,err};if p.ID==""||seen[p.ID]{return nil,fmt.Errorf("invalid corpus metadata: %s",name)};seen[p.ID]=true;out=append(out,p)}
 return out,nil
}

type Institution struct{
 ID string `json:"institution_id"`
 Name string `json:"display_name"`
 Status string `json:"source_status"`
 Evidence []string `json:"evidence_refs"`
}
type ExamReference struct{
 ID string `json:"exam_id"`
 Name string `json:"name"`
 Title string `json:"title"`
 Authority string `json:"authority"`
}
func Institutions()([]Institution,error){
 files,err:=fs.Glob(packs,"consolidated/institutions/*.json");if err!=nil{return nil,err}
 out:=[]Institution{};seen:=map[string]bool{};for _,file:=range files{raw,err:=packs.ReadFile(file);if err!=nil{return nil,err};var x Institution;if err:=json.Unmarshal(raw,&x);err!=nil{return nil,err};if x.ID==""||seen[x.ID]{return nil,fmt.Errorf("invalid institution %s",file)};seen[x.ID]=true;out=append(out,x)};return out,nil
}
func ExamReferences()([]ExamReference,error){
 files,err:=fs.Glob(packs,"consolidated/exams/*.json");if err!=nil{return nil,err}
 out:=[]ExamReference{};for _,file:=range files{raw,err:=packs.ReadFile(file);if err!=nil{return nil,err};var x ExamReference;if err:=json.Unmarshal(raw,&x);err!=nil{return nil,err};if x.ID==""{continue};if x.Name==""{x.Name=x.Title};if x.Name==""{x.Name=x.ID};out=append(out,x)};return out,nil
}

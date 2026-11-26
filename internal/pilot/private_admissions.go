package pilot

import (
 "errors"
 "fmt"
 "strings"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
)
type NextAdmission struct{
 ID string `json:"id"`
 Name string `json:"name"`
 AdmissionYear int `json:"admission_year"`
 Class string `json:"class"`
 Traits []string `json:"traits"`
 Goals []string `json:"goals"`
 Background []string `json:"background"`
 Source string `json:"source"`
}
type NextAdmissions struct{
 Schema string `json:"schema"`
 Candidates []NextAdmission `json:"candidates"`
}
func ValidateNextAdmissions(path string,s Snapshot)error{
 var p NextAdmissions;if err:=readPrivate(path,&p);err!=nil{return err}
 if p.Schema!="private-admissions-v1"||len(p.Candidates)>160{return errors.New("invalid private admissions manifest")}
 seen:=map[string]bool{};for _,v:=range s.People{seen[v.ID]=true}
 for _,c:=range p.Candidates{
  if !strings.HasPrefix(c.ID,"oc-")||domain.ValidateMachineID(c.ID)!=nil||seen[c.ID]||!validText(c.Name,100)||c.AdmissionYear!=2||!has([]string{"A","B","C","D","auto"},c.Class)||!validText(c.Source,300)||len(c.Traits)<1||len(c.Traits)>8||len(c.Goals)<1||len(c.Goals)>5||len(c.Background)>8{return fmt.Errorf("invalid private candidate")}
  for _,list:=range [][]string{c.Traits,c.Goals,c.Background}{for _,x:=range list{if !validText(x,500){return errors.New("invalid candidate text")}}};seen[c.ID]=true
 }
 return nil
}

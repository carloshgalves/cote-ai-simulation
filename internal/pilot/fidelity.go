package pilot
import(
 "encoding/json"
 "fmt"
 "sort"
 "strings"
 canondata "github.com/carloshgalves/cote-ai-simulation/data/canon"
 tgdata "github.com/carloshgalves/cote-ai-simulation/data/external-worlds/tomodachi-game"
)
type PackCoverage struct{World string `json:"world"`; ID string `json:"id"`; Tier string `json:"tier"`; Status string `json:"status"`; Admitted string `json:"admitted,omitempty"`}
type BehaviorGuidance struct{Lesson string `json:"lesson"`; Source string `json:"source"`; Topic string `json:"topic"`}
func packActor(j Journal,id string)string{
 aliases:=map[string]string{"ayanokoji":"kiyotaka-ayanokoji","horikita":"suzune-horikita","ryuen":"kakeru-ryuen","kushida":"kikyo-kushida","hirata":"yosuke-hirata","ichinose":"honami-ichinose"}
 for _,p:=range j.Profiles{if strings.TrimPrefix(p.ID,"tg-")==id&&!strings.HasPrefix(p.ID,"tg-"){return p.ID};if aliases[p.ID]==id{return p.ID}}
 return ""
}
func attachCorpus(j *Journal)error{
 c,err:=canondata.Catalog();if err!=nil{return err};tg,err:=tgdata.Load();if err!=nil{return err}
 j.PackInventory=nil;j.Guidance=map[string][]BehaviorGuidance{}
 for _,p:=range c{j.PackInventory=append(j.PackInventory,PackCoverage{"cote",p.ID,p.Tier,p.SourceStatus,packActor(*j,strings.TrimPrefix(p.ID,"actor."))})}
 keys:=[]string{};for k:=range tg{keys=append(keys,k)};sort.Strings(keys)
 for _,k:=range keys{p:=tg[k];id:="tg-"+strings.TrimPrefix(p.ID,"actor.");if !known(j.Profiles,id){id=""};j.PackInventory=append(j.PackInventory,PackCoverage{"tomodachi",p.ID,p.Tier,p.SourceStatus,id})}
 var g struct{Policy string `json:"policy"`;Records []struct{Actor string `json:"actor"`; Lesson string `json:"lesson"`; CaseRef string `json:"case_ref"`; Game string `json:"game"`} `json:"records"`}
 if err:=json.Unmarshal(tgdata.GuidanceJSON(),&g);err!=nil{return err};if g.Policy!="OPERATOR_ADMITTED_ABSTRACT_GUIDANCE_ONLY"{return fmt.Errorf("unexpected guidance policy")}
 for _,v:=range g.Records{id:="tg-"+strings.TrimPrefix(v.Actor,"actor.");if known(j.Profiles,id){j.Guidance[id]=append(j.Guidance[id],BehaviorGuidance{v.Lesson,v.CaseRef,v.Game})}}
 // Conservative COTE priors are abstract, not case narratives. Freeze
 // exact source links in the operator journal; the model sees lesson only.
 for id,gs:=range map[string][]BehaviorGuidance{
 "ayanokoji":{{"Consider several possible strategies; reveal only what serves your current goals. Competence does not establish omniscience.","y1v1.case.k.swim-middle","strategy"}},
 "horikita":{{"Prioritize independent academic merit; weigh the cost of cooperation without presuming friendship.","y1v1.case.s.refuse-forced-friendship","social"}},
 }{if known(j.Profiles,id){j.Guidance[id]=append(j.Guidance[id],gs...)}}
 return nil
}
func selectBehaviorGuidance(j Journal,s State,id string)[]string{
 e:=examFor(s,id);active:="";if e!=nil&&e.Outcome==0{active=map[string]string{"kokkuri":"kokkuri-san","janken":"friendship-janken","hide":"hide-and-seek","trial":"friend-sin-trial"}[e.Kind]}
 out:=[]string{};for _,g:=range j.Guidance[id]{if g.Topic=="*"||g.Topic=="social"||g.Topic=="strategy"||active!=""&&g.Topic==active{out=append(out,g.Lesson);if len(out)>=4{break}}};return out
}
func contextualMemoryScore(s State,id string,e Event,p MemoryCapabilityProfile)int64{
 m:=s.Minds[id];score:=int64(0)
 if m.Conversation!=nil&&(e.Actor==m.Conversation.Peer||e.Target==m.Conversation.Peer){score+=int64(p.Retrieval)*2}
 if e.Kind=="exam"{if current:=examFor(s,id);current!=nil&&current.Outcome==0{score+=int64(p.Retrieval)}}
 if e.Kind=="claim"||e.Kind=="meeting"||e.Kind=="social"{if m.Social[e.Actor].Interactions>0||m.Social[e.Target].Interactions>0{score+=int64(p.Encoding)}}
 return score
}
func accountPractice(m *Mind,from,to int64){
 if m.Activity==nil||m.Activity.Kind!="task"||to<=from{return}
 key:="";switch m.Activity.Target{
 case "lesson:","study:":subjects:=[]string{"matemática","língua","ciências","história","inglês"};key="study/"+subjects[(from/1440)%5]
 case "read:":key="reading"
 case "exercise:":key="exercise"
 }
 if key==""{return};if m.PracticeMinutes==nil{m.PracticeMinutes=map[string]int64{}}
 m.PracticeMinutes[key]+=to-from
}

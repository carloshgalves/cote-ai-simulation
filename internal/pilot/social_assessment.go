package pilot
import("errors";"fmt")
type SocialAssessment struct{
 Person string `json:"person"`
 Dimension string `json:"dimension"`
 Delta int `json:"delta"`
 Evidence string `json:"evidence"`
}
func validateAssessment(c Context,a SocialAssessment)error{
 if a.Person==c.Self.ID||!has([]string{"trust","affinity","rivalry","esteem","affection"},a.Dimension)||(a.Delta!=1&&a.Delta!=-1){return errors.New("invalid assessment")}
 known:=false;for _,p:=range c.People{if p.ID==a.Person{known=true}};if !known{return errors.New("unknown social subject")}
 e,ok:=evidence(c,a.Evidence);if !ok||!has([]string{"claim","meeting","social","resource","exam","relation"},e.Kind)||(e.Actor!=a.Person&&e.Target!=a.Person)||!has(e.Audience,c.Self.ID){return errors.New("assessment requires witnessed relevant event")}
 if e.Kind=="claim"&&e.Actor==c.Self.ID{return errors.New("own assertion cannot establish peer behavior")}
 bond:=c.Social[a.Person];if bond.AssessmentEvidence[a.Dimension+"/"+a.Evidence]{return errors.New("assessment evidence already used")}
 if a.Dimension=="affection"{self,peer:=c.Population[c.Self.ID],c.Population[a.Person];if self.Role!="student"||peer.Role!="student"||self.Year!=peer.Year||bond.Interactions<3{return errors.New("affection requires repeated same-year student interaction")}}
 return nil
}
func boundedSocial(n int)int{if n>10{return 10};if n< -10{return -10};return n}
func applyAssessment(s *State,id string,m *Mind,a SocialAssessment){
 if m.Social==nil{m.Social=map[string]SocialBond{}}
 b:=m.Social[a.Person];if b.AssessmentEvidence==nil{b.AssessmentEvidence=map[string]bool{}}
 key:=a.Dimension+"/"+a.Evidence;if b.AssessmentEvidence[key]{return};b.AssessmentEvidence[key]=true
 switch a.Dimension{case "trust":b.Trust=boundedSocial(b.Trust+a.Delta);case "affinity":b.Affinity=boundedSocial(b.Affinity+a.Delta);case "rivalry":b.Rivalry=boundedSocial(b.Rivalry+a.Delta);case "esteem":b.Esteem=boundedSocial(b.Esteem+a.Delta);case "affection":b.Affection=boundedSocial(b.Affection+a.Delta)}
 m.Social[a.Person]=b
 appendEvent(s,Event{Kind:"relation",Actor:id,Target:a.Person,Text:fmt.Sprintf("Avaliação subjetiva %s: %+d",a.Dimension,a.Delta),Evidence:a.Evidence,Audience:[]string{id}})
}
func shouldEndDialogue(j Journal,s State,id,peer string,c *Conversation)bool{
 turns,minutes:=12,int64(25)
 if j.Version>=SocialVersion{
  strategic:=false
  for _,p:=range []string{id,peer}{m:=s.Minds[p];if m.Plan!=nil||len(m.Plans)>0{strategic=true};if e:=examFor(s,p);e!=nil&&e.Outcome==0{strategic=true}}
  if strategic{turns,minutes=24,60}else{turns,minutes=8,20}
 }
 return c.Turns>=turns||c.Stale>=2||s.Minute-c.Started>=minutes
}

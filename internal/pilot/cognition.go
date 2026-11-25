package pilot

import (
 "errors"
 "sort"
 "strings"
)
const schoolDisclosure int64=30*1440+480
const classGuarantee="Somente os formandos da Classe A recebem a garantia institucional de recomendação para universidade e emprego; isso não significa que as demais classes não possam se formar."
func examBase(version int)int64{if version>=CognitionVersion{return 31*1440};return 1440}

type BackgroundFact struct{
 ID string `json:"id"`
 Text string `json:"text"`
 Source string `json:"source"`
 Person string `json:"person,omitempty"`
}
type MemoryCapabilityProfile struct{
 Encoding int `json:"encoding"`
 RetentionDays int64 `json:"retention_days"`
 Accessibility int `json:"accessibility"`
 Retrieval int `json:"retrieval"`
 WorkingMemory int `json:"working_memory"`
 Reasoning string `json:"reasoning"`
}
type CharacterIdentity struct{
 Values []string `json:"values"`
 Motivations []string `json:"motivations"`
 Vulnerabilities []string `json:"vulnerabilities"`
 Capacities []string `json:"capacities"`
 Background []BackgroundFact `json:"background"`
 Memory MemoryCapabilityProfile `json:"memory_profile"`
 Status string `json:"status"`
}
func initializeIdentities(j *Journal){
 j.Identities=map[string]CharacterIdentity{}
 for i,p:=range j.Profiles{
  // Admission-time goals cannot presuppose a rule that has not been learned.
  switch p.ID{
  case "horikita":p.Goals=[]string{"Obter excelente desempenho acadêmico e agir com autonomia","Conquistar reconhecimento por mérito próprio"}
  case "ryuen":p.Goals=[]string{"Estabelecer autonomia e influência entre colegas","Avaliar oportunidades concretas de obter vantagens pessoais"}
  case "ichinose":p.Goals=[]string{"Construir confiança com colegas","Ajudar colegas em dificuldades concretas"}
  case "hirata":p.Goals=[]string{"Integrar os colegas","Evitar conflitos e exclusão"}
  }
  j.Profiles[i]=p
  ident:=CharacterIdentity{Values:clone(p.Traits),Motivations:clone(p.Goals),Vulnerabilities:[]string{},Capacities:[]string{"Capacidade específica não estabelecida; não presumir desempenho excepcional."},Background:[]BackgroundFact{},Memory:MemoryCapabilityProfile{Encoding:65,RetentionDays:7,Accessibility:65,Retrieval:65,WorkingMemory:12,Reasoning:"Contextual, compatível com a identidade."},Status:"PROVISIONAL: perfil inicial; parâmetros de memória são escolhas do simulador, não medidas canônicas."}
  ident.Values=append(ident.Values,canonFor(p.ID).Dispositions...)
  for i,x:=range p.Background{if !strings.Contains(x,"Perfil provisório"){ident.Background=append(ident.Background,BackgroundFact{ID:"bio-"+p.ID+"-"+string(rune('a'+i)),Text:x,Source:j.Population[p.ID].Source})}}
  switch p.ID{
  case "ayanokoji":
   ident.Memory=MemoryCapabilityProfile{95,90,95,95,32,"Analítico e capaz de inferências complexas; não onisciente."}
   ident.Capacities=[]string{"Treinamento intelectual e físico intensivo anterior à matrícula; capacidade exata não divulgada."}
   ident.Vulnerabilities=[]string{"Deseja preservar a discrição e a liberdade pessoal."}
  case "horikita":
   ident.Memory=MemoryCapabilityProfile{85,30,80,85,22,"Analítico, acadêmico e inicialmente pouco hábil socialmente."}
   ident.Capacities=[]string{"Disposição acadêmica e autossuficiência."}
   ident.Vulnerabilities=[]string{"Necessidade de reconhecimento do irmão; dificuldade em formar vínculos."}
   ident.Background=append(ident.Background,BackgroundFact{"bio-horikita-manabu","Manabu Horikita é meu irmão mais velho; desejo seu reconhecimento.","data/canon/characters/suzune-horikita.yaml","manabu-horikita"})
  case "ryuen":
   ident.Memory=MemoryCapabilityProfile{80,30,80,85,22,"Estratégico, pragmático, disposto a testar hipóteses."}
   ident.Vulnerabilities=[]string{"Confiança na capacidade de controlar situações; pode assumir riscos."}
  case "kushida":
   ident.Memory=MemoryCapabilityProfile{85,30,85,90,22,"Atenção a relações, reputação e confidências."}
   ident.Vulnerabilities=[]string{"Teme exposição do próprio passado e perda da aprovação social."}
   ident.Background=append(ident.Background,
    BackgroundFact{"bio-kushida-horikita","Suzune Horikita estudou na mesma escola de ensino fundamental que eu. Reconheço-a e não sei quanto ela sabe do meu passado.","y1v6.ev.016; evt.y1.v06.kushida-juniorhigh-secret-breach","horikita"},
    BackgroundFact{"bio-kushida-past","Antes da ANHS, escrevi reclamações anônimas num blog; após a exposição, divulguei confidências de colegas, provocando conflitos. Conheço meus próprios atos e quero preservar minha reputação.","y1v6.ev.001; y1v6.ev.002; y1v6.ev.051",""})
  case "arisu-sakayanagi":
   p.Goals=[]string{"Investigar a natureza do talento e enfrentar desafios intelectuais"}
   ident.Memory=MemoryCapabilityProfile{95,90,95,95,30,"Estratégico e analítico; valoriza a hipótese de talento inato."}
   ident.Motivations=clone(p.Goals)
   ident.Background=append(ident.Background,BackgroundFact{"bio-arisu-kiyotaka","Na infância observei Kiyotaka resolvendo tarefas de xadrez em uma instalação branca acompanhada de meu pai. Ele não me viu. Quero testar minha crença sobre talento inato e educação.","evt.pre-y1.arisu-white-room-observation","ayanokoji"})
  case "hirata":
   ident.Background=append(ident.Background,BackgroundFact{"bio-hirata-friend","Antes da ANHS um amigo sofreu bullying e sobreviveu a uma tentativa de suicídio; tentei conter conflitos de forma coercitiva e essa experiência afetou minha maneira de lidar com colegas.","evt.pre-y1.hirata-bullied-friend-attempted-suicide",""})
   ident.Vulnerabilities=[]string{"Conflitos e exclusão remetem à experiência anterior."}
  case "ichinose":
   ident.Background=append(ident.Background,BackgroundFact{"bio-ichinose-hairclip","No ensino fundamental furtei uma presilha para minha irmã e sinto culpa pelo episódio; não é conhecimento público.","evt.pre-y1.ichinose-hairclip-shoplifting",""})
   ident.Vulnerabilities=[]string{"Culpa pelo episódio anterior; valoriza proteger colegas."}
  case "manabu-horikita":
   ident.Background=append(ident.Background,BackgroundFact{"bio-manabu-suzune","Suzune Horikita é minha irmã mais nova.","data/canon/characters/suzune-horikita.yaml","horikita"})
  }
  if j.Population[p.ID].Year==1&&p.ID!="horikita"&&p.ID!="ryuen"&&p.ID!="ichinose"&&p.ID!="hirata"{
   // Replace goals containing explicit class-rank competition, not the class label.
   for k,g:=range p.Goals{if strings.Contains(strings.ToLower(g),"oportunidades da classe")||strings.Contains(strings.ToLower(g),"riscos e oportunidades da classe"){p.Goals[k]="Avaliar oportunidades acadêmicas e sociais concretas"}}
   j.Profiles[i]=p;ident.Motivations=clone(p.Goals)
  }
  j.Identities[p.ID]=ident
 }
}
func validateIdentities(j Journal)error{
 if len(j.Identities)!=len(j.Profiles){return errors.New("missing character identities")}
 for _,p:=range j.Profiles{
  x,ok:=j.Identities[p.ID];if !ok||x.Memory.WorkingMemory<1||x.Memory.WorkingMemory>64||x.Memory.RetentionDays<1||x.Memory.RetentionDays>365||len(x.Background)>16{return errors.New("invalid identity")}
  for _,f:=range x.Background{if !validText(f.ID,200)||!validText(f.Text,1000)||!validText(f.Source,500)||f.Person!=""&&!known(j.Profiles,f.Person){return errors.New("invalid background fact")}}
 }
 return nil
}
func initializeCognition(j Journal,s *State){
 // Biographical facts stay in identity, not in the run's episodes.
 for _,id:=range ids(j.Profiles){
  m:=s.Minds[id]
  m.Body=&PilotBody{Energy:850,Hydration:900,Fatigue:100}
  m.Social=map[string]SocialBond{};m.Plans=map[string]PlanChange{}
  s.Minds[id]=m
 }
}
func projectIdentity(j Journal,s State,id string,c *Context){
 x:=clone(j.Identities[id]);c.Identity=&x
 m:=s.Minds[id];c.Body=clone(m.Body);c.Social=clone(m.Social);c.Plans=clone(m.Plans)
 if m.PendingStimulus!=""{c.WakeReason=m.PendingStimulus}
 c.OwnCanon.PrivateKnowledge=nil;c.OwnCanon.Dispositions=nil
 c.OwnCanon.Status="Perfil inicial em identity; não recuperar acontecimentos futuros."
 // Do not expose old self.goals or background fields containing redundant
 // knowledge; use the frozen identity instead.
 c.Self.Background=nil
 for _,f:=range x.Background{if f.Person!=""{for _,p:=range c.Present{if p.ID==f.Person&&!has(c.Recognized,p.ID){c.Recognized=append(c.Recognized,p.ID)}}}}
 sort.Strings(c.Recognized)
}
func retrieveMemory(j Journal,s State,id string,events []Event,keep map[string]bool)[]Event{
 p:=j.Identities[id].Memory
 type candidate struct{e Event;score int64}
 pool:=[]candidate{}
 out:=[]Event{}
 for _,e:=range events{
  if keep[e.ID]||e.Kind=="revelation"{out=append(out,clone(e));continue}
  age:=s.Minute-e.Minute
  if age>p.RetentionDays*1440{continue}
  score:=-age*int64(101-p.Accessibility)/60
  if has([]string{"claim","meeting","exam","resource","social"},e.Kind){score+=int64(p.Encoding)*2}
  if e.Actor!=id&&e.Actor!=""&&s.Minds[e.Actor].Location==s.Minds[id].Location{score+=int64(p.Retrieval)}
  if age<120{score+=int64(p.Accessibility)}
  pool=append(pool,candidate{e,score})
 }
 sort.SliceStable(pool,func(i,k int)bool{if pool[i].score!=pool[k].score{return pool[i].score>pool[k].score};return pool[i].e.ID>pool[k].e.ID})
 for i,x:=range pool{if i>=p.WorkingMemory{break};out=append(out,clone(x.e))}
 sort.Slice(out,func(i,k int)bool{return out[i].ID<out[k].ID})
 return out
}

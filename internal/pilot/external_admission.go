package pilot

import (
 "fmt"
 "sort"
 "strings"
)

const tgSource="https://shonenmagazine.com/special_page/tomodachigame/"
type ExternalAdmission struct{
 Slug,Name,Source string
 Year int
 Traits,Goals,Capacities,Limits []string
}
var externalAdmissions=[]ExternalAdmission{
 {"yuuichi-katagiri","Yuuichi Katagiri",tgSource,1,[]string{"Valoriza amizade; observa incentivos e administra o que revela"},[]string{"Construir relações de confiança","Avaliar oportunidades e proteger colegas"},[]string{"Raciocínio sobre incentivos e persuasão; resultados não garantidos"},[]string{"Apego a amigos e recursos limitados"}},
 {"tenji-mikasa","Tenji Mikasa",tgSource,1,[]string{"Acadêmico, comunicativo e analítico"},[]string{"Obter desempenho acadêmico","Construir relações e verificar suspeitas"},[]string{"Estudo e mediação; inferências dependem de evidência"},[]string{"Pode fixar-se numa hipótese"}},
 {"shiho-sawaragi","Shiho Sawaragi",tgSource,1,[]string{"Senso de justiça, franqueza e disciplina"},[]string{"Construir confiança com colegas","Defender tratamento justo"},[]string{"Prática de artes marciais e responsabilidade social; sem medidas físicas"},[]string{"Confiança direta pode ser explorada"}},
 {"makoto-shibe","Makoto Shibe",tgSource,1,[]string{"Sociável, espontâneo, esportivo"},[]string{"Integrar os colegas","Desenvolver aptidões esportivas"},[]string{"Aptidão esportiva descrita pela editora; sem medida máxima"},[]string{"Impulsividade"}},
 {"yutori-kokorogi","Yutori Kokorogi",tgSource,1,[]string{"Apresentação reservada; aprecia mangá e desenho"},[]string{"Construir relações sociais seguras","Preservar autonomia e privacidade"},[]string{"Interesse artístico; comportamento público não prova intenções privadas"},[]string{"História de exclusão social; detalhes temporais incertos"}},
 {"kei-shinomiya","Kei Shinomiya","https://tomodachi-game.fandom.com/wiki/Kei_Shinomiya",1,[]string{"Analítico, competitivo, atento à competência alheia"},[]string{"Enfrentar desafios de raciocínio","Avaliar oportunidades de cooperação"},[]string{"Planejamento e dedução; treino marcial descrito em fonte secundária"},[]string{"Dependência de informação e de parceiros"}},
 {"banri-niwa","Banri Niwa","https://tomodachi-game.fandom.com/wiki/Banri_Niwa",3,[]string{"Reflexivo e pragmático; interesse esportivo"},[]string{"Ajudar colegas","Desenvolver aptidões esportivas"},[]string{"Experiência esportiva e reflexão"},[]string{"Dados comportamentais esparsos"}},
 {"juuzou-kadokura","Juuzou Kadokura","https://tomodachi-game.fandom.com/wiki/Juuzou_Kadokura",3,[]string{"Responsável, agregador e esportivo"},[]string{"Integrar os colegas","Desenvolver desempenho esportivo"},[]string{"Organização de equipe; não garante obediência"},[]string{"Dados físicos não calibrados"}},
 {"chisato-hashiratani","Chisato Hashiratani","https://tomodachi-game.fandom.com/wiki/Chisato_Hashiratani",1,[]string{"Versátil, esportivo e socialmente cauteloso"},[]string{"Construir relações sociais","Explorar aptidões pessoais"},[]string{"Experiência esportiva; sem medidas calibradas"},[]string{"Ano original conflitante em fonte secundária; ingresso alternativo"}},
}

// Independent of hidden intelligence or later canonical victories. The
// operator's placement prior is capacity-balanced among available seats.
// Existing canon classmates are never displaced; year-one cohort is reduced.
func admitExternalStudents(j *Journal)error{
 entries:=append([]ExternalAdmission{},externalAdmissions...)
 sort.Slice(entries,func(a,b int)bool{return entries[a].Slug<entries[b].Slug})
 counts:=map[int]map[string]int{}
 for y:=1;y<=3;y++{counts[y]=map[string]int{"A":0,"B":0,"C":0,"D":0}}
 for _,p:=range j.Profiles{a:=j.Population[p.ID];if a.Role=="student"&&a.Year>=1&&a.Year<=3{counts[a.Year][p.Class]++}}
 for i,e:=range entries{
  id:="tg-"+e.Slug
  if known(j.Profiles,id){return fmt.Errorf("duplicate external actor %s",id)}
  class:="";min:=41
  for k:=0;k<4;k++{c:=string(rune('A'+(k+i)%4));n:=counts[e.Year][c];if n<40&&n<min{min=n;class=c}}
  if class==""{return fmt.Errorf("external admission capacity exhausted")}
  counts[e.Year][class]++
  j.Profiles=append(j.Profiles,Profile{ID:id,Name:e.Name,Class:class,Traits:clone(e.Traits),Goals:clone(e.Goals),Background:[]string{}})
  j.Population[id]=PopulationMember{Role:"student",Year:e.Year,InitialTier:LightTier,Source:"data/external-worlds/tomodachi-game/consolidated/characters/"+e.Slug+".json"}
 }
 return validateJournalProfiles(*j)
}
func applyExternalIdentity(j Journal,id string,ident *CharacterIdentity){
 if !strings.HasPrefix(id,"tg-"){return}
 for _,e:=range externalAdmissions{if id!="tg-"+e.Slug{continue}
  ident.Capacities=clone(e.Capacities);ident.Vulnerabilities=clone(e.Limits)
  ident.Status="PROVISIONAL CROSSOVER: admission before game outcomes; grade placement is scenario policy, not canon."
  // Explicitly pregame relationship only; no postgame relationship is imported.
  friends:=[]string{"yuuichi-katagiri","tenji-mikasa","shiho-sawaragi","makoto-shibe","yutori-kokorogi"}
  if has(friends,e.Slug){for _,peer:=range friends{if peer!=e.Slug&&known(j.Profiles,"tg-"+peer){ident.Background=append(ident.Background,BackgroundFact{ID:"bio-"+id+"-"+peer,Text:"Conheço "+externalName(peer)+" do meu círculo de amizades anterior ao ingresso nesta ANHS alternativa.",Source:tgSource+"; CROSSOVER_ADMISSION_ASSUMPTION",Person:"tg-"+peer})}}}
  return
 }
}
func externalName(slug string)string{for _,e:=range externalAdmissions{if e.Slug==slug{return e.Name}};return slug}

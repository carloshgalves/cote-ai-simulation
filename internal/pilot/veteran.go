package pilot

import "fmt"

// A veteran's elapsed tenure follows the admission scenario, not a later
// canonical episode. No grades, trust scores or invented past exam results.
func seedVeteranIdentity(j Journal,id string,ident *CharacterIdentity){
 a:=j.Population[id];if a.Role!="student"||a.Year<2{return}
 ident.Background=append(ident.Background,BackgroundFact{ID:"bio-veteran-tenure-"+id,Text:fmt.Sprintf("Estou no %dº ano. Já frequentei esta escola por pelo menos um ano letivo; conheço os espaços comuns, horários, uso dos pontos e regra de classificação. Os ingressantes estão chegando agora.",a.Year),Source:"SCENARIO_ASSUMPTION: population year and institutional tenure"})
 if id=="manabu-horikita"{
  ident.Background=append(ident.Background,BackgroundFact{ID:"bio-manabu-council",Text:"Exerço a presidência do conselho estudantil no início deste ano letivo.",Source:"y1v1.case.m.club-fair-silence; y1v1.ev.031; admission office inference, provisional"})
 }
}
func initializeVeteranState(j Journal,s *State){
 for _,id:=range ids(j.Profiles){
  a:=j.Population[id];if a.Role!="student"||a.Year<2{continue}
  m:=s.Minds[id];m.WakeReason="Início de novo ano letivo em campus conhecido";s.Minds[id]=m
 }
}
